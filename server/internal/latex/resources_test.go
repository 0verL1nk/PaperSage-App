package latex

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/ulikunitz/xz"
)

const testResourceProfile = "test-tl-profile"
const testResourceRepository = "https://ctan.example/systems/texlive/tlnet"

type memoryResourceStorage struct{ objects map[string][]byte }

func newMemoryResourceStorage() *memoryResourceStorage {
	return &memoryResourceStorage{objects: map[string][]byte{}}
}

func (m *memoryResourceStorage) Put(_ context.Context, key string, data []byte) error {
	m.objects[key] = append([]byte(nil), data...)
	return nil
}
func (m *memoryResourceStorage) Get(_ context.Context, key string) ([]byte, error) {
	data, ok := m.objects[key]
	if !ok {
		return nil, errors.New("object not found")
	}
	return append([]byte(nil), data...), nil
}
func (m *memoryResourceStorage) Delete(_ context.Context, key string) error {
	delete(m.objects, key)
	return nil
}
func (m *memoryResourceStorage) PresignGet(_ context.Context, key string, _ time.Duration) (string, error) {
	return "memory://" + key, nil
}
func (m *memoryResourceStorage) List(_ context.Context, prefix string) ([]string, error) {
	var keys []string
	for key := range m.objects {
		if strings.HasPrefix(key, prefix) {
			keys = append(keys, key)
		}
	}
	return keys, nil
}
func (m *memoryResourceStorage) Exists(_ context.Context, key string) bool {
	_, ok := m.objects[key]
	return ok
}

func makeTestArchive(t *testing.T, content []byte) []byte {
	return makeTestArchiveAt(t, "tex/latex/amsmath/amsmath.sty", content)
}

func makeTestArchiveAt(t *testing.T, sourcePath string, content []byte) []byte {
	t.Helper()
	return makeTestArchiveFiles(t, map[string][]byte{sourcePath: content})
}

func makeTestArchiveFiles(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	var compressed bytes.Buffer
	xzWriter, err := xz.NewWriter(&compressed)
	require.NoError(t, err)
	tarWriter := tar.NewWriter(xzWriter)
	for sourcePath, content := range files {
		require.NoError(t, tarWriter.WriteHeader(&tar.Header{
			Name: sourcePath,
			Mode: 0o644,
			Size: int64(len(content)),
		}))
		_, err = tarWriter.Write(content)
		require.NoError(t, err)
	}
	require.NoError(t, tarWriter.Close())
	require.NoError(t, xzWriter.Close())
	return compressed.Bytes()
}

func addTestPackageIndex(t *testing.T, stg *memoryResourceStorage, name string, records []resourcePackageIndexRecord) {
	t.Helper()
	addTestPackageIndexEntries(t, stg, map[string][]resourcePackageIndexRecord{name: records})
}

func addTestPackageIndexEntries(t *testing.T, stg *memoryResourceStorage, entries map[string][]resourcePackageIndexRecord) {
	t.Helper()
	key := "latex/texlive/" + testResourceProfile + "/catalog.json"
	encoded, err := stg.Get(t.Context(), key)
	require.NoError(t, err)
	var document resourceCatalogDocument
	require.NoError(t, json.Unmarshal(encoded, &document))
	index, err := json.Marshal(entries)
	require.NoError(t, err)
	var compressed bytes.Buffer
	writer := gzip.NewWriter(&compressed)
	_, err = writer.Write(index)
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	digest := sha256.Sum256(compressed.Bytes())
	document.PackageIndexGzip = base64.StdEncoding.EncodeToString(compressed.Bytes())
	document.PackageIndexSHA256 = hex.EncodeToString(digest[:])
	document.PackageIndexEntries = 0
	for _, records := range entries {
		document.PackageIndexEntries += len(records)
	}
	encoded, err = json.Marshal(document)
	require.NoError(t, err)
	require.NoError(t, stg.Put(t.Context(), key, encoded))
}

