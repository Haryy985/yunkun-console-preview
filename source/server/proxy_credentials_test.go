package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"yunkun/console/db"
	"yunkun/console/proxypool"
)

func changeProxyFixtureVault(t *testing.T, s *Server) {
	t.Helper()
	vault, err := proxypool.NewVault(bytes.Repeat([]byte{0x71}, 32))
	if err != nil {
		t.Fatal(err)
	}
	s.proxyPools.vault = vault
}

func TestProxyCredentialLossKeepsInventoryReadableAndRoutingClosed(t *testing.T) {
	s := newProxyFixture(t)
	id := createTestPool(t, s, "http://127.0.0.1:19999")
	pool, err := s.m.pg.ProxyPool(id)
	if err != nil {
		t.Fatal(err)
	}
	task, err := s.m.pg.CreateTaskWithOptions("credential fixture", "no external traffic",
		db.TaskCreateOptions{ProxyPoolID: &id, NetworkMode: "configured"})
	if err != nil {
		t.Fatal(err)
	}
	changeProxyFixtureVault(t, s)
	list := proxyAPICall(t, s, "GET", "/api/proxy-pools", nil)
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), `"credentials_available":false`) {
		t.Fatalf("missing credential diagnostic: %d %s", list.Code, list.Body)
	}
	for _, suffix := range []string{"/nodes", "/nodes?page=1&page_size=20", "/nodes/export", "/nodes/import-status", "/events"} {
		t.Run(suffix, func(t *testing.T) {
			path := fmt.Sprintf("/api/proxy-pools/%d%s", id, suffix)
			response := proxyAPICall(t, s, "GET", path, nil)
			if response.Code != http.StatusOK {
				t.Fatalf("metadata blocked by credentials: %d %s", response.Code, response.Body)
			}
			if strings.Contains(response.Body.String(), pool.Credentials) ||
				strings.Contains(response.Body.String(), "never-echo") {
				t.Fatal("metadata leaked credentials")
			}
			anonymous := httptest.NewRecorder()
			s.Handler().ServeHTTP(anonymous, httptest.NewRequest("GET", path, nil))
			if anonymous.Code != http.StatusUnauthorized {
				t.Fatalf("anonymous metadata access: %d", anonymous.Code)
			}
			for _, role := range []string{"viewer", "operator"} {
				if roleAllows(role, "GET", path) {
					t.Fatalf("%s can access pool metadata", role)
				}
			}
		})
	}
	inventory := proxyAPICall(t, s, "GET", fmt.Sprintf("/api/proxy-pools/%d/nodes?page=1", id), nil)
	var result struct {
		Total int `json:"total"`
	}
	if err := json.Unmarshal(inventory.Body.Bytes(), &result); err != nil || result.Total != 1 {
		t.Fatal("credential failure hid the existing node")
	}
	s.proxyPools.provider.Client.Transport = discoveryProtocolTransport(func(r *http.Request) (*http.Response, error) {
		t.Error("credential failure reached the network")
		return nil, fmt.Errorf("network forbidden in credential test")
	})
	for _, suffix := range []string{"/fetch", "/quota", "/nodes/1/verify"} {
		response := proxyAPICall(t, s, "POST", fmt.Sprintf("/api/proxy-pools/%d%s", id, suffix), nil)
		if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "代理池凭据不可用") {
			t.Fatalf("outbound operation did not fail closed: %d %s", response.Code, response.Body)
		}
	}
	if _, err := s.resolveProxyRoute(context.Background(), task.ID); err == nil {
		t.Fatal("credential failure allowed task routing")
	}
	response := proxyAPICall(t, s, "PUT", fmt.Sprintf("/api/proxy-pools/%d", id),
		saveProxyPoolRequest{Config: pool.Config, Secrets: &proxypool.Secrets{}})
	if response.Code != http.StatusConflict {
		t.Fatalf("replaced credentials on bound pool: %d %s", response.Code, response.Body)
	}
	after, _ := s.m.pg.ProxyPool(id)
	if after.Credentials != pool.Credentials {
		t.Fatal("failed replacement mutated stored ciphertext")
	}
}

