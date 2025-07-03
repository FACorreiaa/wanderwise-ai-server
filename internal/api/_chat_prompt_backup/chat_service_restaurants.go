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
	"google.golang.org/genai"
)

func (l *ServiceImpl) getRestaurantsByPreferences(wg *sync.WaitGroup, ctx context.Context,
	city string, lat, lon float64, userID uuid.UUID, preferences types.RestaurantUserPreferences,
	resultCh chan<- []types.RestaurantDetailedInfo, config *genai.GenerateContentConfig) {
	defer wg.Done()
	ctx, span := otel.Tracer("LlmInteractionService").Start(ctx, "getRestaurantsByPreferences")
	defer span.End()

	if city == "" || lat == 0 || lon == 0 {
		resultCh <- []types.RestaurantDetailedInfo{{Err: fmt.Errorf("invalid input: city, lat, or lon is empty")}}
		span.SetStatus(codes.Error, "Invalid input")
		return
	}

	startTime := time.Now()
	prompt := getRestaurantsByPreferencesPrompt(city, lat, lon, preferences)
	response, err := l.aiClient.GenerateResponse(ctx, prompt, config)
	if err != nil {
		span.RecordError(err)
		resultCh <- []types.RestaurantDetailedInfo{{Err: fmt.Errorf("failed to generate restaurant details: %w", err)}}
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
		err := fmt.Errorf("no valid restaurant details from AI")
		span.RecordError(err)
		resultCh <- []types.RestaurantDetailedInfo{{Err: err}}
		return
	}

	cleanTxt := cleanJSONResponse(txt)
	var restaurantResponse struct {
		Restaurants []types.RestaurantDetailedInfo `json:"restaurants"`
	}
	if err := json.Unmarshal([]byte(cleanTxt), &restaurantResponse); err != nil {
		span.RecordError(err)
		resultCh <- []types.RestaurantDetailedInfo{{Err: fmt.Errorf("failed to parse restaurant JSON: %w", err)}}
		return
	}

	interaction := types.LlmInteraction{
		UserID:       userID,
		Prompt:       prompt,
		ResponseText: txt,
		ModelUsed:    "model_name", // Adjust as needed
		LatencyMs:    int(time.Since(startTime).Milliseconds()),
		CityName:     city,
	}
	savedInteractionID, err := l.llmInteractionRepo.SaveInteraction(ctx, interaction)
	if err != nil {
		span.RecordError(err)
		resultCh <- []types.RestaurantDetailedInfo{{Err: fmt.Errorf("failed to save LLM interaction: %w", err)}}
		return
	}

	for i := range restaurantResponse.Restaurants {
		restaurantResponse.Restaurants[i].ID = uuid.New()
		restaurantResponse.Restaurants[i].LlmInteractionID = savedInteractionID
	}
	resultCh <- restaurantResponse.Restaurants
	span.SetStatus(codes.Ok, "Restaurants generated successfully")
}

func (l *ServiceImpl) GetRestaurantsByPreferencesResponse(ctx context.Context, userID uuid.UUID, city string, lat, lon float64, preferences types.RestaurantUserPreferences) ([]types.RestaurantDetailedInfo, error) {
	ctx, span := otel.Tracer("LlmInteractionService").Start(ctx, "GetRestaurantsByPreferencesResponse")
	defer span.End()

	cacheKey := generateRestaurantCacheKey(city, lat, lon, userID)
	span.SetAttributes(attribute.String("cache.key", cacheKey))

	if cached, found := l.cache.Get(cacheKey); found {
		if restaurants, ok := cached.([]types.RestaurantDetailedInfo); ok {
			span.SetStatus(codes.Ok, "Served from cache")
			return restaurants, nil
		}
	}

	cityData, err := l.cityRepo.FindCityByNameAndCountry(ctx, city, "")
	if err != nil || cityData == nil {
		span.RecordError(err)
		return nil, fmt.Errorf("city %s not found: %w", city, err)
	}
	cityID := cityData.ID

	restaurants, err := l.poiRepo.FindRestaurantDetails(ctx, cityData.ID, lat, lon, 5000.0, &preferences) // 5km tolerance
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("failed to query restaurants: %w", err)
	}
	if len(restaurants) > 0 {
		l.cache.Set(cacheKey, restaurants, cache.DefaultExpiration)
		span.SetStatus(codes.Ok, "Served from database")
		return restaurants, nil
	}

	resultCh := make(chan []types.RestaurantDetailedInfo, 1)
	var wg sync.WaitGroup
	wg.Add(1)
	go l.getRestaurantsByPreferences(&wg, ctx, city, lat, lon, userID, preferences, resultCh, nil)
	wg.Wait()
	close(resultCh)

	restaurants = <-resultCh
	if restaurants[0].Err != nil {
		span.RecordError(restaurants[0].Err)
		return nil, restaurants[0].Err
	}

	for _, r := range restaurants {
		_, err := l.poiRepo.SaveRestaurantDetails(ctx, r, cityID)
		if err != nil {
			l.logger.WarnContext(ctx, "Failed to save restaurant details to database", slog.Any("error", err), slog.String("restaurant_name", r.Name))
			span.RecordError(err)
			// Continue despite error
		}
	}
	l.cache.Set(cacheKey, restaurants, cache.DefaultExpiration)
	span.SetStatus(codes.Ok, "Restaurants generated and cached")
	return restaurants, nil
}

