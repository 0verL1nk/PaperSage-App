package installer

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// Contract checks for the installer script that the desktop updater runs. They
// inspect the real NSIS script without executing an installer.
func updateScript(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate test source")
	}
	path := filepath.Join(filepath.Dir(filename), "..", "..", "build", "windows", "nsis", "project.nsi")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var lines []string
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "#") && !strings.HasPrefix(trimmed, ";") {
			lines = append(lines, line)
		}
	}
	return strings.Join(lines, "\n")
}

// The desktop auto-updater hands the verified installer to an elevated process,
// which shows one UAC prompt. Switching the installer to per-user scope would
// remove that prompt and must be a deliberate, reviewed change.
func TestUpdateInstallerKeepsElevatedMachineScope(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate test source")
	}
	root := filepath.Join(filepath.Dir(filename), "..", "..", "build", "windows", "nsis")
	tools, err := os.ReadFile(filepath.Join(root, "wails_tools.nsh"))
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		`!define WAILS_INSTALL_SCOPE "machine"`,
		`!define REQUEST_EXECUTION_LEVEL "admin"`,
	} {
		if !strings.Contains(string(tools), required) {
			t.Errorf("wails_tools.nsh no longer defaults to %q", required)
		}
	}
	project, err := os.ReadFile(filepath.Join(root, "project.nsi"))
	if err != nil {
		t.Fatal(err)
	}
	script := updateScript(t)
	if strings.Contains(script, `WAILS_INSTALL_SCOPE "user"`) || strings.Contains(script, `REQUEST_EXECUTION_LEVEL "user"`) {
		t.Error("project.nsi switched to per-user scope without revisiting the auto-update UAC contract")
	}
	if !strings.Contains(string(project), `${INFO_PROJECTNAME}-${ARCH}-installer.exe`) {
		t.Error("installer output name changed; the updater selects papersage-windows-amd64-installer.exe")
	}
}
