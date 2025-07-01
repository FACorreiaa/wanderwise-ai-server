package llmChat

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/FACorreiaa/go-poi-au-suggestions/internal/api"
	"github.com/FACorreiaa/go-poi-au-suggestions/internal/api/auth"
	"github.com/FACorreiaa/go-poi-au-suggestions/internal/types"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
)

type StreamingHandler struct {
	llmService LlmInteractiontService
	logger     *slog.Logger
}

func NewStreamingHandler(llmService LlmInteractiontService, logger *slog.Logger) *StreamingHandler {
	return &StreamingHandler{
		llmService: llmService,
		logger:     logger,
	}
}

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

// ContinueSessionStreamHandler handles streaming requests for continuing a session
func (h *HandlerImpl) ContinueSessionStreamHandler(w http.ResponseWriter, r *http.Request) {
	ctx, span := otel.Tracer("Handler").Start(r.Context(), "ContinueSessionStreamHandler")
	defer span.End()

	// Set headers for SSE
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // Disable buffering for some proxies

	// Parse request body with context support
	var req struct {
		SessionID    string                `json:"session_id"`
		Message      string                `json:"message"`
		UserLocation *types.UserLocation   `json:"user_location"`
		CityName     string                `json:"city_name,omitempty"`
		ContextType  types.ChatContextType `json:"context_type,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.ErrorContext(ctx, "Failed to decode request body", slog.Any("error", err))
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		span.RecordError(err)
		return
	}

	// Default to general context for backward compatibility
	if req.ContextType == "" {
		req.ContextType = types.ContextGeneral
	}

	// Validate and parse sessionID
	sessionIDStr := chi.URLParam(r, "sessionID")
	sessionID, err := uuid.Parse(sessionIDStr)
	if err != nil {
		h.logger.ErrorContext(ctx, "Invalid session ID", slog.String("sessionID", sessionIDStr), slog.Any("error", err))
		http.Error(w, "Invalid session ID", http.StatusBadRequest)
		span.RecordError(err)
		return
	}

	// Validate message
	if req.Message == "" {
		h.logger.ErrorContext(ctx, "Message is empty")
		http.Error(w, "Message cannot be empty", http.StatusBadRequest)
		span.RecordError(fmt.Errorf("message is empty"))
		return
	}

	// Validate userLocation (if provided)
	if req.UserLocation != nil && (req.UserLocation.UserLat == 0 || req.UserLocation.UserLon == 0) {
		h.logger.WarnContext(ctx, "Invalid user location, ignoring", slog.Any("userLocation", req.UserLocation))
		req.UserLocation = nil // Ignore invalid location
	}

	// Create channel for streaming events
	eventCh := make(chan types.StreamEvent)
	defer close(eventCh) // Ensure channel is closed when handler exits

	// Start the service in a goroutine with context support
	go func() {
		err := h.llmInteractionService.ContinueSessionStreamed(ctx, sessionID, req.Message, req.UserLocation, eventCh)
		if err != nil {
			// Fallback to original method
			err = h.llmInteractionService.ContinueSessionStreamed(ctx, sessionID, req.Message, req.UserLocation, eventCh)
			if err != nil {
				h.logger.ErrorContext(ctx, "ContinueSessionStreamed failed", slog.Any("error", err))
				span.RecordError(err)
				// Send error event if the channel is still open
				select {
				case eventCh <- types.StreamEvent{
					Type:      string(types.TypeError),
					Error:     err.Error(),
					IsFinal:   true,
					EventID:   uuid.New().String(),
					Timestamp: time.Now(),
				}:
				case <-ctx.Done():
				}
			}
		}
	}()

	// Stream events to the client
	flusher, ok := w.(http.Flusher)
	if !ok {
		h.logger.ErrorContext(ctx, "ResponseWriter does not support flushing")
		http.Error(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}

	for {
		select {
		case event := <-eventCh:
			// Marshal event to JSON
			data, err := json.Marshal(event)
			if err != nil {
				h.logger.WarnContext(ctx, "Failed to marshal event", slog.Any("error", err))
				continue
			}

			// Write SSE event
			fmt.Fprintf(w, "event: %s\n", event.Type)
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()

			span.AddEvent("Sent SSE event", trace.WithAttributes(
				attribute.String("event.type", event.Type),
				attribute.String("event.id", event.EventID),
			))

			if event.IsFinal {
				return // Exit after final event
			}

		case <-ctx.Done():
			h.logger.InfoContext(ctx, "Client disconnected or context cancelled")
			span.AddEvent("Client disconnected")
			return
		}
	}
}

// ProcessUnifiedChatMessageStream handles unified chat requests with streaming
func (h *HandlerImpl) ProcessUnifiedChatMessageStream(w http.ResponseWriter, r *http.Request) {
	ctx, span := otel.Tracer("HandlerImpl").Start(r.Context(), "ProcessUnifiedChatMessageStream", trace.WithAttributes(
		semconv.HTTPRequestMethodKey.String(r.Method),
		semconv.HTTPRouteKey.String("/prompt-response/unified-chat/stream"),
	))
	defer span.End()

	l := h.logger.With(slog.String("handler", "ProcessUnifiedChatMessageStream"))
	l.DebugContext(ctx, "Processing unified chat message with streaming")

	// Parse profile ID from URL - handle 'guest' as special case
	profileIDStr := chi.URLParam(r, "profileID")
	var profileID uuid.UUID
	var err error
	
	if profileIDStr == "guest" {
		// Use nil UUID for guest users
		profileID = uuid.Nil
		l.DebugContext(ctx, "Guest profile detected", slog.String("profileID", profileIDStr))
	} else {
		profileID, err = uuid.Parse(profileIDStr)
		if err != nil {
			l.ErrorContext(ctx, "Invalid profile ID", slog.String("profileID", profileIDStr), slog.Any("error", err))
			span.RecordError(err)
			span.SetStatus(codes.Error, "Invalid profile ID")
			api.ErrorResponse(w, r, http.StatusBadRequest, "Invalid profile ID")
			return
		}
	}

	// Get user ID from auth context (optional for guest users)
	var userID uuid.UUID
	var isGuest bool
	userIDStr, ok := auth.GetUserIDFromContext(ctx)
	if !ok || userIDStr == "" {
		// Guest user - generate a temporary UUID or use nil UUID
		l.DebugContext(ctx, "No user ID found in context, treating as guest user")
		isGuest = true
		userID = uuid.Nil // Use nil UUID for guest users
	} else {
		var err error
		userID, err = uuid.Parse(userIDStr)
		if err != nil {
			l.ErrorContext(ctx, "Invalid user ID format", slog.Any("error", err))
			api.ErrorResponse(w, r, http.StatusBadRequest, "Invalid user ID format")
			return
		}
		isGuest = false
	}

	// Parse request body
	var req struct {
		Message      string              `json:"message"`
		UserLocation *types.UserLocation `json:"user_location,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		l.ErrorContext(ctx, "Failed to decode request body", slog.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Invalid request body")
		api.ErrorResponse(w, r, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Validate required fields
	if req.Message == "" {
		l.ErrorContext(ctx, "Missing required fields", slog.String("message", req.Message))
		span.SetStatus(codes.Error, "Missing required fields")
		api.ErrorResponse(w, r, http.StatusBadRequest, "message is required")
		return
	}

	span.SetAttributes(
		attribute.String("user.id", userID.String()),
		attribute.String("profile.id", profileID.String()),
		attribute.String("message", req.Message),
		attribute.Bool("is_guest", isGuest),
	)

	// Set up SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Cache-Control")

	// Create event channel
	eventCh := make(chan types.StreamEvent, 100)

	go func() {
		l.InfoContext(ctx, "REST calling service with params",
			slog.String("userID", userID.String()),
			slog.String("profileID", profileID.String()),
			slog.String("cityName", ""),
			slog.String("message", req.Message),
			slog.Bool("isGuest", isGuest))
		err := h.llmInteractionService.ProcessUnifiedChatMessageStream(
			ctx, userID, profileID, "", req.Message, req.UserLocation, eventCh,
		)
		if err != nil {
			l.ErrorContext(ctx, "Failed to process unified chat message stream", slog.Any("error", err))
			span.RecordError(err)

			// Safely send error event, check if context is still active
			select {
			case eventCh <- types.StreamEvent{
				Type:      types.EventTypeError,
				Error:     err.Error(),
				Timestamp: time.Now(),
				EventID:   uuid.New().String(),
			}:
				// Event sent successfully
			case <-ctx.Done():
				// Context cancelled, don't send event
				return
			}
		}
	}()

	// Set up flusher for real-time streaming
	flusher, ok := w.(http.Flusher)
	if !ok {
		l.ErrorContext(ctx, "Response writer does not support flushing")
		span.SetStatus(codes.Error, "Streaming not supported")
		api.ErrorResponse(w, r, http.StatusInternalServerError, "Streaming not supported")
		return
	}

	// Process events in real-time as they arrive
	for {
		select {
		case event, ok := <-eventCh:
			if !ok {
				l.InfoContext(ctx, "Event channel closed, ending stream")
				span.SetStatus(codes.Ok, "Stream completed")
				return
			}

			eventData, err := json.Marshal(event)
			if err != nil {
				l.ErrorContext(ctx, "Failed to marshal event", slog.Any("error", err))
				span.RecordError(err)
				continue
			}

			fmt.Fprintf(w, "data: %s\n\n", eventData)
			flusher.Flush() // Send immediately to client

			if event.Type == types.EventTypeComplete || event.Type == types.EventTypeError {
				l.InfoContext(ctx, "Stream completed", slog.String("eventType", event.Type))
				span.SetStatus(codes.Ok, "Stream completed")
				return
			}

		case <-r.Context().Done():
			l.InfoContext(ctx, "Client disconnected")
			span.SetStatus(codes.Ok, "Client disconnected")
			return
		}
	}
}

func (h *HandlerImpl) ContinueChatSessionHandlerStream(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sessionIDStr := chi.URLParam(r, "sessionID")
	sessionID, err := uuid.Parse(sessionIDStr)
	if err != nil {
		api.ErrorResponse(w, r, http.StatusBadRequest, "Invalid session ID")
		return
	}

	// Support both legacy and new request formats
	var req struct {
		Message      string                `json:"message"`
		CityName     string                `json:"city_name,omitempty"`
		ContextType  types.ChatContextType `json:"context_type,omitempty"`
		UserLocation *types.UserLocation   `json:"user_location,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.ErrorResponse(w, r, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Default to general context for backward compatibility
	if req.ContextType == "" {
		req.ContextType = types.ContextGeneral
	}

	// Set up SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Cache-Control")

	// Create event channel
	eventCh := make(chan types.StreamEvent, 100)

	// Start processing in a goroutine
	go func() {
		err := h.llmInteractionService.ContinueSessionStreamed(ctx, sessionID, req.Message, req.UserLocation, eventCh)
		if err != nil {
			// Safely send error event, check if context is still active
			select {
			case eventCh <- types.StreamEvent{
				Type:      types.EventTypeError,
				Error:     err.Error(),
				Timestamp: time.Now(),
				EventID:   uuid.New().String(),
			}:
				// Event sent successfully
			case <-ctx.Done():
				// Context cancelled, don't send event
				return
			}
		}
	}()

	// Set up flusher for real-time streaming
	flusher, ok := w.(http.Flusher)
	if !ok {
		api.ErrorResponse(w, r, http.StatusInternalServerError, "Streaming not supported")
		return
	}

	// Process events in real-time as they arrive
	for {
		select {
		case event, ok := <-eventCh:
			if !ok {
				return
			}

			eventData, err := json.Marshal(event)
			if err != nil {
				continue
			}

			fmt.Fprintf(w, "data: %s\n\n", eventData)
			flusher.Flush() // Send immediately to client

			if event.Type == types.EventTypeComplete || event.Type == types.EventTypeError {
				return
			}

		case <-r.Context().Done():
			return
		}
	}
}

// ProcessGuestChatMessageStream handles guest chat requests without authentication
// Uses default preferences and doesn't require user profile
func (h *HandlerImpl) ProcessGuestChatMessageStream(w http.ResponseWriter, r *http.Request) {
	ctx, span := otel.Tracer("HandlerImpl").Start(r.Context(), "ProcessGuestChatMessageStream", trace.WithAttributes(
		semconv.HTTPRequestMethodKey.String(r.Method),
		semconv.HTTPRouteKey.String("/llm/guest/chat/stream"),
	))
	defer span.End()

	l := h.logger.With(slog.String("handler", "ProcessGuestChatMessageStream"))
	l.DebugContext(ctx, "Processing guest chat message with streaming")

	// Parse request body
	var req struct {
		Message      string              `json:"message"`
		UserLocation *types.UserLocation `json:"user_location,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		l.ErrorContext(ctx, "Failed to decode request body", slog.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Invalid request body")
		api.ErrorResponse(w, r, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Validate required fields
	if req.Message == "" {
		l.ErrorContext(ctx, "Missing required fields", slog.String("message", req.Message))
		span.SetStatus(codes.Error, "Missing required fields")
		api.ErrorResponse(w, r, http.StatusBadRequest, "message is required")
		return
	}

	span.SetAttributes(
		attribute.String("user.id", "guest"),
		attribute.String("profile.id", "guest"),
		attribute.String("message", req.Message),
		attribute.Bool("is_guest", true),
	)

	// Set up SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Cache-Control")

	// Create event channel
	eventCh := make(chan types.StreamEvent, 100)

	go func() {
		l.InfoContext(ctx, "Processing guest chat message",
			slog.String("message", req.Message))
		
		// Use nil UUIDs for guest users (no user preferences will be loaded)
		err := h.llmInteractionService.ProcessUnifiedChatMessageStream(
			ctx, uuid.Nil, uuid.Nil, "", req.Message, req.UserLocation, eventCh,
		)
		if err != nil {
			l.ErrorContext(ctx, "Failed to process guest chat message stream", slog.Any("error", err))
			span.RecordError(err)

			// Safely send error event
			select {
			case eventCh <- types.StreamEvent{
				Type:      types.EventTypeError,
				Error:     err.Error(),
				Timestamp: time.Now(),
				EventID:   uuid.New().String(),
			}:
			case <-ctx.Done():
				return
			}
		}
	}()

	// Set up flusher for real-time streaming
	flusher, ok := w.(http.Flusher)
	if !ok {
		l.ErrorContext(ctx, "Response writer does not support flushing")
		span.SetStatus(codes.Error, "Streaming not supported")
		api.ErrorResponse(w, r, http.StatusInternalServerError, "Streaming not supported")
		return
	}

	// Process events in real-time as they arrive
	for {
		select {
		case event, ok := <-eventCh:
			if !ok {
				l.InfoContext(ctx, "Event channel closed, ending guest stream")
				span.SetStatus(codes.Ok, "Stream completed")
				return
			}

			eventData, err := json.Marshal(event)
			if err != nil {
				l.ErrorContext(ctx, "Failed to marshal event", slog.Any("error", err))
				span.RecordError(err)
				continue
			}

			fmt.Fprintf(w, "data: %s\n\n", eventData)
			flusher.Flush() // Send immediately to client

			if event.Type == types.EventTypeComplete || event.Type == types.EventTypeError {
				l.InfoContext(ctx, "Guest stream completed", slog.String("eventType", event.Type))
				span.SetStatus(codes.Ok, "Stream completed")
				return
			}

		case <-r.Context().Done():
			l.InfoContext(ctx, "Guest client disconnected")
			span.SetStatus(codes.Ok, "Client disconnected")
			return
		}
	}
}
