package llmChat

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/FACorreiaa/go-poi-au-suggestions/internal/types"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/genai"
)

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
				return false
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

// streamingCityDataWorker generates city data with streaming updates
func (l *ServiceImpl) streamingCityDataWorker(wg *sync.WaitGroup,
	ctx context.Context, cityName string, resultCh chan<- types.GenAIResponse,
	eventCh chan<- types.StreamEvent, userID uuid.UUID) {
	ctxWorker, span := otel.Tracer("LlmInteractionService").Start(ctx, "streamingCityDataWorker", trace.WithAttributes(
		attribute.String("city.name", cityName),
	))
	defer span.End()
	if wg != nil {
		defer wg.Done()
	}

	if !l.sendEvent(ctxWorker, eventCh, types.StreamEvent{
		Type: types.EventTypeProgress,
		Data: map[string]interface{}{"status": "generating_city_data", "progress": 10},
	}, 3) {
		resultCh <- types.GenAIResponse{Err: fmt.Errorf("context cancelled before sending initial progress for city data")}
		return
	}

	startTime := time.Now()
	prompt := getCityDescriptionPrompt(cityName)

	// Generate city data
	cleanTxt, err := l.generateCityData(ctxWorker, cityName)
	if err != nil {
		span.RecordError(err)
		l.sendEvent(ctxWorker, eventCh, types.StreamEvent{
			Type:      types.EventTypeError,
			Error:     err.Error(),
			Timestamp: time.Now(),
			EventID:   uuid.New().String(),
		}, 3)
		resultCh <- types.GenAIResponse{Err: err}
		return
	}

	// Send partial data event (for consistency with original)
	l.sendEvent(ctxWorker, eventCh, types.StreamEvent{
		Type:      types.EventTypeCityData,
		Data:      map[string]string{"partial_city_data": cleanTxt},
		Timestamp: time.Now(),
		EventID:   uuid.New().String(),
	}, 3)

	// Save LLM interaction
	interaction := types.LlmInteraction{
		UserID:       userID,
		Prompt:       prompt,
		ResponseText: cleanTxt,
		Timestamp:    startTime,
		LatencyMs:    int(time.Since(startTime).Milliseconds()),
		CityName:     cityName,
	}
	_, err = l.saveCityInteraction(ctxWorker, interaction)
	if err != nil {
		span.RecordError(err)
		l.sendEvent(ctxWorker, eventCh, types.StreamEvent{
			Type:      types.EventTypeError,
			Error:     fmt.Sprintf("failed to save city data interaction: %v", err),
			Timestamp: time.Now(),
			EventID:   uuid.New().String(),
		}, 3)
		resultCh <- types.GenAIResponse{Err: err}
		return
	}

	// Parse JSON response
	var cityData struct {
		CityName        string  `json:"city_name"`
		StateProvince   *string `json:"state_province,omitempty"`
		Country         string  `json:"country"`
		CenterLatitude  float64 `json:"center_latitude"`
		CenterLongitude float64
		Description     string `json:"description"`
	}
	if err := json.Unmarshal([]byte(cleanTxt), &cityData); err != nil {
		span.RecordError(err)
		l.sendEvent(ctxWorker, eventCh, types.StreamEvent{
			Type:      types.EventTypeError,
			Error:     fmt.Sprintf("failed to parse city data JSON: %v", err),
			Timestamp: time.Now(),
			EventID:   uuid.New().String(),
		}, 3)
		resultCh <- types.GenAIResponse{Err: err}
		return
	}

	stateProvince := ""
	if cityData.StateProvince != nil {
		stateProvince = *cityData.StateProvince
	}

	result := types.GenAIResponse{
		City:            cityData.CityName,
		Country:         cityData.Country,
		StateProvince:   stateProvince,
		CityDescription: cityData.Description,
		Latitude:        cityData.CenterLatitude,
		Longitude:       cityData.CenterLongitude,
	}

	l.sendEvent(ctxWorker, eventCh, types.StreamEvent{
		Type:      types.EventTypeCityData,
		Data:      result,
		Timestamp: time.Now(),
		EventID:   uuid.New().String(),
	}, 3)
	resultCh <- result
}

