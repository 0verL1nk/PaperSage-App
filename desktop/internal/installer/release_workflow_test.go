package installer

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"testing"

	"gopkg.in/yaml.v3"
)

type releaseWorkflow struct {
	On          map[string]yaml.Node  `yaml:"on"`
	Permissions map[string]string     `yaml:"permissions"`
	Jobs        map[string]releaseJob `yaml:"jobs"`
}

type releaseJob struct {
	Needs       yaml.Node         `yaml:"needs"`
	Uses        string            `yaml:"uses"`
	Permissions map[string]string `yaml:"permissions"`
	Steps       []releaseStep     `yaml:"steps"`
}

type releaseStep struct {
	Name string            `yaml:"name"`
	If   string            `yaml:"if"`
	Uses string            `yaml:"uses"`
	Run  string            `yaml:"run"`
	With map[string]string `yaml:"with"`
}

func loadReleaseWorkflow(t *testing.T, path string) releaseWorkflow {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate workflow test")
	}
	root := filepath.Join(filepath.Dir(file), "..", "..", "..")
	var data []byte
	var err error
	if ref := os.Getenv("WORKFLOW_TEST_REF"); ref != "" {
		cmd := exec.Command("git", "show", ref+":"+path)
		cmd.Dir = root
		data, err = cmd.Output()
	} else {
		data, err = os.ReadFile(filepath.Join(root, filepath.FromSlash(path)))
	}
	if err != nil {
		t.Fatal(err)
	}
	var workflow releaseWorkflow
	if err := yaml.Unmarshal(data, &workflow); err != nil {
		t.Fatal(err)
	}
	return workflow
}

func releaseNeeds(t *testing.T, job releaseJob) map[string]bool {
	t.Helper()
	result := map[string]bool{}
	switch job.Needs.Kind {
	case yaml.ScalarNode:
		result[job.Needs.Value] = true
	case yaml.SequenceNode:
		for _, node := range job.Needs.Content {
			result[node.Value] = true
		}
	}
	return result
}

func releaseStepNamed(t *testing.T, job releaseJob, name string) releaseStep {
	t.Helper()
	for _, step := range job.Steps {
		if step.Name == name {
			return step
		}
	}
	t.Fatalf("missing step %q", name)
	return releaseStep{}
}

func requireWorkflowText(t *testing.T, actual string, required ...string) {
	t.Helper()
	for _, text := range required {
		if !strings.Contains(actual, text) {
			t.Errorf("missing %q", text)
		}
	}
}

