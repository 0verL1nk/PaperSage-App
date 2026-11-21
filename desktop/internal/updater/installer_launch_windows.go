package updater

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
)

// launchVerifiedInstaller is called only with a path and digest obtained from staging.
func launchVerifiedInstaller(stagedPath string, expectedDigest []byte) error {
	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("updater: determine current executable: %w", err)
	}
	return launchVerifiedInstallerForExecutable(stagedPath, expectedDigest, executable, func(path, args string) error {
		verb, err := windows.UTF16PtrFromString("runas")
		if err != nil {
			return err
		}
		file, err := windows.UTF16PtrFromString(path)
		if err != nil {
			return err
		}
		parameters, err := windows.UTF16PtrFromString(args)
		if err != nil {
			return err
		}
		return windows.ShellExecute(0, verb, file, parameters, nil, windows.SW_SHOWNORMAL)
	}, 60_000)
}

func launchVerifiedInstallerForExecutable(stagedPath string, expectedDigest []byte, executable string, launch func(string, string) error, readyTimeoutMillis uint32) error {
	return launchVerifiedInstallerWithEvents(stagedPath, expectedDigest, executable, launch, readyTimeoutMillis, windows.SetEvent, windows.CloseHandle)
}

func launchVerifiedInstallerWithEvents(stagedPath string, expectedDigest []byte, executable string, launch func(string, string) error, readyTimeoutMillis uint32, signalEvent func(windows.Handle) error, closeEvent func(windows.Handle) error) error {
	if len(expectedDigest) != sha256.Size {
		return errors.New("updater: installer requires a 32-byte SHA-256 checksum")
	}
	if !validInstallerPath(stagedPath) || filepath.Base(stagedPath) != installerAssetName {
		return errors.New("updater: invalid staged installer path")
	}
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return fmt.Errorf("updater: generate installer handoff nonce: %w", err)
	}
	pid := os.Getpid()
	nonce := hex.EncodeToString(random[:])
	args, err := buildInstallerLaunchArguments(executable, pid, nonce)
	if err != nil {
		return err
	}
	eventBase := fmt.Sprintf(`Local\PaperSageUpdate-%d-%s`, pid, nonce)
	ready, err := createInstallerHandoffEvent(eventBase + "-ready")
	if err != nil {
		return fmt.Errorf("updater: create installer ready event: %w", err)
	}
	// Cleanup must not turn an already committed handoff into a failure.
	defer closeEvent(ready)
	commit, err := createInstallerHandoffEvent(eventBase + "-commit")
	if err != nil {
		return fmt.Errorf("updater: create installer commit event: %w", err)
	}
	defer closeEvent(commit)
	path, err := windows.UTF16PtrFromString(stagedPath)
	if err != nil {
		return fmt.Errorf("updater: invalid staged installer path: %w", err)
	}
	handle, err := windows.CreateFile(path, windows.GENERIC_READ, windows.FILE_SHARE_READ, nil,
		windows.OPEN_EXISTING, windows.FILE_FLAG_OPEN_REPARSE_POINT|windows.FILE_FLAG_SEQUENTIAL_SCAN, 0)
	if err != nil {
		return fmt.Errorf("updater: open staged installer: %w", err)
	}
	file := os.NewFile(uintptr(handle), stagedPath)
	defer file.Close()

	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(handle, &info); err != nil {
		return fmt.Errorf("updater: inspect staged installer: %w", err)
	}
	kind, err := windows.GetFileType(handle)
	if err != nil {
		return fmt.Errorf("updater: inspect staged installer type: %w", err)
	}
	if kind != windows.FILE_TYPE_DISK || info.FileAttributes&(windows.FILE_ATTRIBUTE_DIRECTORY|windows.FILE_ATTRIBUTE_REPARSE_POINT) != 0 {
		return errors.New("updater: staged installer is not a regular non-reparse file")
	}
	digest := sha256.New()
	if _, err := io.Copy(digest, file); err != nil {
		return fmt.Errorf("updater: hash staged installer: %w", err)
	}
	if subtle.ConstantTimeCompare(digest.Sum(nil), expectedDigest) != 1 {
		return errors.New("updater: staged installer checksum mismatch")
	}
	launchErr := launch(stagedPath, args)
	closeErr := file.Close()
	if launchErr != nil {
		if errors.Is(launchErr, windows.ERROR_CANCELLED) {
			return fmt.Errorf("updater: installer launch cancelled by user: %w", launchErr)
		}
		return fmt.Errorf("updater: launch installer: %w", launchErr)
	}
	// ShellExecute does not provide a process handle, so later failures remain pending.
	if closeErr != nil {
		return fmt.Errorf("%w: close staged installer after launch: %w", ErrInstallerHandoffPending, closeErr)
	}
	result, err := windows.WaitForSingleObject(ready, readyTimeoutMillis)
	if err != nil {
		return fmt.Errorf("%w: wait for installer readiness: %w", ErrInstallerHandoffPending, err)
	}
	if result == uint32(windows.WAIT_TIMEOUT) {
		return fmt.Errorf("%w: timed out waiting for installer readiness", ErrInstallerHandoffPending)
	}
	if result != windows.WAIT_OBJECT_0 {
		return fmt.Errorf("%w: unexpected installer readiness result %d", ErrInstallerHandoffPending, result)
	}
	if err := signalEvent(commit); err != nil {
		return fmt.Errorf("%w: commit installer handoff: %w", ErrInstallerHandoffPending, err)
	}
	return nil
}

