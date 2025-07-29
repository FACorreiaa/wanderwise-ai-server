package interests

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
	"go.uber.org/zap/zapcore"

	pb "github.com/FACorreiaa/loci-proto/modules/interests/generated"

	"github.com/FACorreiaa/go-poi-au-suggestions/internal/types"
)

// MockInterestsRepository implements the Repository interface
type MockInterestsRepository struct {
	mock.Mock
}

func (m *MockInterestsRepository) CreateInterest(ctx context.Context, name string, description *string, isActive bool, userID string) (*types.Interest, error) {
	args := m.Called(ctx, name, description, isActive, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*types.Interest), args.Error(1)
}

func (m *MockInterestsRepository) RemoveInterests(ctx context.Context, userID uuid.UUID, interestID uuid.UUID) error {
	args := m.Called(ctx, userID, interestID)
	return args.Error(0)
}

func (m *MockInterestsRepository) GetAllInterests(ctx context.Context) ([]*types.Interest, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*types.Interest), args.Error(1)
}

func (m *MockInterestsRepository) GetInterest(ctx context.Context, interestID uuid.UUID) (*types.Interest, error) {
	args := m.Called(ctx, interestID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*types.Interest), args.Error(1)
}

func (m *MockInterestsRepository) UpdateInterests(ctx context.Context, userID uuid.UUID, interestID uuid.UUID, params types.UpdateinterestsParams) error {
	args := m.Called(ctx, userID, interestID, params)
	return args.Error(0)
}

func (m *MockInterestsRepository) AddInterestToProfile(ctx context.Context, profileID, interestID uuid.UUID) error {
	args := m.Called(ctx, profileID, interestID)
	return args.Error(0)
}

func (m *MockInterestsRepository) GetInterestsForProfile(ctx context.Context, profileID uuid.UUID) ([]*types.Interest, error) {
	args := m.Called(ctx, profileID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*types.Interest), args.Error(1)
}

func createTestInterestsService(t *testing.T, mockRepo *MockInterestsRepository) *Service {
	// Create a simple zap logger for testing
	config := zap.NewDevelopmentConfig()
	config.Level = zap.NewAtomicLevelAt(zapcore.DebugLevel)
	config.OutputPaths = []string{"stdout"}
	logger, _ := config.Build()

	// Create a mock pgxpool.Pool - for this test we don't actually use it
	var pgpool *pgxpool.Pool = nil

	return NewService(context.Background(), mockRepo, pgpool, logger)
}

