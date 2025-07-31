package auth

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"golang.org/x/crypto/bcrypt"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "github.com/FACorreiaa/loci-proto/modules/auth/generated"

	"github.com/FACorreiaa/go-poi-au-suggestions/app/observability/metrics"
	"github.com/FACorreiaa/go-poi-au-suggestions/internal/types"
)

// MockAuthRepository implements the AuthRepository interface
type MockAuthRepository struct {
	mock.Mock
}

func (m *MockAuthRepository) Register(ctx context.Context, username, email, hashedPassword string) (string, error) {
	args := m.Called(ctx, username, email, hashedPassword)
	return args.String(0), args.Error(1)
}

func (m *MockAuthRepository) GetUserByEmail(ctx context.Context, email string) (*UserAuth, error) {
	args := m.Called(ctx, email)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*UserAuth), args.Error(1)
}

func (m *MockAuthRepository) GetUserByID(ctx context.Context, userID string) (*UserAuth, error) {
	args := m.Called(ctx, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*UserAuth), args.Error(1)
}

func (m *MockAuthRepository) CreateUser(ctx context.Context, user *UserAuth) error {
	args := m.Called(ctx, user)
	return args.Error(0)
}

func (m *MockAuthRepository) CreateUserProvider(ctx context.Context, userID, provider, providerUserID string) error {
	args := m.Called(ctx, userID, provider, providerUserID)
	return args.Error(0)
}

func (m *MockAuthRepository) GetUserIDByProvider(ctx context.Context, provider, providerUserID string) (string, error) {
	args := m.Called(ctx, provider, providerUserID)
	return args.String(0), args.Error(1)
}

func (m *MockAuthRepository) UpdatePassword(ctx context.Context, userID, newHashedPassword string) error {
	args := m.Called(ctx, userID, newHashedPassword)
	return args.Error(0)
}

func (m *MockAuthRepository) InvalidateAllUserRefreshTokens(ctx context.Context, userID string) error {
	args := m.Called(ctx, userID)
	return args.Error(0)
}

func (m *MockAuthRepository) VerifyPassword(ctx context.Context, userID, password string) error {
	args := m.Called(ctx, userID, password)
	return args.Error(0)
}

func (m *MockAuthRepository) StoreRefreshToken(ctx context.Context, userID, token string, expiresAt time.Time) error {
	args := m.Called(ctx, userID, token, expiresAt)
	return args.Error(0)
}

func (m *MockAuthRepository) ValidateRefreshTokenAndGetUserID(ctx context.Context, refreshToken string) (string, error) {
	args := m.Called(ctx, refreshToken)
	return args.String(0), args.Error(1)
}

func (m *MockAuthRepository) InvalidateRefreshToken(ctx context.Context, refreshToken string) error {
	args := m.Called(ctx, refreshToken)
	return args.Error(0)
}

func createTestService(t *testing.T, mockRepo *MockAuthRepository) *Service {
	// Initialize metrics for testing
	metrics.InitAppMetrics()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	// Create a mock pgxpool.Pool - for this test we don't actually use it
	var pgpool *pgxpool.Pool = nil

	return NewService(context.Background(), mockRepo, pgpool, logger)
}

