package backend

import (
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestBaseURL(t *testing.T) {
	if got, want := baseURL(5478), "http://127.0.0.1:5478"; got != want {
		t.Fatalf("baseURL = %q, want %q", got, want)
	}
}

func TestFreePortIsBindable(t *testing.T) {
	port, err := freePort()
	if err != nil {
		t.Fatalf("freePort: %v", err)
	}
	if port <= 0 {
		t.Fatalf("freePort returned %d", port)
	}
	l, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		t.Fatalf("port %d reported free but not bindable: %v", port, err)
	}
	_ = l.Close()
}

func TestSelectPort(t *testing.T) {
	// A free preferred port is kept.
	free := listenerPort(t)
	if port, external, err := selectPort(free, func(int) bool { return false }); err != nil || external || port != free {
		t.Fatalf("free preferred: port=%d external=%v err=%v, want %d/false/nil", port, external, err, free)
	}

	// An occupied port that is not papersage forces a different free port.
	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer occupied.Close()
	busy := occupied.Addr().(*net.TCPAddr).Port
	port, external, err := selectPort(busy, func(int) bool { return false })
	if err != nil {
		t.Fatalf("occupied preferred: %v", err)
	}
	if external {
		t.Fatal("occupied preferred reported external")
	}
	if port == busy {
		t.Fatalf("occupied preferred returned the busy port %d", busy)
	}

	// A healthy papersage on the preferred port is reused.
	port, external, err = selectPort(free, func(int) bool { return true })
	if err != nil || !external || port != free {
		t.Fatalf("healthy preferred: port=%d external=%v err=%v, want %d/true/nil", port, external, err, free)
	}
}

func TestProbeHealth(t *testing.T) {
	papersage := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"status":"healthy","version":"1.2.3","goVersion":"go1.26"}`))
	}))
	defer papersage.Close()
	if !probeHealth(papersage.URL) {
		t.Fatal("healthy papersage response was not recognized")
	}

	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"status":"healthy"}`))
	}))
	defer other.Close()
	if probeHealth(other.URL) {
		t.Fatal("foreign healthy service was mistaken for papersage")
	}

	if probeHealth("http://127.0.0.1:1") {
		t.Fatal("closed port reported healthy")
	}
}

func TestLocateBinary(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(envBinary, "")

	if _, err := LocateBinary(dir); err == nil {
		t.Fatal("missing binary did not error")
	}

	// The desktop shell itself is <exe_dir>/papersage; it must never be
	// mistaken for the bundled backend.
	shell := filepath.Join(dir, binaryName())
	if err := os.WriteFile(shell, []byte("shell"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := LocateBinary(dir); err == nil {
		t.Fatal("desktop shell binary was selected as the backend")
	}

	backendDir := filepath.Join(dir, "backend", binaryName())
	if err := os.MkdirAll(filepath.Dir(backendDir), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(backendDir, []byte("bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got, err := LocateBinary(dir); err != nil || got != backendDir {
		t.Fatalf("backend lookup = %q, %v; want %q", got, err, backendDir)
	}

	// The installed backend wins over the environment override.
	override := filepath.Join(dir, "custom-papersage")
	if err := os.WriteFile(override, []byte("bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv(envBinary, override)
	if got, err := LocateBinary(dir); err != nil || got != backendDir {
		t.Fatalf("backend dir precedence = %q, %v; want %q", got, err, backendDir)
	}

	// Without the installed copy, the override is used.
	if err := os.Remove(backendDir); err != nil {
		t.Fatal(err)
	}
	if got, err := LocateBinary(dir); err != nil || got != override {
		t.Fatalf("env override = %q, %v; want %q", got, err, override)
	}

	// A stale override fails loudly instead of falling back to the shell.
	t.Setenv(envBinary, filepath.Join(dir, "missing"))
	if _, err := LocateBinary(dir); err == nil {
		t.Fatal("missing env override did not error")
	}

	// The bin/ layout remains a dev fallback.
	t.Setenv(envBinary, "")
	nested := filepath.Join(dir, "bin", binaryName())
	if err := os.MkdirAll(filepath.Dir(nested), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(nested, []byte("bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got, err := LocateBinary(dir); err != nil || got != nested {
		t.Fatalf("nested lookup = %q, %v; want %q", got, err, nested)
	}
}

func TestBackendEnv(t *testing.T) {
	t.Setenv("PORT", "1")
	t.Setenv("DATABASE_URL", "postgres://stale")

	dataDir := filepath.Join(t.TempDir(), "papersage")
	env := backendEnv(6000, dataDir, "papersage")

	values := map[string]string{}
	for _, kv := range env {
		k, v, ok := strings.Cut(kv, "=")
		if !ok {
			continue
		}
		if _, dup := values[k]; dup && (k == "PORT" || k == "DATABASE_URL") {
			t.Fatalf("duplicate %s in child env", k)
		}
		values[k] = v
	}

	want := map[string]string{
		"HOST":               "127.0.0.1",
		"PORT":               "6000",
		"DATABASE_URL":       "sqlite:///" + filepath.ToSlash(filepath.Join(dataDir, "papersage.db")),
		"LATEX_CLSI_ENABLED": "false",
		"RUN_DB_MIGRATIONS":  "true",
	}
	for k, v := range want {
		if values[k] != v {
			t.Errorf("%s = %q, want %q", k, values[k], v)
		}
	}
}

func listenerPort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	_ = l.Close()
	return port
}

func TestBackendEnvBundlesCatalogAndMirror(t *testing.T) {
	t.Setenv("LATEX_RESOURCE_REPOSITORY", "")

	dir := t.TempDir()
	binary := filepath.Join(dir, "papersage")
	if err := os.WriteFile(filepath.Join(dir, "catalog.json.gz"), []byte("catalog"), 0o600); err != nil {
		t.Fatal(err)
	}
	values := map[string]string{}
	for _, kv := range backendEnv(6000, filepath.Join(dir, "data"), binary) {
		k, v, ok := strings.Cut(kv, "=")
		if ok {
			values[k] = v
		}
	}
	if got := values["LATEX_RESOURCE_CATALOG_FILE"]; got != filepath.Join(dir, "catalog.json.gz") {
		t.Errorf("LATEX_RESOURCE_CATALOG_FILE = %q", got)
	}
	if got := values["LATEX_RESOURCE_REPOSITORY"]; !strings.Contains(got, "tsinghua.edu.cn") {
		t.Errorf("LATEX_RESOURCE_REPOSITORY = %q, want the default mirror", got)
	}

	t.Setenv("LATEX_RESOURCE_REPOSITORY", "https://example.org/tlnet")
	values = map[string]string{}
	for _, kv := range backendEnv(6000, filepath.Join(dir, "data"), binary) {
		k, v, ok := strings.Cut(kv, "=")
		if ok {
			values[k] = v
		}
	}
	if got := values["LATEX_RESOURCE_REPOSITORY"]; got != "https://example.org/tlnet" {
		t.Errorf("explicit LATEX_RESOURCE_REPOSITORY was overridden: %q", got)
	}
}
