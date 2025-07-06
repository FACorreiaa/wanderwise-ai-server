package llmChat

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/FACorreiaa/go-poi-au-suggestions/internal/types"
)

// BenchmarkConfig holds configuration for the benchmark tests
type BenchmarkConfig struct {
	BaseURL          string
	AuthToken        string
	UserID           uuid.UUID
	ProfileID        uuid.UUID
	TestUserLocation *types.UserLocation
	Timeout          time.Duration
}

// BenchmarkResults holds the results of benchmark tests
type BenchmarkResults struct {
	StartChatResults    []ChatBenchmarkResult
	ContinueChatResults []ChatBenchmarkResult
	TotalDuration       time.Duration
	TotalRequests       int
	SuccessfulRequests  int
	FailedRequests      int
}

// ChatBenchmarkResult represents the result of a single chat benchmark
type ChatBenchmarkResult struct {
	TestName        string
	Message         string
	Duration        time.Duration
	EventsReceived  int
	FirstEventTime  time.Duration
	LastEventTime   time.Duration
	Success         bool
	Error           string
	SessionID       uuid.UUID
	ResponseSize    int64
	EventTypes      map[string]int
}

// SSEEvent represents a Server-Sent Event
type SSEEvent struct {
	Type      string      `json:"type"`
	Message   string      `json:"message"`
	Data      interface{} `json:"data,omitempty"`
	Error     string      `json:"error,omitempty"`
	Timestamp time.Time   `json:"timestamp"`
	EventID   string      `json:"event_id"`
	IsFinal   bool        `json:"is_final,omitempty"`
}

// NewBenchmarkConfig creates a new benchmark configuration
func NewBenchmarkConfig() *BenchmarkConfig {
	return &BenchmarkConfig{
		BaseURL:   "http://localhost:8080/api/v1",
		UserID:    uuid.New(),
		ProfileID: uuid.New(),
		TestUserLocation: &types.UserLocation{
			UserLat:  41.4901, // Esposende coordinates
			UserLon: -8.7853,
		},
		Timeout: 60 * time.Second,
	}
}

// BenchmarkChatStreamingRoutes runs comprehensive benchmarks for both StartChat and ContinueChat routes
func BenchmarkChatStreamingRoutes(b *testing.B) {
	config := NewBenchmarkConfig()
	
	// Generate a test auth token (in real scenarios, this would come from login)
	config.AuthToken = generateTestAuthToken(config.UserID)
	
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		results := runFullChatBenchmark(config)
		
		// Log results for this iteration
		b.Logf("Iteration %d: Total Duration: %v, Success Rate: %.2f%%", 
			i+1, 
			results.TotalDuration, 
			float64(results.SuccessfulRequests)/float64(results.TotalRequests)*100)
	}
}

// BenchmarkStartChatMessageStream benchmarks only the StartChat route
func BenchmarkStartChatMessageStream(b *testing.B) {
	config := NewBenchmarkConfig()
	config.AuthToken = generateTestAuthToken(config.UserID)
	
	testMessages := []string{
		"Plan Esposende",
		"Restaurant in Povoa de Varzim",
	}
	
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		for _, message := range testMessages {
			result := benchmarkStartChat(config, message)
			if !result.Success {
				b.Errorf("StartChat failed for message '%s': %s", message, result.Error)
			}
		}
	}
}

// BenchmarkContinueChatSessionStream benchmarks only the ContinueChat route
func BenchmarkContinueChatSessionStream(b *testing.B) {
	config := NewBenchmarkConfig()
	config.AuthToken = generateTestAuthToken(config.UserID)
	
	// First, start a chat session to get a session ID
	initialResult := benchmarkStartChat(config, "Plan Esposende")
	if !initialResult.Success {
		b.Fatalf("Failed to start initial chat session: %s", initialResult.Error)
	}
	
	sessionID := initialResult.SessionID
	testMessages := []string{
		"Add Stadium",
		"Remove Stadium", 
		"Add Ibis Hotel",
		"Remove Ibis Hotel",
	}
	
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		for _, message := range testMessages {
			result := benchmarkContinueChat(config, sessionID, message)
			if !result.Success {
				b.Errorf("ContinueChat failed for message '%s': %s", message, result.Error)
			}
		}
	}
}

