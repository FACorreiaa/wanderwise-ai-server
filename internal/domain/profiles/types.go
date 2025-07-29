package profiles

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/FACorreiaa/go-poi-au-suggestions/internal/domain/interests"
)

// --- ENUM Types ---

// DayPreference represents the DB ENUM 'day_preference_enum'.
type DayPreference string

const (
	DayPreferenceAny   DayPreference = "any"   // No specific preference
	DayPreferenceDay   DayPreference = "day"   // Primarily daytime activities
	DayPreferenceNight DayPreference = "night" // Primarily evening/night activities
)

// Scan implements the sql.Scanner interface for DayPreference.
func (s *DayPreference) Scan(value interface{}) error {
	strVal, ok := value.(string)
	if !ok {
		bytesVal, ok := value.([]byte) // Sometimes comes as bytes
		if !ok {
			return fmt.Errorf("failed to scan DayPreference: expected string or []byte, got %T", value)
		}
		strVal = string(bytesVal)
	}
	// Validate if the scanned value is one of the known enum values
	switch DayPreference(strVal) {
	case DayPreferenceAny, DayPreferenceDay, DayPreferenceNight:
		*s = DayPreference(strVal)
		return nil
	default:
		return fmt.Errorf("unknown DayPreference value: %s", strVal)
	}
}

// Value implements the driver.Valuer interface for DayPreference.
func (s DayPreference) Value() (driver.Value, error) {
	// Optional validation before saving, though DB constraint should catch it
	switch s {
	case DayPreferenceAny, DayPreferenceDay, DayPreferenceNight:
		return string(s), nil
	default:
		return nil, fmt.Errorf("invalid DayPreference value: %s", s)
	}
}

// SearchPace represents the DB ENUM 'search_pace_enum'.
type SearchPace string

const (
	SearchPaceAny      SearchPace = "any"      // No preference
	SearchPaceRelaxed  SearchPace = "relaxed"  // Fewer, longer activities
	SearchPaceModerate SearchPace = "moderate" // Standard pace
	SearchPaceFast     SearchPace = "fast"     // Pack in many activities
)

// Scan implements the sql.Scanner interface for SearchPace.
func (s *SearchPace) Scan(value interface{}) error {
	strVal, ok := value.(string)
	if !ok {
		bytesVal, ok := value.([]byte)
		if !ok {
			return fmt.Errorf("failed to scan SearchPace: expected string or []byte, got %T", value)
		}
		strVal = string(bytesVal)
	}
	switch SearchPace(strVal) {
	case SearchPaceAny, SearchPaceRelaxed, SearchPaceModerate, SearchPaceFast:
		*s = SearchPace(strVal)
		return nil
	default:
		return fmt.Errorf("unknown SearchPace value: %s", strVal)
	}
}

// Value implements the driver.Valuer interface for SearchPace.
func (s SearchPace) Value() (driver.Value, error) {
	switch s {
	case SearchPaceAny, SearchPaceRelaxed, SearchPaceModerate, SearchPaceFast:
		return string(s), nil
	default:
		return nil, fmt.Errorf("invalid SearchPace value: %s", s)
	}
}

type TransportPreference string

const (
	TransportPreferenceAny    TransportPreference = "any"
	TransportPreferenceWalk   TransportPreference = "walk"
	TransportPreferencePublic TransportPreference = "public"
	TransportPreferenceCar    TransportPreference = "car"
)

// Scan implements the sql.Scanner interface for SearchPace.
func (s *TransportPreference) Scan(value interface{}) error {
	strVal, ok := value.(string)
	if !ok {
		bytesVal, ok := value.([]byte)
		if !ok {
			return fmt.Errorf("failed to scan SearchPace: expected string or []byte, got %T", value)
		}
		strVal = string(bytesVal)
	}
	switch TransportPreference(strVal) {
	case TransportPreferenceAny, TransportPreferenceWalk, TransportPreferencePublic, TransportPreferenceCar:
		*s = TransportPreference(strVal)
		return nil
	default:
		return fmt.Errorf("unknown SearchPace value: %s", strVal)
	}
}

