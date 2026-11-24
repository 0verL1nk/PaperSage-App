// Package profile loads and verifies a published TeX Live profile before it is
// embedded in the server binary. Everything here is offline: it reads a profile
// directory and reports every problem it finds instead of stopping at the first.
package profile

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"os"
	"path"
	"sort"
	"strings"

	"github.com/0verL1nk/GraduateProject/server/internal/latex/texlive"
)

const (
	schemaVersion = 1
	maxLookups    = 64 << 20
)

// QualifiedResource is a lookup the school template cannot compile without.
type QualifiedResource struct {
	Kind   string
	Format string
	Name   string
	// Derived marks resources TeX Live generates at install time; they must be
	// published as derived blobs because no package archive contains them.
	Derived bool
}

// RequiredResources is the offline counterpart of the deploy probe: the same
// seven files, checked before a profile is allowed into a build.
var RequiredResources = []QualifiedResource{
	{Kind: "xetex", Format: "26", Name: "kvoptions.sty"},
	{Kind: "xetex", Format: "26", Name: "geometry.sty"},
	{Kind: "xetex", Format: "26", Name: "ifxetex.sty"},
	{Kind: "xetex", Format: "26", Name: "ccaption.sty"},
	{Kind: "xetex", Format: "26", Name: "layouts.sty"},
	{Kind: "xetex", Format: "26", Name: "gbt7714.sty"},
	// First-run format generation asks for this file by exact name; it is not in
	// the xetex/26 suffix list, so it guards the exact-name resolution path.
	{Kind: "xetex", Format: "26", Name: "xelatex.ini"},
	// TeX Live 2026 generates no hyphen.cfg at install time; kpathsea resolves
	// it to babel's copy, so it is served through a lookup override rather than
	// a derived blob.
	{Kind: "xetex", Format: "26", Name: "hyphen.cfg"},
}

type Profile struct {
	SchemaVersion int    `json:"schema_version"`
	Profile       string `json:"profile"`
	Repository    string `json:"repository"`
	// Mirror is an equally-trusted source for the same pinned bytes. It exists
	// because the snapshot host gates automated clients; every archive still
	// has to match the profile's checksum.
	Mirror           string                             `json:"mirror,omitempty"`
	Tlpdb            TlpdbArtifact                      `json:"tlpdb"`
	Lookups          LookupsArtifact                    `json:"lookups"`
	FormatSuffixes   []FormatSuffix                     `json:"format_suffixes"`
	LicenseOverrides map[string]texlive.LicenseOverride `json:"license_overrides"`
}

type TlpdbArtifact struct {
	Path   string `json:"path"`
	SHA512 string `json:"sha512"`
	Size   int64  `json:"size"`
}

type LookupsArtifact struct {
	SHA256  string `json:"sha256"`
	Entries int    `json:"entries"`
}

type FormatSuffix struct {
	Kind     string   `json:"kind"`
	Format   string   `json:"format"`
	Suffixes []string `json:"suffixes"`
}

// Lookup is one exact resolution the filename index cannot express: a font
// alias, an ambiguous basename override, or a generated file.
type Lookup struct {
	Kind    string   `json:"k"`
	Format  string   `json:"f"`
	Name    string   `json:"n"`
	Source  string   `json:"p,omitempty"`
	Derived *Derived `json:"d,omitempty"`
}

type Derived struct {
	SHA256    string `json:"sha256"`
	Size      int64  `json:"size"`
	MediaType string `json:"media_type"`
}

// Bundle is a loaded profile plus everything verification needs in memory.
type Bundle struct {
	Profile Profile
	Lookups map[string]Lookup
	Index   map[string][]texlive.TexliveProvenance
	files   fs.FS
}

// Load reads and structurally validates one profile directory.
func Load(root string) (*Bundle, error) {
	return LoadFS(os.DirFS(root))
}

// LoadFS reads and structurally validates a profile from any filesystem, such as
// the copy embedded in the server binary. Failures that make verification
// impossible are returned as an error; content mismatches are reported by Verify
// so an operator sees every problem at once.
func LoadFS(fsys fs.FS) (*Bundle, error) {
	raw, err := fs.ReadFile(fsys, "profile.json")
	if err != nil {
		return nil, err
	}
	var profile Profile
	if err := json.Unmarshal(raw, &profile); err != nil {
		return nil, fmt.Errorf("decode profile.json: %w", err)
	}
	if profile.SchemaVersion != schemaVersion {
		return nil, fmt.Errorf("profile.json schema_version %d is not %d", profile.SchemaVersion, schemaVersion)
	}
	if profile.Profile == "" || profile.Repository == "" {
		return nil, errors.New("profile.json is missing profile or repository")
	}
	if profile.Mirror != "" {
		parsed, parseErr := url.Parse(profile.Mirror)
		if parseErr != nil || parsed.Scheme != "https" || parsed.Host == "" {
			return nil, errors.New("profile.json mirror must be an HTTPS URL")
		}
	}
	if !fs.ValidPath(profile.Tlpdb.Path) {
		return nil, errors.New("profile.json tlpdb path is not a relative path")
	}
	tlpdb, err := fs.ReadFile(fsys, profile.Tlpdb.Path)
	if err != nil {
		return nil, err
	}
	digest := sha512.Sum512(tlpdb)
	if hex.EncodeToString(digest[:]) != profile.Tlpdb.SHA512 || int64(len(tlpdb)) != profile.Tlpdb.Size {
		return nil, errors.New("tlpdb bytes do not match profile.json")
	}
	database, err := texlive.ParseTlpdbXZ(tlpdb)
	if err != nil {
		return nil, err
	}
	index, _ := texlive.BuildTexliveIndex(database, profile.LicenseOverrides)
	lookups, err := loadLookups(fsys, "lookups.json.gz", profile.Lookups.SHA256)
	if err != nil {
		return nil, err
	}
	return &Bundle{Profile: profile, Lookups: lookups, Index: index, files: fsys}, nil
}

