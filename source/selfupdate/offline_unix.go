//go:build linux || darwin

package selfupdate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"syscall"
	"time"
)

func startUnixOfflineWorker(jobPath string) error {
	work := filepath.Dir(jobPath)
	cmd := exec.Command(filepath.Join(work, "file-helper"), "-offline-worker", jobPath)
	// The launcher kills the console's process group during shutdown. The
	// supervisor must outlive both that group and the original terminal session.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	log, err := os.OpenFile(filepath.Join(work, "worker.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	cmd.Stdout, cmd.Stderr = log, log
	if err := cmd.Start(); err != nil {
		log.Close()
		return err
	}
	log.Close()
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	timer := time.NewTimer(30 * time.Second)
	defer timer.Stop()
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case err := <-done:
			return fmt.Errorf("offline update supervisor exited: %v; inspect worker.log", err)
		case <-timer.C:
			cmd.Process.Kill()
			<-done
			return errors.New("offline update supervisor readiness timed out; service was retained")
		case <-tick.C:
			if _, err := os.Stat(filepath.Join(work, "ready")); err == nil {
				return nil
			}
		}
	}
}

func writeOfflineJSON(file string, value any) error {
	b, err := json.Marshal(value)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(file), ".update-state-")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	_, writeErr := tmp.Write(b)
	syncErr := tmp.Sync()
	closeErr := tmp.Close()
	if err := errors.Join(writeErr, syncErr, closeErr); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), file)
}

func setUnixUpdateState(job offlineJob, phase, message string, failure error) error {
	state := OfflineState{Phase: phase, Percent: -1, Message: message, Version: job.To,
		Job: job.Work, Backup: filepath.Join(job.Work, "backup"), UpdatedAt: time.Now().UTC().Format(time.RFC3339)}
	if failure != nil {
		state.Error = failure.Error()
	}
	return writeOfflineJSON(filepath.Join(job.Root, ".yunkun-local", "update-state.json"), state)
}

func awaitOriginalShutdown(job offlineJob) error {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	lock, err := os.OpenFile(filepath.Join(job.Root, ".yunkun-local", "launcher.lock"), os.O_RDWR|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return err
	}
	defer lock.Close()
	for {
		if !ManagedOfflineProcess(job.ParentPID, filepath.Join(job.Root, "bin", "yunkun")) {
			err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
			if err == nil {
				defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
				for _, name := range []string{"launcher.json", "postgres-data/postmaster.pid"} {
					if _, err := os.Lstat(filepath.Join(job.Root, ".yunkun-local", name)); !errors.Is(err, os.ErrNotExist) {
						return errors.New("launcher or database shutdown is incomplete; no files were replaced")
					}
				}
				return nil
			}
			if err != syscall.EWOULDBLOCK && err != syscall.EAGAIN {
				return err
			}
		}
		select {
		case <-ctx.Done():
			return errors.New("original installation did not finish shutdown")
		case <-time.After(200 * time.Millisecond):
		}
	}
}

type unixUpdateInstance struct {
	cmd  *exec.Cmd
	done chan error
	job  offlineJob
}

