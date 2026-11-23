package texlive

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/ulikunitz/xz"
)

// The tlpdb is the single source of truth for both the published profile and
// the runtime index; parsing it in one place keeps the two from drifting.
const maxTlpdbBytes = int64(64 << 20)

var tlpdbChecksumPattern = regexp.MustCompile(`^[a-f0-9]{128}$`)
var tlpdbCataloguePattern = regexp.MustCompile(`^[A-Za-z0-9._+-]{1,128}$`)

// TexliveProvenance locates one file inside a TeX Live package archive.
type TexliveProvenance struct {
	SourcePath      string
	Package         string
	PackageRevision string
	ContainerSize   int64
	ContainerSHA512 string
	License         string
	LicenseSource   string
}

// TexlivePackage is one archive-backed stanza of the tlpdb.
type TexlivePackage struct {
	Name            string
	Revision        string
	Catalogue       string
	License         string
	ContainerSize   int64
	ContainerSHA512 string
	Files           []string
}

// TexliveDatabase holds the archive-backed packages of one tlpdb snapshot.
type TexliveDatabase struct {
	Packages map[string]*TexlivePackage
	// Files maps a path relative to texmf-dist to its owning package.
	Files map[string]string
}

// ParseTlpdbXZ decompresses and parses a tlpdb snapshot.
func ParseTlpdbXZ(data []byte) (*TexliveDatabase, error) {
	reader, err := (xz.ReaderConfig{DictCap: 32 << 20, SingleStream: true}).NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("open tlpdb: %w", err)
	}
	limited := &io.LimitedReader{R: reader, N: maxTlpdbBytes + 1}
	database, err := ParseTlpdb(limited)
	if err != nil {
		return nil, err
	}
	if limited.N <= 0 {
		return nil, errors.New("tlpdb exceeds the size limit")
	}
	return database, nil
}

// ParseTlpdb reads a plain-text tlpdb. Stanzas without archive metadata (the
// installer and collection stanzas) are skipped: they cannot be fetched from
// archive/<name>.tar.xz and must not claim files from a real package.
func ParseTlpdb(reader io.Reader) (*TexliveDatabase, error) {
	database := &TexliveDatabase{
		Packages: make(map[string]*TexlivePackage),
		Files:    make(map[string]string),
	}
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	var current *TexlivePackage
	var files []string
	inRunfiles := false
	finish := func() error {
		if current == nil || current.Name == "" {
			return nil
		}
		if current.ContainerSize == 0 && current.ContainerSHA512 == "" {
			return nil
		}
		if current.ContainerSize == 0 || current.ContainerSHA512 == "" {
			return fmt.Errorf("incomplete package archive metadata for %s", current.Name)
		}
		if _, exists := database.Packages[current.Name]; exists {
			return fmt.Errorf("duplicate tlpdb package %s", current.Name)
		}
		for _, name := range files {
			normalized := strings.TrimPrefix(strings.TrimPrefix(name, "RELOC/"), "texmf-dist/")
			if owner, exists := database.Files[normalized]; exists && owner != current.Name {
				return fmt.Errorf("ambiguous package provenance for %s: %s and %s", normalized, owner, current.Name)
			}
			database.Files[normalized] = current.Name
			current.Files = append(current.Files, normalized)
		}
		database.Packages[current.Name] = current
		return nil
	}
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			if err := finish(); err != nil {
				return nil, err
			}
			current, files, inRunfiles = nil, nil, false
			continue
		}
		if line[0] == ' ' || line[0] == '\t' {
			if inRunfiles {
				files = append(files, strings.TrimSpace(line))
			}
			continue
		}
		if current == nil {
			current = &TexlivePackage{}
		}
		inRunfiles = false
		key, value, _ := strings.Cut(line, " ")
		value = strings.TrimSpace(value)
		switch key {
		case "name":
			current.Name = value
		case "revision":
			current.Revision = value
		case "catalogue":
			current.Catalogue = value
		case "catalogue-license":
			current.License = value
		case "containersize":
			size, err := strconv.ParseInt(value, 10, 64)
			if err != nil || size <= 0 {
				return nil, fmt.Errorf("invalid package archive size for %s", current.Name)
			}
			current.ContainerSize = size
		case "containerchecksum":
			if !tlpdbChecksumPattern.MatchString(value) {
				return nil, fmt.Errorf("invalid package archive checksum for %s", current.Name)
			}
			current.ContainerSHA512 = value
		case "runfiles":
			inRunfiles = true
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read tlpdb: %w", err)
	}
	if err := finish(); err != nil {
		return nil, err
	}
	return database, nil
}

