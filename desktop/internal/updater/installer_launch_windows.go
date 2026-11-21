//go:build windows

package updater

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	updateHelperFlag = "--apply-update"
	updateHelperName = "papersage-update-helper.exe"

	parentExitWait = 30 * time.Second
	installerWait  = 30 * time.Minute

	seeMaskNoCloseProcess = 0x00000040 // keep the installer process handle
	seeMaskNoAsync        = 0x00000100 // complete before ShellExecuteEx returns
	seeMaskNoUI           = 0x00000400 // the helper reports failures instead
)

var (
	procShellExecuteExW = syscall.NewLazyDLL("shell32.dll").NewProc("ShellExecuteExW")
	procMessageBoxW     = syscall.NewLazyDLL("user32.dll").NewProc("MessageBoxW")
)

// showHelperError is the only visible report when the detached helper cannot
// finish: the app is already restarting, so the failure would be silent.
func showHelperError(cause error) {
	const (
		mbIconError     = 0x00000010
		mbSetForeground = 0x00010000
	)
	title, err := windows.UTF16PtrFromString("PaperSage 更新")
	if err != nil {
		return
	}
	message := cause.Error()
	if errors.Is(cause, windows.ERROR_CANCELLED) {
		message = "已取消管理员授权，未安装更新。"
	}
	text, err := windows.UTF16PtrFromString("更新未完成：" + message + "\n\n如果应用没有自动重新打开，请手动启动 PaperSage。")
	if err != nil {
		return
	}
	_, _, _ = procMessageBoxW.Call(0,
		uintptr(unsafe.Pointer(text)), uintptr(unsafe.Pointer(title)),
		mbIconError|mbSetForeground)
}

// RunUpdateHelper takes over when the process was started as the detached
// update helper; main calls it before starting the application.
func RunUpdateHelper(args []string) (bool, error) {
	if len(args) == 0 || args[0] != updateHelperFlag {
		return false, nil
	}
	if len(args) != 4 {
		return true, errors.New("updater: invalid update helper arguments")
	}
	err := applyUpdate(args[1], args[2], args[3], updateHelperDeps{})
	if err != nil {
		if validInstallerPath(args[1]) {
			writeHelperError(filepath.Dir(args[1]), err)
		}
		showHelperError(err)
	}
	return true, err
}

// spawnUpdateHelper copies the running executable to the staging directory and
// starts it detached, so the installation can replace the installed files while
// the helper keeps running. The copy is pinned until CreateProcess has mapped
// it, so nothing can substitute it between the copy and its first instruction.
func spawnUpdateHelper(stagedPath string, digest []byte) error {
	if len(digest) != sha256.Size {
		return errors.New("updater: installer requires a 32-byte SHA-256 checksum")
	}
	if !validInstallerPath(stagedPath) || filepath.Base(stagedPath) != installerAssetName {
		return errors.New("updater: invalid staged installer path")
	}
	self, err := os.Executable()
	if err != nil {
		return fmt.Errorf("updater: determine current executable: %w", err)
	}
	if _, err := validateInstalledApp(self); err != nil {
		return err
	}
	helperPath := filepath.Join(filepath.Dir(stagedPath), updateHelperName)
	if err := copyFile(self, helperPath); err != nil {
		return fmt.Errorf("updater: stage update helper: %w", err)
	}
	// Rewriting an existing helper does not refresh the directory's mtime;
	// without this a late retry could look stale to cleanStaleStaging.
	now := time.Now()
	_ = os.Chtimes(filepath.Dir(stagedPath), now, now)
	// The helper copy must be our own bytes, not something substituted before
	// CreateProcess maps it.
	selfDigest, err := digestFile(self)
	if err != nil {
		return fmt.Errorf("updater: hash application executable: %w", err)
	}
	pin, err := openVerifiedInstaller(helperPath, selfDigest)
	if err != nil {
		return fmt.Errorf("updater: pin update helper: %w", err)
	}
	args := []string{updateHelperFlag, stagedPath, hex.EncodeToString(digest), self}
	err = startDetached(helperPath, args)
	// The image is mapped by the time CreateProcess returns; closing now is safe.
	_ = windows.CloseHandle(pin)
	if err != nil {
		return fmt.Errorf("updater: start update helper: %w", err)
	}
	return nil
}

func startDetached(path string, args []string) error {
	const (
		detachedProcess        = 0x00000008
		createNewProcessGroup  = 0x00000200
		createBreakawayFromJob = 0x01000000
	)
	start := func(flags uint32) error {
		cmd := exec.Command(path, args...)
		cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: flags}
		if err := cmd.Start(); err != nil {
			return err
		}
		// The helper survives this process; the handle is no longer needed and
		// releasing it must not turn a successful launch into a retryable error.
		_ = cmd.Process.Release()
		return nil
	}
	err := start(detachedProcess | createNewProcessGroup | createBreakawayFromJob)
	if errors.Is(err, windows.ERROR_ACCESS_DENIED) {
		// A job that neither allows breakaway nor kills its processes on close.
		return start(detachedProcess | createNewProcessGroup)
	}
	return err
}

