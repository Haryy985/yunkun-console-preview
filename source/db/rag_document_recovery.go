package db

import (
	"database/sql"
	"errors"
)

// RecoverRAGIngestionJobs runs once at single-server startup, before accepting
// requests. Ingestion workers live in memory and cannot survive a process exit.
func (d *DB) RecoverRAGIngestionJobs() (int64, error) {
	tx, err := d.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	var count int64
	err = tx.QueryRow(`
WITH interrupted AS (
  UPDATE rag_ingestion_jobs
  SET status='failed', error='Service restarted before ingestion completed; retry the document.',
      finished_at=now(), updated_at=now()
  WHERE status IN ('queued','running')
  RETURNING document_id
), documents AS (
  UPDATE rag_documents
  SET status='failed', error='Service restarted before ingestion completed; retry the document.'
  WHERE id IN (SELECT document_id FROM interrupted) AND deleted_at IS NULL
)
SELECT count(*) FROM interrupted`).Scan(&count)
	if err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return count, nil
}

// GetRAGDocumentIncludingDeleted is used by recovery flows after soft deletion.
func (d *DB) GetRAGDocumentIncludingDeleted(id int64) (*RAGDocument, error) {
	item, err := scanRAGDocument(d.QueryRow(ragDocumentSelect+`
WHERE d.id=$1`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return item, nil
}
