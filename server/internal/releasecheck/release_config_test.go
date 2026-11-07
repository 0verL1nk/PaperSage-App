package releasecheck

import (
	"fmt"
	"os"
	"path"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

type releaseConfig struct {
	Builds []struct {
		ID, Dir, Main, Binary          string
		Builder                        string `yaml:"builder"`
		Goos, Goarch, Goamd64, Targets []string
		Skip                           string
		Ignore                         []map[string]string
	} `yaml:"builds"`
	Archives []struct {
		IDs     []string `yaml:"ids"`
		Formats []string `yaml:"formats"`
	} `yaml:"archives"`
}

func validateRelease(data []byte) error {
	var c releaseConfig
	if err := yaml.Unmarshal(data, &c); err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, b := range c.Builds {
		if (b.ID != "papersage" && b.ID != "migrate") || seen[b.ID] {
			return fmt.Errorf("unexpected or duplicate build ID %q", b.ID)
		}
		seen[b.ID] = true
		if b.Dir != "server" || path.Clean(b.Main) != "cmd/"+b.ID || b.Binary != b.ID || (b.Builder != "" && b.Builder != "go") {
			return fmt.Errorf("build %s does not produce the expected binary", b.ID)
		}
		if b.Skip != "" && b.Skip != "false" {
			return fmt.Errorf("build %s may be skipped: %s", b.ID, b.Skip)
		}
		if len(b.Targets) > 0 {
			// GoReleaser ignores matrix and ignore when explicit targets are set.
			targets := map[string]bool{}
			for _, target := range b.Targets {
				if target == "linux_amd64" {
					target += "_v1"
				}
				targets[target] = true
			}
			if len(targets) != 1 {
				return fmt.Errorf("build %s has multiple targets", b.ID)
			}
			for target := range targets {
				if !slices.Contains([]string{"linux_amd64_v1", "linux_amd64_v2", "linux_amd64_v3", "linux_amd64_v4"}, target) {
					return fmt.Errorf("build %s has unexpected target %s", b.ID, target)
				}
			}
			continue
		}
		if len(b.Goos) != 1 || b.Goos[0] != "linux" || len(b.Goarch) != 1 || b.Goarch[0] != "amd64" {
			return fmt.Errorf("build %s matrix must produce only linux/amd64", b.ID)
		}
		amd64 := "v1"
		if len(b.Goamd64) > 1 {
			return fmt.Errorf("build %s has multiple amd64 variants", b.ID)
		}
		if len(b.Goamd64) == 1 {
			amd64 = b.Goamd64[0]
		}
		if !slices.Contains([]string{"v1", "v2", "v3", "v4"}, amd64) {
			return fmt.Errorf("invalid amd64 variant %s", amd64)
		}
		for _, ignored := range b.Ignore {
			matched := true
			for key, value := range ignored {
				actual, known := map[string]string{"goos": "linux", "goarch": "amd64", "goamd64": amd64, "go386": "", "goarm": "", "goarm64": "", "gomips": "", "goppc64": "", "goriscv64": ""}[key]
				if known && value != "" && value != actual {
					matched = false
					break
				}
			}
			if matched {
				return fmt.Errorf("build %s ignores its linux/amd64 target", b.ID)
			}
		}
	}
	if !seen["papersage"] || !seen["migrate"] {
		return fmt.Errorf("both builds are required")
	}
	for _, a := range c.Archives {
		if (len(a.IDs) == 0 || (slices.Contains(a.IDs, "papersage") && slices.Contains(a.IDs, "migrate"))) && !slices.Contains(a.Formats, "binary") {
			return nil
		}
	}
	return fmt.Errorf("an archive must contain both binaries")
}

func TestReleaseConfig(t *testing.T) {
	data, err := os.ReadFile("../../../.goreleaser.yml")
	if err != nil {
		t.Fatal(err)
	}
	if err := validateRelease(data); err != nil {
		t.Fatal(err)
	}
}

func TestReleaseConfigSemantics(t *testing.T) {
	data, err := os.ReadFile("../../../.goreleaser.yml")
	if err != nil {
		t.Fatal(err)
	}
	base := strings.ReplaceAll(string(data), "\r\n", "\n")
	replace := func(old, new string) string {
		if !strings.Contains(base, old) {
			t.Fatalf("fixture missing %q", old)
		}
		return strings.Replace(base, old, new, 1)
	}
	tests := []struct {
		name, config string
		valid        bool
	}{
		{"block list", replace("goarch: [amd64]", "goarch:\n      - amd64"), true},
		{"anchor and alias", strings.ReplaceAll(replace("goarch: [amd64]", "goarch: &arch [amd64]"), "goarch: [amd64]", "goarch: *arch"), true},
		{"unrelated property", replace("project_name: papersage", "project_name: papersage\nmetadata: [anything]"), true},
		{"explicit target", replace("goos: [linux]\n    goarch: [amd64]", "targets: [linux_amd64]"), true},
		{"unrelated ignore", replace("goos: [linux]", "ignore: [{goos: windows}]\n    goos: [linux]"), true},
		{"disabled skip", replace("goarch: [amd64]", "goarch: [amd64]\n    skip: false"), true},
		{"default archive IDs", replace("ids: [papersage, migrate]", "ids: []"), true},
		{"extra build", replace("\narchives:", "\n  - id: extra\n    dir: server\n    main: ./cmd/papersage\n    binary: extra\n    goos: [linux]\n    goarch: [amd64]\n\narchives:"), false},
		{"extra os", replace("goos: [linux]", "goos: [linux, windows]"), false},
		{"extra arch", replace("goarch: [amd64]", "goarch: [amd64, arm64]"), false},
		{"extra explicit target", replace("goarch: [amd64]", "goarch: [amd64]\n    targets: [linux_amd64, linux_arm64]"), false},
		{"extra amd64 variant", replace("goarch: [amd64]", "goarch: [amd64]\n    goamd64: [v1, v3]"), false},
		{"skipped build", replace("goarch: [amd64]", "goarch: [amd64]\n    skip: true"), false},
		{"ignored target", replace("goarch: [amd64]", "goarch: [amd64]\n    ignore: [{goos: linux, goarch: amd64}]"), false},
		{"missing migrate build", replace("  - id: migrate", "  - id: papersage"), false},
		{"missing papersage build", replace("  - id: papersage", "  - id: migrate"), false},
		{"wrong main", replace("main: ./cmd/migrate", "main: ./cmd/papersage"), false},
		{"wrong directory", replace("dir: server", "dir: other"), false},
		{"wrong binary", replace("binary: migrate", "binary: other"), false},
		{"missing migrate archive member", replace("ids: [papersage, migrate]", "ids: [papersage]"), false},
		{"missing papersage archive member", replace("ids: [papersage, migrate]", "ids: [migrate]"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateRelease([]byte(tt.config))
			if (err == nil) != tt.valid {
				t.Fatalf("valid = %v, error = %v", tt.valid, err)
			}
		})
	}
}
