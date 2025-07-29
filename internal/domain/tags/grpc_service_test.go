package tags

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
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "github.com/FACorreiaa/loci-proto/modules/tags/generated"

	"github.com/FACorreiaa/go-poi-au-suggestions/app/observability/metrics"
	"github.com/FACorreiaa/go-poi-au-suggestions/internal/types"
	"github.com/FACorreiaa/go-poi-au-suggestions/protocol/grpc/middleware/grpcrequest"
)

// MockTagsRepository implements the Repository interface
type MockTagsRepository struct {
	mock.Mock
}

func (m *MockTagsRepository) GetAll(ctx context.Context, userID uuid.UUID) ([]*types.Tags, error) {
	args := m.Called(ctx, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*types.Tags), args.Error(1)
}

func (m *MockTagsRepository) Get(ctx context.Context, userID, tagID uuid.UUID) (*types.Tags, error) {
	args := m.Called(ctx, userID, tagID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*types.Tags), args.Error(1)
}

func (m *MockTagsRepository) Create(ctx context.Context, userID uuid.UUID, params types.CreatePersonalTagParams) (*types.PersonalTag, error) {
	args := m.Called(ctx, userID, params)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*types.PersonalTag), args.Error(1)
}

func (m *MockTagsRepository) Delete(ctx context.Context, userID uuid.UUID, tagID uuid.UUID) error {
	args := m.Called(ctx, userID, tagID)
	return args.Error(0)
}

func (m *MockTagsRepository) Update(ctx context.Context, userID, tagsID uuid.UUID, params types.UpdatePersonalTagParams) error {
	args := m.Called(ctx, userID, tagsID, params)
	return args.Error(0)
}

func (m *MockTagsRepository) GetTagByName(ctx context.Context, name string) (*types.Tags, error) {
	args := m.Called(ctx, name)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*types.Tags), args.Error(1)
}

func (m *MockTagsRepository) LinkPersonalTagToProfile(ctx context.Context, userID, profileID uuid.UUID, tagID uuid.UUID) error {
	args := m.Called(ctx, userID, profileID, tagID)
	return args.Error(0)
}

func (m *MockTagsRepository) GetTagsForProfile(ctx context.Context, profileID uuid.UUID) ([]*types.Tags, error) {
	args := m.Called(ctx, profileID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*types.Tags), args.Error(1)
}

func createTestTagsService(t *testing.T, mockRepo *MockTagsRepository) *Service {
	// Initialize metrics for testing
	metrics.InitAppMetrics()

	// Create a simple zap logger for testing
	config := zap.NewDevelopmentConfig()
	config.Level = zap.NewAtomicLevelAt(zapcore.DebugLevel)
	config.OutputPaths = []string{"stdout"}
	logger, _ := config.Build()

	// Create a mock pgxpool.Pool - for this test we don't actually use it
	var pgpool *pgxpool.Pool = nil

	return NewService(context.Background(), mockRepo, pgpool, logger)
}

func createContextWithUserID(userID string) context.Context {
	ctx := context.Background()
	ctx = context.WithValue(ctx, "userID", userID)
	ctx = context.WithValue(ctx, grpcrequest.RequestIDKey{}, "test-request-id")
	return ctx
}

