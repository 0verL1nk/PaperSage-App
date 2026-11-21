package updater

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	wailsupdater "github.com/wailsapp/wails/v3/pkg/updater"
)

const fixtureInstaller = "papersage-windows-amd64-installer.exe"

var fixtureContent = []byte("installer fixture bytes")

type releaseFixture struct {
	assets            []string
	content           []byte
	assetSize         *int64
	omitAssetSize     bool
	checksum          string
	includeChecksum   bool
	installerURL      string
	checksumURL       string
	redirectInstaller bool
	redirectChecksum  bool
	installerStatus   int
	installerRequests *atomic.Int32
}

type fixtureRoundTripper struct {
	host      string
	transport *http.Transport
}

func (rt fixtureRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Scheme != "http" || req.URL.Host != rt.host || (req.Host != "" && req.Host != rt.host) {
		return nil, fmt.Errorf("fixture transport refused %s", req.URL)
	}
	return rt.transport.RoundTrip(req)
}

type checkFixtureProvider struct {
	release *wailsupdater.Release
	err     error
}

func (p checkFixtureProvider) Name() string { return "fixture" }

func (p checkFixtureProvider) Check(context.Context, wailsupdater.CheckRequest) (*wailsupdater.Release, error) {
	return p.release, p.err
}

func (checkFixtureProvider) Download(context.Context, *wailsupdater.Release, io.Writer, func(int64, int64)) error {
	return errors.New("unexpected download")
}

type fixtureWriter func([]byte) (int, error)

func (w fixtureWriter) Write(p []byte) (int, error) { return w(p) }

func checksumFor(content []byte) string {
	digest := sha256.Sum256(content)
	return fmt.Sprintf("%x  %s\n", digest, fixtureInstaller)
}

// The real GitHub provider must not be able to reach anything outside this fixture.
func fixtureProvider(t *testing.T, fixture releaseFixture) wailsupdater.Provider {
	t.Helper()

	content := fixture.content
	if content == nil {
		content = fixtureContent
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/fixture/project/releases/latest":
			assetURL := "http://" + r.Host + "/assets/installer"
			if fixture.installerURL != "" {
				assetURL = fixture.installerURL
			}
			checksumURL := "http://" + r.Host + "/assets/sums"
			if fixture.checksumURL != "" {
				checksumURL = fixture.checksumURL
			}
			assets := make([]map[string]any, 0, len(fixture.assets)+1)
			for _, name := range fixture.assets {
				asset := map[string]any{"name": name, "url": assetURL}
				if !fixture.omitAssetSize {
					size := int64(len(content))
					if fixture.assetSize != nil {
						size = *fixture.assetSize
					}
					asset["size"] = size
				}
				assets = append(assets, asset)
			}
			if fixture.includeChecksum {
				assets = append(assets, map[string]any{"name": "SHA256SUMS", "url": checksumURL})
			}
			if err := json.NewEncoder(w).Encode(map[string]any{"tag_name": "v2.0.0", "assets": assets}); err != nil {
				t.Errorf("encode release fixture: %v", err)
			}
		case "/assets/sums":
			if fixture.redirectChecksum {
				http.Redirect(w, r, "/unexpected-redirect-target", http.StatusFound)
				return
			}
			_, _ = io.WriteString(w, fixture.checksum)
		case "/assets/installer":
			if fixture.installerRequests != nil {
				fixture.installerRequests.Add(1)
			}
			if fixture.redirectInstaller {
				http.Redirect(w, r, "/unexpected-redirect-target", http.StatusFound)
				return
			}
			if fixture.installerStatus != 0 {
				w.WriteHeader(fixture.installerStatus)
				return
			}
			_, _ = w.Write(content)
		default:
			t.Errorf("unexpected fixture request: %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	fixtureURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	transport := &http.Transport{Proxy: nil}
	t.Cleanup(transport.CloseIdleConnections)
	client := &http.Client{
		Transport: fixtureRoundTripper{host: fixtureURL.Host, transport: transport},
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return errors.New("fixture redirects are forbidden")
		},
	}
	provider, err := newInstallerProvider("fixture/project", server.URL, client)
	if err != nil {
		t.Fatalf("newInstallerProvider: %v", err)
	}
	return provider
}

func checkInstaller(t *testing.T, provider wailsupdater.Provider) *wailsupdater.Release {
	t.Helper()
	release, err := provider.Check(context.Background(), wailsupdater.CheckRequest{
		CurrentVersion: "1.0.0", Platform: "windows", Arch: "amd64",
	})
	if err != nil || release == nil {
		t.Fatalf("Check() = (%v, %v), want a release", release, err)
	}
	return release
}

func TestInstallerProviderSelectsExactlyOneWindowsAMD64Asset(t *testing.T) {
	tests := []struct {
		name     string
		assets   []string
		platform string
		arch     string
		want     bool
	}{
		{"exact asset after standalone", []string{"papersage-windows-amd64.exe", fixtureInstaller}, "windows", "amd64", true},
		{"missing installer", []string{"papersage-windows-amd64.exe"}, "windows", "amd64", false},
		{"duplicate installer", []string{fixtureInstaller, fixtureInstaller}, "windows", "amd64", false},
		{"case differs", []string{strings.ToUpper(fixtureInstaller)}, "windows", "amd64", false},
		{"extra prefix", []string{"other-" + fixtureInstaller}, "windows", "amd64", false},
		{"extra suffix", []string{fixtureInstaller + ".exe"}, "windows", "amd64", false},
		{"wrong platform", []string{fixtureInstaller}, "linux", "amd64", false},
		{"wrong architecture", []string{fixtureInstaller}, "windows", "arm64", false},
		{"empty platform", []string{fixtureInstaller}, "", "amd64", false},
		{"empty architecture", []string{fixtureInstaller}, "windows", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := fixtureProvider(t, releaseFixture{assets: tt.assets, includeChecksum: true, checksum: checksumFor(fixtureContent)})
			release, err := provider.Check(context.Background(), wailsupdater.CheckRequest{
				CurrentVersion: "1.0.0", Platform: tt.platform, Arch: tt.arch,
			})
			if !tt.want {
				if err == nil || release != nil {
					t.Fatalf("Check() = (%v, %v), want rejection", release, err)
				}
				return
			}
			if err != nil || release == nil {
				t.Fatalf("Check() = (%v, %v), want a release", release, err)
			}
			digest := sha256.Sum256(fixtureContent)
			if release.Version != "2.0.0" || release.Artifact.Filename != fixtureInstaller ||
				release.Artifact.Platform != "windows" || release.Artifact.Arch != "amd64" {
				t.Errorf("unexpected release: %+v", release)
			}
			if release.Verification == nil || release.Verification.DigestAlgo != "sha256" ||
				!bytes.Equal(release.Verification.Digest, digest[:]) {
				t.Errorf("unexpected verification: %+v", release.Verification)
			}
		})
	}
}

