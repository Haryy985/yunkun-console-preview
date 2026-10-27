package selfupdate

import (
	"bytes"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

// ManagedOfflineProcess uses the process's exec path, not argv[0] or a truncated
// ps command line. A reused PID or unverifiable owner must never be signaled.
func ManagedOfflineProcess(pid int, executable string) bool {
	if pid <= 1 {
		return false
	}
	before, err := unix.SysctlKinfoProc("kern.proc.pid", pid)
	if err != nil || before.Proc.P_pid != int32(pid) || before.Eproc.Ucred.Uid != uint32(os.Geteuid()) {
		return false
	}
	args, err := unix.SysctlRaw("kern.procargs2", pid)
	if err != nil {
		return false
	}
	defer clear(args)
	actual := darwinExecutablePath(args)
	resolved, err := filepath.EvalSymlinks(actual)
	if err != nil || !samePath(resolved, executable) {
		return false
	}
	after, err := unix.SysctlKinfoProc("kern.proc.pid", pid)
	return err == nil && after.Proc.P_pid == before.Proc.P_pid &&
		after.Proc.P_starttime == before.Proc.P_starttime && after.Eproc.Ucred.Uid == before.Eproc.Ucred.Uid
}

func darwinExecutablePath(args []byte) string {
	// KERN_PROCARGS2 starts with argc (int32), followed by a NUL-terminated
	// executable path, padding, argv and environment. Never log the latter.
	if len(args) < 6 {
		return ""
	}
	end := bytes.IndexByte(args[4:], 0)
	if end <= 0 {
		return ""
	}
	path := string(args[4 : 4+end])
	if !filepath.IsAbs(path) {
		return ""
	}
	return path
}
