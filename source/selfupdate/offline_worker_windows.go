//go:build windows

package selfupdate

import _ "embed"

//go:embed offline-worker.ps1
var windowsWorkerScript []byte

func windowsOfflineWorker() ([]byte, error) { return windowsWorkerScript, nil }
