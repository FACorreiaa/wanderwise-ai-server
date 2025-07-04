package middleware

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/FACorreiaa/go-poi-au-suggestions/internal/api"
	"github.com/FACorreiaa/go-poi-au-suggestions/internal/api/auth"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// RateLimitTier defines different rate limiting tiers
type RateLimitTier string

const (
	TierFree     RateLimitTier = "free"
	TierPremium  RateLimitTier = "premium"
	TierAdmin    RateLimitTier = "admin"
	TierPublic   RateLimitTier = "public"
)

// RateLimitConfig defines rate limiting configuration for different endpoints
type RateLimitConfig struct {
	RequestsPerMinute int           `json:"requests_per_minute"`
	RequestsPerHour   int           `json:"requests_per_hour"`
	RequestsPerDay    int           `json:"requests_per_day"`
	BurstLimit        int           `json:"burst_limit"`
	WindowSize        time.Duration `json:"window_size"`
}

// UnmarshalJSON implements custom JSON unmarshaling for RateLimitConfig
func (c *RateLimitConfig) UnmarshalJSON(data []byte) error {
	type Alias RateLimitConfig
	aux := &struct {
		WindowSize string `json:"window_size"`
		*Alias
	}{
		Alias: (*Alias)(c),
	}
	
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	
	// Parse window_size string to duration
	if aux.WindowSize != "" {
		duration, err := time.ParseDuration(aux.WindowSize)
		if err != nil {
			return fmt.Errorf("invalid window_size format: %w", err)
		}
		c.WindowSize = duration
	} else {
		c.WindowSize = time.Minute // Default
	}
	
	return nil
}

// DefaultRateLimits defines default rate limits for different tiers
var DefaultRateLimits = map[RateLimitTier]RateLimitConfig{
	TierPublic: {
		RequestsPerMinute: 30,
		RequestsPerHour:   500,
		RequestsPerDay:    2000,
		BurstLimit:        10,
		WindowSize:        time.Minute,
	},
	TierFree: {
		RequestsPerMinute: 10,
		RequestsPerHour:   100,
		RequestsPerDay:    500,
		BurstLimit:        5,
		WindowSize:        time.Minute,
	},
	TierPremium: {
		RequestsPerMinute: 100,
		RequestsPerHour:   2000,
		RequestsPerDay:    10000,
		BurstLimit:        20,
		WindowSize:        time.Minute,
	},
	TierAdmin: {
		RequestsPerMinute: 1000,
		RequestsPerHour:   10000,
		RequestsPerDay:    50000,
		BurstLimit:        100,
		WindowSize:        time.Minute,
	},
}

// LLMRateLimits defines stricter rate limits for LLM endpoints
var LLMRateLimits = map[RateLimitTier]RateLimitConfig{
	TierPublic: {
		RequestsPerMinute: 2,
		RequestsPerHour:   10,
		RequestsPerDay:    50,
		BurstLimit:        1,
		WindowSize:        time.Minute,
	},
	TierFree: {
		RequestsPerMinute: 5,
		RequestsPerHour:   30,
		RequestsPerDay:    100,
		BurstLimit:        2,
		WindowSize:        time.Minute,
	},
	TierPremium: {
		RequestsPerMinute: 30,
		RequestsPerHour:   500,
		RequestsPerDay:    2000,
		BurstLimit:        10,
		WindowSize:        time.Minute,
	},
	TierAdmin: {
		RequestsPerMinute: 100,
		RequestsPerHour:   2000,
		RequestsPerDay:    10000,
		BurstLimit:        20,
		WindowSize:        time.Minute,
	},
}

// TokenBucket implements a token bucket rate limiter
type TokenBucket struct {
	mu       sync.Mutex
	tokens   int
	capacity int
	refill   int
	lastTime time.Time
	window   time.Duration
}

// NewTokenBucket creates a new token bucket
func NewTokenBucket(capacity, refill int, window time.Duration) *TokenBucket {
	return &TokenBucket{
		tokens:   capacity,
		capacity: capacity,
		refill:   refill,
		lastTime: time.Now(),
		window:   window,
	}
}

// Allow checks if a request should be allowed
func (tb *TokenBucket) Allow() bool {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(tb.lastTime)

	// Refill tokens based on elapsed time
	if elapsed >= tb.window {
		if tb.window > 0 {
			tokensToAdd := int(elapsed/tb.window) * tb.refill
			tb.tokens = min(tb.capacity, tb.tokens+tokensToAdd)
		}
		tb.lastTime = now
	}

	if tb.tokens > 0 {
		tb.tokens--
		return true
	}

	return false
}

