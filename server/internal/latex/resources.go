package latex

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"path"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"

	"github.com/0verL1nk/GraduateProject/server/internal/storage"
)

const (
	resourceCatalogSchema   = 3
	maxResourceCatalog      = 64 * 1024 * 1024
	maxResourcePackageIndex = 128 * 1024 * 1024
	maxResourceBytes        = 128 * 1024 * 1024
)

var resourceProfilePattern = regexp.MustCompile(`^[A-Za-z0-9._-]{1,96}$`)
var resourceHashPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)
var resourceGeneratorPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._+-]{0,127}$`)
var resourceFormatPattern = regexp.MustCompile(`^[A-Za-z0-9._+-]+(?:/[A-Za-z0-9._+-]+)*$`)
var fontVariantPattern = regexp.MustCompile(`^[A-Za-z0-9._-]{1,32}$`)

type resourceLookup struct {
	Kind   string `json:"kind"`
	Format string `json:"format"`
	Name   string `json:"name"`
}

type resourceCatalogEntry struct {
	Lookup resourceLookup `json:"lookup"`
	SHA256 string         `json:"sha256"`
}

type resourceCatalogDerivedGenerator struct {
	Name             string `json:"name"`
	Version          string `json:"version"`
	ExecutableSHA256 string `json:"executable_sha256"`
}

type resourceCatalogDerivedInput struct {
	SourcePath      string `json:"source_path"`
	SHA256          string `json:"sha256"`
	Package         string `json:"package"`
	PackageRevision string `json:"package_revision"`
	ContainerSize   int64  `json:"container_size"`
	ContainerSHA512 string `json:"container_checksum"`
	License         string `json:"license"`
	LicenseSource   string `json:"license_source"`
}

type resourceCatalogDerived struct {
	Lookup    resourceLookup                  `json:"lookup"`
	SHA256    string                          `json:"sha256"`
	Size      int64                           `json:"size"`
	MediaType string                          `json:"media_type"`
	ObjectKey string                          `json:"object_key"`
	Generator resourceCatalogDerivedGenerator `json:"generator"`
	Inputs    []resourceCatalogDerivedInput   `json:"inputs"`
}

type resourceCatalogProvenance struct {
	SourcePath      string `json:"source_path"`
	Package         string `json:"package"`
	PackageRevision string `json:"package_revision"`
	ContainerSize   int64  `json:"container_size"`
	ContainerSHA512 string `json:"container_checksum"`
	License         string `json:"license"`
	LicenseSource   string `json:"license_source"`
}

type resourceCatalogObject struct {
	SHA256     string                      `json:"sha256"`
	Size       int64                       `json:"size"`
	MediaType  string                      `json:"media_type"`
	Provenance []resourceCatalogProvenance `json:"provenance"`
}

type resourceCatalogDocument struct {
	SchemaVersion       int                      `json:"schema_version"`
	Profile             string                   `json:"profile"`
	Revision            string                   `json:"revision"`
	Objects             []resourceCatalogObject  `json:"objects"`
	Entries             []resourceCatalogEntry   `json:"entries"`
	DerivedResources    []resourceCatalogDerived `json:"derived_resources"`
	FormatSuffixes      []resourceCatalogFormat  `json:"format_suffixes"`
	PackageIndexGzip    string                   `json:"package_index_gzip"`
	PackageIndexSHA256  string                   `json:"package_index_sha256"`
	PackageIndexEntries int                      `json:"package_index_entries"`
}

type resourceCatalogFormat struct {
	Kind     string   `json:"kind"`
	Format   string   `json:"format"`
	Suffixes []string `json:"suffixes"`
}

type resourcePackageIndexRecord struct {
	SourcePath      string `json:"p"`
	Package         string `json:"n"`
	PackageRevision string `json:"r"`
	ContainerSize   int64  `json:"s"`
	ContainerSHA512 string `json:"h"`
	License         string `json:"l"`
	LicenseSource   string `json:"u"`
}

type resourceCatalog struct {
	profile        string
	revision       string
	entries        map[string]resourceCatalogEntry
	derived        map[string]resourceCatalogDerived
	objects        map[string]resourceCatalogObject
	packageIndex   map[string][]resourceCatalogProvenance
	packageMaps    map[string][]resourceCatalogProvenance
	formatSuffixes map[string][]string
}

type resourceService struct {
	storage    storage.Storage
	profile    string
	repository string
	archives   *packageArchiveCache

	mu      sync.Mutex
	catalog *resourceCatalog
}

func newResourceService(stg storage.Storage, profile, repository string) (*resourceService, error) {
	if stg == nil {
		return nil, fmt.Errorf("LaTeX resource storage is not configured")
	}
	if !resourceProfilePattern.MatchString(profile) {
		return nil, fmt.Errorf("invalid LaTeX resource profile")
	}
	if err := validateResourceRepository(repository); err != nil {
		return nil, err
	}
	return &resourceService{
		storage: stg, profile: profile, repository: repository,
		archives: newPackageArchiveCache(64 << 20),
	}, nil
}

func resourceLookupKey(lookup resourceLookup) string {
	return lookup.Kind + "\x00" + lookup.Format + "\x00" + lookup.Name
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

func (s *resourceService) loadCatalog(ctx context.Context) (*resourceCatalog, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.catalog != nil {
		return s.catalog, nil
	}
	key := "latex/texlive/" + s.profile + "/catalog.json"
	data, err := s.storage.Get(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("load LaTeX resource catalog %q: %w", key, err)
	}
	if len(data) == 0 || len(data) > maxResourceCatalog {
		return nil, fmt.Errorf("LaTeX resource catalog size is invalid")
	}
	decoded, err := decodeStoredResourceCatalog(data)
	if err != nil {
		return nil, fmt.Errorf("decode LaTeX resource catalog storage: %w", err)
	}
	var document resourceCatalogDocument
	if err := json.Unmarshal(decoded, &document); err != nil {
		return nil, fmt.Errorf("decode LaTeX resource catalog: %w", err)
	}
	if document.SchemaVersion != resourceCatalogSchema || document.Profile != s.profile || document.Revision != s.profile || len(document.Entries) == 0 || len(document.Objects) == 0 {
		return nil, fmt.Errorf("LaTeX resource catalog profile or schema does not match the requested profile")
	}
	entries := make(map[string]resourceCatalogEntry, len(document.Entries))
	derived := make(map[string]resourceCatalogDerived, len(document.DerivedResources))
	objects := make(map[string]resourceCatalogObject, len(document.Objects))
	for _, object := range document.Objects {
		_, _, mediaTypeErr := mime.ParseMediaType(object.MediaType)
		if !resourceHashPattern.MatchString(object.SHA256) || object.Size <= 0 || object.Size > maxResourceBytes || mediaTypeErr != nil || len(object.Provenance) == 0 {
			return nil, fmt.Errorf("LaTeX resource catalog contains an invalid object")
		}
		for _, provenance := range object.Provenance {
			if !validCatalogSourcePath(provenance.SourcePath) || !validResourcePackage(provenance.Package) || provenance.PackageRevision == "" || provenance.ContainerSize <= 0 || provenance.ContainerSize > maxResourceArchiveBytes || !resourceArchiveHashPattern.MatchString(provenance.ContainerSHA512) || strings.TrimSpace(provenance.License) == "" {
				return nil, fmt.Errorf("LaTeX resource catalog contains invalid object provenance")
			}
		}
		if _, exists := objects[object.SHA256]; exists {
			return nil, fmt.Errorf("LaTeX resource catalog contains a duplicate object")
		}
		objects[object.SHA256] = object
	}
	for _, entry := range document.Entries {
		if !validResourceLookup(entry.Lookup) || !resourceHashPattern.MatchString(entry.SHA256) {
			return nil, fmt.Errorf("LaTeX resource catalog contains an invalid entry")
		}
		if _, exists := objects[entry.SHA256]; !exists {
			return nil, fmt.Errorf("LaTeX resource catalog entry references an unknown object")
		}
		key := resourceLookupKey(entry.Lookup)
		if _, exists := entries[key]; exists {
			return nil, fmt.Errorf("LaTeX resource catalog contains a duplicate lookup")
		}
		entries[key] = entry
	}
	for _, resource := range document.DerivedResources {
		_, _, mediaTypeErr := mime.ParseMediaType(resource.MediaType)
		if !validResourceLookup(resource.Lookup) || !resourceHashPattern.MatchString(resource.SHA256) || resource.Size <= 0 || resource.Size > maxResourceBytes || mediaTypeErr != nil || resource.ObjectKey != "derived/"+resource.SHA256 {
			return nil, fmt.Errorf("LaTeX resource catalog contains an invalid derived resource")
		}
		generator := resource.Generator
		if !resourceGeneratorPattern.MatchString(generator.Name) || strings.TrimSpace(generator.Version) == "" || len(generator.Version) > 256 || !resourceHashPattern.MatchString(generator.ExecutableSHA256) || len(resource.Inputs) == 0 {
			return nil, fmt.Errorf("LaTeX resource catalog contains invalid derived resource provenance")
		}
		for _, input := range resource.Inputs {
			if !validCatalogSourcePath(input.SourcePath) || !resourceHashPattern.MatchString(input.SHA256) || !validResourcePackage(input.Package) || input.PackageRevision == "" || input.ContainerSize <= 0 || input.ContainerSize > maxResourceArchiveBytes || !resourceArchiveHashPattern.MatchString(input.ContainerSHA512) || strings.TrimSpace(input.License) == "" || !strings.HasPrefix(input.LicenseSource, "https://") {
				return nil, fmt.Errorf("LaTeX resource catalog contains invalid derived input provenance")
			}
		}
		key := resourceLookupKey(resource.Lookup)
		if _, exists := entries[key]; exists {
			return nil, fmt.Errorf("LaTeX resource catalog has conflicting package and derived lookups")
		}
		if _, exists := derived[key]; exists {
			return nil, fmt.Errorf("LaTeX resource catalog contains a duplicate derived lookup")
		}
		derived[key] = resource
	}
	formatSuffixes := make(map[string][]string)
	for _, format := range document.FormatSuffixes {
		if (format.Kind != "xetex" && format.Kind != "pdftex" && format.Kind != "dvipdfmx") || !resourceFormatPattern.MatchString(format.Format) || len(format.Suffixes) == 0 {
			return nil, fmt.Errorf("LaTeX resource catalog contains invalid Kpathsea format metadata")
		}
		key := format.Kind + "\x00" + format.Format
		if _, exists := formatSuffixes[key]; exists {
			return nil, fmt.Errorf("LaTeX resource catalog contains duplicate Kpathsea format metadata")
		}
		seen := make(map[string]struct{}, len(format.Suffixes))
		for _, suffix := range format.Suffixes {
			if len(suffix) < 2 || len(suffix) > 32 || suffix[0] != '.' || strings.ContainsAny(suffix, `/\\`) {
				return nil, fmt.Errorf("LaTeX resource catalog contains invalid Kpathsea suffix")
			}
			suffix = strings.ToLower(suffix)
			if _, exists := seen[suffix]; exists {
				continue
			}
			seen[suffix] = struct{}{}
			formatSuffixes[key] = append(formatSuffixes[key], suffix)
		}
	}
	if len(document.FormatSuffixes) == 0 {
		for formatID, suffixes := range legacyKpathseaSuffixes {
			for _, kind := range []string{"xetex", "pdftex", "dvipdfmx"} {
				key := kind + "\x00" + formatID
				for _, suffix := range suffixes {
					formatSuffixes[key] = append(formatSuffixes[key], strings.ToLower(suffix))
				}
			}
		}
	}
	for _, entry := range document.Entries {
		if entry.Lookup.Kind == "font" {
			continue
		}
		suffix := strings.ToLower(path.Ext(entry.Lookup.Name))
		if suffix == "" {
			continue
		}
		key := entry.Lookup.Kind + "\x00" + entry.Lookup.Format
		if _, exists := formatSuffixes[key]; !exists {
			formatSuffixes[key] = append(formatSuffixes[key], suffix)
		} else if len(document.FormatSuffixes) == 0 {
			if !containsString(formatSuffixes[key], suffix) {
				formatSuffixes[key] = append(formatSuffixes[key], suffix)
			}
		}
	}
	if len(document.FormatSuffixes) == 0 {
		for key := range formatSuffixes {
			formatID := strings.SplitN(key, "\x00", 2)[1]
			if _, hasKpathseaOrder := legacyKpathseaSuffixes[formatID]; !hasKpathseaOrder {
				sort.Strings(formatSuffixes[key])
			}
		}
	}
	packageIndex, packageMaps, err := decodeResourcePackageIndex(document)
	if err != nil {
		return nil, err
	}
	loaded := &resourceCatalog{
		profile: document.Profile, revision: document.Revision, entries: entries, objects: objects,
		derived: derived, packageIndex: packageIndex, packageMaps: packageMaps, formatSuffixes: formatSuffixes,
	}
	s.catalog = loaded
	return loaded, nil
}

func decodeStoredResourceCatalog(data []byte) ([]byte, error) {
	if len(data) < 2 || data[0] != 0x1f || data[1] != 0x8b {
		return data, nil
	}
	reader, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	decoded, err := io.ReadAll(io.LimitReader(reader, maxResourceCatalog+1))
	if err != nil {
		_ = reader.Close()
		return nil, err
	}
	if err := reader.Close(); err != nil {
		return nil, fmt.Errorf("close compressed LaTeX resource catalog: %w", err)
	}
	if len(decoded) == 0 || len(decoded) > maxResourceCatalog {
		return nil, fmt.Errorf("compressed catalog exceeds supported size")
	}
	return decoded, nil
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func decodeResourcePackageIndex(document resourceCatalogDocument) (map[string][]resourceCatalogProvenance, map[string][]resourceCatalogProvenance, error) {
	if document.PackageIndexGzip == "" {
		if document.PackageIndexSHA256 != "" || document.PackageIndexEntries != 0 {
			return nil, nil, fmt.Errorf("LaTeX resource package index metadata is incomplete")
		}
		return nil, nil, nil
	}
	compressed, err := base64.StdEncoding.DecodeString(document.PackageIndexGzip)
	if err != nil || len(compressed) == 0 || len(compressed) > 8<<20 {
		return nil, nil, fmt.Errorf("LaTeX resource package index encoding is invalid")
	}
	digest := sha256.Sum256(compressed)
	if hex.EncodeToString(digest[:]) != document.PackageIndexSHA256 {
		return nil, nil, fmt.Errorf("LaTeX resource package index checksum is invalid")
	}
	reader, err := gzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		return nil, nil, fmt.Errorf("open LaTeX resource package index: %w", err)
	}
	data, err := io.ReadAll(io.LimitReader(reader, maxResourcePackageIndex+1))
	closeErr := reader.Close()
	if err != nil || len(data) == 0 || len(data) > maxResourcePackageIndex {
		return nil, nil, fmt.Errorf("LaTeX resource package index size is invalid")
	}
	if closeErr != nil {
		return nil, nil, fmt.Errorf("close compressed LaTeX resource package index: %w", closeErr)
	}
	var compact map[string][]resourcePackageIndexRecord
	if err := json.Unmarshal(data, &compact); err != nil {
		return nil, nil, fmt.Errorf("decode LaTeX resource package index: %w", err)
	}
	index := make(map[string][]resourceCatalogProvenance, len(compact))
	packageMaps := make(map[string][]resourceCatalogProvenance)
	seenMaps := make(map[string]struct{})
	count := 0
	for name, records := range compact {
		if name == "" || len(name) > 255 || strings.ContainsAny(name, `/\\`) || len(records) == 0 {
			return nil, nil, fmt.Errorf("LaTeX resource package index contains an invalid filename")
		}
		for _, record := range records {
			if !validCatalogSourcePath(record.SourcePath) || !validResourcePackage(record.Package) || record.PackageRevision == "" || record.ContainerSize <= 0 || record.ContainerSize > maxResourceArchiveBytes || !resourceArchiveHashPattern.MatchString(record.ContainerSHA512) || strings.TrimSpace(record.License) == "" || !strings.HasPrefix(record.LicenseSource, "https://") {
				return nil, nil, fmt.Errorf("LaTeX resource package index contains invalid provenance for %q", name)
			}
			provenance := resourceCatalogProvenance(record)
			index[name] = append(index[name], provenance)
			if strings.EqualFold(path.Ext(provenance.SourcePath), ".map") {
				mapKey := provenance.Package + "\x00" + provenance.SourcePath
				if _, exists := seenMaps[mapKey]; !exists {
					seenMaps[mapKey] = struct{}{}
					packageMaps[provenance.Package] = append(packageMaps[provenance.Package], provenance)
				}
			}
			count++
		}
	}
	if document.PackageIndexEntries != count {
		return nil, nil, fmt.Errorf("LaTeX resource package index entry count does not match its contents")
	}
	return index, packageMaps, nil
}

func validCatalogSourcePath(sourcePath string) bool {
	return sourcePath != "" && len(sourcePath) <= 1024 && !path.IsAbs(sourcePath) && path.Clean(sourcePath) == sourcePath && !strings.Contains(sourcePath, `\`) && sourcePath != "." && !strings.HasPrefix(sourcePath, "../")
}

func (s *resourceService) get(c *gin.Context) {
	// TeX resources are public immutable bytes. Keep CORS independent of the
	// request Origin so CDN cache entries populated by HEAD/health probes remain
	// readable by browsers on every PaperSage frontend origin.
	c.Header("Access-Control-Allow-Origin", "*")
	c.Header("Access-Control-Expose-Headers", "fileid, fontid, X-PaperSage-Package, X-PaperSage-Generated-Resource, X-PaperSage-Resource-Revision, ETag, Content-Length")
	if c.Param("profile") != s.profile {
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
	catalog, err := s.loadCatalog(c.Request.Context())
	if err != nil {
		c.Header("Cache-Control", "no-store")
		c.String(http.StatusServiceUnavailable, "LaTeX resource catalog is unavailable: %v", err)
		return
	}
	entry, ok := catalog.entries[resourceLookupKey(lookup)]
	if !ok {
		if resource, generated := catalog.derived[resourceLookupKey(lookup)]; generated {
			s.serveDerivedResource(c, catalog, resource)
			return
		}
		s.serveIndexedPackageResource(c, catalog, lookup)
		return
	}
	object := catalog.objects[entry.SHA256]
	contentType := object.MediaType
	c.Header("Content-Type", contentType)
	c.Header("Content-Length", fmt.Sprint(object.Size))
	c.Header("Cache-Control", "public, max-age=31536000, immutable")
	c.Header("ETag", `"`+object.SHA256+`"`)
	c.Header("X-PaperSage-Resource-Revision", catalog.revision)
	if lookup.Kind == "font" {
		c.Header("fontid", object.SHA256)
	} else {
		c.Header("fileid", object.SHA256)
	}
	if len(object.Provenance) > 0 {
		c.Header("X-PaperSage-Package", object.Provenance[0].Package)
	}
	if c.Request.Method == http.MethodHead {
		c.Status(http.StatusOK)
		return
	}
	var data []byte
	var fetchErr error
	for _, provenance := range object.Provenance {
		data, fetchErr = s.archives.resource(c.Request.Context(), s.repository, provenance, object)
		if fetchErr == nil {
			break
		}
	}
	if fetchErr != nil {
		c.Header("Cache-Control", "no-store")
		c.String(http.StatusBadGateway, "LaTeX resource could not be fetched and verified: %v", fetchErr)
		return
	}
	c.Header("Content-Length", fmt.Sprint(len(data)))
	c.Data(http.StatusOK, contentType, data)
}

func (s *resourceService) serveDerivedResource(c *gin.Context, catalog *resourceCatalog, resource resourceCatalogDerived) {
	key := "latex/texlive/" + catalog.profile + "/" + resource.ObjectKey
	if c.Request.Method == http.MethodHead {
		setDerivedResourceHeaders(c, catalog, resource)
		c.Status(http.StatusOK)
		return
	}
	data, err := s.storage.Get(c.Request.Context(), key)
	if err != nil || int64(len(data)) != resource.Size {
		c.Header("Cache-Control", "no-store")
		c.String(http.StatusBadGateway, "generated TeX resource is unavailable or has an invalid size")
		return
	}
	digest := sha256.Sum256(data)
	if hex.EncodeToString(digest[:]) != resource.SHA256 {
		c.Header("Cache-Control", "no-store")
		c.String(http.StatusBadGateway, "generated TeX resource failed its content hash check")
		return
	}
	setDerivedResourceHeaders(c, catalog, resource)
	c.Data(http.StatusOK, resource.MediaType, data)
}

func setDerivedResourceHeaders(c *gin.Context, catalog *resourceCatalog, resource resourceCatalogDerived) {
	c.Header("Content-Type", resource.MediaType)
	c.Header("Content-Length", fmt.Sprint(resource.Size))
	c.Header("Cache-Control", "public, max-age=31536000, immutable")
	c.Header("ETag", `"`+resource.SHA256+`"`)
	if resource.Lookup.Kind == "font" {
		c.Header("fontid", resource.SHA256)
	} else {
		c.Header("fileid", resource.SHA256)
	}
	c.Header("X-PaperSage-Resource-Revision", catalog.revision)
	c.Header("X-PaperSage-Generated-Resource", "true")
}

func (s *resourceService) serveIndexedPackageResource(c *gin.Context, catalog *resourceCatalog, lookup resourceLookup) {
	if catalog.packageIndex == nil {
		c.Header("Cache-Control", "public, max-age=300")
		c.String(http.StatusNotFound, "TeX Live package filename index is unavailable for profile %q", catalog.profile)
		return
	}
	suffix := strings.ToLower(path.Ext(lookup.Name))
	if lookup.Kind == "font" {
		switch lookup.Format {
		case "OT":
			suffix = ".otf"
		case "TT":
			suffix = path.Ext(lookup.Name)
			if suffix != ".ttf" && suffix != ".ttc" && suffix != ".dfont" {
				c.Header("Cache-Control", "public, max-age=300")
				c.String(http.StatusNotFound, "font resource %q is not indexed for font format %q", lookup.Name, lookup.Format)
				return
			}
		default:
			c.Header("Cache-Control", "public, max-age=300")
			c.String(http.StatusNotFound, "font format %q is unsupported", lookup.Format)
			return
		}
	}
	formatSuffixes := catalog.formatSuffixes[lookup.Kind+"\x00"+lookup.Format]
	if lookup.Kind != "font" && len(formatSuffixes) == 0 {
		c.Header("Cache-Control", "public, max-age=300")
		if strings.HasPrefix(lookup.Format, "pk/") {
			c.String(http.StatusNotFound, "TeX Live generated font format %q requires local Metafont generation and is not a static package resource", lookup.Format)
		} else {
			c.String(http.StatusNotFound, "TeX filename format %q is unsupported for %s", lookup.Format, lookup.Kind)
		}
		return
	}
	lookupSuffixes := []string{suffix}
	if lookup.Kind != "font" && suffix == "" {
		lookupSuffixes = formatSuffixes
	}
	var filtered []resourceCatalogProvenance
	for _, candidateSuffix := range lookupSuffixes {
		if lookup.Kind != "font" && suffix != "" && !containsString(formatSuffixes, suffix) && !isGraphicsResourceSuffix(lookup.Kind, suffix) {
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
		provenance := catalog.packageIndex[indexName]
		for _, candidate := range provenance {
			if isKpathseaPackageSourcePath(candidate.SourcePath) && strings.EqualFold(path.Ext(candidate.SourcePath), candidateSuffix) {
				filtered = append(filtered, candidate)
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
	data, err := s.archives.packageFile(c.Request.Context(), s.repository, filtered[0])
	if err != nil {
		c.Header("Cache-Control", "no-store")
		c.String(http.StatusBadGateway, "TeX Live resource %q could not be fetched from package %q: %v", lookup.Name, filtered[0].Package, err)
		return
	}
	digest := sha256.Sum256(data)
	hash := hex.EncodeToString(digest[:])
	mediaType := mime.TypeByExtension(suffix)
	if mediaType == "" {
		mediaType = "application/octet-stream"
	}
	c.Header("Content-Type", mediaType)
	c.Header("Content-Length", fmt.Sprint(len(data)))
	c.Header("Cache-Control", "public, max-age=31536000, immutable")
	c.Header("ETag", `"`+hash+`"`)
	c.Header("X-PaperSage-Resource-Revision", catalog.revision)
	c.Header("X-PaperSage-Package", filtered[0].Package)
	if lookup.Kind == "font" {
		c.Header("fontid", hash)
	} else {
		c.Header("fileid", hash)
	}
	if c.Request.Method == http.MethodHead {
		c.Status(http.StatusOK)
		return
	}
	c.Data(http.StatusOK, mediaType, data)
}

// Graphics files are discovered through TeX's generic input search path, so
// their extensions do not belong to the Kpathsea format suffix list for the
// request (often format 26, whose native suffixes are .tex/.sty/.cls). Keep a
// small engine-level allowlist for image formats that the browser compiler can
// actually consume; package provenance still has to match the exact filename.
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

type resourcePackageMapRequest struct {
	Packages []string         `json:"packages"`
	Lookups  []resourceLookup `json:"lookups"`
}

func (s *resourceService) getPackageMaps(c *gin.Context) {
	c.Header("Access-Control-Allow-Origin", "*")
	c.Header("Access-Control-Expose-Headers", "fileid, X-PaperSage-Resource-Revision, ETag, Content-Length")
	if c.Param("profile") != s.profile {
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
	catalog, err := s.loadCatalog(c.Request.Context())
	if err != nil {
		c.Header("Cache-Control", "no-store")
		c.String(http.StatusServiceUnavailable, "LaTeX resource catalog is unavailable: %v", err)
		return
	}
	for _, lookup := range request.Lookups {
		if !validResourceLookup(lookup) || lookup.Kind == "font" {
			c.String(http.StatusBadRequest, "invalid TeX resource lookup in font map request")
			return
		}
		if entry, exists := catalog.entries[resourceLookupKey(lookup)]; exists {
			for _, provenance := range catalog.objects[entry.SHA256].Provenance {
				if !addPackage(provenance.Package) {
					c.String(http.StatusInternalServerError, "catalog contains an invalid font map package reference")
					return
				}
			}
			continue
		}
		suffix := strings.ToLower(path.Ext(lookup.Name))
		suffixes := []string{suffix}
		if suffix == "" {
			suffixes = catalog.formatSuffixes[lookup.Kind+"\x00"+lookup.Format]
		}
		for _, candidateSuffix := range suffixes {
			indexName := lookup.Name
			if suffix == "" {
				indexName += candidateSuffix
			}
			var matched []resourceCatalogProvenance
			for _, provenance := range catalog.packageIndex[indexName] {
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
					c.String(http.StatusInternalServerError, "catalog contains an invalid font map package reference")
					return
				}
				break
			}
		}
	}
	var merged bytes.Buffer
	for _, packageName := range packages {
		mapFiles := catalog.packageMaps[packageName]
		sort.Slice(mapFiles, func(i, j int) bool { return mapFiles[i].SourcePath < mapFiles[j].SourcePath })
		for _, provenance := range mapFiles {
			content, fetchErr := s.archives.packageFile(c.Request.Context(), s.repository, provenance)
			if fetchErr != nil {
				c.Header("Cache-Control", "no-store")
				c.String(http.StatusBadGateway, "font maps from package %q could not be fetched: %v", packageName, fetchErr)
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
	c.Header("X-PaperSage-Resource-Revision", catalog.revision)
	c.Data(http.StatusOK, "text/plain; charset=utf-8", merged.Bytes())
}

// MountResources exposes immutable TeX Live bytes through an exact catalog
// lookup. It contains no TeX runtime and never compiles project input.
func MountResources(engine *gin.Engine, stg storage.Storage, profile, repository string) error {
	service, err := newResourceService(stg, profile, repository)
	if err != nil {
		return err
	}
	engine.GET("/texlive/:profile/resolve/:kind/:format/*name", service.get)
	engine.HEAD("/texlive/:profile/resolve/:kind/:format/*name", service.get)
	engine.GET("/texlive/:profile/resolve/:kind", service.get)
	engine.HEAD("/texlive/:profile/resolve/:kind", service.get)
	engine.GET("/texlive/:profile/package-maps", service.getPackageMaps)
	engine.POST("/texlive/:profile/package-maps", service.getPackageMaps)
	return nil
}
