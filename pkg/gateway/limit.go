package gateway

import (
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/anyclaw/anyclaw/pkg/config"
)

type rateLimiter struct {
	mu      sync.Mutex
	rpm     int
	entries map[string]*rateEntry
	trusted []*net.IPNet
}

type rateEntry struct {
	windowStart time.Time
	count       int
}

func newRateLimiter(cfg *config.SecurityConfig) *rateLimiter {
	rl := &rateLimiter{entries: map[string]*rateEntry{}}
	if cfg == nil {
		return rl
	}
	rl.rpm = cfg.RateLimitRPM
	for _, cidr := range cfg.TrustedCIDRs {
		_, network, err := net.ParseCIDR(strings.TrimSpace(cidr))
		if err == nil && network != nil {
			rl.trusted = append(rl.trusted, network)
		}
	}
	return rl
}

func (r *rateLimiter) Wrap(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		if r == nil || r.rpm <= 0 || r.isTrusted(req.RemoteAddr) {
			next(w, req)
			return
		}
		allowed, remaining := r.allow(req.RemoteAddr)
		w.Header().Set("X-RateLimit-Limit", strconv.Itoa(r.rpm))
		w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(remaining))
		if !allowed {
			http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
			return
		}
		next(w, req)
	}
}

func (r *rateLimiter) allow(remoteAddr string) (bool, int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := remoteAddr
	now := time.Now().UTC()
	entry, ok := r.entries[key]
	if !ok || now.Sub(entry.windowStart) >= time.Minute {
		entry = &rateEntry{windowStart: now, count: 0}
		r.entries[key] = entry
	}
	if entry.count >= r.rpm {
		return false, 0
	}
	entry.count++
	return true, max(0, r.rpm-entry.count)
}

func (r *rateLimiter) isTrusted(remoteAddr string) bool {
	if len(r.trusted) == 0 {
		return false
	}
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	ip := net.ParseIP(strings.TrimSpace(host))
	if ip == nil {
		return false
	}
	for _, network := range r.trusted {
		if network.Contains(ip) {
			return true
		}
	}
	return false
}
