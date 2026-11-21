package installer

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"text/template"

	"gopkg.in/yaml.v3"
)

func metadataFile(t *testing.T, path string) []byte {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate test source")
	}
	root := filepath.Join(filepath.Dir(filename), "..", "..", "..")
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(path)))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func renderMetadataTemplate(t *testing.T, source string, values map[string]string) string {
	t.Helper()
	tmpl, err := template.New("build").Option("missingkey=error").Funcs(template.FuncMap{
		"default": func(fallback, value string) string {
			if value == "" {
				return fallback
			}
			return value
		},
	}).Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := tmpl.Execute(&out, values); err != nil {
		t.Fatal(err)
	}
	return out.String()
}

func TestWindowsBuildEmbedsBackendChecksum(t *testing.T) {
	var taskfile struct {
		Tasks struct {
			Native struct {
				Vars struct {
					BuildFlags    string `yaml:"BUILD_FLAGS"`
					BackendSHA256 string `yaml:"BACKEND_SHA256"`
				} `yaml:"vars"`
				Preconditions []struct {
					Shell string `yaml:"sh"`
				} `yaml:"preconditions"`
			} `yaml:"build:native"`
		} `yaml:"tasks"`
	}
	if err := yaml.Unmarshal(metadataFile(t, "desktop/build/windows/Taskfile.yml"), &taskfile); err != nil {
		t.Fatal(err)
	}
	checksum := strings.Repeat("a", 64)
	if taskfile.Tasks.Native.Vars.BackendSHA256 == "" {
		t.Fatal("missing BACKEND_SHA256 task variable")
	}
	for _, value := range []string{"", checksum} {
		got := renderMetadataTemplate(t, taskfile.Tasks.Native.Vars.BackendSHA256, map[string]string{"BACKEND_SHA256": value})
		if got != value {
			t.Errorf("BACKEND_SHA256 = %q, want %q", got, value)
		}
	}
	for _, dev := range []string{"true", "false"} {
		t.Run("DEV="+dev, func(t *testing.T) {
			values := map[string]string{
				"DEV": dev, "EXTRA_TAGS": "", "OBFUSCATED": "false",
				"VERSION": "1.2.3", "BACKEND_VERSION": "1.2.3", "BACKEND_SHA256": checksum,
			}
			flags := renderMetadataTemplate(t, taskfile.Tasks.Native.Vars.BuildFlags, values)
			ldflags := regexp.MustCompile(`-ldflags="([^"]*)"`).FindStringSubmatch(flags)
			if len(ldflags) != 2 {
				t.Fatalf("missing -ldflags in %s", flags)
			}
			for _, want := range []string{
				"-X github.com/0verL1nk/GraduateProject/desktop/internal/backend.ExpectedVersion=1.2.3",
				"-X github.com/0verL1nk/GraduateProject/desktop/internal/backend.ExpectedBackendSHA256=" + checksum,
			} {
				if !strings.Contains(ldflags[1], want) {
					t.Errorf("rendered linker flags missing %q: %s", want, flags)
				}
			}
		})
	}

	var checksumCheck string
	for _, check := range taskfile.Tasks.Native.Preconditions {
		if strings.Contains(check.Shell, ".BACKEND_SHA256") {
			checksumCheck = check.Shell
		}
	}
	if checksumCheck == "" {
		t.Error("production builds must reject an empty BACKEND_SHA256")
		return
	}
	for _, tc := range []struct{ dev, checksum, want string }{
		{"true", "", "true"},
		{"false", "", `[ -n "" ]`},
		{"false", checksum, `[ -n "` + checksum + `" ]`},
	} {
		got := renderMetadataTemplate(t, checksumCheck, map[string]string{"DEV": tc.dev, "BACKEND_SHA256": tc.checksum})
		if got != tc.want {
			t.Errorf("checksum precondition DEV=%s checksum=%q: got %q, want %q", tc.dev, tc.checksum, got, tc.want)
		}
	}
}

type desktopWorkflowStep struct {
	Name  string            `yaml:"name"`
	If    string            `yaml:"if"`
	Shell string            `yaml:"shell"`
	Dir   string            `yaml:"working-directory"`
	Run   string            `yaml:"run"`
	Env   map[string]string `yaml:"env"`
}

