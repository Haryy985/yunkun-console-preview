package proxypool

import (
	"testing"
	"time"
)

func TestNodeFreshVerification(t *testing.T) {
	now := time.Now()
	recent, expired, failed := now.Add(-time.Minute), now.Add(-16*time.Minute), now.Add(-30*time.Second)
	for _, test := range []struct {
		name string
		node Node
		want bool
	}{
		{"connectivity without IP", Node{LastVerifyMode: VerifyConnectivity, LastHTTPStatus: 200, LastSuccessAt: &recent}, true},
		{"expired connectivity", Node{LastVerifyMode: VerifyConnectivity, LastHTTPStatus: 200, LastSuccessAt: &expired}, false},
		{"rejected response", Node{LastVerifyMode: VerifyConnectivity, LastHTTPStatus: 403, LastSuccessAt: &recent}, false},
		{"failed after success", Node{LastVerifyMode: VerifyConnectivity, LastHTTPStatus: 200, LastSuccessAt: &recent, LastFailureAt: &failed}, false},
		{"error after success", Node{LastVerifyMode: VerifyConnectivity, LastHTTPStatus: 200, LastSuccessAt: &recent, LastError: "failed"}, false},
		{"legacy IP", Node{VerifiedIP: "198.51.100.42", VerifiedAt: &recent}, true},
		{"expired IP", Node{LastVerifyMode: VerifyExitIP, VerifiedIP: "198.51.100.42", VerifiedAt: &expired}, false},
		{"invalid IP", Node{VerifiedIP: "not-an-ip", VerifiedAt: &recent}, false},
		{"no evidence", Node{}, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := test.node.HasFreshVerification(now); got != test.want {
				t.Fatalf("fresh=%v, want %v", got, test.want)
			}
		})
	}
}
