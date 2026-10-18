package report

import (
	"strings"
	"testing"
)

func TestLegacyCurlContinuationOmittedFromDelivery(t *testing.T) {
	for _, continuation := range []string{"\\", "`", "^"} {
		text := "captured proof\ncurl https://example.invalid " + continuation +
			"\n -H 'Cookie: must-not-survive'\nverified conclusion"
		got := reportBodyForExport(text)
		if strings.Contains(got, "must-not-survive") || strings.Contains(got, "curl https:") ||
			!strings.Contains(got, "captured proof") || !strings.Contains(got, "verified conclusion") {
			t.Fatalf("incorrect command filtering: %s", got)
		}
	}
}

func TestEvidenceCannotCloseItsMarkdownFence(t *testing.T) {
	got := findingEvidenceMarkdown("response\n```\n# captured heading")
	if !strings.Contains(got, "\n````\nresponse\n```\n# captured heading\n````") {
		t.Fatalf("evidence escaped its fence: %s", got)
	}
}
