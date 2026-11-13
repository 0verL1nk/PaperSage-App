// Package backend supervises the bundled PaperSage API server that runs
// alongside the desktop client. It has no Wails dependencies.
package backend

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	defaultPort = 5478

	envBinary = "PAPERSAGE_BACKEND_BIN"

	healthInterval    = 200 * time.Millisecond
	startupTimeout    = 15 * time.Second
	stopTimeout       = 5 * time.Second
	healthHTTPTimeout = 1200 * time.Millisecond
	maxRestarts       = 3
)

var errStopped = errors.New("backend: supervisor stopped")

var healthClient = &http.Client{
	Timeout:   healthHTTPTimeout,
	Transport: &http.Transport{DisableKeepAlives: true},
}

// Supervisor owns one local papersage process. All methods are safe for
// concurrent use, but Start is meant to be called once per app lifecycle.
type Supervisor struct {
	mu sync.Mutex

	dataDir       string
	preferredPort int

	binary string
	port   int

	cmd     *exec.Cmd
	logFile *os.File

	baseURL  string
	external bool // reused an already-running papersage on the preferred port
	stopping bool
	restarts int
	lastErr  error

	done   chan struct{}
	stopCh chan struct{}
}

// New returns a supervisor using the per-user application data directory.
func New() *Supervisor {
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = os.TempDir()
	}
	return newSupervisor(filepath.Join(dir, "papersage"))
}

func newSupervisor(dataDir string) *Supervisor {
	return &Supervisor{
		dataDir:       dataDir,
		preferredPort: defaultPort,
	}
}

// Start locates, launches, and waits for the local backend. It returns the
// loopback base URL the frontend should use.
func (s *Supervisor) Start(ctx context.Context) (string, error) {
	s.mu.Lock()
	if s.baseURL != "" {
		url := s.baseURL
		s.mu.Unlock()
		return url, nil
	}
	binary := s.binary
	s.mu.Unlock()

	if binary == "" {
		exe, err := os.Executable()
		if err != nil {
			return "", fmt.Errorf("locate desktop executable: %w", err)
		}
		binary, err = LocateBinary(filepath.Dir(exe))
		if err != nil {
			return "", err
		}
		s.mu.Lock()
		s.binary = binary
		s.mu.Unlock()
	}

	port, external, err := selectPort(s.preferredPort, probePort)
	if err != nil {
		return "", err
	}
	if external {
		s.mu.Lock()
		s.external = true
		s.baseURL = baseURL(port)
		s.mu.Unlock()
		return baseURL(port), nil
	}

	if err := os.MkdirAll(filepath.Join(s.dataDir, "logs"), 0o755); err != nil {
		return "", fmt.Errorf("create local data directory: %w", err)
	}

	s.mu.Lock()
	s.port = port
	s.stopping = false
	s.restarts = 0
	s.lastErr = nil
	s.done = make(chan struct{})
	s.stopCh = make(chan struct{})
	done, stopCh := s.done, s.stopCh
	s.mu.Unlock()

	if _, err := s.launch(port); err != nil {
		s.mu.Lock()
		s.done = nil
		s.stopCh = nil
		if s.logFile != nil {
			_ = s.logFile.Close()
			s.logFile = nil
		}
		s.mu.Unlock()
		return "", err
	}
	go s.monitor(done, stopCh)

	if err := s.waitHealthy(ctx, port, stopCh, startupTimeout); err != nil {
		_ = s.Stop()
		return "", fmt.Errorf("local backend did not become healthy: %w", err)
	}

	url := baseURL(port)
	s.mu.Lock()
	s.baseURL = url
	s.mu.Unlock()
	return url, nil
}

