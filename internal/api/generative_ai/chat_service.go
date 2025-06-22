package generativeAI

import (
	"context"
	"flag"
	"fmt"
	"iter"
	"log"
	"log/slog"
	"os"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/genai"

	"github.com/FACorreiaa/go-poi-au-suggestions/internal/types"
)

var model = flag.String("model", "gemini-2.0-flash", "the model name, e.g. gemini-2.0-flash")

type AIClient struct {
	client *genai.Client
	model  string
}

type ChatSession struct {
	chat *genai.Chat
}

type RAGService struct {
	aiClient         *AIClient
	embeddingService *EmbeddingService
	logger           *slog.Logger
}

type RAGContext struct {
	Query               string                  `json:"query"`
	RelevantPOIs        []types.POIDetailedInfo `json:"relevant_pois"`
	UserPreferences     map[string]interface{}  `json:"user_preferences,omitempty"`
	CityContext         string                  `json:"city_context,omitempty"`
	ConversationHistory []ConversationTurn      `json:"conversation_history,omitempty"`
}

type ConversationTurn struct {
	Role      string                 `json:"role"` // "user" or "assistant"
	Message   string                 `json:"message"`
	Timestamp time.Time              `json:"timestamp"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

type RAGResponse struct {
	Answer      string                  `json:"answer"`
	SourcePOIs  []types.POIDetailedInfo `json:"source_pois"`
	Confidence  float64                 `json:"confidence"`
	Suggestions []string                `json:"suggestions,omitempty"`
}

func NewAIClient(ctx context.Context) (*AIClient, error) {
	ctx, span := otel.Tracer("GenerativeAI").Start(ctx, "NewAIClient")
	defer span.End()

	apiKey := os.Getenv("GOOGLE_GEMINI_API_KEY")
	if apiKey == "" {
		err := fmt.Errorf("GOOGLE_GEMINI_API_KEY environment variable is not set")
		span.RecordError(err)
		span.SetStatus(codes.Error, "API key not set")
		log.Fatal(err)
	}

	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  apiKey,
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to create Gemini client")
		return nil, err
	}

	span.SetStatus(codes.Ok, "AI client created successfully")
	return &AIClient{
		client: client,
		model:  *model,
	}, nil
}

// func (ai *AIClient) SetupFunctionCalling() {
// 	poiFunc := &genai.FunctionDeclaration{
// 		Name:        "findPOIs",
// 		Description: "Find points of interest in a city within a radius.",
// 		Parameters: &genai.Schema{
// 			Type: genai.TypeObject,
// 			Properties: map[string]*genai.Schema{
// 				"city":   {Type: genai.TypeString, Description: "City name"},
// 				"radius": {Type: genai.TypeNumber, Description: "Radius in km"},
// 			},
// 			Required: []string{"city", "radius"},
// 		},
// 	}
// 	ai.model.Tools = []*genai.Tool{{FunctionDeclarations: []*genai.FunctionDeclaration{poiFunc}}}
// }

func (ai *AIClient) GenerateContent(ctx context.Context, prompt string, config *genai.GenerateContentConfig) (string, error) {
	ctx, span := otel.Tracer("GenerativeAI").Start(ctx, "GenerateContent", trace.WithAttributes(
		attribute.String("prompt.length", fmt.Sprintf("%d", len(prompt))),
		attribute.String("model", *model),
	))
	defer span.End()

	newClient, err := NewAIClient(ctx)

	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to create client")
		return "", fmt.Errorf("failed to create client: %w", err)
	}

	result, err := newClient.client.Models.GenerateContent(ctx, *model, genai.Text(prompt), config)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to generate content")
		log.Fatal(err)
	}

	responseText := result.Text()
	span.SetAttributes(attribute.Int("response.length", len(responseText)))
	span.SetStatus(codes.Ok, "Content generated successfully")
	return responseText, nil
}

func (ai *AIClient) GenerateResponse(ctx context.Context, prompt string, config *genai.GenerateContentConfig) (*genai.GenerateContentResponse, error) {
	ctx, span := otel.Tracer("GenerativeAI").Start(ctx, "GenerateResponse", trace.WithAttributes(
		attribute.String("prompt.length", fmt.Sprintf("%d", len(prompt))),
		attribute.String("model", ai.model),
	))
	defer span.End()

	chat, err := ai.client.Chats.Create(ctx, ai.model, config, nil)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to create chat")
		return nil, fmt.Errorf("failed to create chat: %w", err)
	}

	response, err := chat.SendMessage(ctx, genai.Part{Text: prompt})
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to send message")
		return nil, err
	}

	span.SetStatus(codes.Ok, "Response generated successfully")
	return response, nil
}

func (ai *AIClient) StartChatSession(ctx context.Context, config *genai.GenerateContentConfig) (*ChatSession, error) {
	ctx, span := otel.Tracer("GenerativeAI").Start(ctx, "StartChatSession", trace.WithAttributes(
		attribute.String("model", ai.model),
	))
	defer span.End()

	//config = &genai.GenerateContentConfig{Temperature: genai.Ptr[float32](0.5)}
	chat, err := ai.client.Chats.Create(ctx, ai.model, config, nil)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to create chat session")
		return nil, err
	}

	span.SetStatus(codes.Ok, "Chat session created successfully")
	return &ChatSession{chat: chat}, nil
}

func (cs *ChatSession) SendMessage(ctx context.Context, message string) (string, error) {
	ctx, span := otel.Tracer("GenerativeAI").Start(ctx, "SendMessage", trace.WithAttributes(
		attribute.String("message.length", fmt.Sprintf("%d", len(message))),
	))
	defer span.End()

	result, err := cs.chat.SendMessage(ctx, genai.Part{Text: message})
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to send message")
		return "", err
	}

	responseText := result.Text()
	span.SetAttributes(attribute.Int("response.length", len(responseText)))
	span.SetStatus(codes.Ok, "Message sent successfully")
	return responseText, nil
}

//func GenerateEmbedding(ctx context.Context, client *genai.Client, text string) ([]float32, error) {
//	req := &genaipb.EmbedContentRequest{
//		Model: "models/embedding-001", // replace with actual embedding model name
//		Content: &genaipb.Content{
//			Parts: []*genaipb.Part{
//				{Value: &genaipb.Part_Text{Text: text}},
//			},
//		},
//	}
//	resp, err := client.EmbedContent(ctx, req)
//	if err != nil {
//		return nil, err
//	}
//	return resp.Embedding.Values, nil
//}

// GenerateContentStream initiates a streaming content generation process.
func (ai *AIClient) GenerateContentStream(
	ctx context.Context,
	prompt string,
	config *genai.GenerateContentConfig,
) (iter.Seq2[*genai.GenerateContentResponse, error], error) {
	return ai.GenerateContentStreamWithCache(ctx, prompt, config, "")
}

// GenerateContentStreamWithCache initiates a streaming content generation process with cache support
func (ai *AIClient) GenerateContentStreamWithCache(
	ctx context.Context,
	prompt string,
	config *genai.GenerateContentConfig,
	cacheKey string,
) (iter.Seq2[*genai.GenerateContentResponse, error], error) {
	ctx, span := otel.Tracer("GenerativeAI").Start(ctx, "GenerateContentStreamWithCache", trace.WithAttributes(
		attribute.String("prompt.length", fmt.Sprintf("%d", len(prompt))),
		attribute.String("model", ai.model),
		attribute.String("cache.key", cacheKey),
		attribute.Bool("cache.enabled", cacheKey != ""),
	))
	defer span.End()

	if ai.client == nil {
		err := fmt.Errorf("AIClient's internal genai.Client is not initialized")
		span.RecordError(err)
		span.SetStatus(codes.Error, "Client not initialized for stream")
		return nil, err
	}

	var foundCacheName string
	// Try to get cached content first if cache key is provided
	if cacheKey != "" {
		// Check for existing cache
		page, err := ai.client.Caches.List(ctx, &genai.ListCachedContentsConfig{})
		if err == nil {
			for _, cache := range page.Items {
				if cache.DisplayName == cacheKey {
					foundCacheName = cache.Name
					span.SetAttributes(
						attribute.Bool("cache.hit", true),
						attribute.String("cache.name", cache.Name),
					)
					break
				}
			}
		}
		if foundCacheName == "" {
			span.SetAttributes(attribute.Bool("cache.miss", true))
		}
	}

	// Create configuration potentially with cache reference
	configToUse := config
	if configToUse == nil {
		configToUse = &genai.GenerateContentConfig{}
	}

	// Create a chat session
	// Note: For now we create a regular chat session since the exact API for
	// using cached content in streaming is still being clarified
	chat, err := ai.client.Chats.Create(ctx, ai.model, configToUse, nil)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to create chat for stream")
		return nil, fmt.Errorf("failed to create chat: %w", err)
	}

	// Create the prompt part
	part := genai.Part{Text: prompt}

	// If we found a cache, note it in logs but still proceed with regular streaming
	// This allows the infrastructure to be in place while we perfect the cache usage
	if foundCacheName != "" {
		span.SetAttributes(attribute.String("cache.used", foundCacheName))
	}

	span.SetStatus(codes.Ok, "Content stream initiated")
	return chat.SendMessageStream(ctx, part), nil
}

// Add SendMessageStream to ChatSession
func (cs *ChatSession) SendMessageStream(ctx context.Context, message string) iter.Seq2[*genai.GenerateContentResponse, error] {
	return cs.chat.SendMessageStream(ctx, genai.Part{Text: message})
}

// NewRAGService creates a new RAG service instance
func NewRAGService(ctx context.Context, logger *slog.Logger) (*RAGService, error) {
	ctx, span := otel.Tracer("RAGService").Start(ctx, "NewRAGService")
	defer span.End()

	aiClient, err := NewAIClient(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to create AI client")
		return nil, fmt.Errorf("failed to create AI client: %w", err)
	}

	embeddingService, err := NewEmbeddingService(ctx, logger)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to create embedding service")
		return nil, fmt.Errorf("failed to create embedding service: %w", err)
	}

	span.SetStatus(codes.Ok, "RAG service created successfully")
	return &RAGService{
		aiClient:         aiClient,
		embeddingService: embeddingService,
		logger:           logger,
	}, nil
}
