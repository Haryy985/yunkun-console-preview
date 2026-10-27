package selfupdate

import (
	"net/http"
	"testing"
)

func TestNewClientRejectsInvalidExplicitProxy(t *testing.T) {
	client := NewClient("://not-a-proxy")
	tr, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatal("expected a dedicated transport")
	}
	req, err := http.NewRequest(http.MethodGet, latestURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tr.Proxy(req); err == nil {
		t.Fatal("invalid explicit proxy silently fell back to direct connection")
	}
}
