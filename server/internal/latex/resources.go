package latex

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"path"
	"regexp"
	"sort"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/0verL1nk/GraduateProject/server/internal/latex/profile"
	"github.com/0verL1nk/GraduateProject/server/internal/latex/texlive"
)

const maxResourceBytes = 128 * 1024 * 1024

var resourceProfilePattern = regexp.MustCompile(`^[A-Za-z0-9._-]{1,96}$`)
var resourceFormatPattern = regexp.MustCompile(`^[A-Za-z0-9._+-]+(?:/[A-Za-z0-9._+-]+)*$`)
var fontVariantPattern = regexp.MustCompile(`^[A-Za-z0-9._-]{1,32}$`)

type resourceLookup struct {
	Kind   string `json:"kind"`
	Format string `json:"format"`
	Name   string `json:"name"`
}

// resourceService serves immutable TeX Live bytes from the embedded profile:
// exact lookups first, then the filename index, with verified bytes persisted in
// a local disk cache. It has no object-storage dependency.
type resourceService struct {
	bundle       *profile.Bundle
	cache        *DiskCache
	archives     *packageArchiveCache
	repositories []string
	profileID    string

	formatSuffixes map[string][]string
	packageMaps    map[string][]texlive.TexliveProvenance
}

func newResourceService(bundle *profile.Bundle, cache *DiskCache) (*resourceService, error) {
	if bundle == nil {
		return nil, fmt.Errorf("LaTeX resource profile is not configured")
	}
	if cache == nil {
		return nil, fmt.Errorf("LaTeX resource cache is not configured")
	}
	if !resourceProfilePattern.MatchString(bundle.Profile.Profile) {
		return nil, fmt.Errorf("invalid LaTeX resource profile")
	}
	repositories := []string{bundle.Profile.Repository}
	if bundle.Profile.Mirror != "" {
		repositories = append(repositories, bundle.Profile.Mirror)
	}
	for _, repository := range repositories {
		if err := validateResourceRepository(repository); err != nil {
			return nil, err
		}
	}
	service := &resourceService{
		bundle: bundle, cache: cache, repositories: repositories,
		profileID: bundle.Profile.Profile, archives: newPackageArchiveCache(64 << 20),
		formatSuffixes: make(map[string][]string),
	}
	for _, format := range bundle.Profile.FormatSuffixes {
		service.formatSuffixes[format.Kind+"\x00"+format.Format] = format.Suffixes
	}
	service.packageMaps = buildResourcePackageMaps(bundle)
	return service, nil
}

func validResourceLookup(lookup resourceLookup) bool {
	if lookup.Kind == "font" {
		if !fontVariantPattern.MatchString(lookup.Format) {
			return false
		}
	} else if (lookup.Kind != "xetex" && lookup.Kind != "pdftex" && lookup.Kind != "dvipdfmx") || !validResourceFormat(lookup.Format) {
		return false
	}
	if lookup.Name == "" || len(lookup.Name) > 255 || strings.ContainsAny(lookup.Name, `/\\`) || lookup.Name == "." || lookup.Name == ".." {
		return false
	}
	return path.Base(lookup.Name) == lookup.Name
}

func validResourceFormat(value string) bool {
	if len(value) == 0 || len(value) > 96 || !resourceFormatPattern.MatchString(value) {
		return false
	}
	for _, component := range strings.Split(value, "/") {
		if component == "." || component == ".." {
			return false
		}
	}
	return true
}