func TestInstallerProviderPreservesUpToDateResult(t *testing.T) {
	provider := fixtureProvider(t, releaseFixture{}) // no installer or checksum should be needed
	for _, version := range []string{"2.0.0", "3.0.0"} {
		t.Run(version, func(t *testing.T) {
			release, err := provider.Check(context.Background(), wailsupdater.CheckRequest{
				CurrentVersion: version, Platform: "windows", Arch: "amd64",
			})
			if release != nil || err != nil {
				t.Fatalf("Check() = (%v, %v), want (nil, nil)", release, err)
			}
		})
	}
}

func TestInstallerProviderRequiresReleaseSHA256(t *testing.T) {
	tests := []struct {
		name     string
		include  bool
		checksum string
	}{
		{"missing SHA256SUMS", false, ""},
		{"no matching target", true, strings.Repeat("0", 64) + "  another.exe\n"},
		{"malformed digest", true, "not-hex  " + fixtureInstaller + "\n"},
		{"digest too short", true, strings.Repeat("0", 62) + "  " + fixtureInstaller + "\n"},
		{"digest too long", true, strings.Repeat("0", 66) + "  " + fixtureInstaller + "\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := fixtureProvider(t, releaseFixture{assets: []string{fixtureInstaller}, includeChecksum: tt.include, checksum: tt.checksum})
			release, err := provider.Check(context.Background(), wailsupdater.CheckRequest{
				CurrentVersion: "1.0.0", Platform: "windows", Arch: "amd64",
			})
			if release != nil || err == nil {
				t.Fatalf("Check() = (%v, %v), want checksum rejection", release, err)
			}
		})
	}
}

