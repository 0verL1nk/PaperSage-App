package updater

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"io"
	"net/http"

	wailsupdater "github.com/wailsapp/wails/v3/pkg/updater"
	"github.com/wailsapp/wails/v3/pkg/updater/providers/github"
)

const installerAssetName = "papersage-windows-amd64-installer.exe"

type installerProvider struct {
	wailsupdater.Provider
}

func newInstallerProvider(repository, baseURL string, client *http.Client) (wailsupdater.Provider, error) {
	provider, err := github.New(github.Config{
		Repository:    repository,
		BaseURL:       baseURL,
		HTTPClient:    client,
		ChecksumAsset: "SHA256SUMS",
		AssetMatcher: func(req wailsupdater.CheckRequest, assets []github.ReleaseAsset) int {
			if req.Platform != "windows" || req.Arch != "amd64" {
				return -1
			}
			match := -1
			for i, asset := range assets {
				if asset.Name == installerAssetName {
					if match != -1 {
						return -1
					}
					match = i
				}
			}
			return match
		},
	})
	if err != nil {
		return nil, err
	}
	return &installerProvider{Provider: provider}, nil
}

func (p *installerProvider) Check(ctx context.Context, req wailsupdater.CheckRequest) (*wailsupdater.Release, error) {
	release, err := p.Provider.Check(ctx, req)
	if err != nil || release == nil {
		return release, err
	}
	if !hasInstallerChecksum(release) {
		return nil, errors.New("updater: installer release requires a SHA-256 checksum")
	}
	if release.Artifact.Size <= 0 {
		return nil, errors.New("updater: installer release requires a positive artifact size")
	}
	return release, nil
}

func hasInstallerChecksum(release *wailsupdater.Release) bool {
	return release != nil && release.Verification != nil &&
		release.Verification.DigestAlgo == "sha256" && len(release.Verification.Digest) == sha256.Size
}

type installerSizeWriter struct {
	dst       io.Writer
	remaining int64
}

func (w *installerSizeWriter) Write(p []byte) (int, error) {
	if int64(len(p)) > w.remaining {
		return 0, errors.New("updater: installer exceeds declared size")
	}
	n, err := w.dst.Write(p)
	w.remaining -= int64(n)
	return n, err
}

// Callers must discard dst on every error; verification finishes after streaming.
func downloadVerifiedInstaller(ctx context.Context, provider wailsupdater.Provider, release *wailsupdater.Release, dst io.Writer, progress func(int64, int64)) error {
	if release == nil || release.Artifact.Filename != installerAssetName ||
		release.Artifact.Platform != "windows" || release.Artifact.Arch != "amd64" {
		return errors.New("updater: release is not the Windows amd64 installer")
	}
	if !hasInstallerChecksum(release) {
		return errors.New("updater: installer release requires a SHA-256 checksum")
	}
	if release.Artifact.Size <= 0 {
		return errors.New("updater: installer release requires a positive artifact size")
	}
	var expected [sha256.Size]byte
	copy(expected[:], release.Verification.Digest)
	digest := sha256.New()
	bounded := &installerSizeWriter{dst: dst, remaining: release.Artifact.Size}
	if progress == nil {
		progress = func(int64, int64) {}
	}
	if err := provider.Download(ctx, release, io.MultiWriter(bounded, digest), progress); err != nil {
		return err
	}
	if bounded.remaining != 0 {
		return errors.New("updater: installer is shorter than declared size")
	}
	if subtle.ConstantTimeCompare(digest.Sum(nil), expected[:]) != 1 {
		return errors.New("updater: installer checksum mismatch")
	}
	return nil
}
