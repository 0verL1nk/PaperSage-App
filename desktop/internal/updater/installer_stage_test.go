package updater

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	wailsupdater "github.com/wailsapp/wails/v3/pkg/updater"
)

type stagingTestProvider struct {
	content                []byte
	downloadErr            error
	ignoreCancelAfterWrite bool
	downloaded             *bool
}

func (stagingTestProvider) Name() string { return "staging-test" }

func (stagingTestProvider) Check(context.Context, wailsupdater.CheckRequest) (*wailsupdater.Release, error) {
	return nil, errors.New("unexpected check")
}

func (p stagingTestProvider) Download(ctx context.Context, _ *wailsupdater.Release, dst io.Writer, progress func(int64, int64)) error {
	if p.downloaded != nil {
		*p.downloaded = true
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	n, err := dst.Write(p.content)
	if err != nil {
		return err
	}
	progress(int64(n), int64(len(p.content)))
	if p.downloadErr != nil {
		return p.downloadErr
	}
	if !p.ignoreCancelAfterWrite {
		return ctx.Err()
	}
	return nil
}

func stagingTestRelease(content []byte) *wailsupdater.Release {
	digest := sha256.Sum256(content)
	return &wailsupdater.Release{
		Artifact: wailsupdater.Artifact{
			Filename: installerAssetName,
			Platform: "windows",
			Arch:     "amd64",
			Size:     int64(len(content)),
		},
		Verification: &wailsupdater.Verification{DigestAlgo: "sha256", Digest: digest[:]},
	}
}

func stagingRootWithSentinels(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "sentinel"), []byte("leave me"), 0600); err != nil {
		t.Fatal(err)
	}
	keep := filepath.Join(root, "keep")
	if err := os.Mkdir(keep, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(keep, "nested"), []byte("leave me too"), 0600); err != nil {
		t.Fatal(err)
	}
	return root
}

func assertStagingSentinels(t *testing.T, root string) {
	t.Helper()
	for path, want := range map[string]string{
		"sentinel":                      "leave me",
		filepath.Join("keep", "nested"): "leave me too",
	} {
		got, err := os.ReadFile(filepath.Join(root, path))
		if err != nil || string(got) != want {
			t.Errorf("sentinel %q = %q, %v; want %q", path, got, err, want)
		}
	}
}

func assertNoStages(t *testing.T, root string) {
	t.Helper()
	assertStagingSentinels(t, root)
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 || entries[0].Name() != "keep" || entries[1].Name() != "sentinel" {
		t.Fatalf("staging root entries = %v; want only existing sentinels", entries)
	}
}

func TestStageVerifiedInstallerStoresVerifiedBytesInUniqueDirectories(t *testing.T) {
	root := stagingRootWithSentinels(t)
	content := []byte("verified installer contents")
	provider := stagingTestProvider{content: content}
	release := stagingTestRelease(content)
	var previousDir, previousPath string

	for i := 0; i < 2; i++ {
		var written, total int64
		dir, path, err := stageVerifiedInstaller(context.Background(), provider, release, root, func(w, n int64) {
			written, total = w, n
		})
		if err != nil {
			t.Fatalf("stageVerifiedInstaller() error = %v", err)
		}
		if filepath.Dir(dir) != root || filepath.Dir(path) != dir || filepath.Base(path) != installerAssetName {
			t.Fatalf("staging paths dir=%q, installer=%q are not beneath root %q", dir, path, root)
		}
		if dir == previousDir || path == previousPath {
			t.Fatalf("successful updates reused staging paths %q and %q", dir, path)
		}
		previousDir, previousPath = dir, path
		got, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(got, content) {
			t.Fatalf("staged bytes = %q, %v; want %q", got, err, content)
		}
		digest := sha256.Sum256(got)
		if !bytes.Equal(digest[:], release.Verification.Digest) {
			t.Fatal("staged installer checksum does not match release")
		}
		if written != int64(len(content)) || total != int64(len(content)) {
			t.Errorf("progress = (%d, %d), want (%d, %d)", written, total, len(content), len(content))
		}
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if runtime.GOOS != "windows" && info.Mode().Perm()&0077 != 0 {
			t.Errorf("installer mode = %v, want no group or other permissions", info.Mode().Perm())
		}
		assertStagingSentinels(t, root)
	}
}

