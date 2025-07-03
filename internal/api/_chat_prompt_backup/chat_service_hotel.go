package llmChat

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/FACorreiaa/go-poi-au-suggestions/internal/types"
	"github.com/google/uuid"
	"github.com/patrickmn/go-cache"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/genai"
)

// var cacheHitCounter = metric.NewCounter("cache_hits", metric.WithDescription("Number of cache hits"))
// var dbHitCounter = metric.NewCounter("db_hits", metric.WithDescription("Number of database hits"))
// var aiCallCounter = metric.NewCounter("ai_calls", metric.WithDescription("Number of AI calls"))

func (l *ServiceImpl) getHotelsByPreferenceDetails(wg *sync.WaitGroup, ctx context.Context,
	city string, lat float64, lon float64, userID uuid.UUID, userPreferences types.HotelUserPreferences,
	resultCh chan<- []types.HotelDetailedInfo, config *genai.GenerateContentConfig) {
	defer wg.Done()
	ctx, span := otel.Tracer("LlmInteractionService").Start(ctx, "getHotelsByPreferenceDetails", trace.WithAttributes(
		attribute.String("city.name", city),
		attribute.Float64("latitude", lat),
		attribute.Float64("longitude", lon),
	))
	defer span.End()

	if city == "" || lat == 0 || lon == 0 {
		resultCh <- []types.HotelDetailedInfo{{Err: fmt.Errorf("invalid input: city, lat, or lon is empty")}}
		span.SetStatus(codes.Error, "Invalid input")
		return
	}

	startTime := time.Now()

	prompt := getHotelsByPreferencesPrompt(city, lat, lon, userPreferences)
	span.SetAttributes(attribute.Int("prompt.length", len(prompt)))
	response, err := l.aiClient.GenerateResponse(ctx, prompt, config)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to generate hotel details")
		resultCh <- []types.HotelDetailedInfo{{Err: fmt.Errorf("failed to generate hotel details: %w", err)}}
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
		err := fmt.Errorf("no valid hotel details content from AI")
		span.RecordError(err)
		span.SetStatus(codes.Error, "Empty response from AI")
		resultCh <- []types.HotelDetailedInfo{{Err: err}}
		return
	}

	span.SetAttributes(attribute.Int("response.length", len(txt)))
	cleanTxt := cleanJSONResponse(txt)
	var hotelResponse struct {
		Hotels []types.HotelDetailedInfo `json:"hotels"`
	}
	if err := json.Unmarshal([]byte(cleanTxt), &hotelResponse); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to parse hotel details JSON")
		resultCh <- []types.HotelDetailedInfo{{Err: fmt.Errorf("failed to parse hotel details JSON: %w", err)}}
		return
	}

	if len(hotelResponse.Hotels) == 0 {
		err := fmt.Errorf("no hotels returned from AI")
		span.RecordError(err)
		span.SetStatus(codes.Error, "No hotels found")
		resultCh <- []types.HotelDetailedInfo{{Err: err}}
		return
	}

	latencyMs := int(time.Since(startTime).Milliseconds())
	span.SetAttributes(attribute.Int("response.latency_ms", latencyMs))

	interaction := types.LlmInteraction{
		UserID:       userID,
		Prompt:       prompt,
		ResponseText: txt,
		ModelUsed:    model, // Adjust based on AI client
		LatencyMs:    latencyMs,
		CityName:     city,
	}
	savedInteractionID, err := l.llmInteractionRepo.SaveInteraction(ctx, interaction)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to save LLM interaction")
		resultCh <- []types.HotelDetailedInfo{{Err: fmt.Errorf("failed to save LLM interaction: %w", err)}}
		return
	}

	for i := range hotelResponse.Hotels {
		hotelResponse.Hotels[i].ID = uuid.New()
		hotelResponse.Hotels[i].City = city
		hotelResponse.Hotels[i].LlmInteractionID = savedInteractionID
	}

	resultCh <- hotelResponse.Hotels
	span.SetAttributes(attribute.String("llm_interaction.id", savedInteractionID.String()))
	span.SetStatus(codes.Ok, "Hotel details generated and saved successfully")
}