// streamingGeneralPOIWorker generates general POIs with streaming updates
func (l *ServiceImpl) streamingGeneralPOIWorker(wg *sync.WaitGroup,
	ctx context.Context, cityName string,
	resultCh chan<- types.GenAIResponse,
	eventCh chan<- types.StreamEvent,
	userID uuid.UUID) {
	defer wg.Done()

	ctx, span := otel.Tracer("LlmInteractionService").Start(ctx, "streamingGeneralPOIWorker", trace.WithAttributes(
		attribute.String("city.name", cityName),
	))
	defer span.End()

	l.sendEvent(ctx, eventCh, types.StreamEvent{
		Type:      types.EventTypeProgress,
		Data:      map[string]interface{}{"status": "generating_general_pois", "progress": 30},
		Timestamp: time.Now(),
		EventID:   uuid.New().String(),
	}, 3)

	prompt := getGeneralPOIPrompt(cityName)
	startTime := time.Now()
	var responseText strings.Builder

	// Try streaming
	iter, err := l.aiClient.GenerateContentStream(ctx, prompt, &genai.GenerateContentConfig{Temperature: genai.Ptr[float32](defaultTemperature)})
	if err == nil {
		for resp, err := range iter {
			if err != nil {
				span.RecordError(err)
				l.sendEvent(ctx, eventCh, types.StreamEvent{
					Type:      types.EventTypeError,
					Error:     fmt.Sprintf("streaming general POI error: %v", err),
					Timestamp: time.Now(),
					EventID:   uuid.New().String(),
				}, 3)
				resultCh <- types.GenAIResponse{Err: err}
				return
			}
			for _, cand := range resp.Candidates {
				if cand.Content != nil {
					for _, part := range cand.Content.Parts {
						if part.Text != "" {
							responseText.WriteString(string(part.Text))
							l.sendEvent(ctx, eventCh, types.StreamEvent{
								Type:      types.EventTypeGeneralPOI,
								Data:      map[string]string{"partial_poi_data": responseText.String()},
								Timestamp: time.Now(),
								EventID:   uuid.New().String(),
							}, 3)
						}
					}
				}
			}
		}
	} else {
		// Fallback to non-streaming
		l.logger.WarnContext(ctx, "Streaming general POIs failed, falling back to non-streaming", slog.Any("error", err))
		response, err := l.aiClient.GenerateResponse(ctx, prompt, &genai.GenerateContentConfig{Temperature: genai.Ptr[float32](defaultTemperature)})
		if err != nil {
			span.RecordError(err)
			l.sendEvent(ctx, eventCh, types.StreamEvent{
				Type:      types.EventTypeError,
				Error:     fmt.Sprintf("failed to generate general POIs: %v", err),
				Timestamp: time.Now(),
				EventID:   uuid.New().String(),
			}, 3)
			resultCh <- types.GenAIResponse{Err: err}
			return
		}
		for _, cand := range response.Candidates {
			if cand.Content != nil {
				for _, part := range cand.Content.Parts {
					if part.Text != "" {
						responseText.WriteString(string(part.Text))
					}
				}
			}
		}
		l.sendEvent(ctx, eventCh, types.StreamEvent{
			Type:      types.EventTypeGeneralPOI,
			Data:      map[string]string{"partial_poi_data": responseText.String()},
			Timestamp: time.Now(),
			EventID:   uuid.New().String(),
		}, 3)
	}

	fullText := responseText.String()
	if fullText == "" {
		err := fmt.Errorf("empty general POI response")
		span.RecordError(err)
		l.sendEvent(ctx, eventCh, types.StreamEvent{
			Type:      types.EventTypeError,
			Error:     err.Error(),
			Timestamp: time.Now(),
			EventID:   uuid.New().String(),
		}, 3)
		resultCh <- types.GenAIResponse{Err: err}
		return
	}

	// Save LLM interaction
	latencyMs := int(time.Since(startTime).Milliseconds())
	interaction := types.LlmInteraction{
		UserID:       userID, // No specific user for general POIs
		Prompt:       prompt,
		ResponseText: fullText,
		ModelUsed:    model,
		LatencyMs:    latencyMs,
		CityName:     cityName,
	}
	_, err = l.llmInteractionRepo.SaveInteraction(ctx, interaction)
	if err != nil {
		span.RecordError(err)
		l.sendEvent(ctx, eventCh, types.StreamEvent{
			Type:      types.EventTypeError,
			Error:     fmt.Sprintf("failed to save general POI interaction: %v", err),
			Timestamp: time.Now(),
			EventID:   uuid.New().String(),
		}, 3)
		resultCh <- types.GenAIResponse{Err: err}
		return
	}

	cleanTxt := cleanJSONResponse(fullText)
	var poiData struct {
		PointsOfInterest []types.POIDetailedInfo `json:"points_of_interest"`
	}
	if err := json.Unmarshal([]byte(cleanTxt), &poiData); err != nil {
		span.RecordError(err)
		l.sendEvent(ctx, eventCh, types.StreamEvent{
			Type:      types.EventTypeError,
			Error:     fmt.Sprintf("failed to parse general POI JSON: %v", err),
			Timestamp: time.Now(),
			EventID:   uuid.New().String(),
		}, 3)
		resultCh <- types.GenAIResponse{Err: err}
		return
	}

	result := types.GenAIResponse{
		GeneralPOI: poiData.PointsOfInterest,
	}

	l.sendEvent(ctx, eventCh, types.StreamEvent{
		Type:      types.EventTypeGeneralPOI,
		Data:      result.GeneralPOI,
		Timestamp: time.Now(),
		EventID:   uuid.New().String(),
	}, 3)
	resultCh <- result
}

