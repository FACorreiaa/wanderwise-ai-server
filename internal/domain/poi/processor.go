package poi

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/patrickmn/go-cache"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"google.golang.org/genai"
)

func (s *Service) filterAttractions(attractions []POIDetailedInfo, attractionType, isOutdoor string) []POIDetailedInfo {
	if attractionType == "" && isOutdoor == "" {
		return attractions
	}

	filtered := make([]POIDetailedInfo, 0)
	for _, attraction := range attractions {
		if attractionType != "" && attraction.Category != attractionType {
			continue
		}
		if isOutdoor != "" {
			hasOutdoorTag := false
			for _, tag := range attraction.Tags {
				if (isOutdoor == "true" && tag == "outdoor") || (isOutdoor == "false" && tag == "indoor") {
					hasOutdoorTag = true
					break
				}
			}
			if !hasOutdoorTag {
				continue
			}
		}
		filtered = append(filtered, attraction)
	}
	return filtered
}

func (s *Service) generateAttractionsFromLLM(ctx context.Context, userID uuid.UUID, lat, lon, distance float64, _, _ string) (*GenAIResponse, error) {
	resultCh := make(chan GenAIResponse, 1)
	var wg sync.WaitGroup
	wg.Add(1)
	go s.getGeneralAttractionsByDistance(&wg, ctx, userID, lat, lon, distance, resultCh, &genai.GenerateContentConfig{
		Temperature:     genai.Ptr[float32](0.7),
		MaxOutputTokens: 16384,
	})
	wg.Wait()
	close(resultCh)

	result := <-resultCh
	if result.Err != nil {
		return nil, result.Err
	}
	return &result, nil
}

func (s *Service) getGeneralAttractionsByDistance(wg *sync.WaitGroup,
	ctx context.Context,
	userID uuid.UUID,
	lat, lon, distance float64,
	resultCh chan<- GenAIResponse,
	config *genai.GenerateContentConfig) {
	ctx, span := otel.Tracer("LlmInteractionService").Start(ctx, "GenerateGeneralPOIWorker", trace.WithAttributes(
		attribute.Float64("latitude", lat),
		attribute.Float64("longitude", lon),
		attribute.Float64("distance.km", distance),
		attribute.String("user.id", userID.String())))

	defer span.End()
	defer wg.Done()

	userLocation := UserLocation{
		UserLat:        lat,
		UserLon:        lon,
		SearchRadiusKm: distance,
	}
	prompt := getAttractionsNeabyPrompt(userLocation)
	span.SetAttributes(attribute.Int("prompt.length", len(prompt)))

	if s.aiClient == nil {
		err := fmt.Errorf("AI client is not available - check API key configuration")
		span.RecordError(err)
		span.SetStatus(codes.Error, "AI client unavailable")
		resultCh <- GenAIResponse{Err: err}
		return
	}

	startTime := time.Now()
	response, err := s.aiClient.GenerateResponse(ctx, prompt, config)
	latencyMs := int(time.Since(startTime).Milliseconds())
	span.SetAttributes(attribute.Int("response.latency_ms", latencyMs))

	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to generate general POIs")
		resultCh <- GenAIResponse{Err: fmt.Errorf("failed to generate general POIs: %w", err)}
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
		resultCh <- GenAIResponse{Err: err}
		return
	}
	span.SetAttributes(attribute.Int("response.length", len(txt)))

	cleanTxt := cleanJSONResponse(txt)
	var poiData struct {
		PointsOfInterest []POIDetailedInfo `json:"points_of_interest"`
	}
	if err := json.Unmarshal([]byte(cleanTxt), &poiData); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to parse general POI JSON")
		resultCh <- GenAIResponse{Err: fmt.Errorf("failed to parse general POI JSON: %w", err)}
		return
	}

	fmt.Println(cleanTxt)

	span.SetAttributes(attribute.Int("pois.count", len(poiData.PointsOfInterest)))
	span.SetStatus(codes.Ok, "General POIs generated successfully")
	resultCh <- GenAIResponse{
		GeneralPOI: poiData.PointsOfInterest,
		ModelName:  s.aiClient.ModelName,
		Prompt:     prompt,
		Response:   cleanTxt,
	}
}

