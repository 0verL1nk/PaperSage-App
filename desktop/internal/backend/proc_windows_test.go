//go:build windows

package backend

import (
	"os/exec"
	"testing"
)

func TestConfigureChildProcessHidesConsole(t *testing.T) {
	cmd := exec.Command("cmd")
	configureChildProcess(cmd)
	if cmd.SysProcAttr == nil || !cmd.SysProcAttr.HideWindow {
		t.Fatal("backend child process is not hidden")
	}
	if cmd.SysProcAttr.CreationFlags&0x08000000 == 0 {
		t.Fatal("backend child process is missing CREATE_NO_WINDOW")
	}
}
