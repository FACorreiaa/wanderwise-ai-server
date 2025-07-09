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

// type POIDetail struct {
// 	ID               uuid.UUID `json:"id"`
// 	LlmInteractionID uuid.UUID `json:"llm_interaction_id,omitempty"` // ID of the LLM interaction that generated this POI
// 	City             string    `json:"city"`                         // City where the POI is located
// 	CityID           uuid.UUID `json:"city_id"`
// 	//Description    string    `json:"description"`
// 	Name           string  `json:"name"`
// 	Latitude       float64 `json:"latitude"`
// 	Longitude      float64 `json:"longitude"`
// 	Category       string  `json:"category"`
// 	DescriptionPOI string  `json:"description_poi"`
// 	// Rating               float64   `json:"rating"`
// 	Address string `json:"address"`
// 	// PhoneNumber          string    `json:"phone_number"`
// 	Website      string `json:"website"`
// 	OpeningHours string `json:"opening_hours"`
// 	// Images               []string  `json:"images"`
// 	// Reviews              []string  `json:"reviews"`
// 	// PriceRange           string    `json:"price_range"`
// 	Distance float64 `json:"distance"`
// 	// DistanceUnit         string    `json:"distance_unit"`
// 	// DistanceValue        float64   `json:"distance_value"`
// 	// DistanceText         string    `json:"distance_text"`
// 	// LocationType         string    `json:"location_type"`
// 	// LocationID           string    `json:"location_id"`
// 	// LocationURL          string    `json:"location_url"`
// 	// LocationRating       float64   `json:"location_rating"`
// 	// LocationReview       int       `json:"location_review"`
// 	// LocationAddress      string    `json:"location_address"`
// 	// LocationPhone        string    `json:"location_phone"`
// 	// LocationWebsite      string    `json:"location_website"`
// 	// LocationOpeningHours string    `json:"location_opening_hours"`
// 	CuisineType string `json:"cuisine_type,omitempty"` // For restaurants
// 	StarRating  string `json:"star_rating,omitempty"`  // For hotels
// 	Err         error  `json:"-"`
// }

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
