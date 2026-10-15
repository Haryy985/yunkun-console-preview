package proxypool

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestXiaoxiangFetchAndQuota(t *testing.T) {
	now := time.Date(2026, 10, 13, 10, 0, 0, 0, time.FixedZone("CST", 8*3600))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("appKey") != "key" || r.URL.Query().Get("appSecret") != "secret" {
			t.Error("missing credentials")
		}
		switch r.URL.Path {
		case "/ip/get":
			if r.URL.Query().Get("releaseAuto") != "false" || r.URL.Query().Get("method") != "s5" || r.URL.Query().Get("cnt") != "2" || r.URL.Query().Get("wt") != "json" {
				t.Error("unsafe extraction parameters")
			}
			_, _ = w.Write([]byte(`{"code":200,"success":true,"data":[{"ip":"192.0.2.1","port":1080,"realIp":"198.51.100.1","startTime":"2026-10-13 10:00:00","during":2}]}`))
		case "/ip/remaining":
			_, _ = w.Write([]byte(`{"code":200,"data":{"total":500,"remaining":260}}`))
		default:
			t.Error("unexpected path")
		}
	}))
	defer server.Close()
	p := NewProvider()
	p.xiaoxiangBase = server.URL
	c := Config{Name: "test", Kind: "xiaoxiang", Protocol: "socks5", FetchCount: 2, SafetySeconds: 15, QuotaSupported: true}
	s := Secrets{AppKey: "key", AppSecret: "secret"}
	nodes, err := p.Fetch(context.Background(), c, s, now)
	if err != nil || len(nodes) != 1 {
		t.Fatalf("fetch: %v %+v", err, nodes)
	}
	if nodes[0].Address != "socks5://192.0.2.1:1080" || nodes[0].VerifiedIP != "" || nodes[0].DeclaredIP != "198.51.100.1" || !nodes[0].ExpiresAt.Equal(now.Add(2*time.Minute)) {
		t.Fatalf("bad node: %+v", nodes[0])
	}
	q, err := p.Remaining(context.Background(), c, s)
	if err != nil || q.Remaining == nil || *q.Remaining != 260 || q.CheckedAt == nil {
		t.Fatalf("quota: %v %+v", err, q)
	}
	if _, err := p.Fetch(context.Background(), c, s, now.Add(110*time.Second)); err == nil {
		t.Fatal("near-expiry accepted")
	}
}

func TestXiaoxiangQuotaResponseStates(t *testing.T) {
	for _, tt := range []struct {
		name        string
		body        string
		unavailable bool
		wantError   bool
	}{
		{"real empty response", `{"code":200,"success":true,"data":null,"msg":"暂无承载数据"}`, true, false},
		{"zero balance", `{"code":200,"success":true,"data":{"total":500,"remaining":0}}`, false, false},
		{"zero total", `{"code":200,"data":{"total":0,"remaining":0}}`, false, false},
		{"missing data", `{"code":200}`, false, true},
		{"empty object", `{"code":200,"data":{}}`, false, true},
		{"null counter", `{"code":200,"data":{"total":500,"remaining":null}}`, false, true},
		{"string counter", `{"code":200,"data":{"total":500,"remaining":"260"}}`, false, true},
		{"overflow", `{"code":200,"data":{"total":9223372036854775808,"remaining":1}}`, false, true},
		{"remaining over total", `{"code":200,"data":{"total":1,"remaining":2}}`, false, true},
		{"negative", `{"code":200,"data":{"total":500,"remaining":-1}}`, false, true},
		{"fractional", `{"code":200,"data":{"total":500,"remaining":1.5}}`, false, true},
		{"rejected empty", `{"code":200,"success":false,"data":null}`, false, true},
		{"expired", `{"code":1003,"data":null,"msg":"private-secret"}`, false, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/ip/remaining" || r.URL.Query().Get("userKey") != "key" || r.URL.Query().Get("userSecret") != "private-secret" || r.URL.Query().Has("appKey") {
					t.Error("incorrect quota authentication")
				}
				_, _ = w.Write([]byte(tt.body))
			}))
			defer server.Close()
			p := NewProvider()
			p.xiaoxiangBase = server.URL
			q, err := p.Remaining(context.Background(), Config{Name: "test", Kind: "xiaoxiang", QuotaSupported: true, QuotaKeyStyle: "user"}, Secrets{AppKey: "key", AppSecret: "private-secret"})
			if (err != nil) != tt.wantError {
				t.Fatalf("unexpected error: %v", err)
			}
			if err != nil {
				if strings.Contains(err.Error(), "private-secret") || q.CheckedAt != nil {
					t.Fatal("unsafe error or failed query marked successful")
				}
				return
			}
			if !q.Supported || q.Unavailable != tt.unavailable || q.CheckedAt == nil || q.Error != "" {
				t.Fatalf("incorrect quota state: %+v", q)
			}
			if tt.unavailable {
				if q.Total != nil || q.Remaining != nil {
					t.Fatal("unavailable quota fabricated a balance")
				}
			} else if q.Remaining == nil || *q.Remaining != 0 {
				t.Fatal("zero quota not preserved")
			}
		})
	}
}

