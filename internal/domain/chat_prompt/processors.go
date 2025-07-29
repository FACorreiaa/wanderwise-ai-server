package chat_prompt

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"google.golang.org/genai"

	"github.com/FACorreiaa/go-poi-au-suggestions/internal/domain/interests"
	"github.com/FACorreiaa/go-poi-au-suggestions/internal/domain/tags"
	"github.com/FACorreiaa/go-poi-au-suggestions/internal/types"
)

func (l *Service) PreparePromptData(interests []*interests.Interest, tags []*tags.Tags, searchProfile *UserPreferenceProfileResponse) (interestNames []string, tagsPromptPart string, userPrefs string) {
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

func (l *Service) CollectResults(resultCh <-chan GenAIResponse) (itinerary AiCityResponse, llmInteractionID uuid.UUID, rawPersonalisedPOIs []POIDetailedInfo, errors []error) {
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

func (l *Service) HandleCityData(ctx context.Context, cityData GeneralCityData) (cityID uuid.UUID, err error) {
	c, err := l.cityRepo.FindCityByNameAndCountry(ctx, cityData.City, cityData.Country)
	if err != nil {
		return uuid.Nil, fmt.Errorf("failed to check city existence: %w", err)
	}
	if c == nil {
		cityDetail := CityDetail{
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

func (l *Service) HandleGeneralPOIs(ctx context.Context, pois []POIDetailedInfo, cityID uuid.UUID) {
	for _, p := range pois {
		existingPoi, err := l.poiRepo.FindPoiByNameAndCity(ctx, p.Name, cityID)
		if err != nil {
			l.logger.Warn("Failed to check POI existence", zap.String("poi_name", p.Name), zap.Error(err))
			continue
		}
		if existingPoi == nil {
			_, err = l.poiRepo.SavePoi(ctx, p, cityID)
			if err != nil {
				l.logger.Warn("Failed to save POI", zap.String("poi_name", p.Name), zap.Error(err))
			}
		}
	}
}

func (l *Service) HandlePersonalisedPOIs(ctx context.Context, pois []POIDetailedInfo, cityID uuid.UUID, userLocation *UserLocation, llmInteractionID uuid.UUID, userID, profileID uuid.UUID) ([]POIDetailedInfo, error) {
	if userLocation == nil || len(pois) == 0 {
		return pois, nil
	}

	if cityID == uuid.Nil || cityID.String() == "00000000-0000-0000-0000-000000000000" {
		l.logger.Warn("Skipping itinerary creation due to invalid cityID",
			zap.String("cityID", cityID.String()))
		return pois, nil
	}

	err := l.llmInteractionRepo.SaveLlmSuggestedPOIsBatch(ctx, pois, userID, profileID, llmInteractionID, cityID)
	if err != nil {
		return nil, fmt.Errorf("failed to save personalised POIs: %w", err)
	}

	itineraryID, err := l.poiRepo.SaveItinerary(ctx, userID, cityID)
	if err != nil {
		l.logger.Error("Failed to save itinerary, skipping itinerary creation",
			zap.Error(err),
			zap.String("cityID", cityID.String()),
			zap.String("userID", userID.String()))
		return pois, nil
	}

	if err := l.poiRepo.SaveItineraryPOIs(ctx, itineraryID, pois); err != nil {
		return nil, fmt.Errorf("failed to save itinerary POIs: %w", err)
	}

	sortedPois, err := l.llmInteractionRepo.GetLlmSuggestedPOIsByInteractionSortedByDistance(ctx, llmInteractionID, cityID, *userLocation)
	if err != nil {
		l.logger.Error("Failed to fetch sorted POIs", zap.Error(err))
		return pois, nil
	}
	return sortedPois, nil
}

func (l *Service) GenerateEnhancedPersonalisedPOIWorker(wg *sync.WaitGroup, ctx context.Context,
	cityName string, userID, profileID uuid.UUID, resultCh chan<- GenAIResponse,
	enhancedPromptData string, domain DomainType,
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

	prompt := l.getEnhancedPersonalizedPOIPrompt(cityName, enhancedPromptData, domain)
	span.SetAttributes(attribute.Int("prompt.length", len(prompt)))

	response, err := l.aiClient.GenerateResponse(ctx, prompt, config)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "AI generation failed")
		resultCh <- GenAIResponse{Err: fmt.Errorf("failed to generate enhanced personalized POIs: %w", err)}
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
		resultCh <- GenAIResponse{Err: err}
		return
	}
	span.SetAttributes(attribute.Int("response.length", len(txt)))

	cleanTxt := cleanJSONResponse(txt)
	var itineraryData AIItineraryResponse
	if err := json.Unmarshal([]byte(cleanTxt), &itineraryData); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to parse enhanced personalized POI JSON")
		resultCh <- GenAIResponse{Err: fmt.Errorf("failed to parse enhanced personalized POI JSON: %w", err)}
		return
	}

	span.SetAttributes(attribute.Int("pois.count", len(itineraryData.PointsOfInterest)))
	span.SetStatus(codes.Ok, "Enhanced personalized POIs generated successfully")
	resultCh <- GenAIResponse{
		ItineraryName:        itineraryData.ItineraryName,
		ItineraryDescription: itineraryData.OverallDescription,
		PersonalisedPOI:      itineraryData.PointsOfInterest,
		LlmInteractionID:     uuid.New(),
	}
}

func (l *Service) generatePOIData(ctx context.Context, poiName, cityName string, userLocation *UserLocation, userID, cityID uuid.UUID) (POIDetailedInfo, error) {
	ctx, span := otel.Tracer("LlmInteractionService").Start(ctx, "GeneratePOIData", trace.WithAttributes(
		attribute.String("p.name", poiName),
		attribute.String("city.name", cityName),
	))
	defer span.End()

	prompt := generatedContinuedConversationPrompt(poiName, cityName)

	response, err := l.aiClient.GenerateContent(ctx, prompt, "", nil)
	if err != nil {
		span.RecordError(err)
		return POIDetailedInfo{}, fmt.Errorf("failed to generate POI data: %w", err)
	}

	interaction := LlmInteraction{
		UserID:       userID,
		Prompt:       prompt,
		ResponseText: response,
		ModelUsed:    model,
		CityName:     cityName,
	}
	savedLlmInteractionID, err := l.llmInteractionRepo.SaveInteraction(ctx, interaction)
	if err != nil {
		l.logger.Error("Failed to save LLM interaction in generatePOIData", zap.Error(err))
		return POIDetailedInfo{}, fmt.Errorf("failed to save LLM interaction: %w", err)
	}
	span.SetAttributes(attribute.String("llm.interaction_id.for_poi_data", savedLlmInteractionID.String()))

	cleanResponse := cleanJSONResponse(response)
	var poiData POIDetailedInfo
	if err := json.Unmarshal([]byte(cleanResponse), &poiData); err != nil || poiData.Name == "" {
		l.logger.Warn("LLM returned invalid or empty POI data",
			zap.String("poiName", poiName),
			zap.String("llmResponse", response),
			zap.Error(err))
		span.AddEvent("Invalid LLM response")
		poiData = POIDetailedInfo{
			ID:             uuid.New(),
			Name:           poiName,
			Latitude:       0,
			Longitude:      0,
			Category:       "Attraction",
			DescriptionPOI: fmt.Sprintf("Added %s based on user request, but detailed data not available.", poiName),
			Distance:       0,
		}
	}
	if poiData.ID == uuid.Nil {
		poiData.ID = uuid.New()
	}
	poiData.LlmInteractionID = savedLlmInteractionID

	if userLocation != nil && userLocation.UserLat != 0 && userLocation.UserLon != 0 && poiData.Latitude != 0 && poiData.Longitude != 0 {
		distance, err := l.poiRepo.CalculateDistancePostGIS(ctx, userLocation.UserLat, userLocation.UserLon, poiData.Latitude, poiData.Longitude)
		if err != nil {
			l.logger.Warn("Failed to calculate distance", zap.Error(err))
			span.RecordError(err)
			poiData.Distance = 0
		} else {
			poiData.Distance = distance
			span.SetAttributes(attribute.Float64("p.distance_meters", distance))
			l.logger.Debug("Calculated distance for POI",
				zap.String("poiName", poiName),
				zap.Float64("distance_meters", distance))
		}
	} else {
		poiData.Distance = 0
		span.AddEvent("Distance not calculated due to missing location data")
		l.logger.Warn("Cannot calculate distance",
			zap.Bool("userLocationAvailable", userLocation != nil),
			zap.Float64("userLat", userLocation.UserLat),
			zap.Float64("userLon", userLocation.UserLon),
			zap.Float64("poiLatitude", poiData.Latitude),
			zap.Float64("poiLongitude", poiData.Longitude))
	}

	llmInteractionID := uuid.New()
	_, err = l.llmInteractionRepo.SaveSinglePOI(ctx, poiData, userID, cityID, savedLlmInteractionID)
	if err != nil {
		l.logger.Warn("Failed to save POI to database", zap.Error(err))
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

func (l *Service) generateSemanticPOIRecommendations(ctx context.Context, userMessage string, cityID uuid.UUID, userID uuid.UUID, userLocation *UserLocation, semanticWeight float64) ([]POIDetailedInfo, error) {
	ctx, span := otel.Tracer("LlmInteractionService").Start(ctx, "generateSemanticPOIRecommendations", trace.WithAttributes(
		attribute.String("user.message", userMessage),
		attribute.String("city.id", cityID.String()),
		attribute.String("user.id", userID.String()),
		attribute.Float64("semantic.weight", semanticWeight),
	))
	defer span.End()

	l.logger.Debug("Generating semantic POI recommendations",
		zap.String("message", userMessage),
		zap.String("city_id", cityID.String()),
		zap.Float64("semantic_weight", semanticWeight))

	if l.embeddingService == nil {
		err := fmt.Errorf("embedding service not available")
		l.logger.Error("Embedding service not available", zap.Error(err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Embedding service not available")
		return nil, err
	}

	queryEmbedding, err := l.embeddingService.GenerateQueryEmbedding(ctx, userMessage)
	if err != nil {
		l.logger.Error("Failed to generate query embedding", zap.Error(err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to generate query embedding")
		return nil, fmt.Errorf("failed to generate query embedding: %w", err)
	}

	var pois []POIDetailedInfo

	if userLocation != nil && userLocation.UserLat != 0 && userLocation.UserLon != 0 {
		filter := POIFilter{
			Location: GeoPoint{
				Latitude:  userLocation.UserLat,
				Longitude: userLocation.UserLon,
			},
			Radius: userLocation.SearchRadiusKm,
		}

		hybridPOIs, err := l.poiRepo.SearchPOIsHybrid(ctx, filter, queryEmbedding, semanticWeight)
		if err != nil {
			l.logger.Error("Failed to perform hybrid search", zap.Error(err))
			span.RecordError(err)
		} else {
			pois = hybridPOIs
			l.logger.Info("Used hybrid search for POI recommendations",
				zap.Int("poi_count", len(pois)))
			span.AddEvent("Used hybrid search")
		}
	}

	if len(pois) == 0 {
		semanticPOIs, err := l.poiRepo.FindSimilarPOIsByCity(ctx, queryEmbedding, cityID, 10)
		if err != nil {
			l.logger.Error("Failed to find similar POIs", zap.Error(err))
			span.RecordError(err)
			span.SetStatus(codes.Error, "Failed to find similar POIs")
			return nil, fmt.Errorf("failed to find similar POIs: %w", err)
		}
		pois = semanticPOIs
		l.logger.Info("Used semantic-only search for POI recommendations",
			zap.Int("poi_count", len(pois)))
		span.AddEvent("Used semantic-only search")
	}

	for i, p := range pois {
		if p.ID == uuid.Nil {
			continue
		}

		embedding, err := l.embeddingService.GeneratePOIEmbedding(ctx, p.Name, p.DescriptionPOI, p.Category)
		if err != nil {
			l.logger.Warn("Failed to generate embedding for POI",
				zap.Error(err),
				zap.String("poi_name", p.Name))
			continue
		}

		err = l.poiRepo.UpdatePOIEmbedding(ctx, p.ID, embedding)
		if err != nil {
			l.logger.Warn("Failed to update POI embedding",
				zap.Error(err),
				zap.String("poi_id", p.ID.String()))
		}

		pois[i] = p
	}

	l.logger.Info("Generated semantic POI recommendations",
		zap.String("message", userMessage),
		zap.Int("recommendations", len(pois)))
	span.SetAttributes(
		attribute.String("search.query", userMessage),
		attribute.Int("recommendations.count", len(pois)),
		attribute.Float64("semantic.weight", semanticWeight),
	)
	span.SetStatus(codes.Ok, "Semantic POI recommendations generated")

	return pois, nil
}
