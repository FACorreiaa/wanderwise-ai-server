package llmChat

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"go.opentelemetry.io/otel/attribute"

	"github.com/go-chi/chi/v5"
	"go.opentelemetry.io/otel/codes"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"

	"github.com/FACorreiaa/go-poi-au-suggestions/internal/api"
	"github.com/FACorreiaa/go-poi-au-suggestions/internal/api/auth"
	"github.com/FACorreiaa/go-poi-au-suggestions/internal/types"
)

var _ Handler = (*HandlerImpl)(nil)

type Handler interface {
	SaveItenerary(w http.ResponseWriter, r *http.Request)
	RemoveItenerary(w http.ResponseWriter, r *http.Request)
	GetPOIDetails(w http.ResponseWriter, r *http.Request)

	// Unified chat methods
	StartChatMessageStream(w http.ResponseWriter, r *http.Request)
	StartChatMessageStreamFree(w http.ResponseWriter, r *http.Request)
	ContinueChatSessionStream(w http.ResponseWriter, r *http.Request)
	// Chat session management
	GetUserChatSessions(w http.ResponseWriter, r *http.Request)
}
type HandlerImpl struct {
	llmInteractionService LlmInteractiontService
	streamHandler         StreamHandler
	validator             RequestValidator
	emitter               EventEmitter
	logger                *slog.Logger
}

func NewLLMHandlerImpl(llmInteractionService LlmInteractiontService, logger *slog.Logger) *HandlerImpl {
	streamHandler := NewBaseStreamHandler(logger)
	validator := NewBaseRequestValidator(logger)
	emitter := NewBaseEventEmitter(logger)

	return &HandlerImpl{
		llmInteractionService: llmInteractionService,
		streamHandler:         streamHandler,
		validator:             validator,
		emitter:               emitter,
		logger:                logger,
	}
}

func (HandlerImpl *HandlerImpl) SaveItenerary(w http.ResponseWriter, r *http.Request) {
	ctx, span := otel.Tracer("HandlerImpl").Start(r.Context(), "SaveItenerary", trace.WithAttributes(
		semconv.HTTPRequestMethodKey.String(r.Method),
		semconv.HTTPRouteKey.String("/llm_interaction/save_itinerary"),
	))
	defer span.End()

	l := HandlerImpl.logger.With(slog.String("HandlerImpl", "SaveItenerary"))
	l.DebugContext(ctx, "Saving itinerary")

	userIDStr, ok := auth.GetUserIDFromContext(ctx)
	if !ok || userIDStr == "" {
		l.ErrorContext(ctx, "User ID not found in context")
		api.ErrorResponse(w, r, http.StatusUnauthorized, "Authentication required")
		return
	}

	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		l.ErrorContext(ctx, "Invalid user ID format", slog.Any("error", err))
		api.ErrorResponse(w, r, http.StatusBadRequest, "Invalid user ID format")
		return
	}
	span.SetAttributes(semconv.EnduserIDKey.String(userID.String()))
	l = l.With(slog.String("userID", userID.String()))

	var req types.BookmarkRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		l.ErrorContext(ctx, "Failed to decode request body", slog.Any("error", err))
		api.ErrorResponse(w, r, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.LlmInteractionID == nil && req.SessionID == nil {
		l.ErrorContext(ctx, "Either LlmInteractionID or SessionID is required", slog.Any("request", req))
		api.ErrorResponse(w, r, http.StatusBadRequest, "Either llm_interaction_id or session_id is required")
		return
	}

	if strings.TrimSpace(req.Title) == "" {
		l.ErrorContext(ctx, "Title is required", slog.Any("title", req))
		api.ErrorResponse(w, r, http.StatusBadRequest, "Title is required")
		return
	}

	savedItinerary, err := HandlerImpl.llmInteractionService.SaveItenerary(ctx, userID, req)
	if err != nil {
		l.ErrorContext(ctx, "Failed to save itinerary", slog.Any("error", err))
		api.ErrorResponse(w, r, http.StatusInternalServerError, fmt.Sprintf("Failed to save itinerary: %s", err.Error()))
		return
	}

	l.InfoContext(ctx, "Itinerary saved successfully")
	api.WriteJSONResponse(w, r, http.StatusCreated, savedItinerary)
}