func TestXiaoxiangProxyAuthentication(t *testing.T) {
	for _, protocol := range []string{"http", "socks5"} {
		for _, tt := range []struct {
			name    string
			kind    string
			secrets Secrets
			user    string
			pass    string
		}{
			{"application credentials", "xiaoxiang", Secrets{AppKey: "app-user", AppSecret: "app-password"}, "app-user", "app-password"},
			{"explicit credentials", "xiaoxiang", Secrets{AppKey: "app-user", AppSecret: "app-password", ProxyUsername: "proxy-user", ProxyPassword: "proxy-password"}, "proxy-user", "proxy-password"},
			{"fixed remains unchanged", "fixed", Secrets{AppKey: "app-user", AppSecret: "app-password"}, "", ""},
		} {
			t.Run(protocol+"/"+tt.name, func(t *testing.T) {
				u, err := (Config{Kind: tt.kind}).Upstream(Node{Address: protocol + "://192.0.2.1:1080"}, tt.secrets)
				if err != nil {
					t.Fatal(err)
				}
				if tt.user == "" {
					if u.User != nil {
						t.Fatal("application credentials used for unrelated provider")
					}
					return
				}
				password, ok := u.User.Password()
				if u.User.Username() != tt.user || !ok || password != tt.pass {
					t.Fatal("wrong proxy authentication")
				}
			})
		}
	}
}

func TestProviderErrorsAndNoSecretRedirect(t *testing.T) {
	var body string
	var redirect bool
	calls := 0
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(200) }))
	defer destination.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if redirect {
			http.Redirect(w, r, destination.URL, http.StatusFound)
			return
		}
		_, _ = w.Write([]byte(body))
	}))
	p := NewProvider()
	p.xiaoxiangBase = server.URL
	c := Config{Name: "test", Kind: "xiaoxiang", Protocol: "http", QuotaSupported: true}
	s := Secrets{AppKey: "private-key", AppSecret: "private-secret"}
	for _, b := range []string{`invalid`, `{"code":401,"msg":"private-secret"}`, `{"code":200,"data":[]}`, `{"code":200,"data":[{"ip":"x","port":0}]}`, `{"code":200,"data":[{"ip":"x","port":8080.5}]}`, strings.Repeat("x", (1<<20)+1)} {
		body = b
		_, err := p.Fetch(context.Background(), c, s, time.Now())
		if err == nil || strings.Contains(err.Error(), "private") {
			t.Fatalf("unsafe error: %v", err)
		}
	}
	body = `{"code":200,"data":{"remaining":-1,"total":500}}`
	if _, err := p.Remaining(context.Background(), c, s); err == nil {
		t.Fatal("negative quota accepted")
	}
	body = `{"code":200,"data":{"remaining":1.5,"total":500}}`
	if _, err := p.Remaining(context.Background(), c, s); err == nil {
		t.Fatal("fractional quota accepted")
	}
	redirect = true
	if _, err := p.Remaining(context.Background(), c, s); err == nil || calls != 0 {
		t.Fatal("provider redirect followed")
	}
	server.Close()
	if _, err := p.Remaining(context.Background(), c, s); err == nil || strings.Contains(err.Error(), "private") {
		t.Fatalf("URL leaked: %v", err)
	}
}

func TestGenericAndFixedValidation(t *testing.T) {
	c := Config{Name: "static", Kind: "fixed", Addresses: []string{"socks5://[::1]:1080"}}
	nodes, err := NewProvider().Fetch(context.Background(), c, Secrets{}, time.Now())
	if err != nil || len(nodes) != 1 || nodes[0].ExpiresAt != nil {
		t.Fatalf("fixed: %v", err)
	}
	for _, addr := range []string{"http://user:secret@localhost:80", "http://localhost", "file:///tmp/x", "http://x:70000", "http://x:80/?token=secret"} {
		if _, err := ParseAddress(addr); err == nil {
			t.Fatalf("accepted %s", addr)
		}
	}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-API-Key") != "token" {
			t.Error("missing API header")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"result": map[string]any{"nodes": []map[string]any{{"host": "192.0.2.2", "proxy_port": 9000}}}})
	}))
	defer server.Close()
	p := NewProvider()
	p.Client = server.Client()
	now := time.Now()
	nodes, err = p.Fetch(context.Background(), Config{Name: "generic", Kind: "generic", APIURL: server.URL, ItemsPath: "result.nodes", HostField: "host", PortField: "proxy_port", TTLSeconds: 90}, Secrets{APIHeader: "X-API-Key", APIValue: "token"}, now)
	if err != nil || len(nodes) != 1 || !nodes[0].ExpiresAt.Equal(now.Add(90*time.Second)) {
		t.Fatalf("generic: %v %+v", err, nodes)
	}
}

func TestVaultTamperAndPersistence(t *testing.T) {
	v, err := LoadVault(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	want := Secrets{AppKey: "key", AppSecret: "secret", ProxyPassword: "password"}
	encrypted, err := v.Seal(want)
	if err != nil || strings.Contains(encrypted, "password") {
		t.Fatalf("seal: %v", err)
	}
	got, err := v.Open(encrypted)
	if err != nil || got != want {
		t.Fatalf("roundtrip: %v", err)
	}
	tampered := []byte(encrypted)
	tampered[10] ^= 1
	if _, err := v.Open(string(tampered)); err == nil {
		t.Fatal("tamper accepted")
	}
	dir := t.TempDir()
	first, err := LoadVault(dir)
	if err != nil {
		t.Fatal(err)
	}
	encrypted, _ = first.Seal(want)
	second, err := LoadVault(dir)
	if err != nil {
		t.Fatal(err)
	}
	got, err = second.Open(encrypted)
	if err != nil || got != want {
		t.Fatal("key not persisted")
	}
}
