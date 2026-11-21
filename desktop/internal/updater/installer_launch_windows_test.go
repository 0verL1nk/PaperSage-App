//go:build windows

package updater

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows"
)

type helperFixture struct {
	stagingDir string
	stagedPath string
	appExe     string
	content    []byte
	digestHex  string
}

func newHelperFixture(t *testing.T) helperFixture {
	t.Helper()
	root := t.TempDir()
	installDir := filepath.Join(root, "Paper Sage with spaces")
	stagingDir := filepath.Join(root, ".papersage-update-fixture")
	for _, dir := range []string{installDir, stagingDir} {
		if err := os.Mkdir(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	appExe := filepath.Join(installDir, "Paper Sage.exe")
	for path, content := range map[string][]byte{
		appExe: []byte("dummy app"),
		filepath.Join(installDir, "uninstall.exe"): []byte("dummy uninstaller"),
	} {
		if err := os.WriteFile(path, content, 0600); err != nil {
			t.Fatal(err)
		}
	}
	content := []byte("dummy installer; never execute")
	stagedPath := filepath.Join(stagingDir, installerAssetName)
	if err := os.WriteFile(stagedPath, content, 0600); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(content)
	return helperFixture{
		stagingDir: stagingDir,
		stagedPath: stagedPath,
		appExe:     appExe,
		content:    content,
		digestHex:  hex.EncodeToString(sum[:]),
	}
}

func (f helperFixture) deps() (updateHelperDeps, *helperCalls) {
	calls := &helperCalls{}
	return updateHelperDeps{
		waitParent: func() { calls.order = append(calls.order, "wait-parent") },
		launchElevated: func(path, parameters string) (windows.Handle, error) {
			calls.order = append(calls.order, "launch")
			calls.path, calls.parameters = path, parameters
			return windows.Handle(1), nil
		},
		waitProcess: func(windows.Handle) (uint32, error) {
			calls.order = append(calls.order, "wait")
			return 0, nil
		},
		launchApp: func(path string) error {
			calls.order = append(calls.order, "restart")
			calls.appExe = path
			return nil
		},
		removeDir: func(path string) error {
			calls.removed = path
			return nil
		},
	}, calls
}

type helperCalls struct {
	order      []string
	path       string
	parameters string
	appExe     string
	removed    string
}

func TestApplyUpdateInstallsAndRestarts(t *testing.T) {
	fixture := newHelperFixture(t)
	deps, calls := fixture.deps()
	if err := applyUpdate(fixture.stagedPath, fixture.digestHex, fixture.appExe, deps); err != nil {
		t.Fatalf("applyUpdate() error = %v", err)
	}
	want := []string{"wait-parent", "launch", "wait", "restart"}
	if len(calls.order) != len(want) {
		t.Fatalf("call order = %v, want %v", calls.order, want)
	}
	for i := range want {
		if calls.order[i] != want[i] {
			t.Fatalf("call order = %v, want %v", calls.order, want)
		}
	}
	if calls.path != fixture.stagedPath {
		t.Fatalf("installer path = %q", calls.path)
	}
	if calls.parameters != `/S /D=`+filepath.Dir(fixture.appExe) {
		t.Fatalf("installer parameters = %q", calls.parameters)
	}
	if calls.appExe != fixture.appExe || calls.removed != fixture.stagingDir {
		t.Fatalf("restart = %q, removed = %q", calls.appExe, calls.removed)
	}
}

func TestApplyUpdateRejectsChecksumMismatchBeforeLaunching(t *testing.T) {
	fixture := newHelperFixture(t)
	deps, calls := fixture.deps()
	launched := false
	deps.launchElevated = func(string, string) (windows.Handle, error) {
		launched = true
		return 0, nil
	}
	wrong := make([]byte, sha256.Size)
	if err := applyUpdate(fixture.stagedPath, hex.EncodeToString(wrong), fixture.appExe, deps); err == nil {
		t.Fatal("applyUpdate() accepted a wrong checksum")
	}
	if launched {
		t.Fatal("installer launched despite a checksum mismatch")
	}
	if calls.appExe != fixture.appExe {
		t.Fatalf("application not restarted after a failed verification: %q", calls.appExe)
	}
	if _, err := os.Stat(fixture.stagedPath); err != nil {
		t.Fatalf("staged installer removed after a failed verification: %v", err)
	}
}

func TestApplyUpdateRestartsAfterUACDenial(t *testing.T) {
	fixture := newHelperFixture(t)
	deps, calls := fixture.deps()
	deps.launchElevated = func(string, string) (windows.Handle, error) {
		return 0, windows.ERROR_CANCELLED
	}
	if err := applyUpdate(fixture.stagedPath, fixture.digestHex, fixture.appExe, deps); !errors.Is(err, windows.ERROR_CANCELLED) {
		t.Fatalf("applyUpdate() error = %v, want cancellation", err)
	}
	if calls.appExe != fixture.appExe {
		t.Fatalf("application not restarted after UAC denial: %q", calls.appExe)
	}
	if calls.removed != "" {
		t.Fatalf("staging directory removed after UAC denial: %q", calls.removed)
	}
}

func TestApplyUpdateRestartsAfterInstallerFailure(t *testing.T) {
	fixture := newHelperFixture(t)
	deps, calls := fixture.deps()
	deps.waitProcess = func(windows.Handle) (uint32, error) { return 5, nil }
	if err := applyUpdate(fixture.stagedPath, fixture.digestHex, fixture.appExe, deps); err == nil {
		t.Fatal("applyUpdate() accepted a failing installer")
	}
	if calls.appExe != fixture.appExe {
		t.Fatalf("application not restarted after installer failure: %q", calls.appExe)
	}
	if calls.removed != "" {
		t.Fatalf("staging directory removed after installer failure: %q", calls.removed)
	}
}

func TestApplyUpdateDoesNotRestartWhileInstallerMayRun(t *testing.T) {
	fixture := newHelperFixture(t)
	deps, calls := fixture.deps()
	deps.waitProcess = func(windows.Handle) (uint32, error) { return 0, errors.New("wait failed") }
	if err := applyUpdate(fixture.stagedPath, fixture.digestHex, fixture.appExe, deps); err == nil {
		t.Fatal("applyUpdate() accepted a failed wait")
	}
	if calls.appExe != "" {
		t.Fatalf("application restarted while the installer may still be running: %q", calls.appExe)
	}
	if calls.removed != "" {
		t.Fatalf("staging directory removed while the installer may still be running: %q", calls.removed)
	}
}

func TestApplyUpdateRestartsOnPreflightFailure(t *testing.T) {
	fixture := newHelperFixture(t)
	deps, calls := fixture.deps()
	if err := applyUpdate(fixture.stagedPath, "not-a-digest", fixture.appExe, deps); err == nil {
		t.Fatal("applyUpdate() accepted an invalid digest")
	}
	if calls.appExe != fixture.appExe {
		t.Fatalf("application not restarted after a preflight failure: %q", calls.appExe)
	}
}

func TestApplyUpdatePinsInstallerWhileInstalling(t *testing.T) {
	fixture := newHelperFixture(t)
	deps, _ := fixture.deps()
	remove := func() error {
		err := os.Remove(fixture.stagedPath)
		if err == nil {
			t.Error("staged installer could be removed while the installer runs")
		}
		return err
	}
	deps.launchElevated = func(path, _ string) (windows.Handle, error) {
		if path != fixture.stagedPath {
			t.Fatalf("launch path = %q", path)
		}
		_ = remove()
		return windows.Handle(1), nil
	}
	deps.waitProcess = func(windows.Handle) (uint32, error) {
		_ = remove()
		return 0, nil
	}
	if err := applyUpdate(fixture.stagedPath, fixture.digestHex, fixture.appExe, deps); err != nil {
		t.Fatalf("applyUpdate() error = %v", err)
	}
	// Once the installer has exited the pin is released, so cleanup works.
	if err := os.Remove(fixture.stagedPath); err != nil {
		t.Fatalf("installer still pinned after applyUpdate returned: %v", err)
	}
}

func TestApplyUpdateRejectsUnsafeInputs(t *testing.T) {
	fixture := newHelperFixture(t)
	tests := map[string]struct {
		path   string
		digest string
		appExe string
	}{
		"missing digest":    {path: fixture.stagedPath, digest: "", appExe: fixture.appExe},
		"wrong installer":   {path: fixture.stagedPath, digest: fixture.digestHex, appExe: filepath.Join(filepath.Dir(fixture.appExe), "missing.exe")},
		"wrong file name":   {path: fixture.appExe, digest: fixture.digestHex, appExe: fixture.appExe},
		"uninstaller moved": {path: fixture.stagedPath, digest: fixture.digestHex, appExe: fixture.appExe},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			if name == "uninstaller moved" {
				uninstaller := filepath.Join(filepath.Dir(fixture.appExe), "uninstall.exe")
				renamed := uninstaller + ".bak"
				if err := os.Rename(uninstaller, renamed); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = os.Rename(renamed, uninstaller) })
			}
			deps, _ := fixture.deps()
			launched := false
			deps.launchElevated = func(string, string) (windows.Handle, error) {
				launched = true
				return 0, nil
			}
			if err := applyUpdate(tt.path, tt.digest, tt.appExe, deps); err == nil {
				t.Fatal("applyUpdate() accepted unsafe input")
			}
			if launched {
				t.Fatal("installer launched for unsafe input")
			}
		})
	}
}

