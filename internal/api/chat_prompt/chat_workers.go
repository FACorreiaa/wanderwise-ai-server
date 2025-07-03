package llmChat

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
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
