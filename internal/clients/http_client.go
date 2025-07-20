package clients

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"go.uber.org/zap"

	"github.com/FACorreiaa/go-poi-au-suggestions/config"
)

// HTTPClient provides methods for inter-service communication
type HTTPClient struct {
	client *http.Client
	config *config.Config
	logger *zap.Logger
}

// NewHTTPClient creates a new HTTP client for service-to-service communication
func NewHTTPClient(cfg *config.Config, logger *zap.Logger) *HTTPClient {
	return &HTTPClient{
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		config: cfg,
		logger: logger,
	}
}

// ServiceRequest represents a request to another service
type ServiceRequest struct {
	Method  string
	Path    string
	Body    interface{}
	Headers map[string]string
}

// ServiceResponse represents a response from another service
type ServiceResponse struct {
	StatusCode int
	Body       []byte
	Headers    http.Header
}

// CallAuthService makes a request to the auth service
func (c *HTTPClient) CallAuthService(ctx context.Context, req ServiceRequest) (*ServiceResponse, error) {
	return c.callService(ctx, c.config.UpstreamServices.Auth, req)
}

// CallPOIService makes a request to the POI service
func (c *HTTPClient) CallPOIService(ctx context.Context, req ServiceRequest) (*ServiceResponse, error) {
	return c.callService(ctx, c.config.UpstreamServices.POI, req)
}

// CallChatService makes a request to the chat service
func (c *HTTPClient) CallChatService(ctx context.Context, req ServiceRequest) (*ServiceResponse, error) {
	return c.callService(ctx, c.config.UpstreamServices.Chat, req)
}

// CallListsService makes a request to the lists service
func (c *HTTPClient) CallListsService(ctx context.Context, req ServiceRequest) (*ServiceResponse, error) {
	return c.callService(ctx, c.config.UpstreamServices.Lists, req)
}

// CallUsersService makes a request to the users service
func (c *HTTPClient) CallUsersService(ctx context.Context, req ServiceRequest) (*ServiceResponse, error) {
	return c.callService(ctx, c.config.UpstreamServices.Users, req)
}

// CallAdminService makes a request to the admin service
func (c *HTTPClient) CallAdminService(ctx context.Context, req ServiceRequest) (*ServiceResponse, error) {
	return c.callService(ctx, c.config.UpstreamServices.Admin, req)
}

// CallCityService makes a request to the city service
func (c *HTTPClient) CallCityService(ctx context.Context, req ServiceRequest) (*ServiceResponse, error) {
	return c.callService(ctx, c.config.UpstreamServices.City, req)
}

// CallInterestsService makes a request to the interests service
func (c *HTTPClient) CallInterestsService(ctx context.Context, req ServiceRequest) (*ServiceResponse, error) {
	return c.callService(ctx, c.config.UpstreamServices.Interests, req)
}

// CallProfilesService makes a request to the profiles service
func (c *HTTPClient) CallProfilesService(ctx context.Context, req ServiceRequest) (*ServiceResponse, error) {
	return c.callService(ctx, c.config.UpstreamServices.Profiles, req)
}

// CallRecentsService makes a request to the recents service
func (c *HTTPClient) CallRecentsService(ctx context.Context, req ServiceRequest) (*ServiceResponse, error) {
	return c.callService(ctx, c.config.UpstreamServices.Recents, req)
}

// CallReviewsService makes a request to the reviews service
func (c *HTTPClient) CallReviewsService(ctx context.Context, req ServiceRequest) (*ServiceResponse, error) {
	return c.callService(ctx, c.config.UpstreamServices.Reviews, req)
}

// CallStatisticsService makes a request to the statistics service
func (c *HTTPClient) CallStatisticsService(ctx context.Context, req ServiceRequest) (*ServiceResponse, error) {
	return c.callService(ctx, c.config.UpstreamServices.Statistics, req)
}

// CallTagsService makes a request to the tags service
func (c *HTTPClient) CallTagsService(ctx context.Context, req ServiceRequest) (*ServiceResponse, error) {
	return c.callService(ctx, c.config.UpstreamServices.Tags, req)
}