func TestTagsService_GetTags(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mockRepo := new(MockTagsRepository)
		service := createTestTagsService(t, mockRepo)

		userID := uuid.New()
		ctx := createContextWithUserID(userID.String())

		mockTags := []*types.Tags{
			{
				ID:          uuid.New(),
				Name:        "Test Tag 1",
				TagType:     "category",
				Description: stringPtr("Test description"),
				Source:      stringPtr("global"),
				Active:      boolPtr(true),
				CreatedAt:   time.Now(),
			},
			{
				ID:          uuid.New(),
				Name:        "Test Tag 2",
				TagType:     "preference",
				Description: stringPtr("Another description"),
				Source:      stringPtr("personal"),
				Active:      boolPtr(true),
				CreatedAt:   time.Now(),
			},
		}

		req := &pb.GetTagsRequest{
			UserId:  userID.String(),
			Request: &pb.BaseRequest{},
		}

		// Mock expectations
		mockRepo.On("GetAll", mock.Anything, userID).Return(mockTags, nil).Once()

		// Call the gRPC method
		resp, err := service.GetTags(ctx, req)

		// Assertions
		assert.NoError(t, err)
		assert.NotNil(t, resp)
		assert.Len(t, resp.Tags, 2)
		assert.Equal(t, "test-request-id", resp.Response.RequestId)
		assert.Equal(t, "success", resp.Response.Status)
		assert.Equal(t, mockTags[0].Name, resp.Tags[0].Name)
		assert.Equal(t, mockTags[1].Name, resp.Tags[1].Name)
		mockRepo.AssertExpectations(t)
	})

	t.Run("MissingUserIDInContext", func(t *testing.T) {
		mockRepo := new(MockTagsRepository)
		service := createTestTagsService(t, mockRepo)

		ctx := context.WithValue(context.Background(), grpcrequest.RequestIDKey{}, "test-request-id")

		req := &pb.GetTagsRequest{
			UserId:  "some-user-id",
			Request: &pb.BaseRequest{},
		}

		resp, err := service.GetTags(ctx, req)

		assert.Error(t, err)
		assert.Nil(t, resp)
		grpcErr, ok := status.FromError(err)
		assert.True(t, ok)
		assert.Equal(t, codes.Unauthenticated, grpcErr.Code())
		assert.Contains(t, grpcErr.Message(), "userID is missing in metadata")
		mockRepo.AssertExpectations(t)
	})

	t.Run("InvalidUserIDFormat", func(t *testing.T) {
		mockRepo := new(MockTagsRepository)
		service := createTestTagsService(t, mockRepo)

		ctx := createContextWithUserID("invalid-uuid")

		req := &pb.GetTagsRequest{
			UserId:  "invalid-uuid",
			Request: &pb.BaseRequest{},
		}

		resp, err := service.GetTags(ctx, req)

		assert.Error(t, err)
		assert.NotNil(t, resp)
		assert.Empty(t, resp.Tags)
		grpcErr, ok := status.FromError(err)
		assert.True(t, ok)
		assert.Equal(t, codes.InvalidArgument, grpcErr.Code())
		assert.Contains(t, grpcErr.Message(), "invalid user ID format")
		mockRepo.AssertExpectations(t)
	})

	t.Run("RepositoryError", func(t *testing.T) {
		mockRepo := new(MockTagsRepository)
		service := createTestTagsService(t, mockRepo)

		userID := uuid.New()
		ctx := createContextWithUserID(userID.String())

		req := &pb.GetTagsRequest{
			UserId:  userID.String(),
			Request: &pb.BaseRequest{},
		}

		// Mock expectations - repository error
		mockRepo.On("GetAll", mock.Anything, userID).Return(nil, errors.New("database error")).Once()

		resp, err := service.GetTags(ctx, req)

		assert.Error(t, err)
		assert.NotNil(t, resp)
		assert.Empty(t, resp.Tags)
		grpcErr, ok := status.FromError(err)
		assert.True(t, ok)
		assert.Equal(t, codes.Internal, grpcErr.Code())
		assert.Contains(t, grpcErr.Message(), "failed to get tags")
		mockRepo.AssertExpectations(t)
	})

	t.Run("MissingRequestIDInContext", func(t *testing.T) {
		mockRepo := new(MockTagsRepository)
		service := createTestTagsService(t, mockRepo)

		userID := uuid.New()
		ctx := context.WithValue(context.Background(), "userID", userID.String())

		req := &pb.GetTagsRequest{
			UserId:  userID.String(),
			Request: &pb.BaseRequest{},
		}

		resp, err := service.GetTags(ctx, req)

		assert.Error(t, err)
		assert.Nil(t, resp)
		grpcErr, ok := status.FromError(err)
		assert.True(t, ok)
		assert.Equal(t, codes.Internal, grpcErr.Code())
		assert.Contains(t, grpcErr.Message(), "request id not found in context")
		mockRepo.AssertExpectations(t)
	})
}