func TestDecodeResourcePackageIndexAllowsCatalogsLargerThan64MiB(t *testing.T) {
	provenance := resourcePackageIndexRecord{
		SourcePath: "texmf-dist/tex/latex/example/example.sty", Package: "example", PackageRevision: "12345",
		ContainerSize: 1, ContainerSHA512: strings.Repeat("a", 128), License: "LPPL-1.3c", LicenseSource: "https://ctan.org/pkg/example",
	}
	encoded, err := json.Marshal(map[string][]resourcePackageIndexRecord{"example.sty": {provenance}})
	require.NoError(t, err)
	encoded = append(encoded, bytes.Repeat([]byte(" "), 70<<20)...)

	var compressed bytes.Buffer
	writer := gzip.NewWriter(&compressed)
	_, err = writer.Write(encoded)
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	digest := sha256.Sum256(compressed.Bytes())
	document := resourceCatalogDocument{
		PackageIndexGzip:   base64.StdEncoding.EncodeToString(compressed.Bytes()),
		PackageIndexSHA256: hex.EncodeToString(digest[:]), PackageIndexEntries: 1,
	}

	index, _, err := decodeResourcePackageIndex(document)
	require.NoError(t, err)
	require.Len(t, index["example.sty"], 1)
}

func TestLoadCatalogSupportsGzipStoredCatalog(t *testing.T) {
	stg := newMemoryResourceStorage()
	content := []byte("catalog resource bytes")
	archive := makeTestArchive(t, content)
	makeTestCatalog(t, stg, content, archive)
	key := "latex/texlive/" + testResourceProfile + "/catalog.json"
	plain, err := stg.Get(t.Context(), key)
	require.NoError(t, err)
	var compressed bytes.Buffer
	writer := gzip.NewWriter(&compressed)
	_, err = writer.Write(plain)
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	require.NoError(t, stg.Put(t.Context(), key, compressed.Bytes()))

	service, err := newResourceService(stg, testResourceProfile, testResourceRepository)
	require.NoError(t, err)
	catalog, err := service.loadCatalog(t.Context())
	require.NoError(t, err)
	require.Contains(t, catalog.entries, resourceLookupKey(resourceLookup{Kind: "xetex", Format: "26", Name: "amsmath.sty"}))
}

func TestLoadCatalogNormalizesDuplicateCaseKpathseaSuffixes(t *testing.T) {
	stg := newMemoryResourceStorage()
	content := []byte("catalog resource bytes")
	archive := makeTestArchive(t, content)
	makeTestCatalog(t, stg, content, archive)
	key := "latex/texlive/" + testResourceProfile + "/catalog.json"
	encoded, err := stg.Get(t.Context(), key)
	require.NoError(t, err)
	var document resourceCatalogDocument
	require.NoError(t, json.Unmarshal(encoded, &document))
	document.FormatSuffixes = []resourceCatalogFormat{{
		Kind: "xetex", Format: "6", Suffixes: []string{".ttf", ".ttc", ".TTF", ".TTC", ".dfont"},
	}}
	encoded, err = json.Marshal(document)
	require.NoError(t, err)
	require.NoError(t, stg.Put(t.Context(), key, encoded))

	service, err := newResourceService(stg, testResourceProfile, testResourceRepository)
	require.NoError(t, err)
	catalog, err := service.loadCatalog(t.Context())
	require.NoError(t, err)
	require.Equal(t, []string{".ttf", ".ttc", ".dfont"}, catalog.formatSuffixes["xetex\x006"])
}

func makeTestCatalog(t *testing.T, stg *memoryResourceStorage, content, archive []byte) resourceCatalogEntry {
	t.Helper()
	digest := sha256.Sum256(content)
	archiveDigest := sha512.Sum512(archive)
	entry := resourceCatalogEntry{
		Lookup: resourceLookup{Kind: "xetex", Format: "26", Name: "amsmath.sty"},
		SHA256: hex.EncodeToString(digest[:]),
	}
	object := resourceCatalogObject{
		SHA256:    entry.SHA256,
		Size:      int64(len(content)),
		MediaType: "application/octet-stream",
		Provenance: []resourceCatalogProvenance{{
			SourcePath:      "texmf-dist/tex/latex/amsmath/amsmath.sty",
			Package:         "amsmath",
			PackageRevision: "79234",
			ContainerSize:   int64(len(archive)),
			ContainerSHA512: hex.EncodeToString(archiveDigest[:]),
			License:         "LPPL-1.3c",
			LicenseSource:   "https://ctan.org/pkg/amsmath?lang=en",
		}},
	}
	document := resourceCatalogDocument{
		SchemaVersion: resourceCatalogSchema,
		Profile:       testResourceProfile,
		Revision:      testResourceProfile,
		Objects:       []resourceCatalogObject{object},
		Entries:       []resourceCatalogEntry{entry},
	}
	encoded, err := json.Marshal(document)
	require.NoError(t, err)
	require.NoError(t, stg.Put(t.Context(), "latex/texlive/"+testResourceProfile+"/catalog.json", encoded))
	return entry
}

