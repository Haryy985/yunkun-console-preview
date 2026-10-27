// Package proxypool owns provider contracts. It never falls back to direct
// target traffic when a configured proxy is unavailable.
package proxypool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/tidwall/gjson"
)

type Config struct {
	Name                 string   `json:"name"`
	Kind                 string   `json:"kind"`
	Enabled              bool     `json:"enabled"`
	Protocol             string   `json:"protocol"`
	Addresses            []string `json:"addresses"`
	APIURL               string   `json:"api_url,omitempty"`
	ItemsPath            string   `json:"items_path,omitempty"`
	HostField            string   `json:"host_field,omitempty"`
	PortField            string   `json:"port_field,omitempty"`
	TTLSeconds           int      `json:"ttl_seconds"`
	FetchCount           int      `json:"fetch_count"`
	SafetySeconds        int      `json:"safety_seconds"`
	QuotaSupported       bool     `json:"quota_supported"`
	QuotaKeyStyle        string   `json:"quota_key_style,omitempty"`
	LowQuota             int64    `json:"low_quota"`
	VerifyURL            string   `json:"verify_url,omitempty"`
	ExpiryWarningSeconds int      `json:"expiry_warning_seconds"`
}

type Secrets struct {
	AppKey        string `json:"app_key,omitempty"`
	AppSecret     string `json:"app_secret,omitempty"`
	ProxyUsername string `json:"proxy_username,omitempty"`
	ProxyPassword string `json:"proxy_password,omitempty"`
	APIHeader     string `json:"api_header,omitempty"`
	APIValue      string `json:"api_value,omitempty"`
	FofaEmail     string `json:"fofa_email,omitempty"`
	FofaKey       string `json:"fofa_key,omitempty"`
	QuakeKey      string `json:"quake_key,omitempty"`
}

type Node struct {
	RuntimeCooldownUntil *time.Time `json:"runtime_cooldown_until,omitempty"`
	RuntimeError         string     `json:"runtime_error,omitempty"`
	ID                   int64      `json:"id"`
	PoolID               int64      `json:"pool_id"`
	Address              string     `json:"address"`
	DeclaredIP           string     `json:"declared_ip,omitempty"`
	ExpiresAt            *time.Time `json:"expires_at,omitempty"`
	VerifiedIP           string     `json:"verified_ip,omitempty"`
	VerifiedAt           *time.Time `json:"verified_at,omitempty"`
	LastError            string     `json:"last_error,omitempty"`
	Enabled              bool       `json:"enabled"`
	Origin               string     `json:"origin,omitempty"`
	LastCheckedAt        *time.Time `json:"last_checked_at,omitempty"`
	LatencyMS            *int       `json:"latency_ms,omitempty"`
	SuccessCount         int64      `json:"success_count"`
	FailureCount         int64      `json:"failure_count"`
	LastSuccessAt        *time.Time `json:"last_success_at,omitempty"`
	LastFailureAt        *time.Time `json:"last_failure_at,omitempty"`
	Notes                string     `json:"notes,omitempty"`
	DeletedAt            *time.Time `json:"deleted_at,omitempty"`
	LastVerifyMode       string     `json:"last_verify_mode,omitempty"`
	LastVerifyTarget     string     `json:"last_verify_target,omitempty"`
	LastHTTPStatus       int        `json:"last_http_status,omitempty"`
}

type Quota struct {
	Supported   bool       `json:"supported"`
	Unavailable bool       `json:"unavailable,omitempty"`
	Total       *int64     `json:"total,omitempty"`
	Remaining   *int64     `json:"remaining,omitempty"`
	CheckedAt   *time.Time `json:"checked_at,omitempty"`
	Error       string     `json:"error,omitempty"`
}

