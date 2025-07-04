package llmChat

import (
	"log/slog"
	"net/http"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
)

// RefactoredHandlerImpl provides refactored handlers using interfaces
type RefactoredHandlerImpl struct {
	llmInteractionService LlmInteractiontService
	streamHandler         StreamHandler
	validator             RequestValidator
	emitter               EventEmitter
	logger                *slog.Logger
}

// NewRefactoredHandlerImpl creates a new refactored handler instance
func NewRefactoredHandlerImpl(
	llmInteractionService LlmInteractiontService,
	logger *slog.Logger,
) *RefactoredHandlerImpl {
	streamHandler := NewBaseStreamHandler(logger)
	validator := NewBaseRequestValidator(logger)
	emitter := NewBaseEventEmitter(logger)

	return &RefactoredHandlerImpl{
		llmInteractionService: llmInteractionService,
		streamHandler:         streamHandler,
		validator:             validator,
		emitter:               emitter,
		logger:                logger,
	}
}

// StartChatMessageStreamRefactored handles unified chat requests with streaming (refactored)
func (h *RefactoredHandlerImpl) StartChatMessageStreamRefactored(w http.ResponseWriter, r *http.Request) {
	ctx, span := otel.Tracer("RefactoredHandler").Start(r.Context(), "StartChatMessageStreamRefactored", trace.WithAttributes(
		semconv.HTTPRequestMethodKey.String(r.Method),
		semconv.HTTPRouteKey.String("/prompt-response/unified-chat/stream"),
	))
	defer span.End()

	l := h.logger.With(slog.String("handler", "StartChatMessageStreamRefactored"))
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

// StartChatMessageStreamFreeRefactored handles free chat requests with streaming (refactored)
func (h *RefactoredHandlerImpl) StartChatMessageStreamFreeRefactored(w http.ResponseWriter, r *http.Request) {
	ctx, span := otel.Tracer("RefactoredHandler").Start(r.Context(), "StartChatMessageStreamFreeRefactored", trace.WithAttributes(
		semconv.HTTPRequestMethodKey.String(r.Method),
		semconv.HTTPRouteKey.String("/prompt-response/unified-chat/stream/free"),
	))
	defer span.End()

	l := h.logger.With(slog.String("handler", "StartChatMessageStreamFreeRefactored"))
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

// ContinueChatSessionStreamRefactored handles continue chat requests with streaming (refactored)
func (h *RefactoredHandlerImpl) ContinueChatSessionStreamRefactored(w http.ResponseWriter, r *http.Request) {
	ctx, span := otel.Tracer("RefactoredHandler").Start(r.Context(), "ContinueChatSessionStreamRefactored", trace.WithAttributes(
		semconv.HTTPRequestMethodKey.String(r.Method),
		semconv.HTTPRouteKey.String("/chat/session/{sessionID}/continue"),
	))
	defer span.End()

	l := h.logger.With(slog.String("handler", "ContinueChatSessionStreamRefactored"))
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