// updateHelperDeps keeps the Windows side effects injectable for tests.
type updateHelperDeps struct {
	waitParent     func()
	launchElevated func(path, parameters string) (windows.Handle, error)
	waitProcess    func(process windows.Handle) (uint32, error)
	launchApp      func(path string) error
	removeDir      func(path string) error
}

func (d updateHelperDeps) withDefaults() updateHelperDeps {
	if d.waitParent == nil {
		d.waitParent = waitForParentExit
	}
	if d.launchElevated == nil {
		d.launchElevated = shellExecuteElevated
	}
	if d.waitProcess == nil {
		d.waitProcess = waitInstallerExit
	}
	if d.launchApp == nil {
		d.launchApp = launchInstalledApp
	}
	if d.removeDir == nil {
		d.removeDir = os.RemoveAll
	}
	return d
}

func applyUpdate(stagedPath, digestHex, appExe string, deps updateHelperDeps) (err error) {
	deps = deps.withDefaults()
	// The app has already quit: every failure must try to bring it back.
	restart := true
	defer func() {
		if err != nil && restart {
			_ = deps.launchApp(appExe)
		}
	}()

	expected, err := hex.DecodeString(digestHex)
	if err != nil || len(expected) != sha256.Size {
		return errors.New("updater: invalid installer checksum")
	}
	if !validInstallerPath(stagedPath) || filepath.Base(stagedPath) != installerAssetName {
		return errors.New("updater: invalid staged installer path")
	}
	dir, err := validateInstalledApp(appExe)
	if err != nil {
		return err
	}
	deps.waitParent()

	handle, err := openVerifiedInstaller(stagedPath, expected)
	if err != nil {
		return err
	}
	// Denies write and delete: the path cannot be swapped while the elevated
	// installer runs. NSIS opens its own image with FILE_SHARE_READ, so this
	// handle does not block it.
	defer windows.CloseHandle(handle)

	process, err := deps.launchElevated(stagedPath, "/S /D="+dir)
	if err != nil {
		return fmt.Errorf("updater: launch installer: %w", err)
	}
	code, err := deps.waitProcess(process)
	_ = windows.CloseHandle(process)
	if err != nil {
		// The installer may still be running. Restarting now would race it on
		// the same files, so leave the app closed and report instead.
		restart = false
		return fmt.Errorf("updater: wait for installer: %w", err)
	}
	if code != 0 {
		return fmt.Errorf("updater: installer exited with code %d", code)
	}
	restart = false
	if err := deps.launchApp(appExe); err != nil {
		return fmt.Errorf("updater: restart application: %w", err)
	}
	deps.removeDir(filepath.Dir(stagedPath))
	return nil
}

// openVerifiedInstaller returns a handle that pins the file while its digest is
// compared; keep it open until the elevated installer exits.
func openVerifiedInstaller(path string, expected []byte) (windows.Handle, error) {
	handle, err := openPinnedRead(path)
	if err != nil {
		return 0, fmt.Errorf("updater: open staged installer: %w", err)
	}
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(handle, &info); err != nil {
		windows.CloseHandle(handle)
		return 0, fmt.Errorf("updater: inspect staged installer: %w", err)
	}
	kind, err := windows.GetFileType(handle)
	if err != nil {
		windows.CloseHandle(handle)
		return 0, fmt.Errorf("updater: inspect staged installer type: %w", err)
	}
	if kind != windows.FILE_TYPE_DISK || info.FileAttributes&(windows.FILE_ATTRIBUTE_DIRECTORY|windows.FILE_ATTRIBUTE_REPARSE_POINT) != 0 {
		windows.CloseHandle(handle)
		return 0, errors.New("updater: staged installer is not a regular non-reparse file")
	}
	digest := sha256.New()
	buffer := make([]byte, 64*1024)
	for {
		var read uint32
		if err := windows.ReadFile(handle, buffer, &read, nil); err != nil {
			windows.CloseHandle(handle)
			return 0, fmt.Errorf("updater: hash staged installer: %w", err)
		}
		if read == 0 {
			break
		}
		digest.Write(buffer[:read])
	}
	if subtle.ConstantTimeCompare(digest.Sum(nil), expected) != 1 {
		windows.CloseHandle(handle)
		return 0, errors.New("updater: staged installer checksum mismatch")
	}
	return handle, nil
}

