package models

import (
	"time"

	"github.com/google/uuid"
)

// List represents a user-created collection of POIs
type List struct {
	Base
	UserID      uuid.UUID `json:"user_id" db:"user_id"`
	Name        string    `json:"name" db:"name"`
	Description string    `json:"description" db:"description"`
	ImageURL    string    `json:"image_url" db:"image_url"`
	IsPublic    bool      `json:"is_public" db:"is_public"`
	IsItinerary bool      `json:"is_itinerary" db:"is_itinerary"`
	CityID      *uuid.UUID `json:"city_id" db:"city_id"`
	ItemCount   int       `json:"item_count" db:"item_count"`
	ViewCount   int       `json:"view_count" db:"view_count"`
	SaveCount   int       `json:"save_count" db:"save_count"`
}

// ContentType represents the type of content in a list item
type ContentType string

const (
	ContentTypePOI        ContentType = "poi"
	ContentTypeRestaurant ContentType = "restaurant"
	ContentTypeHotel      ContentType = "hotel"
	ContentTypeItinerary  ContentType = "itinerary"
)

// ListItem represents any type of content in a list, with optional ordering for itineraries
type ListItem struct {
	ListID                  uuid.UUID  `json:"list_id" db:"list_id"`
	ItemID                  uuid.UUID  `json:"item_id" db:"item_id"`
	ContentType             ContentType `json:"content_type" db:"content_type"`
	Position                int        `json:"position" db:"position"`
	Notes                   string     `json:"notes" db:"notes"`
	DayNumber               *int       `json:"day_number" db:"day_number"`
	TimeSlot                *time.Time `json:"time_slot" db:"time_slot"`
	Duration                *int       `json:"duration" db:"duration"`
	SourceLLMInteractionID  *uuid.UUID `json:"source_llm_interaction_id" db:"source_llm_interaction_id"`
	ItemAIDescription       string     `json:"item_ai_description" db:"item_ai_description"`
	CreatedAt               time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt               time.Time  `json:"updated_at" db:"updated_at"`
	
	// Backward compatibility - populated only for POI content type
	POIID *uuid.UUID `json:"poi_id,omitempty" db:"-"`
}

// SavedList represents a user saving another user's public list
type SavedList struct {
	UserID    uuid.UUID `json:"user_id" db:"user_id"`
	ListID    uuid.UUID `json:"list_id" db:"list_id"`
	SavedAt   time.Time `json:"saved_at" db:"saved_at"`
}

// NewList creates a new list with default values
func NewList(userID uuid.UUID, name, description string, isPublic, isItinerary bool, cityID *uuid.UUID) *List {
	return &List{
		Base: Base{
			ID:        uuid.New(),
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		},
		UserID:      userID,
		Name:        name,
		Description: description,
		IsPublic:    isPublic,
		IsItinerary: isItinerary,
		CityID:      cityID,
		ItemCount:   0,
		ViewCount:   0,
		SaveCount:   0,
	}
}

// NewListItem creates a new list item with generic content type
func NewListItem(listID, itemID uuid.UUID, contentType ContentType, position int, notes string) *ListItem {
	now := time.Now()
	item := &ListItem{
		ListID:      listID,
		ItemID:      itemID,
		ContentType: contentType,
		Position:    position,
		Notes:       notes,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	
	// Set backward compatibility field for POI type
	if contentType == ContentTypePOI {
		item.POIID = &itemID
	}
	
	return item
}

// NewPOIListItem creates a new POI list item (backward compatibility)
func NewPOIListItem(listID, poiID uuid.UUID, position int, notes string) *ListItem {
	return NewListItem(listID, poiID, ContentTypePOI, position, notes)
}

// IsValid checks if the list item has valid content type
func (li *ListItem) IsValid() bool {
	switch li.ContentType {
	case ContentTypePOI, ContentTypeRestaurant, ContentTypeHotel, ContentTypeItinerary:
		return true
	default:
		return false
	}
}

// NewSavedList creates a new saved list record
func NewSavedList(userID, listID uuid.UUID) *SavedList {
	return &SavedList{
		UserID:  userID,
		ListID:  listID,
		SavedAt: time.Now(),
	}
}