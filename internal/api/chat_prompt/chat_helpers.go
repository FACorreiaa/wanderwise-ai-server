package llmchat

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/google/uuid"

	"github.com/FACorreiaa/go-poi-au-suggestions/internal/types"
	"github.com/FACorreiaa/go-poi-au-suggestions/internal/utils"
)

func generatePOICacheKey(city string, lat, lon, distance float64, userID uuid.UUID) string {
	return fmt.Sprintf("poi:%s:%f:%f:%f:%s", city, lat, lon, distance, userID.String())
}

// extractPOIName extracts the full POI name from the message
func extractPOIName(message string) string {
	// Remove common words and keep the rest as the POI name
	words := strings.Fields(strings.ToLower(message))
	filtered := []string{}
	stopWords := map[string]bool{
		"add": true, "remove": true, "to": true, "from": true, "my": true,
		"itinerary": true, "with": true, "replace": true, "the": true, "in": true,
	}
	for _, w := range words {
		if !stopWords[w] {
			filtered = append(filtered, w)
		}
	}
	if len(filtered) == 0 {
		return "Unknown POI"
	}
	// Capitalize each word for proper formatting
	// cases.Title
	// use this https://pkg.go.dev/golang.org/x/text/cases later and handle language as well
	// TODO: Replace with golang.org/x/text/cases.Title for proper Unicode support
	// For now, use a simple manual title case implementation
	words = strings.Split(strings.Join(filtered, " "), " ")
	for i, word := range words {
		if len(word) > 0 {
			words[i] = strings.ToUpper(word[:1]) + strings.ToLower(word[1:])
		}
	}
	return strings.Join(words, " ")
}

// helpers

func (l *ServiceImpl) ProcessAndSaveUnifiedResponse(
	ctx context.Context,
	responses map[string]*strings.Builder,
	userID, profileID, cityID uuid.UUID,
	llmInteractionID uuid.UUID,
	userLocation *types.UserLocation,
) {
	l.logger.InfoContext(ctx, "Processing unified response for POI extraction",
		slog.String("city_id", cityID.String()),
		slog.Int("response_parts", len(responses)))

	// Process general POIs if available
	if poisContent, ok := responses["general_pois"]; ok && poisContent.Len() > 0 {
		l.logger.InfoContext(ctx, "Processing general POIs from unified response",
			slog.Int("content_length", poisContent.Len()))
		l.handleGeneralPoisFromResponse(ctx, poisContent.String(), cityID)
	}

	// Process itinerary POIs if available
	if itineraryContent, ok := responses["itinerary"]; ok && itineraryContent.Len() > 0 {
		l.logger.InfoContext(ctx, "Processing itinerary POIs from unified response",
			slog.Int("content_length", itineraryContent.Len()))
		l.handleItineraryFromResponse(ctx, itineraryContent.String(), userID, profileID, cityID, llmInteractionID, userLocation)
	}

	// Process activities POIs if available (for DomainActivities)
	if activitiesContent, ok := responses["activities"]; ok && activitiesContent.Len() > 0 {
		l.logger.InfoContext(ctx, "Processing activities POIs from unified response",
			slog.Int("content_length", activitiesContent.Len()))
		l.handleGeneralPoisFromResponse(ctx, activitiesContent.String(), cityID)
	}

	// Process hotel POIs if available (for DomainAccommodation)
	if hotelsContent, ok := responses["hotels"]; ok && hotelsContent.Len() > 0 {
		l.logger.InfoContext(ctx, "Processing hotels from unified response",
			slog.Int("content_length", hotelsContent.Len()))
		l.handleHotelsFromResponse(ctx, hotelsContent.String(), cityID, userID, llmInteractionID)
	}

	// Process restaurant POIs if available (for DomainDining)
	if restaurantsContent, ok := responses["restaurants"]; ok && restaurantsContent.Len() > 0 {
		l.logger.InfoContext(ctx, "Processing restaurants from unified response",
			slog.Int("content_length", restaurantsContent.Len()))
		l.handleRestaurantsFromResponse(ctx, restaurantsContent.String(), cityID, userID, llmInteractionID)
	}
}

