package interests

import (
	"time"

	"github.com/google/uuid"
)

// Interest defines the structure for an interest tag.
type Interest struct {
	ID          uuid.UUID  `json:"id"`
	Name        string     `json:"name"`
	Description *string    `json:"description,omitempty"` // Use pointer if nullable
	Active      *bool      `json:"active"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   *time.Time `json:"updated_at"`
	Source      string     `json:"source"`
}

type UpdateinterestsParams struct {
	Name        *string `json:"name,omitempty"`
	Description *string `json:"description,omitempty"`
	Active      *bool   `json:"active,omitempty"`
}

// Request and Response structures for the HandlerImpls
type AddInterestRequest struct {
	InterestID string `json:"interest_id" binding:"required" example:"d290f1ee-6c54-4b01-90e6-d701748f0851"`
}

type CreateInterestRequest struct {
	Name        string  `json:"name" binding:"required" example:"Hiking"`
	Description *string `json:"description,omitempty" example:"Outdoor hiking activities"`
	Active      bool    `json:"active" example:"true"`
}

type UpdatePreferenceLevelRequest struct {
	PreferenceLevel int `json:"preference_level" binding:"required" example:"2"`
}

type UpdateInterestRequest struct {
	ID          string  `json:"interest_id" binding:"required" example:"d290f1ee-6c54-4b01-90e6-d701748f0851"`
	Name        string  `json:"name" binding:"required" example:"Hiking"`
	Description *string `json:"description,omitempty" example:"Outdoor hiking activities"`
	Active      bool    `json:"active" example:"true"`
}