func (s *resourceService) get(c *gin.Context) {
	c.Header("Access-Control-Allow-Origin", "*")
	c.Header("Access-Control-Expose-Headers", "fileid, fontid, X-PaperSage-Package, X-PaperSage-Resource-Revision, ETag, Content-Length")
	if c.Param("profile") != s.profileID {
		c.Status(http.StatusNotFound)
		return
	}
	lookup := resourceLookup{
		Kind:   c.Param("kind"),
		Format: c.Param("format"),
		Name:   strings.TrimPrefix(c.Param("name"), "/"),
	}
	if lookup.Format == "" {
		lookup.Format = c.Query("format")
		lookup.Name = c.Query("name")
	}
	if !validResourceLookup(lookup) {
		c.Status(http.StatusBadRequest)
		return
	}
	if entry, ok := s.bundle.Lookup(lookup.Kind, lookup.Format, lookup.Name); ok {
		if entry.Derived != nil {
			s.serveDerived(c, lookup, entry.Derived)
			return
		}
		if entry.Source != "" {
			provenance, found := s.provenanceForSource(entry.Source)
			if !found {
				c.Header("Cache-Control", "no-store")
				c.String(http.StatusBadGateway, "LaTeX resource %q points at %q, which this profile does not index", lookup.Name, entry.Source)
				return
			}
			s.serveProvenance(c, lookup, provenance)
			return
		}
	}
	s.serveIndexed(c, lookup)
}

// getProfile lets clients discover the served profile, so a profile upgrade
// only ships a new backend binary instead of a coordinated frontend release.
func (s *resourceService) getProfile(c *gin.Context) {
	c.Header("Access-Control-Allow-Origin", "*")
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, gin.H{"profile": s.profileID})
}

func (s *resourceService) serveDerived(c *gin.Context, lookup resourceLookup, derived *profile.Derived) {
	data, err := s.bundle.ReadDerived(derived.SHA256)
	if err != nil {
		c.Header("Cache-Control", "no-store")
		c.String(http.StatusBadGateway, "generated TeX resource %q is unavailable", lookup.Name)
		return
	}
	digest := sha256.Sum256(data)
	if int64(len(data)) != derived.Size || hex.EncodeToString(digest[:]) != derived.SHA256 {
		c.Header("Cache-Control", "no-store")
		c.String(http.StatusBadGateway, "generated TeX resource %q failed its content check", lookup.Name)
		return
	}
	s.writeResource(c, lookup, derived.MediaType, "", derived.SHA256, data)
}