func TestInstallerProviderRequiresPositiveArtifactSize(t *testing.T) {
	tests := []struct {
		name string
		size int64
		omit bool
	}{
		{"missing size", 0, true},
		{"zero size", 0, false},
		{"negative size", -1, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := fixtureProvider(t, releaseFixture{
				assets: []string{fixtureInstaller}, includeChecksum: true, checksum: checksumFor(fixtureContent),
				assetSize: &tt.size, omitAssetSize: tt.omit,
			})
			release, err := provider.Check(context.Background(), wailsupdater.CheckRequest{
				CurrentVersion: "1.0.0", Platform: "windows", Arch: "amd64",
			})
			if release != nil || err == nil {
				t.Fatalf("Check() = (%v, %v), want invalid-size rejection", release, err)
			}
		})
	}
}

// GitHub always labels parsed sidecars sha256, so inject a provider to exercise
// the wrapper's algorithm check independently of GitHub's parser.
func TestInstallerProviderCheckRejectsWrongDigestAlgorithm(t *testing.T) {
	for _, algorithm := range []string{"", "sha512", "SHA256"} {
		t.Run(algorithm, func(t *testing.T) {
			provider := &installerProvider{Provider: checkFixtureProvider{release: &wailsupdater.Release{
				Verification: &wailsupdater.Verification{DigestAlgo: algorithm, Digest: make([]byte, sha256.Size)},
			}}}
			release, err := provider.Check(context.Background(), wailsupdater.CheckRequest{})
			if release != nil || err == nil {
				t.Fatalf("Check() = (%v, %v), want algorithm rejection", release, err)
			}
		})
	}
}

func TestDownloadVerifiedInstaller(t *testing.T) {
	tests := []struct {
		name     string
		checksum string
		wantErr  bool
	}{
		{"matching digest", checksumFor(fixtureContent), false},
		{"mismatched digest", checksumFor([]byte("different content")), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := fixtureProvider(t, releaseFixture{assets: []string{fixtureInstaller}, includeChecksum: true, checksum: tt.checksum})
			release := checkInstaller(t, provider)
			var output bytes.Buffer
			var progressCalls int
			var lastWritten int64
			err := downloadVerifiedInstaller(context.Background(), provider, release, &output, func(written, total int64) {
				progressCalls++
				if written <= lastWritten || written > int64(len(fixtureContent)) || total != int64(len(fixtureContent)) {
					t.Errorf("progress = (%d, %d), previous written = %d", written, total, lastWritten)
				}
				lastWritten = written
			})
			if (err != nil) != tt.wantErr {
				t.Fatalf("downloadVerifiedInstaller() error = %v, want error %t", err, tt.wantErr)
			}
			if !bytes.Equal(output.Bytes(), fixtureContent) || progressCalls == 0 || lastWritten != int64(len(fixtureContent)) {
				t.Errorf("output = %q, progress calls = %d, last written = %d", output.Bytes(), progressCalls, lastWritten)
			}
		})
	}
}

func TestDownloadVerifiedInstallerStreamsMultipleChunks(t *testing.T) {
	content := bytes.Repeat([]byte("streamed installer data"), 1<<14)
	provider := fixtureProvider(t, releaseFixture{assets: []string{fixtureInstaller}, content: content, includeChecksum: true, checksum: checksumFor(content)})
	release := checkInstaller(t, provider)
	var output bytes.Buffer
	var progressCalls int
	err := downloadVerifiedInstaller(context.Background(), provider, release, &output, func(_, _ int64) {
		progressCalls++
	})
	if err != nil || !bytes.Equal(output.Bytes(), content) || progressCalls < 2 {
		t.Fatalf("download error = %v, received %d/%d bytes in %d callbacks", err, output.Len(), len(content), progressCalls)
	}
}

func TestDownloadVerifiedInstallerRequiresSHA256BeforeDownload(t *testing.T) {
	var requests atomic.Int32
	provider := fixtureProvider(t, releaseFixture{assets: []string{fixtureInstaller}, includeChecksum: true, checksum: checksumFor(fixtureContent), installerRequests: &requests})
	release := checkInstaller(t, provider)
	tests := []struct {
		name         string
		verification *wailsupdater.Verification
	}{
		{"missing verification", nil},
		{"wrong algorithm", &wailsupdater.Verification{DigestAlgo: "sha512", Digest: make([]byte, sha256.Size)}},
		{"empty algorithm", &wailsupdater.Verification{Digest: make([]byte, sha256.Size)}},
		{"short digest", &wailsupdater.Verification{DigestAlgo: "sha256", Digest: make([]byte, sha256.Size-1)}},
		{"long digest", &wailsupdater.Verification{DigestAlgo: "sha256", Digest: make([]byte, sha256.Size+1)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			invalid := *release
			invalid.Verification = tt.verification
			if err := downloadVerifiedInstaller(context.Background(), provider, &invalid, io.Discard, nil); err == nil {
				t.Fatal("downloadVerifiedInstaller() accepted invalid checksum metadata")
			}
		})
	}
	if got := requests.Load(); got != 0 {
		t.Errorf("download requests = %d, want zero", got)
	}
	if err := downloadVerifiedInstaller(context.Background(), provider, nil, io.Discard, nil); err == nil {
		t.Error("downloadVerifiedInstaller() accepted a nil release")
	}
}

