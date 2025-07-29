package profiles

import (
	"context"
	"errors"
	"testing"
	"time"

	pb "github.com/FACorreiaa/loci-proto/modules/profiles/generated"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/FACorreiaa/go-poi-au-suggestions/internal/domain"
)

// MockRepository implements the Repository interface for testing
type MockRepository struct {
	mock.Mock
}

func (m *MockRepository) GetSearchProfiles(ctx context.Context, userID uuid.UUID) ([]UserPreferenceProfileResponse, error) {
	args := m.Called(ctx, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]UserPreferenceProfileResponse), args.Error(1)
}

func (m *MockRepository) GetSearchProfile(ctx context.Context, userID, profileID uuid.UUID) (*UserPreferenceProfileResponse, error) {
	args := m.Called(ctx, userID, profileID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*UserPreferenceProfileResponse), args.Error(1)
}

func (m *MockRepository) GetDefaultSearchProfile(ctx context.Context, userID uuid.UUID) (*UserPreferenceProfileResponse, error) {
	args := m.Called(ctx, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*UserPreferenceProfileResponse), args.Error(1)
}

func (m *MockRepository) CreateSearchProfile(ctx context.Context, userID uuid.UUID, params CreateUserPreferenceProfileParams) (*UserPreferenceProfileResponse, error) {
	args := m.Called(ctx, userID, params)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*UserPreferenceProfileResponse), args.Error(1)
}

func (m *MockRepository) UpdateSearchProfile(ctx context.Context, userID, profileID uuid.UUID, params UpdateSearchProfileParams) error {
	args := m.Called(ctx, userID, profileID, params)
	return args.Error(0)
}

func (m *MockRepository) DeleteSearchProfile(ctx context.Context, userID, profileID uuid.UUID) error {
	args := m.Called(ctx, userID, profileID)
	return args.Error(0)
}

func (m *MockRepository) SetDefaultSearchProfile(ctx context.Context, userID, profileID uuid.UUID) error {
	args := m.Called(ctx, userID, profileID)
	return args.Error(0)
}

func createTestProfilesService(t *testing.T, mockRepo *MockRepository) *Service {
	logger := zap.NewNop()
	
	// Create a mock pgxpool.Pool - for this test we don't actually use it
	var pgpool *pgxpool.Pool = nil

	return NewService(context.Background(), mockRepo, pgpool, logger)
}

func createMockProfile() *UserPreferenceProfileResponse {
	now := time.Now()
	profileID := uuid.New()
	userID := uuid.New()
	
	return &UserPreferenceProfileResponse{
		ID:                   profileID,
		UserID:               userID,
		ProfileName:          "Test Profile",
		IsDefault:            true,
		SearchRadiusKm:       10.0,
		PreferredTime:        DayPreferenceDay,
		BudgetLevel:          3,
		PreferredPace:        SearchPaceModerate,
		PreferAccessiblePOIs: true,
		PreferOutdoorSeating: false,
		PreferDogFriendly:    true,
		PreferredVibes:       []string{"cozy", "modern"},
		PreferredTransport:   TransportPreferenceWalk,
		DietaryNeeds:         []string{"vegetarian"},
		CreatedAt:            now,
		UpdatedAt:            now,
	}
}

func createContextWithUserID(userID string) context.Context {
	return context.WithValue(context.Background(), "userID", userID)
}

