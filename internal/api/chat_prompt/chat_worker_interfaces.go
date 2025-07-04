package llmChat

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/FACorreiaa/go-poi-au-suggestions/internal/types"
	generativeAI "github.com/FACorreiaa/go-poi-au-suggestions/internal/api/generative_ai"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/genai"
)

// BaseAIWorker provides common functionality for AI workers
type BaseAIWorker struct {
	logger      *slog.Logger
	aiClient    *generativeAI.AIClient
	emitter     EventEmitter
	resultCh    chan<- types.GenAIResponse
	eventCh     chan<- types.StreamEvent
	ctx         context.Context
	wg          *sync.WaitGroup
}

// NewBaseAIWorker creates a new base AI worker
func NewBaseAIWorker(
	logger *slog.Logger,
	aiClient *generativeAI.AIClient,
	emitter EventEmitter,
	ctx context.Context,
	resultCh chan<- types.GenAIResponse,
	eventCh chan<- types.StreamEvent,
	wg *sync.WaitGroup,
) *BaseAIWorker {
	return &BaseAIWorker{
		logger:   logger,
		aiClient: aiClient,
		emitter:  emitter,
		resultCh: resultCh,
		eventCh:  eventCh,
		ctx:      ctx,
		wg:       wg,
	}
}

// ExecuteWithSpan runs the worker with OpenTelemetry tracing
func (w *BaseAIWorker) ExecuteWithSpan(spanName string, worker AIWorker) {
	if w.wg != nil {
		defer w.wg.Done()
	}

	traceAttrs := worker.GetTraceAttributes()
	var attrs []trace.SpanStartOption
	for key, value := range traceAttrs {
		switch v := value.(type) {
		case string:
			attrs = append(attrs, trace.WithAttributes(attribute.String(key, v)))
		case int:
			attrs = append(attrs, trace.WithAttributes(attribute.Int(key, v)))
		case float64:
			attrs = append(attrs, trace.WithAttributes(attribute.Float64(key, v)))
		}
	}

	ctx, span := otel.Tracer("AIWorker").Start(w.ctx, spanName, attrs...)
	defer span.End()

	if err := worker.Execute(ctx, w.resultCh, w.eventCh); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Worker execution failed")
		w.resultCh <- types.GenAIResponse{Err: err}
		return
	}

	span.SetStatus(codes.Ok, "Worker executed successfully")
}

// GenerateResponse generates AI response using the configured client
func (w *BaseAIWorker) GenerateResponse(ctx context.Context, prompt string, config *genai.GenerateContentConfig) (string, error) {
	response, err := w.aiClient.GenerateResponse(ctx, prompt, config)
	if err != nil {
		return "", fmt.Errorf("failed to generate AI response: %w", err)
	}

	var txt string
	for _, candidate := range response.Candidates {
		if candidate.Content != nil && len(candidate.Content.Parts) > 0 {
			txt = candidate.Content.Parts[0].Text
			break
		}
	}

	if txt == "" {
		return "", fmt.Errorf("empty response from AI")
	}

	return txt, nil
}

// ParseJSONResponse parses and validates JSON response
func (w *BaseAIWorker) ParseJSONResponse(response string, target interface{}) error {
	cleanTxt := cleanJSONResponse(response)
	if err := json.Unmarshal([]byte(cleanTxt), target); err != nil {
		return fmt.Errorf("failed to parse JSON response: %w", err)
	}
	return nil
}

// CityDataWorker generates city data
type CityDataWorker struct {
	*BaseAIWorker
	cityName string
	userID   uuid.UUID
	repo     Repository
}

// NewCityDataWorker creates a new city data worker
func NewCityDataWorker(
	base *BaseAIWorker,
	cityName string,
	userID uuid.UUID,
	repo Repository,
) *CityDataWorker {
	return &CityDataWorker{
		BaseAIWorker: base,
		cityName:     cityName,
		userID:       userID,
		repo:         repo,
	}
}