func launchUnixUpdate(job offlineJob, probe bool) (*unixUpdateInstance, error) {
	cmd := exec.Command(filepath.Join(job.Root, "bin", "yunkun-offline"),
		"--no-browser", "--port", strconv.Itoa(job.ConsolePort), "--database-port", strconv.Itoa(job.DatabasePort))
	cmd.Dir = job.Root
	// Only the trusted supervisor can pass this job through launcher maintenance.
	cmd.Env = append(os.Environ(), "YUNKUN_UPDATE_JOB="+job.Work, "YUNKUN_UPDATE_PROBE="+strconv.FormatBool(probe))
	if probe {
		cmd.Env = append(cmd.Env, "YUNKUN_UPDATE_PROBE=1")
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	log, err := os.OpenFile(filepath.Join(job.Work, "startup.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return nil, err
	}
	cmd.Stdout, cmd.Stderr = log, log
	if err := cmd.Start(); err != nil {
		log.Close()
		return nil, err
	}
	log.Close()
	instance := &unixUpdateInstance{cmd: cmd, done: make(chan error, 1), job: job}
	go func() { instance.done <- cmd.Wait() }()
	return instance, nil
}

func (i *unixUpdateInstance) stop() error {
	select {
	case <-i.done:
	default:
		if !ManagedOfflineProcess(i.cmd.Process.Pid, filepath.Join(i.job.Root, "bin", "yunkun-offline")) {
			return errors.New("refusing to signal an unverified launcher")
		}
		if err := i.cmd.Process.Signal(syscall.SIGTERM); err != nil {
			return err
		}
		select {
		case <-i.done:
		case <-time.After(90 * time.Second):
			return errors.New("updated installation did not stop; manual recovery required")
		}
	}
	if _, err := os.Lstat(filepath.Join(i.job.Root, ".yunkun-local", "postgres-data", "postmaster.pid")); !errors.Is(err, os.ErrNotExist) {
		return errors.New("database shutdown could not be confirmed")
	}
	return nil
}

func (i *unixUpdateInstance) healthy(version string, probe bool) error {
	client := &http.Client{Timeout: 2 * time.Second, Transport: &http.Transport{Proxy: nil}}
	defer client.CloseIdleConnections()
	deadline := time.NewTimer(110 * time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(500 * time.Millisecond)
	defer tick.Stop()
	consecutive := 0
	for {
		select {
		case err := <-i.done:
			i.done <- err
			return fmt.Errorf("updated launcher exited before readiness: %v", err)
		case <-deadline.C:
			return errors.New("updated installation readiness timed out")
		case <-tick.C:
			base := "http://127.0.0.1:" + strconv.Itoa(i.job.ConsolePort)
			var health struct {
				Ready   bool   `json:"ready"`
				Service string `json:"service"`
				Version string `json:"version"`
				Probe   bool   `json:"update_probe"`
			}
			var auth struct {
				Initialized *bool `json:"initialized"`
			}
			if readUpdateEndpoint(client, base+"/api/health", &health) == nil &&
				health.Ready && health.Service == "yunkun" && health.Version == version && health.Probe == probe &&
				readUpdateEndpoint(client, base+"/api/auth/status", &auth) == nil && auth.Initialized != nil {
				consecutive++
				if consecutive >= 3 {
					return nil
				}
			} else {
				consecutive = 0
			}
		}
	}
}

func readUpdateEndpoint(client *http.Client, url string, value any) error {
	res, err := client.Get(url)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return errors.New("update readiness HTTP error")
	}
	return json.NewDecoder(res.Body).Decode(value)
}

// RunOfflineWorker is executed by a copy of the current, trusted console. It
// never reads a command or URL from the candidate and never copies a live DB.
func RunOfflineWorker(jobPath string) (result error) {
	var job offlineJob
	if err := readJSON(jobPath, &job); err != nil {
		return err
	}
	if job.GOOS != runtime.GOOS || job.GOARCH != runtime.GOARCH ||
		job.Public || job.ConsolePort < 1024 || job.ConsolePort > 65535 ||
		job.DatabasePort < 1024 || job.DatabasePort > 65535 || job.DatabasePort == job.ConsolePort {
		return errors.New("invalid Unix managed update job")
	}
	if err := OfflineFiles(jobPath, "check"); err != nil {
		return err
	}
	helper, err := os.Executable()
	if err != nil || !samePath(helper, filepath.Join(job.Work, "file-helper")) {
		return errors.New("invalid update helper")
	}
	if !ManagedOfflineProcess(job.ParentPID, filepath.Join(job.Root, "bin", "yunkun")) {
		return errors.New("update parent is not the managed console")
	}
	var launcher struct {
		PID int `json:"pid"`
	}
	if readJSON(filepath.Join(job.Root, ".yunkun-local", "launcher.json"), &launcher) != nil ||
		!ManagedOfflineProcess(launcher.PID, filepath.Join(job.Root, "bin", "yunkun-offline")) {
		return errors.New("managed launcher is unavailable")
	}
	lock, err := os.OpenFile(filepath.Join(job.Root, ".yunkun-local", "updates", "update.lock"), os.O_CREATE|os.O_RDWR|syscall.O_NOFOLLOW, 0600)
	if err != nil {
		return err
	}
	defer lock.Close()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		return errors.New("another offline update is running")
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	defer func() {
		if result != nil && OfflineStatus(job.Root).Phase == "waiting" &&
			ManagedOfflineProcess(job.ParentPID, filepath.Join(job.Root, "bin", "yunkun")) {
			_ = setUnixUpdateState(job, "failed", "Update preparation failed; the current service was retained.", result)
		}
	}()
	var size uint64
	err = walkRegular(job.Root, func(p string) bool { return samePath(p, filepath.Join(job.Root, ".yunkun-local", "updates")) },
		func(_ string, _ string, info fs.FileInfo) error { size += uint64(info.Size()); return nil })
	if err != nil {
		return err
	}
	var disk syscall.Statfs_t
	if err := syscall.Statfs(job.Root, &disk); err != nil {
		return err
	}
	if disk.Bavail*uint64(disk.Bsize) < size*2+(1<<30) {
		return errors.New("insufficient space for backup and rollback")
	}
	if err := setUnixUpdateState(job, "waiting", "Waiting for the managed console and database to stop.", nil); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(job.Work, "ready"), []byte("ready"), 0600); err != nil {
		return err
	}
	if err := awaitOriginalShutdown(job); err != nil {
		_ = setUnixUpdateState(job, "recovery_required", "Shutdown was not confirmed; preserve this installation and inspect the logs.", err)
		return err
	}
	return (offlineTransaction{
		job: job,
		state: func(phase, message string, failure error) error {
			return setUnixUpdateState(job, phase, message, failure)
		},
		launch: func(probe bool) (offlineInstance, error) { return launchUnixUpdate(job, probe) },
		files:  func(operation string) error { return OfflineFiles(jobPath, operation) },
	}).run()
}
