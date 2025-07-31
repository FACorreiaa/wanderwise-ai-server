package types

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// POIFilters represents filters for POI queries
type POIFilters struct {
	City       string `json:"city,omitempty"`
	Category   string `json:"category,omitempty"`
	PriceRange string `json:"price_range,omitempty"`
}

type POIDetailedInfo struct {
	ID               uuid.UUID         `json:"id,omitempty"`
	City             string            `json:"city"`
	CityID           uuid.UUID         `json:"city_id"`
	Name             string            `json:"name"`
	DescriptionPOI   string            `json:"description_poi,omitempty"`
	Distance         float64           `json:"distance"`
	Latitude         float64           `json:"latitude,omitempty"`
	Longitude        float64           `json:"longitude,omitempty"`
	Category         string            `json:"category"`
	Description      string            `json:"description"`
	Rating           float64           `json:"rating"`
	Address          string            `json:"address"`
	PhoneNumber      string            `json:"phone_number"`
	Website          string            `json:"website"`
	OpeningHours     map[string]string `json:"opening_hours"`
	Images           []string          `json:"images,omitempty"`
	PriceRange       string            `json:"price_range"`
	PriceLevel       string            `json:"price_level"`
	Reviews          []string          `json:"reviews"`
	LlmInteractionID uuid.UUID         `json:"llm_interaction_id"`
	Tags             []string          `json:"tags,omitempty"`
	Priority         int               `json:"priority,omitempty"` // Popularity score 1-10
	CreatedAt        time.Time         `json:"created_at"`
	CuisineType      string            `json:"cuisine_type,omitempty"` // For restaurants
	StarRating       string            `json:"star_rating,omitempty"`  // For hotels
	Amenities        string            `json:"amenities"`
	Err              error             `json:"-"`
	Source           string            `json:"source,omitempty"` // Source of the POI data (e.g., "google", "yelp", etc.)
}

// UnmarshalJSON implements custom JSON unmarshaling for POIDetailedInfo
// to handle opening_hours field that can be either string or map[string]string
func (p *POIDetailedInfo) UnmarshalJSON(data []byte) error {
	// Define a temporary struct with the same fields as POIDetailedInfo
	// but with OpeningHours as json.RawMessage to handle both string and map
	type Alias POIDetailedInfo
	aux := &struct {
		OpeningHours json.RawMessage `json:"opening_hours"`
		*Alias
	}{
		Alias: (*Alias)(p),
	}

	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	// Handle OpeningHours field
	if len(aux.OpeningHours) > 0 {
		// Try to unmarshal as map[string]string first
		var hoursMap map[string]string
		if err := json.Unmarshal(aux.OpeningHours, &hoursMap); err == nil {
			p.OpeningHours = hoursMap
		} else {
			// If that fails, try to unmarshal as string
			var hoursString string
			if err := json.Unmarshal(aux.OpeningHours, &hoursString); err == nil {
				p.OpeningHours = map[string]string{"general": hoursString}
			}
		}
	}

	return nil
}

type AddPoiRequest struct {
	ID       string           `json:"poi_id"`
	IsLlmPoi bool             `json:"is_llm_poi"`
	POIData  *POIDetailedInfo `json:"poi_data,omitempty"` // Optional POI data for creating new POIs
}