func TestGRPCService_Register(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mockRepo := new(MockAuthRepository)
		service := createTestService(t, mockRepo)

		userID := "test-user-id"
		req := &pb.RegisterRequest{
			Username: "testuser",
			Email:    "test@example.com",
			Password: "password123",
		}

		// Mock expectations
		mockRepo.On("Register", mock.Anything, req.Username, req.Email, mock.AnythingOfType("string")).
			Return(userID, nil).Once()

		// Call the gRPC method
		ctx := context.Background()
		resp, err := service.Register(ctx, req)

		// Assertions
		assert.NoError(t, err)
		assert.NotNil(t, resp)
		assert.True(t, resp.Success)
		assert.Equal(t, "Registration successful", resp.Message)
		mockRepo.AssertExpectations(t)
	})

	t.Run("ValidationError_EmptyUsername", func(t *testing.T) {
		mockRepo := new(MockAuthRepository)
		service := createTestService(t, mockRepo)

		req := &pb.RegisterRequest{
			Username: "", // Empty username
			Email:    "test@example.com",
			Password: "password123",
		}

		ctx := context.Background()
		resp, err := service.Register(ctx, req)

		// Assertions
		assert.Error(t, err)
		assert.Nil(t, resp)

		grpcErr, ok := status.FromError(err)
		assert.True(t, ok)
		assert.Equal(t, codes.InvalidArgument, grpcErr.Code())
		assert.Contains(t, grpcErr.Message(), "username, email, and password are required")
		mockRepo.AssertExpectations(t)
	})

	t.Run("ValidationError_EmptyEmail", func(t *testing.T) {
		mockRepo := new(MockAuthRepository)
		service := createTestService(t, mockRepo)

		req := &pb.RegisterRequest{
			Username: "testuser",
			Email:    "", // Empty email
			Password: "password123",
		}

		ctx := context.Background()
		resp, err := service.Register(ctx, req)

		// Assertions
		assert.Error(t, err)
		assert.Nil(t, resp)

		grpcErr, ok := status.FromError(err)
		assert.True(t, ok)
		assert.Equal(t, codes.InvalidArgument, grpcErr.Code())
		mockRepo.AssertExpectations(t)
	})

	t.Run("UserAlreadyExists", func(t *testing.T) {
		mockRepo := new(MockAuthRepository)
		service := createTestService(t, mockRepo)

		req := &pb.RegisterRequest{
			Username: "existing",
			Email:    "existing@example.com",
			Password: "password123",
		}

		// Mock expectations - user already exists
		mockRepo.On("Register", mock.Anything, req.Username, req.Email, mock.AnythingOfType("string")).
			Return("", types.ErrConflict).Once()

		ctx := context.Background()
		resp, err := service.Register(ctx, req)

		// Assertions
		assert.Error(t, err)
		assert.Nil(t, resp)

		grpcErr, ok := status.FromError(err)
		assert.True(t, ok)
		assert.Equal(t, codes.AlreadyExists, grpcErr.Code())
		assert.Contains(t, grpcErr.Message(), "user with this email already exists")
		mockRepo.AssertExpectations(t)
	})

	t.Run("InternalError", func(t *testing.T) {
		mockRepo := new(MockAuthRepository)
		service := createTestService(t, mockRepo)

		req := &pb.RegisterRequest{
			Username: "testuser",
			Email:    "test@example.com",
			Password: "password123",
		}

		// Mock expectations - internal error
		mockRepo.On("Register", mock.Anything, req.Username, req.Email, mock.AnythingOfType("string")).
			Return("", errors.New("database error")).Once()

		ctx := context.Background()
		resp, err := service.Register(ctx, req)

		// Assertions
		assert.Error(t, err)
		assert.Nil(t, resp)

		grpcErr, ok := status.FromError(err)
		assert.True(t, ok)
		assert.Equal(t, codes.Internal, grpcErr.Code())
		assert.Contains(t, grpcErr.Message(), "registration failed")
		mockRepo.AssertExpectations(t)
	})
}

func TestGRPCService_Login(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mockRepo := new(MockAuthRepository)
		service := createTestService(t, mockRepo)

		password := "password123"
		hashedPassword, _ := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)

		user := &UserAuth{
			ID:       "user123",
			Username: "testuser",
			Email:    "test@example.com",
			Password: string(hashedPassword),
			Role:     "user",
		}

		req := &pb.LoginRequest{
			Email:    "test@example.com",
			Password: password,
		}

		// Mock expectations
		mockRepo.On("GetUserByEmail", mock.Anything, req.Email).Return(user, nil).Once()
		mockRepo.On("VerifyPassword", mock.Anything, user.ID, req.Password).Return(nil).Once()
		mockRepo.On("StoreRefreshToken", mock.Anything, user.ID, mock.AnythingOfType("string"), mock.AnythingOfType("time.Time")).Return(nil).Once()

		ctx := context.Background()
		resp, err := service.Login(ctx, req)

		// Assertions
		assert.NoError(t, err)
		assert.NotNil(t, resp)
		assert.True(t, resp.Success)
		assert.Equal(t, "Login successful", resp.Message)
		assert.NotEmpty(t, resp.AccessToken)
		assert.NotEmpty(t, resp.RefreshToken)
		assert.Equal(t, int64(3600), resp.ExpiresIn) // 1 hour
		mockRepo.AssertExpectations(t)
	})

	t.Run("ValidationError_EmptyEmail", func(t *testing.T) {
		mockRepo := new(MockAuthRepository)
		service := createTestService(t, mockRepo)

		req := &pb.LoginRequest{
			Email:    "", // Empty email
			Password: "password123",
		}

		ctx := context.Background()
		resp, err := service.Login(ctx, req)

		// Assertions
		assert.Error(t, err)
		assert.Nil(t, resp)

		grpcErr, ok := status.FromError(err)
		assert.True(t, ok)
		assert.Equal(t, codes.InvalidArgument, grpcErr.Code())
		assert.Contains(t, grpcErr.Message(), "email and password are required")
		mockRepo.AssertExpectations(t)
	})

	t.Run("UserNotFound", func(t *testing.T) {
		mockRepo := new(MockAuthRepository)
		service := createTestService(t, mockRepo)

		req := &pb.LoginRequest{
			Email:    "nonexistent@example.com",
			Password: "password123",
		}

		// Mock expectations
		mockRepo.On("GetUserByEmail", mock.Anything, req.Email).Return(nil, types.ErrNotFound).Once()

		ctx := context.Background()
		resp, err := service.Login(ctx, req)

		// Assertions
		assert.Error(t, err)
		assert.Nil(t, resp)

		grpcErr, ok := status.FromError(err)
		assert.True(t, ok)
		assert.Equal(t, codes.NotFound, grpcErr.Code())
		assert.Contains(t, grpcErr.Message(), "user not found")
		mockRepo.AssertExpectations(t)
	})

	t.Run("InvalidPassword", func(t *testing.T) {
		mockRepo := new(MockAuthRepository)
		service := createTestService(t, mockRepo)

		correctPassword := "correct123"
		hashedPassword, _ := bcrypt.GenerateFromPassword([]byte(correctPassword), bcrypt.DefaultCost)

		user := &UserAuth{
			ID:       "user123",
			Username: "testuser",
			Email:    "test@example.com",
			Password: string(hashedPassword),
			Role:     "user",
		}

		req := &pb.LoginRequest{
			Email:    "test@example.com",
			Password: "wrongpassword", // Wrong password
		}

		// Mock expectations
		mockRepo.On("GetUserByEmail", mock.Anything, req.Email).Return(user, nil).Once()
		mockRepo.On("VerifyPassword", mock.Anything, user.ID, req.Password).Return(types.ErrUnauthenticated).Once()

		ctx := context.Background()
		resp, err := service.Login(ctx, req)

		// Assertions
		assert.Error(t, err)
		assert.Nil(t, resp)

		grpcErr, ok := status.FromError(err)
		assert.True(t, ok)
		assert.Equal(t, codes.Unauthenticated, grpcErr.Code())
		assert.Contains(t, grpcErr.Message(), "invalid credentials")
		mockRepo.AssertExpectations(t)
	})
}

