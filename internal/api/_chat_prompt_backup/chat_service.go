package llmChatBackup

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/patrickmn/go-cache"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/genai"

	"github.com/FACorreiaa/go-poi-au-suggestions/internal/api/city"
	"github.com/FACorreiaa/go-poi-au-suggestions/internal/api/interests"
	"github.com/FACorreiaa/go-poi-au-suggestions/internal/api/poi"
	"github.com/FACorreiaa/go-poi-au-suggestions/internal/api/profiles"
	"github.com/FACorreiaa/go-poi-au-suggestions/internal/api/tags"
	"github.com/FACorreiaa/go-poi-au-suggestions/internal/types"
)

const (
	model              = "gemini-2.0-flash"
	defaultTemperature = 0.5
)

type ChatSession struct {
	History []genai.Chat
}

// Mutex for thread-safe access

// Ensure implementation satisfies the interface
var _ LlmInteractiontService = (*ServiceImpl)(nil)

// LlmInteractiontService defines the business logic contract for user operations.
type LlmInteractiontService interface {
	GetIteneraryResponse(ctx context.Context, cityName string, userID, profileID uuid.UUID, userLocation *types.UserLocation) (*types.AiCityResponse, error)
	SaveItenerary(ctx context.Context, userID uuid.UUID, req types.BookmarkRequest) (uuid.UUID, error)
	RemoveItenerary(ctx context.Context, userID, itineraryID uuid.UUID) error
	GetPOIDetailedInfosResponse(ctx context.Context, userID uuid.UUID, city string, lat, lon float64) (*types.POIDetailedInfo, error)

	// hotels
	GetHotelsByPreferenceResponse(ctx context.Context, userID uuid.UUID, city string, lat, lon float64, userPreferences types.HotelUserPreferences) ([]types.HotelDetailedInfo, error)
	GetHotelsNearbyResponse(ctx context.Context, userID uuid.UUID, city string, userLocation *types.UserLocation) ([]types.HotelDetailedInfo, error)
	GetHotelByIDResponse(ctx context.Context, hotelID uuid.UUID) (*types.HotelDetailedInfo, error)

	// restaurants
	GetRestaurantsByPreferencesResponse(ctx context.Context, userID uuid.UUID, city string, lat, lon float64, preferences types.RestaurantUserPreferences) ([]types.RestaurantDetailedInfo, error)
	GetRestaurantsNearbyResponse(ctx context.Context, userID uuid.UUID, city string, userLocation types.UserLocation) ([]types.RestaurantDetailedInfo, error)
	GetRestaurantDetailsResponse(ctx context.Context, restaurantID uuid.UUID) (*types.RestaurantDetailedInfo, error)

	StartNewSessionStreamed(ctx context.Context, userID, profileID uuid.UUID, cityName, message string, userLocation *types.UserLocation) (*types.StreamingResponse, error)
	ContinueSessionStreamed(
		ctx context.Context,
		sessionID uuid.UUID,
		message string,
		userLocation *types.UserLocation, // For distance sorting context
		eventCh chan<- types.StreamEvent, // Channel to send events back
	) error

	ProcessUnifiedChatMessageStream(ctx context.Context, userID, profileID uuid.UUID, cityName, message string, userLocation *types.UserLocation, eventCh chan<- types.StreamEvent) error
	ProcessUnifiedChatMessageStreamFree(ctx context.Context, cityName, message string, userLocation *types.UserLocation, eventCh chan<- types.StreamEvent) error

	// Chat session management
	GetUserChatSessions(ctx context.Context, userID uuid.UUID) ([]types.ChatSession, error)
}

type IntentClassifier interface {
	Classify(ctx context.Context, message string) (types.IntentType, error) // e.g., "start_trip", "modify_itinerary"
}

// ServiceImpl provides the implementation for LlmInteractiontService.
type ServiceImpl struct {
	logger             *slog.Logger
	interestRepo       interests.Repository
	searchProfileRepo  profiles.Repository
	searchProfileSvc   profiles.Service // Add service for enhanced methods
	tagsRepo           tags.Repository
	aiClient           *generativeAI.AIClient
	embeddingService   *generativeAI.EmbeddingService
	ragService         *generativeAI.RAGService
	llmInteractionRepo Repository
	cityRepo           city.Repository
	poiRepo            poi.Repository
	cache              *cache.Cache

	// events
	deadLetterCh     chan types.StreamEvent
	intentClassifier IntentClassifier
}

// NewLlmInteractiontService creates a new user service instance.
func NewLlmInteractiontService(interestRepo interests.Repository,
	searchProfileRepo profiles.Repository,
	searchProfileSvc profiles.Service,
	tagsRepo tags.Repository,
	llmInteractionRepo Repository,
	cityRepo city.Repository,
	poiRepo poi.Repository,
	logger *slog.Logger) *ServiceImpl {
	ctx := context.Background()
	aiClient, _ := generativeAI.NewAIClient(ctx)

	// Initialize embedding service
	embeddingService, err := generativeAI.NewEmbeddingService(ctx, logger)
	if err != nil {
		log.Fatalf("Failed to create embedding service: %v", err) // Terminate if initialization fails
	}

	// Initialize RAG service
	ragService, err := generativeAI.NewRAGService(ctx, logger)
	if err != nil {
		log.Fatalf("Failed to create RAG service: %v", err) // Terminate if initialization fails
	}

	cache := cache.New(24*time.Hour, 1*time.Hour) // Cache for 24 hours with cleanup every hour
	service := &ServiceImpl{
		logger:             logger,
		tagsRepo:           tagsRepo,
		interestRepo:       interestRepo,
		searchProfileRepo:  searchProfileRepo,
		searchProfileSvc:   searchProfileSvc,
		aiClient:           aiClient,
		embeddingService:   embeddingService,
		ragService:         ragService,
		llmInteractionRepo: llmInteractionRepo,
		cityRepo:           cityRepo,
		poiRepo:            poiRepo,
		cache:              cache,
		deadLetterCh:       make(chan types.StreamEvent, 100),
		intentClassifier:   &types.SimpleIntentClassifier{},
	}
	go service.processDeadLetterQueue()
	return service
}

func (l *ServiceImpl) GenerateCityDataWorker(wg *sync.WaitGroup,
	ctx context.Context,
	cityName string,
	resultCh chan<- types.GenAIResponse,
	config *genai.GenerateContentConfig) {
	go func() {
		ctx, span := otel.Tracer("LlmInteractionService").Start(ctx, "GenerateCityDataWorker", trace.WithAttributes(
			attribute.String("city.name", cityName),
		))
		defer span.End()
		defer wg.Done()

		prompt := getCityDescriptionPrompt(cityName)
		span.SetAttributes(attribute.Int("prompt.length", len(prompt)))

		response, err := l.aiClient.GenerateResponse(ctx, prompt, config)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, "Failed to generate city data")
			resultCh <- types.GenAIResponse{Err: fmt.Errorf("failed to generate city data: %w", err)}
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
			err := fmt.Errorf("no valid city data content from AI")
			span.RecordError(err)
			span.SetStatus(codes.Error, "Empty response from AI")
			resultCh <- types.GenAIResponse{Err: err}
			return
		}
		span.SetAttributes(attribute.Int("response.length", len(txt)))

		cleanTxt := cleanJSONResponse(txt)
		var cityDataFromAI struct {
			CityName        string  `json:"city_name"`
			StateProvince   *string `json:"state_province"` // Use pointer for nullable string
			Country         string  `json:"country"`
			CenterLatitude  float64 `json:"center_latitude"`
			CenterLongitude float64 `json:"center_longitude"`
			Description     string  `json:"description"`
			// BoundingBox     string  `json:"bounding_box,omitempty"` // If trying to get BBox string
		}
		if err := json.Unmarshal([]byte(cleanTxt), &cityDataFromAI); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, "Failed to parse city data JSON")
			resultCh <- types.GenAIResponse{Err: fmt.Errorf("failed to parse city data JSON: %w", err)}
			return
		}

		stateProvinceValue := ""
		if cityDataFromAI.StateProvince != nil {
			stateProvinceValue = *cityDataFromAI.StateProvince
		}

		span.SetAttributes(
			attribute.String("city.name", cityDataFromAI.CityName),
			attribute.String("city.country", cityDataFromAI.Country),
			attribute.Float64("city.latitude", cityDataFromAI.CenterLatitude),
			attribute.Float64("city.longitude", cityDataFromAI.CenterLongitude),
		)
		span.SetStatus(codes.Ok, "City data generated successfully")

		resultCh <- types.GenAIResponse{
			City:            cityDataFromAI.CityName,
			Country:         cityDataFromAI.Country,
			StateProvince:   stateProvinceValue,
			CityDescription: cityDataFromAI.Description,
			Latitude:        cityDataFromAI.CenterLatitude,
			Longitude:       cityDataFromAI.CenterLongitude,
			// BoundingBoxWKT: cityDataFromAI.BoundingBox, // TODO
		}
	}()
}