// callService is the generic method that handles all service calls
func (c *HTTPClient) callService(ctx context.Context, baseURL string, req ServiceRequest) (*ServiceResponse, error) {
	// Prepare request body
	var bodyReader io.Reader
	if req.Body != nil {
		bodyBytes, err := json.Marshal(req.Body)
		if err != nil {
			c.logger.Error("Failed to marshal request body", 
				zap.Error(err),
				zap.String("service", baseURL),
				zap.String("path", req.Path))
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		bodyReader = bytes.NewReader(bodyBytes)
	}

	// Create HTTP request
	url := fmt.Sprintf("%s%s", baseURL, req.Path)
	httpReq, err := http.NewRequestWithContext(ctx, req.Method, url, bodyReader)
	if err != nil {
		c.logger.Error("Failed to create HTTP request",
			zap.Error(err),
			zap.String("url", url),
			zap.String("method", req.Method))
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}

	// Set headers
	if req.Body != nil {
		httpReq.Header.Set("Content-Type", "application/json")
	}
	for key, value := range req.Headers {
		httpReq.Header.Set(key, value)
	}

	// Add correlation ID if available in context
	if correlationID := ctx.Value("correlation-id"); correlationID != nil {
		if id, ok := correlationID.(string); ok {
			httpReq.Header.Set("X-Correlation-ID", id)
		}
	}

	// Add request ID if available in context
	if requestID := ctx.Value("request-id"); requestID != nil {
		if id, ok := requestID.(string); ok {
			httpReq.Header.Set("X-Request-ID", id)
		}
	}

	// Make the request
	c.logger.Debug("Making service call",
		zap.String("method", req.Method),
		zap.String("url", url),
		zap.String("service", baseURL))

	resp, err := c.client.Do(httpReq)
	if err != nil {
		c.logger.Error("Service call failed",
			zap.Error(err),
			zap.String("url", url),
			zap.String("method", req.Method))
		return nil, fmt.Errorf("service call failed: %w", err)
	}
	defer resp.Body.Close()

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		c.logger.Error("Failed to read response body",
			zap.Error(err),
			zap.String("url", url),
			zap.Int("status_code", resp.StatusCode))
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	c.logger.Debug("Service call completed",
		zap.String("url", url),
		zap.Int("status_code", resp.StatusCode),
		zap.Int("response_size", len(body)))

	return &ServiceResponse{
		StatusCode: resp.StatusCode,
		Body:       body,
		Headers:    resp.Header,
	}, nil
}

// CallServiceGeneric allows calling any service by name
func (c *HTTPClient) CallServiceGeneric(ctx context.Context, serviceName string, req ServiceRequest) (*ServiceResponse, error) {
	var baseURL string
	
	switch serviceName {
	case "auth":
		baseURL = c.config.UpstreamServices.Auth
	case "poi":
		baseURL = c.config.UpstreamServices.POI
	case "chat":
		baseURL = c.config.UpstreamServices.Chat
	case "lists":
		baseURL = c.config.UpstreamServices.Lists
	case "users":
		baseURL = c.config.UpstreamServices.Users
	case "admin":
		baseURL = c.config.UpstreamServices.Admin
	case "city":
		baseURL = c.config.UpstreamServices.City
	case "interests":
		baseURL = c.config.UpstreamServices.Interests
	case "profiles":
		baseURL = c.config.UpstreamServices.Profiles
	case "recents":
		baseURL = c.config.UpstreamServices.Recents
	case "reviews":
		baseURL = c.config.UpstreamServices.Reviews
	case "statistics":
		baseURL = c.config.UpstreamServices.Statistics
	case "tags":
		baseURL = c.config.UpstreamServices.Tags
	default:
		return nil, fmt.Errorf("unknown service: %s", serviceName)
	}
	
	return c.callService(ctx, baseURL, req)
}

// IsServiceHealthy checks if a service is healthy
func (c *HTTPClient) IsServiceHealthy(ctx context.Context, serviceName string) bool {
	resp, err := c.CallServiceGeneric(ctx, serviceName, ServiceRequest{
		Method: "GET",
		Path:   "/health",
	})
	
	if err != nil {
		c.logger.Warn("Service health check failed",
			zap.String("service", serviceName),
			zap.Error(err))
		return false
	}
	
	return resp.StatusCode >= 200 && resp.StatusCode < 300
}