func TestDownloadVerifiedInstallerRequiresPositiveSizeBeforeDownload(t *testing.T) {
	var requests atomic.Int32
	provider := fixtureProvider(t, releaseFixture{assets: []string{fixtureInstaller}, includeChecksum: true, checksum: checksumFor(fixtureContent), installerRequests: &requests})
	release := checkInstaller(t, provider)
	for _, size := range []int64{0, -1} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			invalid := *release
			invalid.Artifact.Size = size
			before := requests.Load()
			var output bytes.Buffer
			err := downloadVerifiedInstaller(context.Background(), provider, &invalid, &output, nil)
			if err == nil || requests.Load() != before || output.Len() != 0 {
				t.Errorf("error = %v, download requests = %d, output bytes = %d; want invalid-size rejection before download", err, requests.Load()-before, output.Len())
			}
		})
	}
}

func TestDownloadVerifiedInstallerEnforcesDeclaredSize(t *testing.T) {
	content := bytes.Repeat([]byte("streamed installer content"), 1<<14)
	tests := []struct {
		name    string
		delta   int64
		wantErr bool
	}{
		{"matching size", 0, false},
		{"download exceeds declared size", -1, true},
		{"download shorter than declared size", 1, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			size := int64(len(content)) + tt.delta
			provider := fixtureProvider(t, releaseFixture{
				assets: []string{fixtureInstaller}, content: content, assetSize: &size,
				includeChecksum: true, checksum: checksumFor(content),
			})
			release := checkInstaller(t, provider)
			var output bytes.Buffer
			err := downloadVerifiedInstaller(context.Background(), provider, release, &output, nil)
			if (err != nil) != tt.wantErr || int64(output.Len()) > size {
				t.Errorf("error = %v, output bytes = %d, declared = %d; want error %t without exceeding size", err, output.Len(), size, tt.wantErr)
			}
			if tt.delta >= 0 && !bytes.Equal(output.Bytes(), content) {
				t.Errorf("output differs from the %d-byte response", len(content))
			}
		})
	}
}

func TestDownloadVerifiedInstallerRejectsWrongArtifactBeforeDownload(t *testing.T) {
	var requests atomic.Int32
	provider := fixtureProvider(t, releaseFixture{assets: []string{fixtureInstaller}, includeChecksum: true, checksum: checksumFor(fixtureContent), installerRequests: &requests})
	release := checkInstaller(t, provider)
	tests := []struct {
		name   string
		mutate func(*wailsupdater.Artifact)
	}{
		{"standalone executable", func(a *wailsupdater.Artifact) { a.Filename = "papersage-windows-amd64.exe" }},
		{"lookalike filename", func(a *wailsupdater.Artifact) { a.Filename = "other-" + fixtureInstaller }},
		{"wrong filename case", func(a *wailsupdater.Artifact) { a.Filename = strings.ToUpper(fixtureInstaller) }},
		{"empty filename", func(a *wailsupdater.Artifact) { a.Filename = "" }},
		{"wrong platform", func(a *wailsupdater.Artifact) { a.Platform = "linux" }},
		{"empty platform", func(a *wailsupdater.Artifact) { a.Platform = "" }},
		{"wrong architecture", func(a *wailsupdater.Artifact) { a.Arch = "arm64" }},
		{"empty architecture", func(a *wailsupdater.Artifact) { a.Arch = "" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			invalid := *release
			tt.mutate(&invalid.Artifact)
			before := requests.Load()
			var output bytes.Buffer
			err := downloadVerifiedInstaller(context.Background(), provider, &invalid, &output, nil)
			if err == nil || requests.Load() != before || output.Len() != 0 {
				t.Errorf("error = %v, download requests = %d, output bytes = %d; want rejection before download", err, requests.Load()-before, output.Len())
			}
		})
	}
}

