//go:build !linux

package selfupdate

import "errors"

func linuxManagedProcess(int, string) bool { return false }
func startLinuxOfflineWorker(string) error { return errors.New("Linux update worker requires Linux") }
func RunOfflineWorker(string) error        { return errors.New("Linux update worker requires Linux") }
