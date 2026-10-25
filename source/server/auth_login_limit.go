package server

import (
	"net"
	"strings"
	"sync"
	"time"
)

const (
	loginWindow    = 10 * time.Minute
	loginIPLimit   = 30
	loginUserLimit = 8
	loginMaxKeys   = 4096
)

type loginWindowEntry struct {
	start time.Time
	count int
}

// authLoginLimiter charges attempts before database access or bcrypt. It uses
// the connection peer, never an unauthenticated forwarded-IP header.
type authLoginLimiter struct {
	mu      sync.Mutex
	entries map[string]loginWindowEntry
}

func (l *authLoginLimiter) allow(remoteAddr, username string, now time.Time) time.Duration {
	ip, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		ip = remoteAddr
	}
	if ip == "" {
		ip = "unknown"
	}
	user := strings.ToLower(strings.TrimSpace(username))
	if user == "" {
		user = "unknown"
	}
	keys := [2]string{"ip:" + ip, "user:" + user}
	limits := [2]int{loginIPLimit, loginUserLimit}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.entries == nil {
		l.entries = make(map[string]loginWindowEntry)
	}
	for key, entry := range l.entries {
		if !now.Before(entry.start.Add(loginWindow)) {
			delete(l.entries, key)
		}
	}
	var retry time.Duration
	for i, key := range keys {
		if entry := l.entries[key]; entry.count >= limits[i] {
			until := entry.start.Add(loginWindow).Sub(now)
			if until > retry {
				retry = until
			}
		}
	}
	if retry > 0 {
		return retry
	}
	missing := 0
	for _, key := range keys {
		if _, exists := l.entries[key]; !exists {
			missing++
		}
	}
	if len(l.entries)+missing > loginMaxKeys {
		// Never evict a live lockout in response to attacker-controlled names.
		return loginWindow
	}
	for _, key := range keys {
		entry := l.entries[key]
		if entry.count == 0 {
			entry.start = now
		}
		entry.count++
		l.entries[key] = entry
	}
	return 0
}