// RemainingTokens returns the number of remaining tokens
func (tb *TokenBucket) RemainingTokens() int {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	return tb.tokens
}

// RateLimiter manages rate limiting for different users and endpoints
type RateLimiter struct {
	buckets  map[string]*TokenBucket
	mu       sync.RWMutex
	logger   *slog.Logger
	config   map[string]RateLimitConfig
	enabled  bool
	settings *RateLimitSettings
}

// NewRateLimiter creates a new rate limiter
func NewRateLimiter(logger *slog.Logger) *RateLimiter {
	// Check environment variable first for quick disable
	if os.Getenv("RATE_LIMIT_DISABLED") == "true" {
		logger.Info("Rate limiting disabled via environment variable")
		return &RateLimiter{
			buckets:  make(map[string]*TokenBucket),
			logger:   logger,
			config:   make(map[string]RateLimitConfig),
			enabled:  false,
			settings: &RateLimitSettings{Enabled: false},
		}
	}

	// Load configuration from environment or config file
	settings, err := LoadRateLimitConfig("config/rate_limits.json")
	if err != nil {
		logger.Warn("Failed to load rate limit config, using defaults", slog.Any("error", err))
		settings = &RateLimitSettings{
			DefaultLimits: DefaultRateLimits,
			LLMLimits:     LLMRateLimits,
			Enabled:       true,
		}
	}

	logger.Info("Rate limiter initialized", 
		slog.Bool("enabled", settings.Enabled),
		slog.String("config_source", "config/rate_limits.json"))

	return &RateLimiter{
		buckets:  make(map[string]*TokenBucket),
		logger:   logger,
		config:   make(map[string]RateLimitConfig),
		enabled:  settings.Enabled,
		settings: settings,
	}
}

// getBucket gets or creates a token bucket for a specific key
func (rl *RateLimiter) getBucket(key string, config RateLimitConfig) *TokenBucket {
	rl.mu.RLock()
	bucket, exists := rl.buckets[key]
	rl.mu.RUnlock()

	if exists {
		return bucket
	}

	rl.mu.Lock()
	defer rl.mu.Unlock()

	// Double-check after acquiring write lock
	if bucket, exists := rl.buckets[key]; exists {
		return bucket
	}

	// Create new bucket
	bucket = NewTokenBucket(
		config.RequestsPerMinute,
		config.RequestsPerMinute,
		config.WindowSize,
	)
	rl.buckets[key] = bucket

	return bucket
}

// IsAllowed checks if a request should be allowed
func (rl *RateLimiter) IsAllowed(userID, endpoint string, tier RateLimitTier, isLLM bool) (bool, int, error) {
	// If rate limiting is disabled, always allow
	if !rl.enabled {
		return true, 999, nil
	}

	// Select appropriate rate limit config
	var config RateLimitConfig
	var exists bool

	if isLLM {
		config, exists = rl.settings.LLMLimits[tier]
	} else {
		config, exists = rl.settings.DefaultLimits[tier]
	}

	if !exists {
		config = rl.settings.DefaultLimits[TierFree] // Fallback to most restrictive
	}

	// Create unique key for user/endpoint combination
	key := fmt.Sprintf("%s:%s:%s", userID, endpoint, tier)
	
	bucket := rl.getBucket(key, config)
	allowed := bucket.Allow()
	remaining := bucket.RemainingTokens()

	rl.logger.Debug("Rate limit check",
		slog.String("user_id", userID),
		slog.String("endpoint", endpoint),
		slog.String("tier", string(tier)),
		slog.Bool("is_llm", isLLM),
		slog.Bool("allowed", allowed),
		slog.Int("remaining", remaining),
	)

	return allowed, remaining, nil
}

// getUserTier determines the user's rate limiting tier
func getUserTier(ctx context.Context, userID string) RateLimitTier {
	// If no user ID, treat as public
	if userID == "" {
		return TierPublic
	}

	// TODO: Implement actual tier checking from database
	// For now, assume all authenticated users are free tier
	// In the future, check user's subscription status:
	// - Query user subscription from database
	// - Check if subscription is active
	// - Return appropriate tier

	return TierFree
}

