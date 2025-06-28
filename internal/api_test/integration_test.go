package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/FACorreiaa/go-poi-au-suggestions/internal/api/auth"
	"github.com/FACorreiaa/go-poi-au-suggestions/internal/api/interests"
	"github.com/FACorreiaa/go-poi-au-suggestions/internal/api/profiles"
	"github.com/FACorreiaa/go-poi-au-suggestions/internal/api/tags"
	"github.com/FACorreiaa/go-poi-au-suggestions/internal/api/user"
	"github.com/FACorreiaa/go-poi-au-suggestions/internal/types"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// IntegrationTestSuite provides a test suite for API integration tests
type IntegrationTestSuite struct {
	router     *chi.Mux
	logger     *slog.Logger
	authToken  string
	userID     string
	profileID  uuid.UUID
	testServer *httptest.Server
}

// SetupIntegrationSuite initializes the test suite with mock services
func SetupIntegrationSuite(t *testing.T) *IntegrationTestSuite {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	// Create mock services
	authService := &auth.MockAuthService{}
	userService := &user.MockUserService{}
	interestsService := &interests.MockinterestsService{}
	profileService := &profiles.MockProfileService{}
	settingsService := &settings.MockSettingsService{}
	tagsService := &tags.MocktagsService{}

	// Setup mock expectations for auth
	testUserID := uuid.New().String()
	testProfileID := uuid.New()
	testToken := "test-jwt-token"

	// Create handlers
	authHandler := auth.NewAuthHandlerImpl(authService, logger)
	userHandler := user.NewUserHandlerImpl(userService, logger)
	interestsHandler := interests.NewinterestsHandlerImpl(interestsService, logger)
	profileHandler := profiles.NewProfileHandlerImpl(profileService, logger)
	settingsHandler := settings.NewSettingsHandlerImpl(settingsService, logger)
	tagsHandler := tags.NewtagsHandlerImpl(tagsService, logger)

	// Create router
	r := router.NewRouter(
		authHandler,
		userHandler,
		interestsHandler,
		profileHandler,
		settingsHandler,
		tagsHandler,
		nil, // poi handler
		nil, // chat handler
		nil, // city handler
		nil, // admin handler
		nil, // list handler
		logger,
	)

	testServer := httptest.NewServer(r)

	return &IntegrationTestSuite{
		router:     r,
		logger:     logger,
		authToken:  testToken,
		userID:     testUserID,
		profileID:  testProfileID,
		testServer: testServer,
	}
}

func (suite *IntegrationTestSuite) TearDown() {
	if suite.testServer != nil {
		suite.testServer.Close()
	}
}

// Helper method to make authenticated requests
func (suite *IntegrationTestSuite) makeAuthenticatedRequest(method, path string, body interface{}) (*httptest.ResponseRecorder, error) {
	var reqBody *bytes.Buffer
	if body != nil {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reqBody = bytes.NewBuffer(jsonBody)
	} else {
		reqBody = bytes.NewBuffer(nil)
	}

	req := httptest.NewRequest(method, path, reqBody)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+suite.authToken)

	// Add user context for auth middleware
	ctx := context.WithValue(req.Context(), "userID", suite.userID)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	suite.router.ServeHTTP(w, req)

	return w, nil
}

// TestUserRegistrationAndAuthFlow tests the complete user registration and authentication flow
func TestUserRegistrationAndAuthFlow(t *testing.T) {
	suite := SetupIntegrationSuite(t)
	defer suite.TearDown()

	t.Run("user registration flow", func(t *testing.T) {
		// Test user registration
		registerData := map[string]interface{}{
			"username": "testuser",
			"email":    "test@example.com",
			"password": "password123",
			"role":     "user",
		}

		reqBody, _ := json.Marshal(registerData)
		req := httptest.NewRequest("POST", "/auth/register", bytes.NewBuffer(reqBody))
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		suite.router.ServeHTTP(w, req)

		// Should return 201 or appropriate response based on implementation
		assert.True(t, w.Code == http.StatusCreated || w.Code == http.StatusOK)
	})

	t.Run("user login flow", func(t *testing.T) {
		// Test user login
		loginData := map[string]interface{}{
			"email":    "test@example.com",
			"password": "password123",
		}

		reqBody, _ := json.Marshal(loginData)
		req := httptest.NewRequest("POST", "/auth/login", bytes.NewBuffer(reqBody))
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		suite.router.ServeHTTP(w, req)

		// Should return 200 or appropriate response based on implementation
		assert.True(t, w.Code == http.StatusOK || w.Code == http.StatusCreated)
	})
}

