package db

import (
	"os"
	"strconv"
	"testing"
)

func TestTaskParentReferenceIsAtomic(t *testing.T) {
	if os.Getenv("YUNKUN_RAG_TEST_ISOLATED") != "1" {
		t.Skip("isolated database test disabled")
	}
	d := openIsolatedRAGTestDB(t)
	parent, err := d.CreateTask("parent", "test", nil, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	child, err := d.CreateTaskWithOptions("child", "test", TaskCreateOptions{ParentTaskID: &parent.ID})
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := d.GetTask(child.ID)
	if err != nil || loaded == nil || loaded.ParentRef != strconv.FormatInt(parent.ID, 10) {
		t.Fatalf("parent reference not persisted: %+v, %v", loaded, err)
	}
	var countBefore int
	if err := d.QueryRow(`SELECT count(*) FROM tasks`).Scan(&countBefore); err != nil {
		t.Fatal(err)
	}
	missing := int64(999999999)
	if _, err := d.CreateTaskWithOptions("orphan", "test", TaskCreateOptions{ParentTaskID: &missing}); err == nil {
		t.Fatal("accepted missing parent")
	}
	var countAfter int
	if err := d.QueryRow(`SELECT count(*) FROM tasks`).Scan(&countAfter); err != nil || countAfter != countBefore {
		t.Fatalf("invalid parent left an orphan task: %d -> %d, %v", countBefore, countAfter, err)
	}
}
