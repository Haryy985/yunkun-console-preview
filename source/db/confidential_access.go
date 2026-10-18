package db

import (
	"context"
	"errors"
	"time"
)

type ConfidentialAccessEvent struct {
	ID         int64     `json:"id"`
	ActorName  string    `json:"actor_name"`
	Action     string    `json:"action"`
	Resource   string    `json:"resource"`
	FindingID  *int64    `json:"finding_id"`
	FindingIDs []int64   `json:"finding_ids"`
	Detail     string    `json:"detail"`
	CreatedAt  time.Time `json:"created_at"`
}

// RecordConfidentialAccess persists metadata only; never store packet contents here.
func (d *DB) RecordConfidentialAccess(ctx context.Context, actorID int64, actorName, action, resource string, findingID int64, detail string, findingIDs ...[]int64) error {
	if d == nil || d.DB == nil || actorName == "" || len(detail) > 128 {
		return errors.New("invalid confidential access event")
	}
	var id any
	if findingID > 0 {
		id = findingID
	}
	ids := []int64{}
	if len(findingIDs) > 0 {
		ids = findingIDs[0]
	}
	_, err := d.ExecContext(ctx, `INSERT INTO confidential_access_events
		(actor_id,actor_name,action,resource,finding_id,detail,finding_ids)
		VALUES (NULLIF($1,0),$2,$3,$4,$5,$6,$7)`,
		actorID, actorName, action, resource, id, detail, ids)
	return err
}

func (d *DB) ListConfidentialAccess(ctx context.Context, before int64, limit int) ([]ConfidentialAccessEvent, error) {
	if d == nil || d.DB == nil {
		return nil, errors.New("audit database unavailable")
	}
	if limit < 1 || limit > 100 {
		limit = 50
	}
	rows, err := d.QueryContext(ctx, `SELECT id,actor_name,action,resource,finding_id,finding_ids,detail,created_at
		FROM confidential_access_events WHERE ($1::bigint=0 OR id<$1)
		ORDER BY id DESC LIMIT $2`, before, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ConfidentialAccessEvent{}
	for rows.Next() {
		var event ConfidentialAccessEvent
		if err := rows.Scan(&event.ID, &event.ActorName, &event.Action, &event.Resource,
			&event.FindingID, &event.FindingIDs, &event.Detail, &event.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, event)
	}
	return out, rows.Err()
}