func (l *ServiceImpl) GenerateGeneralPOIWorker(wg *sync.WaitGroup,
	ctx context.Context,
	cityName string,
	resultCh chan<- types.GenAIResponse,
	config *genai.GenerateContentConfig) {
	ctx, span := otel.Tracer("LlmInteractionService").Start(ctx, "GenerateGeneralPOIWorker", trace.WithAttributes(
		attribute.String("city.name", cityName),
	))
	defer span.End()
	defer wg.Done()

	prompt := getGeneralPOIPrompt(cityName)
	span.SetAttributes(attribute.Int("prompt.length", len(prompt)))

	startTime := time.Now()
	response, err := l.aiClient.GenerateResponse(ctx, prompt, config)
	latencyMs := int(time.Since(startTime).Milliseconds())
	span.SetAttributes(attribute.Int("response.latency_ms", latencyMs))

	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to generate general POIs")
		resultCh <- types.GenAIResponse{Err: fmt.Errorf("failed to generate general POIs: %w", err)}
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
		err := fmt.Errorf("no valid general POI content from AI")
		span.RecordError(err)
		span.SetStatus(codes.Error, "Empty response from AI")
		resultCh <- types.GenAIResponse{Err: err}
		return
	}
	span.SetAttributes(attribute.Int("response.length", len(txt)))

	cleanTxt := cleanJSONResponse(txt)
	var poiData struct {
		PointsOfInterest []types.POIDetailedInfo `json:"points_of_interest"`
	}
	if err := json.Unmarshal([]byte(cleanTxt), &poiData); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to parse general POI JSON")
		resultCh <- types.GenAIResponse{Err: fmt.Errorf("failed to parse general POI JSON: %w", err)}
		return
	}

	span.SetAttributes(attribute.Int("pois.count", len(poiData.PointsOfInterest)))
	span.SetStatus(codes.Ok, "General POIs generated successfully")
	resultCh <- types.GenAIResponse{GeneralPOI: poiData.PointsOfInterest}
}

func (l *ServiceImpl) GeneratePersonalisedPOIWorker(wg *sync.WaitGroup, ctx context.Context,
	cityName string, userID, profileID, sessionID uuid.UUID, resultCh chan<- types.GenAIResponse,
	interestNames []string, tagsPromptPart string, userPrefs string,
	config *genai.GenerateContentConfig) {
	ctx, span := otel.Tracer("LlmInteractionService").Start(ctx, "GeneratePersonalisedPOIWorker", trace.WithAttributes(
		attribute.String("city.name", cityName),
		attribute.String("user.id", userID.String()),
		attribute.String("profile.id", profileID.String()),
		attribute.Int("interests.count", len(interestNames)),
	))
	defer span.End()
	defer wg.Done()

	startTime := time.Now()

	prompt := getPersonalizedPOI(interestNames, cityName, tagsPromptPart, userPrefs)
	span.SetAttributes(attribute.Int("prompt.length", len(prompt)))

	response, err := l.aiClient.GenerateResponse(ctx, prompt, config)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to generate personalized itinerary")
		resultCh <- types.GenAIResponse{Err: fmt.Errorf("failed to generate personalized itinerary: %w", err)}
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
		err := fmt.Errorf("no valid personalized itinerary content from AI")
		span.RecordError(err)
		span.SetStatus(codes.Error, "Empty response from AI")
		resultCh <- types.GenAIResponse{Err: err}
		return
	}
	span.SetAttributes(attribute.Int("response.length", len(txt)))

	cleanTxt := cleanJSONResponse(txt)
	var itineraryData struct {
		ItineraryName      string                  `json:"itinerary_name"`
		OverallDescription string                  `json:"overall_description"`
		PointsOfInterest   []types.POIDetailedInfo `json:"points_of_interest"`
	}

	if err := json.Unmarshal([]byte(cleanTxt), &itineraryData); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to parse personalized itinerary JSON")
		resultCh <- types.GenAIResponse{Err: fmt.Errorf("failed to parse personalized itinerary JSON: %w", err)}
		return
	}
	span.SetAttributes(
		attribute.String("itinerary.name", itineraryData.ItineraryName),
		attribute.Int("personalized_pois.count", len(itineraryData.PointsOfInterest)),
	)

	latencyMs := int(time.Since(startTime).Milliseconds())
	span.SetAttributes(attribute.Int("response.latency_ms", latencyMs))

	interaction := types.LlmInteraction{
		UserID:       userID,
		SessionID:    sessionID,
		Prompt:       prompt,
		ResponseText: txt,
		ModelUsed:    model, // Adjust based on your AI client
		LatencyMs:    latencyMs,
		CityName:     cityName,
		// request payload
		// response payload
		// Add token counts if available from response (depends on genai API)
		// PromptTokens, CompletionTokens, TotalTokens
		// RequestPayload, ResponsePayload if you serialize the full request/response
	}
	savedInteractionID, err := l.llmInteractionRepo.SaveInteraction(ctx, interaction)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to save LLM interaction")
		resultCh <- types.GenAIResponse{Err: fmt.Errorf("failed to save LLM interaction: %w", err)}
		return
	}
	span.SetAttributes(attribute.String("llm_interaction.id", savedInteractionID.String()))
	span.SetStatus(codes.Ok, "Personalized POIs generated successfully")

	resultCh <- types.GenAIResponse{
		ItineraryName:        itineraryData.ItineraryName,
		ItineraryDescription: itineraryData.OverallDescription,
		PersonalisedPOI:      itineraryData.PointsOfInterest,
		LlmInteractionID:     savedInteractionID,
	}
}

