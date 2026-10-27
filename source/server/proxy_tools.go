package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	actool "github.com/Autumn-27/norma/tool"
	"yunkun/console/agent"
	"yunkun/console/proxypool"
)

type taskProxyFetch struct {
	actool.CoreTool
	s      *Server
	taskID int64
}

func (t *taskProxyFetch) Description() string {
	return "Fetch an HTTP(S) page through this task's configured proxy pool. Returns status, headers and raw text (including HTML source when extract=true), truncated at 50,000 bytes. The server rotates eligible nodes in round_robin mode and may retry safe reads after a proxy connection failure; fixed mode stays on the selected node. Exhausted pools stop requests; no direct fallback. Inspect network_status for routing strategy, health and coverage."
}

func (t *taskProxyFetch) Call(ctx context.Context, input json.RawMessage, _ *actool.ToolContext) (actool.Result, error) {
	if gate, ok := t.CoreTool.(interface{ ValidateCall(context.Context) error }); ok {
		if err := gate.ValidateCall(ctx); err != nil {
			return actool.Result{}, err
		}
	}
	var in struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(input, &in); err != nil {
		return actool.Errorf("invalid fetch input"), nil
	}
	u, err := url.Parse(in.URL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil {
		return actool.Errorf("absolute HTTP(S) URL without credentials required"), nil
	}
	if t.s.proxyPools == nil || t.s.proxyPools.gateway == nil {
		return actool.Errorf("task gateway unavailable"), nil
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
	if err != nil {
		return actool.Errorf("invalid fetch URL"), nil
	}
	req.Header.Set("User-Agent", "Yunkun/task-proxy")
	exchange, err := t.s.proxyPools.gateway.RoundTrip(req, t.taskID)
	if err != nil {
		return actool.Errorf("task proxy fetch failed; direct fallback forbidden"), nil
	}
	resp := exchange.Response
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, proxypool.MaxCaptureBody+1))
	_ = resp.Body.Close()
	if err != nil {
		return actool.Errorf("task proxy response interrupted"), nil
	}
	note := ""
	if len(b) > 50000 {
		b = b[:50000]
		note = "\n[response truncated]"
	}
	headers := resp.Header.Clone()
	headers.Del("Set-Cookie")
	headerJSON, _ := json.Marshal(headers)
	capture := "\n[capture: " + exchange.CaptureStatus + "]"
	if exchange.TrafficID != "" {
		capture += "\n[traffic_id: " + exchange.TrafficID + "]"
	}
	return actool.Text(fmt.Sprintf("HTTP %d\n%s\n\n%s%s%s", resp.StatusCode, headerJSON, strings.ToValidUTF8(string(b), "?"), note, capture)), nil
}

func (s *Server) proxyNetworkTools(ctx context.Context, tools []actool.CoreTool) []actool.CoreTool {
	taskID := agent.RunInfoFrom(ctx).TaskID
	if taskID <= 0 {
		return tools
	}
	b, err := s.m.pg.TaskProxyBinding(taskID)
	if err == nil && b == nil {
		return tools
	}
	out := append([]actool.CoreTool(nil), tools...)
	for i, t := range out {
		if t.Name() == "WebFetch" {
			out[i] = &taskProxyFetch{CoreTool: t, s: s, taskID: taskID}
		}
	}
	return out
}
