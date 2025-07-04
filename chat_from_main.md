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
logger                *slog.Logger
}

func NewLLMHandlerImpl(llmInteractionService LlmInteractiontService, logger *slog.Logger) *HandlerImpl {
return &HandlerImpl{
llmInteractionService: llmInteractionService,
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
ctx, span := otel.Tracer("HandlerImpl").Start(r.Context(), "ProcessUnifiedChatMessageStream", trace.WithAttributes(
semconv.HTTPRequestMethodKey.String(r.Method),
semconv.HTTPRouteKey.String("/prompt-response/unified-chat/stream"),
))
defer span.End()

	l := h.logger.With(slog.String("handler", "ProcessUnifiedChatMessageStream"))
	l.DebugContext(ctx, "Processing unified chat message with streaming")

	// Parse profile ID from URL
	profileIDStr := chi.URLParam(r, "profileID")
	profileID, err := uuid.Parse(profileIDStr)
	if err != nil {
		l.ErrorContext(ctx, "Invalid profile ID", slog.String("profileID", profileIDStr), slog.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Invalid profile ID")
		api.ErrorResponse(w, r, http.StatusBadRequest, "Invalid profile ID")
		return
	}

	// Get user ID from auth context
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
			slog.String("message", req.Message))
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

// ProcessUnifiedChatMessageStream handles unified chat requests with streaming
func (h *HandlerImpl) StartChatMessageStreamFree(w http.ResponseWriter, r *http.Request) {
ctx, span := otel.Tracer("HandlerImpl").Start(r.Context(), "ProcessUnifiedChatMessageStream", trace.WithAttributes(
semconv.HTTPRequestMethodKey.String(r.Method),
semconv.HTTPRouteKey.String("/prompt-response/unified-chat/stream/free"),
))
defer span.End()

	l := h.logger.With(slog.String("handler", "ProcessUnifiedChatMessageStream"))
	l.DebugContext(ctx, "Processing unified chat message with streaming")

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
		attribute.String("message", req.Message),
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
			slog.String("cityName", ""),
			slog.String("message", req.Message))
		err := h.llmInteractionService.ProcessUnifiedChatMessageStreamFree(
			ctx, "", req.Message, req.UserLocation, eventCh,
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

func (h *HandlerImpl) ContinueChatSessionStream(w http.ResponseWriter, r *http.Request) {
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

package llmChat

import (
"context"
"encoding/json"
"fmt"
"log/slog"
"strings"

	"github.com/FACorreiaa/go-poi-au-suggestions/internal/types"
	"github.com/google/uuid"
)

func generatePOICacheKey(city string, lat, lon, distance float64, userID uuid.UUID) string {
return fmt.Sprintf("poi:%s:%f:%f:%f:%s", city, lat, lon, distance, userID.String())
}

func generateHotelCacheKey(city string, lat, lon float64, userID uuid.UUID) string {
return fmt.Sprintf("hotel:%s:%.6f:%.6f:%s", city, lat, lon, userID.String())
}

func generateRestaurantCacheKey(city string, lat, lon float64, userID uuid.UUID) string {
return fmt.Sprintf("restaurant:%s:%.6f:%.6f:%s", city, lat, lon, userID.String())
}

func cleanJSONResponse(response string) string {
response = strings.TrimSpace(response)

	// Remove markdown code block markers
	if strings.HasPrefix(response, "```json") {
		response = strings.TrimPrefix(response, "```json")
	} else if strings.HasPrefix(response, "```") {
		response = strings.TrimPrefix(response, "```")
	}

	if strings.HasSuffix(response, "```") {
		response = strings.TrimSuffix(response, "```")
	}

	response = strings.TrimSpace(response)

	// Extract JSON from response that might contain explanatory text
	// Look for the first { and find the matching closing brace
	firstBrace := strings.Index(response, "{")
	if firstBrace == -1 {
		return response // No JSON found, return as is
	}

	// Find the matching closing brace by counting braces
	braceCount := 0
	var lastValidBrace int
	for i := firstBrace; i < len(response); i++ {
		switch response[i] {
		case '{':
			braceCount++
		case '}':
			braceCount--
			if braceCount == 0 {
				lastValidBrace = i
				break
			}
		}
	}

	if braceCount != 0 {
		// Fallback to last brace method if brace counting fails
		lastBrace := strings.LastIndex(response, "}")
		if lastBrace == -1 || lastBrace <= firstBrace {
			return response // No valid JSON structure found
		}
		lastValidBrace = lastBrace
	}

	// Extract the JSON portion
	jsonPortion := response[firstBrace : lastValidBrace+1]

	// Remove any remaining backticks that might be within the JSON content
	// This handles cases where the AI includes markdown formatting within JSON strings
	jsonPortion = strings.ReplaceAll(jsonPortion, "`", "")

	return strings.TrimSpace(jsonPortion)
}

// extractPOIName extracts the full POI name from the message
func extractPOIName(message string) string {
// Remove common words and keep the rest as the POI name
words := strings.Fields(strings.ToLower(message))
filtered := []string{}
stopWords := map[string]bool{
"add": true, "remove": true, "to": true, "from": true, "my": true,
"itinerary": true, "with": true, "replace": true, "the": true, "in": true,
}
for _, w := range words {
if !stopWords[w] {
filtered = append(filtered, w)
}
}
if len(filtered) == 0 {
return "Unknown POI"
}
// Capitalize each word for proper formatting
// cases.Title
// use this https://pkg.go.dev/golang.org/x/text/cases later and handle language as well
return strings.Title(strings.Join(filtered, " "))
}

// helpers

func (l *ServiceImpl) ProcessAndSaveUnifiedResponse(
ctx context.Context,
responses map[string]*strings.Builder,
userID, profileID, cityID uuid.UUID,
llmInteractionID uuid.UUID,
userLocation *types.UserLocation,
) {
l.logger.InfoContext(ctx, "Processing unified response for POI extraction",
slog.String("city_id", cityID.String()),
slog.Int("response_parts", len(responses)))

	// Process general POIs if available
	if poisContent, ok := responses["general_pois"]; ok && poisContent.Len() > 0 {
		l.logger.InfoContext(ctx, "Processing general POIs from unified response",
			slog.Int("content_length", poisContent.Len()))
		l.handleGeneralPoisFromResponse(ctx, poisContent.String(), cityID)
	}

	// Process itinerary POIs if available
	if itineraryContent, ok := responses["itinerary"]; ok && itineraryContent.Len() > 0 {
		l.logger.InfoContext(ctx, "Processing itinerary POIs from unified response",
			slog.Int("content_length", itineraryContent.Len()))
		l.handleItineraryFromResponse(ctx, itineraryContent.String(), userID, profileID, cityID, llmInteractionID, userLocation)
	}

	// Process activities POIs if available (for DomainActivities)
	if activitiesContent, ok := responses["activities"]; ok && activitiesContent.Len() > 0 {
		l.logger.InfoContext(ctx, "Processing activities POIs from unified response",
			slog.Int("content_length", activitiesContent.Len()))
		l.handleGeneralPoisFromResponse(ctx, activitiesContent.String(), cityID)
	}

	// Process hotel POIs if available (for DomainAccommodation)
	if hotelsContent, ok := responses["hotels"]; ok && hotelsContent.Len() > 0 {
		l.logger.InfoContext(ctx, "Processing hotels from unified response",
			slog.Int("content_length", hotelsContent.Len()))
		l.handleHotelsFromResponse(ctx, hotelsContent.String(), cityID, userID, llmInteractionID)
	}

	// Process restaurant POIs if available (for DomainDining)
	if restaurantsContent, ok := responses["restaurants"]; ok && restaurantsContent.Len() > 0 {
		l.logger.InfoContext(ctx, "Processing restaurants from unified response",
			slog.Int("content_length", restaurantsContent.Len()))
		l.handleRestaurantsFromResponse(ctx, restaurantsContent.String(), cityID, userID, llmInteractionID)
	}
}

func (l *ServiceImpl) ProcessAndSaveUnifiedResponseFree(
ctx context.Context,
responses map[string]*strings.Builder,
cityID uuid.UUID,
llmInteractionID uuid.UUID,
userLocation *types.UserLocation,
) {
l.logger.InfoContext(ctx, "Processing unified response for POI extraction",
slog.String("city_id", cityID.String()),
slog.Int("response_parts", len(responses)))

	// Process general POIs if available
	if poisContent, ok := responses["general_pois"]; ok && poisContent.Len() > 0 {
		l.logger.InfoContext(ctx, "Processing general POIs from unified response",
			slog.Int("content_length", poisContent.Len()))
		l.handleGeneralPoisFromResponse(ctx, poisContent.String(), cityID)
	}

	// Process itinerary POIs if available
	if itineraryContent, ok := responses["itinerary"]; ok && itineraryContent.Len() > 0 {
		l.logger.InfoContext(ctx, "Processing itinerary POIs from unified response",
			slog.Int("content_length", itineraryContent.Len()))
		l.handleItineraryFromResponse(ctx, itineraryContent.String(), uuid.Nil, uuid.Nil, cityID, llmInteractionID, userLocation)
	}

	// Process activities POIs if available (for DomainActivities)
	if activitiesContent, ok := responses["activities"]; ok && activitiesContent.Len() > 0 {
		l.logger.InfoContext(ctx, "Processing activities POIs from unified response",
			slog.Int("content_length", activitiesContent.Len()))
		l.handleGeneralPoisFromResponse(ctx, activitiesContent.String(), cityID)
	}

	// Process hotel POIs if available (for DomainAccommodation)
	if hotelsContent, ok := responses["hotels"]; ok && hotelsContent.Len() > 0 {
		l.logger.InfoContext(ctx, "Processing hotels from unified response",
			slog.Int("content_length", hotelsContent.Len()))
		l.handleHotelsFromResponse(ctx, hotelsContent.String(), cityID, uuid.Nil, llmInteractionID)
	}

	// Process restaurant POIs if available (for DomainDining)
	if restaurantsContent, ok := responses["restaurants"]; ok && restaurantsContent.Len() > 0 {
		l.logger.InfoContext(ctx, "Processing restaurants from unified response",
			slog.Int("content_length", restaurantsContent.Len()))
		l.handleRestaurantsFromResponse(ctx, restaurantsContent.String(), cityID, uuid.Nil, llmInteractionID)
	}
}

func (l *ServiceImpl) handleGeneralPoisFromResponse(ctx context.Context, content string, cityID uuid.UUID) {
var poiData struct {
PointsOfInterest []types.POIDetailedInfo `json:"points_of_interest"`
}
if err := json.Unmarshal([]byte(cleanJSONResponse(content)), &poiData); err != nil {
l.logger.ErrorContext(ctx, "Failed to parse general POIs from unified response", slog.Any("error", err))
return
}

	l.HandleGeneralPOIs(ctx, poiData.PointsOfInterest, cityID)
}

func (l *ServiceImpl) handleItineraryFromResponse(
ctx context.Context,
content string,
userID, profileID, cityID uuid.UUID,
llmInteractionID uuid.UUID,
userLocation *types.UserLocation,
) {
var itineraryData struct {
ItineraryName      string                  `json:"itinerary_name"`
OverallDescription string                  `json:"overall_description"`
PointsOfInterest   []types.POIDetailedInfo `json:"points_of_interest"`
}
if err := json.Unmarshal([]byte(cleanJSONResponse(content)), &itineraryData); err != nil {
l.logger.ErrorContext(ctx, "Failed to parse itinerary from unified response", slog.Any("error", err))
return
}

	// Save the itinerary and its POIs
	_, err := l.HandlePersonalisedPOIs(ctx, itineraryData.PointsOfInterest, cityID, userLocation, llmInteractionID, userID, profileID)
	if err != nil {
		l.logger.ErrorContext(ctx, "Failed to save personalised POIs from unified response", slog.Any("error", err))
	}
}

func (l *ServiceImpl) handleHotelsFromResponse(ctx context.Context, content string, cityID, userID, llmInteractionID uuid.UUID) {
var hotelData struct {
Hotels []types.HotelDetailedInfo `json:"hotels"`
}
if err := json.Unmarshal([]byte(cleanJSONResponse(content)), &hotelData); err != nil {
l.logger.ErrorContext(ctx, "Failed to parse hotels from unified response", slog.Any("error", err))
return
}

	// Save hotels to database
	for _, hotel := range hotelData.Hotels {
		hotel.LlmInteractionID = llmInteractionID
		if _, err := l.poiRepo.SaveHotelDetails(ctx, hotel, cityID); err != nil {
			l.logger.WarnContext(ctx, "Failed to save hotel from unified response",
				slog.String("hotel_name", hotel.Name), slog.Any("error", err))
		}
	}
	l.logger.InfoContext(ctx, "Saved hotels from unified response",
		slog.Int("hotel_count", len(hotelData.Hotels)))
}

func (l *ServiceImpl) handleRestaurantsFromResponse(ctx context.Context, content string, cityID, userID, llmInteractionID uuid.UUID) {
var restaurantData struct {
Restaurants []types.RestaurantDetailedInfo `json:"restaurants"`
}
if err := json.Unmarshal([]byte(cleanJSONResponse(content)), &restaurantData); err != nil {
l.logger.ErrorContext(ctx, "Failed to parse restaurants from unified response", slog.Any("error", err))
return
}

	// Save restaurants to database
	for _, restaurant := range restaurantData.Restaurants {
		restaurant.LlmInteractionID = llmInteractionID
		if _, err := l.poiRepo.SaveRestaurantDetails(ctx, restaurant, cityID); err != nil {
			l.logger.WarnContext(ctx, "Failed to save restaurant from unified response",
				slog.String("restaurant_name", restaurant.Name), slog.Any("error", err))
		}
	}
	l.logger.InfoContext(ctx, "Saved restaurants from unified response",
		slog.Int("restaurant_count", len(restaurantData.Restaurants)))
}

package llmChat

import (
"fmt"
"strings"

	"github.com/FACorreiaa/go-poi-au-suggestions/internal/types"
)

func getUserPreferencesPrompt(searchProfile *types.UserPreferenceProfileResponse) string {
// Base preferences
basePrefs := fmt.Sprintf(`
BASIC PREFERENCES:
    - Profile Name: %s
    - Search Radius: %.1f km
    - Preferred Time: %s
    - Budget Level: %d (0=any, 1=cheap, 4=expensive)
    - Prefers Outdoor Seating: %t
    - Prefers Dog Friendly: %t
    - Preferred Dietary Needs: [%s]
    - Preferred Pace: %s
    - Prefers Accessible POIs: %t
    - Preferred Vibes: [%s]
    - Preferred Transport: %s`,
searchProfile.ProfileName, searchProfile.SearchRadiusKm, searchProfile.PreferredTime, searchProfile.BudgetLevel,
searchProfile.PreferOutdoorSeating, searchProfile.PreferDogFriendly, strings.Join(searchProfile.DietaryNeeds, ", "),
searchProfile.PreferredPace, searchProfile.PreferAccessiblePOIs, strings.Join(searchProfile.PreferredVibes, ", "),
searchProfile.PreferredTransport)

	// User location if available
	if searchProfile.UserLatitude != nil && searchProfile.UserLongitude != nil {
		basePrefs += fmt.Sprintf(`
    - User Location: %.4f, %.4f`, *searchProfile.UserLatitude, *searchProfile.UserLongitude)
	}

	// Interests
	if len(searchProfile.Interests) > 0 {
		interests := make([]string, len(searchProfile.Interests))
		for i, interest := range searchProfile.Interests {
			interests[i] = interest.Name
		}
		basePrefs += fmt.Sprintf(`
    - Interests: [%s]`, strings.Join(interests, ", "))
	}

	// Tags to avoid
	if len(searchProfile.Tags) > 0 {
		tags := make([]string, len(searchProfile.Tags))
		for i, tag := range searchProfile.Tags {
			tags[i] = tag.Name
		}
		basePrefs += fmt.Sprintf(`
    - Tags to Avoid: [%s]`, strings.Join(tags, ", "))
	}

	// Accommodation preferences
	if searchProfile.AccommodationPreferences != nil {
		accom := searchProfile.AccommodationPreferences
		basePrefs += `

ACCOMMODATION PREFERENCES:`

		if len(accom.AccommodationType) > 0 {
			basePrefs += fmt.Sprintf(`
    - Accommodation Types: [%s]`, strings.Join(accom.AccommodationType, ", "))
		}

		if accom.StarRating != nil {
			minStar := "any"
			maxStar := "any"
			if accom.StarRating.Min != nil {
				minStar = fmt.Sprintf("%.0f", *accom.StarRating.Min)
			}
			if accom.StarRating.Max != nil {
				maxStar = fmt.Sprintf("%.0f", *accom.StarRating.Max)
			}
			basePrefs += fmt.Sprintf(`
    - Star Rating: %s - %s stars`, minStar, maxStar)
		}

		if accom.PriceRangePerNight != nil {
			minPrice := "any"
			maxPrice := "any"
			if accom.PriceRangePerNight.Min != nil {
				minPrice = fmt.Sprintf("%.0f", *accom.PriceRangePerNight.Min)
			}
			if accom.PriceRangePerNight.Max != nil {
				maxPrice = fmt.Sprintf("%.0f", *accom.PriceRangePerNight.Max)
			}
			basePrefs += fmt.Sprintf(`
    - Price Range Per Night: %s - %s`, minPrice, maxPrice)
		}

		if len(accom.Amenities) > 0 {
			basePrefs += fmt.Sprintf(`
    - Required Amenities: [%s]`, strings.Join(accom.Amenities, ", "))
		}

		if len(accom.RoomType) > 0 {
			basePrefs += fmt.Sprintf(`
    - Room Types: [%s]`, strings.Join(accom.RoomType, ", "))
		}

		if accom.ChainPreference != "" {
			basePrefs += fmt.Sprintf(`
    - Chain Preference: %s`, accom.ChainPreference)
		}
	}

	// Dining preferences
	if searchProfile.DiningPreferences != nil {
		dining := searchProfile.DiningPreferences
		basePrefs += `

DINING PREFERENCES:`

		if len(dining.CuisineTypes) > 0 {
			basePrefs += fmt.Sprintf(`
    - Cuisine Types: [%s]`, strings.Join(dining.CuisineTypes, ", "))
		}

		if len(dining.MealTypes) > 0 {
			basePrefs += fmt.Sprintf(`
    - Meal Types: [%s]`, strings.Join(dining.MealTypes, ", "))
		}

		if len(dining.ServiceStyle) > 0 {
			basePrefs += fmt.Sprintf(`
    - Service Style: [%s]`, strings.Join(dining.ServiceStyle, ", "))
		}

		if dining.PriceRangePerPerson != nil {
			minPrice := "any"
			maxPrice := "any"
			if dining.PriceRangePerPerson.Min != nil {
				minPrice = fmt.Sprintf("%.0f", *dining.PriceRangePerPerson.Min)
			}
			if dining.PriceRangePerPerson.Max != nil {
				maxPrice = fmt.Sprintf("%.0f", *dining.PriceRangePerPerson.Max)
			}
			basePrefs += fmt.Sprintf(`
    - Price Range Per Person: %s - %s`, minPrice, maxPrice)
		}

		if len(dining.AllergenFree) > 0 {
			basePrefs += fmt.Sprintf(`
    - Allergen Free: [%s]`, strings.Join(dining.AllergenFree, ", "))
		}

		if dining.MichelinRated {
			basePrefs += `
    - Michelin Rated: Preferred`
		}

		if dining.LocalRecommendations {
			basePrefs += `
    - Local Recommendations: Preferred`
		}

		if dining.ChainVsLocal != "" {
			basePrefs += fmt.Sprintf(`
    - Chain vs Local: %s`, dining.ChainVsLocal)
		}

		if dining.OrganicPreference {
			basePrefs += `
    - Organic Preference: Yes`
		}

		if dining.OutdoorSeatingPref {
			basePrefs += `
    - Outdoor Seating: Preferred`
		}
	}

	// Activity preferences
	if searchProfile.ActivityPreferences != nil {
		activity := searchProfile.ActivityPreferences
		basePrefs += `

ACTIVITY PREFERENCES:`

		if len(activity.ActivityCategories) > 0 {
			basePrefs += fmt.Sprintf(`
    - Activity Categories: [%s]`, strings.Join(activity.ActivityCategories, ", "))
		}

		if activity.PhysicalActivityLevel != "" {
			basePrefs += fmt.Sprintf(`
    - Physical Activity Level: %s`, activity.PhysicalActivityLevel)
		}

		if activity.IndoorOutdoorPref != "" {
			basePrefs += fmt.Sprintf(`
    - Indoor/Outdoor Preference: %s`, activity.IndoorOutdoorPref)
		}

		if activity.CulturalImmersionLevel != "" {
			basePrefs += fmt.Sprintf(`
    - Cultural Immersion Level: %s`, activity.CulturalImmersionLevel)
		}

		if activity.MustSeeVsHiddenGems != "" {
			basePrefs += fmt.Sprintf(`
    - Must-See vs Hidden Gems: %s`, activity.MustSeeVsHiddenGems)
		}

		if activity.EducationalPreference {
			basePrefs += `
    - Educational Preference: Yes`
		}

		if activity.PhotoOpportunities {
			basePrefs += `
    - Photography Opportunities: Important`
		}

		if len(activity.SeasonSpecific) > 0 {
			basePrefs += fmt.Sprintf(`
    - Season Specific: [%s]`, strings.Join(activity.SeasonSpecific, ", "))
		}

		if activity.AvoidCrowds {
			basePrefs += `
    - Avoid Crowds: Yes`
		}

		if len(activity.LocalEventsInterest) > 0 {
			basePrefs += fmt.Sprintf(`
    - Local Events Interest: [%s]`, strings.Join(activity.LocalEventsInterest, ", "))
		}
	}

	// Itinerary preferences
	if searchProfile.ItineraryPreferences != nil {
		itinerary := searchProfile.ItineraryPreferences
		basePrefs += `

ITINERARY PREFERENCES:`

		if itinerary.PlanningStyle != "" {
			basePrefs += fmt.Sprintf(`
    - Planning Style: %s`, itinerary.PlanningStyle)
		}

		if itinerary.TimeFlexibility != "" {
			basePrefs += fmt.Sprintf(`
    - Time Flexibility: %s`, itinerary.TimeFlexibility)
		}

		if itinerary.MorningVsEvening != "" {
			basePrefs += fmt.Sprintf(`
    - Morning vs Evening: %s`, itinerary.MorningVsEvening)
		}

		if itinerary.WeekendVsWeekday != "" {
			basePrefs += fmt.Sprintf(`
    - Weekend vs Weekday: %s`, itinerary.WeekendVsWeekday)
		}

		if len(itinerary.PreferredSeasons) > 0 {
			basePrefs += fmt.Sprintf(`
    - Preferred Seasons: [%s]`, strings.Join(itinerary.PreferredSeasons, ", "))
		}

		if itinerary.AvoidPeakSeason {
			basePrefs += `
    - Avoid Peak Season: Yes`
		}

		if itinerary.AdventureVsRelaxation != "" {
			basePrefs += fmt.Sprintf(`
    - Adventure vs Relaxation: %s`, itinerary.AdventureVsRelaxation)
		}

		if itinerary.SpontaneousVsPlanned != "" {
			basePrefs += fmt.Sprintf(`
    - Spontaneous vs Planned: %s`, itinerary.SpontaneousVsPlanned)
		}
	}

	return basePrefs
}

func getPOIDetailsPrompt(city string, lat, lon float64) string {
return fmt.Sprintf(`
Generate details for the following POI on the city of %s with the coordinates %0.2f , %0.2f.
The result should be in the following JSON format:
{
"name": "Name of the Point of Interest",
"description": "Detailed description of the POI and why it's relevant to the user's interest.",
"address": "address of the point of interest",
"website": "website of the POI if available",
"phone_number": "phone number of the POI if available",
"opening_hours": "JSONB, -- Store opening hours structured (e.g., OSM opening_hours format or custom JSON)"
"price_range": "price level if available",
"category": "Primary category (e.g., Museum, Historical Site, Park, Restaurant, Bar)",
"tags": ["tag1", "tag2", ...], -- Tags related to the POI
"images": ["image_url_1", "image_url_2", ...], // images from wikipedia or pininterest
"rating": <float> -- Average rating if available
"stars": type of stars if available (e.g., "3 stars", "5 stars")

		}
	`, city, lat, lon)
}

func getHotelsByPreferencesPrompt(city string, lat, lon float64, userPreferences types.HotelUserPreferences) string {
return fmt.Sprintf(`
        Generate a list of maximum 5 hotels in the city of %s, near the coordinates %0.2f , %0.2f.
        The hotels should be relevant to the user's interest.
        The result should be tailored to the user's preferences:
        - Preferred Category: %s
        - Preferred Tags: %s
        - Max Price range: %s
        - Preferred Rating: %0.2f
        - Number of Guests: %d
        - Number of Nights: %d
        - Number of Rooms: %d
        - Preferred Check-In Date: %s
        - Preferred Check-Out Date: %s
        - Distance: %0.2f km (if provided, otherwise use default radius of 5km)
        The result should be in the following JSON format:
        {
            "hotels": [
                {
                    "name": "Name of the Hotel",
                    "latitude": <float>,
                    "longitude": <float>,
                    "category": "Primary category (e.g., Hotel, Hostel, Guesthouse)",
                    "description": "A brief description of this hotel and why it's relevant to the user's interest."
                }
            ]
        }
    `, city, lat, lon, userPreferences.PreferredCategories, userPreferences.PreferredTags,
userPreferences.MaxPriceRange, userPreferences.MinRating,
userPreferences.NumberOfGuests, userPreferences.NumberOfNights, userPreferences.NumberOfRooms,
userPreferences.PreferredCheckIn.Format("2006-01-02"), userPreferences.PreferredCheckOut.Format("2006-01-02"),
userPreferences.SearchRadiusKm)
}

func getHotelsNeabyPrompt(city string, userLocation types.UserLocation) string {
return fmt.Sprintf(`
        Generate a list of maximum 5 hotels nearby the coordinates %0.2f , %0.2f in the city of %s.
        the hotels can be around %0.2f km radius from the user's location or if nothing provided, use the default radius of 5km.
        The hotels should be relevant to the user's interest.
        The result should be in the following JSON format:
        {
            "hotels": [
                {
                    "name": "Name of the Hotel",
                    "latitude": <float>,
                    "longitude": <float>,
                    "category": "Primary category (e.g., Hotel, Hostel, Guesthouse)",
                    "description": "A brief description of this hotel and why it's relevant to the user's interest."
                }
            ]
        }
    `, userLocation.UserLat, userLocation.UserLon, city, userLocation.SearchRadiusKm)
}

func getRestaurantsByPreferencesPrompt(city string, lat, lon float64, userPreferences types.RestaurantUserPreferences) string {
return fmt.Sprintf(`
        Generate a list of up to 10 restaurants in the city of %s, near coordinates %.2f, %.2f.
        Tailor the results to the user's preferences:
        - Preferred Cuisine: %s
        - Preferred Price Range: %s
        - Dietary Restrictions: %s
        - Ambiance: %s
        - Special Features: %s
        The result must be in JSON format:
        {
            "restaurants": [
                {
                    "name": "Restaurant Name",
                    "latitude": <float>,
                    "longitude": <float>,
                    "category": "Restaurant|Bar|Cafe",
                    "description": "Brief description of the restaurant and why it matches preferences."
                }
            ]
        }
    `, city, lat, lon, userPreferences.PreferredCuisine, userPreferences.PreferredPriceRange,
userPreferences.DietaryRestrictions, userPreferences.Ambiance, userPreferences.SpecialFeatures)
}

func getRestaurantsNearbyPrompt(city string, userLocation types.UserLocation) string {
if userLocation.SearchRadiusKm == 0 {
userLocation.SearchRadiusKm = 5.0 // Default radius
}
return fmt.Sprintf(`
        Generate a list of up to 10 restaurants in the city of %s, within %.2f km of coordinates %.2f, %.2f.
        Include a variety of restaurant categories to provide diverse options.
        The result must be in JSON format:
        {
            "restaurants": [
                {
                    "name": "Restaurant Name",
                    "latitude": <float>,
                    "longitude": <float>,
                    "category": "Restaurant|Bar|Cafe",
                    "description": "Brief description of the restaurant and its proximity to the user's location."
                }
            ]
        }
    `, city, userLocation.SearchRadiusKm, userLocation.UserLat, userLocation.UserLon)
}

func generatedContinuedConversationPrompt(poi, city string) string {
return fmt.Sprintf(
`Provide detailed information about "%s" in %s.
If user writes "Restaurant" add "cuisine_type" to final response and hide "description_poi"
If user writes "Hotel" add "star_rating" to final response and hide "description_poi"
Analise this POI (The user can insert a POI name, a Restaurant name or an Hotel/Hostel name) and return the following JSON structure:
{
"name": "string (the POI name)",
"latitude": number (approximate latitude as float),
"longitude": number (approximate longitude as float),
"category": "string (e.g., Museum, Park, Historical Site)",
"description_poi": "string (50-100 words description)"
"cuisine_type": "string (for Restaurant)",
"star_rating": "number (for Hotel/Hostel)"
}

    If the POI is not found, return: {"name": "", "latitude": 0, "longitude": 0, "category": "", "description_poi": ""}`,
		poi, city)
}

// getCityDescriptionPrompt generates a prompt for city data
func getCityDescriptionPrompt(cityName string) string {
return fmt.Sprintf(`
        Provide detailed information about the city %s in JSON format with the following structure:
        {
            "city_name": "%s",
            "country": "Country name",
            "state_province": "State or province, if applicable",
            "description": "A detailed description of the city",
            "center_latitude": float64,
            "center_longitude": float64
        }
    `, cityName, cityName)
}

// GetUnifiedChatPrompt generates context-based prompts for the unified chat system
func GetUnifiedChatPrompt(context, cityName string, lat, lon float64, searchProfile *types.UserPreferenceProfileResponse) string {
basePreferences := ""
if searchProfile != nil {
basePreferences = getUserPreferencesPrompt(searchProfile)
}

	switch context {
	case "traveling", "itinerary":
		return fmt.Sprintf(`
You are a travel planning assistant. Create a personalized itinerary for %s based on the user's location (%.4f, %.4f) and preferences.

USER PREFERENCES:
%s

Generate a comprehensive travel response in JSON format with the following structure:
{
"data": {
"general_city_data": {
"city": "%s",
"country": "Country name",
"state_province": "State/Province if applicable",
"description": "Detailed city description (100-150 words)",
"center_latitude": %.4f,
"center_longitude": %.4f,
"population": "",
"area": "",
"timezone": "",
"language": "",
"weather": "",
"attractions": "",
"history": ""
},
"points_of_interest": [
{
"name": "POI Name",
"latitude": <float>,
"longitude": <float>,
"category": "Category (e.g., Museum, Historical Site)",
"description_poi": "",
"address": "",
"website": "",
"opening_hours": "JSONB, -- Store opening hours structured (e.g., OSM opening_hours format or custom JSON)"

            }
        ],
        "itinerary_response": {
            "itinerary_name": "Creative itinerary name based on user preferences",
            "overall_description": "Detailed description of the itinerary (100-150 words)",
            "points_of_interest": [
                {
                    "name": "POI Name",
                    "latitude": <float>,
                    "longitude": <float>,
                    "category": "",
                    "description_poi": "",
                    "address": "",
                    "website": "",
                    "opening_hours": "JSONB, -- Store opening hours structured (e.g., OSM opening_hours format or custom JSON)"

                }
            ]
        }
    }
}

Focus on creating an itinerary that matches the user's preferences, dietary needs, preferred pace, and transportation method.`,
cityName, lat, lon, basePreferences, cityName, lat, lon)

	case "accommodation":
		return fmt.Sprintf(`
You are a hotel recommendation assistant. Find suitable accommodation in %s near coordinates %.4f, %.4f.

USER PREFERENCES:
%s

Generate a hotel response in JSON format:
{
"hotels": [
{
"city": "%s",
"name": "Hotel Name",
"latitude": <float>,
"longitude": <float>,
"category": "Hotel|Hostel|Guesthouse|Apartment",
"description": "Description matching user preferences and budget level",
"address": "",
"phone_number": null,
"website": null,
"opening_hours": "JSONB, -- Store opening hours structured (e.g., OSM opening_hours format or custom JSON)"
"price_range": null,
"rating": 0,
"tags": null,
"images": null
}
]
}

Consider the user's budget level, preferred amenities, and accessibility needs when selecting accommodation.`,
cityName, lat, lon, basePreferences, cityName)

	case "dining":
		return fmt.Sprintf(`
You are a restaurant recommendation assistant. Find dining options in %s near coordinates %.4f, %.4f.

USER PREFERENCES:
%s

Generate a restaurant response in JSON format:
{
"restaurants": [
{
"city": "%s",
"name": "Restaurant Name",
"latitude": <float>,
"longitude": <float>,
"category": "Fine Dining|Casual Dining|Fast Food|Cafe|Bar",
"description": "Description matching user dietary needs and preferences",
"address": "Complete address",
"website": "Official website URL (if available)",
"phone_number": "Phone number (if available)",
"opening_hours": "JSONB, -- Store opening hours structured (e.g., OSM opening_hours format or custom JSON)",

            "price_level": "$|$$|$$$|$$$$",
            "cuisine_type": "Cuisine type",
            "tags": ["tag1", "tag2"],
            "images": [],
            "rating": 0
        }
    ]
}

Pay special attention to dietary needs, budget level, cuisine preferences, and accessibility options.`,
cityName, lat, lon, basePreferences, cityName)

	case "activities":
		return fmt.Sprintf(`
You are an activity recommendation assistant. Find activities and attractions in %s near coordinates %.4f, %.4f.

USER PREFERENCES:
%s

Generate an activities response in JSON format:
{
"activities": [
{
"city": "%s",
"name": "Activity/Attraction Name",
"latitude": <float>,
"longitude": <float>,
"category": "Museum|Outdoor Activity|Entertainment|Cultural|Sports",
"description": "Description matching user activity preferences",
"address": "Complete address",
"website": "Official website URL (if available)",
"opening_hours": "JSONB, -- Store opening hours structured (e.g., OSM opening_hours format or custom JSON)",

            "price_range": "Free|$|$$|$$$",
            "rating": 0,
            "tags": ["tag1", "tag2"],
            "images": []
        }
    ]
}

Consider the user's physical activity level, cultural interests, and accessibility needs when selecting activities.`,
cityName, lat, lon, basePreferences, cityName)

	default:
		// Default to itinerary if context is not recognized
		return GetUnifiedChatPrompt("traveling", cityName, lat, lon, searchProfile)
	}
}

/*
Testing Fan in Fan out prompt
*/

func getCityDataPrompt(cityName string) string {
return fmt.Sprintf(`
You are a travel assistant. Provide general information about %s.
Respond with JSON:
{
    "city": "%s",
    "country": "Country name",
    "state_province": "State/Province if applicable",
    "description": "Detailed city description (100-150 words)",
    "center_latitude": <float>,
    "center_longitude": <float>,
    "population": "",
    "area": "",
    "timezone": "",
    "language": "",
    "weather": "",
    "attractions": "",
    "history": ""
}`, cityName, cityName)
}

func getGeneralPOIPrompt(cityName string) string {
return fmt.Sprintf(`
You are a travel assistant. List general points of interest in %s.
Respond with JSON:
{
"points_of_interest": [
{
"name": "POI Name",
"latitude": <float>,
"longitude": <float>,
"category": "Category (e.g., Museum, Historical Site)",
"description_poi": "",
"address": "",
"website": "",
"opening_hours": "JSONB, -- Store opening hours structured (e.g., OSM opening_hours format or custom JSON)"

        }
    ]
}`, cityName)
}

func getPersonalizedItineraryPrompt(cityName, basePreferences string) string {
return fmt.Sprintf(`
You are a travel planning assistant. Create a personalized itinerary for %s based on user preferences.
USER PREFERENCES:
%s
Respond with JSON:
{
    "itinerary_name": "Creative itinerary name",
    "overall_description": "Detailed description (100-150 words)",
    "points_of_interest": [
        {
            "name": "POI Name",
            "latitude": <float>,
            "longitude": <float>,
            "category": "",
            "description_poi": "",
            "address": "",
            "website": "",
                		"opening_hours": "JSONB, -- Store opening hours structured (e.g., OSM opening_hours format or custom JSON)"
,
            "distance": <float>
        }
    ]
}`, cityName, basePreferences)
}

func getGeneralizedItineraryPrompt(cityName string) string {
return fmt.Sprintf(`
You are a travel planning assistant. Create a personalized itinerary with a max of 5 results for %s with multi things to do and different activities.
Respond with JSON:
{
    "itinerary_name": "Creative itinerary name",
    "overall_description": "Detailed description (100-150 words)",
    "points_of_interest": [
        {
            "name": "POI Name",
            "latitude": <float>,
            "longitude": <float>,
            "category": "",
            "description_poi": "",
            "address": "",
            "website": "",
                		"opening_hours": "JSONB, -- Store opening hours structured (e.g., OSM opening_hours format or custom JSON)"
,
            "distance": <float>
        }
    ]
}`, cityName)
}

func getAccommodationPrompt(cityName string, lat, lon float64, basePreferences string) string {
return fmt.Sprintf(`
You are a hotel recommendation assistant. Find suitable accommodation in %s near coordinates %.4f, %.4f.
USER PREFERENCES:
%s
Respond with JSON:
{
    "hotels": [
        {
            "city": "%s",
            "name": "Hotel Name",
            "latitude": <float>,
            "longitude": <float>,
            "category": "Hotel|Hostel|Guesthouse|Apartment",
            "description": "Description matching preferences",
            "address": "",
            "phone_number": null,
            "website": null,
                		"opening_hours": "JSONB, -- Store opening hours structured (e.g., OSM opening_hours format or custom JSON)",
            "price_range": null,
            "rating": 0,
            "tags": null,
            "images": null,
            "distance": <float>
        }
    ]
}`, cityName, lat, lon, basePreferences, cityName)
}

func getGeneralAccommodationPrompt(cityName string) string {
return fmt.Sprintf(`
You are a hotel recommendation assistant. Find a max of 5 suitable accommodation in %s.
Respond with JSON:
{
    "hotels": [
        {
            "city": "%s",
            "name": "Hotel Name",
            "latitude": <float>,
            "longitude": <float>,
            "category": "Hotel|Hostel|Guesthouse|Apartment",
            "description": "Description matching preferences",
            "address": "",
            "phone_number": null,
            "website": null,
                		"opening_hours": "JSONB, -- Store opening hours structured (e.g., OSM opening_hours format or custom JSON)",
            "price_range": null,
            "rating": 0,
            "tags": null,
            "images": null,
            "distance": <float>
        }
    ]
}`, cityName, cityName)
}

func getDiningPrompt(cityName string, lat, lon float64, basePreferences string) string {
return fmt.Sprintf(`
You are a restaurant recommendation assistant. Find dining options in %s near coordinates %.4f, %.4f.
USER PREFERENCES:
%s
Respond with JSON:
{
    "restaurants": [
        {
            "city": "%s",
            "name": "Restaurant Name",
            "latitude": <float>,
            "longitude": <float>,
            "category": "Fine Dining|Casual Dining|Fast Food|Cafe|Bar",
            "description": "Description matching preferences",
            "address": "",
            "website": "",
            "phone_number": "",
                		"opening_hours": "JSONB, -- Store opening hours structured (e.g., OSM opening_hours format or custom JSON)"
,
            "price_level": "$|$$|$$$|$$$$",
            "cuisine_type": "",
            "tags": [],
            "images": [],
            "rating": 0,
            "distance": <float>
        }
    ]
}`, cityName, lat, lon, basePreferences, cityName)
}

func getGeneralDiningPrompt(cityName string) string {
return fmt.Sprintf(`
You are a restaurant recommendation assistant. Find a max of 5 dining options in %s.
Respond with JSON:
{
    "restaurants": [
        {
            "city": "%s",
            "name": "Restaurant Name",
            "latitude": <float>,
            "longitude": <float>,
            "category": "Fine Dining|Casual Dining|Fast Food|Cafe|Bar",
            "description": "Description matching preferences",
            "address": "",
            "website": "",
            "phone_number": "",
                		"opening_hours": "JSONB, -- Store opening hours structured (e.g., OSM opening_hours format or custom JSON)"
,
            "price_level": "$|$|$$|$$",
            "cuisine_type": "",
            "tags": [],
            "images": [],
            "rating": 0,
            "distance": <float>
        }
    ]
}`, cityName, cityName)
}

func getActivitiesPrompt(cityName string, lat, lon float64, basePreferences string) string {
return fmt.Sprintf(`
You are an activity recommendation assistant. Find activities in %s near coordinates %.4f, %.4f.
USER PREFERENCES:
%s
Respond with JSON:
{
    "activities": [
        {
            "city": "%s",
            "name": "Activity Name",
            "latitude": <float>,
            "longitude": <float>,
            "category": "Museum|Outdoor Activity|Entertainment|Cultural|Sports",
            "description": "Description matching preferences",
            "address": "",
            "website": "",
                		"opening_hours": "JSONB, -- Store opening hours structured (e.g., OSM opening_hours format or custom JSON)"
,
            "price_range": "Free|$|$$|$$$",
            "rating": 0,
            "tags": [],
            "images": [],
            "distance": <float>
        }
    ]
}`, cityName, lat, lon, basePreferences, cityName)
}

func getGeneralActivitiesPrompt(cityName string) string {
return fmt.Sprintf(`
You are an activity recommendation assistant. Find a max of 5 activities in %s.
Respond with JSON:
{
    "activities": [
        {
            "city": "%s",
            "name": "Activity Name",
            "latitude": <float>,
            "longitude": <float>,
            "category": "Museum|Outdoor Activity|Entertainment|Cultural|Sports",
            "description": "Description matching preferences",
            "address": "",
            "website": "",
                		"opening_hours": "JSONB, -- Store opening hours structured (e.g., OSM opening_hours format or custom JSON)"
,
            "price_range": "Free|$|$$|$$$",
            "rating": 0,
            "tags": [],
            "images": [],
            "distance": <float>
        }
    ]
}`, cityName, cityName)
}


package llmChat

import (
"fmt"
"strings"

	"github.com/FACorreiaa/go-poi-au-suggestions/internal/types"
)

func getUserPreferencesPrompt(searchProfile *types.UserPreferenceProfileResponse) string {
// Base preferences
basePrefs := fmt.Sprintf(`
BASIC PREFERENCES:
    - Profile Name: %s
    - Search Radius: %.1f km
    - Preferred Time: %s
    - Budget Level: %d (0=any, 1=cheap, 4=expensive)
    - Prefers Outdoor Seating: %t
    - Prefers Dog Friendly: %t
    - Preferred Dietary Needs: [%s]
    - Preferred Pace: %s
    - Prefers Accessible POIs: %t
    - Preferred Vibes: [%s]
    - Preferred Transport: %s`,
searchProfile.ProfileName, searchProfile.SearchRadiusKm, searchProfile.PreferredTime, searchProfile.BudgetLevel,
searchProfile.PreferOutdoorSeating, searchProfile.PreferDogFriendly, strings.Join(searchProfile.DietaryNeeds, ", "),
searchProfile.PreferredPace, searchProfile.PreferAccessiblePOIs, strings.Join(searchProfile.PreferredVibes, ", "),
searchProfile.PreferredTransport)

	// User location if available
	if searchProfile.UserLatitude != nil && searchProfile.UserLongitude != nil {
		basePrefs += fmt.Sprintf(`
    - User Location: %.4f, %.4f`, *searchProfile.UserLatitude, *searchProfile.UserLongitude)
	}

	// Interests
	if len(searchProfile.Interests) > 0 {
		interests := make([]string, len(searchProfile.Interests))
		for i, interest := range searchProfile.Interests {
			interests[i] = interest.Name
		}
		basePrefs += fmt.Sprintf(`
    - Interests: [%s]`, strings.Join(interests, ", "))
	}

	// Tags to avoid
	if len(searchProfile.Tags) > 0 {
		tags := make([]string, len(searchProfile.Tags))
		for i, tag := range searchProfile.Tags {
			tags[i] = tag.Name
		}
		basePrefs += fmt.Sprintf(`
    - Tags to Avoid: [%s]`, strings.Join(tags, ", "))
	}

	// Accommodation preferences
	if searchProfile.AccommodationPreferences != nil {
		accom := searchProfile.AccommodationPreferences
		basePrefs += `

ACCOMMODATION PREFERENCES:`

		if len(accom.AccommodationType) > 0 {
			basePrefs += fmt.Sprintf(`
    - Accommodation Types: [%s]`, strings.Join(accom.AccommodationType, ", "))
		}

		if accom.StarRating != nil {
			minStar := "any"
			maxStar := "any"
			if accom.StarRating.Min != nil {
				minStar = fmt.Sprintf("%.0f", *accom.StarRating.Min)
			}
			if accom.StarRating.Max != nil {
				maxStar = fmt.Sprintf("%.0f", *accom.StarRating.Max)
			}
			basePrefs += fmt.Sprintf(`
    - Star Rating: %s - %s stars`, minStar, maxStar)
		}

		if accom.PriceRangePerNight != nil {
			minPrice := "any"
			maxPrice := "any"
			if accom.PriceRangePerNight.Min != nil {
				minPrice = fmt.Sprintf("%.0f", *accom.PriceRangePerNight.Min)
			}
			if accom.PriceRangePerNight.Max != nil {
				maxPrice = fmt.Sprintf("%.0f", *accom.PriceRangePerNight.Max)
			}
			basePrefs += fmt.Sprintf(`
    - Price Range Per Night: %s - %s`, minPrice, maxPrice)
		}

		if len(accom.Amenities) > 0 {
			basePrefs += fmt.Sprintf(`
    - Required Amenities: [%s]`, strings.Join(accom.Amenities, ", "))
		}

		if len(accom.RoomType) > 0 {
			basePrefs += fmt.Sprintf(`
    - Room Types: [%s]`, strings.Join(accom.RoomType, ", "))
		}

		if accom.ChainPreference != "" {
			basePrefs += fmt.Sprintf(`
    - Chain Preference: %s`, accom.ChainPreference)
		}
	}

	// Dining preferences
	if searchProfile.DiningPreferences != nil {
		dining := searchProfile.DiningPreferences
		basePrefs += `

DINING PREFERENCES:`

		if len(dining.CuisineTypes) > 0 {
			basePrefs += fmt.Sprintf(`
    - Cuisine Types: [%s]`, strings.Join(dining.CuisineTypes, ", "))
		}

		if len(dining.MealTypes) > 0 {
			basePrefs += fmt.Sprintf(`
    - Meal Types: [%s]`, strings.Join(dining.MealTypes, ", "))
		}

		if len(dining.ServiceStyle) > 0 {
			basePrefs += fmt.Sprintf(`
    - Service Style: [%s]`, strings.Join(dining.ServiceStyle, ", "))
		}

		if dining.PriceRangePerPerson != nil {
			minPrice := "any"
			maxPrice := "any"
			if dining.PriceRangePerPerson.Min != nil {
				minPrice = fmt.Sprintf("%.0f", *dining.PriceRangePerPerson.Min)
			}
			if dining.PriceRangePerPerson.Max != nil {
				maxPrice = fmt.Sprintf("%.0f", *dining.PriceRangePerPerson.Max)
			}
			basePrefs += fmt.Sprintf(`
    - Price Range Per Person: %s - %s`, minPrice, maxPrice)
		}

		if len(dining.AllergenFree) > 0 {
			basePrefs += fmt.Sprintf(`
    - Allergen Free: [%s]`, strings.Join(dining.AllergenFree, ", "))
		}

		if dining.MichelinRated {
			basePrefs += `
    - Michelin Rated: Preferred`
		}

		if dining.LocalRecommendations {
			basePrefs += `
    - Local Recommendations: Preferred`
		}

		if dining.ChainVsLocal != "" {
			basePrefs += fmt.Sprintf(`
    - Chain vs Local: %s`, dining.ChainVsLocal)
		}

		if dining.OrganicPreference {
			basePrefs += `
    - Organic Preference: Yes`
		}

		if dining.OutdoorSeatingPref {
			basePrefs += `
    - Outdoor Seating: Preferred`
		}
	}

	// Activity preferences
	if searchProfile.ActivityPreferences != nil {
		activity := searchProfile.ActivityPreferences
		basePrefs += `

ACTIVITY PREFERENCES:`

		if len(activity.ActivityCategories) > 0 {
			basePrefs += fmt.Sprintf(`
    - Activity Categories: [%s]`, strings.Join(activity.ActivityCategories, ", "))
		}

		if activity.PhysicalActivityLevel != "" {
			basePrefs += fmt.Sprintf(`
    - Physical Activity Level: %s`, activity.PhysicalActivityLevel)
		}

		if activity.IndoorOutdoorPref != "" {
			basePrefs += fmt.Sprintf(`
    - Indoor/Outdoor Preference: %s`, activity.IndoorOutdoorPref)
		}

		if activity.CulturalImmersionLevel != "" {
			basePrefs += fmt.Sprintf(`
    - Cultural Immersion Level: %s`, activity.CulturalImmersionLevel)
		}

		if activity.MustSeeVsHiddenGems != "" {
			basePrefs += fmt.Sprintf(`
    - Must-See vs Hidden Gems: %s`, activity.MustSeeVsHiddenGems)
		}

		if activity.EducationalPreference {
			basePrefs += `
    - Educational Preference: Yes`
		}

		if activity.PhotoOpportunities {
			basePrefs += `
    - Photography Opportunities: Important`
		}

		if len(activity.SeasonSpecific) > 0 {
			basePrefs += fmt.Sprintf(`
    - Season Specific: [%s]`, strings.Join(activity.SeasonSpecific, ", "))
		}

		if activity.AvoidCrowds {
			basePrefs += `
    - Avoid Crowds: Yes`
		}

		if len(activity.LocalEventsInterest) > 0 {
			basePrefs += fmt.Sprintf(`
    - Local Events Interest: [%s]`, strings.Join(activity.LocalEventsInterest, ", "))
		}
	}

	// Itinerary preferences
	if searchProfile.ItineraryPreferences != nil {
		itinerary := searchProfile.ItineraryPreferences
		basePrefs += `

ITINERARY PREFERENCES:`

		if itinerary.PlanningStyle != "" {
			basePrefs += fmt.Sprintf(`
    - Planning Style: %s`, itinerary.PlanningStyle)
		}

		if itinerary.TimeFlexibility != "" {
			basePrefs += fmt.Sprintf(`
    - Time Flexibility: %s`, itinerary.TimeFlexibility)
		}

		if itinerary.MorningVsEvening != "" {
			basePrefs += fmt.Sprintf(`
    - Morning vs Evening: %s`, itinerary.MorningVsEvening)
		}

		if itinerary.WeekendVsWeekday != "" {
			basePrefs += fmt.Sprintf(`
    - Weekend vs Weekday: %s`, itinerary.WeekendVsWeekday)
		}

		if len(itinerary.PreferredSeasons) > 0 {
			basePrefs += fmt.Sprintf(`
    - Preferred Seasons: [%s]`, strings.Join(itinerary.PreferredSeasons, ", "))
		}

		if itinerary.AvoidPeakSeason {
			basePrefs += `
    - Avoid Peak Season: Yes`
		}

		if itinerary.AdventureVsRelaxation != "" {
			basePrefs += fmt.Sprintf(`
    - Adventure vs Relaxation: %s`, itinerary.AdventureVsRelaxation)
		}

		if itinerary.SpontaneousVsPlanned != "" {
			basePrefs += fmt.Sprintf(`
    - Spontaneous vs Planned: %s`, itinerary.SpontaneousVsPlanned)
		}
	}

	return basePrefs
}

func getPOIDetailsPrompt(city string, lat, lon float64) string {
return fmt.Sprintf(`
Generate details for the following POI on the city of %s with the coordinates %0.2f , %0.2f.
The result should be in the following JSON format:
{
"name": "Name of the Point of Interest",
"description": "Detailed description of the POI and why it's relevant to the user's interest.",
"address": "address of the point of interest",
"website": "website of the POI if available",
"phone_number": "phone number of the POI if available",
"opening_hours": "JSONB, -- Store opening hours structured (e.g., OSM opening_hours format or custom JSON)"
"price_range": "price level if available",
"category": "Primary category (e.g., Museum, Historical Site, Park, Restaurant, Bar)",
"tags": ["tag1", "tag2", ...], -- Tags related to the POI
"images": ["image_url_1", "image_url_2", ...], // images from wikipedia or pininterest
"rating": <float> -- Average rating if available
"stars": type of stars if available (e.g., "3 stars", "5 stars")

		}
	`, city, lat, lon)
}

func getHotelsByPreferencesPrompt(city string, lat, lon float64, userPreferences types.HotelUserPreferences) string {
return fmt.Sprintf(`
        Generate a list of maximum 5 hotels in the city of %s, near the coordinates %0.2f , %0.2f.
        The hotels should be relevant to the user's interest.
        The result should be tailored to the user's preferences:
        - Preferred Category: %s
        - Preferred Tags: %s
        - Max Price range: %s
        - Preferred Rating: %0.2f
        - Number of Guests: %d
        - Number of Nights: %d
        - Number of Rooms: %d
        - Preferred Check-In Date: %s
        - Preferred Check-Out Date: %s
        - Distance: %0.2f km (if provided, otherwise use default radius of 5km)
        The result should be in the following JSON format:
        {
            "hotels": [
                {
                    "name": "Name of the Hotel",
                    "latitude": <float>,
                    "longitude": <float>,
                    "category": "Primary category (e.g., Hotel, Hostel, Guesthouse)",
                    "description": "A brief description of this hotel and why it's relevant to the user's interest."
                }
            ]
        }
    `, city, lat, lon, userPreferences.PreferredCategories, userPreferences.PreferredTags,
userPreferences.MaxPriceRange, userPreferences.MinRating,
userPreferences.NumberOfGuests, userPreferences.NumberOfNights, userPreferences.NumberOfRooms,
userPreferences.PreferredCheckIn.Format("2006-01-02"), userPreferences.PreferredCheckOut.Format("2006-01-02"),
userPreferences.SearchRadiusKm)
}

func getHotelsNeabyPrompt(city string, userLocation types.UserLocation) string {
return fmt.Sprintf(`
        Generate a list of maximum 5 hotels nearby the coordinates %0.2f , %0.2f in the city of %s.
        the hotels can be around %0.2f km radius from the user's location or if nothing provided, use the default radius of 5km.
        The hotels should be relevant to the user's interest.
        The result should be in the following JSON format:
        {
            "hotels": [
                {
                    "name": "Name of the Hotel",
                    "latitude": <float>,
                    "longitude": <float>,
                    "category": "Primary category (e.g., Hotel, Hostel, Guesthouse)",
                    "description": "A brief description of this hotel and why it's relevant to the user's interest."
                }
            ]
        }
    `, userLocation.UserLat, userLocation.UserLon, city, userLocation.SearchRadiusKm)
}

func getRestaurantsByPreferencesPrompt(city string, lat, lon float64, userPreferences types.RestaurantUserPreferences) string {
return fmt.Sprintf(`
        Generate a list of up to 10 restaurants in the city of %s, near coordinates %.2f, %.2f.
        Tailor the results to the user's preferences:
        - Preferred Cuisine: %s
        - Preferred Price Range: %s
        - Dietary Restrictions: %s
        - Ambiance: %s
        - Special Features: %s
        The result must be in JSON format:
        {
            "restaurants": [
                {
                    "name": "Restaurant Name",
                    "latitude": <float>,
                    "longitude": <float>,
                    "category": "Restaurant|Bar|Cafe",
                    "description": "Brief description of the restaurant and why it matches preferences."
                }
            ]
        }
    `, city, lat, lon, userPreferences.PreferredCuisine, userPreferences.PreferredPriceRange,
userPreferences.DietaryRestrictions, userPreferences.Ambiance, userPreferences.SpecialFeatures)
}

func getRestaurantsNearbyPrompt(city string, userLocation types.UserLocation) string {
if userLocation.SearchRadiusKm == 0 {
userLocation.SearchRadiusKm = 5.0 // Default radius
}
return fmt.Sprintf(`
        Generate a list of up to 10 restaurants in the city of %s, within %.2f km of coordinates %.2f, %.2f.
        Include a variety of restaurant categories to provide diverse options.
        The result must be in JSON format:
        {
            "restaurants": [
                {
                    "name": "Restaurant Name",
                    "latitude": <float>,
                    "longitude": <float>,
                    "category": "Restaurant|Bar|Cafe",
                    "description": "Brief description of the restaurant and its proximity to the user's location."
                }
            ]
        }
    `, city, userLocation.SearchRadiusKm, userLocation.UserLat, userLocation.UserLon)
}

func generatedContinuedConversationPrompt(poi, city string) string {
return fmt.Sprintf(
`Provide detailed information about "%s" in %s.
If user writes "Restaurant" add "cuisine_type" to final response and hide "description_poi"
If user writes "Hotel" add "star_rating" to final response and hide "description_poi"
Analise this POI (The user can insert a POI name, a Restaurant name or an Hotel/Hostel name) and return the following JSON structure:
{
"name": "string (the POI name)",
"latitude": number (approximate latitude as float),
"longitude": number (approximate longitude as float),
"category": "string (e.g., Museum, Park, Historical Site)",
"description_poi": "string (50-100 words description)"
"cuisine_type": "string (for Restaurant)",
"star_rating": "number (for Hotel/Hostel)"
}

    If the POI is not found, return: {"name": "", "latitude": 0, "longitude": 0, "category": "", "description_poi": ""}`,
		poi, city)
}

// getCityDescriptionPrompt generates a prompt for city data
func getCityDescriptionPrompt(cityName string) string {
return fmt.Sprintf(`
        Provide detailed information about the city %s in JSON format with the following structure:
        {
            "city_name": "%s",
            "country": "Country name",
            "state_province": "State or province, if applicable",
            "description": "A detailed description of the city",
            "center_latitude": float64,
            "center_longitude": float64
        }
    `, cityName, cityName)
}

// GetUnifiedChatPrompt generates context-based prompts for the unified chat system
func GetUnifiedChatPrompt(context, cityName string, lat, lon float64, searchProfile *types.UserPreferenceProfileResponse) string {
basePreferences := ""
if searchProfile != nil {
basePreferences = getUserPreferencesPrompt(searchProfile)
}

	switch context {
	case "traveling", "itinerary":
		return fmt.Sprintf(`
You are a travel planning assistant. Create a personalized itinerary for %s based on the user's location (%.4f, %.4f) and preferences.

USER PREFERENCES:
%s

Generate a comprehensive travel response in JSON format with the following structure:
{
"data": {
"general_city_data": {
"city": "%s",
"country": "Country name",
"state_province": "State/Province if applicable",
"description": "Detailed city description (100-150 words)",
"center_latitude": %.4f,
"center_longitude": %.4f,
"population": "",
"area": "",
"timezone": "",
"language": "",
"weather": "",
"attractions": "",
"history": ""
},
"points_of_interest": [
{
"name": "POI Name",
"latitude": <float>,
"longitude": <float>,
"category": "Category (e.g., Museum, Historical Site)",
"description_poi": "",
"address": "",
"website": "",
"opening_hours": "JSONB, -- Store opening hours structured (e.g., OSM opening_hours format or custom JSON)"

            }
        ],
        "itinerary_response": {
            "itinerary_name": "Creative itinerary name based on user preferences",
            "overall_description": "Detailed description of the itinerary (100-150 words)",
            "points_of_interest": [
                {
                    "name": "POI Name",
                    "latitude": <float>,
                    "longitude": <float>,
                    "category": "",
                    "description_poi": "",
                    "address": "",
                    "website": "",
                    "opening_hours": "JSONB, -- Store opening hours structured (e.g., OSM opening_hours format or custom JSON)"

                }
            ]
        }
    }
}

Focus on creating an itinerary that matches the user's preferences, dietary needs, preferred pace, and transportation method.`,
cityName, lat, lon, basePreferences, cityName, lat, lon)

	case "accommodation":
		return fmt.Sprintf(`
You are a hotel recommendation assistant. Find suitable accommodation in %s near coordinates %.4f, %.4f.

USER PREFERENCES:
%s

Generate a hotel response in JSON format:
{
"hotels": [
{
"city": "%s",
"name": "Hotel Name",
"latitude": <float>,
"longitude": <float>,
"category": "Hotel|Hostel|Guesthouse|Apartment",
"description": "Description matching user preferences and budget level",
"address": "",
"phone_number": null,
"website": null,
"opening_hours": "JSONB, -- Store opening hours structured (e.g., OSM opening_hours format or custom JSON)"
"price_range": null,
"rating": 0,
"tags": null,
"images": null
}
]
}

Consider the user's budget level, preferred amenities, and accessibility needs when selecting accommodation.`,
cityName, lat, lon, basePreferences, cityName)

	case "dining":
		return fmt.Sprintf(`
You are a restaurant recommendation assistant. Find dining options in %s near coordinates %.4f, %.4f.

USER PREFERENCES:
%s

Generate a restaurant response in JSON format:
{
"restaurants": [
{
"city": "%s",
"name": "Restaurant Name",
"latitude": <float>,
"longitude": <float>,
"category": "Fine Dining|Casual Dining|Fast Food|Cafe|Bar",
"description": "Description matching user dietary needs and preferences",
"address": "Complete address",
"website": "Official website URL (if available)",
"phone_number": "Phone number (if available)",
"opening_hours": "JSONB, -- Store opening hours structured (e.g., OSM opening_hours format or custom JSON)",

            "price_level": "$|$$|$$$|$$$$",
            "cuisine_type": "Cuisine type",
            "tags": ["tag1", "tag2"],
            "images": [],
            "rating": 0
        }
    ]
}

Pay special attention to dietary needs, budget level, cuisine preferences, and accessibility options.`,
cityName, lat, lon, basePreferences, cityName)

	case "activities":
		return fmt.Sprintf(`
You are an activity recommendation assistant. Find activities and attractions in %s near coordinates %.4f, %.4f.

USER PREFERENCES:
%s

Generate an activities response in JSON format:
{
"activities": [
{
"city": "%s",
"name": "Activity/Attraction Name",
"latitude": <float>,
"longitude": <float>,
"category": "Museum|Outdoor Activity|Entertainment|Cultural|Sports",
"description": "Description matching user activity preferences",
"address": "Complete address",
"website": "Official website URL (if available)",
"opening_hours": "JSONB, -- Store opening hours structured (e.g., OSM opening_hours format or custom JSON)",

            "price_range": "Free|$|$$|$$$",
            "rating": 0,
            "tags": ["tag1", "tag2"],
            "images": []
        }
    ]
}

Consider the user's physical activity level, cultural interests, and accessibility needs when selecting activities.`,
cityName, lat, lon, basePreferences, cityName)

	default:
		// Default to itinerary if context is not recognized
		return GetUnifiedChatPrompt("traveling", cityName, lat, lon, searchProfile)
	}
}

/*
Testing Fan in Fan out prompt
*/

func getCityDataPrompt(cityName string) string {
return fmt.Sprintf(`
You are a travel assistant. Provide general information about %s.
Respond with JSON:
{
    "city": "%s",
    "country": "Country name",
    "state_province": "State/Province if applicable",
    "description": "Detailed city description (100-150 words)",
    "center_latitude": <float>,
    "center_longitude": <float>,
    "population": "",
    "area": "",
    "timezone": "",
    "language": "",
    "weather": "",
    "attractions": "",
    "history": ""
}`, cityName, cityName)
}

func getGeneralPOIPrompt(cityName string) string {
return fmt.Sprintf(`
You are a travel assistant. List general points of interest in %s.
Respond with JSON:
{
"points_of_interest": [
{
"name": "POI Name",
"latitude": <float>,
"longitude": <float>,
"category": "Category (e.g., Museum, Historical Site)",
"description_poi": "",
"address": "",
"website": "",
"opening_hours": "JSONB, -- Store opening hours structured (e.g., OSM opening_hours format or custom JSON)"

        }
    ]
}`, cityName)
}

func getPersonalizedItineraryPrompt(cityName, basePreferences string) string {
return fmt.Sprintf(`
You are a travel planning assistant. Create a personalized itinerary for %s based on user preferences.
USER PREFERENCES:
%s
Respond with JSON:
{
    "itinerary_name": "Creative itinerary name",
    "overall_description": "Detailed description (100-150 words)",
    "points_of_interest": [
        {
            "name": "POI Name",
            "latitude": <float>,
            "longitude": <float>,
            "category": "",
            "description_poi": "",
            "address": "",
            "website": "",
                		"opening_hours": "JSONB, -- Store opening hours structured (e.g., OSM opening_hours format or custom JSON)"
,
            "distance": <float>
        }
    ]
}`, cityName, basePreferences)
}

func getGeneralizedItineraryPrompt(cityName string) string {
return fmt.Sprintf(`
You are a travel planning assistant. Create a personalized itinerary with a max of 5 results for %s with multi things to do and different activities.
Respond with JSON:
{
    "itinerary_name": "Creative itinerary name",
    "overall_description": "Detailed description (100-150 words)",
    "points_of_interest": [
        {
            "name": "POI Name",
            "latitude": <float>,
            "longitude": <float>,
            "category": "",
            "description_poi": "",
            "address": "",
            "website": "",
                		"opening_hours": "JSONB, -- Store opening hours structured (e.g., OSM opening_hours format or custom JSON)"
,
            "distance": <float>
        }
    ]
}`, cityName)
}

func getAccommodationPrompt(cityName string, lat, lon float64, basePreferences string) string {
return fmt.Sprintf(`
You are a hotel recommendation assistant. Find suitable accommodation in %s near coordinates %.4f, %.4f.
USER PREFERENCES:
%s
Respond with JSON:
{
    "hotels": [
        {
            "city": "%s",
            "name": "Hotel Name",
            "latitude": <float>,
            "longitude": <float>,
            "category": "Hotel|Hostel|Guesthouse|Apartment",
            "description": "Description matching preferences",
            "address": "",
            "phone_number": null,
            "website": null,
                		"opening_hours": "JSONB, -- Store opening hours structured (e.g., OSM opening_hours format or custom JSON)",
            "price_range": null,
            "rating": 0,
            "tags": null,
            "images": null,
            "distance": <float>
        }
    ]
}`, cityName, lat, lon, basePreferences, cityName)
}

func getGeneralAccommodationPrompt(cityName string) string {
return fmt.Sprintf(`
You are a hotel recommendation assistant. Find a max of 5 suitable accommodation in %s.
Respond with JSON:
{
    "hotels": [
        {
            "city": "%s",
            "name": "Hotel Name",
            "latitude": <float>,
            "longitude": <float>,
            "category": "Hotel|Hostel|Guesthouse|Apartment",
            "description": "Description matching preferences",
            "address": "",
            "phone_number": null,
            "website": null,
                		"opening_hours": "JSONB, -- Store opening hours structured (e.g., OSM opening_hours format or custom JSON)",
            "price_range": null,
            "rating": 0,
            "tags": null,
            "images": null,
            "distance": <float>
        }
    ]
}`, cityName, cityName)
}

func getDiningPrompt(cityName string, lat, lon float64, basePreferences string) string {
return fmt.Sprintf(`
You are a restaurant recommendation assistant. Find dining options in %s near coordinates %.4f, %.4f.
USER PREFERENCES:
%s
Respond with JSON:
{
    "restaurants": [
        {
            "city": "%s",
            "name": "Restaurant Name",
            "latitude": <float>,
            "longitude": <float>,
            "category": "Fine Dining|Casual Dining|Fast Food|Cafe|Bar",
            "description": "Description matching preferences",
            "address": "",
            "website": "",
            "phone_number": "",
                		"opening_hours": "JSONB, -- Store opening hours structured (e.g., OSM opening_hours format or custom JSON)"
,
            "price_level": "$|$$|$$$|$$$$",
            "cuisine_type": "",
            "tags": [],
            "images": [],
            "rating": 0,
            "distance": <float>
        }
    ]
}`, cityName, lat, lon, basePreferences, cityName)
}

func getGeneralDiningPrompt(cityName string) string {
return fmt.Sprintf(`
You are a restaurant recommendation assistant. Find a max of 5 dining options in %s.
Respond with JSON:
{
    "restaurants": [
        {
            "city": "%s",
            "name": "Restaurant Name",
            "latitude": <float>,
            "longitude": <float>,
            "category": "Fine Dining|Casual Dining|Fast Food|Cafe|Bar",
            "description": "Description matching preferences",
            "address": "",
            "website": "",
            "phone_number": "",
                		"opening_hours": "JSONB, -- Store opening hours structured (e.g., OSM opening_hours format or custom JSON)"
,
            "price_level": "$|$|$$|$$",
            "cuisine_type": "",
            "tags": [],
            "images": [],
            "rating": 0,
            "distance": <float>
        }
    ]
}`, cityName, cityName)
}

func getActivitiesPrompt(cityName string, lat, lon float64, basePreferences string) string {
return fmt.Sprintf(`
You are an activity recommendation assistant. Find activities in %s near coordinates %.4f, %.4f.
USER PREFERENCES:
%s
Respond with JSON:
{
    "activities": [
        {
            "city": "%s",
            "name": "Activity Name",
            "latitude": <float>,
            "longitude": <float>,
            "category": "Museum|Outdoor Activity|Entertainment|Cultural|Sports",
            "description": "Description matching preferences",
            "address": "",
            "website": "",
                		"opening_hours": "JSONB, -- Store opening hours structured (e.g., OSM opening_hours format or custom JSON)"
,
            "price_range": "Free|$|$$|$$$",
            "rating": 0,
            "tags": [],
            "images": [],
            "distance": <float>
        }
    ]
}`, cityName, lat, lon, basePreferences, cityName)
}

func getGeneralActivitiesPrompt(cityName string) string {
return fmt.Sprintf(`
You are an activity recommendation assistant. Find a max of 5 activities in %s.
Respond with JSON:
{
    "activities": [
        {
            "city": "%s",
            "name": "Activity Name",
            "latitude": <float>,
            "longitude": <float>,
            "category": "Museum|Outdoor Activity|Entertainment|Cultural|Sports",
            "description": "Description matching preferences",
            "address": "",
            "website": "",
                		"opening_hours": "JSONB, -- Store opening hours structured (e.g., OSM opening_hours format or custom JSON)"
,
            "price_range": "Free|$|$$|$$$",
            "rating": 0,
            "tags": [],
            "images": [],
            "distance": <float>
        }
    ]
}`, cityName, cityName)
}


package llmChat

import (
"context"
"database/sql"
"encoding/json"
"fmt"
"log/slog"
"regexp"
"strings"
"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"

	"github.com/FACorreiaa/go-poi-au-suggestions/internal/types"
)

var _ Repository = (*RepositoryImpl)(nil)

type Repository interface {
SaveInteraction(ctx context.Context, interaction types.LlmInteraction) (uuid.UUID, error)
SaveLlmSuggestedPOIsBatch(ctx context.Context, pois []types.POIDetailedInfo, userID, searchProfileID, llmInteractionID, cityID uuid.UUID) error
GetLlmSuggestedPOIsByInteractionSortedByDistance(ctx context.Context, llmInteractionID uuid.UUID, cityID uuid.UUID, userLocation types.UserLocation) ([]types.POIDetailedInfo, error)
AddChatToBookmark(ctx context.Context, itinerary *types.UserSavedItinerary) (uuid.UUID, error)
RemoveChatFromBookmark(ctx context.Context, userID, itineraryID uuid.UUID) error
GetInteractionByID(ctx context.Context, interactionID uuid.UUID) (*types.LlmInteraction, error)
GetLatestInteractionBySessionID(ctx context.Context, sessionID uuid.UUID) (*types.LlmInteraction, error)

	// Session methods
	CreateSession(ctx context.Context, session types.ChatSession) error
	GetSession(ctx context.Context, sessionID uuid.UUID) (*types.ChatSession, error)
	GetUserChatSessions(ctx context.Context, userID uuid.UUID) ([]types.ChatSession, error)
	UpdateSession(ctx context.Context, session types.ChatSession) error
	AddMessageToSession(ctx context.Context, sessionID uuid.UUID, message types.ConversationMessage) error

	//
	SaveSinglePOI(ctx context.Context, poi types.POIDetailedInfo, userID, cityID uuid.UUID, llmInteractionID uuid.UUID) (uuid.UUID, error)
	GetPOIsBySessionSortedByDistance(ctx context.Context, sessionID, cityID uuid.UUID, userLocation types.UserLocation) ([]types.POIDetailedInfo, error)
	GetOrCreatePOI(ctx context.Context, tx pgx.Tx, POIDetailedInfo types.POIDetailedInfo, cityID uuid.UUID, sourceInteractionID uuid.UUID) (uuid.UUID, error)
	SaveItineraryPOIs(ctx context.Context, itineraryID uuid.UUID, pois []types.POIDetailedInfo) error

	// RAG
	//SaveInteractionWithEmbedding(ctx context.Context, interaction types.LlmInteraction, embedding []float32) (uuid.UUID, error)
	//FindSimilarInteractions(ctx context.Context, queryEmbedding []float32, limit int, threshold float32) ([]types.LlmInteraction, error)
}

type RepositoryImpl struct {
logger *slog.Logger
pgpool *pgxpool.Pool
}

func NewRepositoryImpl(pgxpool *pgxpool.Pool, logger *slog.Logger) *RepositoryImpl {
return &RepositoryImpl{
logger: logger,
pgpool: pgxpool,
}
}

func (r *RepositoryImpl) SaveInteraction(ctx context.Context, interaction types.LlmInteraction) (uuid.UUID, error) {
ctx, span := otel.Tracer("LlmInteractionRepo").Start(ctx, "SaveInteraction", trace.WithAttributes(
semconv.DBSystemKey.String(semconv.DBSystemPostgreSQL.Value.AsString()),
attribute.String("db.operation", "INSERT_COMPLEX"),
attribute.String("db.sql.table", "llm_interactions,itineraries,itinerary_pois"),
attribute.String("user.id", interaction.UserID.String()),
attribute.String("model.used", interaction.ModelUsed),
attribute.Int("latency.ms", interaction.LatencyMs),
attribute.String("city.name_from_interaction", interaction.CityName),
))
defer span.End()

	var err error
	tx, err := r.pgpool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to start transaction")
		return uuid.Nil, fmt.Errorf("failed to start transaction: %w", err)
	}
	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback(ctx)
			panic(p)
		}
		if err != nil {
			if rbErr := tx.Rollback(ctx); rbErr != nil {
				r.logger.ErrorContext(ctx, "Transaction rollback failed after error", "original_error", err, "rollback_error", rbErr)
				span.RecordError(fmt.Errorf("transaction rollback failed: %v (original error: %w)", rbErr, err))
			}
		}
	}()

	interactionQuery := `
        INSERT INTO llm_interactions (
            user_id, session_id, prompt, response, model_name, latency_ms, city_name
        ) VALUES ($1, $2, $3, $4, $5, $6, $7)
        RETURNING id
    `
	var interactionID uuid.UUID
	err = tx.QueryRow(ctx, interactionQuery,
		interaction.UserID,
		interaction.SessionID,
		interaction.Prompt,
		interaction.ResponseText,
		interaction.ModelUsed,
		interaction.LatencyMs,
		interaction.CityName,
	).Scan(&interactionID)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to insert llm_interaction")
		return uuid.Nil, fmt.Errorf("failed to insert llm_interaction: %w", err)
	}
	span.SetAttributes(attribute.String("llm_interaction.id", interactionID.String()))

	var cityID uuid.UUID
	if interaction.CityName != "" {
		cityQuery := `SELECT id FROM cities WHERE name = $1 LIMIT 1`
		err = tx.QueryRow(ctx, cityQuery, interaction.CityName).Scan(&cityID)
		if err != nil {
			if err == pgx.ErrNoRows {
				r.logger.WarnContext(ctx, "City not found in database, itinerary creation will be skipped", "city_name", interaction.CityName, "interaction_id", interactionID.String())
				span.AddEvent("City not found in database", trace.WithAttributes(attribute.String("city.name", interaction.CityName)))
				// err is pgx.ErrNoRows, so cityID remains uuid.Nil, processing continues correctly. Clear err.
				err = nil
			} else {
				span.RecordError(err)
				span.SetStatus(codes.Error, "Failed to get city_id")
				return interactionID, fmt.Errorf("failed to get city_id for city '%s': %w", interaction.CityName, err)
			}
		} else {
			span.SetAttributes(attribute.String("city.id", cityID.String()))
		}
	} else {
		r.logger.InfoContext(ctx, "interaction.CityName is empty, cannot determine city_id. Itinerary creation will be skipped.", "interaction_id", interactionID.String())
		span.AddEvent("interaction.CityName is empty")
	}

	var itineraryID uuid.UUID
	if cityID != uuid.Nil {
		itineraryQuery := `
	        INSERT INTO itineraries (user_id, city_id, source_llm_interaction_id)
	        VALUES ($1, $2, $3)
	        ON CONFLICT (user_id, city_id) DO UPDATE SET
	            updated_at = NOW(),
	            source_llm_interaction_id = EXCLUDED.source_llm_interaction_id
	        RETURNING id
	    `
		err = tx.QueryRow(ctx, itineraryQuery, interaction.UserID, cityID, interactionID).Scan(&itineraryID)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, "Failed to insert or update itinerary")
			return interactionID, fmt.Errorf("failed to insert or update itinerary: %w", err)
		}
		span.SetAttributes(attribute.String("itinerary.id", itineraryID.String()))
	}

	if itineraryID != uuid.Nil {
		var pois []types.POIDetailedInfo
		// Only parse POIs for itinerary/general responses, skip for domain-specific responses
		if strings.Contains(interaction.Prompt, "Unified Chat - Domain: dining") ||
			strings.Contains(interaction.Prompt, "Unified Chat - Domain: accommodation") ||
			strings.Contains(interaction.Prompt, "Unified Chat - Domain: activities") {
			// Skip POI parsing for domain-specific responses that don't contain POIs
			r.logger.DebugContext(ctx, "Skipping POI parsing for domain-specific response", "interaction_id", interactionID.String())
			span.AddEvent("Skipped POI parsing for domain-specific response")
		} else {
			pois, err = parsePOIsFromResponse(interaction.ResponseText, r.logger)
			if err != nil {
				span.RecordError(err)
				span.SetStatus(codes.Error, "Failed to parse POIs from response")
				return interactionID, fmt.Errorf("failed to parse POIs from response: %w", err)
			}
		}
		span.SetAttributes(attribute.Int("parsed_pois.count", len(pois)))

		if len(pois) > 0 {
			poiBatch := &pgx.Batch{}
			itineraryPoiInsertQuery := `
	            INSERT INTO itinerary_pois (itinerary_id, poi_id, order_index, ai_description)
	            VALUES ($1, $2, $3, $4)
	            ON CONFLICT (itinerary_id, poi_id) DO UPDATE SET
	                order_index = EXCLUDED.order_index,
	                ai_description = EXCLUDED.ai_description,
	                updated_at = NOW()
	        `
			for i, POIDetailedInfoFromLlm := range pois {
				var poiDBID uuid.UUID
				poiDBID, err = r.GetOrCreatePOI(ctx, tx, POIDetailedInfoFromLlm, cityID, interactionID)
				if err != nil {
					span.RecordError(err)
					span.SetStatus(codes.Error, "Failed to get or create POI")
					return interactionID, fmt.Errorf("failed to get or create POI '%s': %w", POIDetailedInfoFromLlm.Name, err)
				}
				poiBatch.Queue(itineraryPoiInsertQuery, itineraryID, poiDBID, i, POIDetailedInfoFromLlm.DescriptionPOI) // Assumes types.POIDetailedInfo has DescriptionPOI
			}

			if poiBatch.Len() > 0 {
				br := tx.SendBatch(ctx, poiBatch)
				for i := 0; i < poiBatch.Len(); i++ {
					_, execErr := br.Exec()
					if execErr != nil {
						err = fmt.Errorf("failed to insert itinerary_poi in batch (operation %d of %d for itinerary %s): %w", i+1, poiBatch.Len(), itineraryID.String(), execErr)
						if closeErr := br.Close(); closeErr != nil {
							r.logger.ErrorContext(ctx, "Failed to close batch for itinerary_pois after an exec error", "close_error", closeErr, "original_batch_error", err)
						}
						span.RecordError(err)
						span.SetStatus(codes.Error, "Failed to insert itinerary_poi in batch")
						return interactionID, err
					}
				}
				err = br.Close()
				if err != nil {
					span.RecordError(err)
					span.SetStatus(codes.Error, "Failed to close batch for itinerary_pois")
					return interactionID, fmt.Errorf("failed to close batch for itinerary_pois: %w", err)
				}
				span.SetAttributes(attribute.Int("itinerary_pois.inserted_or_updated.count", poiBatch.Len()))
			}
		}
	} else {
		if cityID != uuid.Nil {
			r.logger.WarnContext(ctx, "ItineraryID is Nil despite valid CityID, indicating itinerary insert/update issue.", "city_id", cityID.String(), "interaction_id", interactionID.String())
			span.AddEvent("ItineraryID is Nil despite valid CityID.")
		} else {
			r.logger.InfoContext(ctx, "Skipping itinerary_pois: itineraryID is Nil (likely city not found or CityName empty).", "interaction_id", interactionID.String())
			span.AddEvent("Skipping itinerary_pois: itineraryID is Nil.")
		}
	}

	err = tx.Commit(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to commit transaction")
		return uuid.Nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	span.SetStatus(codes.Ok, "Interaction and related entities saved successfully")
	return interactionID, nil
}

func (r *RepositoryImpl) SaveLlmSuggestedPOIsBatch(ctx context.Context, pois []types.POIDetailedInfo, userID, searchProfileID, llmInteractionID, cityID uuid.UUID) error {
ctx, span := otel.Tracer("LlmInteractionRepo").Start(ctx, "SaveLlmSuggestedPOIsBatch", trace.WithAttributes(
semconv.DBSystemPostgreSQL,
attribute.String("db.operation", "INSERT"),
attribute.String("db.sql.table", "llm_suggested_pois"),
attribute.String("user.id", userID.String()),
attribute.String("search_profile.id", searchProfileID.String()),
attribute.String("llm_interaction.id", llmInteractionID.String()),
attribute.String("city.id", cityID.String()),
attribute.Int("pois.count", len(pois)),
))
defer span.End()

	r.logger.InfoContext(ctx, "SaveLlmSuggestedPOIsBatch - About to save batch",
		slog.String("llm_interaction_id", llmInteractionID.String()),
		slog.String("user_id", userID.String()),
		slog.String("city_id", cityID.String()),
		slog.Int("poi_count", len(pois)))

	// Verify the llm_interaction_id exists before trying to insert POIs
	var exists bool
	checkQuery := `SELECT EXISTS(SELECT 1 FROM llm_interactions WHERE id = $1)`
	err := r.pgpool.QueryRow(ctx, checkQuery, llmInteractionID).Scan(&exists)
	if err != nil {
		r.logger.ErrorContext(ctx, "Failed to check if llm_interaction exists", slog.Any("error", err))
		return fmt.Errorf("failed to check if llm_interaction exists: %w", err)
	}
	if !exists {
		r.logger.ErrorContext(ctx, "llm_interaction_id does not exist in database", 
			slog.String("llm_interaction_id", llmInteractionID.String()))
		return fmt.Errorf("llm_interaction_id %s does not exist in database", llmInteractionID.String())
	}
	r.logger.InfoContext(ctx, "llm_interaction_id exists, proceeding with POI batch insert")

	batch := &pgx.Batch{}
	query := `
        INSERT INTO llm_suggested_pois 
            (user_id, search_profile_id, llm_interaction_id, city_id, 
             name, description_poi, location)
        VALUES 
            ($1, $2, $3, $4, $5, $6, ST_SetSRID(ST_MakePoint($7, $8), 4326))
    `

	for _, poi := range pois {
		batch.Queue(query,
			userID, searchProfileID, llmInteractionID, cityID,
			poi.Name, poi.DescriptionPOI, poi.Longitude, poi.Latitude, // Lon, Lat order for ST_MakePoint
		)
	}

	br := r.pgpool.SendBatch(ctx, batch)
	defer br.Close()

	for i := 0; i < len(pois); i++ {
		_, err := br.Exec()
		if err != nil {
			// Consider how to handle partial failures. Log and continue, or return error?
			span.RecordError(err)
			span.SetStatus(codes.Error, fmt.Sprintf("Failed to execute batch insert for POI %d", i))
			return fmt.Errorf("failed to execute batch insert for llm_suggested_poi %d: %w", i, err)
		}
	}

	span.SetStatus(codes.Ok, "POIs batch saved successfully")
	return nil
}

func (r *RepositoryImpl) GetLlmSuggestedPOIsByInteractionSortedByDistance(
ctx context.Context, llmInteractionID uuid.UUID, cityID uuid.UUID, userLocation types.UserLocation,
) ([]types.POIDetailedInfo, error) {
ctx, span := otel.Tracer("LlmInteractionRepo").Start(ctx, "GetLlmSuggestedPOIsByInteractionSortedByDistance", trace.WithAttributes(
semconv.DBSystemPostgreSQL,
attribute.String("db.operation", "SELECT"),
attribute.String("db.sql.table", "llm_suggested_pois"),
attribute.String("llm_interaction.id", llmInteractionID.String()),
attribute.String("city.id", cityID.String()),
attribute.Float64("user.latitude", userLocation.UserLat),
attribute.Float64("user.longitude", userLocation.UserLon),
))
defer span.End()

	userPoint := fmt.Sprintf("SRID=4326;POINT(%f %f)", userLocation.UserLon, userLocation.UserLat)

	// Ensure cityID filter is applied if cityID is not Nil
	// We filter by llm_interaction_id, so city_id might be redundant if interaction is specific to a city context
	// But adding it for robustness if an interaction could span POIs from different "requested" cities (unlikely for current setup).
	query := `
        SELECT 
            id, 
            name, 
            description_poi,
            ST_X(location::geometry) AS longitude, 
            ST_Y(location::geometry) AS latitude, 
            ST_Distance(location::geography, ST_GeomFromText($1, 4326)::geography) AS distance
        FROM llm_suggested_pois
        WHERE llm_interaction_id = $2 `

	args := []interface{}{userPoint, llmInteractionID}
	argCounter := 3

	if cityID != uuid.Nil {
		query += fmt.Sprintf("AND city_id = $%d ", argCounter)
		args = append(args, cityID)
		argCounter++
	}

	query += "ORDER BY distance ASC"

	rows, err := r.pgpool.Query(ctx, query, args...)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to query sorted POIs")
		return nil, fmt.Errorf("failed to query sorted llm_suggested_pois: %w", err)
	}
	defer rows.Close()

	var resultPois []types.POIDetailedInfo
	for rows.Next() {
		var p types.POIDetailedInfo
		var descr sql.NullString // Handle nullable fields from DB
		// var cat sql.NullString
		// var addr sql.NullString
		// var web sql.NullString
		// var openH sql.NullString

		err := rows.Scan(
			&p.ID, &p.Name, &descr,
			&p.Longitude, &p.Latitude,
			&p.Distance, // Ensure your types.POIDetailedInfo has Distance field
		)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, "Failed to scan POI row")
			return nil, fmt.Errorf("failed to scan llm_suggested_poi row: %w", err)
		}
		p.DescriptionPOI = descr.String
		//p.Category = cat.String

		resultPois = append(resultPois, p)
	}

	if err = rows.Err(); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Error iterating POI rows")
		return nil, fmt.Errorf("error iterating llm_suggested_poi rows: %w", err)
	}

	span.SetAttributes(attribute.Int("pois.count", len(resultPois)))
	span.SetStatus(codes.Ok, "POIs retrieved successfully")
	return resultPois, nil
}

func (r *RepositoryImpl) AddChatToBookmark(ctx context.Context, itinerary *types.UserSavedItinerary) (uuid.UUID, error) {
ctx, span := otel.Tracer("LlmInteractionRepo").Start(ctx, "AddChatToBookmark", trace.WithAttributes(
semconv.DBSystemPostgreSQL,
attribute.String("db.operation", "INSERT"),
attribute.String("db.sql.table", "user_saved_itineraries"),
attribute.String("user.id", itinerary.UserID.String()),
attribute.String("title", itinerary.Title),
))
defer span.End()

	tx, err := r.pgpool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to start transaction")
		return uuid.Nil, fmt.Errorf("failed to start transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	query := `
		INSERT INTO user_saved_itineraries (
			user_id, source_llm_interaction_id, primary_city_id, title, description,
			markdown_content, tags, estimated_duration_days, estimated_cost_level, is_public
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING id
	`
	var savedItineraryID uuid.UUID
	if err := tx.QueryRow(ctx, query,
		&itinerary.UserID,
		&itinerary.SourceLlmInteractionID,
		&itinerary.PrimaryCityID,
		&itinerary.Title,
		&itinerary.Description,
		&itinerary.MarkdownContent,
		&itinerary.Tags,
		&itinerary.EstimatedDurationDays,
		&itinerary.EstimatedCostLevel,
		&itinerary.IsPublic,
	).Scan(&savedItineraryID); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to insert itinerary")
		return uuid.Nil, fmt.Errorf("failed to insert user_saved_itineraries: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to commit transaction")
		return uuid.Nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	span.SetAttributes(attribute.String("saved_itinerary.id", savedItineraryID.String()))
	span.SetStatus(codes.Ok, "Itinerary saved successfully")
	return savedItineraryID, nil
}

func (r *RepositoryImpl) GetInteractionByID(ctx context.Context, interactionID uuid.UUID) (*types.LlmInteraction, error) {
ctx, span := otel.Tracer("LlmInteractionRepo").Start(ctx, "GetInteractionByID", trace.WithAttributes(
semconv.DBSystemPostgreSQL,
attribute.String("db.operation", "SELECT"),
attribute.String("db.sql.table", "llm_interactions"),
attribute.String("interaction.id", interactionID.String()),
))
defer span.End()

	query := `
		SELECT 
			id, user_id, prompt, response, model_name, latency_ms,
			prompt_tokens, completion_tokens, total_tokens,
			request_payload, response_payload
		FROM llm_interactions
		WHERE id = $1
	`
	row := r.pgpool.QueryRow(ctx, query, interactionID)

	var interaction types.LlmInteraction

	nullPromptTokens := sql.NullInt64{}
	nullCompletionTokens := sql.NullInt64{}
	nullTotalTokens := sql.NullInt64{}
	nullRequestPayload := sql.NullString{}
	nullResponsePayload := sql.NullString{}

	if err := row.Scan(
		&interaction.ID,
		&interaction.UserID,
		&interaction.Prompt,
		&interaction.ResponseText,
		&interaction.ModelUsed,
		&interaction.LatencyMs,
		&nullPromptTokens,
		&nullCompletionTokens,
		&nullTotalTokens,
		&nullRequestPayload,
		&nullResponsePayload,
	); err != nil {
		if err == pgx.ErrNoRows {
			span.SetStatus(codes.Error, "Interaction not found")
			return nil, fmt.Errorf("no interaction found with ID %s", interactionID)
		}
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to scan interaction row")
		return nil, fmt.Errorf("failed to scan llm_interaction row: %w", err)
	}

	span.SetAttributes(
		attribute.String("user.id", interaction.UserID.String()),
		attribute.String("model.used", interaction.ModelUsed),
		attribute.Int("latency.ms", interaction.LatencyMs),
	)
	span.SetStatus(codes.Ok, "Interaction retrieved successfully")
	return &interaction, nil
}

func (r *RepositoryImpl) GetLatestInteractionBySessionID(ctx context.Context, sessionID uuid.UUID) (*types.LlmInteraction, error) {
ctx, span := otel.Tracer("LlmInteractionRepo").Start(ctx, "GetLatestInteractionBySessionID", trace.WithAttributes(
semconv.DBSystemPostgreSQL,
attribute.String("db.operation", "SELECT"),
attribute.String("db.sql.table", "llm_interactions"),
attribute.String("session.id", sessionID.String()),
))
defer span.End()

	query := `
		SELECT 
			id, user_id, session_id, prompt, response, model_name, latency_ms,
			prompt_tokens, completion_tokens, total_tokens,
			request_payload, response_payload, city_name, created_at
		FROM llm_interactions
		WHERE session_id = $1
		ORDER BY created_at DESC
		LIMIT 1
	`
	row := r.pgpool.QueryRow(ctx, query, sessionID)

	var interaction types.LlmInteraction

	nullPromptTokens := sql.NullInt64{}
	nullCompletionTokens := sql.NullInt64{}
	nullTotalTokens := sql.NullInt64{}
	nullRequestPayload := sql.NullString{}
	nullResponsePayload := sql.NullString{}
	nullCityName := sql.NullString{}
	nullSessionID := uuid.NullUUID{}

	if err := row.Scan(
		&interaction.ID,
		&interaction.UserID,
		&nullSessionID,
		&interaction.Prompt,
		&interaction.ResponseText,
		&interaction.ModelUsed,
		&interaction.LatencyMs,
		&nullPromptTokens,
		&nullCompletionTokens,
		&nullTotalTokens,
		&nullRequestPayload,
		&nullResponsePayload,
		&nullCityName,
		&interaction.Timestamp,
	); err != nil {
		if err == pgx.ErrNoRows {
			span.SetStatus(codes.Error, "No interactions found for session")
			return nil, fmt.Errorf("no interactions found for session ID %s", sessionID)
		}
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to scan interaction row")
		return nil, fmt.Errorf("failed to scan llm_interaction row: %w", err)
	}

	// Handle nullable fields
	if nullSessionID.Valid {
		interaction.SessionID = nullSessionID.UUID
	}
	if nullCityName.Valid {
		interaction.CityName = nullCityName.String
	}
	if nullPromptTokens.Valid {
		interaction.PromptTokens = int(nullPromptTokens.Int64)
	}
	if nullCompletionTokens.Valid {
		interaction.CompletionTokens = int(nullCompletionTokens.Int64)
	}
	if nullTotalTokens.Valid {
		interaction.TotalTokens = int(nullTotalTokens.Int64)
	}

	span.SetAttributes(
		attribute.String("user.id", interaction.UserID.String()),
		attribute.String("session.id", interaction.SessionID.String()),
		attribute.String("model.used", interaction.ModelUsed),
		attribute.Int("latency.ms", interaction.LatencyMs),
	)
	span.SetStatus(codes.Ok, "Latest interaction retrieved successfully")
	return &interaction, nil
}

func (r *RepositoryImpl) RemoveChatFromBookmark(ctx context.Context, userID, itineraryID uuid.UUID) error {
ctx, span := otel.Tracer("LlmInteractionRepo").Start(ctx, "RemoveChatFromBookmark", trace.WithAttributes(
semconv.DBSystemPostgreSQL,
attribute.String("db.operation", "DELETE"),
attribute.String("db.sql.table", "user_saved_itineraries"),
attribute.String("user.id", userID.String()),
attribute.String("itinerary.id", itineraryID.String()),
))
defer span.End()

	tx, err := r.pgpool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to start transaction")
		return fmt.Errorf("failed to start transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	query := `
		DELETE FROM user_saved_itineraries
		WHERE id = $1 AND user_id = $2
	`
	tag, err := tx.Exec(ctx, query, itineraryID, userID)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to delete itinerary")
		return fmt.Errorf("failed to delete user_saved_itinerary with ID %s: %w", itineraryID, err)
	}

	if tag.RowsAffected() == 0 {
		err := fmt.Errorf("no itinerary found with ID %s for user %s", itineraryID, userID)
		span.RecordError(err)
		span.SetStatus(codes.Error, "Itinerary not found")
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to commit transaction")
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	span.SetStatus(codes.Ok, "Itinerary removed successfully")
	return nil
}

// sessions
func (r *RepositoryImpl) CreateSession(ctx context.Context, session types.ChatSession) error {
tx, err := r.pgpool.Begin(ctx)
if err != nil {
r.logger.ErrorContext(ctx, "Failed to begin transaction for session creation", slog.Any("error", err))
return fmt.Errorf("failed to begin transaction: %w", err)
}
defer tx.Rollback(ctx)

	query := `
        INSERT INTO chat_sessions (
            id, user_id, profile_id, city_name, current_itinerary, conversation_history, session_context,
            created_at, updated_at, expires_at, status
        ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
    `
	itineraryJSON, _ := json.Marshal(session.CurrentItinerary)
	historyJSON, _ := json.Marshal(session.ConversationHistory)
	contextJSON, _ := json.Marshal(session.SessionContext)

	_, err = tx.Exec(ctx, query, session.ID, session.UserID, session.ProfileID, session.CityName,
		itineraryJSON, historyJSON, contextJSON, session.CreatedAt, session.UpdatedAt, session.ExpiresAt, session.Status)
	if err != nil {
		r.logger.ErrorContext(ctx, "Failed to create session", slog.Any("error", err))
		return fmt.Errorf("failed to create session: %w", err)
	}

	if err = tx.Commit(ctx); err != nil {
		r.logger.ErrorContext(ctx, "Failed to commit session creation transaction", slog.Any("error", err))
		return fmt.Errorf("failed to commit session creation: %w", err)
	}

	return nil
}

// GetSession retrieves a session by ID
func (r *RepositoryImpl) GetSession(ctx context.Context, sessionID uuid.UUID) (*types.ChatSession, error) {
query := `
        SELECT id, user_id, profile_id, city_name, current_itinerary, conversation_history, session_context,
               created_at, updated_at, expires_at, status
        FROM chat_sessions WHERE id = $1
    `
row := r.pgpool.QueryRow(ctx, query, sessionID)

	var session types.ChatSession
	var itineraryJSON, historyJSON, contextJSON []byte
	err := row.Scan(&session.ID, &session.UserID, &session.ProfileID, &session.CityName,
		&itineraryJSON, &historyJSON, &contextJSON, &session.CreatedAt, &session.UpdatedAt, &session.ExpiresAt, &session.Status)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("session %s not found", sessionID)
		}
		r.logger.ErrorContext(ctx, "Failed to get session", slog.Any("error", err))
		return nil, fmt.Errorf("failed to get session: %w", err)
	}

	json.Unmarshal(itineraryJSON, &session.CurrentItinerary)
	json.Unmarshal(historyJSON, &session.ConversationHistory)
	json.Unmarshal(contextJSON, &session.SessionContext)
	return &session, nil
}

// GetUserChatSessions retrieves chat history from LLM interactions grouped by session/city, ordered by most recent first
func (r *RepositoryImpl) GetUserChatSessions(ctx context.Context, userID uuid.UUID) ([]types.ChatSession, error) {
ctx, span := otel.Tracer("LlmInteractionRepo").Start(ctx, "GetUserChatSessions", trace.WithAttributes(
semconv.DBSystemKey.String(semconv.DBSystemPostgreSQL.Value.AsString()),
attribute.String("db.operation", "SELECT"),
attribute.String("db.sql.table", "llm_interactions"),
attribute.String("user.id", userID.String()),
))
defer span.End()

	query := `
        WITH grouped_interactions AS (
            SELECT 
                COALESCE(session_id, city_name || '_' || DATE(created_at)) as session_key,
                user_id,
                city_name,
                MIN(created_at) as first_interaction,
                MAX(created_at) as last_interaction,
                COUNT(*) as interaction_count,
                -- Performance metrics aggregation
                AVG(latency_ms)::int as avg_latency_ms,
                SUM(total_tokens) as total_tokens,
                SUM(prompt_tokens) as total_prompt_tokens,
                SUM(completion_tokens) as total_completion_tokens,
                SUM(latency_ms) as total_latency_ms,
                array_agg(DISTINCT model_name) FILTER (WHERE model_name IS NOT NULL) as models_used,
                json_agg(
                    json_build_object(
                        'id', id,
                        'prompt', prompt,
                        'response', response,
                        'created_at', created_at,
                        'city_name', city_name,
                        'session_id', session_id,
                        'model_name', model_name,
                        'latency_ms', latency_ms,
                        'total_tokens', total_tokens,
                        'prompt_tokens', prompt_tokens,
                        'completion_tokens', completion_tokens
                    ) ORDER BY created_at
                ) as interactions
            FROM llm_interactions 
            WHERE user_id = $1 AND prompt IS NOT NULL
            GROUP BY session_key, user_id, city_name
        )
        SELECT 
            session_key,
            user_id,
            city_name,
            first_interaction,
            last_interaction,
            interaction_count,
            avg_latency_ms,
            total_tokens,
            total_prompt_tokens,
            total_completion_tokens,
            total_latency_ms,
            models_used,
            interactions
        FROM grouped_interactions
        ORDER BY last_interaction DESC
        LIMIT 50
    `

	rows, err := r.pgpool.Query(ctx, query, userID)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to query LLM interactions")
		r.logger.ErrorContext(ctx, "Failed to get user chat sessions from LLM interactions", slog.Any("error", err), slog.String("user_id", userID.String()))
		return nil, fmt.Errorf("failed to get user chat sessions: %w", err)
	}
	defer rows.Close()

	var sessions []types.ChatSession
	for rows.Next() {
		var sessionKey, cityName string
		var userIDFromDB uuid.UUID
		var firstInteraction, lastInteraction time.Time
		var interactionCount int
		var avgLatencyMs, totalTokens, totalPromptTokens, totalCompletionTokens, totalLatencyMs sql.NullInt64
		var modelsUsed []string
		var interactionsJSON string

		err := rows.Scan(
			&sessionKey, &userIDFromDB, &cityName, &firstInteraction, &lastInteraction, &interactionCount,
			&avgLatencyMs, &totalTokens, &totalPromptTokens, &totalCompletionTokens, &totalLatencyMs,
			&modelsUsed, &interactionsJSON,
		)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, "Failed to scan LLM interaction row")
			r.logger.ErrorContext(ctx, "Failed to scan LLM interaction row", slog.Any("error", err))
			return nil, fmt.Errorf("failed to scan LLM interaction row: %w", err)
		}

		var interactions []map[string]interface{}
		if err := json.Unmarshal([]byte(interactionsJSON), &interactions); err != nil {
			r.logger.WarnContext(ctx, "Failed to parse interactions JSON", slog.Any("error", err))
			continue
		}

		var conversationHistory []types.ConversationMessage
		var totalPOIs, totalHotels, totalRestaurants int
		var citiesCovered []string
		var hasItinerary bool
		var dominantCategories []string

		for _, interaction := range interactions {
			if prompt, ok := interaction["prompt"].(string); ok && prompt != "" {
				conversationHistory = append(conversationHistory, types.ConversationMessage{
					Role:      "user",
					Content:   prompt,
					Timestamp: parseTimeFromInterface(interaction["created_at"]),
				})
			}
			if response, ok := interaction["response"].(string); ok {
				if response == "" {
					response = fmt.Sprintf("I provided recommendations for %s", cityName)
				} else {
					// Count content items from response for metrics
					contentCounts := countContentFromResponse(response)
					totalPOIs += contentCounts.POIs
					totalHotels += contentCounts.Hotels
					totalRestaurants += contentCounts.Restaurants
					if contentCounts.HasItinerary {
						hasItinerary = true
					}
					dominantCategories = append(dominantCategories, contentCounts.Categories...)

					// Convert JSON response to human-readable format
					response = formatResponseForDisplay(response, cityName)
				}
				conversationHistory = append(conversationHistory, types.ConversationMessage{
					Role:      "assistant",
					Content:   response,
					Timestamp: parseTimeFromInterface(interaction["created_at"]),
				})
			}
		}

		// Calculate enriched metrics
		performanceMetrics := types.SessionPerformanceMetrics{
			AvgResponseTimeMs: int(avgLatencyMs.Int64),
			TotalTokens:       int(totalTokens.Int64),
			PromptTokens:      int(totalPromptTokens.Int64),
			CompletionTokens:  int(totalCompletionTokens.Int64),
			ModelsUsed:        modelsUsed,
			TotalLatencyMs:    int(totalLatencyMs.Int64),
		}

		// Calculate unique cities covered
		citiesMap := make(map[string]bool)
		citiesMap[cityName] = true
		for _, city := range citiesCovered {
			citiesMap[city] = true
		}
		uniqueCities := make([]string, 0, len(citiesMap))
		for city := range citiesMap {
			uniqueCities = append(uniqueCities, city)
		}

		// Calculate complexity score (1-10)
		complexityScore := calculateComplexityScore(totalPOIs, totalHotels, totalRestaurants, len(conversationHistory), hasItinerary)

		contentMetrics := types.SessionContentMetrics{
			TotalPOIs:          totalPOIs,
			TotalHotels:        totalHotels,
			TotalRestaurants:   totalRestaurants,
			CitiesCovered:      uniqueCities,
			HasItinerary:       hasItinerary,
			ComplexityScore:    complexityScore,
			DominantCategories: uniqueStringSlice(dominantCategories),
		}

		// Calculate engagement metrics
		userMsgCount, assistantMsgCount := countMessagesByRole(conversationHistory)
		conversationDuration := lastInteraction.Sub(firstInteraction)
		avgMsgLength := calculateAverageMessageLength(conversationHistory)
		engagementLevel := calculateEngagementLevel(len(conversationHistory), conversationDuration, complexityScore)

		engagementMetrics := types.SessionEngagementMetrics{
			MessageCount:          len(conversationHistory),
			ConversationDuration:  conversationDuration,
			UserMessageCount:      userMsgCount,
			AssistantMessageCount: assistantMsgCount,
			AvgMessageLength:      avgMsgLength,
			EngagementLevel:       engagementLevel,
		}

		sessionID := uuid.NewSHA1(uuid.NameSpaceOID, []byte(sessionKey))
		session := types.ChatSession{
			ID:                  sessionID,
			UserID:              userIDFromDB,
			CityName:            cityName,
			ConversationHistory: conversationHistory,
			CreatedAt:           firstInteraction,
			UpdatedAt:           lastInteraction,
			Status:              "active",
			PerformanceMetrics:  performanceMetrics,
			ContentMetrics:      contentMetrics,
			EngagementMetrics:   engagementMetrics,
		}
		sessions = append(sessions, session)
	}

	if err = rows.Err(); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Error iterating through LLM interaction rows")
		r.logger.ErrorContext(ctx, "Error iterating through LLM interaction rows", slog.Any("error", err))
		return nil, fmt.Errorf("error iterating through LLM interaction rows: %w", err)
	}

	span.SetAttributes(attribute.Int("sessions.count", len(sessions)))
	return sessions, nil
}

// Helper function to parse time from interface{}
func parseTimeFromInterface(timeInterface interface{}) time.Time {
switch t := timeInterface.(type) {
case time.Time:
return t
case string:
if parsed, err := time.Parse(time.RFC3339, t); err == nil {
return parsed
}
}
return time.Now()
}

// Helper function to format JSON response for human-readable display
func formatResponseForDisplay(response, cityName string) string {
// Handle responses with prefixed tags like [itinerary], [city_data], etc.
cleanedResponse := response

	// Remove common LLM response prefixes
	prefixPatterns := []string{
		`\[itinerary\]\s*`,
		`\[city_data\]\s*`,
		`\[restaurants\]\s*`,
		`\[hotels\]\s*`,
		`\[activities\]\s*`,
		`\[pois\]\s*`,
		`\[general_pois\]\s*`,
		`\[personalized_pois\]\s*`,
	}

	for _, pattern := range prefixPatterns {
		re := regexp.MustCompile(`(?i)^` + pattern)
		cleanedResponse = re.ReplaceAllString(cleanedResponse, "")
	}

	// Remove markdown code blocks if present
	cleanedResponse = regexp.MustCompile("(?s)```json\\s*(.*)\\s*```").ReplaceAllString(cleanedResponse, "$1")
	cleanedResponse = strings.TrimSpace(cleanedResponse)

	// First, check if cleaned response is valid JSON
	if !json.Valid([]byte(cleanedResponse)) {
		// If not JSON, return as-is (might be already formatted text)
		return response
	}

	// Try to parse as GeneralCityData first (for [city_data] responses)
	var generalCity types.GeneralCityData
	if err := json.Unmarshal([]byte(cleanedResponse), &generalCity); err == nil && generalCity.City != "" {
		return formatCityDataResponse(generalCity)
	}

	// Try to parse as AiCityResponse (most common format)
	var cityResponse types.AiCityResponse
	if err := json.Unmarshal([]byte(cleanedResponse), &cityResponse); err == nil {
		// Check if it's a valid itinerary response (either has POIs or itinerary data)
		if len(cityResponse.PointsOfInterest) > 0 || cityResponse.AIItineraryResponse.ItineraryName != "" || len(cityResponse.AIItineraryResponse.PointsOfInterest) > 0 {
			return formatItineraryResponse(cityResponse, cityName)
		}
	}

	// Try to parse as hotel array
	var hotels []types.HotelDetailedInfo
	if err := json.Unmarshal([]byte(cleanedResponse), &hotels); err == nil && len(hotels) > 0 {
		return formatHotelResponse(hotels, cityName)
	}

	// Try to parse as restaurant array
	var restaurants []types.RestaurantDetailedInfo
	if err := json.Unmarshal([]byte(cleanedResponse), &restaurants); err == nil && len(restaurants) > 0 {
		return formatRestaurantResponse(restaurants, cityName)
	}

	// Try to parse as POI array
	var pois []types.POIDetailedInfo
	if err := json.Unmarshal([]byte(cleanedResponse), &pois); err == nil && len(pois) > 0 {
		return formatPOIResponse(pois, cityName)
	}

	// Try to extract meaningful information from malformed JSON or text
	cleanedLower := strings.ToLower(cleanedResponse)

	// Check if it contains city information
	if strings.Contains(cleanedLower, "city") || strings.Contains(cleanedLower, "country") {
		return fmt.Sprintf("I found information about %s and prepared some details for you!", cityName)
	}

	// Check for content type indicators
	if strings.Contains(cleanedLower, "hotel") || strings.Contains(cleanedLower, "accommodation") {
		return fmt.Sprintf("I found some excellent hotel options in %s for you!", cityName)
	}

	if strings.Contains(cleanedLower, "restaurant") || strings.Contains(cleanedLower, "dining") {
		return fmt.Sprintf("I discovered some amazing restaurants in %s for you!", cityName)
	}

	if strings.Contains(cleanedLower, "poi") || strings.Contains(cleanedLower, "attraction") || strings.Contains(cleanedLower, "point") {
		return fmt.Sprintf("I found some exciting places to visit in %s for you!", cityName)
	}

	if strings.Contains(cleanedLower, "itinerary") || strings.Contains(cleanedLower, "plan") {
		return fmt.Sprintf("I created a personalized travel plan for %s!", cityName)
	}

	// If we can't determine the content type, return a generic message
	return fmt.Sprintf("I provided personalized recommendations for %s. Here are some great options I found for you!", cityName)
}

// Format itinerary response to readable text
func formatItineraryResponse(response types.AiCityResponse, cityName string) string {
// Determine which POI list to use and total count
var totalPOIs int
var firstPOIName string

	// Check both POI arrays and get the total count
	if len(response.PointsOfInterest) > 0 {
		totalPOIs += len(response.PointsOfInterest)
		firstPOIName = getFirstPOIName(response.PointsOfInterest)
	}

	if len(response.AIItineraryResponse.PointsOfInterest) > 0 {
		totalPOIs += len(response.AIItineraryResponse.PointsOfInterest)
		if firstPOIName == "" {
			firstPOIName = getFirstPOIName(response.AIItineraryResponse.PointsOfInterest)
		}
	}

	// If we have an itinerary name, use it
	if response.AIItineraryResponse.ItineraryName != "" {
		if totalPOIs > 0 {
			return fmt.Sprintf("I created a personalized itinerary called '%s' for %s with %d amazing places to visit, including %s and more!",
				response.AIItineraryResponse.ItineraryName,
				cityName,
				totalPOIs,
				firstPOIName)
		} else {
			return fmt.Sprintf("I created a personalized itinerary called '%s' for %s with great recommendations!",
				response.AIItineraryResponse.ItineraryName,
				cityName)
		}
	}

	// Fallback to generic response
	if totalPOIs > 0 {
		return fmt.Sprintf("I found %d great places to visit in %s, including %s. Perfect for your trip!",
			totalPOIs,
			cityName,
			firstPOIName)
	}

	return fmt.Sprintf("I provided personalized recommendations for %s. Here are some great options I found for you!", cityName)
}

// Format hotel response to readable text
func formatHotelResponse(hotels []types.HotelDetailedInfo, cityName string) string {
if len(hotels) == 0 {
return fmt.Sprintf("I searched for hotels in %s for you.", cityName)
}

	return fmt.Sprintf("I found %d excellent hotel%s in %s, including %s and other great options that match your preferences!",
		len(hotels),
		pluralize(len(hotels)),
		cityName,
		hotels[0].Name)
}

// Format restaurant response to readable text
func formatRestaurantResponse(restaurants []types.RestaurantDetailedInfo, cityName string) string {
if len(restaurants) == 0 {
return fmt.Sprintf("I searched for restaurants in %s for you.", cityName)
}

	return fmt.Sprintf("I discovered %d fantastic restaurant%s in %s, starting with %s and many more delicious options!",
		len(restaurants),
		pluralize(len(restaurants)),
		cityName,
		restaurants[0].Name)
}

// Format POI response to readable text
func formatPOIResponse(pois []types.POIDetailedInfo, cityName string) string {
if len(pois) == 0 {
return fmt.Sprintf("I searched for activities in %s for you.", cityName)
}

	return fmt.Sprintf("I found %d exciting place%s to visit in %s, including %s and other amazing spots you'll love!",
		len(pois),
		pluralize(len(pois)),
		cityName,
		pois[0].Name)
}

// Format city data response to readable text
func formatCityDataResponse(cityData types.GeneralCityData) string {
result := fmt.Sprintf("Let me tell you about %s, %s! ", cityData.City, cityData.Country)

	if cityData.Description != "" {
		result += cityData.Description + " "
	}

	// Add additional details if available
	details := make([]string, 0)

	if cityData.Population != "" {
		details = append(details, fmt.Sprintf("population of %s", cityData.Population))
	}

	if cityData.Weather != "" {
		details = append(details, fmt.Sprintf("weather: %s", cityData.Weather))
	}

	if cityData.Language != "" {
		details = append(details, fmt.Sprintf("language: %s", cityData.Language))
	}

	if len(details) > 0 {
		result += "Key details: " + strings.Join(details, ", ") + ". "
	}

	if cityData.Attractions != "" {
		result += fmt.Sprintf("Notable attractions include: %s. ", cityData.Attractions)
	}

	if cityData.History != "" {
		result += fmt.Sprintf("History: %s", cityData.History)
	}

	return strings.TrimSpace(result)
}

// Helper functions
func getFirstPOIName(pois []types.POIDetailedInfo) string {
if len(pois) > 0 {
return pois[0].Name
}
return "some amazing attractions"
}

func pluralize(count int) string {
if count == 1 {
return ""
}
return "s"
}

// UpdateSession updates an existing session
func (r *RepositoryImpl) UpdateSession(ctx context.Context, session types.ChatSession) error {
query := `
        UPDATE chat_sessions SET current_itinerary = $2, conversation_history = $3, session_context = $4,
                                 updated_at = $5, expires_at = $6, status = $7
        WHERE id = $1
    `
itineraryJSON, _ := json.Marshal(session.CurrentItinerary)
historyJSON, _ := json.Marshal(session.ConversationHistory)
contextJSON, _ := json.Marshal(session.SessionContext)

	_, err := r.pgpool.Exec(ctx, query, session.ID, itineraryJSON, historyJSON, contextJSON,
		session.UpdatedAt, session.ExpiresAt, session.Status)
	if err != nil {
		r.logger.ErrorContext(ctx, "Failed to update session", slog.Any("error", err))
		return fmt.Errorf("failed to update session: %w", err)
	}
	return nil
}

// AddMessageToSession appends a message to the session's conversation history
func (r *RepositoryImpl) AddMessageToSession(ctx context.Context, sessionID uuid.UUID, message types.ConversationMessage) error {
session, err := r.GetSession(ctx, sessionID)
if err != nil {
return err
}
session.ConversationHistory = append(session.ConversationHistory, message)
session.UpdatedAt = time.Now()
return r.UpdateSession(ctx, *session)
}

func (r *RepositoryImpl) SaveSinglePOI(ctx context.Context, poi types.POIDetailedInfo, userID, cityID, llmInteractionID uuid.UUID) (uuid.UUID, error) {
ctx, span := otel.Tracer("LlmInteractionRepo").Start(ctx, "SaveSinglePOI", trace.WithAttributes(
attribute.String("poi.name", poi.Name), /* ... */
))
defer span.End()

	// Validate coordinates before attempting to use them.
	if poi.Latitude < -90 || poi.Latitude > 90 || poi.Longitude < -180 || poi.Longitude > 180 {
		// Or if they are exactly 0,0 and that's considered invalid from LLM
		err := fmt.Errorf("invalid coordinates for POI %s: lat %f, lon %f", poi.Name, poi.Latitude, poi.Longitude)
		span.RecordError(err)
		return uuid.Nil, err
	}

	tx, err := r.pgpool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return uuid.Nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// If poi.ID is already set (e.g., from LLM or previous step), use it. Otherwise, generate new.
	recordID := poi.ID
	if recordID == uuid.Nil {
		recordID = uuid.New()
	}

	// Columns: id, user_id, city_id, llm_interaction_id, name, latitude, longitude, location, category, description_poi (10 columns)
	// Values: $1, $2, $3, $4, $5, $6, $7, ST_MakePoint($7, $6), $8, $9 (10 value expressions for 9 placeholders + ST_MakePoint)
	// Corrected: 9 distinct columns from poiData + 1 for location, then id for the record.
	// Order of columns in INSERT INTO: id, user_id, city_id, llm_interaction_id, name, latitude, longitude, location, category, description_poi
	// Placeholders:                $1,    $2,      $3,      $4,                 $5,   $6,       $7,        ST_MakePoint($7,$6), $8, $9
	query := `
        INSERT INTO llm_suggested_pois (
            id, user_id, city_id, llm_interaction_id, name, 
            latitude, longitude, "location", -- Ensure "location" is quoted if it's a reserved keyword or mixed case
            category, description_poi 
            -- Removed distance from INSERT list
        ) VALUES (
            $1, $2, $3, $4, $5, 
            $6, $7, ST_SetSRID(ST_MakePoint($7, $6), 4326), -- Longitude ($7) first, then Latitude ($6)
            $8, $9
        )
        RETURNING id
    `
	// Arguments should be:
	// $1: recordID (for llm_suggested_pois.id)
	// $2: userID
	// $3: cityID
	// $4: llmInteractionID
	// $5: poi.Name
	// $6: poi.Latitude  (for the latitude column)
	// $7: poi.Longitude (for the longitude column AND for ST_MakePoint's X)
	// $8: poi.Category
	// $9: poi.DescriptionPOI

	var returnedID uuid.UUID
	err = tx.QueryRow(ctx, query,
		recordID,         // $1: id
		userID,           // $2: user_id
		cityID,           // $3: city_id
		llmInteractionID, // $4: llm_interaction_id
		poi.Name,         // $5: name
		poi.Latitude,     // $6: latitude column value
		poi.Longitude,    // $7: longitude column value (also used as X in ST_MakePoint)
		// ST_MakePoint will use $7 (poi.Longitude) as X and $6 (poi.Latitude) as Y
		poi.Category,       // $8: category
		poi.DescriptionPOI, // $9: description_poi
	).Scan(&returnedID)

	if err != nil {
		r.logger.ErrorContext(ctx, "Failed to insert llm_suggested_poi", slog.Any("error", err), slog.String("query", query), slog.String("name", poi.Name))
		span.RecordError(err)
		return uuid.Nil, fmt.Errorf("failed to save llm_suggested_poi: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		span.RecordError(err)
		return uuid.Nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	r.logger.Info("LLM Suggested POI saved successfully", slog.String("id", returnedID.String()))
	return returnedID, nil
}

func (r *RepositoryImpl) GetPOIsBySessionSortedByDistance(ctx context.Context, sessionID, cityID uuid.UUID, userLocation types.UserLocation) ([]types.POIDetailedInfo, error) {

	query := `
        SELECT id, name, latitude, longitude, category, description_poi, 
               ST_Distance(
                   ST_SetSRID(ST_MakePoint($2, $3), 4326)::geography,
                   location::geography  -- Use the actual geometry column for distance
               ) AS distance
        FROM llm_suggested_pois  -- Assuming this is the correct table to query for session POIs
        WHERE city_id = $1 
        -- Add AND llm_interaction_id IN (SELECT ...) if POIs are tied to specific interactions of the session
        ORDER BY distance ASC;
    `
	rows, err := r.pgpool.Query(ctx, query, cityID, userLocation.UserLon, userLocation.UserLat)
	if err != nil {
		return nil, fmt.Errorf("failed to query POIs for session: %w", err)
	}
	defer rows.Close()

	var pois []types.POIDetailedInfo
	for rows.Next() {
		var p types.POIDetailedInfo
		var lat, lon, dist sql.NullFloat64 // Use nullable types
		var cat, desc sql.NullString

		// Adjust scan to match selected columns and their nullability
		err := rows.Scan(&p.ID, &p.Name, &lat, &lon, &cat, &desc, &dist)
		if err != nil {
			return nil, fmt.Errorf("failed to scan POI for session: %w", err)
		}

		if lat.Valid {
			p.Latitude = lat.Float64
		}
		if lon.Valid {
			p.Longitude = lon.Float64
		}
		if cat.Valid {
			p.Category = cat.String
		}
		if desc.Valid {
			p.DescriptionPOI = desc.String
		}
		if dist.Valid {
			p.Distance = dist.Float64
		}

		pois = append(pois, p)
	}
	return pois, rows.Err()
}

// type POIDetailedInfo struct {
// 	Name        string  `json:"name"`
// 	Latitude    float64 `json:"latitude"`
// 	Longitude   float64 `json:"longitude"`
// 	Category    string  `json:"category"`
// 	Description string  `json:"description"`
// }

// type LlmApiResponseData struct {
// 	GeneralCityData struct {
// 		City            string  `json:"city"`
// 		Country         string  `json:"country"`
// 		Description     string  `json:"description"`
// 		CenterLatitude  float64 `json:"center_latitude"`
// 		CenterLongitude float64 `json:"center_longitude"`
// 		// Add other fields from general_city_data if you need them
// 		// Population       string  `json:"population,omitempty"`
// 		// Area             string  `json:"area,omitempty"`
// 		// Timezone         string  `json:"timezone,omitempty"`
// 		// Language         string  `json:"language,omitempty"`
// 		// Weather          string  `json:"weather,omitempty"`
// 		// Attractions      string  `json:"attractions,omitempty"`
// 		// History          string  `json:"history,omitempty"`
// 	} `json:"general_city_data"`

// 	PointsOfInterest []types.POIDetailedInfo `json:"points_of_interest"` // <--- ADD THIS FIELD for general POIs

// 	ItineraryResponse struct {
// 		ItineraryName      string            `json:"itinerary_name"`
// 		OverallDescription string            `json:"overall_description"`
// 		PointsOfInterest   []types.POIDetailedInfo `json:"points_of_interest"` // This is for itinerary_response.points_of_interest
// 	} `json:"itinerary_response"`
// }

// type LlmApiResponse struct {
// 	SessionID string             `json:"session_id"` // Capture the top-level session_id
// 	Data      LlmApiResponseData `json:"data"`
// 	// Note: The JSON also has a "session_id" inside "data".
// 	// If you need that too, you'd add it to LlmApiResponseData:
// 	// SessionIDInsideData string `json:"session_id,omitempty"`
// }

func parsePOIsFromResponse(responseText string, logger *slog.Logger) ([]types.POIDetailedInfo, error) {
cleanedResponse := cleanJSONResponse(responseText)

	// Debug logging to see the actual cleaned response
	logger.Debug("parsePOIsFromResponse: Cleaned response debug",
		"originalLength", len(responseText),
		"cleanedLength", len(cleanedResponse),
		"cleanedPreview", cleanedResponse[:min(500, len(cleanedResponse))],
		"cleanedSuffix", func() string {
			start := len(cleanedResponse) - 200
			if start < 0 {
				start = 0
			}
			return cleanedResponse[start:]
		}())

	// Check if this looks like an itinerary response instead of a POI response
	if strings.Contains(cleanedResponse, "itinerary_name") {
		logger.Debug("parsePOIsFromResponse: Response appears to be an itinerary, not POI data")
		return []types.POIDetailedInfo{}, nil
	}

	// First try to parse as unified chat response format with "data" wrapper
	var unifiedResponse struct {
		Data types.AiCityResponse `json:"data"`
	}
	err := json.Unmarshal([]byte(cleanedResponse), &unifiedResponse)
	if err == nil {
		// Collect POIs from both general points_of_interest and itinerary points_of_interest
		var allPOIs []types.POIDetailedInfo
		if unifiedResponse.Data.PointsOfInterest != nil {
			allPOIs = append(allPOIs, unifiedResponse.Data.PointsOfInterest...)
		}
		if unifiedResponse.Data.AIItineraryResponse.PointsOfInterest != nil {
			allPOIs = append(allPOIs, unifiedResponse.Data.AIItineraryResponse.PointsOfInterest...)
		}
		if len(allPOIs) > 0 {
			logger.Debug("parsePOIsFromResponse: Parsed as unified chat response", "poiCount", len(allPOIs))
			return allPOIs, nil
		}
	} else if err != nil {
		logger.Debug("parsePOIsFromResponse: Failed to parse as unified response", "error", err.Error())
	}

	// Second, try to parse as a full AiCityResponse (for legacy responses)
	var parsedResponse types.AiCityResponse
	err = json.Unmarshal([]byte(cleanedResponse), &parsedResponse)
	if err == nil && parsedResponse.PointsOfInterest != nil {
		logger.Debug("parsePOIsFromResponse: Parsed as AiCityResponse", "poiCount", len(parsedResponse.PointsOfInterest))
		return parsedResponse.PointsOfInterest, nil
	} else if err != nil {
		logger.Debug("parsePOIsFromResponse: Failed to parse as AiCityResponse", "error", err.Error())
	}

	// Third, try to parse as a single POI (for individual POI additions)
	var singlePOI types.POIDetailedInfo
	err = json.Unmarshal([]byte(cleanedResponse), &singlePOI)
	if err == nil && singlePOI.Name != "" {
		logger.Debug("parsePOIsFromResponse: Parsed as single POI", "poiName", singlePOI.Name)
		return []types.POIDetailedInfo{singlePOI}, nil
	}

	// If all fail, log the error and return empty
	logger.Warn("parsePOIsFromResponse: Could not parse response as unified chat, AiCityResponse, or single POI",
		"error", err,
		"cleanedResponseLength", len(cleanedResponse),
		"responsePreview", cleanedResponse[:min(200, len(cleanedResponse))])
	return []types.POIDetailedInfo{}, nil
}

func (r *RepositoryImpl) GetOrCreatePOI(ctx context.Context, tx pgx.Tx, POIDetailedInfo types.POIDetailedInfo, cityID uuid.UUID, sourceInteractionID uuid.UUID) (uuid.UUID, error) {
var poiDBID uuid.UUID
findPoiQuery := `SELECT id FROM points_of_interest WHERE name = $1 AND city_id = $2 LIMIT 1`
err := tx.QueryRow(ctx, findPoiQuery, POIDetailedInfo.Name, cityID).Scan(&poiDBID)

	if err == pgx.ErrNoRows {
		createPoiQuery := `
            INSERT INTO points_of_interest (name, city_id, location, category, description)
            VALUES ($1, $2, ST_SetSRID(ST_MakePoint($3, $4), 4326), $5, $6) RETURNING id`
		err = tx.QueryRow(ctx, createPoiQuery,
			POIDetailedInfo.Name,
			cityID,
			POIDetailedInfo.Latitude,
			POIDetailedInfo.Longitude,
			POIDetailedInfo.Category,
			POIDetailedInfo.DescriptionPOI, // Assumes types.POIDetailedInfo has DescriptionPOI from JSON
		).Scan(&poiDBID)
		if err != nil {
			r.logger.ErrorContext(ctx, "GetOrCreatePOI: Failed to insert new POI", "error", err, "poi_name", POIDetailedInfo.Name)
			return uuid.Nil, fmt.Errorf("GetOrCreatePOI: failed to insert new POI '%s': %w", POIDetailedInfo.Name, err)
		}
	} else if err != nil {
		r.logger.ErrorContext(ctx, "GetOrCreatePOI: Failed to query existing POI", "error", err, "poi_name", POIDetailedInfo.Name)
		return uuid.Nil, fmt.Errorf("GetOrCreatePOI: failed to query existing POI '%s': %w", POIDetailedInfo.Name, err)
	}
	return poiDBID, nil
}

func (r *RepositoryImpl) SaveItineraryPOIs(ctx context.Context, itineraryID uuid.UUID, pois []types.POIDetailedInfo) error {
batch := &pgx.Batch{}
for i, poi := range pois {
query := `
            INSERT INTO itinerary_pois (itinerary_id, poi_id, order_index, ai_description)
            VALUES ($1, $2, $3, $4)
            ON CONFLICT (itinerary_id, poi_id) DO UPDATE SET
                order_index = EXCLUDED.order_index,
                ai_description = EXCLUDED.ai_description,
                updated_at = NOW()
        `
batch.Queue(query, itineraryID, poi.ID, i, poi.DescriptionPOI)
}

	br := r.pgpool.SendBatch(ctx, batch)
	defer br.Close()

	for i := 0; i < len(pois); i++ {
		_, err := br.Exec()
		if err != nil {
			return fmt.Errorf("failed to execute batch insert for itinerary_poi %d: %w", i, err)
		}
	}

	return nil
}

// func (r *RepositoryImpl) SaveInteractionWithEmbedding(ctx context.Context, interaction types.LlmInteraction, embedding []float32) (uuid.UUID, error) {
// 	ctx, span := otel.Tracer("LlmInteractionRepo").Start(ctx, "SaveInteractionWithEmbedding", trace.WithAttributes(
// 		semconv.DBSystemPostgreSQL,
// 		attribute.String("db.operation", "INSERT_COMPLEX"),
// 		attribute.String("db.sql.table", "llm_interactions,itineraries,itinerary_pois"),
// 		attribute.String("user.id", interaction.UserID.String()),
// 		attribute.String("model.used", interaction.ModelUsed),
// 		attribute.Int("latency.ms", interaction.LatencyMs),
// 		attribute.String("city.name", interaction.CityName),
// 	))
// 	defer span.End()

// 	var err error
// 	tx, err := r.pgpool.BeginTx(ctx, pgx.TxOptions{})
// 	if err != nil {
// 		span.RecordError(err)
// 		span.SetStatus(codes.Error, "Failed to start transaction")
// 		return uuid.Nil, fmt.Errorf("failed to start transaction: %w", err)
// 	}
// 	defer func() {
// 		if p := recover(); p != nil {
// 			_ = tx.Rollback(ctx)
// 			panic(p)
// 		}
// 		if err != nil {
// 			if rbErr := tx.Rollback(ctx); rbErr != nil {
// 				r.logger.ErrorContext(ctx, "Transaction rollback failed", slog.Any("error", rbErr))
// 			}
// 		}
// 	}()

// 	// Convert embedding to pgvector format
// 	vectorParam := pgvector.NewVector(embedding)

// 	interactionQuery := `
//         INSERT INTO llm_interactions (
//             user_id, prompt, response, model_name, latency_ms, city_name, prompt_embedding
//         ) VALUES ($1, $2, $3, $4, $5, $6, $7)
//         RETURNING id
//     `
// 	var interactionID uuid.UUID
// 	err = tx.QueryRow(ctx, interactionQuery,
// 		interaction.UserID,
// 		interaction.Prompt,
// 		interaction.ResponseText,
// 		interaction.ModelUsed,
// 		interaction.LatencyMs,
// 		interaction.CityName,
// 		vectorParam,
// 	).Scan(&interactionID)
// 	if err != nil {
// 		span.RecordError(err)
// 		span.SetStatus(codes.Error, "Failed to insert llm_interaction")
// 		return uuid.Nil, fmt.Errorf("failed to insert llm_interaction: %w", err)
// 	}
// 	span.SetAttributes(attribute.String("llm_interaction.id", interactionID.String()))

// 	// Existing itinerary and POI logic remains unchanged
// 	var cityID uuid.UUID
// 	if interaction.CityName != "" {
// 		cityQuery := `SELECT id FROM cities WHERE name = $1 LIMIT 1`
// 		err = tx.QueryRow(ctx, cityQuery, interaction.CityName).Scan(&cityID)
// 		if err != nil && err != pgx.ErrNoRows {
// 			span.RecordError(err)
// 			return interactionID, fmt.Errorf("failed to get city_id: %w", err)
// 		}
// 	}

// 	var itineraryID uuid.UUID
// 	if cityID != uuid.Nil {
// 		itineraryQuery := `
//             INSERT INTO itineraries (user_id, city_id, source_llm_interaction_id)
//             VALUES ($1, $2, $3)
//             ON CONFLICT (user_id, city_id) DO UPDATE SET
//                 updated_at = NOW(),
//                 source_llm_interaction_id = EXCLUDED.source_llm_interaction_id
//             RETURNING id
//         `
// 		err = tx.QueryRow(ctx, itineraryQuery, interaction.UserID, cityID, interactionID).Scan(&itineraryID)
// 		if err != nil {
// 			span.RecordError(err)
// 			return interactionID, fmt.Errorf("failed to insert itinerary: %w", err)
// 		}
// 	}

// 	if itineraryID != uuid.Nil {
// 		var pois []types.POIDetailedInfo
// 		pois, err = parsePOIsFromResponse(interaction.ResponseText, r.logger)
// 		if err != nil {
// 			span.RecordError(err)
// 			return interactionID, fmt.Errorf("failed to parse POIs: %w", err)
// 		}

// 		if len(pois) > 0 {
// 			poiBatch := &pgx.Batch{}
// 			itineraryPoiInsertQuery := `
//                 INSERT INTO itinerary_pois (itinerary_id, poi_id, order_index, ai_description)
//                 VALUES ($1, $2, $3, $4)
//                 ON CONFLICT (itinerary_id, poi_id) DO UPDATE SET
//                     order_index = EXCLUDED.order_index,
//                     ai_description = EXCLUDED.ai_description,
//                     updated_at = NOW()
//             `
// 			for i, POIDetailedInfo := range pois {
// 				var poiDBID uuid.UUID
// 				poiDBID, err = r.GetOrCreatePOI(ctx, tx, POIDetailedInfo, cityID, interactionID)
// 				if err != nil {
// 					span.RecordError(err)
// 					return interactionID, fmt.Errorf("failed to get or create POI: %w", err)
// 				}
// 				poiBatch.Queue(itineraryPoiInsertQuery, itineraryID, poiDBID, i, POIDetailedInfo.DescriptionPOI)
// 			}

// 			if poiBatch.Len() > 0 {
// 				br := tx.SendBatch(ctx, poiBatch)
// 				for i := 0; i < poiBatch.Len(); i++ {
// 					_, execErr := br.Exec()
// 					if execErr != nil {
// 						err = fmt.Errorf("failed to insert itinerary_poi: %w", execErr)
// 						br.Close()
// 						return interactionID, err
// 					}
// 				}
// 				br.Close()
// 			}
// 		}
// 	}

// 	err = tx.Commit(ctx)
// 	if err != nil {
// 		span.RecordError(err)
// 		return uuid.Nil, fmt.Errorf("failed to commit transaction: %w", err)
// 	}

// 	span.SetStatus(codes.Ok, "Interaction saved successfully")
// 	return interactionID, nil
// }

// ContentCounts represents counts of different content types found in responses
type ContentCounts struct {
POIs         int
Hotels       int
Restaurants  int
HasItinerary bool
Categories   []string
}

// countContentFromResponse analyzes a response to count different content types
func countContentFromResponse(response string) ContentCounts {
counts := ContentCounts{
Categories: make([]string, 0),
}

	// Try to parse as JSON first
	var jsonData map[string]interface{}
	if err := json.Unmarshal([]byte(response), &jsonData); err == nil {
		// Handle JSON response
		if pois, ok := jsonData["points_of_interest"].([]interface{}); ok {
			counts.POIs = len(pois)
			counts.Categories = append(counts.Categories, "attractions")
		}
		if hotels, ok := jsonData["hotels"].([]interface{}); ok {
			counts.Hotels = len(hotels)
			counts.Categories = append(counts.Categories, "accommodation")
		}
		if restaurants, ok := jsonData["restaurants"].([]interface{}); ok {
			counts.Restaurants = len(restaurants)
			counts.Categories = append(counts.Categories, "dining")
		}
		if _, ok := jsonData["itinerary_response"]; ok {
			counts.HasItinerary = true
			counts.Categories = append(counts.Categories, "itinerary")
		}
		if _, ok := jsonData["itinerary_name"]; ok {
			counts.HasItinerary = true
			counts.Categories = append(counts.Categories, "itinerary")
		}
	} else {
		// Handle text response with pattern matching
		lowerResponse := strings.ToLower(response)

		// Count mentions of different content types
		if strings.Contains(lowerResponse, "hotel") || strings.Contains(lowerResponse, "accommodation") {
			counts.Hotels = 1
			counts.Categories = append(counts.Categories, "accommodation")
		}
		if strings.Contains(lowerResponse, "restaurant") || strings.Contains(lowerResponse, "dining") {
			counts.Restaurants = 1
			counts.Categories = append(counts.Categories, "dining")
		}
		if strings.Contains(lowerResponse, "attraction") || strings.Contains(lowerResponse, "visit") || strings.Contains(lowerResponse, "see") {
			counts.POIs = 1
			counts.Categories = append(counts.Categories, "attractions")
		}
		if strings.Contains(lowerResponse, "itinerary") || strings.Contains(lowerResponse, "plan") || strings.Contains(lowerResponse, "schedule") {
			counts.HasItinerary = true
			counts.Categories = append(counts.Categories, "itinerary")
		}
	}

	return counts
}

// calculateComplexityScore calculates a complexity score from 1-10 based on session content
func calculateComplexityScore(pois, hotels, restaurants, messageCount int, hasItinerary bool) int {
score := 1

	// Base score from content count
	totalContent := pois + hotels + restaurants
	if totalContent > 20 {
		score += 3
	} else if totalContent > 10 {
		score += 2
	} else if totalContent > 5 {
		score += 1
	}

	// Bonus for having itinerary
	if hasItinerary {
		score += 2
	}

	// Bonus for message count (engagement)
	if messageCount > 20 {
		score += 2
	} else if messageCount > 10 {
		score += 1
	}

	// Bonus for content diversity
	contentTypes := 0
	if pois > 0 {
		contentTypes++
	}
	if hotels > 0 {
		contentTypes++
	}
	if restaurants > 0 {
		contentTypes++
	}
	if contentTypes >= 3 {
		score += 2
	} else if contentTypes >= 2 {
		score += 1
	}

	// Cap at 10
	if score > 10 {
		score = 10
	}

	return score
}

// countMessagesByRole counts messages by user and assistant roles
func countMessagesByRole(messages []types.ConversationMessage) (userCount, assistantCount int) {
for _, msg := range messages {
if msg.Role == "user" {
userCount++
} else if msg.Role == "assistant" {
assistantCount++
}
}
return
}

// calculateAverageMessageLength calculates the average length of all messages
func calculateAverageMessageLength(messages []types.ConversationMessage) int {
if len(messages) == 0 {
return 0
}

	totalLength := 0
	for _, msg := range messages {
		totalLength += len(msg.Content)
	}

	return totalLength / len(messages)
}

// calculateEngagementLevel determines engagement level based on metrics
func calculateEngagementLevel(messageCount int, duration time.Duration, complexityScore int) string {
score := 0

	// Message count factor
	if messageCount > 15 {
		score += 3
	} else if messageCount > 8 {
		score += 2
	} else if messageCount > 3 {
		score += 1
	}

	// Duration factor (more than 10 minutes indicates engagement)
	if duration > 30*time.Minute {
		score += 3
	} else if duration > 10*time.Minute {
		score += 2
	} else if duration > 2*time.Minute {
		score += 1
	}

	// Complexity factor
	if complexityScore >= 8 {
		score += 2
	} else if complexityScore >= 5 {
		score += 1
	}

	// Determine level
	if score >= 6 {
		return "high"
	} else if score >= 3 {
		return "medium"
	}
	return "low"
}

// uniqueStringSlice removes duplicates from a string slice
func uniqueStringSlice(slice []string) []string {
unique := make(map[string]bool)
result := make([]string, 0)

	for _, item := range slice {
		if !unique[item] && item != "" {
			unique[item] = true
			result = append(result, item)
		}
	}

	return result
}


package llmChat

import (
"context"
"crypto/md5"
"database/sql"
"encoding/hex"
"encoding/json"
"fmt"
"log"
"log/slog"
"net/url"
"regexp"
"strings"
"sync"
"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/google/uuid"
	"github.com/patrickmn/go-cache"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/genai"

	"github.com/FACorreiaa/go-poi-au-suggestions/internal/api/city"
	generativeAI "github.com/FACorreiaa/go-poi-au-suggestions/internal/api/generative_ai"
	"github.com/FACorreiaa/go-poi-au-suggestions/internal/api/interests"
	"github.com/FACorreiaa/go-poi-au-suggestions/internal/api/poi"
	"github.com/FACorreiaa/go-poi-au-suggestions/internal/api/profiles"
	"github.com/FACorreiaa/go-poi-au-suggestions/internal/api/tags"
	"github.com/FACorreiaa/go-poi-au-suggestions/internal/types"
)

const (
model              = "gemini-2.0-flash"
defaultTemperature = 0.5
)

type ChatSession struct {
History []genai.Chat
}

// Mutex for thread-safe access

// Ensure implementation satisfies the interface
var _ LlmInteractiontService = (*ServiceImpl)(nil)

// LlmInteractiontService defines the business logic contract for user operations.
type LlmInteractiontService interface {
SaveItenerary(ctx context.Context, userID uuid.UUID, req types.BookmarkRequest) (uuid.UUID, error)
RemoveItenerary(ctx context.Context, userID, itineraryID uuid.UUID) error
GetPOIDetailedInfosResponse(ctx context.Context, userID uuid.UUID, city string, lat, lon float64) (*types.POIDetailedInfo, error)

	ContinueSessionStreamed(
		ctx context.Context,
		sessionID uuid.UUID,
		message string,
		userLocation *types.UserLocation, // For distance sorting context
		eventCh chan<- types.StreamEvent, // Channel to send events back
	) error

	ProcessUnifiedChatMessageStream(ctx context.Context, userID, profileID uuid.UUID, cityName, message string, userLocation *types.UserLocation, eventCh chan<- types.StreamEvent) error
	ProcessUnifiedChatMessageStreamFree(ctx context.Context, cityName, message string, userLocation *types.UserLocation, eventCh chan<- types.StreamEvent) error

	// Chat session management
	GetUserChatSessions(ctx context.Context, userID uuid.UUID) ([]types.ChatSession, error)
}

type IntentClassifier interface {
Classify(ctx context.Context, message string) (types.IntentType, error) // e.g., "start_trip", "modify_itinerary"
}

// ServiceImpl provides the implementation for LlmInteractiontService.
type ServiceImpl struct {
logger             *slog.Logger
interestRepo       interests.Repository
searchProfileRepo  profiles.Repository
searchProfileSvc   profiles.Service // Add service for enhanced methods
tagsRepo           tags.Repository
aiClient           *generativeAI.AIClient
embeddingService   *generativeAI.EmbeddingService
ragService         *generativeAI.RAGService
llmInteractionRepo Repository
cityRepo           city.Repository
poiRepo            poi.Repository
cache              *cache.Cache

	// events
	deadLetterCh     chan types.StreamEvent
	intentClassifier IntentClassifier
}

// NewLlmInteractiontService creates a new user service instance.
func NewLlmInteractiontService(interestRepo interests.Repository,
searchProfileRepo profiles.Repository,
searchProfileSvc profiles.Service,
tagsRepo tags.Repository,
llmInteractionRepo Repository,
cityRepo city.Repository,
poiRepo poi.Repository,
logger *slog.Logger) *ServiceImpl {
ctx := context.Background()
aiClient, _ := generativeAI.NewAIClient(ctx)

	// Initialize embedding service
	embeddingService, err := generativeAI.NewEmbeddingService(ctx, logger)
	if err != nil {
		log.Fatalf("Failed to create embedding service: %v", err) // Terminate if initialization fails
	}

	// Initialize RAG service
	ragService, err := generativeAI.NewRAGService(ctx, logger)
	if err != nil {
		log.Fatalf("Failed to create RAG service: %v", err) // Terminate if initialization fails
	}

	cache := cache.New(24*time.Hour, 1*time.Hour) // Cache for 24 hours with cleanup every hour
	service := &ServiceImpl{
		logger:             logger,
		tagsRepo:           tagsRepo,
		interestRepo:       interestRepo,
		searchProfileRepo:  searchProfileRepo,
		searchProfileSvc:   searchProfileSvc,
		aiClient:           aiClient,
		embeddingService:   embeddingService,
		ragService:         ragService,
		llmInteractionRepo: llmInteractionRepo,
		cityRepo:           cityRepo,
		poiRepo:            poiRepo,
		cache:              cache,
		deadLetterCh:       make(chan types.StreamEvent, 100),
		intentClassifier:   &types.SimpleIntentClassifier{},
	}
	go service.processDeadLetterQueue()
	return service
}

// getPersonalizedPOIWithSemanticContext creates an enhanced prompt with semantic POI context
func (l *ServiceImpl) getPersonalizedPOIWithSemanticContext(interestNames []string, cityName, tagsPromptPart, userPrefs string, semanticPOIs []types.POIDetailedInfo) string {
prompt := fmt.Sprintf(`
Generate a personalized trip itinerary for %s, tailored to user interests [%s].

        **SEMANTIC CONTEXT - Consider these highly relevant POIs found via semantic search:**
        `, cityName, strings.Join(interestNames, ", "))

	// Add semantic POI context
	if len(semanticPOIs) > 0 {
		prompt += "\n**Contextually Relevant POIs:**\n"
		for i, poi := range semanticPOIs {
			if i >= 10 { // Limit context to avoid token overuse
				break
			}
			prompt += fmt.Sprintf("- %s (%s): %s [Lat: %.6f, Lon: %.6f]\n",
				poi.Name, poi.Category, poi.DescriptionPOI, poi.Latitude, poi.Longitude)
		}
		prompt += "\n**Instructions:** Use these semantic matches as inspiration and context. You may include them directly or use them to find similar places. Ensure variety and avoid exact duplicates.\n\n"
	}

	prompt += `Include:
        1. An itinerary name that reflects both user interests and semantic context.
        2. An overall description highlighting semantic relevance.
        3. A list of points of interest with name, category, coordinates, and detailed description.
        Max points of interest allowed by tokens.

        **PRIORITIZATION:**
        - Highly weight POIs that align with the semantic context provided
        - Ensure semantic relevance in descriptions
        - Balance popular attractions with personalized semantic matches
        - Include variety across different categories while maintaining semantic coherence

        Format the response in JSON with the following structure:
        {
            "itinerary_name": "Name of the itinerary (reflecting semantic context)",
            "overall_description": "Description emphasizing semantic relevance to user interests",
            "points_of_interest": [
                {
                    "name": "POI name",
                    "latitude": latitude_as_number,
                    "longitude": longitude_as_number,
                    "category": "Category",
                    "description_poi": "Detailed description explaining semantic relevance to user interests and why this matches their preferences"
                }
            ]
        }`

	if tagsPromptPart != "" {
		prompt += "\n**User Tags Context:** " + tagsPromptPart
	}
	if userPrefs != "" {
		prompt += "\n**User Preferences:** " + userPrefs
	}

	return prompt
}

func (l *ServiceImpl) FetchUserData(ctx context.Context, userID, profileID uuid.UUID) (interests []*types.Interest, searchProfile *types.UserPreferenceProfileResponse, tags []*types.Tags, err error) {
interests, err = l.interestRepo.GetInterestsForProfile(ctx, profileID)
if err != nil {
return nil, nil, nil, fmt.Errorf("failed to fetch user interests: %w", err)
}
searchProfile, err = l.searchProfileRepo.GetSearchProfile(ctx, userID, profileID)
if err != nil {
return nil, nil, nil, fmt.Errorf("failed to fetch search profile: %w", err)
}
tags, err = l.tagsRepo.GetTagsForProfile(ctx, profileID)
if err != nil {
return nil, nil, nil, fmt.Errorf("failed to fetch user tags: %w", err)
}
return interests, searchProfile, tags, nil
}

func (l *ServiceImpl) PreparePromptData(interests []*types.Interest, tags []*types.Tags, searchProfile *types.UserPreferenceProfileResponse) (interestNames []string, tagsPromptPart string, userPrefs string) {
if len(interests) == 0 {
interestNames = []string{"general sightseeing", "local experiences"}
} else {
for _, interest := range interests {
if interest != nil {
interestNames = append(interestNames, interest.Name)
}
}
}
var tagInfoForPrompt []string
for _, tag := range tags {
if tag != nil {
tagDetail := tag.Name
if tag.Description != nil && *tag.Description != "" {
tagDetail += fmt.Sprintf(" (meaning: %s)", *tag.Description)
}
tagInfoForPrompt = append(tagInfoForPrompt, tagDetail)
}
}
if len(tagInfoForPrompt) > 0 {
tagsPromptPart = fmt.Sprintf("\n    - Additionally, consider these specific user tags/preferences: [%s].", strings.Join(tagInfoForPrompt, "; "))
}
userPrefs = getUserPreferencesPrompt(searchProfile)
return interestNames, tagsPromptPart, userPrefs
}

func (l *ServiceImpl) CollectResults(resultCh <-chan types.GenAIResponse) (itinerary types.AiCityResponse, llmInteractionID uuid.UUID, rawPersonalisedPOIs []types.POIDetailedInfo, errors []error) {
for res := range resultCh {
if res.Err != nil {
errors = append(errors, res.Err)
continue
}
if res.City != "" {
itinerary.GeneralCityData.City = res.City
itinerary.GeneralCityData.Country = res.Country
itinerary.GeneralCityData.Description = res.CityDescription
itinerary.GeneralCityData.StateProvince = res.StateProvince
itinerary.GeneralCityData.CenterLatitude = res.Latitude
itinerary.GeneralCityData.CenterLongitude = res.Longitude
}
if res.ItineraryName != "" {
itinerary.AIItineraryResponse.ItineraryName = res.ItineraryName
itinerary.AIItineraryResponse.OverallDescription = res.ItineraryDescription
}
if len(res.GeneralPOI) > 0 {
itinerary.PointsOfInterest = res.GeneralPOI
}
if len(res.PersonalisedPOI) > 0 {
itinerary.AIItineraryResponse.PointsOfInterest = res.PersonalisedPOI
rawPersonalisedPOIs = res.PersonalisedPOI
llmInteractionID = res.LlmInteractionID
}
}
return itinerary, llmInteractionID, rawPersonalisedPOIs, errors
}

func (l *ServiceImpl) HandleCityData(ctx context.Context, cityData types.GeneralCityData) (cityID uuid.UUID, err error) {
city, err := l.cityRepo.FindCityByNameAndCountry(ctx, cityData.City, cityData.Country)
if err != nil {
return uuid.Nil, fmt.Errorf("failed to check city existence: %w", err)
}
if city == nil {
cityDetail := types.CityDetail{
Name:            cityData.City,
Country:         cityData.Country,
StateProvince:   cityData.StateProvince,
AiSummary:       cityData.Description,
CenterLatitude:  cityData.CenterLatitude,
CenterLongitude: cityData.CenterLongitude,
}
cityID, err = l.cityRepo.SaveCity(ctx, cityDetail)
if err != nil {
return uuid.Nil, fmt.Errorf("failed to save city: %w", err)
}
} else {
cityID = city.ID
}
return cityID, nil
}

func (l *ServiceImpl) HandleGeneralPOIs(ctx context.Context, pois []types.POIDetailedInfo, cityID uuid.UUID) {
for _, poi := range pois {
existingPoi, err := l.poiRepo.FindPoiByNameAndCity(ctx, poi.Name, cityID)
if err != nil {
l.logger.WarnContext(ctx, "Failed to check POI existence", slog.String("poi_name", poi.Name), slog.Any("error", err))
continue
}
if existingPoi == nil {
_, err = l.poiRepo.SavePoi(ctx, poi, cityID)
if err != nil {
l.logger.WarnContext(ctx, "Failed to save POI", slog.String("poi_name", poi.Name), slog.Any("error", err))
}
}
}
}

func (l *ServiceImpl) HandlePersonalisedPOIs(ctx context.Context, pois []types.POIDetailedInfo, cityID uuid.UUID, userLocation *types.UserLocation, llmInteractionID uuid.UUID, userID, profileID uuid.UUID) ([]types.POIDetailedInfo, error) {
if userLocation == nil || cityID == uuid.Nil || len(pois) == 0 {
return pois, nil // No sorting possible
}
err := l.llmInteractionRepo.SaveLlmSuggestedPOIsBatch(ctx, pois, userID, profileID, llmInteractionID, cityID)
if err != nil {
return nil, fmt.Errorf("failed to save personalised POIs: %w", err)
}

	itineraryID, err := l.poiRepo.SaveItinerary(ctx, userID, cityID)
	if err != nil {
		return nil, fmt.Errorf("failed to save itinerary: %w", err)
	}

	if err := l.poiRepo.SaveItineraryPOIs(ctx, itineraryID, pois); err != nil {
		return nil, fmt.Errorf("failed to save itinerary POIs: %w", err)
	}

	sortedPois, err := l.llmInteractionRepo.GetLlmSuggestedPOIsByInteractionSortedByDistance(ctx, llmInteractionID, cityID, *userLocation)
	if err != nil {
		l.logger.ErrorContext(ctx, "Failed to fetch sorted POIs", slog.Any("error", err))
		return pois, nil // Return unsorted POIs
	}
	return sortedPois, nil
}

// GenerateEnhancedPersonalisedPOIWorker generates personalized POIs with domain-aware filtering
func (l *ServiceImpl) GenerateEnhancedPersonalisedPOIWorker(wg *sync.WaitGroup, ctx context.Context,
cityName string, userID, profileID uuid.UUID, resultCh chan<- types.GenAIResponse,
enhancedPromptData string, domain types.DomainType,
config *genai.GenerateContentConfig) {
ctx, span := otel.Tracer("LlmInteractionService").Start(ctx, "GenerateEnhancedPersonalisedPOIWorker", trace.WithAttributes(
attribute.String("city.name", cityName),
attribute.String("user.id", userID.String()),
attribute.String("profile.id", profileID.String()),
attribute.String("domain", string(domain)),
))
defer span.End()
defer wg.Done()

	startTime := time.Now()

	// Create enhanced prompt based on domain
	prompt := l.getEnhancedPersonalizedPOIPrompt(cityName, enhancedPromptData, domain)
	span.SetAttributes(attribute.Int("prompt.length", len(prompt)))

	response, err := l.aiClient.GenerateResponse(ctx, prompt, config)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "AI generation failed")
		resultCh <- types.GenAIResponse{Err: fmt.Errorf("failed to generate enhanced personalized POIs: %w", err)}
		return
	}

	duration := time.Since(startTime)
	span.SetAttributes(attribute.Int64("generation.duration_ms", duration.Milliseconds()))

	var txt string
	for _, candidate := range response.Candidates {
		if candidate.Content != nil && len(candidate.Content.Parts) > 0 {
			txt = candidate.Content.Parts[0].Text
			break
		}
	}
	if txt == "" {
		err := fmt.Errorf("no valid enhanced personalized POI content from AI")
		span.RecordError(err)
		span.SetStatus(codes.Error, "Empty response from AI")
		resultCh <- types.GenAIResponse{Err: err}
		return
	}
	span.SetAttributes(attribute.Int("response.length", len(txt)))

	cleanTxt := cleanJSONResponse(txt)
	var itineraryData types.AIItineraryResponse
	if err := json.Unmarshal([]byte(cleanTxt), &itineraryData); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to parse enhanced personalized POI JSON")
		resultCh <- types.GenAIResponse{Err: fmt.Errorf("failed to parse enhanced personalized POI JSON: %w", err)}
		return
	}

	span.SetAttributes(attribute.Int("pois.count", len(itineraryData.PointsOfInterest)))
	span.SetStatus(codes.Ok, "Enhanced personalized POIs generated successfully")
	resultCh <- types.GenAIResponse{
		ItineraryName:        itineraryData.ItineraryName,
		ItineraryDescription: itineraryData.OverallDescription,
		PersonalisedPOI:      itineraryData.PointsOfInterest,
		LlmInteractionID:     uuid.New(), // Generate a new LLM interaction ID
	}
}

// getEnhancedPersonalizedPOIPrompt creates a domain-aware prompt for personalized POI generation
func (l *ServiceImpl) getEnhancedPersonalizedPOIPrompt(cityName, enhancedPromptData string, domain types.DomainType) string {
domainFocus := ""
switch domain {
case types.DomainAccommodation:
domainFocus = "Focus particularly on accommodation recommendations and nearby attractions that complement the user's accommodation preferences."
case types.DomainDining:
domainFocus = "Focus particularly on restaurant, food, and dining experiences that align with the user's culinary preferences."
case types.DomainActivities:
domainFocus = "Focus particularly on activities, attractions, and experiences that match the user's activity preferences and physical capabilities."
case types.DomainItinerary:
domainFocus = "Focus particularly on creating a well-structured itinerary that respects the user's planning style and pace preferences."
default:
domainFocus = "Provide a balanced mix of attractions, dining, and activities based on all user preferences."
}

	prompt := fmt.Sprintf(`You are a travel AI assistant creating a personalized itinerary for %s.

User Preferences and Filters:
%s

Domain Focus: %s

%s

Create a comprehensive and personalized itinerary that heavily weighs the user's specific preferences and filters. Ensure that every recommendation aligns with their stated preferences.

Format the response in JSON with the following structure:
{
"itinerary_name": "Personalized itinerary name reflecting user preferences",
"overall_description": "Description emphasizing how this itinerary matches user preferences",
"points_of_interest": [
{
"name": "POI name",
"category": "Category",
"coordinates": {
"latitude": float64,
"longitude": float64
},
"description": "Detailed description explaining why this POI matches the user's specific preferences and filters"
}
]
}`, cityName, enhancedPromptData, domainFocus, getBasePersonalizedPromptInstructions())

	return prompt
}

func getBasePersonalizedPromptInstructions() string {
return `
**Instructions:**
- Prioritize POIs that directly align with user preferences and filters
- Explain in descriptions how each POI matches their specific preferences
- Ensure variety while maintaining preference alignment
- Include practical details like accessibility if relevant to user preferences
- Consider user's pace and planning style preferences in the selection
- Maximum 8-10 POIs to maintain quality over quantity`
  }

func TruncateString(str string, num int) string {
if len(str) > num {
return str[0:num] + "..."
}
return str
}

func (l *ServiceImpl) SaveItenerary(ctx context.Context, userID uuid.UUID, req types.BookmarkRequest) (uuid.UUID, error) {
var llmInteractionIDStr string
if req.LlmInteractionID != nil {
llmInteractionIDStr = req.LlmInteractionID.String()
} else {
llmInteractionIDStr = "nil"
}

	ctx, span := otel.Tracer("LlmInteractionService").Start(ctx, "SaveItenerary", trace.WithAttributes(
		attribute.String("user.id", userID.String()),
		attribute.String("llm_interaction.id", llmInteractionIDStr),
		attribute.String("title", req.Title),
	))
	defer span.End()

	l.logger.InfoContext(ctx, "Attempting to bookmark interaction",
		slog.String("userID", userID.String()),
		slog.String("llmInteractionID", llmInteractionIDStr),
		slog.String("title", req.Title))

	var sourceInteractionID pgtype.UUID
	if req.LlmInteractionID != nil {
		sourceInteractionID = pgtype.UUID{
			Bytes: *req.LlmInteractionID,
			Valid: true,
		}
	} else {
		sourceInteractionID = pgtype.UUID{Valid: false} // Explicitly invalid for NULL
	}

	// Prepare primaryCityID
	var primaryCityID pgtype.UUID
	if req.PrimaryCityID != nil {
		primaryCityID = pgtype.UUID{
			Bytes: *req.PrimaryCityID,
			Valid: true,
		}
	} else {
		primaryCityID = pgtype.UUID{Valid: false} // Explicitly invalid for NULL
	}

	// Fetch original interaction only if LlmInteractionID is provided
	var originalInteraction *types.LlmInteraction
	var err error
	if req.LlmInteractionID != nil {
		originalInteraction, err = l.llmInteractionRepo.GetInteractionByID(ctx, *req.LlmInteractionID)
		if err != nil || originalInteraction == nil {
			l.logger.ErrorContext(ctx, "Failed to fetch original LLM interaction", slog.Any("error", err))
			span.RecordError(err)
			return uuid.Nil, fmt.Errorf("could not retrieve original interaction: %w", err)
		}
	}

	// Prepare and save to user_saved_itineraries
	var markdownContent string
	if originalInteraction != nil {
		markdownContent = originalInteraction.ResponseText
	} else {
		if req.Description != nil {
			markdownContent = *req.Description
		} else {
			markdownContent = ""
		}
	}

	var description sql.NullString
	if req.Description != nil {
		description.String = *req.Description
		description.Valid = true
	}
	isPublic := false
	if req.IsPublic != nil {
		isPublic = *req.IsPublic
	}

	newBookmark := &types.UserSavedItinerary{
		UserID:                 userID,
		SourceLlmInteractionID: sourceInteractionID, // Will be nil if not provided
		PrimaryCityID:          primaryCityID,
		Title:                  req.Title,
		Description:            description,
		MarkdownContent:        markdownContent,
		Tags:                   req.Tags,
		IsPublic:               isPublic,
	}
	savedID, err := l.llmInteractionRepo.AddChatToBookmark(ctx, newBookmark)
	if err != nil {
		span.RecordError(err)
		return uuid.Nil, err
	}

	// Save to itineraries
	itineraryID, err := l.poiRepo.SaveItinerary(ctx, userID, primaryCityID.Bytes)
	if err != nil {
		l.logger.WarnContext(ctx, "Failed to save to itineraries", slog.Any("error", err))
		span.RecordError(err)
		return savedID, nil
	}

	// Fetch POIs from llm_suggested_pois only if we have an interaction ID
	if req.LlmInteractionID != nil {
		pois, err := l.llmInteractionRepo.GetLlmSuggestedPOIsByInteractionSortedByDistance(ctx, *req.LlmInteractionID, primaryCityID.Bytes, types.UserLocation{})
		if err != nil {
			l.logger.WarnContext(ctx, "Failed to fetch suggested POIs", slog.Any("error", err))
			span.RecordError(err)
			return savedID, nil
		}

		if len(pois) > 0 {
			l.logger.InfoContext(ctx, "Found POIs to process", slog.Int("count", len(pois)))

			for i := range pois {
				pois[i].CityID = primaryCityID.Bytes
			}

			if err := l.poiRepo.SaveItineraryPOIs(ctx, itineraryID, pois); err != nil {
				l.logger.WarnContext(ctx, "Failed to save to itinerary_pois", slog.Any("error", err))
				span.RecordError(err)
				return savedID, nil
			}
		}
	}

	l.logger.InfoContext(ctx, "Successfully saved itinerary",
		slog.String("savedItineraryID", savedID.String()),
		slog.String("itineraryID", itineraryID.String()))
	span.SetAttributes(attribute.String("itinerary.id", itineraryID.String()))
	span.SetStatus(codes.Ok, "Itinerary saved successfully")
	return savedID, nil
}

func (l *ServiceImpl) RemoveItenerary(ctx context.Context, userID, itineraryID uuid.UUID) error {
ctx, span := otel.Tracer("LlmInteractionService").Start(ctx, "RemoveItenerary", trace.WithAttributes(
attribute.String("user.id", userID.String()),
attribute.String("itinerary.id", itineraryID.String()),
))
defer span.End()

	l.logger.InfoContext(ctx, "Attempting to remove chat from bookmark",
		slog.String("itineraryID", itineraryID.String()))

	if err := l.llmInteractionRepo.RemoveChatFromBookmark(ctx, userID, itineraryID); err != nil {
		l.logger.ErrorContext(ctx, "Failed to remove chat from bookmark", slog.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to remove chat from bookmark")
		return fmt.Errorf("failed to remove chat from bookmark: %w", err)
	}

	l.logger.InfoContext(ctx, "Successfully removed chat from bookmark", slog.String("itineraryID", itineraryID.String()))
	span.SetStatus(codes.Ok, "Itinerary removed successfully")
	return nil
}

// GetUserChatSessions retrieves all chat sessions for a user
func (l *ServiceImpl) GetUserChatSessions(ctx context.Context, userID uuid.UUID) ([]types.ChatSession, error) {
ctx, span := otel.Tracer("LlmInteractionService").Start(ctx, "GetUserChatSessions", trace.WithAttributes(
attribute.String("user.id", userID.String()),
))
defer span.End()

	l.logger.InfoContext(ctx, "Retrieving chat sessions for user",
		slog.String("userID", userID.String()))

	sessions, err := l.llmInteractionRepo.GetUserChatSessions(ctx, userID)
	if err != nil {
		l.logger.ErrorContext(ctx, "Failed to get user chat sessions", slog.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to get user chat sessions")
		return nil, fmt.Errorf("failed to get user chat sessions: %w", err)
	}

	l.logger.InfoContext(ctx, "Successfully retrieved chat sessions",
		slog.String("userID", userID.String()),
		slog.Int("sessionCount", len(sessions)))
	span.SetAttributes(attribute.Int("sessions.count", len(sessions)))
	span.SetStatus(codes.Ok, "Chat sessions retrieved successfully")
	return sessions, nil
}

// getPOIDetailedInfos returns a formatted string with POI details.
func (l *ServiceImpl) getPOIDetailedInfos(wg *sync.WaitGroup, ctx context.Context,
city string, lat float64, lon float64, userID uuid.UUID,
resultCh chan<- types.POIDetailedInfo, config *genai.GenerateContentConfig) {
defer wg.Done()
ctx, span := otel.Tracer("LlmInteractionService").Start(ctx, "getPOIDetailedInfos", trace.WithAttributes(
attribute.String("city.name", city),
attribute.Float64("latitude", lat),
attribute.Float64("longitude", lon),
))
defer span.End()

	if city == "" || lat == 0 || lon == 0 {
		return
	}

	startTime := time.Now()

	prompt := getPOIDetailsPrompt(city, lat, lon)
	span.SetAttributes(attribute.Int("prompt.length", len(prompt)))
	response, err := l.aiClient.GenerateResponse(ctx, prompt, config)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to generate POI details")
		resultCh <- types.POIDetailedInfo{Err: fmt.Errorf("failed to generate POI details: %w", err)}
		return
	}

	var txt string
	for _, candidate := range response.Candidates {
		if candidate.Content != nil && len(candidate.Content.Parts) > 0 {
			txt = candidate.Content.Parts[0].Text
			break
		}
	}
	if txt == "" {
		err := fmt.Errorf("no valid POI details content from AI")
		span.RecordError(err)
		span.SetStatus(codes.Error, "Empty response from AI")
		resultCh <- types.POIDetailedInfo{Err: err}
		return
	}

	span.SetAttributes(attribute.Int("response.length", len(txt)))
	cleanTxt := cleanJSONResponse(txt)
	var detailedInfo types.POIDetailedInfo
	if err := json.Unmarshal([]byte(cleanTxt), &detailedInfo); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to parse POI details JSON")
		resultCh <- types.POIDetailedInfo{Err: fmt.Errorf("failed to parse POI details JSON: %w", err)}
		return
	}
	latencyMs := int(time.Since(startTime).Milliseconds())
	span.SetAttributes(attribute.Int("response.latency_ms", latencyMs))
	span.SetStatus(codes.Ok, "POI details generated successfully")
	interaction := types.LlmInteraction{
		UserID:       userID,
		Prompt:       prompt,
		ResponseText: txt,
		ModelUsed:    model, // Adjust based on your AI client
		LatencyMs:    latencyMs,
		CityName:     city,
		// request payload
		// response payload
		// Add token counts if available from response (depends on genai API)
		// PromptTokens, CompletionTokens, TotalTokens
		// RequestPayload, ResponsePayload if you serialize the full request/response
	}

	savedInteractionID, err := l.llmInteractionRepo.SaveInteraction(ctx, interaction)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to save LLM interaction for POI details")
		resultCh <- types.POIDetailedInfo{Err: fmt.Errorf("failed to save LLM interaction for POI details: %w", err)}
		return
	}
	resultCh <- types.POIDetailedInfo{
		City:         city,
		Name:         detailedInfo.Name,
		Latitude:     detailedInfo.Latitude,
		Longitude:    detailedInfo.Longitude,
		Description:  detailedInfo.Description,
		Address:      detailedInfo.Address,
		OpeningHours: detailedInfo.OpeningHours,
		PhoneNumber:  detailedInfo.PhoneNumber,
		Website:      detailedInfo.Website,
		Rating:       detailedInfo.Rating,
		Tags:         detailedInfo.Tags,
		Images:       detailedInfo.Images,
		PriceRange:   detailedInfo.PriceRange,
		Err:          nil,
		// Include the saved interaction ID for tracking

		LlmInteractionID: savedInteractionID,
	}
	span.SetAttributes(attribute.String("llm_interaction.id", savedInteractionID.String()))
	span.SetStatus(codes.Ok, "POI details generated and saved successfully")
}

func (l *ServiceImpl) GetPOIDetailedInfosResponse(ctx context.Context, userID uuid.UUID, city string, lat, lon float64) (*types.POIDetailedInfo, error) {
ctx, span := otel.Tracer("LlmInteractionService").Start(ctx, "GetPOIDetailedInfosResponse", trace.WithAttributes(
attribute.String("city.name", city),
attribute.Float64("latitude", lat),
attribute.Float64("longitude", lon),
attribute.String("user.id", userID.String()),
))
defer span.End()

	l.logger.DebugContext(ctx, "Starting POI details generation",
		slog.String("city", city), slog.Float64("latitude", lat), slog.Float64("longitude", lon), slog.String("userID", userID.String()))

	// Generate cache key
	cacheKey := generatePOICacheKey(city, lat, lon, 0.0, userID)
	span.SetAttributes(attribute.String("cache.key", cacheKey))

	// Check cache
	if cached, found := l.cache.Get(cacheKey); found {
		if poi, ok := cached.(*types.POIDetailedInfo); ok {
			l.logger.InfoContext(ctx, "Cache hit for POI details", slog.String("cache_key", cacheKey))
			span.AddEvent("Cache hit")
			span.SetStatus(codes.Ok, "POI details served from cache")
			return poi, nil
		}
	}

	// Find city ID
	cityData, err := l.cityRepo.FindCityByNameAndCountry(ctx, city, "") // Adjust country if needed
	if err != nil {
		l.logger.ErrorContext(ctx, "Failed to find city", slog.Any("error", err))
		span.RecordError(err)
		return nil, fmt.Errorf("failed to find city: %w", err)
	}
	if cityData == nil {
		l.logger.WarnContext(ctx, "City not found", slog.String("city", city))
		span.SetStatus(codes.Error, "City not found")
		return nil, fmt.Errorf("city %s not found", city)
	}
	cityID := cityData.ID

	// Check database
	poi, err := l.poiRepo.FindPOIDetails(ctx, cityID, lat, lon, 100.0) // 100m tolerance
	if err != nil {
		l.logger.ErrorContext(ctx, "Failed to query POI details from database", slog.Any("error", err))
		span.RecordError(err)
		return nil, fmt.Errorf("failed to query POI details: %w", err)
	}
	if poi != nil {
		poi.City = city
		l.cache.Set(cacheKey, poi, cache.DefaultExpiration)
		l.logger.InfoContext(ctx, "Database hit for POI details", slog.String("cache_key", cacheKey))
		span.AddEvent("Database hit")
		span.SetStatus(codes.Ok, "POI details served from database")
		return poi, nil
	}

	// Cache and database miss: fetch from Gemini API
	l.logger.DebugContext(ctx, "Cache and database miss, fetching POI details from AI", slog.String("cache_key", cacheKey))
	span.AddEvent("Cache and database miss")

	resultCh := make(chan types.POIDetailedInfo, 1)
	var wg sync.WaitGroup
	wg.Add(1)

	go l.getPOIDetailedInfos(&wg, ctx, city, lat, lon, userID, resultCh, &genai.GenerateContentConfig{Temperature: genai.Ptr[float32](defaultTemperature)})

	go func() {
		wg.Wait()
		close(resultCh)
	}()

	var poiResult *types.POIDetailedInfo
	for res := range resultCh {
		if res.Err != nil {
			l.logger.ErrorContext(ctx, "Error generating POI details", slog.Any("error", res.Err))
			span.RecordError(res.Err)
			span.SetStatus(codes.Error, "Failed to generate POI details")
			return nil, res.Err
		}
		poiResult = &res
		break
	}

	if poiResult == nil {
		l.logger.WarnContext(ctx, "No response received for POI details")
		span.SetStatus(codes.Error, "No response received")
		return nil, fmt.Errorf("no response received for POI details")
	}

	// Save to database
	_, err = l.poiRepo.SavePoi(ctx, *poiResult, cityID)
	if err != nil {
		l.logger.WarnContext(ctx, "Failed to save POI details to database", slog.Any("error", err))
		span.RecordError(err)
		// Continue despite error to avoid blocking user
	}

	// Store in cache
	l.cache.Set(cacheKey, poiResult, cache.DefaultExpiration)
	l.logger.DebugContext(ctx, "Stored POI details in cache", slog.String("cache_key", cacheKey))
	span.AddEvent("Stored in cache")

	span.SetStatus(codes.Ok, "POI details generated and cached successfully")
	return poiResult, nil
}

// generatePOIData queries the LLM for POI details and calculates distance using PostGIS
func (l *ServiceImpl) generatePOIData(ctx context.Context, poiName, cityName string, userLocation *types.UserLocation, userID, cityID uuid.UUID) (types.POIDetailedInfo, error) {
ctx, span := otel.Tracer("LlmInteractionService").Start(ctx, "GeneratePOIData", trace.WithAttributes(
attribute.String("poi.name", poiName),
attribute.String("city.name", cityName),
))
defer span.End()

	// Create a prompt for the LLM
	prompt := generatedContinuedConversationPrompt(poiName, cityName)

	// Generate LLM response
	response, err := l.aiClient.GenerateContent(ctx, prompt, nil)
	if err != nil {
		span.RecordError(err)
		return types.POIDetailedInfo{}, fmt.Errorf("failed to generate POI data: %w", err)
	}

	interaction := types.LlmInteraction{
		UserID:       userID,
		Prompt:       prompt,
		ResponseText: response,
		ModelUsed:    model,
		CityName:     cityName,
	}
	savedLlmInteractionID, err := l.llmInteractionRepo.SaveInteraction(ctx, interaction)
	if err != nil {
		l.logger.ErrorContext(ctx, "Failed to save LLM interaction in generatePOIData", slog.Any("error", err))
		// Decide if this is fatal for POI generation. It might be if FK is NOT NULL.
		return types.POIDetailedInfo{}, fmt.Errorf("failed to save LLM interaction: %w", err)
	}
	span.SetAttributes(attribute.String("llm.interaction_id.for_poi_data", savedLlmInteractionID.String()))

	cleanResponse := cleanJSONResponse(response)
	var poiData types.POIDetailedInfo
	if err := json.Unmarshal([]byte(cleanResponse), &poiData); err != nil || poiData.Name == "" {
		l.logger.WarnContext(ctx, "LLM returned invalid or empty POI data",
			slog.String("poiName", poiName),
			slog.String("llmResponse", response),
			slog.Any("unmarshalError", err))
		span.AddEvent("Invalid LLM response")
		poiData = types.POIDetailedInfo{
			ID:             uuid.New(),
			Name:           poiName,
			Latitude:       0,
			Longitude:      0,
			Category:       "Attraction",
			DescriptionPOI: fmt.Sprintf("Added %s based on user request, but detailed data not available.", poiName),
			Distance:       0,
		}
	}
	if poiData.ID == uuid.Nil { // Assign an ID if LLM didn't provide one
		poiData.ID = uuid.New()
	}
	poiData.LlmInteractionID = savedLlmInteractionID

	// Calculate distance if coordinates are valid
	if userLocation != nil && userLocation.UserLat != 0 && userLocation.UserLon != 0 && poiData.Latitude != 0 && poiData.Longitude != 0 {
		distance, err := l.poiRepo.CalculateDistancePostGIS(ctx, userLocation.UserLat, userLocation.UserLon, poiData.Latitude, poiData.Longitude)
		if err != nil {
			l.logger.WarnContext(ctx, "Failed to calculate distance", slog.Any("error", err))
			span.RecordError(err)
			poiData.Distance = 0
		} else {
			poiData.Distance = distance
			span.SetAttributes(attribute.Float64("poi.distance_meters", distance))
			l.logger.DebugContext(ctx, "Calculated distance for POI",
				slog.String("poiName", poiName),
				slog.Float64("distance_meters", distance))
		}
	} else {
		poiData.Distance = 0
		span.AddEvent("Distance not calculated due to missing location data")
		l.logger.WarnContext(ctx, "Cannot calculate distance",
			slog.Bool("userLocationAvailable", userLocation != nil),
			slog.Float64("userLat", userLocation.UserLat),
			slog.Float64("userLon", userLocation.UserLon),
			slog.Float64("poiLatitude", poiData.Latitude),
			slog.Float64("poiLongitude", poiData.Longitude))
	}

	// Save POI to database
	llmInteractionID := uuid.New()
	_, err = l.llmInteractionRepo.SaveSinglePOI(ctx, poiData, userID, cityID, savedLlmInteractionID)
	if err != nil {
		l.logger.WarnContext(ctx, "Failed to save POI to database", slog.Any("error", err))
		span.RecordError(err)
	}

	span.SetAttributes(
		attribute.String("poi.name", poiData.Name),
		attribute.Float64("poi.latitude", poiData.Latitude),
		attribute.Float64("poi.longitude", poiData.Longitude),
		attribute.String("poi.category", poiData.Category),
		attribute.String("llm_interaction.id", llmInteractionID.String()),
	)
	return poiData, nil
}

// enhancePOIRecommendationsWithSemantics uses embeddings to find similar POIs and enrich recommendations
func (l *ServiceImpl) enhancePOIRecommendationsWithSemantics(ctx context.Context, userMessage string, cityID uuid.UUID, userPreferences []string, limit int) ([]types.POIDetailedInfo, error) {
ctx, span := otel.Tracer("LlmInteractionService").Start(ctx, "enhancePOIRecommendationsWithSemantics", trace.WithAttributes(
attribute.String("user.message", userMessage),
attribute.String("city.id", cityID.String()),
attribute.Int("limit", limit),
))
defer span.End()

	l.logger.DebugContext(ctx, "Enhancing POI recommendations with semantic search",
		slog.String("message", userMessage),
		slog.String("city_id", cityID.String()))

	if l.embeddingService == nil {
		l.logger.WarnContext(ctx, "Embedding service not available, falling back to traditional search")
		span.AddEvent("Embedding service not available")
		return []types.POIDetailedInfo{}, nil
	}

	// Generate embedding for user message combined with preferences
	searchQuery := userMessage
	if len(userPreferences) > 0 {
		searchQuery += " " + strings.Join(userPreferences, " ")
	}

	queryEmbedding, err := l.embeddingService.GenerateQueryEmbedding(ctx, searchQuery)
	if err != nil {
		l.logger.ErrorContext(ctx, "Failed to generate query embedding",
			slog.Any("error", err),
			slog.String("query", searchQuery))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to generate query embedding")
		return []types.POIDetailedInfo{}, fmt.Errorf("failed to generate query embedding: %w", err)
	}

	// Search for similar POIs in the city
	similarPOIs, err := l.poiRepo.FindSimilarPOIsByCity(ctx, queryEmbedding, cityID, limit)
	if err != nil {
		l.logger.ErrorContext(ctx, "Failed to find similar POIs", slog.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to find similar POIs")
		return []types.POIDetailedInfo{}, fmt.Errorf("failed to find similar POIs: %w", err)
	}

	l.logger.InfoContext(ctx, "Found semantically similar POIs",
		slog.Int("count", len(similarPOIs)),
		slog.String("city_id", cityID.String()))
	span.SetAttributes(
		attribute.Int("similar_pois.count", len(similarPOIs)),
		attribute.String("search.query", searchQuery),
	)
	span.SetStatus(codes.Ok, "Semantic POI recommendations enhanced")

	return similarPOIs, nil
}

// generateSemanticPOIRecommendations generates POI recommendations using semantic search
func (l *ServiceImpl) generateSemanticPOIRecommendations(ctx context.Context, userMessage string, cityID uuid.UUID, userID uuid.UUID, userLocation *types.UserLocation, semanticWeight float64) ([]types.POIDetailedInfo, error) {
ctx, span := otel.Tracer("LlmInteractionService").Start(ctx, "generateSemanticPOIRecommendations", trace.WithAttributes(
attribute.String("user.message", userMessage),
attribute.String("city.id", cityID.String()),
attribute.String("user.id", userID.String()),
attribute.Float64("semantic.weight", semanticWeight),
))
defer span.End()

	l.logger.DebugContext(ctx, "Generating semantic POI recommendations",
		slog.String("message", userMessage),
		slog.String("city_id", cityID.String()),
		slog.Float64("semantic_weight", semanticWeight))

	if l.embeddingService == nil {
		err := fmt.Errorf("embedding service not available")
		l.logger.ErrorContext(ctx, "Embedding service not available", slog.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Embedding service not available")
		return nil, err
	}

	// Generate embedding for user message
	queryEmbedding, err := l.embeddingService.GenerateQueryEmbedding(ctx, userMessage)
	if err != nil {
		l.logger.ErrorContext(ctx, "Failed to generate query embedding", slog.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to generate query embedding")
		return nil, fmt.Errorf("failed to generate query embedding: %w", err)
	}

	var pois []types.POIDetailedInfo

	// If user location is available, use hybrid search (spatial + semantic)
	if userLocation != nil && userLocation.UserLat != 0 && userLocation.UserLon != 0 {
		filter := types.POIFilter{
			Location: types.GeoPoint{
				Latitude:  userLocation.UserLat,
				Longitude: userLocation.UserLon,
			},
			Radius: userLocation.SearchRadiusKm,
		}

		hybridPOIs, err := l.poiRepo.SearchPOIsHybrid(ctx, filter, queryEmbedding, semanticWeight)
		if err != nil {
			l.logger.ErrorContext(ctx, "Failed to perform hybrid search", slog.Any("error", err))
			span.RecordError(err)
			// Fall back to semantic-only search
		} else {
			pois = hybridPOIs
			l.logger.InfoContext(ctx, "Used hybrid search for POI recommendations",
				slog.Int("poi_count", len(pois)))
			span.AddEvent("Used hybrid search")
		}
	}

	// If hybrid search failed or no location available, use semantic-only search
	if len(pois) == 0 {
		semanticPOIs, err := l.poiRepo.FindSimilarPOIsByCity(ctx, queryEmbedding, cityID, 10)
		if err != nil {
			l.logger.ErrorContext(ctx, "Failed to find similar POIs", slog.Any("error", err))
			span.RecordError(err)
			span.SetStatus(codes.Error, "Failed to find similar POIs")
			return nil, fmt.Errorf("failed to find similar POIs: %w", err)
		}
		pois = semanticPOIs
		l.logger.InfoContext(ctx, "Used semantic-only search for POI recommendations",
			slog.Int("poi_count", len(pois)))
		span.AddEvent("Used semantic-only search")
	}

	// Generate embeddings for new POIs if needed
	for i, poi := range pois {
		if poi.ID == uuid.Nil {
			continue
		}

		// Generate embedding for this POI if it doesn't have one
		embedding, err := l.embeddingService.GeneratePOIEmbedding(ctx, poi.Name, poi.DescriptionPOI, poi.Category)
		if err != nil {
			l.logger.WarnContext(ctx, "Failed to generate embedding for POI",
				slog.Any("error", err),
				slog.String("poi_name", poi.Name))
			continue
		}

		// Update POI with embedding
		err = l.poiRepo.UpdatePOIEmbedding(ctx, poi.ID, embedding)
		if err != nil {
			l.logger.WarnContext(ctx, "Failed to update POI embedding",
				slog.Any("error", err),
				slog.String("poi_id", poi.ID.String()))
		}

		pois[i] = poi
	}

	l.logger.InfoContext(ctx, "Generated semantic POI recommendations",
		slog.String("message", userMessage),
		slog.Int("recommendations", len(pois)))
	span.SetAttributes(
		attribute.String("search.query", userMessage),
		attribute.Int("recommendations.count", len(pois)),
		attribute.Float64("semantic.weight", semanticWeight),
	)
	span.SetStatus(codes.Ok, "Semantic POI recommendations generated")

	return pois, nil
}

// handleSemanticRemovePOI handles removing POIs with semantic understanding
func (l *ServiceImpl) handleSemanticRemovePOI(ctx context.Context, message string, session *types.ChatSession) string {
ctx, span := otel.Tracer("LlmInteractionService").Start(ctx, "handleSemanticRemovePOI")
defer span.End()

	poiName := extractPOIName(message)
	if poiName == "" {
		return "I'd be happy to remove a POI from your itinerary! Could you please specify which place you'd like to remove?"
	}

	// Use semantic matching for removal - be more flexible with name matching
	for i, poi := range session.CurrentItinerary.AIItineraryResponse.PointsOfInterest {
		// Check for exact match or semantic similarity
		if strings.EqualFold(poi.Name, poiName) ||
			strings.Contains(strings.ToLower(poi.Name), strings.ToLower(poiName)) ||
			strings.Contains(strings.ToLower(poiName), strings.ToLower(poi.Name)) {

			removedName := poi.Name
			session.CurrentItinerary.AIItineraryResponse.PointsOfInterest = append(
				session.CurrentItinerary.AIItineraryResponse.PointsOfInterest[:i],
				session.CurrentItinerary.AIItineraryResponse.PointsOfInterest[i+1:]...,
			)
			l.logger.InfoContext(ctx, "Removed POI from itinerary",
				slog.String("removed_poi", removedName))
			span.SetAttributes(attribute.String("removed_poi", removedName))
			return fmt.Sprintf("I've removed %s from your itinerary.", removedName)
		}
	}

	return fmt.Sprintf("I couldn't find %s in your itinerary. Here's what you currently have: %s",
		poiName, strings.Join(func() []string {
			var names []string
			for _, poi := range session.CurrentItinerary.AIItineraryResponse.PointsOfInterest {
				names = append(names, poi.Name)
			}
			return names
		}(), ", "))
}

// min helper function
func min(a, b int) int {
if a < b {
return a
}
return b
}

// extractCityFromMessage uses AI to extract city name and clean the message
func (l *ServiceImpl) extractCityFromMessage(ctx context.Context, message string) (cityName, cleanedMessage string, err error) {
prompt := fmt.Sprintf(`
You are a text parser. Extract the city name from the user's travel request and return a clean version of the message.

User message: "%s"

Respond with ONLY a JSON object in this exact format:
{
"city": "City Name",
"message": "cleaned message without city"
}

Examples:
- "Find restaurants in Barcelona" → {"city": "Barcelona", "message": "Find restaurants"}
- "What to do in Paris?" → {"city": "Paris", "message": "What to do"}
- "Barcelona restaurants" → {"city": "Barcelona", "message": "restaurants"}
- "Show me hotels in New York" → {"city": "New York", "message": "Show me hotels"}
- "Things to do Madrid" → {"city": "Madrid", "message": "Things to do"}

If no city is mentioned, use empty string for city.
`, message)

	response, err := l.aiClient.GenerateResponse(ctx, prompt, &genai.GenerateContentConfig{
		Temperature: genai.Ptr[float32](0.1), // Low temperature for consistent parsing
	})
	if err != nil {
		return "", "", fmt.Errorf("failed to parse message: %w", err)
	}

	var responseText string
	for _, cand := range response.Candidates {
		if cand.Content != nil {
			for _, part := range cand.Content.Parts {
				if part.Text != "" {
					responseText += string(part.Text)
				}
			}
		}
	}

	if responseText == "" {
		return "", "", fmt.Errorf("empty response from AI parser")
	}

	cleanResponse := cleanJSONResponse(responseText)
	var parsed struct {
		City    string `json:"city"`
		Message string `json:"message"`
	}

	if err := json.Unmarshal([]byte(cleanResponse), &parsed); err != nil {
		return "", "", fmt.Errorf("failed to parse extraction response: %w", err)
	}

	// If no city extracted, return original message
	if parsed.City == "" {
		return "", message, nil
	}

	return parsed.City, parsed.Message, nil
}

// extractTextFromResponse extracts text from the AI response
func extractTextFromResponse(resp *genai.GenerateContentResponse) string {
var txt string
for _, candidate := range resp.Candidates {
if candidate.Content != nil && len(candidate.Content.Parts) > 0 {
txt = candidate.Content.Parts[0].Text
break
}
}
return txt
}

// assignIDs assigns UUIDs and interaction IDs to response items
func assignIDs(response interface{}, interactionID uuid.UUID) {
switch r := response.(type) {
case types.AiCityResponse:
for i := range r.PointsOfInterest {
r.PointsOfInterest[i].ID = uuid.New()
r.PointsOfInterest[i].LlmInteractionID = interactionID
}
for i := range r.AIItineraryResponse.PointsOfInterest {
r.AIItineraryResponse.PointsOfInterest[i].ID = uuid.New()
r.AIItineraryResponse.PointsOfInterest[i].LlmInteractionID = interactionID
}
case struct {
Hotels []types.HotelDetailedInfo `json:"hotels"`
}:
for i := range r.Hotels {
r.Hotels[i].ID = uuid.New()
r.Hotels[i].LlmInteractionID = interactionID
}
case struct {
Restaurants []types.RestaurantDetailedInfo `json:"restaurants"`
}:
for i := range r.Restaurants {
r.Restaurants[i].ID = uuid.New()
r.Restaurants[i].LlmInteractionID = interactionID
}
case struct {
Activities []types.POIDetailedInfo `json:"activities"`
}:
for i := range r.Activities {
r.Activities[i].ID = uuid.New()
r.Activities[i].LlmInteractionID = interactionID
}
}
}

// TODO For robustness, send unprocessed events to a dead letter queue (e.g., a separate channel or database table) for later analysis:
// if !l.sendEvent(ctx, eventCh, event) {
//     l.logger.ErrorContext(ctx, "Sending to dead letter queue", slog.Any("event", event))
//     // Save to a persistent store
// }

func (l *ServiceImpl) sendEvent(ctx context.Context, ch chan<- types.StreamEvent, event types.StreamEvent, retries int) bool {
for i := 0; i < retries; i++ {
if event.EventID == "" {
event.EventID = uuid.New().String()
}
if event.Timestamp.IsZero() {
event.Timestamp = time.Now()
}

		select {
		case <-ctx.Done():
			l.logger.WarnContext(ctx, "Context cancelled, not sending stream event", slog.String("eventType", event.Type))
			l.deadLetterCh <- event // Send to dead letter queue
			return false
		default:
			select {
			case ch <- event:
				return true
			case <-ctx.Done():
				l.logger.WarnContext(ctx, "Context cancelled while trying to send stream event", slog.String("eventType", event.Type))
				l.deadLetterCh <- event // Send to dead letter queue
				return false
			case <-time.After(2 * time.Second): // Use a reasonable timeout
				l.logger.WarnContext(ctx, "Dropped stream event due to slow consumer or blocked channel (timeout)", slog.String("eventType", event.Type))
				l.deadLetterCh <- event // Send to dead letter queue
				return false
			}
		}
		time.Sleep(100 * time.Millisecond) // Backoff
	}
	return false
}

func (l *ServiceImpl) processDeadLetterQueue() {
for event := range l.deadLetterCh {
l.logger.ErrorContext(context.Background(), "Unprocessed event sent to dead letter queue", slog.Any("event", event))
// TODO Save events to DB
}
}

// getPersonalizedPOI generates a prompt for personalized POIs
func getPersonalizedPOI(interestNames []string, cityName, tagsPromptPart, userPrefs string) string {
prompt := fmt.Sprintf(`
        Generate a personalized trip itinerary for %s, tailored to user interests [%s]. Include:
        1. An itinerary name.
        2. An overall description.
        3. A list of points of interest with name, category, coordinates, and detailed description.
		Max points of interest allowed by tokens.
        Format the response in JSON with the following structure:
        {
            "itinerary_name": "Name of the itinerary",
            "overall_description": "Description of the itinerary",
            "points_of_interest": [
                {
                    "name": "POI name",
                    "category": "Category",
                    "coordinates": {
                        "latitude": float64,
                        "longitude": float64
                    },
                    "description": "Detailed description of why this POI matches the user's interests"
                }
            ]
        }
    `, cityName, strings.Join(interestNames, ", "))
if tagsPromptPart != "" {
prompt += "\n" + tagsPromptPart
}
if userPrefs != "" {
prompt += "\n" + userPrefs
}
return prompt
}

// ContinueSessionStreamed handles subsequent messages in an existing session and streams responses/updates.
func (l *ServiceImpl) ContinueSessionStreamed(
ctx context.Context, sessionID uuid.UUID,
message string, userLocation *types.UserLocation,
eventCh chan<- types.StreamEvent, // Output channel for events
) error { // Only returns error for critical setup failures
ctx, span := otel.Tracer("LlmInteractionService").Start(ctx, "ContinueSessionStreamed", trace.WithAttributes(
attribute.String("session.id", sessionID.String()),
attribute.String("message", message),
))
defer span.End()

	l.logger.DebugContext(ctx, "Continuing streamed chat session", slog.String("sessionID", sessionID.String()), slog.String("message", message))

	// --- 1. Fetch Session & Basic Validation ---
	session, err := l.llmInteractionRepo.GetSession(ctx, sessionID)
	if err != nil {
		err = fmt.Errorf("failed to get session %s: %w", sessionID, err)
		l.sendEvent(ctx, eventCh, types.StreamEvent{Type: types.EventTypeError, Error: err.Error(), IsFinal: true}, 3)
		return err
	}
	if session.Status != types.StatusActive {
		err = fmt.Errorf("session %s is not active (status: %s) %w", sessionID, session.Status, err)
		l.sendEvent(ctx, eventCh, types.StreamEvent{Type: types.EventTypeError, Error: err.Error(), IsFinal: true}, 3)
		return err
	}
	l.sendEvent(ctx, eventCh, types.StreamEvent{Type: "session_validated", Data: map[string]string{"status": "active"}}, 3)

	// --- 2. Fetch City ID ---
	cityData, err := l.cityRepo.FindCityByNameAndCountry(ctx, session.SessionContext.CityName, "")
	if err != nil || cityData == nil {
		// If the city is not found, try a fuzzy match
		cityData, err = l.cityRepo.FindCityByFuzzyName(ctx, session.SessionContext.CityName)
		if err != nil || cityData == nil {
			if err == nil {
				err = fmt.Errorf("city '%s' not found for session %s %w", session.SessionContext.CityName, sessionID, err)
			} else {
				err = fmt.Errorf("failed to find city '%s' for session %s: %w", session.SessionContext.CityName, sessionID, err)
			}
			l.sendEvent(ctx, eventCh, types.StreamEvent{Type: types.EventTypeError, Error: err.Error(), IsFinal: true}, 3)
			return err
		}
	}
	cityID := cityData.ID
	l.sendEvent(ctx, eventCh, types.StreamEvent{Type: types.EventTypeProgress, Data: map[string]interface{}{"status": "context_loaded", "city_id": cityID.String()}}, 3)

	// --- 3. Add User Message to History ---
	userMessage := types.ConversationMessage{
		ID: uuid.New(), Role: types.RoleUser, Content: message, Timestamp: time.Now(), MessageType: types.TypeModificationRequest,
	}
	if err := l.llmInteractionRepo.AddMessageToSession(ctx, sessionID, userMessage); err != nil {
		l.logger.WarnContext(ctx, "Failed to persist user message, continuing with in-memory history", slog.Any("error", err))
		span.RecordError(err, trace.WithAttributes(attribute.String("warning", "User message DB save failed")))
	}
	session.ConversationHistory = append(session.ConversationHistory, userMessage)

	// --- 4. Classify Intent ---
	intent, err := l.intentClassifier.Classify(ctx, message)
	if err != nil {
		err = fmt.Errorf("failed to classify intent for message '%s': %w", message, err)
		l.sendEvent(ctx, eventCh, types.StreamEvent{Type: types.EventTypeError, Error: err.Error(), IsFinal: true}, 3)
		return err
	}
	l.logger.InfoContext(ctx, "Intent classified", slog.String("intent", string(intent)))
	l.sendEvent(ctx, eventCh, types.StreamEvent{Type: "intent_classified", Data: map[string]string{"intent": string(intent)}}, 3)

	// --- 5. Enhance with Semantic POI Recommendations ---
	l.sendEvent(ctx, eventCh, types.StreamEvent{
		Type: types.EventTypeProgress,
		Data: map[string]interface{}{"status": "generating_semantic_context", "progress": 20},
	}, 3)

	semanticPOIs, err := l.generateSemanticPOIRecommendations(ctx, message, cityID, session.UserID, userLocation, 0.6)
	if err != nil {
		l.logger.WarnContext(ctx, "Failed to generate semantic POI recommendations for streaming session", slog.Any("error", err))
		l.sendEvent(ctx, eventCh, types.StreamEvent{
			Type: types.EventTypeProgress,
			Data: map[string]interface{}{"status": "semantic_context_failed", "progress": 22},
		}, 3)
	} else {
		l.logger.InfoContext(ctx, "Generated semantic POI recommendations for streaming session",
			slog.Int("semantic_recommendations", len(semanticPOIs)))
		l.sendEvent(ctx, eventCh, types.StreamEvent{
			Type: "semantic_context_generated",
			Data: map[string]interface{}{
				"status":                         "semantic_context_ready",
				"semantic_recommendations_count": len(semanticPOIs),
				"progress":                       25,
			},
		}, 3)
	}

	// --- 5. Handle Intent and Generate Response ---
	var finalResponseMessage string
	var assistantMessageType types.MessageType = types.TypeResponse
	itineraryModifiedByThisTurn := false

	switch intent { // Align with ContinueSession's string-based intents
	case types.IntentAddPOI:
		l.sendEvent(ctx, eventCh, types.StreamEvent{Type: types.EventTypeProgress, Data: "Processing: Adding Point of Interest with semantic enhancement..."}, 3)
		var genErr error
		finalResponseMessage, genErr = l.handleSemanticAddPOIStreamed(ctx, message, session, semanticPOIs, userLocation, cityID, eventCh)
		if genErr != nil {
			finalResponseMessage = "I had trouble understanding your request. Could you please specify which POI you'd like to add?"
			assistantMessageType = types.TypeError
		} else {
			itineraryModifiedByThisTurn = true
		}

	case types.IntentRemovePOI:
		l.sendEvent(ctx, eventCh, types.StreamEvent{Type: types.EventTypeProgress, Data: "Processing: Removing Point of Interest with semantic understanding..."}, 3)
		finalResponseMessage = l.handleSemanticRemovePOI(ctx, message, session)
		if strings.Contains(finalResponseMessage, "I've removed") {
			itineraryModifiedByThisTurn = true
		}

	case types.IntentAskQuestion:
		l.sendEvent(ctx, eventCh, types.StreamEvent{Type: types.EventTypeProgress, Data: "Processing: Answering your question with semantic context..."}, 3)
		finalResponseMessage = "I’m here to help! For now, I’ll assume you’re asking about your trip. What specifically would you like to know?"

	case "replace_poi":
		l.sendEvent(ctx, eventCh, types.StreamEvent{Type: types.EventTypeProgress, Data: "Processing: Replacing Point of Interest..."}, 3)
		if matches := regexp.MustCompile(`replace\s+(.+?)\s+with\s+(.+?)(?:\s+in\s+my\s+itinerary)?`).FindStringSubmatch(strings.ToLower(message)); len(matches) == 3 {
			oldPOI := matches[1]
			newPOIName := matches[2]
			for i, poi := range session.CurrentItinerary.AIItineraryResponse.PointsOfInterest {
				if strings.Contains(strings.ToLower(poi.Name), oldPOI) {
					newPOI, err := l.generatePOIDataStream(ctx, newPOIName, session.SessionContext.CityName, userLocation, session.UserID, cityID, eventCh)
					if err != nil {
						finalResponseMessage = fmt.Sprintf("Could not replace %s with %s due to an error: %v", oldPOI, newPOIName, err)
						assistantMessageType = types.TypeError
					} else {
						session.CurrentItinerary.AIItineraryResponse.PointsOfInterest[i] = newPOI
						finalResponseMessage = fmt.Sprintf("I've replaced %s with %s in your itinerary.", oldPOI, newPOIName)
						itineraryModifiedByThisTurn = true
					}
					break
				}
			}
			if finalResponseMessage == "" {
				finalResponseMessage = fmt.Sprintf("Could not find %s in your itinerary.", oldPOI)
			}
		} else {
			finalResponseMessage = "Please specify the replacement clearly (e.g., 'replace X with Y')."
			assistantMessageType = types.TypeClarification
		}

	default: // modify_itinerary
		l.sendEvent(ctx, eventCh, types.StreamEvent{Type: types.EventTypeProgress, Data: "Processing: Updating itinerary..."}, 3)
		if matches := regexp.MustCompile(`replace\s+(.+?)\s+with\s+(.+?)(?:\s+in\s+my\s+itinerary)?`).FindStringSubmatch(strings.ToLower(message)); len(matches) == 3 {
			oldPOI := matches[1]
			newPOIName := matches[2]
			for i, poi := range session.CurrentItinerary.AIItineraryResponse.PointsOfInterest {
				if strings.Contains(strings.ToLower(poi.Name), oldPOI) {
					newPOI, err := l.generatePOIData(ctx, newPOIName, session.SessionContext.CityName, userLocation, session.UserID, cityID)
					if err != nil {
						l.logger.ErrorContext(ctx, "Failed to generate POI data", slog.Any("error", err))
						span.RecordError(err)
						finalResponseMessage = fmt.Sprintf("Could not replace %s with %s due to an error.", oldPOI, newPOIName)
					} else {
						session.CurrentItinerary.AIItineraryResponse.PointsOfInterest[i] = newPOI
						finalResponseMessage = fmt.Sprintf("I’ve replaced %s with %s in your itinerary.", oldPOI, newPOIName)
					}
					break
				}
			}
			if finalResponseMessage == "" {
				finalResponseMessage = fmt.Sprintf("Could not find %s in your itinerary.", oldPOI)
			}
		} else {
			finalResponseMessage = "I’ve noted your request to modify the itinerary. Please specify the changes (e.g., 'replace X with Y')."
		}
	}

	// --- 6. Post-Modification Processing (Sorting, Saving Session) ---
	if itineraryModifiedByThisTurn && userLocation != nil && userLocation.UserLat != 0 && userLocation.UserLon != 0 && session.CurrentItinerary != nil {
		l.sendEvent(ctx, eventCh, types.StreamEvent{Type: types.EventTypeProgress, Data: "Sorting updated POIs by distance..."}, 3)
		// Save new POIs to DB to ensure they have valid IDs
		for i, poi := range session.CurrentItinerary.AIItineraryResponse.PointsOfInterest {
			if poi.ID == uuid.Nil {
				dbPoiID, saveErr := l.llmInteractionRepo.SaveSinglePOI(ctx, poi, session.UserID, cityID, poi.LlmInteractionID)
				if saveErr != nil {
					l.logger.WarnContext(ctx, "Failed to save new POI", slog.String("name", poi.Name), slog.Any("error", saveErr))
					continue
				}
				session.CurrentItinerary.AIItineraryResponse.PointsOfInterest[i].ID = dbPoiID
			}
		}

		var currentPOIIDs []uuid.UUID
		for _, p := range session.CurrentItinerary.AIItineraryResponse.PointsOfInterest {
			if p.ID != uuid.Nil {
				currentPOIIDs = append(currentPOIIDs, p.ID)
			}
		}
		if (intent == types.IntentAddPOI || intent == types.IntentModifyItinerary) && userLocation != nil && userLocation.UserLat != 0 && userLocation.UserLon != 0 {
			sortedPOIs, err := l.llmInteractionRepo.GetPOIsBySessionSortedByDistance(ctx, sessionID, cityID, *userLocation)
			if err != nil {
				l.logger.WarnContext(ctx, "Failed to sort POIs by distance", slog.Any("error", err))
				span.RecordError(err)
			} else {
				session.CurrentItinerary.AIItineraryResponse.PointsOfInterest = sortedPOIs
				l.logger.InfoContext(ctx, "POIs sorted by distance",
					slog.Int("poi_count", len(sortedPOIs)))
				span.SetAttributes(attribute.Int("sorted_pois.count", len(sortedPOIs)))
			}
		}
	}

	// Add assistant's final response to history
	assistantMessage := types.ConversationMessage{
		ID: uuid.New(), Role: types.RoleAssistant, Content: finalResponseMessage, Timestamp: time.Now(), MessageType: assistantMessageType,
	}
	if err := l.llmInteractionRepo.AddMessageToSession(ctx, sessionID, assistantMessage); err != nil {
		l.logger.WarnContext(ctx, "Failed to save assistant message", slog.Any("error", err))
	}
	session.ConversationHistory = append(session.ConversationHistory, assistantMessage)

	// Update session in the database
	session.UpdatedAt = time.Now()
	session.ExpiresAt = time.Now().Add(24 * time.Hour)
	if err := l.llmInteractionRepo.UpdateSession(ctx, *session); err != nil {
		err = fmt.Errorf("failed to update session %s: %w", sessionID, err)
		l.sendEvent(ctx, eventCh, types.StreamEvent{Type: types.EventTypeError, Error: err.Error(), IsFinal: true}, 3)
		return err
	}

	// --- 7. Send Final Itinerary and Completion Event ---
	l.sendEvent(ctx, eventCh, types.StreamEvent{
		Type:      types.EventTypeItinerary,
		Data:      session.CurrentItinerary,
		Message:   finalResponseMessage,
		Timestamp: time.Now(),
	}, 3)
	l.sendEvent(ctx, eventCh, types.StreamEvent{
		Type:    types.EventTypeComplete,
		Data:    "Turn completed.",
		IsFinal: true,
		Navigation: &types.NavigationData{
			URL:       fmt.Sprintf("/itinerary?sessionId=%s&cityName=%s&domain=itinerary", sessionID.String(), url.QueryEscape(session.CityName)),
			RouteType: "itinerary",
			QueryParams: map[string]string{
				"sessionId": sessionID.String(),
				"cityName":  session.CityName,
				"domain":    "itinerary",
			},
		},
	}, 3)

	l.logger.InfoContext(ctx, "Streamed session continued", slog.String("sessionID", sessionID.String()), slog.String("intent", string(intent)))
	return nil
}

// generatePOIDataStream queries the LLM for POI details and streams updates
func (l *ServiceImpl) generatePOIDataStream(
ctx context.Context, poiName, cityName string,
userLocation *types.UserLocation, userID, cityID uuid.UUID,
eventCh chan<- types.StreamEvent,
) (types.POIDetailedInfo, error) {
ctx, span := otel.Tracer("LlmInteractionService").Start(ctx, "generatePOIDataStream",
trace.WithAttributes(attribute.String("poi.name", poiName), attribute.String("city.name", cityName)))
defer span.End()

	prompt := generatedContinuedConversationPrompt(poiName, cityName)
	config := &genai.GenerateContentConfig{Temperature: genai.Ptr[float32](0.2)}
	startTime := time.Now()

	var responseTextBuilder strings.Builder
	iter, err := l.aiClient.GenerateContentStream(ctx, prompt, config)
	if err != nil {
		l.sendEvent(ctx, eventCh, types.StreamEvent{
			Type:      types.EventTypeError,
			Error:     fmt.Sprintf("Failed to generate POI data for '%s': %v", poiName, err),
			Timestamp: time.Now(),
			EventID:   uuid.New().String(),
		}, 3)
		return types.POIDetailedInfo{}, fmt.Errorf("AI stream init failed for POI '%s': %w", poiName, err)
	}

	l.sendEvent(ctx, eventCh, types.StreamEvent{
		Type:      types.EventTypeProgress,
		Data:      map[string]string{"status": fmt.Sprintf("Getting details for %s...", poiName)},
		Timestamp: time.Now(),
		EventID:   uuid.New().String(),
	}, 3)

	for resp, err := range iter {
		if err != nil {
			l.sendEvent(ctx, eventCh, types.StreamEvent{
				Type:      types.EventTypeError,
				Error:     fmt.Sprintf("Streaming failed for POI '%s': %v", poiName, err),
				Timestamp: time.Now(),
				EventID:   uuid.New().String(),
			}, 3)
			return types.POIDetailedInfo{}, fmt.Errorf("streaming POI details for '%s' failed: %w", poiName, err)
		}
		for _, cand := range resp.Candidates {
			if cand.Content != nil {
				for _, part := range cand.Content.Parts {
					if part.Text != "" {
						responseTextBuilder.WriteString(string(part.Text))
						l.sendEvent(ctx, eventCh, types.StreamEvent{
							Type:      "poi_detail_chunk",
							Data:      map[string]string{"poi_name": poiName, "chunk": string(part.Text)},
							Timestamp: time.Now(),
							EventID:   uuid.New().String(),
						}, 3)
					}
				}
			}
		}
	}

	if ctx.Err() != nil {
		l.sendEvent(ctx, eventCh, types.StreamEvent{
			Type:      types.EventTypeError,
			Error:     ctx.Err().Error(),
			Timestamp: time.Now(),
			EventID:   uuid.New().String(),
		}, 3)
		return types.POIDetailedInfo{}, fmt.Errorf("context cancelled during POI detail generation: %w", ctx.Err())
	}

	fullText := responseTextBuilder.String()
	if fullText == "" {
		l.sendEvent(ctx, eventCh, types.StreamEvent{
			Type:      types.EventTypeError,
			Error:     fmt.Sprintf("Empty response for POI '%s'", poiName),
			Timestamp: time.Now(),
			EventID:   uuid.New().String(),
		}, 3)
		return types.POIDetailedInfo{Name: poiName, DescriptionPOI: "Details not found."}, fmt.Errorf("empty response for POI details '%s'", poiName)
	}

	// Save LLM interaction
	interaction := types.LlmInteraction{
		UserID:       userID,
		Prompt:       prompt,
		ResponseText: fullText,
		Timestamp:    startTime,
		CityName:     cityName,
	}
	llmInteractionID, err := l.saveCityInteraction(ctx, interaction)
	if err != nil {
		l.sendEvent(ctx, eventCh, types.StreamEvent{
			Type:      types.EventTypeError,
			Error:     fmt.Sprintf("Failed to save LLM interaction for POI '%s': %v", poiName, err),
			Timestamp: time.Now(),
			EventID:   uuid.New().String(),
		}, 3)
		return types.POIDetailedInfo{}, fmt.Errorf("failed to save LLM interaction: %w", err)
	}

	// Parse response
	cleanJSON := cleanJSONResponse(fullText)
	var poiData types.POIDetailedInfo
	if err := json.Unmarshal([]byte(cleanJSON), &poiData); err != nil || poiData.Name == "" {
		l.logger.WarnContext(ctx, "Invalid POI data from LLM", slog.String("response", fullText), slog.Any("error", err))
		poiData = types.POIDetailedInfo{
			ID:             uuid.New(),
			Name:           poiName,
			Category:       "Attraction",
			DescriptionPOI: fmt.Sprintf("Added %s based on user request, but detailed data not available.", poiName),
		}
	}
	if poiData.ID == uuid.Nil {
		poiData.ID = uuid.New()
	}
	poiData.LlmInteractionID = llmInteractionID
	poiData.City = cityName

	// Save POI to database
	dbPoiID, err := l.llmInteractionRepo.SaveSinglePOI(ctx, poiData, userID, cityID, llmInteractionID)
	if err != nil {
		l.logger.WarnContext(ctx, "Failed to save POI to database", slog.Any("error", err))
		span.RecordError(err)
		l.sendEvent(ctx, eventCh, types.StreamEvent{
			Type:      types.EventTypeError,
			Error:     fmt.Sprintf("Failed to save POI '%s' to database: %v", poiName, err),
			Timestamp: time.Now(),
			EventID:   uuid.New().String(),
		}, 3)
		return types.POIDetailedInfo{}, fmt.Errorf("failed to save POI to database: %w", err)
	}
	poiData.ID = dbPoiID

	// Calculate distance
	if userLocation != nil && userLocation.UserLat != 0 && userLocation.UserLon != 0 && poiData.Latitude != 0 && poiData.Longitude != 0 {
		distance, err := l.poiRepo.CalculateDistancePostGIS(ctx, userLocation.UserLat, userLocation.UserLon, poiData.Latitude, poiData.Longitude)
		if err != nil {
			l.logger.WarnContext(ctx, "Failed to calculate distance", slog.Any("error", err))
			span.RecordError(err)
		} else {
			poiData.Distance = distance
		}
	}

	l.sendEvent(ctx, eventCh, types.StreamEvent{
		Type:      "poi_detail_complete",
		Data:      poiData,
		Timestamp: time.Now(),
		EventID:   uuid.New().String(),
	}, 3)
	return poiData, nil
}

// streamingCityDataWorker ContinueSessionStreamed

func (l *ServiceImpl) generateCityData(ctx context.Context, cityName string) (string, error) {
ctx, span := otel.Tracer("LlmInteractionService").Start(ctx, "generateCityData", trace.WithAttributes(
attribute.String("city.name", cityName),
))
defer span.End()

	prompt := getCityDescriptionPrompt(cityName)
	var responseText strings.Builder

	// Try streaming
	iter, err := l.aiClient.GenerateContentStream(ctx, prompt, &genai.GenerateContentConfig{Temperature: genai.Ptr[float32](defaultTemperature)})
	if err == nil {
		for resp, err := range iter {
			if err != nil {
				span.RecordError(err)
				return "", fmt.Errorf("streaming city data error: %w", err)
			}
			for _, cand := range resp.Candidates {
				if cand.Content != nil {
					for _, part := range cand.Content.Parts {
						if part.Text != "" {
							responseText.WriteString(string(part.Text))
						}
					}
				}
			}
		}
	} else {
		// Fallback to non-streaming
		l.logger.WarnContext(ctx, "Streaming city data failed, falling back to non-streaming", slog.Any("error", err))
		response, err := l.aiClient.GenerateResponse(ctx, prompt, &genai.GenerateContentConfig{Temperature: genai.Ptr[float32](defaultTemperature)})
		if err != nil {
			span.RecordError(err)
			return "", fmt.Errorf("failed to generate city data: %w", err)
		}
		for _, cand := range response.Candidates {
			if cand.Content != nil {
				for _, part := range cand.Content.Parts {
					if part.Text != "" {
						responseText.WriteString(string(part.Text))
					}
				}
			}
		}
	}

	fullText := responseText.String()
	if fullText == "" {
		err := fmt.Errorf("empty city data response")
		span.RecordError(err)
		return "", err
	}

	return cleanJSONResponse(fullText), nil
}

func (l *ServiceImpl) saveCityInteraction(ctx context.Context, interaction types.LlmInteraction) (uuid.UUID, error) {
ctx, span := otel.Tracer("LlmInteractionService").Start(ctx, "saveCityInteraction")
defer span.End()

	if interaction.LatencyMs == 0 {
		// Ensure latency is set if not provided
		interaction.LatencyMs = int(time.Since(interaction.Timestamp).Milliseconds())
	}
	if interaction.ModelUsed == "" {
		interaction.ModelUsed = model // Default model
	}

	interactionID, err := l.llmInteractionRepo.SaveInteraction(ctx, interaction)
	if err != nil {
		span.RecordError(err)
		l.logger.WarnContext(ctx, "Failed to save LLM interaction", slog.Any("error", err))
		return uuid.Nil, fmt.Errorf("failed to save interaction: %w", err)
	}

	span.SetAttributes(attribute.String("interaction.id", interactionID.String()))
	return interactionID, nil
}

// handleSemanticAddPOIStreamed handles adding POIs with semantic search enhancement and streaming updates
func (l *ServiceImpl) handleSemanticAddPOIStreamed(ctx context.Context, message string, session *types.ChatSession, semanticPOIs []types.POIDetailedInfo, userLocation *types.UserLocation, cityID uuid.UUID, eventCh chan<- types.StreamEvent) (string, error) {
ctx, span := otel.Tracer("LlmInteractionService").Start(ctx, "handleSemanticAddPOIStreamed")
defer span.End()

	// Ensure the session has an initialized itinerary
	l.ensureItineraryExists(session)

	// Try semantic matching first - look for POIs semantically similar to the user's request
	if len(semanticPOIs) > 0 {
		l.sendEvent(ctx, eventCh, types.StreamEvent{
			Type: types.EventTypeProgress,
			Data: map[string]interface{}{
				"status":           "analyzing_semantic_matches",
				"semantic_options": len(semanticPOIs),
			},
		}, 3)

		// Check if any semantic POI matches what user is asking for
		for _, semanticPOI := range semanticPOIs[:min(3, len(semanticPOIs))] {
			// Check if this semantic POI is already in itinerary
			alreadyExists := false
			for _, existingPOI := range session.CurrentItinerary.AIItineraryResponse.PointsOfInterest {
				if strings.EqualFold(existingPOI.Name, semanticPOI.Name) {
					alreadyExists = true
					break
				}
			}

			if !alreadyExists {
				l.sendEvent(ctx, eventCh, types.StreamEvent{
					Type: "semantic_poi_added",
					Data: map[string]interface{}{
						"poi_name":       semanticPOI.Name,
						"poi_category":   semanticPOI.Category,
						"latitude":       semanticPOI.Latitude,
						"longitude":      semanticPOI.Longitude,
						"description":    semanticPOI.DescriptionPOI,
						"semantic_match": true,
					},
				}, 3)

				// Add semantic POI to itinerary
				session.CurrentItinerary.AIItineraryResponse.PointsOfInterest = append(
					session.CurrentItinerary.AIItineraryResponse.PointsOfInterest, semanticPOI)
				l.logger.InfoContext(ctx, "Added semantic POI to streaming itinerary",
					slog.String("poi_name", semanticPOI.Name))
				span.SetAttributes(attribute.String("added_poi", semanticPOI.Name))

				return fmt.Sprintf("Great! I found %s which matches what you're looking for. I've added it to your itinerary. %s",
					semanticPOI.Name, semanticPOI.DescriptionPOI), nil
			}
		}

		// If semantic POIs exist but all are already in itinerary
		l.sendEvent(ctx, eventCh, types.StreamEvent{
			Type: "semantic_alternatives_suggested",
			Data: map[string]interface{}{
				"message": "All semantic matches already in itinerary",
				"alternatives": func() []string {
					var names []string
					for i, poi := range semanticPOIs[:min(3, len(semanticPOIs))] {
						names = append(names, poi.Name)
						if i >= 2 {
							break
						}
					}
					return names
				}(),
			},
		}, 3)

		return fmt.Sprintf("I found some great options matching your request, but they're already in your itinerary. Here are some suggestions: %s",
			strings.Join(func() []string {
				var names []string
				for i, poi := range semanticPOIs[:min(3, len(semanticPOIs))] {
					names = append(names, poi.Name)
					if i >= 2 {
						break
					}
				}
				return names
			}(), ", ")), nil
	}

	// Fallback to traditional POI name extraction and generation
	l.sendEvent(ctx, eventCh, types.StreamEvent{
		Type: types.EventTypeProgress,
		Data: map[string]interface{}{"status": "extracting_poi_name"},
	}, 3)

	poiName := extractPOIName(message)
	if poiName == "" {
		return "I'd be happy to add a POI to your itinerary! Could you please specify which place you'd like to add?", nil
	}

	// Check if already exists
	for _, poi := range session.CurrentItinerary.AIItineraryResponse.PointsOfInterest {
		if strings.EqualFold(poi.Name, poiName) {
			return fmt.Sprintf("%s is already in your itinerary.", poiName), nil
		}
	}

	// Generate new POI data with streaming updates
	l.sendEvent(ctx, eventCh, types.StreamEvent{
		Type: types.EventTypeProgress,
		Data: map[string]interface{}{
			"status":   "generating_poi_data",
			"poi_name": poiName,
		},
	}, 3)

	newPOI, err := l.generatePOIDataStream(ctx, poiName, session.SessionContext.CityName, userLocation, session.UserID, cityID, eventCh)
	if err != nil {
		l.logger.ErrorContext(ctx, "Failed to generate POI data for streaming", slog.Any("error", err))
		span.RecordError(err)
		return "", fmt.Errorf("failed to generate POI data: %w", err)
	}

	session.CurrentItinerary.AIItineraryResponse.PointsOfInterest = append(
		session.CurrentItinerary.AIItineraryResponse.PointsOfInterest, newPOI)

	l.sendEvent(ctx, eventCh, types.StreamEvent{
		Type: "poi_added_successfully",
		Data: map[string]interface{}{
			"poi_name":       newPOI.Name,
			"poi_category":   newPOI.Category,
			"semantic_match": false,
		},
	}, 3)

	return fmt.Sprintf("I've added %s to your itinerary.", poiName), nil
}

/*
** Unified Response
*/
// ProcessUnifiedChatMessageStream handles unified chat with optimized streaming based on Google GenAI patterns
func (l *ServiceImpl) ProcessUnifiedChatMessageStream(ctx context.Context, userID, profileID uuid.UUID, cityName, message string, userLocation *types.UserLocation, eventCh chan<- types.StreamEvent) error {
startTime := time.Now() // Track when processing starts
ctx, span := otel.Tracer("LlmInteractionService").Start(ctx, "ProcessUnifiedChatMessageStream", trace.WithAttributes(
attribute.String("message", message),
))
defer span.End()

	// Extract city and clean message
	extractedCity, cleanedMessage, err := l.extractCityFromMessage(ctx, message)
	if err != nil {
		span.RecordError(err)
		l.sendEvent(ctx, eventCh, types.StreamEvent{Type: types.EventTypeError, Error: err.Error()}, 3)
		return fmt.Errorf("failed to parse message: %w", err)
	}
	if extractedCity != "" {
		cityName = extractedCity
	}
	span.SetAttributes(attribute.String("extracted.city", cityName), attribute.String("cleaned.message", cleanedMessage))

	// Detect domain
	domainDetector := &types.DomainDetector{}
	domain := domainDetector.DetectDomain(ctx, cleanedMessage)
	span.SetAttributes(attribute.String("detected.domain", string(domain)))

	// Step 3: Fetch user data
	_, searchProfile, _, err := l.FetchUserData(ctx, userID, profileID)
	if err != nil {
		span.RecordError(err)
		l.sendEvent(ctx, eventCh, types.StreamEvent{Type: types.EventTypeError, Error: err.Error()}, 3)
		return fmt.Errorf("failed to fetch user data: %w", err)
	}
	basePreferences := getUserPreferencesPrompt(searchProfile)

	// Use default location if not provided
	var lat, lon float64
	if userLocation == nil && searchProfile.UserLatitude != nil && searchProfile.UserLongitude != nil {
		userLocation = &types.UserLocation{
			UserLat: *searchProfile.UserLatitude,
			UserLon: *searchProfile.UserLongitude,
		}
	}
	if userLocation != nil {
		lat, lon = userLocation.UserLat, userLocation.UserLon
	}

	// Step 4: Cache Integration - Generate cache key based on session parameters
	sessionID := uuid.New()

	// Initialize session
	session := types.ChatSession{
		ID:        sessionID,
		UserID:    userID,
		ProfileID: profileID,
		CityName:  cityName,
		ConversationHistory: []types.ConversationMessage{
			{Role: "user", Content: message, Timestamp: time.Now()},
		},
		SessionContext: types.SessionContext{
			CityName:            cityName,
			ConversationSummary: fmt.Sprintf("Trip plan for %s", cityName),
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		ExpiresAt: time.Now().Add(24 * time.Hour),
		Status:    "active",
	}
	if err := l.llmInteractionRepo.CreateSession(ctx, session); err != nil {
		span.RecordError(err)
		l.sendEvent(ctx, eventCh, types.StreamEvent{Type: types.EventTypeError, Error: err.Error()}, 3)
		return fmt.Errorf("failed to create session: %w", err)
	}

	// Generate cache key based on session parameters
	cacheKeyData := map[string]interface{}{
		"user_id":     userID.String(),
		"profile_id":  profileID.String(),
		"city":        cityName,
		"message":     cleanedMessage,
		"domain":      string(domain),
		"preferences": basePreferences,
	}
	cacheKeyBytes, _ := json.Marshal(cacheKeyData)
	hash := md5.Sum(cacheKeyBytes)
	cacheKey := hex.EncodeToString(hash[:])

	// Step 5: Fan-in Fan-out Setup
	var wg sync.WaitGroup
	var closeOnce sync.Once

	l.sendEvent(ctx, eventCh, types.StreamEvent{
		Type: types.EventTypeStart,
		Data: map[string]interface{}{
			"domain":     string(domain),
			"city":       cityName,
			"session_id": sessionID.String(),
			"cache_key":  cacheKey,
		},
	}, 3)

	// Step 5: Collect responses for saving interaction
	responses := make(map[string]*strings.Builder)
	responsesMutex := sync.Mutex{}

	// Modified sendEventWithResponse to capture responses
	sendEventWithResponse := func(event types.StreamEvent) {
		if event.Type == types.EventTypeChunk {
			responsesMutex.Lock()
			if data, ok := event.Data.(map[string]interface{}); ok {
				if partType, exists := data["part"].(string); exists {
					if chunk, chunkExists := data["chunk"].(string); chunkExists {
						if responses[partType] == nil {
							responses[partType] = &strings.Builder{}
						}
						responses[partType].WriteString(chunk)
					}
				}
			}
			responsesMutex.Unlock()
		}
		l.sendEvent(ctx, eventCh, event, 3)
	}

	// Step 6: Spawn streaming workers based on domain with cache support
	switch domain {
	case types.DomainItinerary, types.DomainGeneral:
		wg.Add(3)

		// Worker 1: Stream City Data with cache
		go func() {
			defer wg.Done()
			prompt := getCityDataPrompt(cityName)
			partCacheKey := cacheKey + "_city_data"
			l.streamWorkerWithResponseAndCache(ctx, prompt, "city_data", sendEventWithResponse, domain, partCacheKey)
		}()

		// Worker 2: Stream General POIs with cache
		go func() {
			defer wg.Done()
			prompt := getGeneralPOIPrompt(cityName)
			partCacheKey := cacheKey + "_general_pois"
			l.streamWorkerWithResponseAndCache(ctx, prompt, "general_pois", sendEventWithResponse, domain, partCacheKey)
		}()

		// Worker 3: Stream Personalized Itinerary with cache
		go func() {
			defer wg.Done()
			prompt := getPersonalizedItineraryPrompt(cityName, basePreferences)
			partCacheKey := cacheKey + "_itinerary"
			l.streamWorkerWithResponseAndCache(ctx, prompt, "itinerary", sendEventWithResponse, domain, partCacheKey)
		}()

	case types.DomainAccommodation:
		wg.Add(1)
		go func() {
			defer wg.Done()
			prompt := getAccommodationPrompt(cityName, lat, lon, basePreferences)
			partCacheKey := cacheKey + "_hotels"
			l.streamWorkerWithResponseAndCache(ctx, prompt, "hotels", sendEventWithResponse, domain, partCacheKey)
		}()

	case types.DomainDining:
		wg.Add(1)
		go func() {
			defer wg.Done()
			prompt := getDiningPrompt(cityName, lat, lon, basePreferences)
			partCacheKey := cacheKey + "_restaurants"
			l.streamWorkerWithResponseAndCache(ctx, prompt, "restaurants", sendEventWithResponse, domain, partCacheKey)
		}()

	case types.DomainActivities:
		wg.Add(1)
		go func() {
			defer wg.Done()
			prompt := getActivitiesPrompt(cityName, lat, lon, basePreferences)
			partCacheKey := cacheKey + "_activities"
			l.streamWorkerWithResponseAndCache(ctx, prompt, "activities", sendEventWithResponse, domain, partCacheKey)
		}()

	default:
		sendEventWithResponse(types.StreamEvent{Type: types.EventTypeError, Error: fmt.Sprintf("unhandled domain: %s", domain)})
		return fmt.Errorf("unhandled domain type: %s", domain)
	}

	// Step 7: Completion goroutine with sync.Once for channel closure
	go func() {
		wg.Wait()             // Wait for all workers to complete
		if ctx.Err() == nil { // Only send completion event if context is still active
			// Determine route type based on domain
			var routeType string
			var baseURL string
			switch domain {
			case types.DomainAccommodation:
				routeType = "hotels"
				baseURL = "/hotels"
			case types.DomainDining:
				routeType = "restaurants"
				baseURL = "/restaurants"
			case types.DomainActivities:
				routeType = "activities"
				baseURL = "/activities"
			default:
				routeType = "itinerary"
				baseURL = "/itinerary"
			}

			l.sendEvent(ctx, eventCh, types.StreamEvent{
				Type: types.EventTypeComplete,
				Data: map[string]interface{}{"session_id": sessionID.String()},
				Navigation: &types.NavigationData{
					URL:       fmt.Sprintf("%s?sessionId=%s&cityName=%s&domain=%s", baseURL, sessionID.String(), url.QueryEscape(cityName), routeType),
					RouteType: routeType,
					QueryParams: map[string]string{
						"sessionId": sessionID.String(),
						"cityName":  cityName,
						"domain":    routeType,
					},
				},
			}, 3)
		}
		closeOnce.Do(func() {
			close(eventCh) // Close the channel only once
			l.logger.InfoContext(ctx, "Event channel closed by completion goroutine")
		})
	}()

	// Step 8: Save interaction and process structured data asynchronously after completion
	go func() {
		wg.Wait() // Wait for all workers to complete

		// Save interaction with complete response
		asyncCtx := context.Background()

		// Combine all responses into a single response text
		var fullResponseBuilder strings.Builder
		responsesMutex.Lock()
		cityDataContent := ""
		if responses["city_data"] != nil {
			cityDataContent = responses["city_data"].String()
		}
		for partType, builder := range responses {
			if builder != nil && builder.Len() > 0 {
				fullResponseBuilder.WriteString(fmt.Sprintf("[%s]\n%s\n\n", partType, builder.String()))
			}
		}
		responsesMutex.Unlock()

		fullResponse := fullResponseBuilder.String()
		if fullResponse == "" {
			fullResponse = fmt.Sprintf("Processed %s request for %s", domain, cityName)
		}

		// Process and save city data if available
		var cityID uuid.UUID
		if cityDataContent != "" {
			// Parse city data from the response
			if parsedCityData, parseErr := l.parseCityDataFromResponse(asyncCtx, cityDataContent); parseErr == nil && parsedCityData != nil {
				// Save city data to the cities table
				if savedCityID, handleErr := l.HandleCityData(asyncCtx, *parsedCityData); handleErr != nil {
					l.logger.WarnContext(asyncCtx, "Failed to save city data during unified stream processing",
						slog.String("city", cityName), slog.Any("error", handleErr))
				} else {
					l.logger.InfoContext(asyncCtx, "Successfully saved city data during unified stream processing",
						slog.String("city", cityName))
					cityID = savedCityID
				}
			} else if parseErr != nil {
				l.logger.WarnContext(asyncCtx, "Failed to parse city data from unified stream response",
					slog.String("city", cityName), slog.Any("error", parseErr))
			}
		}

		// If we don't have a cityID from the response, try to get it from the database
		if cityID == uuid.Nil {
			if existingCity, err := l.cityRepo.FindCityByNameAndCountry(asyncCtx, cityName, ""); err == nil && existingCity != nil {
				cityID = existingCity.ID
			} else {
				l.logger.WarnContext(asyncCtx, "Could not find or save city data, skipping POI processing",
					slog.String("city", cityName))
				return
			}
		}

		// Create and save interaction first to get proper llmInteractionID
		interaction := types.LlmInteraction{
			ID:           uuid.New(),
			SessionID:    sessionID,
			UserID:       userID,
			ProfileID:    profileID,
			CityName:     cityName,
			Prompt:       fmt.Sprintf("Unified Chat Stream - Domain: %s, Message: %s", domain, cleanedMessage),
			ResponseText: fullResponse,
			ModelUsed:    model,
			LatencyMs:    int(time.Since(startTime).Milliseconds()),
			Timestamp:    startTime,
		}
		savedInteractionID, err := l.llmInteractionRepo.SaveInteraction(asyncCtx, interaction)
		if err != nil {
			l.logger.ErrorContext(asyncCtx, "Failed to save stream interaction", slog.Any("error", err))
			return
		}

		l.logger.InfoContext(asyncCtx, "Stream interaction saved successfully",
			slog.String("saved_interaction_id", savedInteractionID.String()),
			slog.String("original_session_id", sessionID.String()))

		// Always try to process and save POI data regardless of domain
		// since responses may contain POI data in different formats
		l.ProcessAndSaveUnifiedResponse(asyncCtx, responses, userID, profileID, cityID, savedInteractionID, userLocation)
	}()

	span.SetStatus(codes.Ok, "Unified chat stream processed successfully")
	return nil
}

func (l *ServiceImpl) ProcessUnifiedChatMessageStreamFree(ctx context.Context, cityName, message string, userLocation *types.UserLocation, eventCh chan<- types.StreamEvent) error {
startTime := time.Now() // Track when processing starts
ctx, span := otel.Tracer("LlmInteractionService").Start(ctx, "ProcessUnifiedChatMessageStream", trace.WithAttributes(
attribute.String("message", message),
))
defer span.End()

	// Extract city and clean message
	extractedCity, cleanedMessage, err := l.extractCityFromMessage(ctx, message)
	if err != nil {
		span.RecordError(err)
		l.sendEvent(ctx, eventCh, types.StreamEvent{Type: types.EventTypeError, Error: err.Error()}, 3)
		return fmt.Errorf("failed to parse message: %w", err)
	}
	if extractedCity != "" {
		cityName = extractedCity
	}
	span.SetAttributes(attribute.String("extracted.city", cityName), attribute.String("cleaned.message", cleanedMessage))

	// Detect domain
	domainDetector := &types.DomainDetector{}
	domain := domainDetector.DetectDomain(ctx, cleanedMessage)
	span.SetAttributes(attribute.String("detected.domain", string(domain)))

	// Step 4: Cache Integration - Generate cache key based on session parameters
	sessionID := uuid.New()

	// Initialize session
	session := types.ChatSession{
		ID:       sessionID,
		CityName: cityName,
		ConversationHistory: []types.ConversationMessage{
			{Role: "user", Content: message, Timestamp: time.Now()},
		},
		SessionContext: types.SessionContext{
			CityName:            cityName,
			ConversationSummary: fmt.Sprintf("Trip plan for %s", cityName),
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		ExpiresAt: time.Now().Add(24 * time.Hour),
		Status:    "active",
	}
	if err := l.llmInteractionRepo.CreateSession(ctx, session); err != nil {
		span.RecordError(err)
		l.sendEvent(ctx, eventCh, types.StreamEvent{Type: types.EventTypeError, Error: err.Error()}, 3)
		return fmt.Errorf("failed to create session: %w", err)
	}

	// Generate cache key based on session parameters
	cacheKeyData := map[string]interface{}{
		"city":    cityName,
		"message": cleanedMessage,
		"domain":  string(domain),
	}
	cacheKeyBytes, _ := json.Marshal(cacheKeyData)
	hash := md5.Sum(cacheKeyBytes)
	cacheKey := hex.EncodeToString(hash[:])

	// Step 5: Fan-in Fan-out Setup
	var wg sync.WaitGroup
	var closeOnce sync.Once

	l.sendEvent(ctx, eventCh, types.StreamEvent{
		Type: types.EventTypeStart,
		Data: map[string]interface{}{
			"domain":     string(domain),
			"city":       cityName,
			"session_id": sessionID.String(),
			"cache_key":  cacheKey,
		},
	}, 3)

	// Step 5: Collect responses for saving interaction
	responses := make(map[string]*strings.Builder)
	responsesMutex := sync.Mutex{}

	// Modified sendEventWithResponse to capture responses
	sendEventWithResponse := func(event types.StreamEvent) {
		if event.Type == types.EventTypeChunk {
			responsesMutex.Lock()
			if data, ok := event.Data.(map[string]interface{}); ok {
				if partType, exists := data["part"].(string); exists {
					if chunk, chunkExists := data["chunk"].(string); chunkExists {
						if responses[partType] == nil {
							responses[partType] = &strings.Builder{}
						}
						responses[partType].WriteString(chunk)
					}
				}
			}
			responsesMutex.Unlock()
		}
		l.sendEvent(ctx, eventCh, event, 3)
	}

	// Step 6: Spawn streaming workers based on domain with cache support
	switch domain {
	case types.DomainItinerary, types.DomainGeneral:
		wg.Add(3)

		// Worker 1: Stream City Data with cache
		go func() {
			defer wg.Done()
			prompt := getCityDataPrompt(cityName)
			partCacheKey := cacheKey + "_city_data"
			l.streamWorkerWithResponseAndCache(ctx, prompt, "city_data", sendEventWithResponse, domain, partCacheKey)
		}()

		// Worker 2: Stream General POIs with cache
		go func() {
			defer wg.Done()
			prompt := getGeneralPOIPrompt(cityName)
			partCacheKey := cacheKey + "_general_pois"
			l.streamWorkerWithResponseAndCache(ctx, prompt, "general_pois", sendEventWithResponse, domain, partCacheKey)
		}()

		// Worker 3: Stream Personalized Itinerary with cache
		go func() {
			defer wg.Done()
			prompt := getGeneralizedItineraryPrompt(cityName)
			partCacheKey := cacheKey + "_itinerary"
			l.streamWorkerWithResponseAndCache(ctx, prompt, "itinerary", sendEventWithResponse, domain, partCacheKey)
		}()

	case types.DomainAccommodation:
		wg.Add(1)
		go func() {
			defer wg.Done()
			prompt := getGeneralAccommodationPrompt(cityName)
			partCacheKey := cacheKey + "_hotels"
			l.streamWorkerWithResponseAndCache(ctx, prompt, "hotels", sendEventWithResponse, domain, partCacheKey)
		}()

	case types.DomainDining:
		wg.Add(1)
		go func() {
			defer wg.Done()
			prompt := getGeneralDiningPrompt(cityName)
			partCacheKey := cacheKey + "_restaurants"
			l.streamWorkerWithResponseAndCache(ctx, prompt, "restaurants", sendEventWithResponse, domain, partCacheKey)
		}()

	case types.DomainActivities:
		wg.Add(1)
		go func() {
			defer wg.Done()
			prompt := getGeneralActivitiesPrompt(cityName)
			partCacheKey := cacheKey + "_activities"
			l.streamWorkerWithResponseAndCache(ctx, prompt, "activities", sendEventWithResponse, domain, partCacheKey)
		}()

	default:
		sendEventWithResponse(types.StreamEvent{Type: types.EventTypeError, Error: fmt.Sprintf("unhandled domain: %s", domain)})
		return fmt.Errorf("unhandled domain type: %s", domain)
	}

	// Step 7: Completion goroutine with sync.Once for channel closure
	go func() {
		wg.Wait()             // Wait for all workers to complete
		if ctx.Err() == nil { // Only send completion event if context is still active
			// Determine route type based on domain
			var routeType string
			var baseURL string
			switch domain {
			case types.DomainAccommodation:
				routeType = "hotels"
				baseURL = "/hotels"
			case types.DomainDining:
				routeType = "restaurants"
				baseURL = "/restaurants"
			case types.DomainActivities:
				routeType = "activities"
				baseURL = "/activities"
			default:
				routeType = "itinerary"
				baseURL = "/itinerary"
			}

			l.sendEvent(ctx, eventCh, types.StreamEvent{
				Type: types.EventTypeComplete,
				Data: map[string]interface{}{"session_id": sessionID.String()},
				Navigation: &types.NavigationData{
					URL:       fmt.Sprintf("%s?sessionId=%s&cityName=%s&domain=%s", baseURL, sessionID.String(), url.QueryEscape(cityName), routeType),
					RouteType: routeType,
					QueryParams: map[string]string{
						"sessionId": sessionID.String(),
						"cityName":  cityName,
						"domain":    routeType,
					},
				},
			}, 3)
		}
		closeOnce.Do(func() {
			close(eventCh) // Close the channel only once
			l.logger.InfoContext(ctx, "Event channel closed by completion goroutine")
		})
	}()

	// Step 8: Save interaction and process structured data asynchronously after completion
	go func() {
		wg.Wait() // Wait for all workers to complete

		// Save interaction with complete response
		asyncCtx := context.Background()

		// Combine all responses into a single response text
		var fullResponseBuilder strings.Builder
		responsesMutex.Lock()
		cityDataContent := ""
		if responses["city_data"] != nil {
			cityDataContent = responses["city_data"].String()
		}
		for partType, builder := range responses {
			if builder != nil && builder.Len() > 0 {
				fullResponseBuilder.WriteString(fmt.Sprintf("[%s]\n%s\n\n", partType, builder.String()))
			}
		}
		responsesMutex.Unlock()

		fullResponse := fullResponseBuilder.String()
		if fullResponse == "" {
			fullResponse = fmt.Sprintf("Processed %s request for %s", domain, cityName)
		}

		// Process and save city data if available
		var cityID uuid.UUID
		if cityDataContent != "" {
			// Parse city data from the response
			if parsedCityData, parseErr := l.parseCityDataFromResponse(asyncCtx, cityDataContent); parseErr == nil && parsedCityData != nil {
				// Save city data to the cities table
				if savedCityID, handleErr := l.HandleCityData(asyncCtx, *parsedCityData); handleErr != nil {
					l.logger.WarnContext(asyncCtx, "Failed to save city data during unified stream processing",
						slog.String("city", cityName), slog.Any("error", handleErr))
				} else {
					l.logger.InfoContext(asyncCtx, "Successfully saved city data during unified stream processing",
						slog.String("city", cityName))
					cityID = savedCityID
				}
			} else if parseErr != nil {
				l.logger.WarnContext(asyncCtx, "Failed to parse city data from unified stream response",
					slog.String("city", cityName), slog.Any("error", parseErr))
			}
		}

		// If we don't have a cityID from the response, try to get it from the database
		if cityID == uuid.Nil {
			if existingCity, err := l.cityRepo.FindCityByNameAndCountry(asyncCtx, cityName, ""); err == nil && existingCity != nil {
				cityID = existingCity.ID
			} else {
				l.logger.WarnContext(asyncCtx, "Could not find or save city data, skipping POI processing",
					slog.String("city", cityName))
				return
			}
		}

		// Create and save interaction first to get proper llmInteractionID
		interaction := types.LlmInteraction{
			ID:           uuid.New(),
			SessionID:    sessionID,
			CityName:     cityName,
			Prompt:       fmt.Sprintf("Unified Chat Stream - Domain: %s, Message: %s", domain, cleanedMessage),
			ResponseText: fullResponse,
			ModelUsed:    model,
			LatencyMs:    int(time.Since(startTime).Milliseconds()),
			Timestamp:    startTime,
		}
		savedInteractionID, err := l.llmInteractionRepo.SaveInteraction(asyncCtx, interaction)
		if err != nil {
			l.logger.ErrorContext(asyncCtx, "Failed to save stream interaction", slog.Any("error", err))
			return
		}

		l.logger.InfoContext(asyncCtx, "Stream interaction saved successfully (free)",
			slog.String("saved_interaction_id", savedInteractionID.String()),
			slog.String("original_session_id", sessionID.String()))

		// Always try to process and save POI data regardless of domain
		// since responses may contain POI data in different formats
		l.ProcessAndSaveUnifiedResponseFree(asyncCtx, responses, cityID, savedInteractionID, userLocation)
	}()

	span.SetStatus(codes.Ok, "Unified chat stream processed successfully")
	return nil
}

// ensureItineraryExists initializes the session's CurrentItinerary if it's nil
func (l *ServiceImpl) ensureItineraryExists(session *types.ChatSession) {
if session.CurrentItinerary == nil {
session.CurrentItinerary = &types.AiCityResponse{
AIItineraryResponse: types.AIItineraryResponse{
ItineraryName:      fmt.Sprintf("Trip to %s", session.SessionContext.CityName),
OverallDescription: fmt.Sprintf("Exploring %s", session.SessionContext.CityName),
PointsOfInterest:   []types.POIDetailedInfo{},
},
}
}
if session.CurrentItinerary.AIItineraryResponse.PointsOfInterest == nil {
session.CurrentItinerary.AIItineraryResponse.PointsOfInterest = []types.POIDetailedInfo{}
}
}

// parseCityDataFromResponse extracts and parses city data from streamed response content
func (l *ServiceImpl) parseCityDataFromResponse(ctx context.Context, responseContent string) (*types.GeneralCityData, error) {
// Clean the response by extracting JSON content between ```json and ```
cleanedResponse := responseContent

	// Look for JSON blocks in the response
	if strings.Contains(responseContent, "```json") {
		start := strings.Index(responseContent, "```json")
		if start != -1 {
			start += len("```json")
			end := strings.Index(responseContent[start:], "```")
			if end != -1 {
				cleanedResponse = strings.TrimSpace(responseContent[start : start+end])
			}
		}
	}

	// Validate JSON
	if !json.Valid([]byte(cleanedResponse)) {
		return nil, fmt.Errorf("invalid JSON in city data response")
	}

	// Try to parse as GeneralCityData
	var generalCity types.GeneralCityData
	if err := json.Unmarshal([]byte(cleanedResponse), &generalCity); err != nil {
		return nil, fmt.Errorf("failed to unmarshal city data: %w", err)
	}

	// Validate that we have minimum required city data
	if generalCity.City == "" {
		return nil, fmt.Errorf("parsed city data is missing city name")
	}

	return &generalCity, nil
}

// streamWorkerWithResponseAndCache handles streaming for a single worker with response capture and cache support
func (l *ServiceImpl) streamWorkerWithResponseAndCache(ctx context.Context, prompt, partType string, sendEvent func(types.StreamEvent), domain types.DomainType, cacheKey string) {
iter, err := l.aiClient.GenerateContentStreamWithCache(ctx, prompt, &genai.GenerateContentConfig{Temperature: genai.Ptr[float32](defaultTemperature)}, cacheKey)
if err != nil {
if ctx.Err() == nil {
sendEvent(types.StreamEvent{
Type:  types.EventTypeError,
Error: fmt.Sprintf("%s worker failed: %v", partType, err),
})
}
return
}

	var fullResponse strings.Builder
	for resp, err := range iter {
		if ctx.Err() != nil {
			return // Stop if context is canceled
		}
		if err != nil {
			if ctx.Err() == nil {
				sendEvent(types.StreamEvent{
					Type:  types.EventTypeError,
					Error: fmt.Sprintf("%s streaming error: %v", partType, err),
				})
			}
			return
		}
		for _, cand := range resp.Candidates {
			if cand.Content != nil {
				for _, part := range cand.Content.Parts {
					if part.Text != "" {
						chunk := string(part.Text)
						fullResponse.WriteString(chunk)
						sendEvent(types.StreamEvent{
							Type: types.EventTypeChunk,
							Data: map[string]interface{}{
								"part":       partType,
								"chunk":      chunk,
								"domain":     string(domain),
								"cache_key":  cacheKey,
								"cache_used": cacheKey != "",
							},
						})
					}
				}
			}
		}
	}
}


package llmChat

import (
"context"
"encoding/json"
"fmt"
"log/slog"
"strings"
"sync"
"time"

	"github.com/FACorreiaa/go-poi-au-suggestions/internal/types"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/genai"
)

func (l *ServiceImpl) GenerateCityDataWorker(wg *sync.WaitGroup,
ctx context.Context,
cityName string,
resultCh chan<- types.GenAIResponse,
config *genai.GenerateContentConfig) {
go func() {
ctx, span := otel.Tracer("LlmInteractionService").Start(ctx, "GenerateCityDataWorker", trace.WithAttributes(
attribute.String("city.name", cityName),
))
defer span.End()
defer wg.Done()

		prompt := getCityDescriptionPrompt(cityName)
		span.SetAttributes(attribute.Int("prompt.length", len(prompt)))

		response, err := l.aiClient.GenerateResponse(ctx, prompt, config)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, "Failed to generate city data")
			resultCh <- types.GenAIResponse{Err: fmt.Errorf("failed to generate city data: %w", err)}
			return
		}

		var txt string
		for _, candidate := range response.Candidates {
			if candidate.Content != nil && len(candidate.Content.Parts) > 0 {
				txt = candidate.Content.Parts[0].Text
				break
			}
		}
		if txt == "" {
			err := fmt.Errorf("no valid city data content from AI")
			span.RecordError(err)
			span.SetStatus(codes.Error, "Empty response from AI")
			resultCh <- types.GenAIResponse{Err: err}
			return
		}
		span.SetAttributes(attribute.Int("response.length", len(txt)))

		cleanTxt := cleanJSONResponse(txt)
		var cityDataFromAI struct {
			CityName        string  `json:"city_name"`
			StateProvince   *string `json:"state_province"` // Use pointer for nullable string
			Country         string  `json:"country"`
			CenterLatitude  float64 `json:"center_latitude"`
			CenterLongitude float64 `json:"center_longitude"`
			Description     string  `json:"description"`
			// BoundingBox     string  `json:"bounding_box,omitempty"` // If trying to get BBox string
		}
		if err := json.Unmarshal([]byte(cleanTxt), &cityDataFromAI); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, "Failed to parse city data JSON")
			resultCh <- types.GenAIResponse{Err: fmt.Errorf("failed to parse city data JSON: %w", err)}
			return
		}

		stateProvinceValue := ""
		if cityDataFromAI.StateProvince != nil {
			stateProvinceValue = *cityDataFromAI.StateProvince
		}

		span.SetAttributes(
			attribute.String("city.name", cityDataFromAI.CityName),
			attribute.String("city.country", cityDataFromAI.Country),
			attribute.Float64("city.latitude", cityDataFromAI.CenterLatitude),
			attribute.Float64("city.longitude", cityDataFromAI.CenterLongitude),
		)
		span.SetStatus(codes.Ok, "City data generated successfully")

		resultCh <- types.GenAIResponse{
			City:            cityDataFromAI.CityName,
			Country:         cityDataFromAI.Country,
			StateProvince:   stateProvinceValue,
			CityDescription: cityDataFromAI.Description,
			Latitude:        cityDataFromAI.CenterLatitude,
			Longitude:       cityDataFromAI.CenterLongitude,
			// BoundingBoxWKT: cityDataFromAI.BoundingBox, // TODO
		}
	}()
}

func (l *ServiceImpl) GenerateGeneralPOIWorker(wg *sync.WaitGroup,
ctx context.Context,
cityName string,
resultCh chan<- types.GenAIResponse,
config *genai.GenerateContentConfig) {
ctx, span := otel.Tracer("LlmInteractionService").Start(ctx, "GenerateGeneralPOIWorker", trace.WithAttributes(
attribute.String("city.name", cityName),
))
defer span.End()
defer wg.Done()

	prompt := getGeneralPOIPrompt(cityName)
	span.SetAttributes(attribute.Int("prompt.length", len(prompt)))

	startTime := time.Now()
	response, err := l.aiClient.GenerateResponse(ctx, prompt, config)
	latencyMs := int(time.Since(startTime).Milliseconds())
	span.SetAttributes(attribute.Int("response.latency_ms", latencyMs))

	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to generate general POIs")
		resultCh <- types.GenAIResponse{Err: fmt.Errorf("failed to generate general POIs: %w", err)}
		return
	}

	var txt string
	for _, candidate := range response.Candidates {
		if candidate.Content != nil && len(candidate.Content.Parts) > 0 {
			txt = candidate.Content.Parts[0].Text
			break
		}
	}
	if txt == "" {
		err := fmt.Errorf("no valid general POI content from AI")
		span.RecordError(err)
		span.SetStatus(codes.Error, "Empty response from AI")
		resultCh <- types.GenAIResponse{Err: err}
		return
	}
	span.SetAttributes(attribute.Int("response.length", len(txt)))

	cleanTxt := cleanJSONResponse(txt)
	var poiData struct {
		PointsOfInterest []types.POIDetailedInfo `json:"points_of_interest"`
	}
	if err := json.Unmarshal([]byte(cleanTxt), &poiData); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to parse general POI JSON")
		resultCh <- types.GenAIResponse{Err: fmt.Errorf("failed to parse general POI JSON: %w", err)}
		return
	}

	span.SetAttributes(attribute.Int("pois.count", len(poiData.PointsOfInterest)))
	span.SetStatus(codes.Ok, "General POIs generated successfully")
	resultCh <- types.GenAIResponse{GeneralPOI: poiData.PointsOfInterest}
}

func (l *ServiceImpl) GeneratePersonalisedPOIWorker(wg *sync.WaitGroup, ctx context.Context,
cityName string, userID, profileID, sessionID uuid.UUID, resultCh chan<- types.GenAIResponse,
interestNames []string, tagsPromptPart string, userPrefs string,
config *genai.GenerateContentConfig) {
ctx, span := otel.Tracer("LlmInteractionService").Start(ctx, "GeneratePersonalisedPOIWorker", trace.WithAttributes(
attribute.String("city.name", cityName),
attribute.String("user.id", userID.String()),
attribute.String("profile.id", profileID.String()),
attribute.Int("interests.count", len(interestNames)),
))
defer span.End()
defer wg.Done()

	startTime := time.Now()

	prompt := getPersonalizedPOI(interestNames, cityName, tagsPromptPart, userPrefs)
	span.SetAttributes(attribute.Int("prompt.length", len(prompt)))

	response, err := l.aiClient.GenerateResponse(ctx, prompt, config)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to generate personalized itinerary")
		resultCh <- types.GenAIResponse{Err: fmt.Errorf("failed to generate personalized itinerary: %w", err)}
		return
	}

	var txt string
	for _, candidate := range response.Candidates {
		if candidate.Content != nil && len(candidate.Content.Parts) > 0 {
			txt = candidate.Content.Parts[0].Text
			break
		}
	}
	if txt == "" {
		err := fmt.Errorf("no valid personalized itinerary content from AI")
		span.RecordError(err)
		span.SetStatus(codes.Error, "Empty response from AI")
		resultCh <- types.GenAIResponse{Err: err}
		return
	}
	span.SetAttributes(attribute.Int("response.length", len(txt)))

	cleanTxt := cleanJSONResponse(txt)
	var itineraryData struct {
		ItineraryName      string                  `json:"itinerary_name"`
		OverallDescription string                  `json:"overall_description"`
		PointsOfInterest   []types.POIDetailedInfo `json:"points_of_interest"`
	}

	if err := json.Unmarshal([]byte(cleanTxt), &itineraryData); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to parse personalized itinerary JSON")
		resultCh <- types.GenAIResponse{Err: fmt.Errorf("failed to parse personalized itinerary JSON: %w", err)}
		return
	}
	span.SetAttributes(
		attribute.String("itinerary.name", itineraryData.ItineraryName),
		attribute.Int("personalized_pois.count", len(itineraryData.PointsOfInterest)),
	)

	latencyMs := int(time.Since(startTime).Milliseconds())
	span.SetAttributes(attribute.Int("response.latency_ms", latencyMs))

	interaction := types.LlmInteraction{
		UserID:       userID,
		SessionID:    sessionID,
		Prompt:       prompt,
		ResponseText: txt,
		ModelUsed:    model, // Adjust based on your AI client
		LatencyMs:    latencyMs,
		CityName:     cityName,
		// request payload
		// response payload
		// Add token counts if available from response (depends on genai API)
		// PromptTokens, CompletionTokens, TotalTokens
		// RequestPayload, ResponsePayload if you serialize the full request/response
	}
	savedInteractionID, err := l.llmInteractionRepo.SaveInteraction(ctx, interaction)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to save LLM interaction")
		resultCh <- types.GenAIResponse{Err: fmt.Errorf("failed to save LLM interaction: %w", err)}
		return
	}
	span.SetAttributes(attribute.String("llm_interaction.id", savedInteractionID.String()))
	span.SetStatus(codes.Ok, "Personalized POIs generated successfully")

	resultCh <- types.GenAIResponse{
		ItineraryName:        itineraryData.ItineraryName,
		ItineraryDescription: itineraryData.OverallDescription,
		PersonalisedPOI:      itineraryData.PointsOfInterest,
		LlmInteractionID:     savedInteractionID,
	}
}

// GeneratePersonalisedPOIWorkerWithSemantics generates personalized POIs with semantic search enhancement
func (l *ServiceImpl) GeneratePersonalisedPOIWorkerWithSemantics(wg *sync.WaitGroup, ctx context.Context,
cityName string, userID, profileID, sessionID uuid.UUID, resultCh chan<- types.GenAIResponse,
interestNames []string, tagsPromptPart string, userPrefs string, semanticPOIs []types.POIDetailedInfo,
config *genai.GenerateContentConfig) {
ctx, span := otel.Tracer("LlmInteractionService").Start(ctx, "GeneratePersonalisedPOIWorkerWithSemantics", trace.WithAttributes(
attribute.String("city.name", cityName),
attribute.String("user.id", userID.String()),
attribute.String("profile.id", profileID.String()),
attribute.Int("interests.count", len(interestNames)),
attribute.Int("semantic_pois.count", len(semanticPOIs)),
))
defer span.End()
defer wg.Done()

	startTime := time.Now()

	// Create enhanced prompt with semantic context
	prompt := l.getPersonalizedPOIWithSemanticContext(interestNames, cityName, tagsPromptPart, userPrefs, semanticPOIs)
	span.SetAttributes(attribute.Int("prompt.length", len(prompt)))

	response, err := l.aiClient.GenerateResponse(ctx, prompt, config)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to generate semantic-enhanced personalized itinerary")
		resultCh <- types.GenAIResponse{Err: fmt.Errorf("failed to generate semantic-enhanced personalized itinerary: %w", err)}
		return
	}

	var txt string
	for _, candidate := range response.Candidates {
		if candidate.Content != nil && len(candidate.Content.Parts) > 0 {
			txt = candidate.Content.Parts[0].Text
			break
		}
	}
	if txt == "" {
		err := fmt.Errorf("no valid semantic-enhanced personalized itinerary content from AI")
		span.RecordError(err)
		span.SetStatus(codes.Error, "Empty response from AI")
		resultCh <- types.GenAIResponse{Err: err}
		return
	}
	span.SetAttributes(attribute.Int("response.length", len(txt)))

	cleanTxt := cleanJSONResponse(txt)
	var itineraryData struct {
		ItineraryName      string                  `json:"itinerary_name"`
		OverallDescription string                  `json:"overall_description"`
		PointsOfInterest   []types.POIDetailedInfo `json:"points_of_interest"`
	}

	if err := json.Unmarshal([]byte(cleanTxt), &itineraryData); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to parse semantic-enhanced personalized itinerary JSON")
		resultCh <- types.GenAIResponse{Err: fmt.Errorf("failed to parse semantic-enhanced personalized itinerary JSON: %w", err)}
		return
	}
	span.SetAttributes(
		attribute.String("itinerary.name", itineraryData.ItineraryName),
		attribute.Int("personalized_pois.count", len(itineraryData.PointsOfInterest)),
	)

	latencyMs := int(time.Since(startTime).Milliseconds())
	span.SetAttributes(attribute.Int("response.latency_ms", latencyMs))

	interaction := types.LlmInteraction{
		UserID:       userID,
		SessionID:    sessionID,
		Prompt:       prompt,
		ResponseText: txt,
		ModelUsed:    model,
		LatencyMs:    latencyMs,
		CityName:     cityName,
	}
	savedInteractionID, err := l.llmInteractionRepo.SaveInteraction(ctx, interaction)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to save semantic-enhanced LLM interaction")
		resultCh <- types.GenAIResponse{Err: fmt.Errorf("failed to save semantic-enhanced LLM interaction: %w", err)}
		return
	}
	span.SetAttributes(attribute.String("llm_interaction.id", savedInteractionID.String()))
	span.SetStatus(codes.Ok, "Semantic-enhanced personalized POIs generated successfully")

	resultCh <- types.GenAIResponse{
		ItineraryName:        itineraryData.ItineraryName,
		ItineraryDescription: itineraryData.OverallDescription,
		PersonalisedPOI:      itineraryData.PointsOfInterest,
		LlmInteractionID:     savedInteractionID,
	}
}

// streamingCityDataWorker generates city data with streaming updates
func (l *ServiceImpl) streamingCityDataWorker(wg *sync.WaitGroup,
ctx context.Context, cityName string, resultCh chan<- types.GenAIResponse,
eventCh chan<- types.StreamEvent, userID uuid.UUID) {
ctxWorker, span := otel.Tracer("LlmInteractionService").Start(ctx, "streamingCityDataWorker", trace.WithAttributes(
attribute.String("city.name", cityName),
))
defer span.End()
if wg != nil {
defer wg.Done()
}

	if !l.sendEvent(ctxWorker, eventCh, types.StreamEvent{
		Type: types.EventTypeProgress,
		Data: map[string]interface{}{"status": "generating_city_data", "progress": 10},
	}, 3) {
		resultCh <- types.GenAIResponse{Err: fmt.Errorf("context cancelled before sending initial progress for city data")}
		return
	}

	startTime := time.Now()
	prompt := getCityDescriptionPrompt(cityName)

	// Generate city data
	cleanTxt, err := l.generateCityData(ctxWorker, cityName)
	if err != nil {
		span.RecordError(err)
		l.sendEvent(ctxWorker, eventCh, types.StreamEvent{
			Type:      types.EventTypeError,
			Error:     err.Error(),
			Timestamp: time.Now(),
			EventID:   uuid.New().String(),
		}, 3)
		resultCh <- types.GenAIResponse{Err: err}
		return
	}

	// Send partial data event (for consistency with original)
	l.sendEvent(ctxWorker, eventCh, types.StreamEvent{
		Type:      types.EventTypeCityData,
		Data:      map[string]string{"partial_city_data": cleanTxt},
		Timestamp: time.Now(),
		EventID:   uuid.New().String(),
	}, 3)

	// Save LLM interaction
	interaction := types.LlmInteraction{
		UserID:       userID,
		Prompt:       prompt,
		ResponseText: cleanTxt,
		Timestamp:    startTime,
		LatencyMs:    int(time.Since(startTime).Milliseconds()),
		CityName:     cityName,
	}
	_, err = l.saveCityInteraction(ctxWorker, interaction)
	if err != nil {
		span.RecordError(err)
		l.sendEvent(ctxWorker, eventCh, types.StreamEvent{
			Type:      types.EventTypeError,
			Error:     fmt.Sprintf("failed to save city data interaction: %v", err),
			Timestamp: time.Now(),
			EventID:   uuid.New().String(),
		}, 3)
		resultCh <- types.GenAIResponse{Err: err}
		return
	}

	// Parse JSON response
	var cityData struct {
		CityName        string  `json:"city_name"`
		StateProvince   *string `json:"state_province,omitempty"`
		Country         string  `json:"country"`
		CenterLatitude  float64 `json:"center_latitude"`
		CenterLongitude float64
		Description     string `json:"description"`
	}
	if err := json.Unmarshal([]byte(cleanTxt), &cityData); err != nil {
		span.RecordError(err)
		l.sendEvent(ctxWorker, eventCh, types.StreamEvent{
			Type:      types.EventTypeError,
			Error:     fmt.Sprintf("failed to parse city data JSON: %v", err),
			Timestamp: time.Now(),
			EventID:   uuid.New().String(),
		}, 3)
		resultCh <- types.GenAIResponse{Err: err}
		return
	}

	stateProvince := ""
	if cityData.StateProvince != nil {
		stateProvince = *cityData.StateProvince
	}

	result := types.GenAIResponse{
		City:            cityData.CityName,
		Country:         cityData.Country,
		StateProvince:   stateProvince,
		CityDescription: cityData.Description,
		Latitude:        cityData.CenterLatitude,
		Longitude:       cityData.CenterLongitude,
	}

	l.sendEvent(ctxWorker, eventCh, types.StreamEvent{
		Type:      types.EventTypeCityData,
		Data:      result,
		Timestamp: time.Now(),
		EventID:   uuid.New().String(),
	}, 3)
	resultCh <- result
}

// streamingGeneralPOIWorker generates general POIs with streaming updates
func (l *ServiceImpl) streamingGeneralPOIWorker(wg *sync.WaitGroup,
ctx context.Context, cityName string,
resultCh chan<- types.GenAIResponse,
eventCh chan<- types.StreamEvent,
userID uuid.UUID) {
defer wg.Done()

	ctx, span := otel.Tracer("LlmInteractionService").Start(ctx, "streamingGeneralPOIWorker", trace.WithAttributes(
		attribute.String("city.name", cityName),
	))
	defer span.End()

	l.sendEvent(ctx, eventCh, types.StreamEvent{
		Type:      types.EventTypeProgress,
		Data:      map[string]interface{}{"status": "generating_general_pois", "progress": 30},
		Timestamp: time.Now(),
		EventID:   uuid.New().String(),
	}, 3)

	prompt := getGeneralPOIPrompt(cityName)
	startTime := time.Now()
	var responseText strings.Builder

	// Try streaming
	iter, err := l.aiClient.GenerateContentStream(ctx, prompt, &genai.GenerateContentConfig{Temperature: genai.Ptr[float32](defaultTemperature)})
	if err == nil {
		for resp, err := range iter {
			if err != nil {
				span.RecordError(err)
				l.sendEvent(ctx, eventCh, types.StreamEvent{
					Type:      types.EventTypeError,
					Error:     fmt.Sprintf("streaming general POI error: %v", err),
					Timestamp: time.Now(),
					EventID:   uuid.New().String(),
				}, 3)
				resultCh <- types.GenAIResponse{Err: err}
				return
			}
			for _, cand := range resp.Candidates {
				if cand.Content != nil {
					for _, part := range cand.Content.Parts {
						if part.Text != "" {
							responseText.WriteString(string(part.Text))
							l.sendEvent(ctx, eventCh, types.StreamEvent{
								Type:      types.EventTypeGeneralPOI,
								Data:      map[string]string{"partial_poi_data": responseText.String()},
								Timestamp: time.Now(),
								EventID:   uuid.New().String(),
							}, 3)
						}
					}
				}
			}
		}
	} else {
		// Fallback to non-streaming
		l.logger.WarnContext(ctx, "Streaming general POIs failed, falling back to non-streaming", slog.Any("error", err))
		response, err := l.aiClient.GenerateResponse(ctx, prompt, &genai.GenerateContentConfig{Temperature: genai.Ptr[float32](defaultTemperature)})
		if err != nil {
			span.RecordError(err)
			l.sendEvent(ctx, eventCh, types.StreamEvent{
				Type:      types.EventTypeError,
				Error:     fmt.Sprintf("failed to generate general POIs: %v", err),
				Timestamp: time.Now(),
				EventID:   uuid.New().String(),
			}, 3)
			resultCh <- types.GenAIResponse{Err: err}
			return
		}
		for _, cand := range response.Candidates {
			if cand.Content != nil {
				for _, part := range cand.Content.Parts {
					if part.Text != "" {
						responseText.WriteString(string(part.Text))
					}
				}
			}
		}
		l.sendEvent(ctx, eventCh, types.StreamEvent{
			Type:      types.EventTypeGeneralPOI,
			Data:      map[string]string{"partial_poi_data": responseText.String()},
			Timestamp: time.Now(),
			EventID:   uuid.New().String(),
		}, 3)
	}

	fullText := responseText.String()
	if fullText == "" {
		err := fmt.Errorf("empty general POI response")
		span.RecordError(err)
		l.sendEvent(ctx, eventCh, types.StreamEvent{
			Type:      types.EventTypeError,
			Error:     err.Error(),
			Timestamp: time.Now(),
			EventID:   uuid.New().String(),
		}, 3)
		resultCh <- types.GenAIResponse{Err: err}
		return
	}

	// Save LLM interaction
	latencyMs := int(time.Since(startTime).Milliseconds())
	interaction := types.LlmInteraction{
		UserID:       userID, // No specific user for general POIs
		Prompt:       prompt,
		ResponseText: fullText,
		ModelUsed:    model,
		LatencyMs:    latencyMs,
		CityName:     cityName,
	}
	_, err = l.llmInteractionRepo.SaveInteraction(ctx, interaction)
	if err != nil {
		span.RecordError(err)
		l.sendEvent(ctx, eventCh, types.StreamEvent{
			Type:      types.EventTypeError,
			Error:     fmt.Sprintf("failed to save general POI interaction: %v", err),
			Timestamp: time.Now(),
			EventID:   uuid.New().String(),
		}, 3)
		resultCh <- types.GenAIResponse{Err: err}
		return
	}

	cleanTxt := cleanJSONResponse(fullText)
	var poiData struct {
		PointsOfInterest []types.POIDetailedInfo `json:"points_of_interest"`
	}
	if err := json.Unmarshal([]byte(cleanTxt), &poiData); err != nil {
		span.RecordError(err)
		l.sendEvent(ctx, eventCh, types.StreamEvent{
			Type:      types.EventTypeError,
			Error:     fmt.Sprintf("failed to parse general POI JSON: %v", err),
			Timestamp: time.Now(),
			EventID:   uuid.New().String(),
		}, 3)
		resultCh <- types.GenAIResponse{Err: err}
		return
	}

	result := types.GenAIResponse{
		GeneralPOI: poiData.PointsOfInterest,
	}

	l.sendEvent(ctx, eventCh, types.StreamEvent{
		Type:      types.EventTypeGeneralPOI,
		Data:      result.GeneralPOI,
		Timestamp: time.Now(),
		EventID:   uuid.New().String(),
	}, 3)
	resultCh <- result
}

// streamingPersonalizedPOIWorkerWithSemantics generates personalized POIs with semantic context and streaming updates
func (l *ServiceImpl) streamingPersonalizedPOIWorkerWithSemantics(wg *sync.WaitGroup, ctx context.Context, cityName string, userID, profileID uuid.UUID, resultCh chan<- types.GenAIResponse, eventCh chan<- types.StreamEvent, interestNames []string, tagsPromptPart, userPrefs string, semanticPOIs []types.POIDetailedInfo) {
defer wg.Done()

	ctx, span := otel.Tracer("LlmInteractionService").Start(ctx, "streamingPersonalizedPOIWorkerWithSemantics", trace.WithAttributes(
		attribute.String("city.name", cityName),
		attribute.String("user.id", userID.String()),
		attribute.String("profile.id", profileID.String()),
		attribute.Int("semantic_pois.count", len(semanticPOIs)),
	))
	defer span.End()

	l.sendEvent(ctx, eventCh, types.StreamEvent{
		Type: types.EventTypeProgress,
		Data: map[string]interface{}{
			"status":           "generating_semantic_personalized_pois",
			"progress":         50,
			"semantic_context": len(semanticPOIs) > 0,
		},
		Timestamp: time.Now(),
		EventID:   uuid.New().String(),
	}, 3)

	startTime := time.Now()
	prompt := l.getPersonalizedPOIWithSemanticContext(interestNames, cityName, tagsPromptPart, userPrefs, semanticPOIs)
	var responseText strings.Builder

	// Try streaming
	iter, err := l.aiClient.GenerateContentStream(ctx, prompt, &genai.GenerateContentConfig{Temperature: genai.Ptr[float32](defaultTemperature)})
	if err == nil {
		for resp, err := range iter {
			if err != nil {
				span.RecordError(err)
				l.sendEvent(ctx, eventCh, types.StreamEvent{
					Type:      types.EventTypeError,
					Error:     fmt.Sprintf("streaming semantic personalized POI error: %v", err),
					Timestamp: time.Now(),
					EventID:   uuid.New().String(),
				}, 3)
				resultCh <- types.GenAIResponse{Err: err}
				return
			}
			for _, cand := range resp.Candidates {
				if cand.Content != nil {
					for _, part := range cand.Content.Parts {
						if part.Text != "" {
							responseText.WriteString(string(part.Text))
							l.sendEvent(ctx, eventCh, types.StreamEvent{
								Type: types.EventTypePersonalizedPOI,
								Data: map[string]interface{}{
									"partial_poi_data":       responseText.String(),
									"semantic_enhanced":      true,
									"semantic_context_count": len(semanticPOIs),
								},
								Timestamp: time.Now(),
								EventID:   uuid.New().String(),
							}, 3)
						}
					}
				}
			}
		}
	} else {
		// Fallback to non-streaming
		l.logger.WarnContext(ctx, "Streaming semantic personalized POIs failed, falling back to non-streaming", slog.Any("error", err))
		response, err := l.aiClient.GenerateResponse(ctx, prompt, &genai.GenerateContentConfig{Temperature: genai.Ptr[float32](defaultTemperature)})
		if err != nil {
			span.RecordError(err)
			l.sendEvent(ctx, eventCh, types.StreamEvent{
				Type:      types.EventTypeError,
				Error:     fmt.Sprintf("failed to generate semantic personalized POIs: %v", err),
				Timestamp: time.Now(),
				EventID:   uuid.New().String(),
			}, 3)
			resultCh <- types.GenAIResponse{Err: err}
			return
		}
		for _, cand := range response.Candidates {
			if cand.Content != nil {
				for _, part := range cand.Content.Parts {
					if part.Text != "" {
						responseText.WriteString(string(part.Text))
					}
				}
			}
		}
		l.sendEvent(ctx, eventCh, types.StreamEvent{
			Type: types.EventTypePersonalizedPOI,
			Data: map[string]interface{}{
				"partial_poi_data":       responseText.String(),
				"semantic_enhanced":      true,
				"semantic_context_count": len(semanticPOIs),
			},
			Timestamp: time.Now(),
			EventID:   uuid.New().String(),
		}, 3)
	}

	fullText := responseText.String()
	if fullText == "" {
		err := fmt.Errorf("empty semantic personalized POI response")
		span.RecordError(err)
		l.sendEvent(ctx, eventCh, types.StreamEvent{
			Type:      types.EventTypeError,
			Error:     err.Error(),
			Timestamp: time.Now(),
			EventID:   uuid.New().String(),
		}, 3)
		resultCh <- types.GenAIResponse{Err: err}
		return
	}

	// Save LLM interaction with semantic metadata
	latencyMs := int(time.Since(startTime).Milliseconds())
	interaction := types.LlmInteraction{
		UserID:       userID,
		Prompt:       prompt,
		ResponseText: fullText,
		ModelUsed:    model,
		LatencyMs:    latencyMs,
		CityName:     cityName,
	}
	savedInteractionID, err := l.llmInteractionRepo.SaveInteraction(ctx, interaction)
	if err != nil {
		span.RecordError(err)
		l.sendEvent(ctx, eventCh, types.StreamEvent{
			Type:      types.EventTypeError,
			Error:     fmt.Sprintf("failed to save semantic personalized POI interaction: %v", err),
			Timestamp: time.Now(),
			EventID:   uuid.New().String(),
		}, 3)
		resultCh <- types.GenAIResponse{Err: err}
		return
	}

	cleanTxt := cleanJSONResponse(fullText)
	var itineraryData struct {
		ItineraryName      string                  `json:"itinerary_name"`
		OverallDescription string                  `json:"overall_description"`
		PointsOfInterest   []types.POIDetailedInfo `json:"points_of_interest"`
	}
	if err := json.Unmarshal([]byte(cleanTxt), &itineraryData); err != nil {
		span.RecordError(err)
		l.sendEvent(ctx, eventCh, types.StreamEvent{
			Type:      types.EventTypeError,
			Error:     fmt.Sprintf("failed to parse semantic personalized POI JSON: %v", err),
			Timestamp: time.Now(),
			EventID:   uuid.New().String(),
		}, 3)
		resultCh <- types.GenAIResponse{Err: err}
		return
	}

	result := types.GenAIResponse{
		ItineraryName:        itineraryData.ItineraryName,
		ItineraryDescription: itineraryData.OverallDescription,
		PersonalisedPOI:      itineraryData.PointsOfInterest,
		LlmInteractionID:     savedInteractionID,
	}

	l.sendEvent(ctx, eventCh, types.StreamEvent{
		Type: types.EventTypePersonalizedPOI,
		Data: map[string]interface{}{
			"result":                 result,
			"semantic_enhanced":      true,
			"semantic_context_count": len(semanticPOIs),
		},
		Timestamp: time.Now(),
		EventID:   uuid.New().String(),
	}, 3)
	resultCh <- result
}

This is the code from my main that works. Analyse now with my current llm

when I hit http://localhost:8000/api/v1/llm/prompt-response/chat/sessions/stream/6670676e-d5a9-4b77-96a9-461e267553e9

data: {"type":"start","message":"","data":{"cache_key":"5cbe3f548491c939472e491c75a1f5ab","city":"Madrid","domain":"itinerary","session_id":"f273f102-28c2-4dbf-9122-2b4b06db139d"},"timestamp":"2025-07-04T22:32:06.875154+01:00","event_id":"1e70651d-bd22-49d2-a249-ab886df160b3"}

data: {"type":"chunk","message":"","data":{"cache_key":"5cbe3f548491c939472e491c75a1f5ab_city_data","cache_used":true,"chunk":"```","domain":"itinerary","part":"city_data"},"timestamp":"2025-07-04T22:32:07.442007+01:00","event_id":"6fe511e1-916e-4751-8a48-f5f93a8bcd82"}

data: {"type":"chunk","message":"","data":{"cache_key":"5cbe3f548491c939472e491c75a1f5ab_city_data","cache_used":true,"chunk":"json\n{\n    \"city\": \"Madrid\",\n    \"country\":","domain":"itinerary","part":"city_data"},"timestamp":"2025-07-04T22:32:07.523934+01:00","event_id":"4a9e34ee-2c0a-44c7-9da4-5be2a6750910"}

data: {"type":"chunk","message":"","data":{"cache_key":"5cbe3f548491c939472e491c75a1f5ab_city_data","cache_used":true,"chunk":" \"Spain\",\n    \"state_province\": \"Madrid\",\n    \"","domain":"itinerary","part":"city_data"},"timestamp":"2025-07-04T22:32:07.580584+01:00","event_id":"25fd4a9a-8427-42d5-8ff8-cca9c59aa47e"}

data: {"type":"chunk","message":"","data":{"cache_key":"5cbe3f548491c939472e491c75a1f5ab_general_pois","cache_used":true,"chunk":"```","domain":"itinerary","part":"general_pois"},"timestamp":"2025-07-04T22:32:07.626484+01:00","event_id":"3846504f-9ef2-4e12-b8df-e855ac35003d"}

data: {"type":"chunk","message":"","data":{"cache_key":"5cbe3f548491c939472e491c75a1f5ab_general_pois","cache_used":true,"chunk":"json\n{","domain":"itinerary","part":"general_pois"},"timestamp":"2025-07-04T22:32:07.631028+01:00","event_id":"f4950ea9-d8a4-4959-9554-d2fab6cbe0a8"}

data: {"type":"chunk","message":"","data":{"cache_key":"5cbe3f548491c939472e491c75a1f5ab_city_data","cache_used":true,"chunk":"description\": \"Madrid, the vibrant capital of Spain, is a city that seamlessly blends historical grandeur with modern dynamism. Renowned for its rich cultural heritage, Madrid","domain":"itinerary","part":"city_data"},"timestamp":"2025-07-04T22:32:07.730376+01:00","event_id":"5d3c3957-4d1f-4001-a0bf-47eda789293e"}

data: {"type":"chunk","message":"","data":{"cache_key":"5cbe3f548491c939472e491c75a1f5ab_general_pois","cache_used":true,"chunk":"\n    \"points_of_interest\": [\n        {\n            \"name\":","domain":"itinerary","part":"general_pois"},"timestamp":"2025-07-04T22:32:07.750899+01:00","event_id":"c515b4c4-5116-4930-9049-53d2ff37e52c"}

data: {"type":"chunk","message":"","data":{"cache_key":"5cbe3f548491c939472e491c75a1f5ab_itinerary","cache_used":true,"chunk":"```","domain":"itinerary","part":"itinerary"},"timestamp":"2025-07-04T22:32:07.804305+01:00","event_id":"d1478f47-fe36-4f0e-9187-472ca819022a"}

data: {"type":"chunk","message":"","data":{"cache_key":"5cbe3f548491c939472e491c75a1f5ab_general_pois","cache_used":true,"chunk":" \"Prado Museum\",\n            \"latitude\": 40.413","domain":"itinerary","part":"general_pois"},"timestamp":"2025-07-04T22:32:07.85882+01:00","event_id":"b71eb9a5-1e02-43fe-9452-d4faecaa7fa9"}

data: {"type":"chunk","message":"","data":{"cache_key":"5cbe3f548491c939472e491c75a1f5ab_city_data","cache_used":true,"chunk":" boasts world-class museums like the Prado, Reina Sofia, and Thyssen-Bornemisza, showcasing masterpieces spanning centuries. The city's stunning architecture, from","domain":"itinerary","part":"city_data"},"timestamp":"2025-07-04T22:32:07.885334+01:00","event_id":"1055050f-63cd-4abe-8184-12561a4993cb"}

data: {"type":"chunk","message":"","data":{"cache_key":"5cbe3f548491c939472e491c75a1f5ab_itinerary","cache_used":true,"chunk":"json\n{\n  \"itinerary_name\": \"Madrid Tapestry: A Flexible","domain":"itinerary","part":"itinerary"},"timestamp":"2025-07-04T22:32:07.908992+01:00","event_id":"d969b0e2-25fb-4390-9613-cce26b509a04"}

data: {"type":"chunk","message":"","data":{"cache_key":"5cbe3f548491c939472e491c75a1f5ab_itinerary","cache_used":true,"chunk":" Exploration\",\n  \"overall_description\": \"This itinerary offers a flexible framework for exploring Madrid","domain":"itinerary","part":"itinerary"},"timestamp":"2025-07-04T22:32:07.982989+01:00","event_id":"41388291-6478-42a1-b406-9fa328907031"}

data: {"type":"chunk","message":"","data":{"cache_key":"5cbe3f548491c939472e491c75a1f5ab_general_pois","cache_used":true,"chunk":"8,\n            \"longitude\": -3.6922,\n            \"category\": \"Museum\",\n            \"description_poi\": \"One of the","domain":"itinerary","part":"general_pois"},"timestamp":"2025-07-04T22:32:08.002654+01:00","event_id":"6bbfaa80-a92d-4ea4-9c6f-4365a2bde4df"}

data: {"type":"chunk","message":"","data":{"cache_key":"5cbe3f548491c939472e491c75a1f5ab_city_data","cache_used":true,"chunk":" the Royal Palace to the Plaza Mayor, reflects its regal past. Madrid is also a culinary paradise, offering everything from traditional tapas to innovative gastronomy. Its lively nightlife, characterized by bustling bars and clubs, ensures an unforgettable experience for visitors. The city","domain":"itinerary","part":"city_data"},"timestamp":"2025-07-04T22:32:08.160034+01:00","event_id":"2f439158-8af3-4f20-b666-a0e586728b07"}

data: {"type":"chunk","message":"","data":{"cache_key":"5cbe3f548491c939472e491c75a1f5ab_general_pois","cache_used":true,"chunk":" world's greatest art museums, housing masterpieces by Spanish and European masters like Goya, Velázquez, and El Greco.\",\n            \"address\": \"Calle","domain":"itinerary","part":"general_pois"},"timestamp":"2025-07-04T22:32:08.175647+01:00","event_id":"12b5207d-a5f3-432e-9600-5afe5508969e"}

data: {"type":"chunk","message":"","data":{"cache_key":"5cbe3f548491c939472e491c75a1f5ab_itinerary","cache_used":true,"chunk":", catering to a traveler with no strong preferences regarding budget, pace, or specific vibes. It focuses on iconic landmarks and cultural experiences within a 5km radius of","domain":"itinerary","part":"itinerary"},"timestamp":"2025-07-04T22:32:08.187767+01:00","event_id":"ab89b747-7836-4946-819f-885ee1cf33e2"}

data: {"type":"chunk","message":"","data":{"cache_key":"5cbe3f548491c939472e491c75a1f5ab_itinerary","cache_used":true,"chunk":" the city center, allowing for spontaneity and adaptation to your energy levels. The itinerary includes a mix of historical sites, artistic hubs, and culinary delights, providing a well","domain":"itinerary","part":"itinerary"},"timestamp":"2025-07-04T22:32:08.410667+01:00","event_id":"b04aa211-7960-42a9-897b-363dd499939d"}

data: {"type":"chunk","message":"","data":{"cache_key":"5cbe3f548491c939472e491c75a1f5ab_city_data","cache_used":true,"chunk":"'s numerous parks and gardens, such as Retiro Park, provide tranquil escapes from the urban bustle.\",\n    \"center_latitude\": 40.4168,\n    \"center_longitude\": -3.7","domain":"itinerary","part":"city_data"},"timestamp":"2025-07-04T22:32:08.414808+01:00","event_id":"4090e1f9-4318-4050-9067-c2178d4f6b3b"}

data: {"type":"chunk","message":"","data":{"cache_key":"5cbe3f548491c939472e491c75a1f5ab_general_pois","cache_used":true,"chunk":" Ruiz de Alarcón, 23, 28014 Madrid\",\n            \"website\": \"https://www.museodelprado.es/\",\n            \"opening_hours\": {\n                \"Mon-","domain":"itinerary","part":"general_pois"},"timestamp":"2025-07-04T22:32:08.430993+01:00","event_id":"2d5cae2b-15c1-40f2-ba1e-75d47cd1f1fd"}

data: {"type":"chunk","message":"","data":{"cache_key":"5cbe3f548491c939472e491c75a1f5ab_general_pois","cache_used":true,"chunk":"Sat\": \"10:00-20:00\",\n                \"Sun, Holidays\": \"10:00-19:00\"\n            }\n        },\n        {\n            \"name\": \"Re","domain":"itinerary","part":"general_pois"},"timestamp":"2025-07-04T22:32:08.649943+01:00","event_id":"11ef553c-ee58-428e-871b-65b7b1262e6b"}

data: {"type":"chunk","message":"","data":{"cache_key":"5cbe3f548491c939472e491c75a1f5ab_itinerary","cache_used":true,"chunk":"-rounded introduction to the vibrant capital of Spain. Remember to check opening hours and availability in advance, especially during peak season. Enjoy the freedom to tailor each day to your interests and discover the magic of Madrid at your own pace!\",\n  \"points","domain":"itinerary","part":"itinerary"},"timestamp":"2025-07-04T22:32:08.728686+01:00","event_id":"342ade50-222e-44cf-8a1e-0ccd104c4c2f"}

data: {"type":"chunk","message":"","data":{"cache_key":"5cbe3f548491c939472e491c75a1f5ab_city_data","cache_used":true,"chunk":"038,\n    \"population\": \"Approximately 3.3 million (city proper), 6.7 million (metropolitan area)\",\n    \"area\": \"604.3 km² (city proper)\",\n    \"timezone\": \"CET (Central European Time) / CEST (Central European Summer Time)\",","domain":"itinerary","part":"city_data"},"timestamp":"2025-07-04T22:32:08.773563+01:00","event_id":"d3219ada-66ac-430e-b7a7-053e40b3bf71"}

data: {"type":"chunk","message":"","data":{"cache_key":"5cbe3f548491c939472e491c75a1f5ab_general_pois","cache_used":true,"chunk":"ina Sofía Museum\",\n            \"latitude\": 40.4116,\n            \"longitude\": -3.6957,\n            \"category\": \"Museum\",\n            \"description_poi\": \"Madrid's modern art museum, best known for housing Picasso's 'Guernica'.","domain":"itinerary","part":"general_pois"},"timestamp":"2025-07-04T22:32:08.957759+01:00","event_id":"07a80fbb-cfea-459e-bf06-642d74556863"}

data: {"type":"chunk","message":"","data":{"cache_key":"5cbe3f548491c939472e491c75a1f5ab_itinerary","cache_used":true,"chunk":"_of_interest\": [\n    {\n      \"name\": \"Museo Nacional del Prado\",\n      \"latitude\": 40.4138,\n      \"longitude\": -3.6922,\n      \"","domain":"itinerary","part":"itinerary"},"timestamp":"2025-07-04T22:32:08.987148+01:00","event_id":"19c200c2-c1d4-48c9-a523-652635f99312"}

data: {"type":"chunk","message":"","data":{"cache_key":"5cbe3f548491c939472e491c75a1f5ab_city_data","cache_used":true,"chunk":"\n    \"language\": \"Spanish\",\n    \"weather\": \"Madrid has a Mediterranean climate with hot, dry summers and cool, wet winters. Spring and autumn offer pleasant temperatures and are ideal for visiting.\",\n    \"attractions\": \"Prado Museum, Reina Sofia Museum, Royal Palace of Madrid, Plaza Mayor","domain":"itinerary","part":"city_data"},"timestamp":"2025-07-04T22:32:09.096563+01:00","event_id":"22000952-34eb-43d7-abda-08a6487d3979"}

data: {"type":"chunk","message":"","data":{"cache_key":"5cbe3f548491c939472e491c75a1f5ab_itinerary","cache_used":true,"chunk":"category\": \"Museum\",\n      \"description_poi\": \"One of the world's greatest art museums, housing masterpieces by Spanish masters like Goya, Velázquez, and El Greco, as well as works by international artists.\",\n      \"address\": \"Calle Ruiz de Alarcón, 23, 2","domain":"itinerary","part":"itinerary"},"timestamp":"2025-07-04T22:32:09.286149+01:00","event_id":"17658946-c717-4915-968e-e961585c19fb"}

data: {"type":"chunk","message":"","data":{"cache_key":"5cbe3f548491c939472e491c75a1f5ab_general_pois","cache_used":true,"chunk":" Features a wide collection of 20th-century Spanish art.\",\n            \"address\": \"Calle de Santa Isabel, 52, 28012 Madrid\",\n            \"website\": \"https://www.museoreinasofia.es/\",\n            \"opening_hours\": {\n                \"","domain":"itinerary","part":"general_pois"},"timestamp":"2025-07-04T22:32:09.335836+01:00","event_id":"7871f31b-6fc1-497d-9011-27c92e45fc2a"}

data: {"type":"chunk","message":"","data":{"cache_key":"5cbe3f548491c939472e491c75a1f5ab_city_data","cache_used":true,"chunk":", Retiro Park, Temple of Debod, Gran Vía, Puerta del Sol, Santiago Bernabéu Stadium, El Rastro Flea Market.\",\n    \"history\": \"Madrid's history dates back to the 9th century when a Moorish fortress was built. It became the capital of Spain","domain":"itinerary","part":"city_data"},"timestamp":"2025-07-04T22:32:09.441481+01:00","event_id":"62297de3-96dd-4a05-858c-f7d2d0cc336b"}

data: {"type":"chunk","message":"","data":{"cache_key":"5cbe3f548491c939472e491c75a1f5ab_itinerary","cache_used":true,"chunk":"8014 Madrid\",\n      \"website\": \"https://www.museodelprado.es/en\",\n      \"opening_hours\": \"{\\\"Mon\\\": \\\"10:00-20:00\\\", \\\"Tue\\\": \\\"10:00-20:00","domain":"itinerary","part":"itinerary"},"timestamp":"2025-07-04T22:32:09.612725+01:00","event_id":"bccf9e00-b36a-430f-9c7f-89ef5b79ff83"}

data: {"type":"chunk","message":"","data":{"cache_key":"5cbe3f548491c939472e491c75a1f5ab_general_pois","cache_used":true,"chunk":"Mon, Wed-Sat\": \"10:00-21:00\",\n                \"Sun\": \"10:00-19:00\",\n                \"Tue\": \"Closed\"\n            }\n        },\n        {\n            \"name\": \"Royal Palace of Madrid\",","domain":"itinerary","part":"general_pois"},"timestamp":"2025-07-04T22:32:09.614221+01:00","event_id":"6540a0f1-736d-43bf-97f7-13fa90533d75"}

data: {"type":"chunk","message":"","data":{"cache_key":"5cbe3f548491c939472e491c75a1f5ab_city_data","cache_used":true,"chunk":" in 1561 under King Philip II. The city experienced significant growth and development during the Spanish Golden Age and has since played a crucial role in Spanish history, including the Spanish Civil War. Today, Madrid is a major economic, cultural, and political center in Europe.\"\n}\n```","domain":"itinerary","part":"city_data"},"timestamp":"2025-07-04T22:32:09.738564+01:00","event_id":"96a383fa-b654-4f48-b76d-db48ffde8857"}

data: {"type":"chunk","message":"","data":{"cache_key":"5cbe3f548491c939472e491c75a1f5ab_itinerary","cache_used":true,"chunk":"\\\", \\\"Wed\\\": \\\"10:00-20:00\\\", \\\"Thu\\\": \\\"10:00-20:00\\\", \\\"Fri\\\": \\\"10:00-20:00\\\", \\\"Sat\\\": \\\"10:00-20:00","domain":"itinerary","part":"itinerary"},"timestamp":"2025-07-04T22:32:09.846777+01:00","event_id":"dbd25536-da46-433e-bd55-346c311e8513"}

data: {"type":"chunk","message":"","data":{"cache_key":"5cbe3f548491c939472e491c75a1f5ab_general_pois","cache_used":true,"chunk":"\n            \"latitude\": 40.4187,\n            \"longitude\": -3.7140,\n            \"category\": \"Historical Site\",\n            \"description_poi\": \"The official residence of the Spanish Royal Family, though it is mainly used for state ceremonies. Features opulent","domain":"itinerary","part":"general_pois"},"timestamp":"2025-07-04T22:32:09.964153+01:00","event_id":"7c44f9b5-fa75-4194-b441-a69d92dfe286"}

data: {"type":"chunk","message":"","data":{"cache_key":"5cbe3f548491c939472e491c75a1f5ab_itinerary","cache_used":true,"chunk":"\\\", \\\"Sun\\\": \\\"10:00-19:00\\\"}\",\n      \"distance\": 0.5\n    },\n    {\n      \"name\": \"Parque del Retiro\",\n      \"latitude\": 40.4167,\n      \"longitude\": -3","domain":"itinerary","part":"itinerary"},"timestamp":"2025-07-04T22:32:10.13614+01:00","event_id":"de7833d4-3df1-4519-b4b2-4345f6c444d8"}

data: {"type":"chunk","message":"","data":{"cache_key":"5cbe3f548491c939472e491c75a1f5ab_general_pois","cache_used":true,"chunk":" rooms and stunning architecture.\",\n            \"address\": \"Plaza de la Armería, s/n, 28071 Madrid\",\n            \"website\": \"https://www.patrimonionacional.es/real-sitio/palacio-real-de-madrid\",\n            \"opening","domain":"itinerary","part":"general_pois"},"timestamp":"2025-07-04T22:32:10.263938+01:00","event_id":"cca26269-e079-4bbc-9215-056e097e6299"}

data: {"type":"chunk","message":"","data":{"cache_key":"5cbe3f548491c939472e491c75a1f5ab_itinerary","cache_used":true,"chunk":".6843,\n      \"category\": \"Park\",\n      \"description_poi\": \"A sprawling urban oasis offering boating on the lake, beautiful gardens, historical monuments, and street performers. Perfect for a relaxing stroll or a picnic.\",\n      \"address\": \"Plaza de la Independencia, ","domain":"itinerary","part":"itinerary"},"timestamp":"2025-07-04T22:32:10.514618+01:00","event_id":"16df6509-fee1-4b9c-bab9-076fd91764fe"}

data: {"type":"chunk","message":"","data":{"cache_key":"5cbe3f548491c939472e491c75a1f5ab_general_pois","cache_used":true,"chunk":"_hours\": {\n                \"Oct-Mar\": {\n                    \"Mon-Sun\": \"10:00-18:00\"\n                },\n                \"Apr-Sep\": {\n                    \"Mon-Sun\": \"10:00-19:00\"\n                }","domain":"itinerary","part":"general_pois"},"timestamp":"2025-07-04T22:32:10.582815+01:00","event_id":"abf82d2b-0fb0-4269-ad26-f73fbba46adf"}

data: {"type":"chunk","message":"","data":{"cache_key":"5cbe3f548491c939472e491c75a1f5ab_itinerary","cache_used":true,"chunk":"7, 28001 Madrid\",\n      \"website\": \"https://www.esmadrid.com/en/tourist-information/parque-del-retiro\",\n      \"opening_hours\": \"{\\\"Mon\\\": \\\"06:00-00:00","domain":"itinerary","part":"itinerary"},"timestamp":"2025-07-04T22:32:10.836092+01:00","event_id":"1c23cd8b-14a3-4d1f-839e-5b55ba1387e3"}

data: {"type":"chunk","message":"","data":{"cache_key":"5cbe3f548491c939472e491c75a1f5ab_general_pois","cache_used":true,"chunk":"\n            }\n        },\n        {\n            \"name\": \"Retiro Park\",\n            \"latitude\": 40.4140,\n            \"longitude\": -3.6835,\n            \"category\": \"Park\",\n            \"description_poi\": \"A large and","domain":"itinerary","part":"general_pois"},"timestamp":"2025-07-04T22:32:10.870509+01:00","event_id":"3d814e15-11fc-4d22-ac69-5d0371f15b30"}

data: {"type":"chunk","message":"","data":{"cache_key":"5cbe3f548491c939472e491c75a1f5ab_itinerary","cache_used":true,"chunk":"\\\", \\\"Tue\\\": \\\"06:00-00:00\\\", \\\"Wed\\\": \\\"06:00-00:00\\\", \\\"Thu\\\": \\\"06:00-00:00\\\", \\\"Fri\\\": \\\"06:00-00:00","domain":"itinerary","part":"itinerary"},"timestamp":"2025-07-04T22:32:11.090054+01:00","event_id":"7a0a2348-0eb0-4702-8c55-55c45e1440d8"}

data: {"type":"chunk","message":"","data":{"cache_key":"5cbe3f548491c939472e491c75a1f5ab_general_pois","cache_used":true,"chunk":" beautiful park in the heart of Madrid, perfect for relaxing, boating on the lake, or visiting the Crystal Palace.\",\n            \"address\": \"Plaza de la Independencia, 7, 28001 Madrid\",\n            \"website\": \"https://www.esmadrid.com/en","domain":"itinerary","part":"general_pois"},"timestamp":"2025-07-04T22:32:11.196599+01:00","event_id":"a94ab469-9380-4473-bb86-29858ad80f7c"}

data: {"type":"chunk","message":"","data":{"cache_key":"5cbe3f548491c939472e491c75a1f5ab_itinerary","cache_used":true,"chunk":"\\\", \\\"Sat\\\": \\\"06:00-00:00\\\", \\\"Sun\\\": \\\"06:00-00:00\\\"}\",\n      \"distance\": 1.2\n    },\n    {\n      \"name\": \"Plaza Mayor\",\n      \"latitude\": 40","domain":"itinerary","part":"itinerary"},"timestamp":"2025-07-04T22:32:11.362959+01:00","event_id":"73b30f96-3c90-4dd5-a5a5-849df0aece5e"}

data: {"type":"chunk","message":"","data":{"cache_key":"5cbe3f548491c939472e491c75a1f5ab_general_pois","cache_used":true,"chunk":"/tourist-information/parque-del-retiro\",\n            \"opening_hours\": {\n                \"Mon-Sun\": \"06:00-00:00\"\n            }\n        },\n        {\n            \"name\": \"Plaza Mayor\",\n            \"latitude\":","domain":"itinerary","part":"general_pois"},"timestamp":"2025-07-04T22:32:11.517486+01:00","event_id":"7f075181-95ca-48ed-bb50-e55d35342526"}

data: {"type":"chunk","message":"","data":{"cache_key":"5cbe3f548491c939472e491c75a1f5ab_itinerary","cache_used":true,"chunk":".4156,\n      \"longitude\": -3.7072,\n      \"category\": \"Plaza\",\n      \"description_poi\": \"A grand, historic square surrounded by beautiful buildings and arcades. A popular spot for people-watching and enjoying the atmosphere of Madrid.\",\n      ","domain":"itinerary","part":"itinerary"},"timestamp":"2025-07-04T22:32:11.723867+01:00","event_id":"32f4e359-bfab-43c7-b726-03ce05f3332d"}

data: {"type":"chunk","message":"","data":{"cache_key":"5cbe3f548491c939472e491c75a1f5ab_general_pois","cache_used":true,"chunk":" 40.4156,\n            \"longitude\": -3.7072,\n            \"category\": \"Historical Site\",\n            \"description_poi\": \"A grand and historic square surrounded by beautiful buildings, often hosting events and markets.\",\n            \"address\": \"Plaza Mayor","domain":"itinerary","part":"general_pois"},"timestamp":"2025-07-04T22:32:11.830866+01:00","event_id":"405701d3-550e-4958-a8a7-1dc8a5b4d194"}

data: {"type":"chunk","message":"","data":{"cache_key":"5cbe3f548491c939472e491c75a1f5ab_itinerary","cache_used":true,"chunk":"\"address\": \"Plaza Mayor, 28012 Madrid\",\n      \"website\": \"https://www.esmadrid.com/en/tourist-information/plaza-mayor\",\n      \"opening_hours\": \"{\\\"Mon\\\": \\\"00:00-00:","domain":"itinerary","part":"itinerary"},"timestamp":"2025-07-04T22:32:12.011397+01:00","event_id":"0c276858-9bac-474d-8eee-4a5910208588"}

data: {"type":"chunk","message":"","data":{"cache_key":"5cbe3f548491c939472e491c75a1f5ab_general_pois","cache_used":true,"chunk":", 28012 Madrid\",\n            \"website\": \"https://www.esmadrid.com/en/tourist-information/plaza-mayor\",\n            \"opening_hours\": {}\n        },\n        {\n            \"name\": \"Puerta del Sol\",\n            \"latitude","domain":"itinerary","part":"general_pois"},"timestamp":"2025-07-04T22:32:12.115459+01:00","event_id":"687b0e39-3ce5-4b66-99b0-4e7db2fb4603"}

data: {"type":"chunk","message":"","data":{"cache_key":"5cbe3f548491c939472e491c75a1f5ab_itinerary","cache_used":true,"chunk":"00\\\", \\\"Tue\\\": \\\"00:00-00:00\\\", \\\"Wed\\\": \\\"00:00-00:00\\\", \\\"Thu\\\": \\\"00:00-00:00\\\", \\\"Fri\\\": \\\"00:00-00:","domain":"itinerary","part":"itinerary"},"timestamp":"2025-07-04T22:32:12.254709+01:00","event_id":"4237bcd2-53dc-4061-bba0-4f5c5386af96"}

data: {"type":"chunk","message":"","data":{"cache_key":"5cbe3f548491c939472e491c75a1f5ab_general_pois","cache_used":true,"chunk":"\": 40.4167,\n            \"longitude\": -3.7033,\n            \"category\": \"Historical Site\",\n            \"description_poi\": \"A bustling square considered the center of Spain, marked by Kilometre Zero and the iconic clock tower.\",\n            \"address\": \"","domain":"itinerary","part":"general_pois"},"timestamp":"2025-07-04T22:32:12.471849+01:00","event_id":"e94d3a82-aef0-4a90-95ab-72e2849854dc"}

data: {"type":"chunk","message":"","data":{"cache_key":"5cbe3f548491c939472e491c75a1f5ab_itinerary","cache_used":true,"chunk":"00\\\", \\\"Sat\\\": \\\"00:00-00:00\\\", \\\"Sun\\\": \\\"00:00-00:00\\\"}\",\n      \"distance\": 1.5\n    },\n    {\n      \"name\": \"Mercado de San Miguel\",\n      \"","domain":"itinerary","part":"itinerary"},"timestamp":"2025-07-04T22:32:12.527111+01:00","event_id":"3213c5a8-e774-450c-a98e-48be1ed0f3ac"}

data: {"type":"chunk","message":"","data":{"cache_key":"5cbe3f548491c939472e491c75a1f5ab_general_pois","cache_used":true,"chunk":"Puerta del Sol, 28013 Madrid\",\n            \"website\": \"https://www.esmadrid.com/en/tourist-information/puerta-del-sol\",\n            \"opening_hours\": {}\n        },\n        {\n            \"name\": \"Temple","domain":"itinerary","part":"general_pois"},"timestamp":"2025-07-04T22:32:12.746124+01:00","event_id":"e0ee7647-4ce1-4972-a750-c439315d551b"}

data: {"type":"chunk","message":"","data":{"cache_key":"5cbe3f548491c939472e491c75a1f5ab_itinerary","cache_used":true,"chunk":"latitude\": 40.4154,\n      \"longitude\": -3.7093,\n      \"category\": \"Market\",\n      \"description_poi\": \"A vibrant food market offering a wide variety of Spanish tapas, drinks, and gourmet products. A great place to sample local","domain":"itinerary","part":"itinerary"},"timestamp":"2025-07-04T22:32:12.894932+01:00","event_id":"4d9451e0-f267-438a-aafc-6298ead0163c"}

data: {"type":"chunk","message":"","data":{"cache_key":"5cbe3f548491c939472e491c75a1f5ab_general_pois","cache_used":true,"chunk":" of Debod\",\n            \"latitude\": 40.4221,\n            \"longitude\": -3.7182,\n            \"category\": \"Historical Site\",\n            \"description_poi\": \"An ancient Egyptian temple gifted to Spain, offering stunning sunset views over the city.\",\n            \"","domain":"itinerary","part":"general_pois"},"timestamp":"2025-07-04T22:32:13.15693+01:00","event_id":"a51abdca-01a3-42af-acbf-9096bfe7ce46"}

data: {"type":"chunk","message":"","data":{"cache_key":"5cbe3f548491c939472e491c75a1f5ab_itinerary","cache_used":true,"chunk":" cuisine and experience the lively atmosphere.\",\n      \"address\": \"Plaza de San Miguel, s/n, 28005 Madrid\",\n      \"website\": \"https://www.mercadodesanmiguel.es/en/\",\n      \"opening_hours\": \"{\\\"Mon\\\": \\\"","domain":"itinerary","part":"itinerary"},"timestamp":"2025-07-04T22:32:13.212887+01:00","event_id":"9b1e1919-b938-4d49-97f9-c8179dcd077f"}

data: {"type":"chunk","message":"","data":{"cache_key":"5cbe3f548491c939472e491c75a1f5ab_general_pois","cache_used":true,"chunk":"address\": \"Calle Ferraz, 1, 28008 Madrid\",\n            \"website\": \"https://www.esmadrid.com/en/tourist-information/templo-de-debod\",\n            \"opening_hours\": {\n                \"Oct-Mar\": {\n                    ","domain":"itinerary","part":"general_pois"},"timestamp":"2025-07-04T22:32:13.464731+01:00","event_id":"d30196ef-b04b-4a51-b338-6a331aef235c"}

data: {"type":"chunk","message":"","data":{"cache_key":"5cbe3f548491c939472e491c75a1f5ab_itinerary","cache_used":true,"chunk":"10:00-00:00\\\", \\\"Tue\\\": \\\"10:00-00:00\\\", \\\"Wed\\\": \\\"10:00-00:00\\\", \\\"Thu\\\": \\\"10:00-00:00\\\", \\\"Fri\\\": \\\"10:","domain":"itinerary","part":"itinerary"},"timestamp":"2025-07-04T22:32:13.500622+01:00","event_id":"760e31b6-b408-4036-8d24-13ae64be3a64"}

data: {"type":"chunk","message":"","data":{"cache_key":"5cbe3f548491c939472e491c75a1f5ab_itinerary","cache_used":true,"chunk":"00-01:00\\\", \\\"Sat\\\": \\\"10:00-01:00\\\", \\\"Sun\\\": \\\"10:00-00:00\\\"}\",\n      \"distance\": 1.7\n    },\n    {\n      \"name\": \"Pu","domain":"itinerary","part":"itinerary"},"timestamp":"2025-07-04T22:32:13.738012+01:00","event_id":"07e1109e-536e-4ec4-ad8e-0972e4615cdf"}

data: {"type":"chunk","message":"","data":{"cache_key":"5cbe3f548491c939472e491c75a1f5ab_general_pois","cache_used":true,"chunk":"\"Mon-Fri\": \"10:00-14:00, 16:00-18:00\",\n                    \"Sat, Sun, Holidays\": \"09:30-20:00\"\n                },\n                \"Apr-Sep\": {","domain":"itinerary","part":"general_pois"},"timestamp":"2025-07-04T22:32:13.791185+01:00","event_id":"19019363-ebd6-4772-9d53-ce29a71c1090"}

data: {"type":"chunk","message":"","data":{"cache_key":"5cbe3f548491c939472e491c75a1f5ab_general_pois","cache_used":true,"chunk":"\n                    \"Mon-Fri\": \"10:00-14:00, 18:00-20:00\",\n                    \"Sat, Sun, Holidays\": \"09:30-20:00\"\n                }\n            }\n        },\n        {","domain":"itinerary","part":"general_pois"},"timestamp":"2025-07-04T22:32:14.063525+01:00","event_id":"455d64be-f9c8-4443-9bce-687d95ddf763"}

data: {"type":"chunk","message":"","data":{"cache_key":"5cbe3f548491c939472e491c75a1f5ab_itinerary","cache_used":true,"chunk":"erta del Sol\",\n      \"latitude\": 40.4167,\n      \"longitude\": -3.7033,\n      \"category\": \"Plaza\",\n      \"description_poi\": \"A bustling central square and a major transportation hub. Home to the Kilómetro Cero,","domain":"itinerary","part":"itinerary"},"timestamp":"2025-07-04T22:32:14.064855+01:00","event_id":"c991894b-1e96-451d-aa63-c52bc63c7f4e"}

data: {"type":"chunk","message":"","data":{"cache_key":"5cbe3f548491c939472e491c75a1f5ab_general_pois","cache_used":true,"chunk":"\n            \"name\": \"Gran Vía\",\n            \"latitude\": 40.4196,\n            \"longitude\": -3.7053,\n            \"category\": \"Street\",\n            \"description_poi\": \"Madrid's most famous street, known for its theaters,","domain":"itinerary","part":"general_pois"},"timestamp":"2025-07-04T22:32:14.385478+01:00","event_id":"b8b0e83b-45e6-450e-aee2-998eef787c45"}

data: {"type":"chunk","message":"","data":{"cache_key":"5cbe3f548491c939472e491c75a1f5ab_itinerary","cache_used":true,"chunk":" the symbolic center of Spain, and the iconic Tío Pepe sign.\",\n      \"address\": \"Puerta del Sol, 28013 Madrid\",\n      \"website\": \"https://www.esmadrid.com/en/tourist-information/puerta-del-sol\",\n      \"opening","domain":"itinerary","part":"itinerary"},"timestamp":"2025-07-04T22:32:14.411918+01:00","event_id":"3b32d0f2-06e2-4fc6-9394-fbb71f98b8c0"}

data: {"type":"chunk","message":"","data":{"cache_key":"5cbe3f548491c939472e491c75a1f5ab_general_pois","cache_used":true,"chunk":" shops, and vibrant atmosphere.\",\n            \"address\": \"Gran Vía, Madrid\",\n            \"website\": null,\n            \"opening_hours\": {}\n        },\n        {\n            \"name\": \"Mercado de San Miguel\",\n            \"latitude\": 40.4151","domain":"itinerary","part":"general_pois"},"timestamp":"2025-07-04T22:32:14.626307+01:00","event_id":"5f6e5398-b651-4dbf-ad2c-93dd8e0a54e1"}

data: {"type":"chunk","message":"","data":{"cache_key":"5cbe3f548491c939472e491c75a1f5ab_itinerary","cache_used":true,"chunk":"_hours\": \"{\\\"Mon\\\": \\\"00:00-00:00\\\", \\\"Tue\\\": \\\"00:00-00:00\\\", \\\"Wed\\\": \\\"00:00-00:00\\\", \\\"Thu\\\": \\\"00:00-00","domain":"itinerary","part":"itinerary"},"timestamp":"2025-07-04T22:32:14.657285+01:00","event_id":"206b7cb2-746a-41e7-8e68-f3d962052583"}

data: {"type":"chunk","message":"","data":{"cache_key":"5cbe3f548491c939472e491c75a1f5ab_general_pois","cache_used":true,"chunk":",\n            \"longitude\": -3.7095,\n            \"category\": \"Market\",\n            \"description_poi\": \"A historic market offering a wide variety of Spanish tapas and delicacies.\",\n            \"address\": \"Plaza de San Miguel, s/n, 2800","domain":"itinerary","part":"general_pois"},"timestamp":"2025-07-04T22:32:14.9392+01:00","event_id":"ca95c25f-53c5-4410-b986-8406e3832b94"}

data: {"type":"chunk","message":"","data":{"cache_key":"5cbe3f548491c939472e491c75a1f5ab_itinerary","cache_used":true,"chunk":":00\\\", \\\"Fri\\\": \\\"00:00-00:00\\\", \\\"Sat\\\": \\\"00:00-00:00\\\", \\\"Sun\\\": \\\"00:00-00:00\\\"}\",\n      \"distance\": 1.2\n    ","domain":"itinerary","part":"itinerary"},"timestamp":"2025-07-04T22:32:14.961231+01:00","event_id":"59f9420f-ba9c-4b60-8c1d-b3b1e93a8e3a"}

data: {"type":"chunk","message":"","data":{"cache_key":"5cbe3f548491c939472e491c75a1f5ab_itinerary","cache_used":true,"chunk":"}\n  ]\n}\n```","domain":"itinerary","part":"itinerary"},"timestamp":"2025-07-04T22:32:14.999001+01:00","event_id":"fe4031cd-c93e-480f-8838-bb5284ca9017"}

data: {"type":"chunk","message":"","data":{"cache_key":"5cbe3f548491c939472e491c75a1f5ab_general_pois","cache_used":true,"chunk":"5 Madrid\",\n            \"website\": \"https://www.mercadodesanmiguel.es/en/\",\n            \"opening_hours\": {\n                \"Sun-Thu\": \"10:00-00:00\",\n                \"Fri-Sat\": \"10:00-0","domain":"itinerary","part":"general_pois"},"timestamp":"2025-07-04T22:32:15.284067+01:00","event_id":"01dfffe5-2ffe-464c-a86b-087c00ac13c2"}

data: {"type":"chunk","message":"","data":{"cache_key":"5cbe3f548491c939472e491c75a1f5ab_general_pois","cache_used":true,"chunk":"1:00\"\n            }\n        }\n    ]\n}\n```","domain":"itinerary","part":"general_pois"},"timestamp":"2025-07-04T22:32:15.374047+01:00","event_id":"6eb9ed3a-1a04-4d9e-805e-3e90a7efa079"}

data: {"type":"complete","message":"","data":{"session_id":"f273f102-28c2-4dbf-9122-2b4b06db139d"},"timestamp":"2025-07-04T22:32:15.374108+01:00","event_id":"53159cd7-38c1-4e5b-bf3c-33de93ecd293","navigation":{"url":"/itinerary?sessionId=f273f102-28c2-4dbf-9122-2b4b06db139d\u0026cityName=Madrid\u0026domain=itinerary","route_type":"itinerary","query_params":{"cityName":"Madrid","domain":"itinerary","sessionId":"f273f102-28c2-4dbf-9122-2b4b06db139d"}}}


I get a proper response. Now when I hit that endpoint, I get events printing on my terminal but my response is first returned:


data: {"type":"start","message":"","data":{"cache_key":"5cbe3f548491c939472e491c75a1f5ab","city":"Madrid","domain":"itinerary","session_id":"72324150-43a3-4a1b-99c8-005208b9c0e2"},"timestamp":"2025-07-04T22:33:42.232304+01:00","event_id":"bacf4d49-3b24-4fce-83e1-c73887f12769"}

and on the terminal:

10:33PM WRN chat_prompt/chat_service.go:2736 Client context cancelled, but continuing worker partType=general_pois
10:33PM DBG chat_prompt/chat_service.go:2751 Processing candidates partType=general_pois candidatesCount=1
10:33PM DBG chat_prompt/chat_service.go:2756 Processing content parts partType=general_pois partsCount=1
10:33PM DBG chat_prompt/chat_service.go:2762 Sending chunk partType=general_pois chunkLength=180
10:33PM DBG chat_prompt/chat_service.go:2730 Received response from iterator partType=general_pois responseCount=28
10:33PM WRN chat_prompt/chat_service.go:2736 Client context cancelled, but continuing worker partType=general_pois
10:33PM DBG chat_prompt/chat_service.go:2751 Processing candidates partType=general_pois candidatesCount=1
10:33PM DBG chat_prompt/chat_service.go:2756 Processing content parts partType=general_pois partsCount=1
10:33PM DBG chat_prompt/chat_service.go:2762 Sending chunk partType=general_pois chunkLength=228
10:33PM DBG chat_prompt/chat_service.go:2730 Received response from iterator partType=general_pois responseCount=29
10:33PM WRN chat_prompt/chat_service.go:2736 Client context cancelled, but continuing worker partType=general_pois
10:33PM DBG chat_prompt/chat_service.go:2751 Processing candidates partType=general_pois candidatesCount=1
10:33PM DBG chat_prompt/chat_service.go:2756 Processing content parts partType=general_pois partsCount=1
10:33PM DBG chat_prompt/chat_service.go:2762 Sending chunk partType=general_pois chunkLength=180
10:33PM DBG chat_prompt/chat_service.go:2730 Received response from iterator partType=general_pois responseCount=30
10:33PM WRN chat_prompt/chat_service.go:2736 Client context cancelled, but continuing worker partType=general_pois
10:33PM DBG chat_prompt/chat_service.go:2751 Processing candidates partType=general_pois candidatesCount=1
10:33PM DBG chat_prompt/chat_service.go:2756 Processing content parts partType=general_pois partsCount=1
10:33PM DBG chat_prompt/chat_service.go:2762 Sending chunk partType=general_pois chunkLength=203
10:33PM DBG chat_prompt/chat_service.go:2730 Received response from iterator partType=general_pois responseCount=31
10:33PM WRN chat_prompt/chat_service.go:2736 Client context cancelled, but continuing worker partType=general_pois
10:33PM DBG chat_prompt/chat_service.go:2751 Processing candidates partType=general_pois candidatesCount=1
10:33PM DBG chat_prompt/chat_service.go:2756 Processing content parts partType=general_pois partsCount=1
10:33PM DBG chat_prompt/chat_service.go:2762 Sending chunk partType=general_pois chunkLength=163
10:33PM DBG chat_prompt/chat_service.go:2730 Received response from iterator partType=general_pois responseCount=32
10:33PM WRN chat_prompt/chat_service.go:2736 Client context cancelled, but continuing worker partType=general_pois
10:33PM DBG chat_prompt/chat_service.go:2751 Processing candidates partType=general_pois candidatesCount=1
10:33PM DBG chat_prompt/chat_service.go:2756 Processing content parts partType=general_pois partsCount=1
10:33PM DBG chat_prompt/chat_service.go:2762 Sending chunk partType=general_pois chunkLength=56
10:33PM INF chat_prompt/chat_service.go:2787 Stream worker completed partType=general_pois totalResponses=32 fullResponseLength=4836
10:33PM INF chat_prompt/chat_service.go:2233 Stream completion goroutine finished
10:33PM DBG chat_prompt/chat_repository.go:1334 parsePOIsFromResponse: Response is a default message, not JSON data message="Processed itinerary request for Madrid"
10:33PM INF chat_prompt/chat_service.go:2321 Stream interaction saved successfully saved_interaction_id=f51bffc5-c99f-41b2-a01b-57d01480ffc1 original_session_id=72324150-43a3-4a1b-99c8-005208b9c0e2
10:33PM INF chat_prompt/chat_helpers.go:116 Processing unified response for POI extraction city_id=a6592f1b-373e-47ad-9cc1-c23987cfe541 response_parts=0


This isnt working. Compare the chat_from_main.md that is from my main and add or ajust whats needed. This branch was refactored and probably some implementations are either missing or broker. Make a deep analysis and fix. 

