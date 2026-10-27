package selfupdate

import (
	"fmt"
	"os"
	"syscall"
)

// ManagedOfflineProcess verifies an executable and its owner before signaling it.
func ManagedOfflineProcess(pid int, executable string) bool {
	if pid <= 1 {
		return false
	}
	actual, err := os.Readlink(fmt.Sprintf("/proc/%d/exe", pid))
	if err != nil || !samePath(actual, executable) {
		return false
	}
	info, err := os.Stat(fmt.Sprintf("/proc/%d", pid))
	return err == nil && info.Sys().(*syscall.Stat_t).Uid == uint32(os.Geteuid())
}