// runFullChatBenchmark runs a complete chat flow benchmark
func runFullChatBenchmark(config *BenchmarkConfig) BenchmarkResults {
	startTime := time.Now()
	results := BenchmarkResults{
		StartChatResults:    make([]ChatBenchmarkResult, 0),
		ContinueChatResults: make([]ChatBenchmarkResult, 0),
	}
	
	// Test StartChat messages
	startChatMessages := []string{
		"Plan Esposende",
		"Restaurant in Povoa de Varzim",
	}
	
	var sessionID uuid.UUID
	
	// Run StartChat benchmarks
	for _, message := range startChatMessages {
		result := benchmarkStartChat(config, message)
		results.StartChatResults = append(results.StartChatResults, result)
		results.TotalRequests++
		
		if result.Success {
			results.SuccessfulRequests++
			if sessionID == uuid.Nil {
				sessionID = result.SessionID
			}
		} else {
			results.FailedRequests++
		}
	}
	
	// Test ContinueChat messages (only if we have a valid session)
	if sessionID != uuid.Nil {
		continueChatMessages := []string{
			"Add Stadium",
			"Remove Stadium",
			"Add Ibis Hotel", 
			"Remove Ibis Hotel",
		}
		
		for _, message := range continueChatMessages {
			result := benchmarkContinueChat(config, sessionID, message)
			results.ContinueChatResults = append(results.ContinueChatResults, result)
			results.TotalRequests++
			
			if result.Success {
				results.SuccessfulRequests++
			} else {
				results.FailedRequests++
			}
		}
	}
	
	results.TotalDuration = time.Since(startTime)
	return results
}

// benchmarkStartChat benchmarks a single StartChat request
func benchmarkStartChat(config *BenchmarkConfig, message string) ChatBenchmarkResult {
	startTime := time.Now()
	result := ChatBenchmarkResult{
		TestName:   "StartChatMessageStream",
		Message:    message,
		EventTypes: make(map[string]int),
	}
	
	// Prepare request body
	requestBody := map[string]interface{}{
		"message": message,
		"user_location": config.TestUserLocation,
	}
	
	bodyBytes, err := json.Marshal(requestBody)
	if err != nil {
		result.Error = fmt.Sprintf("Failed to marshal request: %v", err)
		result.Duration = time.Since(startTime)
		return result
	}
	
	// Create HTTP request
	url := fmt.Sprintf("%s/llm/prompt-response/chat/sessions/stream/%s", config.BaseURL, config.ProfileID)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(bodyBytes))
	if err != nil {
		result.Error = fmt.Sprintf("Failed to create request: %v", err)
		result.Duration = time.Since(startTime)
		return result
	}
	
	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+config.AuthToken)
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Cache-Control", "no-cache")
	
	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: config.Timeout,
	}
	
	// Execute request
	resp, err := client.Do(req)
	if err != nil {
		result.Error = fmt.Sprintf("Failed to execute request: %v", err)
		result.Duration = time.Since(startTime)
		return result
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		result.Error = fmt.Sprintf("HTTP error: %d", resp.StatusCode)
		result.Duration = time.Since(startTime)
		return result
	}
	
	// Process SSE stream
	scanner := bufio.NewScanner(resp.Body)
	var firstEventTime, lastEventTime time.Time
	var responseSize int64
	
	for scanner.Scan() {
		line := scanner.Text()
		responseSize += int64(len(line))
		
		if strings.HasPrefix(line, "data: ") {
			if firstEventTime.IsZero() {
				firstEventTime = time.Now()
				result.FirstEventTime = firstEventTime.Sub(startTime)
			}
			lastEventTime = time.Now()
			
			eventData := strings.TrimPrefix(line, "data: ")
			if eventData == "[DONE]" {
				break
			}
			
			var event SSEEvent
			if err := json.Unmarshal([]byte(eventData), &event); err == nil {
				result.EventsReceived++
				result.EventTypes[event.Type]++
				
				// Extract session ID from the first event
				if result.SessionID == uuid.Nil && event.Data != nil {
					if dataMap, ok := event.Data.(map[string]interface{}); ok {
						if sessionIDStr, ok := dataMap["session_id"].(string); ok {
							if parsedID, err := uuid.Parse(sessionIDStr); err == nil {
								result.SessionID = parsedID
							}
						}
					}
				}
				
				if event.IsFinal {
					break
				}
			}
		}
	}
	
	if !lastEventTime.IsZero() {
		result.LastEventTime = lastEventTime.Sub(startTime)
	}
	
	result.Duration = time.Since(startTime)
	result.Success = result.EventsReceived > 0 && scanner.Err() == nil
	result.ResponseSize = responseSize
	
	if scanner.Err() != nil {
		result.Error = fmt.Sprintf("Scanner error: %v", scanner.Err())
		result.Success = false
	}
	
	return result
}

