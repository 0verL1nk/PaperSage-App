package latex

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// Synthetic workload: 20,000 files, 2,000 ambiguous names, 2,000 case
// aliases and 2,000 map files distributed over 200 repeated packages.
func benchmarkPackageIndexFixture(b *testing.B) resourceCatalogDocument {
	b.Helper()
	entries := make(map[string][]resourcePackageIndexRecord)
	count := 0
	for i := 0; i < 20000; i++ {
		pkg := fmt.Sprintf("package%03d", i%200)
		ext := ".sty"
		if i%10 == 0 {
			ext = ".map"
		}
		name := fmt.Sprintf("File%05d%s", i, ext)
		record := resourcePackageIndexRecord{
			SourcePath: "texmf-dist/fonts/map/" + pkg + "/" + name,
			Package:    pkg, PackageRevision: "12345", ContainerSize: 123456,
			ContainerSHA512: strings.Repeat("a", 128), License: "LPPL-1.3c",
			LicenseSource: "https://ctan.org/pkg/" + pkg,
		}
		entries[name] = []resourcePackageIndexRecord{record}
		count++
		if i%10 == 1 {
			other := record
			other.Package = "other-package"
			other.SourcePath = "texmf-dist/other/" + name
			entries[name] = append(entries[name], other)
			count++
		}
		if i%10 == 0 {
			entries[strings.ToLower(name)] = []resourcePackageIndexRecord{record}
			count++
		}
	}
	plain, err := json.Marshal(entries)
	if err != nil {
		b.Fatal(err)
	}
	var compressed bytes.Buffer
	writer := gzip.NewWriter(&compressed)
	if _, err := writer.Write(plain); err != nil {
		b.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		b.Fatal(err)
	}
	digest := sha256.Sum256(compressed.Bytes())
	b.Logf("fixture: keys=%d records=%d json_bytes=%d gzip_bytes=%d", len(entries), count, len(plain), compressed.Len())
	return resourceCatalogDocument{PackageIndexGzip: base64.StdEncoding.EncodeToString(compressed.Bytes()), PackageIndexSHA256: hex.EncodeToString(digest[:]), PackageIndexEntries: count}
}

func TestDecodePackageIndexDuplicateKeysAndIntegrity(t *testing.T) {
	record := resourcePackageIndexRecord{
		SourcePath: "texmf-dist/fonts/map/example/example.map", Package: "example", PackageRevision: "1",
		ContainerSize: 1, ContainerSHA512: strings.Repeat("a", 128), License: "LPPL", LicenseSource: "https://example.org",
	}
	encoded, err := json.Marshal([]resourcePackageIndexRecord{record})
	require.NoError(t, err)
	makeDocument := func(data []byte, count int, corrupt bool) resourceCatalogDocument {
		var compressed bytes.Buffer
		w := gzip.NewWriter(&compressed)
		_, writeErr := w.Write(data)
		require.NoError(t, writeErr)
		require.NoError(t, w.Close())
		packed := compressed.Bytes()
		if corrupt {
			packed[len(packed)-1] ^= 0xff
		}
		digest := sha256.Sum256(packed)
		return resourceCatalogDocument{PackageIndexGzip: base64.StdEncoding.EncodeToString(packed), PackageIndexSHA256: hex.EncodeToString(digest[:]), PackageIndexEntries: count}
	}
	// The last duplicate replaces the first before provenance and count validation.
	data := append([]byte(`{"example.map":[{}],"example.map":`), encoded...)
	data = append(data, '}')
	index, maps, err := decodeResourcePackageIndex(makeDocument(data, 1, false))
	require.NoError(t, err)
	require.Len(t, index["example.map"], 1)
	require.Len(t, maps["example"], 1)
	aliasData := append([]byte(`{"EXAMPLE.map":`), encoded...)
	aliasData = append(aliasData, []byte(`,"example.map":`)...)
	aliasData = append(aliasData, encoded...)
	aliasData = append(aliasData, '}')
	aliasIndex, aliasMaps, err := decodeResourcePackageIndex(makeDocument(aliasData, 2, false))
	require.NoError(t, err)
	require.Len(t, aliasIndex, 2)
	require.Len(t, aliasMaps["example"], 1)
	_, _, err = decodeResourcePackageIndex(makeDocument(data, 2, false))
	require.Error(t, err)
	_, _, err = decodeResourcePackageIndex(makeDocument(data, 1, true))
	require.Error(t, err)
	_, _, err = decodeResourcePackageIndex(makeDocument(append(data, []byte(" {}")...), 1, false))
	require.Error(t, err)
}

