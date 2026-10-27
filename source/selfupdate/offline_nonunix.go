//go:build !linux && !darwin

package selfupdate

import "errors"

func ManagedOfflineProcess(int, string) bool { return false }
func startUnixOfflineWorker(string) error {
	return errors.New("Unix update worker requires Linux or macOS")
}
func RunOfflineWorker(string) error { return errors.New("Unix update worker requires Linux or macOS") }
