package chat_prompt

import (
	"context"
	"crypto/md5"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"log/slog"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	generativeAI "github.com/FACorreiaa/go-genai-sdk/lib"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/patrickmn/go-cache"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/genai"

	"github.com/FACorreiaa/go-poi-au-suggestions/internal/types"
)

// getPersonalizedPOIWithSemanticContext creates an enhanced prompt with semantic POI context
func (l *ServiceImpl) getPersonalizedPOIWithSemanticContext(interestNames []string, cityName, tagsPromptPart, userPrefs string, semanticPOIs []types.POIDetailedInfo) string {
	prompt := fmt.Sprintf(`
        Generate a personalized trip itinerary for %s, tailored to user interests [%s].

        **SEMANTIC CONTEXT - Consider these highly relevant POIs found via semantic search:**
        `, cityName, strings.Join(interestNames, ", "))

	// Add semantic POI context
	if len(semanticPOIs) > 0 {
		prompt += "\n**Contextually Relevant POIs:**\n"
		for i, p := range semanticPOIs {
			if i >= 10 { // Limit context to avoid token overuse
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

func (l *ServiceImpl) FetchUserData(ctx context.Context, userID, profileID uuid.UUID) (interests []*types.Interest, searchProfile *types.UserPreferenceProfileResponse, tags []*types.Tags, err error) {
	interests, err = l.interestRepo.GetInterestsForProfile(ctx, profileID)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to fetch user interests: %w", err)
	}
	searchProfile, err = l.searchProfileRepo.GetSearchProfile(ctx, userID, profileID)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to fetch search profile: %w", err)
	}
	tags, err = l.tagsRepo.GetTagsForProfile(ctx, profileID)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to fetch user tags: %w", err)
	}
	return interests, searchProfile, tags, nil
}

func (l *ServiceImpl) PreparePromptData(interests []*types.Interest, tags []*types.Tags, searchProfile *types.UserPreferenceProfileResponse) (interestNames []string, tagsPromptPart string, userPrefs string) {
	if len(interests) == 0 {
		interestNames = []string{"general sightseeing", "local experiences"}
	} else {
		for _, interest := range interests {
			if interest != nil {
				interestNames = append(interestNames, interest.Name)
			}
		}
	}
	var tagInfoForPrompt []string
	for _, tag := range tags {
		if tag != nil {
			tagDetail := tag.Name
			if tag.Description != nil && *tag.Description != "" {
				tagDetail += fmt.Sprintf(" (meaning: %s)", *tag.Description)
			}
			tagInfoForPrompt = append(tagInfoForPrompt, tagDetail)
		}
	}
	if len(tagInfoForPrompt) > 0 {
		tagsPromptPart = fmt.Sprintf("\n    - Additionally, consider these specific user tags/preferences: [%s].", strings.Join(tagInfoForPrompt, "; "))
	}
	userPrefs = getUserPreferencesPrompt(searchProfile)
	return interestNames, tagsPromptPart, userPrefs
}

func (l *ServiceImpl) CollectResults(resultCh <-chan types.GenAIResponse) (itinerary types.AiCityResponse, llmInteractionID uuid.UUID, rawPersonalisedPOIs []types.POIDetailedInfo, errors []error) {
	for res := range resultCh {
		if res.Err != nil {
			errors = append(errors, res.Err)
			continue
		}
		if res.City != "" {
			itinerary.GeneralCityData.City = res.City
			itinerary.GeneralCityData.Country = res.Country
			itinerary.GeneralCityData.Description = res.CityDescription
			itinerary.GeneralCityData.StateProvince = res.StateProvince
			itinerary.GeneralCityData.CenterLatitude = res.Latitude
			itinerary.GeneralCityData.CenterLongitude = res.Longitude
		}
		if res.ItineraryName != "" {
			itinerary.AIItineraryResponse.ItineraryName = res.ItineraryName
			itinerary.AIItineraryResponse.OverallDescription = res.ItineraryDescription
		}
		if len(res.GeneralPOI) > 0 {
			itinerary.PointsOfInterest = res.GeneralPOI
		}
		if len(res.PersonalisedPOI) > 0 {
			itinerary.AIItineraryResponse.PointsOfInterest = res.PersonalisedPOI
			rawPersonalisedPOIs = res.PersonalisedPOI
			llmInteractionID = res.LlmInteractionID
		}
	}
	return itinerary, llmInteractionID, rawPersonalisedPOIs, errors
}

func (l *ServiceImpl) HandleCityData(ctx context.Context, cityData types.GeneralCityData) (cityID uuid.UUID, err error) {
	c, err := l.cityRepo.FindCityByNameAndCountry(ctx, cityData.City, cityData.Country)
	if err != nil {
		return uuid.Nil, fmt.Errorf("failed to check city existence: %w", err)
	}
	if c == nil {
		cityDetail := types.CityDetail{
			Name:            cityData.City,
			Country:         cityData.Country,
			StateProvince:   cityData.StateProvince,
			AiSummary:       cityData.Description,
			CenterLatitude:  cityData.CenterLatitude,
			CenterLongitude: cityData.CenterLongitude,
		}
		cityID, err = l.cityRepo.SaveCity(ctx, cityDetail)
		if err != nil {
			return uuid.Nil, fmt.Errorf("failed to save city: %w", err)
		}
	} else {
		cityID = c.ID
	}
	return cityID, nil
}

func (l *ServiceImpl) HandleGeneralPOIs(ctx context.Context, pois []types.POIDetailedInfo, cityID uuid.UUID) {
	for _, p := range pois {
		existingPoi, err := l.poiRepo.FindPoiByNameAndCity(ctx, p.Name, cityID)
		if err != nil {
			l.logger.WarnContext(ctx, "Failed to check POI existence", slog.String("poi_name", p.Name), slog.Any("error", err))
			continue
		}
		if existingPoi == nil {
			_, err = l.poiRepo.SavePoi(ctx, p, cityID)
			if err != nil {
				l.logger.WarnContext(ctx, "Failed to save POI", slog.String("poi_name", p.Name), slog.Any("error", err))
			}
		}
	}
}

func (l *ServiceImpl) HandlePersonalisedPOIs(ctx context.Context, pois []types.POIDetailedInfo, cityID uuid.UUID, userLocation *types.UserLocation, llmInteractionID uuid.UUID, userID, profileID uuid.UUID) ([]types.POIDetailedInfo, error) {
	if userLocation == nil || len(pois) == 0 {
		return pois, nil // No sorting possible
	}

	// Check if cityID is valid, if not, skip itinerary creation to avoid foreign key constraint errors
	if cityID == uuid.Nil || cityID.String() == "00000000-0000-0000-0000-000000000000" {
		l.logger.WarnContext(ctx, "Skipping itinerary creation due to invalid cityID",
			slog.String("cityID", cityID.String()))
		return pois, nil // Return POIs without sorting/saving to avoid database errors
	}

	err := l.llmInteractionRepo.SaveLlmSuggestedPOIsBatch(ctx, pois, userID, profileID, llmInteractionID, cityID)
	if err != nil {
		return nil, fmt.Errorf("failed to save personalised POIs: %w", err)
	}

	itineraryID, err := l.poiRepo.SaveItinerary(ctx, userID, cityID)
	if err != nil {
		l.logger.ErrorContext(ctx, "Failed to save itinerary, skipping itinerary creation",
			slog.Any("error", err),
			slog.String("cityID", cityID.String()),
			slog.String("userID", userID.String()))
		// Don't return error, just skip itinerary creation and continue with POI processing
		return pois, nil
	}

	if err := l.poiRepo.SaveItineraryPOIs(ctx, itineraryID, pois); err != nil {
		return nil, fmt.Errorf("failed to save itinerary POIs: %w", err)
	}

	sortedPois, err := l.llmInteractionRepo.GetLlmSuggestedPOIsByInteractionSortedByDistance(ctx, llmInteractionID, cityID, *userLocation)
	if err != nil {
		l.logger.ErrorContext(ctx, "Failed to fetch sorted POIs", slog.Any("error", err))
		return pois, nil // Return unsorted POIs
	}
	return sortedPois, nil
}

// GenerateEnhancedPersonalisedPOIWorker generates personalized POIs with domain-aware filtering
func (l *ServiceImpl) GenerateEnhancedPersonalisedPOIWorker(wg *sync.WaitGroup, ctx context.Context,
	cityName string, userID, profileID uuid.UUID, resultCh chan<- types.GenAIResponse,
	enhancedPromptData string, domain types.DomainType,
	config *genai.GenerateContentConfig) {
	ctx, span := otel.Tracer("LlmInteractionService").Start(ctx, "GenerateEnhancedPersonalisedPOIWorker", trace.WithAttributes(
		attribute.String("city.name", cityName),
		attribute.String("user.id", userID.String()),
		attribute.String("profile.id", profileID.String()),
		attribute.String("domain", string(domain)),
	))
	defer span.End()
	defer wg.Done()

	startTime := time.Now()

	// Create enhanced prompt based on domain
	prompt := l.getEnhancedPersonalizedPOIPrompt(cityName, enhancedPromptData, domain)
	span.SetAttributes(attribute.Int("prompt.length", len(prompt)))

	response, err := l.aiClient.GenerateResponse(ctx, prompt, config)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "AI generation failed")
		resultCh <- types.GenAIResponse{Err: fmt.Errorf("failed to generate enhanced personalized POIs: %w", err)}
		return
	}

	duration := time.Since(startTime)
	span.SetAttributes(attribute.Int64("generation.duration_ms", duration.Milliseconds()))

	var txt string
	for _, candidate := range response.Candidates {
		if candidate.Content != nil && len(candidate.Content.Parts) > 0 {
			txt = candidate.Content.Parts[0].Text
			break
		}
	}
	if txt == "" {
		err := fmt.Errorf("no valid enhanced personalized POI content from AI")
		span.RecordError(err)
		span.SetStatus(codes.Error, "Empty response from AI")
		resultCh <- types.GenAIResponse{Err: err}
		return
	}
	span.SetAttributes(attribute.Int("response.length", len(txt)))

	cleanTxt := cleanJSONResponse(txt)
	var itineraryData types.AIItineraryResponse
	if err := json.Unmarshal([]byte(cleanTxt), &itineraryData); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to parse enhanced personalized POI JSON")
		resultCh <- types.GenAIResponse{Err: fmt.Errorf("failed to parse enhanced personalized POI JSON: %w", err)}
		return
	}

	span.SetAttributes(attribute.Int("pois.count", len(itineraryData.PointsOfInterest)))
	span.SetStatus(codes.Ok, "Enhanced personalized POIs generated successfully")
	resultCh <- types.GenAIResponse{
		ItineraryName:        itineraryData.ItineraryName,
		ItineraryDescription: itineraryData.OverallDescription,
		PersonalisedPOI:      itineraryData.PointsOfInterest,
		LlmInteractionID:     uuid.New(), // Generate a new LLM interaction ID
	}
}

