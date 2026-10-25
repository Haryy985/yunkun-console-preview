package db

import (
	"database/sql"
	"sort"
	"strconv"
	"time"
)

// LLMUsage is one lightweight LLM-call metering row — the always-on usage ledger,
// distinct from llm_records (which stores full request/response bodies and is a
// gated debug feature). One row per completion call, written on both success and
// error, so token accounting is complete even for interrupted/failed runs. Carries
// only the dimensions needed to slice token spend (model / profile / task / agent),
// never any prompt or response content.
type LLMUsage struct {
	RequestID           string `json:"request_id"`
	SessionID           string `json:"session_id"`
	TaskID              string `json:"task_id"`        // task registry id (matches llm_records.task_id)
	ExplorationID       int64  `json:"exploration_id"` // exploration id parsed from the session (0 = unknown/non-task)
	Worker              string `json:"worker"`         // agent lane: worker / planner / mainagent / goals
	Model               string `json:"model"`
	ProfileName         string `json:"profile_name"`
	LatencyMs           int    `json:"latency_ms"`
	InputTokens         int    `json:"input_tokens"`
	OutputTokens        int    `json:"output_tokens"`
	CacheRead           int    `json:"cache_read"`
	CacheWrite          int    `json:"cache_write"`
	UsageKnown          bool   `json:"usage_known"`
	Status              string `json:"status"`                // ok | error
	ContextWindowTokens int    `json:"context_window_tokens"` // 0 = not confirmed for this request
}

// RAGPromptInclusion proves that a retrieved chunk's exact text was present in
// a tool-result block of the request assembled for a specific LLM call. It does
// not prove the model used the content in its answer.
type RAGPromptInclusion struct {
	RequestID      string `json:"request_id"`
	RetrievalLogID int64  `json:"retrieval_log_id"`
	ChunkID        int64  `json:"chunk_id"`
	ToolUseID      string `json:"tool_use_id"`
}

type RAGPromptCandidate struct {
	RetrievalLogID int64
	ChunkID        int64
	ToolUseID      string
	Content        string
}

const llmUsageSchema = `
CREATE TABLE IF NOT EXISTS llm_usage (
    id             BIGSERIAL PRIMARY KEY,
    ts             TIMESTAMPTZ NOT NULL DEFAULT now(),
    task_id        TEXT,
    exploration_id BIGINT,
    request_id     TEXT,
    session_id     TEXT,
    worker         TEXT,
    model          TEXT,
    profile_name   TEXT,
    latency_ms     INTEGER,
    input_tokens   INTEGER NOT NULL DEFAULT 0,
    output_tokens  INTEGER NOT NULL DEFAULT 0,
    cache_read     INTEGER NOT NULL DEFAULT 0,
    cache_write    INTEGER NOT NULL DEFAULT 0,
    usage_known    BOOLEAN NOT NULL DEFAULT false,
    status         TEXT,
    context_window_tokens INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_llm_usage_task  ON llm_usage(task_id);
CREATE INDEX IF NOT EXISTS idx_llm_usage_model ON llm_usage(task_id, model);
CREATE INDEX IF NOT EXISTS idx_llm_usage_exp   ON llm_usage(exploration_id);
CREATE INDEX IF NOT EXISTS idx_llm_usage_task_latest ON llm_usage(task_id, id DESC);
CREATE TABLE IF NOT EXISTS rag_prompt_inclusions (
    request_id TEXT NOT NULL,
    task_id TEXT NOT NULL,
    retrieval_log_id BIGINT NOT NULL REFERENCES rag_retrieval_logs(id) ON DELETE CASCADE,
    chunk_id BIGINT NOT NULL,
    tool_use_id TEXT NOT NULL,
    PRIMARY KEY (request_id, retrieval_log_id, chunk_id)
);
CREATE INDEX IF NOT EXISTS idx_rag_prompt_inclusions_task ON rag_prompt_inclusions(task_id, retrieval_log_id);
`

