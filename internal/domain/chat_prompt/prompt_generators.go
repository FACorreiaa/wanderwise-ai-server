package chat_prompt

import (
	"fmt"
	"strings"

	"github.com/FACorreiaa/go-poi-au-suggestions/internal/domain/poi"
	"github.com/FACorreiaa/go-poi-au-suggestions/internal/domain/profiles"
)

func (l *Service) getPersonalizedPOIWithSemanticContext(interestNames []string, cityName, tagsPromptPart, userPrefs string, semanticPOIs []poi.POIDetailedInfo) string {
	prompt := fmt.Sprintf(`
        Generate a personalized trip itinerary for %s, tailored to user interests [%s].

        **SEMANTIC CONTEXT - Consider these highly relevant POIs found via semantic search:**
        `, cityName, strings.Join(interestNames, ", "))

	if len(semanticPOIs) > 0 {
		prompt += "\n**Contextually Relevant POIs:**\n"
		for i, p := range semanticPOIs {
			if i >= 10 {
				break
			}
			prompt += fmt.Sprintf("- %s (%s): %s [Lat: %.6f, Lon: %.6f]\n",
				p.Name, p.Category, p.DescriptionPOI, p.Latitude, p.Longitude)
		}
		prompt += "\n**Instructions:** Use these semantic matches as inspiration and context. You may include them directly or use them to find similar places. Ensure variety and avoid exact duplicates.\n\n"
	}

	prompt += `Include:
        1. An itinerary name that reflects both user interests and semantic context.
        2. An overall description highlighting semantic relevance.
        3. A list of points of interest with name, category, coordinates, and detailed description.
        Max points of interest allowed by tokens.

        **PRIORITIZATION:**
        - Highly weight POIs that align with the semantic context provided
        - Ensure semantic relevance in descriptions
        - Balance popular attractions with personalized semantic matches
        - Include variety across different categories while maintaining semantic coherence

        Format the response in JSON with the following structure:
        {
            "itinerary_name": "Name of the itinerary (reflecting semantic context)",
            "overall_description": "Description emphasizing semantic relevance to user interests",
            "points_of_interest": [
                {
                    "name": "POI name",
                    "latitude": latitude_as_number,
                    "longitude": longitude_as_number,
                    "category": "Category",
                    "description_poi": "Detailed description explaining semantic relevance to user interests and why this matches their preferences"
                }
            ]
        }`

	if tagsPromptPart != "" {
		prompt += "\n**User Tags Context:** " + tagsPromptPart
	}
	if userPrefs != "" {
		prompt += "\n**User Preferences:** " + userPrefs
	}

	return prompt
}

func (l *Service) getEnhancedPersonalizedPOIPrompt(cityName, enhancedPromptData string, domain profiles.DomainType) string {
	domainFocus := ""
	switch domain {
	case profiles.DomainAccommodation:
		domainFocus = "Focus particularly on accommodation recommendations and nearby attractions that complement the user's accommodation preferences."
	case profiles.DomainDining:
		domainFocus = "Focus particularly on restaurant, food, and dining experiences that align with the user's culinary preferences."
	case profiles.DomainActivities:
		domainFocus = "Focus particularly on activities, attractions, and experiences that match the user's activity preferences and physical capabilities."
	case profiles.DomainItinerary:
		domainFocus = "Focus particularly on creating a well-structured itinerary that respects the user's planning style and pace preferences."
	default:
		domainFocus = "Provide a balanced mix of attractions, dining, and activities based on all user preferences."
	}

	prompt := fmt.Sprintf(`You are a travel AI assistant creating a personalized itinerary for %s.

User Preferences and Filters:
%s

Domain Focus: %s

%s

Create a comprehensive and personalized itinerary that heavily weighs the user's specific preferences and filters. Ensure that every recommendation aligns with their stated preferences.

Format the response in JSON with the following structure:
{
    "itinerary_name": "Personalized itinerary name reflecting user preferences",
    "overall_description": "Description emphasizing how this itinerary matches user preferences",
    "points_of_interest": [
        {
            "name": "POI name",
            "category": "Category",
            "coordinates": {
                "latitude": float64,
                "longitude": float64
            },
            "description": "Detailed description explaining why this POI matches the user's specific preferences and filters"
        }
    ]
}`, cityName, enhancedPromptData, domainFocus, getBasePersonalizedPromptInstructions())

	return prompt
}

func getBasePersonalizedPromptInstructions() string {
	return `
**Base Instructions:**
- Prioritize user preferences above popular tourist attractions
- Ensure all recommendations align with stated preferences
- Provide diverse categories while respecting user filters
- Include practical information relevant to user needs
- Maintain authenticity and local flavor where appropriate`
}

func getPersonalizedPOI(interestNames []string, cityName, tagsPromptPart, userPrefs string) string {
	prompt := fmt.Sprintf(`
        Generate a personalized trip itinerary for %s, tailored to user interests [%s].

        Include:
        1. An itinerary name
        2. An overall description
        3. A list of points of interest with name, category, coordinates, and detailed description.
        Max points of interest allowed by tokens allowed by tokens that is relevant to user interests.

        Format the response in JSON with the following structure:
        {
            "itinerary_name": "Name of the itinerary",
            "overall_description": "Overall description",
            "points_of_interest": [
                {
                    "name": "POI name",
                    "latitude": latitude_as_number,
                    "longitude": longitude_as_number,
                    "category": "Category",
                    "description_poi": "Detailed description"
                }
            ]
        }`, cityName, strings.Join(interestNames, ", "))

	if tagsPromptPart != "" {
		prompt += "\n**User Tags Context:** " + tagsPromptPart
	}
	if userPrefs != "" {
		prompt += "\n**User Preferences:** " + userPrefs
	}

	return prompt
}