// Stop asks the child to exit, force-kills it after stopTimeout, and waits
// for it to be reaped. A reused external backend is left running.
func (s *Supervisor) Stop() error {
	s.mu.Lock()
	if s.external {
		s.external = false
		s.baseURL = ""
		s.mu.Unlock()
		return nil
	}
	if s.cmd == nil && s.done == nil {
		s.baseURL = ""
		s.mu.Unlock()
		return nil
	}
	// cmd may be nil between restart attempts; stopping and stopCh still
	// guarantee the monitor goroutine exits without launching again.
	s.stopping = true
	if s.stopCh != nil {
		close(s.stopCh)
		s.stopCh = nil
	}
	cmd, done := s.cmd, s.done
	s.mu.Unlock()

	var killErr error
	if cmd != nil {
		if err := cmd.Process.Signal(os.Interrupt); err != nil {
			// Windows does not implement os.Interrupt; terminate instead.
			if err := cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
				killErr = err
			}
		}
	}
	if done != nil {
		select {
		case <-done:
		case <-time.After(stopTimeout):
			if cmd != nil {
				_ = cmd.Process.Kill()
			}
			<-done
		}
	}

	s.mu.Lock()
	if s.logFile != nil {
		_ = s.logFile.Close()
		s.logFile = nil
	}
	s.cmd = nil
	s.done = nil
	s.baseURL = ""
	s.stopping = false
	s.restarts = 0
	s.mu.Unlock()
	return killErr
}

// BaseURL returns the loopback address of the managed backend, or "".
func (s *Supervisor) BaseURL() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.baseURL
}

// Healthy reports whether the backend currently answers its health endpoint.
func (s *Supervisor) Healthy() bool {
	s.mu.Lock()
	url := s.baseURL
	s.mu.Unlock()
	return url != "" && probeHealth(url)
}

// LastError returns the most recent process or restart failure.
func (s *Supervisor) LastError() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastErr
}

func (s *Supervisor) launch(port int) (*exec.Cmd, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.logFile == nil {
		f, err := os.OpenFile(filepath.Join(s.dataDir, "logs", "backend.log"),
			os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			return nil, fmt.Errorf("open backend log: %w", err)
		}
		s.logFile = f
	}

	cmd := exec.Command(s.binary)
	cmd.Env = backendEnv(port, s.dataDir, s.binary)
	cmd.Stdout = s.logFile
	cmd.Stderr = s.logFile
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start local backend %s: %w", s.binary, err)
	}
	s.cmd = cmd
	return cmd, nil
}

// monitor waits for the child and restarts it with capped backoff after an
// unexpected exit. Restarts are cumulative for one Start/Stop cycle.
func (s *Supervisor) monitor(done chan struct{}, stop <-chan struct{}) {
	defer close(done)

	for {
		s.mu.Lock()
		cmd := s.cmd
		port := s.port
		s.mu.Unlock()
		if cmd == nil {
			return
		}

		waitErr := cmd.Wait()

		s.mu.Lock()
		if s.cmd == cmd {
			s.cmd = nil
		}
		if waitErr != nil {
			s.lastErr = waitErr
		}
		if s.stopping || s.external {
			s.mu.Unlock()
			return
		}
		attempt := s.restarts
		s.mu.Unlock()

		restarted := false
		for attempt < maxRestarts {
			attempt++
			select {
			case <-time.After(time.Duration(1<<(attempt-1)) * time.Second):
			case <-stop:
				return
			}

			s.mu.Lock()
			if s.stopping {
				s.mu.Unlock()
				return
			}
			s.restarts = attempt
			s.mu.Unlock()

			newCmd, err := s.launch(port)
			if err != nil {
				s.mu.Lock()
				s.lastErr = err
				s.mu.Unlock()
				continue
			}
			if err := s.waitHealthy(context.Background(), port, stop, startupTimeout); err != nil {
				s.mu.Lock()
				s.lastErr = err
				if s.cmd == newCmd {
					s.cmd = nil
				}
				s.mu.Unlock()
				_ = newCmd.Process.Kill()
				_ = newCmd.Wait()
				continue
			}
			restarted = true
			break
		}

		if !restarted {
			s.mu.Lock()
			s.baseURL = ""
			s.lastErr = fmt.Errorf("local backend exited (%v); restart limit (%d) reached", waitErr, maxRestarts)
			s.mu.Unlock()
			return
		}
	}
}

func (s *Supervisor) waitHealthy(ctx context.Context, port int, stop <-chan struct{}, timeout time.Duration) error {
	url := baseURL(port)
	deadline := time.Now().Add(timeout)
	for {
		if probeHealth(url) {
			return nil
		}
		if !time.Now().Before(deadline) {
			return fmt.Errorf("no healthy response from %s within %s", url, timeout)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-stop:
			return errStopped
		case <-time.After(healthInterval):
		}
	}
}

