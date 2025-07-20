package auth

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"golang.org/x/crypto/bcrypt"

	"github.com/FACorreiaa/go-poi-au-suggestions/config"
	"github.com/FACorreiaa/go-poi-au-suggestions/internal/types"
)

// MockAuthRepo is a mock implementation of the AuthRepo interface
type MockAuthRepo struct {
	mock.Mock
}

// Implement all methods of the AuthRepo interface
func (m *MockAuthRepo) GetUserByEmail(ctx context.Context, email string) (*types.UserAuth, error) {
	args := m.Called(ctx, email)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*types.UserAuth), args.Error(1)
}

func (m *MockAuthRepo) GetUserByID(ctx context.Context, userID string) (*types.UserAuth, error) {
	args := m.Called(ctx, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*types.UserAuth), args.Error(1)
}

func (m *MockAuthRepo) Register(ctx context.Context, username, email, hashedPassword string) (string, error) {
	args := m.Called(ctx, username, email, hashedPassword)
	return args.String(0), args.Error(1)
}

func (m *MockAuthRepo) VerifyPassword(ctx context.Context, userID, password string) error {
	args := m.Called(ctx, userID, password)
	return args.Error(0)
}

func (m *MockAuthRepo) UpdatePassword(ctx context.Context, userID, newHashedPassword string) error {
	args := m.Called(ctx, userID, newHashedPassword)
	return args.Error(0)
}

func (m *MockAuthRepo) StoreRefreshToken(ctx context.Context, userID, token string, expiresAt time.Time) error {
	args := m.Called(ctx, userID, token, expiresAt)
	return args.Error(0)
}

func (m *MockAuthRepo) ValidateRefreshTokenAndGetUserID(ctx context.Context, refreshToken string) (string, error) {
	args := m.Called(ctx, refreshToken)
	return args.String(0), args.Error(1)
}

func (m *MockAuthRepo) InvalidateRefreshToken(ctx context.Context, refreshToken string) error {
	args := m.Called(ctx, refreshToken)
	return args.Error(0)
}

func (m *MockAuthRepo) InvalidateAllUserRefreshTokens(ctx context.Context, userID string) error {
	args := m.Called(ctx, userID)
	return args.Error(0)
}

func (m *MockAuthRepo) CreateUser(ctx context.Context, user *types.UserAuth) error {
	args := m.Called(ctx, user)
	return args.Error(0)
}

func (m *MockAuthRepo) CreateUserProvider(ctx context.Context, userID, provider, providerUserID string) error {
	args := m.Called(ctx, userID, provider, providerUserID)
	return args.Error(0)
}

func (m *MockAuthRepo) GetUserIDByProvider(ctx context.Context, provider, providerUserID string) (string, error) {
	args := m.Called(ctx, provider, providerUserID)
	return args.String(0), args.Error(0)
}

// Test cases for AuthService
func TestLogin(t *testing.T) {
	// Create a mock repository
	mockRepo := new(MockAuthRepo)
	logger := slog.Default()
	cfg := &config.Config{
		JWT: config.JWTConfig{
			SecretKey:       "test-access-secret",
			AccessTokenTTL:  15 * time.Minute,
			RefreshTokenTTL: 7 * 24 * time.Hour,
			Issuer:          "test-issuer",
			Audience:        "test-audience",
		},
	}
	service := NewAuthService(mockRepo, cfg, logger)

	// Test case: successful login
	t.Run("Success", func(t *testing.T) {
		ctx := context.Background()
		email := "test@example.com"
		password := "password123"
		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		assert.NoError(t, err)

		user := &types.UserAuth{
			ID:       "user123",
			Username: "testuser",
			Email:    email,
			Password: string(hashedPassword),
		}

		// Set up expectations
		mockRepo.On("GetUserByEmail", ctx, email).Return(user, nil).Once()
		mockRepo.On("StoreRefreshToken", ctx, user.ID, mock.AnythingOfType("string"), mock.AnythingOfType("time.Time")).Return(nil).Once()

		// Call the service method
		accessToken, refreshToken, err := service.Login(ctx, email, password)

		// Assert expectations
		assert.NoError(t, err)
		assert.NotEmpty(t, accessToken)
		assert.NotEmpty(t, refreshToken)
		mockRepo.AssertExpectations(t)
	})

	// Test case: user not found
	t.Run("UserNotFound", func(t *testing.T) {
		ctx := context.Background()
		email := "nonexistent@example.com"
		password := "password123"

		// Set up expectations
		mockRepo.On("GetUserByEmail", ctx, email).Return(nil, types.ErrNotFound).Once()

		// Call the service method
		accessToken, refreshToken, err := service.Login(ctx, email, password)

		// Assert expectations
		assert.Error(t, err)
		assert.Empty(t, accessToken)
		assert.Empty(t, refreshToken)
		assert.ErrorIs(t, err, types.ErrUnauthenticated)
		mockRepo.AssertExpectations(t)
	})

	// Test case: invalid password
	t.Run("InvalidPassword", func(t *testing.T) {
		ctx := context.Background()
		email := "test@example.com"
		password := "wrongpassword"
		hashedPassword, err := bcrypt.GenerateFromPassword([]byte("correctpassword"), bcrypt.DefaultCost)
		assert.NoError(t, err)

		user := &types.UserAuth{
			ID:       "user123",
			Username: "testuser",
			Email:    email,
			Password: string(hashedPassword),
		}

		// Set up expectations
		mockRepo.On("GetUserByEmail", ctx, email).Return(user, nil).Once()

		// Call the service method
		accessToken, refreshToken, err := service.Login(ctx, email, password)

		// Assert expectations
		assert.Error(t, err)
		assert.Empty(t, accessToken)
		assert.Empty(t, refreshToken)
		assert.ErrorIs(t, err, types.ErrUnauthenticated)
		mockRepo.AssertExpectations(t)
	})
}

