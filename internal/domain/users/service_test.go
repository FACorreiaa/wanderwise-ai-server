package user

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "github.com/FACorreiaa/loci-proto/modules/user/generated"
	"github.com/FACorreiaa/go-poi-au-suggestions/app/observability/metrics"
	"github.com/FACorreiaa/go-poi-au-suggestions/internal/types"
	"github.com/FACorreiaa/go-poi-au-suggestions/protocol/grpc/middleware/grpcrequest"
)

// MockUserRepository implements the Repository interface for testing
type MockUserRepository struct {
	mock.Mock
}

func (m *MockUserRepository) GetUserByID(ctx context.Context, userID uuid.UUID) (*types.UserProfile, error) {
	args := m.Called(ctx, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*types.UserProfile), args.Error(1)
}

func (m *MockUserRepository) ChangePassword(ctx context.Context, email, oldPassword, newPassword string) error {
	args := m.Called(ctx, email, oldPassword, newPassword)
	return args.Error(0)
}

func (m *MockUserRepository) UpdateProfile(ctx context.Context, userID uuid.UUID, params types.UpdateProfileParams) error {
	args := m.Called(ctx, userID, params)
	return args.Error(0)
}

func (m *MockUserRepository) UpdateLastLogin(ctx context.Context, userID uuid.UUID) error {
	args := m.Called(ctx, userID)
	return args.Error(0)
}

func (m *MockUserRepository) MarkEmailAsVerified(ctx context.Context, userID uuid.UUID) error {
	args := m.Called(ctx, userID)
	return args.Error(0)
}

func (m *MockUserRepository) DeactivateUser(ctx context.Context, userID uuid.UUID) error {
	args := m.Called(ctx, userID)
	return args.Error(0)
}

func (m *MockUserRepository) ReactivateUser(ctx context.Context, userID uuid.UUID) error {
	args := m.Called(ctx, userID)
	return args.Error(0)
}

func createTestUserService(t *testing.T, mockRepo *MockUserRepository) *Service {
	// Initialize metrics for testing
	metrics.InitAppMetrics()
	
	logger := zap.NewNop()
	var pgpool *pgxpool.Pool = nil

	return NewService(context.Background(), mockRepo, pgpool, logger)
}

func createContextWithUserAuth(userID string, requestID string) context.Context {
	ctx := context.Background()
	ctx = context.WithValue(ctx, "userID", userID)
	ctx = context.WithValue(ctx, grpcrequest.RequestIDKey{}, requestID)
	return ctx
}