func TestValidateInstalledAppRejectsReparseUninstaller(t *testing.T) {
	fixture := newHelperFixture(t)
	uninstaller := filepath.Join(filepath.Dir(fixture.appExe), "uninstall.exe")
	if err := os.Remove(uninstaller); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(fixture.appExe, uninstaller); err != nil {
		t.Skipf("cannot create a test symlink: %v", err)
	}
	if _, err := validateInstalledApp(fixture.appExe); err == nil {
		t.Fatal("validateInstalledApp() accepted a reparse-point uninstaller")
	}
}

func TestRunUpdateHelperArguments(t *testing.T) {
	if handled, err := RunUpdateHelper([]string{"--other"}); handled || err != nil {
		t.Fatalf("RunUpdateHelper(other) = (%t, %v)", handled, err)
	}
	if handled, err := RunUpdateHelper([]string{updateHelperFlag, "a"}); !handled || err == nil {
		t.Fatalf("RunUpdateHelper(short) = (%t, %v), want handled with an error", handled, err)
	}
}

func TestValidInstallerPath(t *testing.T) {
	if !validInstallerPath(`E:\Program Files\PaperSage\uninstall.exe`) {
		t.Fatal("validInstallerPath rejected a normal absolute path")
	}
	for _, path := range []string{"", "relative.exe", `\\?\E:\x.exe`, `E:\dir\`, `E:\dir\trailing.`, `E:\dir\bad|name.exe`} {
		if validInstallerPath(path) {
			t.Fatalf("validInstallerPath(%q) = true", path)
		}
	}
}