func TestReleaseWorkflowSafety(t *testing.T) {
	desktop := loadReleaseWorkflow(t, ".github/workflows/desktop.yml")
	release := loadReleaseWorkflow(t, ".github/workflows/release.yml")

	t.Run("desktop is reusable and never publishes", func(t *testing.T) {
		for _, event := range []string{"workflow_call", "workflow_dispatch", "push", "pull_request"} {
			if _, ok := desktop.On[event]; !ok {
				t.Errorf("desktop missing %s trigger", event)
			}
		}
		var push map[string]yaml.Node
		pushNode := desktop.On["push"]
		if err := pushNode.Decode(&push); err != nil {
			t.Fatal(err)
		}
		if _, ok := push["tags"]; ok {
			t.Error("desktop must not trigger independently for tags")
		}
		if _, ok := push["tags-ignore"]; ok {
			t.Error("desktop must not declare a tag trigger")
		}
		for _, event := range []string{"push", "pull_request"} {
			var filter map[string]yaml.Node
			eventNode := desktop.On[event]
			if err := eventNode.Decode(&filter); err != nil {
				t.Fatal(err)
			}
			if _, ok := filter["paths"]; !ok {
				t.Errorf("desktop %s must remain path-filtered", event)
			}
		}
		if len(desktop.Jobs) != 1 || desktop.Jobs["build"].Steps == nil {
			t.Error("desktop must have only its build job")
		}
		if desktop.Permissions["contents"] != "read" {
			t.Error("desktop contents permission must be read-only")
		}
		var desktopArtifact bool
		for _, step := range desktop.Jobs["build"].Steps {
			if strings.HasPrefix(step.Uses, "actions/upload-artifact@") {
				desktopArtifact = true
				if step.With["overwrite"] != "true" {
					t.Error("desktop artifact must replace its own prior-attempt artifact")
				}
			}
		}
		if !desktopArtifact {
			t.Error("desktop must upload a build artifact")
		}
		for name, job := range desktop.Jobs {
			if job.Permissions["contents"] == "write" {
				t.Errorf("desktop job %s can publish", name)
			}
			for _, step := range job.Steps {
				if strings.Contains(step.Uses, "action-gh-release") || strings.Contains(step.Run, "gh release") {
					t.Errorf("desktop job %s contains a release publisher", name)
				}
			}
		}
	})

	t.Run("validated builds precede the sole publisher", func(t *testing.T) {
		for _, name := range []string{"desktop", "server-build"} {
			job, ok := release.Jobs[name]
			if !ok {
				t.Errorf("missing %s job", name)
				continue
			}
			if !releaseNeeds(t, job)["validate"] {
				t.Errorf("%s must depend on validate", name)
			}
			if job.Permissions["contents"] != "read" {
				t.Errorf("%s must use read-only contents permission", name)
			}
		}
		if got := release.Jobs["desktop"].Uses; got != "./.github/workflows/desktop.yml" {
			t.Errorf("desktop must call the local reusable workflow, got %q", got)
		}
		build := release.Jobs["server-build"]
		var goreleaser, serverArtifact bool
		for _, step := range build.Steps {
			if strings.HasPrefix(step.Uses, "goreleaser/goreleaser-action@") {
				goreleaser = true
				if !strings.Contains(step.With["args"], "--skip=publish") {
					t.Error("server GoReleaser build may publish")
				}
			}
			if strings.HasPrefix(step.Uses, "actions/upload-artifact@") {
				serverArtifact = true
				requireWorkflowText(t, step.With["path"], "dist/papersage_*_linux_amd64.tar.gz", "dist/checksums.txt", "dist/CHANGELOG.md")
				if step.With["if-no-files-found"] != "error" || step.With["overwrite"] != "true" {
					t.Error("server artifact must require files and replace its own prior-attempt artifact")
				}
			}
		}
		if !goreleaser || !serverArtifact {
			t.Error("server build must generate and upload GoReleaser artifacts")
		}
		publisher, ok := release.Jobs["release"]
		if !ok {
			t.Fatal("missing release publisher job")
		}
		for _, need := range []string{"validate", "desktop", "server-build"} {
			if !releaseNeeds(t, publisher)[need] {
				t.Errorf("release must depend on %s", need)
			}
		}
		if publisher.Permissions["contents"] != "write" {
			t.Error("release needs contents write permission")
		}
		for name, job := range release.Jobs {
			if name != "release" && job.Permissions["contents"] == "write" {
				t.Errorf("job %s must not have release write permission", name)
			}
		}
	})

	t.Run("verify complete installer and server before publishing", func(t *testing.T) {
		job := release.Jobs["release"]
		verify := releaseStepNamed(t, job, "Verify release assets")
		prepare := releaseStepNamed(t, job, "Prepare release draft")
		publish := releaseStepNamed(t, job, "Upload and publish release")
		verifyIndex, prepareIndex, publishIndex := -1, -1, -1
		for i, step := range job.Steps {
			switch step.Name {
			case verify.Name:
				verifyIndex = i
			case prepare.Name:
				prepareIndex = i
			case publish.Name:
				publishIndex = i
			}
		}
		if !(verifyIndex < prepareIndex && prepareIndex < publishIndex) {
			t.Error("asset verification must happen before draft preparation and publication")
		}
		requireWorkflowText(t, verify.Run, "papersage-windows-amd64-installer.exe", "SHA256SUMS", "checksums.txt", "linux_amd64.tar.gz")
		if strings.Count(verify.Run, "sha256sum --check --strict") < 2 {
			t.Error("both installer and server checksums must be verified")
		}
		requireWorkflowText(t, prepare.Run, "--paginate", "draft=true", ".tag_name", ".draft", "RELEASE_COMMIT", "GITHUB_RUN_ID", "receipt.json")
		requireWorkflowText(t, publish.Run,
			"papersage-windows-amd64-installer.exe", "SHA256SUMS", "checksums.txt", "linux_amd64.tar.gz",
			"draft=false", ".tag_name", ".draft", ".state", ".digest", "Content-Length", "RELEASE_COMMIT")
		if regexp.MustCompile(`papersage-windows-amd64\.exe\b`).MatchString(prepare.Run + publish.Run) {
			t.Error("standalone Windows executable must never be published")
		}
		if strings.Contains(prepare.Run+publish.Run, "--clobber") || strings.Contains(prepare.Run+publish.Run, "gh release delete") {
			t.Error("release must never overwrite or delete existing assets")
		}
		var downloads int
		for _, step := range job.Steps {
			if strings.HasPrefix(step.Uses, "actions/download-artifact@") {
				downloads++
				if step.With["run-id"] != "" {
					t.Error("release must download artifacts from its own run")
				}
			}
		}
		if downloads != 3 {
			t.Errorf("release must download both builds and the optional receipt, got %d", downloads)
		}
	})

	t.Run("retry uses only this run's immutable receipt", func(t *testing.T) {
		job := release.Jobs["release"]
		if job.Permissions["actions"] != "read" {
			t.Error("release must be able to discover this run's receipt")
		}
		lookup := releaseStepNamed(t, job, "Find draft receipt")
		requireWorkflowText(t, lookup.Run, "GITHUB_RUN_ID", ".expired", "release-draft-")
		prepare := releaseStepNamed(t, job, "Prepare release draft")
		requireWorkflowText(t, prepare.Run, ".run_id", ".commit", ".tag", ".release_id", ".draft", "CHANGELOG.md")
		publish := releaseStepNamed(t, job, "Upload and publish release")
		requireWorkflowText(t, publish.Run, "if [ \"$draft\" = \"true\" ]", ".digest", ".state", "draft=false")
		if strings.Index(publish.Run, "if [ \"$draft\" = \"true\" ]") > strings.Index(publish.Run, "draft=false") {
			t.Error("public release must not be patched on retry")
		}
		var uploadReceipt, downloadReceipt bool
		for _, step := range job.Steps {
			if step.With["name"] != "release-draft-${{ github.run_id }}" {
				continue
			}
			switch {
			case strings.HasPrefix(step.Uses, "actions/upload-artifact@"):
				uploadReceipt = true
				if step.With["overwrite"] == "true" || !strings.Contains(step.If, "created") {
					t.Error("ownership receipt may be uploaded only on creation and never overwritten")
				}
			case strings.HasPrefix(step.Uses, "actions/download-artifact@"):
				downloadReceipt = true
				if step.If == "" {
					t.Error("receipt download must be conditional")
				}
			}
		}
		if !uploadReceipt || !downloadReceipt {
			t.Error("draft creation and retry must share the current run's receipt artifact")
		}
	})

	t.Run("validation cannot wait for its own run", func(t *testing.T) {
		step := releaseStepNamed(t, release.Jobs["validate"], "Require green CI for the tagged commit")
		requireWorkflowText(t, step.Run, "GITHUB_RUN_ID", ".details_url", "filter=all")
		if strings.Contains(step.Run, "latest[$1]") {
			t.Error("checks must not be deduplicated solely by display name")
		}
	})

	t.Run("production deploy requires release success", func(t *testing.T) {
		if !releaseNeeds(t, release.Jobs["deploy"])["release"] {
			t.Error("deploy must depend on the completed release")
		}
	})
}

