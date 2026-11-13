package assets

import (
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func testFS() fs.FS {
	return fstest.MapFS{
		"index.html":    {Data: []byte("<html>app</html>")},
		"assets/app.js": {Data: []byte("console.log(1)")},
	}
}

func TestHandlerServesFilesAndFallsBackToIndex(t *testing.T) {
	h := HandlerFor(testFS())

	tests := []struct {
		path     string
		wantBody string
	}{
		{"/", "<html>app</html>"},
		{"/assets/app.js", "console.log(1)"},
		{"/project/abc/writing", "<html>app</html>"},
	}
	for _, tt := range tests {
		req := httptest.NewRequest(http.MethodGet, tt.path, nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("%s: status %d", tt.path, rec.Code)
			continue
		}
		if rec.Body.String() != tt.wantBody {
			t.Errorf("%s: body %q, want %q", tt.path, rec.Body.String(), tt.wantBody)
		}
	}
}

func TestHandlerProxiesTexliveToBackend(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/texlive/p/resolve/xetex" || r.URL.Query().Get("name") != "latex.ltx" {
			t.Errorf("unexpected backend request %s?%s", r.URL.Path, r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("resource"))
	}))
	defer backend.Close()
	SetBackendURL(backend.URL)
	defer SetBackendURL("")

	req := httptest.NewRequest(http.MethodGet, "/texlive/p/resolve/xetex?format=26&name=latex.ltx", nil)
	rec := httptest.NewRecorder()
	HandlerFor(testFS()).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || rec.Body.String() != "resource" {
		t.Fatalf("proxy response = %d %q", rec.Code, rec.Body.String())
	}
}

func TestHandlerProxiesTexlivePostBody(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("backend method = %s, want POST", r.Method)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("backend Content-Type = %q", got)
		}
		body, _ := io.ReadAll(r.Body)
		if string(body) != `{"packages":[]}` {
			t.Errorf("backend body = %q", body)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer backend.Close()
	SetBackendURL(backend.URL)
	defer SetBackendURL("")

	req := httptest.NewRequest(http.MethodPost, "/texlive/p/package-maps", strings.NewReader(`{"packages":[]}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	HandlerFor(testFS()).ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("proxy response = %d", rec.Code)
	}
}