func TestDecodePackageIndexRejectsExpandedSizeLimit(t *testing.T) {
	var compressed bytes.Buffer
	writer := gzip.NewWriter(&compressed)
	_, err := writer.Write([]byte("{}"))
	require.NoError(t, err)
	chunk := bytes.Repeat([]byte(" "), 1<<20)
	for written := 2; written < maxResourcePackageIndex+1; {
		part := min(len(chunk), maxResourcePackageIndex+1-written)
		_, err := writer.Write(chunk[:part])
		require.NoError(t, err)
		written += part
	}
	require.NoError(t, writer.Close())
	digest := sha256.Sum256(compressed.Bytes())
	document := resourceCatalogDocument{PackageIndexGzip: base64.StdEncoding.EncodeToString(compressed.Bytes()), PackageIndexSHA256: hex.EncodeToString(digest[:])}
	_, _, err = decodeResourcePackageIndex(document)
	require.ErrorContains(t, err, "size is invalid")
}

func TestPackageMapsSortedBeforeConcurrentRequests(t *testing.T) {
	stg := newMemoryResourceStorage()
	archive := makeTestArchiveFiles(t, map[string][]byte{
		"fonts/map/example/a.map": []byte("first\n"),
		"fonts/map/example/z.map": []byte("last\n"),
	})
	makeTestCatalog(t, stg, []byte("seed"), makeTestArchive(t, []byte("seed")))
	archiveDigest := sha512.Sum512(archive)
	makeRecord := func(name string) resourcePackageIndexRecord {
		return resourcePackageIndexRecord{
			SourcePath: "texmf-dist/fonts/map/example/" + name, Package: "example",
			PackageRevision: "12345", ContainerSize: int64(len(archive)),
			ContainerSHA512: hex.EncodeToString(archiveDigest[:]), License: "LPPL",
			LicenseSource: "https://ctan.org/pkg/example",
		}
	}
	addTestPackageIndexEntries(t, stg, map[string][]resourcePackageIndexRecord{
		"maps.map": {makeRecord("z.map"), makeRecord("a.map")},
	})
	service, err := newResourceService(stg, testResourceProfile, testResourceRepository, "")
	require.NoError(t, err)
	catalog, err := service.loadCatalog(t.Context())
	require.NoError(t, err)
	require.Equal(t, "texmf-dist/fonts/map/example/a.map", catalog.packageMaps["example"][0].SourcePath)
	original := append([]resourceCatalogProvenance(nil), catalog.packageMaps["example"]...)
	key := testResourceRepository + "\x00example\x0012345\x00" + hex.EncodeToString(archiveDigest[:])
	service.archives.archives[key] = archive
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.GET("/texlive/:profile/package-maps", service.getPackageMaps)
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			response := httptest.NewRecorder()
			engine.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/texlive/"+testResourceProfile+"/package-maps?packages=example", nil))
			if response.Code != http.StatusOK || response.Body.String() != "first\nlast\n" {
				t.Errorf("unexpected response %d: %q", response.Code, response.Body.String())
			}
		}()
	}
	wg.Wait()
	require.Equal(t, original, catalog.packageMaps["example"])
}

func BenchmarkDecodeResourcePackageIndex(b *testing.B) {
	document := benchmarkPackageIndexFixture(b)
	b.ReportAllocs()
	b.ResetTimer()
	var retained uint64
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		runtime.GC()
		runtime.GC()
		var before, after runtime.MemStats
		runtime.ReadMemStats(&before)
		b.StartTimer()
		index, maps, err := decodeResourcePackageIndex(document)
		b.StopTimer()
		if err != nil {
			b.Fatal(err)
		}
		runtime.GC()
		runtime.ReadMemStats(&after)
		b.Logf("heap before=%d after=%d", before.HeapAlloc, after.HeapAlloc)
		if after.HeapAlloc > before.HeapAlloc {
			retained += after.HeapAlloc - before.HeapAlloc
		}
		runtime.KeepAlive(index)
		runtime.KeepAlive(maps)
		runtime.KeepAlive(document)
		b.StartTimer()
	}
	b.StopTimer()
	b.ReportMetric(float64(retained)/float64(b.N), "retained-B/op")
}