// benchmarkContinueChat benchmarks a single ContinueChat request
func benchmarkContinueChat(config *BenchmarkConfig, sessionID uuid.UUID, message string) ChatBenchmarkResult {
	startTime := time.Now()
	result := ChatBenchmarkResult{
		TestName:   "ContinueChatSessionStream",
		Message:    message,
		SessionID:  sessionID,
		EventTypes: make(map[string]int),
	}
	
	// Prepare request body
	requestBody := map[string]interface{}{
		"message":       message,
		"city_name":     "Esposende",
		"context_type":  "modify_itinerary",
		"user_location": config.TestUserLocation,
	}
	
	bodyBytes, err := json.Marshal(requestBody)
	if err != nil {
		result.Error = fmt.Sprintf("Failed to marshal request: %v", err)
		result.Duration = time.Since(startTime)
		return result
	}
	
	// Create HTTP request
	url := fmt.Sprintf("%s/llm/prompt-response/chat/sessions/%s/continue", config.BaseURL, sessionID)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(bodyBytes))
	if err != nil {
		result.Error = fmt.Sprintf("Failed to create request: %v", err)
		result.Duration = time.Since(startTime)
		return result
	}
	
	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+config.AuthToken)
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Cache-Control", "no-cache")
	
	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: config.Timeout,
	}
	
	// Execute request
	resp, err := client.Do(req)
	if err != nil {
		result.Error = fmt.Sprintf("Failed to execute request: %v", err)
		result.Duration = time.Since(startTime)
		return result
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		result.Error = fmt.Sprintf("HTTP error: %d", resp.StatusCode)
		result.Duration = time.Since(startTime)
		return result
	}
	
	// Process SSE stream
	scanner := bufio.NewScanner(resp.Body)
	var firstEventTime, lastEventTime time.Time
	var responseSize int64
	
	for scanner.Scan() {
		line := scanner.Text()
		responseSize += int64(len(line))
		
		if strings.HasPrefix(line, "data: ") {
			if firstEventTime.IsZero() {
				firstEventTime = time.Now()
				result.FirstEventTime = firstEventTime.Sub(startTime)
			}
			lastEventTime = time.Now()
			
			eventData := strings.TrimPrefix(line, "data: ")
			if eventData == "[DONE]" {
				break
			}
			
			var event SSEEvent
			if err := json.Unmarshal([]byte(eventData), &event); err == nil {
				result.EventsReceived++
				result.EventTypes[event.Type]++
				
				if event.IsFinal {
					break
				}
			}
		}
	}
	
	if !lastEventTime.IsZero() {
		result.LastEventTime = lastEventTime.Sub(startTime)
	}
	
	result.Duration = time.Since(startTime)
	result.Success = result.EventsReceived > 0 && scanner.Err() == nil
	result.ResponseSize = responseSize
	
	if scanner.Err() != nil {
		result.Error = fmt.Sprintf("Scanner error: %v", scanner.Err())
		result.Success = false
	}
	
	return result
}

// generateTestAuthToken generates a test JWT token for benchmarking
func generateTestAuthToken(userID uuid.UUID) string {
	// In a real scenario, this would create a proper JWT token
	// For benchmarking, we'll create a simple token format
	// This assumes your auth middleware can handle test tokens
	claims := map[string]interface{}{
		"user_id": userID.String(),
		"exp":     time.Now().Add(1 * time.Hour).Unix(),
		"iat":     time.Now().Unix(),
	}
	
	claimsBytes, _ := json.Marshal(claims)
	return fmt.Sprintf("test.%s.signature", string(claimsBytes))
}