// HasFreshVerification accepts explicit connectivity evidence without claiming
// an exit IP. Legacy IP proofs remain valid until their original expiry.
func (n Node) HasFreshVerification(now time.Time) bool {
	if n.LastError != "" || (n.LastFailureAt != nil &&
		(n.LastSuccessAt == nil || n.LastSuccessAt.Before(*n.LastFailureAt))) {
		return false
	}
	cutoff := now.Add(-15 * time.Minute)
	if n.LastVerifyMode == VerifyConnectivity {
		return n.LastHTTPStatus == http.StatusOK && n.LastSuccessAt != nil && n.LastSuccessAt.After(cutoff)
	}
	return (n.LastVerifyMode == "" || n.LastVerifyMode == VerifyExitIP) &&
		net.ParseIP(n.VerifiedIP) != nil && n.VerifiedAt != nil && n.VerifiedAt.After(cutoff)
}

func (c *Config) Validate() error {
	c.Name = strings.TrimSpace(c.Name)
	if c.Name == "" || len([]rune(c.Name)) > 80 {
		return errors.New("name must contain 1-80 characters")
	}
	if c.Kind != "fixed" && c.Kind != "xiaoxiang" && c.Kind != "generic" {
		return errors.New("unsupported provider")
	}
	if c.Protocol == "" {
		c.Protocol = "http"
	}
	if c.Protocol != "http" && c.Protocol != "https" && c.Protocol != "socks5" {
		return errors.New("unsupported proxy protocol")
	}
	if c.FetchCount == 0 {
		c.FetchCount = 1
	}
	if c.ExpiryWarningSeconds == 0 {
		c.ExpiryWarningSeconds = 3600
	}
	if c.FetchCount < 1 || c.FetchCount > 100 || c.SafetySeconds < 0 || c.SafetySeconds > 3600 || c.LowQuota < 0 {
		return errors.New("invalid count, expiry margin or quota threshold")
	}
	if c.ExpiryWarningSeconds < 60 || c.ExpiryWarningSeconds > 30*24*3600 {
		return errors.New("invalid expiry warning window")
	}
	if c.Kind == "fixed" {
		if len(c.Addresses) > 100 {
			return errors.New("provide at most 100 proxy addresses")
		}
		for i, raw := range c.Addresses {
			u, err := ParseAddress(raw)
			if err != nil {
				return err
			}
			c.Addresses[i] = u.String()
		}
		c.QuotaSupported = false
	}
	if c.Kind == "xiaoxiang" {
		if c.Protocol == "https" {
			return errors.New("xiaoxiang supports http or socks5")
		}
		if c.QuotaKeyStyle == "" {
			c.QuotaKeyStyle = "app"
		}
		if c.QuotaKeyStyle != "app" && c.QuotaKeyStyle != "user" {
			return errors.New("invalid quota key style")
		}
	}
	if c.Kind == "generic" {
		u, err := url.Parse(c.APIURL)
		if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.Fragment != "" || u.RawQuery != "" {
			return errors.New("generic API must be an HTTPS URL without credentials, query or fragment")
		}
		if c.TTLSeconds < 10 || c.TTLSeconds > 86400*30 || c.TTLSeconds <= c.SafetySeconds {
			return errors.New("generic API needs an explicit TTL greater than the expiry margin")
		}
		if c.HostField == "" {
			c.HostField = "ip"
		}
		if c.PortField == "" {
			c.PortField = "port"
		}
		c.QuotaSupported = false
	}
	if c.VerifyURL != "" {
		if c.VerifyURL == "www.baidu.com" || c.VerifyURL == "www.baidu.com/" {
			c.VerifyURL = BaiduVerifyURL
		}
		u, err := url.Parse(c.VerifyURL)
		if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Hostname() == "" || u.User != nil || u.Fragment != "" {
			return errors.New("invalid verification URL")
		}
	}
	return nil
}

func ParseAddress(raw string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u == nil {
		return nil, errors.New("invalid proxy address")
	}
	if (u.Scheme != "http" && u.Scheme != "https" && u.Scheme != "socks5") ||
		u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return nil, errors.New("proxy address must be http/https/socks5 with credentials in the separate credential fields")
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil || port < 1 || port > 65535 {
		return nil, errors.New("proxy address requires a valid port")
	}
	u.Path = ""
	return u, nil
}

