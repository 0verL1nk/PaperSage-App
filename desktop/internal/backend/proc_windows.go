//go:build windows

package backend

import (
	"os/exec"
	"syscall"
)

// The bundled backend is a console program; without this flag Windows creates a
// console window when the GUI app starts it.
func configureChildProcess(cmd *exec.Cmd) {
	const createNoWindow = 0x08000000
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
}
