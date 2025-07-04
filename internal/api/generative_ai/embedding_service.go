package generativeAI

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/genai"
)

const (
	// Gemini embedding model - using the latest embedding model
	//EmbeddingModel = "text-embedding-004"
	EmbeddingModel = "gemini-embedding-exp-03-07"
	// Standard embedding dimension for Gemini text-embedding-004
	EmbeddingDimension = 768
)

type EmbeddingService struct {
	client *genai.Client
	logger *slog.Logger
}

type EmbeddingRequest struct {
	Text string `json:"text"`
	Type string `json:"type,omitempty"` // "poi", "city", "user_preference", etc.
}

type EmbeddingResponse struct {
	Embedding []float32 `json:"embedding"`
	Dimension int       `json:"dimension"`
}

func NewEmbeddingService(ctx context.Context, logger *slog.Logger) (*EmbeddingService, error) {
	ctx, span := otel.Tracer("EmbeddingService").Start(ctx, "NewEmbeddingService")
	defer span.End()

	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		err := fmt.Errorf("GEMINI_API_KEY environment variable is not set")
		span.RecordError(err)
		span.SetStatus(codes.Error, "API key not set")
		return nil, err
	}

	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  apiKey,
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to create Gemini client")
		return nil, fmt.Errorf("failed to create Gemini client: %w", err)
	}

	span.SetStatus(codes.Ok, "Embedding service created successfully")
	return &EmbeddingService{
		client: client,
		logger: logger,
	}, nil
}

// GenerateEmbedding generates an embedding vector for the given text
func (es *EmbeddingService) GenerateEmbedding(ctx context.Context, text string, config *genai.EmbedContentConfig) ([]float32, error) {
	ctx, span := otel.Tracer("EmbeddingService").Start(ctx, "GenerateEmbedding", trace.WithAttributes(
		attribute.String("text.length", fmt.Sprintf("%d", len(text))),
		attribute.String("model", EmbeddingModel),
	))
	defer span.End()

	if text == "" {
		err := fmt.Errorf("text cannot be empty")
		span.RecordError(err)
		span.SetStatus(codes.Error, "Empty text provided")
		return nil, err
	}

	// Use the embedding model to generate embeddings
	embedding, err := es.client.Models.EmbedContent(ctx, EmbeddingModel, genai.Text(text), config)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to generate embedding")
		es.logger.ErrorContext(ctx, "Failed to generate embedding",
			slog.Any("error", err),
			slog.String("text_preview", text[:min(100, len(text))]))
		return nil, fmt.Errorf("failed to generate embedding: %w", err)
	}

	// Extract the embedding values
	if embedding == nil || len(embedding.Embeddings) == 0 {
		err := fmt.Errorf("received empty embedding from API")
		span.RecordError(err)
		span.SetStatus(codes.Error, "Empty embedding received")
		return nil, err
	}

	// Get the first embedding (assuming single text input)
	contentEmbedding := embedding.Embeddings[0]
	if contentEmbedding == nil || len(contentEmbedding.Values) == 0 {
		err := fmt.Errorf("received empty embedding values from API")
		span.RecordError(err)
		span.SetStatus(codes.Error, "Empty embedding values received")
		return nil, err
	}

	span.SetAttributes(
		attribute.Int("embedding.dimension", len(contentEmbedding.Values)),
		attribute.String("embedding.model", EmbeddingModel),
	)
	span.SetStatus(codes.Ok, "Embedding generated successfully")

	es.logger.DebugContext(ctx, "Embedding generated",
		slog.Int("dimension", len(contentEmbedding.Values)),
		slog.String("model", EmbeddingModel))

	return contentEmbedding.Values, nil
}

// GeneratePOIEmbedding generates an embedding specifically for POI data
func (es *EmbeddingService) GeneratePOIEmbedding(ctx context.Context, name, description, category string) ([]float32, error) {

	ctx, span := otel.Tracer("EmbeddingService").Start(ctx, "GeneratePOIEmbedding", trace.WithAttributes(
		attribute.String("poi.name", name),
		attribute.String("poi.category", category),
	))
	defer span.End()

	// Create a comprehensive text representation of the POI
	var text string
	if description != "" {
		text = fmt.Sprintf("Name: %s\nCategory: %s\nDescription: %s", name, category, description)
	} else {
		text = fmt.Sprintf("Name: %s\nCategory: %s", name, category)
	}

	embedding, err := es.GenerateEmbedding(ctx, text, nil)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("failed to generate POI embedding: %w", err)
	}

	span.SetStatus(codes.Ok, "POI embedding generated successfully")
	return embedding, nil
}

