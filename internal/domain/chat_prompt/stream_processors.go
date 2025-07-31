package chat_prompt

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"google.golang.org/genai"

	"github.com/FACorreiaa/go-poi-au-suggestions/internal/domain/profiles"
)

func (l *Service) extractLocationData(metadata map[string]string) (string, *UserLocation) {
	cityName := metadata["city_name"]
	latStr, latOk := metadata["user_lat"]
	lonStr, lonOk := metadata["user_lon"]

	if latOk && lonOk {
		lat, errLat := strconv.ParseFloat(latStr, 64)
		lon, errLon := strconv.ParseFloat(lonStr, 64)
		if errLat == nil && errLon == nil {
			return cityName, &UserLocation{UserLat: lat, UserLon: lon}
		}
	}

	return cityName, nil
}

func (l *Service) ProcessUnifiedChatMessageStream(ctx context.Context, userID, profileID uuid.UUID, message string, metadata map[string]string, eventCh chan<- StreamEvent) error {
	ctx, span := otel.Tracer("LlmInteractionService").Start(ctx, "ProcessUnifiedChatMessageStream")
	defer span.End()

	fmt.Printf("DEBUG: ProcessUnifiedChatMessageStream started - user: %s, message: %s\n", userID.String(), message)

	cityName, userLocation := l.extractLocationData(metadata)
	fmt.Printf("DEBUG: Extracted city: %s, userLocation: %v\n", cityName, userLocation)

	// Extract city from message if not present in metadata
	if cityName == "" {
		fmt.Printf("DEBUG: No city in metadata, extracting from message\n")
		extractedCity, _, err := l.extractCityFromMessage(ctx, message)
		if err != nil {
			fmt.Printf("DEBUG: Failed to extract city from message: %v\n", err)
			return fmt.Errorf("failed to parse message: %w", err)
		}
		cityName = extractedCity
		fmt.Printf("DEBUG: Extracted city from message: %s\n", cityName)
	}

	fmt.Printf("DEBUG: Calling processStream with city: %s\n", cityName)
	// Don't use goroutine here - we want to wait for processing to complete
	err := l.processStream(ctx, userID, profileID, cityName, message, userLocation, eventCh)
	fmt.Printf("DEBUG: processStream completed with error: %v\n", err)
	return err
}