func TestService_GetSearchProfiles(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mockRepo := new(MockRepository)
		service := createTestProfilesService(t, mockRepo)

		userID := uuid.New()
		profile := createMockProfile()
		profile.UserID = userID
		profiles := []UserPreferenceProfileResponse{*profile}

		req := &pb.GetSearchProfilesRequest{
			UserId: userID.String(),
		}

		// Mock expectations
		mockRepo.On("GetSearchProfiles", mock.Anything, userID).Return(profiles, nil).Once()

		// Create context with userID
		ctx := createContextWithUserID(userID.String())
		resp, err := service.GetSearchProfiles(ctx, req)

		// Assertions
		assert.NoError(t, err)
		assert.NotNil(t, resp)
		assert.Len(t, resp.Profiles, 1)
		assert.Equal(t, profile.ID.String(), resp.Profiles[0].Id)
		assert.Equal(t, profile.ProfileName, resp.Profiles[0].ProfileName)
		assert.Equal(t, profile.ID.String(), resp.DefaultProfileId)
		assert.Equal(t, "success", resp.Response.Status)
		mockRepo.AssertExpectations(t)
	})

	t.Run("MissingUserIDInContext", func(t *testing.T) {
		mockRepo := new(MockRepository)
		service := createTestProfilesService(t, mockRepo)

		req := &pb.GetSearchProfilesRequest{
			UserId: uuid.New().String(),
		}

		ctx := context.Background()
		resp, err := service.GetSearchProfiles(ctx, req)

		// Assertions
		assert.Error(t, err)
		assert.Nil(t, resp)

		grpcErr, ok := status.FromError(err)
		assert.True(t, ok)
		assert.Equal(t, codes.Unauthenticated, grpcErr.Code())
		assert.Contains(t, grpcErr.Message(), "userID is missing in metadata")
		mockRepo.AssertExpectations(t)
	})

	t.Run("InvalidUserIDType", func(t *testing.T) {
		mockRepo := new(MockRepository)
		service := createTestProfilesService(t, mockRepo)

		req := &pb.GetSearchProfilesRequest{
			UserId: uuid.New().String(),
		}

		ctx := context.WithValue(context.Background(), "userID", 123) // Invalid type
		resp, err := service.GetSearchProfiles(ctx, req)

		// Assertions
		assert.Error(t, err)
		assert.Nil(t, resp)

		grpcErr, ok := status.FromError(err)
		assert.True(t, ok)
		assert.Equal(t, codes.Unauthenticated, grpcErr.Code())
		assert.Contains(t, grpcErr.Message(), "userID has invalid type in metadata")
		mockRepo.AssertExpectations(t)
	})

	t.Run("EmptyUserID", func(t *testing.T) {
		mockRepo := new(MockRepository)
		service := createTestProfilesService(t, mockRepo)

		req := &pb.GetSearchProfilesRequest{
			UserId: "",
		}

		ctx := createContextWithUserID("")
		resp, err := service.GetSearchProfiles(ctx, req)

		// Assertions
		assert.Error(t, err)
		assert.Nil(t, resp)

		grpcErr, ok := status.FromError(err)
		assert.True(t, ok)
		assert.Equal(t, codes.Unauthenticated, grpcErr.Code())
		assert.Contains(t, grpcErr.Message(), "userID is empty in metadata")
		mockRepo.AssertExpectations(t)
	})

	t.Run("InvalidUserIDFormat", func(t *testing.T) {
		mockRepo := new(MockRepository)
		service := createTestProfilesService(t, mockRepo)

		req := &pb.GetSearchProfilesRequest{
			UserId: "invalid-uuid",
		}

		ctx := createContextWithUserID("invalid-uuid")
		resp, err := service.GetSearchProfiles(ctx, req)

		// Assertions
		assert.Error(t, err)
		assert.NotNil(t, resp)
		// The service returns empty response on invalid UUID

		grpcErr, ok := status.FromError(err)
		assert.True(t, ok)
		assert.Equal(t, codes.InvalidArgument, grpcErr.Code())
		assert.Contains(t, grpcErr.Message(), "invalid user ID format")
		mockRepo.AssertExpectations(t)
	})

	t.Run("RepositoryError", func(t *testing.T) {
		mockRepo := new(MockRepository)
		service := createTestProfilesService(t, mockRepo)

		userID := uuid.New()
		req := &pb.GetSearchProfilesRequest{
			UserId: userID.String(),
		}

		// Mock expectations - repository error
		mockRepo.On("GetSearchProfiles", mock.Anything, userID).Return(nil, errors.New("database error")).Once()

		ctx := createContextWithUserID(userID.String())
		resp, err := service.GetSearchProfiles(ctx, req)

		// Assertions
		assert.Error(t, err)
		assert.NotNil(t, resp)
		assert.Equal(t, "error", resp.Response.Status)

		grpcErr, ok := status.FromError(err)
		assert.True(t, ok)
		assert.Equal(t, codes.Internal, grpcErr.Code())
		assert.Contains(t, grpcErr.Message(), "failed to get search profiles")
		mockRepo.AssertExpectations(t)
	})
}

func TestService_GetSearchProfile(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mockRepo := new(MockRepository)
		service := createTestProfilesService(t, mockRepo)

		userID := uuid.New()
		profile := createMockProfile()
		profile.UserID = userID

		req := &pb.GetSearchProfileRequest{
			UserId:    userID.String(),
			ProfileId: profile.ID.String(),
		}

		// Mock expectations
		mockRepo.On("GetSearchProfile", mock.Anything, userID, profile.ID).Return(profile, nil).Once()

		ctx := createContextWithUserID(userID.String())
		resp, err := service.GetSearchProfile(ctx, req)

		// Assertions
		assert.NoError(t, err)
		assert.NotNil(t, resp)
		assert.NotNil(t, resp.Profile)
		assert.Equal(t, profile.ID.String(), resp.Profile.Id)
		assert.Equal(t, profile.ProfileName, resp.Profile.ProfileName)
		assert.Equal(t, "success", resp.Response.Status)
		mockRepo.AssertExpectations(t)
	})

	t.Run("InvalidProfileID", func(t *testing.T) {
		mockRepo := new(MockRepository)
		service := createTestProfilesService(t, mockRepo)

		userID := uuid.New()
		req := &pb.GetSearchProfileRequest{
			UserId:    userID.String(),
			ProfileId: "invalid-uuid",
		}

		ctx := createContextWithUserID(userID.String())
		resp, err := service.GetSearchProfile(ctx, req)

		// Assertions
		assert.Error(t, err)
		assert.NotNil(t, resp)
		assert.Equal(t, "error", resp.Response.Status)

		grpcErr, ok := status.FromError(err)
		assert.True(t, ok)
		assert.Equal(t, codes.InvalidArgument, grpcErr.Code())
		assert.Contains(t, grpcErr.Message(), "invalid profile ID format")
		mockRepo.AssertExpectations(t)
	})

	t.Run("ProfileNotFound", func(t *testing.T) {
		mockRepo := new(MockRepository)
		service := createTestProfilesService(t, mockRepo)

		userID := uuid.New()
		profileID := uuid.New()
		req := &pb.GetSearchProfileRequest{
			UserId:    userID.String(),
			ProfileId: profileID.String(),
		}

		// Mock expectations - profile not found
		mockRepo.On("GetSearchProfile", mock.Anything, userID, profileID).Return(nil, domain.ErrNotFound).Once()

		ctx := createContextWithUserID(userID.String())
		resp, err := service.GetSearchProfile(ctx, req)

		// Assertions
		assert.Error(t, err)
		assert.NotNil(t, resp)
		assert.Equal(t, "error", resp.Response.Status)

		grpcErr, ok := status.FromError(err)
		assert.True(t, ok)
		assert.Equal(t, codes.NotFound, grpcErr.Code())
		assert.Contains(t, grpcErr.Message(), "search profile not found")
		mockRepo.AssertExpectations(t)
	})
}