func TestInterestsService_GetAllInterests(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mockRepo := new(MockInterestsRepository)
		service := createTestInterestsService(t, mockRepo)

		ctx := context.Background()

		mockInterests := []*types.Interest{
			{
				ID:          uuid.New(),
				Name:        "Test Interest 1",
				Description: stringPtr("Test description"),
				Active:      boolPtr(true),
				Source:      "global",
				CreatedAt:   time.Now(),
			},
			{
				ID:          uuid.New(),
				Name:        "Test Interest 2",
				Description: stringPtr("Another description"),
				Active:      boolPtr(true),
				Source:      "custom",
				CreatedAt:   time.Now(),
			},
		}

		req := &pb.GetAllInterestsRequest{
			Request: &pb.BaseRequest{
				RequestId: "test-request-id",
			},
		}

		// Mock expectations
		mockRepo.On("GetAllInterests", mock.Anything).Return(mockInterests, nil).Once()

		// Call the gRPC method
		resp, err := service.GetAllInterests(ctx, req)

		// Assertions
		assert.NoError(t, err)
		assert.NotNil(t, resp)
		assert.Len(t, resp.Interests, 2)
		assert.Equal(t, "test-request-id", resp.Response.RequestId)
		assert.Equal(t, "success", resp.Response.Status)
		assert.Equal(t, mockInterests[0].Name, resp.Interests[0].Name)
		assert.Equal(t, mockInterests[1].Name, resp.Interests[1].Name)
		mockRepo.AssertExpectations(t)
	})

	t.Run("NilRequest", func(t *testing.T) {
		mockRepo := new(MockInterestsRepository)
		service := createTestInterestsService(t, mockRepo)

		ctx := context.Background()

		mockInterests := []*types.Interest{
			{
				ID:          uuid.New(),
				Name:        "Test Interest 1",
				Description: stringPtr("Test description"),
				Active:      boolPtr(true),
				Source:      "global",
				CreatedAt:   time.Now(),
			},
		}

		// Mock expectations
		mockRepo.On("GetAllInterests", mock.Anything).Return(mockInterests, nil).Once()

		// Call with nil request
		resp, err := service.GetAllInterests(ctx, nil)

		// Assertions
		assert.NoError(t, err)
		assert.NotNil(t, resp)
		assert.Len(t, resp.Interests, 1)
		assert.Equal(t, "", resp.Response.RequestId) // Should be empty when request is nil
		assert.Equal(t, "success", resp.Response.Status)
		mockRepo.AssertExpectations(t)
	})

	t.Run("NilBaseRequest", func(t *testing.T) {
		mockRepo := new(MockInterestsRepository)
		service := createTestInterestsService(t, mockRepo)

		ctx := context.Background()

		mockInterests := []*types.Interest{
			{
				ID:          uuid.New(),
				Name:        "Test Interest 1",
				Description: stringPtr("Test description"),
				Active:      boolPtr(true),
				Source:      "global",
				CreatedAt:   time.Now(),
			},
		}

		req := &pb.GetAllInterestsRequest{
			Request: nil, // nil BaseRequest
		}

		// Mock expectations
		mockRepo.On("GetAllInterests", mock.Anything).Return(mockInterests, nil).Once()

		// Call the gRPC method
		resp, err := service.GetAllInterests(ctx, req)

		// Assertions
		assert.NoError(t, err)
		assert.NotNil(t, resp)
		assert.Len(t, resp.Interests, 1)
		assert.Equal(t, "", resp.Response.RequestId) // Should be empty when BaseRequest is nil
		assert.Equal(t, "success", resp.Response.Status)
		mockRepo.AssertExpectations(t)
	})

	t.Run("RepositoryError", func(t *testing.T) {
		mockRepo := new(MockInterestsRepository)
		service := createTestInterestsService(t, mockRepo)

		ctx := context.Background()

		req := &pb.GetAllInterestsRequest{
			Request: &pb.BaseRequest{
				RequestId: "test-request-id",
			},
		}

		// Mock expectations - repository error
		mockRepo.On("GetAllInterests", mock.Anything).Return(nil, errors.New("database error")).Once()

		resp, err := service.GetAllInterests(ctx, req)

		assert.Error(t, err)
		assert.NotNil(t, resp)
		assert.Empty(t, resp.Interests)
		assert.Equal(t, "test-request-id", resp.Response.RequestId)
		assert.Equal(t, "error", resp.Response.Status)
		mockRepo.AssertExpectations(t)
	})
}