type redirectingRoundTripper struct {
	base   http.RoundTripper
	target *url.URL
}

func (r redirectingRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	cloned := request.Clone(request.Context())
	cloned.URL.Scheme = r.target.Scheme
	cloned.URL.Host = r.target.Host
	cloned.Host = r.target.Host
	response, err := r.base.RoundTrip(cloned)
	if response != nil {
		response.Request = request
	}
	return response, err
}

func newTestResourceEngine(t *testing.T, stg *memoryResourceStorage, archive []byte) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/systems/texlive/tlnet/archive/") || !strings.HasSuffix(r.URL.Path, ".tar.xz") {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(archive)
	}))
	t.Cleanup(server.Close)
	target, err := url.Parse(server.URL)
	require.NoError(t, err)
	service, err := newResourceService(stg, testResourceProfile, testResourceRepository)
	require.NoError(t, err)
	service.archives.client.Transport = redirectingRoundTripper{base: server.Client().Transport, target: target}
	engine := gin.New()
	engine.GET("/texlive/:profile/resolve/:kind/:format/*name", service.get)
	engine.HEAD("/texlive/:profile/resolve/:kind/:format/*name", service.get)
	engine.GET("/texlive/:profile/resolve/:kind", service.get)
	engine.HEAD("/texlive/:profile/resolve/:kind", service.get)
	engine.GET("/texlive/:profile/package-maps", service.getPackageMaps)
	return engine
}

func TestMountResourcesFetchesExactFileFromVerifiedPackageArchive(t *testing.T) {
	stg := newMemoryResourceStorage()
	content := []byte("% amsmath resource")
	archive := makeTestArchive(t, content)
	entry := makeTestCatalog(t, stg, content, archive)
	engine := newTestResourceEngine(t, stg, archive)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/texlive/"+testResourceProfile+"/resolve/xetex/26/amsmath.sty", nil)
	engine.ServeHTTP(response, request)
	require.Equal(t, http.StatusOK, response.Code)
	require.Equal(t, content, response.Body.Bytes())
	require.Equal(t, "application/octet-stream", response.Header().Get("Content-Type"))
	require.Equal(t, `"`+entry.SHA256+`"`, response.Header().Get("ETag"))
	require.Equal(t, entry.SHA256, response.Header().Get("fileid"))
	require.Equal(t, testResourceProfile, response.Header().Get("X-PaperSage-Resource-Revision"))
	require.Equal(t, "*", response.Header().Get("Access-Control-Allow-Origin"), "resource CORS must not depend on Origin before CDN caching")
	require.Contains(t, response.Header().Get("Access-Control-Expose-Headers"), "fileid")
	require.Contains(t, response.Header().Get("Cache-Control"), "immutable")
	require.Len(t, stg.objects, 1, "only the catalog is stored; resource bytes are fetched on demand")
}

func TestMountResourcesRejectsUnknownOrCorruptResources(t *testing.T) {
	t.Run("unknown lookup", func(t *testing.T) {
		stg := newMemoryResourceStorage()
		content := []byte("bytes")
		archive := makeTestArchive(t, content)
		makeTestCatalog(t, stg, content, archive)
		engine := newTestResourceEngine(t, stg, archive)
		response := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, "/texlive/"+testResourceProfile+"/resolve/xetex/26/not-in-catalog.sty", nil)
		engine.ServeHTTP(response, request)
		require.Equal(t, http.StatusNotFound, response.Code)
	})

	t.Run("archive checksum mismatch", func(t *testing.T) {
		stg := newMemoryResourceStorage()
		content := []byte("expected")
		archive := makeTestArchive(t, content)
		makeTestCatalog(t, stg, content, archive)
		corrupted := append([]byte(nil), archive...)
		corrupted[len(corrupted)-1] ^= 0xff
		engine := newTestResourceEngine(t, stg, corrupted)
		response := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, "/texlive/"+testResourceProfile+"/resolve/xetex/26/amsmath.sty", nil)
		engine.ServeHTTP(response, request)
		require.Equal(t, http.StatusBadGateway, response.Code)
		require.Contains(t, response.Body.String(), "checksum")
	})

	t.Run("file checksum mismatch", func(t *testing.T) {
		stg := newMemoryResourceStorage()
		archive := makeTestArchive(t, []byte("wrong bytes"))
		makeTestCatalog(t, stg, []byte("other bytes"), archive)
		engine := newTestResourceEngine(t, stg, archive)
		response := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, "/texlive/"+testResourceProfile+"/resolve/xetex/26/amsmath.sty", nil)
		engine.ServeHTTP(response, request)
		require.Equal(t, http.StatusBadGateway, response.Code)
		require.Contains(t, response.Body.String(), "checksum")
	})
}