const llmUsageMigrate = `
ALTER TABLE llm_usage ADD COLUMN IF NOT EXISTS request_id TEXT;
ALTER TABLE llm_usage ADD COLUMN IF NOT EXISTS session_id TEXT;
ALTER TABLE llm_usage ADD COLUMN IF NOT EXISTS context_window_tokens INTEGER NOT NULL DEFAULT 0;
ALTER TABLE llm_usage ADD COLUMN IF NOT EXISTS usage_known BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE llm_usage ALTER COLUMN usage_known SET DEFAULT false;
CREATE TABLE IF NOT EXISTS llm_usage_migrations (version TEXT PRIMARY KEY, applied_at TIMESTAMPTZ NOT NULL DEFAULT now());
DO $$ BEGIN
    IF NOT EXISTS (SELECT 1 FROM llm_usage_migrations WHERE version='usage-known-v1') THEN
        UPDATE llm_usage SET usage_known=false;
        INSERT INTO llm_usage_migrations(version) VALUES ('usage-known-v1');
    END IF;
END $$;
CREATE UNIQUE INDEX IF NOT EXISTS idx_llm_usage_request ON llm_usage(request_id) WHERE request_id IS NOT NULL;
`

// EnsureLLMUsageTable creates the llm_usage metering table if it does not exist.
func (d *DB) EnsureLLMUsageTable() error {
	tx, err := d.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck
	if err := coordinateWithSchemaMigration(tx); err != nil {
		return err
	}
	if _, err := tx.Exec(llmUsageSchema); err != nil {
		return err
	}
	if _, err := tx.Exec(llmUsageMigrate); err != nil {
		return err
	}
	return tx.Commit()
}

// InsertLLMUsage appends one metering row. Best-effort: callers log and continue on
// error (a lost metering row must never break the LLM call).
func (d *DB) InsertLLMUsage(u *LLMUsage) error {
	var expID any
	if u.ExplorationID > 0 {
		expID = u.ExplorationID
	}
	_, err := d.Exec(`
INSERT INTO llm_usage(task_id, exploration_id, request_id, session_id, worker, model, profile_name, latency_ms, input_tokens, output_tokens, cache_read, cache_write, usage_known, status, context_window_tokens)
VALUES (NULLIF($1,''),$2,NULLIF($3,''),NULLIF($4,''),NULLIF($5,''),NULLIF($6,''),NULLIF($7,''),$8,$9,$10,$11,$12,$13,$14,$15)`,
		u.TaskID, expID, u.RequestID, u.SessionID, u.Worker, u.Model, u.ProfileName,
		u.LatencyMs, u.InputTokens, u.OutputTokens, u.CacheRead, u.CacheWrite, u.UsageKnown, u.Status, u.ContextWindowTokens)
	return err
}

// RecordRAGPromptInclusions checks each candidate against the persisted search
// result and owning task. Callers cannot manufacture an inclusion by supplying
// an unrelated log ID, chunk ID, or altered chunk text.
func (d *DB) RecordRAGPromptInclusions(taskID, requestID string, candidates []RAGPromptCandidate) error {
	if taskID == "" || requestID == "" {
		return nil
	}
	for _, c := range candidates {
		if c.RetrievalLogID <= 0 || c.ChunkID <= 0 || c.Content == "" {
			continue
		}
		_, err := d.Exec(`
INSERT INTO rag_prompt_inclusions(request_id, task_id, retrieval_log_id, chunk_id, tool_use_id)
SELECT $1, $2, l.id, $4::bigint, $5 FROM rag_retrieval_logs l
WHERE l.id=$3 AND l.task_id=$2
AND jsonb_typeof(l.results)='array'
AND EXISTS (SELECT 1 FROM jsonb_array_elements(l.results) result
            WHERE result->>'chunk_id'=($4::bigint)::text AND result->>'content'=$6)
ON CONFLICT DO NOTHING`, requestID, taskID, c.RetrievalLogID, c.ChunkID, c.ToolUseID, c.Content)
		if err != nil {
			return err
		}
	}
	return nil
}