func openPinnedRead(path string) (windows.Handle, error) {
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, err
	}
	return windows.CreateFile(name, windows.GENERIC_READ, windows.FILE_SHARE_READ, nil,
		windows.OPEN_EXISTING, windows.FILE_FLAG_OPEN_REPARSE_POINT|windows.FILE_FLAG_SEQUENTIAL_SCAN, 0)
}

func waitForParentExit() {
	pid := os.Getppid()
	if pid <= 0 {
		return
	}
	handle, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(pid))
	if err != nil {
		return
	}
	defer windows.CloseHandle(handle)
	_, _ = windows.WaitForSingleObject(handle, uint32(parentExitWait/time.Millisecond))
}

type shellExecuteInfo struct {
	size          uint32
	mask          uint32
	window        uintptr
	verb          *uint16
	file          *uint16
	parameters    *uint16
	directory     *uint16
	show          int32
	instance      uintptr
	idList        uintptr
	class         *uint16
	classKey      uintptr
	hotKey        uint32
	iconOrMonitor uintptr
	process       uintptr
}

func shellExecuteElevated(path, parameters string) (windows.Handle, error) {
	return shellExecuteWithVerb("runas", path, parameters)
}

func shellExecuteWithVerb(verbText, path, parameters string) (windows.Handle, error) {
	verb, err := windows.UTF16PtrFromString(verbText)
	if err != nil {
		return 0, err
	}
	file, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, err
	}
	params, err := windows.UTF16PtrFromString(parameters)
	if err != nil {
		return 0, err
	}
	// ShellExecuteEx resolves a NULL directory against the process working
	// directory, which can fail after the app exits; pin it to the installer.
	directory, err := windows.UTF16PtrFromString(filepath.Dir(path))
	if err != nil {
		return 0, err
	}
	var info shellExecuteInfo
	info.size = uint32(unsafe.Sizeof(info))
	info.mask = seeMaskNoCloseProcess | seeMaskNoAsync | seeMaskNoUI
	info.verb = verb
	info.file = file
	info.parameters = params
	info.directory = directory
	info.show = windows.SW_HIDE
	if ok, _, callErr := procShellExecuteExW.Call(uintptr(unsafe.Pointer(&info))); ok == 0 {
		if callErr != nil && callErr != syscall.Errno(0) {
			return 0, callErr
		}
		return 0, errors.New("updater: ShellExecuteEx failed")
	}
	runtime.KeepAlive(verb)
	runtime.KeepAlive(file)
	runtime.KeepAlive(params)
	runtime.KeepAlive(directory)
	if info.process == 0 {
		return 0, errors.New("updater: ShellExecuteEx returned no process")
	}
	return windows.Handle(info.process), nil
}

func waitInstallerExit(process windows.Handle) (uint32, error) {
	result, err := windows.WaitForSingleObject(process, uint32(installerWait/time.Millisecond))
	if err != nil {
		return 0, err
	}
	if result == uint32(windows.WAIT_TIMEOUT) {
		return 0, errors.New("timed out waiting for the installer")
	}
	var code uint32
	if err := windows.GetExitCodeProcess(process, &code); err != nil {
		return 0, err
	}
	return code, nil
}

func launchInstalledApp(path string) error {
	cmd := exec.Command(path)
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

func copyFile(source, destination string) error {
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, in)
	syncErr := out.Sync()
	closeErr := out.Close()
	if copyErr != nil {
		return copyErr
	}
	if syncErr != nil {
		return syncErr
	}
	return closeErr
}

func digestFile(path string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	digest := sha256.New()
	if _, err := io.Copy(digest, file); err != nil {
		return nil, err
	}
	return digest.Sum(nil), nil
}

func writeHelperError(dir string, cause error) {
	cwd, _ := os.Getwd()
	line := fmt.Sprintf("%s cwd=%q %v\n", time.Now().UTC().Format(time.RFC3339), cwd, cause)
	_ = os.WriteFile(filepath.Join(dir, "update-error.txt"), []byte(line), 0600)
}

// validateInstalledApp returns the installation directory of appExe. It is the
// only directory the helper will hand to the installer as /D.
func validateInstalledApp(appExe string) (string, error) {
	if !validInstallerPath(appExe) {
		return "", errors.New("updater: invalid application executable path")
	}
	dir := filepath.Dir(appExe)
	if filepath.Dir(dir) == dir {
		return "", errors.New("updater: refusing to install into a filesystem root")
	}
	info, err := os.Stat(appExe)
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
		return "", errors.New("updater: invalid uninstaller path")
	}
	attributes, err := windows.GetFileAttributes(uninstallerPath)
	if err != nil {
		return "", fmt.Errorf("updater: inspect application uninstaller: %w", err)
	}
	if attributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		return "", errors.New("updater: application uninstaller is a reparse point")
	}
	return dir, nil
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