// streamingPersonalizedPOIWorkerWithSemantics generates personalized POIs with semantic context and streaming updates
func (l *ServiceImpl) streamingPersonalizedPOIWorkerWithSemantics(wg *sync.WaitGroup, ctx context.Context, cityName string, userID, profileID uuid.UUID, resultCh chan<- types.GenAIResponse, eventCh chan<- types.StreamEvent, interestNames []string, tagsPromptPart, userPrefs string, semanticPOIs []types.POIDetailedInfo) {
	defer wg.Done()

	ctx, span := otel.Tracer("LlmInteractionService").Start(ctx, "streamingPersonalizedPOIWorkerWithSemantics", trace.WithAttributes(
		attribute.String("city.name", cityName),
		attribute.String("user.id", userID.String()),
		attribute.String("profile.id", profileID.String()),
		attribute.Int("semantic_pois.count", len(semanticPOIs)),
	))
	defer span.End()

	l.sendEvent(ctx, eventCh, types.StreamEvent{
		Type: types.EventTypeProgress,
		Data: map[string]interface{}{
			"status":           "generating_semantic_personalized_pois",
			"progress":         50,
			"semantic_context": len(semanticPOIs) > 0,
		},
		Timestamp: time.Now(),
		EventID:   uuid.New().String(),
	}, 3)

	startTime := time.Now()
	prompt := l.getPersonalizedPOIWithSemanticContext(interestNames, cityName, tagsPromptPart, userPrefs, semanticPOIs)
	var responseText strings.Builder

	// Try streaming
	iter, err := l.aiClient.GenerateContentStream(ctx, prompt, &genai.GenerateContentConfig{Temperature: genai.Ptr[float32](defaultTemperature)})
	if err == nil {
		for resp, err := range iter {
			if err != nil {
				span.RecordError(err)
				l.sendEvent(ctx, eventCh, types.StreamEvent{
					Type:      types.EventTypeError,
					Error:     fmt.Sprintf("streaming semantic personalized POI error: %v", err),
					Timestamp: time.Now(),
					EventID:   uuid.New().String(),
				}, 3)
				resultCh <- types.GenAIResponse{Err: err}
				return
			}
			for _, cand := range resp.Candidates {
				if cand.Content != nil {
					for _, part := range cand.Content.Parts {
						if part.Text != "" {
							responseText.WriteString(string(part.Text))
							l.sendEvent(ctx, eventCh, types.StreamEvent{
								Type: types.EventTypePersonalizedPOI,
								Data: map[string]interface{}{
									"partial_poi_data":       responseText.String(),
									"semantic_enhanced":      true,
									"semantic_context_count": len(semanticPOIs),
								},
								Timestamp: time.Now(),
								EventID:   uuid.New().String(),
							}, 3)
						}
					}
				}
			}
		}
	} else {
		// Fallback to non-streaming
		l.logger.WarnContext(ctx, "Streaming semantic personalized POIs failed, falling back to non-streaming", slog.Any("error", err))
		response, err := l.aiClient.GenerateResponse(ctx, prompt, &genai.GenerateContentConfig{Temperature: genai.Ptr[float32](defaultTemperature)})
		if err != nil {
			span.RecordError(err)
			l.sendEvent(ctx, eventCh, types.StreamEvent{
				Type:      types.EventTypeError,
				Error:     fmt.Sprintf("failed to generate semantic personalized POIs: %v", err),
				Timestamp: time.Now(),
				EventID:   uuid.New().String(),
			}, 3)
			resultCh <- types.GenAIResponse{Err: err}
			return
		}
		for _, cand := range response.Candidates {
			if cand.Content != nil {
				for _, part := range cand.Content.Parts {
					if part.Text != "" {
						responseText.WriteString(string(part.Text))
					}
				}
			}
		}
		l.sendEvent(ctx, eventCh, types.StreamEvent{
			Type: types.EventTypePersonalizedPOI,
			Data: map[string]interface{}{
				"partial_poi_data":       responseText.String(),
				"semantic_enhanced":      true,
				"semantic_context_count": len(semanticPOIs),
			},
			Timestamp: time.Now(),
			EventID:   uuid.New().String(),
		}, 3)
	}

	fullText := responseText.String()
	if fullText == "" {
		err := fmt.Errorf("empty semantic personalized POI response")
		span.RecordError(err)
		l.sendEvent(ctx, eventCh, types.StreamEvent{
			Type:      types.EventTypeError,
			Error:     err.Error(),
			Timestamp: time.Now(),
			EventID:   uuid.New().String(),
		}, 3)
		resultCh <- types.GenAIResponse{Err: err}
		return
	}

	// Save LLM interaction with semantic metadata
	latencyMs := int(time.Since(startTime).Milliseconds())
	interaction := types.LlmInteraction{
		UserID:       userID,
		Prompt:       prompt,
		ResponseText: fullText,
		ModelUsed:    model,
		LatencyMs:    latencyMs,
		CityName:     cityName,
	}
	savedInteractionID, err := l.llmInteractionRepo.SaveInteraction(ctx, interaction)
	if err != nil {
		span.RecordError(err)
		l.sendEvent(ctx, eventCh, types.StreamEvent{
			Type:      types.EventTypeError,
			Error:     fmt.Sprintf("failed to save semantic personalized POI interaction: %v", err),
			Timestamp: time.Now(),
			EventID:   uuid.New().String(),
		}, 3)
		resultCh <- types.GenAIResponse{Err: err}
		return
	}

	cleanTxt := cleanJSONResponse(fullText)
	var itineraryData struct {
		ItineraryName      string                  `json:"itinerary_name"`
		OverallDescription string                  `json:"overall_description"`
		PointsOfInterest   []types.POIDetailedInfo `json:"points_of_interest"`
	}
	if err := json.Unmarshal([]byte(cleanTxt), &itineraryData); err != nil {
		span.RecordError(err)
		l.sendEvent(ctx, eventCh, types.StreamEvent{
			Type:      types.EventTypeError,
			Error:     fmt.Sprintf("failed to parse semantic personalized POI JSON: %v", err),
			Timestamp: time.Now(),
			EventID:   uuid.New().String(),
		}, 3)
		resultCh <- types.GenAIResponse{Err: err}
		return
	}

	result := types.GenAIResponse{
		ItineraryName:        itineraryData.ItineraryName,
		ItineraryDescription: itineraryData.OverallDescription,
		PersonalisedPOI:      itineraryData.PointsOfInterest,
		LlmInteractionID:     savedInteractionID,
	}

	l.sendEvent(ctx, eventCh, types.StreamEvent{
		Type: types.EventTypePersonalizedPOI,
		Data: map[string]interface{}{
			"result":                 result,
			"semantic_enhanced":      true,
			"semantic_context_count": len(semanticPOIs),
		},
		Timestamp: time.Now(),
		EventID:   uuid.New().String(),
	}, 3)
	resultCh <- result
}

