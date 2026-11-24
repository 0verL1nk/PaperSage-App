package profile

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/ulikunitz/xz"
)

const testChecksum = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func tlpdbStanza(name string, files ...string) string {
	lines := []string{
		"name " + name,
		"revision 12345",
		"catalogue " + name,
		"catalogue-license LPPL-1.3c",
		"containersize 42",
		"containerchecksum " + testChecksum,
		"runfiles",
	}
	for _, file := range files {
		lines = append(lines, " "+file)
	}
	return strings.Join(lines, "\n") + "\n\n"
}

func sha512Hex(data []byte) string {
	digest := sha512.Sum512(data)
	return hex.EncodeToString(digest[:])
}

func sha256Hex(data []byte) string {
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

func lookupsFor(derivedSHA string, derivedSize int64) []Lookup {
	return []Lookup{
		{Kind: "xetex", Format: "26", Name: "hyphen.cfg", Source: "texmf-dist/tex/generic/babel/hyphen.cfg"},
		{Kind: "xetex", Format: "26", Name: "kanjix.map",
			Derived: &Derived{SHA256: derivedSHA, Size: derivedSize, MediaType: "text/plain"}},
	}
}

func writeProfile(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	var compressed bytes.Buffer
	writer, err := xz.NewWriter(&compressed)
	require.NoError(t, err)
	document := ""
	for _, name := range []string{"kvoptions", "geometry", "ccaption", "layouts", "gbt7714"} {
		document += tlpdbStanza(name, "tex/latex/"+name+"/"+name+".sty")
	}
	document += tlpdbStanza("iftex", "tex/latex/iftex/iftex.sty", "tex/latex/iftex/ifxetex.sty")
	// Two packages ship a hyphen.cfg, so the basename is ambiguous and needs a
	// kpsewhich override, exactly like the real snapshot.
	document += tlpdbStanza("texinifiles", "tex/xetex/config/xelatex.ini")
	document += tlpdbStanza("hyphenafrikaans", "tex/generic/hyph-utf8/loadhyph/loadhyph-af.tex")
	document += tlpdbStanza("babel", "tex/generic/babel/hyphen.cfg")
	document += tlpdbStanza("cslatex", "tex/cslatex/base/hyphen.cfg")
	_, err = writer.Write([]byte(document))
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	require.NoError(t, os.WriteFile(filepath.Join(root, "tlpdb.xz"), compressed.Bytes(), 0o600))

	derived := []byte("% generated kanjix.map")
	derivedSHA := sha256Hex(derived)
	require.NoError(t, os.MkdirAll(filepath.Join(root, "derived"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(root, "derived", derivedSHA), derived, 0o600))

	lookups := lookupsFor(derivedSHA, int64(len(derived)))
	encoded, err := json.Marshal(lookups)
	require.NoError(t, err)
	var lookupsGzip bytes.Buffer
	gzipWriter := gzip.NewWriter(&lookupsGzip)
	_, err = gzipWriter.Write(encoded)
	require.NoError(t, err)
	require.NoError(t, gzipWriter.Close())
	require.NoError(t, os.WriteFile(filepath.Join(root, "lookups.json.gz"), lookupsGzip.Bytes(), 0o600))

	profile := Profile{
		SchemaVersion: schemaVersion,
		Profile:       "tl2026-test",
		Repository:    "https://example.test/tlnet",
		Tlpdb:         TlpdbArtifact{Path: "tlpdb.xz", SHA512: sha512Hex(compressed.Bytes()), Size: int64(compressed.Len())},
		Lookups:       LookupsArtifact{SHA256: sha256Hex(lookupsGzip.Bytes()), Entries: len(lookups)},
		FormatSuffixes: []FormatSuffix{
			{Kind: "xetex", Format: "26", Suffixes: []string{".tex", ".sty"}},
		},
	}
	raw, err := json.Marshal(profile)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(root, "profile.json"), raw, 0o600))
	return root
}

