package backend

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
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

func TestWaitHealthyVersion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"status":"healthy","version":"1.2.3","goVersion":"go1.26"}`))
	}))
	defer server.Close()
	port := server.Listener.Addr().(*net.TCPAddr).Port

	old := ExpectedVersion
	defer func() { ExpectedVersion = old }()

	s := &Supervisor{}
	ExpectedVersion = "1.2.3"
	if err := s.waitHealthy(context.Background(), port, nil, time.Second); err != nil {
		t.Fatalf("matching version: %v", err)
	}

	ExpectedVersion = "2.0.0"
	err := s.waitHealthy(context.Background(), port, nil, time.Second)
	if err == nil || !strings.Contains(err.Error(), "2.0.0") || !strings.Contains(err.Error(), "1.2.3") {
		t.Fatalf("mismatched version = %v, want an error naming both versions", err)
	}

	ExpectedVersion = ""
	if err := s.waitHealthy(context.Background(), port, nil, time.Second); err != nil {
		t.Fatalf("empty expected version: %v", err)
	}
}

func TestStartExternalBackendVersion(t *testing.T) {
	previous := ExpectedVersion
	t.Cleanup(func() { ExpectedVersion = previous })

	for _, tc := range []struct {
		name, expected, actual string
		wantError              bool
	}{
		{name: "matching version", expected: "v1.2.3", actual: "1.2.3"},
		{name: "matching prefixed actual", expected: "1.2.3", actual: "v1.2.3"},
		{name: "mismatched version", expected: "2.0.0", actual: "1.2.3", wantError: true},
		{name: "empty expected", expected: "", actual: "1.2.3"},
		{name: "empty actual", expected: "2.0.0", actual: "", wantError: true},
		{name: "different development build", expected: "2.0.0", actual: "dev", wantError: true},
		{name: "matching development build", expected: "dev", actual: "dev"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ExpectedVersion = tc.expected
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(`{"status":"healthy","version":"` + tc.actual + `","goVersion":"go1.26"}`))
			}))
			defer server.Close()

			s := newSupervisor(t.TempDir())
			s.binary = "unused"
			s.preferredPort = server.Listener.Addr().(*net.TCPAddr).Port
			url, err := s.Start(context.Background())
			if tc.wantError {
				if err == nil || !strings.Contains(err.Error(), tc.expected) || !strings.Contains(err.Error(), tc.actual) {
					t.Fatalf("Start = %q, %v; want error naming both versions", url, err)
				}
				if tc.actual == "" && !strings.Contains(err.Error(), "实际 （空）") {
					t.Fatalf("Start returned unclear empty-version error: %v", err)
				}
				if url != "" || s.BaseURL() != "" {
					t.Fatalf("mismatched external backend was accepted: url=%q baseURL=%q", url, s.BaseURL())
				}
				return
			}
			if err != nil || url != server.URL || s.BaseURL() != server.URL {
				t.Fatalf("Start = %q, %v, baseURL=%q; want %q", url, err, s.BaseURL(), server.URL)
			}
			if err := s.Stop(); err != nil {
				t.Fatalf("Stop: %v", err)
			}
			if !probeHealth(server.URL) {
				t.Fatal("Stop affected the external backend")
			}
		})
	}
}

func TestStartValidatesBackendSHA256BeforeLaunch(t *testing.T) {
	previous := ExpectedBackendSHA256
	t.Cleanup(func() { ExpectedBackendSHA256 = previous })

	dir := t.TempDir()
	binary := filepath.Join(dir, "backend")
	contents := []byte("this is a test fixture, not an executable")
	if err := os.WriteFile(binary, contents, 0o600); err != nil {
		t.Fatal(err)
	}
	// Blocking the log directory ensures none of these cases execute the fixture.
	dataDir := filepath.Join(dir, "not-a-directory")
	if err := os.WriteFile(dataDir, []byte("blocker"), 0o600); err != nil {
		t.Fatal(err)
	}
	foreign := httptest.NewServer(http.NotFoundHandler())
	defer foreign.Close()
	port := foreign.Listener.Addr().(*net.TCPAddr).Port
	actual := sha256.Sum256(contents)
	correct := fmt.Sprintf("%x", actual)

	for _, tc := range []struct {
		name, expected, path, wantError string
	}{
		{name: "matching digest", expected: correct, path: binary, wantError: "create local data directory"},
		{name: "mismatched digest", expected: strings.Repeat("0", 64), path: binary, wantError: "SHA-256 不匹配"},
		{name: "missing binary", expected: correct, path: filepath.Join(dir, "missing"), wantError: "无法读取"},
		{name: "unreadable binary", expected: correct, path: dir, wantError: "无法读取"},
		{name: "invalid digest length", expected: "1234", path: binary, wantError: "格式无效"},
		{name: "invalid digest characters", expected: strings.Repeat("z", 64), path: binary, wantError: "格式无效"},
		{name: "empty digest skips verification", expected: "", path: filepath.Join(dir, "missing"), wantError: "create local data directory"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ExpectedBackendSHA256 = tc.expected
			s := newSupervisor(dataDir)
			s.binary = tc.path
			s.preferredPort = port
			url, err := s.Start(context.Background())
			if err == nil || !strings.Contains(err.Error(), tc.wantError) {
				t.Fatalf("Start = %q, %v; want error containing %q", url, err, tc.wantError)
			}
			if tc.expected != "" && tc.wantError != "create local data directory" && !strings.Contains(err.Error(), "请重新安装") {
				t.Fatalf("integrity error does not advise reinstalling: %v", err)
			}
			if url != "" || s.BaseURL() != "" {
				t.Fatalf("Start exposed a backend: url=%q baseURL=%q", url, s.BaseURL())
			}
		})
	}
}

func TestLaunchChecksBackendSHA256BeforeRestart(t *testing.T) {
	previous := ExpectedBackendSHA256
	t.Cleanup(func() { ExpectedBackendSHA256 = previous })
	ExpectedBackendSHA256 = strings.Repeat("0", 64)

	dir := t.TempDir()
	binary := filepath.Join(dir, "backend")
	if err := os.WriteFile(binary, []byte("not an executable"), 0o600); err != nil {
		t.Fatal(err)
	}
	// No log directory exists, so this test cannot execute the fixture.
	s := newSupervisor(filepath.Join(dir, "missing-data-directory"))
	s.binary = binary
	_, err := s.launch(0)
	if err == nil || !strings.Contains(err.Error(), "SHA-256 不匹配") || !strings.Contains(err.Error(), "请重新安装") {
		t.Fatalf("launch = %v; want integrity error before restart", err)
	}
}

func TestMonitorKeepsIntegrityError(t *testing.T) {
	if os.Getenv("PAPERSAGE_TEST_MONITOR_CHILD") == "1" {
		return
	}
	previous := ExpectedBackendSHA256
	t.Cleanup(func() { ExpectedBackendSHA256 = previous })
	ExpectedBackendSHA256 = strings.Repeat("0", 64)

	dir := t.TempDir()
	binary := filepath.Join(dir, "backend")
	if err := os.WriteFile(binary, []byte("not an executable"), 0o600); err != nil {
		t.Fatal(err)
	}
	// Wait on a test process; the backend fixture itself must never execute.
	cmd := exec.Command(os.Args[0], "-test.run=^TestMonitorKeepsIntegrityError$")
	cmd.Env = append(os.Environ(), "PAPERSAGE_TEST_MONITOR_CHILD=1")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	s := newSupervisor(dir)
	s.binary, s.cmd = binary, cmd
	s.restarts = maxRestarts - 1
	s.baseURL = "http://127.0.0.1:1"
	s.monitor(make(chan struct{}), make(chan struct{}))
	if err := s.LastError(); err == nil || !strings.Contains(err.Error(), "SHA-256 不匹配") || !strings.Contains(err.Error(), "请重新安装") {
		t.Fatalf("LastError = %v; want the integrity failure and reinstall guidance", err)
	}
	if s.BaseURL() != "" {
		t.Fatalf("backend URL remained available after failed restart: %q", s.BaseURL())
	}
}

func TestStartExternalBackendSkipsBinaryHash(t *testing.T) {
	previousHash, previousVersion := ExpectedBackendSHA256, ExpectedVersion
	t.Cleanup(func() {
		ExpectedBackendSHA256, ExpectedVersion = previousHash, previousVersion
	})
	ExpectedBackendSHA256, ExpectedVersion = strings.Repeat("0", 64), "1.2.3"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"status":"healthy","version":"1.2.3","goVersion":"go1.26"}`))
	}))
	defer server.Close()
	s := newSupervisor(t.TempDir())
	s.binary = filepath.Join(t.TempDir(), "missing")
	s.preferredPort = server.Listener.Addr().(*net.TCPAddr).Port
	url, err := s.Start(context.Background())
	if err != nil || url != server.URL {
		t.Fatalf("Start = %q, %v; want external backend %q", url, err, server.URL)
	}
	if err := s.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if !probeHealth(server.URL) {
		t.Fatal("Stop affected the external backend")
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