func TestService_GetDefaultSearchProfile(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mockRepo := new(MockRepository)
		service := createTestProfilesService(t, mockRepo)

		userID := uuid.New()
		profile := createMockProfile()
		profile.UserID = userID
		profile.IsDefault = true

		req := &pb.GetDefaultSearchProfileRequest{
			UserId: userID.String(),
		}

		// Mock expectations
		mockRepo.On("GetDefaultSearchProfile", mock.Anything, userID).Return(profile, nil).Once()

		ctx := createContextWithUserID(userID.String())
		resp, err := service.GetDefaultSearchProfile(ctx, req)

		// Assertions
		assert.NoError(t, err)
		assert.NotNil(t, resp)
		assert.NotNil(t, resp.Profile)
		assert.Equal(t, profile.ID.String(), resp.Profile.Id)
		assert.True(t, resp.Profile.IsDefault)
		assert.Equal(t, "success", resp.Response.Status)
		mockRepo.AssertExpectations(t)
	})

	t.Run("DefaultProfileNotFound", func(t *testing.T) {
		mockRepo := new(MockRepository)
		service := createTestProfilesService(t, mockRepo)

		userID := uuid.New()
		req := &pb.GetDefaultSearchProfileRequest{
			UserId: userID.String(),
		}

		// Mock expectations - default profile not found
		mockRepo.On("GetDefaultSearchProfile", mock.Anything, userID).Return(nil, domain.ErrNotFound).Once()

		ctx := createContextWithUserID(userID.String())
		resp, err := service.GetDefaultSearchProfile(ctx, req)

		// Assertions
		assert.Error(t, err)
		assert.NotNil(t, resp)
		assert.Equal(t, "error", resp.Response.Status)

		grpcErr, ok := status.FromError(err)
		assert.True(t, ok)
		assert.Equal(t, codes.NotFound, grpcErr.Code())
		assert.Contains(t, grpcErr.Message(), "default search profile not found")
		mockRepo.AssertExpectations(t)
	})
}

func TestService_CreateSearchProfile(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mockRepo := new(MockRepository)
		service := createTestProfilesService(t, mockRepo)

		userID := uuid.New()
		profile := createMockProfile()
		profile.UserID = userID

		req := &pb.CreateSearchProfileRequest{
			UserId: userID.String(),
			Profile: &pb.CreateUserPreferenceProfileParams{
				ProfileName:          "New Profile",
				IsDefault:            false,
				SearchRadiusKm:       5.0,
				PreferredTime:        pb.DayPreference_DAY_PREFERENCE_DAY,
				BudgetLevel:          2,
				PreferredPace:        pb.SearchPace_SEARCH_PACE_RELAXED,
				PreferAccessiblePois: true,
				PreferOutdoorSeating: false,
				PreferDogFriendly:    true,
				PreferredVibes:       []string{"cozy"},
				PreferredTransport:   pb.TransportPreference_TRANSPORT_PREFERENCE_WALK,
				DietaryNeeds:         []string{"vegan"},
			},
		}

		// Mock expectations
		mockRepo.On("CreateSearchProfile", mock.Anything, userID, mock.AnythingOfType("CreateUserPreferenceProfileParams")).Return(profile, nil).Once()

		ctx := createContextWithUserID(userID.String())
		resp, err := service.CreateSearchProfile(ctx, req)

		// Assertions
		assert.NoError(t, err)
		assert.NotNil(t, resp)
		assert.True(t, resp.Success)
		assert.Equal(t, "search profile created successfully", resp.Message)
		assert.NotNil(t, resp.Profile)
		assert.Equal(t, "success", resp.Response.Status)
		mockRepo.AssertExpectations(t)
	})

	t.Run("ProfileNameConflict", func(t *testing.T) {
		mockRepo := new(MockRepository)
		service := createTestProfilesService(t, mockRepo)

		userID := uuid.New()
		req := &pb.CreateSearchProfileRequest{
			UserId: userID.String(),
			Profile: &pb.CreateUserPreferenceProfileParams{
				ProfileName: "Existing Profile",
			},
		}

		// Mock expectations - profile name conflict
		mockRepo.On("CreateSearchProfile", mock.Anything, userID, mock.AnythingOfType("CreateUserPreferenceProfileParams")).Return(nil, domain.ErrConflict).Once()

		ctx := createContextWithUserID(userID.String())
		resp, err := service.CreateSearchProfile(ctx, req)

		// Assertions
		assert.Error(t, err)
		assert.NotNil(t, resp)
		assert.False(t, resp.Success)
		assert.Equal(t, "profile name already exists", resp.Message)
		assert.Equal(t, "error", resp.Response.Status)

		grpcErr, ok := status.FromError(err)
		assert.True(t, ok)
		assert.Equal(t, codes.AlreadyExists, grpcErr.Code())
		assert.Contains(t, grpcErr.Message(), "profile name already exists")
		mockRepo.AssertExpectations(t)
	})

	t.Run("RepositoryError", func(t *testing.T) {
		mockRepo := new(MockRepository)
		service := createTestProfilesService(t, mockRepo)

		userID := uuid.New()
		req := &pb.CreateSearchProfileRequest{
			UserId: userID.String(),
			Profile: &pb.CreateUserPreferenceProfileParams{
				ProfileName: "Test Profile",
			},
		}

		// Mock expectations - repository error
		mockRepo.On("CreateSearchProfile", mock.Anything, userID, mock.AnythingOfType("CreateUserPreferenceProfileParams")).Return(nil, errors.New("database error")).Once()

		ctx := createContextWithUserID(userID.String())
		resp, err := service.CreateSearchProfile(ctx, req)

		// Assertions
		assert.Error(t, err)
		assert.NotNil(t, resp)
		assert.False(t, resp.Success)
		assert.Equal(t, "failed to create search profile", resp.Message)
		assert.Equal(t, "error", resp.Response.Status)

		grpcErr, ok := status.FromError(err)
		assert.True(t, ok)
		assert.Equal(t, codes.Internal, grpcErr.Code())
		assert.Contains(t, grpcErr.Message(), "failed to create search profile")
		mockRepo.AssertExpectations(t)
	})
}

