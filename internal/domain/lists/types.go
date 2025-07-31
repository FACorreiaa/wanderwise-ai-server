package lists

import (
	"database/sql"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
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

type RestaurantDetailedInfo struct {
	ID               uuid.UUID `json:"id"`
	City             string    `json:"city"`
	Name             string    `json:"name"`
	Latitude         float64   `json:"latitude"`
	Longitude        float64   `json:"longitude"`
	Category         string    `json:"category"`
	Description      string    `json:"description"`
	Address          *string   `json:"address"`
	Website          *string   `json:"website"`
	PhoneNumber      *string   `json:"phone_number"`
	OpeningHours     *string   `json:"opening_hours"`
	PriceLevel       *string   `json:"price_level"`  // Changed to *string
	CuisineType      *string   `json:"cuisine_type"` // Changed to *string
	Tags             []string  `json:"tags"`
	Images           []string  `json:"images"`
	Rating           float64   `json:"rating"`
	LlmInteractionID uuid.UUID `json:"llm_interaction_id"`
	Err              error     `json:"-"`
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

type HotelDetailedInfo struct {
	ID               uuid.UUID `json:"id"`
	City             string    `json:"city"`
	Name             string    `json:"name"`
	Latitude         float64   `json:"latitude"`
	Longitude        float64   `json:"longitude"`
	Category         string    `json:"category"` // e.g., "Hotel", "Hostel"
	Description      string    `json:"description"`
	Address          string    `json:"address"`
	PhoneNumber      *string   `json:"phone_number"`
	Website          *string   `json:"website"`
	OpeningHours     *string   `json:"opening_hours"`
	PriceRange       *string   `json:"price_range"`
	Rating           float64   `json:"rating"`
	Tags             []string  `json:"tags"`
	Images           []string  `json:"images"`
	LlmInteractionID uuid.UUID `json:"llm_interaction_id"`
	Err              error     `json:"-"` // Not serialized
}

type UserSavedItinerary struct {
	ID                     uuid.UUID      `json:"id"`
	UserID                 uuid.UUID      `json:"user_id"`
	SourceLlmInteractionID pgtype.UUID    `json:"source_llm_interaction_id,omitempty"` // Nullable UUID for the source LLM interaction
	SessionID              pgtype.UUID    `json:"session_id,omitempty"`                // Nullable UUID for the chat session
	PrimaryCityID          pgtype.UUID    `json:"primary_city_id,omitempty"`           // Nullable UUID for the primary city
	Title                  string         `json:"title"`
	Description            sql.NullString `json:"description"`             // Use sql.NullString for nullable text fields
	MarkdownContent        string         `json:"markdown_content"`        // Markdown content for the itinerary
	Tags                   []string       `json:"tags"`                    // Tags for the itinerary
	EstimatedDurationDays  sql.NullInt32  `json:"estimated_duration_days"` // Nullable int32 for estimated duration in days
	EstimatedCostLevel     sql.NullInt32  `json:"estimated_cost_level"`    // Nullable int32 for estimated cost level
	IsPublic               bool           `json:"is_public"`               // Indicates if the itinerary is public
	CreatedAt              time.Time      `json:"created_at"`
	UpdatedAt              time.Time      `json:"updated_at"`
}
