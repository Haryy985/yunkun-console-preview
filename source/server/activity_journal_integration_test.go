package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"yunkun/console/db"
)

// This gate interrupts only connections created by this test. The upstream
// PostgreSQL process and all other callers remain untouched.
type journalDatabaseGate struct {
	listener net.Listener
	upstream string
	mu       sync.Mutex
	enabled  bool
	conns    map[net.Conn]bool
	wg       sync.WaitGroup
}

func newJournalDatabaseGate(t *testing.T, upstream string) *journalDatabaseGate {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	g := &journalDatabaseGate{listener: listener, upstream: upstream, enabled: true, conns: map[net.Conn]bool{}}
	g.wg.Add(1)
	go func() {
		defer g.wg.Done()
		for {
			client, err := listener.Accept()
			if err != nil {
				return
			}
			g.mu.Lock()
			if !g.enabled {
				g.mu.Unlock()
				_ = client.Close()
				continue
			}
			g.conns[client] = true
			g.mu.Unlock()
			g.wg.Add(1)
			go g.forward(client)
		}
	}()
	t.Cleanup(func() {
		_ = listener.Close()
		g.setEnabled(false)
		g.wg.Wait()
	})
	return g
}

func (g *journalDatabaseGate) forward(client net.Conn) {
	defer g.wg.Done()
	defer client.Close()
	upstream, err := net.DialTimeout("tcp", g.upstream, time.Second)
	if err != nil {
		g.mu.Lock()
		delete(g.conns, client)
		g.mu.Unlock()
		return
	}
	defer upstream.Close()
	g.mu.Lock()
	if !g.enabled {
		delete(g.conns, client)
		g.mu.Unlock()
		return
	}
	g.conns[upstream] = true
	g.mu.Unlock()
	done := make(chan struct{})
	go func() {
		_, _ = io.Copy(upstream, client)
		_ = upstream.Close()
		close(done)
	}()
	_, _ = io.Copy(client, upstream)
	_ = client.Close()
	<-done
	g.mu.Lock()
	delete(g.conns, client)
	delete(g.conns, upstream)
	g.mu.Unlock()
}

func (g *journalDatabaseGate) setEnabled(enabled bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.enabled = enabled
	if !enabled {
		for conn := range g.conns {
			_ = conn.Close()
		}
	}
}