func (s *Service) GetNearbyAttractions(ctx context.Context, userID uuid.UUID, lat, lon, distance float64, attractionType, isOutdoor string) ([]POIDetailedInfo, error) {
	ctx, span := otel.Tracer("POIService").Start(ctx, "GetNearbyAttractions", trace.WithAttributes(
		attribute.Float64("location.lat", lat),
		attribute.Float64("location.lon", lon),
		attribute.Float64("distance", distance),
		attribute.String("attraction_type", attractionType),
		attribute.String("is_outdoor", isOutdoor),
	))
	defer span.End()

	// Build cache key with domain-specific filters
	cacheKey := fmt.Sprintf("attractions_%f_%f_%f_%s_%s_%s", lat, lon, distance, userID.String(), attractionType, isOutdoor)

	if cached, found := s.cache.Get(cacheKey); found {
		if pois, ok := cached.([]POIDetailedInfo); ok {
			s.logger.Info("Serving attractions from cache")
			return pois, nil
		}
	}

	s.logger.Info("Querying attractions from database")

	// Get attractions from database with filters
	attractions, err := s.repo.GetPOIsByLocationAndDistanceWithCategory(ctx, lat, lon, distance, "attraction")
	if err == nil && len(attractions) > 0 {
		// Apply domain-specific filters
		filteredAttractions := s.filterAttractions(attractions, attractionType, isOutdoor)

		// Mark as database source
		for i := range filteredAttractions {
			filteredAttractions[i].Source = "points_of_interest"
		}

		s.cache.Set(cacheKey, filteredAttractions, cache.DefaultExpiration)
		return filteredAttractions, nil
	}

	s.logger.Info("No attractions found in database, falling back to LLM generation")

	// Generate attractions using LLM with domain-specific prompt
	genAIResponse, err := s.generateAttractionsFromLLM(ctx, userID, lat, lon, distance, attractionType, isOutdoor)
	if err != nil {
		span.RecordError(err)
		return nil, err
	}

	enrichedAttractions := s.enrichAndFilterLLMResponse(genAIResponse.GeneralPOI, lat, lon, distance)
	for i := range enrichedAttractions {
		enrichedAttractions[i].Source = "llm_suggested_pois"
	}

	s.cache.Set(cacheKey, enrichedAttractions, cache.DefaultExpiration)
	return enrichedAttractions, nil
}

//

func (s *Service) GetNearbyActivities(ctx context.Context, userID uuid.UUID, lat, lon, distance float64, activityType, duration string) ([]POIDetailedInfo, error) {
	ctx, span := otel.Tracer("POIService").Start(ctx, "GetNearbyActivities", trace.WithAttributes(
		attribute.Float64("location.lat", lat),
		attribute.Float64("location.lon", lon),
		attribute.Float64("distance", distance),
		attribute.String("activity_type", activityType),
		attribute.String("duration", duration),
	))
	defer span.End()

	// Build cache key with domain-specific filters
	cacheKey := fmt.Sprintf("activities_%f_%f_%f_%s_%s_%s", lat, lon, distance, userID.String(), activityType, duration)

	if cached, found := s.cache.Get(cacheKey); found {
		if pois, ok := cached.([]POIDetailedInfo); ok {
			s.logger.Info("Serving activities from cache", zap.String("key", cacheKey))
			return pois, nil
		}
	}

	s.logger.Info("Querying activities from database",
		zap.Float64("lat", lat),
		zap.Float64("lon", lon),
		zap.Float64("distance", distance))

	// Get activities from database with filters
	activities, err := s.repo.GetPOIsByLocationAndDistanceWithCategory(ctx, lat, lon, distance, "activity")
	if err == nil && len(activities) > 0 {
		// Apply domain-specific filters
		filteredActivities := s.filterActivities(activities, activityType, duration)

		// Mark as database source
		for i := range filteredActivities {
			filteredActivities[i].Source = "points_of_interest"
		}

		s.cache.Set(cacheKey, filteredActivities, cache.DefaultExpiration)
		return filteredActivities, nil
	}

	s.logger.Info("No activities found in database, falling back to LLM generation")

	// Generate activities using LLM with domain-specific prompt
	genAIResponse, err := s.generateActivitiesFromLLM(ctx, userID, lat, lon, distance, activityType, duration)
	if err != nil {
		span.RecordError(err)
		return nil, err
	}

	enrichedActivities := s.enrichAndFilterLLMResponse(genAIResponse.GeneralPOI, lat, lon, distance)
	for i := range enrichedActivities {
		enrichedActivities[i].Source = "llm_suggested_pois"
	}

	s.cache.Set(cacheKey, enrichedActivities, cache.DefaultExpiration)
	return enrichedActivities, nil
}

func (s *Service) generateActivitiesFromLLM(ctx context.Context, userID uuid.UUID, lat, lon, distance float64, _, _ string) (*GenAIResponse, error) {
	resultCh := make(chan GenAIResponse, 1)
	var wg sync.WaitGroup
	wg.Add(1)
	go s.getGeneralActivitiesByDistance(&wg, ctx, userID, lat, lon, distance, resultCh, &genai.GenerateContentConfig{
		Temperature:     genai.Ptr[float32](0.7),
		MaxOutputTokens: 16384,
	})
	wg.Wait()
	close(resultCh)

	result := <-resultCh
	if result.Err != nil {
		return nil, result.Err
	}
	return &result, nil
}