// getEnhancedPersonalizedPOIPrompt creates a domain-aware prompt for personalized POI generation
func (l *ServiceImpl) getEnhancedPersonalizedPOIPrompt(cityName, enhancedPromptData string, domain types.DomainType) string {
	domainFocus := ""
	switch domain {
	case types.DomainAccommodation:
		domainFocus = "Focus particularly on accommodation recommendations and nearby attractions that complement the user's accommodation preferences."
	case types.DomainDining:
		domainFocus = "Focus particularly on restaurant, food, and dining experiences that align with the user's culinary preferences."
	case types.DomainActivities:
		domainFocus = "Focus particularly on activities, attractions, and experiences that match the user's activity preferences and physical capabilities."
	case types.DomainItinerary:
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
**Instructions:**
- Prioritize POIs that directly align with user preferences and filters
- Explain in descriptions how each POI matches their specific preferences
- Ensure variety while maintaining preference alignment
- Include practical details like accessibility if relevant to user preferences
- Consider user's pace and planning style preferences in the selection
- Maximum 8-10 POIs to maintain quality over quantity`
}

func (l *ServiceImpl) SaveItenerary(ctx context.Context, userID uuid.UUID, req types.BookmarkRequest) (uuid.UUID, error) {
	var llmInteractionIDStr string
	if req.LlmInteractionID != nil {
		llmInteractionIDStr = req.LlmInteractionID.String()
	} else {
		llmInteractionIDStr = "nil"
	}

	ctx, span := otel.Tracer("LlmInteractionService").Start(ctx, "SaveItenerary", trace.WithAttributes(
		attribute.String("user.id", userID.String()),
		attribute.String("llm_interaction.id", llmInteractionIDStr),
		attribute.String("title", req.Title),
	))
	defer span.End()

	l.logger.InfoContext(ctx, "Attempting to bookmark interaction",
		slog.String("userID", userID.String()),
		slog.String("llmInteractionID", llmInteractionIDStr),
		slog.String("title", req.Title))

	var sourceInteractionID pgtype.UUID
	if req.LlmInteractionID != nil {
		// Use specific interaction ID if provided
		sourceInteractionID = pgtype.UUID{
			Bytes: *req.LlmInteractionID,
			Valid: true,
		}
		l.logger.InfoContext(ctx, "Using provided LlmInteractionID for bookmark",
			slog.String("llmInteractionID", req.LlmInteractionID.String()))
	} else if req.SessionID != nil {
		// If SessionID is provided, try to find the latest LLM interaction in that session
		// But always store the session ID for tracking purposes
		latestInteraction, err := l.llmInteractionRepo.GetLatestInteractionBySessionID(ctx, *req.SessionID)
		if err != nil || latestInteraction == nil {
			l.logger.InfoContext(ctx, "No interaction found for session, storing session ID without interaction reference",
				slog.String("sessionID", req.SessionID.String()),
				slog.Any("findError", err))
			sourceInteractionID = pgtype.UUID{Valid: false} // Set to NULL for interaction reference
		} else {
			sourceInteractionID = pgtype.UUID{
				Bytes: latestInteraction.ID,
				Valid: true,
			}
			l.logger.InfoContext(ctx, "Found latest interaction for session",
				slog.String("sessionID", req.SessionID.String()),
				slog.String("interactionID", latestInteraction.ID.String()))
		}
	} else {
		sourceInteractionID = pgtype.UUID{Valid: false} // Explicitly invalid for NULL
		l.logger.InfoContext(ctx, "No LlmInteractionID or SessionID provided, bookmark will have no source reference")
	}

	// Prepare primaryCityID - handle both PrimaryCityID and PrimaryCityName
	var primaryCityID pgtype.UUID

	// Handle city resolution
	if req.PrimaryCityID != nil {
		// Use provided city ID
		primaryCityID = pgtype.UUID{
			Bytes: *req.PrimaryCityID,
			Valid: true,
		}
	} else if req.PrimaryCityName != "" {
		// Look up or create city by name
		city, err := l.cityRepo.FindCityByNameAndCountry(ctx, req.PrimaryCityName, "")
		if err != nil {
			l.logger.ErrorContext(ctx, "Failed to find city", slog.Any("error", err))
			span.RecordError(err)
			return uuid.Nil, fmt.Errorf("failed to find city: %w", err)
		}

		if city == nil {
			// City doesn't exist, create it
			cityDetail := types.CityDetail{
				Name:      req.PrimaryCityName,
				Country:   "Unknown", // Could be extracted from LLM interaction context
				AiSummary: "",
			}
			cityID, err := l.cityRepo.SaveCity(ctx, cityDetail)
			if err != nil {
				l.logger.ErrorContext(ctx, "Failed to save city", slog.Any("error", err))
				span.RecordError(err)
				return uuid.Nil, fmt.Errorf("failed to save city: %w", err)
			}
			primaryCityID = pgtype.UUID{
				Bytes: cityID,
				Valid: true,
			}
			l.logger.InfoContext(ctx, "Created new city", slog.String("cityName", req.PrimaryCityName), slog.String("cityID", cityID.String()))
		} else {
			primaryCityID = pgtype.UUID{
				Bytes: city.ID,
				Valid: true,
			}
			l.logger.InfoContext(ctx, "Found existing city", slog.String("cityName", req.PrimaryCityName), slog.String("cityID", city.ID.String()))
		}
	} else {
		primaryCityID = pgtype.UUID{Valid: false}
	}

	// Fetch original interaction only if LlmInteractionID is provided
	var originalInteraction *types.LlmInteraction
	var err error
	if req.LlmInteractionID != nil {
		originalInteraction, err = l.llmInteractionRepo.GetInteractionByID(ctx, *req.LlmInteractionID)
		if err != nil || originalInteraction == nil {
			l.logger.ErrorContext(ctx, "Failed to fetch original LLM interaction", slog.Any("error", err))
			span.RecordError(err)
			return uuid.Nil, fmt.Errorf("could not retrieve original interaction: %w", err)
		}
	}

	// Prepare and save to user_saved_itineraries
	var markdownContent string
	if originalInteraction != nil {
		markdownContent = originalInteraction.ResponseText
	} else {
		if req.Description != nil {
			markdownContent = *req.Description
		} else {
			markdownContent = ""
		}
	}

	var description sql.NullString
	if req.Description != nil {
		description.String = *req.Description
		description.Valid = true
	}
	isPublic := false
	if req.IsPublic != nil {
		isPublic = *req.IsPublic
	}

	// Prepare session ID if provided
	var sessionID pgtype.UUID
	if req.SessionID != nil {
		sessionID = pgtype.UUID{
			Bytes: *req.SessionID,
			Valid: true,
		}
	} else {
		sessionID = pgtype.UUID{Valid: false}
	}

	newBookmark := &types.UserSavedItinerary{
		UserID:                 userID,
		SourceLlmInteractionID: sourceInteractionID, // Will be nil if not provided
		SessionID:              sessionID,           // Store the session ID separately
		PrimaryCityID:          primaryCityID,
		Title:                  req.Title,
		Description:            description,
		MarkdownContent:        markdownContent,
		Tags:                   req.Tags,
		IsPublic:               isPublic,
	}
	savedID, err := l.llmInteractionRepo.AddChatToBookmark(ctx, newBookmark)
	if err != nil {
		span.RecordError(err)
		return uuid.Nil, err
	}

	// Note: We skip saving to the old 'itineraries' table for bookmarks
	// because it has a unique constraint on (user_id, city_id) that prevents
	// multiple itineraries per city. Bookmarks should only use user_saved_itineraries.

	l.logger.InfoContext(ctx, "Successfully saved bookmark to user_saved_itineraries",
		slog.String("savedID", savedID.String()),
		slog.String("title", req.Title))

	span.SetAttributes(attribute.String("saved_itinerary.id", savedID.String()))
	span.SetStatus(codes.Ok, "Bookmark saved successfully")
	return savedID, nil
}

func (l *ServiceImpl) GetBookmarkedItineraries(ctx context.Context, userID uuid.UUID, page, limit int) (*types.PaginatedUserItinerariesResponse, error) {
	ctx, span := otel.Tracer("LlmInteractionService").Start(ctx, "GetBookmarkedItineraries", trace.WithAttributes(
		attribute.String("user.id", userID.String()),
		attribute.Int("page", page),
		attribute.Int("limit", limit),
	))
	defer span.End()

	l.logger.InfoContext(ctx, "Retrieving bookmarked itineraries",
		slog.String("userID", userID.String()),
		slog.Int("page", page),
		slog.Int("limit", limit))

	response, err := l.llmInteractionRepo.GetBookmarkedItineraries(ctx, userID, page, limit)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to retrieve bookmarked itineraries")
		return nil, fmt.Errorf("failed to retrieve bookmarked itineraries: %w", err)
	}

	l.logger.InfoContext(ctx, "Successfully retrieved bookmarked itineraries",
		slog.String("userID", userID.String()),
		slog.Int("totalRecords", response.TotalRecords),
		slog.Int("page", response.Page),
		slog.Int("pageSize", response.PageSize))

	span.SetAttributes(
		attribute.Int("total_records", response.TotalRecords),
		attribute.Int("returned_count", len(response.Itineraries)),
	)
	span.SetStatus(codes.Ok, "Bookmarked itineraries retrieved successfully")
	return response, nil
}

func (l *ServiceImpl) RemoveItenerary(ctx context.Context, userID, itineraryID uuid.UUID) error {
	ctx, span := otel.Tracer("LlmInteractionService").Start(ctx, "RemoveItenerary", trace.WithAttributes(
		attribute.String("user.id", userID.String()),
		attribute.String("itinerary.id", itineraryID.String()),
	))
	defer span.End()

	l.logger.InfoContext(ctx, "Attempting to remove chat from bookmark",
		slog.String("itineraryID", itineraryID.String()))

	if err := l.llmInteractionRepo.RemoveChatFromBookmark(ctx, userID, itineraryID); err != nil {
		l.logger.ErrorContext(ctx, "Failed to remove chat from bookmark", slog.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to remove chat from bookmark")
		return fmt.Errorf("failed to remove chat from bookmark: %w", err)
	}

	l.logger.InfoContext(ctx, "Successfully removed chat from bookmark", slog.String("itineraryID", itineraryID.String()))
	span.SetStatus(codes.Ok, "Itinerary removed successfully")
	return nil
}

// GetUserChatSessions retrieves paginated chat sessions for a user
func (l *ServiceImpl) GetUserChatSessions(ctx context.Context, userID uuid.UUID, page, limit int) (*types.ChatSessionsResponse, error) {
	ctx, span := otel.Tracer("LlmInteractionService").Start(ctx, "GetUserChatSessions", trace.WithAttributes(
		attribute.String("user.id", userID.String()),
		attribute.Int("page", page),
		attribute.Int("limit", limit),
	))
	defer span.End()

	l.logger.InfoContext(ctx, "Retrieving paginated chat sessions for user",
		slog.String("userID", userID.String()),
		slog.Int("page", page),
		slog.Int("limit", limit))

	response, err := l.llmInteractionRepo.GetUserChatSessions(ctx, userID, page, limit)
	if err != nil {
		l.logger.ErrorContext(ctx, "Failed to get user chat sessions", slog.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to get user chat sessions")
		return nil, fmt.Errorf("failed to get user chat sessions: %w", err)
	}

	l.logger.InfoContext(ctx, "Successfully retrieved paginated chat sessions",
		slog.String("userID", userID.String()),
		slog.Int("sessionCount", len(response.Sessions)),
		slog.Int("total", response.Total),
		slog.Int("page", response.Page),
		slog.Int("limit", response.Limit))
	span.SetAttributes(
		attribute.Int("sessions.count", len(response.Sessions)),
		attribute.Int("sessions.total", response.Total),
		attribute.Int("response.page", response.Page),
		attribute.Int("response.limit", response.Limit),
	)
	span.SetStatus(codes.Ok, "Chat sessions retrieved successfully")
	return response, nil
}

// getPOIDetailedInfos returns a formatted string with POI details.
func (l *ServiceImpl) getPOIDetailedInfos(wg *sync.WaitGroup, ctx context.Context,
	city string, lat float64, lon float64, userID uuid.UUID,
	resultCh chan<- types.POIDetailedInfo, config *genai.GenerateContentConfig) {
	defer wg.Done()
	ctx, span := otel.Tracer("LlmInteractionService").Start(ctx, "getPOIDetailedInfos", trace.WithAttributes(
		attribute.String("city.name", city),
		attribute.Float64("latitude", lat),
		attribute.Float64("longitude", lon),
	))
	defer span.End()

	if city == "" || lat == 0 || lon == 0 {
		return
	}

	startTime := time.Now()

	prompt := getPOIDetailsPrompt(city, lat, lon)
	span.SetAttributes(attribute.Int("prompt.length", len(prompt)))
	response, err := l.aiClient.GenerateResponse(ctx, prompt, config)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to generate POI details")
		resultCh <- types.POIDetailedInfo{Err: fmt.Errorf("failed to generate POI details: %w", err)}
		return
	}

	var txt string
	for _, candidate := range response.Candidates {
		if candidate.Content != nil && len(candidate.Content.Parts) > 0 {
			txt = candidate.Content.Parts[0].Text
			break
		}
	}
	if txt == "" {
		err := fmt.Errorf("no valid POI details content from AI")
		span.RecordError(err)
		span.SetStatus(codes.Error, "Empty response from AI")
		resultCh <- types.POIDetailedInfo{Err: err}
		return
	}

	span.SetAttributes(attribute.Int("response.length", len(txt)))
	cleanTxt := cleanJSONResponse(txt)
	var detailedInfo types.POIDetailedInfo
	if err := json.Unmarshal([]byte(cleanTxt), &detailedInfo); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to parse POI details JSON")
		resultCh <- types.POIDetailedInfo{Err: fmt.Errorf("failed to parse POI details JSON: %w", err)}
		return
	}
	latencyMs := int(time.Since(startTime).Milliseconds())
	span.SetAttributes(attribute.Int("response.latency_ms", latencyMs))
	span.SetStatus(codes.Ok, "POI details generated successfully")
	interaction := types.LlmInteraction{
		UserID:       userID,
		Prompt:       prompt,
		ResponseText: txt,
		ModelUsed:    model, // Adjust based on your AI client
		LatencyMs:    latencyMs,
		CityName:     city,
		// request payload
		// response payload
		// Add token counts if available from response (depends on genai API)
		// PromptTokens, CompletionTokens, TotalTokens
		// RequestPayload, ResponsePayload if you serialize the full request/response
	}

	savedInteractionID, err := l.llmInteractionRepo.SaveInteraction(ctx, interaction)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to save LLM interaction for POI details")
		resultCh <- types.POIDetailedInfo{Err: fmt.Errorf("failed to save LLM interaction for POI details: %w", err)}
		return
	}
	resultCh <- types.POIDetailedInfo{
		City:         city,
		Name:         detailedInfo.Name,
		Latitude:     detailedInfo.Latitude,
		Longitude:    detailedInfo.Longitude,
		Description:  detailedInfo.Description,
		Address:      detailedInfo.Address,
		OpeningHours: detailedInfo.OpeningHours,
		PhoneNumber:  detailedInfo.PhoneNumber,
		Website:      detailedInfo.Website,
		Rating:       detailedInfo.Rating,
		Tags:         detailedInfo.Tags,
		Images:       detailedInfo.Images,
		PriceRange:   detailedInfo.PriceRange,
		Err:          nil,
		// Include the saved interaction ID for tracking

		LlmInteractionID: savedInteractionID,
	}
	span.SetAttributes(attribute.String("llm_interaction.id", savedInteractionID.String()))
	span.SetStatus(codes.Ok, "POI details generated and saved successfully")
}

func (l *ServiceImpl) GetPOIDetailedInfosResponse(ctx context.Context, userID uuid.UUID, city string, lat, lon float64) (*types.POIDetailedInfo, error) {
	ctx, span := otel.Tracer("LlmInteractionService").Start(ctx, "GetPOIDetailedInfosResponse", trace.WithAttributes(
		attribute.String("city.name", city),
		attribute.Float64("latitude", lat),
		attribute.Float64("longitude", lon),
		attribute.String("user.id", userID.String()),
	))
	defer span.End()

	l.logger.DebugContext(ctx, "Starting POI details generation",
		slog.String("city", city), slog.Float64("latitude", lat), slog.Float64("longitude", lon), slog.String("userID", userID.String()))

	// Generate cache key
	cacheKey := generatePOICacheKey(city, lat, lon, 0.0, userID)
	span.SetAttributes(attribute.String("cache.key", cacheKey))

	// Check cache
	if cached, found := l.cache.Get(cacheKey); found {
		if p, ok := cached.(*types.POIDetailedInfo); ok {
			l.logger.InfoContext(ctx, "Cache hit for POI details", slog.String("cache_key", cacheKey))
			span.AddEvent("Cache hit")
			span.SetStatus(codes.Ok, "POI details served from cache")
			return p, nil
		}
	}

	// Find city ID
	cityData, err := l.cityRepo.FindCityByNameAndCountry(ctx, city, "") // Adjust country if needed
	if err != nil {
		l.logger.ErrorContext(ctx, "Failed to find city", slog.Any("error", err))
		span.RecordError(err)
		return nil, fmt.Errorf("failed to find city: %w", err)
	}
	if cityData == nil {
		l.logger.WarnContext(ctx, "City not found", slog.String("city", city))
		span.SetStatus(codes.Error, "City not found")
		return nil, fmt.Errorf("city %s not found", city)
	}
	cityID := cityData.ID

	p, err := l.poiRepo.FindPOIDetails(ctx, cityID, lat, lon, 100.0) // 100m tolerance
	if err != nil {
		l.logger.ErrorContext(ctx, "Failed to query POI details from database", slog.Any("error", err))
		span.RecordError(err)
		return nil, fmt.Errorf("failed to query POI details: %w", err)
	}
	if p != nil {
		p.City = city
		l.cache.Set(cacheKey, p, cache.DefaultExpiration)
		l.logger.InfoContext(ctx, "Database hit for POI details", slog.String("cache_key", cacheKey))
		span.AddEvent("Database hit")
		span.SetStatus(codes.Ok, "POI details served from database")
		return p, nil
	}

	// Cache and database miss: fetch from Gemini API
	l.logger.DebugContext(ctx, "Cache and database miss, fetching POI details from AI", slog.String("cache_key", cacheKey))
	span.AddEvent("Cache and database miss")

	resultCh := make(chan types.POIDetailedInfo, 1)
	var wg sync.WaitGroup
	wg.Add(1)

	go l.getPOIDetailedInfos(&wg, ctx, city, lat, lon, userID, resultCh, &genai.GenerateContentConfig{Temperature: genai.Ptr[float32](defaultTemperature)})

	go func() {
		wg.Wait()
		close(resultCh)
	}()

	var poiResult *types.POIDetailedInfo
	for res := range resultCh {
		if res.Err != nil {
			l.logger.ErrorContext(ctx, "Error generating POI details", slog.Any("error", res.Err))
			span.RecordError(res.Err)
			span.SetStatus(codes.Error, "Failed to generate POI details")
			return nil, res.Err
		}
		poiResult = &res
		break
	}

	if poiResult == nil {
		l.logger.WarnContext(ctx, "No response received for POI details")
		span.SetStatus(codes.Error, "No response received")
		return nil, fmt.Errorf("no response received for POI details")
	}

	// Save to database
	_, err = l.poiRepo.SavePoi(ctx, *poiResult, cityID)
	if err != nil {
		l.logger.WarnContext(ctx, "Failed to save POI details to database", slog.Any("error", err))
		span.RecordError(err)
		// Continue despite error to avoid blocking user
	}

	// Store in cache
	l.cache.Set(cacheKey, poiResult, cache.DefaultExpiration)
	l.logger.DebugContext(ctx, "Stored POI details in cache", slog.String("cache_key", cacheKey))
	span.AddEvent("Stored in cache")

	span.SetStatus(codes.Ok, "POI details generated and cached successfully")
	return poiResult, nil
}

// generatePOIData queries the LLM for POI details and calculates distance using PostGIS
func (l *ServiceImpl) generatePOIData(ctx context.Context, poiName, cityName string, userLocation *types.UserLocation, userID, cityID uuid.UUID) (types.POIDetailedInfo, error) {
	ctx, span := otel.Tracer("LlmInteractionService").Start(ctx, "GeneratePOIData", trace.WithAttributes(
		attribute.String("p.name", poiName),
		attribute.String("city.name", cityName),
	))
	defer span.End()

	// Create a prompt for the LLM
	prompt := generatedContinuedConversationPrompt(poiName, cityName)

	// Generate LLM response
	response, err := l.aiClient.GenerateContent(ctx, prompt, "", nil)
	if err != nil {
		span.RecordError(err)
		return types.POIDetailedInfo{}, fmt.Errorf("failed to generate POI data: %w", err)
	}

	interaction := types.LlmInteraction{
		UserID:       userID,
		Prompt:       prompt,
		ResponseText: response,
		ModelUsed:    model,
		CityName:     cityName,
	}
	savedLlmInteractionID, err := l.llmInteractionRepo.SaveInteraction(ctx, interaction)
	if err != nil {
		l.logger.ErrorContext(ctx, "Failed to save LLM interaction in generatePOIData", slog.Any("error", err))
		// Decide if this is fatal for POI generation. It might be if FK is NOT NULL.
		return types.POIDetailedInfo{}, fmt.Errorf("failed to save LLM interaction: %w", err)
	}
	span.SetAttributes(attribute.String("llm.interaction_id.for_poi_data", savedLlmInteractionID.String()))

	cleanResponse := cleanJSONResponse(response)
	var poiData types.POIDetailedInfo
	if err := json.Unmarshal([]byte(cleanResponse), &poiData); err != nil || poiData.Name == "" {
		l.logger.WarnContext(ctx, "LLM returned invalid or empty POI data",
			slog.String("poiName", poiName),
			slog.String("llmResponse", response),
			slog.Any("unmarshalError", err))
		span.AddEvent("Invalid LLM response")
		poiData = types.POIDetailedInfo{
			ID:             uuid.New(),
			Name:           poiName,
			Latitude:       0,
			Longitude:      0,
			Category:       "Attraction",
			DescriptionPOI: fmt.Sprintf("Added %s based on user request, but detailed data not available.", poiName),
			Distance:       0,
		}
	}
	if poiData.ID == uuid.Nil { // Assign an ID if LLM didn't provide one
		poiData.ID = uuid.New()
	}
	poiData.LlmInteractionID = savedLlmInteractionID

	// Calculate distance if coordinates are valid
	if userLocation != nil && userLocation.UserLat != 0 && userLocation.UserLon != 0 && poiData.Latitude != 0 && poiData.Longitude != 0 {
		distance, err := l.poiRepo.CalculateDistancePostGIS(ctx, userLocation.UserLat, userLocation.UserLon, poiData.Latitude, poiData.Longitude)
		if err != nil {
			l.logger.WarnContext(ctx, "Failed to calculate distance", slog.Any("error", err))
			span.RecordError(err)
			poiData.Distance = 0
		} else {
			poiData.Distance = distance
			span.SetAttributes(attribute.Float64("p.distance_meters", distance))
			l.logger.DebugContext(ctx, "Calculated distance for POI",
				slog.String("poiName", poiName),
				slog.Float64("distance_meters", distance))
		}
	} else {
		poiData.Distance = 0
		span.AddEvent("Distance not calculated due to missing location data")
		l.logger.WarnContext(ctx, "Cannot calculate distance",
			slog.Bool("userLocationAvailable", userLocation != nil),
			slog.Float64("userLat", userLocation.UserLat),
			slog.Float64("userLon", userLocation.UserLon),
			slog.Float64("poiLatitude", poiData.Latitude),
			slog.Float64("poiLongitude", poiData.Longitude))
	}

	// Save POI to database
	llmInteractionID := uuid.New()
	_, err = l.llmInteractionRepo.SaveSinglePOI(ctx, poiData, userID, cityID, savedLlmInteractionID)
	if err != nil {
		l.logger.WarnContext(ctx, "Failed to save POI to database", slog.Any("error", err))
		span.RecordError(err)
	}

	span.SetAttributes(
		attribute.String("p.name", poiData.Name),
		attribute.Float64("p.latitude", poiData.Latitude),
		attribute.Float64("p.longitude", poiData.Longitude),
		attribute.String("p.category", poiData.Category),
		attribute.String("llm_interaction.id", llmInteractionID.String()),
	)
	return poiData, nil
}

// enhancePOIRecommendationsWithSemantics uses embeddings to find similar POIs and enrich recommendations
//func (l *ServiceImpl) enhancePOIRecommendationsWithSemantics(ctx context.Context, userMessage string, cityID uuid.UUID, userPreferences []string, limit int) ([]types.POIDetailedInfo, error) {
//	ctx, span := otel.Tracer("LlmInteractionService").Start(ctx, "enhancePOIRecommendationsWithSemantics", trace.WithAttributes(
//		attribute.String("user.message", userMessage),
//		attribute.String("city.id", cityID.String()),
//		attribute.Int("limit", limit),
//	))
//	defer span.End()
//
//	l.logger.DebugContext(ctx, "Enhancing POI recommendations with semantic search",
//		slog.String("message", userMessage),
//		slog.String("city_id", cityID.String()))
//
//	if l.embeddingService == nil {
//		l.logger.WarnContext(ctx, "Embedding service not available, falling back to traditional search")
//		span.AddEvent("Embedding service not available")
//		return []types.POIDetailedInfo{}, nil
//	}
//
//	// Generate embedding for user message combined with preferences
//	searchQuery := userMessage
//	if len(userPreferences) > 0 {
//		searchQuery += " " + strings.Join(userPreferences, " ")
//	}
//
//	queryEmbedding, err := l.embeddingService.GenerateQueryEmbedding(ctx, searchQuery)
//	if err != nil {
//		l.logger.ErrorContext(ctx, "Failed to generate query embedding",
//			slog.Any("error", err),
//			slog.String("query", searchQuery))
//		span.RecordError(err)
//		span.SetStatus(codes.Error, "Failed to generate query embedding")
//		return []types.POIDetailedInfo{}, fmt.Errorf("failed to generate query embedding: %w", err)
//	}
//
//	// Search for similar POIs in the city
//	similarPOIs, err := l.poiRepo.FindSimilarPOIsByCity(ctx, queryEmbedding, cityID, limit)
//	if err != nil {
//		l.logger.ErrorContext(ctx, "Failed to find similar POIs", slog.Any("error", err))
//		span.RecordError(err)
//		span.SetStatus(codes.Error, "Failed to find similar POIs")
//		return []types.POIDetailedInfo{}, fmt.Errorf("failed to find similar POIs: %w", err)
//	}
//
//	l.logger.InfoContext(ctx, "Found semantically similar POIs",
//		slog.Int("count", len(similarPOIs)),
//		slog.String("city_id", cityID.String()))
//	span.SetAttributes(
//		attribute.Int("similar_pois.count", len(similarPOIs)),
//		attribute.String("search.query", searchQuery),
//	)
//	span.SetStatus(codes.Ok, "Semantic POI recommendations enhanced")
//
//	return similarPOIs, nil
//}

// generateSemanticPOIRecommendations generates POI recommendations using semantic search
func (l *ServiceImpl) generateSemanticPOIRecommendations(ctx context.Context, userMessage string, cityID uuid.UUID, userID uuid.UUID, userLocation *types.UserLocation, semanticWeight float64) ([]types.POIDetailedInfo, error) {
	ctx, span := otel.Tracer("LlmInteractionService").Start(ctx, "generateSemanticPOIRecommendations", trace.WithAttributes(
		attribute.String("user.message", userMessage),
		attribute.String("city.id", cityID.String()),
		attribute.String("user.id", userID.String()),
		attribute.Float64("semantic.weight", semanticWeight),
	))
	defer span.End()

	l.logger.DebugContext(ctx, "Generating semantic POI recommendations",
		slog.String("message", userMessage),
		slog.String("city_id", cityID.String()),
		slog.Float64("semantic_weight", semanticWeight))

	if l.embeddingService == nil {
		err := fmt.Errorf("embedding service not available")
		l.logger.ErrorContext(ctx, "Embedding service not available", slog.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Embedding service not available")
		return nil, err
	}

	// Generate embedding for user message
	queryEmbedding, err := l.embeddingService.GenerateQueryEmbedding(ctx, userMessage)
	if err != nil {
		l.logger.ErrorContext(ctx, "Failed to generate query embedding", slog.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to generate query embedding")
		return nil, fmt.Errorf("failed to generate query embedding: %w", err)
	}

	var pois []types.POIDetailedInfo

	// If user location is available, use hybrid search (spatial + semantic)
	if userLocation != nil && userLocation.UserLat != 0 && userLocation.UserLon != 0 {
		filter := types.POIFilter{
			Location: types.GeoPoint{
				Latitude:  userLocation.UserLat,
				Longitude: userLocation.UserLon,
			},
			Radius: userLocation.SearchRadiusKm,
		}

		hybridPOIs, err := l.poiRepo.SearchPOIsHybrid(ctx, filter, queryEmbedding, semanticWeight)
		if err != nil {
			l.logger.ErrorContext(ctx, "Failed to perform hybrid search", slog.Any("error", err))
			span.RecordError(err)
			// Fall back to semantic-only search
		} else {
			pois = hybridPOIs
			l.logger.InfoContext(ctx, "Used hybrid search for POI recommendations",
				slog.Int("poi_count", len(pois)))
			span.AddEvent("Used hybrid search")
		}
	}

	// If hybrid search failed or no location available, use semantic-only search
	if len(pois) == 0 {
		semanticPOIs, err := l.poiRepo.FindSimilarPOIsByCity(ctx, queryEmbedding, cityID, 10)
		if err != nil {
			l.logger.ErrorContext(ctx, "Failed to find similar POIs", slog.Any("error", err))
			span.RecordError(err)
			span.SetStatus(codes.Error, "Failed to find similar POIs")
			return nil, fmt.Errorf("failed to find similar POIs: %w", err)
		}
		pois = semanticPOIs
		l.logger.InfoContext(ctx, "Used semantic-only search for POI recommendations",
			slog.Int("poi_count", len(pois)))
		span.AddEvent("Used semantic-only search")
	}

	// Generate embeddings for new POIs if needed
	for i, p := range pois {
		if p.ID == uuid.Nil {
			continue
		}

		// Generate embedding for this POI if it doesn't have one
		embedding, err := l.embeddingService.GeneratePOIEmbedding(ctx, p.Name, p.DescriptionPOI, p.Category)
		if err != nil {
			l.logger.WarnContext(ctx, "Failed to generate embedding for POI",
				slog.Any("error", err),
				slog.String("poi_name", p.Name))
			continue
		}

		// Update POI with embedding
		err = l.poiRepo.UpdatePOIEmbedding(ctx, p.ID, embedding)
		if err != nil {
			l.logger.WarnContext(ctx, "Failed to update POI embedding",
				slog.Any("error", err),
				slog.String("poi_id", p.ID.String()))
		}

		pois[i] = p
	}

	l.logger.InfoContext(ctx, "Generated semantic POI recommendations",
		slog.String("message", userMessage),
		slog.Int("recommendations", len(pois)))
	span.SetAttributes(
		attribute.String("search.query", userMessage),
		attribute.Int("recommendations.count", len(pois)),
		attribute.Float64("semantic.weight", semanticWeight),
	)
	span.SetStatus(codes.Ok, "Semantic POI recommendations generated")

	return pois, nil
}

// handleSemanticRemovePOI handles removing POIs with semantic understanding
func (l *ServiceImpl) handleSemanticRemovePOI(ctx context.Context, message string, session *types.ChatSession) string {
	ctx, span := otel.Tracer("LlmInteractionService").Start(ctx, "handleSemanticRemovePOI")
	defer span.End()

	poiName := extractPOIName(message)
	if poiName == "" {
		return "I'd be happy to remove a POI from your itinerary! Could you please specify which place you'd like to remove?"
	}

	// Use semantic matching for removal - be more flexible with name matching
	for i, p := range session.CurrentItinerary.AIItineraryResponse.PointsOfInterest {
		// Check for exact match or semantic similarity
		if strings.EqualFold(p.Name, poiName) ||
			strings.Contains(strings.ToLower(p.Name), strings.ToLower(poiName)) ||
			strings.Contains(strings.ToLower(poiName), strings.ToLower(p.Name)) {

			removedName := p.Name
			session.CurrentItinerary.AIItineraryResponse.PointsOfInterest = append(
				session.CurrentItinerary.AIItineraryResponse.PointsOfInterest[:i],
				session.CurrentItinerary.AIItineraryResponse.PointsOfInterest[i+1:]...,
			)
			l.logger.InfoContext(ctx, "Removed POI from itinerary",
				slog.String("removed_poi", removedName))
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

// min helper function
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// extractCityFromMessage uses AI to extract city name and clean the message
func (l *ServiceImpl) extractCityFromMessage(ctx context.Context, message string) (cityName, cleanedMessage string, err error) {
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
		Temperature: genai.Ptr[float32](0.1), // Low temperature for consistent parsing
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

	// If no city extracted, return original message
	if parsed.City == "" {
		return "", message, nil
	}

	return parsed.City, parsed.Message, nil
}

// TODO For robustness, send unprocessed events to a dead letter queue (e.g., a separate channel or database table) for later analysis:
// if !l.sendEvent(ctx, eventCh, event) {
//     l.logger.ErrorContext(ctx, "Sending to dead letter queue", slog.Any("event", event))
//     // Save to a persistent store
// }

func (l *ServiceImpl) sendEvent(ctx context.Context, ch chan<- types.StreamEvent, event types.StreamEvent, retries int) bool {
	for i := 0; i < retries; i++ {
		if event.EventID == "" {
			event.EventID = uuid.New().String()
		}
		if event.Timestamp.IsZero() {
			event.Timestamp = time.Now()
		}

		select {
		case <-ctx.Done():
			l.logger.WarnContext(ctx, "Context cancelled, not sending stream event", slog.String("eventType", event.Type))
			l.deadLetterCh <- event // Send to dead letter queue
			return false
		default:
			select {
			case ch <- event:
				return true
			case <-ctx.Done():
				l.logger.WarnContext(ctx, "Context cancelled while trying to send stream event", slog.String("eventType", event.Type))
				l.deadLetterCh <- event // Send to dead letter queue
				return false
			case <-time.After(2 * time.Second): // Use a reasonable timeout
				l.logger.WarnContext(ctx, "Dropped stream event due to slow consumer or blocked channel (timeout)", slog.String("eventType", event.Type))
				l.deadLetterCh <- event // Send to dead letter queue
				// Continue to retry after backoff
			}
		}
		time.Sleep(100 * time.Millisecond) // Backoff
	}
	return false
}

func (l *ServiceImpl) processDeadLetterQueue() {
	for event := range l.deadLetterCh {
		l.logger.ErrorContext(context.Background(), "Unprocessed event sent to dead letter queue", slog.Any("event", event))
		// TODO Save events to DB
	}
}

// getPersonalizedPOI generates a prompt for personalized POIs
func getPersonalizedPOI(interestNames []string, cityName, tagsPromptPart, userPrefs string) string {
	prompt := fmt.Sprintf(`
        Generate a personalized trip itinerary for %s, tailored to user interests [%s]. Include:
        1. An itinerary name.
        2. An overall description.
        3. A list of points of interest with name, category, coordinates, and detailed description.
		Max points of interest allowed by tokens.
        Format the response in JSON with the following structure:
        {
            "itinerary_name": "Name of the itinerary",
            "overall_description": "Description of the itinerary",
            "points_of_interest": [
                {
                    "name": "POI name",
                    "category": "Category",
                    "coordinates": {
                        "latitude": float64,
                        "longitude": float64
                    },
                    "description": "Detailed description of why this POI matches the user's interests"
                }
            ]
        }
    `, cityName, strings.Join(interestNames, ", "))
	if tagsPromptPart != "" {
		prompt += "\n" + tagsPromptPart
	}
	if userPrefs != "" {
		prompt += "\n" + userPrefs
	}
	return prompt
}

// ContinueSessionStreamed handles subsequent messages in an existing session and streams responses/updates.
func (l *ServiceImpl) ContinueSessionStreamed(
	ctx context.Context, sessionID uuid.UUID,
	message string, userLocation *types.UserLocation,
	eventCh chan<- types.StreamEvent, // Output channel for events
) error { // Only returns error for critical setup failures
	ctx, span := otel.Tracer("LlmInteractionService").Start(ctx, "ContinueSessionStreamed", trace.WithAttributes(
		attribute.String("session.id", sessionID.String()),
		attribute.String("message", message),
	))
	defer span.End()

	l.logger.DebugContext(ctx, "Continuing streamed chat session", slog.String("sessionID", sessionID.String()), slog.String("message", message))

	// --- 1. Fetch Session & Basic Validation ---
	session, err := l.llmInteractionRepo.GetSession(ctx, sessionID)
	if err != nil {
		err = fmt.Errorf("failed to get session %s: %w", sessionID, err)
		l.sendEvent(ctx, eventCh, types.StreamEvent{Type: types.EventTypeError, Error: err.Error(), IsFinal: true}, 3)
		return err
	}
	if session.Status != types.StatusActive {
		err = fmt.Errorf("session %s is not active (status: %s) %w", sessionID, session.Status, err)
		l.sendEvent(ctx, eventCh, types.StreamEvent{Type: types.EventTypeError, Error: err.Error(), IsFinal: true}, 3)
		return err
	}
	l.sendEvent(ctx, eventCh, types.StreamEvent{Type: "session_validated", Data: map[string]string{"status": "active"}}, 3)

	// --- 2. Fetch City ID ---
	cityData, err := l.cityRepo.FindCityByNameAndCountry(ctx, session.SessionContext.CityName, "")
	if err != nil || cityData == nil {
		// If the city is not found, try a fuzzy match
		cityData, err = l.cityRepo.FindCityByFuzzyName(ctx, session.SessionContext.CityName)
		if err != nil || cityData == nil {
			if err == nil {
				err = fmt.Errorf("city '%s' not found for session %s %w", session.SessionContext.CityName, sessionID, err)
			} else {
				err = fmt.Errorf("failed to find city '%s' for session %s: %w", session.SessionContext.CityName, sessionID, err)
			}
			l.sendEvent(ctx, eventCh, types.StreamEvent{Type: types.EventTypeError, Error: err.Error(), IsFinal: true}, 3)
			return err
		}
	}
	cityID := cityData.ID
	l.sendEvent(ctx, eventCh, types.StreamEvent{Type: types.EventTypeProgress, Data: map[string]interface{}{"status": "context_loaded", "city_id": cityID.String()}}, 3)

	// --- 3. Add User Message to History ---
	userMessage := types.ConversationMessage{
		ID: uuid.New(), Role: types.RoleUser, Content: message, Timestamp: time.Now(), MessageType: types.TypeModificationRequest,
	}
	if err := l.llmInteractionRepo.AddMessageToSession(ctx, sessionID, userMessage); err != nil {
		l.logger.WarnContext(ctx, "Failed to persist user message, continuing with in-memory history", slog.Any("error", err))
		span.RecordError(err, trace.WithAttributes(attribute.String("warning", "User message DB save failed")))
	}
	session.ConversationHistory = append(session.ConversationHistory, userMessage)

	// --- 4. Classify Intent ---
	intent, err := l.intentClassifier.Classify(ctx, message)
	if err != nil {
		err = fmt.Errorf("failed to classify intent for message '%s': %w", message, err)
		l.sendEvent(ctx, eventCh, types.StreamEvent{Type: types.EventTypeError, Error: err.Error(), IsFinal: true}, 3)
		return err
	}
	l.logger.InfoContext(ctx, "Intent classified", slog.String("intent", string(intent)))
	l.sendEvent(ctx, eventCh, types.StreamEvent{Type: "intent_classified", Data: map[string]string{"intent": string(intent)}}, 3)

	// --- 5. Enhance with Semantic POI Recommendations ---
	l.sendEvent(ctx, eventCh, types.StreamEvent{
		Type: types.EventTypeProgress,
		Data: map[string]interface{}{"status": "generating_semantic_context", "progress": 20},
	}, 3)

	semanticPOIs, err := l.generateSemanticPOIRecommendations(ctx, message, cityID, session.UserID, userLocation, 0.6)
	if err != nil {
		l.logger.WarnContext(ctx, "Failed to generate semantic POI recommendations for streaming session", slog.Any("error", err))
		l.sendEvent(ctx, eventCh, types.StreamEvent{
			Type: types.EventTypeProgress,
			Data: map[string]interface{}{"status": "semantic_context_failed", "progress": 22},
		}, 3)
	} else {
		l.logger.InfoContext(ctx, "Generated semantic POI recommendations for streaming session",
			slog.Int("semantic_recommendations", len(semanticPOIs)))
		l.sendEvent(ctx, eventCh, types.StreamEvent{
			Type: "semantic_context_generated",
			Data: map[string]interface{}{
				"status":                         "semantic_context_ready",
				"semantic_recommendations_count": len(semanticPOIs),
				"progress":                       25,
			},
		}, 3)
	}

	// --- 5. Handle Intent and Generate Response ---
	var finalResponseMessage string
	assistantMessageType := types.TypeResponse
	itineraryModifiedByThisTurn := false

	switch intent { // Align with ContinueSession's string-based intents
	case types.IntentAddPOI:
		l.sendEvent(ctx, eventCh, types.StreamEvent{Type: types.EventTypeProgress, Data: "Processing: Adding Point of Interest with semantic enhancement..."}, 3)
		var genErr error
		finalResponseMessage, genErr = l.handleSemanticAddPOIStreamed(ctx, message, session, semanticPOIs, userLocation, cityID, eventCh)
		if genErr != nil {
			finalResponseMessage = "I had trouble understanding your request. Could you please specify which POI you'd like to add?"
			assistantMessageType = types.TypeError
		} else {
			itineraryModifiedByThisTurn = true
		}

	case types.IntentRemovePOI:
		l.sendEvent(ctx, eventCh, types.StreamEvent{Type: types.EventTypeProgress, Data: "Processing: Removing Point of Interest with semantic understanding..."}, 3)
		finalResponseMessage = l.handleSemanticRemovePOI(ctx, message, session)
		if strings.Contains(finalResponseMessage, "I've removed") {
			itineraryModifiedByThisTurn = true
		}

	case types.IntentAskQuestion:
		l.sendEvent(ctx, eventCh, types.StreamEvent{Type: types.EventTypeProgress, Data: "Processing: Answering your question with semantic context..."}, 3)
		finalResponseMessage = "I’m here to help! For now, I’ll assume you’re asking about your trip. What specifically would you like to know?"

	case "replace_poi":
		l.sendEvent(ctx, eventCh, types.StreamEvent{Type: types.EventTypeProgress, Data: "Processing: Replacing Point of Interest..."}, 3)
		if matches := regexp.MustCompile(`replace\s+(.+?)\s+with\s+(.+?)(?:\s+in\s+my\s+itinerary)?`).FindStringSubmatch(strings.ToLower(message)); len(matches) == 3 {
			oldPOI := matches[1]
			newPOIName := matches[2]
			for i, p := range session.CurrentItinerary.AIItineraryResponse.PointsOfInterest {
				if strings.Contains(strings.ToLower(p.Name), oldPOI) {
					newPOI, err := l.generatePOIDataStream(ctx, newPOIName, session.SessionContext.CityName, userLocation, session.UserID, cityID, eventCh)
					if err != nil {
						finalResponseMessage = fmt.Sprintf("Could not replace %s with %s due to an error: %v", oldPOI, newPOIName, err)
						assistantMessageType = types.TypeError
					} else {
						session.CurrentItinerary.AIItineraryResponse.PointsOfInterest[i] = newPOI
						finalResponseMessage = fmt.Sprintf("I've replaced %s with %s in your itinerary.", oldPOI, newPOIName)
						itineraryModifiedByThisTurn = true
					}
					break
				}
			}
			if finalResponseMessage == "" {
				finalResponseMessage = fmt.Sprintf("Could not find %s in your itinerary.", oldPOI)
			}
		} else {
			finalResponseMessage = "Please specify the replacement clearly (e.g., 'replace X with Y')."
			assistantMessageType = types.TypeClarification
		}

	default: // modify_itinerary
		l.sendEvent(ctx, eventCh, types.StreamEvent{Type: types.EventTypeProgress, Data: "Processing: Updating itinerary..."}, 3)
		if matches := regexp.MustCompile(`replace\s+(.+?)\s+with\s+(.+?)(?:\s+in\s+my\s+itinerary)?`).FindStringSubmatch(strings.ToLower(message)); len(matches) == 3 {
			oldPOI := matches[1]
			newPOIName := matches[2]
			for i, p := range session.CurrentItinerary.AIItineraryResponse.PointsOfInterest {
				if strings.Contains(strings.ToLower(p.Name), oldPOI) {
					newPOI, err := l.generatePOIData(ctx, newPOIName, session.SessionContext.CityName, userLocation, session.UserID, cityID)
					if err != nil {
						l.logger.ErrorContext(ctx, "Failed to generate POI data", slog.Any("error", err))
						span.RecordError(err)
						finalResponseMessage = fmt.Sprintf("Could not replace %s with %s due to an error.", oldPOI, newPOIName)
					} else {
						session.CurrentItinerary.AIItineraryResponse.PointsOfInterest[i] = newPOI
						finalResponseMessage = fmt.Sprintf("I’ve replaced %s with %s in your itinerary.", oldPOI, newPOIName)
					}
					break
				}
			}
			if finalResponseMessage == "" {
				finalResponseMessage = fmt.Sprintf("Could not find %s in your itinerary.", oldPOI)
			}
		} else {
			finalResponseMessage = "I’ve noted your request to modify the itinerary. Please specify the changes (e.g., 'replace X with Y')."
		}
	}

	// --- 6. Post-Modification Processing (Sorting, Saving Session) ---
	if itineraryModifiedByThisTurn && userLocation != nil && userLocation.UserLat != 0 && userLocation.UserLon != 0 && session.CurrentItinerary != nil {
		l.sendEvent(ctx, eventCh, types.StreamEvent{Type: types.EventTypeProgress, Data: "Sorting updated POIs by distance..."}, 3)
		// Save new POIs to DB to ensure they have valid IDs
		for i, p := range session.CurrentItinerary.AIItineraryResponse.PointsOfInterest {
			if p.ID == uuid.Nil {
				dbPoiID, saveErr := l.llmInteractionRepo.SaveSinglePOI(ctx, p, session.UserID, cityID, p.LlmInteractionID)
				if saveErr != nil {
					l.logger.WarnContext(ctx, "Failed to save new POI", slog.String("name", p.Name), slog.Any("error", saveErr))
					continue
				}
				session.CurrentItinerary.AIItineraryResponse.PointsOfInterest[i].ID = dbPoiID
			}
		}

		if (intent == types.IntentAddPOI || intent == types.IntentModifyItinerary) && userLocation != nil && userLocation.UserLat != 0 && userLocation.UserLon != 0 {
			sortedPOIs, err := l.llmInteractionRepo.GetPOIsBySessionSortedByDistance(ctx, sessionID, cityID, *userLocation)
			if err != nil {
				l.logger.WarnContext(ctx, "Failed to sort POIs by distance", slog.Any("error", err))
				span.RecordError(err)
			} else {
				session.CurrentItinerary.AIItineraryResponse.PointsOfInterest = sortedPOIs
				l.logger.InfoContext(ctx, "POIs sorted by distance",
					slog.Int("poi_count", len(sortedPOIs)))
				span.SetAttributes(attribute.Int("sorted_pois.count", len(sortedPOIs)))
			}
		}
	}

	// Add assistant's final response to history
	assistantMessage := types.ConversationMessage{
		ID: uuid.New(), Role: types.RoleAssistant, Content: finalResponseMessage, Timestamp: time.Now(), MessageType: assistantMessageType,
	}
	if err := l.llmInteractionRepo.AddMessageToSession(ctx, sessionID, assistantMessage); err != nil {
		l.logger.WarnContext(ctx, "Failed to save assistant message", slog.Any("error", err))
	}
	session.ConversationHistory = append(session.ConversationHistory, assistantMessage)

	// Update session in the database
	session.UpdatedAt = time.Now()
	session.ExpiresAt = time.Now().Add(24 * time.Hour)
	if err := l.llmInteractionRepo.UpdateSession(ctx, *session); err != nil {
		err = fmt.Errorf("failed to update session %s: %w", sessionID, err)
		l.sendEvent(ctx, eventCh, types.StreamEvent{Type: types.EventTypeError, Error: err.Error(), IsFinal: true}, 3)
		return err
	}

	// --- 7. Send Final Itinerary and Completion Event ---
	l.sendEvent(ctx, eventCh, types.StreamEvent{
		Type:      types.EventTypeItinerary,
		Data:      session.CurrentItinerary,
		Message:   finalResponseMessage,
		Timestamp: time.Now(),
	}, 3)
	l.sendEvent(ctx, eventCh, types.StreamEvent{
		Type:    types.EventTypeComplete,
		Data:    "Turn completed.",
		IsFinal: true,
		Navigation: &types.NavigationData{
			URL:       fmt.Sprintf("/itinerary?sessionId=%s&cityName=%s&domain=itinerary", sessionID.String(), url.QueryEscape(session.CityName)),
			RouteType: "itinerary",
			QueryParams: map[string]string{
				"sessionId": sessionID.String(),
				"cityName":  session.CityName,
				"domain":    "itinerary",
			},
		},
	}, 3)

	l.logger.InfoContext(ctx, "Streamed session continued", slog.String("sessionID", sessionID.String()), slog.String("intent", string(intent)))
	return nil
}

// generatePOIDataStream queries the LLM for POI details and streams updates
func (l *ServiceImpl) generatePOIDataStream(
	ctx context.Context, poiName, cityName string,
	userLocation *types.UserLocation, userID, cityID uuid.UUID,
	eventCh chan<- types.StreamEvent,
) (types.POIDetailedInfo, error) {
	ctx, span := otel.Tracer("LlmInteractionService").Start(ctx, "generatePOIDataStream",
		trace.WithAttributes(attribute.String("p.name", poiName), attribute.String("city.name", cityName)))
	defer span.End()

	prompt := generatedContinuedConversationPrompt(poiName, cityName)
	config := &genai.GenerateContentConfig{Temperature: genai.Ptr[float32](0.2)}
	startTime := time.Now()

	var responseTextBuilder strings.Builder
	iter, err := l.aiClient.GenerateContentStream(ctx, prompt, config)
	if err != nil {
		l.sendEvent(ctx, eventCh, types.StreamEvent{
			Type:      types.EventTypeError,
			Error:     fmt.Sprintf("Failed to generate POI data for '%s': %v", poiName, err),
			Timestamp: time.Now(),
			EventID:   uuid.New().String(),
		}, 3)
		return types.POIDetailedInfo{}, fmt.Errorf("AI stream init failed for POI '%s': %w", poiName, err)
	}

	l.sendEvent(ctx, eventCh, types.StreamEvent{
		Type:      types.EventTypeProgress,
		Data:      map[string]string{"status": fmt.Sprintf("Getting details for %s...", poiName)},
		Timestamp: time.Now(),
		EventID:   uuid.New().String(),
	}, 3)

	for resp, err := range iter {
		if err != nil {
			l.sendEvent(ctx, eventCh, types.StreamEvent{
				Type:      types.EventTypeError,
				Error:     fmt.Sprintf("Streaming failed for POI '%s': %v", poiName, err),
				Timestamp: time.Now(),
				EventID:   uuid.New().String(),
			}, 3)
			return types.POIDetailedInfo{}, fmt.Errorf("streaming POI details for '%s' failed: %w", poiName, err)
		}
		for _, cand := range resp.Candidates {
			if cand.Content != nil {
				for _, part := range cand.Content.Parts {
					if part.Text != "" {
						responseTextBuilder.WriteString(string(part.Text))
						l.sendEvent(ctx, eventCh, types.StreamEvent{
							Type:      "poi_detail_chunk",
							Data:      map[string]string{"poi_name": poiName, "chunk": string(part.Text)},
							Timestamp: time.Now(),
							EventID:   uuid.New().String(),
						}, 3)
					}
				}
			}
		}
	}

	if ctx.Err() != nil {
		l.sendEvent(ctx, eventCh, types.StreamEvent{
			Type:      types.EventTypeError,
			Error:     ctx.Err().Error(),
			Timestamp: time.Now(),
			EventID:   uuid.New().String(),
		}, 3)
		return types.POIDetailedInfo{}, fmt.Errorf("context cancelled during POI detail generation: %w", ctx.Err())
	}

	fullText := responseTextBuilder.String()
	if fullText == "" {
		l.sendEvent(ctx, eventCh, types.StreamEvent{
			Type:      types.EventTypeError,
			Error:     fmt.Sprintf("Empty response for POI '%s'", poiName),
			Timestamp: time.Now(),
			EventID:   uuid.New().String(),
		}, 3)
		return types.POIDetailedInfo{Name: poiName, DescriptionPOI: "Details not found."}, fmt.Errorf("empty response for POI details '%s'", poiName)
	}

	// Save LLM interaction
	interaction := types.LlmInteraction{
		UserID:       userID,
		Prompt:       prompt,
		ResponseText: fullText,
		Timestamp:    startTime,
		CityName:     cityName,
	}
	llmInteractionID, err := l.saveCityInteraction(ctx, interaction)
	if err != nil {
		l.sendEvent(ctx, eventCh, types.StreamEvent{
			Type:      types.EventTypeError,
			Error:     fmt.Sprintf("Failed to save LLM interaction for POI '%s': %v", poiName, err),
			Timestamp: time.Now(),
			EventID:   uuid.New().String(),
		}, 3)
		return types.POIDetailedInfo{}, fmt.Errorf("failed to save LLM interaction: %w", err)
	}

	// Parse response
	cleanJSON := cleanJSONResponse(fullText)
	var poiData types.POIDetailedInfo
	if err := json.Unmarshal([]byte(cleanJSON), &poiData); err != nil || poiData.Name == "" {
		l.logger.WarnContext(ctx, "Invalid POI data from LLM", slog.String("response", fullText), slog.Any("error", err))
		poiData = types.POIDetailedInfo{
			ID:             uuid.New(),
			Name:           poiName,
			Category:       "Attraction",
			DescriptionPOI: fmt.Sprintf("Added %s based on user request, but detailed data not available.", poiName),
		}
	}
	if poiData.ID == uuid.Nil {
		poiData.ID = uuid.New()
	}
	poiData.LlmInteractionID = llmInteractionID
	poiData.City = cityName

	// Save POI to database
	dbPoiID, err := l.llmInteractionRepo.SaveSinglePOI(ctx, poiData, userID, cityID, llmInteractionID)
	if err != nil {
		l.logger.WarnContext(ctx, "Failed to save POI to database", slog.Any("error", err))
		span.RecordError(err)
		l.sendEvent(ctx, eventCh, types.StreamEvent{
			Type:      types.EventTypeError,
			Error:     fmt.Sprintf("Failed to save POI '%s' to database: %v", poiName, err),
			Timestamp: time.Now(),
			EventID:   uuid.New().String(),
		}, 3)
		return types.POIDetailedInfo{}, fmt.Errorf("failed to save POI to database: %w", err)
	}
	poiData.ID = dbPoiID

	// Calculate distance
	if userLocation != nil && userLocation.UserLat != 0 && userLocation.UserLon != 0 && poiData.Latitude != 0 && poiData.Longitude != 0 {
		distance, err := l.poiRepo.CalculateDistancePostGIS(ctx, userLocation.UserLat, userLocation.UserLon, poiData.Latitude, poiData.Longitude)
		if err != nil {
			l.logger.WarnContext(ctx, "Failed to calculate distance", slog.Any("error", err))
			span.RecordError(err)
		} else {
			poiData.Distance = distance
		}
	}

	l.sendEvent(ctx, eventCh, types.StreamEvent{
		Type:      "poi_detail_complete",
		Data:      poiData,
		Timestamp: time.Now(),
		EventID:   uuid.New().String(),
	}, 3)
	return poiData, nil
}

func (l *ServiceImpl) saveCityInteraction(ctx context.Context, interaction types.LlmInteraction) (uuid.UUID, error) {
	ctx, span := otel.Tracer("LlmInteractionService").Start(ctx, "saveCityInteraction")
	defer span.End()

	if interaction.LatencyMs == 0 {
		// Ensure latency is set if not provided
		interaction.LatencyMs = int(time.Since(interaction.Timestamp).Milliseconds())
	}
	if interaction.ModelUsed == "" {
		interaction.ModelUsed = model // Default model
	}

	interactionID, err := l.llmInteractionRepo.SaveInteraction(ctx, interaction)
	if err != nil {
		span.RecordError(err)
		l.logger.WarnContext(ctx, "Failed to save LLM interaction", slog.Any("error", err))
		return uuid.Nil, fmt.Errorf("failed to save interaction: %w", err)
	}

	span.SetAttributes(attribute.String("interaction.id", interactionID.String()))
	return interactionID, nil
}

// handleSemanticAddPOIStreamed handles adding POIs with semantic search enhancement and streaming updates
func (l *ServiceImpl) handleSemanticAddPOIStreamed(ctx context.Context, message string, session *types.ChatSession, semanticPOIs []types.POIDetailedInfo, userLocation *types.UserLocation, cityID uuid.UUID, eventCh chan<- types.StreamEvent) (string, error) {
	ctx, span := otel.Tracer("LlmInteractionService").Start(ctx, "handleSemanticAddPOIStreamed")
	defer span.End()

	// Ensure the session has an initialized itinerary
	l.ensureItineraryExists(session)

	// Try semantic matching first - look for POIs semantically similar to the user's request
	if len(semanticPOIs) > 0 {
		l.sendEvent(ctx, eventCh, types.StreamEvent{
			Type: types.EventTypeProgress,
			Data: map[string]interface{}{
				"status":           "analyzing_semantic_matches",
				"semantic_options": len(semanticPOIs),
			},
		}, 3)

		for _, semanticPOI := range semanticPOIs[:min(3, len(semanticPOIs))] {
			alreadyExists := false
			for _, existingPOI := range session.CurrentItinerary.AIItineraryResponse.PointsOfInterest {
				if strings.EqualFold(existingPOI.Name, existingPOI.Name) {
					alreadyExists = true
					break
				}
			}

			if !alreadyExists {
				l.sendEvent(ctx, eventCh, types.StreamEvent{
					Type: "semantic_poi_added",
					Data: map[string]interface{}{
						"poi_name":       semanticPOI.Name,
						"poi_category":   semanticPOI.Category,
						"latitude":       semanticPOI.Latitude,
						"longitude":      semanticPOI.Longitude,
						"description":    semanticPOI.DescriptionPOI,
						"semantic_match": true,
					},
				}, 3)

				// Add semantic POI to itinerary
				session.CurrentItinerary.AIItineraryResponse.PointsOfInterest = append(
					session.CurrentItinerary.AIItineraryResponse.PointsOfInterest, semanticPOI)
				l.logger.InfoContext(ctx, "Added semantic POI to streaming itinerary",
					slog.String("poi_name", semanticPOI.Name))
				span.SetAttributes(attribute.String("added_poi", semanticPOI.Name))

				return fmt.Sprintf("Great! I found %s which matches what you're looking for. I've added it to your itinerary. %s",
					semanticPOI.Name, semanticPOI.DescriptionPOI), nil
			}
		}

		// If semantic POIs exist but all are already in itinerary
		l.sendEvent(ctx, eventCh, types.StreamEvent{
			Type: "semantic_alternatives_suggested",
			Data: map[string]interface{}{
				"message": "All semantic matches already in itinerary",
				"alternatives": func() []string {
					var names []string
					for i, p := range semanticPOIs[:min(3, len(semanticPOIs))] {
						names = append(names, p.Name)
						if i >= 2 {
							break
						}
					}
					return names
				}(),
			},
		}, 3)

		return fmt.Sprintf("I found some great options matching your request, but they're already in your itinerary. Here are some suggestions: %s",
			strings.Join(func() []string {
				var names []string
				for i, p := range semanticPOIs[:min(3, len(semanticPOIs))] {
					names = append(names, p.Name)
					if i >= 2 {
						break
					}
				}
				return names
			}(), ", ")), nil
	}

	// Fallback to traditional POI name extraction and generation
	l.sendEvent(ctx, eventCh, types.StreamEvent{
		Type: types.EventTypeProgress,
		Data: map[string]interface{}{"status": "extracting_poi_name"},
	}, 3)

	poiName := extractPOIName(message)
	if poiName == "" {
		return "I'd be happy to add a POI to your itinerary! Could you please specify which place you'd like to add?", nil
	}

	// Check if already exists
	for _, p := range session.CurrentItinerary.AIItineraryResponse.PointsOfInterest {
		if strings.EqualFold(p.Name, poiName) {
			return fmt.Sprintf("%s is already in your itinerary.", poiName), nil
		}
	}

	// Generate new POI data with streaming updates
	l.sendEvent(ctx, eventCh, types.StreamEvent{
		Type: types.EventTypeProgress,
		Data: map[string]interface{}{
			"status":   "generating_poi_data",
			"poi_name": poiName,
		},
	}, 3)

	newPOI, err := l.generatePOIDataStream(ctx, poiName, session.SessionContext.CityName, userLocation, session.UserID, cityID, eventCh)
	if err != nil {
		l.logger.ErrorContext(ctx, "Failed to generate POI data for streaming", slog.Any("error", err))
		span.RecordError(err)
		return "", fmt.Errorf("failed to generate POI data: %w", err)
	}

	session.CurrentItinerary.AIItineraryResponse.PointsOfInterest = append(
		session.CurrentItinerary.AIItineraryResponse.PointsOfInterest, newPOI)

	l.sendEvent(ctx, eventCh, types.StreamEvent{
		Type: "poi_added_successfully",
		Data: map[string]interface{}{
			"poi_name":       newPOI.Name,
			"poi_category":   newPOI.Category,
			"semantic_match": false,
		},
	}, 3)

	return fmt.Sprintf("I've added %s to your itinerary.", poiName), nil
}

// ProcessUnifiedChatMessageStream handles unified chat with optimized streaming based on Google GenAI patterns
func (l *ServiceImpl) ProcessUnifiedChatMessageStream(ctx context.Context, userID, profileID uuid.UUID, cityName, message string, userLocation *types.UserLocation, eventCh chan<- types.StreamEvent) error {
	startTime := time.Now() // Track when processing starts
	ctx, span := otel.Tracer("LlmInteractionService").Start(ctx, "ProcessUnifiedChatMessageStream", trace.WithAttributes(
		attribute.String("message", message),
	))
	defer span.End()

	// Extract city and clean message
	extractedCity, cleanedMessage, err := l.extractCityFromMessage(ctx, message)
	if err != nil {
		span.RecordError(err)
		l.sendEvent(ctx, eventCh, types.StreamEvent{Type: types.EventTypeError, Error: err.Error()}, 3)
		return fmt.Errorf("failed to parse message: %w", err)
	}
	if extractedCity != "" {
		cityName = extractedCity
	}
	span.SetAttributes(attribute.String("extracted.city", cityName), attribute.String("cleaned.message", cleanedMessage))

	// Detect domain
	domainDetector := &types.DomainDetector{}
	domain := domainDetector.DetectDomain(ctx, cleanedMessage)
	span.SetAttributes(attribute.String("detected.domain", string(domain)))

	// Step 3: Fetch user data
	_, searchProfile, _, err := l.FetchUserData(ctx, userID, profileID)
	if err != nil {
		span.RecordError(err)
		l.sendEvent(ctx, eventCh, types.StreamEvent{Type: types.EventTypeError, Error: err.Error()}, 3)
		return fmt.Errorf("failed to fetch user data: %w", err)
	}
	basePreferences := getUserPreferencesPrompt(searchProfile)

	// Use default location if not provided
	var lat, lon float64
	if userLocation == nil && searchProfile.UserLatitude != nil && searchProfile.UserLongitude != nil {
		userLocation = &types.UserLocation{
			UserLat: *searchProfile.UserLatitude,
			UserLon: *searchProfile.UserLongitude,
		}
	}
	if userLocation != nil {
		lat, lon = userLocation.UserLat, userLocation.UserLon
	}

	// Step 4: Cache Integration - Generate cache key based on session parameters
	sessionID := uuid.New()

	// Initialize session
	session := types.ChatSession{
		ID:        sessionID,
		UserID:    userID,
		ProfileID: profileID,
		CityName:  cityName,
		ConversationHistory: []types.ConversationMessage{
			{Role: "user", Content: message, Timestamp: time.Now()},
		},
		SessionContext: types.SessionContext{
			CityName:            cityName,
			ConversationSummary: fmt.Sprintf("Trip plan for %s", cityName),
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		ExpiresAt: time.Now().Add(24 * time.Hour),
		Status:    "active",
	}
	if err := l.llmInteractionRepo.CreateSession(ctx, session); err != nil {
		span.RecordError(err)
		l.sendEvent(ctx, eventCh, types.StreamEvent{Type: types.EventTypeError, Error: err.Error()}, 3)
		return fmt.Errorf("failed to create session: %w", err)
	}

	// Generate cache key based on session parameters
	cacheKeyData := map[string]interface{}{
		"user_id":     userID.String(),
		"profile_id":  profileID.String(),
		"city":        cityName,
		"message":     cleanedMessage,
		"domain":      string(domain),
		"preferences": basePreferences,
	}
	cacheKeyBytes, err := json.Marshal(cacheKeyData)
	if err != nil {
		l.logger.ErrorContext(ctx, "Failed to marshal cache key data", slog.Any("error", err))
		// Use a fallback cache key
		cacheKeyBytes = []byte(fmt.Sprintf("fallback_%s_%s", cleanedMessage, cityName))
	}
	hash := md5.Sum(cacheKeyBytes)
	cacheKey := hex.EncodeToString(hash[:])

	// Step 5: Fan-in Fan-out Setup
	var wg sync.WaitGroup
	var closeOnce sync.Once

	l.sendEvent(ctx, eventCh, types.StreamEvent{
		Type: types.EventTypeStart,
		Data: map[string]interface{}{
			"domain":     string(domain),
			"city":       cityName,
			"session_id": sessionID.String(),
			"cache_key":  cacheKey,
		},
	}, 3)

	// Step 5: Collect responses for saving interaction
	responses := make(map[string]*strings.Builder)
	responsesMutex := sync.Mutex{}

	// Modified sendEventWithResponse to capture responses
	sendEventWithResponse := func(event types.StreamEvent) {
		if event.Type == types.EventTypeChunk {
			responsesMutex.Lock()
			if data, ok := event.Data.(map[string]interface{}); ok {
				if partType, exists := data["part"].(string); exists {
					if chunk, chunkExists := data["chunk"].(string); chunkExists {
						if responses[partType] == nil {
							responses[partType] = &strings.Builder{}
						}
						responses[partType].WriteString(chunk)
					}
				}
			}
			responsesMutex.Unlock()
		}
		l.sendEvent(ctx, eventCh, event, 3)
	}

	// Step 6: Spawn streaming workers based on domain with cache support
	switch domain {
	case types.DomainItinerary, types.DomainGeneral:
		wg.Add(3)

		// Worker 1: Stream City Data with cache
		go func() {
			defer wg.Done()
			prompt := getCityDataPrompt(cityName)
			partCacheKey := cacheKey + "_city_data"
			l.streamWorkerWithResponseAndCache(ctx, prompt, "city_data", sendEventWithResponse, domain, partCacheKey)
		}()

		// Worker 2: Stream General POIs with cache
		go func() {
			defer wg.Done()
			prompt := getGeneralPOIPrompt(cityName)
			partCacheKey := cacheKey + "_general_pois"
			l.streamWorkerWithResponseAndCache(ctx, prompt, "general_pois", sendEventWithResponse, domain, partCacheKey)
		}()

		// Worker 3: Stream Personalized Itinerary with cache
		go func() {
			defer wg.Done()
			prompt := getPersonalizedItineraryPrompt(cityName, basePreferences)
			partCacheKey := cacheKey + "_itinerary"
			l.streamWorkerWithResponseAndCache(ctx, prompt, "itinerary", sendEventWithResponse, domain, partCacheKey)
		}()

	case types.DomainAccommodation:
		wg.Add(1)
		go func() {
			defer wg.Done()
			prompt := getAccommodationPrompt(cityName, lat, lon, basePreferences)
			partCacheKey := cacheKey + "_hotels"
			l.streamWorkerWithResponseAndCache(ctx, prompt, "hotels", sendEventWithResponse, domain, partCacheKey)
		}()

	case types.DomainDining:
		wg.Add(1)
		go func() {
			defer wg.Done()
			prompt := getDiningPrompt(cityName, lat, lon, basePreferences)
			partCacheKey := cacheKey + "_restaurants"
			l.streamWorkerWithResponseAndCache(ctx, prompt, "restaurants", sendEventWithResponse, domain, partCacheKey)
		}()

	case types.DomainActivities:
		wg.Add(1)
		go func() {
			defer wg.Done()
			prompt := getActivitiesPrompt(cityName, lat, lon, basePreferences)
			partCacheKey := cacheKey + "_activities"
			l.streamWorkerWithResponseAndCache(ctx, prompt, "activities", sendEventWithResponse, domain, partCacheKey)
		}()

	default:
		sendEventWithResponse(types.StreamEvent{Type: types.EventTypeError, Error: fmt.Sprintf("unhandled domain: %s", domain)})
		return fmt.Errorf("unhandled domain type: %s", domain)
	}

	// Step 7: Completion goroutine with sync.Once for channel closure
	go func() {
		wg.Wait()             // Wait for all workers to complete
		if ctx.Err() == nil { // Only send completion event if context is still active
			// Determine route type based on domain
			var routeType string
			var baseURL string
			switch domain {
			case types.DomainAccommodation:
				routeType = "hotels"
				baseURL = "/hotels"
			case types.DomainDining:
				routeType = "restaurants"
				baseURL = "/restaurants"
			case types.DomainActivities:
				routeType = "activities"
				baseURL = "/activities"
			default:
				routeType = "itinerary"
				baseURL = "/itinerary"
			}

			l.sendEvent(ctx, eventCh, types.StreamEvent{
				Type: types.EventTypeComplete,
				Data: map[string]interface{}{"session_id": sessionID.String()},
				Navigation: &types.NavigationData{
					URL:       fmt.Sprintf("%s?sessionId=%s&cityName=%s&domain=%s", baseURL, sessionID.String(), url.QueryEscape(cityName), routeType),
					RouteType: routeType,
					QueryParams: map[string]string{
						"sessionId": sessionID.String(),
						"cityName":  cityName,
						"domain":    routeType,
					},
				},
			}, 3)
		}
		closeOnce.Do(func() {
			close(eventCh) // Close the channel only once
			l.logger.InfoContext(ctx, "Event channel closed by completion goroutine")
		})
	}()

	go func() {
		//wg.Wait() // Wait for all workers to complete
		asyncCtx := context.Background()

		var fullResponseBuilder strings.Builder
		responsesMutex.Lock()
		cityDataContent := ""
		if responses["city_data"] != nil {
			cityDataContent = responses["city_data"].String()
		}
		for partType, builder := range responses {
			if builder != nil && builder.Len() > 0 {
				fullResponseBuilder.WriteString(fmt.Sprintf("[%s]\n%s\n\n", partType, builder.String()))
			}
		}
		responsesMutex.Unlock()

		fullResponse := fullResponseBuilder.String()
		if fullResponse == "" {
			fullResponse = fmt.Sprintf("Processed %s request for %s", domain, cityName)
		}

		// Process and save city data if available
		var cityID uuid.UUID
		if cityDataContent != "" {
			// Parse city data from the response
			if parsedCityData, parseErr := l.parseCityDataFromResponse(asyncCtx, cityDataContent); parseErr == nil && parsedCityData != nil {
				// Save city data to the cities table
				if savedCityID, handleErr := l.HandleCityData(asyncCtx, *parsedCityData); handleErr != nil {
					l.logger.WarnContext(asyncCtx, "Failed to save city data during unified stream processing",
						slog.String("city", cityName), slog.Any("error", handleErr))
				} else {
					l.logger.InfoContext(asyncCtx, "Successfully saved city data during unified stream processing",
						slog.String("city", cityName))
					cityID = savedCityID
				}
			} else if parseErr != nil {
				l.logger.WarnContext(asyncCtx, "Failed to parse city data from unified stream response",
					slog.String("city", cityName), slog.Any("error", parseErr))
			}
		}

		// If we don't have a cityID from the response, try to get it from the database
		if cityID == uuid.Nil {
			if existingCity, err := l.cityRepo.FindCityByNameAndCountry(asyncCtx, cityName, ""); err == nil && existingCity != nil {
				cityID = existingCity.ID
			} else {
				l.logger.WarnContext(asyncCtx, "Could not find or save city data, skipping POI processing",
					slog.String("city", cityName))
				return
			}
		}

		// Create and save interaction first to get proper llmInteractionID
		interaction := types.LlmInteraction{
			ID:           uuid.New(),
			SessionID:    sessionID,
			UserID:       userID,
			ProfileID:    profileID,
			CityName:     cityName,
			Prompt:       fmt.Sprintf("Unified Chat Stream - Domain: %s, Message: %s", domain, cleanedMessage),
			ResponseText: fullResponse,
			ModelUsed:    model,
			LatencyMs:    int(time.Since(startTime).Milliseconds()),
			Timestamp:    startTime,
		}
		savedInteractionID, err := l.llmInteractionRepo.SaveInteraction(asyncCtx, interaction)
		if err != nil {
			l.logger.ErrorContext(asyncCtx, "Failed to save stream interaction", slog.Any("error", err))
			return
		}

		l.logger.InfoContext(asyncCtx, "Stream interaction saved successfully",
			slog.String("saved_interaction_id", savedInteractionID.String()),
			slog.String("original_session_id", sessionID.String()))

		// Always try to process and save POI data regardless of domain
		// since responses may contain POI data in different formats
		l.ProcessAndSaveUnifiedResponse(asyncCtx, responses, userID, profileID, cityID, savedInteractionID, userLocation)
	}()

	span.SetStatus(codes.Ok, "Unified chat stream processed successfully")
	return nil
}

func (l *ServiceImpl) ProcessUnifiedChatMessageStreamFree(ctx context.Context, cityName, message string, userLocation *types.UserLocation, eventCh chan<- types.StreamEvent) error {
	startTime := time.Now() // Track when processing starts
	ctx, span := otel.Tracer("LlmInteractionService").Start(ctx, "ProcessUnifiedChatMessageStream", trace.WithAttributes(
		attribute.String("message", message),
	))
	defer span.End()

	// Extract city and clean message
	extractedCity, cleanedMessage, err := l.extractCityFromMessage(ctx, message)
	if err != nil {
		span.RecordError(err)
		l.sendEvent(ctx, eventCh, types.StreamEvent{Type: types.EventTypeError, Error: err.Error()}, 3)
		return fmt.Errorf("failed to parse message: %w", err)
	}
	if extractedCity != "" {
		cityName = extractedCity
	}
	span.SetAttributes(attribute.String("extracted.city", cityName), attribute.String("cleaned.message", cleanedMessage))

	// Detect domain
	domainDetector := &types.DomainDetector{}
	domain := domainDetector.DetectDomain(ctx, cleanedMessage)
	span.SetAttributes(attribute.String("detected.domain", string(domain)))

	// Step 4: Cache Integration - Generate cache key based on session parameters
	sessionID := uuid.New()

	// Initialize session
	session := types.ChatSession{
		ID:       sessionID,
		CityName: cityName,
		ConversationHistory: []types.ConversationMessage{
			{Role: "user", Content: message, Timestamp: time.Now()},
		},
		SessionContext: types.SessionContext{
			CityName:            cityName,
			ConversationSummary: fmt.Sprintf("Trip plan for %s", cityName),
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		ExpiresAt: time.Now().Add(24 * time.Hour),
		Status:    "active",
	}
	if err := l.llmInteractionRepo.CreateSession(ctx, session); err != nil {
		span.RecordError(err)
		l.sendEvent(ctx, eventCh, types.StreamEvent{Type: types.EventTypeError, Error: err.Error()}, 3)
		return fmt.Errorf("failed to create session: %w", err)
	}

	// Generate cache key based on session parameters
	cacheKeyData := map[string]interface{}{
		"city":    cityName,
		"message": cleanedMessage,
		"domain":  string(domain),
	}
	cacheKeyBytes, err := json.Marshal(cacheKeyData)
	if err != nil {
		l.logger.ErrorContext(ctx, "Failed to marshal cache key data", slog.Any("error", err))
		// Use a fallback cache key
		cacheKeyBytes = []byte(fmt.Sprintf("fallback_%s_%s", cleanedMessage, cityName))
	}
	hash := md5.Sum(cacheKeyBytes)
	cacheKey := hex.EncodeToString(hash[:])

	// Step 5: Fan-in Fan-out Setup
	var wg sync.WaitGroup
	var closeOnce sync.Once

	l.sendEvent(ctx, eventCh, types.StreamEvent{
		Type: types.EventTypeStart,
		Data: map[string]interface{}{
			"domain":     string(domain),
			"city":       cityName,
			"session_id": sessionID.String(),
			"cache_key":  cacheKey,
		},
	}, 3)

	// Step 5: Collect responses for saving interaction
	responses := make(map[string]*strings.Builder)
	responsesMutex := sync.Mutex{}

	// Modified sendEventWithResponse to capture responses
	sendEventWithResponse := func(event types.StreamEvent) {
		if event.Type == types.EventTypeChunk {
			responsesMutex.Lock()
			if data, ok := event.Data.(map[string]interface{}); ok {
				if partType, exists := data["part"].(string); exists {
					if chunk, chunkExists := data["chunk"].(string); chunkExists {
						if responses[partType] == nil {
							responses[partType] = &strings.Builder{}
						}
						responses[partType].WriteString(chunk)
					}
				}
			}
			responsesMutex.Unlock()
		}
		l.sendEvent(ctx, eventCh, event, 3)
	}

	// Step 6: Spawn streaming workers based on domain with cache support
	switch domain {
	case types.DomainItinerary, types.DomainGeneral:
		wg.Add(3)

		// Worker 1: Stream City Data with cache
		go func() {
			defer wg.Done()
			prompt := getCityDataPrompt(cityName)
			partCacheKey := cacheKey + "_city_data"
			l.streamWorkerWithResponseAndCache(ctx, prompt, "city_data", sendEventWithResponse, domain, partCacheKey)
		}()

		// Worker 2: Stream General POIs with cache
		go func() {
			defer wg.Done()
			prompt := getGeneralPOIPrompt(cityName)
			partCacheKey := cacheKey + "_general_pois"
			l.streamWorkerWithResponseAndCache(ctx, prompt, "general_pois", sendEventWithResponse, domain, partCacheKey)
		}()

		// Worker 3: Stream Personalized Itinerary with cache
		go func() {
			defer wg.Done()
			prompt := getGeneralizedItineraryPrompt(cityName)
			partCacheKey := cacheKey + "_itinerary"
			l.streamWorkerWithResponseAndCache(ctx, prompt, "itinerary", sendEventWithResponse, domain, partCacheKey)
		}()

	case types.DomainAccommodation:
		wg.Add(1)
		go func() {
			defer wg.Done()
			prompt := getGeneralAccommodationPrompt(cityName)
			partCacheKey := cacheKey + "_hotels"
			l.streamWorkerWithResponseAndCache(ctx, prompt, "hotels", sendEventWithResponse, domain, partCacheKey)
		}()

	case types.DomainDining:
		wg.Add(1)
		go func() {
			defer wg.Done()
			prompt := getGeneralDiningPrompt(cityName)
			partCacheKey := cacheKey + "_restaurants"
			l.streamWorkerWithResponseAndCache(ctx, prompt, "restaurants", sendEventWithResponse, domain, partCacheKey)
		}()

	case types.DomainActivities:
		wg.Add(1)
		go func() {
			defer wg.Done()
			prompt := getGeneralActivitiesPrompt(cityName)
			partCacheKey := cacheKey + "_activities"
			l.streamWorkerWithResponseAndCache(ctx, prompt, "activities", sendEventWithResponse, domain, partCacheKey)
		}()

	default:
		sendEventWithResponse(types.StreamEvent{Type: types.EventTypeError, Error: fmt.Sprintf("unhandled domain: %s", domain)})
		return fmt.Errorf("unhandled domain type: %s", domain)
	}

	// Step 7: Completion goroutine with sync.Once for channel closure
	go func() {
		wg.Wait()             // Wait for all workers to complete
		if ctx.Err() == nil { // Only send completion event if context is still active
			// Determine route type based on domain
			var routeType string
			var baseURL string
			switch domain {
			case types.DomainAccommodation:
				routeType = "hotels"
				baseURL = "/hotels"
			case types.DomainDining:
				routeType = "restaurants"
				baseURL = "/restaurants"
			case types.DomainActivities:
				routeType = "activities"
				baseURL = "/activities"
			default:
				routeType = "itinerary"
				baseURL = "/itinerary"
			}

			l.sendEvent(ctx, eventCh, types.StreamEvent{
				Type: types.EventTypeComplete,
				Data: map[string]interface{}{"session_id": sessionID.String()},
				Navigation: &types.NavigationData{
					URL:       fmt.Sprintf("%s?sessionId=%s&cityName=%s&domain=%s", baseURL, sessionID.String(), url.QueryEscape(cityName), routeType),
					RouteType: routeType,
					QueryParams: map[string]string{
						"sessionId": sessionID.String(),
						"cityName":  cityName,
						"domain":    routeType,
					},
				},
			}, 3)
		}
		closeOnce.Do(func() {
			close(eventCh) // Close the channel only once
			l.logger.InfoContext(ctx, "Event channel closed by completion goroutine")
		})
	}()

	// Step 8: Save interaction and process structured data asynchronously after completion
	go func() {
		wg.Wait() // Wait for all workers to complete

		// Save interaction with complete response
		asyncCtx := context.Background()

		// Combine all responses into a single response text
		var fullResponseBuilder strings.Builder
		responsesMutex.Lock()
		cityDataContent := ""
		if responses["city_data"] != nil {
			cityDataContent = responses["city_data"].String()
		}
		for partType, builder := range responses {
			if builder != nil && builder.Len() > 0 {
				fullResponseBuilder.WriteString(fmt.Sprintf("[%s]\n%s\n\n", partType, builder.String()))
			}
		}
		responsesMutex.Unlock()

		fullResponse := fullResponseBuilder.String()
		if fullResponse == "" {
			fullResponse = fmt.Sprintf("Processed %s request for %s", domain, cityName)
		}

		// Process and save city data if available
		var cityID uuid.UUID
		if cityDataContent != "" {
			// Parse city data from the response
			if parsedCityData, parseErr := l.parseCityDataFromResponse(asyncCtx, cityDataContent); parseErr == nil && parsedCityData != nil {
				// Save city data to the cities table
				if savedCityID, handleErr := l.HandleCityData(asyncCtx, *parsedCityData); handleErr != nil {
					l.logger.WarnContext(asyncCtx, "Failed to save city data during unified stream processing",
						slog.String("city", cityName), slog.Any("error", handleErr))
				} else {
					l.logger.InfoContext(asyncCtx, "Successfully saved city data during unified stream processing",
						slog.String("city", cityName))
					cityID = savedCityID
				}
			} else if parseErr != nil {
				l.logger.WarnContext(asyncCtx, "Failed to parse city data from unified stream response",
					slog.String("city", cityName), slog.Any("error", parseErr))
			}
		}

		// If we don't have a cityID from the response, try to get it from the database
		if cityID == uuid.Nil {
			if existingCity, err := l.cityRepo.FindCityByNameAndCountry(asyncCtx, cityName, ""); err == nil && existingCity != nil {
				cityID = existingCity.ID
			} else {
				l.logger.WarnContext(asyncCtx, "Could not find or save city data, skipping POI processing",
					slog.String("city", cityName))
				return
			}
		}

		// Create and save interaction first to get proper llmInteractionID
		interaction := types.LlmInteraction{
			ID:           uuid.New(),
			SessionID:    sessionID,
			CityName:     cityName,
			Prompt:       fmt.Sprintf("Unified Chat Stream - Domain: %s, Message: %s", domain, cleanedMessage),
			ResponseText: fullResponse,
			ModelUsed:    model,
			LatencyMs:    int(time.Since(startTime).Milliseconds()),
			Timestamp:    startTime,
		}
		savedInteractionID, err := l.llmInteractionRepo.SaveInteraction(asyncCtx, interaction)
		if err != nil {
			l.logger.ErrorContext(asyncCtx, "Failed to save stream interaction", slog.Any("error", err))
			return
		}

		l.logger.InfoContext(asyncCtx, "Stream interaction saved successfully (free)",
			slog.String("saved_interaction_id", savedInteractionID.String()),
			slog.String("original_session_id", sessionID.String()))

		// Always try to process and save POI data regardless of domain
		// since responses may contain POI data in different formats
		l.ProcessAndSaveUnifiedResponseFree(asyncCtx, responses, cityID, savedInteractionID, userLocation)
	}()

	span.SetStatus(codes.Ok, "Unified chat stream processed successfully")
	return nil
}

// ensureItineraryExists initializes the session's CurrentItinerary if it's nil
func (l *ServiceImpl) ensureItineraryExists(session *types.ChatSession) {
	if session.CurrentItinerary == nil {
		session.CurrentItinerary = &types.AiCityResponse{
			AIItineraryResponse: types.AIItineraryResponse{
				ItineraryName:      fmt.Sprintf("Trip to %s", session.SessionContext.CityName),
				OverallDescription: fmt.Sprintf("Exploring %s", session.SessionContext.CityName),
				PointsOfInterest:   []types.POIDetailedInfo{},
			},
		}
	}
	if session.CurrentItinerary.AIItineraryResponse.PointsOfInterest == nil {
		session.CurrentItinerary.AIItineraryResponse.PointsOfInterest = []types.POIDetailedInfo{}
	}
}

// parseCityDataFromResponse extracts and parses city data from streamed response content
func (l *ServiceImpl) parseCityDataFromResponse(_ context.Context, responseContent string) (*types.GeneralCityData, error) {
	// Clean the response by extracting JSON content between ```json and ```
	cleanedResponse := responseContent

	// Look for JSON blocks in the response
	if strings.Contains(responseContent, "```json") {
		start := strings.Index(responseContent, "```json")
		if start != -1 {
			start += len("```json")
			end := strings.Index(responseContent[start:], "```")
			if end != -1 {
				cleanedResponse = strings.TrimSpace(responseContent[start : start+end])
			}
		}
	}

	// Validate JSON
	if !json.Valid([]byte(cleanedResponse)) {
		return nil, fmt.Errorf("invalid JSON in city data response")
	}

	// Try to parse as GeneralCityData
	var generalCity types.GeneralCityData
	if err := json.Unmarshal([]byte(cleanedResponse), &generalCity); err != nil {
		return nil, fmt.Errorf("failed to unmarshal city data: %w", err)
	}

	// Validate that we have minimum required city data
	if generalCity.City == "" {
		return nil, fmt.Errorf("parsed city data is missing city name")
	}

	return &generalCity, nil
}