func TestVerifyAcceptsACompleteProfile(t *testing.T) {
	bundle, err := Load(writeProfile(t))
	require.NoError(t, err)
	require.Empty(t, bundle.Verify())

	provenance, ok := bundle.Resolve(QualifiedResource{Kind: "xetex", Format: "26", Name: "ifxetex.sty"})
	require.True(t, ok)
	require.Equal(t, "iftex", provenance.Package)
	require.Equal(t, "texmf-dist/tex/latex/iftex/ifxetex.sty", provenance.SourcePath)

	hyphen, ok := bundle.Resolve(QualifiedResource{Kind: "xetex", Format: "26", Name: "hyphen.cfg"})
	require.True(t, ok)
	require.Equal(t, "texmf-dist/tex/generic/babel/hyphen.cfg", hyphen.SourcePath)
}

func TestVerifyReportsMissingRequiredResources(t *testing.T) {
	root := writeProfile(t)
	var profile Profile
	raw, err := os.ReadFile(filepath.Join(root, "profile.json"))
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(raw, &profile))

	var compressed bytes.Buffer
	writer, err := xz.NewWriter(&compressed)
	require.NoError(t, err)
	_, err = writer.Write([]byte(tlpdbStanza("kvoptions", "tex/latex/kvoptions/kvoptions.sty")))
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	require.NoError(t, os.WriteFile(filepath.Join(root, "tlpdb.xz"), compressed.Bytes(), 0o600))
	profile.Tlpdb = TlpdbArtifact{Path: "tlpdb.xz", SHA512: sha512Hex(compressed.Bytes()), Size: int64(compressed.Len())}
	raw, err = json.Marshal(profile)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(root, "profile.json"), raw, 0o600))

	bundle, err := Load(root)
	require.NoError(t, err)
	findings := bundle.Verify()
	require.Len(t, findings, 8, strings.Join(findings, "\n"))
	joined := strings.Join(findings, "\n")
	require.Contains(t, joined, "geometry.sty: does not resolve to a single source")
	require.Contains(t, joined, "hyphen.cfg: does not resolve to a single source")
}

func TestVerifyRequiresAnOverrideForAmbiguousHyphenCfg(t *testing.T) {
	root := writeProfile(t)
	entries, err := json.Marshal([]Lookup{{
		Kind: "xetex", Format: "26", Name: "kanjix.map",
		Derived: &Derived{SHA256: strings.Repeat("b", 64), Size: 1, MediaType: "text/plain"},
	}})
	require.NoError(t, err)
	var lookupsGzip bytes.Buffer
	writer := gzip.NewWriter(&lookupsGzip)
	_, err = writer.Write(entries)
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	require.NoError(t, os.WriteFile(filepath.Join(root, "lookups.json.gz"), lookupsGzip.Bytes(), 0o600))

	var profile Profile
	raw, err := os.ReadFile(filepath.Join(root, "profile.json"))
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(raw, &profile))
	profile.Lookups = LookupsArtifact{SHA256: sha256Hex(lookupsGzip.Bytes()), Entries: 1}
	raw, err = json.Marshal(profile)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(root, "profile.json"), raw, 0o600))

	bundle, err := Load(root)
	require.NoError(t, err)
	require.Contains(t, strings.Join(bundle.Verify(), "\n"), "hyphen.cfg: does not resolve to a single source")
}

func TestVerifyReportsCorruptDerivedBlob(t *testing.T) {
	root := writeProfile(t)
	entries, err := os.ReadDir(filepath.Join(root, "derived"))
	require.NoError(t, err)
	require.Len(t, entries, 1)
	require.NoError(t, os.WriteFile(filepath.Join(root, "derived", entries[0].Name()), []byte("tampered"), 0o600))

	bundle, err := Load(root)
	require.NoError(t, err)
	require.Contains(t, strings.Join(bundle.Verify(), "\n"), "derived blob does not match its recorded hash")
}

func TestEmbeddedProfileVerifiesAllRequiredResources(t *testing.T) {
	bundle, err := Embedded()
	require.NoError(t, err)
	require.Equal(t, "tl2026-tlnet-20261122", bundle.Profile.Profile)
	require.Empty(t, bundle.Verify(), "the committed profile must keep all seven template resources resolvable")
}

func TestLoadRejectsTamperedArtifacts(t *testing.T) {
	root := writeProfile(t)
	require.NoError(t, os.WriteFile(filepath.Join(root, "tlpdb.xz"), []byte("not the pinned bytes"), 0o600))
	_, err := Load(root)
	require.ErrorContains(t, err, "tlpdb bytes do not match")
}
