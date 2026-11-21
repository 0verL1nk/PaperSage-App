package updater

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	wailsupdater "github.com/wailsapp/wails/v3/pkg/updater"
)

type flowTestProvider struct {
	check    func(context.Context, wailsupdater.CheckRequest) (*wailsupdater.Release, error)
	download func(context.Context, *wailsupdater.Release, io.Writer, func(int64, int64)) error
}

func (flowTestProvider) Name() string { return "flow-test" }

func (p flowTestProvider) Check(ctx context.Context, req wailsupdater.CheckRequest) (*wailsupdater.Release, error) {
	return p.check(ctx, req)
}

func (p flowTestProvider) Download(ctx context.Context, release *wailsupdater.Release, dst io.Writer, progress func(int64, int64)) error {
	return p.download(ctx, release, dst, progress)
}

func flowFixtureProvider(release *wailsupdater.Release, content []byte) flowTestProvider {
	return flowTestProvider{
		check: func(context.Context, wailsupdater.CheckRequest) (*wailsupdater.Release, error) {
			return release, nil
		},
		download: func(ctx context.Context, _ *wailsupdater.Release, dst io.Writer, progress func(int64, int64)) error {
			n, err := dst.Write(content)
			if err != nil {
				return err
			}
			progress(int64(n), int64(len(content)))
			return ctx.Err()
		},
	}
}

func newFlowFixture(t *testing.T, p wailsupdater.Provider, root string) *installerFlow {
	t.Helper()
	flow, err := newInstallerFlow(p, "1.2.3", "windows", "amd64", root)
	if err != nil {
		t.Fatalf("newInstallerFlow() error = %v", err)
	}
	return flow
}

func TestInstallerFlowCheckDownloadDiscard(t *testing.T) {
	root := stagingRootWithSentinels(t)
	content := []byte("verified update contents")
	release := stagingTestRelease(content)
	release.Version = "2.0.0"
	provider := flowFixtureProvider(release, content)
	provider.check = func(_ context.Context, req wailsupdater.CheckRequest) (*wailsupdater.Release, error) {
		if req.CurrentVersion != "1.2.3" || req.Platform != "windows" || req.Arch != "amd64" {
			t.Errorf("Check request = %+v", req)
		}
		return release, nil
	}
	flow := newFlowFixture(t, provider, root)
	if _, err := flow.Download(context.Background(), nil); err == nil {
		t.Fatal("download without a successful check was accepted")
	}
	if err := flow.Discard(); err != nil {
		t.Fatalf("Discard() without a release error = %v, want idempotent success", err)
	}
	got, err := flow.Check(context.Background())
	if err != nil || got == nil || got.Version != release.Version {
		t.Fatalf("Check() = (%v, %v), want available release", got, err)
	}
	if again, err := flow.Check(context.Background()); err != nil || again == nil || again.Version != release.Version {
		t.Fatalf("repeated Check() = (%v, %v), want the same release", again, err)
	}
	var written, total int64
	path, err := flow.Download(context.Background(), func(w, n int64) { written, total = w, n })
	if err != nil {
		t.Fatalf("Download() error = %v", err)
	}
	if filepath.Dir(filepath.Dir(path)) != root || filepath.Base(path) != installerAssetName {
		t.Fatalf("download path %q is not in an isolated directory under %q", path, root)
	}
	bytesOnDisk, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(bytesOnDisk, content) {
		t.Fatalf("downloaded bytes = %q, %v; want %q", bytesOnDisk, err, content)
	}
	if written != int64(len(content)) || total != int64(len(content)) {
		t.Errorf("progress = (%d, %d)", written, total)
	}
	if _, err := flow.Download(context.Background(), nil); err == nil {
		t.Fatal("duplicate download replaced the staged installer")
	}
	if _, err := flow.Check(context.Background()); err == nil {
		t.Fatal("check while staged was accepted")
	}
	if err := flow.Discard(); err != nil {
		t.Fatalf("Discard() error = %v", err)
	}
	if _, err := os.Stat(filepath.Dir(path)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("staging directory still exists: %v", err)
	}
	assertNoStages(t, root)
	if err := flow.Discard(); err != nil {
		t.Fatalf("repeated Discard() error = %v, want idempotent success", err)
	}
	if _, err := flow.Check(context.Background()); err != nil {
		t.Fatalf("Check() after discard error = %v", err)
	}
	if err := flow.Discard(); err != nil {
		t.Fatalf("Discard() of pending release error = %v", err)
	}
	assertNoStages(t, root)
}

