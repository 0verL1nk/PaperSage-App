//go:build !windows

package backend

import "os/exec"

func configureChildProcess(_ *exec.Cmd) {}
