package chat_prompt

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
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

	"github.com/FACorreiaa/go-poi-au-suggestions/internal/domain/profiles"
)

func (l *Service) ProcessUnifiedChatMessageStream(ctx context.Context, userID, profileID uuid.UUID, cityName, message string, userLocation *UserLocation, eventCh chan<- StreamEvent) error {
	//startTime := time.Now()
	ctx, span := otel.Tracer("LlmInteractionService").Start(ctx, "ProcessUnifiedChatMessageStream", trace.WithAttributes(
		attribute.String("message", message),
	))
	defer span.End()

	extractedCity, cleanedMessage, err := l.extractCityFromMessage(ctx, message)
	if err != nil {
		span.RecordError(err)
		l.sendEvent(ctx, eventCh, StreamEvent{Type: EventTypeError, Error: err.Error()}, 3)
		return fmt.Errorf("failed to parse message: %w", err)
	}
	if extractedCity != "" {
		cityName = extractedCity
	}
	span.SetAttributes(attribute.String("extracted.city", cityName), attribute.String("cleaned.message", cleanedMessage))

	domainDetector := &DomainDetector{}
	domain := domainDetector.DetectDomain(ctx, cleanedMessage)
	span.SetAttributes(attribute.String("detected.domain", string(domain)))

	_, searchProfile, _, err := l.FetchUserData(ctx, userID, profileID)
	if err != nil {
		span.RecordError(err)
		l.sendEvent(ctx, eventCh, StreamEvent{Type: EventTypeError, Error: err.Error()}, 3)
		return fmt.Errorf("failed to fetch user data: %w", err)
	}
	basePreferences := getUserPreferencesPrompt(searchProfile)

	var lat, lon float64
	if userLocation == nil && searchProfile.UserLatitude != nil && searchProfile.UserLongitude != nil {
		userLocation = &UserLocation{
			UserLat: *searchProfile.UserLatitude,
			UserLon: *searchProfile.UserLongitude,
		}
	}
	if userLocation != nil {
		lat, lon = userLocation.UserLat, userLocation.UserLon
	}

	sessionID := uuid.New()

	session := ChatSession{
		ID:        sessionID,
		UserID:    userID,
		ProfileID: profileID,
		CityName:  cityName,
		ConversationHistory: []ConversationMessage{
			{Role: "user", Content: message, Timestamp: time.Now()},
		},
		SessionContext: SessionContext{
			CityName:            cityName,
			ConversationSummary: fmt.Sprintf("Trip plan for %s", cityName),
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		ExpiresAt: time.Now().Add(24 * time.Hour),
		Status:    "active",
	}
	if err := l.repo.CreateSession(ctx, session); err != nil {
		span.RecordError(err)
		l.sendEvent(ctx, eventCh, StreamEvent{Type: EventTypeError, Error: err.Error()}, 3)
		return fmt.Errorf("failed to create session: %w", err)
	}

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
		l.logger.Error("Failed to marshal cache key data", zap.Error(err))
		cacheKeyBytes = []byte(fmt.Sprintf("fallback_%s_%s", cleanedMessage, cityName))
	}
	hash := md5.Sum(cacheKeyBytes)
	cacheKey := hex.EncodeToString(hash[:])

	var wg sync.WaitGroup
	var closeOnce sync.Once

	l.sendEvent(ctx, eventCh, StreamEvent{
		Type: EventTypeStart,
		Data: map[string]interface{}{
			"domain":     string(domain),
			"city":       cityName,
			"session_id": sessionID.String(),
			"cache_key":  cacheKey,
		},
	}, 3)

	responses := make(map[string]*strings.Builder)
	responsesMutex := sync.Mutex{}

	sendEventWithResponse := func(event StreamEvent) {
		if event.Type == EventTypeChunk {
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

	switch domain {
	case profiles.DomainItinerary, profiles.DomainGeneral:
		wg.Add(3)

		go func() {
			defer wg.Done()
			prompt := getCityDataPrompt(cityName)
			partCacheKey := cacheKey + "_city_data"
			l.streamWorkerWithResponseAndCache(ctx, prompt, "city_data", sendEventWithResponse, domain, partCacheKey)
		}()

		go func() {
			defer wg.Done()
			prompt := getGeneralPOIPrompt(cityName)
			partCacheKey := cacheKey + "_general_pois"
			l.streamWorkerWithResponseAndCache(ctx, prompt, "general_pois", sendEventWithResponse, domain, partCacheKey)
		}()

		go func() {
			defer wg.Done()
			prompt := getPersonalizedItineraryPrompt(cityName, basePreferences)
			partCacheKey := cacheKey + "_itinerary"
			l.streamWorkerWithResponseAndCache(ctx, prompt, "itinerary", sendEventWithResponse, domain, partCacheKey)
		}()

	case profiles.DomainAccommodation:
		wg.Add(1)
		go func() {
			defer wg.Done()
			prompt := getAccommodationPrompt(cityName, lat, lon, basePreferences)
			partCacheKey := cacheKey + "_hotels"
			l.streamWorkerWithResponseAndCache(ctx, prompt, "hotels", sendEventWithResponse, domain, partCacheKey)
		}()

	case profiles.DomainDining:
		wg.Add(1)
		go func() {
			defer wg.Done()
			prompt := getDiningPrompt(cityName, lat, lon, basePreferences)
			partCacheKey := cacheKey + "_restaurants"
			l.streamWorkerWithResponseAndCache(ctx, prompt, "restaurants", sendEventWithResponse, domain, partCacheKey)
		}()

	case profiles.DomainActivities:
		wg.Add(1)
		go func() {
			defer wg.Done()
			prompt := getActivitiesPrompt(cityName, lat, lon, basePreferences)
			partCacheKey := cacheKey + "_activities"
			l.streamWorkerWithResponseAndCache(ctx, prompt, "activities", sendEventWithResponse, domain, partCacheKey)
		}()

	default:
		sendEventWithResponse(StreamEvent{Type: EventTypeError, Error: fmt.Sprintf("unhandled domain: %s", domain)})
		return fmt.Errorf("unhandled domain type: %s", domain)
	}

	go func() {
		wg.Wait()
		if ctx.Err() == nil {
			var routeType string
			var baseURL string
			switch domain {
			case profiles.DomainAccommodation:
				routeType = "hotels"
				baseURL = "/hotels"
			case profiles.DomainDining:
				routeType = "restaurants"
				baseURL = "/restaurants"
			case profiles.DomainActivities:
				routeType = "activities"
				baseURL = "/activities"
			default:
				routeType = "itinerary"
				baseURL = "/itinerary"
			}

			l.sendEvent(ctx, eventCh, StreamEvent{
				Type: EventTypeComplete,
				Data: map[string]interface{}{"session_id": sessionID.String()},
				Navigation: &NavigationData{
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
			// Don't close eventCh here - it's managed by the caller
			// close(eventCh)
			l.logger.Info("Event processing completed by completion goroutine")
		})
	}()

	//go func() {
	//	asyncCtx := context.Background()
	//
	//	var fullResponseBuilder strings.Builder
	//	responsesMutex.Lock()
	//	cityDataContent := ""
	//	if responses["city_data"] != nil {
	//		cityDataContent = responses["city_data"].String()
	//	}
	//	for partType, builder := range responses {
	//		if builder != nil && builder.Len() > 0 {
	//			fullResponseBuilder.WriteString(fmt.Sprintf("[%s]\n%s\n\n", partType, builder.String()))
	//		}
	//	}
	//	responsesMutex.Unlock()
	//
	//	fullResponse := fullResponseBuilder.String()
	//	if fullResponse == "" {
	//		fullResponse = fmt.Sprintf("Processed %s request for %s", domain, cityName)
	//	}
	//
	//	var cityID uuid.UUID
	//	if cityDataContent != "" {
	//		l.logger.Debug("City data content received", zap.String("content", cityDataContent))
	//		if parsedCityData, parseErr := l.parseCityDataFromResponse(asyncCtx, cityDataContent); parseErr == nil && parsedCityData != nil {
	//			if savedCityID, handleErr := l.HandleCityData(asyncCtx, *parsedCityData); handleErr != nil {
	//				l.logger.Warn("Failed to save city data during unified stream processing",
	//					zap.String("city", cityName), zap.Error(handleErr))
	//			} else {
	//				l.logger.Info("Successfully saved city data during unified stream processing",
	//					zap.String("city", cityName))
	//				cityID = savedCityID
	//			}
	//		} else if parseErr != nil {
	//			l.logger.Warn("Failed to parse city data from unified stream response",
	//				zap.String("city", cityName), zap.Error(parseErr))
	//		}
	//	}
	//
	//	if cityID == uuid.Nil {
	//		if existingCity, err := l.cityRepo.FindCityByNameAndCountry(asyncCtx, cityName, ""); err == nil && existingCity != nil {
	//			cityID = existingCity.ID
	//		} else {
	//			l.logger.Warn("Could not find or save city data, skipping POI processing",
	//				zap.String("city", cityName))
	//			return
	//		}
	//	}
	//
	//	interaction := LlmInteraction{
	//		ID:           uuid.New(),
	//		SessionID:    sessionID,
	//		UserID:       userID,
	//		ProfileID:    profileID,
	//		CityName:     cityName,
	//		Prompt:       fmt.Sprintf("Unified Chat Stream - Domain: %s, Message: %s", domain, cleanedMessage),
	//		ResponseText: fullResponse,
	//		ModelUsed:    model,
	//		LatencyMs:    int(time.Since(startTime).Milliseconds()),
	//		Timestamp:    startTime,
	//	}
	//	savedInteractionID, err := l.repo.SaveInteraction(asyncCtx, interaction)
	//	if err != nil {
	//		l.logger.Error("Failed to save stream interaction", zap.Error(err))
	//		return
	//	}
	//
	//	l.logger.Info("Stream interaction saved successfully",
	//		zap.String("saved_interaction_id", savedInteractionID.String()),
	//		zap.String("original_session_id", sessionID.String()))
	//
	//	l.ProcessAndSaveUnifiedResponse(asyncCtx, responses, userID, profileID, cityID, savedInteractionID, userLocation)
	//}()

	span.SetStatus(codes.Ok, "Unified chat stream processed successfully")
	return nil
}

func (l *Service) ProcessUnifiedChatMessageStreamFree(ctx context.Context, cityName, message string, userLocation *UserLocation, eventCh chan<- StreamEvent) error {
	startTime := time.Now()
	ctx, span := otel.Tracer("LlmInteractionService").Start(ctx, "ProcessUnifiedChatMessageStream", trace.WithAttributes(
		attribute.String("message", message),
	))
	defer span.End()

	extractedCity, cleanedMessage, err := l.extractCityFromMessage(ctx, message)
	if err != nil {
		span.RecordError(err)
		l.sendEvent(ctx, eventCh, StreamEvent{Type: EventTypeError, Error: err.Error()}, 3)
		return fmt.Errorf("failed to parse message: %w", err)
	}
	if extractedCity != "" {
		cityName = extractedCity
	}
	span.SetAttributes(attribute.String("extracted.city", cityName), attribute.String("cleaned.message", cleanedMessage))

	domainDetector := &DomainDetector{}
	domain := domainDetector.DetectDomain(ctx, cleanedMessage)
	span.SetAttributes(attribute.String("detected.domain", string(domain)))

	sessionID := uuid.New()

	session := ChatSession{
		ID:       sessionID,
		CityName: cityName,
		ConversationHistory: []ConversationMessage{
			{Role: "user", Content: message, Timestamp: time.Now()},
		},
		SessionContext: SessionContext{
			CityName:            cityName,
			ConversationSummary: fmt.Sprintf("Trip plan for %s", cityName),
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		ExpiresAt: time.Now().Add(24 * time.Hour),
		Status:    "active",
	}
	if err := l.repo.CreateSession(ctx, session); err != nil {
		span.RecordError(err)
		l.sendEvent(ctx, eventCh, StreamEvent{Type: EventTypeError, Error: err.Error()}, 3)
		return fmt.Errorf("failed to create session: %w", err)
	}

	cacheKeyData := map[string]interface{}{
		"city":    cityName,
		"message": cleanedMessage,
		"domain":  string(domain),
	}
	cacheKeyBytes, err := json.Marshal(cacheKeyData)
	if err != nil {
		l.logger.Error("Failed to marshal cache key data", zap.Error(err))
		cacheKeyBytes = []byte(fmt.Sprintf("fallback_%s_%s", cleanedMessage, cityName))
	}
	hash := md5.Sum(cacheKeyBytes)
	cacheKey := hex.EncodeToString(hash[:])

	var wg sync.WaitGroup
	var closeOnce sync.Once

	l.sendEvent(ctx, eventCh, StreamEvent{
		Type: EventTypeStart,
		Data: map[string]interface{}{
			"domain":     string(domain),
			"city":       cityName,
			"session_id": sessionID.String(),
			"cache_key":  cacheKey,
		},
	}, 3)

	responses := make(map[string]*strings.Builder)
	responsesMutex := sync.Mutex{}

	sendEventWithResponse := func(event StreamEvent) {
		if event.Type == EventTypeChunk {
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

	switch domain {
	case profiles.DomainItinerary, profiles.DomainGeneral:
		wg.Add(3)

		go func() {
			defer wg.Done()
			prompt := getCityDataPrompt(cityName)
			partCacheKey := cacheKey + "_city_data"
			l.streamWorkerWithResponseAndCache(ctx, prompt, "city_data", sendEventWithResponse, domain, partCacheKey)
		}()

		go func() {
			defer wg.Done()
			prompt := getGeneralPOIPrompt(cityName)
			partCacheKey := cacheKey + "_general_pois"
			l.streamWorkerWithResponseAndCache(ctx, prompt, "general_pois", sendEventWithResponse, domain, partCacheKey)
		}()

		go func() {
			defer wg.Done()
			prompt := getGeneralizedItineraryPrompt(cityName)
			partCacheKey := cacheKey + "_itinerary"
			l.streamWorkerWithResponseAndCache(ctx, prompt, "itinerary", sendEventWithResponse, domain, partCacheKey)
		}()

	case profiles.DomainAccommodation:
		wg.Add(1)
		go func() {
			defer wg.Done()
			prompt := getGeneralAccommodationPrompt(cityName)
			partCacheKey := cacheKey + "_hotels"
			l.streamWorkerWithResponseAndCache(ctx, prompt, "hotels", sendEventWithResponse, domain, partCacheKey)
		}()

	case profiles.DomainDining:
		wg.Add(1)
		go func() {
			defer wg.Done()
			prompt := getGeneralDiningPrompt(cityName)
			partCacheKey := cacheKey + "_restaurants"
			l.streamWorkerWithResponseAndCache(ctx, prompt, "restaurants", sendEventWithResponse, domain, partCacheKey)
		}()

	case profiles.DomainActivities:
		wg.Add(1)
		go func() {
			defer wg.Done()
			prompt := getGeneralActivitiesPrompt(cityName)
			partCacheKey := cacheKey + "_activities"
			l.streamWorkerWithResponseAndCache(ctx, prompt, "activities", sendEventWithResponse, domain, partCacheKey)
		}()

	default:
		sendEventWithResponse(StreamEvent{Type: EventTypeError, Error: fmt.Sprintf("unhandled domain: %s", domain)})
		return fmt.Errorf("unhandled domain type: %s", domain)
	}

	go func() {
		wg.Wait()
		if ctx.Err() == nil {
			var routeType string
			var baseURL string
			switch domain {
			case profiles.DomainAccommodation:
				routeType = "hotels"
				baseURL = "/hotels"
			case profiles.DomainDining:
				routeType = "restaurants"
				baseURL = "/restaurants"
			case profiles.DomainActivities:
				routeType = "activities"
				baseURL = "/activities"
			default:
				routeType = "itinerary"
				baseURL = "/itinerary"
			}

			l.sendEvent(ctx, eventCh, StreamEvent{
				Type: EventTypeComplete,
				Data: map[string]interface{}{"session_id": sessionID.String()},
				Navigation: &NavigationData{
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
			// Don't close eventCh here - it's managed by the caller
			// close(eventCh)
			l.logger.Info("Event processing completed by completion goroutine")
		})
	}()

	go func() {
		wg.Wait()

		//asyncCtx := context.Background()

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

		var cityID uuid.UUID
		if cityDataContent != "" {
			l.logger.Debug("City data content received", zap.String("content", cityDataContent))
			if parsedCityData, parseErr := l.parseCityDataFromResponse(ctx, cityDataContent); parseErr == nil && parsedCityData != nil {
				if savedCityID, handleErr := l.HandleCityData(ctx, *parsedCityData); handleErr != nil {
					l.logger.Warn("Failed to save city data during unified stream processing",
						zap.String("city", cityName), zap.Error(handleErr))
				} else {
					l.logger.Info("Successfully saved city data during unified stream processing",
						zap.String("city", cityName))
					cityID = savedCityID
				}
			} else if parseErr != nil {
				l.logger.Warn("Failed to parse city data from unified stream response",
					zap.String("city", cityName), zap.Error(parseErr))
			}
		}

		if cityID == uuid.Nil {
			if existingCity, err := l.cityRepo.FindCityByNameAndCountry(ctx, cityName, ""); err == nil && existingCity != nil {
				cityID = existingCity.ID
			} else {
				l.logger.Warn("Could not find or save city data, skipping POI processing",
					zap.String("city", cityName))
				return
			}
		}

		interaction := LlmInteraction{
			ID:           uuid.New(),
			SessionID:    sessionID,
			CityName:     cityName,
			Prompt:       fmt.Sprintf("Unified Chat Stream - Domain: %s, Message: %s", domain, cleanedMessage),
			ResponseText: fullResponse,
			ModelUsed:    model,
			LatencyMs:    int(time.Since(startTime).Milliseconds()),
			Timestamp:    startTime,
		}
		savedInteractionID, err := l.repo.SaveInteraction(ctx, interaction)
		if err != nil {
			l.logger.Error("Failed to save stream interaction", zap.Error(err))
			return
		}

		l.logger.Info("Stream interaction saved successfully (free)",
			zap.String("saved_interaction_id", savedInteractionID.String()),
			zap.String("original_session_id", sessionID.String()))

		l.ProcessAndSaveUnifiedResponseFree(ctx, responses, cityID, savedInteractionID, userLocation)
	}()

	span.SetStatus(codes.Ok, "Unified chat stream processed successfully")
	return nil
}

func (l *Service) parseCityDataFromResponse(_ context.Context, responseContent string) (*GeneralCityData, error) {
	cleanedResponse := responseContent

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

	if !json.Valid([]byte(cleanedResponse)) {
		return nil, fmt.Errorf("invalid JSON in city data response")
	}

	var generalCity GeneralCityData
	if err := json.Unmarshal([]byte(cleanedResponse), &generalCity); err != nil {
		return nil, fmt.Errorf("failed to unmarshal city data: %w", err)
	}

	if generalCity.City == "" {
		return nil, fmt.Errorf("parsed city data is missing city name")
	}

	return &generalCity, nil
}

func (l *Service) streamWorkerWithResponseAndCache(ctx context.Context, prompt, partType string, sendEvent func(StreamEvent), domain profiles.DomainType, cacheKey string) {
	iter, err := l.aiClient.GenerateContentStreamWithCache(ctx, prompt, &genai.GenerateContentConfig{Temperature: genai.Ptr[float32](defaultTemperature)}, cacheKey)
	if err != nil {
		if ctx.Err() == nil {
			sendEvent(StreamEvent{
				Type:  EventTypeError,
				Error: fmt.Sprintf("%s worker failed: %v", partType, err),
			})
		}
		return
	}

	var fullResponse strings.Builder
	for resp, err := range iter {
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			if ctx.Err() == nil {
				sendEvent(StreamEvent{
					Type:  EventTypeError,
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
						sendEvent(StreamEvent{
							Type: EventTypeChunk,
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
