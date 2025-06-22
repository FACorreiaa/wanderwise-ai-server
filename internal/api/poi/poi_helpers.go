package poi

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
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

func generateFilteredPOICacheKeyWithFilters(lat, lon, distance float64, filters map[string]string, userID uuid.UUID) string {
	// Serialize filters to JSON for consistent cache key
	filtersJSON, _ := json.Marshal(filters)
	return fmt.Sprintf("poi_filtered:%f:%f:%f:%s:%s", lat, lon, distance, userID.String(), string(filtersJSON))
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

	// Extract JSON from response that might contain explanatory text
	// Look for the first { and last } to extract the JSON object
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

// applyClientFilters applies filtering logic to POI slice
// func applyClientFilters(pois []types.POIDetailedInfo, filters map[string]string) []types.POIDetailedInfo {
// 	if len(filters) == 0 {
// 		return pois
// 	}

// 	var filtered []types.POIDetailedInfo
// 	for _, poi := range pois {
// 		include := true

// 		// Category filter
// 		if category, exists := filters["category"]; exists && category != "" && category != "all" {
// 			if !strings.EqualFold(poi.Category, category) {
// 				include = false
// 			}
// 		}

// 		// Price range filter
// 		if priceRange, exists := filters["price_range"]; exists && priceRange != "" && priceRange != "all" {
// 			if poi.PriceRange == "" || !strings.EqualFold(poi.PriceRange, priceRange) {
// 				include = false
// 			}
// 		}

// 		// Min rating filter
// 		if minRatingStr, exists := filters["min_rating"]; exists && minRatingStr != "" && minRatingStr != "all" {
// 			if minRating := parseFloat(minRatingStr); minRating > 0 && poi.Rating < minRating {
// 				include = false
// 			}
// 		}

// 		if include {
// 			filtered = append(filtered, poi)
// 		}
// 	}
// 	return filtered
// }

// parseFloat safely parses a string to float64
func parseFloat(s string) float64 {
	if val, err := strconv.ParseFloat(s, 64); err == nil {
		return val
	}
	return 0
}
