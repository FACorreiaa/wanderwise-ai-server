package llmChat

import (
	"context"
	"fmt"
	"net/http"

	"github.com/FACorreiaa/go-poi-au-suggestions/internal/types"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// UnifiedChatStreamProcessor handles unified chat streaming requests
type UnifiedChatStreamProcessor struct {
	service   LlmInteractiontService
	validator RequestValidator
	emitter   EventEmitter
	userID    uuid.UUID
	profileID uuid.UUID
	message   string
	location  *types.UserLocation
	r         *http.Request
}

// NewUnifiedChatStreamProcessor creates a new unified chat stream processor
func NewUnifiedChatStreamProcessor(
	service LlmInteractiontService,
	validator RequestValidator,
	emitter EventEmitter,
	r *http.Request,
) *UnifiedChatStreamProcessor {
	return &UnifiedChatStreamProcessor{
		service:   service,
		validator: validator,
		emitter:   emitter,
		r:         r,
	}
}

// ValidateRequest validates the incoming request
func (p *UnifiedChatStreamProcessor) ValidateRequest() error {
	ctx := p.r.Context()

	// Validate user authentication
	userID, err := p.validator.ValidateUserAuth(ctx)
	if err != nil {
		return err
	}
	p.userID = userID

	// Validate profile ID from URL
	profileIDStr := chi.URLParam(p.r, "profileID")
	profileID, err := uuid.Parse(profileIDStr)
	if err != nil {
		return fmt.Errorf("invalid profile ID: %w", err)
	}
	p.profileID = profileID

	// Validate request body
	var req struct {
		Message      string              `json:"message"`
		UserLocation *types.UserLocation `json:"user_location,omitempty"`
	}
	if err := p.validator.ValidateRequestBody(p.r, &req); err != nil {
		return err
	}

	if req.Message == "" {
		return fmt.Errorf("message is required")
	}

	p.message = req.Message
	p.location = req.UserLocation

	return nil
}

// ProcessRequest processes the streaming request
func (p *UnifiedChatStreamProcessor) ProcessRequest(ctx context.Context, eventCh chan<- types.StreamEvent) error {
	return p.service.ProcessUnifiedChatMessageStream(
		ctx, p.userID, p.profileID, "", p.message, p.location, eventCh,
	)
}

// GetTraceAttributes returns trace attributes for the request
func (p *UnifiedChatStreamProcessor) GetTraceAttributes() map[string]interface{} {
	return map[string]interface{}{
		"user.id":    p.userID.String(),
		"profile.id": p.profileID.String(),
		"message":    p.message,
	}
}

// FreeChatStreamProcessor handles free chat streaming requests
type FreeChatStreamProcessor struct {
	service   LlmInteractiontService
	validator RequestValidator
	emitter   EventEmitter
	message   string
	location  *types.UserLocation
	r         *http.Request
}

// NewFreeChatStreamProcessor creates a new free chat stream processor
func NewFreeChatStreamProcessor(
	service LlmInteractiontService,
	validator RequestValidator,
	emitter EventEmitter,
	r *http.Request,
) *FreeChatStreamProcessor {
	return &FreeChatStreamProcessor{
		service:   service,
		validator: validator,
		emitter:   emitter,
		r:         r,
	}
}

// ValidateRequest validates the incoming request
func (p *FreeChatStreamProcessor) ValidateRequest() error {
	// Validate request body
	var req struct {
		Message      string              `json:"message"`
		UserLocation *types.UserLocation `json:"user_location,omitempty"`
	}
	if err := p.validator.ValidateRequestBody(p.r, &req); err != nil {
		return err
	}

	if req.Message == "" {
		return fmt.Errorf("message is required")
	}

	p.message = req.Message
	p.location = req.UserLocation

	return nil
}

// ProcessRequest processes the streaming request
func (p *FreeChatStreamProcessor) ProcessRequest(ctx context.Context, eventCh chan<- types.StreamEvent) error {
	return p.service.ProcessUnifiedChatMessageStreamFree(
		ctx, "", p.message, p.location, eventCh,
	)
}

// GetTraceAttributes returns trace attributes for the request
func (p *FreeChatStreamProcessor) GetTraceAttributes() map[string]interface{} {
	return map[string]interface{}{
		"message": p.message,
	}
}

// ContinueChatStreamProcessor handles continue chat streaming requests
type ContinueChatStreamProcessor struct {
	service   LlmInteractiontService
	validator RequestValidator
	emitter   EventEmitter
	sessionID uuid.UUID
	message   string
	location  *types.UserLocation
	r         *http.Request
}

// NewContinueChatStreamProcessor creates a new continue chat stream processor
func NewContinueChatStreamProcessor(
	service LlmInteractiontService,
	validator RequestValidator,
	emitter EventEmitter,
	r *http.Request,
) *ContinueChatStreamProcessor {
	return &ContinueChatStreamProcessor{
		service:   service,
		validator: validator,
		emitter:   emitter,
		r:         r,
	}
}

// ValidateRequest validates the incoming request
func (p *ContinueChatStreamProcessor) ValidateRequest() error {
	// Validate session ID from URL
	sessionIDStr := chi.URLParam(p.r, "sessionID")
	sessionID, err := uuid.Parse(sessionIDStr)
	if err != nil {
		return fmt.Errorf("invalid session ID: %w", err)
	}
	p.sessionID = sessionID

	// Validate request body
	var req struct {
		Message      string                `json:"message"`
		CityName     string                `json:"city_name,omitempty"`
		ContextType  types.ChatContextType `json:"context_type,omitempty"`
		UserLocation *types.UserLocation   `json:"user_location,omitempty"`
	}
	if err := p.validator.ValidateRequestBody(p.r, &req); err != nil {
		return err
	}

	// Default to general context for backward compatibility
	if req.ContextType == "" {
		req.ContextType = types.ContextGeneral
	}

	p.message = req.Message
	p.location = req.UserLocation

	return nil
}

// ProcessRequest processes the streaming request
func (p *ContinueChatStreamProcessor) ProcessRequest(ctx context.Context, eventCh chan<- types.StreamEvent) error {
	return p.service.ContinueSessionStreamed(
		ctx, p.sessionID, p.message, p.location, eventCh,
	)
}

// GetTraceAttributes returns trace attributes for the request
func (p *ContinueChatStreamProcessor) GetTraceAttributes() map[string]interface{} {
	return map[string]interface{}{
		"session.id": p.sessionID.String(),
		"message":    p.message,
	}
}