func (s *resourceService) serveIndexed(c *gin.Context, lookup resourceLookup) {
	suffix := strings.ToLower(path.Ext(lookup.Name))
	// The engine asks for concrete filenames (xelatex.ini, *.bst, …). A unique
	// exact match in the profile is authoritative even when the format's suffix
	// list does not mention the extension; the list only decides lookups that
	// arrive without one.
	if suffix != "" && lookup.Kind != "font" {
		records := s.exactIndexMatches(lookup.Name, suffix)
		if len(records) == 1 {
			s.serveProvenance(c, lookup, records[0])
			return
		}
		if len(records) > 1 {
			packages := make([]string, 0, len(records))
			for _, record := range records {
				packages = append(packages, record.Package)
			}
			c.Header("Cache-Control", "no-store")
			c.String(http.StatusConflict, "ambiguous TeX Live filename %q matches multiple packages: %s", lookup.Name, strings.Join(packages, ", "))
			return
		}
	}
	formatSuffixes := s.formatSuffixes[lookup.Kind+"\x00"+lookup.Format]
	// The engine's Kpathsea enum can contain formats this profile does not
	// list. It still sends an explicit filename suffix, and the index is keyed
	// by filename, so resolve those by name instead of rejecting the request.
	unknownFormat := lookup.Kind != "font" && len(formatSuffixes) == 0
	if unknownFormat && (suffix == "" || strings.HasPrefix(lookup.Format, "pk/")) {
		c.Header("Cache-Control", "public, max-age=300")
		if strings.HasPrefix(lookup.Format, "pk/") {
			c.String(http.StatusNotFound, "TeX Live generated font format %q requires local Metafont generation and is not a static package resource", lookup.Format)
		} else {
			c.String(http.StatusNotFound, "TeX filename format %q is unsupported for %s", lookup.Format, lookup.Kind)
		}
		return
	}
	if unknownFormat {
		formatSuffixes = []string{suffix}
	}
	lookupSuffixes := []string{suffix}
	if lookup.Kind != "font" && suffix == "" {
		lookupSuffixes = formatSuffixes
	}
	var filtered []texlive.TexliveProvenance
	for _, candidateSuffix := range lookupSuffixes {
		if lookup.Kind != "font" && suffix != "" && !unknownFormat && !containsString(formatSuffixes, suffix) && !isGraphicsResourceSuffix(lookup.Kind, suffix) {
			c.Header("Cache-Control", "public, max-age=300")
			c.String(http.StatusNotFound, "TeX filename format %q does not support suffix %q", lookup.Format, suffix)
			return
		}
		indexName := lookup.Name
		if lookup.Kind == "font" {
			indexName = strings.ToLower(indexName)
		} else if suffix == "" {
			indexName += candidateSuffix
		}
		for _, provenance := range s.bundle.Index[indexName] {
			if isKpathseaPackageSourcePath(provenance.SourcePath) && strings.EqualFold(path.Ext(provenance.SourcePath), candidateSuffix) {
				filtered = append(filtered, provenance)
			}
		}
		if len(filtered) > 0 {
			break
		}
	}
	if len(filtered) == 0 {
		c.Header("Cache-Control", "public, max-age=300")
		c.String(http.StatusNotFound, "TeX Live filename %q is not present in this profile", lookup.Name)
		return
	}
	if len(filtered) != 1 {
		packages := make([]string, 0, len(filtered))
		for _, candidate := range filtered {
			packages = append(packages, candidate.Package)
		}
		c.Header("Cache-Control", "no-store")
		c.String(http.StatusConflict, "ambiguous TeX Live filename %q matches multiple packages: %s", lookup.Name, strings.Join(packages, ", "))
		return
	}
	s.serveProvenance(c, lookup, filtered[0])
}

// exactIndexMatches returns every indexed file whose basename and extension match
// the request exactly.
func (s *resourceService) exactIndexMatches(name, suffix string) []texlive.TexliveProvenance {
	var matched []texlive.TexliveProvenance
	for _, provenance := range s.bundle.Index[name] {
		if isKpathseaPackageSourcePath(provenance.SourcePath) && strings.EqualFold(path.Ext(provenance.SourcePath), suffix) {
			matched = append(matched, provenance)
		}
	}
	return matched
}

func (s *resourceService) serveProvenance(c *gin.Context, lookup resourceLookup, provenance texlive.TexliveProvenance) {
	key := diskCacheKey(s.profileID, lookup.Kind, lookup.Format, lookup.Name)
	entry, err := s.cache.LoadOrFill(key, func() (DiskCacheEntry, error) {
		data, fetchErr := s.packageFile(c.Request.Context(), provenance)
		if fetchErr != nil {
			return DiskCacheEntry{}, fetchErr
		}
		digest := sha256.Sum256(data)
		mediaType := mime.TypeByExtension(strings.ToLower(path.Ext(lookup.Name)))
		if mediaType == "" {
			mediaType = "application/octet-stream"
		}
		return DiskCacheEntry{
			Meta: DiskCacheMeta{
				Profile: s.profileID, SourcePath: provenance.SourcePath, Package: provenance.Package,
				SHA256: hex.EncodeToString(digest[:]), Size: int64(len(data)), MediaType: mediaType,
			},
			Data: data,
		}, nil
	})
	if err != nil {
		c.Header("Cache-Control", "no-store")
		c.String(http.StatusBadGateway, "TeX Live resource %q could not be fetched from package %q: %v", lookup.Name, provenance.Package, err)
		return
	}
	s.writeResource(c, lookup, entry.Meta.MediaType, entry.Meta.Package, entry.Meta.SHA256, entry.Data)
}

