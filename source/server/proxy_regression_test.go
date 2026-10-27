package server

import (
	"context"
	"fmt"
	"testing"
	"time"

	"yunkun/console/db"
	"yunkun/console/proxypool"
)

func TestProxyMetadataEditPreservesCredentialIdentity(t *testing.T) {
	s := newProxyFixture(t)
	poolID := createTestPool(t, s, "http://127.0.0.1:19991")
	task, err := s.m.pg.CreateTaskWithOptions("metadata", "fixture", db.TaskCreateOptions{ProxyPoolID: &poolID})
	if err != nil {
		t.Fatal(err)
	}
	route, err := s.selectProxyRoute(context.Background(), task.ID)
	if err != nil {
		t.Fatal(err)
	}
	before, _ := s.m.pg.ProxyPool(poolID)
	config := before.Config
	config.Name = "renamed fixture"
	response := proxyAPICall(t, s, "PUT", fmt.Sprintf("/api/proxy-pools/%d", poolID), saveProxyPoolRequest{Config: config})
	if response.Code != 200 {
		t.Fatal(response.Code, response.Body)
	}
	after, _ := s.m.pg.ProxyPool(poolID)
	if after.Credentials != before.Credentials {
		t.Error("metadata edit rotated ciphertext identity")
	}
	if err := s.validateProxyRoute(context.Background(), route); err != nil {
		t.Error("metadata edit revoked in-flight route:", err)
	}
	if err := s.m.pg.RecordProxyNodeVerification(poolID, route.Node.ID, proxypool.VerificationResult{}, "", 1, before.Credentials); err != nil {
		t.Error("metadata edit rejected in-flight verification:", err)
	}
}

func TestProxyRoutingRejectsIneligibleFixedSelection(t *testing.T) {
	for _, mutation := range []string{
		`runtime_cooldown_until=now()+interval '1 minute'`,
		`expires_at=now()+interval '30 seconds'`,
		`origin='quake',last_verify_mode='connectivity',last_http_status=200,last_success_at=now()-interval '16 minutes'`,
	} {
		t.Run(mutation, func(t *testing.T) {
			s := newProxyFixture(t)
			poolID := createTestPool(t, s, "http://127.0.0.1:19991")
			task, err := s.m.pg.CreateTaskWithOptions("fixed eligibility", "fixture", db.TaskCreateOptions{ProxyPoolID: &poolID})
			if err != nil {
				t.Fatal(err)
			}
			nodes, _ := s.m.pg.ProxyNodes(poolID)
			if _, err := s.m.pg.Exec(`UPDATE tasks SET paused=true WHERE id=$1`, task.ID); err != nil {
				t.Fatal(err)
			}
			if _, err := s.m.pg.Exec(`UPDATE proxy_pools SET config=jsonb_set(config,'{safety_seconds}','60') WHERE id=$1`, poolID); err != nil {
				t.Fatal(err)
			}
			if _, err := s.m.pg.Exec(`UPDATE proxy_nodes SET `+mutation+` WHERE id=$1`, nodes[0].ID); err != nil {
				t.Fatal(err)
			}
			if err := s.m.pg.SetProxyRouting(task.ID, "fixed", &nodes[0].ID); err == nil {
				t.Error("accepted fixed node that scheduler cannot use")
			}
		})
	}
}

