package middleware

import (
	"net/http"
	"strings"
	"sync"
	"time"
)

type RateLimiter struct {
	mu        sync.Mutex
	requests  map[string][]time.Time
	maxReqs   int
	windowSec int
}

func NewRateLimiter(maxReqs int, windowSec int) *RateLimiter {
	return &RateLimiter{
		requests:  make(map[string][]time.Time),
		maxReqs:   maxReqs,
		windowSec: windowSec,
	}
}

func (r *RateLimiter) Wrap(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		ip := getClientIP(req)
		if !r.allow(ip) {
			http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
			return
		}
		next(w, req)
	}
}

func (r *RateLimiter) allow(ip string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	window := now.Add(-time.Duration(r.windowSec) * time.Second)

	reqs := r.requests[ip]
	var valid []time.Time
	for _, t := range reqs {
		if t.After(window) {
			valid = append(valid, t)
		}
	}

	if len(valid) >= r.maxReqs {
		r.requests[ip] = valid
		return false
	}

	r.requests[ip] = append(valid, now)
	return true
}

func getClientIP(req *http.Request) string {
	xff := req.Header.Get("X-Forwarded-For")
	if xff != "" {
		parts := strings.Split(xff, ",")
		return strings.TrimSpace(parts[0])
	}
	return req.RemoteAddr
}