func TestRegister(t *testing.T) {
	// Create a mock repository
	mockRepo := new(MockAuthRepo)
	logger := slog.Default()
	cfg := &config.Config{
		JWT: config.JWTConfig{
			SecretKey:       "test-access-secret",
			AccessTokenTTL:  15 * time.Minute,
			RefreshTokenTTL: 7 * 24 * time.Hour,
			Issuer:          "test-issuer",
			Audience:        "test-audience",
		},
	}
	service := NewAuthService(mockRepo, cfg, logger)

	// Test case: successful registration
	t.Run("Success", func(t *testing.T) {
		ctx := context.Background()
		username := "newuser"
		email := "new@example.com"
		password := "password123"
		userID := "new-user-id"

		// Set up expectations - we can't predict the exact hashed password, so use mock.AnythingOfType
		mockRepo.On("Register", ctx, username, email, mock.AnythingOfType("string")).Return(userID, nil).Once()

		// Call the service method
		err := service.Register(ctx, username, email, password, "user")

		// Assert expectations
		assert.NoError(t, err)
		mockRepo.AssertExpectations(t)
	})

	// Test case: email already exists
	t.Run("EmailExists", func(t *testing.T) {
		ctx := context.Background()
		username := "existinguser"
		email := "existing@example.com"
		password := "password123"

		// Set up expectations
		mockRepo.On("Register", ctx, username, email, mock.AnythingOfType("string")).Return("", types.ErrConflict).Once()

		// Call the service method
		err := service.Register(ctx, username, email, password, "user")

		// Assert expectations
		assert.Error(t, err)
		assert.ErrorIs(t, err, types.ErrConflict)
		mockRepo.AssertExpectations(t)
	})
}

func TestLogout(t *testing.T) {
	// Create a mock repository
	mockRepo := new(MockAuthRepo)
	logger := slog.Default()
	cfg := &config.Config{
		JWT: config.JWTConfig{
			SecretKey:       "test-access-secret",
			AccessTokenTTL:  15 * time.Minute,
			RefreshTokenTTL: 7 * 24 * time.Hour,
			Issuer:          "test-issuer",
			Audience:        "test-audience",
		},
	}
	service := NewAuthService(mockRepo, cfg, logger)

	// Test case: successful logout
	t.Run("Success", func(t *testing.T) {
		ctx := context.Background()
		refreshToken := "valid-refresh-token"

		// Set up expectations
		mockRepo.On("InvalidateRefreshToken", ctx, refreshToken).Return(nil).Once()

		// Call the service method
		err := service.Logout(ctx, refreshToken)

		// Assert expectations
		assert.NoError(t, err)
		mockRepo.AssertExpectations(t)
	})

	// Test case: error invalidating token
	t.Run("Error", func(t *testing.T) {
		ctx := context.Background()
		refreshToken := "invalid-refresh-token"
		expectedError := errors.New("database error")

		// Set up expectations
		mockRepo.On("InvalidateRefreshToken", ctx, refreshToken).Return(expectedError).Once()

		// Call the service method
		err := service.Logout(ctx, refreshToken)

		// Assert expectations
		assert.Error(t, err)
		assert.Contains(t, err.Error(), expectedError.Error())
		mockRepo.AssertExpectations(t)
	})
}

