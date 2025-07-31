package chat_prompt

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"google.golang.org/genai"

	"github.com/FACorreiaa/go-poi-au-suggestions/internal/domain/poi"
	"github.com/FACorreiaa/go-poi-au-suggestions/internal/domain/profiles"
)

type LlmInteraction struct {
	ID                 uuid.UUID       `json:"id"`
	SessionID          uuid.UUID       `json:"session_id"`
	UserID             uuid.UUID       `json:"user_id"`
	ProfileID          uuid.UUID       `json:"profile_id"`
	CityName           string          `json:"city_name,omitempty"` // The city context for this interaction
	Prompt             string          `json:"prompt"`
	RequestPayload     json.RawMessage `json:"request_payload"`
	ResponseText       string          `json:"response"`
	ResponsePayload    json.RawMessage `json:"response_payload"`
	ModelUsed          string          `json:"model_name"`
	PromptTokens       int             `json:"prompt_tokens"`
	CompletionTokens   int             `json:"completion_tokens"`
	TotalTokens        int             `json:"total_tokens"`
	LatencyMs          int             `json:"latency_ms"`
	Timestamp          time.Time       `json:"timestamp"`
	ModelName          string          `json:"model"`
	Response           string          `json:"response_content"`
	Latitude           *float64        `json:"latitude"`
	Longitude          *float64        `json:"longitude"`
	Distance           *float64        `json:"distance"`
	PromptTokenCount   int             `json:"prompt_token_count"`
	ResponseTokenCount int             `json:"response_token_count"`
}

type AIItineraryResponse struct {
	ItineraryName      string                `json:"itinerary_name"`
	OverallDescription string                `json:"overall_description"`
	PointsOfInterest   []poi.POIDetailedInfo `json:"points_of_interest"`
	Restaurants        []poi.POIDetailedInfo `json:"restaurants,omitempty"`
	Bars               []poi.POIDetailedInfo `json:"bars,omitempty"`
}

type GeneralCityData struct {
	City            string  `json:"city"`
	Country         string  `json:"country"`
	StateProvince   string  `json:"state_province,omitempty"`
	Description     string  `json:"description"`
	CenterLatitude  float64 `json:"center_latitude,omitempty"`
	CenterLongitude float64 `json:"center_longitude,omitempty"`
	Population      string  `json:"population"`
	Area            string  `json:"area"`
	Timezone        string  `json:"timezone"`
	Language        string  `json:"language"`
	Weather         string  `json:"weather"`
	Attractions     string  `json:"attractions"`
	History         string  `json:"history"`
}

type AiCityResponse struct {
	GeneralCityData     GeneralCityData       `json:"general_city_data"`
	PointsOfInterest    []poi.POIDetailedInfo `json:"points_of_interest"`
	AIItineraryResponse AIItineraryResponse   `json:"itinerary_response"`
	SessionID           uuid.UUID             `json:"session_id"`
}

type GenAIResponse struct {
	SessionID            string                `json:"session_id"`
	LlmInteractionID     uuid.UUID             `json:"llm_interaction_id"`
	City                 string                `json:"city,omitempty"`
	Country              string                `json:"country,omitempty"`
	StateProvince        string                `json:"state_province,omitempty"` // New
	CityDescription      string                `json:"city_description,omitempty"`
	Latitude             float64               `json:"latitude,omitempty"`  // New: for city center
	Longitude            float64               `json:"longitude,omitempty"` // New: for city center
	ItineraryName        string                `json:"itinerary_name,omitempty"`
	ItineraryDescription string                `json:"itinerary_description,omitempty"`
	GeneralPOI           []poi.POIDetailedInfo `json:"general_poi,omitempty"`
	PersonalisedPOI      []poi.POIDetailedInfo `json:"personalised_poi,omitempty"` // Consider changing to []PersonalizedPOIDetail
	POIDetailedInfo      []poi.POIDetailedInfo `json:"poi_detailed_info,omitempty"`
	Err                  error                 `json:"-"`
	ModelName            string                `json:"model_name"`
	Prompt               string                `json:"prompt"`
	Response             string                `json:"response"`
}

type AIRequestPayloadForLog struct {
	ModelName        string                       `json:"model_name"`
	GenerationConfig *genai.GenerateContentConfig `json:"generation_config,omitempty"`
	Content          *genai.Content               `json:"content"` // The actual content sent (prompt)
	// You could add other things like "tools" if you use function calling
}