func TestGRPCService_RefreshToken(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mockRepo := new(MockAuthRepository)
		service := createTestService(t, mockRepo)

		userID := "user123"
		user := &UserAuth{
			ID:       userID,
			Username: "testuser",
			Email:    "test@example.com",
			Role:     "user",
		}

		req := &pb.RefreshTokenRequest{
			RefreshToken: "valid-refresh-token",
		}

		// Mock expectations
		mockRepo.On("ValidateRefreshTokenAndGetUserID", mock.Anything, req.RefreshToken).Return(userID, nil).Once()
		mockRepo.On("GetUserByID", mock.Anything, userID).Return(user, nil).Once()
		mockRepo.On("InvalidateRefreshToken", mock.Anything, req.RefreshToken).Return(nil).Once()
		mockRepo.On("StoreRefreshToken", mock.Anything, userID, mock.AnythingOfType("string"), mock.AnythingOfType("time.Time")).Return(nil).Once()

		ctx := context.Background()
		resp, err := service.RefreshToken(ctx, req)

		// Assertions
		assert.NoError(t, err)
		assert.NotNil(t, resp)
		assert.NotEmpty(t, resp.AccessToken)
		assert.NotEmpty(t, resp.RefreshToken)
		assert.Equal(t, int64(3600), resp.ExpiresIn)
		mockRepo.AssertExpectations(t)
	})

	t.Run("ValidationError_EmptyToken", func(t *testing.T) {
		mockRepo := new(MockAuthRepository)
		service := createTestService(t, mockRepo)

		req := &pb.RefreshTokenRequest{
			RefreshToken: "", // Empty refresh token
		}

		ctx := context.Background()
		resp, err := service.RefreshToken(ctx, req)

		// Assertions
		assert.Error(t, err)
		assert.Nil(t, resp)

		grpcErr, ok := status.FromError(err)
		assert.True(t, ok)
		assert.Equal(t, codes.InvalidArgument, grpcErr.Code())
		assert.Contains(t, grpcErr.Message(), "refresh token is required")
		mockRepo.AssertExpectations(t)
	})

	t.Run("InvalidRefreshToken", func(t *testing.T) {
		mockRepo := new(MockAuthRepository)
		service := createTestService(t, mockRepo)

		req := &pb.RefreshTokenRequest{
			RefreshToken: "invalid-token",
		}

		// Mock expectations
		mockRepo.On("ValidateRefreshTokenAndGetUserID", mock.Anything, req.RefreshToken).Return("", types.ErrUnauthenticated).Once()

		ctx := context.Background()
		resp, err := service.RefreshToken(ctx, req)

		// Assertions
		assert.Error(t, err)
		assert.Nil(t, resp)

		grpcErr, ok := status.FromError(err)
		assert.True(t, ok)
		assert.Equal(t, codes.Unauthenticated, grpcErr.Code())
		assert.Contains(t, grpcErr.Message(), "invalid or expired refresh token")
		mockRepo.AssertExpectations(t)
	})

	t.Run("UserNotFound", func(t *testing.T) {
		mockRepo := new(MockAuthRepository)
		service := createTestService(t, mockRepo)

		userID := "nonexistent-user"
		req := &pb.RefreshTokenRequest{
			RefreshToken: "valid-token",
		}

		// Mock expectations
		mockRepo.On("ValidateRefreshTokenAndGetUserID", mock.Anything, req.RefreshToken).Return(userID, nil).Once()
		mockRepo.On("GetUserByID", mock.Anything, userID).Return(nil, types.ErrNotFound).Once()

		ctx := context.Background()
		resp, err := service.RefreshToken(ctx, req)

		// Assertions
		assert.Error(t, err)
		assert.Nil(t, resp)

		grpcErr, ok := status.FromError(err)
		assert.True(t, ok)
		assert.Equal(t, codes.NotFound, grpcErr.Code())
		assert.Contains(t, grpcErr.Message(), "user not found")
		mockRepo.AssertExpectations(t)
	})
}

