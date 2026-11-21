// Package updater checks GitHub for a newer desktop release, downloads the
// complete installer over a pinned handle, and hands it to a detached helper.
package updater

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Desktop releases ship from the public app repository.
const repository = "0verL1nk/PaperSage-App"

// EventUpdateError carries updater failures to the web layer.
const EventUpdateError = "desktop:update-error"

var flow *installerFlow

// Setup prepares the verified installer flow. It does not start any background
// download or check.
func Setup(version string) error {
	root, err := stagingRoot()
	if err != nil {
		return err
	}
	cleanStaleStaging(root)
	// The default Wails client times out at 30s, too short for a ~50 MB
	// installer on a slow link; the context adds no deadline of its own.
	provider, err := newInstallerProvider(repository, "", &http.Client{Timeout: 15 * time.Minute})
	if err != nil {
		return err
	}
	created, err := newInstallerFlow(provider, strings.TrimPrefix(strings.TrimSpace(version), "v"), "windows", "amd64", root)
	if err != nil {
		return err
	}
	flow = created
	return nil
}

func stagingRoot() (string, error) {
	// Temp, not the user cache: elevated launch of a file under a redirected or
	// EFS-encrypted LocalAppData path fails with ERROR_PATH_NOT_FOUND.
	root := filepath.Join(os.TempDir(), "PaperSage-updates")
	if err := os.MkdirAll(root, 0700); err != nil {
		return "", fmt.Errorf("updater: create staging directory: %w", err)
	}
	return root, nil
}

// cleanStaleStaging drops leftovers from an update that was interrupted. Very
// recent directories are kept: a helper may still be starting against one, and
// the app can be relaunched while an update is in flight.
func cleanStaleStaging(root string) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return
	}
	cutoff := time.Now().Add(-time.Hour)
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), ".papersage-update-") {
			continue
		}
		info, err := entry.Info()
		if err != nil || info.ModTime().After(cutoff) {
			continue
		}
		if err := os.RemoveAll(filepath.Join(root, entry.Name())); err != nil {
			log.Printf("desktop: clean stale update directory: %v", err)
		}
	}
}

// UpdateInfo is the read-only release summary shown before downloading.
type UpdateInfo struct {
	Version string `json:"version"`
	Name    string `json:"name,omitempty"`
	Notes   string `json:"notes,omitempty"`
}

// Check looks for a newer release and keeps it for Download. A nil result means
// the current version is up to date.
func Check(ctx context.Context) (*UpdateInfo, error) {
	if flow == nil {
		return nil, errors.New("updater: Setup has not been called")
	}
	release, err := flow.Check(ctx)
	if err != nil || release == nil {
		return nil, err
	}
	return &UpdateInfo{Version: release.Version, Name: release.Name, Notes: release.Notes}, nil
}

// Download fetches and verifies the installer staged by Check.
func Download(ctx context.Context, progress func(received, total int64)) error {
	if flow == nil {
		return errors.New("updater: Setup has not been called")
	}
	_, err := flow.Download(ctx, progress)
	return err
}

// Handoff starts the detached helper that installs the staged installer. The
// caller quits the application; the helper restarts it after the install.
func Handoff() error {
	if flow == nil {
		return errors.New("updater: Setup has not been called")
	}
	return flow.Handoff(spawnUpdateHelper)
}

// Discard removes a downloaded installer that will not be used.
func Discard() error {
	if flow == nil {
		return errors.New("updater: Setup has not been called")
	}
	return flow.Discard()
}