// BuildTexliveIndex maps every basename (and its lowercase alias) to the
// packages that ship it. Packages with no declared license and no override are
// excluded and reported, mirroring the published index policy.
func BuildTexliveIndex(database *TexliveDatabase, licenseOverrides map[string]LicenseOverride) (map[string][]TexliveProvenance, map[string][]string) {
	paths := make([]string, 0, len(database.Files))
	for path := range database.Files {
		if strings.HasPrefix(path, "tlpkg/") {
			continue
		}
		paths = append(paths, path)
	}
	sort.Strings(paths)
	index := make(map[string][]TexliveProvenance, len(paths))
	excluded := make(map[string][]string)
	for _, path := range paths {
		pkg := database.Packages[database.Files[path]]
		license, source := pkg.License, ""
		if license == "" {
			override, ok := licenseOverrides[pkg.Name]
			if !ok {
				excluded[pkg.Name] = append(excluded[pkg.Name], path)
				continue
			}
			license, source = override.License, override.Source
		} else if tlpdbCataloguePattern.MatchString(pkg.Catalogue) {
			source = fmt.Sprintf("https://ctan.org/pkg/%s?lang=en", pkg.Catalogue)
		} else {
			source = "https://www.tug.org/texlive/copying.html"
		}
		record := TexliveProvenance{
			SourcePath:      "texmf-dist/" + path,
			Package:         pkg.Name,
			PackageRevision: pkg.Revision,
			ContainerSize:   pkg.ContainerSize,
			ContainerSHA512: pkg.ContainerSHA512,
			License:         license,
			LicenseSource:   source,
		}
		name := path[strings.LastIndex(path, "/")+1:]
		// Both spellings of a mixed-case name must resolve; exact duplicates
		// are collapsed, records from the same package are not.
		keys := []string{name}
		if lower := strings.ToLower(name); lower != name {
			keys = append(keys, lower)
		}
		for _, key := range keys {
			index[key] = append(index[key], record)
		}
	}
	return index, excluded
}

// AmbiguousBasenames returns the sorted filenames claimed by more than one
// package; each needs an explicit kpsewhich override in the published profile.
func AmbiguousBasenames(index map[string][]TexliveProvenance) []string {
	var ambiguous []string
	for name, records := range index {
		if len(records) > 1 {
			ambiguous = append(ambiguous, name)
		}
	}
	sort.Strings(ambiguous)
	return ambiguous
}

// LicenseOverride supplies a license for a package whose tlpdb stanza has none.
type LicenseOverride struct {
	License string
	Source  string
}

// LoadLicenseOverrides reads the shared license override file. A source must be
// an HTTPS URL so every published license keeps a citable origin.
func LoadLicenseOverrides(path string) (map[string]LicenseOverride, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var document map[string]struct {
		License string `json:"license"`
		Source  string `json:"source"`
	}
	if err := json.Unmarshal(data, &document); err != nil {
		return nil, fmt.Errorf("decode license overrides: %w", err)
	}
	overrides := make(map[string]LicenseOverride, len(document))
	for name, entry := range document {
		source, parseErr := url.Parse(entry.Source)
		if name == "" || strings.TrimSpace(entry.License) == "" || parseErr != nil ||
			source.Scheme != "https" || source.Host == "" {
			return nil, fmt.Errorf("invalid license override for %q", name)
		}
		overrides[name] = LicenseOverride{License: strings.TrimSpace(entry.License), Source: entry.Source}
	}
	return overrides, nil
}