func (HandlerImpl *HandlerImpl) GetUserChatSessions(w http.ResponseWriter, r *http.Request) {
	ctx, span := otel.Tracer("HandlerImpl").Start(r.Context(), "GetUserChatSessions", trace.WithAttributes(
		semconv.HTTPRequestMethodKey.String(r.Method),
		semconv.HTTPRouteKey.String("/llm/prompt-response/chat/sessions/user/{profileID}"),
	))
	defer span.End()

	l := HandlerImpl.logger.With(slog.String("HandlerImpl", "GetUserChatSessions"))
	l.DebugContext(ctx, "Getting user chat sessions")

	// Get user ID from context
	userIDStr, ok := auth.GetUserIDFromContext(ctx)
	if !ok || userIDStr == "" {
		l.ErrorContext(ctx, "User ID not found in context")
		api.ErrorResponse(w, r, http.StatusUnauthorized, "Authentication required")
		return
	}

	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		l.ErrorContext(ctx, "Invalid user ID format", slog.Any("error", err))
		api.ErrorResponse(w, r, http.StatusBadRequest, "Invalid user ID format")
		return
	}
	span.SetAttributes(semconv.EnduserIDKey.String(userID.String()))
	l = l.With(slog.String("userID", userID.String()))

	// Get chat sessions from service
	sessions, err := HandlerImpl.llmInteractionService.GetUserChatSessions(ctx, userID)
	if err != nil {
		l.ErrorContext(ctx, "Failed to get user chat sessions", slog.Any("error", err))
		api.ErrorResponse(w, r, http.StatusInternalServerError, fmt.Sprintf("Failed to get chat sessions: %s", err.Error()))
		return
	}

	l.InfoContext(ctx, "Successfully retrieved user chat sessions", slog.Int("sessionCount", len(sessions)))
	api.WriteJSONResponse(w, r, http.StatusOK, sessions)
}

func (HandlerImpl *HandlerImpl) RemoveItenerary(w http.ResponseWriter, r *http.Request) {
	ctx, span := otel.Tracer("HandlerImpl").Start(r.Context(), "RemoveItenerary", trace.WithAttributes(
		semconv.HTTPRequestMethodKey.String(r.Method),
		semconv.HTTPRouteKey.String("/llm_interaction/remove_itinerary"),
	))
	defer span.End()

	l := HandlerImpl.logger.With(slog.String("HandlerImpl", "RemoveItenerary"))
	l.DebugContext(ctx, "Removing itinerary")

	userIDStr, ok := auth.GetUserIDFromContext(ctx)
	if !ok || userIDStr == "" {
		l.ErrorContext(ctx, "User ID not found in context")
		api.ErrorResponse(w, r, http.StatusUnauthorized, "Authentication required")
		return
	}

	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		l.ErrorContext(ctx, "Invalid user ID format", slog.Any("error", err))
		api.ErrorResponse(w, r, http.StatusBadRequest, "Invalid user ID format")
		return
	}
	span.SetAttributes(semconv.EnduserIDKey.String(userID.String()))
	l = l.With(slog.String("userID", userID.String()))

	itineraryIDStr := chi.URLParam(r, "itineraryID")
	itineraryID, err := uuid.Parse(itineraryIDStr)
	if err != nil {
		l.ErrorContext(ctx, "Invalid itinerary ID format", slog.Any("error", err))
		api.ErrorResponse(w, r, http.StatusBadRequest, "Invalid itinerary ID format")
		return
	}
	span.SetAttributes(attribute.String("app.itinerary.id", itineraryID.String()))
	l = l.With(slog.String("itineraryID", itineraryID.String()))

	if err := HandlerImpl.llmInteractionService.RemoveItenerary(ctx, userID, itineraryID); err != nil {
		l.ErrorContext(ctx, "Failed to remove itinerary", slog.Any("error", err))
		api.ErrorResponse(w, r, http.StatusInternalServerError, fmt.Sprintf("Failed to remove itinerary: %s", err.Error()))
		return
	}

	l.InfoContext(ctx, "Itinerary removed successfully")
	api.WriteJSONResponse(w, r, http.StatusNoContent, nil)
}