func TestMountResourcesResolvesUniquePackageFilesOnDemand(t *testing.T) {
	stg := newMemoryResourceStorage()
	content := []byte("% dynamically indexed fullpage package resource")
	archive := makeTestArchiveAt(t, "tex/latex/preprint/fullpage.sty", content)
	makeTestCatalog(t, stg, []byte("catalog seed"), makeTestArchive(t, []byte("catalog seed")))
	archiveDigest := sha512.Sum512(archive)
	addTestPackageIndex(t, stg, "fullpage.sty", []resourcePackageIndexRecord{{
		SourcePath: "texmf-dist/tex/latex/preprint/fullpage.sty", Package: "preprint", PackageRevision: "12345",
		ContainerSize: int64(len(archive)), ContainerSHA512: hex.EncodeToString(archiveDigest[:]),
		License: "LPPL-1.3c", LicenseSource: "https://ctan.org/pkg/preprint?lang=en",
	}})
	engine := newTestResourceEngine(t, stg, archive)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/texlive/"+testResourceProfile+"/resolve/xetex/26/fullpage.sty", nil)
	engine.ServeHTTP(response, request)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	require.Equal(t, content, response.Body.Bytes())
	digest := sha256.Sum256(content)
	require.Equal(t, hex.EncodeToString(digest[:]), response.Header().Get("fileid"))
	require.Equal(t, testResourceProfile, response.Header().Get("X-PaperSage-Resource-Revision"))
	require.Equal(t, "preprint", response.Header().Get("X-PaperSage-Package"))
	require.Contains(t, response.Header().Get("Cache-Control"), "immutable")
}