func TestDownloadVerifiedInstallerPropagatesErrors(t *testing.T) {
	t.Run("cancelled context", func(t *testing.T) {
		provider := fixtureProvider(t, releaseFixture{assets: []string{fixtureInstaller}, includeChecksum: true, checksum: checksumFor(fixtureContent)})
		release := checkInstaller(t, provider)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if err := downloadVerifiedInstaller(ctx, provider, release, io.Discard, nil); !errors.Is(err, context.Canceled) {
			t.Fatalf("error = %v, want context.Canceled", err)
		}
	})
	t.Run("HTTP failure", func(t *testing.T) {
		provider := fixtureProvider(t, releaseFixture{assets: []string{fixtureInstaller}, includeChecksum: true, checksum: checksumFor(fixtureContent), installerStatus: http.StatusServiceUnavailable})
		release := checkInstaller(t, provider)
		if err := downloadVerifiedInstaller(context.Background(), provider, release, io.Discard, nil); err == nil || !strings.Contains(err.Error(), "503") {
			t.Fatalf("error = %v, want HTTP 503", err)
		}
	})
	t.Run("destination failure", func(t *testing.T) {
		provider := fixtureProvider(t, releaseFixture{assets: []string{fixtureInstaller}, includeChecksum: true, checksum: checksumFor(fixtureContent)})
		release := checkInstaller(t, provider)
		writeErr := errors.New("destination failure")
		writer := fixtureWriter(func([]byte) (int, error) { return 0, writeErr })
		if err := downloadVerifiedInstaller(context.Background(), provider, release, writer, nil); !errors.Is(err, writeErr) {
			t.Fatalf("error = %v, want destination failure", err)
		}
	})
	t.Run("short write", func(t *testing.T) {
		provider := fixtureProvider(t, releaseFixture{assets: []string{fixtureInstaller}, includeChecksum: true, checksum: checksumFor(fixtureContent)})
		release := checkInstaller(t, provider)
		writer := fixtureWriter(func([]byte) (int, error) { return 0, nil })
		if err := downloadVerifiedInstaller(context.Background(), provider, release, writer, nil); !errors.Is(err, io.ErrShortWrite) {
			t.Fatalf("error = %v, want io.ErrShortWrite", err)
		}
	})
}

func TestFixtureClientRejectsExternalAssetsAndRedirects(t *testing.T) {
	tests := []struct {
		name        string
		fixture     releaseFixture
		failOnCheck bool
	}{
		{"external checksum", releaseFixture{checksumURL: "http://external.invalid/checksum"}, true},
		{"checksum redirect", releaseFixture{redirectChecksum: true}, true},
		{"external installer", releaseFixture{installerURL: "http://external.invalid/installer"}, false},
		{"installer redirect", releaseFixture{redirectInstaller: true}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.fixture.assets = []string{fixtureInstaller}
			tt.fixture.includeChecksum = true
			tt.fixture.checksum = checksumFor(fixtureContent)
			provider := fixtureProvider(t, tt.fixture)
			release, err := provider.Check(context.Background(), wailsupdater.CheckRequest{
				CurrentVersion: "1.0.0", Platform: "windows", Arch: "amd64",
			})
			if tt.failOnCheck {
				if err == nil || release != nil {
					t.Fatalf("Check() = (%v, %v), want rejection", release, err)
				}
				return
			}
			if err != nil || release == nil {
				t.Fatalf("Check() = (%v, %v), want a release", release, err)
			}
			if err := downloadVerifiedInstaller(context.Background(), provider, release, io.Discard, nil); err == nil {
				t.Fatal("downloadVerifiedInstaller() accepted a forbidden URL or redirect")
			}
		})
	}
}

func TestNewInstallerProviderUsesDefaultClient(t *testing.T) {
	provider, err := newInstallerProvider("fixture/project", "", nil)
	if err != nil || provider == nil || provider.Name() != "github" {
		t.Fatalf("newInstallerProvider() = (%v, %v), want GitHub provider", provider, err)
	}
}

func TestNewInstallerProviderRejectsInvalidRepository(t *testing.T) {
	for _, repo := range []string{"", "invalid"} {
		if _, err := newInstallerProvider(repo, "", nil); err == nil {
			t.Errorf("newInstallerProvider(%q) succeeded", repo)
		}
	}
}
