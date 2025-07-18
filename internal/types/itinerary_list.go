package types

import (
	"time"

	"github.com/google/uuid"
)

// ItineraryList represents the top-level list containing itineraries
type ItineraryList struct {
	ID          uuid.UUID
	UserID      uuid.UUID
	Name        string
	Description string
	IsPublic    bool
	CityID      uuid.UUID
	Itineraries []Itinerary
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// Itinerary represents a single itinerary (a sub-list)
type Itinerary struct {
	ID          uuid.UUID
	Name        string
	Description string
	POIs        []POI
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// POI represents a point of interest within an itinerary
type POI struct {
	ID          uuid.UUID
	Name        string
	Latitude    float64
	Longitude   float64
	Category    string
	Description string
	Position    int
	Notes       string
}

type List struct {
	ID           uuid.UUID
	UserID       uuid.UUID
	Name         string
	Description  string
	ImageURL     string
	IsPublic     bool
	IsItinerary  bool
	ParentListID *uuid.UUID // Nullable, as per schema
	CityID       uuid.UUID
	ViewCount    int
	SaveCount    int
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// ContentType defines the type of content in a list item
type ContentType string

const (
	ContentTypePOI        ContentType = "poi"
	ContentTypeRestaurant ContentType = "restaurant"
	ContentTypeHotel      ContentType = "hotel"
	ContentTypeItinerary  ContentType = "itinerary"
)

type ListItem struct {
	ListID      uuid.UUID   `json:"list_id"`
	ItemID      uuid.UUID   `json:"item_id"` // Generic ID that could reference POI, Restaurant, Hotel, or Itinerary
	PoiID       uuid.UUID   `json:"poi_id"`
	ContentType ContentType `json:"content_type"` // Type of content this item represents
	Position    int         `json:"position"`
	Notes       string      `json:"notes"`
	DayNumber   *int        `json:"day_number"` // Nullable, as per schema
	TimeSlot    *time.Time  `json:"time_slot"`  // Nullable, as per schema
	Duration    *int        `json:"duration"`   // Nullable, as per schema
	CreatedAt   time.Time   `json:"created_at"`
	UpdatedAt   time.Time   `json:"updated_at"`

	// Additional metadata for different content types
	SourceLlmInteractionID *uuid.UUID `json:"source_llm_interaction_id,omitempty"` // Reference to the original LLM interaction
	ItemAIDescription      string     `json:"item_ai_description,omitempty"`       // AI-generated description for this item
}

type UpdateListRequest struct {
	Name        *string    `json:"name,omitempty" validate:"omitempty,min=3,max=100"`
	Description *string    `json:"description,omitempty" validate:"omitempty,max=500"`
	ImageURL    *string    `json:"image_url,omitempty" validate:"omitempty,url"`
	IsPublic    *bool      `json:"is_public,omitempty"`
	CityID      *uuid.UUID `json:"city_id,omitempty"`
}

type AddListItemRequest struct {
	ItemID                 uuid.UUID   `json:"item_id" validate:"required"`                                           // Generic ID for POI, Restaurant, Hotel, or Itinerary
	ContentType            ContentType `json:"content_type" validate:"required,oneof=poi restaurant hotel itinerary"` // Type of content being added
	Position               int         `json:"position" validate:"gte=0"`
	Notes                  string      `json:"notes,omitempty" validate:"max=1000"`
	DayNumber              *int        `json:"day_number,omitempty" validate:"omitempty,gt=0"`
	TimeSlot               *time.Time  `json:"time_slot,omitempty"`
	DurationMinutes        *int        `json:"duration_minutes,omitempty" validate:"omitempty,gt=0"`
	SourceLlmInteractionID *uuid.UUID  `json:"source_llm_interaction_id,omitempty"` // Reference to the LLM interaction that generated this content
	ItemAIDescription      string      `json:"item_ai_description,omitempty"`
}

type UpdateListItemRequest struct {
	ItemID                 *uuid.UUID   `json:"item_id,omitempty"`                                                                // Generic ID for POI, Restaurant, Hotel, or Itinerary
	ContentType            *ContentType `json:"content_type,omitempty" validate:"omitempty,oneof=poi restaurant hotel itinerary"` // Type of content
	Position               *int         `json:"position,omitempty" validate:"omitempty,gte=0"`
	Notes                  *string      `json:"notes,omitempty" validate:"omitempty,max=1000"`
	DayNumber              *int         `json:"day_number,omitempty" validate:"omitempty,gt=0"`
	TimeSlot               *time.Time   `json:"time_slot,omitempty"`
	DurationMinutes        *int         `json:"duration_minutes,omitempty" validate:"omitempty,gt=0"`
	SourceLlmInteractionID *uuid.UUID   `json:"source_llm_interaction_id,omitempty"`
	ItemAIDescription      *string      `json:"item_ai_description,omitempty"`
}

// ListWithItems combines a List with its items
type ListWithItems struct {
	List  List
	Items []*ListItem
}

// ListItemWithContent combines a ListItem with its actual content details
type ListItemWithContent struct {
	ListItem   ListItem                `json:"list_item"`
	POI        *POIDetailedInfo        `json:"poi,omitempty"`        // Populated when ContentType is "poi"
	Restaurant *RestaurantDetailedInfo `json:"restaurant,omitempty"` // Populated when ContentType is "restaurant"
	Hotel      *HotelDetailedInfo      `json:"hotel,omitempty"`      // Populated when ContentType is "hotel"
	Itinerary  *UserSavedItinerary     `json:"itinerary,omitempty"`  // Populated when ContentType is "itinerary"
}

// ListWithDetailedItems combines a List with its items and their content details
type ListWithDetailedItems struct {
	List  List                   `json:"list"`
	Items []*ListItemWithContent `json:"items"`
}

type CreateListRequest struct {
	Name        string     `json:"name" validate:"required,min=3,max=100"`
	Description string     `json:"description,omitempty" validate:"max=500"`
	CityID      *uuid.UUID `json:"city_id,omitempty"` // Optional: if the list/itinerary is city-specific
	IsItinerary bool       `json:"is_itinerary"`      // True if this top-level list IS an itinerary itself
	IsPublic    bool       `json:"is_public"`
}

type CreateItineraryForListRequest struct {
	Name        string `json:"name" validate:"required,min=3,max=100"`
	Description string `json:"description,omitempty" validate:"max=500"`
	IsPublic    bool   `json:"is_public"`
}