func TestMountResourcesServesVerifiedProfileDerivedResource(t *testing.T) {
	stg := newMemoryResourceStorage()
	content := []byte("% base package resource")
	archive := makeTestArchive(t, content)
	makeTestCatalog(t, stg, content, archive)
	derivedBytes := []byte("generated map data\n")
	digest := sha256.Sum256(derivedBytes)
	hash := hex.EncodeToString(digest[:])
	derived := resourceCatalogDerived{
		Lookup:    resourceLookup{Kind: "dvipdfmx", Format: "11", Name: "kanjix.map"},
		SHA256:    hash,
		Size:      int64(len(derivedBytes)),
		MediaType: "text/plain; charset=utf-8",
		ObjectKey: "derived/" + hash,
		Generator: resourceCatalogDerivedGenerator{
			Name: "updmap-sys", Version: "TeX Live 2026 r80534", ExecutableSHA256: strings.Repeat("b", 64),
		},
		Inputs: []resourceCatalogDerivedInput{{
			SourcePath: "texmf-dist/web2c/updmap.cfg", SHA256: strings.Repeat("c", 64),
			Package: "texlive-config", PackageRevision: "12345", ContainerSize: 100,
			ContainerSHA512: strings.Repeat("a", 128), License: "LPPL-1.3c", LicenseSource: "https://ctan.org/pkg/texlive-config",
		}},
	}
	encoded, err := stg.Get(t.Context(), "latex/texlive/"+testResourceProfile+"/catalog.json")
	require.NoError(t, err)
	var document resourceCatalogDocument
	require.NoError(t, json.Unmarshal(encoded, &document))
	document.DerivedResources = []resourceCatalogDerived{derived}
	encoded, err = json.Marshal(document)
	require.NoError(t, err)
	require.NoError(t, stg.Put(t.Context(), "latex/texlive/"+testResourceProfile+"/catalog.json", encoded))
	require.NoError(t, stg.Put(t.Context(), "latex/texlive/"+testResourceProfile+"/derived/"+hash, derivedBytes))

	fontBytes := []byte("derived OpenType font bytes")
	fontDigest := sha256.Sum256(fontBytes)
	fontHash := hex.EncodeToString(fontDigest[:])
	fontResource := derived
	fontResource.Lookup = resourceLookup{Kind: "font", Format: "OT", Name: "FandolSong-Regular"}
	fontResource.SHA256 = fontHash
	fontResource.Size = int64(len(fontBytes))
	fontResource.MediaType = "application/octet-stream"
	fontResource.ObjectKey = "derived/" + fontHash
	document.DerivedResources = append(document.DerivedResources, fontResource)
	encoded, err = json.Marshal(document)
	require.NoError(t, err)
	require.NoError(t, stg.Put(t.Context(), "latex/texlive/"+testResourceProfile+"/catalog.json", encoded))
	require.NoError(t, stg.Put(t.Context(), "latex/texlive/"+testResourceProfile+"/derived/"+fontHash, fontBytes))
	engine := newTestResourceEngine(t, stg, archive)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/texlive/"+testResourceProfile+"/resolve/dvipdfmx/11/kanjix.map", nil)
	engine.ServeHTTP(response, request)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	require.Equal(t, derivedBytes, response.Body.Bytes())
	require.Equal(t, hash, response.Header().Get("fileid"))
	require.Empty(t, response.Header().Get("fontid"))
	require.Equal(t, "true", response.Header().Get("X-PaperSage-Generated-Resource"))

	response = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/texlive/"+testResourceProfile+"/resolve/font/OT/FandolSong-Regular", nil)
	engine.ServeHTTP(response, request)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	require.Equal(t, fontBytes, response.Body.Bytes())
	require.Equal(t, fontHash, response.Header().Get("fontid"))
	require.Empty(t, response.Header().Get("fileid"))

	require.NoError(t, stg.Put(t.Context(), "latex/texlive/"+testResourceProfile+"/derived/"+hash, bytes.Repeat([]byte("x"), len(derivedBytes))))
	response = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/texlive/"+testResourceProfile+"/resolve/dvipdfmx/11/kanjix.map", nil)
	engine.ServeHTTP(response, request)
	require.Equal(t, http.StatusBadGateway, response.Code)
	require.Contains(t, response.Body.String(), "hash check")
}

func TestMountResourcesMergesFontMapsFromUsedPackages(t *testing.T) {
	stg := newMemoryResourceStorage()
	mapBytes := []byte("ntxsym txsym <txsym.pfb\n")
	archive := makeTestArchiveAt(t, "fonts/map/dvips/newtx/newtx.map", mapBytes)
	makeTestCatalog(t, stg, []byte("catalog seed"), makeTestArchive(t, []byte("catalog seed")))
	archiveDigest := sha512.Sum512(archive)
	addTestPackageIndex(t, stg, "newtx.map", []resourcePackageIndexRecord{{
		SourcePath: "texmf-dist/fonts/map/dvips/newtx/newtx.map", Package: "newtx", PackageRevision: "12345",
		ContainerSize: int64(len(archive)), ContainerSHA512: hex.EncodeToString(archiveDigest[:]),
		License: "LPPL-1.3c", LicenseSource: "https://ctan.org/pkg/newtx?lang=en",
	}})
	engine := newTestResourceEngine(t, stg, archive)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/texlive/"+testResourceProfile+"/package-maps?packages=newtx", nil)
	engine.ServeHTTP(response, request)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	require.Equal(t, mapBytes, response.Body.Bytes())
	require.Equal(t, "text/plain; charset=utf-8", response.Header().Get("Content-Type"))
	require.Equal(t, testResourceProfile, response.Header().Get("X-PaperSage-Resource-Revision"))
}