// GeneratePersonalisedPOIWorkerWithSemantics generates personalized POIs with semantic search enhancement
func (l *ServiceImpl) GeneratePersonalisedPOIWorkerWithSemantics(wg *sync.WaitGroup, ctx context.Context,
	cityName string, userID, profileID, sessionID uuid.UUID, resultCh chan<- types.GenAIResponse,
	interestNames []string, tagsPromptPart string, userPrefs string, semanticPOIs []types.POIDetailedInfo,
	config *genai.GenerateContentConfig) {
	ctx, span := otel.Tracer("LlmInteractionService").Start(ctx, "GeneratePersonalisedPOIWorkerWithSemantics", trace.WithAttributes(
		attribute.String("city.name", cityName),
		attribute.String("user.id", userID.String()),
		attribute.String("profile.id", profileID.String()),
		attribute.Int("interests.count", len(interestNames)),
		attribute.Int("semantic_pois.count", len(semanticPOIs)),
	))
	defer span.End()
	defer wg.Done()

	startTime := time.Now()

	// Create enhanced prompt with semantic context
	prompt := l.getPersonalizedPOIWithSemanticContext(interestNames, cityName, tagsPromptPart, userPrefs, semanticPOIs)
	span.SetAttributes(attribute.Int("prompt.length", len(prompt)))

	response, err := l.aiClient.GenerateResponse(ctx, prompt, config)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to generate semantic-enhanced personalized itinerary")
		resultCh <- types.GenAIResponse{Err: fmt.Errorf("failed to generate semantic-enhanced personalized itinerary: %w", err)}
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
		err := fmt.Errorf("no valid semantic-enhanced personalized itinerary content from AI")
		span.RecordError(err)
		span.SetStatus(codes.Error, "Empty response from AI")
		resultCh <- types.GenAIResponse{Err: err}
		return
	}
	span.SetAttributes(attribute.Int("response.length", len(txt)))

	cleanTxt := cleanJSONResponse(txt)
	var itineraryData struct {
		ItineraryName      string                  `json:"itinerary_name"`
		OverallDescription string                  `json:"overall_description"`
		PointsOfInterest   []types.POIDetailedInfo `json:"points_of_interest"`
	}

	if err := json.Unmarshal([]byte(cleanTxt), &itineraryData); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to parse semantic-enhanced personalized itinerary JSON")
		resultCh <- types.GenAIResponse{Err: fmt.Errorf("failed to parse semantic-enhanced personalized itinerary JSON: %w", err)}
		return
	}
	span.SetAttributes(
		attribute.String("itinerary.name", itineraryData.ItineraryName),
		attribute.Int("personalized_pois.count", len(itineraryData.PointsOfInterest)),
	)

	latencyMs := int(time.Since(startTime).Milliseconds())
	span.SetAttributes(attribute.Int("response.latency_ms", latencyMs))

	interaction := types.LlmInteraction{
		UserID:       userID,
		SessionID:    sessionID,
		Prompt:       prompt,
		ResponseText: txt,
		ModelUsed:    model,
		LatencyMs:    latencyMs,
		CityName:     cityName,
	}
	savedInteractionID, err := l.llmInteractionRepo.SaveInteraction(ctx, interaction)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to save semantic-enhanced LLM interaction")
		resultCh <- types.GenAIResponse{Err: fmt.Errorf("failed to save semantic-enhanced LLM interaction: %w", err)}
		return
	}
	span.SetAttributes(attribute.String("llm_interaction.id", savedInteractionID.String()))
	span.SetStatus(codes.Ok, "Semantic-enhanced personalized POIs generated successfully")

	resultCh <- types.GenAIResponse{
		ItineraryName:        itineraryData.ItineraryName,
		ItineraryDescription: itineraryData.OverallDescription,
		PersonalisedPOI:      itineraryData.PointsOfInterest,
		LlmInteractionID:     savedInteractionID,
	}
}

