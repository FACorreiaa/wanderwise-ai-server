package chat_prompt

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"go.uber.org/zap"
	"google.golang.org/genai"

	"github.com/FACorreiaa/go-poi-au-suggestions/internal/types"
)

func (l *Service) extractCityFromMessage(ctx context.Context, message string) (cityName, cleanedMessage string, err error) {
	prompt := fmt.Sprintf(`
You are a text parser. Extract the city name from the user's travel request and return a clean version of the message.

User message: "%s"

Respond with ONLY a JSON object in this exact format:
{
    "city": "City Name",
    "message": "cleaned message without city"
}

Examples:
- "Find restaurants in Barcelona" → {"city": "Barcelona", "message": "Find restaurants"}
- "What to do in Paris?" → {"city": "Paris", "message": "What to do"}
- "Barcelona restaurants" → {"city": "Barcelona", "message": "restaurants"}
- "Show me hotels in New York" → {"city": "New York", "message": "Show me hotels"}
- "Things to do Madrid" → {"city": "Madrid", "message": "Things to do"}

If no city is mentioned, use empty string for city.
`, message)

	response, err := l.aiClient.GenerateResponse(ctx, prompt, &genai.GenerateContentConfig{
		Temperature: genai.Ptr[float32](0.1),
	})
	if err != nil {
		return "", "", fmt.Errorf("failed to parse message: %w", err)
	}

	var responseText string
	for _, cand := range response.Candidates {
		if cand.Content != nil {
			for _, part := range cand.Content.Parts {
				if part.Text != "" {
					responseText += string(part.Text)
				}
			}
		}
	}

	if responseText == "" {
		return "", "", fmt.Errorf("empty response from AI parser")
	}

	cleanResponse := cleanJSONResponse(responseText)
	var parsed struct {
		City    string `json:"city"`
		Message string `json:"message"`
	}

	if err := json.Unmarshal([]byte(cleanResponse), &parsed); err != nil {
		return "", "", fmt.Errorf("failed to parse extraction response: %w", err)
	}

	if parsed.City == "" {
		return "", message, nil
	}

	return parsed.City, parsed.Message, nil
}

func (l *Service) handleSemanticRemovePOI(ctx context.Context, message string, session *ChatSession) string {
	ctx, span := otel.Tracer("LlmInteractionService").Start(ctx, "handleSemanticRemovePOI")
	defer span.End()

	poiName := extractPOIName(message)
	if poiName == "" {
		return "I'd be happy to remove a POI from your itinerary! Could you please specify which place you'd like to remove?"
	}

	for i, p := range session.CurrentItinerary.AIItineraryResponse.PointsOfInterest {
		if strings.EqualFold(p.Name, poiName) ||
			strings.Contains(strings.ToLower(p.Name), strings.ToLower(poiName)) ||
			strings.Contains(strings.ToLower(poiName), strings.ToLower(p.Name)) {

			removedName := p.Name
			session.CurrentItinerary.AIItineraryResponse.PointsOfInterest = append(
				session.CurrentItinerary.AIItineraryResponse.PointsOfInterest[:i],
				session.CurrentItinerary.AIItineraryResponse.PointsOfInterest[i+1:]...,
			)
			l.logger.Info("Removed POI from itinerary",
				zap.String("removed_poi", removedName))
			span.SetAttributes(attribute.String("removed_poi", removedName))
			return fmt.Sprintf("I've removed %s from your itinerary.", removedName)
		}
	}

	return fmt.Sprintf("I couldn't find %s in your itinerary. Here's what you currently have: %s",
		poiName, strings.Join(func() []string {
			var names []string
			for _, p := range session.CurrentItinerary.AIItineraryResponse.PointsOfInterest {
				names = append(names, p.Name)
			}
			return names
		}(), ", "))
}

func (l *Service) ensureItineraryExists(session *ChatSession) {
	if session.CurrentItinerary == nil {
		session.CurrentItinerary = &AiCityResponse{
			AIItineraryResponse: AIItineraryResponse{
				ItineraryName:      fmt.Sprintf("Trip to %s", session.SessionContext.CityName),
				OverallDescription: fmt.Sprintf("Exploring %s", session.SessionContext.CityName),
				PointsOfInterest:   []POIDetailedInfo{},
			},
		}
	}
	if session.CurrentItinerary.AIItineraryResponse.PointsOfInterest == nil {
		session.CurrentItinerary.AIItineraryResponse.PointsOfInterest = []POIDetailedInfo{}
	}
}

func extractPOIName(message string) string {
	message = strings.ToLower(message)

	patterns := []string{
		`add\s+(.+?)(?:\s+to\s+(?:my\s+)?itinerary)?$`,
		`include\s+(.+?)(?:\s+in\s+(?:my\s+)?itinerary)?$`,
		`visit\s+(.+)$`,
		`go\s+to\s+(.+)$`,
		`see\s+(.+)$`,
		`^(.+?)(?:\s+please)?$`,
	}

	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		if matches := re.FindStringSubmatch(message); len(matches) > 1 {
			return strings.TrimSpace(matches[1])
		}
	}

	return ""
}
