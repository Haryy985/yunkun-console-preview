package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"runtime/debug"

	"github.com/Autumn-27/norma/agentcore"
	actool "github.com/Autumn-27/norma/tool"
)

// DeferredInfo carries the deferred-tools wiring an agent needs to build its
// Options: the MCP tool names whose schemas are withheld, the subset listed in the
// global system-prompt block (non-skill-gated), and the shared session unlock set.
// UnlockSkill unlocks a named skill's MCPs — hosts call it to rebuild the unlock set
// from history on a resumed session (design doc C2).
type DeferredInfo struct {
	ResourceGuidance string
	FindingGuidance  string            // derived from the final permitted tools, including DB overrides
	Deferred         []string          // all MCP tool names (schema withheld)
	GlobalNames      []string          // MCP names to list in the system-prompt block
	Unlock           *actool.UnlockSet // shared call-gate; nil when no MCP tools
	UnlockSkill      func(skillName string)
}

// ToolAugment, if set, returns the EXTRA tools an agent should see beyond its
// built-in base set — the agent's visible skills (packed into one Skill meta-tool)
// and visible MCP servers (expanded to mcp__server__tool). It also returns the
// DeferredInfo describing how those MCP tools are deferred/gated. The server wires
// it to the PG agent_visibility table. cleanup releases any spawned MCP clients.
//
// When nil, agents run with only their built-in tools — behavior is unchanged
// until a user assigns a skill/MCP to the agent in the UI.
var ToolAugment func(ctx context.Context, agentKey string) (extra []actool.CoreTool, def DeferredInfo, cleanup func())

// NetworkTools applies server-owned routing after configurable tool assembly.
var NetworkTools func(context.Context, []actool.CoreTool) []actool.CoreTool

// The SDK adds WebFetch inside NewSession, after AugmentTools. Materialize it
// here so server routing also covers that late-added tool.
func newRunSession(ctx context.Context, opts agentcore.Options) *agentcore.Session {
	if opts.EnableWebFetch && NetworkTools != nil {
		if opts.Tools == nil {
			opts.Tools = actool.DefaultTools()
		}
		base := actool.NewWebFetch(actool.WebFetchConfig{
			Proxy: opts.WebFetchProxy, CACert: opts.WebFetchCACert, InsecureTLS: opts.WebFetchInsecureTLS,
		})
		tools := make([]actool.CoreTool, 0, len(opts.Tools)+1)
		for _, t := range opts.Tools {
			if t.Name() != "WebFetch" {
				tools = append(tools, t)
			}
		}
		lateTools := []actool.CoreTool{base}
		if ToolResolve != nil {
			resolved := ToolResolve(ctx, RunInfoFrom(ctx).AgentID, lateTools)
			lateTools = nil
			for _, t := range resolved {
				if t.Name() == "WebFetch" {
					lateTools = append(lateTools, t)
				}
			}
		}
		for _, t := range NetworkTools(ctx, lateTools) {
			tools = append(tools, guardPanic(t))
		}
		opts.Tools, opts.EnableWebFetch = tools, false
	}
	return agentcore.NewSession(opts)
}

// AugmentTools returns base plus the agent's visible skill/MCP tools, the
// DeferredInfo, and a cleanup func the caller must defer (closes MCP clients).
// Built-in base tools are kept as-is — never filtered (内置工具留代码层，不做可见性过滤).
func AugmentTools(ctx context.Context, agentKey string, base []actool.CoreTool) ([]actool.CoreTool, DeferredInfo, func()) {
	var (
		def     DeferredInfo
		cleanup = func() {}
		out     = base
	)
	if ToolAugment != nil {
		var extra []actool.CoreTool
		var cl func()
		extra, def, cl = ToolAugment(ctx, agentKey)
		if cl != nil {
			cleanup = cl
		}
		if len(extra) > 0 {
			out = append(append([]actool.CoreTool{}, base...), extra...)
		}
	}
	// DB tools table has the final say on the built-in tools: drop the ones this
	// agent isn't bound to (or that are disabled) and swap in overridden
	// descriptions/schemas + default injection. MCP/skill/host tools have no row
	// and pass through untouched, so deferred/unlock wiring stays consistent.
	if ToolResolve != nil {
		out = ToolResolve(ctx, agentKey, out)
	}
	if NetworkTools != nil {
		out = NetworkTools(ctx, out)
	}
	if NetworkStatus != nil {
		if taskID := RunInfoFrom(ctx).TaskID; taskID > 0 {
			out = append(out, networkStatusTool(taskID))
		}
	}
	out, def.FindingGuidance = findingWorkflowTools(agentKey, out)
	def.ResourceGuidance = ""
	for _, tool := range out {
		if tool.Name() == "available_resources" {
			def.ResourceGuidance = resourceGuidance
			break
		}
	}
	for i, t := range out {
		out[i] = guardPanic(t)
	}
	return out, def, cleanup
}

// guardPanic turns a panicking tool handler into an ordinary tool error. The
// harness runs each tool on its own goroutine, so a panic inside a handler can't
// be recovered by the caller that started the run — it takes the whole process
// down, and on restart the agent replays the same call and crashes again. Applied
// last, so it covers every tool the agent can reach: domain, SDK, MCP and skill.
func guardPanic(t actool.CoreTool) actool.CoreTool { return &guardedTool{CoreTool: t} }

type guardedTool struct{ actool.CoreTool }

func (g *guardedTool) Call(ctx context.Context, in json.RawMessage, tc *actool.ToolContext) (res actool.Result, err error) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[tools] %s panic: %v\n%s", g.Name(), r, debug.Stack())
			res, err = actool.Errorf(fmt.Sprintf("工具 %s 内部错误：%v（本次调用已失败，可换个参数或改用别的工具）", g.Name(), r)), nil
		}
	}()
	return g.CoreTool.Call(ctx, in, tc)
}