// getPersonalizedPOIWithSemanticContext creates an enhanced prompt with semantic POI context
func (l *ServiceImpl) getPersonalizedPOIWithSemanticContext(interestNames []string, cityName, tagsPromptPart, userPrefs string, semanticPOIs []types.POIDetailedInfo) string {
	prompt := fmt.Sprintf(`
        Generate a personalized trip itinerary for %s, tailored to user interests [%s].

        **SEMANTIC CONTEXT - Consider these highly relevant POIs found via semantic search:**
        `, cityName, strings.Join(interestNames, ", "))

	// Add semantic POI context
	if len(semanticPOIs) > 0 {
		prompt += "\n**Contextually Relevant POIs:**\n"
		for i, poi := range semanticPOIs {
			if i >= 10 { // Limit context to avoid token overuse
				break
			}
			prompt += fmt.Sprintf("- %s (%s): %s [Lat: %.6f, Lon: %.6f]\n",
				poi.Name, poi.Category, poi.DescriptionPOI, poi.Latitude, poi.Longitude)
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

// FetchEnhancedUserData fetches user data including domain-specific preferences
// func (l *ServiceImpl) FetchEnhancedUserData(ctx context.Context, userID, profileID uuid.UUID, domain types.DomainType) (*types.CombinedFilters, error) {
// 	ctx, span := otel.Tracer("LlmInteractionService").Start(ctx, "FetchEnhancedUserData", trace.WithAttributes(
// 		attribute.String("profile.id", profileID.String()),
// 		attribute.String("domain", string(domain)),
// 	))
// 	defer span.End()

// 	l.logger.DebugContext(ctx, "Fetching enhanced user data", slog.String("profileID", profileID.String()), slog.String("domain", string(domain)))

// 	// Get combined filters through the profile service
// 	combinedFilters, err := l.searchProfileSvc.GetCombinedFilters(ctx, userID, profileID, domain)
// 	if err != nil {
// 		l.logger.ErrorContext(ctx, "Failed to fetch combined filters", slog.Any("error", err))
// 		span.RecordError(err)
// 		span.SetStatus(codes.Error, "Failed to fetch combined filters")
// 		return nil, fmt.Errorf("failed to fetch enhanced user data: %w", err)
// 	}

// 	l.logger.InfoContext(ctx, "Enhanced user data fetched successfully")
// 	span.SetStatus(codes.Ok, "Enhanced user data fetched successfully")
// 	return combinedFilters, nil
// }

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

// RAG-enhanced methods for improved responses using semantic search

// SearchRelevantPOIsForRAG searches for POIs semantically similar to the user's query
// func (l *ServiceImpl) SearchRelevantPOIsForRAG(ctx context.Context, query string, cityID *uuid.UUID, limit int) ([]types.POIDetailedInfo, error) {
// 	ctx, span := otel.Tracer("LlmInteractionService").Start(ctx, "SearchRelevantPOIsForRAG", trace.WithAttributes(
// 		attribute.String("query", query),
// 		attribute.Int("limit", limit),
// 	))
// 	defer span.End()

// 	// Generate embedding for the query
// 	queryEmbedding, err := l.embeddingService.GenerateQueryEmbedding(ctx, query)
// 	if err != nil {
// 		span.RecordError(err)
// 		span.SetStatus(codes.Error, "Failed to generate query embedding")
// 		l.logger.ErrorContext(ctx, "Failed to generate query embedding", slog.Any("error", err))
// 		return nil, fmt.Errorf("failed to generate query embedding: %w", err)
// 	}

// 	// Search for similar POIs
// 	var relevantPOIs []types.POIDetailedInfo
// 	if cityID != nil {
// 		// City-specific search
// 		relevantPOIs, err = l.poiRepo.FindSimilarPOIsByCity(ctx, queryEmbedding, *cityID, limit)
// 	} else {
// 		// Global search
// 		relevantPOIs, err = l.poiRepo.FindSimilarPOIs(ctx, queryEmbedding, limit)
// 	}

// 	if err != nil {
// 		span.RecordError(err)
// 		span.SetStatus(codes.Error, "Failed to search similar POIs")
// 		l.logger.ErrorContext(ctx, "Failed to search similar POIs", slog.Any("error", err))
// 		return nil, fmt.Errorf("failed to search similar POIs: %w", err)
// 	}

// 	span.SetAttributes(
// 		attribute.Int("relevant_pois.count", len(relevantPOIs)),
// 		attribute.Int("embedding.dimension", len(queryEmbedding)),
// 	)
// 	span.SetStatus(codes.Ok, "Relevant POIs found for RAG")

// 	l.logger.InfoContext(ctx, "Found relevant POIs for RAG",
// 		slog.Int("count", len(relevantPOIs)),
// 		slog.String("query", query))

// 	return relevantPOIs, nil
// }

// GenerateRAGResponse generates a response using retrieved POI context
// func (l *ServiceImpl) GenerateRAGResponse(ctx context.Context, query string, userID, profileID uuid.UUID, cityContext string, conversationHistory []generativeAI.ConversationTurn) (*generativeAI.RAGResponse, error) {
// 	ctx, span := otel.Tracer("LlmInteractionService").Start(ctx, "GenerateRAGResponse", trace.WithAttributes(
// 		attribute.String("query", query),
// 		attribute.String("user.id", userID.String()),
// 		attribute.String("profile.id", profileID.String()),
// 	))
// 	defer span.End()

// 	// Fetch user data for context
// 	interests, searchProfile, tags, err := l.FetchUserData(ctx, userID, profileID)
// 	if err != nil {
// 		span.RecordError(err)
// 		span.SetStatus(codes.Error, "Failed to fetch user data")
// 		return nil, fmt.Errorf("failed to fetch user data: %w", err)
// 	}

// 	// Prepare user preferences context
// 	userPreferences := make(map[string]interface{})
// 	if searchProfile != nil {
// 		userPreferences["search_radius"] = searchProfile.SearchRadiusKm
// 		//userPreferences["travel_pace"] = searchProfile.TravelPace
// 		userPreferences["preferred_transport"] = searchProfile.PreferredTransport
// 		userPreferences["budget_level"] = searchProfile.BudgetLevel
// 		//userPreferences["group_size"] = searchProfile.GroupSize
// 		//userPreferences["accessibility_needs"] = searchProfile.AccessibilityNeeds
// 		//userPreferences["privacy_level"] = searchProfile.PrivacyLevel
// 		//userPreferences["preferred_atmosphere"] = searchProfile.PreferredAtmosphere
// 	}

// 	// Add interests to preferences
// 	interestNames := make([]string, 0, len(interests))
// 	for _, interest := range interests {
// 		if interest != nil {
// 			interestNames = append(interestNames, interest.Name)
// 		}
// 	}
// 	userPreferences["interests"] = interestNames

// 	// Add tags to preferences
// 	tagNames := make([]string, 0, len(tags))
// 	for _, tag := range tags {
// 		if tag != nil {
// 			tagNames = append(tagNames, tag.Name)
// 		}
// 	}
// 	userPreferences["tags"] = tagNames

// 	// Search for relevant POIs (limit to 5 for context)
// 	relevantPOIs, err := l.SearchRelevantPOIsForRAG(ctx, query, nil, 5)
// 	if err != nil {
// 		l.logger.WarnContext(ctx, "Failed to search relevant POIs, continuing without semantic context", slog.Any("error", err))
// 		relevantPOIs = []types.POIDetailedInfo{} // Continue with empty context
// 	}

// 	// Build RAG context
// 	ragContext := generativeAI.RAGContext{
// 		Query:               query,
// 		RelevantPOIs:        relevantPOIs,
// 		UserPreferences:     userPreferences,
// 		CityContext:         cityContext,
// 		ConversationHistory: conversationHistory,
// 	}

// 	// Generate RAG response
// 	ragResponse, err := l.ragService.GenerateRAGResponse(ctx, ragContext)
// 	if err != nil {
// 		span.RecordError(err)
// 		span.SetStatus(codes.Error, "Failed to generate RAG response")
// 		l.logger.ErrorContext(ctx, "Failed to generate RAG response", slog.Any("error", err))
// 		return nil, fmt.Errorf("failed to generate RAG response: %w", err)
// 	}

// 	span.SetAttributes(
// 		attribute.Float64("response.confidence", ragResponse.Confidence),
// 		attribute.Int("response.suggestions.count", len(ragResponse.Suggestions)),
// 		attribute.Int("source_pois.count", len(ragResponse.SourcePOIs)),
// 	)
// 	span.SetStatus(codes.Ok, "RAG response generated successfully")

// 	l.logger.InfoContext(ctx, "RAG response generated",
// 		slog.Float64("confidence", ragResponse.Confidence),
// 		slog.Int("source_pois", len(ragResponse.SourcePOIs)),
// 		slog.Int("suggestions", len(ragResponse.Suggestions)))

// 	return ragResponse, nil
// }

// // EnhancePersonalizedPOIWithRAG enhances personalized POI generation with semantic context
// func (l *ServiceImpl) EnhancePersonalizedPOIWithRAG(ctx context.Context, cityName string, userID, profileID uuid.UUID, cityID *uuid.UUID) ([]types.POIDetailedInfo, error) {
// 	ctx, span := otel.Tracer("LlmInteractionService").Start(ctx, "EnhancePersonalizedPOIWithRAG", trace.WithAttributes(
// 		attribute.String("city.name", cityName),
// 		attribute.String("user.id", userID.String()),
// 		attribute.String("profile.id", profileID.String()),
// 	))
// 	defer span.End()

// 	// Fetch user data
// 	interests, searchProfile, tags, err := l.FetchUserData(ctx, userID, profileID)
// 	if err != nil {
// 		span.RecordError(err)
// 		span.SetStatus(codes.Error, "Failed to fetch user data")
// 		return nil, fmt.Errorf("failed to fetch user data: %w", err)
// 	}

// 	// Build search query from user interests and preferences
// 	interestNames, tagsPromptPart, userPrefs := l.PreparePromptData(interests, tags, searchProfile)

// 	// Create a search query combining interests and city
// 	searchQuery := fmt.Sprintf("%s in %s %s %s",
// 		strings.Join(interestNames, " "),
// 		cityName,
// 		tagsPromptPart,
// 		userPrefs)

// 	// Search for semantically similar POIs
// 	var relevantPOIs []types.POIDetailedInfo
// 	if cityID != nil {
// 		relevantPOIs, err = l.SearchRelevantPOIsForRAG(ctx, searchQuery, cityID, 10)
// 	} else {
// 		relevantPOIs, err = l.SearchRelevantPOIsForRAG(ctx, searchQuery, nil, 10)
// 	}

// 	if err != nil {
// 		l.logger.WarnContext(ctx, "Failed to search semantically similar POIs", slog.Any("error", err))
// 		// Fall back to regular POI generation if semantic search fails
// 		return nil, err
// 	}

// 	span.SetAttributes(
// 		attribute.Int("semantic_pois.count", len(relevantPOIs)),
// 		attribute.String("search_query", searchQuery[:min(100, len(searchQuery))]),
// 	)
// 	span.SetStatus(codes.Ok, "Enhanced personalized POIs with RAG")

// 	l.logger.InfoContext(ctx, "Enhanced personalized POIs with semantic search",
// 		slog.Int("semantic_pois", len(relevantPOIs)),
// 		slog.String("city", cityName))

// 	return relevantPOIs, nil
// }

// // GetRAGEnabledChatResponse generates a chat response using RAG for better context
// func (l *ServiceImpl) GetRAGEnabledChatResponse(ctx context.Context, message string, userID, profileID uuid.UUID, sessionID uuid.UUID, cityContext string) (*generativeAI.RAGResponse, error) {
// 	ctx, span := otel.Tracer("LlmInteractionService").Start(ctx, "GetRAGEnabledChatResponse", trace.WithAttributes(
// 		attribute.String("message", message[:min(100, len(message))]),
// 		attribute.String("user.id", userID.String()),
// 		attribute.String("session.id", sessionID.String()),
// 	))
// 	defer span.End()

// 	// TODO: Retrieve conversation history from session storage
// 	// For now, we'll use an empty history
// 	conversationHistory := []generativeAI.ConversationTurn{}

// 	// Store the current user message
// 	err := l.ragService.StoreConversationTurn(ctx, userID.String(), "user", message, map[string]interface{}{
// 		"session_id": sessionID.String(),
// 		"profile_id": profileID.String(),
// 	})
// 	if err != nil {
// 		l.logger.WarnContext(ctx, "Failed to store conversation turn", slog.Any("error", err))
// 	}

// 	// Generate RAG response
// 	ragResponse, err := l.GenerateRAGResponse(ctx, message, userID, profileID, cityContext, conversationHistory)
// 	if err != nil {
// 		span.RecordError(err)
// 		span.SetStatus(codes.Error, "Failed to generate RAG chat response")
// 		return nil, fmt.Errorf("failed to generate RAG chat response: %w", err)
// 	}

// 	// Store the assistant response
// 	err = l.ragService.StoreConversationTurn(ctx, userID.String(), "assistant", ragResponse.Answer, map[string]interface{}{
// 		"session_id":  sessionID.String(),
// 		"profile_id":  profileID.String(),
// 		"confidence":  ragResponse.Confidence,
// 		"source_pois": len(ragResponse.SourcePOIs),
// 	})
// 	if err != nil {
// 		l.logger.WarnContext(ctx, "Failed to store assistant response", slog.Any("error", err))
// 	}

// 	span.SetStatus(codes.Ok, "RAG chat response generated")
// 	l.logger.InfoContext(ctx, "RAG chat response generated",
// 		slog.Float64("confidence", ragResponse.Confidence),
// 		slog.Int("source_pois", len(ragResponse.SourcePOIs)))

// 	return ragResponse, nil
// }

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
	city, err := l.cityRepo.FindCityByNameAndCountry(ctx, cityData.City, cityData.Country)
	if err != nil {
		return uuid.Nil, fmt.Errorf("failed to check city existence: %w", err)
	}
	if city == nil {
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
		cityID = city.ID
	}
	return cityID, nil
}

func (l *ServiceImpl) HandleGeneralPOIs(ctx context.Context, pois []types.POIDetailedInfo, cityID uuid.UUID) {
	for _, poi := range pois {
		existingPoi, err := l.poiRepo.FindPoiByNameAndCity(ctx, poi.Name, cityID)
		if err != nil {
			l.logger.WarnContext(ctx, "Failed to check POI existence", slog.String("poi_name", poi.Name), slog.Any("error", err))
			continue
		}
		if existingPoi == nil {
			_, err = l.poiRepo.SavePoi(ctx, poi, cityID)
			if err != nil {
				l.logger.WarnContext(ctx, "Failed to save POI", slog.String("poi_name", poi.Name), slog.Any("error", err))
			}
		}
	}
}

func (l *ServiceImpl) HandlePersonalisedPOIs(ctx context.Context, pois []types.POIDetailedInfo, cityID uuid.UUID, userLocation *types.UserLocation, llmInteractionID uuid.UUID, userID, profileID uuid.UUID) ([]types.POIDetailedInfo, error) {
	if userLocation == nil || cityID == uuid.Nil || len(pois) == 0 {
		return pois, nil // No sorting possible
	}
	err := l.llmInteractionRepo.SaveLlmSuggestedPOIsBatch(ctx, pois, userID, profileID, llmInteractionID, cityID)
	if err != nil {
		return nil, fmt.Errorf("failed to save personalised POIs: %w", err)
	}

	itineraryID, err := l.poiRepo.SaveItinerary(ctx, userID, cityID)
	if err != nil {
		return nil, fmt.Errorf("failed to save itinerary: %w", err)
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

func (l *ServiceImpl) GetIteneraryResponse(ctx context.Context, cityName string, userID, profileID uuid.UUID, userLocation *types.UserLocation) (*types.AiCityResponse, error) {
	ctx, span := otel.Tracer("LlmInteractionService").Start(ctx, "GetIteneraryResponse", trace.WithAttributes(
		attribute.String("city.name", cityName),
		attribute.String("user.id", userID.String()),
		attribute.String("profile.id", profileID.String()),
	))
	defer span.End()

	l.logger.DebugContext(ctx, "Starting itinerary generation", slog.String("cityName", cityName), slog.String("userID", userID.String()), slog.String("profileID", profileID.String()))

	// Fetch user data
	interests, searchProfile, tags, err := l.FetchUserData(ctx, userID, profileID)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to fetch user data")
		return nil, err
	}

	// Prepare prompt data
	interestNames, tagsPromptPart, userPrefs := l.PreparePromptData(interests, tags, searchProfile)
	span.SetAttributes(
		attribute.Int("interests.count", len(interestNames)),
		attribute.Int("tags.count", len(tags)),
	)

	// Determine user location
	if searchProfile.UserLatitude != nil && searchProfile.UserLongitude != nil {
		userLocation = &types.UserLocation{
			UserLat: *searchProfile.UserLatitude,
			UserLon: *searchProfile.UserLongitude,
		}
		span.SetAttributes(
			attribute.Float64("user.latitude", *searchProfile.UserLatitude),
			attribute.Float64("user.longitude", *searchProfile.UserLongitude),
		)
	} else {
		l.logger.WarnContext(ctx, "User location not available, cannot sort personalised POIs by distance")
		span.AddEvent("User location not available")
	}

	// Set up channels and wait group for fan-in fan-out
	resultCh := make(chan types.GenAIResponse, 3)
	var wg sync.WaitGroup
	wg.Add(3)

	// Fan-out: Start workers
	go l.GenerateCityDataWorker(&wg, ctx, cityName, resultCh, &genai.GenerateContentConfig{Temperature: genai.Ptr[float32](defaultTemperature)})
	go l.GenerateGeneralPOIWorker(&wg, ctx, cityName, resultCh, &genai.GenerateContentConfig{Temperature: genai.Ptr[float32](defaultTemperature)})
	go l.GeneratePersonalisedPOIWorker(&wg, ctx, cityName, userID, profileID, uuid.New(), resultCh, interestNames, tagsPromptPart, userPrefs, &genai.GenerateContentConfig{Temperature: genai.Ptr[float32](defaultTemperature)})

	// Close channel after workers complete
	go func() {
		wg.Wait()
		close(resultCh)
	}()

	// Fan-in: Collect results
	itinerary, llmInteractionID, rawPersonalisedPOIs, errors := l.CollectResults(resultCh)
	if len(errors) > 0 {
		l.logger.ErrorContext(ctx, "Errors during itinerary generation", slog.Any("errors", errors))
		for _, err := range errors {
			span.RecordError(err)
		}
		span.SetStatus(codes.Error, "Failed to generate itinerary")
		return nil, fmt.Errorf("failed to generate itinerary: %v", errors)
	}

	// Handle city data
	cityID, err := l.HandleCityData(ctx, itinerary.GeneralCityData)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to handle city data")
		return nil, err
	}
	span.SetAttributes(attribute.String("city.id", cityID.String()))

	// Handle general POIs
	l.HandleGeneralPOIs(ctx, itinerary.PointsOfInterest, cityID)
	span.SetAttributes(attribute.Int("general_pois.count", len(itinerary.PointsOfInterest)))

	// Handle personalized POIs
	sortedPois, err := l.HandlePersonalisedPOIs(ctx, rawPersonalisedPOIs, cityID, userLocation, llmInteractionID, userID, profileID)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to handle personalized POIs")
		return nil, err
	}
	itinerary.AIItineraryResponse.PointsOfInterest = sortedPois
	span.SetAttributes(
		attribute.Int("personalized_pois.count", len(sortedPois)),
		attribute.String("llm_interaction.id", llmInteractionID.String()),
	)

	l.logger.InfoContext(ctx, "Final itinerary ready",
		slog.String("itinerary_name", itinerary.AIItineraryResponse.ItineraryName),
		slog.Int("final_personalised_poi_count", len(itinerary.AIItineraryResponse.PointsOfInterest)))

	span.SetStatus(codes.Ok, "Itinerary generated successfully")
	return &itinerary, nil
}

// GetEnhancedIteneraryResponse generates an itinerary with enhanced domain-specific filtering
// func (l *ServiceImpl) GetEnhancedIteneraryResponse(ctx context.Context, cityName, userMessage string, userID, profileID uuid.UUID, userLocation *types.UserLocation) (*types.AiCityResponse, error) {
// 	ctx, span := otel.Tracer("LlmInteractionService").Start(ctx, "GetEnhancedIteneraryResponse", trace.WithAttributes(
// 		attribute.String("city.name", cityName),
// 		attribute.String("user.id", userID.String()),
// 		attribute.String("profile.id", profileID.String()),
// 		attribute.String("user.message", userMessage),
// 	))
// 	defer span.End()

// 	l.logger.DebugContext(ctx, "Starting enhanced itinerary generation",
// 		slog.String("cityName", cityName),
// 		slog.String("userID", userID.String()),
// 		slog.String("profileID", profileID.String()),
// 		slog.String("userMessage", userMessage))

// 	// Detect domain from user message
// 	domainDetector := &DomainDetector{}
// 	detectedDomain := domainDetector.DetectDomain(ctx, userMessage)
// 	l.logger.DebugContext(ctx, "Detected domain", slog.String("domain", string(detectedDomain)))
// 	span.SetAttributes(attribute.String("detected.domain", string(detectedDomain)))

// 	// Fetch enhanced user data including domain-specific preferences
// 	combinedFilters, err := l.FetchEnhancedUserData(ctx, userID, profileID, detectedDomain)
// 	if err != nil {
// 		span.RecordError(err)
// 		span.SetStatus(codes.Error, "Failed to fetch enhanced user data")
// 		return nil, err
// 	}

// 	// Prepare enhanced prompt data with domain-specific filters
// 	enhancedPromptData := l.PrepareEnhancedPromptData(combinedFilters, detectedDomain)
// 	span.SetAttributes(
// 		attribute.Int("base_interests.count", len(combinedFilters.BasePreferences.Interests)),
// 		attribute.Bool("has_accommodation_prefs", combinedFilters.AccommodationPreferences != nil),
// 		attribute.Bool("has_dining_prefs", combinedFilters.DiningPreferences != nil),
// 		attribute.Bool("has_activity_prefs", combinedFilters.ActivityPreferences != nil),
// 		attribute.Bool("has_itinerary_prefs", combinedFilters.ItineraryPreferences != nil),
// 	)

// 	// Determine user location from profile if not provided
// 	if userLocation == nil && combinedFilters.BasePreferences.UserLatitude != nil && combinedFilters.BasePreferences.UserLongitude != nil {
// 		userLocation = &types.UserLocation{
// 			UserLat: *combinedFilters.BasePreferences.UserLatitude,
// 			UserLon: *combinedFilters.BasePreferences.UserLongitude,
// 		}
// 		span.SetAttributes(
// 			attribute.Float64("user.latitude", *combinedFilters.BasePreferences.UserLatitude),
// 			attribute.Float64("user.longitude", *combinedFilters.BasePreferences.UserLongitude),
// 		)
// 	}

// 	// Set up channels and wait group for fan-in fan-out
// 	resultCh := make(chan types.GenAIResponse, 3)
// 	var wg sync.WaitGroup
// 	wg.Add(3)

// 	// Fan-out: Start workers with enhanced prompts
// 	go l.GenerateCityDataWorker(&wg, ctx, cityName, resultCh, &genai.GenerateContentConfig{Temperature: genai.Ptr[float32](defaultTemperature)})
// 	go l.GenerateGeneralPOIWorker(&wg, ctx, cityName, resultCh, &genai.GenerateContentConfig{Temperature: genai.Ptr[float32](defaultTemperature)})
// 	go l.GenerateEnhancedPersonalisedPOIWorker(&wg, ctx, cityName, userID, profileID, resultCh, enhancedPromptData, detectedDomain, &genai.GenerateContentConfig{Temperature: genai.Ptr[float32](defaultTemperature)})

// 	// Close channel after workers complete
// 	go func() {
// 		wg.Wait()
// 		close(resultCh)
// 	}()

// 	// Fan-in: Collect results
// 	itinerary, llmInteractionID, rawPersonalisedPOIs, errors := l.CollectResults(resultCh)
// 	if len(errors) > 0 {
// 		l.logger.ErrorContext(ctx, "Errors during enhanced itinerary generation", slog.Any("errors", errors))
// 		for _, err := range errors {
// 			span.RecordError(err)
// 		}
// 		span.SetStatus(codes.Error, "Failed to generate enhanced itinerary")
// 		return nil, fmt.Errorf("failed to generate enhanced itinerary: %v", errors)
// 	}

// 	// Handle city data
// 	cityID, err := l.HandleCityData(ctx, itinerary.GeneralCityData)
// 	if err != nil {
// 		span.RecordError(err)
// 		span.SetStatus(codes.Error, "Failed to handle city data")
// 		return nil, err
// 	}
// 	span.SetAttributes(attribute.String("city.id", cityID.String()))

// 	// Handle general POIs
// 	l.HandleGeneralPOIs(ctx, itinerary.PointsOfInterest, cityID)
// 	span.SetAttributes(attribute.Int("general_pois.count", len(itinerary.PointsOfInterest)))

// 	// Handle personalized POIs with domain-aware filtering
// 	sortedPois, err := l.HandlePersonalisedPOIs(ctx, rawPersonalisedPOIs, cityID, userLocation, llmInteractionID, userID, profileID)
// 	if err != nil {
// 		span.RecordError(err)
// 		span.SetStatus(codes.Error, "Failed to handle personalized POIs")
// 		return nil, err
// 	}
// 	itinerary.AIItineraryResponse.PointsOfInterest = sortedPois
// 	span.SetAttributes(
// 		attribute.Int("personalized_pois.count", len(sortedPois)),
// 		attribute.String("llm_interaction.id", llmInteractionID.String()),
// 	)

// 	l.logger.InfoContext(ctx, "Enhanced itinerary ready",
// 		slog.String("itinerary_name", itinerary.AIItineraryResponse.ItineraryName),
// 		slog.Int("final_personalised_poi_count", len(itinerary.AIItineraryResponse.PointsOfInterest)),
// 		slog.String("detected_domain", string(detectedDomain)))

// 	span.SetStatus(codes.Ok, "Enhanced itinerary generated successfully")
// 	return &itinerary, nil
// }

// PrepareEnhancedPromptData prepares prompt data with domain-specific filters
func (l *ServiceImpl) PrepareEnhancedPromptData(filters *types.CombinedFilters, domain types.DomainType) string {
	var promptParts []string

	// Base preferences
	if filters.BasePreferences != nil {
		basePrompt := getUserPreferencesPrompt(filters.BasePreferences)
		if basePrompt != "" {
			promptParts = append(promptParts, "Base Preferences: "+basePrompt)
		}
	}

	// Domain-specific preferences based on detected domain
	switch domain {
	case types.DomainAccommodation:
		if filters.AccommodationPreferences != nil {
			accommodationPrompt := l.getAccommodationPreferencesPrompt(filters.AccommodationPreferences)
			if accommodationPrompt != "" {
				promptParts = append(promptParts, "Accommodation Focus: "+accommodationPrompt)
			}
		}
	case types.DomainDining:
		if filters.DiningPreferences != nil {
			diningPrompt := l.getDiningPreferencesPrompt(filters.DiningPreferences)
			if diningPrompt != "" {
				promptParts = append(promptParts, "Dining Focus: "+diningPrompt)
			}
		}
	case types.DomainActivities:
		if filters.ActivityPreferences != nil {
			activityPrompt := l.getActivityPreferencesPrompt(filters.ActivityPreferences)
			if activityPrompt != "" {
				promptParts = append(promptParts, "Activity Focus: "+activityPrompt)
			}
		}
	case types.DomainItinerary:
		if filters.ItineraryPreferences != nil {
			itineraryPrompt := l.getItineraryPreferencesPrompt(filters.ItineraryPreferences)
			if itineraryPrompt != "" {
				promptParts = append(promptParts, "Planning Focus: "+itineraryPrompt)
			}
		}
	default:
		// For general domain, include all available preferences
		if filters.AccommodationPreferences != nil {
			accommodationPrompt := l.getAccommodationPreferencesPrompt(filters.AccommodationPreferences)
			if accommodationPrompt != "" {
				promptParts = append(promptParts, "Accommodation: "+accommodationPrompt)
			}
		}
		if filters.DiningPreferences != nil {
			diningPrompt := l.getDiningPreferencesPrompt(filters.DiningPreferences)
			if diningPrompt != "" {
				promptParts = append(promptParts, "Dining: "+diningPrompt)
			}
		}
		if filters.ActivityPreferences != nil {
			activityPrompt := l.getActivityPreferencesPrompt(filters.ActivityPreferences)
			if activityPrompt != "" {
				promptParts = append(promptParts, "Activities: "+activityPrompt)
			}
		}
		if filters.ItineraryPreferences != nil {
			itineraryPrompt := l.getItineraryPreferencesPrompt(filters.ItineraryPreferences)
			if itineraryPrompt != "" {
				promptParts = append(promptParts, "Planning: "+itineraryPrompt)
			}
		}
	}

	return strings.Join(promptParts, "\n")
}

// Domain-specific prompt generation methods
func (l *ServiceImpl) getAccommodationPreferencesPrompt(prefs *types.AccommodationPreferences) string {
	var parts []string

	if len(prefs.AccommodationType) > 0 {
		parts = append(parts, fmt.Sprintf("preferred accommodation types: %s", strings.Join(prefs.AccommodationType, ", ")))
	}
	if len(prefs.Amenities) > 0 {
		parts = append(parts, fmt.Sprintf("required amenities: %s", strings.Join(prefs.Amenities, ", ")))
	}
	if prefs.StarRating != nil {
		if prefs.StarRating.Min != nil && prefs.StarRating.Max != nil {
			parts = append(parts, fmt.Sprintf("star rating: %.0f-%.0f stars", *prefs.StarRating.Min, *prefs.StarRating.Max))
		}
	}

	return strings.Join(parts, "; ")
}

func (l *ServiceImpl) getDiningPreferencesPrompt(prefs *types.DiningPreferences) string {
	var parts []string

	if len(prefs.CuisineTypes) > 0 {
		parts = append(parts, fmt.Sprintf("preferred cuisines: %s", strings.Join(prefs.CuisineTypes, ", ")))
	}
	if len(prefs.ServiceStyle) > 0 {
		parts = append(parts, fmt.Sprintf("service style: %s", strings.Join(prefs.ServiceStyle, ", ")))
	}
	if len(prefs.DietaryNeeds) > 0 {
		parts = append(parts, fmt.Sprintf("dietary requirements: %s", strings.Join(prefs.DietaryNeeds, ", ")))
	}
	if prefs.MichelinRated {
		parts = append(parts, "prefer Michelin-rated restaurants")
	}
	if prefs.LocalRecommendations {
		parts = append(parts, "prioritize local recommendations")
	}

	return strings.Join(parts, "; ")
}

func (l *ServiceImpl) getActivityPreferencesPrompt(prefs *types.ActivityPreferences) string {
	var parts []string

	if len(prefs.ActivityCategories) > 0 {
		parts = append(parts, fmt.Sprintf("preferred activities: %s", strings.Join(prefs.ActivityCategories, ", ")))
	}
	if prefs.PhysicalActivityLevel != "" {
		parts = append(parts, fmt.Sprintf("physical activity level: %s", prefs.PhysicalActivityLevel))
	}
	if prefs.CulturalImmersionLevel != "" {
		parts = append(parts, fmt.Sprintf("cultural immersion: %s", prefs.CulturalImmersionLevel))
	}
	if prefs.EducationalPreference {
		parts = append(parts, "prefer educational experiences")
	}
	if prefs.PhotoOpportunities {
		parts = append(parts, "value photo opportunities")
	}

	return strings.Join(parts, "; ")
}

func (l *ServiceImpl) getItineraryPreferencesPrompt(prefs *types.ItineraryPreferences) string {
	var parts []string

	if prefs.PlanningStyle != "" {
		parts = append(parts, fmt.Sprintf("planning style: %s", prefs.PlanningStyle))
	}
	if prefs.PreferredPace != "" {
		parts = append(parts, fmt.Sprintf("preferred pace: %s", prefs.PreferredPace))
	}
	if prefs.TimeFlexibility != "" {
		parts = append(parts, fmt.Sprintf("time flexibility: %s", prefs.TimeFlexibility))
	}
	if len(prefs.PreferredSeasons) > 0 {
		parts = append(parts, fmt.Sprintf("preferred seasons: %s", strings.Join(prefs.PreferredSeasons, ", ")))
	}
	if prefs.AvoidPeakSeason {
		parts = append(parts, "avoid peak season")
	}

	return strings.Join(parts, "; ")
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

func TruncateString(str string, num int) string {
	if len(str) > num {
		return str[0:num] + "..."
	}
	return str
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

	// Use pointers for nullable fields
	var sourceInteractionID *uuid.UUID
	if req.LlmInteractionID != nil {
		sourceInteractionID = req.LlmInteractionID // Already a pointer, no dereference needed
	}

	var primaryCityID *uuid.UUID
	if req.PrimaryCityID != nil {
		primaryCityID = req.PrimaryCityID // Already a pointer, no dereference needed
	}

	newBookmark := &types.UserSavedItinerary{
		UserID:                 userID,
		SourceLlmInteractionID: sourceInteractionID,
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

	// Handle cityID for further processing
	if newBookmark.PrimaryCityID == nil {
		l.logger.WarnContext(ctx, "PrimaryCityID not provided, skipping itinerary save")
		return savedID, nil
	}
	cityID := *newBookmark.PrimaryCityID // Safe to dereference since we checked for nil

	// Save to itineraries
	itineraryID, err := l.poiRepo.SaveItinerary(ctx, userID, cityID)
	if err != nil {
		l.logger.WarnContext(ctx, "Failed to save to itineraries", slog.Any("error", err))
		span.RecordError(err)
		return savedID, nil
	}

	// Fetch POIs from llm_suggested_pois only if we have an interaction ID
	if req.LlmInteractionID != nil {
		pois, err := l.llmInteractionRepo.GetLlmSuggestedPOIsByInteractionSortedByDistance(ctx, *req.LlmInteractionID, cityID, types.UserLocation{})
		if err != nil {
			l.logger.WarnContext(ctx, "Failed to fetch suggested POIs", slog.Any("error", err))
			span.RecordError(err)
			return savedID, nil
		}

		if len(pois) > 0 {
			l.logger.InfoContext(ctx, "Found POIs to process", slog.Int("count", len(pois)))

			for i := range pois {
				pois[i].CityID = cityID
			}

			if err := l.poiRepo.SaveItineraryPOIs(ctx, itineraryID, pois); err != nil {
				l.logger.WarnContext(ctx, "Failed to save to itinerary_pois", slog.Any("error", err))
				span.RecordError(err)
				return savedID, nil
			}
		}
	}

	l.logger.InfoContext(ctx, "Successfully saved itinerary",
		slog.String("savedItineraryID", savedID.String()),
		slog.String("itineraryID", itineraryID.String()))
	span.SetAttributes(attribute.String("itinerary.id", itineraryID.String()))
	span.SetStatus(codes.Ok, "Itinerary saved successfully")
	return savedID, nil
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

// GetUserChatSessions retrieves all chat sessions for a user
func (l *ServiceImpl) GetUserChatSessions(ctx context.Context, userID uuid.UUID) ([]types.ChatSession, error) {
	ctx, span := otel.Tracer("LlmInteractionService").Start(ctx, "GetUserChatSessions", trace.WithAttributes(
		attribute.String("user.id", userID.String()),
	))
	defer span.End()

	l.logger.InfoContext(ctx, "Retrieving chat sessions for user",
		slog.String("userID", userID.String()))

	sessions, err := l.llmInteractionRepo.GetUserChatSessions(ctx, userID)
	if err != nil {
		l.logger.ErrorContext(ctx, "Failed to get user chat sessions", slog.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to get user chat sessions")
		return nil, fmt.Errorf("failed to get user chat sessions: %w", err)
	}

	l.logger.InfoContext(ctx, "Successfully retrieved chat sessions",
		slog.String("userID", userID.String()),
		slog.Int("sessionCount", len(sessions)))
	span.SetAttributes(attribute.Int("sessions.count", len(sessions)))
	span.SetStatus(codes.Ok, "Chat sessions retrieved successfully")
	return sessions, nil
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
		if poi, ok := cached.(*types.POIDetailedInfo); ok {
			l.logger.InfoContext(ctx, "Cache hit for POI details", slog.String("cache_key", cacheKey))
			span.AddEvent("Cache hit")
			span.SetStatus(codes.Ok, "POI details served from cache")
			return poi, nil
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

	// Check database
	poi, err := l.poiRepo.FindPOIDetails(ctx, cityID, lat, lon, 100.0) // 100m tolerance
	if err != nil {
		l.logger.ErrorContext(ctx, "Failed to query POI details from database", slog.Any("error", err))
		span.RecordError(err)
		return nil, fmt.Errorf("failed to query POI details: %w", err)
	}
	if poi != nil {
		poi.City = city
		l.cache.Set(cacheKey, poi, cache.DefaultExpiration)
		l.logger.InfoContext(ctx, "Database hit for POI details", slog.String("cache_key", cacheKey))
		span.AddEvent("Database hit")
		span.SetStatus(codes.Ok, "POI details served from database")
		return poi, nil
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
		attribute.String("poi.name", poiName),
		attribute.String("city.name", cityName),
	))
	defer span.End()

	// Create a prompt for the LLM
	prompt := generatedContinuedConversationPrompt(poiName, cityName)

	// Generate LLM response
	response, err := l.aiClient.GenerateContent(ctx, prompt, nil)
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
			span.SetAttributes(attribute.Float64("poi.distance_meters", distance))
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
		attribute.String("poi.name", poiData.Name),
		attribute.Float64("poi.latitude", poiData.Latitude),
		attribute.Float64("poi.longitude", poiData.Longitude),
		attribute.String("poi.category", poiData.Category),
		attribute.String("llm_interaction.id", llmInteractionID.String()),
	)
	return poiData, nil
}

// enhancePOIRecommendationsWithSemantics uses embeddings to find similar POIs and enrich recommendations
func (l *ServiceImpl) enhancePOIRecommendationsWithSemantics(ctx context.Context, userMessage string, cityID uuid.UUID, userPreferences []string, limit int) ([]types.POIDetailedInfo, error) {
	ctx, span := otel.Tracer("LlmInteractionService").Start(ctx, "enhancePOIRecommendationsWithSemantics", trace.WithAttributes(
		attribute.String("user.message", userMessage),
		attribute.String("city.id", cityID.String()),
		attribute.Int("limit", limit),
	))
	defer span.End()

	l.logger.DebugContext(ctx, "Enhancing POI recommendations with semantic search",
		slog.String("message", userMessage),
		slog.String("city_id", cityID.String()))

	if l.embeddingService == nil {
		l.logger.WarnContext(ctx, "Embedding service not available, falling back to traditional search")
		span.AddEvent("Embedding service not available")
		return []types.POIDetailedInfo{}, nil
	}

	// Generate embedding for user message combined with preferences
	searchQuery := userMessage
	if len(userPreferences) > 0 {
		searchQuery += " " + strings.Join(userPreferences, " ")
	}

	queryEmbedding, err := l.embeddingService.GenerateQueryEmbedding(ctx, searchQuery)
	if err != nil {
		l.logger.ErrorContext(ctx, "Failed to generate query embedding",
			slog.Any("error", err),
			slog.String("query", searchQuery))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to generate query embedding")
		return []types.POIDetailedInfo{}, fmt.Errorf("failed to generate query embedding: %w", err)
	}

	// Search for similar POIs in the city
	similarPOIs, err := l.poiRepo.FindSimilarPOIsByCity(ctx, queryEmbedding, cityID, limit)
	if err != nil {
		l.logger.ErrorContext(ctx, "Failed to find similar POIs", slog.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to find similar POIs")
		return []types.POIDetailedInfo{}, fmt.Errorf("failed to find similar POIs: %w", err)
	}

	l.logger.InfoContext(ctx, "Found semantically similar POIs",
		slog.Int("count", len(similarPOIs)),
		slog.String("city_id", cityID.String()))
	span.SetAttributes(
		attribute.Int("similar_pois.count", len(similarPOIs)),
		attribute.String("search.query", searchQuery),
	)
	span.SetStatus(codes.Ok, "Semantic POI recommendations enhanced")

	return similarPOIs, nil
}

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
	for i, poi := range pois {
		if poi.ID == uuid.Nil {
			continue
		}

		// Generate embedding for this POI if it doesn't have one
		embedding, err := l.embeddingService.GeneratePOIEmbedding(ctx, poi.Name, poi.DescriptionPOI, poi.Category)
		if err != nil {
			l.logger.WarnContext(ctx, "Failed to generate embedding for POI",
				slog.Any("error", err),
				slog.String("poi_name", poi.Name))
			continue
		}

		// Update POI with embedding
		err = l.poiRepo.UpdatePOIEmbedding(ctx, poi.ID, embedding)
		if err != nil {
			l.logger.WarnContext(ctx, "Failed to update POI embedding",
				slog.Any("error", err),
				slog.String("poi_id", poi.ID.String()))
		}

		pois[i] = poi
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
	for i, poi := range session.CurrentItinerary.AIItineraryResponse.PointsOfInterest {
		// Check for exact match or semantic similarity
		if strings.EqualFold(poi.Name, poiName) ||
			strings.Contains(strings.ToLower(poi.Name), strings.ToLower(poiName)) ||
			strings.Contains(strings.ToLower(poiName), strings.ToLower(poi.Name)) {

			removedName := poi.Name
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
			for _, poi := range session.CurrentItinerary.AIItineraryResponse.PointsOfInterest {
				names = append(names, poi.Name)
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

// extractTextFromResponse extracts text from the AI response
func extractTextFromResponse(resp *genai.GenerateContentResponse) string {
	var txt string
	for _, candidate := range resp.Candidates {
		if candidate.Content != nil && len(candidate.Content.Parts) > 0 {
			txt = candidate.Content.Parts[0].Text
			break
		}
	}
	return txt
}

// assignIDs assigns UUIDs and interaction IDs to response items
func assignIDs(response interface{}, interactionID uuid.UUID) {
	switch r := response.(type) {
	case types.AiCityResponse:
		for i := range r.PointsOfInterest {
			r.PointsOfInterest[i].ID = uuid.New()
			r.PointsOfInterest[i].LlmInteractionID = interactionID
		}
		for i := range r.AIItineraryResponse.PointsOfInterest {
			r.AIItineraryResponse.PointsOfInterest[i].ID = uuid.New()
			r.AIItineraryResponse.PointsOfInterest[i].LlmInteractionID = interactionID
		}
	case struct {
		Hotels []types.HotelDetailedInfo `json:"hotels"`
	}:
		for i := range r.Hotels {
			r.Hotels[i].ID = uuid.New()
			r.Hotels[i].LlmInteractionID = interactionID
		}
	case struct {
		Restaurants []types.RestaurantDetailedInfo `json:"restaurants"`
	}:
		for i := range r.Restaurants {
			r.Restaurants[i].ID = uuid.New()
			r.Restaurants[i].LlmInteractionID = interactionID
		}
	case struct {
		Activities []types.POIDetailedInfo `json:"activities"`
	}:
		for i := range r.Activities {
			r.Activities[i].ID = uuid.New()
			r.Activities[i].LlmInteractionID = interactionID
		}
	}
}