// GenerateCityEmbedding generates an embedding specifically for city data
func (es *EmbeddingService) GenerateCityEmbedding(ctx context.Context, name, country, description string) ([]float32, error) {
	ctx, span := otel.Tracer("EmbeddingService").Start(ctx, "GenerateCityEmbedding", trace.WithAttributes(
		attribute.String("city.name", name),
		attribute.String("city.country", country),
	))
	defer span.End()

	// Create a comprehensive text representation of the city
	text := fmt.Sprintf("City: %s, Country: %s", name, country)
	if description != "" {
		text += fmt.Sprintf("\nDescription: %s", description)
	}

	embedding, err := es.GenerateEmbedding(ctx, text, nil)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("failed to generate city embedding: %w", err)
	}

	span.SetStatus(codes.Ok, "City embedding generated successfully")
	return embedding, nil
}

// GenerateUserPreferenceEmbedding generates an embedding for user preferences
func (es *EmbeddingService) GenerateUserPreferenceEmbedding(ctx context.Context, interests []string, preferences map[string]string) ([]float32, error) {
	ctx, span := otel.Tracer("EmbeddingService").Start(ctx, "GenerateUserPreferenceEmbedding", trace.WithAttributes(
		attribute.Int("interests.count", len(interests)),
		attribute.Int("preferences.count", len(preferences)),
	))
	defer span.End()

	// Create a text representation of user preferences
	text := "User Interests: "
	for i, interest := range interests {
		if i > 0 {
			text += ", "
		}
		text += interest
	}

	if len(preferences) > 0 {
		text += "\nPreferences: "
		for key, value := range preferences {
			text += fmt.Sprintf("%s: %s; ", key, value)
		}
	}

	embedding, err := es.GenerateEmbedding(ctx, text, nil)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("failed to generate user preference embedding: %w", err)
	}

	span.SetStatus(codes.Ok, "User preference embedding generated successfully")
	return embedding, nil
}

// GenerateQueryEmbedding generates an embedding for search queries
func (es *EmbeddingService) GenerateQueryEmbedding(ctx context.Context, query string) ([]float32, error) {
	ctx, span := otel.Tracer("EmbeddingService").Start(ctx, "GenerateQueryEmbedding", trace.WithAttributes(
		attribute.String("query", query),
	))
	defer span.End()

	embedding, err := es.GenerateEmbedding(ctx, query, nil)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("failed to generate query embedding: %w", err)
	}

	span.SetAttributes(attribute.String("query.text", query))
	span.SetStatus(codes.Ok, "Query embedding generated successfully")
	return embedding, nil
}

// BatchGenerateEmbeddings generates embeddings for multiple texts at once
func (es *EmbeddingService) BatchGenerateEmbeddings(ctx context.Context, texts []string) ([][]float32, error) {
	ctx, span := otel.Tracer("EmbeddingService").Start(ctx, "BatchGenerateEmbeddings", trace.WithAttributes(
		attribute.Int("batch.size", len(texts)),
	))
	defer span.End()

	if len(texts) == 0 {
		err := fmt.Errorf("no texts provided for batch embedding")
		span.RecordError(err)
		span.SetStatus(codes.Error, "Empty batch")
		return nil, err
	}

	embeddings := make([][]float32, len(texts))
	var err error

	// Generate embeddings sequentially
	// TODO: Implement concurrent processing with rate limiting if needed
	for i, text := range texts {
		embeddings[i], err = es.GenerateEmbedding(ctx, text, nil)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, fmt.Sprintf("Failed at index %d", i))
			return nil, fmt.Errorf("failed to generate embedding for text at index %d: %w", i, err)
		}
	}

	span.SetAttributes(
		attribute.Int("successful.embeddings", len(embeddings)),
		attribute.String("model", EmbeddingModel),
	)
	span.SetStatus(codes.Ok, "Batch embeddings generated successfully")

	es.logger.InfoContext(ctx, "Batch embeddings generated",
		slog.Int("count", len(embeddings)),
		slog.String("model", EmbeddingModel))

	return embeddings, nil
}

// Close closes the embedding service and cleans up resources
func (es *EmbeddingService) Close() error {
	if es.client != nil {
		return es.Close()
	}
	return nil
}