func TestReleaseCheckGateDoesNotHideFailures(t *testing.T) {
	workflow := loadReleaseWorkflow(t, ".github/workflows/release.yml")
	step := releaseStepNamed(t, workflow.Jobs["validate"], "Require green CI for the tagged commit")
	stub := `gh() { printf '{}'; }
jq() { printf '%s\n' "$CHECK_FIXTURE"; }
sleep() { return 73; }
`
	for _, tc := range []struct {
		name, checks, output string
		success              bool
	}{
		{"same-name failure", "Build & Push\tfailure\t2026-01-01T00:00:00Z\nBuild & Push\tsuccess\t2026-01-01T00:01:00Z", "failing checks", false},
		{"pending waits", "build\tpending\t2026-01-01T00:00:00Z\ntest\tsuccess\t2026-01-01T00:01:00Z", "waiting for CI", false},
		{"same-name successes", "Build & Push\tsuccess\t2026-01-01T00:00:00Z\nBuild & Push\tsuccess\t2026-01-01T00:01:00Z", "completed checks are green", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := exec.Command("bash", "--noprofile", "--norc", "-e", "-o", "pipefail", "-c", stub+step.Run)
			cmd.Dir = t.TempDir()
			cmd.Env = append(os.Environ(), "GITHUB_RUN_ID=77", "GITHUB_REPOSITORY=example/example", "SHA=fixture", "CHECK_FIXTURE="+tc.checks)
			out, err := cmd.CombinedOutput()
			if (err == nil) != tc.success || !strings.Contains(string(out), tc.output) {
				t.Errorf("exit error = %v, output = %s; want success=%t and %q", err, out, tc.success, tc.output)
			}
		})
	}
}

