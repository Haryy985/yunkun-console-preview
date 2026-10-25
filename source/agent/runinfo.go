package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strings"
	"sync"

	"github.com/Autumn-27/norma/harness"
	"yunkun/console/db"
)

// RunInfo identifies WHICH run a tool call belongs to. Tool assembly only receives
// (ctx, agentKey) — the task/exploration ids live in the caller's arguments, not the
// ctx — so anything wired at assembly time (currently the Skill ledger in
// server/assembly.go) has no way to attribute a call to a task. Each run attaches
// its own RunInfo before calling AugmentTools; the wiring closure reads it once and
// captures it, so per-run attribution stays correct without threading parameters
// through the tool layer. Same pattern as TaskClock (see taskclock.go).
//
// Zero value = attribution unknown; every consumer must treat it as optional.
type RunInfo struct {
	TaskID        int64  // task registry id; 0 for non-task runs (chat sessions)
	ExplorationID int64  // exploration id; 0 when unknown
	IntentID      int64  // worker's intent node; 0 for planner/mainagent/chat
	SessionID     string // chat conversation id; empty for task runs
	AgentID       string // selected chat agent key; task runs use the ToolSet worker
	Trace         *RunTrace
}

// RunTrace captures resources made available while assembling one agent run.
// It intentionally stores names only, never skill contents or prompts.
type RunTrace struct {
	mu           sync.Mutex
	agentKey     string
	loadedSkills []string
	observer     func(RunTraceEvent)
	finished     bool
}

type RunTraceEvent struct {
	Kind         string   `json:"kind"`
	AgentKey     string   `json:"agent_key"`
	LoadedSkills []string `json:"loaded_skills,omitempty"`
	Skill        string   `json:"skill,omitempty"`
	Found        bool     `json:"found,omitempty"`
	Success      bool     `json:"success,omitempty"`
	Status       string   `json:"status,omitempty"`
}

func NewRunTrace(agentKey string, observer func(RunTraceEvent)) *RunTrace {
	return &RunTrace{agentKey: agentKey, observer: observer}
}

func NewActivityRunTrace(agentKey string, emit func(db.Activity)) *RunTrace {
	if emit == nil {
		return NewRunTrace(agentKey, nil)
	}
	return NewRunTrace(agentKey, func(event RunTraceEvent) {
		detail, _ := json.Marshal(event)
		tool, summary := "run", fmt.Sprintf("Agent %s 配置已解析", agentKey)
		isError := false
		switch event.Kind {
		case "configured":
			if len(event.LoadedSkills) == 0 {
				summary = fmt.Sprintf("Agent %s 启动，未加载 Skill", agentKey)
			} else {
				summary = fmt.Sprintf("Agent %s 启动，可调用 Skill：%s", agentKey, strings.Join(event.LoadedSkills, ", "))
			}
		case "skill_call":
			tool = "Skill"
			state := "失败"
			if event.Success {
				state = "成功"
			} else {
				isError = true
			}
			summary = fmt.Sprintf("Agent %s 调用 Skill %s：%s", agentKey, event.Skill, state)
			if !event.Found {
				summary += "（未找到）"
			}
		case "finished":
			state := "失败"
			switch event.Status {
			case "completed":
				state = "完成"
			case "timeout":
				state = "超时"
				isError = true
			case "max_turns":
				state = "达到轮数上限"
				isError = true
			default:
				isError = true
			}
			summary = fmt.Sprintf("Agent %s 执行%s", agentKey, state)
		}
		emit(db.Activity{Worker: agentKey, Kind: "agent_run", Tool: tool, IsError: isError,
			Summary: summary, Detail: string(detail)})
	})
}

func (t *RunTrace) SetLoadedSkills(skills []string) {
	if t == nil {
		return
	}
	t.mu.Lock()
	t.loadedSkills = slices.Clone(skills)
	sort.Strings(t.loadedSkills)
	event := RunTraceEvent{Kind: "configured", AgentKey: t.agentKey, LoadedSkills: slices.Clone(t.loadedSkills)}
	observer := t.observer
	t.mu.Unlock()
	if observer != nil {
		observer(event)
	}
}

func (t *RunTrace) LoadedSkills() []string {
	if t == nil {
		return nil
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return slices.Clone(t.loadedSkills)
}

func (t *RunTrace) RecordSkillCall(skill string, found, success bool) {
	if t == nil {
		return
	}
	t.mu.Lock()
	event := RunTraceEvent{Kind: "skill_call", AgentKey: t.agentKey, Skill: skill, Found: found, Success: success}
	observer := t.observer
	t.mu.Unlock()
	if observer != nil {
		observer(event)
	}
}

func (t *RunTrace) Complete(status string) {
	if t == nil {
		return
	}
	t.mu.Lock()
	if t.finished {
		t.mu.Unlock()
		return
	}
	t.finished = true
	event := RunTraceEvent{Kind: "finished", AgentKey: t.agentKey, LoadedSkills: slices.Clone(t.loadedSkills), Status: status}
	observer := t.observer
	t.mu.Unlock()
	if observer != nil {
		observer(event)
	}
}

func (t *RunTrace) Finish(reason harness.TerminalReason, err error) {
	status := string(reason)
	if err != nil {
		status = "failed"
	} else if status == "" {
		status = "unknown"
	}
	t.Complete(status)
}

// explorationID reads a store's exploration id, tolerating a nil store (planner and
// worker runs can be driven without one in tests).
func explorationID(ts *db.ExplorationStore) int64 {
	if ts == nil {
		return 0
	}
	return ts.ID()
}

type runInfoKey struct{}

// WithRunInfo attaches run attribution to ctx.
func WithRunInfo(ctx context.Context, ri RunInfo) context.Context {
	return context.WithValue(ctx, runInfoKey{}, ri)
}

// RunInfoFrom reads the RunInfo (zero value if none attached).
func RunInfoFrom(ctx context.Context) RunInfo {
	if v, ok := ctx.Value(runInfoKey{}).(RunInfo); ok {
		return v
	}
	return RunInfo{}
}
