package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"log/slog"

	"github.com/markbates/goth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/FACorreiaa/go-poi-au-suggestions/internal/types"
)

// MockAuthService is a mock implementation of the AuthService interface
type MockAuthService struct {
	mock.Mock
}

// Implement all methods of the AuthService interface
func (m *MockAuthService) Login(ctx context.Context, email, password string) (string, string, error) {
	args := m.Called(ctx, email, password)
	return args.String(0), args.String(1), args.Error(2)
}

func (m *MockAuthService) Register(ctx context.Context, username, email, password, role string) error {
	args := m.Called(ctx, username, email, password, role)
	return args.Error(0)
}

func (m *MockAuthService) RefreshSession(ctx context.Context, refreshToken string) (string, string, error) {
	args := m.Called(ctx, refreshToken)
	return args.String(0), args.String(1), args.Error(2)
}

func (m *MockAuthService) Logout(ctx context.Context, refreshToken string) error {
	args := m.Called(ctx, refreshToken)
	return args.Error(0)
}

func (m *MockAuthService) UpdatePassword(ctx context.Context, userID, oldPassword, newPassword string) error {
	args := m.Called(ctx, userID, oldPassword, newPassword)
	return args.Error(0)
}

func (m *MockAuthService) InvalidateAllUserRefreshTokens(ctx context.Context, userID string) error {
	args := m.Called(ctx, userID)
	return args.Error(0)
}

func (m *MockAuthService) GetUserByID(ctx context.Context, userID string) (*types.UserAuth, error) {
	args := m.Called(ctx, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*types.UserAuth), args.Error(1)
}

func (m *MockAuthService) VerifyPassword(ctx context.Context, userID, password string) error {
	args := m.Called(ctx, userID, password)
	return args.Error(0)
}

func (m *MockAuthService) ValidateRefreshToken(ctx context.Context, refreshToken string) (string, error) {
	args := m.Called(ctx, refreshToken)
	return args.String(0), args.Error(1)
}

func (m *MockAuthService) GenerateTokens(ctx context.Context, user *types.UserAuth, sub *types.Subscription) (accessToken string, refreshToken string, err error) {
	args := m.Called(ctx, user, sub)
	return args.String(0), args.String(0), args.Error(1)
}

func (m *MockAuthService) GetOrCreateUserFromProvider(ctx context.Context, provider string, providerUser goth.User) (*types.UserAuth, error) {
	args := m.Called(ctx, provider, providerUser)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*types.UserAuth), args.Error(1)
}

