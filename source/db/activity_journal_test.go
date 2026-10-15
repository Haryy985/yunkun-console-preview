package db

import (
	"fmt"
	"testing"
	"time"
)

func TestActivityReplayIdempotentPreservesOriginal(t *testing.T) {
	d, err := Open(testDSN(t))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	task, err := d.CreateTaskWithOptions("activity journal fixture", "local fixture", TaskCreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = d.Exec("DELETE FROM tasks WHERE id=$1", task.ID)
		_, _ = d.Exec("DELETE FROM explorations WHERE id=$1", task.ExplorationID)
	})
	store := d.Exploration(task.ExplorationID)
	row := Activity{EventKey: fmt.Sprintf("journal-test-%d", time.Now().UnixNano()),
		Kind: "result", Detail: "original detail", CreatedAt: time.Now().UTC().Add(-time.Hour).Truncate(time.Microsecond)}
	first, err := store.AppendActivity(row)
	if err != nil {
		t.Fatal(err)
	}
	row.Detail = "must not overwrite"
	second, err := store.AppendActivity(row)
	if err != nil || first != second {
		t.Fatalf("duplicate replay: %d %d %v", first, second, err)
	}
	var count int
	var detail string
	var created time.Time
	if err := d.QueryRow("SELECT detail,created_at FROM activity WHERE id=$1", first).Scan(&detail, &created); err != nil {
		t.Fatal(err)
	}
	if err := d.QueryRow("SELECT COUNT(*) FROM activity WHERE event_key=$1", row.EventKey).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 || detail != "original detail" || !created.Equal(row.CreatedAt) {
		t.Fatalf("replay altered original: %d %q %v", count, detail, created)
	}
}
