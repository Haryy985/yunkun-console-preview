package db

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrRAGProposalInvalid  = errors.New("invalid RAG proposal")
	ErrRAGProposalNotFound = errors.New("RAG proposal not found")
	ErrRAGProposalConflict = errors.New("RAG proposal changed or document version conflict")
)

type RAGChangeProposal struct {
	ID                  int64      `json:"id"`
	KnowledgeBaseID     int64      `json:"knowledge_base_id"`
	TaskID              int64      `json:"task_id"`
	AgentID             string     `json:"agent_id"`
	Kind                string     `json:"kind"`
	TargetDocumentID    *int64     `json:"target_document_id"`
	BaseVersion         *int       `json:"base_version"`
	Title               string     `json:"title"`
	Content             string     `json:"content"`
	Rationale           string     `json:"rationale"`
	SourceRef           string     `json:"source_ref"`
	Status              string     `json:"status"`
	Reviewer            string     `json:"reviewer"`
	ReviewReason        string     `json:"review_reason"`
	ReviewedAt          *time.Time `json:"reviewed_at"`
	PublishedDocumentID *int64     `json:"published_document_id"`
	CreatedAt           time.Time  `json:"created_at"`
}

const ragProposalColumns = `id, knowledge_base_id, task_id, agent_id, kind, target_document_id,
 base_version, title, content, rationale, source_ref, status, reviewer, review_reason,
 reviewed_at, published_document_id, created_at`

func scanRAGProposal(row interface{ Scan(...any) error }) (*RAGChangeProposal, error) {
	p := new(RAGChangeProposal)
	err := row.Scan(&p.ID, &p.KnowledgeBaseID, &p.TaskID, &p.AgentID, &p.Kind,
		&p.TargetDocumentID, &p.BaseVersion, &p.Title, &p.Content, &p.Rationale,
		&p.SourceRef, &p.Status, &p.Reviewer, &p.ReviewReason, &p.ReviewedAt,
		&p.PublishedDocumentID, &p.CreatedAt)
	return p, err
}

func (d *DB) CreateRAGProposal(p RAGChangeProposal) (*RAGChangeProposal, error) {
	p.Title, p.Content = strings.TrimSpace(p.Title), strings.TrimSpace(p.Content)
	p.Rationale, p.SourceRef = strings.TrimSpace(p.Rationale), strings.TrimSpace(p.SourceRef)
	p.AgentID = strings.TrimSpace(p.AgentID)
	if p.KnowledgeBaseID <= 0 || p.TaskID <= 0 || p.AgentID == "" ||
		p.Title == "" || len(p.Title) > 200 || p.Content == "" || len(p.Content) > 65536 ||
		p.Rationale == "" || len(p.Rationale) > 2000 || p.SourceRef == "" || len(p.SourceRef) > 1000 ||
		(p.Kind != "create" && p.Kind != "update") ||
		(p.Kind == "create" && p.TargetDocumentID != nil) ||
		(p.Kind == "update" && (p.TargetDocumentID == nil || *p.TargetDocumentID <= 0)) {
		return nil, ErrRAGProposalInvalid
	}
	tx, err := d.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var exists bool
	var lockedTask int64
	if err = tx.QueryRow(`SELECT id FROM tasks WHERE id=$1 AND deleted_at IS NULL FOR SHARE`, p.TaskID).Scan(&lockedTask); errors.Is(err, sql.ErrNoRows) {
		return nil, ErrTaskRAGAccessDenied
	} else if err != nil {
		return nil, err
	}
	if err = tx.QueryRow(`SELECT EXISTS(
SELECT 1 FROM task_rag_knowledge_bases g
JOIN tasks task ON task.id=g.task_id
JOIN rag_knowledge_bases kb ON kb.id=g.knowledge_base_id
WHERE g.task_id=$1 AND g.knowledge_base_id=$2
AND task.deleted_at IS NULL AND kb.enabled AND kb.deleted_at IS NULL
)`, p.TaskID, p.KnowledgeBaseID).Scan(&exists); err != nil {
		return nil, err
	}
	if !exists {
		return nil, ErrTaskRAGAccessDenied
	}
	if err = tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM rag_knowledge_bases WHERE id=$1 AND enabled AND deleted_at IS NULL)`, p.KnowledgeBaseID).Scan(&exists); err != nil {
		return nil, err
	}
	if !exists {
		return nil, ErrRAGKnowledgeBaseNotFound
	}
	if p.Kind == "update" {
		var version int
		err = tx.QueryRow(`SELECT version FROM rag_documents WHERE id=$1 AND knowledge_base_id=$2 AND deleted_at IS NULL`,
			*p.TargetDocumentID, p.KnowledgeBaseID).Scan(&version)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrRAGDocumentNotFound
		}
		if err != nil {
			return nil, err
		}
		p.BaseVersion = &version
	}
	published, err := scanRAGProposal(tx.QueryRow(`