func Upstream(n Node, s Secrets) (*url.URL, error) {
	u, err := ParseAddress(n.Address)
	if err != nil {
		return nil, err
	}
	if s.ProxyUsername != "" || s.ProxyPassword != "" {
		u.User = url.UserPassword(s.ProxyUsername, s.ProxyPassword)
	}
	return u, nil
}

// Xiaoxiang uses application credentials for proxy authentication as well.
// Explicit proxy credentials take precedence for existing configurations.
func (c Config) Upstream(n Node, s Secrets) (*url.URL, error) {
	if c.Kind == "xiaoxiang" && s.ProxyUsername == "" && s.ProxyPassword == "" {
		s.ProxyUsername, s.ProxyPassword = s.AppKey, s.AppSecret
	}
	return Upstream(n, s)
}

type Provider struct {
	Client *http.Client
	// Only tests override this endpoint; public config cannot redirect app secrets.
	xiaoxiangBase string
}

func NewProvider() *Provider {
	return &Provider{Client: &http.Client{
		Timeout:       20 * time.Second,
		Transport:     &http.Transport{Proxy: nil, DialContext: (&net.Dialer{Timeout: 10 * time.Second}).DialContext, TLSHandshakeTimeout: 10 * time.Second},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}, xiaoxiangBase: "https://api.xiaoxiangdaili.com"}
}

func (p *Provider) get(ctx context.Context, endpoint string, s Secrets) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, errors.New("invalid provider request")
	}
	if s.APIHeader != "" {
		if strings.ContainsAny(s.APIHeader+s.APIValue, "\r\n") {
			return nil, errors.New("invalid API header")
		}
		req.Header.Set(s.APIHeader, s.APIValue)
	}
	resp, err := p.Client.Do(req)
	if err != nil {
		return nil, errors.New("provider request failed or timed out")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("provider HTTP status %d", resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, (1<<20)+1))
	if err != nil || len(b) > 1<<20 {
		return nil, errors.New("invalid or oversized provider response")
	}
	if !json.Valid(b) {
		return nil, errors.New("provider did not return valid JSON")
	}
	return b, nil
}

func (p *Provider) xURL(path string, s Secrets, style string) string {
	u, _ := url.Parse(p.xiaoxiangBase + path)
	q := u.Query()
	prefix := "app"
	if style == "user" {
		prefix = "user"
	}
	q.Set(prefix+"Key", s.AppKey)
	q.Set(prefix+"Secret", s.AppSecret)
	u.RawQuery = q.Encode()
	return u.String()
}

func providerOK(b []byte) error {
	code, err := strconv.ParseInt(gjson.GetBytes(b, "code").String(), 10, 64)
	if err != nil || code != 200 || gjson.GetBytes(b, "success").Type == gjson.False {
		return fmt.Errorf("provider rejected request (code %d)", gjson.GetBytes(b, "code").Int())
	}
	return nil
}

