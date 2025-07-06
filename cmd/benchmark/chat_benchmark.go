package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// BenchmarkConfig holds configuration for the benchmark
type BenchmarkConfig struct {
	BaseURL          string
	AuthToken        string
	UserID           uuid.UUID
	ProfileID        uuid.UUID
	TestUserLocation *UserLocation
	Timeout          time.Duration
	Iterations       int
	ConcurrentUsers  int
}

// UserLocation represents user location
type UserLocation struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

// BenchmarkResult represents the result of a benchmark test
type BenchmarkResult struct {
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

// BenchmarkStats holds aggregated statistics
type BenchmarkStats struct {
	TotalRequests      int
	SuccessfulRequests int
	FailedRequests     int
	TotalDuration      time.Duration
	AverageDuration    time.Duration
	MinDuration        time.Duration
	MaxDuration        time.Duration
	TotalEvents        int
	TotalResponseSize  int64
	RequestsPerSecond  float64
}

func main() {
	var (
		baseURL         = flag.String("url", "http://localhost:8080/api/v1", "Base URL for the API")
		authToken       = flag.String("token", "", "Authentication token (if empty, will generate test token)")
		iterations      = flag.Int("iterations", 5, "Number of iterations to run")
		concurrentUsers = flag.Int("concurrent", 1, "Number of concurrent users")
		timeout         = flag.Duration("timeout", 60*time.Second, "Request timeout")
		verbose         = flag.Bool("verbose", false, "Verbose output")
	)
	flag.Parse()

	config := &BenchmarkConfig{
		BaseURL:         *baseURL,
		AuthToken:       *authToken,
		UserID:          uuid.New(),
		ProfileID:       uuid.New(),
		Iterations:      *iterations,
		ConcurrentUsers: *concurrentUsers,
		Timeout:         *timeout,
		TestUserLocation: &UserLocation{
			Latitude:  41.4901, // Esposende coordinates
			Longitude: -8.7853,
		},
	}

	// Generate test token if not provided
	if config.AuthToken == "" {
		config.AuthToken = generateTestAuthToken(config.UserID)
	}

	fmt.Printf("🚀 Starting Chat Streaming Benchmark\n")
	fmt.Printf("📍 Base URL: %s\n", config.BaseURL)
	fmt.Printf("🔁 Iterations: %d\n", config.Iterations)
	fmt.Printf("👥 Concurrent Users: %d\n", config.ConcurrentUsers)
	fmt.Printf("⏱️  Timeout: %v\n", config.Timeout)
	fmt.Println()

	// Run StartChat benchmarks
	fmt.Println("🎯 Testing StartChatMessageStream...")
	startChatStats := runStartChatBenchmark(config, *verbose)
	printStats("StartChatMessageStream", startChatStats)

	// Run ContinueChat benchmarks
	fmt.Println("🔄 Testing ContinueChatSessionStream...")
	continueChatStats := runContinueChatBenchmark(config, *verbose)
	printStats("ContinueChatSessionStream", continueChatStats)

	// Run concurrent benchmark
	if config.ConcurrentUsers > 1 {
		fmt.Println("⚡ Testing Concurrent Requests...")
		concurrentStats := runConcurrentBenchmark(config, *verbose)
		printStats("Concurrent Requests", concurrentStats)
	}

	fmt.Println("✅ Benchmark completed!")
}

func runStartChatBenchmark(config *BenchmarkConfig, verbose bool) BenchmarkStats {
	testMessages := []string{
		"Plan Esposende",
		"Restaurant in Povoa de Varzim",
	}

	var results []BenchmarkResult
	var mu sync.Mutex

	for i := 0; i < config.Iterations; i++ {
		for _, message := range testMessages {
			result := benchmarkStartChat(config, message)
			mu.Lock()
			results = append(results, result)
			mu.Unlock()

			if verbose {
				fmt.Printf("  📝 '%s': %v (%d events) %s\n", 
					message, result.Duration, result.EventsReceived, getStatusEmoji(result.Success))
			}
		}
	}

	return calculateStats(results)
}

func runContinueChatBenchmark(config *BenchmarkConfig, verbose bool) BenchmarkStats {
	// First, start a chat session to get session ID
	initialResult := benchmarkStartChat(config, "Plan Esposende")
	if !initialResult.Success {
		log.Fatalf("Failed to start initial chat session: %s", initialResult.Error)
	}

	sessionID := initialResult.SessionID
	testMessages := []string{
		"Add Stadium",
		"Remove Stadium",
		"Add Ibis Hotel",
		"Remove Ibis Hotel",
	}

	var results []BenchmarkResult
	var mu sync.Mutex

	for i := 0; i < config.Iterations; i++ {
		for _, message := range testMessages {
			result := benchmarkContinueChat(config, sessionID, message)
			mu.Lock()
			results = append(results, result)
			mu.Unlock()

			if verbose {
				fmt.Printf("  📝 '%s': %v (%d events) %s\n", 
					message, result.Duration, result.EventsReceived, getStatusEmoji(result.Success))
			}
		}
	}

	return calculateStats(results)
}

func runConcurrentBenchmark(config *BenchmarkConfig, verbose bool) BenchmarkStats {
	// Start initial session for continue chat tests
	initialResult := benchmarkStartChat(config, "Plan Esposende")
	if !initialResult.Success {
		log.Fatalf("Failed to start initial session: %s", initialResult.Error)
	}
	sessionID := initialResult.SessionID

	var results []BenchmarkResult
	var mu sync.Mutex
	var wg sync.WaitGroup

	startMessages := []string{"Plan Esposende", "Restaurant in Povoa de Varzim"}
	continueMessages := []string{"Add Stadium", "Remove Stadium"}

	for i := 0; i < config.Iterations; i++ {
		// Launch concurrent requests
		for j := 0; j < config.ConcurrentUsers; j++ {
			// StartChat requests
			for _, msg := range startMessages {
				wg.Add(1)
				go func(message string, userIndex int) {
					defer wg.Done()
					
					// Create unique config for this user
					userConfig := *config
					userConfig.UserID = uuid.New()
					userConfig.ProfileID = uuid.New()
					userConfig.AuthToken = generateTestAuthToken(userConfig.UserID)
					
					result := benchmarkStartChat(&userConfig, message)
					mu.Lock()
					results = append(results, result)
					mu.Unlock()

					if verbose {
						fmt.Printf("  👤 User%d StartChat '%s': %v %s\n", 
							userIndex, message, result.Duration, getStatusEmoji(result.Success))
					}
				}(msg, j)
			}

			// ContinueChat requests
			for _, msg := range continueMessages {
				wg.Add(1)
				go func(message string, userIndex int) {
					defer wg.Done()
					
					result := benchmarkContinueChat(config, sessionID, message)
					mu.Lock()
					results = append(results, result)
					mu.Unlock()

					if verbose {
						fmt.Printf("  👤 User%d ContinueChat '%s': %v %s\n", 
							userIndex, message, result.Duration, getStatusEmoji(result.Success))
					}
				}(msg, j)
			}
		}

		wg.Wait()
	}

	return calculateStats(results)
}

func benchmarkStartChat(config *BenchmarkConfig, message string) BenchmarkResult {
	startTime := time.Now()
	result := BenchmarkResult{
		TestName:   "StartChatMessageStream",
		Message:    message,
		EventTypes: make(map[string]int),
	}

	// Prepare request body
	requestBody := map[string]interface{}{
		"message":       message,
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

func benchmarkContinueChat(config *BenchmarkConfig, sessionID uuid.UUID, message string) BenchmarkResult {
	startTime := time.Now()
	result := BenchmarkResult{
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

func generateTestAuthToken(userID uuid.UUID) string {
	// Simple test token format
	claims := map[string]interface{}{
		"user_id": userID.String(),
		"exp":     time.Now().Add(1 * time.Hour).Unix(),
		"iat":     time.Now().Unix(),
	}

	claimsBytes, _ := json.Marshal(claims)
	return fmt.Sprintf("test.%s.signature", string(claimsBytes))
}

func calculateStats(results []BenchmarkResult) BenchmarkStats {
	stats := BenchmarkStats{
		TotalRequests: len(results),
		MinDuration:   time.Hour, // Initialize to high value
	}

	var totalDurationNs int64
	var totalEvents int
	var totalResponseSize int64

	for _, result := range results {
		if result.Success {
			stats.SuccessfulRequests++
		} else {
			stats.FailedRequests++
		}

		totalDurationNs += result.Duration.Nanoseconds()
		totalEvents += result.EventsReceived
		totalResponseSize += result.ResponseSize

		if result.Duration < stats.MinDuration {
			stats.MinDuration = result.Duration
		}
		if result.Duration > stats.MaxDuration {
			stats.MaxDuration = result.Duration
		}
	}

	if len(results) > 0 {
		stats.TotalDuration = time.Duration(totalDurationNs)
		stats.AverageDuration = time.Duration(totalDurationNs / int64(len(results)))
		stats.TotalEvents = totalEvents
		stats.TotalResponseSize = totalResponseSize
		
		if stats.TotalDuration > 0 {
			stats.RequestsPerSecond = float64(stats.TotalRequests) / stats.TotalDuration.Seconds()
		}
	}

	return stats
}

func printStats(testName string, stats BenchmarkStats) {
	fmt.Printf("\n📊 %s Results:\n", testName)
	fmt.Printf("  📈 Total Requests: %d\n", stats.TotalRequests)
	fmt.Printf("  ✅ Successful: %d\n", stats.SuccessfulRequests)
	fmt.Printf("  ❌ Failed: %d\n", stats.FailedRequests)
	fmt.Printf("  📊 Success Rate: %.2f%%\n", float64(stats.SuccessfulRequests)/float64(stats.TotalRequests)*100)
	fmt.Printf("  ⏱️  Average Duration: %v\n", stats.AverageDuration)
	fmt.Printf("  🏃 Min Duration: %v\n", stats.MinDuration)
	fmt.Printf("  🐌 Max Duration: %v\n", stats.MaxDuration)
	fmt.Printf("  🚀 Requests/Second: %.2f\n", stats.RequestsPerSecond)
	fmt.Printf("  📡 Total Events: %d\n", stats.TotalEvents)
	fmt.Printf("  📦 Total Response Size: %d bytes\n", stats.TotalResponseSize)
	fmt.Println()
}

func getStatusEmoji(success bool) string {
	if success {
		return "✅"
	}
	return "❌"
}