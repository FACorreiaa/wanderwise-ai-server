package statistics

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"

	"github.com/FACorreiaa/go-poi-au-suggestions/config"
	"github.com/FACorreiaa/go-poi-au-suggestions/internal/api"
	"github.com/FACorreiaa/go-poi-au-suggestions/internal/api/auth"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

type HandlerImpl struct {
	service Service
	logger  *slog.Logger
	jwtCfg  config.JWTConfig
}

func NewHandler(service Service, logger *slog.Logger, jwtCfg config.JWTConfig) *HandlerImpl {
	return &HandlerImpl{
		service: service,
		logger:  logger,
		jwtCfg:  jwtCfg,
	}
}

// GetMainPageStatisticsHandler godoc
// @Summary      Get Main Page Statistics
// @Description  Retrieves main page statistics for the authenticated user including total users, itineraries, and POIs
// @Tags         Statistics
// @Accept       json
// @Produce      json
// @Success      200 {object} interface{} "Main page statistics"
// @Failure      400 {object} types.Response "Invalid Input"
// @Failure      401 {object} types.Response "Authentication required"
// @Failure      500 {object} types.Response "Internal Server Error"
// @Security     BearerAuth
// @Router       /statistics/main [get]
func (h *HandlerImpl) GetMainPageStatisticsHandler(w http.ResponseWriter, r *http.Request) {
	ctx, span := otel.Tracer("StatisticsHandler").Start(r.Context(), "GetMainPageStatistics")
	defer span.End()

	// Get user ID from auth context
	userIDStr, ok := auth.GetUserIDFromContext(ctx)
	if !ok || userIDStr == "" {
		h.logger.ErrorContext(ctx, "User ID not found in context")
		api.ErrorResponse(w, r, http.StatusUnauthorized, "Authentication required")
		return
	}

	l := h.logger.With(slog.String("HandlerImpl", "SaveItenerary"))
	l.DebugContext(ctx, "Saving itinerary")

	// googleUUID, err := uuid.Parse(userIDStr)
	// if err != nil {
	// 	h.logger.ErrorContext(ctx, "Invalid user ID format", slog.Any("error", err))
	// 	api.ErrorResponse(w, r, http.StatusBadRequest, "Invalid user ID format")
	// 	return
	// }

	// // Convert google UUID to pgx UUID for repository call
	// var userID [16]byte
	// copy(userID[:], googleUUID[:])

	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		l.ErrorContext(ctx, "Invalid user ID format", slog.Any("error", err))
		api.ErrorResponse(w, r, http.StatusBadRequest, "Invalid user ID format")
		return
	}

	span.SetAttributes(semconv.EnduserIDKey.String(userID.String()))

	stats, err := h.service.GetMainPageStatistics(ctx, userID)
	if err != nil {
		h.logger.ErrorContext(ctx, "Failed to get main page statistics", slog.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to get statistics")
		api.ErrorResponse(w, r, http.StatusInternalServerError, "Failed to retrieve statistics")
		return
	}

	span.SetAttributes(
		attribute.Int64("stats.total_users", stats.TotalUsersCount),
		attribute.Int64("stats.total_itineraries", stats.TotalItinerariesSaved),
		attribute.Int64("stats.total_pois", stats.TotalUniquePOIs),
	)
	span.SetStatus(codes.Ok, "Statistics retrieved successfully")

	api.WriteJSONResponse(w, r, http.StatusOK, stats)
}

