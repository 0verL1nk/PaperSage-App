package latex

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/ulikunitz/xz"

	"github.com/0verL1nk/GraduateProject/server/internal/latex/profile"
)

const testResourceProfile = "tl-test-profile"
const testResourceRepository = "https://ctan.example/systems/texlive/tlnet"

type resourceFixture struct {
	bundle   *profile.Bundle
	cache    *DiskCache
	engine   *gin.Engine
	server   *httptest.Server
	archive  []byte
	requests int
}

func sha512Hex(data []byte) string {
	digest := sha512.Sum512(data)
	return hex.EncodeToString(digest[:])
}

func tlpdbStanza(name string, size int, checksum string, files ...string) string {
	lines := []string{
		"name " + name, "revision 12345", "catalogue " + name,
		"catalogue-license LPPL-1.3c", fmt.Sprintf("containersize %d", size),
		"containerchecksum " + checksum, "runfiles",
	}
	for _, file := range files {
		lines = append(lines, " "+file)
	}
	return strings.Join(lines, "\n") + "\n\n"
}

func makeTestArchive(t *testing.T, content []byte) []byte {
	t.Helper()
	var compressed bytes.Buffer
	xzWriter, err := xz.NewWriter(&compressed)
	require.NoError(t, err)
	tarWriter := tar.NewWriter(xzWriter)
	require.NoError(t, tarWriter.WriteHeader(&tar.Header{Name: "tex/latex/amsmath/amsmath.sty", Mode: 0o644, Size: int64(len(content))}))
	_, err = tarWriter.Write(content)
	require.NoError(t, err)
	require.NoError(t, tarWriter.Close())
	require.NoError(t, xzWriter.Close())
	return compressed.Bytes()
}

func sha256Hex(data []byte) string {
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

func writeTestProfile(t *testing.T, archive []byte, lookups []profile.Lookup, derived map[string][]byte) *profile.Bundle {
	t.Helper()
	root := t.TempDir()
	if len(derived) > 0 {
		require.NoError(t, os.MkdirAll(filepath.Join(root, "derived"), 0o700))
		for name, data := range derived {
			require.NoError(t, os.WriteFile(filepath.Join(root, "derived", name), data, 0o600))
		}
	}
	var tlpdb bytes.Buffer
	writer, err := xz.NewWriter(&tlpdb)
	require.NoError(t, err)
	size, checksum := len(archive), sha512Hex(archive)
	document := tlpdbStanza("amsmath", size, checksum, "tex/latex/amsmath/amsmath.sty")
	document += tlpdbStanza("duplicate", size, checksum, "tex/latex/duplicate/duplicate.sty")
	document += tlpdbStanza("amanother", size, checksum, "tex/latex/amanother/duplicate.sty")
	document += tlpdbStanza("babel", size, checksum, "tex/generic/babel/hyphen.cfg")
	document += tlpdbStanza("cslatex", size, checksum, "tex/cslatex/base/hyphen.cfg")
	_, err = writer.Write([]byte(document))
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	require.NoError(t, os.WriteFile(filepath.Join(root, "tlpdb.xz"), tlpdb.Bytes(), 0o600))

	var lookupsGzip bytes.Buffer
	gzipWriter := gzip.NewWriter(&lookupsGzip)
	encoded, err := json.Marshal(lookups)
	require.NoError(t, err)
	_, err = gzipWriter.Write(encoded)
	require.NoError(t, err)
	require.NoError(t, gzipWriter.Close())
	require.NoError(t, os.WriteFile(filepath.Join(root, "lookups.json.gz"), lookupsGzip.Bytes(), 0o600))

	digest := sha512.Sum512(tlpdb.Bytes())
	manifest := profile.Profile{
		SchemaVersion: 1,
		Profile:       testResourceProfile,
		Repository:    testResourceRepository,
		Tlpdb:         profile.TlpdbArtifact{Path: "tlpdb.xz", SHA512: hex.EncodeToString(digest[:]), Size: int64(tlpdb.Len())},
		Lookups:       profile.LookupsArtifact{SHA256: sha256Hex(lookupsGzip.Bytes()), Entries: len(lookups)},
		FormatSuffixes: []profile.FormatSuffix{
			{Kind: "xetex", Format: "26", Suffixes: []string{".tex", ".sty"}},
		},
	}
	raw, err := json.Marshal(manifest)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(root, "profile.json"), raw, 0o600))
	bundle, err := profile.Load(root)
	require.NoError(t, err)
	return bundle
}