func createInstallerHandoffEvent(name string) (windows.Handle, error) {
	namePtr, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return 0, err
	}
	handle, err := windows.CreateEvent(nil, 1, 0, namePtr)
	if err != nil {
		if handle != 0 {
			windows.CloseHandle(handle)
		}
		return 0, err
	}
	return handle, nil
}

func buildInstallerLaunchArguments(executable string, pid int, nonce string) (string, error) {
	if !validInstallerPath(executable) || pid <= 0 || !validHandoffNonce(nonce) {
		return "", errors.New("updater: invalid application executable path, parent PID, or handoff nonce")
	}
	dir := filepath.Dir(executable)
	if filepath.Dir(dir) == dir {
		return "", errors.New("updater: refusing to install into a filesystem root")
	}
	info, err := os.Stat(executable)
	if err != nil {
		return "", fmt.Errorf("updater: inspect application executable: %w", err)
	}
	if !info.Mode().IsRegular() {
		return "", errors.New("updater: application executable is not a file")
	}
	uninstaller := filepath.Join(dir, "uninstall.exe")
	info, err = os.Lstat(uninstaller)
	if err != nil {
		return "", fmt.Errorf("updater: installed application uninstaller is missing: %w", err)
	}
	if !info.Mode().IsRegular() {
		return "", errors.New("updater: installed application uninstaller is not a regular file")
	}
	uninstallerPath, err := windows.UTF16PtrFromString(uninstaller)
	if err != nil {
		return "", fmt.Errorf("updater: invalid uninstaller path: %w", err)
	}
	attributes, err := windows.GetFileAttributes(uninstallerPath)
	if err != nil {
		return "", fmt.Errorf("updater: inspect application uninstaller: %w", err)
	}
	if attributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		return "", errors.New("updater: application uninstaller is a reparse point")
	}
	return fmt.Sprintf("/PAPERSAGE_PARENT_PID=%d /PAPERSAGE_HANDOFF=%s /D=%s", pid, nonce, dir), nil
}

func validHandoffNonce(nonce string) bool {
	if len(nonce) != 32 {
		return false
	}
	for _, c := range nonce {
		if !(c >= '0' && c <= '9') && !(c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

func validInstallerPath(path string) bool {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path || strings.HasPrefix(path, `\\?\`) || strings.HasPrefix(path, `\\.\`) {
		return false
	}
	rest := strings.TrimPrefix(path[len(filepath.VolumeName(path)):], `\`)
	if !filepath.IsLocal(rest) || strings.ContainsAny(rest, `<>"|?*`) {
		return false
	}
	for _, part := range strings.Split(rest, `\`) {
		if strings.HasSuffix(part, ".") || strings.HasSuffix(part, " ") {
			return false
		}
		for _, r := range part {
			if r < 32 {
				return false
			}
		}
	}
	return true
}