type ChatTurn struct { // You might not need this explicit struct if directly using []*genai.Content
	Role  string       `json:"role"` // "user" or "model"
	Parts []genai.Part `json:"parts"`
}

type UserLocation struct {
	UserLat        float64 `json:"user_lat"`
	UserLon        float64 `json:"user_lon"`
	SearchRadiusKm float64 // Radius in kilometers for searching nearby POIs
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

type UpdateItineraryRequest struct {
	Title                 *string  `json:"title,omitempty"`
	Description           *string  `json:"description,omitempty"` // If nil, means no change. If empty string, means clear description.
	Tags                  []string `json:"tags,omitempty"`        // If nil, no change. If empty slice, clear tags.
	EstimatedDurationDays *int32   `json:"estimated_duration_days,omitempty"`
	EstimatedCostLevel    *int32   `json:"estimated_cost_level,omitempty"`
	IsPublic              *bool    `json:"is_public,omitempty"`
	MarkdownContent       *string  `json:"markdown_content,omitempty"`
}

type PaginatedUserItinerariesResponse struct {
	Itineraries  []UserSavedItinerary `json:"itineraries"`
	TotalRecords int                  `json:"total_records"`
	Page         int                  `json:"page"`
	PageSize     int                  `json:"page_size"`
}

type BookmarkRequest struct {
	LlmInteractionID *uuid.UUID `json:"llm_interaction_id,omitempty"` // Optional - if provided, use this specific interaction
	SessionID        *uuid.UUID `json:"session_id,omitempty"`         // Optional - if provided, use latest interaction from this session
	PrimaryCityID    *uuid.UUID `json:"primary_city_id,omitempty"`    // Optional - if provided, use this
	PrimaryCityName  string     `json:"primary_city_name"`            // City name to look up if PrimaryCityID not provided
	Title            string     `json:"title"`
	Description      *string    `json:"description"` // Optional
	Tags             []string   `json:"tags"`        // Optional
	IsPublic         *bool      `json:"is_public"`   // Optional
}

type ChatMessage struct {
	ID        uuid.UUID
	UserID    uuid.UUID
	Timestamp time.Time
	Role      string
	Content   string
}

type POIDetailrequest struct {
	CityName  string  `json:"city_name"` // e.g., "New York"
	Latitude  float64 `json:"latitude"`  // e.g., 40.7128
	Longitude float64 `json:"longitude"` // e.g., -74.0060
}

type GeoPoint struct {
	Latitude  float64 `json:"latitude"`  // Latitude of the point
	Longitude float64 `json:"longitude"` // Longitude of the point
}
type SearchPOIRequest struct {
	Query      string   `json:"query"` // The search query text
	CityName   string   `json:"city"`
	Latitude   float64  `json:"lat"`
	Longitude  float64  `json:"lon"`
	RadiusKm   float64  `json:"radius_km"`   // Optional, for filtering POIs within a certain radius
	SearchText string   `json:"search_text"` // Optional, for searching by name or description
	SearchTags []string `json:"search_tags"` // Optional, for filtering by tags
	SearchType string   `json:"search_type"` // Optional, e.g., "restaurant", "hotel", "bar"
	SortBy     string   `json:"sort_by"`     // Optional, e.g., "rating", "distance"
	SortOrder  string   `json:"sort_order"`  // Optional, e.g., "asc", "desc"
	MinRating  float64  `json:"min_rating"`  // Optional, for filtering by minimum rating
	MinPrice   string   `json:"min_price"`   // Optional, for filtering by minimum price range
	MinGuests  int32    `json:"min_guests"`  // Optional, for filtering by minimum number of guests (for restaurants)
}

type HotelUserPreferences struct {
	NumberOfGuests      int32     `json:"number_of_guests"`
	PreferredCategories string    `json:"preferred_category"`    // e.g., "budget", "luxury"
	PreferredTags       []string  `json:"preferredTags"`         // e.g., ["pet-friendly", "free wifi"]
	MaxPriceRange       string    `json:"preferred_price_range"` // e.g., "$", "$$"
	MinRating           float64   `json:"preferred_rating"`      // e.g., 4.0
	NumberOfNights      int64     `json:"number_of_nights"`
	NumberOfRooms       int32     `json:"number_of_rooms"`
	PreferredCheckIn    time.Time `json:"preferred_check_in"`
	PreferredCheckOut   time.Time `json:"preferred_check_out"`
	SearchRadiusKm      float64   `json:"search_radius_km"` // Optional, for filtering hotels within a certain radius
}

type HotelPreferenceRequest struct {
	City        string               `json:"city"`
	Lat         float64              `json:"lat"`
	Lon         float64              `json:"lon"`
	Preferences HotelUserPreferences `json:"preferences"`
	Distance    float64              `json:"distance"` // Optional, for filtering hotels within a certain radius
}

type RestaurantUserPreferences struct {
	PreferredCuisine    string
	PreferredPriceRange string
	DietaryRestrictions string
	Ambiance            string
	SpecialFeatures     string
}

// Context-aware chat types
type ChatContextType string

const (
	ContextHotels      ChatContextType = "hotels"
	ContextRestaurants ChatContextType = "restaurants"
	ContextItineraries ChatContextType = "itineraries"
	ContextGeneral     ChatContextType = "general"
)

type StartChatRequest struct {
	CityName       string          `json:"city_name"`
	ContextType    ChatContextType `json:"context_type"`
	InitialMessage string          `json:"initial_message,omitempty"`
}

type ContinueChatRequest struct {
	Message     string          `json:"message"`
	CityName    string          `json:"city_name,omitempty"`
	ContextType ChatContextType `json:"context_type"`
}

//

type SimpleIntentClassifier struct{}

func (c *SimpleIntentClassifier) Classify(_ context.Context, message string) (IntentType, error) {
	message = strings.ToLower(message)
	matched, err := regexp.MatchString(`add|include|visit`, message)
	if err != nil {
		return IntentModifyItinerary, fmt.Errorf("failed to match add pattern: %w", err)
	}
	if matched {
		return IntentAddPOI, nil
	}
	matched, err = regexp.MatchString(`remove|delete|skip`, message)
	if err != nil {
		return IntentModifyItinerary, fmt.Errorf("failed to match remove pattern: %w", err)
	}
	if matched {
		return IntentRemovePOI, nil
	}
	matched, err = regexp.MatchString(`what|where|how|why|when`, message)
	if err != nil {
		return IntentModifyItinerary, fmt.Errorf("failed to match question pattern: %w", err)
	}
	if matched {
		return IntentAskQuestion, nil
	}
	return IntentModifyItinerary, nil // Default intent
}

// DomainDetector detects the primary domain from user queries
type DomainDetector struct{}

func (d *DomainDetector) DetectDomain(_ context.Context, message string) profiles.DomainType {
	message = strings.ToLower(message)

	// Accommodation domain keywords
	matched, err := regexp.MatchString(`hotel|hostel|accommodation|stay|sleep|room|booking|airbnb|lodge|resort|guesthouse`, message)
	if err == nil && matched {
		return profiles.DomainAccommodation
	}

	// Dining domain keywords
	matched, err = regexp.MatchString(`restaurant|food|eat|dine|meal|cuisine|drink|cafe|bar|lunch|dinner|breakfast|brunch`, message)
	if err == nil && matched {
		return profiles.DomainDining
	}

	// Activity domain keywords
	matched, err = regexp.MatchString(`activity|museum|park|attraction|tour|visit|see|do|experience|adventure|shopping|nightlife`, message)
	if err == nil && matched {
		return profiles.DomainActivities
	}

	// Itinerary domain keywords
	matched, err = regexp.MatchString(`itinerary|plan|schedule|trip|day|week|journey|route|organize|arrange`, message)
	if err == nil && matched {
		return profiles.DomainItinerary
	}

	// Default to general domain
	return profiles.DomainGeneral
}

// RecentInteraction represents a recent user interaction with cities and POIs
type RecentInteraction struct {
	ID           uuid.UUID                    `json:"id"`
	UserID       uuid.UUID                    `json:"user_id"`
	CityName     string                       `json:"city_name"`
	CityID       *uuid.UUID                   `json:"city_id,omitempty"`
	Prompt       string                       `json:"prompt"`
	ResponseText string                       `json:"response,omitempty"`
	ModelUsed    string                       `json:"model_name"`
	LatencyMs    int                          `json:"latency_ms"`
	CreatedAt    time.Time                    `json:"created_at"`
	POIs         []poi.POIDetailedInfo        `json:"pois,omitempty"`
	Hotels       []poi.HotelDetailedInfo      `json:"hotels,omitempty"`
	Restaurants  []poi.RestaurantDetailedInfo `json:"restaurants,omitempty"`
}

// RecentInteractionsResponse groups interactions by city
type RecentInteractionsResponse struct {
	Cities  []CityInteractions `json:"cities"`
	Total   int                `json:"total"`
	Page    int                `json:"page"`
	Limit   int                `json:"limit"`
	HasMore bool               `json:"has_more"`
}

// CityInteractions groups interactions for a specific city
type CityInteractions struct {
	CityName         string              `json:"city_name"`
	SessionID        uuid.UUID           `json:"session_id"`
	Interactions     []RecentInteraction `json:"interactions"`
	POICount         int                 `json:"poi_count"`
	LastActivity     time.Time           `json:"last_activity"`
	SessionIDs       []uuid.UUID         `json:"session_ids"` // Changed from SessionID
	Title            string              `json:"title"`
	TotalFavorites   *int                `json:"total_favorites,omitempty"`
	TotalItineraries *int                `json:"total_itineraries,omitempty"`
}

// RecentInteractionsFilter defines filters for recent interactions
type RecentInteractionsFilter struct {
	SortBy          string `json:"sort_by"`          // last_activity, city_name, interaction_count, poi_count
	SortOrder       string `json:"sort_order"`       // asc, desc
	Search          string `json:"search"`           // Search term for city name
	MinInteractions int    `json:"min_interactions"` // Minimum number of interactions
	MaxInteractions int    `json:"max_interactions"` // Maximum number of interactions
}

const (
	SendEventTimeout     = 2 * time.Second
	ContinueEventTimeout = 3 * time.Second
)

type ChatSession struct {
	ID                  uuid.UUID             `json:"id"`
	UserID              uuid.UUID             `json:"user_id"`
	ProfileID           uuid.UUID             `json:"profile_id"`
	CityName            string                `json:"city_name"`
	CurrentItinerary    *AiCityResponse       `json:"current_itinerary,omitempty"`
	ConversationHistory []ConversationMessage `json:"conversation_history"`
	SessionContext      SessionContext        `json:"session_context"` // cigty_name: Barcelona
	CreatedAt           time.Time             `json:"created_at"`
	UpdatedAt           time.Time             `json:"updated_at"`
	ExpiresAt           time.Time             `json:"expires_at"`
	Status              SessionStatus         `json:"status"` // "active", "expired", etc.

	// Enriched fields for better chat history display
	PerformanceMetrics SessionPerformanceMetrics `json:"performance_metrics"`
	ContentMetrics     SessionContentMetrics     `json:"content_metrics"`
	EngagementMetrics  SessionEngagementMetrics  `json:"engagement_metrics"`
}

type ConversationMessage struct {
	ID          uuid.UUID       `json:"id"`
	Role        MessageRole     `json:"role"` // user, assistant, system
	Content     string          `json:"content"`
	MessageType MessageType     `json:"message_type"` // initial_request, modification_request, response
	Timestamp   time.Time       `json:"timestamp"`
	Metadata    MessageMetadata `json:"metadata,omitempty"`
}

type MessageRole string

const (
	RoleUser      MessageRole = "user"
	RoleAssistant MessageRole = "assistant"
	RoleSystem    MessageRole = "system"
)

type MessageType string

const (
	TypeInitialRequest      MessageType = "initial_request"
	TypeModificationRequest MessageType = "modification_request"
	TypeResponse            MessageType = "response"
	TypeClarification       MessageType = "clarification"
	TypeItineraryResponse               = "itinerary_response"
	TypeError               MessageType = "error" // For errors or unhandled cases
)

type MessageMetadata struct {
	LlmInteractionID *uuid.UUID `json:"llm_interaction_id,omitempty"`
	ModifiedPOICount int        `json:"modified_poi_count,omitempty"`
	RequestType      string     `json:"request_type,omitempty"` // add_poi, remove_poi, modify_preferences, etc.
}

type SessionContext struct {
	CityName            string                                  `json:"city_name"` // e.g., "Barcelona"
	LastCityID          uuid.UUID                               `json:"last_city_id"`
	UserPreferences     *profiles.UserPreferenceProfileResponse `json:"user_preferences"`
	ActiveInterests     []string                                `json:"active_interests"`
	ActiveTags          []string                                `json:"active_tags"`
	ConversationSummary string                                  `json:"conversation_summary"`
	ModificationHistory []ModificationRecord                    `json:"modification_history"`
}

type ModificationRecord struct {
	Type        string    `json:"type"` // add_poi, remove_poi, change_preferences
	Description string    `json:"description"`
	Timestamp   time.Time `json:"timestamp"`
	Applied     bool      `json:"applied"`
}

type SessionStatus string

const (
	StatusActive  SessionStatus = "active"
	StatusExpired SessionStatus = "expired"
	StatusClosed  SessionStatus = "closed"
)

// Request/Response types for chat API
type ChatRequest struct {
	SessionID *uuid.UUID `json:"session_id,omitempty"` // nil for new session
	Message   string     `json:"message"`
	CityName  string     `json:"city_name,omitempty"` // required for new session
}

type ChatResponse struct {
	SessionID             uuid.UUID       `json:"session_id"`
	Message               string          `json:"message"`
	UpdatedItinerary      *AiCityResponse `json:"updated_itinerary,omitempty"`
	IsNewSession          bool            `json:"is_new_session"`
	RequiresClarification bool            `json:"requires_clarification"`
	SuggestedActions      []string        `json:"suggested_actions,omitempty"`
}

// Session Repository Interface
type ChatSessionRepository interface {
	CreateSession(ctx context.Context, session ChatSession) error
	GetSession(ctx context.Context, sessionID uuid.UUID) (*ChatSession, error)
	UpdateSession(ctx context.Context, session ChatSession) error
	AddMessageToSession(ctx context.Context, sessionID uuid.UUID, message ConversationMessage) error
	GetActiveSessionsForUser(ctx context.Context, userID uuid.UUID) ([]ChatSession, error)
	ExpireSession(ctx context.Context, sessionID uuid.UUID) error
	CleanupExpiredSessions(ctx context.Context) error
}

// Chat Service Interface
type ChatService interface {
	StartNewSession(ctx context.Context, userID, profileID uuid.UUID, cityName, initialMessage string) (*ChatResponse, error)
	ContinueSession(ctx context.Context, sessionID uuid.UUID, message string) (*ChatResponse, error)
	GetSessionHistory(ctx context.Context, sessionID uuid.UUID) (*ChatSession, error)
	EndSession(ctx context.Context, sessionID uuid.UUID) error
}

type StreamingChatEvent struct {
	Timestamp        time.Time             `json:"timestamp"`  // Time of the event
	EventID          string                `json:"event_id"`   // Unique identifier for the event
	EventType        string                `json:"event_type"` // e.g., "session_started", "city_info", "general_pois", "personalized_poi_chunk", "final_itinerary", "error"
	SessionID        uuid.UUID             `json:"session_id,omitempty"`
	Message          string                `json:"message,omitempty"` // For general messages or errors
	CityData         *GeneralCityData      `json:"city_data,omitempty"`
	GeneralPOIs      []poi.POIDetailedInfo `json:"general_pois,omitempty"`
	PersonalizedPOIs []poi.POIDetailedInfo `json:"personalized_pois,omitempty"` // Could send chunks or final list
	Itinerary        *AiCityResponse       `json:"itinerary,omitempty"`         // Could be a partial or final one
	Error            string                `json:"error_message,omitempty"`
	IsFinal          bool                  `json:"is_final,omitempty"` // Indicates the end of a sequence or the whole stream
	// Add any other relevant data for different event types
}

type Intent struct {
	Type           IntentType             `json:"type"`
	Confidence     float64                `json:"confidence"`
	Entities       map[string]interface{} `json:"entities"` // Good for flexibility
	RequiredAction ActionType             `json:"required_action"`
}

type IntentType string

const (
	IntentInitialRequest    IntentType = "initial_request" // Might not be needed if StartNewSession is distinct
	IntentAddPOI            IntentType = "add_poi"
	IntentRemovePOI         IntentType = "remove_poi"
	IntentModifyItinerary   IntentType = "modify_itinerary" // General modification
	IntentChangePreferences IntentType = "change_preferences"
	IntentAskQuestion       IntentType = "ask_question"
	IntentClarification     IntentType = "clarification"    // Bot asks user for clarification
	IntentProvideFeedback   IntentType = "provide_feedback" // User gives feedback
	IntentChitChat          IntentType = "chit_chat"        // Non-task oriented conversation
	// Add more specific intents as your bot's capabilities grow
	IntentGetPOIDetails   IntentType = "get_poi_details"
	IntentFindHotels      IntentType = "find_hotels"
	IntentFindRestaurants IntentType = "find_restaurants"
	IntentReplacePOI      IntentType = "replace_poi" // More specific than general modify
	IntentChangeDate      IntentType = "change_date"
	IntentChangeLocation  IntentType = "change_location" // For the whole trip or part of it
	IntentSortItinerary   IntentType = "sort_itinerary"  // e.g., "sort by distance", "optimize for morning"
)

type ActionType string

const (
	ActionGenerateNew          ActionType = "generate_new"          // e.g., new itinerary, new POI details from scratch
	ActionUpdateExisting       ActionType = "update_existing"       // e.g., modify current itinerary, update preferences
	ActionProvideInfo          ActionType = "provide_info"          // e.g., answer a question, get POI details
	ActionRequestClarification ActionType = "request_clarification" // Bot needs more info from user
	ActionStoreFeedback        ActionType = "store_feedback"
	ActionNoOp                 ActionType = "no_op"             // For chit-chat or unhandled
	ActionExecuteDBQuery       ActionType = "execute_db_query"  // For simple info retrieval
	ActionCallExternalAPI      ActionType = "call_external_api" // e.g., weather, flight status
)

// StreamEvent represents different types of streaming events
type StreamEvent struct {
	Type       string          `json:"type"`
	Message    string          `json:"message"`
	Data       interface{}     `json:"data,omitempty"`
	Error      string          `json:"error,omitempty"`
	Timestamp  time.Time       `json:"timestamp"`
	EventID    string          `json:"event_id"`
	IsFinal    bool            `json:"is_final,omitempty"`
	Navigation *NavigationData `json:"navigation,omitempty"`
}

// NavigationData contains information for URL navigation
type NavigationData struct {
	URL         string            `json:"url"`
	RouteType   string            `json:"route_type"`   // "itinerary", "restaurants", "activities", "hotels"
	QueryParams map[string]string `json:"query_params"` // sessionId, cityName, domain, etc.
}

// StreamEventType constants
const (
	EventTypeStart           = "start"
	EventTypeProgress        = "progress"
	EventTypeCityData        = "city_data"
	EventTypeGeneralPOI      = "general_poi"
	EventTypePersonalizedPOI = "personalized_poi"
	EventTypeItinerary       = "itinerary"
	EventTypeMessage         = "message"
	EventTypeError           = "error"
	EventTypeComplete        = "complete"
	EventTypeDomainDetected  = "domain_detected"
	EventTypePromptGenerated = "prompt_generated"
	EventTypeParsingResponse = "parsing_response"
	EventTypeUnifiedChat     = "unified_chat"
	EventTypeHotels          = "hotels"
	EventTypeRestaurants     = "restaurants"
	EventTypeChunk           = "chunk" // For immediate text chunks (Google GenAI pattern)
)

// StreamingResponse wraps the streaming channel and metadata
type StreamingResponse struct {
	SessionID uuid.UUID
	Stream    <-chan StreamEvent
	Cancel    context.CancelFunc
}

// SessionPerformanceMetrics contains performance-related metrics for chat sessions
type SessionPerformanceMetrics struct {
	AvgResponseTimeMs int      `json:"avg_response_time_ms"`
	TotalTokens       int      `json:"total_tokens"`
	PromptTokens      int      `json:"prompt_tokens"`
	CompletionTokens  int      `json:"completion_tokens"`
	ModelsUsed        []string `json:"models_used"`
	TotalLatencyMs    int      `json:"total_latency_ms"`
}

// SessionContentMetrics contains content-related metrics for chat sessions
type SessionContentMetrics struct {
	TotalPOIs          int      `json:"total_pois"`
	TotalHotels        int      `json:"total_hotels"`
	TotalRestaurants   int      `json:"total_restaurants"`
	CitiesCovered      []string `json:"cities_covered"`
	HasItinerary       bool     `json:"has_itinerary"`
	ComplexityScore    int      `json:"complexity_score"` // 1-10 based on content richness
	DominantCategories []string `json:"dominant_categories"`
}

// SessionEngagementMetrics contains engagement-related metrics for chat sessions
type SessionEngagementMetrics struct {
	MessageCount          int           `json:"message_count"`
	ConversationDuration  time.Duration `json:"conversation_duration"`
	UserMessageCount      int           `json:"user_message_count"`
	AssistantMessageCount int           `json:"assistant_message_count"`
	AvgMessageLength      int           `json:"avg_message_length"`
	PeakActivityTime      *time.Time    `json:"peak_activity_time,omitempty"`
	EngagementLevel       string        `json:"engagement_level"` // "low", "medium", "high"
}

// ChatSessionsResponse represents paginated chat sessions response
type ChatSessionsResponse struct {
	Sessions []ChatSession `json:"sessions"`
	Total    int           `json:"total"`
	Page     int           `json:"page"`
	Limit    int           `json:"limit"`
	HasMore  bool          `json:"has_more"`
}