func newTestResourceEngine(t *testing.T, bundle *profile.Bundle, archive []byte) *resourceFixture {
	t.Helper()
	gin.SetMode(gin.TestMode)
	fixture := &resourceFixture{archive: archive}
	fixture.server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fixture.requests++
		if !strings.HasPrefix(r.URL.Path, "/systems/texlive/tlnet/archive/") || !strings.HasSuffix(r.URL.Path, ".tar.xz") {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(archive)
	}))
	t.Cleanup(fixture.server.Close)
	target, err := url.Parse(fixture.server.URL)
	require.NoError(t, err)
	cache, err := NewDiskCache(t.TempDir(), 1<<20)
	require.NoError(t, err)
	service, err := newResourceService(bundle, cache)
	require.NoError(t, err)
	service.archives.client.Transport = redirectingRoundTripper{base: fixture.server.Client().Transport, target: target}
	fixture.cache = cache
	fixture.bundle = bundle
	fixture.engine = gin.New()
	fixture.engine.GET("/texlive/profile", service.getProfile)
	fixture.engine.GET("/texlive/:profile/resolve/:kind/:format/*name", service.get)
	fixture.engine.HEAD("/texlive/:profile/resolve/:kind/:format/*name", service.get)
	fixture.engine.GET("/texlive/:profile/resolve/:kind", service.get)
	fixture.engine.HEAD("/texlive/:profile/resolve/:kind", service.get)
	fixture.engine.GET("/texlive/:profile/package-maps", service.getPackageMaps)
	fixture.engine.POST("/texlive/:profile/package-maps", service.getPackageMaps)
	return fixture
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

func (f *resourceFixture) get(t *testing.T, method, target string) *httptest.ResponseRecorder {
	t.Helper()
	response := httptest.NewRecorder()
	f.engine.ServeHTTP(response, httptest.NewRequest(method, target, nil))
	return response
}

func TestResourceServiceServesIndexedFileFromCache(t *testing.T) {
	content := []byte("% amsmath resource")
	archive := makeTestArchive(t, content)
	fixture := newTestResourceEngine(t, writeTestProfile(t, archive, nil, nil), archive)

	response := fixture.get(t, http.MethodGet, "/texlive/"+testResourceProfile+"/resolve/xetex/26/amsmath.sty")
	require.Equal(t, http.StatusOK, response.Code)
	require.Equal(t, content, response.Body.Bytes())
	require.Equal(t, `"`+sha256Hex(content)+`"`, response.Header().Get("ETag"))
	require.Equal(t, sha256Hex(content), response.Header().Get("fileid"))
	require.Equal(t, "amsmath", response.Header().Get("X-PaperSage-Package"))
	require.Equal(t, testResourceProfile, response.Header().Get("X-PaperSage-Resource-Revision"))
	require.Equal(t, "*", response.Header().Get("Access-Control-Allow-Origin"))
	require.Contains(t, response.Header().Get("Cache-Control"), "immutable")

	again := fixture.get(t, http.MethodGet, "/texlive/"+testResourceProfile+"/resolve/xetex/26/amsmath.sty")
	require.Equal(t, http.StatusOK, again.Code)
	require.Equal(t, 1, fixture.requests, "the second request must be served from the disk cache")
	require.Positive(t, fixture.cache.Bytes())
}

func TestResourceServiceServesLookupOverridesAndDerived(t *testing.T) {
	content := []byte("% amsmath resource")
	archive := makeTestArchive(t, content)
	derived := []byte("% generated hyphen.cfg")

	bundle := writeTestProfile(t, archive, []profile.Lookup{
		{Kind: "xetex", Format: "26", Name: "hyphen.cfg", Source: "texmf-dist/tex/generic/babel/hyphen.cfg"},
		{Kind: "xetex", Format: "26", Name: "generated.cfg",
			Derived: &profile.Derived{SHA256: sha256Hex(derived), Size: int64(len(derived)), MediaType: "text/plain"}},
	}, map[string][]byte{sha256Hex(derived): derived})
	fixture := newTestResourceEngine(t, bundle, archive)

	override := fixture.get(t, http.MethodGet, "/texlive/"+testResourceProfile+"/resolve/xetex/26/hyphen.cfg")
	require.Equal(t, http.StatusBadGateway, override.Code, "babel's hyphen.cfg is not in this fixture's archive")

	generated := fixture.get(t, http.MethodGet, "/texlive/"+testResourceProfile+"/resolve/xetex/26/generated.cfg")
	require.Equal(t, http.StatusOK, generated.Code)
	require.Equal(t, derived, generated.Body.Bytes())
	require.Equal(t, sha256Hex(derived), generated.Header().Get("fileid"))
}