// LocateBinary resolves the bundled server binary. The installer places it in
// <exe_dir>/backend/; the desktop shell itself is named papersage, so the
// executable directory is deliberately not searched. Callers never supply a
// path from the frontend.
func LocateBinary(exeDir string) (string, error) {
	backendDir := filepath.Join(exeDir, "backend", binaryName())
	if checkExecutable(backendDir) == nil {
		return backendDir, nil
	}
	if p := strings.TrimSpace(os.Getenv(envBinary)); p != "" {
		if err := checkExecutable(p); err != nil {
			return "", fmt.Errorf("%s=%q: %w", envBinary, p, err)
		}
		return p, nil
	}
	if dev := filepath.Join(exeDir, "bin", binaryName()); checkExecutable(dev) == nil {
		return dev, nil
	}
	return "", fmt.Errorf("papersage backend binary not found in %s; reinstall the desktop client or set %s",
		filepath.Join(exeDir, "backend"), envBinary)
}

func checkExecutable(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return fmt.Errorf("%s is a directory", path)
	}
	return nil
}

func binaryName() string {
	if runtime.GOOS == "windows" {
		return "papersage.exe"
	}
	return "papersage"
}

func baseURL(port int) string {
	return "http://127.0.0.1:" + strconv.Itoa(port)
}

// selectPort prefers the default port. A healthy papersage already there is
// reused; any other occupant forces a free loopback port.
func selectPort(preferred int, papersage func(int) bool) (port int, external bool, err error) {
	if papersage(preferred) {
		return preferred, true, nil
	}
	if portFree(preferred) {
		return preferred, false, nil
	}
	port, err = freePort()
	if err != nil {
		return 0, false, fmt.Errorf("choose free loopback port: %w", err)
	}
	return port, false, nil
}

func portFree(port int) bool {
	l, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		return false
	}
	_ = l.Close()
	return true
}

func freePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

func probePort(port int) bool {
	return probeHealth(baseURL(port))
}

// probeHealth identifies a PaperSage server. Loopback /health includes build
// metadata, so a bare status is not enough to distinguish another service.
func probeHealth(base string) bool {
	resp, err := healthClient.Get(base + "/health")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return false
	}
	var h struct {
		Status    string `json:"status"`
		Version   string `json:"version"`
		GoVersion string `json:"goVersion"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4096)).Decode(&h); err != nil {
		return false
	}
	return h.Status == "healthy" && (h.Version != "" || h.GoVersion != "")
}

func backendEnv(port int, dataDir, binary string) []string {
	db := filepath.ToSlash(filepath.Join(dataDir, "papersage.db"))
	overrides := map[string]string{
		"HOST":               "127.0.0.1",
		"PORT":               strconv.Itoa(port),
		"DATABASE_URL":       "sqlite:///" + db,
		"LATEX_CLSI_ENABLED": "false",
		"RUN_DB_MIGRATIONS":  "true",
	}
	// The desktop build ships a catalog next to the backend so TeX resources
	// resolve without object storage.
	catalog := filepath.Join(filepath.Dir(binary), "catalog.json.gz")
	if info, err := os.Stat(catalog); err == nil && !info.IsDir() {
		overrides["LATEX_RESOURCE_CATALOG_FILE"] = catalog
	}
	// mirror.ctan.org round-robins across mirrors that rate-limit archive
	// downloads; a stable mirror keeps on-demand package fetches reliable.
	if os.Getenv("LATEX_RESOURCE_REPOSITORY") == "" {
		overrides["LATEX_RESOURCE_REPOSITORY"] = "https://mirrors.tuna.tsinghua.edu.cn/CTAN/systems/texlive/tlnet"
	}
	return withEnv(overrides)
}

// withEnv inherits the process environment with the given keys replaced, so
// the child cannot pick up a stale PORT or DATABASE_URL from the desktop env.
func withEnv(overrides map[string]string) []string {
	base := os.Environ()
	out := make([]string, 0, len(base)+len(overrides))
	for _, kv := range base {
		key, _, ok := strings.Cut(kv, "=")
		if !ok {
			out = append(out, kv)
			continue
		}
		if _, replaced := overrides[key]; !replaced {
			out = append(out, kv)
		}
	}
	for k, v := range overrides {
		out = append(out, k+"="+v)
	}
	return out
}
