package db

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

var (
	ErrTaskProfileInvalid      = errors.New("invalid task profile entry")
	ErrTaskProfileTaskNotFound = errors.New("task profile task not found")
)

const MaxTaskProfilePage = 50

type TaskProfileInput struct {
	ScopeID       int64     `json:"scope_id"`
	Kind          string    `json:"kind"`
	Label         string    `json:"label"`
	EvidenceKind  string    `json:"evidence_kind"`
	SourceNodeID  *int64    `json:"source_node_id,omitempty"`
	SourceSummary string    `json:"source_summary"`
	SourceQuery   string    `json:"source_query"`
	Confidence    int       `json:"confidence"`
	ObservedAt    time.Time `json:"observed_at"`
}

type TaskProfileEntry struct {
	ID            int64     `json:"id"`
	TaskID        int64     `json:"task_id"`
	ScopeID       *int64    `json:"scope_id"`
	Kind          string    `json:"kind"`
	Label         string    `json:"label"`
	EvidenceKind  string    `json:"evidence_kind"`
	SourceNodeID  *int64    `json:"source_node_id"`
	SourceSummary string    `json:"source_summary"`
	SourceQuery   string    `json:"source_query"`
	Confidence    int       `json:"confidence"`
	DedupeKey     string    `json:"dedupe_key"`
	ObservedAt    time.Time `json:"observed_at"`
	CreatedAt     time.Time `json:"created_at"`
	SameKeyCount  int       `json:"same_key_count"`
}

func normalizeTaskProfileInput(input TaskProfileInput) (TaskProfileInput, string, error) {
	input.Kind = strings.TrimSpace(input.Kind)
	input.Label = strings.TrimSpace(input.Label)
	input.EvidenceKind = strings.TrimSpace(input.EvidenceKind)
	input.SourceSummary = strings.TrimSpace(input.SourceSummary)
	input.SourceQuery = strings.TrimSpace(input.SourceQuery)
	switch input.Kind {
	case "host", "application", "route_family", "identity", "tenant", "technology",
		"data_class", "trust_boundary", "exclusion", "open_question":
	default:
		return input, "", fmt.Errorf("%w: unsupported kind", ErrTaskProfileInvalid)
	}
	if input.EvidenceKind != "observed" && input.EvidenceKind != "inferred" {
		return input, "", fmt.Errorf("%w: unsupported evidence_kind", ErrTaskProfileInvalid)
	}
	if input.ScopeID <= 0 || input.Confidence < 0 || input.Confidence > 100 ||
		input.SourceNodeID != nil && *input.SourceNodeID <= 0 ||
		utf8.RuneCountInString(input.Label) < 1 || utf8.RuneCountInString(input.Label) > 240 ||
		utf8.RuneCountInString(input.SourceSummary) < 1 || utf8.RuneCountInString(input.SourceSummary) > 300 ||
		utf8.RuneCountInString(input.SourceQuery) > 200 {
		return input, "", fmt.Errorf("%w: field out of bounds", ErrTaskProfileInvalid)
	}
	if input.ObservedAt.IsZero() {
		input.ObservedAt = time.Now().UTC()
	}
	if input.ObservedAt.After(time.Now().Add(5 * time.Minute)) {
		return input, "", fmt.Errorf("%w: observed_at is in the future", ErrTaskProfileInvalid)
	}
	key := sha256.Sum256([]byte(input.Kind + "\x00" + strings.ToLower(input.Label)))
	return input, hex.EncodeToString(key[:]), nil
}

// AddTaskProfileEntry does not write task_scope or assets. The referenced
// scope and optional evidence node must belong to this live task.
func (d *DB) AddTaskProfileEntry(taskID int64, input TaskProfileInput) (*TaskProfileEntry, error) {
	input, key, err := normalizeTaskProfileInput(input)
	if err != nil {
		return nil, err
	}
	if taskID <= 0 {
		return nil, ErrTaskProfileInvalid
	}
	entry := &TaskProfileEntry{TaskID: taskID, ScopeID: &input.ScopeID, Kind: input.Kind,
		Label: input.Label, EvidenceKind: input.EvidenceKind, SourceNodeID: input.SourceNodeID,
		SourceSummary: input.SourceSummary, SourceQuery: input.SourceQuery,
		Confidence: input.Confidence, DedupeKey: key, ObservedAt: input.ObservedAt}
	err = d.QueryRow(`
INSERT INTO task_target_profile_entries
    (task_id,scope_id,kind,label,evidence_kind,source_node_id,source_summary,
     source_query,confidence,dedupe_key,observed_at)
SELECT t.id, ts.id, $3,$4,$5,$6,$7,$8,$9,$10,$11
FROM tasks t
JOIN task_scope ts ON ts.task_id=t.id AND ts.id=$2
WHERE t.id=$1 AND t.deleted_at IS NULL
  AND ($6::bigint IS NULL OR EXISTS (
      SELECT 1 FROM exploration_nodes n WHERE n.id=$6 AND n.exploration_id=t.exploration_id))
RETURNING id,created_at`, taskID, input.ScopeID, input.Kind, input.Label,
		input.EvidenceKind, input.SourceNodeID, input.SourceSummary,
		input.SourceQuery, input.Confidence, key, input.ObservedAt).Scan(&entry.ID, &entry.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrTaskProfileTaskNotFound
	}
	return entry, err
}

// ListTaskProfileEntries is bounded and keeps individual observations, even
// when their dedupe keys agree. A detached scope remains visible as null.
func (d *DB) ListTaskProfileEntries(taskID, before int64, limit int) ([]TaskProfileEntry, error) {
	if taskID <= 0 || before < 0 || limit < 1 || limit > MaxTaskProfilePage {
		return nil, ErrTaskProfileInvalid
	}
	rows, err := d.Query(`
SELECT p.id,p.task_id,p.scope_id,p.kind,p.label,p.evidence_kind,p.source_node_id,
       p.source_summary,p.source_query,p.confidence,p.dedupe_key,p.observed_at,p.created_at,
       (SELECT count(*) FROM task_target_profile_entries same
        WHERE same.task_id=p.task_id AND same.dedupe_key=p.dedupe_key)
FROM task_target_profile_entries p
WHERE p.task_id=$1 AND ($2::bigint=0 OR p.id<$2)
ORDER BY p.id DESC LIMIT $3`, taskID, before, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	entries := []TaskProfileEntry{}
	for rows.Next() {
		var entry TaskProfileEntry
		if err := rows.Scan(&entry.ID, &entry.TaskID, &entry.ScopeID, &entry.Kind, &entry.Label,
			&entry.EvidenceKind, &entry.SourceNodeID, &entry.SourceSummary, &entry.SourceQuery,
			&entry.Confidence, &entry.DedupeKey, &entry.ObservedAt, &entry.CreatedAt,
			&entry.SameKeyCount); err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	return entries, rows.Err()
}