func TestMountResourcesResolvesExtensionlessKpathseaNamesInDeclaredOrder(t *testing.T) {
	stg := newMemoryResourceStorage()
	content := []byte("% plain TeX lookup wins")
	archive := makeTestArchiveFiles(t, map[string][]byte{
		"tex/latex/example/tracklang.tex": content,
		"tex/latex/example/tracklang.sty": []byte("% LaTeX package, must not be chosen for an extensionless TeX lookup"),
	})
	makeTestCatalog(t, stg, []byte("catalog seed"), makeTestArchive(t, []byte("catalog seed")))
	archiveDigest := sha512.Sum512(archive)
	provenance := func(file string) []resourcePackageIndexRecord {
		return []resourcePackageIndexRecord{{
			SourcePath: "texmf-dist/tex/latex/example/" + file, Package: "example", PackageRevision: "12345",
			ContainerSize: int64(len(archive)), ContainerSHA512: hex.EncodeToString(archiveDigest[:]),
			License: "LPPL-1.3c", LicenseSource: "https://ctan.org/pkg/example?lang=en",
		}}
	}
	addTestPackageIndexEntries(t, stg, map[string][]resourcePackageIndexRecord{
		"tracklang.tex": provenance("tracklang.tex"),
		"tracklang.sty": provenance("tracklang.sty"),
	})
	engine := newTestResourceEngine(t, stg, archive)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/texlive/"+testResourceProfile+"/resolve/xetex?format=26&name=tracklang", nil)
	engine.ServeHTTP(response, request)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	require.Equal(t, content, response.Body.Bytes())
}

func TestMountResourcesResolvesGraphicsWithGenericTeXInputFormat(t *testing.T) {
	stg := newMemoryResourceStorage()
	content := []byte("%PDF-1.4\nimage fixture")
	archive := makeTestArchiveFiles(t, map[string][]byte{
		"doc/latex/mwe/example-image-a.pdf": content,
	})
	makeTestCatalog(t, stg, []byte("catalog seed"), makeTestArchive(t, []byte("catalog seed")))
	archiveDigest := sha512.Sum512(archive)
	addTestPackageIndexEntries(t, stg, map[string][]resourcePackageIndexRecord{
		"example-image-a.pdf": {{
			SourcePath: "texmf-dist/doc/latex/mwe/example-image-a.pdf", Package: "mwe", PackageRevision: "12345",
			ContainerSize: int64(len(archive)), ContainerSHA512: hex.EncodeToString(archiveDigest[:]),
			License: "LPPL-1.3c", LicenseSource: "https://ctan.org/pkg/mwe?lang=en",
		}},
	})
	engine := newTestResourceEngine(t, stg, archive)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/texlive/"+testResourceProfile+"/resolve/pdftex?format=26&name=example-image-a.pdf", nil)
	engine.ServeHTTP(response, request)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	require.Equal(t, content, response.Body.Bytes())
}

func TestMountResourcesIgnoresNonSearchablePackageTreeForFontLookup(t *testing.T) {
	stg := newMemoryResourceStorage()
	content := []byte("type1 font")
	archive := makeTestArchiveFiles(t, map[string][]byte{
		"fonts/type1/urw/courier/ucrr8a.pfb":  content,
		"tlpkg/tlgs/Resource/Font/ucrr8a.pfb": []byte("ghostscript copy"),
	})
	makeTestCatalog(t, stg, []byte("catalog seed"), makeTestArchive(t, []byte("catalog seed")))
	archiveDigest := sha512.Sum512(archive)
	provenance := func(sourcePath string) resourcePackageIndexRecord {
		return resourcePackageIndexRecord{
			SourcePath: sourcePath, Package: "courier", PackageRevision: "12345",
			ContainerSize: int64(len(archive)), ContainerSHA512: hex.EncodeToString(archiveDigest[:]),
			License: "GPL-2.0-or-later", LicenseSource: "https://ctan.org/pkg/courier?lang=en",
		}
	}
	addTestPackageIndexEntries(t, stg, map[string][]resourcePackageIndexRecord{
		"ucrr8a.pfb": {
			provenance("texmf-dist/fonts/type1/urw/courier/ucrr8a.pfb"),
			provenance("texmf-dist/tlpkg/tlgs/Resource/Font/ucrr8a.pfb"),
		},
	})
	engine := newTestResourceEngine(t, stg, archive)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/texlive/"+testResourceProfile+"/resolve/pdftex/32/ucrr8a.pfb", nil)
	engine.ServeHTTP(response, request)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	require.Equal(t, content, response.Body.Bytes())
}