func TestStageVerifiedInstallerRemovesOnlyFailedStage(t *testing.T) {
	content := []byte("installer bytes")
	downloadErr := errors.New("partial download failed")
	tooSmall := stagingTestRelease(content)
	tooSmall.Artifact.Size--
	tooLarge := stagingTestRelease(content)
	tooLarge.Artifact.Size++
	tests := []struct {
		name     string
		release  *wailsupdater.Release
		provider wailsupdater.Provider
		cancel   bool
		wantErr  error
	}{
		{name: "nil release", provider: stagingTestProvider{content: content}},
		{name: "nil provider", release: stagingTestRelease(content)},
		{name: "missing checksum", release: &wailsupdater.Release{Artifact: stagingTestRelease(content).Artifact}, provider: stagingTestProvider{content: content}},
		{name: "invalid checksum", release: &wailsupdater.Release{Artifact: stagingTestRelease(content).Artifact, Verification: &wailsupdater.Verification{DigestAlgo: "sha256", Digest: []byte("short")}}, provider: stagingTestProvider{content: content}},
		{name: "mismatched digest", release: stagingTestRelease([]byte("different bytes")), provider: stagingTestProvider{content: content}},
		{name: "size exceeds release metadata", release: tooSmall, provider: stagingTestProvider{content: content}},
		{name: "size below release metadata", release: tooLarge, provider: stagingTestProvider{content: content}},
		{name: "download error after write", release: stagingTestRelease(content), provider: stagingTestProvider{content: content, downloadErr: downloadErr}, wantErr: downloadErr},
		{name: "cancel during download", release: stagingTestRelease(content), provider: stagingTestProvider{content: content}, cancel: true, wantErr: context.Canceled},
		{name: "cancel even when provider returns success", release: stagingTestRelease(content), provider: stagingTestProvider{content: content, ignoreCancelAfterWrite: true}, cancel: true, wantErr: context.Canceled},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := stagingRootWithSentinels(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			dir, path, err := stageVerifiedInstaller(ctx, tt.provider, tt.release, root, func(_, _ int64) {
				if tt.cancel {
					cancel()
				}
			})
			if err == nil || dir != "" || path != "" {
				t.Fatalf("stageVerifiedInstaller() = (%q, %q, %v), want empty paths and error", dir, path, err)
			}
			if tt.wantErr != nil && !errors.Is(err, tt.wantErr) {
				t.Errorf("error = %v, want %v", err, tt.wantErr)
			}
			assertNoStages(t, root)
		})
	}
}

func TestStageVerifiedInstallerRejectsNonpositiveSizeBeforeDownload(t *testing.T) {
	for name, size := range map[string]int64{"missing": 0, "negative": -1} {
		t.Run(name, func(t *testing.T) {
			root := stagingRootWithSentinels(t)
			content := []byte("installer bytes")
			release := stagingTestRelease(content)
			release.Artifact.Size = size
			downloaded := false
			provider := stagingTestProvider{content: content, downloaded: &downloaded}
			dir, path, err := stageVerifiedInstaller(context.Background(), provider, release, root, nil)
			if err == nil || dir != "" || path != "" || downloaded {
				t.Fatalf("staging = (%q, %q, %v), downloaded = %t; want rejection before download", dir, path, err, downloaded)
			}
			assertNoStages(t, root)
		})
	}
}

func TestStageVerifiedInstallerRejectsInvalidRoot(t *testing.T) {
	root := stagingRootWithSentinels(t)
	for name, invalid := range map[string]string{
		"empty":        "",
		"missing":      filepath.Join(root, "does-not-exist"),
		"regular file": filepath.Join(root, "sentinel"),
	} {
		t.Run(name, func(t *testing.T) {
			content := []byte("installer bytes")
			dir, path, err := stageVerifiedInstaller(context.Background(), stagingTestProvider{content: content}, stagingTestRelease(content), invalid, nil)
			if err == nil || dir != "" || path != "" {
				t.Fatalf("stageVerifiedInstaller() = (%q, %q, %v), want empty paths and error", dir, path, err)
			}
			assertNoStages(t, root)
		})
	}
}

type stagingFaultFile struct {
	*os.File
	writeErr error
	syncErr  error
	closeErr error
	closed   bool
}

func (f *stagingFaultFile) Write(p []byte) (int, error) {
	if f.writeErr != nil {
		return 0, f.writeErr
	}
	return f.File.Write(p)
}

func (f *stagingFaultFile) Sync() error {
	if f.syncErr != nil {
		return f.syncErr
	}
	return f.File.Sync()
}

func (f *stagingFaultFile) Close() error {
	f.closed = true
	err := f.File.Close()
	if f.closeErr != nil {
		return f.closeErr
	}
	return err
}

func TestStageVerifiedInstallerCleansUpFileFailures(t *testing.T) {
	for _, operation := range []string{"open", "write", "sync", "close"} {
		t.Run(operation, func(t *testing.T) {
			root := stagingRootWithSentinels(t)
			content := []byte("installer bytes")
			injected := errors.New(operation + " failure")
			var created *stagingFaultFile
			dir, path, err := stageVerifiedInstallerWithOpen(context.Background(), stagingTestProvider{content: content}, stagingTestRelease(content), root, nil, func(path string) (stagedInstallerFile, error) {
				if operation == "open" {
					return nil, injected
				}
				f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
				if err != nil {
					return nil, err
				}
				created = &stagingFaultFile{File: f}
				switch operation {
				case "write":
					created.writeErr = injected
				case "sync":
					created.syncErr = injected
				case "close":
					created.closeErr = injected
				}
				return created, nil
			})
			if !errors.Is(err, injected) || dir != "" || path != "" {
				t.Fatalf("staging = (%q, %q, %v), want empty paths and %v", dir, path, err, injected)
			}
			if operation != "open" && (created == nil || !created.closed) {
				t.Error("staged file was not closed")
			}
			assertNoStages(t, root)
		})
	}
}