func (l *ServiceImpl) GetHotelsByPreferenceResponse(ctx context.Context, userID uuid.UUID, city string, lat, lon float64, userPreferences types.HotelUserPreferences) ([]types.HotelDetailedInfo, error) {
	ctx, span := otel.Tracer("LlmInteractionService").Start(ctx, "GetHotelsByPreferenceResponse", trace.WithAttributes(
		attribute.String("city.name", city),
		attribute.Float64("latitude", lat),
		attribute.Float64("longitude", lon),
		attribute.String("user.id", userID.String()),
	))
	defer span.End()

	l.logger.DebugContext(ctx, "Starting hotel details generation by preferences",
		slog.String("city", city), slog.Float64("latitude", lat), slog.Float64("longitude", lon), slog.String("userID", userID.String()))

	// Generate cache key
	cacheKey := generateHotelCacheKey(city, lat, lon, userID)
	span.SetAttributes(attribute.String("cache.key", cacheKey))

	// Check cache
	if cached, found := l.cache.Get(cacheKey); found {
		if hotels, ok := cached.([]types.HotelDetailedInfo); ok {
			l.logger.InfoContext(ctx, "Cache hit for hotel details", slog.String("cache_key", cacheKey))
			span.AddEvent("Cache hit")
			span.SetStatus(codes.Ok, "Hotel details served from cache")
			return hotels, nil
		}
	}

	// Find city ID
	cityData, err := l.cityRepo.FindCityByNameAndCountry(ctx, city, "")
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
	hotels, err := l.poiRepo.FindHotelDetails(ctx, cityID, lat, lon, 1000.0) // 1km tolerance
	if err != nil {
		l.logger.ErrorContext(ctx, "Failed to query hotel details from database", slog.Any("error", err))
		span.RecordError(err)
		return nil, fmt.Errorf("failed to query hotel details: %w", err)
	}
	if len(hotels) > 0 {
		for i := range hotels {
			hotels[i].City = city
		}
		l.cache.Set(cacheKey, hotels, cache.DefaultExpiration)
		l.logger.InfoContext(ctx, "Database hit for hotel details", slog.String("cache_key", cacheKey))
		span.AddEvent("Database hit")
		span.SetStatus(codes.Ok, "Hotel details served from database")
		return hotels, nil
	}

	// Cache and database miss: fetch from AI
	l.logger.DebugContext(ctx, "Cache and database miss, fetching hotel details from AI", slog.String("cache_key", cacheKey))
	span.AddEvent("Cache and database miss")

	resultCh := make(chan []types.HotelDetailedInfo, 1)
	var wg sync.WaitGroup
	wg.Add(1)

	go l.getHotelsByPreferenceDetails(&wg, ctx, city, lat, lon, userID, userPreferences, resultCh, &genai.GenerateContentConfig{Temperature: genai.Ptr[float32](defaultTemperature)})

	go func() {
		wg.Wait()
		close(resultCh)
	}()

	var hotelResults []types.HotelDetailedInfo
	for res := range resultCh {
		if res[0].Err != nil {
			l.logger.ErrorContext(ctx, "Error generating hotel details", slog.Any("error", res[0].Err))
			span.RecordError(res[0].Err)
			span.SetStatus(codes.Error, "Failed to generate hotel details")
			return nil, res[0].Err
		}
		hotelResults = res
		break
	}

	if len(hotelResults) == 0 {
		l.logger.WarnContext(ctx, "No hotels received for hotel details")
		span.SetStatus(codes.Error, "No hotels received")
		return nil, fmt.Errorf("no hotels received for hotel details")
	}

	// Save to database
	for _, hotel := range hotelResults {
		_, err = l.poiRepo.SaveHotelDetails(ctx, hotel, cityID)
		if err != nil {
			l.logger.WarnContext(ctx, "Failed to save hotel details to database", slog.Any("error", err))
			span.RecordError(err)
			// Continue despite error
		}
	}

	// Store in cache
	l.cache.Set(cacheKey, hotelResults, cache.DefaultExpiration)
	l.logger.DebugContext(ctx, "Stored hotel details in cache", slog.String("cache_key", cacheKey))
	span.AddEvent("Stored in cache")

	span.SetStatus(codes.Ok, "Hotel details generated and cached successfully")
	return hotelResults, nil
}

