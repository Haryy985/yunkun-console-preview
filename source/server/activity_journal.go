package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"yunkun/console/db"
)

type activityEnvelope struct {
	ExplorationID int64       `json:"exploration_id"`
	EventKey      string      `json:"event_key"`
	Detail        string      `json:"detail"`
	Activity      db.Activity `json:"activity"`
}

// A synced file precedes each database write. It lives inside the task's files,
// so task deletion removes pending payloads as well as committed records.
type activityJournal struct {
	mu   sync.Mutex
	root string
}

func (j *activityJournal) directory(taskID string) (string, error) {
	id, err := strconv.ParseInt(taskID, 10, 64)
	if err != nil || id <= 0 || strconv.FormatInt(id, 10) != taskID {
		return "", errors.New("invalid activity task ID")
	}
	return filepath.Join(j.root, "tasks", taskID, "activity-pending"), nil
}

func (j *activityJournal) enqueue(taskID string, expID int64, a db.Activity) (db.Activity, error) {
	dir, err := j.directory(taskID)
	if err != nil {
		return a, err
	}
	key := make([]byte, 16)
	if _, err := rand.Read(key); err != nil {
		return a, err
	}
	a.EventKey = hex.EncodeToString(key)
	if a.CreatedAt.IsZero() {
		a.CreatedAt = time.Now().UTC()
	}
	payload, err := json.Marshal(activityEnvelope{expID, a.EventKey, a.Detail, a})
	if err != nil {
		return a, err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return a, err
	}
	name := filepath.Join(dir, fmt.Sprintf("%020d-%s.json", time.Now().UnixNano(), a.EventKey))
	f, err := os.OpenFile(name+".tmp", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return a, err
	}
	_, writeErr := f.Write(payload)
	syncErr := f.Sync()
	closeErr := f.Close()
	if err := errors.Join(writeErr, syncErr, closeErr); err != nil {
		return a, err
	}
	if err := os.Rename(name+".tmp", name); err != nil {
		return a, err
	}
	return a, nil
}

// flush is serialized with enqueue. Stop at the first failure to preserve tool
// call/result ordering. Replaying a committed event returns its original DB ID.
func (j *activityJournal) flush(taskID string, expID int64, appendRow func(db.Activity) (int64, error), publish func(db.Activity)) (map[string]int64, error) {
	ids := map[string]int64{}
	dir, err := j.directory(taskID)
	if err != nil {
		return ids, err
	}
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return ids, nil
	}
	if err != nil {
		return ids, err
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := filepath.Join(dir, entry.Name())
		// A process may stop after Sync but before Rename. A complete temporary
		// envelope can be replayed too; malformed files remain for investigation.
		if filepath.Ext(name) != ".json" && filepath.Ext(name) != ".tmp" {
			continue
		}
		raw, err := os.ReadFile(name)
		if err != nil {
			return ids, err
		}
		var item activityEnvelope
		if err := json.Unmarshal(raw, &item); err != nil {
			return ids, fmt.Errorf("invalid activity journal %s: %w", entry.Name(), err)
		}
		if item.ExplorationID != expID || item.EventKey == "" {
			return ids, errors.New("activity journal exploration mismatch or missing event key")
		}
		row := item.Activity
		row.EventKey, row.Detail = item.EventKey, item.Detail
		id, err := appendRow(row)
		if err != nil {
			return ids, err
		}
		row.ID = id
		ids[row.EventKey] = id
		publish(row)
		if err := os.Remove(name); err != nil {
			return ids, err
		}
	}
	return ids, nil
}

func (e *Engine) journalActivity(t *Task, row db.Activity) db.Activity {
	j := e.journal
	j.mu.Lock()
	defer j.mu.Unlock()
	row, err := j.enqueue(t.ID, t.ExpID, row)
	if err != nil {
		e.Pause(t.ID, fmt.Errorf("activity persistence unavailable: %w", err))
		log.Printf("[activity] task %s paused: cannot preserve execution record: %v", t.ID, err)
		return row
	}
	ids, err := j.flush(t.ID, t.ExpID, t.Store.AppendActivity, func(a db.Activity) { e.bc.Publish(t.ID, a) })
	row.ID = ids[row.EventKey]
	if err != nil {
		log.Printf("[activity] task %s record retained locally for replay: %v", t.ID, err)
	}
	e.touch(t.ID)
	return row
}

func (e *Engine) flushTaskJournal(taskID string, expID int64) error {
	if e.journal == nil {
		return nil
	}
	e.journal.mu.Lock()
	defer e.journal.mu.Unlock()
	_, err := e.journal.flush(taskID, expID, e.m.pg.Exploration(expID).AppendActivity,
		func(a db.Activity) { e.bc.Publish(taskID, a) })
	return err
}

func (e *Engine) startActivityJournal(ctx context.Context) <-chan struct{} {
	e.journal = &activityJournal{root: e.m.dir}
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				for _, task := range e.m.List() {
					if ctx.Err() != nil {
						return
					}
					if err := e.reconcileIdleIntentStates(ctx, task); err != nil {
						log.Printf("[engine] task %s reconcile interrupted intents: %v", task.ID, err)
					}
				}
				entries, err := os.ReadDir(filepath.Join(e.m.dir, "tasks"))
				if err != nil && !os.IsNotExist(err) {
					log.Printf("[activity] scan journal: %v", err)
				}
				for _, entry := range entries {
					if ctx.Err() != nil {
						return
					}
					if !entry.IsDir() {
						continue
					}
					id, err := strconv.ParseInt(entry.Name(), 10, 64)
					if err != nil || id <= 0 || !e.beginTaskOperation(entry.Name()) {
						continue
					}
					func() {
						defer e.decInflight(entry.Name())
						queryCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
						defer cancel()
						var expID int64
						err := e.m.pg.QueryRowContext(queryCtx,
							"SELECT exploration_id FROM tasks WHERE id=$1 AND deleted_at IS NULL", id).Scan(&expID)
						if err != nil {
							return
						}
						if err := e.flushTaskJournal(entry.Name(), expID); err != nil {
							log.Printf("[activity] task %d pending replay: %v", id, err)
						}
					}()
				}
			}
		}
	}()
	return done
}

func (e *Engine) reconcileIdleIntentStates(ctx context.Context, task *Task) error {
	// Admission and the zero-operation check share a lock: a worker cannot claim
	// an intent between the ownership check and the database repair.
	e.deleteMu.Lock()
	defer e.deleteMu.Unlock()
	if e.shuttingDown || e.IsDeleting(task.ID) || e.inflightCount(task.ID) != 0 {
		return nil
	}
	queryCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	n, err := task.Store.RecoverUnownedRunningIntents(queryCtx)
	if n > 0 {
		log.Printf("[engine] task %s recovered %d unowned running intents as interrupted", task.ID, n)
	}
	return err
}
