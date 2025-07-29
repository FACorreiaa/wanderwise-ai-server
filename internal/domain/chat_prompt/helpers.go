package chat_prompt

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/FACorreiaa/go-poi-au-suggestions/internal/domain/poi"
)

func generatePOICacheKey(city string, lat, lon, distance float64, userID uuid.UUID) string {
	return fmt.Sprintf("poi:%s:%f:%f:%f:%s", city, lat, lon, distance, userID.String())
}

func cleanJSONResponse(response string) string {
	response = strings.TrimSpace(response)

	// Remove markdown code block markers
	if strings.HasPrefix(response, "```json") {
		response = strings.TrimPrefix(response, "```json")
	} else if strings.HasPrefix(response, "```") {
		response = strings.TrimPrefix(response, "```")
	}

	response = strings.TrimSuffix(response, "```")

	response = strings.TrimSpace(response)

	firstBrace := strings.Index(response, "{")
	if firstBrace == -1 {
		return response // No JSON found, return as is
	}

	// Find the matching closing brace by counting braces
	braceCount := 0
	var lastValidBrace int
	for i := firstBrace; i < len(response); i++ {
		switch response[i] {
		case '{':
			braceCount++
		case '}':
			braceCount--
			if braceCount == 0 {
				lastValidBrace = i
				break
			}
		}
	}

	if braceCount != 0 {
		// Fallback to last brace method if brace counting fails
		lastBrace := strings.LastIndex(response, "}")
		if lastBrace == -1 || lastBrace <= firstBrace {
			return response // No valid JSON structure found
		}
		lastValidBrace = lastBrace
	}

	// Extract the JSON portion
	jsonPortion := response[firstBrace : lastValidBrace+1]

	// Remove any remaining backticks that might be within the JSON content
	// This handles cases where the AI includes markdown formatting within JSON strings
	jsonPortion = strings.ReplaceAll(jsonPortion, "`", "")

	return strings.TrimSpace(jsonPortion)
}

// helpers
// TODO: These methods need to be properly implemented with correct method calls and types
// They are commented out for now to avoid compilation errors

func (l *Service) ProcessAndSaveUnifiedResponse(
	ctx context.Context,
	responses map[string]*strings.Builder,
	userID, profileID, cityID uuid.UUID,
	llmInteractionID uuid.UUID,
	userLocation *UserLocation,
) {
	l.logger.Info("Processing unified response for POI extraction",
		zap.String("city_id", cityID.String()),
		zap.Int("response_parts", len(responses)))

	// Process general POIs if available
	if poisContent, ok := responses["general_pois"]; ok && poisContent.Len() > 0 {
		l.logger.Info("Processing general POIs from unified response",
			zap.Int("content_length", poisContent.Len()))
		l.handleGeneralPoisFromResponse(ctx, poisContent.String(), cityID)
	}

	// Process itinerary POIs if available
	if itineraryContent, ok := responses["itinerary"]; ok && itineraryContent.Len() > 0 {
		l.logger.Info("Processing itinerary POIs from unified response",
			zap.Int("content_length", itineraryContent.Len()))
		l.handleItineraryFromResponse(ctx, itineraryContent.String(), userID, profileID, cityID, llmInteractionID, userLocation)
	}

	// Process activities POIs if available (for DomainActivities)
	if activitiesContent, ok := responses["activities"]; ok && activitiesContent.Len() > 0 {
		l.logger.Info("Processing activities POIs from unified response",
			zap.Int("content_length", activitiesContent.Len()))
		l.handleGeneralPoisFromResponse(ctx, activitiesContent.String(), cityID)
	}

	// Process hotel POIs if available (for DomainAccommodation)
	if hotelsContent, ok := responses["hotels"]; ok && hotelsContent.Len() > 0 {
		l.logger.Info("Processing hotels from unified response",
			zap.Int("content_length", hotelsContent.Len()))
		l.handleHotelsFromResponse(ctx, hotelsContent.String(), cityID, userID, llmInteractionID)
	}

	// Process restaurant POIs if available (for DomainDining)
	if restaurantsContent, ok := responses["restaurants"]; ok && restaurantsContent.Len() > 0 {
		l.logger.Info("Processing restaurants from unified response",
			zap.Int("content_length", restaurantsContent.Len()))
		l.handleRestaurantsFromResponse(ctx, restaurantsContent.String(), cityID, userID, llmInteractionID)
	}
}