func TestProxyCredentialReplacementPreservesNodesAndInvalidatesEvidence(t *testing.T) {
	for _, replacement := range []proxypool.Secrets{{}, {ProxyUsername: "replacement-user", ProxyPassword: "replacement-password"}} {
		name := "explicit-no-auth"
		if replacement.ProxyUsername != "" {
			name = "explicit-new-auth"
		}
		t.Run(name, func(t *testing.T) {
			s := newProxyFixture(t)
			id := createTestPool(t, s, "http://127.0.0.1:19999")
			if err := s.m.pg.StoreProxyNodes(id, []proxypool.Node{{Address: "socks5://127.0.0.1:19998"}}); err != nil {
				t.Fatal(err)
			}
			if _, err := s.m.pg.ImportProxyNodes(id, []proxypool.Node{{Address: "http://127.0.0.1:19997"}}); err != nil {
				t.Fatal(err)
			}
			before, err := s.m.pg.ProxyNodes(id)
			if err != nil || len(before) != 3 {
				t.Fatal("fixture inventory missing")
			}
			for _, node := range before {
				if err := s.m.pg.RecordProxyNodeVerification(id, node.ID, proxypool.VerificationResult{
					Mode: proxypool.VerifyConnectivity, Target: proxypool.BaiduVerifyURL, HTTPStatus: 200,
				}, "", 12); err != nil {
					t.Fatal(err)
				}
			}
			pool, _ := s.m.pg.ProxyPool(id)
			changeProxyFixtureVault(t, s)
			path := fmt.Sprintf("/api/proxy-pools/%d", id)
			implicit := proxyAPICall(t, s, "PUT", path, saveProxyPoolRequest{Config: pool.Config})
			if implicit.Code != http.StatusServiceUnavailable {
				t.Fatal("implicit empty-credential fallback was allowed")
			}
			unchanged, _ := s.m.pg.ProxyPool(id)
			if unchanged.Credentials != pool.Credentials {
				t.Fatal("failed implicit update changed credentials")
			}
			response := proxyAPICall(t, s, "PUT", path, saveProxyPoolRequest{Config: pool.Config, Secrets: &replacement})
			if response.Code != http.StatusOK {
				t.Fatalf("explicit replacement blocked: %d %s", response.Code, response.Body)
			}
			err = s.m.pg.RecordProxyNodeVerification(id, before[0].ID, proxypool.VerificationResult{
				Mode: proxypool.VerifyConnectivity, Target: proxypool.BaiduVerifyURL, HTTPStatus: 200,
			}, "", 12, pool.Credentials)
			if !errors.Is(err, db.ErrProxyPoolCredentialsChanged) {
				t.Fatal("old in-flight verification was allowed to restore stale evidence")
			}
			after, err := s.m.pg.ProxyNodes(id)
			if err != nil || len(after) != len(before) {
				t.Fatal("credential recovery deleted imported or discovered nodes")
			}
			for _, node := range after {
				found := false
				for _, old := range before {
					found = found || (old.ID == node.ID && old.Address == node.Address && old.Origin == node.Origin)
				}
				if !found || node.Enabled || node.VerifiedAt != nil || node.VerifiedIP != "" ||
					node.LastSuccessAt != nil || node.LastCheckedAt != nil || node.LastHTTPStatus != 0 ||
					node.LastVerifyMode != "" || node.LatencyMS != nil {
					t.Fatal("recovery changed identity or reused old verification evidence")
				}
			}
			updated, _ := s.m.pg.ProxyPool(id)
			got, err := s.proxyPools.vault.Open(updated.Credentials)
			if err != nil || got != replacement {
				t.Fatal("replacement credentials cannot be opened by current vault")
			}
			status := proxyAPICall(t, s, "GET", path+"/nodes/import-status", nil)
			if status.Code != http.StatusOK || !strings.Contains(status.Body.String(), `"pending":0`) {
				t.Fatal("recovery unexpectedly scheduled outbound verification")
			}
			list := proxyAPICall(t, s, "GET", "/api/proxy-pools", nil)
			if !strings.Contains(list.Body.String(), `"credentials_available":true`) ||
				strings.Contains(list.Body.String(), "replacement-password") {
				t.Fatal("recovered status missing or secret leaked")
			}
		})
	}
}

func TestProxyCredentialHealthyConfigUpdatePreservesEvidence(t *testing.T) {
	s := newProxyFixture(t)
	id := createTestPool(t, s, "http://127.0.0.1:19999")
	nodes, _ := s.m.pg.ProxyNodes(id)
	if err := s.m.pg.RecordProxyNodeVerification(id, nodes[0].ID, proxypool.VerificationResult{
		Mode: proxypool.VerifyConnectivity, Target: proxypool.BaiduVerifyURL, HTTPStatus: 200,
	}, "", 12); err != nil {
		t.Fatal(err)
	}
	pool, _ := s.m.pg.ProxyPool(id)
	pool.Config.Name = "renamed"
	response := proxyAPICall(t, s, "PUT", fmt.Sprintf("/api/proxy-pools/%d", id), saveProxyPoolRequest{Config: pool.Config})
	if response.Code != http.StatusOK {
		t.Fatal("healthy ordinary update failed")
	}
	after, _ := s.m.pg.ProxyNodes(id)
	if len(after) != 1 || after[0].ID != nodes[0].ID || !after[0].Enabled ||
		after[0].LastSuccessAt == nil || after[0].LastHTTPStatus != 200 {
		t.Fatal("ordinary config update invalidated inventory")
	}
}