func TestResourceServiceRejectsUnknownAndAmbiguousNames(t *testing.T) {
	content := []byte("% amsmath resource")
	archive := makeTestArchive(t, content)
	fixture := newTestResourceEngine(t, writeTestProfile(t, archive, nil, nil), archive)

	missing := fixture.get(t, http.MethodGet, "/texlive/"+testResourceProfile+"/resolve/xetex/26/not-in-profile.sty")
	require.Equal(t, http.StatusNotFound, missing.Code)

	duplicate := fixture.get(t, http.MethodGet, "/texlive/"+testResourceProfile+"/resolve/xetex/26/duplicate.sty")
	require.Equal(t, http.StatusConflict, duplicate.Code)
	require.Contains(t, duplicate.Body.String(), "amanother")

	wrongProfile := fixture.get(t, http.MethodGet, "/texlive/other/resolve/xetex/26/amsmath.sty")
	require.Equal(t, http.StatusNotFound, wrongProfile.Code)
}

func TestResourceServiceHeadAnswersWithoutBody(t *testing.T) {
	content := []byte("% amsmath resource")
	archive := makeTestArchive(t, content)
	fixture := newTestResourceEngine(t, writeTestProfile(t, archive, nil, nil), archive)
	response := fixture.get(t, http.MethodHead, "/texlive/"+testResourceProfile+"/resolve/xetex/26/amsmath.sty")
	require.Equal(t, http.StatusOK, response.Code)
	require.Equal(t, fmt.Sprint(len(content)), response.Header().Get("Content-Length"))
	require.Zero(t, response.Body.Len())
}

func TestResourceServiceReportsArchiveFailures(t *testing.T) {
	content := []byte("% amsmath resource")
	archive := makeTestArchive(t, content)
	corrupted := append([]byte(nil), archive...)
	corrupted[len(corrupted)-1] ^= 0xff
	fixture := newTestResourceEngine(t, writeTestProfile(t, archive, nil, nil), corrupted)
	response := fixture.get(t, http.MethodGet, "/texlive/"+testResourceProfile+"/resolve/xetex/26/amsmath.sty")
	require.Equal(t, http.StatusBadGateway, response.Code)
	require.Contains(t, response.Body.String(), "checksum")
}

func TestResourceServiceServesProfileDiscovery(t *testing.T) {
	content := []byte("% amsmath resource")
	archive := makeTestArchive(t, content)
	fixture := newTestResourceEngine(t, writeTestProfile(t, archive, nil, nil), archive)
	response := fixture.get(t, http.MethodGet, "/texlive/profile")
	require.Equal(t, http.StatusOK, response.Code)
	require.JSONEq(t, `{"profile":"`+testResourceProfile+`"}`, response.Body.String())
	require.Equal(t, "no-store", response.Header().Get("Cache-Control"))
}

func TestResourceServiceFallsBackWhenTheSnapshotIsBotGated(t *testing.T) {
	content := []byte("% amsmath resource")
	archive := makeTestArchive(t, content)
	bundle := writeTestProfile(t, archive, nil, nil)
	bundle.Profile.Repository = "https://snapshot.example/tlnet"
	bundle.Profile.Mirror = testResourceRepository

	requests := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if requests == 1 {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte("<html>challenge</html>"))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(archive)
	}))
	t.Cleanup(server.Close)
	target, err := url.Parse(server.URL)
	require.NoError(t, err)
	cache, err := NewDiskCache(t.TempDir(), 1<<20)
	require.NoError(t, err)
	service, err := newResourceService(bundle, cache)
	require.NoError(t, err)
	service.archives.client.Transport = redirectingRoundTripper{base: server.Client().Transport, target: target}
	engine := gin.New()
	engine.GET("/texlive/:profile/resolve/:kind/:format/*name", service.get)

	response := httptest.NewRecorder()
	engine.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/texlive/"+testResourceProfile+"/resolve/xetex/26/amsmath.sty", nil))
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	require.Equal(t, content, response.Body.Bytes())
	require.Equal(t, 2, requests, "the bot-gated primary must be followed by the mirror")
}

func TestResourceServiceReportsAnHTMLChallengeClearly(t *testing.T) {
	content := []byte("% amsmath resource")
	archive := makeTestArchive(t, content)
	bundle := writeTestProfile(t, archive, nil, nil)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte("<html>challenge</html>"))
	}))
	t.Cleanup(server.Close)
	target, err := url.Parse(server.URL)
	require.NoError(t, err)
	cache, err := NewDiskCache(t.TempDir(), 1<<20)
	require.NoError(t, err)
	service, err := newResourceService(bundle, cache)
	require.NoError(t, err)
	service.archives.client.Transport = redirectingRoundTripper{base: server.Client().Transport, target: target}
	engine := gin.New()
	engine.GET("/texlive/:profile/resolve/:kind/:format/*name", service.get)

	response := httptest.NewRecorder()
	engine.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/texlive/"+testResourceProfile+"/resolve/xetex/26/amsmath.sty", nil))
	require.Equal(t, http.StatusBadGateway, response.Code)
	require.Contains(t, response.Body.String(), "HTML page")
}
