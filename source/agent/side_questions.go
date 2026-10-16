package agent

import (
	"context"
	"strconv"
	"strings"

	"github.com/Autumn-27/norma/agentcore"
	"yunkun/console/sidequestion"
)

func attachSideCapture(ctx context.Context, opts *agentcore.Options) context.Context {
	ri := RunInfoFrom(ctx)
	p, ok := sideParentFromRunInfo(ri)
	if !ok {
		return ctx
	}
	ctx, opts.Deps = sidequestion.Attach(ctx, p, opts.Deps, opts.Provider)
	return ctx
}

// sideParentFromRunInfo keeps conversation and task ownership mutually
// exclusive. A conversation session may inherit task attribution for proxy and
// audit purposes, but its side-question checkpoint belongs to the conversation
// row only.
func sideParentFromRunInfo(ri RunInfo) (sidequestion.Parent, bool) {
	if strings.HasPrefix(ri.SessionID, "conv-") {
		if id, err := strconv.ParseInt(strings.TrimPrefix(ri.SessionID, "conv-"), 10, 64); err == nil && id > 0 {
			return sidequestion.Parent{ConversationID: id}, true
		}
	}
	if ri.TaskID > 0 && ri.ExplorationID > 0 {
		return sidequestion.Parent{
			TaskID:        ri.TaskID,
			ExplorationID: ri.ExplorationID,
			IntentID:      ri.IntentID,
		}, true
	}
	return sidequestion.Parent{}, false
}