func (l *Service) processStream(ctx context.Context, userID, profileID uuid.UUID, cityName, message string, userLocation *UserLocation, eventCh chan<- StreamEvent) error {
	_, span := otel.Tracer("LlmInteractionService").Start(ctx, "processStream")
	defer span.End()

	startTime := time.Now()
	fmt.Printf("DEBUG: processStream started for city: %s at %s\n", cityName, startTime.Format(time.RFC3339))

	// Create a new chat session
	sessionID := uuid.New()
	session := ChatSession{
		ID:        sessionID,
		UserID:    userID,
		ProfileID: profileID,
		CityName:  cityName,
		ConversationHistory: []ConversationMessage{
			{
				ID:          uuid.New(),
				Role:        RoleUser,
				Content:     message,
				Timestamp:   startTime,
				MessageType: TypeInitialRequest,
			},
		},
		SessionContext: SessionContext{
			CityName:            cityName,
			ConversationSummary: fmt.Sprintf("Initial request for %s", cityName),
		},
		CreatedAt: startTime,
		UpdatedAt: startTime,
		ExpiresAt: startTime.Add(24 * time.Hour),
		Status:    StatusActive,
	}

	// Save the session to database
	if err := l.repo.CreateSession(ctx, session); err != nil {
		fmt.Printf("DEBUG: Failed to create session: %v\n", err)
		l.sendEvent(ctx, eventCh, StreamEvent{
			Type:  EventTypeError,
			Error: fmt.Sprintf("Failed to create session: %v", err),
		}, 3)
		return fmt.Errorf("failed to create session: %w", err)
	}

	fmt.Printf("DEBUG: Created session ID: %s\n", sessionID.String())

	// Send start event with session ID
	fmt.Printf("DEBUG: Sending start event\n")
	l.sendEvent(ctx, eventCh, StreamEvent{
		Type: EventTypeStart,
		Data: map[string]interface{}{
			"message":    fmt.Sprintf("Starting chat for city: %s", cityName),
			"start_time": startTime.Format(time.RFC3339),
			"session_id": sessionID.String(),
		},
	}, 3)

	// Create a simple prompt for the LLM
	prompt := fmt.Sprintf("You are a helpful travel assistant. The user asked: %s. Please provide a helpful response about %s.", message, cityName)
	fmt.Printf("DEBUG: Created prompt: %s\n", prompt)
	
	// Use the AI client to generate streaming response  
	fmt.Printf("DEBUG: Calling GenerateContentStream\n")
	iter, err := l.aiClient.GenerateContentStream(ctx, prompt, nil)
	if err != nil {
		fmt.Printf("DEBUG: GenerateContentStream failed: %v\n", err)
		l.sendEvent(ctx, eventCh, StreamEvent{
			Type:  EventTypeError,
			Error: fmt.Sprintf("Failed to start LLM stream: %v", err), 
		}, 3)
		return err
	}

	fmt.Printf("DEBUG: Starting to iterate over stream responses\n")
	// Stream the response chunks
	chunkCount := 0
	var fullResponse strings.Builder
	for resp, err := range iter {
		chunkCount++
		fmt.Printf("DEBUG: Received chunk %d\n", chunkCount)
		if err != nil {
			l.sendEvent(ctx, eventCh, StreamEvent{
				Type:  EventTypeError,
				Error: fmt.Sprintf("Error during streaming: %v", err),
			}, 3)
			return err
		}
		
		if resp != nil && len(resp.Candidates) > 0 && resp.Candidates[0].Content != nil {
			for _, part := range resp.Candidates[0].Content.Parts {
				if part.Text != "" {
					chunkText := part.Text
					fullResponse.WriteString(chunkText)
					fmt.Printf("DEBUG: AI chunk content: %q\n", chunkText)
					
					// Send each chunk as a streaming event with session ID
					l.sendEvent(ctx, eventCh, StreamEvent{
						Type: EventTypeChunk,
						Data: map[string]interface{}{
							"session_id": sessionID.String(),
							"chunk":      chunkText,
						},
					}, 3)
				}
			}
		}
	}
	
	endTime := time.Now()
	duration := endTime.Sub(startTime)
	fmt.Printf("DEBUG: Full AI response: %q\n", fullResponse.String())
	fmt.Printf("DEBUG: processStream completed at %s, duration: %v\n", endTime.Format(time.RFC3339), duration)

	// Save the assistant's response to the session
	assistantMessage := ConversationMessage{
		ID:          uuid.New(),
		Role:        RoleAssistant,
		Content:     fullResponse.String(),
		Timestamp:   endTime,
		MessageType: TypeResponse,
	}
	
	if err := l.repo.AddMessageToSession(ctx, sessionID, assistantMessage); err != nil {
		fmt.Printf("DEBUG: Failed to save assistant message to session: %v\n", err)
		// Don't fail the entire stream for this, just log it
	}

	// Send completion event with session ID and full response
	l.sendEvent(ctx, eventCh, StreamEvent{
		Type: EventTypeComplete,
		Data: map[string]interface{}{
			"message":         "Stream completed successfully",
			"start_time":      startTime.Format(time.RFC3339),
			"end_time":        endTime.Format(time.RFC3339),
			"duration_ms":     duration.Milliseconds(),
			"total_chunks":    chunkCount,
			"session_id":      sessionID.String(),
			"full_response":   fullResponse.String(),
		},
	}, 3)
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

func (l *Service) ProcessUnifiedChatMessageStreamFree(ctx context.Context, cityName, message string, userLocation *UserLocation, eventCh chan<- StreamEvent) error {
	ctx, span := otel.Tracer("LlmInteractionService").Start(ctx, "ProcessUnifiedChatMessageStreamFree")
	defer span.End()

	fmt.Printf("DEBUG: ProcessUnifiedChatMessageStreamFree started - message: %s\n", message)

	// Extract city from message if not present in metadata
	if cityName == "" {
		fmt.Printf("DEBUG: No city provided, extracting from message\n")
		extractedCity, _, err := l.extractCityFromMessage(ctx, message)
		if err != nil {
			fmt.Printf("DEBUG: Failed to extract city from message: %v\n", err)
			return fmt.Errorf("failed to parse message: %w", err)
		}
		cityName = extractedCity
		fmt.Printf("DEBUG: Extracted city from message: %s\n", cityName)
	}

	fmt.Printf("DEBUG: Calling processStreamFree with city: %s\n", cityName)
	// Process as a free chat stream (no user/profile requirements)
	err := l.processStreamFree(ctx, cityName, message, userLocation, eventCh)
	fmt.Printf("DEBUG: processStreamFree completed with error: %v\n", err)
	return err
}

func (l *Service) processStreamFree(ctx context.Context, cityName, message string, userLocation *UserLocation, eventCh chan<- StreamEvent) error {
	_, span := otel.Tracer("LlmInteractionService").Start(ctx, "processStreamFree")
	defer span.End()

	startTime := time.Now()
	fmt.Printf("DEBUG: processStreamFree started for city: %s at %s\n", cityName, startTime.Format(time.RFC3339))

	// Send start event
	fmt.Printf("DEBUG: Sending start event\n")
	l.sendEvent(ctx, eventCh, StreamEvent{
		Type: EventTypeStart,
		Data: map[string]interface{}{
			"message":    fmt.Sprintf("Starting free chat for city: %s", cityName),
			"start_time": startTime.Format(time.RFC3339),
		},
	}, 3)

	// Create a simple prompt for the LLM (free version - basic travel assistance)
	prompt := fmt.Sprintf("You are a helpful travel assistant. The user asked: %s. Please provide a helpful response about %s.", message, cityName)
	fmt.Printf("DEBUG: Created prompt: %s\n", prompt)
	
	// Use the AI client to generate streaming response  
	fmt.Printf("DEBUG: Calling GenerateContentStream\n")
	iter, err := l.aiClient.GenerateContentStream(ctx, prompt, nil)
	if err != nil {
		fmt.Printf("DEBUG: GenerateContentStream failed: %v\n", err)
		l.sendEvent(ctx, eventCh, StreamEvent{
			Type:  EventTypeError,
			Error: fmt.Sprintf("Failed to start LLM stream: %v", err), 
		}, 3)
		return err
	}

	fmt.Printf("DEBUG: Starting to iterate over stream responses\n")
	// Stream the response chunks
	chunkCount := 0
	var fullResponse strings.Builder
	for resp, err := range iter {
		chunkCount++
		fmt.Printf("DEBUG: Received chunk %d\n", chunkCount)
		if err != nil {
			l.sendEvent(ctx, eventCh, StreamEvent{
				Type:  EventTypeError,
				Error: fmt.Sprintf("Error during streaming: %v", err),
			}, 3)
			return err
		}
		
		if resp != nil && len(resp.Candidates) > 0 && resp.Candidates[0].Content != nil {
			for _, part := range resp.Candidates[0].Content.Parts {
				if part.Text != "" {
					chunkText := part.Text
					fullResponse.WriteString(chunkText)
					fmt.Printf("DEBUG: AI chunk content: %q\n", chunkText)
					
					// Send each chunk as a streaming event with session ID
					l.sendEvent(ctx, eventCh, StreamEvent{
						Type: EventTypeChunk,
						Data: map[string]interface{}{
							"session_id": "",
							"chunk":      chunkText,
						},
					}, 3)
				}
			}
		}
	}
	
	endTime := time.Now()
	duration := endTime.Sub(startTime)
	fmt.Printf("DEBUG: Full AI response: %q\n", fullResponse.String())
	fmt.Printf("DEBUG: processStreamFree completed at %s, duration: %v\n", endTime.Format(time.RFC3339), duration)

	// Send completion event
	l.sendEvent(ctx, eventCh, StreamEvent{
		Type: EventTypeComplete,
		Data: map[string]interface{}{
			"message":      "Free chat stream completed successfully",
			"start_time":   startTime.Format(time.RFC3339),
			"end_time":     endTime.Format(time.RFC3339),
			"duration_ms":  duration.Milliseconds(),
			"total_chunks": chunkCount,
		},
	}, 3)
	return nil
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