func TestTagsService_GetTag(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mockRepo := new(MockTagsRepository)
		service := createTestTagsService(t, mockRepo)

		userID := uuid.New()
		tagID := uuid.New()
		ctx := createContextWithUserID(userID.String())

		mockTag := &types.Tags{
			ID:          tagID,
			Name:        "Test Tag",
			TagType:     "category",
			Description: stringPtr("Test description"),
			Source:      stringPtr("global"),
			Active:      boolPtr(true),
			CreatedAt:   time.Now(),
		}

		req := &pb.GetTagRequest{
			UserId:  userID.String(),
			TagId:   tagID.String(),
			Request: &pb.BaseRequest{},
		}

		// Mock expectations
		mockRepo.On("Get", mock.Anything, userID, tagID).Return(mockTag, nil).Once()

		resp, err := service.GetTag(ctx, req)

		assert.NoError(t, err)
		assert.NotNil(t, resp)
		assert.NotNil(t, resp.Tag)
		assert.Equal(t, "test-request-id", resp.Response.RequestId)
		assert.Equal(t, "success", resp.Response.Status)
		assert.Equal(t, mockTag.Name, resp.Tag.Name)
		assert.Equal(t, tagID.String(), resp.Tag.Id)
		mockRepo.AssertExpectations(t)
	})

	t.Run("TagNotFound", func(t *testing.T) {
		mockRepo := new(MockTagsRepository)
		service := createTestTagsService(t, mockRepo)

		userID := uuid.New()
		tagID := uuid.New()
		ctx := createContextWithUserID(userID.String())

		req := &pb.GetTagRequest{
			UserId:  userID.String(),
			TagId:   tagID.String(),
			Request: &pb.BaseRequest{},
		}

		// Mock expectations - tag not found
		mockRepo.On("Get", mock.Anything, userID, tagID).Return(nil, types.ErrNotFound).Once()

		resp, err := service.GetTag(ctx, req)

		assert.Error(t, err)
		assert.Nil(t, resp)
		grpcErr, ok := status.FromError(err)
		assert.True(t, ok)
		assert.Equal(t, codes.NotFound, grpcErr.Code())
		assert.Contains(t, grpcErr.Message(), "tag not found")
		mockRepo.AssertExpectations(t)
	})

	t.Run("InvalidTagIDFormat", func(t *testing.T) {
		mockRepo := new(MockTagsRepository)
		service := createTestTagsService(t, mockRepo)

		userID := uuid.New()
		ctx := createContextWithUserID(userID.String())

		req := &pb.GetTagRequest{
			UserId:  userID.String(),
			TagId:   "invalid-tag-id",
			Request: &pb.BaseRequest{},
		}

		resp, err := service.GetTag(ctx, req)

		assert.Error(t, err)
		assert.Nil(t, resp)
		grpcErr, ok := status.FromError(err)
		assert.True(t, ok)
		assert.Equal(t, codes.InvalidArgument, grpcErr.Code())
		assert.Contains(t, grpcErr.Message(), "invalid tag ID format")
		mockRepo.AssertExpectations(t)
	})

	t.Run("RepositoryError", func(t *testing.T) {
		mockRepo := new(MockTagsRepository)
		service := createTestTagsService(t, mockRepo)

		userID := uuid.New()
		tagID := uuid.New()
		ctx := createContextWithUserID(userID.String())

		req := &pb.GetTagRequest{
			UserId:  userID.String(),
			TagId:   tagID.String(),
			Request: &pb.BaseRequest{},
		}

		// Mock expectations - repository error
		mockRepo.On("Get", mock.Anything, userID, tagID).Return(nil, errors.New("database error")).Once()

		resp, err := service.GetTag(ctx, req)

		assert.Error(t, err)
		assert.Nil(t, resp)
		grpcErr, ok := status.FromError(err)
		assert.True(t, ok)
		assert.Equal(t, codes.Internal, grpcErr.Code())
		assert.Contains(t, grpcErr.Message(), "failed to get tag")
		mockRepo.AssertExpectations(t)
	})
}