func (l *ServiceImpl) getRestaurantsNearby(wg *sync.WaitGroup, ctx context.Context,
	city string, lat float64, lon float64, userID uuid.UUID, resultCh chan<- []types.RestaurantDetailedInfo, config *genai.GenerateContentConfig) {
	defer wg.Done()
	ctx, span := otel.Tracer("LlmInteractionService").Start(ctx, "getRestaurantsNearby")
	defer span.End()

	// Validate input parameters
	if city == "" || lat == 0 || lon == 0 {
		resultCh <- []types.RestaurantDetailedInfo{{Err: fmt.Errorf("invalid input: city, lat, or lon is empty")}}
		span.SetStatus(codes.Error, "Invalid input")
		return
	}

	// Define user location with a default search radius of 5.0 km
	userLocation := types.UserLocation{UserLat: lat, UserLon: lon, SearchRadiusKm: 5.0}
	prompt := getRestaurantsNearbyPrompt(city, userLocation)
	span.SetAttributes(attribute.Int("prompt.length", len(prompt)))

	// Record the start time for latency calculation
	startTime := time.Now()

	// Generate response from the AI client
	response, err := l.aiClient.GenerateResponse(ctx, prompt, config)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to generate restaurant details")
		resultCh <- []types.RestaurantDetailedInfo{{Err: fmt.Errorf("failed to generate restaurant details: %w", err)}}
		return
	}

	// Extract text content from the AI response
	var txt string
	for _, candidate := range response.Candidates {
		if candidate.Content != nil && len(candidate.Content.Parts) > 0 {
			txt = candidate.Content.Parts[0].Text
			break
		}
	}
	if txt == "" {
		err := fmt.Errorf("no valid restaurant details content from AI")
		span.RecordError(err)
		span.SetStatus(codes.Error, "Empty response from AI")
		resultCh <- []types.RestaurantDetailedInfo{{Err: err}}
		return
	}

	// Log response length
	span.SetAttributes(attribute.Int("response.length", len(txt)))

	// Parse the JSON response
	cleanTxt := cleanJSONResponse(txt)
	var restaurantResponse struct {
		Restaurants []types.RestaurantDetailedInfo `json:"restaurants"`
	}
	if err := json.Unmarshal([]byte(cleanTxt), &restaurantResponse); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to parse restaurant details JSON")
		resultCh <- []types.RestaurantDetailedInfo{{Err: fmt.Errorf("failed to parse restaurant details JSON: %w", err)}}
		return
	}

	// Check if any restaurants were returned
	if len(restaurantResponse.Restaurants) == 0 {
		err := fmt.Errorf("no restaurants returned from AI")
		span.RecordError(err)
		span.SetStatus(codes.Error, "No restaurants found")
		resultCh <- []types.RestaurantDetailedInfo{{Err: err}}
		return
	}

	// Calculate latency
	latencyMs := int(time.Since(startTime).Milliseconds())
	span.SetAttributes(attribute.Int("response.latency_ms", latencyMs))

	// Save interaction details
	interaction := types.LlmInteraction{
		UserID:       userID,
		Prompt:       prompt,
		ResponseText: txt,
		ModelUsed:    "default-model", // Replace with actual model name from your AI client
		LatencyMs:    latencyMs,
		CityName:     city,
	}
	savedInteractionID, err := l.llmInteractionRepo.SaveInteraction(ctx, interaction)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to save LLM interaction")
		resultCh <- []types.RestaurantDetailedInfo{{Err: fmt.Errorf("failed to save LLM interaction: %w", err)}}
		return
	}

	// Assign IDs and city to each restaurant
	for i := range restaurantResponse.Restaurants {
		restaurantResponse.Restaurants[i].ID = uuid.New()
		restaurantResponse.Restaurants[i].City = city
		restaurantResponse.Restaurants[i].LlmInteractionID = savedInteractionID
	}

	// Send results to the channel
	resultCh <- restaurantResponse.Restaurants
	span.SetStatus(codes.Ok, "Restaurant details generated successfully")
}