func (HandlerImpl *HandlerImpl) GetPOIDetails(w http.ResponseWriter, r *http.Request) {
	ctx, span := otel.Tracer("HandlerImpl").Start(r.Context(), "GetPOIDetails", trace.WithAttributes(
		semconv.HTTPRequestMethodKey.String(r.Method),
		semconv.HTTPRouteKey.String("/llm_interaction/get_poi_details"),
	))
	defer span.End()

	l := HandlerImpl.logger.With(slog.String("HandlerImpl", "GetPOIDetails"))
	l.DebugContext(ctx, "Get POI details")

	// Authenticate user
	userIDStr, ok := auth.GetUserIDFromContext(ctx)
	if !ok || userIDStr == "" {
		l.ErrorContext(ctx, "User ID not found in context")
		span.SetStatus(codes.Error, "Unauthorized")
		api.ErrorResponse(w, r, http.StatusUnauthorized, "Authentication required")
		return
	}

	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		l.ErrorContext(ctx, "Invalid user ID format", slog.Any("error", err))
		span.SetStatus(codes.Error, "Invalid user ID")
		api.ErrorResponse(w, r, http.StatusBadRequest, "Invalid user ID format")
		return
	}
	span.SetAttributes(semconv.EnduserIDKey.String(userID.String()))
	l = l.With(slog.String("userID", userID.String()))

	// Decode request body
	var req types.POIDetailrequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		l.ErrorContext(ctx, "Failed to decode request body", slog.Any("error", err))
		span.SetStatus(codes.Error, "Invalid request body")
		api.ErrorResponse(w, r, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Validate request
	if req.CityName == "" {
		l.ErrorContext(ctx, "City name is required")
		span.SetStatus(codes.Error, "Missing city name")
		api.ErrorResponse(w, r, http.StatusBadRequest, "City name is required")
		return
	}
	if req.Latitude < -90 || req.Latitude > 90 || req.Longitude < -180 || req.Longitude > 180 {
		l.ErrorContext(ctx, "Invalid coordinates", slog.Float64("latitude", req.Latitude), slog.Float64("longitude", req.Longitude))
		span.SetStatus(codes.Error, "Invalid coordinates")
		api.ErrorResponse(w, r, http.StatusBadRequest, "Invalid latitude or longitude")
		return
	}

	// Convert to service request type (if different)
	serviceReq := types.POIDetailrequest{
		CityName:  req.CityName,
		Latitude:  req.Latitude,
		Longitude: req.Longitude,
	}

	// Call service to get POI details
	pois, err := HandlerImpl.llmInteractionService.GetPOIDetailedInfosResponse(ctx, userID, serviceReq.CityName, serviceReq.Latitude, serviceReq.Longitude)
	if err != nil {
		l.ErrorContext(ctx, "Failed to fetch POI details", slog.Any("error", err))
		span.SetStatus(codes.Error, "Service error")
		api.ErrorResponse(w, r, http.StatusInternalServerError, "Failed to fetch POI details")
		return
	}

	// Prepare response
	response := struct {
		POIs *types.POIDetailedInfo `json:"pois"`
	}{
		POIs: pois,
	}

	// Encode response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		l.ErrorContext(ctx, "Failed to encode response", slog.Any("error", err))
		span.SetStatus(codes.Error, "Response encoding failed")
		return
	}

	l.InfoContext(ctx, "Successfully fetched POI details")
	span.SetStatus(codes.Ok, "Success")
}