// The workflow's shell runs against a loopback-only GitHub API; no real release is touched.
type releaseSimulation struct {
	mu                sync.Mutex
	exists            bool
	draft             bool
	receipt           bool
	commit            string
	assets            map[string][]byte
	creates           int
	uploads           int
	patches           int
	failUploadAt      int
	losePatchResponse bool
}

func (s *releaseSimulation) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/jq" {
		w.Header().Set("Content-Type", "application/json")
		body, _ := io.ReadAll(r.Body)
		_, _ = w.Write(body)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	release := map[string]any{"id": 21, "tag_name": "v1.2.3", "draft": s.draft}
	var response any
	switch {
	case r.Method == "GET" && strings.HasSuffix(r.URL.Path, "/actions/runs/77/artifacts"):
		artifacts := []map[string]any{}
		if s.receipt {
			artifacts = append(artifacts, map[string]any{"name": "release-draft-77", "expired": false})
		}
		response = map[string]any{"artifacts": artifacts}
	case r.Method == "GET" && strings.HasSuffix(r.URL.Path, "/commits/v1.2.3"):
		response = map[string]any{"sha": s.commit}
	case r.Method == "GET" && strings.HasSuffix(r.URL.Path, "/releases"):
		response = []any{}
		if s.exists {
			response = []any{release}
		}
	case r.Method == "POST" && strings.HasSuffix(r.URL.Path, "/releases"):
		s.creates++
		if s.exists {
			http.Error(w, "release already exists", http.StatusUnprocessableEntity)
			return
		}
		s.exists, s.draft = true, true
		release["draft"] = true
		response = release
	case r.Method == "GET" && strings.HasSuffix(r.URL.Path, "/releases/21"):
		response = release
	case r.Method == "GET" && strings.HasSuffix(r.URL.Path, "/releases/21/assets"):
		items := []any{}
		for name, data := range s.assets {
			items = append(items, map[string]any{"name": name, "state": "uploaded", "digest": fmt.Sprintf("sha256:%x", sha256.Sum256(data))})
		}
		response = items
	case r.Method == "POST" && strings.HasSuffix(r.URL.Path, "/releases/21/assets"):
		s.uploads++
		if s.uploads == s.failUploadAt {
			http.Error(w, "simulated upload failure", http.StatusBadGateway)
			return
		}
		if !s.draft {
			http.Error(w, "cannot upload to public fixture", http.StatusConflict)
			return
		}
		data, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		name := r.URL.Query().Get("name")
		if _, found := s.assets[name]; found {
			http.Error(w, "asset already exists", http.StatusUnprocessableEntity)
			return
		}
		s.assets[name] = data
		response = map[string]any{"name": name}
	case r.Method == "PATCH" && strings.HasSuffix(r.URL.Path, "/releases/21"):
		s.patches++
		s.draft = false
		if s.losePatchResponse {
			s.losePatchResponse = false
			http.Error(w, "simulated lost publish response", http.StatusBadGateway)
			return
		}
		release["draft"] = false
		response = release
	default:
		http.Error(w, "unexpected simulated request: "+r.Method+" "+r.URL.String(), http.StatusNotFound)
		return
	}
	_ = json.NewEncoder(w).Encode(response)
}