func TestTagsService_CreateTag(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mockRepo := new(MockTagsRepository)
		service := createTestTagsService(t, mockRepo)

		userID := uuid.New()
		ctx := createContextWithUserID(userID.String())

		createdTag := &types.PersonalTag{
			ID:          uuid.New(),
			UserID:      userID,
			Name:        "New Tag",
			TagType:     "category",
			Description: stringPtr("New tag description"),
			Source:      "personal",
			CreatedAt:   time.Now(),
		}

		req := &pb.CreateTagRequest{
			UserId: userID.String(),
			Tag: &pb.CreatePersonalTagParams{
				Name:        "New Tag",
				TagType:     "category",
				Description: "New tag description",
				Active:      true,
			},
			Request: &pb.BaseRequest{},
		}

		active := req.Tag.Active
		expectedParams := types.CreatePersonalTagParams{
			Name:        "New Tag",
			Description: "New tag description",
			TagType:     "category",
			Active:      &active,
		}

		// Mock expectations
		mockRepo.On("Create", mock.Anything, userID, expectedParams).Return(createdTag, nil).Once()

		resp, err := service.CreateTag(ctx, req)

		assert.NoError(t, err)
		assert.NotNil(t, resp)
		assert.True(t, resp.Success)
		assert.Equal(t, "Tag created successfully", resp.Message)
		assert.NotNil(t, resp.Tag)
		assert.Equal(t, createdTag.Name, resp.Tag.Name)
		assert.Equal(t, "test-request-id", resp.Response.RequestId)
		assert.Equal(t, "success", resp.Response.Status)
		mockRepo.AssertExpectations(t)
	})

	t.Run("MissingTagData", func(t *testing.T) {
		mockRepo := new(MockTagsRepository)
		service := createTestTagsService(t, mockRepo)

		userID := uuid.New()
		ctx := createContextWithUserID(userID.String())

		req := &pb.CreateTagRequest{
			UserId:  userID.String(),
			Tag:     nil, // Missing tag data
			Request: &pb.BaseRequest{},
		}

		resp, err := service.CreateTag(ctx, req)

		assert.Error(t, err)
		assert.Nil(t, resp)
		grpcErr, ok := status.FromError(err)
		assert.True(t, ok)
		assert.Equal(t, codes.InvalidArgument, grpcErr.Code())
		assert.Contains(t, grpcErr.Message(), "tag data is required")
		mockRepo.AssertExpectations(t)
	})

	t.Run("RepositoryError", func(t *testing.T) {
		mockRepo := new(MockTagsRepository)
		service := createTestTagsService(t, mockRepo)

		userID := uuid.New()
		ctx := createContextWithUserID(userID.String())

		req := &pb.CreateTagRequest{
			UserId: userID.String(),
			Tag: &pb.CreatePersonalTagParams{
				Name:        "New Tag",
				TagType:     "category",
				Description: "New tag description",
				Active:      true,
			},
			Request: &pb.BaseRequest{},
		}

		active := req.Tag.Active
		expectedParams := types.CreatePersonalTagParams{
			Name:        "New Tag",
			Description: "New tag description",
			TagType:     "category",
			Active:      &active,
		}

		// Mock expectations - repository error
		mockRepo.On("Create", mock.Anything, userID, expectedParams).Return(nil, errors.New("database error")).Once()

		resp, err := service.CreateTag(ctx, req)

		assert.Error(t, err)
		assert.Nil(t, resp)
		grpcErr, ok := status.FromError(err)
		assert.True(t, ok)
		assert.Equal(t, codes.Internal, grpcErr.Code())
		assert.Contains(t, grpcErr.Message(), "failed to create tag")
		mockRepo.AssertExpectations(t)
	})
}