// Execute runs the city data generation
func (w *CityDataWorker) Execute(ctx context.Context, resultCh chan<- types.GenAIResponse, eventCh chan<- types.StreamEvent) error {
	// Emit progress
	w.emitter.EmitProgress(ctx, eventCh, "generating_city_data", 10)

	startTime := time.Now()
	prompt := getCityDescriptionPrompt(w.cityName)

	// Generate response
	responseText, err := w.GenerateResponse(ctx, prompt, &genai.GenerateContentConfig{
		Temperature: genai.Ptr[float32](defaultTemperature),
	})
	if err != nil {
		w.emitter.EmitError(ctx, eventCh, err)
		return err
	}

	// Parse response
	var cityData struct {
		CityName        string  `json:"city_name"`
		StateProvince   *string `json:"state_province,omitempty"`
		Country         string  `json:"country"`
		CenterLatitude  float64 `json:"center_latitude"`
		CenterLongitude float64 `json:"center_longitude"`
		Description     string  `json:"description"`
	}

	if err := w.ParseJSONResponse(responseText, &cityData); err != nil {
		w.emitter.EmitError(ctx, eventCh, err)
		return err
	}

	// Save interaction
	interaction := types.LlmInteraction{
		UserID:       w.userID,
		Prompt:       prompt,
		ResponseText: responseText,
		ModelUsed:    model,
		LatencyMs:    int(time.Since(startTime).Milliseconds()),
		CityName:     w.cityName,
	}

	if _, err := w.repo.SaveInteraction(ctx, interaction); err != nil {
		w.logger.ErrorContext(ctx, "Failed to save city data interaction", slog.Any("error", err))
		// Don't fail the entire operation for save errors
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

	// Emit result
	w.emitter.EmitData(ctx, eventCh, types.EventTypeCityData, result)
	resultCh <- result

	return nil
}

// GetPrompt returns the prompt for city data generation
func (w *CityDataWorker) GetPrompt() string {
	return getCityDescriptionPrompt(w.cityName)
}

// GetTraceAttributes returns trace attributes
func (w *CityDataWorker) GetTraceAttributes() map[string]interface{} {
	return map[string]interface{}{
		"city.name": w.cityName,
		"user.id":   w.userID.String(),
	}
}

// GeneralPOIWorker generates general POIs
type GeneralPOIWorker struct {
	*BaseAIWorker
	cityName string
	userID   uuid.UUID
	repo     Repository
}

// NewGeneralPOIWorker creates a new general POI worker
func NewGeneralPOIWorker(
	base *BaseAIWorker,
	cityName string,
	userID uuid.UUID,
	repo Repository,
) *GeneralPOIWorker {
	return &GeneralPOIWorker{
		BaseAIWorker: base,
		cityName:     cityName,
		userID:       userID,
		repo:         repo,
	}
}

// Execute runs the general POI generation
func (w *GeneralPOIWorker) Execute(ctx context.Context, resultCh chan<- types.GenAIResponse, eventCh chan<- types.StreamEvent) error {
	// Emit progress
	w.emitter.EmitProgress(ctx, eventCh, "generating_general_pois", 30)

	startTime := time.Now()
	prompt := getGeneralPOIPrompt(w.cityName)

	// Generate response
	responseText, err := w.GenerateResponse(ctx, prompt, &genai.GenerateContentConfig{
		Temperature: genai.Ptr[float32](defaultTemperature),
	})
	if err != nil {
		w.emitter.EmitError(ctx, eventCh, err)
		return err
	}

	// Parse response
	var poiData struct {
		PointsOfInterest []types.POIDetailedInfo `json:"points_of_interest"`
	}

	if err := w.ParseJSONResponse(responseText, &poiData); err != nil {
		w.emitter.EmitError(ctx, eventCh, err)
		return err
	}

	// Save interaction
	interaction := types.LlmInteraction{
		UserID:       w.userID,
		Prompt:       prompt,
		ResponseText: responseText,
		ModelUsed:    model,
		LatencyMs:    int(time.Since(startTime).Milliseconds()),
		CityName:     w.cityName,
	}

	if _, err := w.repo.SaveInteraction(ctx, interaction); err != nil {
		w.logger.ErrorContext(ctx, "Failed to save general POI interaction", slog.Any("error", err))
		// Don't fail the entire operation for save errors
	}

	result := types.GenAIResponse{
		GeneralPOI: poiData.PointsOfInterest,
	}

	// Emit result
	w.emitter.EmitData(ctx, eventCh, types.EventTypeGeneralPOI, result.GeneralPOI)
	resultCh <- result

	return nil
}

// GetPrompt returns the prompt for general POI generation
func (w *GeneralPOIWorker) GetPrompt() string {
	return getGeneralPOIPrompt(w.cityName)
}

// GetTraceAttributes returns trace attributes
func (w *GeneralPOIWorker) GetTraceAttributes() map[string]interface{} {
	return map[string]interface{}{
		"city.name": w.cityName,
		"user.id":   w.userID.String(),
	}
}