func (p *Provider) Fetch(ctx context.Context, c Config, s Secrets, now time.Time) ([]Node, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}
	if c.Kind == "fixed" {
		out := make([]Node, 0, len(c.Addresses))
		for _, addr := range c.Addresses {
			out = append(out, Node{Address: addr})
		}
		return out, nil
	}
	endpoint := c.APIURL
	if c.Kind == "xiaoxiang" {
		if s.AppKey == "" || s.AppSecret == "" {
			return nil, errors.New("provider credentials required")
		}
		u, _ := url.Parse(p.xURL("/ip/get", s, "app"))
		q := u.Query()
		q.Set("cnt", strconv.Itoa(c.FetchCount))
		q.Set("wt", "json")
		method := "http"
		if c.Protocol == "socks5" {
			method = "s5"
		}
		q.Set("method", method)
		q.Set("releaseAuto", "false")
		u.RawQuery = q.Encode()
		endpoint = u.String()
	}
	b, err := p.get(ctx, endpoint, s)
	if err != nil {
		return nil, err
	}
	path := c.ItemsPath
	if c.Kind == "xiaoxiang" {
		if err := providerOK(b); err != nil {
			return nil, err
		}
		path = "data"
	}
	items := gjson.ParseBytes(b)
	if path != "" {
		items = gjson.GetBytes(b, path)
	}
	if !items.IsArray() || len(items.Array()) == 0 || len(items.Array()) > 100 {
		return nil, errors.New("provider returned an empty or invalid node list")
	}
	nodes := make([]Node, 0, len(items.Array()))
	for _, item := range items.Array() {
		hostField, portField := c.HostField, c.PortField
		if c.Kind == "xiaoxiang" {
			hostField, portField = "ip", "port"
		}
		host := item.Get(hostField).String()
		port, err := strconv.ParseInt(item.Get(portField).String(), 10, 64)
		if err != nil || host == "" || strings.ContainsAny(host, "/@?# \r\n") || port < 1 || port > 65535 {
			return nil, errors.New("provider returned an invalid proxy endpoint")
		}
		n := Node{Address: c.Protocol + "://" + net.JoinHostPort(host, strconv.FormatInt(port, 10))}
		expires := now.Add(time.Duration(c.TTLSeconds) * time.Second)
		if c.Kind == "xiaoxiang" {
			start, err := time.ParseInLocation("2006-01-02 15:04:05", item.Get("startTime").String(), time.FixedZone("Asia/Shanghai", 8*3600))
			duration, durationErr := strconv.ParseInt(item.Get("during").String(), 10, 64)
			if err != nil || durationErr != nil || duration < 1 || duration > 43200 {
				return nil, errors.New("provider returned invalid expiry data")
			}
			expires = start.Add(time.Duration(duration) * time.Minute)
			n.DeclaredIP = item.Get("realIp").String()
		}
		if !expires.After(now.Add(time.Duration(c.SafetySeconds) * time.Second)) {
			continue
		}
		if _, err := ParseAddress(n.Address); err != nil {
			return nil, err
		}
		n.ExpiresAt = &expires
		nodes = append(nodes, n)
	}
	if len(nodes) == 0 {
		return nil, errors.New("all returned nodes are expired or near expiry")
	}
	return nodes, nil
}

func (p *Provider) Remaining(ctx context.Context, c Config, s Secrets) (Quota, error) {
	q := Quota{Supported: c.Kind == "xiaoxiang" && c.QuotaSupported}
	if !q.Supported {
		return q, nil
	}
	if err := c.Validate(); err != nil {
		return q, err
	}
	if s.AppKey == "" || s.AppSecret == "" {
		return q, errors.New("provider credentials required")
	}
	b, err := p.get(ctx, p.xURL("/ip/remaining", s, c.QuotaKeyStyle), s)
	if err != nil {
		return q, err
	}
	if err := providerOK(b); err != nil {
		return q, err
	}
	now := time.Now().UTC()
	data := gjson.GetBytes(b, "data")
	// A successful response with explicit null means no quota data is available.
	// It does not establish zero balance or prove the subscription type.
	if data.Type == gjson.Null && data.Raw == "null" {
		q.Unavailable, q.CheckedAt = true, &now
		return q, nil
	}
	total, remaining := gjson.GetBytes(b, "data.total"), gjson.GetBytes(b, "data.remaining")
	t, totalErr := strconv.ParseInt(total.String(), 10, 64)
	r, remainingErr := strconv.ParseInt(remaining.String(), 10, 64)
	if total.Type != gjson.Number || remaining.Type != gjson.Number || totalErr != nil || remainingErr != nil || t < 0 || r < 0 || r > t {
		return q, errors.New("provider returned invalid quota")
	}
	q.Total, q.Remaining, q.CheckedAt = &t, &r, &now
	return q, nil
}
