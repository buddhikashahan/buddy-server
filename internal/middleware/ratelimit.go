package middleware

import (
	"net"
	"net/http"
	"sync"
	"time"

	"buddy/server/pkg/response"

	"golang.org/x/time/rate"
)

type clientBucket struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// RateLimiter implements an IP-based token bucket rate limiter.
type RateLimiter struct {
	mu      sync.Mutex
	clients map[string]*clientBucket
	rps     rate.Limit
	burst   int
}

// NewRateLimiter creates and starts a RateLimiter with periodic memory cleanup.
func NewRateLimiter(rps int, burst int) *RateLimiter {
	rl := &RateLimiter{
		clients: make(map[string]*clientBucket),
		rps:     rate.Limit(rps),
		burst:   burst,
	}

	go rl.cleanupOldEntries(3 * time.Minute)
	return rl
}

func (rl *RateLimiter) getLimiter(ip string) *rate.Limiter {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	bucket, exists := rl.clients[ip]
	if !exists {
		limiter := rate.NewLimiter(rl.rps, rl.burst)
		rl.clients[ip] = &clientBucket{limiter: limiter, lastSeen: time.Now()}
		return limiter
	}

	bucket.lastSeen = time.Now()
	return bucket.limiter
}

func (rl *RateLimiter) cleanupOldEntries(ttl time.Duration) {
	ticker := time.NewTicker(ttl)
	for range ticker.C {
		rl.mu.Lock()
		cutoff := time.Now().Add(-ttl)
		for ip, b := range rl.clients {
			if b.lastSeen.Before(cutoff) {
				delete(rl.clients, ip)
			}
		}
		rl.mu.Unlock()
	}
}

// getClientIP extracts the client IP for rate-limit bucketing.
//
// SECURITY: this deliberately does NOT re-parse X-Forwarded-For/X-Real-IP itself.
// Those headers are attacker-controlled on any request that doesn't pass through a
// trusted proxy, and honoring an arbitrary client-supplied value here would let a
// single caller bypass per-IP rate limiting entirely by sending a different header
// value on every request. Client IP resolution is centralized in chi's RealIP
// middleware (registered ahead of this one in cmd/api/main.go), which already
// mutates r.RemoteAddr for us — this function simply reads that resolved value.
// Chi's RealIP trusts X-Forwarded-For/X-Real-IP too, so this server must still be
// deployed behind a proxy/load balancer (e.g. Cloud Run, GCLB) that overwrites
// those headers rather than being exposed directly to the internet.
func getClientIP(r *http.Request) string {
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}

// Handler returns the HTTP middleware.
func (rl *RateLimiter) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := getClientIP(r)

		limiter := rl.getLimiter(ip)
		if !limiter.Allow() {
			response.Error(w, http.StatusTooManyRequests, "RATE_LIMIT_EXCEEDED", "Too many requests. Please try again later.", nil)
			return
		}

		next.ServeHTTP(w, r)
	})
}