func TestService_UpdateSearchProfile(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mockRepo := new(MockRepository)
		service := createTestProfilesService(t, mockRepo)

		userID := uuid.New()
		profileID := uuid.New()
		profile := createMockProfile()
		profile.UserID = userID
		profile.ID = profileID

		req := &pb.UpdateSearchProfileRequest{
			UserId:    userID.String(),
			ProfileId: profileID.String(),
			Profile: &pb.UpdateSearchProfileParams{
				ProfileName:    "Updated Profile",
				SearchRadiusKm: 15.0,
			},
		}

		// Mock expectations
		mockRepo.On("UpdateSearchProfile", mock.Anything, userID, profileID, mock.AnythingOfType("UpdateSearchProfileParams")).Return(nil).Once()
		mockRepo.On("GetSearchProfile", mock.Anything, userID, profileID).Return(profile, nil).Once()

		ctx := createContextWithUserID(userID.String())
		resp, err := service.UpdateSearchProfile(ctx, req)

		// Assertions
		assert.NoError(t, err)
		assert.NotNil(t, resp)
		assert.True(t, resp.Success)
		assert.Equal(t, "search profile updated successfully", resp.Message)
		assert.NotNil(t, resp.Profile)
		assert.Equal(t, "success", resp.Response.Status)
		mockRepo.AssertExpectations(t)
	})

	t.Run("InvalidProfileID", func(t *testing.T) {
		mockRepo := new(MockRepository)
		service := createTestProfilesService(t, mockRepo)

		userID := uuid.New()
		req := &pb.UpdateSearchProfileRequest{
			UserId:    userID.String(),
			ProfileId: "invalid-uuid",
			Profile: &pb.UpdateSearchProfileParams{
				ProfileName: "Updated Profile",
			},
		}

		ctx := createContextWithUserID(userID.String())
		resp, err := service.UpdateSearchProfile(ctx, req)

		// Assertions
		assert.Error(t, err)
		assert.NotNil(t, resp)
		assert.False(t, resp.Success)
		assert.Equal(t, "invalid profile ID format", resp.Message)
		assert.Equal(t, "error", resp.Response.Status)

		grpcErr, ok := status.FromError(err)
		assert.True(t, ok)
		assert.Equal(t, codes.InvalidArgument, grpcErr.Code())
		assert.Contains(t, grpcErr.Message(), "invalid profile ID format")
		mockRepo.AssertExpectations(t)
	})

	t.Run("ProfileNotFound", func(t *testing.T) {
		mockRepo := new(MockRepository)
		service := createTestProfilesService(t, mockRepo)

		userID := uuid.New()
		profileID := uuid.New()
		req := &pb.UpdateSearchProfileRequest{
			UserId:    userID.String(),
			ProfileId: profileID.String(),
			Profile: &pb.UpdateSearchProfileParams{
				ProfileName: "Updated Profile",
			},
		}

		// Mock expectations - profile not found
		mockRepo.On("UpdateSearchProfile", mock.Anything, userID, profileID, mock.AnythingOfType("UpdateSearchProfileParams")).Return(domain.ErrNotFound).Once()

		ctx := createContextWithUserID(userID.String())
		resp, err := service.UpdateSearchProfile(ctx, req)

		// Assertions
		assert.Error(t, err)
		assert.NotNil(t, resp)
		assert.False(t, resp.Success)
		assert.Equal(t, "search profile not found", resp.Message)
		assert.Equal(t, "error", resp.Response.Status)

		grpcErr, ok := status.FromError(err)
		assert.True(t, ok)
		assert.Equal(t, codes.NotFound, grpcErr.Code())
		assert.Contains(t, grpcErr.Message(), "search profile not found")
		mockRepo.AssertExpectations(t)
	})

	t.Run("ProfileNameConflict", func(t *testing.T) {
		mockRepo := new(MockRepository)
		service := createTestProfilesService(t, mockRepo)

		userID := uuid.New()
		profileID := uuid.New()
		req := &pb.UpdateSearchProfileRequest{
			UserId:    userID.String(),
			ProfileId: profileID.String(),
			Profile: &pb.UpdateSearchProfileParams{
				ProfileName: "Existing Profile Name",
			},
		}

		// Mock expectations - profile name conflict
		mockRepo.On("UpdateSearchProfile", mock.Anything, userID, profileID, mock.AnythingOfType("UpdateSearchProfileParams")).Return(domain.ErrConflict).Once()

		ctx := createContextWithUserID(userID.String())
		resp, err := service.UpdateSearchProfile(ctx, req)

		// Assertions
		assert.Error(t, err)
		assert.NotNil(t, resp)
		assert.False(t, resp.Success)
		assert.Equal(t, "profile name already exists", resp.Message)
		assert.Equal(t, "error", resp.Response.Status)

		grpcErr, ok := status.FromError(err)
		assert.True(t, ok)
		assert.Equal(t, codes.AlreadyExists, grpcErr.Code())
		assert.Contains(t, grpcErr.Message(), "profile name already exists")
		mockRepo.AssertExpectations(t)
	})
}