func (l *Service) ProcessAndSaveUnifiedResponseFree(
	ctx context.Context,
	responses map[string]*strings.Builder,
	cityID uuid.UUID,
	llmInteractionID uuid.UUID,
	userLocation *UserLocation,
) {
	l.logger.Info("Processing unified response for POI extraction",
		zap.String("city_id", cityID.String()),
		zap.Int("response_parts", len(responses)))

	// Process general POIs if available
	if poisContent, ok := responses["general_pois"]; ok && poisContent.Len() > 0 {
		l.logger.Info("Processing general POIs from unified response",
			zap.Int("content_length", poisContent.Len()))
		l.handleGeneralPoisFromResponse(ctx, poisContent.String(), cityID)
	}

	// Process itinerary POIs if available
	if itineraryContent, ok := responses["itinerary"]; ok && itineraryContent.Len() > 0 {
		l.logger.Info("Processing itinerary POIs from unified response",
			zap.Int("content_length", itineraryContent.Len()))
		l.handleItineraryFromResponse(ctx, itineraryContent.String(), uuid.Nil, uuid.Nil, cityID, llmInteractionID, userLocation)
	}

	// Process activities POIs if available (for DomainActivities)
	if activitiesContent, ok := responses["activities"]; ok && activitiesContent.Len() > 0 {
		l.logger.Info("Processing activities POIs from unified response",
			zap.Int("content_length", activitiesContent.Len()))
		l.handleGeneralPoisFromResponse(ctx, activitiesContent.String(), cityID)
	}

	// Process hotel POIs if available (for DomainAccommodation)
	if hotelsContent, ok := responses["hotels"]; ok && hotelsContent.Len() > 0 {
		l.logger.Info("Processing hotels from unified response",
			zap.Int("content_length", hotelsContent.Len()))
		l.handleHotelsFromResponse(ctx, hotelsContent.String(), cityID, uuid.Nil, llmInteractionID)
	}

	// Process restaurant POIs if available (for DomainDining)
	if restaurantsContent, ok := responses["restaurants"]; ok && restaurantsContent.Len() > 0 {
		l.logger.Info("Processing restaurants from unified response",
			zap.Int("content_length", restaurantsContent.Len()))
		l.handleRestaurantsFromResponse(ctx, restaurantsContent.String(), cityID, uuid.Nil, llmInteractionID)
	}
}

func (l *Service) handleGeneralPoisFromResponse(ctx context.Context, content string, cityID uuid.UUID) {
	var poiData struct {
		PointsOfInterest []POIDetailedInfo `json:"points_of_interest"`
	}
	if err := json.Unmarshal([]byte(cleanJSONResponse(content)), &poiData); err != nil {
		l.logger.Error("Failed to parse general POIs from unified response", zap.Error(err))
		return
	}

	l.HandleGeneralPOIs(ctx, poiData.PointsOfInterest, cityID)
}

func (l *Service) handleItineraryFromResponse(
	ctx context.Context,
	content string,
	userID, profileID, cityID uuid.UUID,
	llmInteractionID uuid.UUID,
	userLocation *UserLocation,
) {
	var itineraryData struct {
		ItineraryName      string            `json:"itinerary_name"`
		OverallDescription string            `json:"overall_description"`
		PointsOfInterest   []POIDetailedInfo `json:"points_of_interest"`
	}
	if err := json.Unmarshal([]byte(cleanJSONResponse(content)), &itineraryData); err != nil {
		l.logger.Error("Failed to parse itinerary from unified response", zap.Error(err))
		return
	}

	// Save the itinerary and its POIs
	_, err := l.HandlePersonalisedPOIs(ctx, itineraryData.PointsOfInterest, cityID, userLocation, llmInteractionID, userID, profileID)
	if err != nil {
		l.logger.Error("Failed to save personalised POIs from unified response", zap.Error(err))
	}
}

func (l *Service) handleHotelsFromResponse(ctx context.Context, content string, cityID, _, llmInteractionID uuid.UUID) {
	var hotelData struct {
		Hotels []poi.HotelDetailedInfo `json:"hotels"`
	}
	if err := json.Unmarshal([]byte(cleanJSONResponse(content)), &hotelData); err != nil {
		l.logger.Error("Failed to parse hotels from unified response", zap.Error(err))
		return
	}

	// Save hotels to database
	for _, hotel := range hotelData.Hotels {
		hotel.LlmInteractionID = llmInteractionID
		if _, err := l.poiRepo.SaveHotelDetails(ctx, hotel, cityID); err != nil {
			l.logger.Warn("Failed to save hotel from unified response",
				zap.String("hotel_name", hotel.Name), zap.Error(err))
		}
	}
	l.logger.Info("Saved hotels from unified response",
		zap.Int("hotel_count", len(hotelData.Hotels)))
}

func (l *Service) handleRestaurantsFromResponse(ctx context.Context, content string, cityID, _, llmInteractionID uuid.UUID) {
	var restaurantData struct {
		Restaurants []poi.RestaurantDetailedInfo `json:"restaurants"`
	}
	if err := json.Unmarshal([]byte(cleanJSONResponse(content)), &restaurantData); err != nil {
		l.logger.Error("Failed to parse restaurants from unified response", zap.Error(err))
		return
	}

	// Save restaurants to database
	for _, restaurant := range restaurantData.Restaurants {
		restaurant.LlmInteractionID = llmInteractionID
		if _, err := l.poiRepo.SaveRestaurantDetails(ctx, restaurant, cityID); err != nil {
			l.logger.Warn("Failed to save restaurant from unified response",
				zap.String("restaurant_name", restaurant.Name), zap.Error(err))
		}
	}
	l.logger.Info("Saved restaurants from unified response",
		zap.Int("restaurant_count", len(restaurantData.Restaurants)))
}
