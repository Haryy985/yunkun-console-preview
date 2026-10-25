package agent

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/Autumn-27/norma/harness"
	"yunkun/console/db"
)

func TestActivityRunTraceRecordsLoadedSkillsCallsAndCompletion(t *testing.T) {
	var activities []db.Activity
	trace := NewActivityRunTrace("custom-auditor", func(activity db.Activity) {
		activities = append(activities, activity)
	})
	trace.SetLoadedSkills([]string{"z-skill", "a-skill"})
	trace.RecordSkillCall("a-skill", true, true)
	trace.RecordSkillCall("missing-skill", false, false)
	trace.Complete("completed")
	if len(activities) != 4 {
		t.Fatalf("got %d trace events, want 4", len(activities))
	}
	for _, activity := range activities {
		if activity.Kind != "agent_run" || activity.Worker != "custom-auditor" || activity.Detail == "" {
			t.Fatalf("incomplete trace activity: %+v", activity)
		}
	}
	var configured RunTraceEvent
	if err := json.Unmarshal([]byte(activities[0].Detail), &configured); err != nil {
		t.Fatal(err)
	}
	if configured.Kind != "configured" || len(configured.LoadedSkills) != 2 ||
		configured.LoadedSkills[0] != "a-skill" || configured.LoadedSkills[1] != "z-skill" {
		t.Fatalf("loaded skills were not captured: %+v", configured)
	}
	if activities[2].IsError != true || activities[3].Summary != "Agent custom-auditor 执行完成" {
		t.Fatalf("skill failure or completion was not represented: %+v %+v", activities[2], activities[3])
	}
}

func TestRunTraceTerminalStatusIsAccurateAndIdempotent(t *testing.T) {
	for _, test := range []struct {
		reason harness.TerminalReason
		err    error
		want   string
	}{
		{harness.ReasonCompleted, nil, "completed"},
		{harness.ReasonTimeout, nil, "timeout"},
		{harness.ReasonMaxTurns, nil, "max_turns"},
		{harness.ReasonCompleted, errors.New("fixture"), "failed"},
		{"", nil, "unknown"},
	} {
		t.Run(test.want, func(t *testing.T) {
			var events []RunTraceEvent
			trace := NewRunTrace("worker", func(event RunTraceEvent) { events = append(events, event) })
			trace.Finish(test.reason, test.err)
			trace.Finish(harness.ReasonCompleted, nil)
			if len(events) != 1 || events[0].Status != test.want {
				t.Fatalf("terminal events: %+v", events)
			}
		})
	}
}
