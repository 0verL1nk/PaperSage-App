package installer

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

// BLOCKED: the elevated NSIS update transaction is not shipped because
// user-writable install roots cannot be proven safe against reparent/reparse
// races. These contract tests stay skipped until a handle-relative
// installer is implemented and validated in an isolated Windows VM.
// These checks inspect the real installer script without executing an installer.
// Runtime failure injection is still required to verify Windows/NSIS behavior.
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

func updateFunction(t *testing.T, script, name string) string {
	t.Helper()
	pattern := `(?ims)^[ \t]*Function[ \t]+` + regexp.QuoteMeta(name) + `[ \t]*\r?\n(.*?)^[ \t]*FunctionEnd[ \t]*\r?$`
	match := regexp.MustCompile(pattern).FindStringSubmatch(script)
	if len(match) != 2 {
		t.Errorf("missing installer function %s", name)
		return ""
	}
	return match[1]
}

func TestUpdateHandshakeBeforeFileChanges(t *testing.T) {
	t.Skip("blocked: elevated NSIS update transaction not implemented")
	script := updateScript(t)
	init := updateFunction(t, script, ".onInit")
	if !strings.Contains(init, "Call UpdateHandshake") {
		t.Error("installer initialization must perform the update handoff")
	}
	handshake := updateFunction(t, script, "UpdateHandshake")
	for _, required := range []string{
		"/PAPERSAGE_PARENT_PID=", "/PAPERSAGE_HANDOFF=",
		"OpenProcess", "OpenEventW", "SetEvent", "WaitForSingleObject", "CloseHandle",
		"-ready", "-commit",
	} {
		if !strings.Contains(handshake, required) {
			t.Errorf("handoff missing %q", required)
		}
	}
	ready := strings.Index(handshake, "SetEvent")
	wait := strings.Index(handshake, "WaitForSingleObject")
	if ready < 0 || wait <= ready || strings.Count(handshake, "WaitForSingleObject") < 2 {
		t.Error("ready must precede bounded waits for both commit and parent exit")
	}
}

func TestUpdateRollbackRestoresEveryManagedFile(t *testing.T) {
	t.Skip("blocked: elevated NSIS update transaction not implemented")
	script := updateScript(t)
	rollback := updateFunction(t, script, "UpdateRollback")
	for _, path := range []string{
		`$INSTDIR\${PRODUCT_EXECUTABLE}`,
		`$INSTDIR\backend\papersage.exe`,
		`$INSTDIR\backend\catalog.json.gz`,
		`$INSTDIR\uninstall.exe`,
	} {
		if !strings.Contains(rollback, `"`+path+`"`) {
			t.Errorf("rollback does not restore managed destination %s", path)
		}
	}
	if !strings.Contains(rollback, "RestoreManaged") {
		t.Error("rollback must restore the old files without overwriting existing destinations")
	}
	if strings.Contains(strings.ToLower(rollback), "rmdir /r") {
		t.Error("rollback must not recursively remove application or user files")
	}
}
