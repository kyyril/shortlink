package httpx

import (
	"context"
	"math"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

type clientLimiter struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// RateLimiter manages per-IP token bucket limiters with idle cleanup.
type RateLimiter struct {
	mu           sync.Mutex
	clients      map[string]*clientLimiter
	rps          rate.Limit
	burst        int
	idleTTL      time.Duration
	stopCleaner  chan struct{}
	trustForward bool
}

// NewRateLimiter creates a RateLimiter with periodic cleanup of idle entries.
func NewRateLimiter(rps float64, burst int, idleTTL time.Duration, cleanupInterval time.Duration, trustForwarded bool) *RateLimiter {
	if idleTTL <= 0 {
		idleTTL = 3 * time.Minute
	}
	if cleanupInterval <= 0 {
		cleanupInterval = time.Minute
	}

	rl := &RateLimiter{
		clients:      make(map[string]*clientLimiter),
		rps:          rate.Limit(rps),
		burst:        burst,
		idleTTL:      idleTTL,
		stopCleaner:  make(chan struct{}),
		trustForward: trustForwarded,
	}

	go rl.cleanerLoop(cleanupInterval)
	return rl
}

func (rl *RateLimiter) cleanerLoop(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			rl.CleanupIdle()
		case <-rl.stopCleaner:
			return
		}
	}
}

// CleanupIdle purges client limiters that haven't been seen for longer than idleTTL.
func (rl *RateLimiter) CleanupIdle() {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	for ip, cl := range rl.clients {
		if now.Sub(cl.lastSeen) > rl.idleTTL {
			delete(rl.clients, ip)
		}
	}
}

// Close stops the background cleaner goroutine.
func (rl *RateLimiter) Close(_ context.Context) error {
	select {
	case <-rl.stopCleaner:
	default:
		close(rl.stopCleaner)
	}
	return nil
}

func (rl *RateLimiter) getLimiter(ip string) *rate.Limiter {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	cl, exists := rl.clients[ip]
	if !exists {
		cl = &clientLimiter{
			limiter:  rate.NewLimiter(rl.rps, rl.burst),
			lastSeen: time.Now(),
		}
		rl.clients[ip] = cl
	} else {
		cl.lastSeen = time.Now()
	}

	return cl.limiter
}

// ClientIP extracts the remote client IP.
// Only trusts X-Forwarded-For if trustForward is enabled.
func (rl *RateLimiter) ClientIP(r *http.Request) string {
	if rl.trustForward {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			parts := strings.Split(xff, ",")
			clientIP := strings.TrimSpace(parts[0])
			if clientIP != "" {
				return clientIP
			}
		}
	}

	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// Middleware returns an HTTP middleware enforcing rate limits per client IP.
// AC-17: When exceeded, returns 429 with Retry-After header.
func (rl *RateLimiter) Middleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := rl.ClientIP(r)
			limiter := rl.getLimiter(ip)

			if !limiter.Allow() {
				// Calculate estimated wait time for at least 1 token
				retryAfter := int(math.Ceil(1.0 / float64(rl.rps)))
				if retryAfter < 1 {
					retryAfter = 1
				}
				w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
				WriteProblem(w, r, http.StatusTooManyRequests, "rate_limit_exceeded", "too many requests, please slow down")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
