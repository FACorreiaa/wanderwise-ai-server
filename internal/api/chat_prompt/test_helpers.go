package llmchat

import (
	"context"
	"iter"

	generativeAI "github.com/FACorreiaa/go-genai-sdk/lib"
	"google.golang.org/genai"
)

// TestLLMClient wraps the mock to satisfy the concrete type requirement
type TestLLMClient struct {
	GenerateContentStreamWithCacheFn func(ctx context.Context, prompt string, config *genai.GenerateContentConfig, cacheKey string) (iter.Seq2[*genai.GenerateContentResponse, error], error)
	GenerateContentStreamFn          func(ctx context.Context, prompt string, config *genai.GenerateContentConfig) (iter.Seq2[*genai.GenerateContentResponse, error], error)
	GenerateResponseFn               func(ctx context.Context, prompt string, config *genai.GenerateContentConfig) (*genai.GenerateContentResponse, error)
	GenerateContentFn                func(ctx context.Context, prompt, apiKey string, config *genai.GenerateContentConfig) (string, error)
	StartChatSessionFn               func(ctx context.Context, config *genai.GenerateContentConfig) (*generativeAI.ChatSession, error)
}

func (t *TestLLMClient) GenerateContentStreamWithCache(
	ctx context.Context,
	prompt string,
	config *genai.GenerateContentConfig,
	cacheKey string,
) (iter.Seq2[*genai.GenerateContentResponse, error], error) {
	if t.GenerateContentStreamWithCacheFn != nil {
		return t.GenerateContentStreamWithCacheFn(ctx, prompt, config, cacheKey)
	}
	return nil, nil
}

func (t *TestLLMClient) GenerateContentStream(
	ctx context.Context,
	prompt string,
	config *genai.GenerateContentConfig,
) (iter.Seq2[*genai.GenerateContentResponse, error], error) {
	if t.GenerateContentStreamFn != nil {
		return t.GenerateContentStreamFn(ctx, prompt, config)
	}
	return nil, nil
}

func (t *TestLLMClient) GenerateResponse(ctx context.Context, prompt string, config *genai.GenerateContentConfig) (*genai.GenerateContentResponse, error) {
	if t.GenerateResponseFn != nil {
		return t.GenerateResponseFn(ctx, prompt, config)
	}
	return nil, nil
}

func (t *TestLLMClient) GenerateContent(ctx context.Context, prompt, apiKey string, config *genai.GenerateContentConfig) (string, error) {
	if t.GenerateContentFn != nil {
		return t.GenerateContentFn(ctx, prompt, apiKey, config)
	}
	return "", nil
}

func (t *TestLLMClient) StartChatSession(ctx context.Context, config *genai.GenerateContentConfig) (*generativeAI.ChatSession, error) {
	if t.StartChatSessionFn != nil {
		return t.StartChatSessionFn(ctx, config)
	}
	return nil, nil
}

// Helper function to create a mock iterator with test data
func createMockIterator(responses []string) iter.Seq2[*genai.GenerateContentResponse, error] {
	return func(yield func(*genai.GenerateContentResponse, error) bool) {
		for _, resp := range responses {
			response := &genai.GenerateContentResponse{
				Candidates: []*genai.Candidate{
					{
						Content: &genai.Content{
							Parts: []*genai.Part{
								{Text: resp},
							},
						},
					},
				},
			}
			if !yield(response, nil) {
				return
			}
		}
	}
}
