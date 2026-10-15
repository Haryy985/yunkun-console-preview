package traffic

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHTTPRecorderRejectsFailedBlobWrite(t *testing.T) {
	dir := t.TempDir()
	tr, err := Open(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	defer tr.Close()
	// A file at the bucket root deterministically prevents blob creation.
	if err := os.WriteFile(filepath.Join(dir, "_blobs", "sha256"), []byte("blocked"), 0600); err != nil {
		t.Fatal(err)
	}
	req, _ := http.NewRequest("POST", "http://fixture.invalid/", nil)
	resp := &http.Response{StatusCode: 200, Header: make(http.Header)}
	id, err := tr.RecordHTTP(req, resp, nil, []byte(strings.Repeat("x", maxInlineBody+1)))
	if err == nil || id != "" {
		t.Fatal("incomplete storage advertised a traffic ID")
	}
	count, err := tr.Count()
	if err != nil || count != 0 {
		t.Fatal("incomplete record was indexed", err)
	}
}

func TestStorageOnlyRecorderSurvivesReopen(t *testing.T) {
	dir := t.TempDir()
	tr, err := Open(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	req, _ := http.NewRequest("GET", "http://fixture.invalid/", nil)
	resp := &http.Response{StatusCode: 200, Header: make(http.Header)}
	first, err := tr.RecordHTTP(req, resp, nil, []byte("one"))
	if err != nil {
		t.Fatal(err)
	}
	if err := tr.Close(); err != nil {
		t.Fatal(err)
	}
	tr, err = Open(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	defer tr.Close()
	second, err := tr.RecordHTTP(req, resp, nil, []byte("two"))
	if err != nil || first == second {
		t.Fatal("capture ID reused after reopen", err)
	}
	if tr.ProxyAddr() != "" || tr.Start() == nil {
		t.Fatal("store-only mode opened a listener")
	}
	_, body, err := tr.Get(first)
	if err != nil || !strings.Contains(body, "one") {
		t.Fatal("previous capture lost", err)
	}
}