func loadLookups(fsys fs.FS, name, expectedSHA256 string) (map[string]Lookup, error) {
	compressed, err := fs.ReadFile(fsys, name)
	if err != nil {
		return nil, err
	}
	digest := sha256.Sum256(compressed)
	if hex.EncodeToString(digest[:]) != expectedSHA256 {
		return nil, errors.New("lookups.json.gz does not match profile.json")
	}
	reader, err := gzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		return nil, fmt.Errorf("open lookups.json.gz: %w", err)
	}
	defer func() { _ = reader.Close() }()
	var entries []Lookup
	if err := json.NewDecoder(io.LimitReader(reader, maxLookups)).Decode(&entries); err != nil {
		return nil, fmt.Errorf("decode lookups.json.gz: %w", err)
	}
	lookups := make(map[string]Lookup, len(entries))
	for _, entry := range entries {
		key := lookupKey(entry)
		if _, exists := lookups[key]; exists {
			return nil, fmt.Errorf("duplicate lookup %s/%s/%s", entry.Kind, entry.Format, entry.Name)
		}
		lookups[key] = entry
	}
	return lookups, nil
}

func lookupKey(entry Lookup) string {
	return entry.Kind + "\x00" + entry.Format + "\x00" + entry.Name
}

// Lookup returns the exact resolution recorded for one request.
func (b *Bundle) Lookup(kind, format, name string) (Lookup, bool) {
	entry, ok := b.Lookups[lookupKey(Lookup{Kind: kind, Format: format, Name: name})]
	return entry, ok
}

// ReadDerived returns one generated blob by its content hash; the caller
// verifies the bytes before serving them.
func (b *Bundle) ReadDerived(sha256Hex string) ([]byte, error) {
	return fs.ReadFile(b.files, path.Join("derived", sha256Hex))
}

// Verify reports every problem that would keep this profile from serving the
// required resources. An empty result means the profile is publishable.
func (b *Bundle) Verify() []string {
	var findings []string
	if len(b.Profile.FormatSuffixes) == 0 {
		findings = append(findings, "profile.json has no format_suffixes")
	}
	if b.Profile.Lookups.Entries != len(b.Lookups) {
		findings = append(findings, fmt.Sprintf("lookups.json.gz has %d entries, profile.json declares %d", len(b.Lookups), b.Profile.Lookups.Entries))
	}
	for _, entry := range b.Lookups {
		if entry.Derived == nil {
			continue
		}
		if finding := b.verifyDerived(entry); finding != "" {
			findings = append(findings, finding)
		}
	}
	for _, required := range RequiredResources {
		if finding := b.verifyRequired(required); finding != "" {
			findings = append(findings, finding)
		}
	}
	sort.Strings(findings)
	return findings
}

func (b *Bundle) verifyDerived(entry Lookup) string {
	data, err := fs.ReadFile(b.files, path.Join("derived", entry.Derived.SHA256))
	if err != nil {
		return fmt.Sprintf("%s/%s/%s: derived blob is unreadable", entry.Kind, entry.Format, entry.Name)
	}
	digest := sha256.Sum256(data)
	if hex.EncodeToString(digest[:]) != entry.Derived.SHA256 || int64(len(data)) != entry.Derived.Size {
		return fmt.Sprintf("%s/%s/%s: derived blob does not match its recorded hash", entry.Kind, entry.Format, entry.Name)
	}
	return ""
}

func (b *Bundle) verifyRequired(required QualifiedResource) string {
	label := required.Kind + "/" + required.Format + "/" + required.Name
	lookup, hasLookup := b.Lookups[lookupKey(Lookup{Kind: required.Kind, Format: required.Format, Name: required.Name})]
	if required.Derived {
		if !hasLookup || lookup.Derived == nil {
			return label + ": must be published as a derived resource"
		}
		return ""
	}
	if hasLookup && lookup.Derived != nil {
		return ""
	}
	// Ambiguous basenames need the lookup override; the runtime 409s otherwise.
	if _, ok := b.Resolve(required); !ok {
		return label + ": does not resolve to a single source in this profile"
	}
	return ""
}

// Resolve returns the provenance a lookup resolves to, mirroring the runtime
// order: an explicit source path first, then the unique filename index entry.
func (b *Bundle) Resolve(entry QualifiedResource) (texlive.TexliveProvenance, bool) {
	if lookup, ok := b.Lookups[lookupKey(Lookup{Kind: entry.Kind, Format: entry.Format, Name: entry.Name})]; ok && lookup.Source != "" {
		for _, record := range b.Index[baseName(lookup.Source)] {
			if record.SourcePath == lookup.Source {
				return record, true
			}
		}
		return texlive.TexliveProvenance{}, false
	}
	records := b.Index[entry.Name]
	if len(records) != 1 {
		return texlive.TexliveProvenance{}, false
	}
	return records[0], true
}

func baseName(path string) string {
	if index := strings.LastIndex(path, "/"); index >= 0 {
		return path[index+1:]
	}
	return path
}