func TestService_DeleteSearchProfile(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mockRepo := new(MockRepository)
		service := createTestProfilesService(t, mockRepo)

		userID := uuid.New()
		profileID := uuid.New()
		req := &pb.DeleteSearchProfileRequest{
			UserId:    userID.String(),
			ProfileId: profileID.String(),
		}

		// Mock expectations
		mockRepo.On("DeleteSearchProfile", mock.Anything, userID, profileID).Return(nil).Once()

		ctx := createContextWithUserID(userID.String())
		resp, err := service.DeleteSearchProfile(ctx, req)

		// Assertions
		assert.NoError(t, err)
		assert.NotNil(t, resp)
		assert.True(t, resp.Success)
		assert.Equal(t, "search profile deleted successfully", resp.Message)
		assert.Equal(t, "success", resp.Response.Status)
		mockRepo.AssertExpectations(t)
	})

	t.Run("InvalidProfileID", func(t *testing.T) {
		mockRepo := new(MockRepository)
		service := createTestProfilesService(t, mockRepo)

		userID := uuid.New()
		req := &pb.DeleteSearchProfileRequest{
			UserId:    userID.String(),
			ProfileId: "invalid-uuid",
		}

		ctx := createContextWithUserID(userID.String())
		resp, err := service.DeleteSearchProfile(ctx, req)

		// Assertions
		assert.Error(t, err)
		assert.NotNil(t, resp)
		assert.False(t, resp.Success)
		assert.Equal(t, "invalid profile ID format", resp.Message)
		assert.Equal(t, "error", resp.Response.Status)

		grpcErr, ok := status.FromError(err)
		assert.True(t, ok)
		assert.Equal(t, codes.InvalidArgument, grpcErr.Code())
		assert.Contains(t, grpcErr.Message(), "invalid profile ID format")
		mockRepo.AssertExpectations(t)
	})

	t.Run("ProfileNotFound", func(t *testing.T) {
		mockRepo := new(MockRepository)
		service := createTestProfilesService(t, mockRepo)

		userID := uuid.New()
		profileID := uuid.New()
		req := &pb.DeleteSearchProfileRequest{
			UserId:    userID.String(),
			ProfileId: profileID.String(),
		}

		// Mock expectations - profile not found
		mockRepo.On("DeleteSearchProfile", mock.Anything, userID, profileID).Return(domain.ErrNotFound).Once()

		ctx := createContextWithUserID(userID.String())
		resp, err := service.DeleteSearchProfile(ctx, req)

		// Assertions
		assert.Error(t, err)
		assert.NotNil(t, resp)
		assert.False(t, resp.Success)
		assert.Equal(t, "search profile not found", resp.Message)
		assert.Equal(t, "error", resp.Response.Status)

		grpcErr, ok := status.FromError(err)
		assert.True(t, ok)
		assert.Equal(t, codes.NotFound, grpcErr.Code())
		assert.Contains(t, grpcErr.Message(), "search profile not found")
		mockRepo.AssertExpectations(t)
	})

	t.Run("CannotDeleteDefaultProfile", func(t *testing.T) {
		mockRepo := new(MockRepository)
		service := createTestProfilesService(t, mockRepo)

		userID := uuid.New()
		profileID := uuid.New()
		req := &pb.DeleteSearchProfileRequest{
			UserId:    userID.String(),
			ProfileId: profileID.String(),
		}

		// Mock expectations - cannot delete default profile
		mockRepo.On("DeleteSearchProfile", mock.Anything, userID, profileID).Return(errors.New("cannot delete default profile")).Once()

		ctx := createContextWithUserID(userID.String())
		resp, err := service.DeleteSearchProfile(ctx, req)

		// Assertions
		assert.Error(t, err)
		assert.NotNil(t, resp)
		assert.False(t, resp.Success)
		assert.Equal(t, "cannot delete default profile", resp.Message)
		assert.Equal(t, "error", resp.Response.Status)

		grpcErr, ok := status.FromError(err)
		assert.True(t, ok)
		assert.Equal(t, codes.FailedPrecondition, grpcErr.Code())
		assert.Contains(t, grpcErr.Message(), "cannot delete default profile")
		mockRepo.AssertExpectations(t)
	})
}

