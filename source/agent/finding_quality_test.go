package agent

import (
	"strings"
	"testing"

	"yunkun/console/db"
)

func completeFindingValidation() FindingValidation {
	return FindingValidation{
		Authorization: "approved fixture", Asset: "fixture.local",
		Identity: "test user", Preconditions: "signed in",
		Baseline: "normal response", Mutation: "one changed object ID",
		ObservedDelta: "different response", SecurityImpact: "observed state difference",
		Cleanup: "no persistent change",
	}
}

func TestFindingQualityRequiresProofAndTrace(t *testing.T) {
	proof := completeFindingValidation()
	if err := validateFindingProof(proof, nil, nil, 0); err == nil || !strings.Contains(err.Error(), "evidence_step_ids") {
		t.Fatalf("missing evidence accepted: %v", err)
	}
	proof.ObservedDelta = ""
	if err := validateFindingProof(proof, []db.TrafficRef{{TrafficID: "fixture"}}, nil, 0); err == nil || !strings.Contains(err.Error(), "observed_delta") {
		t.Fatalf("missing delta accepted: %v", err)
	}
	proof = completeFindingValidation()
	if err := validateFindingProof(proof, []db.TrafficRef{{TrafficID: "fixture"}}, nil, 0); err != nil {
		t.Fatal(err)
	}
}

func TestFindingQualityRejectsUnrelatedOrFailedStep(t *testing.T) {
	d := testDB(t)
	defer d.Close()
	task, err := d.CreateTask("proof", "fixture", nil, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer d.DeleteTask(task.ID)
	ts := d.Exploration(task.ExplorationID)
	intent, err := ts.AddIntent(map[string]any{"summary": "validate"}, 1, nil, "planner")
	if err != nil {
		t.Fatal(err)
	}
	proof := completeFindingValidation()
	proof.EvidenceStepIDs = []int64{999999999}
	if err := validateFindingProof(proof, nil, ts, intent); err == nil {
		t.Fatal("unknown activity accepted")
	}
	id, err := ts.AppendActivity(db.Activity{NodeID: &intent, Kind: "tool_result", Tool: "Bash", IsError: true, Detail: "failed"})
	if err != nil {
		t.Fatal(err)
	}
	proof.EvidenceStepIDs = []int64{id}
	if err := validateFindingProof(proof, nil, ts, intent); err == nil {
		t.Fatal("failed tool result accepted")
	}
	if err := validateFindingProof(proof, nil, ts, 0); err == nil {
		t.Fatal("missing intent accepted")
	}
	id, err = ts.AppendActivity(db.Activity{NodeID: &intent, Kind: "tool_result", Tool: "report_finding", Detail: "finding recorded"})
	if err != nil {
		t.Fatal(err)
	}
	proof.EvidenceStepIDs = []int64{id}
	if err := validateFindingProof(proof, nil, ts, intent); err == nil {
		t.Fatal("prior finding receipt accepted as proof")
	}
}