// TestUserProfileManagementFlow tests user profile creation and management
func TestUserProfileManagementFlow(t *testing.T) {
	suite := SetupIntegrationSuite(t)
	defer suite.TearDown()

	t.Run("create user profile", func(t *testing.T) {
		profileData := types.CreateProfileRequest{
			Name:               "Test Profile",
			DefaultRadiusKm:    10.0,
			PreferredBudget:    2,
			PreferredTime:      types.DayPreference("morning"),
			PreferredPace:      types.SearchPace("moderate"),
			AccessibilityNeeds: false,
		}

		w, err := suite.makeAuthenticatedRequest("POST", "/profiles", profileData)
		require.NoError(t, err)

		// Should successfully create profile
		assert.True(t, w.Code == http.StatusCreated || w.Code == http.StatusOK)
	})

	t.Run("get user profiles", func(t *testing.T) {
		w, err := suite.makeAuthenticatedRequest("GET", "/profiles", nil)
		require.NoError(t, err)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("update user profile", func(t *testing.T) {
		updateData := types.UpdateProfileRequest{
			Name:            &[]string{"Updated Profile"}[0],
			DefaultRadiusKm: &[]float64{15.0}[0],
		}

		profileIDStr := suite.profileID.String()
		w, err := suite.makeAuthenticatedRequest("PUT", "/profiles/"+profileIDStr, updateData)
		require.NoError(t, err)

		assert.Equal(t, http.StatusOK, w.Code)
	})
}

// TestInterestsManagementFlow tests interest management endpoints
func TestInterestsManagementFlow(t *testing.T) {
	suite := SetupIntegrationSuite(t)
	defer suite.TearDown()

	t.Run("get available interests", func(t *testing.T) {
		w, err := suite.makeAuthenticatedRequest("GET", "/interests", nil)
		require.NoError(t, err)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("add user interest", func(t *testing.T) {
		interestData := types.CreateInterestRequest{
			Name:        "Photography",
			Category:    "hobby",
			Description: "Love taking photos",
		}

		w, err := suite.makeAuthenticatedRequest("POST", "/interests", interestData)
		require.NoError(t, err)

		assert.True(t, w.Code == http.StatusCreated || w.Code == http.StatusOK)
	})

	t.Run("get user interests", func(t *testing.T) {
		w, err := suite.makeAuthenticatedRequest("GET", "/interests/user", nil)
		require.NoError(t, err)

		assert.Equal(t, http.StatusOK, w.Code)
	})
}

// TestSettingsManagementFlow tests user settings management
func TestSettingsManagementFlow(t *testing.T) {
	suite := SetupIntegrationSuite(t)
	defer suite.TearDown()

	t.Run("get user settings", func(t *testing.T) {
		w, err := suite.makeAuthenticatedRequest("GET", "/settings", nil)
		require.NoError(t, err)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("update user settings", func(t *testing.T) {
		settingsData := types.UpdatesettingsParams{
			DefaultSearchRadiusKm: &[]float64{20.0}[0],
			PreferAccessiblePOIs:  &[]bool{true}[0],
			BudgetLevel:           &[]int{3}[0],
		}

		profileIDStr := suite.profileID.String()
		w, err := suite.makeAuthenticatedRequest("PUT", "/settings/"+profileIDStr, settingsData)
		require.NoError(t, err)

		assert.Equal(t, http.StatusOK, w.Code)
	})
}

// TestTagsManagementFlow tests tag management endpoints
func TestTagsManagementFlow(t *testing.T) {
	suite := SetupIntegrationSuite(t)
	defer suite.TearDown()

	t.Run("get available tags", func(t *testing.T) {
		w, err := suite.makeAuthenticatedRequest("GET", "/tags", nil)
		require.NoError(t, err)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("create personal tag", func(t *testing.T) {
		tagData := types.CreatePersonalTagParams{
			Name:        "Spicy Food",
			Description: "Love spicy food",
			TagType:     "preference",
		}

		w, err := suite.makeAuthenticatedRequest("POST", "/tags", tagData)
		require.NoError(t, err)

		assert.True(t, w.Code == http.StatusCreated || w.Code == http.StatusOK)
	})
}

// TestAPIEndpointSecurity tests that endpoints properly require authentication
func TestAPIEndpointSecurity(t *testing.T) {
	suite := SetupIntegrationSuite(t)
	defer suite.TearDown()

	protectedEndpoints := []struct {
		method string
		path   string
	}{
		{"GET", "/profiles"},
		{"POST", "/profiles"},
		{"GET", "/interests"},
		{"POST", "/interests"},
		{"GET", "/settings"},
		{"PUT", "/settings/" + suite.profileID.String()},
		{"GET", "/tags"},
		{"POST", "/tags"},
	}

	for _, endpoint := range protectedEndpoints {
		t.Run(fmt.Sprintf("unauthorized access to %s %s", endpoint.method, endpoint.path), func(t *testing.T) {
			req := httptest.NewRequest(endpoint.method, endpoint.path, nil)
			// No authorization header
			w := httptest.NewRecorder()
			suite.router.ServeHTTP(w, req)

			// Should return 401 Unauthorized or redirect to login
			assert.True(t, w.Code == http.StatusUnauthorized || w.Code == http.StatusForbidden)
		})
	}
}

// TestAPIErrorHandling tests proper error responses
func TestAPIErrorHandling(t *testing.T) {
	suite := SetupIntegrationSuite(t)
	defer suite.TearDown()

	t.Run("invalid JSON request", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/auth/register", bytes.NewBufferString("invalid json"))
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		suite.router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("missing required fields", func(t *testing.T) {
		incompleteData := map[string]interface{}{
			"username": "testuser",
			// missing email and password
		}

		reqBody, _ := json.Marshal(incompleteData)
		req := httptest.NewRequest("POST", "/auth/register", bytes.NewBuffer(reqBody))
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		suite.router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("invalid UUID in path", func(t *testing.T) {
		w, err := suite.makeAuthenticatedRequest("GET", "/profiles/invalid-uuid", nil)
		require.NoError(t, err)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

// TestAPIConcurrency tests that the API handles concurrent requests properly
func TestAPIConcurrency(t *testing.T) {
	suite := SetupIntegrationSuite(t)
	defer suite.TearDown()

	const numGoroutines = 10
	results := make(chan int, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func() {
			w, err := suite.makeAuthenticatedRequest("GET", "/profiles", nil)
			if err != nil {
				results <- http.StatusInternalServerError
				return
			}
			results <- w.Code
		}()
	}

	// Collect results
	for i := 0; i < numGoroutines; i++ {
		select {
		case statusCode := <-results:
			assert.True(t, statusCode == http.StatusOK || statusCode == http.StatusUnauthorized)
		case <-time.After(5 * time.Second):
			t.Fatal("Timeout waiting for concurrent request")
		}
	}
}

// TestAPIRateLimiting tests rate limiting if implemented
func TestAPIRateLimiting(t *testing.T) {
	suite := SetupIntegrationSuite(t)
	defer suite.TearDown()

	// This test assumes rate limiting is implemented
	// Send many requests rapidly and check for rate limit responses
	const numRequests = 100
	rateLimitedCount := 0

	for i := 0; i < numRequests; i++ {
		req := httptest.NewRequest("GET", "/auth/health", nil)
		w := httptest.NewRecorder()
		suite.router.ServeHTTP(w, req)

		if w.Code == http.StatusTooManyRequests {
			rateLimitedCount++
		}
	}

	// If rate limiting is implemented, we should see some rate limited responses
	// If not implemented, this test will pass with rateLimitedCount = 0
	t.Logf("Rate limited requests: %d out of %d", rateLimitedCount, numRequests)
}