func TestInterestsService_CreateInterest(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mockRepo := new(MockInterestsRepository)
		service := createTestInterestsService(t, mockRepo)

		ctx := context.Background()
		userID := uuid.New().String()

		createdInterest := &types.Interest{
			ID:          uuid.New(),
			Name:        "New Interest",
			Description: stringPtr("New interest description"),
			Active:      boolPtr(true),
			Source:      "custom",
			CreatedAt:   time.Now(),
		}

		req := &pb.CreateInterestRequest{
			UserId: userID,
			Interest: &pb.CreateInterestParams{
				Name:        "New Interest",
				Description: "New interest description",
				Active:      true,
			},
			Request: &pb.BaseRequest{
				RequestId: "test-request-id",
			},
		}

		// Mock expectations
		mockRepo.On("CreateInterest", mock.Anything, req.Interest.Name, &req.Interest.Description, req.Interest.Active, userID).Return(createdInterest, nil).Once()

		resp, err := service.CreateInterest(ctx, req)

		assert.NoError(t, err)
		assert.NotNil(t, resp)
		assert.True(t, resp.Success)
		assert.Equal(t, "Interest created successfully", resp.Message)
		assert.NotNil(t, resp.Interest)
		assert.Equal(t, createdInterest.Name, resp.Interest.Name)
		assert.Equal(t, "test-request-id", resp.Response.RequestId)
		assert.Equal(t, "success", resp.Response.Status)
		mockRepo.AssertExpectations(t)
	})

	t.Run("RepositoryError", func(t *testing.T) {
		mockRepo := new(MockInterestsRepository)
		service := createTestInterestsService(t, mockRepo)

		ctx := context.Background()
		userID := uuid.New().String()

		req := &pb.CreateInterestRequest{
			UserId: userID,
			Interest: &pb.CreateInterestParams{
				Name:        "New Interest",
				Description: "New interest description",
				Active:      true,
			},
			Request: &pb.BaseRequest{
				RequestId: "test-request-id",
			},
		}

		// Mock expectations - repository error
		mockRepo.On("CreateInterest", mock.Anything, req.Interest.Name, &req.Interest.Description, req.Interest.Active, userID).Return(nil, errors.New("database error")).Once()

		resp, err := service.CreateInterest(ctx, req)

		assert.Error(t, err)
		assert.NotNil(t, resp)
		assert.False(t, resp.Success)
		assert.Equal(t, "Failed to create interest", resp.Message)
		assert.Nil(t, resp.Interest)
		assert.Equal(t, "test-request-id", resp.Response.RequestId)
		assert.Equal(t, "error", resp.Response.Status)
		mockRepo.AssertExpectations(t)
	})
}

