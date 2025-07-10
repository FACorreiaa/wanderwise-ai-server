package recents

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"github.com/FACorreiaa/go-poi-au-suggestions/internal/api/auth"
	"github.com/FACorreiaa/go-poi-au-suggestions/internal/types"
)

var _ Handler = (*HandlerImpl)(nil)

type Handler interface {
	GetUserRecentInteractions(w http.ResponseWriter, r *http.Request)
	GetCityDetailsForUser(w http.ResponseWriter, r *http.Request)
}

type HandlerImpl struct {
	service Service
	logger  *slog.Logger
}

func NewHandler(service Service, logger *slog.Logger) *HandlerImpl {
	return &HandlerImpl{
		service: service,
		logger:  logger,
	}
}

// GetUserRecentInteractions retrieves recent interactions grouped by city
// @Summary Get user's recent interactions
// @Description Retrieves recent interactions grouped by city for the authenticated user
// @Tags recents
// @Accept json
// @Produce json
// @Param page query int false "Page number (default: 1)"
// @Param limit query int false "Limit number of cities per page (default: 10, max: 50)"
// @Param sort_by query string false "Sort field (last_activity, city_name, interaction_count, poi_count) (default: last_activity)"
// @Param sort_order query string false "Sort order (asc, desc) (default: desc)"
// @Param search query string false "Search term to filter by city name"
// @Param min_interactions query int false "Minimum number of interactions to filter by"
// @Param max_interactions query int false "Maximum number of interactions to filter by"
// @Success 200 {object} types.RecentInteractionsResponse
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/v1/recents [get]
func (h *HandlerImpl) GetUserRecentInteractions(w http.ResponseWriter, r *http.Request) {
	ctx, span := otel.Tracer("RecentsHandler").Start(r.Context(), "GetUserRecentInteractions")
	defer span.End()

	l := h.logger.With(slog.String("method", "GetUserRecentInteractions"))

	// Get user ID from context (set by auth middleware)
	userIDStr, ok := auth.GetUserIDFromContext(ctx)
	if !ok {
		l.ErrorContext(ctx, "User ID not found in context")
		span.RecordError(nil)
		span.SetStatus(codes.Error, "User not authenticated")
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Parse user ID as UUID
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		l.ErrorContext(ctx, "Invalid user ID format", slog.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Invalid user ID")
		http.Error(w, "Invalid user ID", http.StatusBadRequest)
		return
	}

	// Parse page parameter
	page := 1 // default
	if pageStr := r.URL.Query().Get("page"); pageStr != "" {
		if parsedPage, err := strconv.Atoi(pageStr); err == nil {
			page = parsedPage
		}
	}

	// Parse limit parameter
	limit := 10 // default
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if parsedLimit, err := strconv.Atoi(limitStr); err == nil {
			limit = parsedLimit
		}
	}

	// Parse sort_by parameter
	sortBy := r.URL.Query().Get("sort_by")
	if sortBy == "" {
		sortBy = "last_activity" // default
	}

	// Parse sort_order parameter
	sortOrder := r.URL.Query().Get("sort_order")
	if sortOrder == "" {
		sortOrder = "desc" // default
	}

	// Parse search parameter
	search := r.URL.Query().Get("search")

	// Parse min_interactions parameter
	minInteractions := -1 // Use -1 to indicate "no filter"
	if minStr := r.URL.Query().Get("min_interactions"); minStr != "" {
		if parsedMin, err := strconv.Atoi(minStr); err == nil && parsedMin >= 0 {
			minInteractions = parsedMin
		}
	}

	// Parse max_interactions parameter
	maxInteractions := -1 // Use -1 to indicate "no filter"
	if maxStr := r.URL.Query().Get("max_interactions"); maxStr != "" {
		if parsedMax, err := strconv.Atoi(maxStr); err == nil && parsedMax >= 0 {
			maxInteractions = parsedMax
		}
	}

	// Validate page
	if page <= 0 {
		page = 1
	}

	// Validate limit
	if limit <= 0 {
		limit = 10
	}
	if limit > 50 {
		limit = 50
	}

	// Validate sort_by
	validSortFields := map[string]bool{
		"last_activity":     true,
		"city_name":        true,
		"interaction_count": true,
		"poi_count":        true,
	}
	if !validSortFields[sortBy] {
		sortBy = "last_activity"
	}

	// Validate sort_order
	if sortOrder != "asc" && sortOrder != "desc" {
		sortOrder = "desc"
	}

	// Validate interactions range
	if maxInteractions >= 0 && minInteractions >= 0 && minInteractions > maxInteractions {
		// If min > max, disable both filters
		minInteractions = -1
		maxInteractions = -1
	}

	l.InfoContext(ctx, "Processing get recent interactions request", 
		slog.String("user_id", userID.String()),
		slog.Int("page", page),
		slog.Int("limit", limit),
		slog.String("sort_by", sortBy),
		slog.String("sort_order", sortOrder),
		slog.String("search", search),
		slog.Int("min_interactions", minInteractions),
		slog.Int("max_interactions", maxInteractions))

	span.SetAttributes(
		attribute.String("user_id", userID.String()),
		attribute.Int("page", page),
		attribute.Int("limit", limit),
		attribute.String("sort_by", sortBy),
		attribute.String("sort_order", sortOrder),
		attribute.String("search", search),
		attribute.Int("min_interactions", minInteractions),
		attribute.Int("max_interactions", maxInteractions),
	)

	// Create filter options
	filterOptions := &types.RecentInteractionsFilter{
		SortBy:          sortBy,
		SortOrder:       sortOrder,
		Search:          search,
		MinInteractions: minInteractions,
		MaxInteractions: maxInteractions,
	}

	// Call service to get recent interactions
	response, err := h.service.GetUserRecentInteractions(ctx, userID, page, limit, filterOptions)
	if err != nil {
		l.ErrorContext(ctx, "Failed to get recent interactions", slog.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to get recent interactions")
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Set response headers
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	// Encode response
	if err := json.NewEncoder(w).Encode(response); err != nil {
		l.ErrorContext(ctx, "Failed to encode response", slog.Any("error", err))
		span.RecordError(err)
		return
	}

	l.InfoContext(ctx, "Successfully returned recent interactions", 
		slog.String("user_id", userID.String()),
		slog.Int("cities_count", len(response.Cities)))

	span.SetAttributes(
		attribute.Int("response.cities_count", len(response.Cities)),
		attribute.Int("response.total", response.Total),
	)
	span.SetStatus(codes.Ok, "Recent interactions retrieved successfully")
}

// GetCityDetailsForUser retrieves detailed information for a specific city
// @Summary Get city details for user
// @Description Retrieves detailed information for a specific city from user's interactions
// @Tags recents
// @Accept json
// @Produce json
// @Param cityName path string true "City name"
// @Success 200 {object} types.CityInteractions
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/v1/recents/city/{cityName} [get]
func (h *HandlerImpl) GetCityDetailsForUser(w http.ResponseWriter, r *http.Request) {
	ctx, span := otel.Tracer("RecentsHandler").Start(r.Context(), "GetCityDetailsForUser")
	defer span.End()

	l := h.logger.With(slog.String("method", "GetCityDetailsForUser"))

	// Get user ID from context (set by auth middleware)
	userIDStr, ok := auth.GetUserIDFromContext(ctx)
	if !ok {
		l.ErrorContext(ctx, "User ID not found in context")
		span.RecordError(nil)
		span.SetStatus(codes.Error, "User not authenticated")
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Parse user ID as UUID
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		l.ErrorContext(ctx, "Invalid user ID format", slog.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Invalid user ID")
		http.Error(w, "Invalid user ID", http.StatusBadRequest)
		return
	}

	// Get city name from URL parameters
	cityName := chi.URLParam(r, "cityName")
	if cityName == "" {
		l.ErrorContext(ctx, "City name parameter is required")
		span.SetStatus(codes.Error, "Missing city name parameter")
		http.Error(w, "City name is required", http.StatusBadRequest)
		return
	}

	l.InfoContext(ctx, "Processing get city details request", 
		slog.String("user_id", userID.String()),
		slog.String("city_name", cityName))

	span.SetAttributes(
		attribute.String("user_id", userID.String()),
		attribute.String("city_name", cityName),
	)

	// Call service to get city details
	cityDetails, err := h.service.GetCityDetailsForUser(ctx, userID, cityName)
	if err != nil {
		l.ErrorContext(ctx, "Failed to get city details", 
			slog.String("city_name", cityName), 
			slog.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to get city details")
		
		// Check if it's a not found error
		if err.Error() == "no interactions found for city: "+cityName {
			http.Error(w, "City not found in user interactions", http.StatusNotFound)
			return
		}
		
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Set response headers
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	// Encode response
	if err := json.NewEncoder(w).Encode(cityDetails); err != nil {
		l.ErrorContext(ctx, "Failed to encode response", slog.Any("error", err))
		span.RecordError(err)
		return
	}

	l.InfoContext(ctx, "Successfully returned city details", 
		slog.String("user_id", userID.String()),
		slog.String("city_name", cityName),
		slog.Int("poi_count", cityDetails.POICount),
		slog.Int("interaction_count", len(cityDetails.Interactions)))

	span.SetAttributes(
		attribute.Int("response.poi_count", cityDetails.POICount),
		attribute.Int("response.interaction_count", len(cityDetails.Interactions)),
	)
	span.SetStatus(codes.Ok, "City details retrieved successfully")
}