func TestRefreshSession(t *testing.T) {
	// Create a mock repository
	mockRepo := new(MockAuthRepo)
	logger := slog.Default()
	cfg := &config.Config{
		JWT: config.JWTConfig{
			SecretKey:       "test-access-secret",
			AccessTokenTTL:  15 * time.Minute,
			RefreshTokenTTL: 7 * 24 * time.Hour,
			Issuer:          "test-issuer",
			Audience:        "test-audience",
		},
	}
	service := NewAuthService(mockRepo, cfg, logger)

	// Test case: successful refresh
	t.Run("Success", func(t *testing.T) {
		ctx := context.Background()
		refreshToken := "valid-refresh-token"
		userID := "user123"

		user := &types.UserAuth{
			ID:       userID,
			Username: "testuser",
			Email:    "test@example.com",
			Role:     "user",
		}

		// Set up expectations
		mockRepo.On("ValidateRefreshTokenAndGetUserID", ctx, refreshToken).Return(userID, nil).Once()
		mockRepo.On("GetUserByID", ctx, userID).Return(user, nil).Once()
		mockRepo.On("InvalidateRefreshToken", ctx, refreshToken).Return(nil).Once()
		mockRepo.On("StoreRefreshToken", ctx, userID, mock.AnythingOfType("string"), mock.AnythingOfType("time.Time")).Return(nil).Once()

		// Call the service method
		accessToken, newRefreshToken, err := service.RefreshSession(ctx, refreshToken)

		// Assert expectations
		assert.NoError(t, err)
		assert.NotEmpty(t, accessToken)
		assert.NotEmpty(t, newRefreshToken)
		mockRepo.AssertExpectations(t)
	})

	// Test case: invalid refresh token
	t.Run("InvalidRefreshToken", func(t *testing.T) {
		ctx := context.Background()
		refreshToken := "invalid-refresh-token"

		// Set up expectations
		mockRepo.On("ValidateRefreshTokenAndGetUserID", ctx, refreshToken).Return("", types.ErrUnauthenticated).Once()

		// Call the service method
		accessToken, newRefreshToken, err := service.RefreshSession(ctx, refreshToken)

		// Assert expectations
		assert.Error(t, err)
		assert.Empty(t, accessToken)
		assert.Empty(t, newRefreshToken)
		assert.ErrorIs(t, err, types.ErrUnauthenticated)
		mockRepo.AssertExpectations(t)
	})

	// Test case: user not found
	t.Run("UserNotFound", func(t *testing.T) {
		ctx := context.Background()
		refreshToken := "valid-refresh-token"
		userID := "nonexistent-user"

		// Set up expectations
		mockRepo.On("ValidateRefreshTokenAndGetUserID", ctx, refreshToken).Return(userID, nil).Once()
		mockRepo.On("GetUserByID", ctx, userID).Return(nil, types.ErrNotFound).Once()

		// Call the service method
		accessToken, newRefreshToken, err := service.RefreshSession(ctx, refreshToken)

		// Assert expectations
		assert.Error(t, err)
		assert.Empty(t, accessToken)
		assert.Empty(t, newRefreshToken)
		assert.ErrorIs(t, err, types.ErrUnauthenticated)
		mockRepo.AssertExpectations(t)
	})

	// Test case: error invalidating old token
	t.Run("ErrorInvalidatingToken", func(t *testing.T) {
		ctx := context.Background()
		refreshToken := "valid-refresh-token"
		userID := "user123"
		expectedError := errors.New("database error")

		user := &types.UserAuth{
			ID:       userID,
			Username: "testuser",
			Email:    "test@example.com",
			Role:     "user",
		}

		// Set up expectations
		mockRepo.On("ValidateRefreshTokenAndGetUserID", ctx, refreshToken).Return(userID, nil).Once()
		mockRepo.On("GetUserByID", ctx, userID).Return(user, nil).Once()
		mockRepo.On("InvalidateRefreshToken", ctx, refreshToken).Return(expectedError).Once()

		// Call the service method
		accessToken, newRefreshToken, err := service.RefreshSession(ctx, refreshToken)

		// Assert expectations
		assert.Error(t, err)
		assert.Empty(t, accessToken)
		assert.Empty(t, newRefreshToken)
		assert.Contains(t, err.Error(), expectedError.Error())
		mockRepo.AssertExpectations(t)
	})

	// Test case: error storing new token
	t.Run("ErrorStoringToken", func(t *testing.T) {
		ctx := context.Background()
		refreshToken := "valid-refresh-token"
		userID := "user123"
		expectedError := errors.New("database error")

		user := &types.UserAuth{
			ID:       userID,
			Username: "testuser",
			Email:    "test@example.com",
			Role:     "user",
		}

		// Set up expectations
		mockRepo.On("ValidateRefreshTokenAndGetUserID", ctx, refreshToken).Return(userID, nil).Once()
		mockRepo.On("GetUserByID", ctx, userID).Return(user, nil).Once()
		mockRepo.On("InvalidateRefreshToken", ctx, refreshToken).Return(nil).Once()
		mockRepo.On("StoreRefreshToken", ctx, userID, mock.AnythingOfType("string"), mock.AnythingOfType("time.Time")).Return(expectedError).Once()

		// Call the service method
		accessToken, newRefreshToken, err := service.RefreshSession(ctx, refreshToken)

		// Assert expectations
		assert.Error(t, err)
		assert.Empty(t, accessToken)
		assert.Empty(t, newRefreshToken)
		assert.Contains(t, err.Error(), expectedError.Error())
		mockRepo.AssertExpectations(t)
	})
}