func (l *ServiceImpl) getHotelsNearby(wg *sync.WaitGroup, ctx context.Context,
	city string, lat float64, lon float64, userID uuid.UUID,
	resultCh chan<- []types.HotelDetailedInfo, config *genai.GenerateContentConfig) {
	defer wg.Done()
	ctx, span := otel.Tracer("LlmInteractionService").Start(ctx, "getHotelsNearby", trace.WithAttributes(
		attribute.String("city.name", city),
		attribute.Float64("latitude", lat),
		attribute.Float64("longitude", lon),
	))
	defer span.End()

	if city == "" || lat == 0 || lon == 0 {
		resultCh <- []types.HotelDetailedInfo{{Err: fmt.Errorf("invalid input: city, lat, or lon is empty")}}
		span.SetStatus(codes.Error, "Invalid input")
		return
	}

	startTime := time.Now()

	userLocation := types.UserLocation{
		UserLat: lat,
		UserLon: lon,
	}
	prompt := getHotelsNeabyPrompt(city, userLocation)
	span.SetAttributes(attribute.Int("prompt.length", len(prompt)))
	response, err := l.aiClient.GenerateResponse(ctx, prompt, config)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to generate hotel details")
		resultCh <- []types.HotelDetailedInfo{{Err: fmt.Errorf("failed to generate hotel details: %w", err)}}
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
		err := fmt.Errorf("no valid hotel details content from AI")
		span.RecordError(err)
		span.SetStatus(codes.Error, "Empty response from AI")
		resultCh <- []types.HotelDetailedInfo{{Err: err}}
		return
	}

	span.SetAttributes(attribute.Int("response.length", len(txt)))
	cleanTxt := cleanJSONResponse(txt)
	var hotelResponse struct {
		Hotels []types.HotelDetailedInfo `json:"hotels"`
	}
	if err := json.Unmarshal([]byte(cleanTxt), &hotelResponse); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to parse hotel details JSON")
		resultCh <- []types.HotelDetailedInfo{{Err: fmt.Errorf("failed to parse hotel details JSON: %w", err)}}
		return
	}

	if len(hotelResponse.Hotels) == 0 {
		err := fmt.Errorf("no hotels returned from AI")
		span.RecordError(err)
		span.SetStatus(codes.Error, "No hotels found")
		resultCh <- []types.HotelDetailedInfo{{Err: err}}
		return
	}

	latencyMs := int(time.Since(startTime).Milliseconds())
	span.SetAttributes(attribute.Int("response.latency_ms", latencyMs))

	interaction := types.LlmInteraction{
		UserID:       userID,
		Prompt:       prompt,
		ResponseText: txt,
		ModelUsed:    model, // Adjust based on AI client
		LatencyMs:    latencyMs,
		CityName:     city,
	}
	savedInteractionID, err := l.llmInteractionRepo.SaveInteraction(ctx, interaction)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to save LLM interaction")
		resultCh <- []types.HotelDetailedInfo{{Err: fmt.Errorf("failed to save LLM interaction: %w", err)}}
		return
	}

	for i := range hotelResponse.Hotels {
		hotelResponse.Hotels[i].ID = uuid.New()
		hotelResponse.Hotels[i].City = city
		hotelResponse.Hotels[i].LlmInteractionID = savedInteractionID
	}

	resultCh <- hotelResponse.Hotels
	span.SetAttributes(attribute.String("llm_interaction.id", savedInteractionID.String()))
	span.SetStatus(codes.Ok, "Hotel details generated and saved successfully")
}