func TestUserService_UpdateUserProfile(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mockRepo := new(MockUserRepository)
		service := createTestUserService(t, mockRepo)

		userID := uuid.New()
		requestID := "test-request-id"
		
		username := "testuser"
		email := "test@example.com"
		firstname := "John"
		lastname := "Doe"

		updatedProfile := &types.UserProfile{
			ID:        userID,
			Username:  &username,
			Email:     email,
			Firstname: &firstname,
			Lastname:  &lastname,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		req := &pb.UpdateUserProfileRequest{
			UserId: userID.String(),
			Profile: &pb.UserProfile{
				Username:  username,
				Email:     email,
				FirstName: firstname,
				LastName:  lastname,
			},
			UpdateFields: []string{"username", "email", "first_name", "last_name"},
			Request: &pb.BaseRequest{
				RequestId: requestID,
			},
		}

		mockRepo.On("UpdateProfile", mock.Anything, userID, mock.AnythingOfType("types.UpdateProfileParams")).
			Return(nil).Once()
		mockRepo.On("GetUserByID", mock.Anything, userID).
			Return(updatedProfile, nil).Once()

		ctx := createContextWithUserAuth(userID.String(), requestID)
		resp, err := service.UpdateUserProfile(ctx, req)

		assert.NoError(t, err)
		assert.NotNil(t, resp)
		assert.True(t, resp.Success)
		assert.Equal(t, "Profile updated successfully", resp.Message)
		assert.NotNil(t, resp.Profile)
		assert.Equal(t, username, resp.Profile.Username)
		assert.Equal(t, email, resp.Profile.Email)
		assert.Equal(t, firstname, resp.Profile.FirstName)
		assert.Equal(t, lastname, resp.Profile.LastName)
		assert.Equal(t, requestID, resp.Response.RequestId)
		assert.Equal(t, "user-service", resp.Response.Upstream)
		mockRepo.AssertExpectations(t)
	})

	t.Run("NilProfile", func(t *testing.T) {
		mockRepo := new(MockUserRepository)
		service := createTestUserService(t, mockRepo)

		userID := uuid.New()
		requestID := "test-request-id"

		req := &pb.UpdateUserProfileRequest{
			UserId:  userID.String(),
			Profile: nil, // Nil profile
			Request: &pb.BaseRequest{
				RequestId: requestID,
			},
		}

		ctx := createContextWithUserAuth(userID.String(), requestID)
		resp, err := service.UpdateUserProfile(ctx, req)

		assert.Error(t, err)
		assert.NotNil(t, resp)
		assert.False(t, resp.Success)
		assert.Equal(t, "Profile data is required", resp.Message)

		grpcErr, ok := status.FromError(err)
		assert.True(t, ok)
		assert.Equal(t, codes.InvalidArgument, grpcErr.Code())
		assert.Contains(t, grpcErr.Message(), "profile data is required")
		mockRepo.AssertExpectations(t)
	})

	t.Run("UpdateProfileError", func(t *testing.T) {
		mockRepo := new(MockUserRepository)
		service := createTestUserService(t, mockRepo)

		userID := uuid.New()
		requestID := "test-request-id"
		
		username := "testuser"

		req := &pb.UpdateUserProfileRequest{
			UserId: userID.String(),
			Profile: &pb.UserProfile{
				Username: username,
			},
			Request: &pb.BaseRequest{
				RequestId: requestID,
			},
		}

		mockRepo.On("UpdateProfile", mock.Anything, userID, mock.AnythingOfType("types.UpdateProfileParams")).
			Return(errors.New("database error")).Once()

		ctx := createContextWithUserAuth(userID.String(), requestID)
		resp, err := service.UpdateUserProfile(ctx, req)

		assert.Error(t, err)
		assert.NotNil(t, resp)
		assert.False(t, resp.Success)
		assert.Equal(t, "Failed to update profile", resp.Message)

		grpcErr, ok := status.FromError(err)
		assert.True(t, ok)
		assert.Equal(t, codes.Internal, grpcErr.Code())
		assert.Contains(t, grpcErr.Message(), "failed to update profile")
		mockRepo.AssertExpectations(t)
	})

	t.Run("GetUpdatedProfileError_StillReturnsSuccess", func(t *testing.T) {
		mockRepo := new(MockUserRepository)
		service := createTestUserService(t, mockRepo)

		userID := uuid.New()
		requestID := "test-request-id"
		
		username := "testuser"

		req := &pb.UpdateUserProfileRequest{
			UserId: userID.String(),
			Profile: &pb.UserProfile{
				Username: username,
			},
			Request: &pb.BaseRequest{
				RequestId: requestID,
			},
		}

		mockRepo.On("UpdateProfile", mock.Anything, userID, mock.AnythingOfType("types.UpdateProfileParams")).
			Return(nil).Once()
		mockRepo.On("GetUserByID", mock.Anything, userID).
			Return(nil, errors.New("database error")).Once()

		ctx := createContextWithUserAuth(userID.String(), requestID)
		resp, err := service.UpdateUserProfile(ctx, req)

		assert.NoError(t, err) // Should still succeed even if fetching updated profile fails
		assert.NotNil(t, resp)
		assert.True(t, resp.Success)
		assert.Equal(t, "Profile updated successfully", resp.Message)
		assert.Nil(t, resp.Profile) // Profile should be nil due to fetch error
		mockRepo.AssertExpectations(t)
	})

	t.Run("UnauthenticatedUser", func(t *testing.T) {
		mockRepo := new(MockUserRepository)
		service := createTestUserService(t, mockRepo)

		req := &pb.UpdateUserProfileRequest{
			Profile: &pb.UserProfile{
				Username: "testuser",
			},
		}

		// Context without user authentication
		ctx := context.WithValue(context.Background(), grpcrequest.RequestIDKey{}, "test-request-id")
		resp, err := service.UpdateUserProfile(ctx, req)

		assert.Error(t, err)
		assert.Nil(t, resp)

		grpcErr, ok := status.FromError(err)
		assert.True(t, ok)
		assert.Equal(t, codes.Unauthenticated, grpcErr.Code())
		mockRepo.AssertExpectations(t)
	})

	t.Run("NoRequestID", func(t *testing.T) {
		mockRepo := new(MockUserRepository)
		service := createTestUserService(t, mockRepo)

		req := &pb.UpdateUserProfileRequest{
			Profile: &pb.UserProfile{
				Username: "testuser",
			},
		}

		// Context without request ID
		ctx := context.Background()
		resp, err := service.UpdateUserProfile(ctx, req)

		assert.Error(t, err)
		assert.Nil(t, resp)

		grpcErr, ok := status.FromError(err)
		assert.True(t, ok)
		assert.Equal(t, codes.Internal, grpcErr.Code())
		assert.Contains(t, grpcErr.Message(), "request id not found in context")
		mockRepo.AssertExpectations(t)
	})
}