func TestTagsService_UpdateTag(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mockRepo := new(MockTagsRepository)
		service := createTestTagsService(t, mockRepo)

		userID := uuid.New()
		tagID := uuid.New()
		ctx := createContextWithUserID(userID.String())

		updatedTag := &types.Tags{
			ID:          tagID,
			Name:        "Updated Tag",
			TagType:     "preference",
			Description: stringPtr("Updated description"),
			Source:      stringPtr("personal"),
			Active:      boolPtr(true),
			CreatedAt:   time.Now(),
		}

		req := &pb.UpdateTagRequest{
			UserId: userID.String(),
			TagId:  tagID.String(),
			Tag: &pb.UpdatePersonalTagParams{
				Name:        "Updated Tag",
				TagType:     "preference",
				Description: "Updated description",
				Active:      true,
			},
			Request: &pb.BaseRequest{},
		}

		expectedParams := types.UpdatePersonalTagParams{
			Name:        "Updated Tag",
			Description: "Updated description",
			TagType:     "preference",
			Active:      true,
		}

		// Mock expectations
		mockRepo.On("Update", mock.Anything, userID, tagID, expectedParams).Return(nil).Once()
		mockRepo.On("Get", mock.Anything, userID, tagID).Return(updatedTag, nil).Once()

		resp, err := service.UpdateTag(ctx, req)

		assert.NoError(t, err)
		assert.NotNil(t, resp)
		assert.True(t, resp.Success)
		assert.Equal(t, "Tag updated successfully", resp.Message)
		assert.NotNil(t, resp.Tag)
		assert.Equal(t, updatedTag.Name, resp.Tag.Name)
		assert.Equal(t, "test-request-id", resp.Response.RequestId)
		mockRepo.AssertExpectations(t)
	})

	t.Run("TagNotFound", func(t *testing.T) {
		mockRepo := new(MockTagsRepository)
		service := createTestTagsService(t, mockRepo)

		userID := uuid.New()
		tagID := uuid.New()
		ctx := createContextWithUserID(userID.String())

		req := &pb.UpdateTagRequest{
			UserId: userID.String(),
			TagId:  tagID.String(),
			Tag: &pb.UpdatePersonalTagParams{
				Name:        "Updated Tag",
				TagType:     "preference",
				Description: "Updated description",
				Active:      true,
			},
			Request: &pb.BaseRequest{},
		}

		expectedParams := types.UpdatePersonalTagParams{
			Name:        "Updated Tag",
			Description: "Updated description",
			TagType:     "preference",
			Active:      true,
		}

		// Mock expectations - tag not found
		mockRepo.On("Update", mock.Anything, userID, tagID, expectedParams).Return(types.ErrNotFound).Once()

		resp, err := service.UpdateTag(ctx, req)

		assert.Error(t, err)
		assert.Nil(t, resp)
		grpcErr, ok := status.FromError(err)
		assert.True(t, ok)
		assert.Equal(t, codes.NotFound, grpcErr.Code())
		assert.Contains(t, grpcErr.Message(), "tag not found")
		mockRepo.AssertExpectations(t)
	})

	t.Run("MissingTagData", func(t *testing.T) {
		mockRepo := new(MockTagsRepository)
		service := createTestTagsService(t, mockRepo)

		userID := uuid.New()
		tagID := uuid.New()
		ctx := createContextWithUserID(userID.String())

		req := &pb.UpdateTagRequest{
			UserId:  userID.String(),
			TagId:   tagID.String(),
			Tag:     nil, // Missing tag data
			Request: &pb.BaseRequest{},
		}

		resp, err := service.UpdateTag(ctx, req)

		assert.Error(t, err)
		assert.Nil(t, resp)
		grpcErr, ok := status.FromError(err)
		assert.True(t, ok)
		assert.Equal(t, codes.InvalidArgument, grpcErr.Code())
		assert.Contains(t, grpcErr.Message(), "tag data is required")
		mockRepo.AssertExpectations(t)
	})

	t.Run("FailedToFetchUpdatedTag", func(t *testing.T) {
		mockRepo := new(MockTagsRepository)
		service := createTestTagsService(t, mockRepo)

		userID := uuid.New()
		tagID := uuid.New()
		ctx := createContextWithUserID(userID.String())

		req := &pb.UpdateTagRequest{
			UserId: userID.String(),
			TagId:  tagID.String(),
			Tag: &pb.UpdatePersonalTagParams{
				Name:        "Updated Tag",
				TagType:     "preference",
				Description: "Updated description",
				Active:      true,
			},
			Request: &pb.BaseRequest{},
		}

		expectedParams := types.UpdatePersonalTagParams{
			Name:        "Updated Tag",
			Description: "Updated description",
			TagType:     "preference",
			Active:      true,
		}

		// Mock expectations - update succeeds but fetch fails
		mockRepo.On("Update", mock.Anything, userID, tagID, expectedParams).Return(nil).Once()
		mockRepo.On("Get", mock.Anything, userID, tagID).Return(nil, errors.New("database error")).Once()

		resp, err := service.UpdateTag(ctx, req)

		assert.Error(t, err)
		assert.Nil(t, resp)
		grpcErr, ok := status.FromError(err)
		assert.True(t, ok)
		assert.Equal(t, codes.Internal, grpcErr.Code())
		assert.Contains(t, grpcErr.Message(), "failed to fetch updated tag")
		mockRepo.AssertExpectations(t)
	})
}

