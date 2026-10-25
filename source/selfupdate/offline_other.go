//go:build !windows

package selfupdate

import "errors"

func windowsOfflineWorker() ([]byte, error) {
	return nil, errors.New("Windows offline update worker is unavailable on this platform")
}
