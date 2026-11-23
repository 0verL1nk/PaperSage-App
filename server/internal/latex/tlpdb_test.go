package latex

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/ulikunitz/xz"
)

var (
	tlpdbChecksumA = strings.Repeat("a", 128)
	tlpdbChecksumB = strings.Repeat("b", 128)
)

func tlpdbStanza(name string, size int, checksum string, files ...string) string {
	lines := []string{
		"name " + name,
		"category Package",
		"revision 12345",
		"catalogue " + name,
		"catalogue-license LPPL-1.3c",
		fmt.Sprintf("containersize %d", size),
		"containerchecksum " + checksum,
		"runfiles",
	}
	for _, file := range files {
		lines = append(lines, " "+file)
	}
	return strings.Join(lines, "\n") + "\n\n"
}

func TestParseTlpdbNormalizesPackageFilePaths(t *testing.T) {
	document := tlpdbStanza("alpha", 42, tlpdbChecksumA,
		"texmf-dist/tex/latex/alpha/alpha.sty",
		"RELOC/texmf-dist/fonts/opentype/public/alpha/Alpha.otf")
	document += tlpdbStanza("beta", 7, tlpdbChecksumB, "tex/latex/beta/beta.sty")

	database, err := ParseTlpdb(strings.NewReader(document))
	require.NoError(t, err)
	require.Len(t, database.Packages, 2)
	require.Equal(t, int64(42), database.Packages["alpha"].ContainerSize)
	require.Equal(t, tlpdbChecksumA, database.Packages["alpha"].ContainerSHA512)
	require.Equal(t, "alpha", database.Files["tex/latex/alpha/alpha.sty"])
	require.Equal(t, "alpha", database.Files["fonts/opentype/public/alpha/Alpha.otf"])
	require.Equal(t, "beta", database.Files["tex/latex/beta/beta.sty"])
}

func TestParseTlpdbSkipsStanzasWithoutArchiveMetadata(t *testing.T) {
	document := "name 00texlive.installer\ncategory TLCore\nrunfiles\n texmf-dist/scripts/texlive/tlmgr.pl\n\n"
	document += tlpdbStanza("alpha", 42, tlpdbChecksumA, "tex/latex/alpha/alpha.sty")

	database, err := ParseTlpdb(strings.NewReader(document))
	require.NoError(t, err)
	require.NotContains(t, database.Packages, "00texlive.installer")
	require.NotContains(t, database.Files, "scripts/texlive/tlmgr.pl")
	require.Len(t, database.Files, 1)
}

func TestParseTlpdbRejectsCorruptArchiveMetadata(t *testing.T) {
	cases := map[string]string{
		"half metadata":    "name alpha\ncontainersize 42\nrunfiles\n tex/latex/alpha/alpha.sty\n\n",
		"invalid size":     "name alpha\ncontainersize 0\ncontainerchecksum " + tlpdbChecksumA + "\n\n",
		"invalid checksum": "name alpha\ncontainersize 42\ncontainerchecksum not-a-checksum\n\n",
		"duplicate package": tlpdbStanza("alpha", 42, tlpdbChecksumA) +
			tlpdbStanza("alpha", 42, tlpdbChecksumA),
		"conflicting file": tlpdbStanza("alpha", 42, tlpdbChecksumA, "tex/latex/shared/shared.sty") +
			tlpdbStanza("beta", 7, tlpdbChecksumB, "tex/latex/shared/shared.sty"),
	}
	for name, document := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := ParseTlpdb(strings.NewReader(document))
			require.Error(t, err)
		})
	}
}

func TestBuildTexliveIndexAppliesLicensePolicy(t *testing.T) {
	document := tlpdbStanza("alpha", 42, tlpdbChecksumA,
		"tex/latex/alpha/Alpha.sty",
		"tex/latex/alpha/alpha.sty",
		"tlpkg/alpha/ignored.txt")
	document += strings.Replace(tlpdbStanza("beta", 7, tlpdbChecksumB, "tex/latex/beta/beta.sty"),
		"catalogue-license LPPL-1.3c\n", "", 1)
	document += strings.Replace(tlpdbStanza("gamma", 9, tlpdbChecksumB, "tex/latex/gamma/gamma.sty"),
		"catalogue-license LPPL-1.3c\n", "", 1)
	database, err := ParseTlpdb(strings.NewReader(document))
	require.NoError(t, err)

	index, excluded := BuildTexliveIndex(database, map[string]LicenseOverride{
		"gamma": {License: "LPPL-1.3c", Source: "https://example.test/gamma"},
	})
	require.NotContains(t, index, "ignored.txt")
	require.Contains(t, index, "Alpha.sty")
	require.Contains(t, index, "alpha.sty", "mixed-case names must also resolve lowercase")
	require.Len(t, index["alpha.sty"], 2, "both files of one package stay separate records")
	require.Equal(t, "https://ctan.org/pkg/alpha?lang=en", index["alpha.sty"][0].LicenseSource)
	require.Equal(t, "LPPL-1.3c", index["gamma.sty"][0].License)
	require.Equal(t, "https://example.test/gamma", index["gamma.sty"][0].LicenseSource)
	require.Equal(t, "texmf-dist/tex/latex/alpha/Alpha.sty", index["Alpha.sty"][0].SourcePath)
	require.NotContains(t, index, "beta.sty")
	require.Equal(t, []string{"tex/latex/beta/beta.sty"}, excluded["beta"])
}

func TestAmbiguousBasenamesListsOnlySharedNames(t *testing.T) {
	document := tlpdbStanza("alpha", 42, tlpdbChecksumA, "tex/latex/alpha/shared.sty", "tex/latex/alpha/only.sty")
	document += tlpdbStanza("beta", 7, tlpdbChecksumB, "tex/latex/beta/shared.sty")
	database, err := ParseTlpdb(strings.NewReader(document))
	require.NoError(t, err)
	index, _ := BuildTexliveIndex(database, nil)
	require.Equal(t, []string{"shared.sty"}, AmbiguousBasenames(index))
}

func TestParseTlpdbXZReadsCompressedSnapshots(t *testing.T) {
	var compressed bytes.Buffer
	writer, err := xz.NewWriter(&compressed)
	require.NoError(t, err)
	_, err = writer.Write([]byte(tlpdbStanza("alpha", 42, tlpdbChecksumA, "tex/latex/alpha/alpha.sty")))
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	database, err := ParseTlpdbXZ(compressed.Bytes())
	require.NoError(t, err)
	require.Equal(t, int64(42), database.Packages["alpha"].ContainerSize)
}