func (s *Service) getGeneralActivitiesByDistance(wg *sync.WaitGroup,
	ctx context.Context,
	userID uuid.UUID,
	lat, lon, distance float64,
	resultCh chan<- GenAIResponse,
	config *genai.GenerateContentConfig) {
	ctx, span := otel.Tracer("LlmInteractionService").Start(ctx, "GenerateGeneralPOIWorker", trace.WithAttributes(
		attribute.Float64("latitude", lat),
		attribute.Float64("longitude", lon),
		attribute.Float64("distance.km", distance),
		attribute.String("user.id", userID.String())))

	defer span.End()
	defer wg.Done()

	userLocation := UserLocation{
		UserLat:        lat,
		UserLon:        lon,
		SearchRadiusKm: distance,
	}
	prompt := getActivitiesNearbyPrompt(userLocation)
	span.SetAttributes(attribute.Int("prompt.length", len(prompt)))

	if s.aiClient == nil {
		err := fmt.Errorf("AI client is not available - check API key configuration")
		span.RecordError(err)
		span.SetStatus(codes.Error, "AI client unavailable")
		resultCh <- GenAIResponse{Err: err}
		return
	}

	startTime := time.Now()
	response, err := s.aiClient.GenerateResponse(ctx, prompt, config)
	latencyMs := int(time.Since(startTime).Milliseconds())
	span.SetAttributes(attribute.Int("response.latency_ms", latencyMs))

	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to generate general POIs")
		resultCh <- GenAIResponse{Err: fmt.Errorf("failed to generate general POIs: %w", err)}
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
		resultCh <- GenAIResponse{Err: err}
		return
	}
	span.SetAttributes(attribute.Int("response.length", len(txt)))

	cleanTxt := cleanJSONResponse(txt)
	var poiData struct {
		PointsOfInterest []POIDetailedInfo `json:"points_of_interest"`
	}
	if err := json.Unmarshal([]byte(cleanTxt), &poiData); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to parse general POI JSON")
		resultCh <- GenAIResponse{Err: fmt.Errorf("failed to parse general POI JSON: %w", err)}
		return
	}

	fmt.Println(cleanTxt)

	span.SetAttributes(attribute.Int("pois.count", len(poiData.PointsOfInterest)))
	span.SetStatus(codes.Ok, "General POIs generated successfully")
	resultCh <- GenAIResponse{
		GeneralPOI: poiData.PointsOfInterest,
		ModelName:  s.aiClient.ModelName,
		Prompt:     prompt,
		Response:   cleanTxt,
	}
}

func (s *Service) filterActivities(activities []POIDetailedInfo, activityType, duration string) []POIDetailedInfo {
	if activityType == "" && duration == "" {
		return activities
	}

	filtered := make([]POIDetailedInfo, 0)
	for _, activity := range activities {
		// Filter by activity type
		if activityType != "" && activity.Category != activityType {
			continue
		}
		// Filter by duration (using description as proxy for duration since TimeToSpend field doesn't exist)
		if duration != "" && !strings.Contains(strings.ToLower(activity.Description), strings.ToLower(duration)) {
			continue
		}
		filtered = append(filtered, activity)
	}
	return filtered
}

// Hotels

func (s *Service) GetNearbyHotels(ctx context.Context, userID uuid.UUID, lat, lon, distance float64, starRating, amenities string) ([]POIDetailedInfo, error) {
	ctx, span := otel.Tracer("POIService").Start(ctx, "GetNearbyHotels", trace.WithAttributes(
		attribute.Float64("location.lat", lat),
		attribute.Float64("location.lon", lon),
		attribute.Float64("distance", distance),
		attribute.String("star_rating", starRating),
		attribute.String("amenities", amenities),
	))
	defer span.End()

	// Build cache key with domain-specific filters
	cacheKey := fmt.Sprintf("hotels_%f_%f_%f_%s_%s_%s", lat, lon, distance, userID.String(), starRating, amenities)

	if cached, found := s.cache.Get(cacheKey); found {
		if pois, ok := cached.([]POIDetailedInfo); ok {
			s.logger.Info("Serving hotels from cache")
			return pois, nil
		}
	}

	s.logger.Info("Querying hotels from database")

	// Get hotels from database with filters
	hotels, err := s.repo.GetPOIsByLocationAndDistanceWithCategory(ctx, lat, lon, distance, "hotel")
	if err == nil && len(hotels) > 0 {
		// Apply domain-specific filters
		filteredHotels := s.filterHotels(hotels, starRating, amenities)

		// Mark as database source
		for i := range filteredHotels {
			filteredHotels[i].Source = "points_of_interest"
		}

		s.cache.Set(cacheKey, filteredHotels, cache.DefaultExpiration)
		return filteredHotels, nil
	}

	s.logger.Info("No hotels found in database, falling back to LLM generation")

	// Generate hotels using LLM with domain-specific prompt
	genAIResponse, err := s.generateHotelsFromLLM(ctx, userID, lat, lon, distance, starRating, amenities)
	if err != nil {
		span.RecordError(err)
		return nil, err
	}

	enrichedHotels := s.enrichAndFilterLLMResponse(genAIResponse.GeneralPOI, lat, lon, distance)
	for i := range enrichedHotels {
		enrichedHotels[i].Source = "llm_suggested_pois"
	}

	s.cache.Set(cacheKey, enrichedHotels, cache.DefaultExpiration)
	return enrichedHotels, nil
}

