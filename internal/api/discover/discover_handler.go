package discover

import (
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"

	"github.com/FACorreiaa/go-poi-au-suggestions/internal/api"
	"github.com/FACorreiaa/go-poi-au-suggestions/internal/api/auth"
)

var _ Handler = (*HandlerImpl)(nil)

type Handler interface {
	// Main discover page data
	GetDiscoverPageData(w http.ResponseWriter, r *http.Request)

	// Trending discoveries
	GetTrendingDiscoveries(w http.ResponseWriter, r *http.Request)

	// Featured collections
	GetFeaturedCollections(w http.ResponseWriter, r *http.Request)

	// User's recent discoveries
	GetRecentDiscoveries(w http.ResponseWriter, r *http.Request)

	// Category-based discovery
	GetCategoryResults(w http.ResponseWriter, r *http.Request)
}

type HandlerImpl struct {
	discoverService Service
	logger          *slog.Logger
}

func NewHandlerImpl(discoverService Service, logger *slog.Logger) *HandlerImpl {
	return &HandlerImpl{
		discoverService: discoverService,
		logger:          logger,
	}
}

// GetDiscoverPageData godoc
// @Summary      Get Discover Page Data
// @Description  Returns all data needed for the discover page (trending, featured, recent)
// @Tags         Discover
// @Accept       json
// @Produce      json
// @Param        limit query int false "Limit for each section" default(5)
// @Success      200 {object} types.DiscoverPageResponse "Discover page data"
// @Failure      500 {object} types.Response "Internal Server Error"
// @Security     BearerAuth
// @Router       /discover [get]
func (h *HandlerImpl) GetDiscoverPageData(w http.ResponseWriter, r *http.Request) {
	ctx, span := otel.Tracer("GetDiscoverPageData").Start(r.Context(), "GetDiscoverPageData", trace.WithAttributes(
		semconv.HTTPRequestMethodKey.String(r.Method),
		semconv.HTTPRouteKey.String("/discover"),
	))
	defer span.End()

	l := h.logger.With(slog.String("handler", "GetDiscoverPageData"))
	l.DebugContext(ctx, "GetDiscoverPageData handler invoked")

	// Parse limit parameter
	limitStr := r.URL.Query().Get("limit")
	limit := 5 // default
	if limitStr != "" {
		if parsedLimit, err := strconv.Atoi(limitStr); err == nil && parsedLimit > 0 {
			limit = parsedLimit
		}
	}

	// Get user ID (optional - may be anonymous)
	var userID uuid.UUID
	if userIDStr, ok := auth.GetUserIDFromContext(ctx); ok && userIDStr != "" {
		if parsedID, err := uuid.Parse(userIDStr); err == nil {
			userID = parsedID
		}
	}

	// Get all discover page data
	pageData, err := h.discoverService.GetDiscoverPageData(ctx, userID, limit)
	if err != nil {
		l.ErrorContext(ctx, "Failed to get discover page data", slog.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to get discover page data")
		api.ErrorResponse(w, r, http.StatusInternalServerError, "Failed to load discover page data")
		return
	}

	span.SetAttributes(
		attribute.Int("trending_count", len(pageData.Trending)),
		attribute.Int("featured_count", len(pageData.Featured)),
		attribute.Int("recent_count", len(pageData.RecentDiscoveries)),
	)
	span.SetStatus(codes.Ok, "Successfully retrieved discover page data")

	api.WriteJSONResponse(w, r, http.StatusOK, pageData)
}

// GetTrendingDiscoveries godoc
// @Summary      Get Trending Discoveries
// @Description  Returns currently trending discoveries
// @Tags         Discover
// @Accept       json
// @Produce      json
// @Param        limit query int false "Number of trending items to return" default(5)
// @Success      200 {object} types.TrendingDiscoveriesResponse "Trending discoveries"
// @Failure      500 {object} types.Response "Internal Server Error"
// @Router       /discover/trending [get]
func (h *HandlerImpl) GetTrendingDiscoveries(w http.ResponseWriter, r *http.Request) {
	ctx, span := otel.Tracer("GetTrendingDiscoveries").Start(r.Context(), "GetTrendingDiscoveries", trace.WithAttributes(
		semconv.HTTPRequestMethodKey.String(r.Method),
		semconv.HTTPRouteKey.String("/discover/trending"),
	))
	defer span.End()

	l := h.logger.With(slog.String("handler", "GetTrendingDiscoveries"))
	l.DebugContext(ctx, "GetTrendingDiscoveries handler invoked")

	// Parse limit parameter
	limitStr := r.URL.Query().Get("limit")
	limit := 5 // default
	if limitStr != "" {
		if parsedLimit, err := strconv.Atoi(limitStr); err == nil && parsedLimit > 0 {
			limit = parsedLimit
		}
	}

	trending, err := h.discoverService.GetTrendingDiscoveries(ctx, limit)
	if err != nil {
		l.ErrorContext(ctx, "Failed to get trending discoveries", slog.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to get trending discoveries")
		api.ErrorResponse(w, r, http.StatusInternalServerError, "Failed to load trending discoveries")
		return
	}

	span.SetAttributes(attribute.Int("count", len(trending)))
	span.SetStatus(codes.Ok, "Successfully retrieved trending discoveries")

	api.WriteJSONResponse(w, r, http.StatusOK, map[string]interface{}{
		"trending": trending,
	})
}

// GetFeaturedCollections godoc
// @Summary      Get Featured Collections
// @Description  Returns featured collections of places
// @Tags         Discover
// @Accept       json
// @Produce      json
// @Param        limit query int false "Number of featured collections to return" default(4)
// @Success      200 {object} types.FeaturedCollectionsResponse "Featured collections"
// @Failure      500 {object} types.Response "Internal Server Error"
// @Router       /discover/featured [get]
func (h *HandlerImpl) GetFeaturedCollections(w http.ResponseWriter, r *http.Request) {
	ctx, span := otel.Tracer("GetFeaturedCollections").Start(r.Context(), "GetFeaturedCollections", trace.WithAttributes(
		semconv.HTTPRequestMethodKey.String(r.Method),
		semconv.HTTPRouteKey.String("/discover/featured"),
	))
	defer span.End()

	l := h.logger.With(slog.String("handler", "GetFeaturedCollections"))
	l.DebugContext(ctx, "GetFeaturedCollections handler invoked")

	// Parse limit parameter
	limitStr := r.URL.Query().Get("limit")
	limit := 4 // default
	if limitStr != "" {
		if parsedLimit, err := strconv.Atoi(limitStr); err == nil && parsedLimit > 0 {
			limit = parsedLimit
		}
	}

	featured, err := h.discoverService.GetFeaturedCollections(ctx, limit)
	if err != nil {
		l.ErrorContext(ctx, "Failed to get featured collections", slog.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to get featured collections")
		api.ErrorResponse(w, r, http.StatusInternalServerError, "Failed to load featured collections")
		return
	}

	span.SetAttributes(attribute.Int("count", len(featured)))
	span.SetStatus(codes.Ok, "Successfully retrieved featured collections")

	api.WriteJSONResponse(w, r, http.StatusOK, map[string]interface{}{
		"featured": featured,
	})
}

// GetRecentDiscoveries godoc
// @Summary      Get Recent Discoveries
// @Description  Returns user's recent discovery searches
// @Tags         Discover
// @Accept       json
// @Produce      json
// @Param        limit query int false "Number of recent discoveries to return" default(6)
// @Success      200 {object} types.RecentDiscoveriesResponse "Recent discoveries"
// @Failure      401 {object} types.Response "Authentication required"
// @Failure      500 {object} types.Response "Internal Server Error"
// @Security     BearerAuth
// @Router       /discover/recent [get]
func (h *HandlerImpl) GetRecentDiscoveries(w http.ResponseWriter, r *http.Request) {
	ctx, span := otel.Tracer("GetRecentDiscoveries").Start(r.Context(), "GetRecentDiscoveries", trace.WithAttributes(
		semconv.HTTPRequestMethodKey.String(r.Method),
		semconv.HTTPRouteKey.String("/discover/recent"),
	))
	defer span.End()

	l := h.logger.With(slog.String("handler", "GetRecentDiscoveries"))
	l.DebugContext(ctx, "GetRecentDiscoveries handler invoked")

	// Get user ID from context
	userIDStr, ok := auth.GetUserIDFromContext(ctx)
	if !ok || userIDStr == "" {
		l.ErrorContext(ctx, "User ID not found in context")
		api.ErrorResponse(w, r, http.StatusUnauthorized, "Authentication required")
		return
	}

	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		l.ErrorContext(ctx, "Invalid user ID", slog.Any("error", err))
		span.RecordError(err)
		api.ErrorResponse(w, r, http.StatusBadRequest, "Invalid user ID")
		return
	}

	// Parse limit parameter
	limitStr := r.URL.Query().Get("limit")
	limit := 6 // default
	if limitStr != "" {
		if parsedLimit, err := strconv.Atoi(limitStr); err == nil && parsedLimit > 0 {
			limit = parsedLimit
		}
	}

	recent, err := h.discoverService.GetRecentDiscoveries(ctx, userID, limit)
	if err != nil {
		l.ErrorContext(ctx, "Failed to get recent discoveries", slog.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to get recent discoveries")
		api.ErrorResponse(w, r, http.StatusInternalServerError, "Failed to load recent discoveries")
		return
	}

	span.SetAttributes(
		attribute.String("user_id", userID.String()),
		attribute.Int("count", len(recent)),
	)
	span.SetStatus(codes.Ok, "Successfully retrieved recent discoveries")

	api.WriteJSONResponse(w, r, http.StatusOK, map[string]interface{}{
		"sessions": recent,
	})
}

// GetCategoryResults godoc
// @Summary      Get Category Results
// @Description  Returns results for a specific category
// @Tags         Discover
// @Accept       json
// @Produce      json
// @Param        category path string true "Category name (restaurant, hotel, activity, etc.)"
// @Success      200 {object} types.CategoryResultsResponse "Category results"
// @Failure      400 {object} types.Response "Invalid category"
// @Failure      500 {object} types.Response "Internal Server Error"
// @Router       /discover/category/{category} [get]
func (h *HandlerImpl) GetCategoryResults(w http.ResponseWriter, r *http.Request) {
	ctx, span := otel.Tracer("GetCategoryResults").Start(r.Context(), "GetCategoryResults", trace.WithAttributes(
		semconv.HTTPRequestMethodKey.String(r.Method),
		semconv.HTTPRouteKey.String("/discover/category/{category}"),
	))
	defer span.End()

	l := h.logger.With(slog.String("handler", "GetCategoryResults"))
	l.DebugContext(ctx, "GetCategoryResults handler invoked")

	// Get category from URL parameter
	category := chi.URLParam(r, "category")
	if category == "" {
		l.ErrorContext(ctx, "Category parameter is required")
		api.ErrorResponse(w, r, http.StatusBadRequest, "Category parameter is required")
		return
	}

	results, err := h.discoverService.GetCategoryResults(ctx, category)
	if err != nil {
		l.ErrorContext(ctx, "Failed to get category results", slog.String("category", category), slog.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to get category results")
		api.ErrorResponse(w, r, http.StatusInternalServerError, "Failed to load category results")
		return
	}

	span.SetAttributes(
		attribute.String("category", category),
		attribute.Int("count", len(results)),
	)
	span.SetStatus(codes.Ok, "Successfully retrieved category results")

	api.WriteJSONResponse(w, r, http.StatusOK, map[string]interface{}{
		"category": category,
		"results":  results,
	})
}
