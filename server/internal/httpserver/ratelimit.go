package httpserver

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

type bucket struct {
	tokens float64
	last   time.Time
}

// RateLimiter implements an in-memory IP-based token bucket rate limiter.
type RateLimiter struct {
	mu      sync.Mutex
	buckets map[string]*bucket
	rate    float64
	burst   int
}

// NewRateLimiter creates a rate limiter that refills at rate tokens/second
// with a maximum burst capacity.
func NewRateLimiter(rate float64, burst int) *RateLimiter {
	if rate <= 0 {
		rate = 1
	}
	if burst <= 0 {
		burst = 1
	}

	rl := &RateLimiter{
		buckets: make(map[string]*bucket),
		rate:    rate,
		burst:   burst,
	}
	go rl.cleanupLoop()
	return rl
}

// Allow checks whether the given IP is allowed to proceed.
func (r *RateLimiter) Allow(ip string) bool {
	if ip == "" {
		ip = "unknown"
	}
	now := time.Now()

	r.mu.Lock()
	defer r.mu.Unlock()

	b, ok := r.buckets[ip]
	if !ok {
		r.buckets[ip] = &bucket{
			tokens: float64(r.burst - 1),
			last:   now,
		}
		return true
	}

	elapsed := now.Sub(b.last).Seconds()
	if elapsed > 0 {
		b.tokens += elapsed * r.rate
		if max := float64(r.burst); b.tokens > max {
			b.tokens = max
		}
	}
	b.last = now

	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

// Middleware returns a Gin middleware that rate-limits by client IP.
func (r *RateLimiter) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !r.Allow(c.ClientIP()) {
			c.JSON(http.StatusTooManyRequests, gin.H{"error": "rate_limited"})
			c.Abort()
			return
		}
		c.Next()
	}
}

func (r *RateLimiter) cleanupLoop() {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		cutoff := time.Now().Add(-1 * time.Hour)
		r.mu.Lock()
		for ip, b := range r.buckets {
			if b.last.Before(cutoff) {
				delete(r.buckets, ip)
			}
		}
		r.mu.Unlock()
	}
}