// Value implements the driver.Valuer interface for SearchPace.
func (s TransportPreference) Value() (driver.Value, error) {
	switch s {
	case TransportPreferenceAny, TransportPreferenceWalk, TransportPreferencePublic, TransportPreferenceCar:
		return string(s), nil
	default:
		return nil, fmt.Errorf("invalid SearchPace value: %s", s)
	}
}

// UserPreferenceProfileResponse represents a user's saved preference profile.
type UserPreferenceProfileResponse struct {
	ID                   uuid.UUID             `json:"id"`
	UserID               uuid.UUID             `json:"user_id"` // Might omit in some API responses
	ProfileName          string                `json:"profile_name"`
	IsDefault            bool                  `json:"is_default"`
	SearchRadiusKm       float64               `json:"search_radius_km"`
	PreferredTime        DayPreference         `json:"preferred_time"`
	BudgetLevel          int                   `json:"budget_level"`
	PreferredPace        SearchPace            `json:"preferred_pace"`
	PreferAccessiblePOIs bool                  `json:"prefer_accessible_pois"`
	PreferOutdoorSeating bool                  `json:"prefer_outdoor_seating"`
	PreferDogFriendly    bool                  `json:"prefer_dog_friendly"`
	PreferredVibes       []string              `json:"preferred_vibes"` // Assuming TEXT[] maps to []string
	PreferredTransport   TransportPreference   `json:"preferred_transport"`
	DietaryNeeds         []string              `json:"dietary_needs"` // Assuming TEXT[] maps to []string
	Interests            []*interests.Interest `json:"interests"`     // Interests linked to this profile
	Tags                 []*Tags               `json:"tags"`          // Tags to avoid for this profile
	UserLatitude         *float64              `json:"user_latitude"`
	UserLongitude        *float64              `json:"user_longitude"`
	// Enhanced domain-specific preferences
	AccommodationPreferences *AccommodationPreferences `json:"accommodation_preferences,omitempty"`
	DiningPreferences        *DiningPreferences        `json:"dining_preferences,omitempty"`
	ActivityPreferences      *ActivityPreferences      `json:"activity_preferences,omitempty"`
	ItineraryPreferences     *ItineraryPreferences     `json:"itinerary_preferences,omitempty"`
	CreatedAt                time.Time                 `json:"created_at"`
	UpdatedAt                time.Time                 `json:"updated_at"`
}

// CreateUserPreferenceProfileParams defines required fields for creating a new profile.
// Optional fields can be added here or assumed to use DB defaults.
type CreateUserPreferenceProfileParams struct {
	ProfileName              string                    `json:"profile_name"`
	IsDefault                *bool                     `json:"is_default,omitempty"`
	SearchRadiusKm           *float64                  `json:"search_radius_km,omitempty"`
	PreferredTime            *DayPreference            `json:"preferred_time,omitempty"`
	BudgetLevel              *int                      `json:"budget_level,omitempty"`
	PreferredPace            *SearchPace               `json:"preferred_pace,omitempty"`
	PreferAccessiblePOIs     *bool                     `json:"prefer_accessible_pois,omitempty"`
	PreferOutdoorSeating     *bool                     `json:"prefer_outdoor_seating,omitempty"`
	PreferDogFriendly        *bool                     `json:"prefer_dog_friendly,omitempty"`
	PreferredVibes           []string                  `json:"preferred_vibes,omitempty"`
	PreferredTransport       *TransportPreference      `json:"preferred_transport,omitempty"`
	DietaryNeeds             []string                  `json:"dietary_needs,omitempty"`
	Tags                     []uuid.UUID               `json:"tags,omitempty"`
	Interests                []uuid.UUID               `json:"interests,omitempty"`
	AccommodationPreferences *AccommodationPreferences `json:"accommodation_preferences,omitempty"`
	DiningPreferences        *DiningPreferences        `json:"dining_preferences,omitempty"`
	ActivityPreferences      *ActivityPreferences      `json:"activity_preferences,omitempty"`
	ItineraryPreferences     *ItineraryPreferences     `json:"itinerary_preferences,omitempty"`
}

