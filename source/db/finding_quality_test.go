package db

import "testing"

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
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			q := evaluateFindingQuality(RecordFindingInput{
				VulnClass: "access-control", Severity: tc.severity,
				Summary: tc.summary, Evidence: tc.evidence,
			}, nil)
			if q.QualityStatus != tc.wantStatus {
				t.Fatalf("quality status = %q, want %q", q.QualityStatus, tc.wantStatus)
			}
		})
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
