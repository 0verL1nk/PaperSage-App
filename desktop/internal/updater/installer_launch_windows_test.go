package updater

import (
	"crypto/sha256"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"golang.org/x/sys/windows"
)

type launchFixture struct {
	installer  string
	executable string
	digest     []byte
}

func newLaunchFixture(t *testing.T) launchFixture {
	t.Helper()
	root := t.TempDir()
	if !strings.EqualFold(filepath.VolumeName(root), "E:") {
		t.Fatalf("temporary directory is not E:-rooted: %q", root)
	}
	stageDir := filepath.Join(root, "stage with spaces")
	installDir := filepath.Join(root, "Paper Sage with spaces")
	for _, dir := range []string{stageDir, installDir} {
		if err := os.Mkdir(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	installer := filepath.Join(stageDir, installerAssetName)
	executable := filepath.Join(installDir, "Paper Sage.exe")
	content := []byte("dummy installer; never execute")
	for path, bytes := range map[string][]byte{
		installer:  content,
		executable: []byte("dummy app"),
		filepath.Join(installDir, "uninstall.exe"): []byte("dummy uninstaller"),
	} {
		if err := os.WriteFile(path, bytes, 0600); err != nil {
			t.Fatal(err)
		}
	}
	digest := sha256.Sum256(content)
	return launchFixture{installer: installer, executable: executable, digest: digest[:]}
}

func openTestHandoff(t *testing.T, args, installDir string) (windows.Handle, windows.Handle) {
	t.Helper()
	prefix := "/PAPERSAGE_PARENT_PID=" + strconv.Itoa(os.Getpid()) + " /PAPERSAGE_HANDOFF="
	suffix := " /D=" + installDir
	if !strings.HasPrefix(args, prefix) || !strings.HasSuffix(args, suffix) {
		t.Fatalf("launcher arguments = %q; want %q<nonce>%q", args, prefix, suffix)
	}
	nonce := strings.TrimSuffix(strings.TrimPrefix(args, prefix), suffix)
	if !regexp.MustCompile(`^[0-9a-f]{32}$`).MatchString(nonce) {
		t.Fatalf("handoff nonce = %q, want 32 lowercase hexadecimal digits", nonce)
	}
	base := `Local\PaperSageUpdate-` + strconv.Itoa(os.Getpid()) + "-" + nonce
	open := func(name string, access uint32) windows.Handle {
		ptr, err := windows.UTF16PtrFromString(name)
		if err != nil {
			t.Fatal(err)
		}
		handle, err := windows.OpenEvent(access, false, ptr)
		if err != nil {
			t.Fatalf("open event %q before launch: %v", name, err)
		}
		t.Cleanup(func() { windows.CloseHandle(handle) })
		return handle
	}
	return open(base+"-ready", windows.EVENT_MODIFY_STATE), open(base+"-commit", windows.SYNCHRONIZE)
}

func assertEventNotSignaled(t *testing.T, event windows.Handle) {
	t.Helper()
	result, err := windows.WaitForSingleObject(event, 0)
	if err != nil || result != uint32(windows.WAIT_TIMEOUT) {
		t.Fatalf("event state = %v, %v; want not signaled", result, err)
	}
}

func TestInstallerLaunchArgumentsPreserveSpacesAndPutDirectoryLast(t *testing.T) {
	fixture := newLaunchFixture(t)
	nonce := "0123456789abcdef0123456789abcdef"
	got, err := buildInstallerLaunchArguments(fixture.executable, 12345, nonce)
	if err != nil {
		t.Fatal(err)
	}
	want := "/PAPERSAGE_PARENT_PID=12345 /PAPERSAGE_HANDOFF=" + nonce + " /D=" + filepath.Dir(fixture.executable)
	if got != want {
		t.Fatalf("arguments = %q, want %q", got, want)
	}
}

func TestLaunchVerifiedInstallerRejectsInvalidInputsBeforeLaunch(t *testing.T) {
	tests := []struct {
		name   string
		change func(*testing.T, *launchFixture)
	}{
		{"empty checksum", func(_ *testing.T, f *launchFixture) { f.digest = nil }},
		{"short checksum", func(_ *testing.T, f *launchFixture) { f.digest = f.digest[:31] }},
		{"long checksum", func(_ *testing.T, f *launchFixture) { f.digest = append(f.digest, 0) }},
		{"mismatched checksum", func(_ *testing.T, f *launchFixture) { f.digest[0] ^= 0xff }},
		{"changed staged bytes", func(t *testing.T, f *launchFixture) {
			if err := os.WriteFile(f.installer, []byte("changed after staging"), 0600); err != nil {
				t.Fatal(err)
			}
		}},
		{"relative staged path", func(t *testing.T, f *launchFixture) {
			cwd, err := os.Getwd()
			if err != nil {
				t.Fatal(err)
			}
			f.installer, err = filepath.Rel(cwd, f.installer)
			if err != nil {
				t.Fatal(err)
			}
		}},
		{"drive-relative staged path", func(_ *testing.T, f *launchFixture) { f.installer = `E:relative\` + installerAssetName }},
		{"wrong staged filename", func(t *testing.T, f *launchFixture) {
			wrong := filepath.Join(filepath.Dir(f.installer), "other.exe")
			if err := os.Rename(f.installer, wrong); err != nil {
				t.Fatal(err)
			}
			f.installer = wrong
		}},
		{"missing staged file", func(t *testing.T, f *launchFixture) {
			if err := os.Remove(f.installer); err != nil {
				t.Fatal(err)
			}
		}},
		{"staged path is a directory", func(t *testing.T, f *launchFixture) {
			if err := os.Remove(f.installer); err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(f.installer, 0700); err != nil {
				t.Fatal(err)
			}
		}},
		{"relative executable", func(_ *testing.T, f *launchFixture) { f.executable = `relative\PaperSage.exe` }},
		{"drive-relative executable", func(_ *testing.T, f *launchFixture) { f.executable = `E:PaperSage.exe` }},
		{"executable contains NUL", func(_ *testing.T, f *launchFixture) { f.executable += "\x00.exe" }},
		{"executable contains quote", func(_ *testing.T, f *launchFixture) { f.executable += `"` }},
		{"filesystem root install", func(_ *testing.T, f *launchFixture) { f.executable = `E:\PaperSage.exe` }},
		{"missing executable", func(_ *testing.T, f *launchFixture) { f.executable += ".missing" }},
		{"missing managed uninstaller", func(t *testing.T, f *launchFixture) {
			if err := os.Remove(filepath.Join(filepath.Dir(f.executable), "uninstall.exe")); err != nil {
				t.Fatal(err)
			}
		}},
		{"uninstaller is directory", func(t *testing.T, f *launchFixture) {
			path := filepath.Join(filepath.Dir(f.executable), "uninstall.exe")
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(path, 0700); err != nil {
				t.Fatal(err)
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixture := newLaunchFixture(t)
			tt.change(t, &fixture)
			called := false
			err := launchVerifiedInstallerForExecutable(fixture.installer, fixture.digest, fixture.executable, func(_, _ string) error {
				called = true
				return nil
			}, 0)
			if err == nil || called || errors.Is(err, ErrInstallerHandoffPending) {
				t.Fatalf("launch returned %v; called OS launcher = %t, want non-pending error before launch", err, called)
			}
		})
	}
}

func TestLaunchVerifiedInstallerRejectsReparsePoint(t *testing.T) {
	fixture := newLaunchFixture(t)
	real := filepath.Join(filepath.Dir(fixture.installer), "real.exe")
	if err := os.Rename(fixture.installer, real); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(real, fixture.installer); err != nil {
		t.Skipf("creating symlinks is unavailable: %v", err)
	}
	called := false
	err := launchVerifiedInstallerForExecutable(fixture.installer, fixture.digest, fixture.executable, func(_, _ string) error {
		called = true
		return nil
	}, 0)
	if err == nil || called {
		t.Fatalf("launch returned %v; called OS launcher = %t, want reparse rejection", err, called)
	}
}

func TestLaunchVerifiedInstallerRejectsReparseUninstaller(t *testing.T) {
	fixture := newLaunchFixture(t)
	uninstaller := filepath.Join(filepath.Dir(fixture.executable), "uninstall.exe")
	real := filepath.Join(filepath.Dir(fixture.executable), "real-uninstall.exe")
	if err := os.Rename(uninstaller, real); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(real, uninstaller); err != nil {
		t.Skipf("creating symlinks is unavailable: %v", err)
	}
	called := false
	err := launchVerifiedInstallerForExecutable(fixture.installer, fixture.digest, fixture.executable, func(_, _ string) error {
		called = true
		return nil
	}, 0)
	if err == nil || called {
		t.Fatalf("launch returned %v; called OS launcher = %t, want reparse uninstaller rejection", err, called)
	}
}

func TestLaunchVerifiedInstallerKeepsStagedFileLockedUntilLauncherReturns(t *testing.T) {
	fixture := newLaunchFixture(t)
	called := false
	var commit windows.Handle
	err := launchVerifiedInstallerForExecutable(fixture.installer, fixture.digest, fixture.executable, func(path, args string) error {
		called = true
		if path != fixture.installer {
			t.Errorf("launcher path = %q, want %q", path, fixture.installer)
		}
		ready, openedCommit := openTestHandoff(t, args, filepath.Dir(fixture.executable))
		commit = openedCommit
		assertEventNotSignaled(t, commit)
		if err := os.Rename(path, path+".replaced"); err == nil {
			t.Error("installer could be renamed while launcher was running")
		}
		if err := os.WriteFile(path, []byte("replace"), 0600); err == nil {
			t.Error("installer could be overwritten while launcher was running")
		}
		return windows.SetEvent(ready)
	}, 1000)
	if err != nil || !called {
		t.Fatalf("launch returned %v; launcher called = %t", err, called)
	}
	result, waitErr := windows.WaitForSingleObject(commit, 0)
	if waitErr != nil || result != windows.WAIT_OBJECT_0 {
		t.Fatalf("commit after ready = %v, %v; want signaled", result, waitErr)
	}
	if err := os.Rename(fixture.installer, fixture.installer+".closed"); err != nil {
		t.Fatalf("installer handle remained open after launch: %v", err)
	}
}

func TestLaunchVerifiedInstallerWaitsForReadyBeforeCommitting(t *testing.T) {
	fixture := newLaunchFixture(t)
	type observation struct {
		result uint32
		err    error
	}
	observed := make(chan observation, 1)
	var commit windows.Handle
	err := launchVerifiedInstallerForExecutable(fixture.installer, fixture.digest, fixture.executable, func(_, args string) error {
		ready, openedCommit := openTestHandoff(t, args, filepath.Dir(fixture.executable))
		commit = openedCommit
		go func() {
			result, err := windows.WaitForSingleObject(commit, 100)
			if err == nil && result == uint32(windows.WAIT_TIMEOUT) {
				err = windows.SetEvent(ready)
			}
			observed <- observation{result, err}
		}()
		return nil
	}, 2000)
	got := <-observed
	if got.err != nil || got.result != uint32(windows.WAIT_TIMEOUT) {
		t.Fatalf("commit before ready = %v, %v; want unsignaled", got.result, got.err)
	}
	if err != nil {
		t.Fatalf("launch after signaling ready: %v", err)
	}
	result, waitErr := windows.WaitForSingleObject(commit, 0)
	if waitErr != nil || result != windows.WAIT_OBJECT_0 {
		t.Fatalf("commit after ready = %v, %v; want signaled", result, waitErr)
	}
}

func TestLaunchVerifiedInstallerReportsUACDenialAndClosesHandle(t *testing.T) {
	fixture := newLaunchFixture(t)
	var commit windows.Handle
	err := launchVerifiedInstallerForExecutable(fixture.installer, fixture.digest, fixture.executable, func(_, args string) error {
		ready, openedCommit := openTestHandoff(t, args, filepath.Dir(fixture.executable))
		commit = openedCommit
		if err := windows.SetEvent(ready); err != nil {
			return err
		}
		return windows.ERROR_CANCELLED
	}, 0)
	if !errors.Is(err, windows.ERROR_CANCELLED) || errors.Is(err, ErrInstallerHandoffPending) || !strings.Contains(strings.ToLower(err.Error()), "cancel") {
		t.Fatalf("launch error = %v, want non-pending wrapped UAC cancellation", err)
	}
	assertEventNotSignaled(t, commit)
	if err := os.Rename(fixture.installer, fixture.installer+".closed"); err != nil {
		t.Fatalf("installer handle remained open after cancellation: %v", err)
	}
}

func TestLaunchVerifiedInstallerReportsShellFailureWithoutCommitting(t *testing.T) {
	fixture := newLaunchFixture(t)
	var commit windows.Handle
	err := launchVerifiedInstallerForExecutable(fixture.installer, fixture.digest, fixture.executable, func(_, args string) error {
		ready, openedCommit := openTestHandoff(t, args, filepath.Dir(fixture.executable))
		commit = openedCommit
		if err := windows.SetEvent(ready); err != nil {
			return err
		}
		return windows.ERROR_ACCESS_DENIED
	}, 0)
	if !errors.Is(err, windows.ERROR_ACCESS_DENIED) || errors.Is(err, ErrInstallerHandoffPending) || !strings.Contains(err.Error(), "launch installer") {
		t.Fatalf("launch error = %v, want non-pending wrapped shell failure", err)
	}
	assertEventNotSignaled(t, commit)
}

func TestLaunchVerifiedInstallerTimesOutWithoutCommitting(t *testing.T) {
	fixture := newLaunchFixture(t)
	var commit windows.Handle
	err := launchVerifiedInstallerForExecutable(fixture.installer, fixture.digest, fixture.executable, func(_, args string) error {
		_, commit = openTestHandoff(t, args, filepath.Dir(fixture.executable))
		return nil
	}, 0)
	if err == nil || !errors.Is(err, ErrInstallerHandoffPending) || !strings.Contains(strings.ToLower(err.Error()), "timed out") {
		t.Fatalf("launch error = %v, want pending ready timeout", err)
	}
	assertEventNotSignaled(t, commit)
}

func TestLaunchVerifiedInstallerCommitFailureIsPendingWithoutCommitting(t *testing.T) {
	fixture := newLaunchFixture(t)
	var commit windows.Handle
	err := launchVerifiedInstallerWithEvents(fixture.installer, fixture.digest, fixture.executable, func(_, args string) error {
		ready, openedCommit := openTestHandoff(t, args, filepath.Dir(fixture.executable))
		commit = openedCommit
		return windows.SetEvent(ready)
	}, 0, func(windows.Handle) error { return windows.ERROR_ACCESS_DENIED }, windows.CloseHandle)
	if !errors.Is(err, ErrInstallerHandoffPending) || !errors.Is(err, windows.ERROR_ACCESS_DENIED) {
		t.Fatalf("commit error = %v, want pending error wrapping access denied", err)
	}
	assertEventNotSignaled(t, commit)
}

func TestLaunchVerifiedInstallerIgnoresCleanupFailureAfterCommit(t *testing.T) {
	fixture := newLaunchFixture(t)
	var commit windows.Handle
	closeCalls := 0
	err := launchVerifiedInstallerWithEvents(fixture.installer, fixture.digest, fixture.executable, func(_, args string) error {
		ready, openedCommit := openTestHandoff(t, args, filepath.Dir(fixture.executable))
		commit = openedCommit
		return windows.SetEvent(ready)
	}, 0, windows.SetEvent, func(handle windows.Handle) error {
		closeCalls++
		if err := windows.CloseHandle(handle); err != nil {
			return err
		}
		return windows.ERROR_ACCESS_DENIED
	})
	if err != nil || closeCalls != 2 {
		t.Fatalf("launch after commit = %v, cleanup calls = %d; want success and two cleanup attempts", err, closeCalls)
	}
	result, waitErr := windows.WaitForSingleObject(commit, 0)
	if waitErr != nil || result != windows.WAIT_OBJECT_0 {
		t.Fatalf("commit after cleanup = %v, %v; want signaled", result, waitErr)
	}
}

func TestCreateInstallerHandoffEventRejectsExistingName(t *testing.T) {
	name := `Local\PaperSageTest-` + strconv.Itoa(os.Getpid()) + "-" + t.Name()
	first, err := createInstallerHandoffEvent(name)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { windows.CloseHandle(first) })
	second, err := createInstallerHandoffEvent(name)
	if second != 0 {
		windows.CloseHandle(second)
		t.Fatalf("existing event returned handle %v", second)
	}
	if !errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
		t.Fatalf("existing event error = %v, want ERROR_ALREADY_EXISTS", err)
	}
}

func TestInstallerLaunchArgumentsRejectFilesystemRoot(t *testing.T) {
	_, err := buildInstallerLaunchArguments(`E:\PaperSage.exe`, 12345, "0123456789abcdef0123456789abcdef")
	if err == nil || !strings.Contains(err.Error(), "filesystem root") {
		t.Fatalf("root executable error = %v, want explicit root rejection", err)
	}
}

func TestInstallerLaunchArgumentsRejectInvalidHandoffNonce(t *testing.T) {
	fixture := newLaunchFixture(t)
	for _, nonce := range []string{"", "short", strings.Repeat("A", 32), strings.Repeat("g", 32), strings.Repeat("0", 33)} {
		if args, err := buildInstallerLaunchArguments(fixture.executable, 12345, nonce); err == nil {
			t.Errorf("invalid nonce %q accepted as %q", nonce, args)
		}
	}
}
