package installer

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

func TestUninstallPreservesUnmanagedFiles(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate test source")
	}
	nsisDir := filepath.Join(filepath.Dir(filename), "..", "..", "build", "windows", "nsis")
	source, err := os.ReadFile(filepath.Join(nsisDir, "project.nsi"))
	if err != nil {
		t.Fatal(err)
	}
	section := regexp.MustCompile(`(?ims)^[ \t]*Section[ \t]+"uninstall"[ \t]*\r?\n(.*?)^[ \t]*SectionEnd[ \t]*\r?$`).FindAllStringSubmatch(string(source), -1)
	if len(section) != 1 {
		t.Fatalf("expected exactly one uninstall section, found %d", len(section))
	}

	files := map[string]bool{
		`$INSTDIR\${PRODUCT_EXECUTABLE}`:      false,
		`$INSTDIR\backend\papersage.exe`:      false,
		`$INSTDIR\backend\catalog.json.gz`:    false,
		`$SMPROGRAMS\${INFO_PRODUCTNAME}.lnk`: false,
		`$DESKTOP\${INFO_PRODUCTNAME}.lnk`:    false,
	}
	dirs := map[string]bool{`$INSTDIR\backend`: false, `$INSTDIR`: false}
	macros := map[string]bool{
		"wails.setShellContext":            false,
		"wails.unassociateFiles":           false,
		"wails.unassociateCustomProtocols": false,
		"wails.deleteUninstaller":          false,
	}
	uninstallerLine, installDirLine := -1, -1
	for i, line := range strings.Split(section[0][1], "\n") {
		line = strings.TrimSpace(strings.SplitN(line, "#", 2)[0])
		if line == "" || strings.HasPrefix(line, ";") {
			continue
		}
		fields := strings.Fields(line)
		switch strings.ToLower(fields[0]) {
		case "delete":
			if len(fields) != 2 {
				t.Errorf("unexpected file deletion: %s", line)
				continue
			}
			path := strings.Trim(fields[1], `"`)
			if _, ok := files[path]; !ok {
				t.Errorf("unexpected file deletion: %s", line)
			} else {
				files[path] = true
			}
		case "rmdir":
			if len(fields) != 2 {
				t.Errorf("directory removal must be non-recursive: %s", line)
				continue
			}
			path := strings.Trim(fields[1], `"`)
			if _, ok := dirs[path]; !ok {
				t.Errorf("unexpected directory removal: %s", line)
			} else {
				dirs[path] = true
				if path == `$INSTDIR` {
					installDirLine = i
				}
			}
		case "!insertmacro":
			if len(fields) >= 2 {
				if _, ok := macros[fields[1]]; ok {
					macros[fields[1]] = true
				}
				if fields[1] == "wails.deleteUninstaller" {
					uninstallerLine = i
				}
			}
		}
	}
	for path, present := range files {
		if !present {
			t.Errorf("missing managed file deletion: %s", path)
		}
	}
	for path, present := range dirs {
		if !present {
			t.Errorf("missing non-recursive directory removal: %s", path)
		}
	}
	for name, present := range macros {
		if !present {
			t.Errorf("missing cleanup macro: %s", name)
		}
	}
	if uninstallerLine >= 0 && installDirLine >= 0 && uninstallerLine > installDirLine {
		t.Error("installation directory removed before uninstaller cleanup")
	}

	tools, err := os.ReadFile(filepath.Join(nsisDir, "wails_tools.nsh"))
	if err != nil {
		t.Fatal(err)
	}
	uninstaller := regexp.MustCompile(`(?ims)^[ \t]*!macro[ \t]+wails\.deleteUninstaller[ \t]*\r?\n(.*?)^[ \t]*!macroend[ \t]*\r?$`).FindStringSubmatch(string(tools))
	if len(uninstaller) != 2 || !strings.Contains(uninstaller[1], `Delete "$INSTDIR\uninstall.exe"`) {
		t.Error("uninstaller cleanup macro must delete the managed uninstaller")
	}
}
