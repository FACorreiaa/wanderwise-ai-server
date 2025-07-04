package llmChat

import (
	"context"
	"net/http"

	"github.com/FACorreiaa/go-poi-au-suggestions/internal/types"
	"github.com/google/uuid"
)

// StreamHandler defines the interface for streaming chat operations
type StreamHandler interface {
	// SetupSSE configures Server-Sent Events headers
	SetupSSE(w http.ResponseWriter)
	// ProcessStream handles the streaming logic and event processing
	ProcessStream(ctx context.Context, w http.ResponseWriter, r *http.Request, processor StreamProcessor) error
	// WriteSSEError writes an error event to the SSE stream
	WriteSSEError(w http.ResponseWriter, errorMsg string)
}

// StreamProcessor defines the interface for processing different types of streaming requests
type StreamProcessor interface {
	// ProcessRequest processes the streaming request and sends events to the channel
	ProcessRequest(ctx context.Context, eventCh chan<- types.StreamEvent) error
	// ValidateRequest validates the incoming request
	ValidateRequest() error
	// GetTraceAttributes returns OpenTelemetry trace attributes for the request
	GetTraceAttributes() map[string]interface{}
}

// RequestValidator defines the interface for request validation
type RequestValidator interface {
	// ValidateUserAuth validates user authentication and returns user ID
	ValidateUserAuth(ctx context.Context) (uuid.UUID, error)
	// ValidateProfileID validates profile ID from URL parameters
	ValidateProfileID(r *http.Request) (uuid.UUID, error)
	// ValidateRequestBody validates and decodes request body
	ValidateRequestBody(r *http.Request, target interface{}) error
}

// AIWorker defines the interface for AI generation workers
type AIWorker interface {
	// Execute runs the AI generation task
	Execute(ctx context.Context, resultCh chan<- types.GenAIResponse, eventCh chan<- types.StreamEvent) error
	// GetPrompt returns the prompt for the AI generation
	GetPrompt() string
	// GetTraceAttributes returns OpenTelemetry trace attributes
	GetTraceAttributes() map[string]interface{}
}

// EventEmitter defines the interface for event emission during streaming
type EventEmitter interface {
	// EmitProgress emits a progress event
	EmitProgress(ctx context.Context, eventCh chan<- types.StreamEvent, status string, progress int) bool
	// EmitError emits an error event
	EmitError(ctx context.Context, eventCh chan<- types.StreamEvent, err error) bool
	// EmitData emits a data event
	EmitData(ctx context.Context, eventCh chan<- types.StreamEvent, eventType string, data interface{}) bool
	// EmitComplete emits a completion event
	EmitComplete(ctx context.Context, eventCh chan<- types.StreamEvent, data interface{}) bool
}

// ChatSessionManager defines the interface for chat session management
type ChatSessionManager interface {
	// CreateSession creates a new chat session
	CreateSession(ctx context.Context, userID uuid.UUID) (uuid.UUID, error)
	// GetSession retrieves an existing chat session
	GetSession(ctx context.Context, sessionID uuid.UUID) (*types.ChatSession, error)
	// UpdateSession updates a chat session
	UpdateSession(ctx context.Context, session *types.ChatSession) error
	// DeleteSession deletes a chat session
	DeleteSession(ctx context.Context, sessionID uuid.UUID) error
}

// ResponseProcessor defines the interface for processing AI responses
type ResponseProcessor interface {
	// ProcessResponse processes the AI response and extracts structured data
	ProcessResponse(ctx context.Context, response string) (interface{}, error)
	// ValidateResponse validates the response format
	ValidateResponse(response string) error
	// CleanResponse cleans and formats the response
	CleanResponse(response string) string
}