func TestService_SetDefaultSearchProfile(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mockRepo := new(MockRepository)
		service := createTestProfilesService(t, mockRepo)

		userID := uuid.New()
		profileID := uuid.New()
		req := &pb.SetDefaultSearchProfileRequest{
			UserId:    userID.String(),
			ProfileId: profileID.String(),
		}

		// Mock expectations
		mockRepo.On("SetDefaultSearchProfile", mock.Anything, userID, profileID).Return(nil).Once()

		ctx := createContextWithUserID(userID.String())
		resp, err := service.SetDefaultSearchProfile(ctx, req)

		// Assertions
		assert.NoError(t, err)
		assert.NotNil(t, resp)
		assert.True(t, resp.Success)
		assert.Equal(t, "default search profile set successfully", resp.Message)
		assert.Equal(t, "success", resp.Response.Status)
		mockRepo.AssertExpectations(t)
	})

	t.Run("InvalidProfileID", func(t *testing.T) {
		mockRepo := new(MockRepository)
		service := createTestProfilesService(t, mockRepo)

		userID := uuid.New()
		req := &pb.SetDefaultSearchProfileRequest{
			UserId:    userID.String(),
			ProfileId: "invalid-uuid",
		}

		ctx := createContextWithUserID(userID.String())
		resp, err := service.SetDefaultSearchProfile(ctx, req)

		// Assertions
		assert.Error(t, err)
		assert.NotNil(t, resp)
		assert.False(t, resp.Success)
		assert.Equal(t, "invalid profile ID format", resp.Message)
		assert.Equal(t, "error", resp.Response.Status)

		grpcErr, ok := status.FromError(err)
		assert.True(t, ok)
		assert.Equal(t, codes.InvalidArgument, grpcErr.Code())
		assert.Contains(t, grpcErr.Message(), "invalid profile ID format")
		mockRepo.AssertExpectations(t)
	})

	t.Run("ProfileNotFound", func(t *testing.T) {
		mockRepo := new(MockRepository)
		service := createTestProfilesService(t, mockRepo)

		userID := uuid.New()
		profileID := uuid.New()
		req := &pb.SetDefaultSearchProfileRequest{
			UserId:    userID.String(),
			ProfileId: profileID.String(),
		}

		// Mock expectations - profile not found
		mockRepo.On("SetDefaultSearchProfile", mock.Anything, userID, profileID).Return(domain.ErrNotFound).Once()

		ctx := createContextWithUserID(userID.String())
		resp, err := service.SetDefaultSearchProfile(ctx, req)

		// Assertions
		assert.Error(t, err)
		assert.NotNil(t, resp)
		assert.False(t, resp.Success)
		assert.Equal(t, "search profile not found", resp.Message)
		assert.Equal(t, "error", resp.Response.Status)

		grpcErr, ok := status.FromError(err)
		assert.True(t, ok)
		assert.Equal(t, codes.NotFound, grpcErr.Code())
		assert.Contains(t, grpcErr.Message(), "search profile not found")
		mockRepo.AssertExpectations(t)
	})

	t.Run("RepositoryError", func(t *testing.T) {
		mockRepo := new(MockRepository)
		service := createTestProfilesService(t, mockRepo)

		userID := uuid.New()
		profileID := uuid.New()
		req := &pb.SetDefaultSearchProfileRequest{
			UserId:    userID.String(),
			ProfileId: profileID.String(),
		}

		// Mock expectations - repository error
		mockRepo.On("SetDefaultSearchProfile", mock.Anything, userID, profileID).Return(errors.New("database error")).Once()

		ctx := createContextWithUserID(userID.String())
		resp, err := service.SetDefaultSearchProfile(ctx, req)

		// Assertions
		assert.Error(t, err)
		assert.NotNil(t, resp)
		assert.False(t, resp.Success)
		assert.Equal(t, "failed to set default search profile", resp.Message)
		assert.Equal(t, "error", resp.Response.Status)

		grpcErr, ok := status.FromError(err)
		assert.True(t, ok)
		assert.Equal(t, codes.Internal, grpcErr.Code())
		assert.Contains(t, grpcErr.Message(), "failed to set default search profile")
		mockRepo.AssertExpectations(t)
	})
}

