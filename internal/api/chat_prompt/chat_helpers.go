package llmChat

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/FACorreiaa/go-poi-au-suggestions/internal/types"
	"github.com/google/uuid"
)

func generatePOICacheKey(city string, lat, lon, distance float64, userID uuid.UUID) string {
	return fmt.Sprintf("poi:%s:%f:%f:%f:%s", city, lat, lon, distance, userID.String())
}

func generateHotelCacheKey(city string, lat, lon float64, userID uuid.UUID) string {
	return fmt.Sprintf("hotel:%s:%.6f:%.6f:%s", city, lat, lon, userID.String())
}

func generateRestaurantCacheKey(city string, lat, lon float64, userID uuid.UUID) string {
	return fmt.Sprintf("restaurant:%s:%.6f:%.6f:%s", city, lat, lon, userID.String())
}

func cleanJSONResponse(response string) string {
	response = strings.TrimSpace(response)

	// Remove markdown code block markers
	if strings.HasPrefix(response, "```json") {
		response = strings.TrimPrefix(response, "```json")
	} else if strings.HasPrefix(response, "```") {
		response = strings.TrimPrefix(response, "```")
	}

	if strings.HasSuffix(response, "```") {
		response = strings.TrimSuffix(response, "```")
	}

	response = strings.TrimSpace(response)

	// Extract JSON from response that might contain explanatory text
	// Look for the first { and find the matching closing brace
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
	return strings.Title(strings.Join(filtered, " "))
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
	if err := json.Unmarshal([]byte(cleanJSONResponse(content)), &poiData); err != nil {
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
	if err := json.Unmarshal([]byte(cleanJSONResponse(content)), &itineraryData); err != nil {
		l.logger.ErrorContext(ctx, "Failed to parse itinerary from unified response", slog.Any("error", err))
		return
	}

	// Save the itinerary and its POIs
	_, err := l.HandlePersonalisedPOIs(ctx, itineraryData.PointsOfInterest, cityID, userLocation, llmInteractionID, userID, profileID)
	if err != nil {
		l.logger.ErrorContext(ctx, "Failed to save personalised POIs from unified response", slog.Any("error", err))
	}
}

func (l *ServiceImpl) handleHotelsFromResponse(ctx context.Context, content string, cityID, userID, llmInteractionID uuid.UUID) {
	var hotelData struct {
		Hotels []types.HotelDetailedInfo `json:"hotels"`
	}
	if err := json.Unmarshal([]byte(cleanJSONResponse(content)), &hotelData); err != nil {
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

func (l *ServiceImpl) handleRestaurantsFromResponse(ctx context.Context, content string, cityID, userID, llmInteractionID uuid.UUID) {
	var restaurantData struct {
		Restaurants []types.RestaurantDetailedInfo `json:"restaurants"`
	}
	if err := json.Unmarshal([]byte(cleanJSONResponse(content)), &restaurantData); err != nil {
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
