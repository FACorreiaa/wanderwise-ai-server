package middleware

import (
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/FACorreiaa/go-poi-au-suggestions/config"
	"github.com/FACorreiaa/go-poi-au-suggestions/internal/api/auth"
)

// RateLimiter manages rate limiting for different user tiers
type RateLimiter struct {
	config   *config.RateLimitConfig
	logger   *slog.Logger
	limiters map[string]*UserLimiter
	mu       sync.RWMutex
}

// UserLimiter tracks rate limiting for a specific user
type UserLimiter struct {
	PerMinute *TokenBucket
	PerHour   *TokenBucket
	PerDay    *TokenBucket
}

// TokenBucket implements a token bucket rate limiter
type TokenBucket struct {
	capacity   int
	tokens     int
	refillRate time.Duration
	lastRefill time.Time
	mu         sync.Mutex
}

// NewTokenBucket creates a new token bucket with the specified capacity and refill rate
func NewTokenBucket(capacity int, refillRate time.Duration) *TokenBucket {
	return &TokenBucket{
		capacity:   capacity,
		tokens:     capacity,
		refillRate: refillRate,
		lastRefill: time.Now(),
	}
}

// Allow checks if a request can be allowed and consumes a token if so
func (tb *TokenBucket) Allow() bool {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(tb.lastRefill)

	// Refill tokens based on elapsed time
	tokensToAdd := int(elapsed / tb.refillRate)
	if tokensToAdd > 0 {
		tb.tokens = min(tb.capacity, tb.tokens+tokensToAdd)
		tb.lastRefill = now
	}

	if tb.tokens > 0 {
		tb.tokens--
		return true
	}

	return false
}

// NewRateLimiter creates a new rate limiter with the given configuration
func NewRateLimiter(config *config.RateLimitConfig, logger *slog.Logger) *RateLimiter {
	return &RateLimiter{
		config:   config,
		logger:   logger,
		limiters: make(map[string]*UserLimiter),
	}
}

// getUserLimiter gets or creates a rate limiter for the specified user
func (rl *RateLimiter) getUserLimiter(userID string, tier UserTier) *UserLimiter {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	if limiter, exists := rl.limiters[userID]; exists {
		return limiter
	}

	// Create new limiter based on user tier
	var limits struct {
		perMinute, perHour, perDay int
	}

	switch tier {
	case TierAnonymous:
		limits.perMinute = rl.config.AnonymousRequests.PerMinute
		limits.perHour = rl.config.AnonymousRequests.PerHour
		limits.perDay = rl.config.AnonymousRequests.PerDay
	case TierLoggedIn:
		limits.perMinute = rl.config.LoggedInUsers.PerMinute
		limits.perHour = rl.config.LoggedInUsers.PerHour
		limits.perDay = rl.config.LoggedInUsers.PerDay
	case TierPaying:
		limits.perMinute = rl.config.PayingUsers.PerMinute
		limits.perHour = rl.config.PayingUsers.PerHour
		limits.perDay = rl.config.PayingUsers.PerDay
	}

	limiter := &UserLimiter{
		PerMinute: NewTokenBucket(limits.perMinute, time.Minute/time.Duration(limits.perMinute)),
		PerHour:   NewTokenBucket(limits.perHour, time.Hour/time.Duration(limits.perHour)),
		PerDay:    NewTokenBucket(limits.perDay, time.Hour*24/time.Duration(limits.perDay)),
	}

	rl.limiters[userID] = limiter
	return limiter
}

// UserTier represents different user subscription tiers
type UserTier int

const (
	TierAnonymous UserTier = iota
	TierLoggedIn
	TierPaying
)

// Allow checks if a request should be allowed for the given user
func (rl *RateLimiter) Allow(userID string, tier UserTier) bool {
	if !rl.config.Enabled {
		return true // Rate limiting disabled
	}

	limiter := rl.getUserLimiter(userID, tier)

	// Check all time windows - request must pass all limits
	if !limiter.PerMinute.Allow() {
		rl.logger.Debug("Rate limit exceeded: per minute", "userID", userID, "tier", tier)
		return false
	}

	if !limiter.PerHour.Allow() {
		rl.logger.Debug("Rate limit exceeded: per hour", "userID", userID, "tier", tier)
		return false
	}

	if !limiter.PerDay.Allow() {
		rl.logger.Debug("Rate limit exceeded: per day", "userID", userID, "tier", tier)
		return false
	}

	return true
}

// RateLimitMiddleware returns a middleware that enforces rate limiting for LLM calls
func RateLimitMiddleware(rateLimiter *RateLimiter, logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Extract user information from context
			userID, tier := getUserInfoFromRequest(r)

			// Check rate limit
			if !rateLimiter.Allow(userID, tier) {
				logger.Warn("Rate limit exceeded", "userID", userID, "tier", tier, "path", r.URL.Path)

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusTooManyRequests)
				if _, err := w.Write([]byte(`{"error":"Rate limit exceeded. Please try again later."}`)); err != nil {
					logger.ErrorContext(r.Context(), "Failed to write rate limit response", slog.Any("error", err))
				}
				return
			}

			// Request allowed, continue
			next.ServeHTTP(w, r)
		})
	}
}

// getUserInfoFromRequest extracts user ID and tier from the request
func getUserInfoFromRequest(r *http.Request) (string, UserTier) {
	// Check if user is authenticated by looking for user context
	if userID, ok := auth.GetUserIDFromContext(r.Context()); ok && userID != "" {
		// Determine user tier based on subscription status
		if plan, hasPlan := auth.GetUserPlanFromContext(r.Context()); hasPlan {
			if status, hasStatus := auth.GetUserSubStatusFromContext(r.Context()); hasStatus && status == "active" {
				// Check if user has premium subscription
				if plan == "premium_monthly" || plan == "premium_annual" {
					return userID, TierPaying
				}
			}
		}
		
		// User is logged in but not paying
		return userID, TierLoggedIn
	}

	// Anonymous user - use IP address as identifier
	clientIP := getClientIP(r)
	return fmt.Sprintf("anon_%s", clientIP), TierAnonymous
}

// getClientIP extracts the client IP address from the request
func getClientIP(r *http.Request) string {
	// Check X-Forwarded-For header first (for reverse proxies)
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		return xff
	}

	// Check X-Real-IP header
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}

	// Fall back to remote address
	return r.RemoteAddr
}

// min returns the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