// Test converter functions
func TestEnumConverters(t *testing.T) {
	t.Run("DayPreferenceConverters", func(t *testing.T) {
		// Test convertToPBDayPreference
		assert.Equal(t, pb.DayPreference_DAY_PREFERENCE_ANY, convertToPBDayPreference(DayPreferenceAny))
		assert.Equal(t, pb.DayPreference_DAY_PREFERENCE_DAY, convertToPBDayPreference(DayPreferenceDay))
		assert.Equal(t, pb.DayPreference_DAY_PREFERENCE_NIGHT, convertToPBDayPreference(DayPreferenceNight))
		assert.Equal(t, pb.DayPreference_DAY_PREFERENCE_UNSPECIFIED, convertToPBDayPreference("invalid"))

		// Test convertFromPBDayPreference
		assert.Equal(t, DayPreferenceAny, convertFromPBDayPreference(pb.DayPreference_DAY_PREFERENCE_ANY))
		assert.Equal(t, DayPreferenceDay, convertFromPBDayPreference(pb.DayPreference_DAY_PREFERENCE_DAY))
		assert.Equal(t, DayPreferenceNight, convertFromPBDayPreference(pb.DayPreference_DAY_PREFERENCE_NIGHT))
		assert.Equal(t, DayPreferenceAny, convertFromPBDayPreference(pb.DayPreference_DAY_PREFERENCE_UNSPECIFIED))
	})

	t.Run("SearchPaceConverters", func(t *testing.T) {
		// Test convertToPBSearchPace
		assert.Equal(t, pb.SearchPace_SEARCH_PACE_ANY, convertToPBSearchPace(SearchPaceAny))
		assert.Equal(t, pb.SearchPace_SEARCH_PACE_RELAXED, convertToPBSearchPace(SearchPaceRelaxed))
		assert.Equal(t, pb.SearchPace_SEARCH_PACE_MODERATE, convertToPBSearchPace(SearchPaceModerate))
		assert.Equal(t, pb.SearchPace_SEARCH_PACE_FAST, convertToPBSearchPace(SearchPaceFast))
		assert.Equal(t, pb.SearchPace_SEARCH_PACE_UNSPECIFIED, convertToPBSearchPace("invalid"))

		// Test convertFromPBSearchPace
		assert.Equal(t, SearchPaceAny, convertFromPBSearchPace(pb.SearchPace_SEARCH_PACE_ANY))
		assert.Equal(t, SearchPaceRelaxed, convertFromPBSearchPace(pb.SearchPace_SEARCH_PACE_RELAXED))
		assert.Equal(t, SearchPaceModerate, convertFromPBSearchPace(pb.SearchPace_SEARCH_PACE_MODERATE))
		assert.Equal(t, SearchPaceFast, convertFromPBSearchPace(pb.SearchPace_SEARCH_PACE_FAST))
		assert.Equal(t, SearchPaceAny, convertFromPBSearchPace(pb.SearchPace_SEARCH_PACE_UNSPECIFIED))
	})

	t.Run("TransportPreferenceConverters", func(t *testing.T) {
		// Test convertToPBTransportPreference
		assert.Equal(t, pb.TransportPreference_TRANSPORT_PREFERENCE_ANY, convertToPBTransportPreference(TransportPreferenceAny))
		assert.Equal(t, pb.TransportPreference_TRANSPORT_PREFERENCE_WALK, convertToPBTransportPreference(TransportPreferenceWalk))
		assert.Equal(t, pb.TransportPreference_TRANSPORT_PREFERENCE_PUBLIC, convertToPBTransportPreference(TransportPreferencePublic))
		assert.Equal(t, pb.TransportPreference_TRANSPORT_PREFERENCE_CAR, convertToPBTransportPreference(TransportPreferenceCar))
		assert.Equal(t, pb.TransportPreference_TRANSPORT_PREFERENCE_UNSPECIFIED, convertToPBTransportPreference("invalid"))

		// Test convertFromPBTransportPreference
		assert.Equal(t, TransportPreferenceAny, convertFromPBTransportPreference(pb.TransportPreference_TRANSPORT_PREFERENCE_ANY))
		assert.Equal(t, TransportPreferenceWalk, convertFromPBTransportPreference(pb.TransportPreference_TRANSPORT_PREFERENCE_WALK))
		assert.Equal(t, TransportPreferencePublic, convertFromPBTransportPreference(pb.TransportPreference_TRANSPORT_PREFERENCE_PUBLIC))
		assert.Equal(t, TransportPreferenceCar, convertFromPBTransportPreference(pb.TransportPreference_TRANSPORT_PREFERENCE_CAR))
		assert.Equal(t, TransportPreferenceAny, convertFromPBTransportPreference(pb.TransportPreference_TRANSPORT_PREFERENCE_UNSPECIFIED))
	})
}