func TestDesktopWorkflowCarriesBuiltBackendChecksum(t *testing.T) {
	var workflow struct {
		Jobs struct {
			Build struct {
				Steps []desktopWorkflowStep `yaml:"steps"`
			} `yaml:"build"`
		} `yaml:"jobs"`
	}
	if err := yaml.Unmarshal(metadataFile(t, ".github/workflows/desktop.yml"), &workflow); err != nil {
		t.Fatal(err)
	}
	steps := map[string]desktopWorkflowStep{}
	for _, step := range workflow.Jobs.Build.Steps {
		steps[step.Name] = step
	}
	for _, tc := range []struct{ name, condition, command string }{
		{"Build (pull request)", "github.event_name == 'pull_request'", "go tool wails3 task build VERSION=${{ env.VERSION }}"},
		{"Build & package", "github.event_name != 'pull_request'", "go tool wails3 task package VERSION=${{ env.VERSION }}"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			step, ok := steps[tc.name]
			if !ok {
				t.Fatal("missing build step")
			}
			if step.If != tc.condition || step.Dir != "desktop" || step.Run != tc.command {
				t.Errorf("unexpected build step context: if=%q directory=%q command=%q", step.If, step.Dir, step.Run)
			}
			for key, want := range map[string]string{
				"BACKEND_VERSION": "${{ env.VERSION }}",
				"BACKEND_SHA256":  "${{ env.BACKEND_SHA256 }}",
			} {
				if step.Env[key] != want {
					t.Errorf("%s = %q, want %q", key, step.Env[key], want)
				}
			}
		})
	}

	backend, ok := steps["Build bundled backend"]
	if !ok {
		t.Fatal("missing bundled backend build step")
	}
	if backend.Shell != "bash" || backend.Dir != "server" {
		t.Fatalf("unexpected backend build context: shell=%q directory=%q", backend.Shell, backend.Dir)
	}
	if !strings.Contains(backend.Run, "github.com/0verL1nk/GraduateProject/server/internal/buildinfo.Version=$VERSION") {
		t.Error("bundled backend build must embed the client version")
	}
	t.Run("hashes the built executable", func(t *testing.T) {
		env, err, output := runBackendBuildStep(t, backend.Run, "")
		if err != nil {
			t.Fatalf("backend build step failed: %v\n%s", err, output)
		}
		want := fmt.Sprintf("BACKEND_SHA256=%x", sha256.Sum256([]byte("backend-under-test")))
		var found []string
		for _, line := range strings.Split(env, "\n") {
			if strings.HasPrefix(line, "BACKEND_SHA256=") {
				found = append(found, line)
			}
		}
		if len(found) != 1 || found[0] != want {
			t.Fatalf("GITHUB_ENV checksum entries = %q, want exactly %q", found, want)
		}
	})
	for _, bad := range []string{"invalid", strings.Repeat("a", 63), strings.Repeat("g", 64), strings.Repeat("A", 64)} {
		t.Run("rejects "+bad, func(t *testing.T) {
			env, err, output := runBackendBuildStep(t, backend.Run, bad)
			if err == nil {
				t.Errorf("invalid sha256sum output was accepted: %s", output)
			} else if !strings.Contains(output, "Invalid backend SHA-256") {
				t.Errorf("build failed before checksum validation: %v\n%s", err, output)
			}
			if regexp.MustCompile(`(?m)^BACKEND_SHA256=`).MatchString(env) {
				t.Errorf("invalid checksum published to GITHUB_ENV: %q", env)
			}
		})
	}
}

func runBackendBuildStep(t *testing.T, script, fakeDigest string) (string, error, string) {
	t.Helper()
	root := t.TempDir()
	server := filepath.Join(root, "server")
	if err := os.Mkdir(server, 0o700); err != nil {
		t.Fatal(err)
	}
	envFile := filepath.Join(root, "github_env")
	stub := `go() {
  [[ "$1" == "build" ]] || return 1
  local previous="" out="" source="" version=""
  for arg in "$@"; do
    if [[ "$previous" == "-o" ]]; then out="$arg"; fi
    if [[ "$arg" == "./cmd/papersage" ]]; then source=1; fi
    if [[ "$arg" == *"-X github.com/0verL1nk/GraduateProject/server/internal/buildinfo.Version=$VERSION"* ]]; then version=1; fi
    previous="$arg"
  done
  [[ "$out" == "../desktop/bin/backend/papersage.exe" && "$source" == 1 && "$version" == 1 ]] || return 1
  printf 'backend-under-test' > "$out"
}
`
	if fakeDigest != "" {
		stub += `sha256sum() { printf '%s  %s\n' "$TEST_FAKE_DIGEST" "$1"; }
`
	}
	cmd := exec.Command("bash", "--noprofile", "--norc", "-e", "-o", "pipefail", "-c", stub+script)
	cmd.Dir = server
	cmd.Env = append(os.Environ(), "RUNNER_OS=Windows", "VERSION=1.2.3", "GITHUB_ENV="+filepath.ToSlash(envFile), "TEST_FAKE_DIGEST="+fakeDigest)
	output, err := cmd.CombinedOutput()
	env, readErr := os.ReadFile(envFile)
	if readErr != nil && !os.IsNotExist(readErr) {
		t.Fatal(readErr)
	}
	return string(env), err, string(output)
}