func (l *ServiceImpl) ProcessAndSaveUnifiedResponseFree(
	ctx context.Context,
	responses map[string]*strings.Builder,
	cityID uuid.UUID,
	llmInteractionID uuid.UUID,
	userLocation *types.UserLocation,
) {
	l.logger.InfoContext(ctx, "Processing unified response for POI extraction",
		slog.String("city_id", cityID.String()),
		slog.Int("response_parts", len(responses)))

	// Process general POIs if available
	if poisContent, ok := responses["general_pois"]; ok && poisContent.Len() > 0 {
		l.logger.InfoContext(ctx, "Processing general POIs from unified response",
			slog.Int("content_length", poisContent.Len()))
		l.handleGeneralPoisFromResponse(ctx, poisContent.String(), cityID)
	}

	// Process itinerary POIs if available
	if itineraryContent, ok := responses["itinerary"]; ok && itineraryContent.Len() > 0 {
		l.logger.InfoContext(ctx, "Processing itinerary POIs from unified response",
			slog.Int("content_length", itineraryContent.Len()))
		l.handleItineraryFromResponse(ctx, itineraryContent.String(), uuid.Nil, uuid.Nil, cityID, llmInteractionID, userLocation)
	}

	// Process activities POIs if available (for DomainActivities)
	if activitiesContent, ok := responses["activities"]; ok && activitiesContent.Len() > 0 {
		l.logger.InfoContext(ctx, "Processing activities POIs from unified response",
			slog.Int("content_length", activitiesContent.Len()))
		l.handleGeneralPoisFromResponse(ctx, activitiesContent.String(), cityID)
	}

	// Process hotel POIs if available (for DomainAccommodation)
	if hotelsContent, ok := responses["hotels"]; ok && hotelsContent.Len() > 0 {
		l.logger.InfoContext(ctx, "Processing hotels from unified response",
			slog.Int("content_length", hotelsContent.Len()))
		l.handleHotelsFromResponse(ctx, hotelsContent.String(), cityID, uuid.Nil, llmInteractionID)
	}

	// Process restaurant POIs if available (for DomainDining)
	if restaurantsContent, ok := responses["restaurants"]; ok && restaurantsContent.Len() > 0 {
		l.logger.InfoContext(ctx, "Processing restaurants from unified response",
			slog.Int("content_length", restaurantsContent.Len()))
		l.handleRestaurantsFromResponse(ctx, restaurantsContent.String(), cityID, uuid.Nil, llmInteractionID)
	}
}

func (l *ServiceImpl) handleGeneralPoisFromResponse(ctx context.Context, content string, cityID uuid.UUID) {
	var poiData struct {
		PointsOfInterest []types.POIDetailedInfo `json:"points_of_interest"`
	}
	if err := json.Unmarshal([]byte(utils.CleanJSONResponse(content)), &poiData); err != nil {
		l.logger.ErrorContext(ctx, "Failed to parse general POIs from unified response", slog.Any("error", err))
		return
	}

	l.HandleGeneralPOIs(ctx, poiData.PointsOfInterest, cityID)
}

func (l *ServiceImpl) handleItineraryFromResponse(
	ctx context.Context,
	content string,
	userID, profileID, cityID uuid.UUID,
	llmInteractionID uuid.UUID,
	userLocation *types.UserLocation,
) {
	var itineraryData struct {
		ItineraryName      string                  `json:"itinerary_name"`
		OverallDescription string                  `json:"overall_description"`
		PointsOfInterest   []types.POIDetailedInfo `json:"points_of_interest"`
	}
	if err := json.Unmarshal([]byte(utils.CleanJSONResponse(content)), &itineraryData); err != nil {
		l.logger.ErrorContext(ctx, "Failed to parse itinerary from unified response", slog.Any("error", err))
		return
	}

	// Save the itinerary and its POIs
	_, err := l.HandlePersonalisedPOIs(ctx, itineraryData.PointsOfInterest, cityID, userLocation, llmInteractionID, userID, profileID)
	if err != nil {
		l.logger.ErrorContext(ctx, "Failed to save personalised POIs from unified response", slog.Any("error", err))
	}
}

func (l *ServiceImpl) handleHotelsFromResponse(ctx context.Context, content string, cityID, _, llmInteractionID uuid.UUID) {
	var hotelData struct {
		Hotels []types.HotelDetailedInfo `json:"hotels"`
	}
	if err := json.Unmarshal([]byte(utils.CleanJSONResponse(content)), &hotelData); err != nil {
		l.logger.ErrorContext(ctx, "Failed to parse hotels from unified response", slog.Any("error", err))
		return
	}

	// Save hotels to database
	for _, hotel := range hotelData.Hotels {
		hotel.LlmInteractionID = llmInteractionID
		if _, err := l.poiRepo.SaveHotelDetails(ctx, hotel, cityID); err != nil {
			l.logger.WarnContext(ctx, "Failed to save hotel from unified response",
				slog.String("hotel_name", hotel.Name), slog.Any("error", err))
		}
	}
	l.logger.InfoContext(ctx, "Saved hotels from unified response",
		slog.Int("hotel_count", len(hotelData.Hotels)))
}

func (l *ServiceImpl) handleRestaurantsFromResponse(ctx context.Context, content string, cityID, _, llmInteractionID uuid.UUID) {
	var restaurantData struct {
		Restaurants []types.RestaurantDetailedInfo `json:"restaurants"`
	}
	if err := json.Unmarshal([]byte(utils.CleanJSONResponse(content)), &restaurantData); err != nil {
		l.logger.ErrorContext(ctx, "Failed to parse restaurants from unified response", slog.Any("error", err))
		return
	}

	// Save restaurants to database
	for _, restaurant := range restaurantData.Restaurants {
		restaurant.LlmInteractionID = llmInteractionID
		if _, err := l.poiRepo.SaveRestaurantDetails(ctx, restaurant, cityID); err != nil {
			l.logger.WarnContext(ctx, "Failed to save restaurant from unified response",
				slog.String("restaurant_name", restaurant.Name), slog.Any("error", err))
		}
	}
	l.logger.InfoContext(ctx, "Saved restaurants from unified response",
		slog.Int("restaurant_count", len(restaurantData.Restaurants)))
}
