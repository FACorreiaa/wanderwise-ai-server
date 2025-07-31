package chat_prompt

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"google.golang.org/genai"

	"github.com/FACorreiaa/go-poi-au-suggestions/internal/domain/city"
	"github.com/FACorreiaa/go-poi-au-suggestions/internal/domain/poi"
)

func (l *Service) ContinueSessionStreamed(
	ctx context.Context, sessionID uuid.UUID,
	message string, userLocation *UserLocation,
	eventCh chan<- StreamEvent,
) error {
	ctx, span := otel.Tracer("LlmInteractionService").Start(ctx, "ContinueSessionStreamed", trace.WithAttributes(
		attribute.String("session.id", sessionID.String()),
		attribute.String("message", message),
	))
	defer span.End()

	l.logger.Debug("Continuing streamed chat session", zap.String("sessionID", sessionID.String()), zap.String("message", message))

	session, err := l.repo.GetSession(ctx, sessionID)
	if err != nil {
		err = fmt.Errorf("failed to get session %s: %w", sessionID, err)
		l.sendEvent(ctx, eventCh, StreamEvent{Type: EventTypeError, Error: err.Error(), IsFinal: true}, 3)
		return err
	}
	if session.Status != StatusActive {
		err = fmt.Errorf("session %s is not active (status: %s) %w", sessionID, session.Status, err)
		l.sendEvent(ctx, eventCh, StreamEvent{Type: EventTypeError, Error: err.Error(), IsFinal: true}, 3)
		return err
	}
	l.sendEvent(ctx, eventCh, StreamEvent{Type: "session_validated", Data: map[string]string{"status": "active"}}, 3)

	cityData, err := l.cityRepo.FindCityByNameAndCountry(ctx, session.SessionContext.CityName, "")
	if err != nil || cityData == nil {
		cityData, err = l.cityRepo.FindCityByFuzzyName(ctx, session.SessionContext.CityName)
		if err != nil || cityData == nil {
			// Create a fallback city entry if none exists
			l.logger.Warn("City not found in database, creating fallback entry", 
				zap.String("city_name", session.SessionContext.CityName),
				zap.String("session_id", sessionID.String()))
			
			fallbackCity := city.CityDetail{
				Name:    session.SessionContext.CityName,
				Country: "Unknown", // Will be updated when we have more data
				AiSummary: fmt.Sprintf("City information for %s (created as fallback)", session.SessionContext.CityName),
			}
			
			cityID, saveErr := l.cityRepo.SaveCity(ctx, fallbackCity)
			if saveErr != nil {
				l.logger.Error("Failed to create fallback city", 
					zap.String("city_name", session.SessionContext.CityName),
					zap.Error(saveErr))
				if err == nil {
					err = fmt.Errorf("city '%s' not found for session %s and failed to create fallback", session.SessionContext.CityName, sessionID)
				} else {
					err = fmt.Errorf("failed to find city '%s' for session %s: %w", session.SessionContext.CityName, sessionID, err)
				}
				l.sendEvent(ctx, eventCh, StreamEvent{Type: EventTypeError, Error: err.Error(), IsFinal: true}, 3)
				return err
			}
			
			// Create cityData with the new ID
			cityData = &city.CityDetail{
				ID:        cityID,
				Name:      session.SessionContext.CityName,
				Country:   "Unknown",
				AiSummary: fallbackCity.AiSummary,
			}
			
			l.logger.Info("Created fallback city entry", 
				zap.String("city_name", session.SessionContext.CityName),
				zap.String("city_id", cityID.String()))
		}
	}
	cityID := cityData.ID
	l.sendEvent(ctx, eventCh, StreamEvent{Type: EventTypeProgress, Data: map[string]interface{}{"status": "context_loaded", "city_id": cityID.String()}}, 3)

	userMessage := ConversationMessage{
		ID: uuid.New(), Role: RoleUser, Content: message, Timestamp: time.Now(), MessageType: TypeModificationRequest,
	}
	if err := l.repo.AddMessageToSession(ctx, sessionID, userMessage); err != nil {
		l.logger.Warn("Failed to persist user message, continuing with in-memory history", zap.Error(err))
		span.RecordError(err, trace.WithAttributes(attribute.String("warning", "User message DB save failed")))
	}
	session.ConversationHistory = append(session.ConversationHistory, userMessage)

	intent, err := l.intentClassifier.Classify(ctx, message)
	if err != nil {
		err = fmt.Errorf("failed to classify intent for message '%s': %w", message, err)
		l.sendEvent(ctx, eventCh, StreamEvent{Type: EventTypeError, Error: err.Error(), IsFinal: true}, 3)
		return err
	}
	l.logger.Info("Intent classified", zap.String("intent", string(intent)))
	l.sendEvent(ctx, eventCh, StreamEvent{Type: "intent_classified", Data: map[string]string{"intent": string(intent)}}, 3)

	l.sendEvent(ctx, eventCh, StreamEvent{
		Type: EventTypeProgress,
		Data: map[string]interface{}{"status": "generating_semantic_context", "progress": 20},
	}, 3)

	semanticPOIs, err := l.generateSemanticPOIRecommendations(ctx, message, cityID, session.UserID, userLocation, 0.6)
	if err != nil {
		l.logger.Warn("Failed to generate semantic POI recommendations for streaming session", zap.Error(err))
		l.sendEvent(ctx, eventCh, StreamEvent{
			Type: EventTypeProgress,
			Data: map[string]interface{}{"status": "semantic_context_failed", "progress": 22},
		}, 3)
	} else {
		l.logger.Info("Generated semantic POI recommendations for streaming session",
			zap.Int("semantic_recommendations", len(semanticPOIs)))
		l.sendEvent(ctx, eventCh, StreamEvent{
			Type: "semantic_context_generated",
			Data: map[string]interface{}{
				"status":                         "semantic_context_ready",
				"semantic_recommendations_count": len(semanticPOIs),
				"progress":                       25,
			},
		}, 3)
	}

	var finalResponseMessage string
	assistantMessageType := TypeResponse
	itineraryModifiedByThisTurn := false

	switch intent {
	case IntentAddPOI:
		l.sendEvent(ctx, eventCh, StreamEvent{Type: EventTypeProgress, Data: "Processing: Adding Point of Interest with semantic enhancement..."}, 3)
		var genErr error
		finalResponseMessage, genErr = l.handleSemanticAddPOIStreamed(ctx, message, session, semanticPOIs, userLocation, cityID, eventCh)
		if genErr != nil {
			finalResponseMessage = "I had trouble understanding your request. Could you please specify which POI you'd like to add?"
			assistantMessageType = TypeError
		} else {
			itineraryModifiedByThisTurn = true
		}

	case IntentRemovePOI:
		l.sendEvent(ctx, eventCh, StreamEvent{Type: EventTypeProgress, Data: "Processing: Removing Point of Interest with semantic understanding..."}, 3)
		finalResponseMessage = l.handleSemanticRemovePOI(ctx, message, session)
		if strings.Contains(finalResponseMessage, "I've removed") {
			itineraryModifiedByThisTurn = true
		}

	case IntentAskQuestion:
		l.sendEvent(ctx, eventCh, StreamEvent{Type: EventTypeProgress, Data: "Processing: Answering your question with semantic context..."}, 3)
		finalResponseMessage = "I'm here to help! For now, I'll assume you're asking about your trip. What specifically would you like to know?"

	case "replace_poi":
		l.sendEvent(ctx, eventCh, StreamEvent{Type: EventTypeProgress, Data: "Processing: Replacing Point of Interest..."}, 3)
		if matches := regexp.MustCompile(`replace\s+(.+?)\s+with\s+(.+?)(?:\s+in\s+my\s+itinerary)?`).FindStringSubmatch(strings.ToLower(message)); len(matches) == 3 {
			oldPOI := matches[1]
			newPOIName := matches[2]
			for i, p := range session.CurrentItinerary.AIItineraryResponse.PointsOfInterest {
				if strings.Contains(strings.ToLower(p.Name), oldPOI) {
					newPOI, err := l.generatePOIDataStream(ctx, newPOIName, session.SessionContext.CityName, userLocation, session.UserID, cityID, eventCh)
					if err != nil {
						finalResponseMessage = fmt.Sprintf("Could not replace %s with %s due to an error: %v", oldPOI, newPOIName, err)
						assistantMessageType = TypeError
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
			assistantMessageType = TypeClarification
		}

	default:
		l.sendEvent(ctx, eventCh, StreamEvent{Type: EventTypeProgress, Data: "Processing: Updating itinerary..."}, 3)
		if matches := regexp.MustCompile(`replace\s+(.+?)\s+with\s+(.+?)(?:\s+in\s+my\s+itinerary)?`).FindStringSubmatch(strings.ToLower(message)); len(matches) == 3 {
			oldPOI := matches[1]
			newPOIName := matches[2]
			for i, p := range session.CurrentItinerary.AIItineraryResponse.PointsOfInterest {
				if strings.Contains(strings.ToLower(p.Name), oldPOI) {
					newPOI, err := l.generatePOIData(ctx, newPOIName, session.SessionContext.CityName, userLocation, session.UserID, cityID)
					if err != nil {
						l.logger.Error("Failed to generate POI data", zap.Error(err))
						span.RecordError(err)
						finalResponseMessage = fmt.Sprintf("Could not replace %s with %s due to an error.", oldPOI, newPOIName)
					} else {
						session.CurrentItinerary.AIItineraryResponse.PointsOfInterest[i] = newPOI
						finalResponseMessage = fmt.Sprintf("I've replaced %s with %s in your itinerary.", oldPOI, newPOIName)
					}
					break
				}
			}
			if finalResponseMessage == "" {
				finalResponseMessage = fmt.Sprintf("Could not find %s in your itinerary.", oldPOI)
			}
		} else {
			finalResponseMessage = "I've noted your request to modify the itinerary. Please specify the changes (e.g., 'replace X with Y')."
		}
	}

	if itineraryModifiedByThisTurn && userLocation != nil && userLocation.UserLat != 0 && userLocation.UserLon != 0 && session.CurrentItinerary != nil {
		l.sendEvent(ctx, eventCh, StreamEvent{Type: EventTypeProgress, Data: "Sorting updated POIs by distance..."}, 3)
		for i, p := range session.CurrentItinerary.AIItineraryResponse.PointsOfInterest {
			if p.ID == uuid.Nil {
				dbPoiID, saveErr := l.repo.SaveSinglePOI(ctx, p, session.UserID, cityID, p.LlmInteractionID)
				if saveErr != nil {
					l.logger.Warn("Failed to save new POI", zap.String("name", p.Name), zap.Error(saveErr))
					continue
				}
				session.CurrentItinerary.AIItineraryResponse.PointsOfInterest[i].ID = dbPoiID
			}
		}

		if (intent == IntentAddPOI || intent == IntentModifyItinerary) && userLocation != nil && userLocation.UserLat != 0 && userLocation.UserLon != 0 {
			sortedPOIs, err := l.repo.GetPOIsBySessionSortedByDistance(ctx, sessionID, cityID, *userLocation)
			if err != nil {
				l.logger.Warn("Failed to sort POIs by distance", zap.Error(err))
				span.RecordError(err)
			} else {
				session.CurrentItinerary.AIItineraryResponse.PointsOfInterest = sortedPOIs
				l.logger.Info("POIs sorted by distance",
					zap.Int("poi_count", len(sortedPOIs)))
				span.SetAttributes(attribute.Int("sorted_pois.count", len(sortedPOIs)))
			}
		}
	}

	assistantMessage := ConversationMessage{
		ID: uuid.New(), Role: RoleAssistant, Content: finalResponseMessage, Timestamp: time.Now(), MessageType: assistantMessageType,
	}
	if err := l.repo.AddMessageToSession(ctx, sessionID, assistantMessage); err != nil {
		l.logger.Warn("Failed to save assistant message", zap.Error(err))
	}
	session.ConversationHistory = append(session.ConversationHistory, assistantMessage)

	session.UpdatedAt = time.Now()
	session.ExpiresAt = time.Now().Add(24 * time.Hour)
	if err := l.repo.UpdateSession(ctx, *session); err != nil {
		err = fmt.Errorf("failed to update session %s: %w", sessionID, err)
		l.sendEvent(ctx, eventCh, StreamEvent{Type: EventTypeError, Error: err.Error(), IsFinal: true}, 3)
		return err
	}

	l.sendEvent(ctx, eventCh, StreamEvent{
		Type:      EventTypeItinerary,
		Data:      session.CurrentItinerary,
		Message:   finalResponseMessage,
		Timestamp: time.Now(),
	}, 3)
	l.sendEvent(ctx, eventCh, StreamEvent{
		Type: EventTypeComplete,
		Data: map[string]interface{}{
			"message":         "Turn completed successfully",
			"session_id":      sessionID.String(),
			"full_response":   finalResponseMessage,
			"final_response":  finalResponseMessage,
		},
		IsFinal: true,
		Navigation: &NavigationData{
			URL:       fmt.Sprintf("/itinerary?sessionId=%s&cityName=%s&domain=itinerary", sessionID.String(), url.QueryEscape(session.CityName)),
			RouteType: "itinerary",
			QueryParams: map[string]string{
				"sessionId": sessionID.String(),
				"cityName":  session.CityName,
				"domain":    "itinerary",
			},
		},
	}, 3)

	l.logger.Info("Streamed session continued", zap.String("sessionID", sessionID.String()), zap.String("intent", string(intent)))
	return nil
}

func (l *Service) generatePOIDataStream(
	ctx context.Context, poiName, cityName string,
	userLocation *UserLocation, userID, cityID uuid.UUID,
	eventCh chan<- StreamEvent,
) (poi.POIDetailedInfo, error) {
	ctx, span := otel.Tracer("LlmInteractionService").Start(ctx, "generatePOIDataStream",
		trace.WithAttributes(attribute.String("p.name", poiName), attribute.String("city.name", cityName)))
	defer span.End()

	prompt := generatedContinuedConversationPrompt(poiName, cityName)
	config := &genai.GenerateContentConfig{Temperature: genai.Ptr[float32](0.2)}
	startTime := time.Now()

	var responseTextBuilder strings.Builder
	iter, err := l.aiClient.GenerateContentStream(ctx, prompt, config)
	if err != nil {
		l.sendEvent(ctx, eventCh, StreamEvent{
			Type:      EventTypeError,
			Error:     fmt.Sprintf("Failed to generate POI data for '%s': %v", poiName, err),
			Timestamp: time.Now(),
			EventID:   uuid.New().String(),
		}, 3)
		return poi.POIDetailedInfo{}, fmt.Errorf("AI stream init failed for POI '%s': %w", poiName, err)
	}

	l.sendEvent(ctx, eventCh, StreamEvent{
		Type:      EventTypeProgress,
		Data:      map[string]string{"status": fmt.Sprintf("Getting details for %s...", poiName)},
		Timestamp: time.Now(),
		EventID:   uuid.New().String(),
	}, 3)

	for resp, err := range iter {
		if err != nil {
			l.sendEvent(ctx, eventCh, StreamEvent{
				Type:      EventTypeError,
				Error:     fmt.Sprintf("Streaming failed for POI '%s': %v", poiName, err),
				Timestamp: time.Now(),
				EventID:   uuid.New().String(),
			}, 3)
			return poi.POIDetailedInfo{}, fmt.Errorf("streaming POI details for '%s' failed: %w", poiName, err)
		}
		for _, cand := range resp.Candidates {
			if cand.Content != nil {
				for _, part := range cand.Content.Parts {
					if part.Text != "" {
						responseTextBuilder.WriteString(string(part.Text))
						l.sendEvent(ctx, eventCh, StreamEvent{
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
		l.sendEvent(ctx, eventCh, StreamEvent{
			Type:      EventTypeError,
			Error:     ctx.Err().Error(),
			Timestamp: time.Now(),
			EventID:   uuid.New().String(),
		}, 3)
		return poi.POIDetailedInfo{}, fmt.Errorf("context cancelled during POI detail generation: %w", ctx.Err())
	}

	fullText := responseTextBuilder.String()
	if fullText == "" {
		l.sendEvent(ctx, eventCh, StreamEvent{
			Type:      EventTypeError,
			Error:     fmt.Sprintf("Empty response for POI '%s'", poiName),
			Timestamp: time.Now(),
			EventID:   uuid.New().String(),
		}, 3)
		return poi.POIDetailedInfo{Name: poiName, DescriptionPOI: "Details not found."}, fmt.Errorf("empty response for POI details '%s'", poiName)
	}

	interaction := LlmInteraction{
		UserID:       userID,
		Prompt:       prompt,
		ResponseText: fullText,
		Timestamp:    startTime,
		CityName:     cityName,
	}
	llmInteractionID, err := l.saveCityInteraction(ctx, interaction)
	if err != nil {
		l.sendEvent(ctx, eventCh, StreamEvent{
			Type:      EventTypeError,
			Error:     fmt.Sprintf("Failed to save LLM interaction for POI '%s': %v", poiName, err),
			Timestamp: time.Now(),
			EventID:   uuid.New().String(),
		}, 3)
		return poi.POIDetailedInfo{}, fmt.Errorf("failed to save LLM interaction: %w", err)
	}

	cleanJSON := cleanJSONResponse(fullText)
	var poiData poi.POIDetailedInfo
	if err := json.Unmarshal([]byte(cleanJSON), &poiData); err != nil || poiData.Name == "" {
		l.logger.Warn("Invalid POI data from LLM", zap.String("response", fullText), zap.Error(err))
		poiData = poi.POIDetailedInfo{
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

	dbPoiID, err := l.repo.SaveSinglePOI(ctx, poiData, userID, cityID, llmInteractionID)
	if err != nil {
		l.logger.Warn("Failed to save POI to database", zap.Error(err))
		span.RecordError(err)
		l.sendEvent(ctx, eventCh, StreamEvent{
			Type:      EventTypeError,
			Error:     fmt.Sprintf("Failed to save POI '%s' to database: %v", poiName, err),
			Timestamp: time.Now(),
			EventID:   uuid.New().String(),
		}, 3)
		return poi.POIDetailedInfo{}, fmt.Errorf("failed to save POI to database: %w", err)
	}
	poiData.ID = dbPoiID

	if userLocation != nil && userLocation.UserLat != 0 && userLocation.UserLon != 0 && poiData.Latitude != 0 && poiData.Longitude != 0 {
		distance, err := l.poiRepo.CalculateDistancePostGIS(ctx, userLocation.UserLat, userLocation.UserLon, poiData.Latitude, poiData.Longitude)
		if err != nil {
			l.logger.Warn("Failed to calculate distance", zap.Error(err))
			span.RecordError(err)
		} else {
			poiData.Distance = distance
		}
	}

	l.sendEvent(ctx, eventCh, StreamEvent{
		Type:      "poi_detail_complete",
		Data:      poiData,
		Timestamp: time.Now(),
		EventID:   uuid.New().String(),
	}, 3)
	return poiData, nil
}

func (l *Service) handleSemanticAddPOIStreamed(ctx context.Context, message string, session *ChatSession, semanticPOIs []poi.POIDetailedInfo, userLocation *UserLocation, cityID uuid.UUID, eventCh chan<- StreamEvent) (string, error) {
	ctx, span := otel.Tracer("LlmInteractionService").Start(ctx, "handleSemanticAddPOIStreamed")
	defer span.End()

	l.ensureItineraryExists(session)

	if len(semanticPOIs) > 0 {
		l.sendEvent(ctx, eventCh, StreamEvent{
			Type: EventTypeProgress,
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
				l.sendEvent(ctx, eventCh, StreamEvent{
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

				session.CurrentItinerary.AIItineraryResponse.PointsOfInterest = append(
					session.CurrentItinerary.AIItineraryResponse.PointsOfInterest, semanticPOI)
				l.logger.Info("Added semantic POI to streaming itinerary",
					zap.String("poi_name", semanticPOI.Name))
				span.SetAttributes(attribute.String("added_poi", semanticPOI.Name))

				return fmt.Sprintf("Great! I found %s which matches what you're looking for. I've added it to your itinerary. %s",
					semanticPOI.Name, semanticPOI.DescriptionPOI), nil
			}
		}

		l.sendEvent(ctx, eventCh, StreamEvent{
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

	l.sendEvent(ctx, eventCh, StreamEvent{
		Type: EventTypeProgress,
		Data: map[string]interface{}{"status": "extracting_poi_name"},
	}, 3)

	poiName := extractPOIName(message)
	if poiName == "" {
		return "I'd be happy to add a POI to your itinerary! Could you please specify which place you'd like to add?", nil
	}

	for _, p := range session.CurrentItinerary.AIItineraryResponse.PointsOfInterest {
		if strings.EqualFold(p.Name, poiName) {
			return fmt.Sprintf("%s is already in your itinerary.", poiName), nil
		}
	}

	l.sendEvent(ctx, eventCh, StreamEvent{
		Type: EventTypeProgress,
		Data: map[string]interface{}{
			"status":   "generating_poi_data",
			"poi_name": poiName,
		},
	}, 3)

	newPOI, err := l.generatePOIDataStream(ctx, poiName, session.SessionContext.CityName, userLocation, session.UserID, cityID, eventCh)
	if err != nil {
		l.logger.Error("Failed to generate POI data for streaming", zap.Error(err))
		span.RecordError(err)
		return "", fmt.Errorf("failed to generate POI data: %w", err)
	}

	session.CurrentItinerary.AIItineraryResponse.PointsOfInterest = append(
		session.CurrentItinerary.AIItineraryResponse.PointsOfInterest, newPOI)

	l.sendEvent(ctx, eventCh, StreamEvent{
		Type: "poi_added_successfully",
		Data: map[string]interface{}{
			"poi_name":       newPOI.Name,
			"poi_category":   newPOI.Category,
			"semantic_match": false,
		},
	}, 3)

	return fmt.Sprintf("I've added %s to your itinerary.", poiName), nil
}