func TestActivityJournalPostgresOutageRecovery(t *testing.T) {
	s := newProxyFixture(t)
	dsn, _, err := db.DSN()
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	var database string
	if err := s.m.pg.QueryRow("SELECT current_database()").Scan(&database); err != nil {
		t.Fatal(err)
	}
	u.Path = "/" + database
	port := u.Port()
	if port == "" {
		port = "5432"
	}
	gate := newJournalDatabaseGate(t, net.JoinHostPort(u.Hostname(), port))
	u.Host = gate.listener.Addr().String()
	query := u.Query()
	query.Set("connect_timeout", "1")
	query.Set("sslmode", "disable")
	u.RawQuery = query.Encode()
	pool, err := sql.Open("pgx", u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = pool.Close() })
	proxied := &db.DB{DB: pool}
	task, err := s.m.pg.CreateTaskWithOptions("journal recovery", "local only", db.TaskCreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.m.pg.SetSetting(authPassKey, "outage-fixture-hash"); err != nil {
		t.Fatal(err)
	}
	auth := &Server{m: &Manager{pg: proxied}, jwtKey: []byte("outage-fixture-key")}
	token, err := signIdentityJWT(auth.jwtKey, adminUsername, passwordVersion("outage-fixture-hash"))
	if err != nil {
		t.Fatal(err)
	}
	handler := auth.requireAuth(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	checkAuth := func(want int) {
		t.Helper()
		req := httptest.NewRequest("GET", "/api/auth/me", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != want {
			t.Fatalf("auth status %d, want %d", rec.Code, want)
		}
		if want == http.StatusServiceUnavailable && rec.Header().Get("Retry-After") != "5" {
			t.Fatal("outage did not request retry")
		}
	}
	checkAuth(http.StatusNoContent)
	j := &activityJournal{root: t.TempDir()}
	id := strconv.FormatInt(task.ID, 10)
	tokens := 17
	for _, kind := range []string{"tool_use", "tool_result", "result"} {
		_, err := j.enqueue(id, task.ExplorationID, db.Activity{
			Kind: kind, Detail: "POST /fixture HTTP/1.1\r\nHost: fixture.invalid\r\n\r\n",
			ToolUseID: "fixture-call", InputTokens: &tokens,
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	gate.setEnabled(false)
	checkAuth(http.StatusServiceUnavailable)
	store := proxied.Exploration(task.ExplorationID)
	if _, err := j.flush(id, task.ExplorationID, store.AppendActivity,
		func(db.Activity) { t.Fatal("published activity during outage") }); err == nil {
		t.Fatal("database outage unexpectedly succeeded")
	}
	gate.setEnabled(true)
	checkAuth(http.StatusNoContent) // The same token survives a real connection interruption.
	j = &activityJournal{root: j.root}
	first := true
	appendLostReply := func(a db.Activity) (int64, error) {
		n, err := store.AppendActivity(a)
		if err == nil && first {
			first = false
			return 0, errors.New("fixture lost commit acknowledgment")
		}
		return n, err
	}
	if _, err := j.flush(id, task.ExplorationID, appendLostReply,
		func(db.Activity) { t.Fatal("published before acknowledgment") }); err == nil {
		t.Fatal("lost reply was not surfaced")
	}
	var kinds []string
	_, err = j.flush(id, task.ExplorationID, store.AppendActivity, func(a db.Activity) {
		kinds = append(kinds, a.Kind)
	})
	if err != nil || len(kinds) != 3 || kinds[0] != "tool_use" || kinds[1] != "tool_result" || kinds[2] != "result" {
		t.Fatalf("replay order %v: %v", kinds, err)
	}
	var count, sum int
	if err := s.m.pg.QueryRow("SELECT count(*),sum(input_tokens) FROM activity WHERE exploration_id=$1", task.ExplorationID).Scan(&count, &sum); err != nil {
		t.Fatal(err)
	}
	if count != 3 || sum != 51 {
		t.Fatalf("replay duplicated rows or tokens: count=%d sum=%d", count, sum)
	}
	dir, _ := j.directory(id)
	files, err := os.ReadDir(dir)
	if err != nil || len(files) != 0 {
		t.Fatalf("pending journal not drained: %v", err)
	}
	if err := s.m.pg.SetSetting(authPassKey, "revoked-fixture-hash"); err != nil {
		t.Fatal(err)
	}
	checkAuth(http.StatusUnauthorized)
}

func TestActivityJournalArchiveRestoreRetainsPendingRecords(t *testing.T) {
	s := newProxyFixture(t)
	s.m.dir = t.TempDir()
	s.engine.journal = &activityJournal{root: s.m.dir}
	if err := s.m.pg.EnsureLLMRecordsTable(); err != nil {
		t.Fatal(err)
	}
	if err := s.m.pg.EnsureLLMUsageTable(); err != nil {
		t.Fatal(err)
	}
	task, err := s.m.pg.CreateTaskWithOptions("journal archive", "local only", db.TaskCreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.m.pg.SetStatus(task.ID, "done"); err != nil {
		t.Fatal(err)
	}
	id := strconv.FormatInt(task.ID, 10)
	tokens := 17
	detail := "GET /fixture HTTP/1.1\r\nHost: fixture.invalid\r\n\r\n"
	if _, err := s.engine.journal.enqueue(id, task.ExplorationID, db.Activity{
		Kind: "result", Detail: detail, InputTokens: &tokens,
	}); err != nil {
		t.Fatal(err)
	}
	archive, err := s.m.pg.QueueTaskArchive(task.ID)
	if err != nil {
		t.Fatal(err)
	}
	job, err := s.m.pg.ClaimTaskArchiveJob(t.Context())
	if err != nil || job == nil || job.ID != archive.ID {
		t.Fatalf("claim archive: %+v %v", job, err)
	}
	if err := s.archiveTask(job); err != nil {
		t.Fatal(err)
	}
	if _, err := s.m.pg.QueueTaskArchiveRestore(archive.ID); err != nil {
		t.Fatal(err)
	}
	job, err = s.m.pg.ClaimTaskArchiveJob(t.Context())
	if err != nil || job == nil || job.ID != archive.ID {
		t.Fatalf("claim restore: %+v %v", job, err)
	}
	if err := s.restoreTaskArchive(job); err != nil {
		t.Fatal(err)
	}
	// A subsequent journal scan must not replay a record already in the archive.
	if err := s.engine.flushTaskJournal(id, task.ExplorationID); err != nil {
		t.Fatal(err)
	}
	var count, sum int
	var restoredDetail, eventKey string
	err = s.m.pg.QueryRow(`SELECT count(*),sum(input_tokens),min(detail),min(event_key)
		FROM activity WHERE exploration_id=$1`, task.ExplorationID).Scan(&count, &sum, &restoredDetail, &eventKey)
	if err != nil || count != 1 || sum != tokens || restoredDetail != detail || eventKey == "" {
		t.Fatalf("archive lost or duplicated pending record: count=%d tokens=%d err=%v", count, sum, err)
	}
}

func TestActivityStreamReplayCursorAndMultipleBatches(t *testing.T) {
	s := newProxyFixture(t)
	task, err := s.m.pg.CreateTaskWithOptions("stream replay", "local only", db.TaskCreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	id := strconv.FormatInt(task.ID, 10)
	store := s.m.pg.Exploration(task.ExplorationID)
	s.m.tasks[id] = &Task{ID: id, Store: store}
	var ids []int64
	for i := 0; i < 503; i++ {
		seq, err := store.AppendActivity(db.Activity{Kind: "text", Summary: "replay fixture"})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, seq)
	}
	for _, tc := range []struct {
		name   string
		since  string
		header string
		start  int
	}{
		{"multiple batches", "0", "", 0},
		{"header overrides snapshot", "0", strconv.FormatInt(ids[500], 10), 501},
		{"invalid header uses snapshot", strconv.FormatInt(ids[500], 10), "invalid", 501},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			// Replay is synchronous; cancel the idle live tail without opening a listener.
			cancel()
			req := httptest.NewRequest("GET", "/api/exploration/activity/stream?task="+id+"&since="+tc.since, nil).WithContext(ctx)
			req.Header.Set("Last-Event-ID", tc.header)
			rec := httptest.NewRecorder()
			s.streamActivity(rec, req)
			if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "text/event-stream" {
				t.Fatalf("stream status=%d", rec.Code)
			}
			var received []string
			for _, line := range strings.Split(rec.Body.String(), "\n") {
				if strings.HasPrefix(line, "id: ") {
					received = append(received, strings.TrimPrefix(line, "id: "))
				}
			}
			if len(received) != len(ids)-tc.start {
				t.Fatalf("replay count=%d want=%d", len(received), len(ids)-tc.start)
			}
			for i, got := range received {
				if got != strconv.FormatInt(ids[i+tc.start], 10) {
					t.Fatalf("replay order at %d: %s", i, got)
				}
			}
		})
	}
}

func TestIdleIntentRecoveryPreservesOwnedWork(t *testing.T) {
	s := newProxyFixture(t)
	row, err := s.m.pg.CreateTaskWithOptions("interrupted state", "local only", db.TaskCreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	id := strconv.FormatInt(row.ID, 10)
	task := &Task{ID: id, Store: s.m.pg.Exploration(row.ExplorationID)}
	s.m.tasks[id] = task
	intent, err := task.Store.AddNode("intent", map[string]any{"summary": "fixture"}, 0, "running", "test", nil)
	if err != nil {
		t.Fatal(err)
	}
	state := func(want string) {
		t.Helper()
		var got string
		if err := s.m.pg.QueryRow("SELECT state FROM exploration_nodes WHERE id=$1", intent).Scan(&got); err != nil || got != want {
			t.Fatalf("state=%s want=%s err=%v", got, want, err)
		}
	}
	if !s.engine.beginTaskOperation(id) {
		t.Fatal("task admission failed")
	}
	if err := s.engine.reconcileIdleIntentStates(t.Context(), task); err != nil {
		t.Fatal(err)
	}
	state("running")
	s.engine.decInflight(id)
	if err := s.engine.reconcileIdleIntentStates(t.Context(), task); err != nil {
		t.Fatal(err)
	}
	state("blocked")
	if err := task.Store.SetIntentState(intent, "running"); err != nil {
		t.Fatal(err)
	}
	if err := s.m.pg.SetStatus(row.ID, "done"); err != nil {
		t.Fatal(err)
	}
	if err := s.engine.reconcileIdleIntentStates(t.Context(), task); err != nil {
		t.Fatal(err)
	}
	state("stopped")
	if err := s.engine.reconcileIdleIntentStates(t.Context(), task); err != nil {
		t.Fatal(err)
	}
	state("stopped")
	if err := task.Store.SetIntentState(intent, "running"); err != nil {
		t.Fatal(err)
	}
	s.engine.deleting.Store(id, true)
	if err := s.engine.reconcileIdleIntentStates(t.Context(), task); err != nil {
		t.Fatal(err)
	}
	state("running")
	s.engine.deleting.Delete(id)
}

func TestTerminalTaskStatsDoesNotReportLiveEngine(t *testing.T) {
	fixture := newProxyFixture(t)
	row, err := fixture.m.pg.CreateTaskWithOptions("finished", "local only", db.TaskCreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	id := strconv.FormatInt(row.ID, 10)
	task := &Task{ID: id, Status: "done", Store: fixture.m.pg.Exploration(row.ExplorationID)}
	fixture.m.tasks[id] = task
	fixture.m.assets = fixture.m.pg.Assets()
	s := newAdmissionTestServer(fixture.m, nil)
	s.engine.started.Store(id, true)
	rec := httptest.NewRecorder()
	s.stats(rec, httptest.NewRequest("GET", "/api/stats?task="+id, nil))
	var body struct {
		ActiveTask struct {
			Running bool `json:"running"`
		} `json:"active_task"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || rec.Code != 200 || body.ActiveTask.Running {
		t.Fatalf("terminal engine status=%d running=%v err=%v", rec.Code, body.ActiveTask.Running, err)
	}
}