// TestChatStreamingRoutes is a comprehensive integration test
func TestChatStreamingRoutes(t *testing.T) {
	// Skip if integration tests are not enabled
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	
	config := NewBenchmarkConfig()
	config.AuthToken = generateTestAuthToken(config.UserID)
	
	t.Run("StartChatMessageStream", func(t *testing.T) {
		testMessages := []string{
			"Plan Esposende",
			"Restaurant in Povoa de Varzim",
		}
		
		for _, message := range testMessages {
			t.Run(fmt.Sprintf("Message_%s", strings.ReplaceAll(message, " ", "_")), func(t *testing.T) {
				result := benchmarkStartChat(config, message)
				
				assert.True(t, result.Success, "StartChat should succeed: %s", result.Error)
				assert.Greater(t, result.EventsReceived, 0, "Should receive at least one event")
				assert.NotEqual(t, uuid.Nil, result.SessionID, "Should receive a valid session ID")
				assert.Less(t, result.Duration, 30*time.Second, "Should complete within 30 seconds")
				
				t.Logf("StartChat '%s': Duration=%v, Events=%d, FirstEvent=%v, LastEvent=%v", 
					message, result.Duration, result.EventsReceived, result.FirstEventTime, result.LastEventTime)
			})
		}
	})
	
	t.Run("ContinueChatSessionStream", func(t *testing.T) {
		// Start initial session
		initialResult := benchmarkStartChat(config, "Plan Esposende")
		require.True(t, initialResult.Success, "Initial chat should succeed")
		require.NotEqual(t, uuid.Nil, initialResult.SessionID, "Should get valid session ID")
		
		sessionID := initialResult.SessionID
		testMessages := []string{
			"Add Stadium",
			"Remove Stadium",
			"Add Ibis Hotel",
			"Remove Ibis Hotel",
		}
		
		for _, message := range testMessages {
			t.Run(fmt.Sprintf("Message_%s", strings.ReplaceAll(message, " ", "_")), func(t *testing.T) {
				result := benchmarkContinueChat(config, sessionID, message)
				
				assert.True(t, result.Success, "ContinueChat should succeed: %s", result.Error)
				assert.Greater(t, result.EventsReceived, 0, "Should receive at least one event")
				assert.Less(t, result.Duration, 30*time.Second, "Should complete within 30 seconds")
				
				t.Logf("ContinueChat '%s': Duration=%v, Events=%d, FirstEvent=%v, LastEvent=%v", 
					message, result.Duration, result.EventsReceived, result.FirstEventTime, result.LastEventTime)
			})
		}
	})
}

// BenchmarkConcurrentChatRequests tests concurrent requests to both endpoints
func BenchmarkConcurrentChatRequests(b *testing.B) {
	config := NewBenchmarkConfig()
	config.AuthToken = generateTestAuthToken(config.UserID)
	
	// Start initial session for continue chat tests
	initialResult := benchmarkStartChat(config, "Plan Esposende")
	if !initialResult.Success {
		b.Fatalf("Failed to start initial session: %s", initialResult.Error)
	}
	sessionID := initialResult.SessionID
	
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		var wg sync.WaitGroup
		
		// Test messages for concurrent execution
		startMessages := []string{"Plan Esposende", "Restaurant in Povoa de Varzim"}
		continueMessages := []string{"Add Stadium", "Remove Stadium"}
		
		// Start concurrent StartChat requests
		for _, msg := range startMessages {
			wg.Add(1)
			go func(message string) {
				defer wg.Done()
				result := benchmarkStartChat(config, message)
				if !result.Success {
					b.Errorf("Concurrent StartChat failed: %s", result.Error)
				}
			}(msg)
		}
		
		// Start concurrent ContinueChat requests
		for _, msg := range continueMessages {
			wg.Add(1)
			go func(message string) {
				defer wg.Done()
				result := benchmarkContinueChat(config, sessionID, message)
				if !result.Success {
					b.Errorf("Concurrent ContinueChat failed: %s", result.Error)
				}
			}(msg)
		}
		
		wg.Wait()
	}
}

// PrintBenchmarkResults prints detailed benchmark results
func PrintBenchmarkResults(results BenchmarkResults) {
	fmt.Println("=== Chat Streaming Benchmark Results ===")
	fmt.Printf("Total Duration: %v\n", results.TotalDuration)
	fmt.Printf("Total Requests: %d\n", results.TotalRequests)
	fmt.Printf("Successful Requests: %d\n", results.SuccessfulRequests)
	fmt.Printf("Failed Requests: %d\n", results.FailedRequests)
	fmt.Printf("Success Rate: %.2f%%\n", float64(results.SuccessfulRequests)/float64(results.TotalRequests)*100)
	
	fmt.Println("\n--- StartChat Results ---")
	for _, result := range results.StartChatResults {
		fmt.Printf("Message: %s\n", result.Message)
		fmt.Printf("  Duration: %v\n", result.Duration)
		fmt.Printf("  Events: %d\n", result.EventsReceived)
		fmt.Printf("  First Event: %v\n", result.FirstEventTime)
		fmt.Printf("  Success: %t\n", result.Success)
		if result.Error != "" {
			fmt.Printf("  Error: %s\n", result.Error)
		}
		fmt.Println()
	}
	
	fmt.Println("--- ContinueChat Results ---")
	for _, result := range results.ContinueChatResults {
		fmt.Printf("Message: %s\n", result.Message)
		fmt.Printf("  Duration: %v\n", result.Duration)
		fmt.Printf("  Events: %d\n", result.EventsReceived)
		fmt.Printf("  First Event: %v\n", result.FirstEventTime)
		fmt.Printf("  Success: %t\n", result.Success)
		if result.Error != "" {
			fmt.Printf("  Error: %s\n", result.Error)
		}
		fmt.Println()
	}
}