// streamWorkerWithResponseAndCache handles streaming for a single worker with response capture and cache support
func (l *ServiceImpl) streamWorkerWithResponseAndCache(ctx context.Context, prompt, partType string, sendEvent func(types.StreamEvent), domain types.DomainType, cacheKey string) {
	iter, err := l.aiClient.GenerateContentStreamWithCache(ctx, prompt, &genai.GenerateContentConfig{Temperature: genai.Ptr[float32](defaultTemperature)}, cacheKey)
	if err != nil {
		if ctx.Err() == nil {
			sendEvent(types.StreamEvent{
				Type:  types.EventTypeError,
				Error: fmt.Sprintf("%s worker failed: %v", partType, err),
			})
		}
		return
	}

	var fullResponse strings.Builder
	for resp, err := range iter {
		if ctx.Err() != nil {
			return // Stop if context is canceled
		}
		if err != nil {
			if ctx.Err() == nil {
				sendEvent(types.StreamEvent{
					Type:  types.EventTypeError,
					Error: fmt.Sprintf("%s streaming error: %v", partType, err),
				})
			}
			return
		}
		for _, cand := range resp.Candidates {
			if cand.Content != nil {
				for _, part := range cand.Content.Parts {
					if part.Text != "" {
						chunk := string(part.Text)
						fullResponse.WriteString(chunk)
						sendEvent(types.StreamEvent{
							Type: types.EventTypeChunk,
							Data: map[string]interface{}{
								"part":       partType,
								"chunk":      chunk,
								"domain":     string(domain),
								"cache_key":  cacheKey,
								"cache_used": cacheKey != "",
							},
						})
					}
				}
			}
		}
	}
}