// getEndpointCategory categorizes endpoints for rate limiting
func getEndpointCategory(path string) (string, bool) {
	// Check if it's an LLM endpoint
	isLLM := strings.Contains(path, "/llm/") || 
		strings.Contains(path, "/chat/") || 
		strings.Contains(path, "/prompt-response/")

	// Normalize endpoint path for grouping
	if strings.Contains(path, "/llm/chat/stream") {
		return "llm_chat_stream", isLLM
	} else if strings.Contains(path, "/llm/prompt-response") {
		return "llm_prompt_response", isLLM
	} else if strings.Contains(path, "/pois/search") {
		return "poi_search", false
	} else if strings.Contains(path, "/pois/") {
		return "poi_general", false
	} else if strings.Contains(path, "/auth/") {
		return "auth", false
	} else if strings.Contains(path, "/user/") {
		return "user", false
	}

	// Default category
	return "general", isLLM
}

// RateLimitMiddleware creates a rate limiting middleware
func RateLimitMiddleware(logger *slog.Logger) func(http.Handler) http.Handler {
	limiter := NewRateLimiter(logger)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Check if rate limiting is disabled globally
			if !limiter.enabled {
				next.ServeHTTP(w, r)
				return
			}

			ctx, span := otel.Tracer("RateLimitMiddleware").Start(r.Context(), "RateLimit", 
				trace.WithAttributes(
					attribute.String("http.method", r.Method),
					attribute.String("http.path", r.URL.Path),
				))
			defer span.End()

			// Get user ID from context (if authenticated)
			userID := ""
			if uid, ok := auth.GetUserIDFromContext(ctx); ok {
				userID = uid
			}

			// Use IP address as fallback identifier for unauthenticated users
			clientIP := getClientIP(r)
			identifier := userID
			if identifier == "" {
				identifier = clientIP
			}

			// Determine user tier
			tier := getUserTier(ctx, userID)

			// Get endpoint category and check if it's LLM
			endpoint, isLLM := getEndpointCategory(r.URL.Path)

			// Check rate limit
			allowed, remaining, err := limiter.IsAllowed(identifier, endpoint, tier, isLLM)
			if err != nil {
				logger.ErrorContext(ctx, "Rate limiter error", slog.Any("error", err))
				span.RecordError(err)
				span.SetStatus(codes.Error, "Rate limiter error")
				api.ErrorResponse(w, r, http.StatusInternalServerError, "Rate limiting error")
				return
			}

			// Set rate limit headers
			w.Header().Set("X-RateLimit-Limit", strconv.Itoa(getRateLimitForTier(tier, isLLM)))
			w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(remaining))
			w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(time.Now().Add(time.Minute).Unix(), 10))

			if !allowed {
				logger.WarnContext(ctx, "Rate limit exceeded",
					slog.String("user_id", userID),
					slog.String("client_ip", clientIP),
					slog.String("endpoint", endpoint),
					slog.String("tier", string(tier)),
					slog.Bool("is_llm", isLLM),
				)

				span.SetAttributes(
					attribute.String("rate_limit.tier", string(tier)),
					attribute.String("rate_limit.endpoint", endpoint),
					attribute.Bool("rate_limit.is_llm", isLLM),
					attribute.Bool("rate_limit.exceeded", true),
				)
				span.SetStatus(codes.Error, "Rate limit exceeded")

				// Return rate limit error with retry information
				w.Header().Set("Retry-After", "60") // Retry after 60 seconds
				api.ErrorResponse(w, r, http.StatusTooManyRequests, "Rate limit exceeded. Please try again later.")
				return
			}

			span.SetAttributes(
				attribute.String("rate_limit.tier", string(tier)),
				attribute.String("rate_limit.endpoint", endpoint),
				attribute.Bool("rate_limit.is_llm", isLLM),
				attribute.Int("rate_limit.remaining", remaining),
			)

			// Continue to next handler
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// getClientIP extracts the client IP address from the request
func getClientIP(r *http.Request) string {
	// Check X-Forwarded-For header first (for proxies)
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// Take the first IP in the list
		if idx := strings.Index(xff, ","); idx >= 0 {
			return strings.TrimSpace(xff[:idx])
		}
		return strings.TrimSpace(xff)
	}

	// Check X-Real-IP header
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return strings.TrimSpace(xri)
	}

	// Fall back to RemoteAddr
	if idx := strings.LastIndex(r.RemoteAddr, ":"); idx >= 0 {
		return r.RemoteAddr[:idx]
	}

	return r.RemoteAddr
}

// getRateLimitForTier returns the rate limit for a given tier
func getRateLimitForTier(tier RateLimitTier, isLLM bool) int {
	var config RateLimitConfig
	var exists bool

	if isLLM {
		config, exists = LLMRateLimits[tier]
	} else {
		config, exists = DefaultRateLimits[tier]
	}

	if !exists {
		config = DefaultRateLimits[TierFree]
	}

	return config.RequestsPerMinute
}

// min helper function
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}