package chat_prompt

import (
	"context"
	"encoding/json"
	"fmt"
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

	"github.com/FACorreiaa/go-poi-au-suggestions/internal/domain/interests"
	"github.com/FACorreiaa/go-poi-au-suggestions/internal/domain/poi"
	"github.com/FACorreiaa/go-poi-au-suggestions/internal/domain/tags"
)

const (
	model              = "gemini-2.0-flash"
	defaultTemperature = 0.5
)

func (l *Service) FetchUserData(ctx context.Context, userID, profileID uuid.UUID) (interests []*interests.Interest, searchProfile *UserPreferenceProfileResponse, tags []*tags.Tags, err error) {
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

func (l *Service) GetBookmarkedItineraries(ctx context.Context, userID uuid.UUID, page, limit int) (*PaginatedUserItinerariesResponse, error) {
	ctx, span := otel.Tracer("LlmInteractionService").Start(ctx, "GetBookmarkedItineraries", trace.WithAttributes(
		attribute.String("user.id", userID.String()),
		attribute.Int("page", page),
		attribute.Int("limit", limit),
	))
	defer span.End()

	l.logger.Info("Retrieving bookmarked itineraries",
		zap.String("userID", userID.String()),
		zap.Int("page", page),
		zap.Int("limit", limit))

	response, err := l.repo.GetBookmarkedItineraries(ctx, userID, page, limit)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to retrieve bookmarked itineraries")
		return nil, fmt.Errorf("failed to retrieve bookmarked itineraries: %w", err)
	}

	l.logger.Info("Successfully retrieved bookmarked itineraries",
		zap.String("userID", userID.String()),
		zap.Int("totalRecords", response.TotalRecords),
		zap.Int("page", response.Page),
		zap.Int("pageSize", response.PageSize))

	span.SetAttributes(
		attribute.Int("total_records", response.TotalRecords),
		attribute.Int("returned_count", len(response.Itineraries)),
	)
	span.SetStatus(codes.Ok, "Bookmarked itineraries retrieved successfully")
	return response, nil
}

func (l *Service) GetUserChatSessions(ctx context.Context, userID uuid.UUID, page, limit int) (*ChatSessionsResponse, error) {
	ctx, span := otel.Tracer("LlmInteractionService").Start(ctx, "GetUserChatSessions", trace.WithAttributes(
		attribute.String("user.id", userID.String()),
		attribute.Int("page", page),
		attribute.Int("limit", limit),
	))
	defer span.End()

	l.logger.Info("Retrieving paginated chat sessions for user",
		zap.String("userID", userID.String()),
		zap.Int("page", page),
		zap.Int("limit", limit))

	response, err := l.repo.GetUserChatSessions(ctx, userID, page, limit)
	if err != nil {
		l.logger.Error("Failed to get user chat sessions", zap.Error(err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to get user chat sessions")
		return nil, fmt.Errorf("failed to get user chat sessions: %w", err)
	}

	l.logger.Info("Successfully retrieved paginated chat sessions",
		zap.String("userID", userID.String()),
		zap.Int("sessionCount", len(response.Sessions)),
		zap.Int("total", response.Total),
		zap.Int("page", response.Page),
		zap.Int("limit", response.Limit))
	span.SetAttributes(
		attribute.Int("sessions.count", len(response.Sessions)),
		attribute.Int("sessions.total", response.Total),
		attribute.Int("response.page", response.Page),
		attribute.Int("response.limit", response.Limit),
	)
	span.SetStatus(codes.Ok, "Chat sessions retrieved successfully")
	return response, nil
}

func (l *Service) getPOIDetailedInfos(wg *sync.WaitGroup, ctx context.Context,
	city string, lat float64, lon float64, userID uuid.UUID,
	resultCh chan<- POIDetailedInfo, config *genai.GenerateContentConfig) {
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
		resultCh <- POIDetailedInfo{Err: fmt.Errorf("failed to generate POI details: %w", err)}
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
		resultCh <- POIDetailedInfo{Err: err}
		return
	}

	span.SetAttributes(attribute.Int("response.length", len(txt)))
	cleanTxt := cleanJSONResponse(txt)
	var detailedInfo POIDetailedInfo
	if err := json.Unmarshal([]byte(cleanTxt), &detailedInfo); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to parse POI details JSON")
		resultCh <- POIDetailedInfo{Err: fmt.Errorf("failed to parse POI details JSON: %w", err)}
		return
	}
	latencyMs := int(time.Since(startTime).Milliseconds())
	span.SetAttributes(attribute.Int("response.latency_ms", latencyMs))
	span.SetStatus(codes.Ok, "POI details generated successfully")
	interaction := LlmInteraction{
		UserID:       userID,
		Prompt:       prompt,
		ResponseText: txt,
		ModelUsed:    model,
		LatencyMs:    latencyMs,
		CityName:     city,
	}

	savedInteractionID, err := l.repo.SaveInteraction(ctx, interaction)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to save LLM interaction for POI details")
		resultCh <- POIDetailedInfo{Err: fmt.Errorf("failed to save LLM interaction for POI details: %w", err)}
		return
	}
	resultCh <- POIDetailedInfo{
		City:             city,
		Name:             detailedInfo.Name,
		Latitude:         detailedInfo.Latitude,
		Longitude:        detailedInfo.Longitude,
		Description:      detailedInfo.Description,
		Address:          detailedInfo.Address,
		OpeningHours:     detailedInfo.OpeningHours,
		PhoneNumber:      detailedInfo.PhoneNumber,
		Website:          detailedInfo.Website,
		Rating:           detailedInfo.Rating,
		Tags:             detailedInfo.Tags,
		Images:           detailedInfo.Images,
		PriceRange:       detailedInfo.PriceRange,
		Err:              nil,
		LlmInteractionID: savedInteractionID,
	}
	span.SetAttributes(attribute.String("llm_interaction.id", savedInteractionID.String()))
	span.SetStatus(codes.Ok, "POI details generated and saved successfully")
}

