//go:build !windows

package selfupdate

import "os/exec"

func hideProcess(cmd *exec.Cmd) {}
