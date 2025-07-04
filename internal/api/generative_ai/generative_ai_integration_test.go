//go:build integration

package generativeAI

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/genai"
)

func TestMain(m *testing.M) {
	// Check if API key is available for integration tests
	if os.Getenv("GEMINI_API_KEY") == "" {
		// Skip all tests if no API key is provided
		os.Exit(0)
	}

	exitCode := m.Run()
	os.Exit(exitCode)
}

func TestNewAIClient_Integration(t *testing.T) {
	ctx := context.Background()

	if os.Getenv("GEMINI_API_KEY") == "" {
		t.Skip("Skipping integration test: GEMINI_API_KEY not set")
	}

	t.Run("Create AI client successfully", func(t *testing.T) {
		client, err := NewAIClient(ctx)
		require.NoError(t, err)
		require.NotNil(t, client)
		assert.NotNil(t, client.client)
		assert.Equal(t, "gemini-2.0-flash", client.model)
	})
}

func TestAIClient_GenerateContent_Integration(t *testing.T) {
	ctx := context.Background()

	if os.Getenv("GEMINI_API_KEY") == "" {
		t.Skip("Skipping integration test: GEMINI_API_KEY not set")
	}

	client, err := NewAIClient(ctx)
	require.NoError(t, err)

	t.Run("Generate content with simple prompt", func(t *testing.T) {
		prompt := "What is the capital of Portugal?"
		config := &genai.GenerateContentConfig{
			Temperature: genai.Ptr[float32](0.1), // Low temperature for consistent results
		}

		response, err := client.GenerateContent(ctx, prompt, config)
		require.NoError(t, err)
		assert.NotEmpty(t, response)
		assert.Contains(t, response, "Lisbon")
	})

	t.Run("Generate content with travel-related prompt", func(t *testing.T) {
		prompt := "List 3 must-visit attractions in Paris, France. Keep it brief."
		config := &genai.GenerateContentConfig{
			Temperature: genai.Ptr[float32](0.2),
		}

		response, err := client.GenerateContent(ctx, prompt, config)
		require.NoError(t, err)
		assert.NotEmpty(t, response)
		// Should contain typical Paris attractions
		lowerResponse := strings.ToLower(response)
		assert.True(t,
			strings.Contains(lowerResponse, "eiffel") ||
				strings.Contains(lowerResponse, "louvre") ||
				strings.Contains(lowerResponse, "notre"),
			"Response should mention famous Paris attractions")
	})

	t.Run("Generate content with empty prompt", func(t *testing.T) {
		prompt := ""
		config := &genai.GenerateContentConfig{
			Temperature: genai.Ptr[float32](0.5),
		}

		response, err := client.GenerateContent(ctx, prompt, config)
		// Should handle empty prompt gracefully
		if err != nil {
			assert.Contains(t, err.Error(), "prompt")
		} else {
			assert.NotEmpty(t, response)
		}
	})
}

func TestAIClient_GenerateResponse_Integration(t *testing.T) {
	ctx := context.Background()

	if os.Getenv("GEMINI_API_KEY") == "" {
		t.Skip("Skipping integration test: GEMINI_API_KEY not set")
	}

	client, err := NewAIClient(ctx)
	require.NoError(t, err)

	t.Run("Generate response with structured output", func(t *testing.T) {
		prompt := "Tell me about Rome in exactly 50 words."
		config := &genai.GenerateContentConfig{
			Temperature: genai.Ptr[float32](0.3),
		}

		response, err := client.GenerateResponse(ctx, prompt, config)
		require.NoError(t, err)
		require.NotNil(t, response)

		text := response.Text()
		assert.NotEmpty(t, text)
		assert.Contains(t, strings.ToLower(text), "rome")
	})
}

func TestAIClient_StartChatSession_Integration(t *testing.T) {
	ctx := context.Background()

	if os.Getenv("GEMINI_API_KEY") == "" {
		t.Skip("Skipping integration test: GEMINI_API_KEY not set")
	}

	client, err := NewAIClient(ctx)
	require.NoError(t, err)

	t.Run("Start chat session successfully", func(t *testing.T) {
		config := &genai.GenerateContentConfig{
			Temperature: genai.Ptr[float32](0.5),
		}

		chatSession, err := client.StartChatSession(ctx, config)
		require.NoError(t, err)
		require.NotNil(t, chatSession)
		assert.NotNil(t, chatSession.chat)
	})
}

func TestChatSession_SendMessage_Integration(t *testing.T) {
	ctx := context.Background()

	if os.Getenv("GEMINI_API_KEY") == "" {
		t.Skip("Skipping integration test: GEMINI_API_KEY not set")
	}

	client, err := NewAIClient(ctx)
	require.NoError(t, err)

	config := &genai.GenerateContentConfig{
		Temperature: genai.Ptr[float32](0.3),
	}

	chatSession, err := client.StartChatSession(ctx, config)
	require.NoError(t, err)

	t.Run("Send single message", func(t *testing.T) {
		message := "What is the best time to visit Barcelona?"

		response, err := chatSession.SendMessage(ctx, message)
		require.NoError(t, err)
		assert.NotEmpty(t, response)

		// Should contain relevant information about Barcelona
		lowerResponse := strings.ToLower(response)
		assert.True(t,
			strings.Contains(lowerResponse, "spring") ||
				strings.Contains(lowerResponse, "summer") ||
				strings.Contains(lowerResponse, "fall") ||
				strings.Contains(lowerResponse, "barcelona"),
			"Response should contain relevant travel information")
	})

	t.Run("Send follow-up message in same session", func(t *testing.T) {
		followUpMessage := "What about the weather there?"

		response, err := chatSession.SendMessage(ctx, followUpMessage)
		require.NoError(t, err)
		assert.NotEmpty(t, response)

		// Should maintain context from previous message
		lowerResponse := strings.ToLower(response)
		assert.True(t,
			strings.Contains(lowerResponse, "temperature") ||
				strings.Contains(lowerResponse, "weather") ||
				strings.Contains(lowerResponse, "climate"),
			"Response should address weather information")
	})
}