func TestInterestsService_UpdateInterest(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mockRepo := new(MockInterestsRepository)
		service := createTestInterestsService(t, mockRepo)

		ctx := context.Background()
		userID := uuid.New()
		interestID := uuid.New()

		updatedInterest := &types.Interest{
			ID:          interestID,
			Name:        "Updated Interest",
			Description: stringPtr("Updated description"),
			Active:      boolPtr(true),
			Source:      "custom",
			CreatedAt:   time.Now(),
			UpdatedAt:   timePtr(time.Now()),
		}

		req := &pb.UpdateInterestRequest{
			UserId:     userID.String(),
			InterestId: interestID.String(),
			Interest: &pb.UpdateInterestParams{
				Name:        "Updated Interest",
				Description: "Updated description",
				Active:      true,
			},
			Request: &pb.BaseRequest{
				RequestId: "test-request-id",
			},
		}

		updateParams := types.UpdateinterestsParams{
			Name:        &req.Interest.Name,
			Description: &req.Interest.Description,
			Active:      &req.Interest.Active,
		}

		// Mock expectations
		mockRepo.On("UpdateInterests", mock.Anything, userID, interestID, updateParams).Return(nil).Once()
		mockRepo.On("GetInterest", mock.Anything, interestID).Return(updatedInterest, nil).Once()

		resp, err := service.UpdateInterest(ctx, req)

		assert.NoError(t, err)
		assert.NotNil(t, resp)
		assert.True(t, resp.Success)
		assert.Equal(t, "Interest updated successfully", resp.Message)
		assert.NotNil(t, resp.Interest)
		assert.Equal(t, updatedInterest.Name, resp.Interest.Name)
		assert.Equal(t, "test-request-id", resp.Response.RequestId)
		assert.Equal(t, "success", resp.Response.Status)
		mockRepo.AssertExpectations(t)
	})

	t.Run("InvalidUserID", func(t *testing.T) {
		mockRepo := new(MockInterestsRepository)
		service := createTestInterestsService(t, mockRepo)

		ctx := context.Background()

		req := &pb.UpdateInterestRequest{
			UserId:     "invalid-user-id",
			InterestId: uuid.New().String(),
			Interest: &pb.UpdateInterestParams{
				Name:        "Updated Interest",
				Description: "Updated description",
				Active:      true,
			},
			Request: &pb.BaseRequest{
				RequestId: "test-request-id",
			},
		}

		resp, err := service.UpdateInterest(ctx, req)

		assert.Error(t, err)
		assert.NotNil(t, resp)
		assert.False(t, resp.Success)
		assert.Equal(t, "Invalid user ID", resp.Message)
		assert.Nil(t, resp.Interest)
		assert.Equal(t, "test-request-id", resp.Response.RequestId)
		assert.Equal(t, "error", resp.Response.Status)
		mockRepo.AssertExpectations(t)
	})

	t.Run("InvalidInterestID", func(t *testing.T) {
		mockRepo := new(MockInterestsRepository)
		service := createTestInterestsService(t, mockRepo)

		ctx := context.Background()

		req := &pb.UpdateInterestRequest{
			UserId:     uuid.New().String(),
			InterestId: "invalid-interest-id",
			Interest: &pb.UpdateInterestParams{
				Name:        "Updated Interest",
				Description: "Updated description",
				Active:      true,
			},
			Request: &pb.BaseRequest{
				RequestId: "test-request-id",
			},
		}

		resp, err := service.UpdateInterest(ctx, req)

		assert.Error(t, err)
		assert.NotNil(t, resp)
		assert.False(t, resp.Success)
		assert.Equal(t, "Invalid interest ID", resp.Message)
		assert.Nil(t, resp.Interest)
		assert.Equal(t, "test-request-id", resp.Response.RequestId)
		assert.Equal(t, "error", resp.Response.Status)
		mockRepo.AssertExpectations(t)
	})

	t.Run("UpdateRepositoryError", func(t *testing.T) {
		mockRepo := new(MockInterestsRepository)
		service := createTestInterestsService(t, mockRepo)

		ctx := context.Background()
		userID := uuid.New()
		interestID := uuid.New()

		req := &pb.UpdateInterestRequest{
			UserId:     userID.String(),
			InterestId: interestID.String(),
			Interest: &pb.UpdateInterestParams{
				Name:        "Updated Interest",
				Description: "Updated description",
				Active:      true,
			},
			Request: &pb.BaseRequest{
				RequestId: "test-request-id",
			},
		}

		updateParams := types.UpdateinterestsParams{
			Name:        &req.Interest.Name,
			Description: &req.Interest.Description,
			Active:      &req.Interest.Active,
		}

		// Mock expectations - update fails
		mockRepo.On("UpdateInterests", mock.Anything, userID, interestID, updateParams).Return(errors.New("database error")).Once()

		resp, err := service.UpdateInterest(ctx, req)

		assert.Error(t, err)
		assert.NotNil(t, resp)
		assert.False(t, resp.Success)
		assert.Equal(t, "Failed to update interest", resp.Message)
		assert.Nil(t, resp.Interest)
		assert.Equal(t, "test-request-id", resp.Response.RequestId)
		assert.Equal(t, "error", resp.Response.Status)
		mockRepo.AssertExpectations(t)
	})

	t.Run("GetUpdatedInterestError", func(t *testing.T) {
		mockRepo := new(MockInterestsRepository)
		service := createTestInterestsService(t, mockRepo)

		ctx := context.Background()
		userID := uuid.New()
		interestID := uuid.New()

		req := &pb.UpdateInterestRequest{
			UserId:     userID.String(),
			InterestId: interestID.String(),
			Interest: &pb.UpdateInterestParams{
				Name:        "Updated Interest",
				Description: "Updated description",
				Active:      true,
			},
			Request: &pb.BaseRequest{
				RequestId: "test-request-id",
			},
		}

		updateParams := types.UpdateinterestsParams{
			Name:        &req.Interest.Name,
			Description: &req.Interest.Description,
			Active:      &req.Interest.Active,
		}

		// Mock expectations - update succeeds but get fails
		mockRepo.On("UpdateInterests", mock.Anything, userID, interestID, updateParams).Return(nil).Once()
		mockRepo.On("GetInterest", mock.Anything, interestID).Return(nil, errors.New("database error")).Once()

		resp, err := service.UpdateInterest(ctx, req)

		assert.Error(t, err)
		assert.NotNil(t, resp)
		assert.False(t, resp.Success)
		assert.Equal(t, "Interest updated but failed to retrieve", resp.Message)
		assert.Nil(t, resp.Interest)
		assert.Equal(t, "test-request-id", resp.Response.RequestId)
		assert.Equal(t, "error", resp.Response.Status)
		mockRepo.AssertExpectations(t)
	})
}

