package db

import (
	"context"
	"crypto/sha256"
	"errors"

	"yunkun/console/proxypool"
)

// The cursor belongs to the pool, not the task lease. Advancing it never
// revokes another worker's in-flight route.
func (d *DB) SelectProxyRouteNode(ctx context.Context, b *TaskProxyBinding, advance bool) (*ProxyPool, *proxypool.Node, error) {
	tx, err := d.BeginTx(ctx, nil)
	if err != nil {
		return nil, nil, err
	}
	defer tx.Rollback()
	var cursor int64
	if err := tx.QueryRowContext(ctx, `SELECT route_cursor FROM proxy_pools WHERE id=$1 FOR UPDATE`, b.PoolID).Scan(&cursor); err != nil {
		return nil, nil, err
	}
	p, err := scanProxyPool(tx.QueryRowContext(ctx, `SELECT id,config,credentials,quota FROM proxy_pools WHERE id=$1`, b.PoolID))
	if err != nil || !p.Config.Enabled {
		return nil, nil, errors.New("proxy pool disabled or unavailable")
	}
	var selected int64
	err = tx.QueryRowContext(ctx, `SELECT id FROM proxy_nodes WHERE pool_id=$1 AND enabled AND deleted_at IS NULL AND last_error=''
AND (expires_at IS NULL OR expires_at>now()+make_interval(secs=>$2))
AND (runtime_cooldown_until IS NULL OR runtime_cooldown_until<=now())
AND ($3='round_robin' OR ($3='fixed' AND id=$4))
AND (origin NOT IN ('fofa','quake') OR (((last_verify_mode='connectivity' AND last_http_status=200 AND last_success_at>now()-interval '15 minutes')
OR (last_verify_mode IN ('','exit_ip') AND verified_at>now()-interval '15 minutes' AND verified_ip<>''))
AND last_error='' AND (last_failure_at IS NULL OR last_success_at>=last_failure_at)))
ORDER BY CASE WHEN id>$5 THEN 0 ELSE 1 END,id LIMIT 1`,
		b.PoolID, p.Config.SafetySeconds, b.Strategy, b.NodeID, cursor).Scan(&selected)
	if err != nil {
		return nil, nil, errors.New("no eligible proxy nodes; verify nodes or wait for cooldown; direct fallback forbidden")
	}
	if advance && b.Strategy == "round_robin" {
		if _, err := tx.ExecContext(ctx, `UPDATE proxy_pools SET route_cursor=$2 WHERE id=$1`, b.PoolID, selected); err != nil {
			return nil, nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, nil, err
	}
	n, err := d.ProxyNode(b.PoolID, selected)
	return p, n, err
}

func (d *DB) SetProxyRouting(taskID int64, strategy string, nodeID *int64) error {
	if strategy != "round_robin" && strategy != "fixed" {
		return errors.New("invalid proxy routing strategy")
	}
	if strategy == "fixed" && (nodeID == nil || *nodeID <= 0) {
		return errors.New("fixed routing requires an explicit node")
	}
	if strategy == "round_robin" && nodeID != nil {
		return errors.New("round robin must not specify a fixed node")
	}
	tx, err := d.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var allowed bool
	if err := tx.QueryRow(`SELECT deleted_at IS NULL AND (paused OR queued OR status IN ('done','failed','timeout')) FROM tasks WHERE id=$1 FOR UPDATE`, taskID).Scan(&allowed); err != nil || !allowed {
		return errors.New("pause task before changing proxy routing")
	}
	var poolID int64
	if err := tx.QueryRow(`SELECT pool_id FROM task_proxy_bindings WHERE task_id=$1 FOR UPDATE`, taskID).Scan(&poolID); err != nil {
		return errors.New("task has no proxy binding")
	}
	if strategy == "fixed" {
		p, err := scanProxyPool(tx.QueryRow(`SELECT id,config,credentials,quota FROM proxy_pools WHERE id=$1 FOR SHARE`, poolID))
		if err != nil || !p.Config.Enabled {
			return errors.New("proxy pool unavailable")
		}
		var valid bool
		if err := tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM proxy_nodes WHERE id=$1 AND pool_id=$2 AND enabled AND deleted_at IS NULL AND last_error=''
AND (runtime_cooldown_until IS NULL OR runtime_cooldown_until<=now())
AND (expires_at IS NULL OR expires_at>now()+make_interval(secs=>$3))
AND (origin NOT IN ('fofa','quake') OR (((last_verify_mode='connectivity' AND last_http_status=200 AND last_success_at>now()-interval '15 minutes')
OR (last_verify_mode IN ('','exit_ip') AND verified_at>now()-interval '15 minutes' AND verified_ip<>''))
AND (last_failure_at IS NULL OR last_success_at>=last_failure_at))))`, *nodeID, poolID, p.Config.SafetySeconds).Scan(&valid); err != nil || !valid {
			return errors.New("selected node is unavailable or belongs to another pool")
		}
	}
	if _, err := tx.Exec(`UPDATE task_proxy_bindings SET strategy=$2,node_id=CASE WHEN $2='fixed' THEN $3 ELSE node_id END,
expires_at=CASE WHEN $2='fixed' THEN (SELECT expires_at FROM proxy_nodes WHERE id=$3) ELSE NULL END,
revision=revision+1 WHERE task_id=$1`, taskID, strategy, nodeID); err != nil {
		return err
	}
	if _, err := tx.Exec(`INSERT INTO proxy_route_events(task_id,pool_id,node_id,target,result) VALUES($1,$2,$3,'',$4)`,
		taskID, poolID, nodeID, "routing_"+strategy); err != nil {
		return err
	}
	return tx.Commit()
}

// Runtime failures are separate from verification evidence. A cooldown allows
// a later trial, but never refreshes a discovery node's verification timestamp.
func (d *DB) RecordProxyRuntimeFailure(route proxypool.Route, reason string) error {
	tx, err := d.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var credentials string
	if err := tx.QueryRow(`SELECT credentials FROM proxy_pools WHERE id=$1 FOR SHARE`, route.PoolID).Scan(&credentials); err != nil {
		return err
	}
	if sha256.Sum256([]byte(credentials)) != route.CredentialTag {
		return ErrProxyPoolCredentialsChanged
	}
	_, err = tx.Exec(`UPDATE proxy_nodes SET runtime_cooldown_until=now()+interval '60 seconds',runtime_error=$3
WHERE pool_id=$1 AND id=$2 AND deleted_at IS NULL`, route.PoolID, route.Node.ID, reason)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (d *DB) RecordProxyRuntimeSuccess(route proxypool.Route) error {
	tx, err := d.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var credentials string
	if err := tx.QueryRow(`SELECT credentials FROM proxy_pools WHERE id=$1 FOR SHARE`, route.PoolID).Scan(&credentials); err != nil {
		return err
	}
	if sha256.Sum256([]byte(credentials)) != route.CredentialTag {
		return ErrProxyPoolCredentialsChanged
	}
	// Only a trial that observed this cooldown can clear it. An older request
	// finishing late must not clear a subsequently recorded failure.
	_, err = tx.Exec(`UPDATE proxy_nodes SET runtime_error='',runtime_cooldown_until=NULL
WHERE pool_id=$1 AND id=$2 AND runtime_error<>'' AND runtime_cooldown_until<=now()
AND runtime_cooldown_until=$3`,
		route.PoolID, route.Node.ID, route.Node.RuntimeCooldownUntil)
	if err != nil {
		return err
	}
	return tx.Commit()
}

// Recovery is explicit, preserves inventory and old ciphertext, and cannot
// replace credentials while bound tasks are allowed to send traffic.
func (d *DB) RecoverProxyCredentials(poolID int64, expected, replacement string) error {
	tx, err := d.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var current string
	if err := tx.QueryRow(`SELECT credentials FROM proxy_pools WHERE id=$1 FOR UPDATE`, poolID).Scan(&current); err != nil {
		return err
	}
	if current != expected {
		return ErrProxyPoolCredentialsChanged
	}
	// Task operations lock tasks before pools; do not wait in the reverse order.
	rows, err := tx.Query(`SELECT t.deleted_at IS NOT NULL OR t.paused OR t.queued OR t.status IN ('done','failed','timeout')
FROM tasks t JOIN task_proxy_bindings b ON b.task_id=t.id WHERE b.pool_id=$1 ORDER BY t.id FOR UPDATE OF t NOWAIT`, poolID)
	if err != nil {
		return err
	}
	for rows.Next() {
		var safe bool
		if err := rows.Scan(&safe); err != nil {
			rows.Close()
			return err
		}
		if !safe {
			rows.Close()
			return errors.New("pause all bound tasks before recovering pool credentials")
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	if _, err := tx.Exec(`INSERT INTO proxy_credential_recoveries(pool_id,previous_credentials) VALUES($1,$2)`, poolID, current); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE proxy_pools SET credentials=$2,quota='{}',updated_at=now() WHERE id=$1`, poolID, replacement); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE proxy_nodes SET enabled=false,verified_ip='',verified_at=NULL,last_success_at=NULL,last_checked_at=NULL,
last_verify_mode='',last_verify_target='',last_http_status=0,latency_ms=NULL,
last_error='proxy credentials recovered; reverify node',runtime_error='',runtime_cooldown_until=NULL,
import_verify_pending=false,import_verify_claimed_at=NULL WHERE pool_id=$1 AND deleted_at IS NULL`, poolID); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE task_proxy_bindings SET revision=revision+1 WHERE pool_id=$1`, poolID); err != nil {
		return err
	}
	if _, err := tx.Exec(`INSERT INTO proxy_route_events(pool_id,target,result) VALUES($1,'','credentials_recovered_reverify_required')`, poolID); err != nil {
		return err
	}
	return tx.Commit()
}