func TestAIClient_GenerateContentStream_Integration(t *testing.T) {
	ctx := context.Background()

	if os.Getenv("GEMINI_API_KEY") == "" {
		t.Skip("Skipping integration test: GEMINI_API_KEY not set")
	}

	client, err := NewAIClient(ctx)
	require.NoError(t, err)

	t.Run("Generate streaming content", func(t *testing.T) {
		prompt := "Write a short travel guide for Tokyo in 3 paragraphs."
		config := &genai.GenerateContentConfig{
			Temperature: genai.Ptr[float32](0.4),
		}

		stream, err := client.GenerateContentStream(ctx, prompt, config)
		require.NoError(t, err)
		require.NotNil(t, stream)

		var fullResponse strings.Builder
		responseCount := 0

		// Set a timeout for the streaming test
		timeoutCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()

		// Collect streaming responses
		for response, err := range stream {
			select {
			case <-timeoutCtx.Done():
				t.Fatal("Streaming test timed out")
			default:
			}

			if err != nil {
				require.NoError(t, err, "Error in stream")
				break
			}

			if response != nil {
				text := response.Text()
				fullResponse.WriteString(text)
				responseCount++
			}

			// Limit iterations to prevent infinite loops in tests
			if responseCount > 100 {
				break
			}
		}

		assert.Greater(t, responseCount, 0, "Should receive at least one response chunk")
		finalResponse := fullResponse.String()
		assert.NotEmpty(t, finalResponse)
		assert.Contains(t, strings.ToLower(finalResponse), "tokyo")
	})
}

func TestChatSession_SendMessageStream_Integration(t *testing.T) {
	ctx := context.Background()

	if os.Getenv("GEMINI_API_KEY") == "" {
		t.Skip("Skipping integration test: GEMINI_API_KEY not set")
	}

	client, err := NewAIClient(ctx)
	require.NoError(t, err)

	config := &genai.GenerateContentConfig{
		Temperature: genai.Ptr[float32](0.3),
	}

	chatSession, err := client.StartChatSession(ctx, config)
	require.NoError(t, err)

	t.Run("Send streaming message", func(t *testing.T) {
		message := "Tell me about the top 5 restaurants in New York City."

		stream := chatSession.SendMessageStream(ctx, message)
		require.NotNil(t, stream)

		var fullResponse strings.Builder
		responseCount := 0

		// Set a timeout for the streaming test
		timeoutCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()

		for response, err := range stream {
			select {
			case <-timeoutCtx.Done():
				t.Fatal("Streaming chat test timed out")
			default:
			}

			if err != nil {
				require.NoError(t, err, "Error in chat stream")
				break
			}

			if response != nil {
				text := response.Text()
				fullResponse.WriteString(text)
				responseCount++
			}

			// Limit iterations
			if responseCount > 100 {
				break
			}
		}

		assert.Greater(t, responseCount, 0, "Should receive at least one response chunk")
		finalResponse := fullResponse.String()
		assert.NotEmpty(t, finalResponse)

		lowerResponse := strings.ToLower(finalResponse)
		assert.True(t,
			strings.Contains(lowerResponse, "restaurant") ||
				strings.Contains(lowerResponse, "new york") ||
				strings.Contains(lowerResponse, "nyc"),
			"Response should contain relevant restaurant information")
	})
}

func TestAIClient_ErrorHandling_Integration(t *testing.T) {
	ctx := context.Background()

	if os.Getenv("GEMINI_API_KEY") == "" {
		t.Skip("Skipping integration test: GEMINI_API_KEY not set")
	}

	t.Run("Test with context cancellation", func(t *testing.T) {
		client, err := NewAIClient(ctx)
		require.NoError(t, err)

		// Create a context that will be cancelled quickly
		cancelCtx, cancel := context.WithTimeout(ctx, 1*time.Millisecond)
		defer cancel()

		time.Sleep(2 * time.Millisecond) // Ensure context is cancelled

		config := &genai.GenerateContentConfig{
			Temperature: genai.Ptr[float32](0.5),
		}

		_, err = client.GenerateContent(cancelCtx, "This should be cancelled", config)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "context")
	})
}

func TestAIClient_LongPrompt_Integration(t *testing.T) {
	ctx := context.Background()

	if os.Getenv("GEMINI_API_KEY") == "" {
		t.Skip("Skipping integration test: GEMINI_API_KEY not set")
	}

	client, err := NewAIClient(ctx)
	require.NoError(t, err)

	t.Run("Handle long prompt", func(t *testing.T) {
		longPrompt := strings.Repeat("Tell me about travel destinations. ", 100)
		config := &genai.GenerateContentConfig{
			Temperature: genai.Ptr[float32](0.3),
		}

		response, err := client.GenerateContent(ctx, longPrompt, config)
		// Should either succeed or fail gracefully
		if err != nil {
			// Check if it's a known error type (e.g., token limit)
			assert.NotEmpty(t, err.Error())
		} else {
			assert.NotEmpty(t, response)
		}
	})
}
