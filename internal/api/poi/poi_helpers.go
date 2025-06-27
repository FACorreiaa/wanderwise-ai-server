package poi

import (
	"fmt"
	"math"
	"strings"

	"github.com/google/uuid"
)

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

	if strings.HasSuffix(response, "```") {
		response = strings.TrimSuffix(response, "```")
	}

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
