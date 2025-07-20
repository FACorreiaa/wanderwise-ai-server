package clients

import (
	"context"
	"encoding/json"
	"fmt"

	"go.uber.org/zap"
)

// ExampleService demonstrates how to use the HTTP client for inter-service communication
type ExampleService struct {
	httpClient *HTTPClient
	logger     *zap.Logger
}

// NewExampleService creates a new example service
func NewExampleService(httpClient *HTTPClient, logger *zap.Logger) *ExampleService {
	return &ExampleService{
		httpClient: httpClient,
		logger:     logger,
	}
}

// User represents a user entity
type User struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

// POI represents a point of interest
type POI struct {
	ID          int     `json:"id"`
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Latitude    float64 `json:"latitude"`
	Longitude   float64 `json:"longitude"`
}

// GetUserFromUsersService fetches a user from the users service
func (e *ExampleService) GetUserFromUsersService(ctx context.Context, userID int) (*User, error) {
	resp, err := e.httpClient.CallUsersService(ctx, ServiceRequest{
		Method: "GET",
		Path:   fmt.Sprintf("/users/%d", userID),
		Headers: map[string]string{
			"Accept": "application/json",
		},
	})
	if err != nil {
		e.logger.Error("Failed to call users service",
			zap.Error(err),
			zap.Int("user_id", userID))
		return nil, fmt.Errorf("failed to call users service: %w", err)
	}

	if resp.StatusCode != 200 {
		e.logger.Error("Users service returned error",
			zap.Int("status_code", resp.StatusCode),
			zap.Int("user_id", userID))
		return nil, fmt.Errorf("users service returned status %d", resp.StatusCode)
	}

	var user User
	if err := json.Unmarshal(resp.Body, &user); err != nil {
		e.logger.Error("Failed to unmarshal user response",
			zap.Error(err),
			zap.Int("user_id", userID))
		return nil, fmt.Errorf("failed to unmarshal user response: %w", err)
	}

	return &user, nil
}

// CreatePOIInPOIService creates a new POI in the POI service
func (e *ExampleService) CreatePOIInPOIService(ctx context.Context, poi POI) (*POI, error) {
	resp, err := e.httpClient.CallPOIService(ctx, ServiceRequest{
		Method: "POST",
		Path:   "/poi",
		Body:   poi,
		Headers: map[string]string{
			"Content-Type": "application/json",
			"Accept":       "application/json",
		},
	})
	if err != nil {
		e.logger.Error("Failed to call POI service",
			zap.Error(err),
			zap.String("poi_name", poi.Name))
		return nil, fmt.Errorf("failed to call POI service: %w", err)
	}

	if resp.StatusCode != 201 && resp.StatusCode != 200 {
		e.logger.Error("POI service returned error",
			zap.Int("status_code", resp.StatusCode),
			zap.String("poi_name", poi.Name))
		return nil, fmt.Errorf("POI service returned status %d", resp.StatusCode)
	}

	var createdPOI POI
	if err := json.Unmarshal(resp.Body, &createdPOI); err != nil {
		e.logger.Error("Failed to unmarshal POI response",
			zap.Error(err),
			zap.String("poi_name", poi.Name))
		return nil, fmt.Errorf("failed to unmarshal POI response: %w", err)
	}

	return &createdPOI, nil
}

// AuthenticateWithAuthService validates a token with the auth service
func (e *ExampleService) AuthenticateWithAuthService(ctx context.Context, token string) (bool, error) {
	resp, err := e.httpClient.CallAuthService(ctx, ServiceRequest{
		Method: "POST",
		Path:   "/auth/validate",
		Body: map[string]string{
			"token": token,
		},
		Headers: map[string]string{
			"Content-Type": "application/json",
			"Accept":       "application/json",
		},
	})
	if err != nil {
		e.logger.Error("Failed to call auth service",
			zap.Error(err))
		return false, fmt.Errorf("failed to call auth service: %w", err)
	}

	// Return true if the auth service returns 200 (valid token)
	return resp.StatusCode == 200, nil
}

// GetRecommendationsFromChatService gets AI recommendations from the chat service
func (e *ExampleService) GetRecommendationsFromChatService(ctx context.Context, userID int, preferences []string) ([]string, error) {
	requestBody := map[string]interface{}{
		"user_id":     userID,
		"preferences": preferences,
		"type":        "recommendations",
	}

	resp, err := e.httpClient.CallChatService(ctx, ServiceRequest{
		Method: "POST",
		Path:   "/chat/recommendations",
		Body:   requestBody,
		Headers: map[string]string{
			"Content-Type": "application/json",
			"Accept":       "application/json",
		},
	})
	if err != nil {
		e.logger.Error("Failed to call chat service",
			zap.Error(err),
			zap.Int("user_id", userID))
		return nil, fmt.Errorf("failed to call chat service: %w", err)
	}

	if resp.StatusCode != 200 {
		e.logger.Error("Chat service returned error",
			zap.Int("status_code", resp.StatusCode),
			zap.Int("user_id", userID))
		return nil, fmt.Errorf("chat service returned status %d", resp.StatusCode)
	}

	var response struct {
		Recommendations []string `json:"recommendations"`
	}
	if err := json.Unmarshal(resp.Body, &response); err != nil {
		e.logger.Error("Failed to unmarshal chat response",
			zap.Error(err),
			zap.Int("user_id", userID))
		return nil, fmt.Errorf("failed to unmarshal chat response: %w", err)
	}

	return response.Recommendations, nil
}

// BulkOperationExample shows how to call multiple services in a coordinated way
func (e *ExampleService) BulkOperationExample(ctx context.Context, userID int) error {
	// 1. First, get user details
	user, err := e.GetUserFromUsersService(ctx, userID)
	if err != nil {
		return fmt.Errorf("failed to get user: %w", err)
	}

	e.logger.Info("Retrieved user", zap.String("user_name", user.Name))

	// 2. Get user's interests from interests service
	interestsResp, err := e.httpClient.CallInterestsService(ctx, ServiceRequest{
		Method: "GET",
		Path:   fmt.Sprintf("/interests/user/%d", userID),
	})
	if err != nil {
		return fmt.Errorf("failed to get user interests: %w", err)
	}

	var interests []string
	if interestsResp.StatusCode == 200 {
		json.Unmarshal(interestsResp.Body, &interests)
	}

	// 3. Get recommendations based on interests
	recommendations, err := e.GetRecommendationsFromChatService(ctx, userID, interests)
	if err != nil {
		e.logger.Warn("Failed to get recommendations, continuing without them", zap.Error(err))
		recommendations = []string{"default recommendation"}
	}

	// 4. Update user's recent activity
	_, err = e.httpClient.CallRecentsService(ctx, ServiceRequest{
		Method: "POST",
		Path:   "/recents",
		Body: map[string]interface{}{
			"user_id":       userID,
			"activity_type": "bulk_operation",
			"data": map[string]interface{}{
				"recommendations": recommendations,
				"interests":       interests,
			},
		},
	})
	if err != nil {
		e.logger.Error("Failed to update recent activity", zap.Error(err))
		// Don't fail the entire operation for this
	}

	e.logger.Info("Bulk operation completed successfully",
		zap.Int("user_id", userID),
		zap.Int("recommendations_count", len(recommendations)),
		zap.Int("interests_count", len(interests)))

	return nil
}