// Stream Handlers
func (h *HandlerImpl) writeSSEError(w http.ResponseWriter, errorMsg string) {
	event := types.StreamEvent{
		Type:      types.EventTypeError,
		Error:     errorMsg,
		Timestamp: time.Now(),
		EventID:   uuid.New().String(),
	}
	data, _ := json.Marshal(event)
	fmt.Fprintf(w, "id: %s\n", event.EventID)
	fmt.Fprintf(w, "event: %s\n", event.Type)
	fmt.Fprintf(w, "data: %s\n\n", data)
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}
}

// StartChatMessageStream handles unified chat requests with streaming
func (h *HandlerImpl) StartChatMessageStream(w http.ResponseWriter, r *http.Request) {
	ctx, span := otel.Tracer("HandlerImpl").Start(r.Context(), "StartChatMessageStream", trace.WithAttributes(
		semconv.HTTPRequestMethodKey.String(r.Method),
		semconv.HTTPRouteKey.String("/prompt-response/chat/sessions/stream/{profileID}"),
	))
	defer span.End()

	l := h.logger.With(slog.String("handler", "StartChatMessageStream"))
	l.DebugContext(ctx, "Processing unified chat message with streaming")

	// Create processor
	processor := NewUnifiedChatStreamProcessor(
		h.llmInteractionService,
		h.validator,
		h.emitter,
		r,
	)

	// Process stream using base handler
	if err := h.streamHandler.ProcessStream(ctx, w, r, processor); err != nil {
		l.ErrorContext(ctx, "Failed to process stream", slog.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Stream processing failed")
		return
	}

	span.SetStatus(codes.Ok, "Stream processed successfully")
}

// StartChatMessageStreamFree handles free chat requests with streaming
func (h *HandlerImpl) StartChatMessageStreamFree(w http.ResponseWriter, r *http.Request) {
	ctx, span := otel.Tracer("HandlerImpl").Start(r.Context(), "StartChatMessageStreamFree", trace.WithAttributes(
		semconv.HTTPRequestMethodKey.String(r.Method),
		semconv.HTTPRouteKey.String("/llm/chat/stream/free"),
	))
	defer span.End()

	l := h.logger.With(slog.String("handler", "StartChatMessageStreamFree"))
	l.DebugContext(ctx, "Processing free chat message with streaming")

	// Create processor
	processor := NewFreeChatStreamProcessor(
		h.llmInteractionService,
		h.validator,
		h.emitter,
		r,
	)

	// Process stream using base handler
	if err := h.streamHandler.ProcessStream(ctx, w, r, processor); err != nil {
		l.ErrorContext(ctx, "Failed to process stream", slog.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Stream processing failed")
		return
	}

	span.SetStatus(codes.Ok, "Stream processed successfully")
}

func (h *HandlerImpl) ContinueChatSessionStream(w http.ResponseWriter, r *http.Request) {
	ctx, span := otel.Tracer("HandlerImpl").Start(r.Context(), "ContinueChatSessionStream", trace.WithAttributes(
		semconv.HTTPRequestMethodKey.String(r.Method),
		semconv.HTTPRouteKey.String("/prompt-response/chat/sessions/{sessionID}/continue"),
	))
	defer span.End()

	l := h.logger.With(slog.String("handler", "ContinueChatSessionStream"))
	l.DebugContext(ctx, "Processing continue chat session with streaming")

	// Create processor
	processor := NewContinueChatStreamProcessor(
		h.llmInteractionService,
		h.validator,
		h.emitter,
		r,
	)

	// Process stream using base handler
	if err := h.streamHandler.ProcessStream(ctx, w, r, processor); err != nil {
		l.ErrorContext(ctx, "Failed to process stream", slog.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Stream processing failed")
		return
	}

	span.SetStatus(codes.Ok, "Stream processed successfully")
}
