package server

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"yunkun/console/db"
)

func TestActivityJournalOutageRestartRetainsDetailAndOrder(t *testing.T) {
	root := t.TempDir()
	j := &activityJournal{root: root}
	created := time.Now().UTC().Add(-time.Hour)
	tokens := 123
	for _, kind := range []string{"tool_use", "tool_result", "result"} {
		_, err := j.enqueue("42", 7, db.Activity{
			Kind: kind, Detail: "完整请求及返回\nPOST /test HTTP/1.1",
			ToolUseID: "call-1", CreatedAt: created, InputTokens: &tokens,
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	down := errors.New("database offline")
	_, err := j.flush("42", 7, func(db.Activity) (int64, error) { return 0, down },
		func(db.Activity) { t.Fatal("uncommitted activity must not advance SSE cursor") })
	if !errors.Is(err, down) {
		t.Fatal(err)
	}
	j = &activityJournal{root: root}
	var kinds []string
	var nextID int64
	_, err = j.flush("42", 7, func(a db.Activity) (int64, error) {
		if a.Detail != "完整请求及返回\nPOST /test HTTP/1.1" || !a.CreatedAt.Equal(created) ||
			a.EventKey == "" || a.InputTokens == nil || *a.InputTokens != tokens {
			t.Fatalf("payload changed: %+v", a)
		}
		nextID++
		return nextID, nil
	}, func(a db.Activity) {
		kinds = append(kinds, a.Kind)
		if a.ID != int64(len(kinds)) {
			t.Fatalf("SSE cursor order: %+v", a)
		}
	})
	if err != nil || !reflect.DeepEqual(kinds, []string{"tool_use", "tool_result", "result"}) {
		t.Fatalf("order=%v err=%v", kinds, err)
	}
	dir, _ := j.directory("42")
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Fatal("replayed journal not consumed")
	}
}

func TestActivityJournalCommitReplyLostReusesEventKey(t *testing.T) {
	j := &activityJournal{root: t.TempDir()}
	_, err := j.enqueue("42", 7, db.Activity{Kind: "result"})
	if err != nil {
		t.Fatal(err)
	}
	committed := map[string]int64{}
	first := true
	appendRow := func(a db.Activity) (int64, error) {
		if id := committed[a.EventKey]; id != 0 {
			return id, nil
		}
		committed[a.EventKey] = 101
		if first {
			first = false
			return 0, errors.New("connection lost after commit")
		}
		return 101, nil
	}
	_, _ = j.flush("42", 7, appendRow, func(db.Activity) { t.Fatal("published before acknowledgment") })
	var got int64
	_, err = j.flush("42", 7, appendRow, func(a db.Activity) { got = a.ID })
	if err != nil || got != 101 || len(committed) != 1 {
		t.Fatalf("replay not idempotent: id=%d count=%d err=%v", got, len(committed), err)
	}
}

func TestActivityJournalRejectsMismatchedTaskAndPartialFile(t *testing.T) {
	j := &activityJournal{root: t.TempDir()}
	for _, id := range []string{"../42", "0", "-1", "0042"} {
		if _, err := j.enqueue(id, 7, db.Activity{}); err == nil {
			t.Fatalf("accepted %q", id)
		}
	}
	_, err := j.enqueue("42", 7, db.Activity{Detail: "preserve"})
	if err != nil {
		t.Fatal(err)
	}
	appendRow := func(db.Activity) (int64, error) { t.Fatal("must not write"); return 0, nil }
	if _, err := j.flush("42", 8, appendRow, func(db.Activity) {}); err == nil {
		t.Fatal("wrong exploration accepted")
	}
	dir, _ := j.directory("42")
	if err := os.WriteFile(filepath.Join(dir, "000.tmp"), []byte("{partial"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := j.flush("42", 7, appendRow, func(db.Activity) {}); err == nil {
		t.Fatal("partial record silently discarded")
	}
}

func TestActivityJournalRecoversSyncedTemporaryFile(t *testing.T) {
	j := &activityJournal{root: t.TempDir()}
	row, err := j.enqueue("42", 7, db.Activity{Detail: "full payload"})
	if err != nil {
		t.Fatal(err)
	}
	dir, _ := j.directory("42")
	files, _ := os.ReadDir(dir)
	file := filepath.Join(dir, files[0].Name())
	if err := os.Rename(file, file+".tmp"); err != nil {
		t.Fatal(err)
	}
	ids, err := j.flush("42", 7, func(a db.Activity) (int64, error) {
		if a.Detail != "full payload" {
			t.Fatal("detail missing")
		}
		return 5, nil
	}, func(db.Activity) {})
	if err != nil || ids[row.EventKey] != 5 {
		t.Fatalf("temporary recovery: %v %v", ids, err)
	}
}
