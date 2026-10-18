package db

import (
	"errors"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"
)

func TestNormalizeTaskProfileInput(t *testing.T) {
	valid := TaskProfileInput{
		ScopeID: 4, Kind: "application", Label: "  Portal  ",
		EvidenceKind: "observed", SourceSummary: "operator observation",
		Confidence: 65,
	}
	entry, key, err := normalizeTaskProfileInput(valid)
	if err != nil || entry.Label != "Portal" || len(key) != 64 || entry.ObservedAt.IsZero() {
		t.Fatalf("normalized=%+v key=%q err=%v", entry, key, err)
	}
	capitalized := valid
	capitalized.Label = "portal"
	_, same, err := normalizeTaskProfileInput(capitalized)
	if err != nil || same != key {
		t.Fatal("case-only duplicate must share a dedupe key")
	}
	for _, tc := range []struct {
		name string
		edit func(*TaskProfileInput)
	}{
		{"no scope", func(x *TaskProfileInput) { x.ScopeID = 0 }},
		{"invalid kind", func(x *TaskProfileInput) { x.Kind = "scope" }},
		{"empty label", func(x *TaskProfileInput) { x.Label = "  " }},
		{"long label", func(x *TaskProfileInput) { x.Label = strings.Repeat("x", 241) }},
		{"missing source", func(x *TaskProfileInput) { x.SourceSummary = "" }},
		{"long query", func(x *TaskProfileInput) { x.SourceQuery = strings.Repeat("x", 201) }},
		{"invalid confidence", func(x *TaskProfileInput) { x.Confidence = 101 }},
		{"future observation", func(x *TaskProfileInput) { x.ObservedAt = time.Now().Add(time.Hour) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input := valid
			tc.edit(&input)
			if _, _, err := normalizeTaskProfileInput(input); !errors.Is(err, ErrTaskProfileInvalid) {
				t.Fatalf("expected validation failure, got %v", err)
			}
		})
	}
}

func TestTaskProfileProvenanceDoesNotExpandScope(t *testing.T) {
	dsn := os.Getenv("YUNKUN_TEST_PG_DSN")
	u, err := url.Parse(dsn)
	if dsn == "" || err != nil || !strings.HasPrefix(strings.TrimPrefix(u.Path, "/"), "yunkun_test_") {
		t.Skip("isolated YUNKUN_TEST_PG_DSN required")
	}
	t.Setenv("YUNKUN_PG_DSN", dsn)
	d, err := Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	task, err := d.CreateTask("profile fixture", "goal", nil, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer d.DeleteTask(task.ID)
	_, err = d.Assets().AddAgentScope(task.ID, "ip", "192.0.2.10", "authorized test", "manual")
	if err != nil {
		t.Fatal(err)
	}
	rows, err := d.Assets().ListTaskScope(task.ID)
	if err != nil || len(rows) != 1 {
		t.Fatalf("initial scope=%v err=%v", rows, err)
	}
	input := TaskProfileInput{ScopeID: rows[0].ID, Kind: "host", Label: "candidate.example.test",
		EvidenceKind: "inferred", SourceSummary: "observed in public inventory", Confidence: 30}
	if _, err := d.AddTaskProfileEntry(task.ID, input); err != nil {
		t.Fatal(err)
	}
	input.SourceSummary = "second independent source"
	if _, err := d.AddTaskProfileEntry(task.ID, input); err != nil {
		t.Fatal(err)
	}
	entries, err := d.ListTaskProfileEntries(task.ID, 0, 1)
	if err != nil || len(entries) != 1 || entries[0].SameKeyCount != 2 {
		t.Fatalf("page=%+v err=%v", entries, err)
	}
	next, err := d.ListTaskProfileEntries(task.ID, entries[0].ID, 5)
	if err != nil || len(next) != 1 || next[0].SourceSummary != "observed in public inventory" {
		t.Fatalf("next=%+v err=%v", next, err)
	}
	after, err := d.Assets().ListTaskScope(task.ID)
	if err != nil || len(after) != 1 || after[0].ID != rows[0].ID {
		t.Fatalf("profile altered scope: %+v err=%v", after, err)
	}
	other, err := d.CreateTask("unrelated fixture", "goal", nil, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer d.DeleteTask(other.ID)
	if _, err := d.AddTaskProfileEntry(other.ID, input); !errors.Is(err, ErrTaskProfileTaskNotFound) {
		t.Fatalf("cross-task scope accepted: %v", err)
	}
	var otherNode int64
	if err := d.QueryRow(`SELECT id FROM exploration_nodes WHERE exploration_id=$1 ORDER BY id LIMIT 1`,
		other.ExplorationID).Scan(&otherNode); err != nil {
		t.Fatal(err)
	}
	input.SourceNodeID = &otherNode
	if _, err := d.AddTaskProfileEntry(task.ID, input); !errors.Is(err, ErrTaskProfileTaskNotFound) {
		t.Fatalf("cross-task evidence node accepted: %v", err)
	}
}
