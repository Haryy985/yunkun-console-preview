package agent

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"yunkun/console/db"
)

func TestProviderIntelQueryUsesProviderSyntax(t *testing.T) {
	tests := []struct {
		provider string
		want     string
	}{
		{"fofa", `domain="example.test"&&ip="192.0.2.4"&&port="8443"`},
		{"quake", `domain:"example.test" AND ip:"192.0.2.4" AND port:"8443"`},
		{"hunter", `domain.suffix="example.test" && ip="192.0.2.4" && port="8443"`},
		{"zoomeye", `domain="example.test" AND ip="192.0.2.4" AND port="8443"`},
	}
	for _, test := range tests {
		got, err := providerIntelQuery(test.provider, "example.test", "192.0.2.4", 8443)
		if err != nil || got != test.want {
			t.Errorf("%s query = %q, %v; want %q", test.provider, got, err, test.want)
		}
	}
	if _, err := providerIntelQuery("fofa", "", "", 0); err == nil {
		t.Fatal("empty query unexpectedly accepted")
	}
	if _, err := providerIntelQuery("fofa", `example.test"||ip="8.8.8.8`, "", 0); err == nil {
		t.Fatal("query expression injection unexpectedly accepted")
	}
}

func TestIntelScopeMatchingIsConservative(t *testing.T) {
	scopes := []db.TaskScope{
		{Kind: "root_domain", Domain: "example.test"},
		{Kind: "ip", Net: "192.0.2.0/24"},
		{Kind: "subdomain", Domain: "exact.other.test"},
	}
	for _, domain := range []string{"example.test", "a.example.test", "exact.other.test"} {
		if !intelDomainInScope(domain, scopes) {
			t.Errorf("%s should be in scope", domain)
		}
	}
	for _, domain := range []string{"notexample.test", "other.test", "child.exact.other.test"} {
		if intelDomainInScope(domain, scopes) {
			t.Errorf("%s must remain out of scope", domain)
		}
	}
	if !intelIPInScope("192.0.2.17", scopes) || intelIPInScope("192.0.3.1", scopes) {
		t.Fatal("CIDR scope matching failed")
	}
	if !intelCandidateInScope(intelCandidate{Domain: "https://a.example.test:8443/path"}, scopes) {
		t.Fatal("in-scope URL candidate was rejected")
	}
	if intelCandidateInScope(intelCandidate{Domain: "https://example.invalid"}, scopes) {
		t.Fatal("out-of-scope URL candidate was accepted")
	}
}

type intelRoundTripper func(*http.Request) (*http.Response, error)

func (f intelRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestFOFAAdapterParsesOnlyNormalizedCandidateFields(t *testing.T) {
	client := &http.Client{Transport: intelRoundTripper(func(request *http.Request) (*http.Response, error) {
		if request.URL.Host != "fofa.info" || request.URL.Query().Get("key") != "test-key" {
			t.Fatalf("unexpected request URL: %s", request.URL.Redacted())
		}
		body := `{"results":[["a.example.test","192.0.2.4",443,"https","Example"],["b.example.test","192.0.2.5",80,"http","B"]],"size":2}`
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})}
	rows, total, retry, err := searchFOFA(context.Background(), client, `domain="example.test"`, 1, 20,
		intelCredentials{email: "operator@example.test", key: "test-key"})
	if err != nil || retry || total != float64(2) || len(rows) != 2 {
		t.Fatalf("rows=%+v total=%v retry=%v err=%v", rows, total, retry, err)
	}
	if rows[0].Provider != "fofa" || rows[0].Domain != "a.example.test" || rows[0].IP != "192.0.2.4" {
		t.Fatalf("unexpected normalized result: %+v", rows[0])
	}
}

func TestFOFAParameterFailureDoesNotRetryAccountsOrLeakSecrets(t *testing.T) {
	client := &http.Client{Transport: intelRoundTripper(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header),
			Body: io.NopCloser(strings.NewReader(`{"error":true,"error_code":-4,"errmsg":"[-4] 参数错误 secret-fixture custom-query"}`))}, nil
	})}
	_, _, retry, err := searchFOFA(context.Background(), client, "custom-query", 3, 100,
		intelCredentials{key: "secret-fixture"})
	if err == nil || retry || !strings.Contains(err.Error(), "参数错误") ||
		strings.Contains(err.Error(), "secret-fixture") || strings.Contains(err.Error(), "custom-query") {
		t.Fatalf("unsafe or retryable parameter failure: retry=%v err=%v", retry, err)
	}
}