// packageFile tries every trusted source in order. All sources serve the same
// pinned bytes, so a fallback never weakens the checksum guarantee.
func (s *resourceService) packageFile(ctx context.Context, provenance texlive.TexliveProvenance) ([]byte, error) {
	var failures []string
	for _, repository := range s.repositories {
		data, err := s.archives.packageFile(ctx, repository, provenance)
		if err == nil {
			return data, nil
		}
		failures = append(failures, fmt.Sprintf("%s: %v", repository, err))
	}
	return nil, errors.New(strings.Join(failures, "; "))
}

func (s *resourceService) packageFiles(ctx context.Context, provenance texlive.TexliveProvenance, paths []string) (map[string][]byte, error) {
	var failures []string
	for _, repository := range s.repositories {
		content, err := s.archives.packageFiles(ctx, repository, provenance, paths)
		if err == nil {
			return content, nil
		}
		failures = append(failures, fmt.Sprintf("%s: %v", repository, err))
	}
	return nil, errors.New(strings.Join(failures, "; "))
}

func (s *resourceService) writeResource(c *gin.Context, lookup resourceLookup, mediaType, packageName, digest string, data []byte) {
	if mediaType == "" {
		mediaType = "application/octet-stream"
	}
	c.Header("Content-Type", mediaType)
	c.Header("Content-Length", fmt.Sprint(len(data)))
	c.Header("Cache-Control", "public, max-age=31536000, immutable")
	c.Header("ETag", `"`+digest+`"`)
	c.Header("X-PaperSage-Resource-Revision", s.profileID)
	if packageName != "" {
		c.Header("X-PaperSage-Package", packageName)
	}
	if lookup.Kind == "font" {
		c.Header("fontid", digest)
	} else {
		c.Header("fileid", digest)
	}
	if c.Request.Method == http.MethodHead {
		c.Status(http.StatusOK)
		return
	}
	c.Data(http.StatusOK, mediaType, data)
}

func (s *resourceService) provenanceForSource(source string) (texlive.TexliveProvenance, bool) {
	base := source
	if index := strings.LastIndex(source, "/"); index >= 0 {
		base = source[index+1:]
	}
	for _, record := range s.bundle.Index[base] {
		if record.SourcePath == source {
			return record, true
		}
	}
	return texlive.TexliveProvenance{}, false
}

// Graphics files are discovered through TeX's generic input search path, so
// their extensions do not belong to the Kpathsea format suffix list for the
// request (often format 26, whose native suffixes are .tex/.sty/.cls). Keep a
// small engine-level allowlist for image formats that the browser compiler can
// actually consume; provenance still has to match the exact filename.
func isGraphicsResourceSuffix(kind, suffix string) bool {
	if kind != "pdftex" && kind != "xetex" && kind != "dvipdfmx" {
		return false
	}
	switch strings.ToLower(suffix) {
	case ".pdf", ".png", ".jpg", ".jpeg", ".jbig2", ".jb2", ".eps", ".epsi", ".mps", ".ps":
		return true
	default:
		return false
	}
}