// gh supplies the Go jq engine locally; every HTTP endpoint is rewritten to the fixture.
const releaseSimulationShell = `gh() {
  local arg
  local -a args=()
  local redirected=false
  for arg in "$@"; do
    case "$arg" in
      repos/*) args+=("$TEST_API_URL/$arg"); redirected=true ;;
      https://uploads.github.com/*) args+=("$TEST_API_URL/${arg#https://uploads.github.com/}"); redirected=true ;;
      *) args+=("$arg") ;;
    esac
  done
  [ "$redirected" = true ] || return 91
  command gh "${args[@]}"
}
jq() {
  local prefix='' key value filter result input status require_truth=false null_input=false raw_output=false
  while [ "$#" -gt 0 ]; do
    case "$1" in
      --arg) key="$2"; value="$3"; prefix+="\"$value\" as \$$key | "; shift 3 ;;
      --argjson) key="$2"; value="$3"; prefix+="$value as \$$key | "; shift 3 ;;
      -*) [[ "$1" == *e* ]] && require_truth=true; [[ "$1" == *n* ]] && null_input=true; [[ "$1" == *r* ]] && raw_output=true; shift ;;
      *) filter="$1"; shift; break ;;
    esac
  done
  if [ "$#" -gt 0 ]; then
    result=$(command gh api --method POST --input "$1" --jq "$prefix$filter" "$TEST_API_URL/jq") || return
  else
    # gh on Windows truncates piped stdin, so spool it to a regular file first.
    input=$(mktemp)
    if [ "$null_input" = true ]; then printf 'null' > "$input"; else cat > "$input"; fi
    result=$(command gh api --method POST --input "$input" --jq "$prefix$filter" "$TEST_API_URL/jq")
    status=$?
    rm -f "$input"
    [ "$status" -eq 0 ] || return "$status"
  fi
  # -r turns booleans into strings, and jq -e treats those as truthy.
  if [ "$require_truth" = true ] && { [ -z "$result" ] || [ "$result" = null ] || { [ "$raw_output" = false ] && [ "$result" = false ]; }; }; then return 1; fi
  printf '%s\n' "$result"
}
`

func simulationStep(t *testing.T, root, url, script string) (string, error) {
	t.Helper()
	output := filepath.Join(root, "output")
	if err := os.WriteFile(output, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("bash", "--noprofile", "--norc", "-e", "-o", "pipefail", "-c", releaseSimulationShell+script)
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "GH_TOKEN=local-test-only", "GITHUB_REPOSITORY=example/example", "GITHUB_RUN_ID=77",
		"RELEASE_TAG=v1.2.3", "RELEASE_COMMIT=abc123", "RELEASE_ID=21", "GITHUB_OUTPUT="+filepath.ToSlash(output), "TEST_API_URL="+url)
	data, err := cmd.CombinedOutput()
	return string(data), err
}

