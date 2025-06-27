package models

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// LLMInteraction represents an interaction with the LLM (Gemini)
type LLMInteraction struct {
	ID               uuid.UUID       `json:"id" db:"id"`
	UserID           *uuid.UUID      `json:"user_id" db:"user_id"`
	SessionID        string          `json:"session_id" db:"session_id"`
	Prompt           string          `json:"prompt" db:"prompt"`
	RequestPayload   json.RawMessage `json:"request_payload" db:"request_payload"`
	ResponseText     string          `json:"response" db:"response"`
	ResponsePayload  json.RawMessage `json:"response_payload" db:"response_payload"`
	ModelUsed        string          `json:"model_name" db:"model_name"`
	PromptTokens     int             `json:"prompt_tokens" db:"prompt_tokens"`
	CompletionTokens int             `json:"completion_tokens" db:"completion_tokens"`
	TotalTokens      int             `json:"total_tokens" db:"total_tokens"`
	LatencyMs        int             `json:"latency_ms" db:"latency_ms"`
	CreatedAt        time.Time       `json:"created_at" db:"created_at"`
}

// NewLLMInteraction creates a new LLM interaction with default values
func NewLLMInteraction(userID *uuid.UUID, prompt, responseText, modelUsed string) *LLMInteraction {
	return &LLMInteraction{
		ID:           uuid.New(),
		UserID:       userID,
		Prompt:       prompt,
		ResponseText: responseText,
		ModelUsed:    modelUsed,
		CreatedAt:    time.Now(),
	}
}