func TestUpdatePassword(t *testing.T) {
	// Create a mock repository
	mockRepo := new(MockAuthRepo)
	logger := slog.Default()
	cfg := &config.Config{
		JWT: config.JWTConfig{
			SecretKey:       "test-access-secret",
			AccessTokenTTL:  15 * time.Minute,
			RefreshTokenTTL: 7 * 24 * time.Hour,
			Issuer:          "test-issuer",
			Audience:        "test-audience",
		},
	}
	service := NewAuthService(mockRepo, cfg, logger)

	// Test case: successful password update
	t.Run("Success", func(t *testing.T) {
		ctx := context.Background()
		userID := "user123"
		oldPassword := "oldpassword"
		newPassword := "newpassword"

		// Set up expectations
		mockRepo.On("VerifyPassword", ctx, userID, oldPassword).Return(nil).Once()
		mockRepo.On("UpdatePassword", ctx, userID, mock.AnythingOfType("string")).Return(nil).Once()
		mockRepo.On("InvalidateAllUserRefreshTokens", ctx, userID).Return(nil).Once()

		// Call the service method
		err := service.UpdatePassword(ctx, userID, oldPassword, newPassword)

		// Assert expectations
		assert.NoError(t, err)
		mockRepo.AssertExpectations(t)
	})

	// Test case: incorrect old password
	t.Run("IncorrectOldPassword", func(t *testing.T) {
		ctx := context.Background()
		userID := "user123"
		oldPassword := "wrongpassword"
		newPassword := "newpassword"
		expectedError := types.ErrUnauthenticated

		// Set up expectations
		mockRepo.On("VerifyPassword", ctx, userID, oldPassword).Return(expectedError).Once()

		// Call the service method
		err := service.UpdatePassword(ctx, userID, oldPassword, newPassword)

		// Assert expectations
		assert.Error(t, err)
		assert.ErrorIs(t, err, expectedError)
		mockRepo.AssertExpectations(t)
	})

	// Test case: error updating password
	t.Run("ErrorUpdatingPassword", func(t *testing.T) {
		ctx := context.Background()
		userID := "user123"
		oldPassword := "oldpassword"
		newPassword := "newpassword"
		expectedError := errors.New("database error")

		// Set up expectations
		mockRepo.On("VerifyPassword", ctx, userID, oldPassword).Return(nil).Once()
		mockRepo.On("UpdatePassword", ctx, userID, mock.AnythingOfType("string")).Return(expectedError).Once()

		// Call the service method
		err := service.UpdatePassword(ctx, userID, oldPassword, newPassword)

		// Assert expectations
		assert.Error(t, err)
		assert.Contains(t, err.Error(), expectedError.Error())
		mockRepo.AssertExpectations(t)
	})

	// Test case: error invalidating refresh tokens
	t.Run("ErrorInvalidatingTokens", func(t *testing.T) {
		ctx := context.Background()
		userID := "user123"
		oldPassword := "oldpassword"
		newPassword := "newpassword"
		expectedError := errors.New("database error")

		// Set up expectations
		mockRepo.On("VerifyPassword", ctx, userID, oldPassword).Return(nil).Once()
		mockRepo.On("UpdatePassword", ctx, userID, mock.AnythingOfType("string")).Return(nil).Once()
		mockRepo.On("InvalidateAllUserRefreshTokens", ctx, userID).Return(expectedError).Once()

		// Call the service method
		err := service.UpdatePassword(ctx, userID, oldPassword, newPassword)

		// Assert expectations
		assert.Error(t, err)
		assert.Contains(t, err.Error(), expectedError.Error())
		mockRepo.AssertExpectations(t)
	})
}