func TestGRPCService_Logout(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mockRepo := new(MockAuthRepository)
		service := createTestService(t, mockRepo)

		userID := "user123"
		req := &pb.LogoutRequest{
			UserId: userID,
		}

		// Mock expectations
		mockRepo.On("InvalidateAllUserRefreshTokens", mock.Anything, userID).Return(nil).Once()

		ctx := context.Background()
		resp, err := service.Logout(ctx, req)

		// Assertions
		assert.NoError(t, err)
		assert.NotNil(t, resp)
		assert.True(t, resp.Success)
		assert.Equal(t, "Logged out successfully", resp.Message)
		mockRepo.AssertExpectations(t)
	})

	t.Run("ValidationError_EmptyUserID", func(t *testing.T) {
		mockRepo := new(MockAuthRepository)
		service := createTestService(t, mockRepo)

		req := &pb.LogoutRequest{
			UserId: "", // Empty user ID
		}

		ctx := context.Background()
		resp, err := service.Logout(ctx, req)

		// Assertions
		assert.Error(t, err)
		assert.Nil(t, resp)

		grpcErr, ok := status.FromError(err)
		assert.True(t, ok)
		assert.Equal(t, codes.InvalidArgument, grpcErr.Code())
		assert.Contains(t, grpcErr.Message(), "user ID is required")
		mockRepo.AssertExpectations(t)
	})

	t.Run("InternalError", func(t *testing.T) {
		mockRepo := new(MockAuthRepository)
		service := createTestService(t, mockRepo)

		userID := "user123"
		req := &pb.LogoutRequest{
			UserId: userID,
		}

		// Mock expectations - database error
		mockRepo.On("InvalidateAllUserRefreshTokens", mock.Anything, userID).Return(errors.New("database error")).Once()

		ctx := context.Background()
		resp, err := service.Logout(ctx, req)

		// Assertions
		assert.Error(t, err)
		assert.Nil(t, resp)

		grpcErr, ok := status.FromError(err)
		assert.True(t, ok)
		assert.Equal(t, codes.Internal, grpcErr.Code())
		assert.Contains(t, grpcErr.Message(), "logout failed")
		mockRepo.AssertExpectations(t)
	})
}

func TestGRPCService_ValidateSession(t *testing.T) {
	t.Run("ValidationError_EmptyToken", func(t *testing.T) {
		mockRepo := new(MockAuthRepository)
		service := createTestService(t, mockRepo)

		req := &pb.ValidateSessionRequest{
			AccessToken: "", // Empty access token
		}

		ctx := context.Background()
		resp, err := service.ValidateSession(ctx, req)

		// Assertions
		assert.Error(t, err)
		assert.Nil(t, resp)

		grpcErr, ok := status.FromError(err)
		assert.True(t, ok)
		assert.Equal(t, codes.InvalidArgument, grpcErr.Code())
		assert.Contains(t, grpcErr.Message(), "access token is required")
		mockRepo.AssertExpectations(t)
	})

	t.Run("InvalidToken", func(t *testing.T) {
		mockRepo := new(MockAuthRepository)
		service := createTestService(t, mockRepo)

		req := &pb.ValidateSessionRequest{
			AccessToken: "invalid.jwt.token",
		}

		ctx := context.Background()
		resp, err := service.ValidateSession(ctx, req)

		// Since we're using an invalid JWT token, it should return a valid response
		// but with Valid: false (as per the current service implementation)
		assert.NoError(t, err)
		assert.NotNil(t, resp)
		assert.False(t, resp.Valid)
		mockRepo.AssertExpectations(t)
	})

	// TODO: Add test for valid JWT token - requires proper JWT setup with JwtSecretKey
}
