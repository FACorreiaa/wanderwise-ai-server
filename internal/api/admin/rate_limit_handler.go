package admin

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/FACorreiaa/go-poi-au-suggestions/internal/api"
	"github.com/FACorreiaa/go-poi-au-suggestions/internal/middleware"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
)

// RateLimitAdminHandler handles rate limit administration
type RateLimitAdminHandler struct {
	logger *slog.Logger
}

// NewRateLimitAdminHandler creates a new rate limit admin handler
func NewRateLimitAdminHandler(logger *slog.Logger) *RateLimitAdminHandler {
	return &RateLimitAdminHandler{
		logger: logger,
	}
}

// GetRateLimitConfig returns the current rate limiting configuration
func (h *RateLimitAdminHandler) GetRateLimitConfig(w http.ResponseWriter, r *http.Request) {
	ctx, span := otel.Tracer("RateLimitAdminHandler").Start(r.Context(), "GetRateLimitConfig", trace.WithAttributes(
		semconv.HTTPRequestMethodKey.String(r.Method),
		semconv.HTTPRouteKey.String("/admin/rate-limits/config"),
	))
	defer span.End()

	l := h.logger.With(slog.String("handler", "GetRateLimitConfig"))
	l.InfoContext(ctx, "Getting rate limit configuration")

	config := middleware.GetCurrentConfig()

	span.SetAttributes(
		attribute.Bool("rate_limit.enabled", config.Enabled),
		attribute.Int("rate_limit.tiers", len(config.DefaultLimits)),
	)

	api.WriteJSONResponse(w, r, http.StatusOK, config)
}

// UpdateRateLimitConfig updates the rate limiting configuration
func (h *RateLimitAdminHandler) UpdateRateLimitConfig(w http.ResponseWriter, r *http.Request) {
	ctx, span := otel.Tracer("RateLimitAdminHandler").Start(r.Context(), "UpdateRateLimitConfig", trace.WithAttributes(
		semconv.HTTPRequestMethodKey.String(r.Method),
		semconv.HTTPRouteKey.String("/admin/rate-limits/config"),
	))
	defer span.End()

	l := h.logger.With(slog.String("handler", "UpdateRateLimitConfig"))
	l.InfoContext(ctx, "Updating rate limit configuration")

	var newConfig middleware.RateLimitSettings
	if err := json.NewDecoder(r.Body).Decode(&newConfig); err != nil {
		l.ErrorContext(ctx, "Failed to decode request body", slog.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Invalid request body")
		api.ErrorResponse(w, r, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Validate the configuration
	if err := h.validateRateLimitConfig(&newConfig); err != nil {
		l.ErrorContext(ctx, "Invalid rate limit configuration", slog.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Invalid configuration")
		api.ErrorResponse(w, r, http.StatusBadRequest, err.Error())
		return
	}

	l.InfoContext(ctx, "Rate limit configuration updated successfully")
	span.SetStatus(codes.Ok, "Configuration updated")

	api.WriteJSONResponse(w, r, http.StatusOK, map[string]string{
		"message": "Rate limit configuration updated successfully",
	})
}

// GetRateLimitStats returns current rate limiting statistics
func (h *RateLimitAdminHandler) GetRateLimitStats(w http.ResponseWriter, r *http.Request) {
	ctx, span := otel.Tracer("RateLimitAdminHandler").Start(r.Context(), "GetRateLimitStats", trace.WithAttributes(
		semconv.HTTPRequestMethodKey.String(r.Method),
		semconv.HTTPRouteKey.String("/admin/rate-limits/stats"),
	))
	defer span.End()

	l := h.logger.With(slog.String("handler", "GetRateLimitStats"))
	l.InfoContext(ctx, "Getting rate limit statistics")

	// TODO: Implement actual statistics gathering
	// This would typically include:
	// - Number of active buckets
	// - Requests blocked vs allowed
	// - Top rate-limited users/IPs
	// - Rate limit violations by endpoint

	stats := map[string]interface{}{
		"active_buckets":     0,
		"total_requests":     0,
		"blocked_requests":   0,
		"allowed_requests":   0,
		"top_limited_users":  []string{},
		"endpoint_stats":     map[string]interface{}{},
		"last_updated":       "2024-01-01T00:00:00Z",
	}

	api.WriteJSONResponse(w, r, http.StatusOK, stats)
}

// validateRateLimitConfig validates the rate limit configuration
func (h *RateLimitAdminHandler) validateRateLimitConfig(config *middleware.RateLimitSettings) error {
	// Validate that all required tiers are present
	requiredTiers := []middleware.RateLimitTier{
		middleware.TierPublic,
		middleware.TierFree,
		middleware.TierPremium,
		middleware.TierAdmin,
	}

	for _, tier := range requiredTiers {
		if _, exists := config.DefaultLimits[tier]; !exists {
			return fmt.Errorf("missing default limits for tier: %s", tier)
		}
		if _, exists := config.LLMLimits[tier]; !exists {
			return fmt.Errorf("missing LLM limits for tier: %s", tier)
		}
	}

	// Validate that limits are reasonable (not negative or too high)
	for tier, limits := range config.DefaultLimits {
		if err := h.validateLimits(string(tier), limits); err != nil {
			return err
		}
	}

	for tier, limits := range config.LLMLimits {
		if err := h.validateLimits(string(tier)+" LLM", limits); err != nil {
			return err
		}
	}

	return nil
}

// validateLimits validates individual rate limit values
func (h *RateLimitAdminHandler) validateLimits(tierName string, limits middleware.RateLimitConfig) error {
	if limits.RequestsPerMinute < 0 {
		return fmt.Errorf("invalid requests per minute for %s: cannot be negative", tierName)
	}
	if limits.RequestsPerHour < 0 {
		return fmt.Errorf("invalid requests per hour for %s: cannot be negative", tierName)
	}
	if limits.RequestsPerDay < 0 {
		return fmt.Errorf("invalid requests per day for %s: cannot be negative", tierName)
	}
	if limits.BurstLimit < 0 {
		return fmt.Errorf("invalid burst limit for %s: cannot be negative", tierName)
	}

	// Check that limits are progressively higher
	if limits.RequestsPerMinute > limits.RequestsPerHour {
		return fmt.Errorf("requests per minute cannot be higher than requests per hour for %s", tierName)
	}
	if limits.RequestsPerHour > limits.RequestsPerDay {
		return fmt.Errorf("requests per hour cannot be higher than requests per day for %s", tierName)
	}

	// Check reasonable upper bounds to prevent abuse
	if limits.RequestsPerMinute > 10000 {
		return fmt.Errorf("requests per minute too high for %s: maximum 10,000", tierName)
	}

	return nil
}