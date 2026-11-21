package middleware

import (
	"container/list"
	"math"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"arkive/pkg/apierror"
)

const maxRateLimitEntries = 4096

type rateLimitBucket struct {
	ip        string
	tokens    float64
	updatedAt time.Time
}

type ipRateLimiter struct {
	mu                sync.Mutex
	capacity          float64
	refillPerSec      float64
	entries           map[string]*list.Element
	leastRecentlyUsed *list.List
}

// shortcut: per-process and bounded to 4096 IPs; use shared state if replicas need a common limit.
func LimitRequestsPerIP(requestsPerMinute int) gin.HandlerFunc {
	if requestsPerMinute <= 0 {
		return func(c *gin.Context) { c.Next() }
	}

	limiter := &ipRateLimiter{
		capacity:          float64(requestsPerMinute),
		refillPerSec:      float64(requestsPerMinute) / 60,
		entries:           make(map[string]*list.Element),
		leastRecentlyUsed: list.New(),
	}

	return func(c *gin.Context) {
		clientIP := c.ClientIP()
		if clientIP == "" {
			clientIP = c.Request.RemoteAddr
		}
		allowed, retryAfter := limiter.allow(clientIP, time.Now())
		if !allowed {
			apierror.TooManyRequests(c, "rate_limited", "Too many requests", retryAfter)
			c.Abort()
			return
		}
		c.Next()
	}
}

func (l *ipRateLimiter) allow(ip string, now time.Time) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()

	element, ok := l.entries[ip]
	if !ok {
		if l.leastRecentlyUsed.Len() >= maxRateLimitEntries {
			oldest := l.leastRecentlyUsed.Back()
			delete(l.entries, oldest.Value.(*rateLimitBucket).ip)
			l.leastRecentlyUsed.Remove(oldest)
		}
		bucket := &rateLimitBucket{ip: ip, tokens: l.capacity, updatedAt: now}
		element = l.leastRecentlyUsed.PushFront(bucket)
		l.entries[ip] = element
	} else {
		bucket := element.Value.(*rateLimitBucket)
		elapsed := now.Sub(bucket.updatedAt).Seconds()
		bucket.tokens = math.Min(l.capacity, bucket.tokens+elapsed*l.refillPerSec)
		bucket.updatedAt = now
		l.leastRecentlyUsed.MoveToFront(element)
	}

	bucket := element.Value.(*rateLimitBucket)
	if bucket.tokens < 1 {
		seconds := (1 - bucket.tokens) / l.refillPerSec
		return false, time.Duration(seconds * float64(time.Second))
	}
	bucket.tokens--
	return true, 0
}