func TestTagsService_DeleteTag(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mockRepo := new(MockTagsRepository)
		service := createTestTagsService(t, mockRepo)

		userID := uuid.New()
		tagID := uuid.New()
		ctx := createContextWithUserID(userID.String())

		req := &pb.DeleteTagRequest{
			UserId:  userID.String(),
			TagId:   tagID.String(),
			Request: &pb.BaseRequest{},
		}

		// Mock expectations
		mockRepo.On("Delete", mock.Anything, userID, tagID).Return(nil).Once()

		resp, err := service.DeleteTag(ctx, req)

		assert.NoError(t, err)
		assert.NotNil(t, resp)
		assert.True(t, resp.Success)
		assert.Equal(t, "Tag deleted successfully", resp.Message)
		assert.Equal(t, "test-request-id", resp.Response.RequestId)
		assert.Equal(t, "success", resp.Response.Status)
		mockRepo.AssertExpectations(t)
	})

	t.Run("TagNotFound", func(t *testing.T) {
		mockRepo := new(MockTagsRepository)
		service := createTestTagsService(t, mockRepo)

		userID := uuid.New()
		tagID := uuid.New()
		ctx := createContextWithUserID(userID.String())

		req := &pb.DeleteTagRequest{
			UserId:  userID.String(),
			TagId:   tagID.String(),
			Request: &pb.BaseRequest{},
		}

		// Mock expectations - tag not found
		mockRepo.On("Delete", mock.Anything, userID, tagID).Return(types.ErrNotFound).Once()

		resp, err := service.DeleteTag(ctx, req)

		assert.Error(t, err)
		assert.Nil(t, resp)
		grpcErr, ok := status.FromError(err)
		assert.True(t, ok)
		assert.Equal(t, codes.NotFound, grpcErr.Code())
		assert.Contains(t, grpcErr.Message(), "tag not found")
		mockRepo.AssertExpectations(t)
	})

	t.Run("InvalidTagIDFormat", func(t *testing.T) {
		mockRepo := new(MockTagsRepository)
		service := createTestTagsService(t, mockRepo)

		userID := uuid.New()
		ctx := createContextWithUserID(userID.String())

		req := &pb.DeleteTagRequest{
			UserId:  userID.String(),
			TagId:   "invalid-tag-id",
			Request: &pb.BaseRequest{},
		}

		resp, err := service.DeleteTag(ctx, req)

		assert.Error(t, err)
		assert.Nil(t, resp)
		grpcErr, ok := status.FromError(err)
		assert.True(t, ok)
		assert.Equal(t, codes.InvalidArgument, grpcErr.Code())
		assert.Contains(t, grpcErr.Message(), "invalid tag ID format")
		mockRepo.AssertExpectations(t)
	})

	t.Run("RepositoryError", func(t *testing.T) {
		mockRepo := new(MockTagsRepository)
		service := createTestTagsService(t, mockRepo)

		userID := uuid.New()
		tagID := uuid.New()
		ctx := createContextWithUserID(userID.String())

		req := &pb.DeleteTagRequest{
			UserId:  userID.String(),
			TagId:   tagID.String(),
			Request: &pb.BaseRequest{},
		}

		// Mock expectations - repository error
		mockRepo.On("Delete", mock.Anything, userID, tagID).Return(errors.New("database error")).Once()

		resp, err := service.DeleteTag(ctx, req)

		assert.Error(t, err)
		assert.Nil(t, resp)
		grpcErr, ok := status.FromError(err)
		assert.True(t, ok)
		assert.Equal(t, codes.Internal, grpcErr.Code())
		assert.Contains(t, grpcErr.Message(), "failed to delete tag")
		mockRepo.AssertExpectations(t)
	})
}

// Helper function to create string pointers
func stringPtr(s string) *string {
	return &s
}

// Helper function to create bool pointers
func boolPtr(b bool) *bool {
	return &b
}
