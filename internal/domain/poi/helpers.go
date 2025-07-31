package poi

import (
	"fmt"
	"math"
	"strings"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (l *Service) enrichAndFilterLLMResponse(rawPOIs []POIDetailedInfo, userLat, userLon, searchRadius float64) []POIDetailedInfo {
	var processedPOIs []POIDetailedInfo
	for _, p := range rawPOIs {
		distanceKm := calculateDistance(userLat, userLon, p.Latitude, p.Longitude)

		if distanceKm <= searchRadius/1000 {
			poiID := p.ID
			if poiID == uuid.Nil {
				poiID = uuid.New()
			}
			detailedPOI := POIDetailedInfo{
				ID:               poiID,
				Name:             p.Name,
				Latitude:         p.Latitude,
				Longitude:        p.Longitude,
				Category:         p.Category,
				Description:      p.DescriptionPOI,
				Distance:         distanceKm * 1000,
				City:             p.City,
				CityID:           p.CityID,
				Address:          p.Address,
				PhoneNumber:      p.PhoneNumber,
				Website:          p.Website,
				OpeningHours:     p.OpeningHours,
				Rating:           p.Rating,
				PriceRange:       p.PriceRange,
				PriceLevel:       p.PriceLevel,
				Reviews:          p.Reviews,
				LlmInteractionID: p.LlmInteractionID,
				CreatedAt:        p.CreatedAt,
				Amenities:        p.Amenities,
				Source:           p.Source,
			}
			processedPOIs = append(processedPOIs, detailedPOI)
		}
	}
	return processedPOIs
}

// calculateDistance calculates the distance between two coordinates using the Haversine formula
// Returns distance in kilometers
func calculateDistance(lat1, lon1, lat2, lon2 float64) float64 {
	const R = 6371 // Earth's radius in kilometers

	// Convert degrees to radians
	lat1Rad := lat1 * math.Pi / 180
	lon1Rad := lon1 * math.Pi / 180
	lat2Rad := lat2 * math.Pi / 180
	lon2Rad := lon2 * math.Pi / 180

	// Differences
	dlat := lat2Rad - lat1Rad
	dlon := lon2Rad - lon1Rad

	// Haversine formula
	a := math.Sin(dlat/2)*math.Sin(dlat/2) +
		math.Cos(lat1Rad)*math.Cos(lat2Rad)*math.Sin(dlon/2)*math.Sin(dlon/2)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))

	// Distance in kilometers
	distance := R * c
	return distance
}

func generateFilteredPOICacheKey(lat, lon, distance float64, userID uuid.UUID) string {
	return fmt.Sprintf("poi_filtered:%f:%f:%f:%s", lat, lon, distance, userID.String())
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

	lastBrace := strings.LastIndex(response, "}")
	if lastBrace == -1 || lastBrace <= firstBrace {
		return response // No valid JSON structure found
	}

	// Extract the JSON portion
	jsonPortion := response[firstBrace : lastBrace+1]
	return strings.TrimSpace(jsonPortion)
}

// ValidateLocationAndRadius validates latitude, longitude, and radius parameters
func ValidateLocationAndRadius(latitude, longitude, radiusMeters float64) error {
	if latitude < -90 || latitude > 90 {
		return status.Errorf(codes.InvalidArgument, "invalid latitude: must be between -90 and 90")
	}
	if longitude < -180 || longitude > 180 {
		return status.Errorf(codes.InvalidArgument, "invalid longitude: must be between -180 and 180")
	}
	if radiusMeters <= 0 {
		return status.Errorf(codes.InvalidArgument, "radius must be positive")
	}
	return nil
}
