package updater

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"sync"

	wailsupdater "github.com/wailsapp/wails/v3/pkg/updater"
)

var (
	errInstallerFlowBusy  = errors.New("updater: installer flow operation in progress")
	errInstallerFlowState = errors.New("updater: installer flow operation not allowed in current state")
)

type installerFlowState uint8

const (
	installerFlowIdle installerFlowState = iota
	installerFlowChecking
	installerFlowAvailable
	installerFlowDownloading
	installerFlowStaged
	installerFlowDiscarding
	installerFlowHandingOff
	installerFlowHandedOff
)

type installerFlow struct {
	mu             sync.Mutex
	provider       wailsupdater.Provider
	currentVersion string
	platform       string
	arch           string
	stagingRoot    string
	state          installerFlowState
	release        *wailsupdater.Release
	stagingDir     string
	installerPath  string
}

func newInstallerFlow(provider wailsupdater.Provider, currentVersion, platform, arch, stagingRoot string) (*installerFlow, error) {
	if provider == nil {
		return nil, errors.New("updater: installer provider is nil")
	}
	if stagingRoot == "" {
		return nil, errors.New("updater: staging root is empty")
	}
	info, err := os.Stat(stagingRoot)
	if err != nil {
		return nil, fmt.Errorf("updater: inspect staging root: %w", err)
	}
	if !info.IsDir() {
		return nil, errors.New("updater: staging root is not a directory")
	}
	return &installerFlow{provider: provider, currentVersion: currentVersion, platform: platform, arch: arch, stagingRoot: stagingRoot}, nil
}

func (f *installerFlow) begin(expected, next installerFlowState) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.state == installerFlowChecking || f.state == installerFlowDownloading || f.state == installerFlowDiscarding || f.state == installerFlowHandingOff {
		return errInstallerFlowBusy
	}
	if f.state != expected {
		return errInstallerFlowState
	}
	f.state = next
	return nil
}

// beginCheck permits a refresh after a previous check found an update.
func (f *installerFlow) beginCheck() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	switch f.state {
	case installerFlowIdle, installerFlowAvailable:
	default:
		if f.state == installerFlowChecking || f.state == installerFlowDownloading ||
			f.state == installerFlowDiscarding || f.state == installerFlowHandingOff {
			return errInstallerFlowBusy
		}
		return errInstallerFlowState
	}
	f.state = installerFlowChecking
	return nil
}

func (f *installerFlow) Check(ctx context.Context) (*wailsupdater.Release, error) {
	if err := f.beginCheck(); err != nil {
		return nil, err
	}
	release, err := f.provider.Check(ctx, wailsupdater.CheckRequest{
		CurrentVersion: f.currentVersion,
		Platform:       f.platform,
		Arch:           f.arch,
	})
	var stored, result *wailsupdater.Release
	if err == nil {
		err = ctx.Err()
	}
	if err == nil && release != nil {
		stored, err = cloneInstallerRelease(release)
		if err == nil {
			result, err = cloneInstallerRelease(stored)
		}
	}
	if err == nil {
		err = ctx.Err()
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if err != nil || stored == nil {
		f.release = nil
		f.state = installerFlowIdle
		return nil, err
	}
	f.release = stored
	f.state = installerFlowAvailable
	return result, nil
}

func (f *installerFlow) Download(ctx context.Context, progress func(int64, int64)) (string, error) {
	if err := f.begin(installerFlowAvailable, installerFlowDownloading); err != nil {
		return "", err
	}
	release, err := cloneInstallerRelease(f.release)
	var dir, path string
	if err == nil {
		dir, path, err = stageVerifiedInstaller(ctx, f.provider, release, f.stagingRoot, progress)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if err != nil {
		f.state = installerFlowAvailable
		return "", err
	}
	f.stagingDir = dir
	f.installerPath = path
	f.state = installerFlowStaged
	return path, nil
}

func (f *installerFlow) Handoff(launch func(path string, digest []byte) error) error {
	if err := f.begin(installerFlowStaged, installerFlowHandingOff); err != nil {
		return err
	}
	var err error
	if launch == nil {
		err = errors.New("updater: installer launcher is nil")
	} else {
		err = launch(f.installerPath, bytes.Clone(f.release.Verification.Digest))
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if err != nil {
		f.state = installerFlowStaged
		return err
	}
	f.state = installerFlowHandedOff
	return nil
}

func (f *installerFlow) Discard() error {
	f.mu.Lock()
	if f.state == installerFlowIdle {
		f.mu.Unlock()
		return nil
	}
	if f.state == installerFlowChecking || f.state == installerFlowDownloading || f.state == installerFlowDiscarding || f.state == installerFlowHandingOff {
		f.mu.Unlock()
		return errInstallerFlowBusy
	}
	previous := f.state
	if previous != installerFlowAvailable && previous != installerFlowStaged {
		f.mu.Unlock()
		return errInstallerFlowState
	}
	dir := f.stagingDir
	f.state = installerFlowDiscarding
	f.mu.Unlock()

	var err error
	if dir != "" {
		if removeErr := os.RemoveAll(dir); removeErr != nil {
			err = fmt.Errorf("updater: discard staging directory: %w", removeErr)
		}
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if err != nil {
		f.state = previous
		return err
	}
	f.release = nil
	f.stagingDir = ""
	f.installerPath = ""
	f.state = installerFlowIdle
	return nil
}

func cloneInstallerRelease(release *wailsupdater.Release) (*wailsupdater.Release, error) {
	if release == nil {
		return nil, nil
	}
	copied := *release
	if release.Verification != nil {
		verification := *release.Verification
		verification.Digest = bytes.Clone(release.Verification.Digest)
		verification.Signature = bytes.Clone(release.Verification.Signature)
		copied.Verification = &verification
	}
	if release.Metadata != nil {
		copied.Metadata = make(map[string]any, len(release.Metadata))
		for key, value := range release.Metadata {
			switch value.(type) {
			case nil, bool, string, int, int8, int16, int32, int64,
				uint, uint8, uint16, uint32, uint64, float32, float64:
				copied.Metadata[key] = value
			default:
				return nil, fmt.Errorf("updater: unsupported release metadata %q type %T", key, value)
			}
		}
	}
	return &copied, nil
}