func TestProxyReassignRejectsFailedAndCoolingNodes(t *testing.T) {
	s := newProxyFixture(t)
	poolID := createTestPool(t, s, "http://127.0.0.1:19991")
	if err := s.m.pg.StoreProxyNodes(poolID, []proxypool.Node{{Address: "http://127.0.0.1:19992"}}); err != nil {
		t.Fatal(err)
	}
	task, err := s.m.pg.CreateTaskWithOptions("reassign", "fixture", db.TaskCreateOptions{ProxyPoolID: &poolID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.m.pg.Exec(`UPDATE tasks SET paused=true WHERE id=$1`, task.ID); err != nil {
		t.Fatal(err)
	}
	b, _ := s.m.pg.TaskProxyBinding(task.ID)
	if err := s.m.pg.SetProxyRouting(task.ID, "round_robin", b.NodeID); err == nil {
		t.Error("round robin accepted an unrelated explicit node argument")
	}
	for _, mutation := range []string{`last_error='verification failed'`, `last_error='',runtime_cooldown_until=now()+interval '1 minute'`} {
		if _, err := s.m.pg.Exec(`UPDATE proxy_nodes SET `+mutation+` WHERE pool_id=$1`, poolID); err != nil {
			t.Fatal(err)
		}
		if err := s.m.pg.ReassignProxyLease(task.ID); err == nil {
			t.Error("reassigned to unavailable node:", mutation)
		}
	}
}

func TestProxyOldRuntimeSuccessCannotMutateNewCredentialState(t *testing.T) {
	s := newProxyFixture(t)
	poolID := createTestPool(t, s, "http://127.0.0.1:19991")
	task, err := s.m.pg.CreateTaskWithOptions("old completion", "fixture", db.TaskCreateOptions{ProxyPoolID: &poolID})
	if err != nil {
		t.Fatal(err)
	}
	route, err := s.selectProxyRoute(context.Background(), task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.m.pg.Exec(`UPDATE proxy_pools SET credentials='replacement-fixture' WHERE id=$1`, poolID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.m.pg.Exec(`UPDATE proxy_nodes SET runtime_error='new-fault',runtime_cooldown_until=now()-interval '1 second' WHERE id=$1`, route.Node.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.m.pg.RecordProxyRuntimeSuccess(route); err != db.ErrProxyPoolCredentialsChanged {
		t.Error("stale success was not rejected:", err)
	}
	node, _ := s.m.pg.ProxyNode(poolID, route.Node.ID)
	if node.RuntimeError != "new-fault" {
		t.Error("stale success cleared new credential health")
	}
}

func TestProxyOldSuccessCannotClearLaterCooldown(t *testing.T) {
	s := newProxyFixture(t)
	poolID := createTestPool(t, s, "http://127.0.0.1:19991")
	task, err := s.m.pg.CreateTaskWithOptions("late success", "fixture", db.TaskCreateOptions{ProxyPoolID: &poolID})
	if err != nil {
		t.Fatal(err)
	}
	old, err := s.selectProxyRoute(context.Background(), task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.m.pg.Exec(`UPDATE proxy_nodes SET runtime_error='later-failure',runtime_cooldown_until=now()-interval '1 second' WHERE id=$1`, old.Node.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.m.pg.RecordProxyRuntimeSuccess(old); err != nil {
		t.Fatal(err)
	}
	n, _ := s.m.pg.ProxyNode(poolID, old.Node.ID)
	if n.RuntimeError != "later-failure" {
		t.Fatal("old request cleared a newer failure after cooldown elapsed")
	}
	trial, err := s.selectProxyRoute(context.Background(), task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.m.pg.RecordProxyRuntimeSuccess(trial); err != nil {
		t.Fatal(err)
	}
	n, _ = s.m.pg.ProxyNode(poolID, old.Node.ID)
	if n.RuntimeError != "" {
		t.Fatal("fresh trial failed to clear its observed cooldown")
	}
}

func TestProxyRecoveryDoesNotWaitWithInvertedTaskLock(t *testing.T) {
	s := newProxyFixture(t)
	poolID := createTestPool(t, s, "http://127.0.0.1:19991")
	task, err := s.m.pg.CreateTaskWithOptions("lock", "fixture", db.TaskCreateOptions{ProxyPoolID: &poolID})
	if err != nil {
		t.Fatal(err)
	}
	pool, _ := s.m.pg.ProxyPool(poolID)
	tx, err := s.m.pg.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`UPDATE tasks SET paused=true WHERE id=$1`, task.ID); err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() { result <- s.m.pg.RecoverProxyCredentials(poolID, pool.Credentials, "fixture") }()
	select {
	case err := <-result:
		if err == nil {
			t.Error("recovery should reject busy task")
		}
	case <-time.After(time.Second):
		// Release the fixture lock even on regression; do not leave a blocked goroutine.
		t.Error("recovery waits on task while holding pool lock; reverse order can deadlock")
		tx.Rollback()
		<-result
	}
}
