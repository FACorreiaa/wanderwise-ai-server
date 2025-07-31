package city

import "github.com/google/uuid"

// CityDetail matches the cities table structure.
type CityDetail struct {
	ID              uuid.UUID `json:"id"`
	Name            string    `json:"name"`
	Country         string    `json:"country"`
	StateProvince   string    `json:"state_province,omitempty"`
	AiSummary       string    `json:"ai_summary"`
	CenterLatitude  float64   `json:"center_latitude,omitempty"`
	CenterLongitude float64   `json:"center_longitude,omitempty"`
}