func (l *ServiceImpl) GetHotelsNearbyResponse(ctx context.Context, userID uuid.UUID, city string, userLocation *types.UserLocation) ([]types.HotelDetailedInfo, error) {
	lat := userLocation.UserLat
	lon := userLocation.UserLon
	distance := userLocation.SearchRadiusKm
	ctx, span := otel.Tracer("LlmInteractionService").Start(ctx, "GetHotelsByPreferenceResponse", trace.WithAttributes(
		attribute.String("city.name", city),
		attribute.Float64("latitude", lat),
		attribute.Float64("longitude", lon),
		attribute.String("user.id", userID.String()),
	))
	defer span.End()

	l.logger.DebugContext(ctx, "Starting hotel details generation by preferences",
		slog.String("city", city), slog.Float64("latitude", lat), slog.Float64("longitude", lon), slog.String("userID", userID.String()))

	// Generate cache key
	cacheKey := generateHotelCacheKey(city, lat, lon, userID)
	span.SetAttributes(attribute.String("cache.key", cacheKey))

	// Check cache
	if cached, found := l.cache.Get(cacheKey); found {
		if hotels, ok := cached.([]types.HotelDetailedInfo); ok {
			l.logger.InfoContext(ctx, "Cache hit for hotel details", slog.String("cache_key", cacheKey))
			span.AddEvent("Cache hit")
			span.SetStatus(codes.Ok, "Hotel details served from cache")
			return hotels, nil
		}
	}

	// Find city ID
	cityData, err := l.cityRepo.FindCityByNameAndCountry(ctx, city, "")
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
	hotels, err := l.poiRepo.FindHotelDetails(ctx, cityID, lat, lon, distance) // 1km tolerance
	if err != nil {
		l.logger.ErrorContext(ctx, "Failed to query hotel details from database", slog.Any("error", err))
		span.RecordError(err)
		return nil, fmt.Errorf("failed to query hotel details: %w", err)
	}
	if len(hotels) > 0 {
		for i := range hotels {
			hotels[i].City = city
		}
		l.cache.Set(cacheKey, hotels, cache.DefaultExpiration)
		l.logger.InfoContext(ctx, "Database hit for hotel details", slog.String("cache_key", cacheKey))
		span.AddEvent("Database hit")
		span.SetStatus(codes.Ok, "Hotel details served from database")
		return hotels, nil
	}

	// Cache and database miss: fetch from AI
	l.logger.DebugContext(ctx, "Cache and database miss, fetching hotel details from AI", slog.String("cache_key", cacheKey))
	span.AddEvent("Cache and database miss")

	resultCh := make(chan []types.HotelDetailedInfo, 1)
	var wg sync.WaitGroup
	wg.Add(1)

	go l.getHotelsNearby(&wg, ctx, city, lat, lon, userID, resultCh, &genai.GenerateContentConfig{Temperature: genai.Ptr[float32](defaultTemperature)})

	go func() {
		wg.Wait()
		close(resultCh)
	}()

	var hotelResults []types.HotelDetailedInfo
	for res := range resultCh {
		if res[0].Err != nil {
			l.logger.ErrorContext(ctx, "Error generating hotel details", slog.Any("error", res[0].Err))
			span.RecordError(res[0].Err)
			span.SetStatus(codes.Error, "Failed to generate hotel details")
			return nil, res[0].Err
		}
		hotelResults = res
		break
	}

	if len(hotelResults) == 0 {
		l.logger.WarnContext(ctx, "No hotels received for hotel details")
		span.SetStatus(codes.Error, "No hotels received")
		return nil, fmt.Errorf("no hotels received for hotel details")
	}

	// Save to database
	for _, hotel := range hotelResults {
		_, err = l.poiRepo.SaveHotelDetails(ctx, hotel, cityID)
		if err != nil {
			l.logger.WarnContext(ctx, "Failed to save hotel details to database", slog.Any("error", err))
			span.RecordError(err)
			// Continue despite error
		}
	}

	// Store in cache
	l.cache.Set(cacheKey, hotelResults, cache.DefaultExpiration)
	l.logger.DebugContext(ctx, "Stored hotel details in cache", slog.String("cache_key", cacheKey))
	span.AddEvent("Stored in cache")

	span.SetStatus(codes.Ok, "Hotel details generated and cached successfully")
	return hotelResults, nil
}

func (s *ServiceImpl) GetHotelByIDResponse(ctx context.Context, hotelID uuid.UUID) (*types.HotelDetailedInfo, error) {
	hotel, err := s.poiRepo.GetHotelByID(ctx, hotelID)
	if err != nil {
		s.logger.Error("failed to get hotel by ID", "error", err)
		return nil, err
	}
	return hotel, nil
}