func (p *CreateUserPreferenceProfileParams) UnmarshalJSON(b []byte) error {
	type Alias CreateUserPreferenceProfileParams // Avoid recursion
	aux := struct {
		AccommodationPreferences *AccommodationPreferences `json:"accommodation_preferences"`
		DiningPreferences        *DiningPreferences        `json:"dining_preferences"`
		ActivityPreferences      *ActivityPreferences      `json:"activity_preferences"`
		ItineraryPreferences     *ItineraryPreferences     `json:"itinerary_preferences"`
		*Alias
	}{
		Alias: (*Alias)(p),
	}
	if err := json.Unmarshal(b, &aux); err != nil {
		return err
	}
	p.AccommodationPreferences = aux.AccommodationPreferences
	p.DiningPreferences = aux.DiningPreferences
	p.ActivityPreferences = aux.ActivityPreferences
	p.ItineraryPreferences = aux.ItineraryPreferences
	return nil
}

type UpdateUserPreferenceProfileParams struct {
	ProfileName          string               `json:"profile_name" binding:"required"`
	IsDefault            *bool                `json:"is_default,omitempty"` // Default is FALSE in DB
	SearchRadiusKm       *float64             `json:"search_radius_km,omitempty"`
	PreferredTime        *DayPreference       `json:"preferred_time,omitempty"`
	BudgetLevel          *int                 `json:"budget_level,omitempty"`
	PreferredPace        *SearchPace          `json:"preferred_pace,omitempty"`
	PreferAccessiblePOIs *bool                `json:"prefer_accessible_pois,omitempty"`
	PreferOutdoorSeating *bool                `json:"prefer_outdoor_seating,omitempty"`
	PreferDogFriendly    *bool                `json:"prefer_dog_friendly,omitempty"`
	PreferredVibes       []string             `json:"preferred_vibes,omitempty"` // Use empty slice if not provided?
	PreferredTransport   *TransportPreference `json:"preferred_transport,omitempty"`
	DietaryNeeds         []string             `json:"dietary_needs,omitempty"`
	Tags                 []uuid.UUID          `json:"tags,omitempty"`
	Interests            []uuid.UUID          `json:"interests,omitempty"`
	UpdatedAt            *time.Time           `json:"updated_at,omitempty"` // Optional, can be set to nil
}

