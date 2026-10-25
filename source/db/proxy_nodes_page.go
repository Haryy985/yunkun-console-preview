package db

import (
	"errors"
	"strconv"
	"strings"

	"yunkun/console/proxypool"
)

type ProxyNodeFilter struct {
	Query          string
	Status         string
	Protocol       string
	Sort           string
	WarningSeconds int
}

func (d *DB) ProxyNodesPage(poolID int64, page, size int, filter ProxyNodeFilter) ([]proxypool.Node, int, error) {
	if page < 1 || page > 1000000 || size < 1 || size > 100 || len(filter.Query) > 200 {
		return nil, 0, errors.New("invalid node page")
	}
	warning := filter.WarningSeconds
	if warning <= 0 {
		warning = 3600
	}
	where := `pool_id=$1 AND deleted_at IS NULL AND ($2='' OR
position(lower($2) in lower(concat_ws(' ',address,declared_ip,verified_ip,origin,last_error,notes)))>0)`
	args := []any{poolID, strings.TrimSpace(filter.Query)}
	switch filter.Status {
	case "":
	case "disabled":
		where += ` AND NOT enabled`
	case "available":
		args = append(args, warning)
		where += ` AND enabled AND last_error='' AND last_success_at IS NOT NULL AND (expires_at IS NULL OR expires_at>now()+make_interval(secs=>$3::double precision))`
	case "pending":
		args = append(args, warning)
		where += ` AND enabled AND last_error='' AND last_success_at IS NULL AND (expires_at IS NULL OR expires_at>now()+make_interval(secs=>$3::double precision))`
	case "expiring":
		args = append(args, warning)
		where += ` AND enabled AND expires_at>now() AND expires_at<=now()+make_interval(secs=>$3::double precision)`
	case "error":
		where += ` AND enabled AND (last_error<>'' OR expires_at<=now())`
	default:
		return nil, 0, errors.New("invalid node status")
	}
	if filter.Protocol != "" {
		if filter.Protocol != "http" && filter.Protocol != "https" && filter.Protocol != "socks5" {
			return nil, 0, errors.New("invalid node protocol")
		}
		where += ` AND split_part(address,':',1)=$` + strconv.Itoa(len(args)+1)
		args = append(args, filter.Protocol)
	}
	order := "id DESC"
	switch filter.Sort {
	case "", "newest":
	case "latency":
		order = "(last_error<>'') ASC,latency_ms ASC NULLS LAST,id DESC"
	default:
		return nil, 0, errors.New("invalid node sort")
	}
	var total int
	if err := d.QueryRow(`SELECT count(*) FROM proxy_nodes WHERE `+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	// Size and offset have been range checked; every user text value is bound.
	rows, err := d.Query(`SELECT id,pool_id,address,declared_ip,expires_at,verified_ip,verified_at,last_error,
enabled,origin,last_checked_at,latency_ms,success_count,failure_count,last_success_at,last_failure_at,notes,deleted_at,
last_verify_mode,last_verify_target,last_http_status
FROM proxy_nodes WHERE `+where+` ORDER BY `+order+` LIMIT `+strconv.Itoa(size)+` OFFSET `+strconv.Itoa((page-1)*size), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	nodes := []proxypool.Node{}
	for rows.Next() {
		var n proxypool.Node
		if err := rows.Scan(&n.ID, &n.PoolID, &n.Address, &n.DeclaredIP, &n.ExpiresAt, &n.VerifiedIP, &n.VerifiedAt, &n.LastError,
			&n.Enabled, &n.Origin, &n.LastCheckedAt, &n.LatencyMS, &n.SuccessCount, &n.FailureCount,
			&n.LastSuccessAt, &n.LastFailureAt, &n.Notes, &n.DeletedAt,
			&n.LastVerifyMode, &n.LastVerifyTarget, &n.LastHTTPStatus); err != nil {
			return nil, 0, err
		}
		nodes = append(nodes, n)
	}
	return nodes, total, rows.Err()
}
