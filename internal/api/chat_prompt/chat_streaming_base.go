package llmChat

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/FACorreiaa/go-poi-au-suggestions/internal/api"
	"github.com/FACorreiaa/go-poi-au-suggestions/internal/api/auth"
	"github.com/FACorreiaa/go-poi-au-suggestions/internal/types"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// BaseStreamHandler provides common streaming functionality
type BaseStreamHandler struct {
	logger *slog.Logger
}

// NewBaseStreamHandler creates a new base stream handler
func NewBaseStreamHandler(logger *slog.Logger) *BaseStreamHandler {
	return &BaseStreamHandler{logger: logger}
}

// SetupSSE configures Server-Sent Events headers
func (h *BaseStreamHandler) SetupSSE(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Cache-Control")
}

// ProcessStream handles the streaming logic and event processing
func (h *BaseStreamHandler) ProcessStream(ctx context.Context, w http.ResponseWriter, r *http.Request, processor StreamProcessor) error {
	// Validate request
	if err := processor.ValidateRequest(); err != nil {
		api.ErrorResponse(w, r, http.StatusBadRequest, err.Error())
		return err
	}

	// Get trace attributes
	traceAttrs := processor.GetTraceAttributes()
	var attrs []trace.SpanStartOption
	if traceAttrs != nil {
		var spanAttrs []attribute.KeyValue
		for key, value := range traceAttrs {
			switch v := value.(type) {
			case string:
				spanAttrs = append(spanAttrs, attribute.String(key, v))
			case int:
				spanAttrs = append(spanAttrs, attribute.Int(key, v))
			case float64:
				spanAttrs = append(spanAttrs, attribute.Float64(key, v))
			}
		}
		if len(spanAttrs) > 0 {
			attrs = append(attrs, trace.WithAttributes(spanAttrs...))
		}
	}

	ctx, span := otel.Tracer("StreamHandler").Start(ctx, "ProcessStream", attrs...)
	defer span.End()

	// Setup SSE
	h.SetupSSE(w)

	// Create event channel
	eventCh := make(chan types.StreamEvent, 100)

	// Process in goroutine
	go func() {
		defer close(eventCh)
		if err := processor.ProcessRequest(ctx, eventCh); err != nil {
			h.logger.ErrorContext(ctx, "Stream processing failed", slog.Any("error", err))
			span.RecordError(err)
			span.SetStatus(codes.Error, "Stream processing failed")
		}
	}()

	// Stream events
	return h.streamEvents(ctx, w, r, eventCh, span)
}

// streamEvents handles the event streaming loop
func (h *BaseStreamHandler) streamEvents(ctx context.Context, w http.ResponseWriter, r *http.Request, eventCh <-chan types.StreamEvent, span trace.Span) error {
	flusher, ok := w.(http.Flusher)
	if !ok {
		err := fmt.Errorf("streaming not supported")
		span.SetStatus(codes.Error, "Streaming not supported")
		api.ErrorResponse(w, r, http.StatusInternalServerError, "Streaming not supported")
		return err
	}

	for {
		select {
		case event, ok := <-eventCh:
			if !ok {
				h.logger.InfoContext(ctx, "Event channel closed, ending stream")
				span.SetStatus(codes.Ok, "Stream completed")
				return nil
			}

			eventData, err := json.Marshal(event)
			if err != nil {
				h.logger.ErrorContext(ctx, "Failed to marshal event", slog.Any("error", err))
				span.RecordError(err)
				continue
			}

			fmt.Fprintf(w, "data: %s\n\n", eventData)
			flusher.Flush()

			if event.Type == types.EventTypeComplete || event.Type == types.EventTypeError {
				h.logger.InfoContext(ctx, "Stream completed", slog.String("eventType", event.Type))
				span.SetStatus(codes.Ok, "Stream completed")
				return nil
			}

		case <-r.Context().Done():
			h.logger.InfoContext(ctx, "Client disconnected")
			span.SetStatus(codes.Ok, "Client disconnected")
			return nil
		}
	}
}