// StatisticsSSEHandler godoc
// @Summary      Statistics Server-Sent Events
// @Description  Provides real-time statistics updates via Server-Sent Events (SSE) for aggregate statistics
// @Tags         Statistics
// @Accept       json
// @Produce      text/event-stream
// @Success      200 {string} string "Event stream connection established"
// @Failure      500 {object} types.Response "Internal Server Error"
// @Router       /statistics/sse [get]
func (h *HandlerImpl) StatisticsSSEHandler(w http.ResponseWriter, r *http.Request) {
	ctx, span := otel.Tracer("StatisticsHandler").Start(r.Context(), "StatisticsSSE")
	defer span.End()

	h.logger.InfoContext(ctx, "New public SSE connection for aggregate statistics")

	// No authentication required for aggregate statistics - this is public data
	// We'll use a default/system user ID for getting aggregate stats
	userID := uuid.MustParse("00000000-0000-0000-0000-000000000000") // System/aggregate user

	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Cache-Control")

	// Create a flusher to send data immediately
	flusher, ok := w.(http.Flusher)
	if !ok {
		h.logger.ErrorContext(ctx, "Streaming unsupported")
		api.ErrorResponse(w, r, http.StatusInternalServerError, "Streaming not supported")
		return
	}

	// Send initial statistics immediately
	if err := h.sendStatisticsEvent(ctx, w, flusher, userID, "initial"); err != nil {
		h.logger.ErrorContext(ctx, "Failed to send initial statistics", slog.Any("error", err))
		return
	}

	// Create a ticker for periodic updates (every 30 seconds)
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	// Handle client disconnect
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Monitor client disconnect
	go func() {
		<-r.Context().Done()
		cancel()
		h.logger.InfoContext(ctx, "SSE client disconnected")
	}()

	// Send periodic updates
	for {
		select {
		case <-ctx.Done():
			h.logger.InfoContext(ctx, "SSE connection context cancelled")
			span.SetStatus(codes.Ok, "SSE connection closed")
			return
		case <-ticker.C:
			if err := h.sendStatisticsEvent(ctx, w, flusher, userID, "update"); err != nil {
				h.logger.ErrorContext(ctx, "Failed to send statistics update", slog.Any("error", err))
				span.RecordError(err)
				return
			}
		}
	}
}

// sendStatisticsEvent sends a statistics event via SSE
func (h *HandlerImpl) sendStatisticsEvent(ctx context.Context, w http.ResponseWriter, flusher http.Flusher, userID uuid.UUID, eventType string) error {
	stats, err := h.service.GetMainPageStatistics(ctx, userID)
	if err != nil {
		return fmt.Errorf("failed to get statistics: %w", err)
	}

	// Create event payload
	eventData := map[string]interface{}{
		"type":      eventType,
		"timestamp": time.Now().Unix(),
		"data":      stats,
	}

	// Marshal to JSON
	jsonData, err := json.Marshal(eventData)
	if err != nil {
		return fmt.Errorf("failed to marshal event data: %w", err)
	}

	// Send SSE event
	fmt.Fprintf(w, "id: %d\n", time.Now().Unix())
	fmt.Fprintf(w, "event: statistics\n")
	fmt.Fprintf(w, "data: %s\n\n", jsonData)

	// Flush immediately
	flusher.Flush()

	h.logger.DebugContext(ctx, "Sent statistics SSE event",
		slog.String("event_type", eventType),
		slog.Int64("total_users", stats.TotalUsersCount),
		slog.Int64("total_itineraries", stats.TotalItinerariesSaved),
		slog.Int64("total_pois", stats.TotalUniquePOIs))

	return nil
}