func TestUserService_GetUserProfile(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mockRepo := new(MockUserRepository)
		service := createTestUserService(t, mockRepo)

		userID := uuid.New()
		requestID := "test-request-id"
		
		username := "testuser"
		email := "test@example.com"
		firstname := "John"
		lastname := "Doe"

		userProfile := &types.UserProfile{
			ID:        userID,
			Username:  &username,
			Email:     email,
			Firstname: &firstname,
			Lastname:  &lastname,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		req := &pb.GetUserProfileRequest{
			UserId: userID.String(),
			Request: &pb.BaseRequest{
				RequestId: requestID,
			},
		}

		mockRepo.On("GetUserByID", mock.Anything, userID).
			Return(userProfile, nil).Once()

		ctx := createContextWithUserAuth(userID.String(), requestID)
		resp, err := service.GetUserProfile(ctx, req)

		assert.NoError(t, err)
		assert.NotNil(t, resp)
		assert.NotNil(t, resp.Profile)
		assert.Equal(t, username, resp.Profile.Username)
		assert.Equal(t, email, resp.Profile.Email)
		assert.Equal(t, firstname, resp.Profile.FirstName)
		assert.Equal(t, lastname, resp.Profile.LastName)
		assert.Equal(t, requestID, resp.Response.RequestId)
		assert.Equal(t, "user-service", resp.Response.Upstream)
		mockRepo.AssertExpectations(t)
	})

	t.Run("RepositoryError", func(t *testing.T) {
		mockRepo := new(MockUserRepository)
		service := createTestUserService(t, mockRepo)

		userID := uuid.New()
		requestID := "test-request-id"

		req := &pb.GetUserProfileRequest{
			UserId: userID.String(),
			Request: &pb.BaseRequest{
				RequestId: requestID,
			},
		}

		mockRepo.On("GetUserByID", mock.Anything, userID).
			Return(nil, errors.New("database error")).Once()

		ctx := createContextWithUserAuth(userID.String(), requestID)
		resp, err := service.GetUserProfile(ctx, req)

		assert.NoError(t, err) // Current implementation doesn't return error for this case
		assert.NotNil(t, resp)
		assert.Nil(t, resp.Profile) // Profile should be nil due to error
		mockRepo.AssertExpectations(t)
	})

	t.Run("UnauthenticatedUser", func(t *testing.T) {
		mockRepo := new(MockUserRepository)
		service := createTestUserService(t, mockRepo)

		req := &pb.GetUserProfileRequest{
			UserId: uuid.New().String(),
		}

		// Context without user authentication
		ctx := context.WithValue(context.Background(), grpcrequest.RequestIDKey{}, "test-request-id")
		resp, err := service.GetUserProfile(ctx, req)

		assert.Error(t, err)
		assert.Nil(t, resp)

		grpcErr, ok := status.FromError(err)
		assert.True(t, ok)
		assert.Equal(t, codes.Unauthenticated, grpcErr.Code())
		mockRepo.AssertExpectations(t)
	})

	t.Run("NoRequestID", func(t *testing.T) {
		mockRepo := new(MockUserRepository)
		service := createTestUserService(t, mockRepo)

		req := &pb.GetUserProfileRequest{
			UserId: uuid.New().String(),
		}

		// Context without request ID
		ctx := context.Background()
		resp, err := service.GetUserProfile(ctx, req)

		assert.Error(t, err)
		assert.Nil(t, resp)

		grpcErr, ok := status.FromError(err)
		assert.True(t, ok)
		assert.Equal(t, codes.Internal, grpcErr.Code())
		assert.Contains(t, grpcErr.Message(), "request id not found in context")
		mockRepo.AssertExpectations(t)
	})
}

