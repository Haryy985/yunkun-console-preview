package report

import (
	"strings"
	"testing"
)

func TestLegacyCurlContinuationOmittedFromDelivery(t *testing.T) {
	for _, prefix := range []string{"", "- ", "1. ", "> ", "`"} {
		for _, continuation := range []string{"\\", "`", "^"} {
			text := "captured proof\n" + prefix + "curl https://example.invalid " + continuation +
				"\n -H 'Cookie: must-not-survive'\nverified conclusion"
			got := reportBodyForExport(text)
			if strings.Contains(got, "must-not-survive") || strings.Contains(got, "curl https:") ||
				!strings.Contains(got, "captured proof") || !strings.Contains(got, "verified conclusion") {
				t.Fatalf("incorrect command filtering for prefix %q: %s", prefix, got)
			}
		}
	}
}

func TestLegacyCurlDeliveryPointsToCapturedBurpPacket(t *testing.T) {
	got := reportBodyForExport("## 复现\n\ncurl https://example.invalid/proof")
	if strings.Contains(got, "curl https://") ||
		!strings.Contains(got, "真实捕获请求包") {
		t.Fatalf("legacy curl was not replaced with a clear evidence notice: %s", got)
	}
}

func TestEvidenceCannotCloseItsMarkdownFence(t *testing.T) {
	got := findingEvidenceMarkdown("response\n```\n# captured heading")
	if !strings.Contains(got, "\n````\nresponse\n```\n# captured heading\n````") {
		t.Fatalf("evidence escaped its fence: %s", got)
	}
}