func (l *ServiceImpl) StartNewSessionStreamed(ctx context.Context, userID, profileID uuid.UUID, cityName, message string, userLocation *types.UserLocation) (*types.StreamingResponse, error) {
	ctx, span := otel.Tracer("LlmInteractionService").Start(ctx, "StartNewSessionStreamed", trace.WithAttributes(
		attribute.String("city.name", cityName),
		attribute.String("user.id", userID.String()),
		attribute.String("profile.id", profileID.String()),
	))
	defer span.End()

	l.logger.DebugContext(ctx, "Starting new streamed chat session", slog.String("cityName", cityName), slog.String("userID", userID.String()))

	if message == "" {
		message = fmt.Sprintf("Plan a trip to %s", cityName)
	}

	// Create session early to persist partial data
	sessionID := uuid.New()
	eventCh := make(chan types.StreamEvent, 100)
	ctx, cancel := context.WithCancel(ctx)

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
		return nil, fmt.Errorf("failed to create session: %w", err)
	}

	go func() {
		defer close(eventCh)

		l.sendEvent(ctx, eventCh, types.StreamEvent{
			Type:      types.EventTypeStart,
			Data:      map[string]string{"session_id": sessionID.String()},
			Timestamp: time.Now(),
			EventID:   uuid.New().String(),
		}, 3)

		// Fetch user data
		l.sendEvent(ctx, eventCh, types.StreamEvent{
			Type:      types.EventTypeProgress,
			Data:      map[string]interface{}{"status": "fetching_user_data", "progress": 5},
			Timestamp: time.Now(),
			EventID:   uuid.New().String(),
		}, 3)
		interests, searchProfile, tags, err := l.FetchUserData(ctx, userID, profileID)
		if err != nil {
			span.RecordError(err)
			l.sendEvent(ctx, eventCh, types.StreamEvent{
				Type:      types.EventTypeError,
				Error:     fmt.Sprintf("failed to fetch user data: %v", err),
				Timestamp: time.Now(),
				EventID:   uuid.New().String(),
			}, 3)
			return
		}

		interestNames, tagsPromptPart, userPrefs := l.PreparePromptData(interests, tags, searchProfile)

		// Enhance with semantic search context - get contextually relevant POIs
		l.sendEvent(ctx, eventCh, types.StreamEvent{
			Type:      types.EventTypeProgress,
			Data:      map[string]interface{}{"status": "generating_semantic_context", "progress": 10},
			Timestamp: time.Now(),
			EventID:   uuid.New().String(),
		}, 3)

		var semanticPOIs []types.POIDetailedInfo
		if len(interestNames) > 0 {
			cityUUID, cityErr := l.cityRepo.GetCityIDByName(ctx, cityName)
			if cityErr == nil {
				// Generate semantic recommendations based on user message and interests
				searchQuery := fmt.Sprintf("%s %s", message, strings.Join(interestNames, " "))
				semanticPOIs, err = l.enhancePOIRecommendationsWithSemantics(ctx, searchQuery, cityUUID, interestNames, 20)
				if err != nil {
					l.logger.WarnContext(ctx, "Failed to get semantic POI recommendations", slog.Any("error", err))
					l.sendEvent(ctx, eventCh, types.StreamEvent{
						Type:      types.EventTypeProgress,
						Data:      map[string]interface{}{"status": "semantic_context_failed", "progress": 12},
						Timestamp: time.Now(),
						EventID:   uuid.New().String(),
					}, 3)
				} else {
					l.logger.InfoContext(ctx, "Enhanced streaming session with semantic POI context",
						slog.Int("semantic_pois_count", len(semanticPOIs)))
					l.sendEvent(ctx, eventCh, types.StreamEvent{
						Type: types.EventTypeProgress,
						Data: map[string]interface{}{
							"status":              "semantic_context_generated",
							"progress":            15,
							"semantic_pois_count": len(semanticPOIs),
						},
						Timestamp: time.Now(),
						EventID:   uuid.New().String(),
					}, 3)
				}
			}
		}

		// Channels for worker results
		cityDataCh := make(chan types.GenAIResponse, 1)
		generalPOICh := make(chan types.GenAIResponse, 1)
		personalizedPOICh := make(chan types.GenAIResponse, 1)

		var wg sync.WaitGroup
		wg.Add(3)

		go l.streamingCityDataWorker(&wg, ctx, cityName, cityDataCh, eventCh, userID)
		go l.streamingGeneralPOIWorker(&wg, ctx, cityName, generalPOICh, eventCh, userID)
		go l.streamingPersonalizedPOIWorkerWithSemantics(&wg, ctx, cityName, userID, profileID, personalizedPOICh, eventCh, interestNames, tagsPromptPart, userPrefs, semanticPOIs)

		go func() {
			wg.Wait()
			close(cityDataCh)
			close(generalPOICh)
			close(personalizedPOICh)
		}()

		var itinerary types.AiCityResponse
		var llmInteractionID uuid.UUID
		var rawPersonalisedPOIs []types.POIDetailedInfo
		var errors []error

		for i := 0; i < 3; i++ {
			select {
			case result, ok := <-cityDataCh:
				if ok && result.Err != nil {
					errors = append(errors, result.Err)
				} else if ok {
					itinerary.GeneralCityData = types.GeneralCityData{
						City:            result.City,
						Country:         result.Country,
						StateProvince:   result.StateProvince,
						Description:     result.CityDescription,
						CenterLatitude:  result.Latitude,
						CenterLongitude: result.Longitude,
					}
				}
			case result, ok := <-generalPOICh:
				if ok && result.Err != nil {
					errors = append(errors, result.Err)
				} else if ok {
					itinerary.PointsOfInterest = result.GeneralPOI
				}
			case result, ok := <-personalizedPOICh:
				if ok && result.Err != nil {
					errors = append(errors, result.Err)
				} else if ok {
					itinerary.AIItineraryResponse = types.AIItineraryResponse{
						ItineraryName:      result.ItineraryName,
						OverallDescription: result.ItineraryDescription,
						PointsOfInterest:   result.PersonalisedPOI,
					}
					llmInteractionID = result.LlmInteractionID
					rawPersonalisedPOIs = result.PersonalisedPOI
				}
			case <-ctx.Done():
				l.sendEvent(ctx, eventCh, types.StreamEvent{
					Type:      types.EventTypeError,
					Error:     ctx.Err().Error(),
					Timestamp: time.Now(),
					EventID:   uuid.New().String(),
				}, 3)
				return
			}
		}

		if len(errors) > 0 {
			err := fmt.Errorf("worker errors: %v", errors)
			span.RecordError(err)
			l.sendEvent(ctx, eventCh, types.StreamEvent{
				Type:      types.EventTypeError,
				Error:     err.Error(),
				Timestamp: time.Now(),
				EventID:   uuid.New().String(),
			}, 3)
			return
		}

		// Save data
		l.sendEvent(ctx, eventCh, types.StreamEvent{
			Type:      types.EventTypeProgress,
			Data:      map[string]interface{}{"status": "saving_data", "progress": 80},
			Timestamp: time.Now(),
			EventID:   uuid.New().String(),
		}, 3)

		cityID, err := l.HandleCityData(ctx, itinerary.GeneralCityData)
		if err != nil {
			span.RecordError(err)
			l.sendEvent(ctx, eventCh, types.StreamEvent{
				Type:      types.EventTypeError,
				Error:     fmt.Sprintf("failed to save city data: %v", err),
				Timestamp: time.Now(),
				EventID:   uuid.New().String(),
			}, 3)
			return
		}

		l.HandleGeneralPOIs(ctx, itinerary.PointsOfInterest, cityID)
		sortedPOIs, err := l.HandlePersonalisedPOIs(ctx, rawPersonalisedPOIs, cityID, userLocation, llmInteractionID, userID, profileID)
		if err != nil {
			span.RecordError(err)
			l.sendEvent(ctx, eventCh, types.StreamEvent{
				Type:      types.EventTypeError,
				Error:     fmt.Sprintf("failed to save personalised POIs: %v", err),
				Timestamp: time.Now(),
				EventID:   uuid.New().String(),
			}, 3)
			return
		}
		itinerary.AIItineraryResponse.PointsOfInterest = sortedPOIs

		// Update session with itinerary
		session.CurrentItinerary = &itinerary
		session.ConversationHistory = append(session.ConversationHistory, types.ConversationMessage{
			Role:        "assistant",
			Content:     fmt.Sprintf("Here's your personalized itinerary for %s", cityName),
			Timestamp:   time.Now(),
			MessageType: types.TypeItineraryResponse,
		})
		session.UpdatedAt = time.Now()
		if err := l.llmInteractionRepo.UpdateSession(ctx, session); err != nil {
			span.RecordError(err)
			l.sendEvent(ctx, eventCh, types.StreamEvent{
				Type:      types.EventTypeError,
				Error:     fmt.Sprintf("failed to update session: %v", err),
				Timestamp: time.Now(),
				EventID:   uuid.New().String(),
			}, 3)
			return
		}

		l.sendEvent(ctx, eventCh, types.StreamEvent{
			Type:      types.EventTypeItinerary,
			Data:      itinerary,
			Timestamp: time.Now(),
			EventID:   uuid.New().String(),
		}, 3)

		l.sendEvent(ctx, eventCh, types.StreamEvent{
			Type:      types.EventTypeComplete,
			Data:      map[string]interface{}{"session_id": sessionID.String(), "progress": 100},
			Timestamp: time.Now(),
			EventID:   uuid.New().String(),
			IsFinal:   true,
		}, 3)

		l.logger.InfoContext(ctx, "New session created and streamed",
			slog.String("session_id", sessionID.String()),
			slog.String("city_name", cityName))
		span.SetStatus(codes.Ok, "Session started successfully")
	}()

	return &types.StreamingResponse{
		SessionID: sessionID,
		Stream:    eventCh,
		Cancel:    cancel,
	}, nil
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
	var assistantMessageType types.MessageType = types.TypeResponse
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
			for i, poi := range session.CurrentItinerary.AIItineraryResponse.PointsOfInterest {
				if strings.Contains(strings.ToLower(poi.Name), oldPOI) {
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
			for i, poi := range session.CurrentItinerary.AIItineraryResponse.PointsOfInterest {
				if strings.Contains(strings.ToLower(poi.Name), oldPOI) {
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
		for i, poi := range session.CurrentItinerary.AIItineraryResponse.PointsOfInterest {
			if poi.ID == uuid.Nil {
				dbPoiID, saveErr := l.llmInteractionRepo.SaveSinglePOI(ctx, poi, session.UserID, cityID, poi.LlmInteractionID)
				if saveErr != nil {
					l.logger.WarnContext(ctx, "Failed to save new POI", slog.String("name", poi.Name), slog.Any("error", saveErr))
					continue
				}
				session.CurrentItinerary.AIItineraryResponse.PointsOfInterest[i].ID = dbPoiID
			}
		}

		var currentPOIIDs []uuid.UUID
		for _, p := range session.CurrentItinerary.AIItineraryResponse.PointsOfInterest {
			if p.ID != uuid.Nil {
				currentPOIIDs = append(currentPOIIDs, p.ID)
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
	l.sendEvent(ctx, eventCh, types.StreamEvent{Type: types.EventTypeComplete, Data: "Turn completed.", IsFinal: true}, 3)

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
		trace.WithAttributes(attribute.String("poi.name", poiName), attribute.String("city.name", cityName)))
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

// streamingCityDataWorker ContinueSessionStreamed

func (l *ServiceImpl) generateCityData(ctx context.Context, cityName string) (string, error) {
	ctx, span := otel.Tracer("LlmInteractionService").Start(ctx, "generateCityData", trace.WithAttributes(
		attribute.String("city.name", cityName),
	))
	defer span.End()

	prompt := getCityDescriptionPrompt(cityName)
	var responseText strings.Builder

	// Try streaming
	iter, err := l.aiClient.GenerateContentStream(ctx, prompt, &genai.GenerateContentConfig{Temperature: genai.Ptr[float32](defaultTemperature)})
	if err == nil {
		for resp, err := range iter {
			if err != nil {
				span.RecordError(err)
				return "", fmt.Errorf("streaming city data error: %w", err)
			}
			for _, cand := range resp.Candidates {
				if cand.Content != nil {
					for _, part := range cand.Content.Parts {
						if part.Text != "" {
							responseText.WriteString(string(part.Text))
						}
					}
				}
			}
		}
	} else {
		// Fallback to non-streaming
		l.logger.WarnContext(ctx, "Streaming city data failed, falling back to non-streaming", slog.Any("error", err))
		response, err := l.aiClient.GenerateResponse(ctx, prompt, &genai.GenerateContentConfig{Temperature: genai.Ptr[float32](defaultTemperature)})
		if err != nil {
			span.RecordError(err)
			return "", fmt.Errorf("failed to generate city data: %w", err)
		}
		for _, cand := range response.Candidates {
			if cand.Content != nil {
				for _, part := range cand.Content.Parts {
					if part.Text != "" {
						responseText.WriteString(string(part.Text))
					}
				}
			}
		}
	}

	fullText := responseText.String()
	if fullText == "" {
		err := fmt.Errorf("empty city data response")
		span.RecordError(err)
		return "", err
	}

	return cleanJSONResponse(fullText), nil
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

		// Check if any semantic POI matches what user is asking for
		for _, semanticPOI := range semanticPOIs[:min(3, len(semanticPOIs))] {
			// Check if this semantic POI is already in itinerary
			alreadyExists := false
			for _, existingPOI := range session.CurrentItinerary.AIItineraryResponse.PointsOfInterest {
				if strings.EqualFold(existingPOI.Name, semanticPOI.Name) {
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
					for i, poi := range semanticPOIs[:min(3, len(semanticPOIs))] {
						names = append(names, poi.Name)
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
				for i, poi := range semanticPOIs[:min(3, len(semanticPOIs))] {
					names = append(names, poi.Name)
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
	for _, poi := range session.CurrentItinerary.AIItineraryResponse.PointsOfInterest {
		if strings.EqualFold(poi.Name, poiName) {
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

/*
** Unified Response
 */
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
	cacheKeyBytes, _ := json.Marshal(cacheKeyData)
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
			l.sendEvent(ctx, eventCh, types.StreamEvent{
				Type: types.EventTypeComplete,
				Data: map[string]interface{}{"session_id": sessionID.String()},
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
		if cityDataContent != "" && domain == types.DomainItinerary {
			// Parse city data from the response
			if parsedCityData, parseErr := l.parseCityDataFromResponse(asyncCtx, cityDataContent); parseErr == nil && parsedCityData != nil {
				// Save city data to the cities table
				if _, handleErr := l.HandleCityData(asyncCtx, *parsedCityData); handleErr != nil {
					l.logger.WarnContext(asyncCtx, "Failed to save city data during unified stream processing",
						slog.String("city", cityName), slog.Any("error", handleErr))
				} else {
					l.logger.InfoContext(asyncCtx, "Successfully saved city data during unified stream processing",
						slog.String("city", cityName))
				}
			} else if parseErr != nil {
				l.logger.WarnContext(asyncCtx, "Failed to parse city data from unified stream response",
					slog.String("city", cityName), slog.Any("error", parseErr))
			}
		}

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
		if _, err := l.llmInteractionRepo.SaveInteraction(asyncCtx, interaction); err != nil {
			l.logger.ErrorContext(asyncCtx, "Failed to save stream interaction", slog.Any("error", err))
		}
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
func (l *ServiceImpl) parseCityDataFromResponse(ctx context.Context, responseContent string) (*types.GeneralCityData, error) {
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
