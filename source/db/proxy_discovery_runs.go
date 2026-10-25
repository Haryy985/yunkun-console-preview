package db

import (
	"database/sql"
	"errors"
	"time"
)

type ProxyDiscoveryRun struct {
	ID              int64     `json:"id"`
	SourceID        int64     `json:"source_id"`
	OffsetStart     int       `json:"offset_start"`
	Requested       int       `json:"requested"`
	Returned        int       `json:"returned"`
	Parsed          int       `json:"parsed"`
	Inserted        int       `json:"inserted"`
	Authorized      int       `json:"authorized"`
	UnknownProtocol int       `json:"unknown_protocol"`
	CreatedAt       time.Time `json:"created_at"`
}

type ProxyVerificationBreakdown struct {
	Stage string `json:"stage"`
	Code  string `json:"code"`
	Count int    `json:"count"`
}

type ProxyDiscoveryRunSummary struct {
	Run       ProxyDiscoveryRun            `json:"run"`
	Attempted int                          `json:"attempted"`
	Succeeded int                          `json:"succeeded"`
	States    map[string]int               `json:"states"`
	Failures  []ProxyVerificationBreakdown `json:"failures"`
}

// A process interruption leaves old in-flight rows behind. A one-minute
// grace period avoids taking over another live instance's short validation.
func (d *DB) RecoverStaleProxyDiscoveryVerifications() (int64, error) {
	result, err := d.Exec(`UPDATE proxy_discovery_results SET state='pending',verified_ip='',verified_at=NULL,
latency_ms=NULL,last_error='验证中断，请重新验证',updated_at=now()
WHERE state='verifying' AND updated_at<now()-interval '1 minute'`)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (d *DB) LatestProxyDiscoveryRun(sourceID int64) (*ProxyDiscoveryRunSummary, error) {
	var run ProxyDiscoveryRun
	err := d.QueryRow(`SELECT id,source_id,offset_start,requested,returned,parsed,inserted,authorized,unknown_protocol,created_at
FROM proxy_discovery_runs WHERE source_id=$1 ORDER BY id DESC LIMIT 1`, sourceID).Scan(
		&run.ID, &run.SourceID, &run.OffsetStart, &run.Requested, &run.Returned,
		&run.Parsed, &run.Inserted, &run.Authorized, &run.UnknownProtocol, &run.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	summary := &ProxyDiscoveryRunSummary{Run: run, States: map[string]int{}, Failures: []ProxyVerificationBreakdown{}}
	stateRows, err := d.Query(`SELECT state,count(*) FROM proxy_discovery_results
WHERE first_run_id=$1 GROUP BY state`, run.ID)
	if err != nil {
		return nil, err
	}
	for stateRows.Next() {
		var state string
		var count int
		if err := stateRows.Scan(&state, &count); err != nil {
			stateRows.Close()
			return nil, err
		}
		summary.States[state] = count
	}
	if err := stateRows.Err(); err != nil {
		stateRows.Close()
		return nil, err
	}
	stateRows.Close()
	rows, err := d.Query(`SELECT stage,code,count(*) FROM (
SELECT DISTINCT ON (v.result_id) v.result_id,v.stage,v.code,v.outcome
FROM proxy_discovery_verifications v
JOIN proxy_discovery_results r ON r.id=v.result_id AND r.first_run_id=$1
WHERE v.run_id=$1 ORDER BY v.result_id,v.id DESC
) latest GROUP BY stage,code ORDER BY count(*) DESC,stage,code`, run.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var item ProxyVerificationBreakdown
		if err := rows.Scan(&item.Stage, &item.Code, &item.Count); err != nil {
			return nil, err
		}
		summary.Attempted += item.Count
		if item.Code == "ok" {
			summary.Succeeded += item.Count
		} else {
			summary.Failures = append(summary.Failures, item)
		}
	}
	return summary, rows.Err()
}

// CompleteProxyDiscoveryVerification commits the candidate state and the
// evidence row together, so a failed diagnostic write cannot claim success.
func (d *DB) CompleteProxyDiscoveryVerification(id int64, state string, authorized bool, ip string, latencyMS int, problem, mode, targetOrigin, outcome, stage, code string, httpStatus int) error {
	tx, err := d.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.Exec(`UPDATE proxy_discovery_results SET state=$2,authorized=$3,verified_ip=$4,
verified_at=CASE WHEN $4<>'' THEN now() ELSE NULL END,latency_ms=$5,last_error=$6,updated_at=now(),
last_verify_mode=$7,last_verify_target=$8,last_http_status=$9,last_checked_at=now()
WHERE id=$1 AND state='verifying'`, id, state, authorized, ip, latencyMS, problem, mode, targetOrigin, httpStatus)
	if err != nil {
		return err
	}
	if n, err := result.RowsAffected(); err != nil || n != 1 {
		return errors.New("candidate changed during verification")
	}
	_, err = tx.Exec(`INSERT INTO proxy_discovery_verifications(result_id,run_id,mode,target_origin,outcome,stage,code,latency_ms,exit_ip,http_status)
SELECT id,first_run_id,$2,$3,$4,$5,$6,$7,$8,$9 FROM proxy_discovery_results WHERE id=$1`,
		id, mode, targetOrigin, outcome, stage, code, latencyMS, ip, httpStatus)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (d *DB) SetProxyDiscoveryProtocol(id int64, protocol string) error {
	if protocol != "http" && protocol != "https" && protocol != "socks5" {
		return errors.New("invalid proxy protocol")
	}
	result, err := d.Exec(`UPDATE proxy_discovery_results SET protocol=$2,
address=$2 || '://' || CASE WHEN strpos(host,':')>0 THEN '[' || host || ']' ELSE host END || ':' || port::text,
state='pending',verified_ip='',verified_at=NULL,latency_ms=NULL,last_error='',updated_at=now()
WHERE id=$1 AND state IN ('pending','invalid') AND protocol='unknown'`, id, protocol)
	if err != nil {
		return err
	}
	if n, err := result.RowsAffected(); err != nil || n != 1 {
		return errors.New("candidate is no longer awaiting protocol confirmation")
	}
	return nil
}