func TestInterestsService_RemoveInterest(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mockRepo := new(MockInterestsRepository)
		service := createTestInterestsService(t, mockRepo)

		ctx := context.Background()
		userID := uuid.New()
		interestID := uuid.New()

		req := &pb.RemoveInterestRequest{
			UserId:     userID.String(),
			InterestId: interestID.String(),
			Request: &pb.BaseRequest{
				RequestId: "test-request-id",
			},
		}

		// Mock expectations
		mockRepo.On("RemoveInterests", mock.Anything, userID, interestID).Return(nil).Once()

		resp, err := service.RemoveInterest(ctx, req)

		assert.NoError(t, err)
		assert.NotNil(t, resp)
		assert.True(t, resp.Success)
		assert.Equal(t, "Interest removed successfully", resp.Message)
		assert.Equal(t, "test-request-id", resp.Response.RequestId)
		assert.Equal(t, "success", resp.Response.Status)
		mockRepo.AssertExpectations(t)
	})

	t.Run("InvalidUserID", func(t *testing.T) {
		mockRepo := new(MockInterestsRepository)
		service := createTestInterestsService(t, mockRepo)

		ctx := context.Background()

		req := &pb.RemoveInterestRequest{
			UserId:     "invalid-user-id",
			InterestId: uuid.New().String(),
			Request: &pb.BaseRequest{
				RequestId: "test-request-id",
			},
		}

		resp, err := service.RemoveInterest(ctx, req)

		assert.Error(t, err)
		assert.NotNil(t, resp)
		assert.False(t, resp.Success)
		assert.Equal(t, "Invalid user ID", resp.Message)
		assert.Equal(t, "test-request-id", resp.Response.RequestId)
		assert.Equal(t, "error", resp.Response.Status)
		mockRepo.AssertExpectations(t)
	})

	t.Run("InvalidInterestID", func(t *testing.T) {
		mockRepo := new(MockInterestsRepository)
		service := createTestInterestsService(t, mockRepo)

		ctx := context.Background()

		req := &pb.RemoveInterestRequest{
			UserId:     uuid.New().String(),
			InterestId: "invalid-interest-id",
			Request: &pb.BaseRequest{
				RequestId: "test-request-id",
			},
		}

		resp, err := service.RemoveInterest(ctx, req)

		assert.Error(t, err)
		assert.NotNil(t, resp)
		assert.False(t, resp.Success)
		assert.Equal(t, "Invalid interest ID", resp.Message)
		assert.Equal(t, "test-request-id", resp.Response.RequestId)
		assert.Equal(t, "error", resp.Response.Status)
		mockRepo.AssertExpectations(t)
	})

	t.Run("RepositoryError", func(t *testing.T) {
		mockRepo := new(MockInterestsRepository)
		service := createTestInterestsService(t, mockRepo)

		ctx := context.Background()
		userID := uuid.New()
		interestID := uuid.New()

		req := &pb.RemoveInterestRequest{
			UserId:     userID.String(),
			InterestId: interestID.String(),
			Request: &pb.BaseRequest{
				RequestId: "test-request-id",
			},
		}

		// Mock expectations - repository error
		mockRepo.On("RemoveInterests", mock.Anything, userID, interestID).Return(errors.New("database error")).Once()

		resp, err := service.RemoveInterest(ctx, req)

		assert.Error(t, err)
		assert.NotNil(t, resp)
		assert.False(t, resp.Success)
		assert.Equal(t, "Failed to remove interest", resp.Message)
		assert.Equal(t, "test-request-id", resp.Response.RequestId)
		assert.Equal(t, "error", resp.Response.Status)
		mockRepo.AssertExpectations(t)
	})
}

// Helper functions to create pointers
func stringPtr(s string) *string {
	return &s
}

func boolPtr(b bool) *bool {
	return &b
}

func timePtr(t time.Time) *time.Time {
	return &t
}
