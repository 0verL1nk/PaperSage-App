package assets

import (
	"io"
	"io/fs"
	"net/http"
	"path"
	"strings"
	"sync/atomic"
	"time"
)

var (
	backendURL atomic.Value // string
	// Package-map requests make the backend download TeX archives from CTAN;
	// on a slow mirror the first compile can outlast a short proxy timeout.
	proxyClient = &http.Client{Timeout: 10 * time.Minute}
)

// SetBackendURL enables same-origin /texlive proxying to the supervised
// backend. The web layer then resolves TeX resources without a cross-origin
// request (WebView2 blocks worker XHRs to a private address).
func SetBackendURL(raw string) {
	backendURL.Store(strings.TrimRight(raw, "/"))
}

// FS returns the embedded frontend rooted at its dist directory.
func FS() fs.FS {
	sub, err := fs.Sub(Dist, "dist")
	if err != nil {
		panic(err)
	}
	return sub
}

// Handler serves the SPA and falls back to index.html for client-side routes.
func Handler() http.Handler {
	return HandlerFor(FS())
}

func HandlerFor(root fs.FS) http.Handler {
	files := http.FileServer(http.FS(root))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/texlive/") {
			if base, _ := backendURL.Load().(string); base != "" {
				proxyResource(w, r, base)
				return
			}
		}
		name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if name == "" || name == "." {
			name = "index.html"
		}
		if f, err := root.Open(name); err == nil {
			_ = f.Close()
			files.ServeHTTP(w, r)
			return
		}
		cloned := r.Clone(r.Context())
		cloned.URL.Path = "/"
		files.ServeHTTP(w, cloned)
	})
}

func proxyResource(w http.ResponseWriter, r *http.Request, base string) {
	target := base + r.URL.Path
	if r.URL.RawQuery != "" {
		target += "?" + r.URL.RawQuery
	}
	req, err := http.NewRequestWithContext(r.Context(), r.Method, target, r.Body)
	if err != nil {
		http.Error(w, "resource proxy: "+err.Error(), http.StatusBadGateway)
		return
	}
	req.ContentLength = r.ContentLength
	for _, header := range []string{"Accept", "Content-Type", "Range", "If-None-Match"} {
		if value := r.Header.Get(header); value != "" {
			req.Header.Set(header, value)
		}
	}
	resp, err := proxyClient.Do(req)
	if err != nil {
		http.Error(w, "resource proxy: "+err.Error(), http.StatusBadGateway)
		return
	}
	defer func() { _ = resp.Body.Close() }()
	for _, header := range []string{"Content-Type", "Content-Length", "Cache-Control", "ETag", "fileid", "fontid", "X-PaperSage-Resource-Revision", "Accept-Ranges", "Content-Range"} {
		if value := resp.Header.Get(header); value != "" {
			w.Header().Set(header, value)
		}
	}
	w.WriteHeader(resp.StatusCode)
	if r.Method != http.MethodHead {
		_, _ = io.Copy(w, resp.Body)
	}
}
