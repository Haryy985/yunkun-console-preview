package db

import (
	"context"
	"testing"
)

func TestFindingCreatedNotificationIsImmediate(t *testing.T) {
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
	if err := d.QueryRow(`SELECT count(*) FROM notification_events WHERE finding_id=$1 AND kind='finding_created'`, finding.FindingID).Scan(&count); err != nil || count != 1 {
		t.Fatalf("finding creation event missing: count=%d err=%v", count, err)
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