// Test cases for AuthHandlerImpl
func TestLoginHandlerImpl(t *testing.T) {
	// Create a mock service
	mockService := new(MockAuthService)
	logger := slog.Default()
	HandlerImpl := NewAuthHandlerImpl(mockService, logger)

	// Test case: successful login
	t.Run("Success", func(t *testing.T) {
		// Create request body
		loginRequest := map[string]string{
			"email":    "test@example.com",
			"password": "password123",
		}
		body, _ := json.Marshal(loginRequest)

		// Create request
		req := httptest.NewRequest(http.MethodPost, "/login", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		// Set up expectations
		mockService.On("Login", mock.Anything, loginRequest["email"], loginRequest["password"]).
			Return("access-token", "refresh-token", nil).Once()

		// Call the HandlerImpl
		HandlerImpl.Login(w, req)

		// Assert response
		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)

		assert.Equal(t, "access-token", response["access_token"])
		assert.Equal(t, "Login successful", response["message"])

		mockService.AssertExpectations(t)
	})

	// Test case: invalid request body
	t.Run("InvalidRequestBody", func(t *testing.T) {
		// Create invalid request body
		body := []byte(`{"email": "test@example.com", "password":}`) // Invalid JSON

		// Create request
		req := httptest.NewRequest(http.MethodPost, "/login", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		// Call the HandlerImpl
		HandlerImpl.Login(w, req)

		// Assert response
		assert.Equal(t, http.StatusBadRequest, w.Code)
		mockService.AssertExpectations(t)
	})

	// Test case: missing fields
	t.Run("MissingFields", func(t *testing.T) {
		// Create request with missing fields
		loginRequest := map[string]string{
			"email": "test@example.com",
			// Missing password
		}
		body, _ := json.Marshal(loginRequest)

		// Create request
		req := httptest.NewRequest(http.MethodPost, "/login", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		// Call the HandlerImpl
		HandlerImpl.Login(w, req)

		// Assert response
		assert.Equal(t, http.StatusBadRequest, w.Code)
		mockService.AssertExpectations(t)
	})

	// Test case: authentication error
	t.Run("AuthenticationError", func(t *testing.T) {
		// Create request body
		loginRequest := map[string]string{
			"email":    "test@example.com",
			"password": "wrong-password",
		}
		body, _ := json.Marshal(loginRequest)

		// Create request
		req := httptest.NewRequest(http.MethodPost, "/login", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		// Set up expectations
		mockService.On("Login", mock.Anything, loginRequest["email"], loginRequest["password"]).
			Return("", "", types.ErrUnauthenticated).Once()

		// Call the HandlerImpl
		HandlerImpl.Login(w, req)

		// Assert response
		assert.Equal(t, http.StatusUnauthorized, w.Code)
		mockService.AssertExpectations(t)
	})

	// Test case: internal server error
	t.Run("InternalServerError", func(t *testing.T) {
		// Create request body
		loginRequest := map[string]string{
			"email":    "test@example.com",
			"password": "password123",
		}
		body, _ := json.Marshal(loginRequest)

		// Create request
		req := httptest.NewRequest(http.MethodPost, "/login", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		// Set up expectations
		mockService.On("Login", mock.Anything, loginRequest["email"], loginRequest["password"]).
			Return("", "", errors.New("internal error")).Once()

		// Call the HandlerImpl
		HandlerImpl.Login(w, req)

		// Assert response
		assert.Equal(t, http.StatusInternalServerError, w.Code)
		mockService.AssertExpectations(t)
	})
}

func TestRegisterHandlerImpl(t *testing.T) {
	// Create a mock service
	mockService := new(MockAuthService)
	logger := slog.Default()
	HandlerImpl := NewAuthHandlerImpl(mockService, logger)

	// Test case: successful registration
	t.Run("Success", func(t *testing.T) {
		// Create request body
		registerRequest := map[string]string{
			"username": "testuser",
			"email":    "test@example.com",
			"password": "password123",
			"role":     "user",
		}
		body, _ := json.Marshal(registerRequest)

		// Create request
		req := httptest.NewRequest(http.MethodPost, "/register", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		// Set up expectations
		mockService.On("Register", mock.Anything, registerRequest["username"], registerRequest["email"], registerRequest["password"], registerRequest["role"]).Return(nil)

		// Call the HandlerImpl
		HandlerImpl.Register(w, req)

		// Assert response
		assert.Equal(t, http.StatusCreated, w.Code)
		mockService.AssertExpectations(t)
	})

	// Test case: invalid request body
	t.Run("InvalidRequestBody", func(t *testing.T) {
		// Create invalid request body
		body := []byte(`{"username": "testuser", "email": "test@example.com", "password":}`) // Invalid JSON

		// Create request
		req := httptest.NewRequest(http.MethodPost, "/register", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		// Call the HandlerImpl
		HandlerImpl.Register(w, req)

		// Assert response
		assert.Equal(t, http.StatusBadRequest, w.Code)
		mockService.AssertExpectations(t)
	})

	// Test case: missing fields
	t.Run("MissingFields", func(t *testing.T) {
		// Create request with missing fields
		registerRequest := map[string]string{
			"username": "testuser",
			"email":    "test@example.com",
			// Missing password
		}
		body, _ := json.Marshal(registerRequest)

		// Create request
		req := httptest.NewRequest(http.MethodPost, "/register", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		// Call the HandlerImpl
		HandlerImpl.Register(w, req)

		// Assert response
		assert.Equal(t, http.StatusBadRequest, w.Code)
		mockService.AssertExpectations(t)
	})

	// Test case: email already exists
	t.Run("EmailExists", func(t *testing.T) {
		// Create request body
		registerRequest := map[string]string{
			"username": "testuser",
			"email":    "existing@example.com",
			"password": "password123",
			"role":     "user",
		}
		body, _ := json.Marshal(registerRequest)

		// Create request
		req := httptest.NewRequest(http.MethodPost, "/register", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		// Set up expectations
		mockService.On("Register", mock.Anything, registerRequest["username"], registerRequest["email"], registerRequest["password"], registerRequest["role"]).Return(types.ErrConflict).Once()

		// Call the HandlerImpl
		HandlerImpl.Register(w, req)

		// Assert response
		assert.Equal(t, http.StatusConflict, w.Code)
		mockService.AssertExpectations(t)
	})

	// Test case: internal server error
	t.Run("InternalServerError", func(t *testing.T) {
		// Create request body
		registerRequest := map[string]string{
			"username": "testuser",
			"email":    "test@example.com",
			"password": "password123",
			"role":     "user",
		}
		body, _ := json.Marshal(registerRequest)

		// Create request
		req := httptest.NewRequest(http.MethodPost, "/register", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		// Set up expectations
		mockService.On("Register", mock.Anything, registerRequest["username"], registerRequest["email"], registerRequest["password"], registerRequest["role"]).Return(errors.New("internal error")).Once()

		// Call the HandlerImpl
		HandlerImpl.Register(w, req)

		// Assert response
		assert.Equal(t, http.StatusInternalServerError, w.Code)
		mockService.AssertExpectations(t)
	})
}

func TestLogoutHandlerImpl(t *testing.T) {
	// Create a mock service
	mockService := new(MockAuthService)
	logger := slog.Default()
	HandlerImpl := NewAuthHandlerImpl(mockService, logger)

	// Test case: successful logout
	t.Run("Success", func(t *testing.T) {
		// Create request body
		logoutRequest := map[string]string{
			"refreshToken": "valid-refresh-token",
		}
		body, _ := json.Marshal(logoutRequest)

		// Create request
		req := httptest.NewRequest(http.MethodPost, "/logout", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		// Set up expectations
		mockService.On("Logout", mock.Anything, logoutRequest["refreshToken"]).
			Return(nil).Once()

		// Call the HandlerImpl
		HandlerImpl.Logout(w, req)

		// Assert response
		assert.Equal(t, http.StatusOK, w.Code)
		mockService.AssertExpectations(t)
	})

	// Test case: invalid request body
	t.Run("InvalidRequestBody", func(t *testing.T) {
		// Create invalid request body
		body := []byte(`{"refreshToken":}`) // Invalid JSON

		// Create request
		req := httptest.NewRequest(http.MethodPost, "/logout", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		// Call the HandlerImpl
		HandlerImpl.Logout(w, req)

		// Assert response
		assert.Equal(t, http.StatusBadRequest, w.Code)
		mockService.AssertExpectations(t)
	})

	// Test case: missing refresh token
	t.Run("MissingRefreshToken", func(t *testing.T) {
		// Create request with missing refresh token
		logoutRequest := map[string]string{
			// Missing refreshToken
		}
		body, _ := json.Marshal(logoutRequest)

		// Create request
		req := httptest.NewRequest(http.MethodPost, "/logout", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		// Call the HandlerImpl
		HandlerImpl.Logout(w, req)

		// Assert response
		assert.Equal(t, http.StatusBadRequest, w.Code)
		mockService.AssertExpectations(t)
	})

	// Test case: invalid refresh token
	t.Run("InvalidRefreshToken", func(t *testing.T) {
		// Create request body
		logoutRequest := map[string]string{
			"refreshToken": "invalid-refresh-token",
		}
		body, _ := json.Marshal(logoutRequest)

		// Create request
		req := httptest.NewRequest(http.MethodPost, "/logout", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		// Set up expectations
		mockService.On("Logout", mock.Anything, logoutRequest["refreshToken"]).
			Return(types.ErrUnauthenticated).Once()

		// Call the HandlerImpl
		HandlerImpl.Logout(w, req)

		// Assert response
		assert.Equal(t, http.StatusUnauthorized, w.Code)
		mockService.AssertExpectations(t)
	})

	// Test case: internal server error
	t.Run("InternalServerError", func(t *testing.T) {
		// Create request body
		logoutRequest := map[string]string{
			"refreshToken": "valid-refresh-token",
		}
		body, _ := json.Marshal(logoutRequest)

		// Create request
		req := httptest.NewRequest(http.MethodPost, "/logout", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		// Set up expectations
		mockService.On("Logout", mock.Anything, logoutRequest["refreshToken"]).
			Return(errors.New("internal error")).Once()

		// Call the HandlerImpl
		HandlerImpl.Logout(w, req)

		// Assert response
		assert.Equal(t, http.StatusInternalServerError, w.Code)
		mockService.AssertExpectations(t)
	})
}

func TestRefreshTokenHandlerImpl(t *testing.T) {
	// Create a mock service
	mockService := new(MockAuthService)
	logger := slog.Default()
	HandlerImpl := NewAuthHandlerImpl(mockService, logger)

	// Test case: successful token refresh
	t.Run("Success", func(t *testing.T) {
		// Create request body
		refreshRequest := map[string]string{
			"refreshToken": "valid-refresh-token",
		}
		body, _ := json.Marshal(refreshRequest)

		// Create request
		req := httptest.NewRequest(http.MethodPost, "/refresh", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		// Set up expectations
		mockService.On("RefreshSession", mock.Anything, refreshRequest["refreshToken"]).
			Return("new-access-token", "new-refresh-token", nil).Once()

		// Call the HandlerImpl
		HandlerImpl.RefreshToken(w, req)

		// Assert response
		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)

		assert.Equal(t, "new-access-token", response["accessToken"])
		assert.Equal(t, "new-refresh-token", response["refreshToken"])

		mockService.AssertExpectations(t)
	})

	// Test case: invalid request body
	t.Run("InvalidRequestBody", func(t *testing.T) {
		// Create invalid request body
		body := []byte(`{"refreshToken":}`) // Invalid JSON

		// Create request
		req := httptest.NewRequest(http.MethodPost, "/refresh", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		// Call the HandlerImpl
		HandlerImpl.RefreshToken(w, req)

		// Assert response
		assert.Equal(t, http.StatusBadRequest, w.Code)
		mockService.AssertExpectations(t)
	})

	// Test case: missing refresh token
	t.Run("MissingRefreshToken", func(t *testing.T) {
		// Create request with missing refresh token
		refreshRequest := map[string]string{
			// Missing refreshToken
		}
		body, _ := json.Marshal(refreshRequest)

		// Create request
		req := httptest.NewRequest(http.MethodPost, "/refresh", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		// Call the HandlerImpl
		HandlerImpl.RefreshToken(w, req)

		// Assert response
		assert.Equal(t, http.StatusBadRequest, w.Code)
		mockService.AssertExpectations(t)
	})

	// Test case: invalid refresh token
	t.Run("InvalidRefreshToken", func(t *testing.T) {
		// Create request body
		refreshRequest := map[string]string{
			"refreshToken": "invalid-refresh-token",
		}
		body, _ := json.Marshal(refreshRequest)

		// Create request
		req := httptest.NewRequest(http.MethodPost, "/refresh", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		// Set up expectations
		mockService.On("RefreshSession", mock.Anything, refreshRequest["refreshToken"]).
			Return("", "", types.ErrUnauthenticated).Once()

		// Call the HandlerImpl
		HandlerImpl.RefreshToken(w, req)

		// Assert response
		assert.Equal(t, http.StatusUnauthorized, w.Code)
		mockService.AssertExpectations(t)
	})

	// Test case: internal server error
	t.Run("InternalServerError", func(t *testing.T) {
		// Create request body
		refreshRequest := map[string]string{
			"refreshToken": "valid-refresh-token",
		}
		body, _ := json.Marshal(refreshRequest)

		// Create request
		req := httptest.NewRequest(http.MethodPost, "/refresh", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		// Set up expectations
		mockService.On("RefreshSession", mock.Anything, refreshRequest["refreshToken"]).
			Return("", "", errors.New("internal error")).Once()

		// Call the HandlerImpl
		HandlerImpl.RefreshToken(w, req)

		// Assert response
		assert.Equal(t, http.StatusInternalServerError, w.Code)
		mockService.AssertExpectations(t)
	})
}

func TestValidateSessionHandlerImpl(t *testing.T) {
	// Create a mock service
	mockService := new(MockAuthService)
	logger := slog.Default()
	HandlerImpl := NewAuthHandlerImpl(mockService, logger)

	// Test case: successful validation
	t.Run("Success", func(t *testing.T) {
		// Create request with Authorization header
		req := httptest.NewRequest(http.MethodGet, "/validate", nil)
		req.Header.Set("Authorization", "Bearer valid-access-token")
		w := httptest.NewRecorder()

		// Set up context with user ID (simulating middleware)
		ctx := context.WithValue(req.Context(), UserIDKey, "user123")
		req = req.WithContext(ctx)

		// Set up expectations
		user := &types.UserAuth{
			ID:       "user123",
			Username: "testuser",
			Email:    "test@example.com",
			Role:     "user",
		}
		mockService.On("GetUserByID", mock.Anything, "user123").Return(user, nil).Once()

		// Call the HandlerImpl
		HandlerImpl.ValidateSession(w, req)

		// Assert response
		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)

		assert.Equal(t, user.ID, response["id"])
		assert.Equal(t, user.Username, response["username"])
		assert.Equal(t, user.Email, response["email"])
		assert.Equal(t, user.Role, response["role"])

		mockService.AssertExpectations(t)
	})

	// Test case: missing user ID in context
	t.Run("MissingUserID", func(t *testing.T) {
		// Create request with Authorization header but no user ID in context
		req := httptest.NewRequest(http.MethodGet, "/validate", nil)
		req.Header.Set("Authorization", "Bearer valid-access-token")
		w := httptest.NewRecorder()

		// Call the HandlerImpl
		HandlerImpl.ValidateSession(w, req)

		// Assert response
		assert.Equal(t, http.StatusUnauthorized, w.Code)
		mockService.AssertExpectations(t)
	})

	// Test case: user not found
	t.Run("UserNotFound", func(t *testing.T) {
		// Create request with Authorization header
		req := httptest.NewRequest(http.MethodGet, "/validate", nil)
		req.Header.Set("Authorization", "Bearer valid-access-token")
		w := httptest.NewRecorder()

		// Set up context with user ID (simulating middleware)
		ctx := context.WithValue(req.Context(), UserIDKey, "nonexistent-user")
		req = req.WithContext(ctx)

		// Set up expectations
		mockService.On("GetUserByID", mock.Anything, "nonexistent-user").Return(nil, types.ErrNotFound).Once()

		// Call the HandlerImpl
		HandlerImpl.ValidateSession(w, req)

		// Assert response
		assert.Equal(t, http.StatusUnauthorized, w.Code)
		mockService.AssertExpectations(t)
	})

	// Test case: internal server error
	t.Run("InternalServerError", func(t *testing.T) {
		// Create request with Authorization header
		req := httptest.NewRequest(http.MethodGet, "/validate", nil)
		req.Header.Set("Authorization", "Bearer valid-access-token")
		w := httptest.NewRecorder()

		// Set up context with user ID (simulating middleware)
		ctx := context.WithValue(req.Context(), UserIDKey, "user123")
		req = req.WithContext(ctx)

		// Set up expectations
		mockService.On("GetUserByID", mock.Anything, "user123").Return(nil, errors.New("database error")).Once()

		// Call the HandlerImpl
		HandlerImpl.ValidateSession(w, req)

		// Assert response
		assert.Equal(t, http.StatusInternalServerError, w.Code)
		mockService.AssertExpectations(t)
	})
}

func TestChangePasswordHandlerImpl(t *testing.T) {
	// Create a mock service
	mockService := new(MockAuthService)
	logger := slog.Default()
	HandlerImpl := NewAuthHandlerImpl(mockService, logger)

	// Test case: successful password change
	t.Run("Success", func(t *testing.T) {
		// Create request body
		changePasswordRequest := map[string]string{
			"oldPassword": "oldpassword",
			"newPassword": "newpassword",
		}
		body, _ := json.Marshal(changePasswordRequest)

		// Create request
		req := httptest.NewRequest(http.MethodPost, "/change-password", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		// Set up context with user ID (simulating middleware)
		ctx := context.WithValue(req.Context(), UserIDKey, "user123")
		req = req.WithContext(ctx)

		// Set up expectations
		mockService.On("UpdatePassword", mock.Anything, "user123", changePasswordRequest["oldPassword"], changePasswordRequest["newPassword"]).
			Return(nil).Once()

		// Call the HandlerImpl
		HandlerImpl.ChangePassword(w, req)

		// Assert response
		assert.Equal(t, http.StatusOK, w.Code)
		mockService.AssertExpectations(t)
	})

	// Test case: invalid request body
	t.Run("InvalidRequestBody", func(t *testing.T) {
		// Create invalid request body
		body := []byte(`{"oldPassword": "oldpassword", "newPassword":}`) // Invalid JSON

		// Create request
		req := httptest.NewRequest(http.MethodPost, "/change-password", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		// Set up context with user ID (simulating middleware)
		ctx := context.WithValue(req.Context(), UserIDKey, "user123")
		req = req.WithContext(ctx)

		// Call the HandlerImpl
		HandlerImpl.ChangePassword(w, req)

		// Assert response
		assert.Equal(t, http.StatusBadRequest, w.Code)
		mockService.AssertExpectations(t)
	})

	// Test case: missing fields
	t.Run("MissingFields", func(t *testing.T) {
		// Create request with missing fields
		changePasswordRequest := map[string]string{
			"oldPassword": "oldpassword",
			// Missing newPassword
		}
		body, _ := json.Marshal(changePasswordRequest)

		// Create request
		req := httptest.NewRequest(http.MethodPost, "/change-password", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		// Call the HandlerImpl
		HandlerImpl.ChangePassword(w, req)

		// Assert response
		assert.Equal(t, http.StatusBadRequest, w.Code)
		mockService.AssertExpectations(t)
	})

	// Test case: missing user ID in context
	t.Run("MissingUserID", func(t *testing.T) {
		// Create request body
		changePasswordRequest := map[string]string{
			"oldPassword": "oldpassword",
			"newPassword": "newpassword",
		}
		body, _ := json.Marshal(changePasswordRequest)

		// Create request with no user ID in context
		req := httptest.NewRequest(http.MethodPost, "/change-password", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		// Call the HandlerImpl
		HandlerImpl.ChangePassword(w, req)

		// Assert response
		assert.Equal(t, http.StatusUnauthorized, w.Code)
		mockService.AssertExpectations(t)
	})

	// Test case: incorrect old password
	t.Run("IncorrectOldPassword", func(t *testing.T) {
		// Create request body
		changePasswordRequest := map[string]string{
			"oldPassword": "wrongpassword",
			"newPassword": "newpassword",
		}
		body, _ := json.Marshal(changePasswordRequest)

		// Create request
		req := httptest.NewRequest(http.MethodPost, "/change-password", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		// Set up context with user ID (simulating middleware)
		ctx := context.WithValue(req.Context(), UserIDKey, "user123")
		req = req.WithContext(ctx)

		// Set up expectations
		mockService.On("UpdatePassword", mock.Anything, "user123", changePasswordRequest["oldPassword"], changePasswordRequest["newPassword"]).
			Return(types.ErrUnauthenticated).Once()

		// Call the HandlerImpl
		HandlerImpl.ChangePassword(w, req)

		// Assert response
		assert.Equal(t, http.StatusUnauthorized, w.Code)
		mockService.AssertExpectations(t)
	})

	// Test case: internal server error
	t.Run("InternalServerError", func(t *testing.T) {
		// Create request body
		changePasswordRequest := map[string]string{
			"oldPassword": "oldpassword",
			"newPassword": "newpassword",
		}
		body, _ := json.Marshal(changePasswordRequest)

		// Create request
		req := httptest.NewRequest(http.MethodPost, "/change-password", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		// Set up context with user ID (simulating middleware)
		ctx := context.WithValue(req.Context(), UserIDKey, "user123")
		req = req.WithContext(ctx)

		// Set up expectations
		mockService.On("UpdatePassword", mock.Anything, "user123", changePasswordRequest["oldPassword"], changePasswordRequest["newPassword"]).
			Return(errors.New("database error")).Once()

		// Call the HandlerImpl
		HandlerImpl.ChangePassword(w, req)

		// Assert response
		assert.Equal(t, http.StatusInternalServerError, w.Code)
		mockService.AssertExpectations(t)
	})
}

func TestChangeEmailHandlerImpl(t *testing.T) {
	// Create a mock service
	mockService := new(MockAuthService)
	logger := slog.Default()
	_ = NewAuthHandlerImpl(mockService, logger)

	// Note: Since the ChangeEmail HandlerImpl is not fully implemented in the auth_HandlerImpl.go file,
	// we can add a basic test when it's implemented
}