type Tags struct {
	ID          uuid.UUID  `json:"id"`
	Name        string     `json:"name"`
	TagType     string     `json:"tag_type"` // Consider using a specific enum type
	Description *string    `json:"description"`
	Source      *string    `json:"source"`
	Active      *bool      `json:"active"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   *time.Time `json:"updated_at"`
}

// UpdateSearchProfileParams defines fields allowed for updating a profile.
// Pointers allow partial updates.
type UpdateSearchProfileParams struct {
	ProfileName          string               `json:"profile_name" binding:"required"`
	IsDefault            *bool                `json:"is_default,omitempty"` // Default is FALSE in DB
	SearchRadiusKm       *float64             `json:"search_radius_km,omitempty"`
	PreferredTime        *DayPreference       `json:"preferred_time,omitempty"`
	BudgetLevel          *int                 `json:"budget_level,omitempty"`
	PreferredPace        *SearchPace          `json:"preferred_pace,omitempty"`
	PreferAccessiblePOIs *bool                `json:"prefer_accessible_pois,omitempty"`
	PreferOutdoorSeating *bool                `json:"prefer_outdoor_seating,omitempty"`
	PreferDogFriendly    *bool                `json:"prefer_dog_friendly,omitempty"`
	PreferredVibes       []string             `json:"preferred_vibes,omitempty"` // Use empty slice if not provided?
	PreferredTransport   *TransportPreference `json:"preferred_transport,omitempty"`
	DietaryNeeds         []string             `json:"dietary_needs,omitempty"`
	Tags                 []*string            `json:"tags,omitempty"`
	Interests            []*string            `json:"interests,omitempty"`
	// Enhanced domain-specific preferences
	AccommodationPreferences *AccommodationPreferences `json:"accommodation_preferences,omitempty"`
	DiningPreferences        *DiningPreferences        `json:"dining_preferences,omitempty"`
	ActivityPreferences      *ActivityPreferences      `json:"activity_preferences,omitempty"`
	ItineraryPreferences     *ItineraryPreferences     `json:"itinerary_preferences,omitempty"`
	UpdateAt                 *time.Time                `json:"updated_at,omitempty"` // Optional, can be set to nil
}

// Domain-specific preference structures

// AccommodationPreferences represents accommodation-specific filters
type AccommodationPreferences struct {
	ID                 uuid.UUID    `json:"id"`
	UserPreferenceID   uuid.UUID    `json:"user_preference_profile_id"`
	AccommodationType  []string     `json:"accommodation_type,omitempty"`    // ["hotel", "hostel", "apartment", "guesthouse", "resort", "boutique"]
	StarRating         *RangeFilter `json:"star_rating,omitempty"`           // 1-5 stars
	PriceRangePerNight *RangeFilter `json:"price_range_per_night,omitempty"` // {"min": 0, "max": 1000}
	Amenities          []string     `json:"amenities,omitempty"`             // ["wifi", "parking", "pool", "gym", "spa", "breakfast", "pet_friendly", "business_center", "concierge"]
	RoomType           []string     `json:"room_type,omitempty"`             // ["single", "double", "suite", "dorm", "private_bathroom", "shared_bathroom"]
	ChainPreference    string       `json:"chain_preference,omitempty"`      // "independent", "major_chains", "boutique_chains"
	CancellationPolicy []string     `json:"cancellation_policy,omitempty"`   // ["free_cancellation", "partial_refund", "non_refundable"]
	BookingFlexibility string       `json:"booking_flexibility,omitempty"`   // "instant_book", "request_only"
	CreatedAt          time.Time    `json:"created_at"`
	UpdatedAt          time.Time    `json:"updated_at"`
}

// DiningPreferences represents dining-specific filters
type DiningPreferences struct {
	ID                   uuid.UUID    `json:"id"`
	UserPreferenceID     uuid.UUID    `json:"user_preference_profile_id"`
	CuisineTypes         []string     `json:"cuisine_types,omitempty"`          // ["italian", "asian", "mediterranean", "mexican", "indian", "french", "american", "local_specialty"]
	MealTypes            []string     `json:"meal_types,omitempty"`             // ["breakfast", "brunch", "lunch", "dinner", "late_night", "snacks"]
	ServiceStyle         []string     `json:"service_style,omitempty"`          // ["fine_dining", "casual", "fast_casual", "street_food", "buffet", "takeaway"]
	PriceRangePerPerson  *RangeFilter `json:"price_range_per_person,omitempty"` // {"min": 0, "max": 200}
	DietaryNeeds         []string     `json:"dietary_needs,omitempty"`          // ["vegetarian", "vegan", "gluten_free", "halal", "kosher"]
	AllergenFree         []string     `json:"allergen_free,omitempty"`          // ["gluten", "nuts", "dairy", "shellfish", "soy"]
	MichelinRated        bool         `json:"michelin_rated,omitempty"`
	LocalRecommendations bool         `json:"local_recommendations,omitempty"`
	ChainVsLocal         string       `json:"chain_vs_local,omitempty"` // "local_only", "chains_ok", "chains_preferred"
	OrganicPreference    bool         `json:"organic_preference,omitempty"`
	OutdoorSeatingPref   bool         `json:"outdoor_seating_preferred,omitempty"`
	CreatedAt            time.Time    `json:"created_at"`
	UpdatedAt            time.Time    `json:"updated_at"`
}

// ActivityPreferences represents activity-specific filters
type ActivityPreferences struct {
	ID                     uuid.UUID `json:"id"`
	UserPreferenceID       uuid.UUID `json:"user_preference_profile_id"`
	ActivityCategories     []string  `json:"activity_categories,omitempty"`       // ["museums", "nightlife", "shopping", "nature", "sports", "arts", "history", "food_tours", "adventure"]
	PhysicalActivityLevel  string    `json:"physical_activity_level,omitempty"`   // "low", "moderate", "high", "extreme"
	IndoorOutdoorPref      string    `json:"indoor_outdoor_preference,omitempty"` // "indoor", "outdoor", "mixed", "weather_dependent"
	CulturalImmersionLevel string    `json:"cultural_immersion_level,omitempty"`  // "tourist", "moderate", "deep_local"
	MustSeeVsHiddenGems    string    `json:"must_see_vs_hidden_gems,omitempty"`   // "popular_attractions", "off_beaten_path", "mixed"
	EducationalPreference  bool      `json:"educational_preference,omitempty"`
	PhotoOpportunities     bool      `json:"photography_opportunities,omitempty"`
	SeasonSpecific         []string  `json:"season_specific_activities,omitempty"` // ["summer_only", "winter_sports", "year_round"]
	AvoidCrowds            bool      `json:"avoid_crowds,omitempty"`
	LocalEventsInterest    []string  `json:"local_events_interest,omitempty"` // ["festivals", "concerts", "sports", "cultural_events", "food_events"]
	CreatedAt              time.Time `json:"created_at"`
	UpdatedAt              time.Time `json:"updated_at"`
}

// ItineraryPreferences represents itinerary planning-specific filters
type ItineraryPreferences struct {
	ID                    uuid.UUID `json:"id"`
	UserPreferenceID      uuid.UUID `json:"user_preference_profile_id"`
	PlanningStyle         string    `json:"planning_style,omitempty"`     // "structured", "flexible", "spontaneous"
	PreferredPace         string    `json:"preferred_pace,omitempty"`     // "relaxed", "moderate", "fast"
	TimeFlexibility       string    `json:"time_flexibility,omitempty"`   // "strict_schedule", "loose_schedule", "completely_flexible"
	MorningVsEvening      string    `json:"morning_vs_evening,omitempty"` // "early_bird", "night_owl", "flexible"
	WeekendVsWeekday      string    `json:"weekend_vs_weekday,omitempty"` // "weekends", "weekdays", "any"
	PreferredSeasons      []string  `json:"preferred_seasons,omitempty"`  // ["spring", "summer", "fall", "winter"]
	AvoidPeakSeason       bool      `json:"avoid_peak_season,omitempty"`
	AdventureVsRelaxation string    `json:"adventure_vs_relaxation,omitempty"` // "adventure_focused", "relaxation_focused", "balanced"
	SpontaneousVsPlanned  string    `json:"spontaneous_vs_planned,omitempty"`  // "highly_planned", "semi_planned", "spontaneous"
	CreatedAt             time.Time `json:"created_at"`
	UpdatedAt             time.Time `json:"updated_at"`
}

// RangeFilter represents a min/max range filter
type RangeFilter struct {
	Min *float64 `json:"min,omitempty"`
	Max *float64 `json:"max,omitempty"`
}

// DomainType represents different search domains
type DomainType string

const (
	DomainGeneral       DomainType = "general"
	DomainAccommodation DomainType = "accommodation"
	DomainDining        DomainType = "dining"
	DomainActivities    DomainType = "activities"
	DomainItinerary     DomainType = "itinerary"
	DomainTransport     DomainType = "transport"
)

// CombinedFilters represents merged filters from all domains
type CombinedFilters struct {
	BasePreferences          *UserPreferenceProfileResponse `json:"base_preferences"`
	AccommodationPreferences *AccommodationPreferences      `json:"accommodation_preferences,omitempty"`
	DiningPreferences        *DiningPreferences             `json:"dining_preferences,omitempty"`
	ActivityPreferences      *ActivityPreferences           `json:"activity_preferences,omitempty"`
	ItineraryPreferences     *ItineraryPreferences          `json:"itinerary_preferences,omitempty"`
	InferredFilters          map[string]interface{}         `json:"inferred_filters,omitempty"`
}