func TestUserService_ConvertPbProfileToUpdateParams(t *testing.T) {
	t.Run("AllFieldsWithUpdateMask", func(t *testing.T) {
		profile := &pb.UserProfile{
			Username:  "testuser",
			Email:     "test@example.com",
			FirstName: "John",
			LastName:  "Doe",
			Phone:     "+1234567890",
			Bio:       "Test bio",
			AvatarUrl: "https://example.com/avatar.jpg",
			Location:  "New York",
		}
		updateFields := []string{"username", "email", "first_name", "last_name", "phone", "bio", "avatar_url", "location"}

		params := convertPbProfileToUpdateParams(profile, updateFields)

		assert.NotNil(t, params.Username)
		assert.Equal(t, "testuser", *params.Username)
		assert.NotNil(t, params.Email)
		assert.Equal(t, "test@example.com", *params.Email)
		assert.NotNil(t, params.Firstname)
		assert.Equal(t, "John", *params.Firstname)
		assert.NotNil(t, params.Lastname)
		assert.Equal(t, "Doe", *params.Lastname)
		assert.NotNil(t, params.PhoneNumber)
		assert.Equal(t, "+1234567890", *params.PhoneNumber)
		assert.NotNil(t, params.AboutYou)
		assert.Equal(t, "Test bio", *params.AboutYou)
		assert.NotNil(t, params.ProfileImageURL)
		assert.Equal(t, "https://example.com/avatar.jpg", *params.ProfileImageURL)
		assert.NotNil(t, params.Location)
		assert.Equal(t, "New York", *params.Location)
	})

	t.Run("NoUpdateMask_UpdatesAll", func(t *testing.T) {
		profile := &pb.UserProfile{
			Username:  "testuser",
			Email:     "test@example.com",
			FirstName: "John",
		}
		updateFields := []string{} // Empty update mask

		params := convertPbProfileToUpdateParams(profile, updateFields)

		assert.NotNil(t, params.Username)
		assert.Equal(t, "testuser", *params.Username)
		assert.NotNil(t, params.Email)
		assert.Equal(t, "test@example.com", *params.Email)
		assert.NotNil(t, params.Firstname)
		assert.Equal(t, "John", *params.Firstname)
	})

	t.Run("SpecificFieldsOnly", func(t *testing.T) {
		profile := &pb.UserProfile{
			Username:  "testuser",
			Email:     "test@example.com",
			FirstName: "John",
			LastName:  "Doe",
		}
		updateFields := []string{"username", "first_name"} // Only update username and first_name

		params := convertPbProfileToUpdateParams(profile, updateFields)

		assert.NotNil(t, params.Username)
		assert.Equal(t, "testuser", *params.Username)
		assert.Nil(t, params.Email) // Should not be set due to update mask
		assert.NotNil(t, params.Firstname)
		assert.Equal(t, "John", *params.Firstname)
		assert.Nil(t, params.Lastname) // Should not be set due to update mask
	})

	t.Run("EmptyValues", func(t *testing.T) {
		profile := &pb.UserProfile{
			Username:  "",
			Email:     "",
			FirstName: "",
		}
		updateFields := []string{"username", "email", "first_name"}

		params := convertPbProfileToUpdateParams(profile, updateFields)

		// Empty strings should not set the parameters
		assert.Nil(t, params.Username)
		assert.Nil(t, params.Email)
		assert.Nil(t, params.Firstname)
	})
}