func TestValidResourceFormatAllowsGeneratedKpathseaFormatSafely(t *testing.T) {
	require.True(t, validResourceFormat("pk/600"))
	for _, value := range []string{"../600", "pk/../600", "/pk/600", "pk//600", "pk\\600", strings.Repeat("x", 97)} {
		require.False(t, validResourceFormat(value), value)
	}
}

func TestMountResourcesRejectsAmbiguousPackageFilenames(t *testing.T) {
	stg := newMemoryResourceStorage()
	archive := makeTestArchive(t, []byte("catalog seed"))
	makeTestCatalog(t, stg, []byte("catalog seed"), archive)
	provenance := func(packageName, sourcePath string) resourcePackageIndexRecord {
		archiveDigest := sha512.Sum512(archive)
		return resourcePackageIndexRecord{
			SourcePath: sourcePath, Package: packageName, PackageRevision: "12345",
			ContainerSize: int64(len(archive)), ContainerSHA512: hex.EncodeToString(archiveDigest[:]),
			License: "LPPL-1.3c", LicenseSource: "https://ctan.org/pkg/amsmath?lang=en",
		}
	}
	addTestPackageIndex(t, stg, "duplicate.sty", []resourcePackageIndexRecord{
		provenance("amsmath", "texmf-dist/tex/latex/amsmath/duplicate.sty"),
		provenance("mathtools", "texmf-dist/tex/latex/mathtools/duplicate.sty"),
	})
	engine := newTestResourceEngine(t, stg, archive)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/texlive/"+testResourceProfile+"/resolve/xetex/26/duplicate.sty", nil)
	engine.ServeHTTP(response, request)
	require.Equal(t, http.StatusConflict, response.Code)
	require.Contains(t, response.Body.String(), "ambiguous")
}

func TestResourceServiceRetriesCatalogAfterStorageRecovery(t *testing.T) {
	stg := newMemoryResourceStorage()
	engine := newTestResourceEngine(t, stg, nil)
	request := httptest.NewRequest(http.MethodGet, "/texlive/"+testResourceProfile+"/resolve/xetex/26/amsmath.sty", nil)
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, request)
	require.Equal(t, http.StatusServiceUnavailable, response.Code)

	content := []byte("resource became available")
	archive := makeTestArchive(t, content)
	makeTestCatalog(t, stg, content, archive)
	engine = newTestResourceEngine(t, stg, archive)
	response = httptest.NewRecorder()
	engine.ServeHTTP(response, httptest.NewRequest(http.MethodGet, request.URL.String(), nil))
	require.Equal(t, http.StatusOK, response.Code)
}

func TestValidateResourceRepositoryRejectsUnsafeURLs(t *testing.T) {
	for _, raw := range []string{
		"http://mirror.ctan.org/systems/texlive/tlnet",
		"https://user:pass@example.com/systems/texlive/tlnet",
		"https://127.0.0.1/systems/texlive/tlnet",
		"https://example.com/systems/texlive/tlnet?redirect=1",
	} {
		require.Error(t, validateResourceRepository(raw), raw)
	}
}

func TestLiveTeXLivePackageArchive(t *testing.T) {
	if os.Getenv("PAPERSAGE_LIVE_TEXLIVE_SMOKE") != "1" {
		t.Skip("set PAPERSAGE_LIVE_TEXLIVE_SMOKE=1 to verify the pinned public TeX Live archive")
	}
	provenance := resourceCatalogProvenance{
		SourcePath:      "texmf-dist/tex/latex/amsmath/amsmath.sty",
		Package:         "amsmath",
		PackageRevision: "79234",
		ContainerSize:   31724,
		ContainerSHA512: "5d6768eea6404bcde83f6fdd7025f6f63dfafc633c689a308f1465ffbeb74385daddc4cd2cf4eb6f0152e8a2c9f8ee1cdc94aa8ad796051155a299d355503c4c",
	}
	cache := newPackageArchiveCache(64 << 20)
	archive, err := cache.archive(context.Background(), "https://mirror.ctan.org/systems/texlive/tlnet", provenance)
	require.NoError(t, err)
	resource, err := extractCataloguedResource(archive, provenance.SourcePath, resourceCatalogObject{
		SHA256: "8bf35130be39e123cbd0e4f0c09d3806d745d4c08ee391a543a8b9675191e6a7",
		Size:   88826,
	})
	require.NoError(t, err)
	require.Len(t, resource, 88826)
}