func (s *Service) filterHotels(hotels []POIDetailedInfo, starRating, amenities string) []POIDetailedInfo {
	if starRating == "" && amenities == "" {
		return hotels
	}

	filtered := make([]POIDetailedInfo, 0)
	for _, hotel := range hotels {
		// Filter by star rating
		if starRating != "" && hotel.PriceLevel != starRating {
			continue
		}
		// Filter by amenities (basic string matching)
		if amenities != "" {
			if !strings.Contains(strings.ToLower(hotel.Amenities), strings.ToLower(amenities)) {
				continue
			}
		}
		filtered = append(filtered, hotel)
	}
	return filtered
}

func (s *Service) generateHotelsFromLLM(ctx context.Context, userID uuid.UUID, lat, lon, distance float64, _, _ string) (*GenAIResponse, error) {
	resultCh := make(chan GenAIResponse, 1)
	var wg sync.WaitGroup
	wg.Add(1)
	go s.getGeneralHotelsByDistance(&wg, ctx, userID, lat, lon, distance, resultCh, &genai.GenerateContentConfig{
		Temperature:     genai.Ptr[float32](0.7),
		MaxOutputTokens: 16384,
	})
	wg.Wait()
	close(resultCh)

	result := <-resultCh
	if result.Err != nil {
		return nil, result.Err
	}
	return &result, nil
}

func (s *Service) getGeneralHotelsByDistance(wg *sync.WaitGroup,
	ctx context.Context,
	userID uuid.UUID,
	lat, lon, distance float64,
	resultCh chan<- GenAIResponse,
	config *genai.GenerateContentConfig) {
	ctx, span := otel.Tracer("LlmInteractionService").Start(ctx, "GenerateGeneralPOIWorker", trace.WithAttributes(
		attribute.Float64("latitude", lat),
		attribute.Float64("longitude", lon),
		attribute.Float64("distance.km", distance),
		attribute.String("user.id", userID.String())))

	defer span.End()
	defer wg.Done()

	userLocation := UserLocation{
		UserLat:        lat,
		UserLon:        lon,
		SearchRadiusKm: distance,
	}
	prompt := getHotelsNeabyPrompt(userLocation)
	span.SetAttributes(attribute.Int("prompt.length", len(prompt)))

	if s.aiClient == nil {
		err := fmt.Errorf("AI client is not available - check API key configuration")
		span.RecordError(err)
		span.SetStatus(codes.Error, "AI client unavailable")
		resultCh <- GenAIResponse{Err: err}
		return
	}

	startTime := time.Now()
	response, err := s.aiClient.GenerateResponse(ctx, prompt, config)
	latencyMs := int(time.Since(startTime).Milliseconds())
	span.SetAttributes(attribute.Int("response.latency_ms", latencyMs))

	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to generate general POIs")
		resultCh <- GenAIResponse{Err: fmt.Errorf("failed to generate general POIs: %w", err)}
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
		resultCh <- GenAIResponse{Err: err}
		return
	}
	span.SetAttributes(attribute.Int("response.length", len(txt)))

	cleanTxt := cleanJSONResponse(txt)
	var poiData struct {
		PointsOfInterest []POIDetailedInfo `json:"points_of_interest"`
	}
	if err := json.Unmarshal([]byte(cleanTxt), &poiData); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to parse general POI JSON")
		resultCh <- GenAIResponse{Err: fmt.Errorf("failed to parse general POI JSON: %w", err)}
		return
	}

	fmt.Println(cleanTxt)

	span.SetAttributes(attribute.Int("pois.count", len(poiData.PointsOfInterest)))
	span.SetStatus(codes.Ok, "General POIs generated successfully")
	resultCh <- GenAIResponse{
		GeneralPOI: poiData.PointsOfInterest,
		ModelName:  s.aiClient.ModelName,
		Prompt:     prompt,
		Response:   cleanTxt,
	}
}
