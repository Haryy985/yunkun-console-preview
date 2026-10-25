package server

import (
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"yunkun/console/proxypool"
)

func TestProxyImportCSVMultilineBOMAndLimit(t *testing.T) {
	var b bytes.Buffer
	b.WriteString("\ufeff")
	w := csv.NewWriter(&b)
	_ = w.Write([]string{"address", "last_error"})
	_ = w.Write([]string{"https://EXAMPLE.test:443", "failed\nTLS handshake"})
	_ = w.Write([]string{"https://example.test:443", "duplicate"})
	w.Flush()
	got, err := parseProxyImport(b.Bytes(), "http")
	if err != nil || len(got) != 1 || got[0] != "https://example.test:443" {
		t.Fatalf("%v %v", got, err)
	}
	var many strings.Builder
	for i := 0; i < 501; i++ {
		fmt.Fprintf(&many, "http://192.0.2.1:%d\n", 10000+i)
	}
	if _, err := parseProxyImport([]byte(many.String()), "http"); err == nil {
		t.Fatal("over-limit import accepted")
	}
	if _, err := parseProxyImport([]byte("address,error\n\"unterminated"), "http"); err == nil {
		t.Fatal("malformed CSV accepted")
	}
}

func TestProxyImportPreservesExistingNodesAndResumesVerification(t *testing.T) {
	s := newProxyFixture(t)
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/failure" {
			w.WriteHeader(503)
			return
		}
		_, _ = io.WriteString(w, `{"ip":"198.51.100.42"}`)
	}))
	defer proxy.Close()
	poolID := createTestPool(t, s, "http://127.0.0.1:9999")
	nodes, _ := s.m.pg.ProxyNodes(poolID)
	id := nodes[0].ID
	_, err := s.m.pg.Exec(`UPDATE proxy_nodes SET origin='fofa',enabled=true,verified_ip='198.51.100.1',
verified_at=now(),last_error='existing',success_count=7 WHERE id=$1`, id)
	if err != nil {
		t.Fatal(err)
	}
	imported, err := s.m.pg.ImportProxyNodes(poolID, []proxypool.Node{{Address: nodes[0].Address}, {Address: proxy.URL}})
	if err != nil || len(imported) != 1 {
		t.Fatalf("import: %v %v", imported, err)
	}
	existing, _ := s.m.pg.ProxyNode(poolID, id)
	if existing.Origin != "fofa" || !existing.Enabled || existing.LastError != "existing" ||
		existing.SuccessCount != 7 || existing.VerifiedIP != "198.51.100.1" {
		t.Fatal("duplicate import overwrote existing metadata")
	}
	newNode, _ := s.m.pg.ProxyNode(poolID, imported[0])
	if newNode.Enabled {
		t.Fatal("unverified import enabled")
	}
	p, _ := s.m.pg.ProxyPool(poolID)
	p.Config.VerifyURL = "http://echo.invalid/success"
	if _, err := s.m.pg.SaveProxyPool(poolID, p.Config, p.Credentials, false); err != nil {
		t.Fatal(err)
	}
	// Simulate an interrupted worker and expired claim before another worker
	// resumes. No browser or live discovery target participates in this test.
	_, err = s.m.pg.Exec(`UPDATE proxy_nodes SET import_verify_claimed_at=now()-interval '2 minutes' WHERE id=$1`, newNode.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !s.verifyNextImport(context.Background()) {
		t.Fatal("persisted import did not resume")
	}
	newNode, _ = s.m.pg.ProxyNode(poolID, newNode.ID)
	if !newNode.Enabled || newNode.VerifiedIP != "198.51.100.42" || newNode.LastSuccessAt == nil {
		t.Fatal("successful verification not enabled")
	}
	status := proxyAPICall(t, s, "GET", fmt.Sprintf("/api/proxy-pools/%d/nodes/import-status", poolID), nil)
	if status.Code != 200 || !strings.Contains(status.Body.String(), `"pending":0`) {
		t.Fatal(status.Body.String())
	}
	// Re-queue a disabled node and force a target response failure.
	p.Config.VerifyURL = "http://echo.invalid/failure"
	if _, err := s.m.pg.SaveProxyPool(poolID, p.Config, p.Credentials, false); err != nil {
		t.Fatal(err)
	}
	_, err = s.m.pg.Exec(`UPDATE proxy_nodes SET enabled=false,import_verify_pending=true,import_verify_claimed_at=NULL WHERE id=$1`, newNode.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !s.verifyNextImport(context.Background()) {
		t.Fatal("failed verification never finished")
	}
	newNode, _ = s.m.pg.ProxyNode(poolID, newNode.ID)
	if newNode.Enabled || newNode.LastError == "" || newNode.FailureCount != 1 {
		t.Fatal("failed imported node was enabled or lost diagnostic")
	}
	export := proxyAPICall(t, s, "GET", fmt.Sprintf("/api/proxy-pools/%d/nodes/export", poolID), nil)
	if export.Code != 200 || strings.Contains(export.Body.String(), "never-echo") {
		t.Fatal("export failed or leaked credentials")
	}
	addresses, err := parseProxyImport(export.Body.Bytes(), "http")
	if err != nil || len(addresses) != 2 {
		t.Fatalf("export roundtrip: %v %v", addresses, err)
	}
}

func TestProxyImportMultipartAndAuthentication(t *testing.T) {
	s := newProxyFixture(t)
	poolID := createTestPool(t, s, "http://127.0.0.1:9999")
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	part, _ := form.CreateFormFile("file", "nodes.txt")
	_, _ = io.WriteString(part, "http://192.0.2.1:8080")
	form.Close()
	path := fmt.Sprintf("/api/proxy-pools/%d/nodes/import", poolID)
	token, _ := signJWT(s.jwtKey)
	req := httptest.NewRequest("POST", path, &body)
	req.Header.Set("Content-Type", form.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("multipart import: %d %s", rec.Code, rec.Body)
	}
	for _, path := range []string{path, fmt.Sprintf("/api/proxy-pools/%d/nodes/export", poolID),
		fmt.Sprintf("/api/proxy-pools/%d/nodes/import-status", poolID)} {
		method := "GET"
		if strings.HasSuffix(path, "/import") {
			method = "POST"
		}
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, httptest.NewRequest(method, path, nil))
		if rec.Code != 401 {
			t.Fatalf("anonymous access: %s %d", path, rec.Code)
		}
	}
}