func TestProfileConverters(t *testing.T) {
	t.Run("ConvertToPBProfile", func(t *testing.T) {
		profile := createMockProfile()
		pbProfile := convertToPBProfile(profile)

		assert.Equal(t, profile.ID.String(), pbProfile.Id)
		assert.Equal(t, profile.UserID.String(), pbProfile.UserId)
		assert.Equal(t, profile.ProfileName, pbProfile.ProfileName)
		assert.Equal(t, profile.IsDefault, pbProfile.IsDefault)
		assert.Equal(t, profile.SearchRadiusKm, pbProfile.SearchRadiusKm)
		assert.Equal(t, profile.BudgetLevel, int(pbProfile.BudgetLevel))
		assert.Equal(t, profile.PreferAccessiblePOIs, pbProfile.PreferAccessiblePois)
		assert.Equal(t, profile.PreferOutdoorSeating, pbProfile.PreferOutdoorSeating)
		assert.Equal(t, profile.PreferDogFriendly, pbProfile.PreferDogFriendly)
		assert.Equal(t, profile.PreferredVibes, pbProfile.PreferredVibes)
		assert.Equal(t, profile.DietaryNeeds, pbProfile.DietaryNeeds)
		assert.NotNil(t, pbProfile.CreatedAt)
		assert.NotNil(t, pbProfile.UpdatedAt)
	})

	t.Run("ConvertPBToCreateParams", func(t *testing.T) {
		pbProfile := &pb.CreateUserPreferenceProfileParams{
			ProfileName:          "Test Profile",
			IsDefault:            true,
			SearchRadiusKm:       10.0,
			PreferredTime:        pb.DayPreference_DAY_PREFERENCE_DAY,
			BudgetLevel:          3,
			PreferredPace:        pb.SearchPace_SEARCH_PACE_MODERATE,
			PreferAccessiblePois: true,
			PreferOutdoorSeating: false, // This will be nil in result since it's false
			PreferDogFriendly:    true,
			PreferredVibes:       []string{"cozy", "modern"},
			PreferredTransport:   pb.TransportPreference_TRANSPORT_PREFERENCE_WALK,
			DietaryNeeds:         []string{"vegetarian"},
		}

		params := convertPBToCreateParams(pbProfile)

		assert.Equal(t, pbProfile.ProfileName, params.ProfileName)
		
		// Only true boolean values get set as pointers
		assert.NotNil(t, params.IsDefault)
		assert.Equal(t, pbProfile.IsDefault, *params.IsDefault)
		
		assert.NotNil(t, params.SearchRadiusKm)
		assert.Equal(t, pbProfile.SearchRadiusKm, *params.SearchRadiusKm)
		
		assert.NotNil(t, params.PreferredTime)
		assert.Equal(t, DayPreferenceDay, *params.PreferredTime)
		
		assert.NotNil(t, params.BudgetLevel)
		assert.Equal(t, int(pbProfile.BudgetLevel), *params.BudgetLevel)
		
		assert.NotNil(t, params.PreferredPace)
		assert.Equal(t, SearchPaceModerate, *params.PreferredPace)
		
		assert.NotNil(t, params.PreferAccessiblePOIs)
		assert.Equal(t, pbProfile.PreferAccessiblePois, *params.PreferAccessiblePOIs)
		
		// This should be nil since PreferOutdoorSeating is false
		assert.Nil(t, params.PreferOutdoorSeating)
		
		assert.NotNil(t, params.PreferDogFriendly)
		assert.Equal(t, pbProfile.PreferDogFriendly, *params.PreferDogFriendly)
		
		assert.Equal(t, pbProfile.PreferredVibes, params.PreferredVibes)
		
		assert.NotNil(t, params.PreferredTransport)
		assert.Equal(t, TransportPreferenceWalk, *params.PreferredTransport)
		
		assert.Equal(t, pbProfile.DietaryNeeds, params.DietaryNeeds)
	})

	t.Run("ConvertPBToUpdateParams", func(t *testing.T) {
		pbProfile := &pb.UpdateSearchProfileParams{
			ProfileName:          "Updated Profile",
			IsDefault:            false, // This will be nil since it's false
			SearchRadiusKm:       15.0,
			PreferredTime:        pb.DayPreference_DAY_PREFERENCE_NIGHT,
			BudgetLevel:          2,
			PreferredPace:        pb.SearchPace_SEARCH_PACE_FAST,
			PreferAccessiblePois: false, // This will be nil since it's false
			PreferOutdoorSeating: true,
			PreferDogFriendly:    false, // This will be nil since it's false
			PreferredVibes:       []string{"modern"},
			PreferredTransport:   pb.TransportPreference_TRANSPORT_PREFERENCE_CAR,
			DietaryNeeds:         []string{"vegan"},
		}

		params := convertPBToUpdateParams(pbProfile)

		assert.Equal(t, pbProfile.ProfileName, params.ProfileName)
		
		// IsDefault is false, so it should be nil
		assert.Nil(t, params.IsDefault)
		
		assert.NotNil(t, params.SearchRadiusKm)
		assert.Equal(t, pbProfile.SearchRadiusKm, *params.SearchRadiusKm)
		
		assert.NotNil(t, params.PreferredTime)
		assert.Equal(t, DayPreferenceNight, *params.PreferredTime)
		
		assert.NotNil(t, params.BudgetLevel)
		assert.Equal(t, int(pbProfile.BudgetLevel), *params.BudgetLevel)
		
		assert.NotNil(t, params.PreferredPace)
		assert.Equal(t, SearchPaceFast, *params.PreferredPace)
		
		// PreferAccessiblePois is false, so it should be nil
		assert.Nil(t, params.PreferAccessiblePOIs)
		
		assert.NotNil(t, params.PreferOutdoorSeating)
		assert.Equal(t, pbProfile.PreferOutdoorSeating, *params.PreferOutdoorSeating)
		
		// PreferDogFriendly is false, so it should be nil
		assert.Nil(t, params.PreferDogFriendly)
		
		assert.Equal(t, pbProfile.PreferredVibes, params.PreferredVibes)
		
		assert.NotNil(t, params.PreferredTransport)
		assert.Equal(t, TransportPreferenceCar, *params.PreferredTransport)
		
		assert.Equal(t, pbProfile.DietaryNeeds, params.DietaryNeeds)
	})
}