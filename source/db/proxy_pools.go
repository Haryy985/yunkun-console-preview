package db

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strconv"
	"time"

	"yunkun/console/proxypool"
)

type ProxyPool struct {
	ID                   int64            `json:"id"`
	Config               proxypool.Config `json:"config"`
	Credentials          string           `json:"-"`
	HasCredentials       bool             `json:"has_credentials"`
	CredentialsAvailable *bool            `json:"credentials_available,omitempty"`
	Quota                proxypool.Quota  `json:"quota"`
}

var ErrProxyPoolInUse = errors.New("proxy pool is bound to tasks")
var ErrProxyPoolCredentialsChanged = errors.New("proxy pool credentials changed during verification")

var (
	ErrProxyPoolNotFound             = errors.New("proxy pool does not exist")
	ErrProxyDiscoveryNotFound        = errors.New("discovery result does not exist")
	ErrProxyDiscoveryNotVerified     = errors.New("discovery result has no successful verification")
	ErrProxyDiscoveryAlreadyApproved = errors.New("discovery result is already approved")
	ErrProxyDiscoveryOutOfScope      = errors.New("discovery result is outside the authorized scope")
)

type TaskProxyBinding struct {
	Strategy  string     `json:"strategy"`
	Revision  int64      `json:"revision"`
	TaskID    int64      `json:"task_id"`
	PoolID    int64      `json:"pool_id"`
	NodeID    *int64     `json:"node_id,omitempty"`
	Mode      string     `json:"mode"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

type ProxyRouteEvent struct {
	ID        int64     `json:"id"`
	TaskID    *int64    `json:"task_id,omitempty"`
	PoolID    *int64    `json:"pool_id,omitempty"`
	NodeID    *int64    `json:"node_id,omitempty"`
	Target    string    `json:"target"`
	Result    string    `json:"result"`
	TrafficID string    `json:"traffic_id,omitempty"`
	Revision  int64     `json:"revision,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

type ProxyNodeSummary struct {
	Total     int `json:"total"`
	Available int `json:"available"`
	Pending   int `json:"pending"`
	Expiring  int `json:"expiring"`
	Error     int `json:"error"`
	Disabled  int `json:"disabled"`
}

type ProxyDiscoverySource struct {
	ID              int64      `json:"id"`
	Provider        string     `json:"provider"`
	Name            string     `json:"name"`
	Query           string     `json:"query"`
	AuthorizedScope string     `json:"authorized_scope"`
	VerifyURL       string     `json:"verify_url,omitempty"`
	FetchLimit      int        `json:"fetch_limit"`
	NextOffset      int        `json:"next_offset"`
	Exhausted       bool       `json:"exhausted"`
	Enabled         bool       `json:"enabled"`
	HasCredentials  bool       `json:"has_credentials"`
	LastRunAt       *time.Time `json:"last_run_at,omitempty"`
	LastError       string     `json:"last_error,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

type ProxyDiscoveryResult struct {
	ID                  int64          `json:"id"`
	SourceID            int64          `json:"source_id"`
	Address             string         `json:"address"`
	Protocol            string         `json:"protocol"`
	Host                string         `json:"host"`
	Port                int            `json:"port"`
	Metadata            map[string]any `json:"metadata"`
	State               string         `json:"state"`
	Authorized          bool           `json:"authorized"`
	AuthorizationStatus string         `json:"authorization_status,omitempty"`
	VerifiedIP          string         `json:"verified_ip,omitempty"`
	VerifiedAt          *time.Time     `json:"verified_at,omitempty"`
	LastVerifyMode      string         `json:"last_verify_mode,omitempty"`
	LastVerifyTarget    string         `json:"last_verify_target,omitempty"`
	LastHTTPStatus      int            `json:"last_http_status,omitempty"`
	LastCheckedAt       *time.Time     `json:"last_checked_at,omitempty"`
	LatencyMS           *int           `json:"latency_ms,omitempty"`
	LastError           string         `json:"last_error,omitempty"`
	CreatedAt           time.Time      `json:"created_at"`
	UpdatedAt           time.Time      `json:"updated_at"`
}

func (r *ProxyDiscoveryResult) HasSuccessfulVerification() bool {
	if r == nil || r.State != "verified" || r.LastError != "" {
		return false
	}
	if r.LastVerifyMode == proxypool.VerifyConnectivity {
		return r.LastCheckedAt != nil && r.LastHTTPStatus == 200
	}
	return (r.LastVerifyMode == "" || r.LastVerifyMode == proxypool.VerifyExitIP) &&
		net.ParseIP(r.VerifiedIP) != nil && r.VerifiedAt != nil
}

func scanProxyPool(row interface{ Scan(...any) error }) (*ProxyPool, error) {
	var p ProxyPool
	var config, quota []byte
	if err := row.Scan(&p.ID, &config, &p.Credentials, &quota); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(config, &p.Config); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(quota, &p.Quota); err != nil {
		return nil, err
	}
	return &p, nil
}

func (d *DB) ProxyPool(id int64) (*ProxyPool, error) {
	p, err := scanProxyPool(d.QueryRow(`SELECT id,config,credentials,quota FROM proxy_pools WHERE id=$1`, id))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return p, err
}

func (d *DB) ListProxyPools() ([]*ProxyPool, error) {
	rows, err := d.Query(`SELECT id,config,credentials,quota FROM proxy_pools ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*ProxyPool{}
	for rows.Next() {
		p, err := scanProxyPool(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (d *DB) SaveProxyPool(id int64, c proxypool.Config, credentials string, requireUnused ...bool) (int64, error) {
	raw, err := json.Marshal(c)
	if err != nil {
		return 0, err
	}
	tx, err := d.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	if id == 0 {
		err = tx.QueryRow(`INSERT INTO proxy_pools(config,credentials) VALUES($1,$2) RETURNING id`, string(raw), credentials).Scan(&id)
	} else {
		var previousRaw []byte
		var previousCredentials string
		if err = tx.QueryRow(`SELECT config,credentials FROM proxy_pools WHERE id=$1 FOR UPDATE`, id).Scan(&previousRaw, &previousCredentials); err != nil {
			return 0, err
		}
		// Metadata-only saves must not rotate ciphertext or overwrite a
		// concurrently recovered credential with a stale copy.
		if len(requireUnused) == 0 || !requireUnused[0] {
			credentials = previousCredentials
		}
		if len(requireUnused) > 0 && requireUnused[0] {
			var bound int
			if err = tx.QueryRow(`SELECT count(*) FROM task_proxy_bindings WHERE pool_id=$1`, id).Scan(&bound); err != nil {
				return 0, err
			}
			if bound > 0 {
				return 0, ErrProxyPoolInUse
			}
			var previous proxypool.Config
			if err = json.Unmarshal(previousRaw, &previous); err != nil {
				return 0, err
			}
			if previous.Kind != c.Kind || previous.Protocol != c.Protocol || previous.APIURL != c.APIURL ||
				!sameProxyAddresses(previous.Addresses, c.Addresses) {
				if _, err = tx.Exec(`DELETE FROM proxy_nodes WHERE pool_id=$1`, id); err != nil {
					return 0, err
				}
			} else {
				// Credential replacement keeps imported/discovered inventory, but
				// evidence acquired using the old credentials cannot authorize use.
				if _, err = tx.Exec(`UPDATE proxy_nodes SET enabled=false,
verified_ip='',verified_at=NULL,last_success_at=NULL,last_checked_at=NULL,
last_verify_mode='',last_verify_target='',last_http_status=0,latency_ms=NULL,
last_error='proxy credentials changed; reverify node',
import_verify_pending=false,import_verify_claimed_at=NULL
WHERE pool_id=$1 AND deleted_at IS NULL`, id); err != nil {
					return 0, err
				}
			}
			if _, err = tx.Exec(`UPDATE proxy_pools SET quota='{}' WHERE id=$1`, id); err != nil {
				return 0, err
			}
		}
		err = tx.QueryRow(`UPDATE proxy_pools SET config=$2,credentials=$3,updated_at=now() WHERE id=$1 RETURNING id`, id, string(raw), credentials).Scan(&id)
	}
	if err != nil {
		return 0, err
	}
	if c.Kind == "fixed" {
		for _, address := range c.Addresses {
			if _, err = tx.Exec(`INSERT INTO proxy_nodes(pool_id,address) VALUES($1,$2) ON CONFLICT(pool_id,address) DO NOTHING`, id, address); err != nil {
				return 0, err
			}
		}
	}
	return id, tx.Commit()
}

func sameProxyAddresses(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func (d *DB) DeleteProxyPool(id int64) error {
	res, err := d.Exec(`DELETE FROM proxy_pools WHERE id=$1 AND NOT EXISTS (SELECT 1 FROM task_proxy_bindings WHERE pool_id=$1)`, id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err == nil && n == 0 {
		return errors.New("pool does not exist or is still bound to a task")
	}
	return err
}

func (d *DB) StoreProxyQuota(id int64, q proxypool.Quota) error {
	raw, err := json.Marshal(q)
	if err != nil {
		return err
	}
	_, err = d.Exec(`UPDATE proxy_pools SET quota=$2 WHERE id=$1`, id, string(raw))
	return err
}

func (d *DB) ClaimProxyProviderCall(key string, seconds int) (bool, error) {
	var out string
	err := d.QueryRow(`INSERT INTO proxy_provider_limits(account_hash,next_at)
VALUES($1,now()+make_interval(secs=>$2)) ON CONFLICT(account_hash) DO UPDATE
SET next_at=EXCLUDED.next_at WHERE proxy_provider_limits.next_at<=now() RETURNING account_hash`, key, seconds).Scan(&out)
	if err == sql.ErrNoRows {
		return false, nil
	}
	return err == nil, err
}

func (d *DB) StoreProxyNodes(poolID int64, nodes []proxypool.Node) error {
	tx, err := d.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, n := range nodes {
		// Lease expiry is snapshotted on the binding, so refreshing inventory
		// never silently extends an existing task's lease.
		_, err = tx.Exec(`INSERT INTO proxy_nodes(pool_id,address,declared_ip,expires_at,origin) VALUES($1,$2,$3,$4,'provider')
ON CONFLICT(pool_id,address) DO UPDATE SET declared_ip=EXCLUDED.declared_ip,expires_at=EXCLUDED.expires_at,
verified_ip='',verified_at=NULL,last_error='',last_checked_at=NULL,latency_ms=NULL,
last_verify_mode='',last_verify_target='',last_http_status=0`,
			poolID, n.Address, n.DeclaredIP, n.ExpiresAt)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (d *DB) ImportProxyNodes(poolID int64, nodes []proxypool.Node) ([]int64, error) {
	tx, err := d.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	ids := make([]int64, 0, len(nodes))
	for _, node := range nodes {
		var id int64
		err := tx.QueryRow(`INSERT INTO proxy_nodes(pool_id,address,origin,enabled,import_verify_pending)
VALUES($1,$2,'manual',false,true) ON CONFLICT(pool_id,address) DO NOTHING
RETURNING id`, poolID, node.Address).Scan(&id)
		if err == sql.ErrNoRows {
			continue
		}
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return ids, nil
}

func (d *DB) ProxyNodesForExport(poolID int64) ([]proxypool.Node, error) {
	rows, err := d.Query(`SELECT id,pool_id,address,enabled,last_error,latency_ms,
last_verify_mode,last_verify_target,last_http_status
FROM proxy_nodes WHERE pool_id=$1 AND deleted_at IS NULL ORDER BY id`, poolID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]proxypool.Node, 0)
	for rows.Next() {
		var node proxypool.Node
		if err := rows.Scan(&node.ID, &node.PoolID, &node.Address, &node.Enabled, &node.LastError,
			&node.LatencyMS, &node.LastVerifyMode, &node.LastVerifyTarget, &node.LastHTTPStatus); err != nil {
			return nil, err
		}
		out = append(out, node)
	}
	return out, rows.Err()
}

func (d *DB) ProxyNodes(poolID int64) ([]proxypool.Node, error) {
	rows, err := d.Query(`SELECT id,pool_id,address,declared_ip,expires_at,verified_ip,verified_at,last_error,
enabled,origin,last_checked_at,latency_ms,success_count,failure_count,last_success_at,last_failure_at,notes,deleted_at,
last_verify_mode,last_verify_target,last_http_status,runtime_cooldown_until,runtime_error
FROM proxy_nodes WHERE pool_id=$1 AND deleted_at IS NULL ORDER BY id DESC LIMIT 500`, poolID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []proxypool.Node{}
	for rows.Next() {
		var n proxypool.Node
		if err := rows.Scan(&n.ID, &n.PoolID, &n.Address, &n.DeclaredIP, &n.ExpiresAt, &n.VerifiedIP, &n.VerifiedAt, &n.LastError,
			&n.Enabled, &n.Origin, &n.LastCheckedAt, &n.LatencyMS, &n.SuccessCount, &n.FailureCount,
			&n.LastSuccessAt, &n.LastFailureAt, &n.Notes, &n.DeletedAt,
			&n.LastVerifyMode, &n.LastVerifyTarget, &n.LastHTTPStatus, &n.RuntimeCooldownUntil, &n.RuntimeError); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

func (d *DB) ProxyNode(poolID, nodeID int64) (*proxypool.Node, error) {
	var n proxypool.Node
	err := d.QueryRow(`SELECT id,pool_id,address,declared_ip,expires_at,verified_ip,verified_at,last_error,
enabled,origin,last_checked_at,latency_ms,success_count,failure_count,last_success_at,last_failure_at,notes,deleted_at,
last_verify_mode,last_verify_target,last_http_status,runtime_cooldown_until,runtime_error
FROM proxy_nodes WHERE pool_id=$1 AND id=$2 AND deleted_at IS NULL`, poolID, nodeID).
		Scan(&n.ID, &n.PoolID, &n.Address, &n.DeclaredIP, &n.ExpiresAt, &n.VerifiedIP, &n.VerifiedAt, &n.LastError,
			&n.Enabled, &n.Origin, &n.LastCheckedAt, &n.LatencyMS, &n.SuccessCount, &n.FailureCount,
			&n.LastSuccessAt, &n.LastFailureAt, &n.Notes, &n.DeletedAt,
			&n.LastVerifyMode, &n.LastVerifyTarget, &n.LastHTTPStatus, &n.RuntimeCooldownUntil, &n.RuntimeError)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &n, err
}

func (d *DB) ReassignProxyLease(taskID int64) error {
	tx, err := d.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var allowed bool
	if err := tx.QueryRow(`SELECT deleted_at IS NULL AND (paused OR queued OR status IN ('done','failed','timeout')) FROM tasks WHERE id=$1 FOR UPDATE`, taskID).Scan(&allowed); err != nil {
		return err
	}
	if !allowed {
		return errors.New("pause task before reassigning its proxy lease")
	}
	var poolID int64
	if err := tx.QueryRow(`SELECT pool_id FROM task_proxy_bindings WHERE task_id=$1 FOR UPDATE`, taskID).Scan(&poolID); err != nil {
		return errors.New("task has no proxy binding")
	}
	p, err := scanProxyPool(tx.QueryRow(`SELECT id,config,credentials,quota FROM proxy_pools WHERE id=$1 FOR SHARE`, poolID))
	if err != nil || !p.Config.Enabled {
		return errors.New("proxy pool unavailable")
	}
	var nodeID int64
	if err := tx.QueryRow(`SELECT id FROM proxy_nodes WHERE pool_id=$1 AND enabled=true AND deleted_at IS NULL AND last_error=''
AND (runtime_cooldown_until IS NULL OR runtime_cooldown_until<=now())
AND (expires_at IS NULL OR expires_at>now()+make_interval(secs=>$2))
AND (origin NOT IN ('fofa','quake') OR (((last_verify_mode='connectivity' AND last_http_status=200 AND last_success_at>now()-interval '15 minutes')
OR (last_verify_mode IN ('','exit_ip') AND verified_at>now()-interval '15 minutes' AND verified_ip<>''))
AND last_error='' AND (last_failure_at IS NULL OR last_success_at>=last_failure_at)))
ORDER BY (SELECT count(*) FROM task_proxy_bindings b WHERE b.node_id=proxy_nodes.id),id DESC LIMIT 1`, poolID, p.Config.SafetySeconds).Scan(&nodeID); err != nil {
		return errors.New("no valid nodes; fetch nodes first")
	}
	if _, err := tx.Exec(`UPDATE task_proxy_bindings SET node_id=$2,expires_at=(SELECT expires_at FROM proxy_nodes WHERE id=$2),revision=revision+1 WHERE task_id=$1`, taskID, nodeID); err != nil {
		return err
	}
	if _, err := tx.Exec(`INSERT INTO proxy_route_events(task_id,pool_id,node_id,target,result) VALUES($1,$2,$3,'','lease_reassigned')`, taskID, poolID, nodeID); err != nil {
		return err
	}
	return tx.Commit()
}

func (d *DB) VerifyProxyNode(id int64, ip, problem string) error {
	return d.VerifyProxyNodeWithLatency(id, ip, problem, nil)
}

func (d *DB) VerifyProxyNodeWithLatency(id int64, ip, problem string, latencyMS *int) error {
	if problem != "" {
		_, err := d.Exec(`UPDATE proxy_nodes SET last_error=$2,last_checked_at=now(),last_failure_at=now(),
failure_count=failure_count+1,latency_ms=$3 WHERE id=$1`, id, problem, latencyMS)
		return err
	}
	_, err := d.Exec(`UPDATE proxy_nodes SET verified_ip=$2,verified_at=now(),last_checked_at=now(),
last_success_at=now(),success_count=success_count+1,last_error='',latency_ms=$3 WHERE id=$1`,
		id, ip, latencyMS)
	return err
}

// Connectivity checks update health without inventing or erasing IP evidence.
func (d *DB) RecordProxyNodeVerification(poolID, nodeID int64, result proxypool.VerificationResult, problem string, latencyMS int, expectedCredentials ...string) error {
	exec := d.Exec
	var tx *sql.Tx
	if len(expectedCredentials) > 0 {
		var err error
		tx, err = d.Begin()
		if err != nil {
			return err
		}
		defer tx.Rollback()
		// Use the same pool-before-node lock order as SaveProxyPool so an old
		// in-flight verification cannot restore evidence after key replacement.
		var current string
		if err := tx.QueryRow(`SELECT credentials FROM proxy_pools WHERE id=$1 FOR SHARE`, poolID).Scan(&current); err != nil {
			return err
		}
		if current != expectedCredentials[0] {
			return ErrProxyPoolCredentialsChanged
		}
		exec = tx.Exec
	}
	res, err := exec(`UPDATE proxy_nodes SET
last_verify_mode=$3,last_verify_target=$4,last_http_status=$5,
last_checked_at=now(),latency_ms=$6,last_error=$7,
runtime_cooldown_until=CASE WHEN $7='' THEN NULL ELSE runtime_cooldown_until END,
runtime_error=CASE WHEN $7='' THEN '' ELSE runtime_error END,
last_success_at=CASE WHEN $7='' THEN now() ELSE last_success_at END,
last_failure_at=CASE WHEN $7<>'' THEN now() ELSE last_failure_at END,
success_count=success_count+CASE WHEN $7='' THEN 1 ELSE 0 END,
failure_count=failure_count+CASE WHEN $7<>'' THEN 1 ELSE 0 END,
verified_ip=CASE WHEN $7='' AND $8<>'' THEN $8 ELSE verified_ip END,
verified_at=CASE WHEN $7='' AND $8<>'' THEN now() ELSE verified_at END
WHERE pool_id=$1 AND id=$2 AND deleted_at IS NULL`,
		poolID, nodeID, result.Mode, result.Target, result.HTTPStatus, latencyMS, problem, result.IP)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	if tx != nil {
		return tx.Commit()
	}
	return nil
}

func (d *DB) SetProxyNodeEnabled(poolID, nodeID int64, enabled bool) error {
	res, err := d.Exec(`UPDATE proxy_nodes SET enabled=$3,import_verify_pending=false WHERE pool_id=$1 AND id=$2 AND deleted_at IS NULL`,
		poolID, nodeID, enabled)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (d *DB) SoftDeleteProxyNode(poolID, nodeID int64) error {
	res, err := d.Exec(`UPDATE proxy_nodes SET enabled=false,import_verify_pending=false,deleted_at=now()
WHERE pool_id=$1 AND id=$2 AND deleted_at IS NULL
AND NOT EXISTS (SELECT 1 FROM task_proxy_bindings WHERE node_id=$2)`, poolID, nodeID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return errors.New("node does not exist or is bound to a task")
	}
	return nil
}

func (d *DB) BatchSetProxyNodes(poolID int64, nodeIDs []int64, enabled bool) (int64, error) {
	if len(nodeIDs) == 0 {
		return 0, nil
	}
	tx, err := d.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	res, err := tx.Exec(`UPDATE proxy_nodes SET enabled=$3,import_verify_pending=false
WHERE pool_id=$1 AND id=ANY($2) AND deleted_at IS NULL
`, poolID, nodeIDs, enabled)
	if err != nil {
		return 0, err
	}
	changed, _ := res.RowsAffected()
	return changed, tx.Commit()
}

func (d *DB) ProxyNodeSummary(poolID int64, warning time.Duration) (ProxyNodeSummary, error) {
	var out ProxyNodeSummary
	interval := fmt.Sprintf("%d seconds", int(warning.Seconds()))
	err := d.QueryRow(`SELECT
count(*) FILTER (WHERE deleted_at IS NULL),
count(*) FILTER (WHERE deleted_at IS NULL AND enabled=true AND last_success_at IS NOT NULL
  AND (expires_at IS NULL OR expires_at>now()+$2::interval) AND last_error=''
  AND (runtime_cooldown_until IS NULL OR runtime_cooldown_until<=now())),
count(*) FILTER (WHERE deleted_at IS NULL AND enabled=true AND last_error=''
  AND last_success_at IS NULL AND (expires_at IS NULL OR expires_at>now()+$2::interval)
  AND (runtime_cooldown_until IS NULL OR runtime_cooldown_until<=now())),
count(*) FILTER (WHERE deleted_at IS NULL AND enabled=true AND expires_at IS NOT NULL
  AND expires_at<=now()+$2::interval AND expires_at>now()),
count(*) FILTER (WHERE deleted_at IS NULL AND enabled=true AND
  (last_error<>'' OR runtime_cooldown_until>now() OR (expires_at IS NOT NULL AND expires_at<=now()))),
count(*) FILTER (WHERE deleted_at IS NULL AND enabled=false)
FROM proxy_nodes WHERE pool_id=$1`, poolID, interval).
		Scan(&out.Total, &out.Available, &out.Pending, &out.Expiring, &out.Error, &out.Disabled)
	return out, err
}

func (d *DB) ListProxyDiscoverySources() ([]ProxyDiscoverySource, error) {
	rows, err := d.Query(`SELECT id,provider,name,query,authorized_scope,verify_url,fetch_limit,next_offset,exhausted,enabled,
credentials<>'',last_run_at,last_error,created_at,updated_at
FROM proxy_discovery_sources ORDER BY id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ProxyDiscoverySource{}
	for rows.Next() {
		var item ProxyDiscoverySource
		if err := rows.Scan(&item.ID, &item.Provider, &item.Name, &item.Query, &item.AuthorizedScope,
			&item.VerifyURL, &item.FetchLimit, &item.NextOffset, &item.Exhausted, &item.Enabled, &item.HasCredentials, &item.LastRunAt, &item.LastError, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (d *DB) ProxyDiscoverySource(id int64) (*ProxyDiscoverySource, string, error) {
	var item ProxyDiscoverySource
	var credentials string
	err := d.QueryRow(`SELECT id,provider,name,query,authorized_scope,verify_url,fetch_limit,next_offset,exhausted,enabled,credentials,
last_run_at,last_error,created_at,updated_at FROM proxy_discovery_sources WHERE id=$1`, id).
		Scan(&item.ID, &item.Provider, &item.Name, &item.Query, &item.AuthorizedScope, &item.VerifyURL, &item.FetchLimit, &item.NextOffset, &item.Exhausted, &item.Enabled,
			&credentials, &item.LastRunAt, &item.LastError, &item.CreatedAt, &item.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, "", nil
	}
	return &item, credentials, err
}

func (d *DB) SaveProxyDiscoverySource(id int64, provider, name, query, scope, verifyURL, credentials string, fetchLimit int) (int64, error) {
	var out int64
	if id == 0 {
		err := d.QueryRow(`INSERT INTO proxy_discovery_sources(provider,name,query,authorized_scope,verify_url,fetch_limit,credentials)
VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING id`, provider, name, query, scope, verifyURL, fetchLimit, credentials).Scan(&out)
		return out, err
	}
	tx, err := d.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	// A changed verification target or scope requires fresh evidence before approval.
	if _, err = tx.Exec(`UPDATE proxy_discovery_results r SET state='pending',authorized=false,
verified_ip='',verified_at=NULL,latency_ms=NULL,last_error='',updated_at=now()
FROM proxy_discovery_sources s WHERE r.source_id=s.id AND s.id=$1
AND r.state NOT IN ('approved','ignored')
AND (s.authorized_scope<>$2 OR s.verify_url<>$3 OR s.credentials<>$4)`,
		id, scope, verifyURL, credentials); err != nil {
		return 0, err
	}
	err = tx.QueryRow(`UPDATE proxy_discovery_sources SET provider=$2,name=$3,query=$4,
authorized_scope=$5,verify_url=$6,fetch_limit=$7,credentials=$8,
next_offset=CASE WHEN provider<>$2 OR query<>$4 OR fetch_limit<>$7 OR credentials<>$8 THEN 0 ELSE next_offset END,
exhausted=CASE WHEN provider<>$2 OR query<>$4 OR fetch_limit<>$7 OR credentials<>$8 THEN false ELSE exhausted END,
updated_at=now() WHERE id=$1 RETURNING id`,
		id, provider, name, query, scope, verifyURL, fetchLimit, credentials).Scan(&out)
	if err != nil {
		return 0, err
	}
	return out, tx.Commit()
}

func (d *DB) DeleteProxyDiscoverySource(id int64) error {
	res, err := d.Exec(`DELETE FROM proxy_discovery_sources WHERE id=$1`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (d *DB) MarkProxyDiscoveryRun(id int64, lastError string) error {
	_, err := d.Exec(`UPDATE proxy_discovery_sources SET last_run_at=now(),last_error=$2,updated_at=now() WHERE id=$1`, id, lastError)
	return err
}

// StoreProxyDiscoveryRun commits candidates and the provider cursor together.
func (d *DB) StoreProxyDiscoveryRun(sourceID int64, results []ProxyDiscoveryResult, nextOffset int, exhausted bool) error {
	_, err := d.StoreProxyDiscoveryRunWithIDs(sourceID, results, nextOffset, exhausted)
	return err
}

// StoreProxyDiscoveryRunWithIDs is the same atomic write as StoreProxyDiscoveryRun,
// and also returns IDs that were newly inserted during this run. Existing
// candidates are refreshed without being re-queued for automatic verification.
func (d *DB) StoreProxyDiscoveryRunWithIDs(sourceID int64, results []ProxyDiscoveryResult, nextOffset int, exhausted bool) ([]int64, error) {
	var current, requested int
	if err := d.QueryRow(`SELECT next_offset,fetch_limit FROM proxy_discovery_sources WHERE id=$1`, sourceID).Scan(&current, &requested); err != nil {
		return nil, err
	}
	ids, _, err := d.StoreProxyDiscoveryRunWithMetrics(sourceID, results, current, requested, nextOffset-current, nextOffset, exhausted)
	return ids, err
}

// StoreProxyDiscoveryRunWithMetrics stores a fetched page, its candidates and
// the cursor in one transaction; a failed write never skips a provider page.
func (d *DB) StoreProxyDiscoveryRunWithMetrics(sourceID int64, results []ProxyDiscoveryResult, offsetStart, requested, returned, nextOffset int, exhausted bool) ([]int64, int64, error) {
	tx, err := d.Begin()
	if err != nil {
		return nil, 0, err
	}
	defer tx.Rollback()
	var current int
	if err := tx.QueryRow(`SELECT next_offset FROM proxy_discovery_sources WHERE id=$1 FOR UPDATE`, sourceID).Scan(&current); err != nil {
		return nil, 0, err
	}
	if offsetStart != current || requested < 1 || returned < 0 || returned > requested || nextOffset-current != returned {
		return nil, 0, errors.New("discovery cursor changed; retry the current page")
	}
	var runID int64
	if err := tx.QueryRow(`INSERT INTO proxy_discovery_runs(source_id,offset_start,requested,returned,parsed,inserted,authorized,unknown_protocol)
VALUES($1,$2,$3,$4,$5,0,0,0) RETURNING id`, sourceID, offsetStart, requested, returned, len(results)).Scan(&runID); err != nil {
		return nil, 0, err
	}
	newIDs := make([]int64, 0, len(results))
	var authorized, unknown int
	seen := make(map[string]bool, len(results))
	for _, item := range results {
		raw, err := json.Marshal(item.Metadata)
		if err != nil {
			return nil, 0, err
		}
		if !seen[item.Address] {
			seen[item.Address] = true
			if item.Authorized {
				authorized++
			}
			if item.Protocol == "unknown" {
				unknown++
			}
		}
		var id int64
		var inserted bool
		err = tx.QueryRow(`WITH existing AS MATERIALIZED (
SELECT id FROM proxy_discovery_results WHERE source_id=$1 AND address=$2
), upserted AS (
INSERT INTO proxy_discovery_results(source_id,address,protocol,host,port,metadata,authorized,last_run_id,first_run_id)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$8)
ON CONFLICT(source_id,address) DO UPDATE SET protocol=EXCLUDED.protocol,host=EXCLUDED.host,
port=EXCLUDED.port,metadata=EXCLUDED.metadata,authorized=EXCLUDED.authorized,last_run_id=EXCLUDED.last_run_id,updated_at=now()
RETURNING id
)
SELECT upserted.id, NOT EXISTS (SELECT 1 FROM existing) FROM upserted`,
			sourceID, item.Address, item.Protocol, item.Host, item.Port, string(raw), item.Authorized, runID).Scan(&id, &inserted)
		if err != nil {
			return nil, 0, err
		}
		if inserted {
			newIDs = append(newIDs, id)
		}
	}
	if _, err = tx.Exec(`UPDATE proxy_discovery_runs SET inserted=$2,authorized=$3,unknown_protocol=$4 WHERE id=$1`, runID, len(newIDs), authorized, unknown); err != nil {
		return nil, 0, err
	}
	result, err := tx.Exec(`UPDATE proxy_discovery_sources SET next_offset=$2,exhausted=$3,
last_run_at=now(),last_error='',updated_at=now() WHERE id=$1 AND next_offset=$4`, sourceID, nextOffset, exhausted, current)
	if err != nil {
		return nil, 0, err
	}
	if n, err := result.RowsAffected(); err != nil || n != 1 {
		return nil, 0, errors.New("discovery cursor changed; retry the current page")
	}
	if err := tx.Commit(); err != nil {
		return nil, 0, err
	}
	return newIDs, runID, nil
}

func (d *DB) UpsertProxyDiscoveryResults(sourceID int64, results []ProxyDiscoveryResult) error {
	tx, err := d.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, item := range results {
		raw, err := json.Marshal(item.Metadata)
		if err != nil {
			return err
		}
		_, err = tx.Exec(`INSERT INTO proxy_discovery_results(source_id,address,protocol,host,port,metadata,authorized)
VALUES($1,$2,$3,$4,$5,$6,$7)
ON CONFLICT(source_id,address) DO UPDATE SET protocol=EXCLUDED.protocol,host=EXCLUDED.host,
port=EXCLUDED.port,metadata=EXCLUDED.metadata,authorized=EXCLUDED.authorized,updated_at=now()`,
			sourceID, item.Address, item.Protocol, item.Host, item.Port, string(raw), item.Authorized)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

// RepairQuakeDiscoveryProtocols preserves candidate IDs and requires fresh
// verification after correcting the old HTTP default. Existing target addresses
// are left intact rather than deleting or merging independent review records.
func (d *DB) RepairQuakeDiscoveryProtocols() (int64, error) {
	result, err := d.Exec(`UPDATE proxy_discovery_results AS r
SET protocol='socks5',
address='socks5://' || CASE WHEN strpos(r.host, ':')>0 THEN '[' || r.host || ']' ELSE r.host END || ':' || r.port::text,
state='pending',verified_ip='',verified_at=NULL,latency_ms=NULL,last_error='',updated_at=now()
FROM proxy_discovery_sources AS s
WHERE s.id=r.source_id AND s.provider='quake' AND r.protocol='http'
AND r.state IN ('pending','verifying','verified','invalid')
AND lower(btrim(r.metadata #>> '{service,name}')) IN ('socks5','socks5h')
AND NOT EXISTS (
SELECT 1 FROM proxy_discovery_results AS existing
WHERE existing.source_id=r.source_id
AND existing.address='socks5://' || CASE WHEN strpos(r.host, ':')>0 THEN '[' || r.host || ']' ELSE r.host END || ':' || r.port::text
)`)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (d *DB) ListProxyDiscoveryResults(sourceID int64) ([]ProxyDiscoveryResult, error) {
	return d.ProxyDiscoveryResultsPage(sourceID, 1000, 0)
}

type ProxyDiscoverySummary struct {
	Total      int `json:"total"`
	Unapproved int `json:"unapproved"`
	Verified   int `json:"verified"`
	Approved   int `json:"approved"`
	Pending    int `json:"pending"`
	Invalid    int `json:"invalid"`
	Ignored    int `json:"ignored"`
}

func (d *DB) ProxyDiscoveryResultSummary(sourceID int64) (ProxyDiscoverySummary, error) {
	var out ProxyDiscoverySummary
	err := d.QueryRow(`SELECT count(*),
count(*) FILTER (WHERE state NOT IN ('approved','ignored')),
count(*) FILTER (WHERE state='verified'),count(*) FILTER (WHERE state='approved'),
count(*) FILTER (WHERE state='pending'),count(*) FILTER (WHERE state='invalid'),
count(*) FILTER (WHERE state='ignored')
FROM proxy_discovery_results WHERE ($1::bigint=0 OR source_id=$1)`, sourceID).
		Scan(&out.Total, &out.Unapproved, &out.Verified, &out.Approved, &out.Pending, &out.Invalid, &out.Ignored)
	return out, err
}

const proxyDiscoveryResultColumns = `id,source_id,address,protocol,host,port,metadata,state,authorized,
verified_ip,verified_at,latency_ms,last_error,created_at,updated_at,
last_verify_mode,last_verify_target,last_http_status,last_checked_at`

func scanProxyDiscoveryResult(row interface{ Scan(...any) error }) (*ProxyDiscoveryResult, error) {
	var item ProxyDiscoveryResult
	var raw []byte
	err := row.Scan(&item.ID, &item.SourceID, &item.Address, &item.Protocol, &item.Host, &item.Port,
		&raw, &item.State, &item.Authorized, &item.VerifiedIP, &item.VerifiedAt, &item.LatencyMS,
		&item.LastError, &item.CreatedAt, &item.UpdatedAt,
		&item.LastVerifyMode, &item.LastVerifyTarget, &item.LastHTTPStatus, &item.LastCheckedAt)
	if err != nil {
		return nil, err
	}
	_ = json.Unmarshal(raw, &item.Metadata)
	if item.Metadata == nil {
		item.Metadata = map[string]any{}
	}
	return &item, nil
}

func (d *DB) ProxyDiscoveryResult(id int64) (*ProxyDiscoveryResult, error) {
	item, err := scanProxyDiscoveryResult(d.QueryRow(`SELECT `+proxyDiscoveryResultColumns+`
FROM proxy_discovery_results WHERE id=$1`, id))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return item, err
}

func (d *DB) ProxyDiscoveryResultsPage(sourceID int64, limit, offset int) ([]ProxyDiscoveryResult, error) {
	return d.FilteredProxyDiscoveryResultsPage(sourceID, "", limit, offset)
}

func (d *DB) ProxyDiscoveryResultCount(sourceID int64, state string) (int, error) {
	var total int
	err := d.QueryRow(`SELECT count(*) FROM proxy_discovery_results
WHERE ($1::bigint=0 OR source_id=$1) AND ($2::text='' OR state=$2)`, sourceID, state).Scan(&total)
	return total, err
}

func (d *DB) FilteredProxyDiscoverySelectableIDs(sourceID int64, state string) ([]int64, error) {
	rows, err := d.Query(`SELECT id FROM proxy_discovery_results
WHERE source_id=$1 AND state<>'approved' AND ($2::text='' OR state=$2) ORDER BY id DESC`, sourceID, state)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := []int64{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (d *DB) FilteredProxyDiscoveryResultsPage(sourceID int64, state string, limit, offset int) ([]ProxyDiscoveryResult, error) {
	rows, err := d.Query(`SELECT `+proxyDiscoveryResultColumns+`
FROM proxy_discovery_results WHERE ($1::bigint=0 OR source_id=$1) AND ($2::text='' OR state=$2)
ORDER BY id DESC LIMIT $3 OFFSET $4`, sourceID, state, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ProxyDiscoveryResult{}
	for rows.Next() {
		item, err := scanProxyDiscoveryResult(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *item)
	}
	return out, rows.Err()
}

func (d *DB) DeleteProxyDiscoveryResult(id int64) error {
	result, err := d.Exec(`DELETE FROM proxy_discovery_results WHERE id=$1 AND state<>'approved'`, id)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (d *DB) UpdateProxyDiscoveryResult(id int64, state string, authorized bool, ip string, latencyMS *int, problem string) error {
	_, err := d.Exec(`UPDATE proxy_discovery_results SET state=$2,authorized=$3,verified_ip=$4,
verified_at=CASE WHEN $4<>'' THEN now() ELSE NULL END,latency_ms=$5,last_error=$6,updated_at=now(),
last_verify_mode='',last_verify_target='',last_http_status=0,last_checked_at=NULL
WHERE id=$1`, id, state, authorized, ip, latencyMS, problem)
	return err
}

type ProxyDiscoveryAdmissionProof struct {
	IP         string
	Mode       string
	Target     string
	HTTPStatus int
	CheckedAt  time.Time
	LatencyMS  int
}

func (d *DB) PromoteProxyDiscoveryResult(resultID, poolID int64, origin string, enabled bool, proof *ProxyDiscoveryAdmissionProof) (int64, error) {
	tx, err := d.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	var nodeID int64
	var address, protocol, host, state, verifiedIP, mode, target, problem string
	var verifiedAt, checkedAt *time.Time
	var latencyMS *int
	var port int
	var httpStatus int
	var authorized bool
	err = tx.QueryRow(`SELECT id,address,protocol,host,port,authorized,state,verified_ip,verified_at,latency_ms,
last_verify_mode,last_verify_target,last_http_status,last_checked_at,last_error
FROM proxy_discovery_results WHERE id=$1 FOR UPDATE`, resultID).
		Scan(&resultID, &address, &protocol, &host, &port, &authorized, &state, &verifiedIP, &verifiedAt, &latencyMS,
			&mode, &target, &httpStatus, &checkedAt, &problem)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrProxyDiscoveryNotFound
	}
	if err != nil {
		return 0, err
	}
	if state == "approved" {
		return 0, ErrProxyDiscoveryAlreadyApproved
	}
	candidate := &ProxyDiscoveryResult{State: state, VerifiedIP: verifiedIP, VerifiedAt: verifiedAt,
		LastVerifyMode: mode, LastHTTPStatus: httpStatus, LastCheckedAt: checkedAt, LastError: problem}
	if !candidate.HasSuccessfulVerification() {
		return 0, ErrProxyDiscoveryNotVerified
	}
	if !authorized {
		return 0, ErrProxyDiscoveryOutOfScope
	}
	if mode == "" {
		mode = proxypool.VerifyExitIP
	}
	if checkedAt == nil {
		checkedAt = verifiedAt
	}
	if proof != nil {
		mode = proof.Mode
		if mode == "" {
			mode = "exit_ip"
		}
		if (mode != "exit_ip" && mode != "connectivity") ||
			(mode == "exit_ip" && net.ParseIP(proof.IP) == nil) ||
			(mode == "connectivity" && (proof.IP != "" || proof.HTTPStatus != 200)) ||
			proof.CheckedAt.IsZero() || proof.LatencyMS < 0 {
			return 0, ErrProxyDiscoveryNotVerified
		}
		if mode == "exit_ip" {
			verifiedIP, verifiedAt = proof.IP, &proof.CheckedAt
		}
		latencyMS = &proof.LatencyMS
		checkedAt, target, httpStatus = &proof.CheckedAt, proof.Target, proof.HTTPStatus
	}
	var targetPoolID int64
	err = tx.QueryRow(`SELECT id FROM proxy_pools WHERE id=$1 FOR KEY SHARE`, poolID).Scan(&targetPoolID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrProxyPoolNotFound
	}
	if err != nil {
		return 0, err
	}
	address = protocol + "://" + net.JoinHostPort(host, strconv.Itoa(port))
	err = tx.QueryRow(`INSERT INTO proxy_nodes(pool_id,address,origin,enabled,verified_ip,verified_at,
last_checked_at,last_success_at,latency_ms,success_count,last_verify_mode,last_verify_target,last_http_status)
VALUES($1,$2,$3,$4,$5,$6,$7,$7,$8,1,$9,$10,$11)
ON CONFLICT(pool_id,address) DO UPDATE SET enabled=$4,deleted_at=NULL,
verified_ip=CASE WHEN EXCLUDED.verified_ip<>'' THEN EXCLUDED.verified_ip ELSE proxy_nodes.verified_ip END,
verified_at=CASE WHEN EXCLUDED.verified_ip<>'' THEN EXCLUDED.verified_at ELSE proxy_nodes.verified_at END,
last_checked_at=EXCLUDED.last_checked_at,
last_success_at=EXCLUDED.last_success_at,latency_ms=EXCLUDED.latency_ms,last_error='',
last_verify_mode=EXCLUDED.last_verify_mode,last_verify_target=EXCLUDED.last_verify_target,last_http_status=EXCLUDED.last_http_status,
success_count=proxy_nodes.success_count+1
RETURNING id`, poolID, address, origin, enabled, verifiedIP, verifiedAt, checkedAt, latencyMS, mode, target, httpStatus).Scan(&nodeID)
	if err != nil {
		return 0, err
	}
	if _, err := tx.Exec(`UPDATE proxy_discovery_results SET state='approved',updated_at=now() WHERE id=$1`, resultID); err != nil {
		return 0, err
	}
	return nodeID, tx.Commit()
}

func (d *DB) TaskProxyBinding(taskID int64) (*TaskProxyBinding, error) {
	var b TaskProxyBinding
	err := d.QueryRow(`SELECT task_id,pool_id,node_id,mode,expires_at,revision,strategy FROM task_proxy_bindings WHERE task_id=$1`, taskID).Scan(&b.TaskID, &b.PoolID, &b.NodeID, &b.Mode, &b.ExpiresAt, &b.Revision, &b.Strategy)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &b, err
}

func insertTaskProxyBinding(tx *sql.Tx, taskID int64, poolID *int64, nodeID *int64, mode string, parentTaskID *int64) error {
	strategy := "round_robin"
	if nodeID != nil {
		strategy = "fixed"
	}
	if poolID == nil && parentTaskID != nil {
		var b TaskProxyBinding
		err := tx.QueryRow(`SELECT pool_id,mode,node_id,strategy FROM task_proxy_bindings WHERE task_id=$1`, *parentTaskID).Scan(&b.PoolID, &b.Mode, &b.NodeID, &b.Strategy)
		if err != nil && err != sql.ErrNoRows {
			return err
		}
		if err == nil {
			poolID, mode = &b.PoolID, b.Mode
			if nodeID == nil && b.Strategy == "fixed" {
				nodeID = b.NodeID
				strategy = "fixed"
			}
		}
	}
	if poolID == nil {
		if mode != "" {
			return errors.New("network mode requires a proxy pool")
		}
		return nil
	}
	if mode == "" {
		mode = "configured"
	}
	if mode != "configured" {
		return errors.New("enforced execution environment is not available")
	}
	p, err := scanProxyPool(tx.QueryRow(`SELECT id,config,credentials,quota FROM proxy_pools WHERE id=$1 FOR SHARE`, *poolID))
	if err != nil {
		return errors.New("proxy pool not found")
	}
	if !p.Config.Enabled {
		return errors.New("proxy pool disabled")
	}
	var selectedNodeID int64
	if nodeID != nil {
		err = tx.QueryRow(`SELECT id FROM proxy_nodes WHERE id=$1 AND pool_id=$2 AND enabled=true AND last_error=''
AND (runtime_cooldown_until IS NULL OR runtime_cooldown_until<=now())
AND deleted_at IS NULL AND (expires_at IS NULL OR expires_at>now()+make_interval(secs=>$3))
AND (origin NOT IN ('fofa','quake') OR (((last_verify_mode='connectivity' AND last_http_status=200 AND last_success_at>now()-interval '15 minutes')
OR (last_verify_mode IN ('','exit_ip') AND verified_at>now()-interval '15 minutes' AND verified_ip<>''))
AND last_error='' AND (last_failure_at IS NULL OR last_success_at>=last_failure_at)))`,
			*nodeID, *poolID, p.Config.SafetySeconds).Scan(&selectedNodeID)
	} else {
		err = tx.QueryRow(`SELECT id FROM proxy_nodes WHERE pool_id=$1 AND enabled=true AND deleted_at IS NULL AND last_error=''
AND (runtime_cooldown_until IS NULL OR runtime_cooldown_until<=now())
AND (expires_at IS NULL OR expires_at>now()+make_interval(secs=>$2))
AND (origin NOT IN ('fofa','quake') OR (((last_verify_mode='connectivity' AND last_http_status=200 AND last_success_at>now()-interval '15 minutes')
OR (last_verify_mode IN ('','exit_ip') AND verified_at>now()-interval '15 minutes' AND verified_ip<>''))
AND last_error='' AND (last_failure_at IS NULL OR last_success_at>=last_failure_at)))
ORDER BY (SELECT count(*) FROM task_proxy_bindings b WHERE b.node_id=proxy_nodes.id),id LIMIT 1`,
			*poolID, p.Config.SafetySeconds).Scan(&selectedNodeID)
	}
	if err != nil {
		return errors.New("proxy pool has no valid selected node; fetch or verify nodes first")
	}
	_, err = tx.Exec(`INSERT INTO task_proxy_bindings(task_id,pool_id,node_id,mode,expires_at,strategy) SELECT $1,$2,$3,$4,expires_at,$5 FROM proxy_nodes WHERE id=$3`, taskID, *poolID, selectedNodeID, mode, strategy)
	return err
}

func (d *DB) AddProxyRouteEvent(taskID, poolID, nodeID int64, target, result string) error {
	_, err := d.Exec(`INSERT INTO proxy_route_events(task_id,pool_id,node_id,target,result) VALUES(NULLIF($1,0),NULLIF($2,0),NULLIF($3,0),$4,$5)`,
		taskID, poolID, nodeID, target, result)
	return err
}

func (d *DB) ProxyRouteEvents(taskID, poolID int64) ([]ProxyRouteEvent, error) {
	rows, err := d.Query(`SELECT id,task_id,pool_id,node_id,target,result,created_at,traffic_id,revision FROM proxy_route_events
WHERE ($1::bigint=0 OR task_id=$1) AND ($2::bigint=0 OR pool_id=$2) ORDER BY id DESC LIMIT 100`, taskID, poolID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ProxyRouteEvent{}
	for rows.Next() {
		var e ProxyRouteEvent
		if err := rows.Scan(&e.ID, &e.TaskID, &e.PoolID, &e.NodeID, &e.Target, &e.Result, &e.CreatedAt, &e.TrafficID, &e.Revision); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (d *DB) AddProxyRouteCompletion(route proxypool.Route, target, result, trafficID string) error {
	_, err := d.Exec(`INSERT INTO proxy_route_events(task_id,pool_id,node_id,target,result,traffic_id,revision)
VALUES(NULLIF($1,0),NULLIF($2,0),NULLIF($3,0),$4,$5,$6,$7)`,
		route.TaskID, route.PoolID, route.Node.ID, target, result, trafficID, route.Revision)
	return err
}