// TeX Live's tlpkg tree contains package-manager and Ghostscript support
// assets. Kpathsea does not search it, so those files cannot disambiguate a
// runtime filename lookup against texmf-dist resources.
func isKpathseaPackageSourcePath(sourcePath string) bool {
	clean := path.Clean(sourcePath)
	return strings.HasPrefix(clean, "texmf-dist/") && !strings.HasPrefix(clean, "texmf-dist/tlpkg/")
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

// buildResourcePackageMaps groups the profile's font map files by package. One
// archive can hold many maps; callers decompress it once instead of once per
// file (a package like `lm` ships 20 maps in a 12 MiB archive).
func buildResourcePackageMaps(bundle *profile.Bundle) map[string][]texlive.TexliveProvenance {
	maps := make(map[string][]texlive.TexliveProvenance)
	seen := make(map[string]bool)
	for _, records := range bundle.Index {
		for _, record := range records {
			if seen[record.SourcePath] || !strings.HasSuffix(record.SourcePath, ".map") {
				continue
			}
			if !strings.Contains(record.SourcePath, "/fonts/map/") {
				continue
			}
			seen[record.SourcePath] = true
			maps[record.Package] = append(maps[record.Package], record)
		}
	}
	for _, records := range maps {
		sort.Slice(records, func(i, j int) bool { return records[i].SourcePath < records[j].SourcePath })
	}
	return maps
}

type resourcePackageMapRequest struct {
	Packages []string         `json:"packages"`
	Lookups  []resourceLookup `json:"lookups"`
}

func (s *resourceService) getPackageMaps(c *gin.Context) {
	c.Header("Access-Control-Allow-Origin", "*")
	c.Header("Access-Control-Expose-Headers", "fileid, X-PaperSage-Resource-Revision, ETag, Content-Length")
	if c.Param("profile") != s.profileID {
		c.Status(http.StatusNotFound)
		return
	}
	request := resourcePackageMapRequest{}
	if c.Request.Method == http.MethodPost {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1<<20)
		decoder := json.NewDecoder(c.Request.Body)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&request); err != nil {
			c.String(http.StatusBadRequest, "invalid TeX package map request: %v", err)
			return
		}
		if len(request.Lookups) > 2048 || len(request.Packages) > 512 {
			c.String(http.StatusRequestEntityTooLarge, "TeX package map request exceeds the supported lookup limit")
			return
		}
	} else {
		requested := strings.Split(c.Query("packages"), ",")
		if len(requested) == 0 || len(requested) > 64 || requested[0] == "" {
			c.String(http.StatusBadRequest, "package map request must contain between 1 and 64 packages")
			return
		}
		request.Packages = requested
	}
	packages := make([]string, 0, len(request.Packages))
	seen := make(map[string]struct{}, len(request.Packages))
	addPackage := func(packageName string) bool {
		if !validResourcePackage(packageName) {
			return false
		}
		if _, exists := seen[packageName]; exists {
			return true
		}
		if len(packages) >= 512 {
			return false
		}
		seen[packageName] = struct{}{}
		packages = append(packages, packageName)
		return true
	}
	for _, packageName := range request.Packages {
		if !addPackage(packageName) {
			c.String(http.StatusBadRequest, "invalid TeX Live package name %q", packageName)
			return
		}
	}
	for _, lookup := range request.Lookups {
		if !validResourceLookup(lookup) || lookup.Kind == "font" {
			c.String(http.StatusBadRequest, "invalid TeX resource lookup in font map request")
			return
		}
		if entry, exists := s.bundle.Lookup(lookup.Kind, lookup.Format, lookup.Name); exists && entry.Source != "" {
			provenance, found := s.provenanceForSource(entry.Source)
			if !found {
				c.String(http.StatusInternalServerError, "profile lookup points at an unindexed source")
				return
			}
			if !addPackage(provenance.Package) {
				c.String(http.StatusInternalServerError, "profile contains an invalid font map package reference")
				return
			}
			continue
		}
		suffix := strings.ToLower(path.Ext(lookup.Name))
		suffixes := []string{suffix}
		if suffix == "" {
			suffixes = s.formatSuffixes[lookup.Kind+"\x00"+lookup.Format]
		}
		for _, candidateSuffix := range suffixes {
			indexName := lookup.Name
			if suffix == "" {
				indexName += candidateSuffix
			}
			var matched []texlive.TexliveProvenance
			for _, provenance := range s.bundle.Index[indexName] {
				if isKpathseaPackageSourcePath(provenance.SourcePath) && strings.EqualFold(path.Ext(provenance.SourcePath), candidateSuffix) {
					matched = append(matched, provenance)
				}
			}
			if len(matched) > 1 {
				c.Header("Cache-Control", "no-store")
				c.String(http.StatusConflict, "ambiguous TeX resource lookup %q in font map request", lookup.Name)
				return
			}
			if len(matched) == 1 {
				if !addPackage(matched[0].Package) {
					c.String(http.StatusInternalServerError, "profile contains an invalid font map package reference")
					return
				}
				break
			}
		}
	}
	var merged bytes.Buffer
	for _, packageName := range packages {
		mapFiles := s.packageMaps[packageName]
		if len(mapFiles) == 0 {
			continue
		}
		sameArchive := true
		paths := make([]string, 0, len(mapFiles))
		for _, provenance := range mapFiles {
			if provenance.ContainerSHA512 != mapFiles[0].ContainerSHA512 {
				sameArchive = false
			}
			paths = append(paths, provenance.SourcePath)
		}
		contents := map[string][]byte{}
		if sameArchive {
			extracted, fetchErr := s.packageFiles(c.Request.Context(), mapFiles[0], paths)
			if fetchErr != nil {
				c.Header("Cache-Control", "no-store")
				c.String(http.StatusBadGateway, "font maps from package %q could not be fetched: %v", packageName, fetchErr)
				return
			}
			contents = extracted
		}
		for _, provenance := range mapFiles {
			content := contents[provenance.SourcePath]
			if !sameArchive {
				var fetchErr error
				content, fetchErr = s.packageFile(c.Request.Context(), provenance)
				if fetchErr != nil {
					c.Header("Cache-Control", "no-store")
					c.String(http.StatusBadGateway, "font maps from package %q could not be fetched: %v", packageName, fetchErr)
					return
				}
			}
			if content == nil {
				c.Header("Cache-Control", "no-store")
				c.String(http.StatusBadGateway, "font map %q is absent from package %q", provenance.SourcePath, packageName)
				return
			}
			if int64(merged.Len())+int64(len(content))+1 > 32<<20 {
				c.Header("Cache-Control", "no-store")
				c.String(http.StatusRequestEntityTooLarge, "merged TeX font maps exceed the 32 MiB limit")
				return
			}
			if _, err := merged.Write(content); err != nil {
				c.String(http.StatusInternalServerError, "could not merge TeX font maps")
				return
			}
			if !bytes.HasSuffix(content, []byte("\n")) {
				merged.WriteByte('\n')
			}
		}
	}
	if merged.Len() == 0 {
		c.Header("Cache-Control", "public, max-age=300")
		c.Status(http.StatusNoContent)
		return
	}
	digest := sha256.Sum256(merged.Bytes())
	hash := hex.EncodeToString(digest[:])
	c.Header("Content-Type", "text/plain; charset=utf-8")
	c.Header("Content-Length", fmt.Sprint(merged.Len()))
	c.Header("Cache-Control", "public, max-age=31536000, immutable")
	c.Header("ETag", `"`+hash+`"`)
	c.Header("fileid", hash)
	c.Header("X-PaperSage-Resource-Revision", s.profileID)
	c.Data(http.StatusOK, "text/plain; charset=utf-8", merged.Bytes())
}

// MountResources exposes the profile's TeX Live bytes through exact lookups and
// the filename index. It contains no TeX runtime and never compiles input.
func MountResources(engine *gin.Engine, bundle *profile.Bundle, cache *DiskCache) error {
	service, err := newResourceService(bundle, cache)
	if err != nil {
		return err
	}
	// Static sibling of /texlive/:profile/...: clients discover the served
	// profile instead of baking it into a frontend build.
	engine.GET("/texlive/profile", service.getProfile)
	engine.GET("/texlive/:profile/resolve/:kind/:format/*name", service.get)
	engine.HEAD("/texlive/:profile/resolve/:kind/:format/*name", service.get)
	engine.GET("/texlive/:profile/resolve/:kind", service.get)
	engine.HEAD("/texlive/:profile/resolve/:kind", service.get)
	engine.GET("/texlive/:profile/package-maps", service.getPackageMaps)
	engine.POST("/texlive/:profile/package-maps", service.getPackageMaps)
	return nil
}
