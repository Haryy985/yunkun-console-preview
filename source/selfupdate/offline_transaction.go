package selfupdate

import (
	"errors"
	"os"
	"path/filepath"
)

type offlineInstance interface {
	stop() error
	healthy(string, bool) error
}

type offlineTransaction struct {
	job    offlineJob
	state  func(string, string, error) error
	launch func(bool) (offlineInstance, error)
	files  func(string) error
}

// Once shutdown is confirmed, program and database move as one transaction.
// A failed stop prevents restore: copying a running database is never recovery.
func (t offlineTransaction) run() (result error) {
	changed, backedUp := false, false
	var current offlineInstance
	defer func() {
		if result == nil {
			return
		}
		failure := result
		recoverErr := func() error {
			if current != nil {
				if err := current.stop(); err != nil {
					return err
				}
			}
			if changed && backedUp {
				if err := t.state("rolling_back", "Restoring the matching program and database snapshot.", failure); err != nil {
					return err
				}
				if err := t.files("restore"); err != nil {
					return err
				}
			}
			if err := t.state("starting", "Restarting the previous installation.", failure); err != nil {
				return err
			}
			old, err := t.launch(false)
			if err != nil {
				return err
			}
			if err := old.healthy(t.job.From, false); err != nil {
				return errors.Join(err, old.stop())
			}
			phase := "failed"
			if changed {
				phase = "rolled_back"
			}
			return t.state(phase, "Previous installation is ready; data and backup were preserved.", failure)
		}()
		if recoverErr != nil {
			result = errors.Join(failure, recoverErr)
			_ = t.state("recovery_required", "Automatic recovery stopped. Preserve backup and failed installation.", result)
		}
	}()
	if err := t.state("backing_up", "Backing up the stopped database, keys, configuration and program.", nil); err != nil {
		return err
	}
	if err := t.files("backup"); err != nil {
		return err
	}
	backedUp = true
	if err := os.WriteFile(filepath.Join(t.job.Work, "backup-verified"), []byte("verified"), 0600); err != nil {
		return err
	}
	if err := t.state("installing", "Installing the verified offline package.", nil); err != nil {
		return err
	}
	changed = true
	if err := t.files("install"); err != nil {
		return err
	}
	if err := t.state("starting", "Checking migrations and authentication before accepting traffic.", nil); err != nil {
		return err
	}
	var err error
	current, err = t.launch(true)
	if err != nil {
		return err
	}
	if err := current.healthy(t.job.To, true); err != nil {
		return err
	}
	if err := current.stop(); err != nil {
		return err
	}
	current = nil
	current, err = t.launch(false)
	if err != nil {
		return err
	}
	if err := current.healthy(t.job.To, false); err != nil {
		return err
	}
	return t.state("completed", "Update completed. User data and a full backup were retained.", nil)
}
