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
	_, err = writer.Write([]byte(document))
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	require.NoError(t, os.WriteFile(filepath.Join(root, "tlpdb.xz"), compressed.Bytes(), 0o600))

	derived := []byte("% generated hyphen.cfg")
	derivedDigest := sha256.Sum256(derived)
	derivedSHA := hex.EncodeToString(derivedDigest[:])
	require.NoError(t, os.MkdirAll(filepath.Join(root, "derived"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(root, "derived", derivedSHA), derived, 0o600))

	lookups := []Lookup{{
		Kind: "xetex", Format: "26", Name: "hyphen.cfg",
		Derived: &Derived{SHA256: derivedSHA, Size: int64(len(derived)), MediaType: "text/plain"},
	}}
	encoded, err := json.Marshal(lookups)
	require.NoError(t, err)
	var lookupsGzip bytes.Buffer
	gzipWriter := gzip.NewWriter(&lookupsGzip)
	_, err = gzipWriter.Write(encoded)
	require.NoError(t, err)
	require.NoError(t, gzipWriter.Close())
	require.NoError(t, os.WriteFile(filepath.Join(root, "lookups.json.gz"), lookupsGzip.Bytes(), 0o600))

	tlpdbDigest := sha512.Sum512(compressed.Bytes())
	lookupsDigest := sha256.Sum256(lookupsGzip.Bytes())
	manifest := Profile{
		SchemaVersion: schemaVersion,
		Profile:       "tl2026-test",
		Repository:    "https://example.test/tlnet",
		Tlpdb:         TlpdbArtifact{Path: "tlpdb.xz", SHA512: hex.EncodeToString(tlpdbDigest[:]), Size: int64(compressed.Len())},
		Lookups:       LookupsArtifact{SHA256: hex.EncodeToString(lookupsDigest[:]), Entries: len(lookups)},
		FormatSuffixes: []FormatSuffix{
			{Kind: "xetex", Format: "26", Suffixes: []string{".tex", ".sty"}},
		},
	}
	raw, err := json.Marshal(manifest)
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
}

func TestVerifyReportsMissingRequiredResources(t *testing.T) {
	root := writeProfile(t)
	var document Profile
	raw, err := os.ReadFile(filepath.Join(root, "profile.json"))
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(raw, &document))

	var compressed bytes.Buffer
	writer, err := xz.NewWriter(&compressed)
	require.NoError(t, err)
	_, err = writer.Write([]byte(tlpdbStanza("kvoptions", "tex/latex/kvoptions/kvoptions.sty")))
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	require.NoError(t, os.WriteFile(filepath.Join(root, "tlpdb.xz"), compressed.Bytes(), 0o600))
	digest := sha512.Sum512(compressed.Bytes())
	document.Tlpdb = TlpdbArtifact{Path: "tlpdb.xz", SHA512: hex.EncodeToString(digest[:]), Size: int64(compressed.Len())}
	raw, err = json.Marshal(document)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(root, "profile.json"), raw, 0o600))

	bundle, err := Load(root)
	require.NoError(t, err)
	findings := bundle.Verify()
	require.Len(t, findings, 5, strings.Join(findings, "\n"))
	require.Contains(t, strings.Join(findings, "\n"), "geometry.sty: not present in the tlpdb index")
}

func TestVerifyRequiresHyphenCfgToBeDerived(t *testing.T) {
	root := writeProfile(t)
	var lookupsGzip bytes.Buffer
	writer := gzip.NewWriter(&lookupsGzip)
	_, err := writer.Write([]byte("[]"))
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	require.NoError(t, os.WriteFile(filepath.Join(root, "lookups.json.gz"), lookupsGzip.Bytes(), 0o600))

	var document Profile
	raw, err := os.ReadFile(filepath.Join(root, "profile.json"))
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(raw, &document))
	digest := sha256.Sum256(lookupsGzip.Bytes())
	document.Lookups = LookupsArtifact{SHA256: hex.EncodeToString(digest[:]), Entries: 0}
	raw, err = json.Marshal(document)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(root, "profile.json"), raw, 0o600))

	bundle, err := Load(root)
	require.NoError(t, err)
	require.Contains(t, strings.Join(bundle.Verify(), "\n"), "hyphen.cfg: must be published as a derived resource")
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

func TestLoadRejectsTamperedArtifacts(t *testing.T) {
	root := writeProfile(t)
	require.NoError(t, os.WriteFile(filepath.Join(root, "tlpdb.xz"), []byte("not the pinned bytes"), 0o600))
	_, err := Load(root)
	require.ErrorContains(t, err, "tlpdb bytes do not match")
}