func TestInstallerFlowWithGitHubProvider(t *testing.T) {
	root := stagingRootWithSentinels(t)
	provider := fixtureProvider(t, releaseFixture{
		assets: []string{fixtureInstaller}, includeChecksum: true, checksum: checksumFor(fixtureContent),
	})
	flow := newFlowFixture(t, provider, root)
	release, err := flow.Check(context.Background())
	if err != nil || release == nil || release.Version != "2.0.0" {
		t.Fatalf("Check() = (%v, %v), want GitHub fixture release", release, err)
	}
	path, err := flow.Download(context.Background(), nil)
	if err != nil {
		t.Fatalf("Download() error = %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(got, fixtureContent) {
		t.Fatalf("staged bytes = %q, %v", got, err)
	}
	if err := flow.Discard(); err != nil {
		t.Fatal(err)
	}
	assertNoStages(t, root)
}

func TestInstallerFlowKeepsReleaseDigestAndMetadataPrivate(t *testing.T) {
	root := stagingRootWithSentinels(t)
	content := []byte("verified metadata fixture")
	release := stagingTestRelease(content)
	release.Verification.Signature = []byte("signature")
	release.Metadata = map[string]any{
		"github.asset.url":         "original-url",
		"github.asset.id":          int64(12345),
		"github.asset.contentType": "application/octet-stream",
	}
	provider := flowFixtureProvider(release, content)
	download := provider.download
	provider.download = func(ctx context.Context, actual *wailsupdater.Release, dst io.Writer, progress func(int64, int64)) error {
		if actual.Metadata["github.asset.url"] != "original-url" || actual.Metadata["github.asset.id"] != int64(12345) || actual.Metadata["github.asset.contentType"] != "application/octet-stream" {
			t.Errorf("download metadata changed: %#v", actual.Metadata)
		}
		if string(actual.Verification.Signature) != "signature" {
			t.Error("signature changed through another reference")
		}
		return download(ctx, actual, dst, progress)
	}
	flow := newFlowFixture(t, provider, root)
	got, err := flow.Check(context.Background())
	if err != nil || got == nil {
		t.Fatalf("Check() = (%v, %v)", got, err)
	}
	if got == release || got.Verification == release.Verification {
		t.Fatal("Check returned the provider-owned release or verification")
	}
	got.Verification.Digest[0] ^= 0xff
	got.Verification.Signature[0] = 'X'
	got.Metadata["github.asset.url"] = "caller-url"
	got.Metadata["github.asset.id"] = int64(99)
	release.Verification.Digest[1] ^= 0xff
	release.Verification.Signature[1] = 'Y'
	release.Metadata["github.asset.url"] = "provider-url"
	release.Metadata["github.asset.contentType"] = "provider-content-type"
	path, err := flow.Download(context.Background(), nil)
	if err != nil {
		t.Fatalf("Download() after mutating copies error = %v", err)
	}
	gotBytes, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(gotBytes, content) {
		t.Fatalf("staged bytes = %q, %v", gotBytes, err)
	}
	if err := flow.Discard(); err != nil {
		t.Fatal(err)
	}
	assertNoStages(t, root)
}

func TestInstallerFlowNoUpdateCheckErrorsAndCancellationCanRetry(t *testing.T) {
	root := stagingRootWithSentinels(t)
	content := []byte("installer bytes")
	release := stagingTestRelease(content)
	provider := flowFixtureProvider(release, content)
	checkErr := errors.New("check failure")
	calls := 0
	provider.check = func(context.Context, wailsupdater.CheckRequest) (*wailsupdater.Release, error) {
		calls++
		switch calls {
		case 1:
			return nil, nil
		case 2:
			return nil, checkErr
		default:
			return release, nil
		}
	}
	flow := newFlowFixture(t, provider, root)
	if got, err := flow.Check(context.Background()); got != nil || err != nil {
		t.Fatalf("no-update Check() = (%v, %v)", got, err)
	}
	if err := flow.Discard(); err != nil {
		t.Fatalf("Discard() after no update error = %v", err)
	}
	if _, err := flow.Download(context.Background(), nil); err == nil {
		t.Fatal("download after no update was accepted")
	}
	if got, err := flow.Check(context.Background()); got != nil || !errors.Is(err, checkErr) {
		t.Fatalf("failed Check() = (%v, %v), want %v", got, err, checkErr)
	}
	if err := flow.Discard(); err != nil {
		t.Fatalf("Discard() after check failure error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if got, err := flow.Check(ctx); got != nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled Check() = (%v, %v)", got, err)
	}
	if err := flow.Discard(); err != nil {
		t.Fatalf("Discard() after cancelled check error = %v", err)
	}
	if _, err := flow.Download(context.Background(), nil); err == nil {
		t.Fatal("download after cancelled check was accepted")
	}
	if got, err := flow.Check(context.Background()); err != nil || got == nil {
		t.Fatalf("retry Check() = (%v, %v)", got, err)
	}
	if err := flow.Discard(); err != nil {
		t.Fatal(err)
	}
	assertNoStages(t, root)
}

func TestInstallerFlowDownloadFailureAndCancellationAllowRetry(t *testing.T) {
	for _, operation := range []string{"download", "checksum", "cancel"} {
		t.Run(operation, func(t *testing.T) {
			root := stagingRootWithSentinels(t)
			content := []byte("installer bytes")
			release := stagingTestRelease(content)
			provider := flowFixtureProvider(release, content)
			originalDownload := provider.download
			first := true
			downloadErr := errors.New("download failed")
			provider.download = func(ctx context.Context, actual *wailsupdater.Release, dst io.Writer, progress func(int64, int64)) error {
				if !first {
					return originalDownload(ctx, actual, dst, progress)
				}
				first = false
				if operation == "download" {
					_, _ = dst.Write(content[:4])
					actual.Verification.Digest[0] ^= 0xff
					actual.Metadata = map[string]any{"mutated": true}
					return downloadErr
				}
				if operation == "checksum" {
					wrong := bytes.Repeat([]byte{'x'}, len(content))
					_, err := dst.Write(wrong)
					return err
				}
				return originalDownload(ctx, actual, dst, progress)
			}
			flow := newFlowFixture(t, provider, root)
			if _, err := flow.Check(context.Background()); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			path, err := flow.Download(ctx, func(_, _ int64) {
				if operation == "cancel" {
					cancel()
				}
			})
			if path != "" || err == nil {
				t.Fatalf("failed Download() = (%q, %v)", path, err)
			}
			if operation == "download" && !errors.Is(err, downloadErr) {
				t.Errorf("error = %v, want %v", err, downloadErr)
			}
			if operation == "cancel" && !errors.Is(err, context.Canceled) {
				t.Errorf("error = %v, want context.Canceled", err)
			}
			assertNoStages(t, root)
			path, err = flow.Download(context.Background(), nil)
			if err != nil {
				t.Fatalf("retry Download() error = %v", err)
			}
			if _, err := os.Stat(path); err != nil {
				t.Fatal(err)
			}
			if err := flow.Discard(); err != nil {
				t.Fatal(err)
			}
			assertNoStages(t, root)
		})
	}
}

func expectFlowBusy(t *testing.T, operation string, fn func() error) {
	t.Helper()
	result := make(chan error, 1)
	go func() { result <- fn() }()
	select {
	case err := <-result:
		if !errors.Is(err, errInstallerFlowBusy) {
			t.Fatalf("%s error = %v, want flow busy", operation, err)
		}
	case <-time.After(3 * time.Second):
		t.Fatalf("%s blocked instead of returning a busy error", operation)
	}
}

func TestInstallerFlowRejectsConcurrentOperations(t *testing.T) {
	root := stagingRootWithSentinels(t)
	content := []byte("installer bytes")
	release := stagingTestRelease(content)
	checkStarted := make(chan struct{}, 1)
	downloadStarted := make(chan struct{}, 1)
	allowCheck := make(chan struct{})
	allowDownload := make(chan struct{})
	var checkOnce, downloadOnce sync.Once
	unblockCheck := func() { checkOnce.Do(func() { close(allowCheck) }) }
	unblockDownload := func() { downloadOnce.Do(func() { close(allowDownload) }) }
	t.Cleanup(unblockCheck)
	t.Cleanup(unblockDownload)
	provider := flowFixtureProvider(release, content)
	provider.check = func(ctx context.Context, _ wailsupdater.CheckRequest) (*wailsupdater.Release, error) {
		checkStarted <- struct{}{}
		select {
		case <-allowCheck:
			return release, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	provider.download = func(ctx context.Context, _ *wailsupdater.Release, dst io.Writer, progress func(int64, int64)) error {
		if _, err := dst.Write(content[:4]); err != nil {
			return err
		}
		downloadStarted <- struct{}{}
		select {
		case <-allowDownload:
			if _, err := dst.Write(content[4:]); err != nil {
				return err
			}
			progress(int64(len(content)), int64(len(content)))
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	flow := newFlowFixture(t, provider, root)
	checked := make(chan error, 1)
	go func() { _, err := flow.Check(context.Background()); checked <- err }()
	<-checkStarted
	expectFlowBusy(t, "Check while checking", func() error { _, err := flow.Check(context.Background()); return err })
	expectFlowBusy(t, "Download while checking", func() error { _, err := flow.Download(context.Background(), nil); return err })
	expectFlowBusy(t, "Discard while checking", flow.Discard)
	expectFlowBusy(t, "Handoff while checking", func() error { return flow.Handoff(func(string, []byte) error { return nil }) })
	unblockCheck()
	if err := <-checked; err != nil {
		t.Fatal(err)
	}
	type result struct {
		path string
		err  error
	}
	downloaded := make(chan result, 1)
	go func() { path, err := flow.Download(context.Background(), nil); downloaded <- result{path, err} }()
	<-downloadStarted
	expectFlowBusy(t, "Check while downloading", func() error { _, err := flow.Check(context.Background()); return err })
	expectFlowBusy(t, "Download while downloading", func() error { _, err := flow.Download(context.Background(), nil); return err })
	expectFlowBusy(t, "Discard while downloading", flow.Discard)
	expectFlowBusy(t, "Handoff while downloading", func() error { return flow.Handoff(func(string, []byte) error { return nil }) })
	assertStagingSentinels(t, root)
	unblockDownload()
	got := <-downloaded
	if got.err != nil {
		t.Fatal(got.err)
	}
	if _, err := os.Stat(got.path); err != nil {
		t.Fatalf("in-flight file was removed: %v", err)
	}
	if err := flow.Discard(); err != nil {
		t.Fatal(err)
	}
	assertNoStages(t, root)
}

func TestInstallerFlowDiscardDoesNotRemoveAnotherFlowsStage(t *testing.T) {
	root := stagingRootWithSentinels(t)
	content := []byte("installer bytes")
	provider := flowFixtureProvider(stagingTestRelease(content), content)
	first := newFlowFixture(t, provider, root)
	second := newFlowFixture(t, provider, root)
	if _, err := first.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := second.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	firstPath, err := first.Download(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	secondPath, err := second.Download(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if firstPath == secondPath {
		t.Fatal("two flows used the same staging path")
	}
	if err := first.Discard(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(firstPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("discarded installer remains: %v", err)
	}
	if got, err := os.ReadFile(secondPath); err != nil || !bytes.Equal(got, content) {
		t.Fatalf("other flow installer = %q, %v", got, err)
	}
	assertStagingSentinels(t, root)
	if err := second.Discard(); err != nil {
		t.Fatal(err)
	}
	assertNoStages(t, root)
}

func TestInstallerFlowHandoffRequiresStagedInstaller(t *testing.T) {
	root := stagingRootWithSentinels(t)
	content := []byte("installer bytes")
	flow := newFlowFixture(t, flowFixtureProvider(stagingTestRelease(content), content), root)
	called := false
	launch := func(string, []byte) error { called = true; return nil }
	if err := flow.Handoff(launch); !errors.Is(err, errInstallerFlowState) {
		t.Fatalf("Handoff() before check error = %v, want invalid state", err)
	}
	if _, err := flow.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := flow.Handoff(launch); !errors.Is(err, errInstallerFlowState) {
		t.Fatalf("Handoff() before download error = %v, want invalid state", err)
	}
	if called {
		t.Fatal("launcher was called before installer was staged")
	}
	if _, err := flow.Download(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	if err := flow.Handoff(nil); err == nil {
		t.Fatal("nil launcher was accepted")
	}
	if err := flow.Discard(); err != nil {
		t.Fatalf("Discard() after nil launcher error = %v", err)
	}
	assertNoStages(t, root)
}

func TestInstallerFlowHandoffFailurePreservesStageAndDigest(t *testing.T) {
	root := stagingRootWithSentinels(t)
	content := []byte("installer bytes")
	release := stagingTestRelease(content)
	wantDigest := bytes.Clone(release.Verification.Digest)
	flow := newFlowFixture(t, flowFixtureProvider(release, content), root)
	checked, err := flow.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	checked.Verification.Digest[0] ^= 0xff
	release.Verification.Digest[1] ^= 0xff
	path, err := flow.Download(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	launchErr := errors.New("launch cancelled")
	err = flow.Handoff(func(gotPath string, digest []byte) error {
		if gotPath != path || !bytes.Equal(digest, wantDigest) {
			t.Errorf("launcher arguments = (%q, %x), want (%q, %x)", gotPath, digest, path, wantDigest)
		}
		digest[0] ^= 0xff
		return launchErr
	})
	if !errors.Is(err, launchErr) {
		t.Fatalf("Handoff() error = %v, want %v", err, launchErr)
	}
	if got, err := os.ReadFile(path); err != nil || !bytes.Equal(got, content) {
		t.Fatalf("installer after failed handoff = %q, %v", got, err)
	}
	assertStagingSentinels(t, root)
	err = flow.Handoff(func(gotPath string, digest []byte) error {
		if gotPath != path || !bytes.Equal(digest, wantDigest) {
			t.Errorf("retry launcher arguments = (%q, %x), want (%q, %x)", gotPath, digest, path, wantDigest)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("retry Handoff() error = %v", err)
	}
	if err := flow.Discard(); !errors.Is(err, errInstallerFlowState) {
		t.Fatalf("Discard() after handoff error = %v, want invalid state", err)
	}
	if _, err := flow.Check(context.Background()); !errors.Is(err, errInstallerFlowState) {
		t.Fatalf("Check() after handoff error = %v, want invalid state", err)
	}
	if _, err := flow.Download(context.Background(), nil); !errors.Is(err, errInstallerFlowState) {
		t.Fatalf("Download() after handoff error = %v, want invalid state", err)
	}
	if err := flow.Handoff(func(string, []byte) error { t.Error("launched twice"); return nil }); !errors.Is(err, errInstallerFlowState) {
		t.Fatalf("repeated Handoff() error = %v, want invalid state", err)
	}
	if got, err := os.ReadFile(path); err != nil || !bytes.Equal(got, content) {
		t.Fatalf("installer after successful handoff = %q, %v", got, err)
	}
	assertStagingSentinels(t, root)
}

func TestInstallerFlowHandoffBlocksConcurrentOperations(t *testing.T) {
	root := stagingRootWithSentinels(t)
	content := []byte("installer bytes")
	flow := newFlowFixture(t, flowFixtureProvider(stagingTestRelease(content), content), root)
	if _, err := flow.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	path, err := flow.Download(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{})
	proceed := make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(proceed) }) }
	t.Cleanup(unblock)
	finished := make(chan error, 1)
	go func() {
		finished <- flow.Handoff(func(string, []byte) error {
			close(started)
			<-proceed
			return nil
		})
	}()
	select {
	case <-started:
	case err := <-finished:
		t.Fatalf("Handoff() returned before starting launcher: %v", err)
	case <-time.After(3 * time.Second):
		t.Fatal("Handoff() did not start launcher")
	}
	expectFlowBusy(t, "Check while handing off", func() error { _, err := flow.Check(context.Background()); return err })
	expectFlowBusy(t, "Download while handing off", func() error { _, err := flow.Download(context.Background(), nil); return err })
	expectFlowBusy(t, "Discard while handing off", flow.Discard)
	expectFlowBusy(t, "Handoff while handing off", func() error { return flow.Handoff(func(string, []byte) error { return nil }) })
	expectFlowBusy(t, "nil Handoff while handing off", func() error { return flow.Handoff(nil) })
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("installer removed during handoff: %v", err)
	}
	assertStagingSentinels(t, root)
	unblock()
	if err := <-finished; err != nil {
		t.Fatalf("Handoff() error = %v", err)
	}
	if err := flow.Discard(); !errors.Is(err, errInstallerFlowState) {
		t.Fatalf("Discard() after successful handoff error = %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("installer removed after handoff: %v", err)
	}
}

func TestInstallerFlowCheckClearsReleaseWhenUpToDate(t *testing.T) {
	root := stagingRootWithSentinels(t)
	content := []byte("installer bytes")
	upToDate := false
	provider := flowFixtureProvider(stagingTestRelease(content), content)
	check := provider.check
	provider.check = func(ctx context.Context, req wailsupdater.CheckRequest) (*wailsupdater.Release, error) {
		if upToDate {
			return nil, nil
		}
		return check(ctx, req)
	}
	flow := newFlowFixture(t, provider, root)
	if got, err := flow.Check(context.Background()); err != nil || got == nil {
		t.Fatalf("Check() = (%v, %v), want a release", got, err)
	}
	upToDate = true
	if got, err := flow.Check(context.Background()); err != nil || got != nil {
		t.Fatalf("up-to-date Check() = (%v, %v), want nil", got, err)
	}
	if _, err := flow.Download(context.Background(), nil); err == nil {
		t.Fatal("Download() used a release cleared by the up-to-date check")
	}
}

func TestInstallerFlowRejectsUnsafeMetadataAndInvalidRoots(t *testing.T) {
	root := stagingRootWithSentinels(t)
	content := []byte("installer bytes")
	release := stagingTestRelease(content)
	provider := flowFixtureProvider(release, content)
	for name, value := range map[string]any{"channel": make(chan int), "nested map": map[string]string{"key": "value"}, "slice": []string{"value"}} {
		t.Run(name, func(t *testing.T) {
			release.Metadata = map[string]any{"unsupported": value}
			flow := newFlowFixture(t, provider, root)
			if got, err := flow.Check(context.Background()); got != nil || err == nil {
				t.Fatalf("Check() with non-scalar metadata = (%v, %v)", got, err)
			}
			if _, err := flow.Download(context.Background(), nil); err == nil {
				t.Fatal("download after failed metadata copy was accepted")
			}
		})
	}
	for name, invalid := range map[string]string{"empty": "", "missing": filepath.Join(root, "missing"), "file": filepath.Join(root, "sentinel")} {
		t.Run(name, func(t *testing.T) {
			if flow, err := newInstallerFlow(provider, "1.0.0", "windows", "amd64", invalid); err == nil || flow != nil {
				t.Fatalf("newInstallerFlow() = (%v, %v), want invalid root error", flow, err)
			}
		})
	}
	if flow, err := newInstallerFlow(nil, "1.0.0", "windows", "amd64", root); err == nil || flow != nil {
		t.Fatalf("newInstallerFlow(nil provider) = (%v, %v)", flow, err)
	}
	assertNoStages(t, root)
}
