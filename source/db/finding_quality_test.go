package db

import (
	"context"
	"testing"
)

func TestFindingQualityDefaultsRequireHumanReview(t *testing.T) {
	tests := []struct {
		name       string
		severity   string
		summary    string
		evidence   string
		wantStatus string
	}{
		{"medium is not auto-approved", SeverityMedium, "confirmed impact", "observed proof", FindingQualityNeedsMore},
		{"high is not auto-approved", SeverityHigh, "confirmed impact", "observed proof", FindingQualityNeedsMore},
		{"low is downgraded", SeverityLow, "minor observation", "observed proof", FindingQualityDowngrade},
		{"missing proof stays pending", SeverityMedium, "claim only", "", FindingQualityNeedsMore},
		{"CORS stays a lead", SeverityHigh, "CORS", "observed proof", FindingQualityBlocked},
		{"IDOR stays a lead", SeverityHigh, "IDOR", "observed proof", FindingQualityBlocked},
		{"BOLA stays a lead", SeverityHigh, "Broken Object Level Authorization", "observed proof", FindingQualityBlocked},
		{"incidental word is not excluded", SeverityMedium, "access control", "observed proof", FindingQualityNeedsMore},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			class := "access-control"
			if ExcludedFindingClass(tc.summary) != "" {
				class = tc.summary
			}
			q := evaluateFindingQuality(RecordFindingInput{
				VulnClass: class, Severity: tc.severity,
				Summary: tc.summary, Evidence: tc.evidence,
			}, nil)
			if q.QualityStatus != tc.wantStatus {
				t.Fatalf("quality status = %q, want %q", q.QualityStatus, tc.wantStatus)
			}
		})
	}
}

func TestFindingQualityDoesNotBlockIncidentalSummaryMentions(t *testing.T) {
	q := evaluateFindingQuality(RecordFindingInput{
		VulnClass: "SQL injection", Name: "controlled local fixture", Severity: SeverityHigh,
		Summary: "CORS was ruled out; separate proof requires review", Evidence: "observed proof",
	}, nil)
	if q.QualityStatus != FindingQualityNeedsMore {
		t.Fatalf("incidental mention changed the evidence review gate: %+v", q)
	}
}

func TestExcludedFindingClassUsesTokenAndPhraseBoundaries(t *testing.T) {
	for _, test := range []struct {
		input string
		want  string
	}{
		{"CORS misconfiguration", "CORS"},
		{"cross-origin resource sharing", "CORS"},
		{"IDOR", "IDOR"},
		{"insecure direct object reference", "IDOR"},
		{"BOLA", "BOLA"},
		{"broken object-level authorization", "BOLA"},
		{"access control in a corset product", ""},
		{"ordinary business logic", ""},
	} {
		if got := ExcludedFindingClass(test.input); got != test.want {
			t.Errorf("ExcludedFindingClass(%q)=%q want %q", test.input, got, test.want)
		}
	}
}

func TestFindingVerifiedNotificationCreatedOnceOnQualityPass(t *testing.T) {
	d := notifyTestDB(t)
	ctx := context.Background()
	task, err := d.CreateTask("质量通知测试", "本地 fixture", nil, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer d.DeleteTask(task.ID)
	finding, err := d.Exploration(task.ExplorationID).RecordFinding(ctx, RecordFindingInput{
		TaskID: task.ID, Worker: "test", VulnClass: "SQL injection", Name: "fixture",
		Severity: SeverityHigh, Summary: "controlled test", Evidence: "observed proof",
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = d.Exec(`DELETE FROM notification_events WHERE finding_id=$1`, finding.FindingID) })
	var count int
	if err := d.QueryRow(`SELECT count(*) FROM notification_events WHERE finding_id=$1 AND kind='finding_verified'`, finding.FindingID).Scan(&count); err != nil || count != 0 {
		t.Fatalf("unreviewed finding already notified: count=%d err=%v", count, err)
	}
	quality, err := d.GetFindingQuality(finding.FindingID)
	if err != nil || quality == nil {
		t.Fatalf("quality=%+v err=%v", quality, err)
	}
	quality.QualityStatus = FindingQualityPass
	quality.Reviewer = "fixture reviewer"
	quality.VerificationType = "controlled reproduction"
	quality.ProofEvidence = "proof"
	quality.VerificationEvidence = "retest"
	quality.VerificationRationale = "baseline compared"
	quality.ReproductionCount = 3
	quality.SuccessCount = 3
	quality.EvidenceIntegrity = "verified"
	for i := 0; i < 2; i++ {
		if err := d.SetFindingQuality(ctx, *quality); err != nil {
			t.Fatal(err)
		}
	}
	if err := d.QueryRow(`SELECT count(*) FROM notification_events WHERE finding_id=$1 AND kind='finding_verified'`, finding.FindingID).Scan(&count); err != nil || count != 1 {
		t.Fatalf("verified finding event count=%d err=%v, want one", count, err)
	}
}

func TestFindingDeliveryMetadataBlocksLowOrUnconfirmedRecords(t *testing.T) {
	for _, tc := range []struct {
		severity string
		status   string
		want     int
	}{
		{SeverityHigh, FindingConfirmed, 0},
		{SeverityLow, FindingConfirmed, 1},
		{SeverityHigh, FindingPending, 1},
		{SeverityHigh, FindingDuplicate, 1},
	} {
		if got := len(FindingDeliveryMetadataBlockers(7, tc.severity, tc.status)); got != tc.want {
			t.Errorf("severity=%s status=%s blockers=%d, want %d", tc.severity, tc.status, got, tc.want)
		}
	}
}

func TestRAGProposalRejectsCredentialLikeValues(t *testing.T) {
	for _, input := range []string{
		"Authorization: Bearer abcdefghijklmnopqrstuvwxyz",
		"Cookie: sessionid=abcdef1234567890",
		"password=SuperSecretValue123",
		"https://operator:secret-pass@example.test/api",
	} {
		if !containsRAGProposalSecret(input) {
			t.Errorf("secret pattern not detected: %q", input)
		}
	}
	for _, input := range []string{
		"Authorization: Bearer <REDACTED>",
		"Use a password manager and rotate API keys.",
		"Cookie: <captured cookie omitted>",
	} {
		if containsRAGProposalSecret(input) {
			t.Errorf("placeholder/instruction rejected: %q", input)
		}
	}
}