// WriteSSEError writes an error event to the SSE stream
func (h *BaseStreamHandler) WriteSSEError(w http.ResponseWriter, errorMsg string) {
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

// BaseRequestValidator provides common request validation functionality
type BaseRequestValidator struct {
	logger *slog.Logger
}

// NewBaseRequestValidator creates a new base request validator
func NewBaseRequestValidator(logger *slog.Logger) *BaseRequestValidator {
	return &BaseRequestValidator{logger: logger}
}

// ValidateUserAuth validates user authentication and returns user ID
func (v *BaseRequestValidator) ValidateUserAuth(ctx context.Context) (uuid.UUID, error) {
	userIDStr, ok := auth.GetUserIDFromContext(ctx)
	if !ok || userIDStr == "" {
		return uuid.Nil, fmt.Errorf("authentication required")
	}

	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		return uuid.Nil, fmt.Errorf("invalid user ID format: %w", err)
	}

	return userID, nil
}

// ValidateProfileID validates profile ID from URL parameters
func (v *BaseRequestValidator) ValidateProfileID(r *http.Request) (uuid.UUID, error) {
	profileIDStr := r.URL.Query().Get("profileID")
	if profileIDStr == "" {
		return uuid.Nil, fmt.Errorf("profile ID is required")
	}

	profileID, err := uuid.Parse(profileIDStr)
	if err != nil {
		return uuid.Nil, fmt.Errorf("invalid profile ID format: %w", err)
	}

	return profileID, nil
}

// ValidateRequestBody validates and decodes request body
func (v *BaseRequestValidator) ValidateRequestBody(r *http.Request, target interface{}) error {
	if err := json.NewDecoder(r.Body).Decode(target); err != nil {
		return fmt.Errorf("invalid request body: %w", err)
	}
	return nil
}

// BaseEventEmitter provides common event emission functionality
type BaseEventEmitter struct {
	logger *slog.Logger
}

// NewBaseEventEmitter creates a new base event emitter
func NewBaseEventEmitter(logger *slog.Logger) *BaseEventEmitter {
	return &BaseEventEmitter{logger: logger}
}

// EmitProgress emits a progress event
func (e *BaseEventEmitter) EmitProgress(ctx context.Context, eventCh chan<- types.StreamEvent, status string, progress int) bool {
	return e.sendEvent(ctx, eventCh, types.StreamEvent{
		Type:      types.EventTypeProgress,
		Data:      map[string]interface{}{"status": status, "progress": progress},
		Timestamp: time.Now(),
		EventID:   uuid.New().String(),
	})
}

// EmitError emits an error event
func (e *BaseEventEmitter) EmitError(ctx context.Context, eventCh chan<- types.StreamEvent, err error) bool {
	return e.sendEvent(ctx, eventCh, types.StreamEvent{
		Type:      types.EventTypeError,
		Error:     err.Error(),
		Timestamp: time.Now(),
		EventID:   uuid.New().String(),
	})
}

// EmitData emits a data event
func (e *BaseEventEmitter) EmitData(ctx context.Context, eventCh chan<- types.StreamEvent, eventType string, data interface{}) bool {
	return e.sendEvent(ctx, eventCh, types.StreamEvent{
		Type:      eventType,
		Data:      data,
		Timestamp: time.Now(),
		EventID:   uuid.New().String(),
	})
}

// EmitComplete emits a completion event
func (e *BaseEventEmitter) EmitComplete(ctx context.Context, eventCh chan<- types.StreamEvent, data interface{}) bool {
	return e.sendEvent(ctx, eventCh, types.StreamEvent{
		Type:      types.EventTypeComplete,
		Data:      data,
		Timestamp: time.Now(),
		EventID:   uuid.New().String(),
	})
}

// sendEvent safely sends an event to the channel
func (e *BaseEventEmitter) sendEvent(ctx context.Context, eventCh chan<- types.StreamEvent, event types.StreamEvent) bool {
	select {
	case eventCh <- event:
		return true
	case <-ctx.Done():
		e.logger.WarnContext(ctx, "Context cancelled while sending event")
		return false
	default:
		e.logger.WarnContext(ctx, "Event channel full, dropping event")
		return false
	}
}