func (l *Service) GetPOIDetailedInfosResponse(ctx context.Context, userID uuid.UUID, city string, lat, lon float64) (*POIDetailedInfo, error) {
	ctx, span := otel.Tracer("LlmInteractionService").Start(ctx, "GetPOIDetailedInfosResponse", trace.WithAttributes(
		attribute.String("city.name", city),
		attribute.Float64("latitude", lat),
		attribute.Float64("longitude", lon),
		attribute.String("user.id", userID.String()),
	))
	defer span.End()

	l.logger.Debug("Starting POI details generation",
		zap.String("city", city), zap.Float64("latitude", lat), zap.Float64("longitude", lon), zap.String("userID", userID.String()))

	cacheKey := generatePOICacheKey(city, lat, lon, 0.0, userID)
	span.SetAttributes(attribute.String("cache.key", cacheKey))

	if cached, found := l.cache.Get(cacheKey); found {
		if p, ok := cached.(*POIDetailedInfo); ok {
			l.logger.Info("Cache hit for POI details", zap.String("cache_key", cacheKey))
			span.AddEvent("Cache hit")
			span.SetStatus(codes.Ok, "POI details served from cache")
			return p, nil
		}
	}

	cityData, err := l.cityRepo.FindCityByNameAndCountry(ctx, city, "")
	if err != nil {
		l.logger.Error("Failed to find city", zap.Error(err))
		span.RecordError(err)
		return nil, fmt.Errorf("failed to find city: %w", err)
	}
	if cityData == nil {
		l.logger.Warn("City not found", zap.String("city", city))
		span.SetStatus(codes.Error, "City not found")
		return nil, fmt.Errorf("city %s not found", city)
	}
	cityID := cityData.ID

	p, err := l.poiRepo.FindPOIDetails(ctx, cityID, lat, lon, 100.0)
	if err != nil {
		l.logger.Error("Failed to query POI details from database", zap.Error(err))
		span.RecordError(err)
		return nil, fmt.Errorf("failed to query POI details: %w", err)
	}
	if p != nil {
		p.City = city
		l.cache.Set(cacheKey, p, cache.DefaultExpiration)
		l.logger.Info("Database hit for POI details", zap.String("cache_key", cacheKey))
		span.AddEvent("Database hit")
		span.SetStatus(codes.Ok, "POI details served from database")
		return p, nil
	}

	l.logger.Debug("Cache and database miss, fetching POI details from AI", zap.String("cache_key", cacheKey))
	span.AddEvent("Cache and database miss")

	resultCh := make(chan POIDetailedInfo, 1)
	var wg sync.WaitGroup
	wg.Add(1)

	go l.getPOIDetailedInfos(&wg, ctx, city, lat, lon, userID, resultCh, &genai.GenerateContentConfig{Temperature: genai.Ptr[float32](defaultTemperature)})

	go func() {
		wg.Wait()
		close(resultCh)
	}()

	var poiResult *poi.POIDetailedInfo
	for res := range resultCh {
		if res.Err != nil {
			l.logger.Error("Error generating POI details", zap.Error(res.Err))
			span.RecordError(res.Err)
			span.SetStatus(codes.Error, "Failed to generate POI details")
			return nil, res.Err
		}
		poiResult = &res
		break
	}

	if poiResult == nil {
		l.logger.Warn("No response received for POI details")
		span.SetStatus(codes.Error, "No response received")
		return nil, fmt.Errorf("no response received for POI details")
	}

	_, err = l.poiRepo.SavePoi(ctx, *poiResult, cityID)
	if err != nil {
		l.logger.Warn("Failed to save POI details to database", zap.Error(err))
		span.RecordError(err)
	}

	l.cache.Set(cacheKey, poiResult, cache.DefaultExpiration)
	l.logger.Debug("Stored POI details in cache", zap.String("cache_key", cacheKey))
	span.AddEvent("Stored in cache")

	span.SetStatus(codes.Ok, "POI details generated and cached successfully")
	return poiResult, nil
}