func TestInvalidateAllUserRefreshTokens(t *testing.T) {
	// Create a mock repository
	mockRepo := new(MockAuthRepo)
	logger := slog.Default()
	cfg := &config.Config{
		JWT: config.JWTConfig{
			SecretKey:       "test-access-secret",
			AccessTokenTTL:  15 * time.Minute,
			RefreshTokenTTL: 7 * 24 * time.Hour,
			Issuer:          "test-issuer",
			Audience:        "test-audience",
		},
	}
	service := NewAuthService(mockRepo, cfg, logger)

	// Test case: successful invalidation
	t.Run("Success", func(t *testing.T) {
		ctx := context.Background()
		userID := "user123"

		// Set up expectations
		mockRepo.On("InvalidateAllUserRefreshTokens", ctx, userID).Return(nil).Once()

		// Call the service method
		err := service.InvalidateAllUserRefreshTokens(ctx, userID)

		// Assert expectations
		assert.NoError(t, err)
		mockRepo.AssertExpectations(t)
	})

	// Test case: error invalidating tokens
	t.Run("Error", func(t *testing.T) {
		ctx := context.Background()
		userID := "user123"
		expectedError := errors.New("database error")

		// Set up expectations
		mockRepo.On("InvalidateAllUserRefreshTokens", ctx, userID).Return(expectedError).Once()

		// Call the service method
		err := service.InvalidateAllUserRefreshTokens(ctx, userID)

		// Assert expectations
		assert.Error(t, err)
		assert.Contains(t, err.Error(), expectedError.Error())
		mockRepo.AssertExpectations(t)
	})
}

func TestGetUserByID(t *testing.T) {
	// Create a mock repository
	mockRepo := new(MockAuthRepo)
	logger := slog.Default()
	cfg := &config.Config{
		JWT: config.JWTConfig{
			SecretKey:       "test-access-secret",
			AccessTokenTTL:  15 * time.Minute,
			RefreshTokenTTL: 7 * 24 * time.Hour,
			Issuer:          "test-issuer",
			Audience:        "test-audience",
		},
	}
	service := NewAuthService(mockRepo, cfg, logger)

	// Test case: successful retrieval
	t.Run("Success", func(t *testing.T) {
		ctx := context.Background()
		userID := "user123"

		expectedUser := &types.UserAuth{
			ID:       userID,
			Username: "testuser",
			Email:    "test@example.com",
			Role:     "user",
		}

		// Set up expectations
		mockRepo.On("GetUserByID", ctx, userID).Return(expectedUser, nil).Once()

		// Call the service method
		user, err := service.GetUserByID(ctx, userID)

		// Assert expectations
		assert.NoError(t, err)
		assert.Equal(t, expectedUser, user)
		mockRepo.AssertExpectations(t)
	})

	// Test case: user not found
	t.Run("UserNotFound", func(t *testing.T) {
		ctx := context.Background()
		userID := "nonexistent-user"

		// Set up expectations
		mockRepo.On("GetUserByID", ctx, userID).Return(nil, types.ErrNotFound).Once()

		// Call the service method
		user, err := service.GetUserByID(ctx, userID)

		// Assert expectations
		assert.Error(t, err)
		assert.Nil(t, user)
		assert.ErrorIs(t, err, types.ErrNotFound)
		mockRepo.AssertExpectations(t)
	})

	// Test case: database error
	t.Run("DatabaseError", func(t *testing.T) {
		ctx := context.Background()
		userID := "user123"
		expectedError := errors.New("database error")

		// Set up expectations
		mockRepo.On("GetUserByID", ctx, userID).Return(nil, expectedError).Once()

		// Call the service method
		user, err := service.GetUserByID(ctx, userID)

		// Assert expectations
		assert.Error(t, err)
		assert.Nil(t, user)
		assert.Contains(t, err.Error(), expectedError.Error())
		mockRepo.AssertExpectations(t)
	})
}