// GetDetailedPOIStatisticsHandler godoc
// @Summary      Get Detailed POI Statistics
// @Description  Retrieves detailed POI statistics for the authenticated user including general POIs, suggested POIs, hotels, restaurants
// @Tags         Statistics
// @Accept       json
// @Produce      json
// @Success      200 {object} interface{} "Detailed POI statistics"
// @Failure      400 {object} types.Response "Invalid Input"
// @Failure      401 {object} types.Response "Authentication required"
// @Failure      500 {object} types.Response "Internal Server Error"
// @Security     BearerAuth
// @Router       /statistics/poi [get]
func (h *HandlerImpl) GetDetailedPOIStatisticsHandler(w http.ResponseWriter, r *http.Request) {
	ctx, span := otel.Tracer("StatisticsHandler").Start(r.Context(), "GetDetailedPOIStatistics")
	defer span.End()

	// Get user ID from auth context
	userIDStr, ok := auth.GetUserIDFromContext(ctx)
	if !ok || userIDStr == "" {
		h.logger.ErrorContext(ctx, "User ID not found in context")
		api.ErrorResponse(w, r, http.StatusUnauthorized, "Authentication required")
		return
	}

	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		h.logger.ErrorContext(ctx, "Invalid user ID format", slog.Any("error", err))
		api.ErrorResponse(w, r, http.StatusBadRequest, "Invalid user ID format")
		return
	}

	stats, err := h.service.GetDetailedPOIStatistics(ctx, userID)
	if err != nil {
		h.logger.ErrorContext(ctx, "Failed to get detailed POI statistics", slog.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to get detailed statistics")
		api.ErrorResponse(w, r, http.StatusInternalServerError, "Failed to retrieve detailed statistics")
		return
	}

	span.SetAttributes(
		attribute.Int64("stats.general_pois", stats.GeneralPOIs),
		attribute.Int64("stats.suggested_pois", stats.SuggestedPOIs),
		attribute.Int64("stats.hotels", stats.Hotels),
		attribute.Int64("stats.restaurants", stats.Restaurants),
		attribute.Int64("stats.total_pois", stats.TotalPOIs),
	)
	span.SetStatus(codes.Ok, "Detailed statistics retrieved successfully")

	api.WriteJSONResponse(w, r, http.StatusOK, stats)
}

// GetLandingPageStatisticsHandler godoc
// @Summary      Get Landing Page Statistics
// @Description  Retrieves user-specific landing page statistics including saved places, itineraries, cities explored, and discoveries
// @Tags         Statistics
// @Accept       json
// @Produce      json
// @Success      200 {object} interface{} "Landing page statistics"
// @Failure      400 {object} types.Response "Invalid Input"
// @Failure      401 {object} types.Response "Authentication required"
// @Failure      500 {object} types.Response "Internal Server Error"
// @Security     BearerAuth
// @Router       /statistics/landing [get]
func (h *HandlerImpl) GetLandingPageStatisticsHandler(w http.ResponseWriter, r *http.Request) {
	ctx, span := otel.Tracer("StatisticsHandler").Start(r.Context(), "GetLandingPageStatistics")
	defer span.End()

	// Get user ID from auth context
	userIDStr, ok := auth.GetUserIDFromContext(ctx)
	if !ok || userIDStr == "" {
		h.logger.ErrorContext(ctx, "User ID not found in context")
		api.ErrorResponse(w, r, http.StatusUnauthorized, "Authentication required")
		return
	}

	userID, err := uuid.Parse(userIDStr)
	fmt.Printf("userID %v \n", userID)
	if err != nil {
		h.logger.ErrorContext(ctx, "Invalid user ID format", slog.Any("error", err))
		api.ErrorResponse(w, r, http.StatusBadRequest, "Invalid user ID format")
		return
	}

	span.SetAttributes(semconv.EnduserIDKey.String(userID.String()))

	stats, err := h.service.GetLandingPageStatistics(ctx, userID)
	if err != nil {
		h.logger.ErrorContext(ctx, "Failed to get landing page statistics", slog.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to get landing page statistics")
		api.ErrorResponse(w, r, http.StatusInternalServerError, "Failed to retrieve landing page statistics")
		return
	}

	span.SetAttributes(
		attribute.Int("stats.saved_places", stats.SavedPlaces),
		attribute.Int("stats.itineraries", stats.Itineraries),
		attribute.Int("stats.cities_explored", stats.CitiesExplored),
		attribute.Int("stats.discoveries", stats.Discoveries),
	)
	span.SetStatus(codes.Ok, "Landing page statistics retrieved successfully")

	api.WriteJSONResponse(w, r, http.StatusOK, stats)
}