func (l *ServiceImpl) GetRestaurantsNearbyResponse(ctx context.Context, userID uuid.UUID, city string, userLocation types.UserLocation) ([]types.RestaurantDetailedInfo, error) {
	lat := userLocation.UserLat
	lon := userLocation.UserLon
	distance := userLocation.SearchRadiusKm
	if distance == 0 {
		distance = 5.0 // Default to 5km
	}

	ctx, span := otel.Tracer("LlmInteractionService").Start(ctx, "GetRestaurantsNearbyResponse")
	defer span.End()

	l.logger.DebugContext(ctx, "Starting nearby restaurant details generation",
		slog.String("city", city), slog.Float64("latitude", lat), slog.Float64("longitude", lon), slog.String("userID", userID.String()))

	// Generate cache key
	cacheKey := fmt.Sprintf("restaurants:nearby:%s:%.6f:%.6f:%s", city, lat, lon, userID.String())
	span.SetAttributes(attribute.String("cache.key", cacheKey))

	// Check cache
	if cached, found := l.cache.Get(cacheKey); found {
		if restaurants, ok := cached.([]types.RestaurantDetailedInfo); ok {
			l.logger.InfoContext(ctx, "Cache hit for nearby restaurant details", slog.String("cache_key", cacheKey))
			span.AddEvent("Cache hit")
			span.SetStatus(codes.Ok, "Restaurant details served from cache")
			return restaurants, nil
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
	restaurants, err := l.poiRepo.FindRestaurantDetails(ctx, cityID, lat, lon, distance*1000, nil) // Convert km to meters
	if err != nil {
		l.logger.ErrorContext(ctx, "Failed to query restaurant details from database", slog.Any("error", err))
		span.RecordError(err)
		return nil, fmt.Errorf("failed to query restaurant details: %w", err)
	}
	if len(restaurants) > 0 {
		for i := range restaurants {
			restaurants[i].City = city
		}
		l.cache.Set(cacheKey, restaurants, cache.DefaultExpiration)
		l.logger.InfoContext(ctx, "Database hit for nearby restaurant details", slog.String("cache_key", cacheKey))
		span.AddEvent("Database hit")
		span.SetStatus(codes.Ok, "Restaurant details served from database")
		return restaurants, nil
	}

	// Cache and database miss: fetch from AI
	l.logger.DebugContext(ctx, "Cache and database miss, fetching nearby restaurant details from AI", slog.String("cache_key", cacheKey))
	span.AddEvent("Cache and database miss")

	resultCh := make(chan []types.RestaurantDetailedInfo, 1)
	var wg sync.WaitGroup
	wg.Add(1)

	go l.getRestaurantsNearby(&wg, ctx, city, lat, lon, userID, resultCh, nil)

	go func() {
		wg.Wait()
		close(resultCh)
	}()

	var restaurantResults []types.RestaurantDetailedInfo
	for res := range resultCh {
		if res[0].Err != nil {
			l.logger.ErrorContext(ctx, "Error generating nearby restaurant details", slog.Any("error", res[0].Err))
			span.RecordError(res[0].Err)
			span.SetStatus(codes.Error, "Failed to generate restaurant details")
			return nil, res[0].Err
		}
		restaurantResults = res
		break
	}

	if len(restaurantResults) == 0 {
		l.logger.WarnContext(ctx, "No restaurants received for nearby details")
		span.SetStatus(codes.Error, "No restaurants received")
		return nil, fmt.Errorf("no restaurants received for nearby details")
	}

	// Save to database
	for _, restaurant := range restaurantResults {
		_, err = l.poiRepo.SaveRestaurantDetails(ctx, restaurant, cityID)
		if err != nil {
			l.logger.WarnContext(ctx, "Failed to save restaurant details to database", slog.Any("error", err))
			span.RecordError(err)
			// Continue despite error
		}
	}

	// Cache the results
	l.cache.Set(cacheKey, restaurantResults, cache.DefaultExpiration)
	l.logger.DebugContext(ctx, "Stored nearby restaurant details in cache", slog.String("cache_key", cacheKey))
	span.AddEvent("Stored in cache")

	span.SetStatus(codes.Ok, "Nearby restaurant details generated and cached successfully")
	return restaurantResults, nil
}

func (l *ServiceImpl) GetRestaurantDetailsResponse(ctx context.Context, restaurantID uuid.UUID) (*types.RestaurantDetailedInfo, error) {
	ctx, span := otel.Tracer("LlmInteractionService").Start(ctx, "GetRestaurantDetailsResponse")
	defer span.End()

	restaurant, err := l.poiRepo.GetRestaurantByID(ctx, restaurantID)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("failed to get restaurant: %w", err)
	}
	if restaurant != nil {
		span.SetStatus(codes.Ok, "Restaurant found")
		return restaurant, nil
	}
	// AI generation logic can be added here if needed
	return nil, fmt.Errorf("restaurant not found")
}
