package server

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"time"
)

// Claims expire so shutdown or a crash never loses an imported node. Workers
// share the same verification slots and implementation as manual verification.
func (s *Server) runProxyImports() {
	for range 4 {
		go func() {
			for s.ctx.Err() == nil {
				if !s.verifyNextImport(s.ctx) {
					if sleepCtx(s.ctx, time.Second) {
						return
					}
				}
			}
		}()
	}
}

func (s *Server) verifyNextImport(ctx context.Context) bool {
	var poolID, nodeID int64
	err := s.m.pg.QueryRowContext(ctx, `UPDATE proxy_nodes SET import_verify_claimed_at=now()
WHERE id=(SELECT id FROM proxy_nodes WHERE import_verify_pending AND deleted_at IS NULL
AND (import_verify_claimed_at IS NULL OR import_verify_claimed_at<now()-interval '1 minute')
ORDER BY id FOR UPDATE SKIP LOCKED LIMIT 1) RETURNING pool_id,id`).Scan(&poolID, &nodeID)
	if err != nil {
		return false
	}
	p, err := s.m.pg.ProxyPool(poolID)
	if err != nil || p == nil || !p.Config.Enabled {
		return false
	}
	secrets, err := s.proxyPools.vault.Open(p.Credentials)
	if err != nil {
		return false
	}
	n, err := s.m.pg.ProxyNode(poolID, nodeID)
	if err != nil || n == nil {
		return false
	}
	endpoint, mode, err := nodeVerificationTarget(p.Config.VerifyURL, "")
	if err != nil {
		return false
	}
	_, _, err = s.checkProxyNode(ctx, p, secrets, n, endpoint, mode)
	if ctx.Err() != nil || errors.Is(err, errProxyVerifyBusy) || errors.Is(err, errProxyVerifyDuplicate) {
		return false
	}
	// A database write failure must remain pending; do not accidentally enable
	// a node for which verification evidence was not saved.
	if errors.Is(err, errProxyVerifyStore) {
		return false
	}
	result, finishErr := s.m.pg.ExecContext(ctx, `UPDATE proxy_nodes SET enabled=$2,import_verify_pending=false,
import_verify_claimed_at=NULL WHERE id=$1 AND import_verify_pending AND deleted_at IS NULL
AND EXISTS (SELECT 1 FROM proxy_pools p WHERE p.id=proxy_nodes.pool_id AND p.credentials=$3
AND p.config->>'enabled'='true' AND p.config->>'kind'='fixed' AND COALESCE(p.config->>'verify_url','')=$4)`,
		nodeID, err == nil, p.Credentials, p.Config.VerifyURL)
	if finishErr != nil {
		return false
	}
	changed, finishErr := result.RowsAffected()
	return finishErr == nil && changed == 1
}

func (s *Server) proxyImportStatus(w http.ResponseWriter, r *http.Request) {
	p, _, ok := s.poolForRequest(w, r)
	if !ok {
		return
	}
	var pending int
	err := s.m.pg.QueryRowContext(r.Context(), `SELECT count(*) FROM proxy_nodes
WHERE pool_id=$1 AND deleted_at IS NULL AND import_verify_pending`, p.ID).Scan(&pending)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		writeErr(w, 500, "读取导入验证状态失败")
		return
	}
	writeJSON(w, 200, map[string]int{"pending": pending})
}