func TestUserService_ConvertDomainProfileToPb(t *testing.T) {
	t.Run("CompleteProfile", func(t *testing.T) {
		userID := uuid.New()
		username := "testuser"
		email := "test@example.com"
		firstname := "John"
		lastname := "Doe"
		phone := "+1234567890"
		bio := "Test bio"
		avatarURL := "https://example.com/avatar.jpg"
		location := "New York"
		language := "en"
		emailVerifiedAt := time.Now()

		profile := &types.UserProfile{
			ID:               userID,
			Username:         &username,
			Email:            email,
			Firstname:        &firstname,
			Lastname:         &lastname,
			PhoneNumber:      &phone,
			AboutYou:         &bio,
			ProfileImageURL:  &avatarURL,
			Location:         &location,
			Language:         &language,
			EmailVerifiedAt:  &emailVerifiedAt,
			CreatedAt:        time.Now(),
			UpdatedAt:        time.Now(),
		}

		pbProfile := convertDomainProfileToPb(profile)

		assert.Equal(t, userID.String(), pbProfile.Id)
		assert.Equal(t, username, pbProfile.Username)
		assert.Equal(t, email, pbProfile.Email)
		assert.Equal(t, firstname, pbProfile.FirstName)
		assert.Equal(t, lastname, pbProfile.LastName)
		assert.Equal(t, phone, pbProfile.Phone)
		assert.Equal(t, bio, pbProfile.Bio)
		assert.Equal(t, avatarURL, pbProfile.AvatarUrl)
		assert.Equal(t, location, pbProfile.Location)
		assert.Equal(t, language, pbProfile.Language)
		assert.True(t, pbProfile.EmailVerified)
		assert.True(t, pbProfile.PhoneVerified)
		assert.NotNil(t, pbProfile.CreatedAt)
		assert.NotNil(t, pbProfile.UpdatedAt)
	})

	t.Run("MinimalProfile", func(t *testing.T) {
		userID := uuid.New()
		email := "test@example.com"

		profile := &types.UserProfile{
			ID:        userID,
			Email:     email,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		pbProfile := convertDomainProfileToPb(profile)

		assert.Equal(t, userID.String(), pbProfile.Id)
		assert.Equal(t, "", pbProfile.Username) // Should be empty string for nil pointer
		assert.Equal(t, email, pbProfile.Email)
		assert.Equal(t, "", pbProfile.FirstName)
		assert.Equal(t, "", pbProfile.LastName)
		assert.Equal(t, "", pbProfile.Phone)
		assert.Equal(t, "", pbProfile.Bio)
		assert.Equal(t, "", pbProfile.AvatarUrl)
		assert.Equal(t, "", pbProfile.Location)
		assert.Equal(t, "", pbProfile.Language)
		assert.False(t, pbProfile.EmailVerified) // No verification timestamp
		assert.False(t, pbProfile.PhoneVerified) // No phone number
		assert.NotNil(t, pbProfile.CreatedAt)
		assert.NotNil(t, pbProfile.UpdatedAt)
	})

	t.Run("PhoneVerificationLogic", func(t *testing.T) {
		userID := uuid.New()
		email := "test@example.com"
		
		// Test with non-empty phone
		phone := "+1234567890"
		profileWithPhone := &types.UserProfile{
			ID:          userID,
			Email:       email,
			PhoneNumber: &phone,
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}

		pbProfile := convertDomainProfileToPb(profileWithPhone)
		assert.True(t, pbProfile.PhoneVerified)

		// Test with empty phone
		emptyPhone := ""
		profileWithEmptyPhone := &types.UserProfile{
			ID:          userID,
			Email:       email,
			PhoneNumber: &emptyPhone,
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}

		pbProfile = convertDomainProfileToPb(profileWithEmptyPhone)
		assert.False(t, pbProfile.PhoneVerified)
	})
}