INSERT INTO rag_change_proposals
 (knowledge_base_id, task_id, agent_id, kind, target_document_id, base_version, title, content, rationale, source_ref)
 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) RETURNING `+ragProposalColumns,
		p.KnowledgeBaseID, p.TaskID, p.AgentID, p.Kind, p.TargetDocumentID, p.BaseVersion,
		p.Title, p.Content, p.Rationale, p.SourceRef))
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return published, nil
}

func (d *DB) ListRAGProposals(kbID int64, status string) ([]*RAGChangeProposal, error) {
	if status != "" && status != "pending" && status != "approved" && status != "rejected" {
		return nil, ErrRAGProposalInvalid
	}
	rows, err := d.Query(`SELECT `+ragProposalColumns+`
 FROM rag_change_proposals WHERE knowledge_base_id=$1 AND ($2='' OR status=$2)
 ORDER BY created_at DESC, id DESC LIMIT 100`, kbID, status)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]*RAGChangeProposal, 0)
	for rows.Next() {
		p, err := scanRAGProposal(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// ReviewRAGProposal atomically decides once and queues a new immutable revision.
// The prior revision remains searchable until the new revision finishes ingestion.
func (d *DB) ReviewRAGProposal(id int64, approve bool, reviewer, reason string) (*RAGChangeProposal, int64, error) {
	reviewer, reason = strings.TrimSpace(reviewer), strings.TrimSpace(reason)
	if id <= 0 || reviewer == "" || len(reason) > 2000 || (!approve && reason == "") {
		return nil, 0, ErrRAGProposalInvalid
	}
	tx, err := d.Begin()
	if err != nil {
		return nil, 0, err
	}
	defer tx.Rollback()
	p, err := scanRAGProposal(tx.QueryRow(`SELECT `+ragProposalColumns+` FROM rag_change_proposals WHERE id=$1 FOR UPDATE`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, 0, ErrRAGProposalNotFound
	}
	if err != nil {
		return nil, 0, err
	}
	if p.Status != "pending" {
		return nil, 0, ErrRAGProposalConflict
	}
	if p.Kind == "update" && (p.TargetDocumentID == nil || p.BaseVersion == nil) {
		return nil, 0, ErrRAGProposalInvalid
	}
	var jobID int64
	if approve {
		var lockedTask int64
		if err = tx.QueryRow(`SELECT id FROM tasks WHERE id=$1 AND deleted_at IS NULL FOR SHARE`, p.TaskID).Scan(&lockedTask); errors.Is(err, sql.ErrNoRows) {
			return nil, 0, ErrTaskRAGAccessDenied
		} else if err != nil {
			return nil, 0, err
		}
		var authorized bool
		if err = tx.QueryRow(`SELECT EXISTS(
SELECT 1 FROM task_rag_knowledge_bases g
JOIN tasks task ON task.id=g.task_id
WHERE g.task_id=$1 AND g.knowledge_base_id=$2 AND task.deleted_at IS NULL
)`, p.TaskID, p.KnowledgeBaseID).Scan(&authorized); err != nil {
			return nil, 0, err
		}
		if !authorized {
			return nil, 0, ErrTaskRAGAccessDenied
		}
		var enabled bool
		err = tx.QueryRow(`SELECT enabled FROM rag_knowledge_bases WHERE id=$1 AND deleted_at IS NULL FOR UPDATE`, p.KnowledgeBaseID).Scan(&enabled)
		if errors.Is(err, sql.ErrNoRows) || (err == nil && !enabled) {
			return nil, 0, ErrRAGKnowledgeBaseNotFound
		}
		if err != nil {
			return nil, 0, err
		}
		version := 1
		if p.Kind == "update" {
			var current int
			err = tx.QueryRow(`SELECT version FROM rag_documents WHERE id=$1 AND knowledge_base_id=$2 AND deleted_at IS NULL FOR UPDATE`,
				*p.TargetDocumentID, p.KnowledgeBaseID).Scan(&current)
			if errors.Is(err, sql.ErrNoRows) || (err == nil && current != *p.BaseVersion) {
				return nil, 0, ErrRAGProposalConflict
			}
			if err != nil {
				return nil, 0, err
			}
			var replaced bool
			err = tx.QueryRow(`SELECT EXISTS(
 SELECT 1 FROM rag_change_proposals prior
 JOIN rag_ingestion_jobs j ON j.document_id=prior.published_document_id AND j.status='succeeded'
 WHERE prior.target_document_id=$1 AND prior.status='approved'
)`, *p.TargetDocumentID).Scan(&replaced)
			if err != nil {
				return nil, 0, err
			}
			if replaced {
				return nil, 0, ErrRAGProposalConflict
			}
			version = current + 1
		}
		sum := sha256.Sum256([]byte(p.Content))
		hash := hex.EncodeToString(sum[:])
		var docID int64
		err = tx.QueryRow(`INSERT INTO rag_documents
 (knowledge_base_id, name, source_type, source_path, mime_type, content_hash, sha256, content, version)
 VALUES ($1,$2,'manual',$3,'text/markdown',$4,$4,$5,$6)
 ON CONFLICT DO NOTHING RETURNING id`, p.KnowledgeBaseID, p.Title,
			fmt.Sprintf("agent-proposal/%d/task/%d", p.ID, p.TaskID), hash, p.Content, version).Scan(&docID)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, 0, ErrRAGDocumentDuplicate
		}
		if err != nil {
			return nil, 0, err
		}
		p.PublishedDocumentID = &docID
		err = tx.QueryRow(`INSERT INTO rag_ingestion_jobs(knowledge_base_id,document_id,kind)
 VALUES ($1,$2,'document_ingest') RETURNING id`, p.KnowledgeBaseID, docID).Scan(&jobID)
		if err != nil {
			return nil, 0, err
		}
		p.Status = "approved"
	} else {
		p.Status = "rejected"
	}
	p, err = scanRAGProposal(tx.QueryRow(`UPDATE rag_change_proposals SET
 status=$2, reviewer=$3, review_reason=$4, reviewed_at=now(), published_document_id=$5
 WHERE id=$1 RETURNING `+ragProposalColumns, id, p.Status, reviewer, reason, p.PublishedDocumentID))
	if err != nil {
		return nil, 0, err
	}
	if err = tx.Commit(); err != nil {
		return nil, 0, err
	}
	return p, jobID, nil
}

func (d *DB) CompleteRAGProposalIngestion(documentID, jobID int64) error {
	tx, err := d.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.Exec(`UPDATE rag_ingestion_jobs
 SET status='succeeded', finished_at=now(), error=''
 WHERE id=$1 AND document_id=$2 AND status='running'`, jobID, documentID)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return ErrRAGProposalConflict
	}
	_, err = tx.Exec(`UPDATE rag_documents old SET deleted_at=now(), deleted_by='rag-review'
 FROM rag_change_proposals p
 WHERE p.published_document_id=$1 AND p.kind='update' AND old.id=p.target_document_id
 AND old.deleted_at IS NULL AND EXISTS (
   SELECT 1 FROM rag_documents newer WHERE newer.id=$1 AND newer.status='ready'
 )`, documentID)
	if err != nil {
		return err
	}
	return tx.Commit()
}