func (d *DB) ListTaskRAGPromptInclusions(taskID string, limit int) ([]RAGPromptInclusion, error) {
	if limit <= 0 || limit > 1000 {
		limit = 1000
	}
	rows, err := d.Query(`SELECT request_id, retrieval_log_id, chunk_id, tool_use_id
FROM rag_prompt_inclusions WHERE task_id=$1
ORDER BY retrieval_log_id DESC, request_id DESC, chunk_id LIMIT $2`, taskID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]RAGPromptInclusion, 0)
	for rows.Next() {
		var item RAGPromptInclusion
		if err := rows.Scan(&item.RequestID, &item.RetrievalLogID, &item.ChunkID, &item.ToolUseID); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

// LatestTaskLLMUsage returns the most recently completed request, not an
// aggregate. A missing row has no usable context measurement.
func (d *DB) LatestTaskLLMUsage(taskID string) (*LLMUsage, error) {
	var u LLMUsage
	err := d.QueryRow(`
SELECT COALESCE(request_id,''), COALESCE(session_id,''), COALESCE(worker,''),
       COALESCE(model,''), input_tokens, output_tokens, usage_known, COALESCE(status,''), context_window_tokens
FROM llm_usage WHERE task_id=$1 ORDER BY id DESC LIMIT 1`, taskID).
		Scan(&u.RequestID, &u.SessionID, &u.Worker, &u.Model, &u.InputTokens, &u.OutputTokens, &u.UsageKnown, &u.Status, &u.ContextWindowTokens)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	u.TaskID = taskID
	return &u, nil
}

// LatestTaskLLMUsageAll loads one completed request per task in one query.
func (d *DB) LatestTaskLLMUsageAll() (map[string]LLMUsage, error) {
	rows, err := d.Query(`
SELECT DISTINCT ON (task_id) task_id, COALESCE(request_id,''), COALESCE(session_id,''),
       COALESCE(worker,''), COALESCE(model,''), input_tokens, output_tokens, usage_known, COALESCE(status,''), context_window_tokens
FROM llm_usage WHERE task_id IS NOT NULL
ORDER BY task_id, id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[string]LLMUsage)
	for rows.Next() {
		var u LLMUsage
		if err := rows.Scan(&u.TaskID, &u.RequestID, &u.SessionID, &u.Worker,
			&u.Model, &u.InputTokens, &u.OutputTokens, &u.UsageKnown, &u.Status, &u.ContextWindowTokens); err != nil {
			return nil, err
		}
		out[u.TaskID] = u
	}
	return out, rows.Err()
}

// TokenByModel aggregates a task's LLM token usage grouped by model, most-used
// first, from the always-on llm_usage ledger. taskID is the task registry id.
// Accurate even with per-agent model bindings, pool rotation/failover, and
// interrupted runs, since every call (success or error) is metered.
func (d *DB) TokenByModel(taskID string) ([]ModelTokenStat, error) {
	rows, err := d.Query(`
SELECT COALESCE(NULLIF(model,''),'(unknown)') AS model, COUNT(*) AS calls,
       COALESCE(SUM(input_tokens),0), COALESCE(SUM(output_tokens),0),
       COALESCE(SUM(cache_read),0), COALESCE(SUM(cache_write),0),
       COUNT(*) FILTER (WHERE NOT usage_known)
FROM llm_usage
WHERE COALESCE(task_id,'') = $1
GROUP BY model
ORDER BY SUM(input_tokens) + SUM(output_tokens) DESC, model`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ModelTokenStat{}
	for rows.Next() {
		var m ModelTokenStat
		if err := rows.Scan(&m.Model, &m.Calls, &m.InputTokens, &m.OutputTokens,
			&m.CacheReadTokens, &m.CacheWriteTokens, &m.UnknownCalls); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// ProfileUsage aggregates the whole ledger's token spend for one LLM profile
// (global, all tasks). Powers the dashboard's per-profile token card (new source).
type ProfileUsage struct {
	ProfileName      string `json:"profile_name"`
	Calls            int    `json:"calls"`
	UnknownCalls     int    `json:"unknown_calls"`
	Tasks            int    `json:"tasks"`
	InputTokens      int    `json:"input_tokens"`
	OutputTokens     int    `json:"output_tokens"`
	CacheReadTokens  int    `json:"cache_read_tokens"`
	CacheWriteTokens int    `json:"cache_write_tokens"`
}

// UsageByProfile returns global token spend grouped by profile name, most-used
// first. profile_name may be empty for calls made on env/non-persisted configs.
func (d *DB) UsageByProfile() ([]ProfileUsage, error) {
	rows, err := d.Query(`
SELECT COALESCE(profile_name,'') AS profile_name, COUNT(*) AS calls,
       COUNT(*) FILTER (WHERE NOT usage_known),
       COUNT(DISTINCT task_id) AS tasks,
       COALESCE(SUM(input_tokens),0), COALESCE(SUM(output_tokens),0),
       COALESCE(SUM(cache_read),0), COALESCE(SUM(cache_write),0)
FROM llm_usage
GROUP BY profile_name
ORDER BY SUM(input_tokens) + SUM(output_tokens) DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ProfileUsage{}
	for rows.Next() {
		var p ProfileUsage
		if err := rows.Scan(&p.ProfileName, &p.Calls, &p.UnknownCalls, &p.Tasks,
			&p.InputTokens, &p.OutputTokens, &p.CacheReadTokens, &p.CacheWriteTokens); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	archived, err := d.archivedTaskAggregates()
	if err != nil {
		return nil, err
	}
	byName := make(map[string]ProfileUsage, len(out))
	for _, current := range out {
		byName[current.ProfileName] = current
	}
	for _, aggregate := range archived {
		for _, cold := range aggregate.TokenProfiles {
			current := byName[cold.ProfileName]
			current.ProfileName = cold.ProfileName
			current.Calls += cold.Calls
			current.UnknownCalls += cold.UnknownCalls
			current.Tasks += cold.Tasks
			current.InputTokens += cold.InputTokens
			current.OutputTokens += cold.OutputTokens
			current.CacheReadTokens += cold.CacheReadTokens
			current.CacheWriteTokens += cold.CacheWriteTokens
			byName[cold.ProfileName] = current
		}
	}
	out = out[:0]
	for _, current := range byName {
		out = append(out, current)
	}
	sort.Slice(out, func(i, j int) bool {
		left := out[i].InputTokens + out[i].OutputTokens
		right := out[j].InputTokens + out[j].OutputTokens
		if left != right {
			return left > right
		}
		return out[i].ProfileName < out[j].ProfileName
	})
	return out, nil
}

// ProfileDayUsage is one (profile, UTC calendar day) token bucket for the daily
// chart. Unlike the activity-based chart, ts is the real call time, so this is
// actual per-day consumption rather than tokens bucketed by task creation date.
type ProfileDayUsage struct {
	ProfileName     string `json:"profile_name"`
	Date            string `json:"date"` // YYYY-MM-DD (UTC)
	InputTokens     int    `json:"input_tokens"`
	OutputTokens    int    `json:"output_tokens"`
	CacheReadTokens int    `json:"cache_read_tokens"`
}

// UsageDaily returns per-(profile, day) token buckets for the past `days` days
// (default 365 when days<=0), so the dashboard can slice by profile + range.
func (d *DB) UsageDaily(days int) ([]ProfileDayUsage, error) {
	if days <= 0 {
		days = 365
	}
	rows, err := d.Query(`
SELECT COALESCE(profile_name,'') AS profile_name,
       to_char(ts AT TIME ZONE 'UTC', 'YYYY-MM-DD') AS day,
       COALESCE(SUM(input_tokens),0), COALESCE(SUM(output_tokens),0), COALESCE(SUM(cache_read),0)
FROM llm_usage
WHERE ts >= now() - ($1 * interval '1 day')
GROUP BY profile_name, day
ORDER BY day`, days)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ProfileDayUsage{}
	for rows.Next() {
		var p ProfileDayUsage
		if err := rows.Scan(&p.ProfileName, &p.Date, &p.InputTokens, &p.OutputTokens, &p.CacheReadTokens); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	archived, err := d.archivedTaskAggregates()
	if err != nil {
		return nil, err
	}
	cutoff := time.Now().UTC().AddDate(0, 0, -days).Format("2006-01-02")
	byKey := make(map[string]ProfileDayUsage, len(out))
	for _, current := range out {
		byKey[current.ProfileName+"\x00"+current.Date] = current
	}
	for _, aggregate := range archived {
		for _, cold := range aggregate.TokenDaily {
			if cold.Date < cutoff {
				continue
			}
			key := cold.ProfileName + "\x00" + cold.Date
			current := byKey[key]
			current.ProfileName = cold.ProfileName
			current.Date = cold.Date
			current.InputTokens += cold.InputTokens
			current.OutputTokens += cold.OutputTokens
			current.CacheReadTokens += cold.CacheReadTokens
			byKey[key] = current
		}
	}
	out = out[:0]
	for _, current := range byKey {
		out = append(out, current)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Date != out[j].Date {
			return out[i].Date < out[j].Date
		}
		return out[i].ProfileName < out[j].ProfileName
	})
	return out, nil
}

// JudgeDayUsage is one UTC-day token bucket for the intercept fallback judge,
// for the config page's recent-spend sparkline.
type JudgeDayUsage struct {
	Date         string `json:"date"` // YYYY-MM-DD (UTC)
	Calls        int    `json:"calls"`
	InputTokens  int    `json:"input_tokens"`
	OutputTokens int    `json:"output_tokens"`
}

// JudgeUsage is the cumulative token spend of the intercept fallback judge
// (worker='judge' rows in the always-on ledger), plus a recent daily series.
type JudgeUsage struct {
	Calls            int             `json:"calls"`
	InputTokens      int             `json:"input_tokens"`
	OutputTokens     int             `json:"output_tokens"`
	CacheReadTokens  int             `json:"cache_read_tokens"`
	CacheWriteTokens int             `json:"cache_write_tokens"`
	Daily            []JudgeDayUsage `json:"daily"`
}

// JudgeUsageStats returns the fallback judge's all-time token totals and a
// per-day series over the past `days` days (default 30). Sourced from the
// always-on llm_usage ledger, so it is accurate across pool rotation and
// interrupted/failed judge calls. days only bounds the daily series; totals are
// all-time.
func (d *DB) JudgeUsageStats(days int) (JudgeUsage, error) {
	if days <= 0 {
		days = 30
	}
	var u JudgeUsage
	err := d.QueryRow(`
SELECT COUNT(*), COALESCE(SUM(input_tokens),0), COALESCE(SUM(output_tokens),0),
       COALESCE(SUM(cache_read),0), COALESCE(SUM(cache_write),0)
FROM llm_usage
WHERE worker = 'judge'`).Scan(&u.Calls, &u.InputTokens, &u.OutputTokens,
		&u.CacheReadTokens, &u.CacheWriteTokens)
	if err != nil {
		return JudgeUsage{}, err
	}
	rows, err := d.Query(`
SELECT to_char(ts AT TIME ZONE 'UTC', 'YYYY-MM-DD') AS day,
       COUNT(*), COALESCE(SUM(input_tokens),0), COALESCE(SUM(output_tokens),0)
FROM llm_usage
WHERE worker = 'judge' AND ts >= now() - ($1 * interval '1 day')
GROUP BY day
ORDER BY day`, days)
	if err != nil {
		return JudgeUsage{}, err
	}
	defer rows.Close()
	u.Daily = []JudgeDayUsage{}
	for rows.Next() {
		var day JudgeDayUsage
		if err := rows.Scan(&day.Date, &day.Calls, &day.InputTokens, &day.OutputTokens); err != nil {
			return JudgeUsage{}, err
		}
		u.Daily = append(u.Daily, day)
	}
	if err := rows.Err(); err != nil {
		return JudgeUsage{}, err
	}
	return u, nil
}

// ParseExpID turns the exploration-id segment parsed from a session string into an
// int64 (0 when empty/non-numeric, e.g. chat sessions keyed by conversation id).
func ParseExpID(s string) int64 {
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil || n < 0 {
		return 0
	}
	return n
}
