package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// TaskRAGResource deliberately excludes embedding configuration and ownership.
type TaskRAGResource struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

func (d *DB) TaskRAGResources(ctx context.Context, taskID int64) ([]TaskRAGResource, error) {
	rows, err := d.QueryContext(ctx, `SELECT kb.id, kb.name, kb.description
FROM task_rag_knowledge_bases g
JOIN rag_knowledge_bases kb ON kb.id=g.knowledge_base_id
JOIN tasks task ON task.id=g.task_id
WHERE g.task_id=$1 AND task.deleted_at IS NULL AND kb.enabled AND kb.deleted_at IS NULL
ORDER BY g.created_at, kb.id`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]TaskRAGResource, 0)
	for rows.Next() {
		var resource TaskRAGResource
		if err := rows.Scan(&resource.ID, &resource.Name, &resource.Description); err != nil {
			return nil, err
		}
		out = append(out, resource)
	}
	return out, rows.Err()
}

const MaxTaskRAGKnowledgeBases = 32

var (
	ErrTaskRAGKnowledgeBaseIDsInvalid = errors.New("invalid task knowledge base ids")
	ErrTaskRAGKnowledgeBaseNotFound   = errors.New("task knowledge base not found or disabled")
	ErrTaskRAGAccessDenied            = errors.New("task knowledge base access denied")
)

func NormalizeTaskRAGKnowledgeBaseIDs(ids []int64) ([]int64, error) {
	if len(ids) > MaxTaskRAGKnowledgeBases {
		return nil, ErrTaskRAGKnowledgeBaseIDsInvalid
	}
	seen := make(map[int64]bool, len(ids))
	out := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id <= 0 || seen[id] {
			return nil, ErrTaskRAGKnowledgeBaseIDsInvalid
		}
		seen[id] = true
		out = append(out, id)
	}
	return out, nil
}

func insertTaskRAGKnowledgeBases(tx *sql.Tx, taskID int64, ids []int64) error {
	for _, id := range ids {
		result, err := tx.Exec(`INSERT INTO task_rag_knowledge_bases(task_id, knowledge_base_id)
SELECT $1, id FROM rag_knowledge_bases WHERE id=$2 AND enabled AND deleted_at IS NULL`, taskID, id)
		if err != nil {
			return err
		}
		if count, _ := result.RowsAffected(); count != 1 {
			return fmt.Errorf("%w: %d", ErrTaskRAGKnowledgeBaseNotFound, id)
		}
	}
	return nil
}

// SetTaskRAGKnowledgeBaseIDs replaces a task's grants atomically. An invalid
// replacement rolls back, leaving the previous grants intact.
func (d *DB) SetTaskRAGKnowledgeBaseIDs(taskID int64, ids []int64) ([]int64, error) {
	normalized, err := NormalizeTaskRAGKnowledgeBaseIDs(ids)
	if err != nil {
		return nil, err
	}
	tx, err := d.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var id int64
	if err := tx.QueryRow(`SELECT id FROM tasks WHERE id=$1 AND deleted_at IS NULL FOR UPDATE`, taskID).Scan(&id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, sql.ErrNoRows
		}
		return nil, err
	}
	if _, err := tx.Exec(`DELETE FROM task_rag_knowledge_bases WHERE task_id=$1`, taskID); err != nil {
		return nil, err
	}
	if err := insertTaskRAGKnowledgeBases(tx, taskID, normalized); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return normalized, nil
}

func (d *DB) TaskRAGKnowledgeBaseIDs(taskID int64) ([]int64, error) {
	rows, err := d.Query(`SELECT kb.id FROM task_rag_knowledge_bases g
JOIN rag_knowledge_bases kb ON kb.id=g.knowledge_base_id
JOIN tasks task ON task.id=g.task_id
WHERE g.task_id=$1 AND task.deleted_at IS NULL AND kb.enabled AND kb.deleted_at IS NULL
ORDER BY g.created_at, kb.id`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := make([]int64, 0)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (d *DB) TaskCanAccessRAGKnowledgeBase(taskID, kbID int64) (bool, error) {
	if taskID <= 0 || kbID <= 0 {
		return false, nil
	}
	var allowed bool
	err := d.QueryRow(`SELECT EXISTS(
SELECT 1 FROM task_rag_knowledge_bases g
JOIN tasks task ON task.id=g.task_id
JOIN rag_knowledge_bases kb ON kb.id=g.knowledge_base_id
WHERE g.task_id=$1 AND g.knowledge_base_id=$2
AND task.deleted_at IS NULL AND kb.enabled AND kb.deleted_at IS NULL
)`, taskID, kbID).Scan(&allowed)
	return allowed, err
}