func simulationFiles(t *testing.T, root string) {
	t.Helper()
	files := map[string][]byte{
		"bundle/server/papersage_1.2.3_linux_amd64.tar.gz":     []byte("server archive"),
		"bundle/server/CHANGELOG.md":                           []byte("release notes"),
		"bundle/desktop/papersage-windows-amd64-installer.exe": []byte("complete installer"),
	}
	for name, data := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, item := range []struct{ file, sum string }{
		{"bundle/server/papersage_1.2.3_linux_amd64.tar.gz", "bundle/server/checksums.txt"},
		{"bundle/desktop/papersage-windows-amd64-installer.exe", "bundle/desktop/SHA256SUMS"},
	} {
		data := files[item.file]
		sum := fmt.Sprintf("%x  %s\n", sha256.Sum256(data), filepath.Base(item.file))
		if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(item.sum)), []byte(sum), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestReleaseDraftRecoverySimulation(t *testing.T) {
	for _, tool := range []string{"bash", "gh"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("local simulation requires %s", tool)
		}
	}
	workflow := loadReleaseWorkflow(t, ".github/workflows/release.yml")
	job := workflow.Jobs["release"]
	verify := releaseStepNamed(t, job, "Verify release assets").Run
	find := releaseStepNamed(t, job, "Find draft receipt").Run
	prepare := releaseStepNamed(t, job, "Prepare release draft").Run
	publish := releaseStepNamed(t, job, "Upload and publish release").Run
	setup := func(t *testing.T) (string, *releaseSimulation, string) {
		t.Helper()
		root := t.TempDir()
		simulationFiles(t, root)
		state := &releaseSimulation{commit: "abc123", assets: map[string][]byte{}}
		server := httptest.NewServer(state)
		t.Cleanup(server.Close)
		if output, err := simulationStep(t, root, server.URL, verify); err != nil {
			t.Fatalf("fixture asset verification failed: %v\n%s", err, output)
		}
		return root, state, server.URL
	}
	run := func(t *testing.T, root, url, step string, success bool) string {
		t.Helper()
		output, err := simulationStep(t, root, url, step)
		if (err == nil) != success {
			t.Fatalf("step exit error = %v; want success=%t\n%s", err, success, output)
		}
		return output
	}

	t.Run("partial retry resumes matching assets", func(t *testing.T) {
		root, state, url := setup(t)
		run(t, root, url, find, true)
		run(t, root, url, prepare, true)
		receipt, err := os.ReadFile(filepath.Join(root, "draft-receipt", "receipt.json"))
		if err != nil || !strings.Contains(string(receipt), `"run_id":"77"`) {
			t.Fatalf("missing same-run receipt: %s, %v", receipt, err)
		}
		state.receipt = true
		run(t, root, url, find, true)
		state.failUploadAt = 3
		run(t, root, url, publish, false)
		if len(state.assets) != 2 || !state.draft {
			t.Fatalf("failed upload should leave two assets on a draft: %+v", state)
		}
		run(t, root, url, prepare, true)
		run(t, root, url, publish, true)
		if state.creates != 1 || state.uploads != 5 || state.patches != 1 || len(state.assets) != 4 || state.draft {
			t.Fatalf("unsafe partial retry: %+v", state)
		}
		// A repeated build with different bytes must not mutate the public release.
		path := filepath.Join(root, "bundle", "desktop", "papersage-windows-amd64-installer.exe")
		if err := os.WriteFile(path, []byte("different installer"), 0o600); err != nil {
			t.Fatal(err)
		}
		run(t, root, url, prepare, true)
		run(t, root, url, publish, false)
		if state.uploads != 5 || state.patches != 1 {
			t.Fatalf("public release was modified on full retry: %+v", state)
		}
	})

	t.Run("lost publish response recovers without writing", func(t *testing.T) {
		root, state, url := setup(t)
		run(t, root, url, prepare, true)
		state.losePatchResponse = true
		output := run(t, root, url, publish, false)
		if state.draft || state.patches != 1 || len(state.assets) != 4 {
			t.Fatalf("fixture did not publish before response loss: %+v\n%s", state, output)
		}
		run(t, root, url, prepare, true)
		run(t, root, url, publish, true)
		if state.creates != 1 || state.uploads != 4 || state.patches != 1 {
			t.Fatalf("public release was modified during recovery: %+v", state)
		}
	})

	t.Run("unknown draft or public release is untouched", func(t *testing.T) {
		for _, draft := range []bool{true, false} {
			t.Run(fmt.Sprintf("draft=%t", draft), func(t *testing.T) {
				root, state, url := setup(t)
				state.exists, state.draft = true, draft
				run(t, root, url, prepare, false)
				if state.creates != 0 || state.uploads != 0 || state.patches != 0 {
					t.Fatalf("unknown release was modified: %+v", state)
				}
			})
		}
	})
}
