package updater

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	wailsupdater "github.com/wailsapp/wails/v3/pkg/updater"
)

type stagedInstallerFile interface {
	io.Writer
	Sync() error
	Close() error
}

func stageVerifiedInstaller(ctx context.Context, provider wailsupdater.Provider, release *wailsupdater.Release, stagingRoot string, progress func(int64, int64)) (stagingDir, installerPath string, err error) {
	return stageVerifiedInstallerWithOpen(ctx, provider, release, stagingRoot, progress, func(path string) (stagedInstallerFile, error) {
		return os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	})
}

// The opener permits deterministic write, sync, and close failure tests.
func stageVerifiedInstallerWithOpen(ctx context.Context, provider wailsupdater.Provider, release *wailsupdater.Release, stagingRoot string, progress func(int64, int64), open func(string) (stagedInstallerFile, error)) (stagingDir, installerPath string, err error) {
	if stagingRoot == "" {
		return "", "", errors.New("updater: staging root is empty")
	}
	if provider == nil {
		return "", "", errors.New("updater: installer provider is nil")
	}
	info, err := os.Stat(stagingRoot)
	if err != nil {
		return "", "", fmt.Errorf("updater: inspect staging root: %w", err)
	}
	if !info.IsDir() {
		return "", "", errors.New("updater: staging root is not a directory")
	}
	stagingDir, err = os.MkdirTemp(stagingRoot, ".papersage-update-")
	if err != nil {
		return "", "", fmt.Errorf("updater: create staging directory: %w", err)
	}
	dir := stagingDir
	defer func() {
		if err != nil {
			if cleanupErr := os.RemoveAll(dir); cleanupErr != nil {
				err = errors.Join(err, fmt.Errorf("updater: remove staging directory %q: %w", dir, cleanupErr))
			}
			stagingDir, installerPath = "", ""
		}
	}()

	installerPath = filepath.Join(stagingDir, installerAssetName)
	file, err := open(installerPath)
	if err != nil {
		return "", "", fmt.Errorf("updater: create staged installer: %w", err)
	}
	err = downloadVerifiedInstaller(ctx, provider, release, file, progress)
	if err == nil {
		err = ctx.Err()
	}
	if err == nil {
		err = file.Sync()
	}
	err = errors.Join(err, file.Close())
	if err != nil {
		return "", "", fmt.Errorf("updater: stage installer: %w", err)
	}
	if err = ctx.Err(); err != nil {
		return "", "", fmt.Errorf("updater: stage installer: %w", err)
	}
	return stagingDir, installerPath, nil
}