func TestVerifyPassword(t *testing.T) {
	// Create a mock repository
	mockRepo := new(MockAuthRepo)
	logger := slog.Default()
	cfg := &config.Config{
		JWT: config.JWTConfig{
			SecretKey:       "test-access-secret",
			AccessTokenTTL:  15 * time.Minute,
			RefreshTokenTTL: 7 * 24 * time.Hour,
			Issuer:          "test-issuer",
			Audience:        "test-audience",
		},
	}
	service := NewAuthService(mockRepo, cfg, logger)

	// Test case: successful verification
	t.Run("Success", func(t *testing.T) {
		ctx := context.Background()
		userID := "user123"
		password := "correct-password"

		// Set up expectations
		mockRepo.On("VerifyPassword", ctx, userID, password).Return(nil).Once()

		// Call the service method
		err := service.VerifyPassword(ctx, userID, password)

		// Assert expectations
		assert.NoError(t, err)
		mockRepo.AssertExpectations(t)
	})

	// Test case: incorrect password
	t.Run("IncorrectPassword", func(t *testing.T) {
		ctx := context.Background()
		userID := "user123"
		password := "wrong-password"
		expectedError := types.ErrUnauthenticated

		// Set up expectations
		mockRepo.On("VerifyPassword", ctx, userID, password).Return(expectedError).Once()

		// Call the service method
		err := service.VerifyPassword(ctx, userID, password)

		// Assert expectations
		assert.Error(t, err)
		assert.ErrorIs(t, err, expectedError)
		mockRepo.AssertExpectations(t)
	})

	// Test case: database error
	t.Run("DatabaseError", func(t *testing.T) {
		ctx := context.Background()
		userID := "user123"
		password := "password"
		expectedError := errors.New("database error")

		// Set up expectations
		mockRepo.On("VerifyPassword", ctx, userID, password).Return(expectedError).Once()

		// Call the service method
		err := service.VerifyPassword(ctx, userID, password)

		// Assert expectations
		assert.Error(t, err)
		assert.Contains(t, err.Error(), expectedError.Error())
		mockRepo.AssertExpectations(t)
	})
}

func TestValidateRefreshToken(t *testing.T) {
	// Create a mock repository
	mockRepo := new(MockAuthRepo)
	logger := slog.Default()
	cfg := &config.Config{
		JWT: config.JWTConfig{
			SecretKey:       "test-access-secret",
			AccessTokenTTL:  15 * time.Minute,
			RefreshTokenTTL: 7 * 24 * time.Hour,
			Issuer:          "test-issuer",
			Audience:        "test-audience",
		},
	}
	service := NewAuthService(mockRepo, cfg, logger)

	// Test case: successful validation
	t.Run("Success", func(t *testing.T) {
		ctx := context.Background()
		refreshToken := "valid-refresh-token"
		userID := "user123"

		// Set up expectations
		mockRepo.On("ValidateRefreshTokenAndGetUserID", ctx, refreshToken).Return(userID, nil).Once()

		// Call the service method
		resultUserID, err := service.ValidateRefreshToken(ctx, refreshToken)

		// Assert expectations
		assert.NoError(t, err)
		assert.Equal(t, userID, resultUserID)
		mockRepo.AssertExpectations(t)
	})

	// Test case: invalid token
	t.Run("InvalidToken", func(t *testing.T) {
		ctx := context.Background()
		refreshToken := "invalid-refresh-token"
		expectedError := types.ErrUnauthenticated

		// Set up expectations
		mockRepo.On("ValidateRefreshTokenAndGetUserID", ctx, refreshToken).Return("", expectedError).Once()

		// Call the service method
		userID, err := service.ValidateRefreshToken(ctx, refreshToken)

		// Assert expectations
		assert.Error(t, err)
		assert.Empty(t, userID)
		assert.ErrorIs(t, err, expectedError)
		mockRepo.AssertExpectations(t)
	})

	// Test case: database error
	t.Run("DatabaseError", func(t *testing.T) {
		ctx := context.Background()
		refreshToken := "valid-refresh-token"
		expectedError := errors.New("database error")

		// Set up expectations
		mockRepo.On("ValidateRefreshTokenAndGetUserID", ctx, refreshToken).Return("", expectedError).Once()

		// Call the service method
		userID, err := service.ValidateRefreshToken(ctx, refreshToken)

		// Assert expectations
		assert.Error(t, err)
		assert.Empty(t, userID)
		assert.Contains(t, err.Error(), expectedError.Error())
		mockRepo.AssertExpectations(t)
	})
}
