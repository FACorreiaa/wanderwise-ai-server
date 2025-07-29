package lists

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
	"google.golang.org/protobuf/types/known/timestamppb"

	pb "github.com/FACorreiaa/loci-proto/modules/list/generated"

	"github.com/FACorreiaa/go-poi-au-suggestions/app/observability/metrics"
	"github.com/FACorreiaa/go-poi-au-suggestions/internal/domain"
	"github.com/FACorreiaa/go-poi-au-suggestions/protocol/grpc/middleware/grpcrequest"
)

// MockListsRepository implements the Repository interface
type MockListsRepository struct {
	mock.Mock
}

func (m *MockListsRepository) CreateList(ctx context.Context, list List) error {
	args := m.Called(ctx, list)
	return args.Error(0)
}

func (m *MockListsRepository) GetList(ctx context.Context, listID uuid.UUID) (List, error) {
	args := m.Called(ctx, listID)
	if args.Get(0) == nil {
		return List{}, args.Error(1)
	}
	return args.Get(0).(List), args.Error(1)
}

func (m *MockListsRepository) UpdateList(ctx context.Context, list List) error {
	args := m.Called(ctx, list)
	return args.Error(0)
}

func (m *MockListsRepository) DeleteList(ctx context.Context, listID uuid.UUID) error {
	args := m.Called(ctx, listID)
	return args.Error(0)
}

func (m *MockListsRepository) GetUserLists(ctx context.Context, userID uuid.UUID, isItinerary bool) ([]*List, error) {
	args := m.Called(ctx, userID, isItinerary)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*List), args.Error(1)
}

func (m *MockListsRepository) GetListItems(ctx context.Context, listID uuid.UUID) ([]*ListItem, error) {
	args := m.Called(ctx, listID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*ListItem), args.Error(1)
}

func (m *MockListsRepository) AddListItem(ctx context.Context, item ListItem) error {
	args := m.Called(ctx, item)
	return args.Error(0)
}

func (m *MockListsRepository) DeleteListItemByID(ctx context.Context, listID, itemID uuid.UUID) error {
	args := m.Called(ctx, listID, itemID)
	return args.Error(0)
}

func (m *MockListsRepository) GetSubLists(ctx context.Context, parentListID uuid.UUID) ([]*List, error) {
	args := m.Called(ctx, parentListID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*List), args.Error(1)
}

func (m *MockListsRepository) GetListItemByID(ctx context.Context, listID, itemID uuid.UUID) (ListItem, error) {
	args := m.Called(ctx, listID, itemID)
	if args.Get(0) == nil {
		return ListItem{}, args.Error(1)
	}
	return args.Get(0).(ListItem), args.Error(1)
}

func (m *MockListsRepository) SaveList(ctx context.Context, userID, listID uuid.UUID) error {
	args := m.Called(ctx, userID, listID)
	return args.Error(0)
}

func (m *MockListsRepository) UnsaveList(ctx context.Context, userID, listID uuid.UUID) error {
	args := m.Called(ctx, userID, listID)
	return args.Error(0)
}

func (m *MockListsRepository) GetUserSavedLists(ctx context.Context, userID uuid.UUID) ([]*List, error) {
	args := m.Called(ctx, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*List), args.Error(1)
}

func (m *MockListsRepository) GetListItemsByContentType(ctx context.Context, listID uuid.UUID, contentType ContentType) ([]*ListItem, error) {
	args := m.Called(ctx, listID, contentType)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*ListItem), args.Error(1)
}

func (m *MockListsRepository) SearchLists(ctx context.Context, searchTerm, category, contentType, theme string, cityID *uuid.UUID) ([]*List, error) {
	args := m.Called(ctx, searchTerm, category, contentType, theme, cityID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*List), args.Error(1)
}

func (m *MockListsRepository) GetListItem(ctx context.Context, listID, itemID uuid.UUID, contentType string) (ListItem, error) {
	args := m.Called(ctx, listID, itemID, contentType)
	if args.Get(0) == nil {
		return ListItem{}, args.Error(1)
	}
	return args.Get(0).(ListItem), args.Error(1)
}

func (m *MockListsRepository) UpdateListItem(ctx context.Context, item ListItem) error {
	args := m.Called(ctx, item)
	return args.Error(0)
}

func (m *MockListsRepository) DeleteListItem(ctx context.Context, listID, itemID uuid.UUID, contentType string) error {
	args := m.Called(ctx, listID, itemID, contentType)
	return args.Error(0)
}

func createTestListsService(t *testing.T, mockRepo *MockListsRepository) *Service {
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

func TestListsService_CreateList(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mockRepo := new(MockListsRepository)
		service := createTestListsService(t, mockRepo)

		userID := uuid.New()
		cityID := uuid.New()
		ctx := createContextWithUserID(userID.String())

		req := &pb.CreateListRequest{
			UserId:      userID.String(),
			Name:        "My Travel List",
			Description: "A list of places to visit",
			CityId:      cityID.String(),
			IsItinerary: false,
			IsPublic:    true,
			Request:     &pb.BaseRequest{},
		}

		// Mock expectations
		mockRepo.On("CreateList", mock.Anything, mock.MatchedBy(func(list List) bool {
			return list.Name == req.Name &&
				list.Description == req.Description &&
				list.IsPublic == req.IsPublic &&
				list.IsItinerary == req.IsItinerary &&
				list.UserID == userID &&
				list.CityID == cityID
		})).Return(nil).Once()

		// Call the gRPC method
		resp, err := service.CreateList(ctx, req)

		// Assertions
		assert.NoError(t, err)
		assert.NotNil(t, resp)
		assert.True(t, resp.Success)
		assert.Equal(t, "list created successfully", resp.Message)
		assert.NotNil(t, resp.List)
		assert.Equal(t, req.Name, resp.List.Name)
		assert.Equal(t, req.Description, resp.List.Description)
		assert.Equal(t, req.IsPublic, resp.List.IsPublic)
		assert.Equal(t, req.IsItinerary, resp.List.IsItinerary)
		assert.Equal(t, "test-request-id", resp.Response.RequestId)
		assert.Equal(t, "lists-service", resp.Response.Upstream)
		assert.Equal(t, "success", resp.Response.Status)
		mockRepo.AssertExpectations(t)
	})

	t.Run("MissingUserIDInContext", func(t *testing.T) {
		mockRepo := new(MockListsRepository)
		service := createTestListsService(t, mockRepo)

		ctx := context.WithValue(context.Background(), grpcrequest.RequestIDKey{}, "test-request-id")

		req := &pb.CreateListRequest{
			UserId:      "some-user-id",
			Name:        "My Travel List",
			Description: "A list of places to visit",
			Request:     &pb.BaseRequest{},
		}

		resp, err := service.CreateList(ctx, req)

		assert.Error(t, err)
		assert.Nil(t, resp)
		grpcErr, ok := status.FromError(err)
		assert.True(t, ok)
		assert.Equal(t, codes.Unauthenticated, grpcErr.Code())
		assert.Contains(t, grpcErr.Message(), "userID is missing in metadata")
		mockRepo.AssertExpectations(t)
	})

	t.Run("EmptyListName", func(t *testing.T) {
		mockRepo := new(MockListsRepository)
		service := createTestListsService(t, mockRepo)

		userID := uuid.New()
		ctx := createContextWithUserID(userID.String())

		req := &pb.CreateListRequest{
			UserId:      userID.String(),
			Name:        "", // Empty name
			Description: "A list of places to visit",
			Request:     &pb.BaseRequest{},
		}

		resp, err := service.CreateList(ctx, req)

		assert.Error(t, err)
		assert.NotNil(t, resp)
		assert.False(t, resp.Success)
		assert.Equal(t, "list name is required", resp.Message)
		grpcErr, ok := status.FromError(err)
		assert.True(t, ok)
		assert.Equal(t, codes.InvalidArgument, grpcErr.Code())
		assert.Contains(t, grpcErr.Message(), "list name is required")
		mockRepo.AssertExpectations(t)
	})

	t.Run("InvalidUserIDFormat", func(t *testing.T) {
		mockRepo := new(MockListsRepository)
		service := createTestListsService(t, mockRepo)

		ctx := createContextWithUserID("invalid-uuid")

		req := &pb.CreateListRequest{
			UserId:      "invalid-uuid",
			Name:        "My Travel List",
			Description: "A list of places to visit",
			Request:     &pb.BaseRequest{},
		}

		resp, err := service.CreateList(ctx, req)

		assert.Error(t, err)
		assert.NotNil(t, resp)
		assert.False(t, resp.Success)
		assert.Equal(t, "invalid user ID format", resp.Message)
		grpcErr, ok := status.FromError(err)
		assert.True(t, ok)
		assert.Equal(t, codes.InvalidArgument, grpcErr.Code())
		assert.Contains(t, grpcErr.Message(), "invalid user ID format")
		mockRepo.AssertExpectations(t)
	})

	t.Run("InvalidCityIDFormat", func(t *testing.T) {
		mockRepo := new(MockListsRepository)
		service := createTestListsService(t, mockRepo)

		userID := uuid.New()
		ctx := createContextWithUserID(userID.String())

		req := &pb.CreateListRequest{
			UserId:      userID.String(),
			Name:        "My Travel List",
			Description: "A list of places to visit",
			CityId:      "invalid-city-uuid",
			Request:     &pb.BaseRequest{},
		}

		resp, err := service.CreateList(ctx, req)

		assert.Error(t, err)
		assert.NotNil(t, resp)
		assert.False(t, resp.Success)
		assert.Equal(t, "invalid city ID format", resp.Message)
		grpcErr, ok := status.FromError(err)
		assert.True(t, ok)
		assert.Equal(t, codes.InvalidArgument, grpcErr.Code())
		assert.Contains(t, grpcErr.Message(), "invalid city ID format")
		mockRepo.AssertExpectations(t)
	})

	t.Run("RepositoryError", func(t *testing.T) {
		mockRepo := new(MockListsRepository)
		service := createTestListsService(t, mockRepo)

		userID := uuid.New()
		ctx := createContextWithUserID(userID.String())

		req := &pb.CreateListRequest{
			UserId:      userID.String(),
			Name:        "My Travel List",
			Description: "A list of places to visit",
			Request:     &pb.BaseRequest{},
		}

		// Mock expectations - repository error
		mockRepo.On("CreateList", mock.Anything, mock.AnythingOfType("List")).Return(errors.New("database error")).Once()

		resp, err := service.CreateList(ctx, req)

		assert.Error(t, err)
		assert.NotNil(t, resp)
		assert.False(t, resp.Success)
		assert.Equal(t, "failed to create list", resp.Message)
		grpcErr, ok := status.FromError(err)
		assert.True(t, ok)
		assert.Equal(t, codes.Internal, grpcErr.Code())
		assert.Contains(t, grpcErr.Message(), "failed to create list")
		mockRepo.AssertExpectations(t)
	})

	t.Run("MissingRequestIDInContext", func(t *testing.T) {
		mockRepo := new(MockListsRepository)
		service := createTestListsService(t, mockRepo)

		userID := uuid.New()
		ctx := context.WithValue(context.Background(), "userID", userID.String())

		req := &pb.CreateListRequest{
			UserId:      userID.String(),
			Name:        "My Travel List",
			Description: "A list of places to visit",
			Request:     &pb.BaseRequest{},
		}

		resp, err := service.CreateList(ctx, req)

		assert.Error(t, err)
		assert.Nil(t, resp)
		grpcErr, ok := status.FromError(err)
		assert.True(t, ok)
		assert.Equal(t, codes.Internal, grpcErr.Code())
		assert.Contains(t, grpcErr.Message(), "request id not found in context")
		mockRepo.AssertExpectations(t)
	})
}

func TestListsService_GetList(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mockRepo := new(MockListsRepository)
		service := createTestListsService(t, mockRepo)

		userID := uuid.New()
		listID := uuid.New()
		cityID := uuid.New()
		ctx := createContextWithUserID(userID.String())

		mockList := List{
			ID:          listID,
			UserID:      userID,
			Name:        "My Travel List",
			Description: "A list of places to visit",
			IsPublic:    true,
			IsItinerary: false,
			CityID:      cityID,
			ViewCount:   10,
			SaveCount:   5,
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}

		req := &pb.GetListRequest{
			UserId:               "any-user-id", // Will be overridden by auth
			ListId:               listID.String(),
			IncludeDetailedItems: false,
			Request:              &pb.BaseRequest{},
		}

		// Mock expectations
		mockRepo.On("GetList", mock.Anything, listID).Return(mockList, nil).Once()

		// Call the gRPC method
		resp, err := service.GetList(ctx, req)

		// Assertions
		assert.NoError(t, err)
		assert.NotNil(t, resp)
		assert.NotNil(t, resp.List)
		assert.NotNil(t, resp.List.List)
		assert.Equal(t, mockList.Name, resp.List.List.Name)
		assert.Equal(t, mockList.Description, resp.List.List.Description)
		assert.Equal(t, mockList.IsPublic, resp.List.List.IsPublic)
		assert.Equal(t, "test-request-id", resp.Response.RequestId)
		assert.Equal(t, "lists-service", resp.Response.Upstream)
		assert.Equal(t, "success", resp.Response.Status)
		mockRepo.AssertExpectations(t)
	})

	t.Run("SuccessWithDetailedItems", func(t *testing.T) {
		mockRepo := new(MockListsRepository)
		service := createTestListsService(t, mockRepo)

		userID := uuid.New()
		listID := uuid.New()
		cityID := uuid.New()
		itemID := uuid.New()
		ctx := createContextWithUserID(userID.String())

		mockList := List{
			ID:          listID,
			UserID:      userID,
			Name:        "My Travel List",
			Description: "A list of places to visit",
			IsPublic:    true,
			IsItinerary: false,
			CityID:      cityID,
			ViewCount:   10,
			SaveCount:   5,
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}

		mockItems := []*ListItem{
			{
				ListID:      listID,
				ItemID:      itemID,
				ContentType: ContentTypePOI,
				Position:    1,
				Notes:       "Great place to visit",
				CreatedAt:   time.Now(),
				UpdatedAt:   time.Now(),
			},
		}

		req := &pb.GetListRequest{
			UserId:               "any-user-id", // Will be overridden by auth
			ListId:               listID.String(),
			IncludeDetailedItems: true,
			Request:              &pb.BaseRequest{},
		}

		// Mock expectations
		mockRepo.On("GetList", mock.Anything, listID).Return(mockList, nil).Once()
		mockRepo.On("GetListItems", mock.Anything, listID).Return(mockItems, nil).Once()

		// Call the gRPC method
		resp, err := service.GetList(ctx, req)

		// Assertions
		assert.NoError(t, err)
		assert.NotNil(t, resp)
		assert.NotNil(t, resp.List)
		assert.NotNil(t, resp.List.List)
		assert.Len(t, resp.List.Items, 1)
		assert.Equal(t, mockList.Name, resp.List.List.Name)
		assert.Equal(t, mockItems[0].Notes, resp.List.Items[0].ListItem.Notes)
		mockRepo.AssertExpectations(t)
	})

	t.Run("ListNotFound", func(t *testing.T) {
		mockRepo := new(MockListsRepository)
		service := createTestListsService(t, mockRepo)

		userID := uuid.New()
		listID := uuid.New()
		ctx := createContextWithUserID(userID.String())

		req := &pb.GetListRequest{
			UserId:               "any-user-id",
			ListId:               listID.String(),
			IncludeDetailedItems: false,
			Request:              &pb.BaseRequest{},
		}

		// Mock expectations - list not found
		mockRepo.On("GetList", mock.Anything, listID).Return(List{}, domain.ErrNotFound).Once()

		resp, err := service.GetList(ctx, req)

		assert.Error(t, err)
		assert.NotNil(t, resp)
		assert.Nil(t, resp.List)
		grpcErr, ok := status.FromError(err)
		assert.True(t, ok)
		assert.Equal(t, codes.NotFound, grpcErr.Code())
		assert.Contains(t, grpcErr.Message(), "list not found")
		mockRepo.AssertExpectations(t)
	})

	t.Run("InvalidListIDFormat", func(t *testing.T) {
		mockRepo := new(MockListsRepository)
		service := createTestListsService(t, mockRepo)

		userID := uuid.New()
		ctx := createContextWithUserID(userID.String())

		req := &pb.GetListRequest{
			UserId:               "any-user-id",
			ListId:               "invalid-list-id",
			IncludeDetailedItems: false,
			Request:              &pb.BaseRequest{},
		}

		resp, err := service.GetList(ctx, req)

		assert.Error(t, err)
		assert.NotNil(t, resp)
		assert.Nil(t, resp.List)
		grpcErr, ok := status.FromError(err)
		assert.True(t, ok)
		assert.Equal(t, codes.InvalidArgument, grpcErr.Code())
		assert.Contains(t, grpcErr.Message(), "invalid list ID format")
		mockRepo.AssertExpectations(t)
	})

	t.Run("RepositoryError", func(t *testing.T) {
		mockRepo := new(MockListsRepository)
		service := createTestListsService(t, mockRepo)

		userID := uuid.New()
		listID := uuid.New()
		ctx := createContextWithUserID(userID.String())

		req := &pb.GetListRequest{
			UserId:               "any-user-id",
			ListId:               listID.String(),
			IncludeDetailedItems: false,
			Request:              &pb.BaseRequest{},
		}

		// Mock expectations - repository error
		mockRepo.On("GetList", mock.Anything, listID).Return(List{}, errors.New("database error")).Once()

		resp, err := service.GetList(ctx, req)

		assert.Error(t, err)
		assert.NotNil(t, resp)
		assert.Nil(t, resp.List)
		grpcErr, ok := status.FromError(err)
		assert.True(t, ok)
		assert.Equal(t, codes.Internal, grpcErr.Code())
		assert.Contains(t, grpcErr.Message(), "failed to get list")
		mockRepo.AssertExpectations(t)
	})

	t.Run("FailedToGetListItems", func(t *testing.T) {
		mockRepo := new(MockListsRepository)
		service := createTestListsService(t, mockRepo)

		userID := uuid.New()
		listID := uuid.New()
		cityID := uuid.New()
		ctx := createContextWithUserID(userID.String())

		mockList := List{
			ID:          listID,
			UserID:      userID,
			Name:        "My Travel List",
			Description: "A list of places to visit",
			IsPublic:    true,
			IsItinerary: false,
			CityID:      cityID,
			ViewCount:   10,
			SaveCount:   5,
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}

		req := &pb.GetListRequest{
			UserId:               "any-user-id",
			ListId:               listID.String(),
			IncludeDetailedItems: true,
			Request:              &pb.BaseRequest{},
		}

		// Mock expectations
		mockRepo.On("GetList", mock.Anything, listID).Return(mockList, nil).Once()
		mockRepo.On("GetListItems", mock.Anything, listID).Return(nil, errors.New("database error")).Once()

		resp, err := service.GetList(ctx, req)

		assert.Error(t, err)
		assert.NotNil(t, resp)
		assert.Nil(t, resp.List)
		grpcErr, ok := status.FromError(err)
		assert.True(t, ok)
		assert.Equal(t, codes.Internal, grpcErr.Code())
		assert.Contains(t, grpcErr.Message(), "failed to get list items")
		mockRepo.AssertExpectations(t)
	})
}

func TestListsService_GetLists(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mockRepo := new(MockListsRepository)
		service := createTestListsService(t, mockRepo)

		userID := uuid.New()
		listID1 := uuid.New()
		listID2 := uuid.New()
		cityID := uuid.New()
		ctx := createContextWithUserID(userID.String())

		mockRegularLists := []*List{
			{
				ID:          listID1,
				UserID:      userID,
				Name:        "Regular List 1",
				Description: "A regular list",
				IsPublic:    true,
				IsItinerary: false,
				CityID:      cityID,
				CreatedAt:   time.Now(),
				UpdatedAt:   time.Now(),
			},
		}

		mockItineraryLists := []*List{
			{
				ID:          listID2,
				UserID:      userID,
				Name:        "Itinerary List 1",
				Description: "An itinerary list",
				IsPublic:    false,
				IsItinerary: true,
				CityID:      cityID,
				CreatedAt:   time.Now(),
				UpdatedAt:   time.Now(),
			},
		}

		req := &pb.GetListsRequest{
			UserId:       "any-user-id", // Will be overridden by auth
			Limit:        10,
			Offset:       0,
			IncludeItems: false,
			Request:      &pb.BaseRequest{},
		}

		// Mock expectations
		mockRepo.On("GetUserLists", mock.Anything, userID, false).Return(mockRegularLists, nil).Once()
		mockRepo.On("GetUserLists", mock.Anything, userID, true).Return(mockItineraryLists, nil).Once()

		// Call the gRPC method
		resp, err := service.GetLists(ctx, req)

		// Assertions
		assert.NoError(t, err)
		assert.NotNil(t, resp)
		assert.Len(t, resp.Lists, 2)
		assert.Equal(t, int32(2), resp.TotalCount)
		assert.Equal(t, mockRegularLists[0].Name, resp.Lists[0].List.Name)
		assert.Equal(t, mockItineraryLists[0].Name, resp.Lists[1].List.Name)
		assert.Equal(t, "test-request-id", resp.Response.RequestId)
		assert.Equal(t, "lists-service", resp.Response.Upstream)
		assert.Equal(t, "success", resp.Response.Status)
		mockRepo.AssertExpectations(t)
	})

	t.Run("SuccessWithItems", func(t *testing.T) {
		mockRepo := new(MockListsRepository)
		service := createTestListsService(t, mockRepo)

		userID := uuid.New()
		listID := uuid.New()
		itemID := uuid.New()
		cityID := uuid.New()
		ctx := createContextWithUserID(userID.String())

		mockRegularLists := []*List{
			{
				ID:          listID,
				UserID:      userID,
				Name:        "Regular List 1",
				Description: "A regular list",
				IsPublic:    true,
				IsItinerary: false,
				CityID:      cityID,
				CreatedAt:   time.Now(),
				UpdatedAt:   time.Now(),
			},
		}

		mockItems := []*ListItem{
			{
				ListID:      listID,
				ItemID:      itemID,
				ContentType: ContentTypePOI,
				Position:    1,
				Notes:       "Great place",
				CreatedAt:   time.Now(),
				UpdatedAt:   time.Now(),
			},
		}

		req := &pb.GetListsRequest{
			UserId:       "any-user-id",
			Limit:        10,
			Offset:       0,
			IncludeItems: true,
			Request:      &pb.BaseRequest{},
		}

		// Mock expectations
		mockRepo.On("GetUserLists", mock.Anything, userID, false).Return(mockRegularLists, nil).Once()
		mockRepo.On("GetUserLists", mock.Anything, userID, true).Return([]*List{}, nil).Once()
		mockRepo.On("GetListItems", mock.Anything, listID).Return(mockItems, nil).Once()

		// Call the gRPC method
		resp, err := service.GetLists(ctx, req)

		// Assertions
		assert.NoError(t, err)
		assert.NotNil(t, resp)
		assert.Len(t, resp.Lists, 1)
		assert.Len(t, resp.Lists[0].Items, 1)
		assert.Equal(t, mockItems[0].Notes, resp.Lists[0].Items[0].Notes)
		mockRepo.AssertExpectations(t)
	})

	t.Run("MissingUserIDInContext", func(t *testing.T) {
		mockRepo := new(MockListsRepository)
		service := createTestListsService(t, mockRepo)

		ctx := context.WithValue(context.Background(), grpcrequest.RequestIDKey{}, "test-request-id")

		req := &pb.GetListsRequest{
			UserId:  "some-user-id",
			Request: &pb.BaseRequest{},
		}

		resp, err := service.GetLists(ctx, req)

		assert.Error(t, err)
		assert.Nil(t, resp)
		grpcErr, ok := status.FromError(err)
		assert.True(t, ok)
		assert.Equal(t, codes.Unauthenticated, grpcErr.Code())
		assert.Contains(t, grpcErr.Message(), "userID is missing in metadata")
		mockRepo.AssertExpectations(t)
	})

	t.Run("InvalidUserIDFormat", func(t *testing.T) {
		mockRepo := new(MockListsRepository)
		service := createTestListsService(t, mockRepo)

		ctx := createContextWithUserID("invalid-uuid")

		req := &pb.GetListsRequest{
			UserId:  "invalid-uuid",
			Request: &pb.BaseRequest{},
		}

		resp, err := service.GetLists(ctx, req)

		assert.Error(t, err)
		assert.NotNil(t, resp)
		assert.Nil(t, resp.Lists)
		grpcErr, ok := status.FromError(err)
		assert.True(t, ok)
		assert.Equal(t, codes.InvalidArgument, grpcErr.Code())
		assert.Contains(t, grpcErr.Message(), "invalid user ID format")
		mockRepo.AssertExpectations(t)
	})

	t.Run("RepositoryErrorRegularLists", func(t *testing.T) {
		mockRepo := new(MockListsRepository)
		service := createTestListsService(t, mockRepo)

		userID := uuid.New()
		ctx := createContextWithUserID(userID.String())

		req := &pb.GetListsRequest{
			UserId:  "any-user-id",
			Request: &pb.BaseRequest{},
		}

		// Mock expectations - repository error for regular lists
		mockRepo.On("GetUserLists", mock.Anything, userID, false).Return(nil, errors.New("database error")).Once()

		resp, err := service.GetLists(ctx, req)

		assert.Error(t, err)
		assert.NotNil(t, resp)
		assert.Nil(t, resp.Lists)
		grpcErr, ok := status.FromError(err)
		assert.True(t, ok)
		assert.Equal(t, codes.Internal, grpcErr.Code())
		assert.Contains(t, grpcErr.Message(), "failed to get regular lists")
		mockRepo.AssertExpectations(t)
	})

	t.Run("RepositoryErrorItineraryLists", func(t *testing.T) {
		mockRepo := new(MockListsRepository)
		service := createTestListsService(t, mockRepo)

		userID := uuid.New()
		ctx := createContextWithUserID(userID.String())

		req := &pb.GetListsRequest{
			UserId:  "any-user-id",
			Request: &pb.BaseRequest{},
		}

		// Mock expectations - success for regular lists, error for itinerary lists
		mockRepo.On("GetUserLists", mock.Anything, userID, false).Return([]*List{}, nil).Once()
		mockRepo.On("GetUserLists", mock.Anything, userID, true).Return(nil, errors.New("database error")).Once()

		resp, err := service.GetLists(ctx, req)

		assert.Error(t, err)
		assert.NotNil(t, resp)
		assert.Nil(t, resp.Lists)
		grpcErr, ok := status.FromError(err)
		assert.True(t, ok)
		assert.Equal(t, codes.Internal, grpcErr.Code())
		assert.Contains(t, grpcErr.Message(), "failed to get itinerary lists")
		mockRepo.AssertExpectations(t)
	})
}

func TestListsService_UpdateList(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mockRepo := new(MockListsRepository)
		service := createTestListsService(t, mockRepo)

		userID := uuid.New()
		listID := uuid.New()
		cityID := uuid.New()
		ctx := createContextWithUserID(userID.String())

		existingList := List{
			ID:          listID,
			UserID:      userID,
			Name:        "Old Name",
			Description: "Old Description",
			IsPublic:    false,
			IsItinerary: false,
			CityID:      cityID,
			ViewCount:   10,
			SaveCount:   5,
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}

		req := &pb.UpdateListRequest{
			UserId:      "any-user-id", // Will be overridden by auth
			ListId:      listID.String(),
			Name:        "New Name",
			Description: "New Description",
			ImageUrl:    "https://example.com/image.jpg",
			IsPublic:    true,
			CityId:      cityID.String(),
			Request:     &pb.BaseRequest{},
		}

		// Mock expectations
		mockRepo.On("GetList", mock.Anything, listID).Return(existingList, nil).Once()
		mockRepo.On("UpdateList", mock.Anything, mock.MatchedBy(func(list List) bool {
			return list.ID == listID &&
				list.Name == req.Name &&
				list.Description == req.Description &&
				list.ImageURL == req.ImageUrl &&
				list.IsPublic == req.IsPublic &&
				list.CityID == cityID
		})).Return(nil).Once()

		// Call the gRPC method
		resp, err := service.UpdateList(ctx, req)

		// Assertions
		assert.NoError(t, err)
		assert.NotNil(t, resp)
		assert.True(t, resp.Success)
		assert.Equal(t, "list updated successfully", resp.Message)
		assert.NotNil(t, resp.List)
		assert.Equal(t, req.Name, resp.List.Name)
		assert.Equal(t, req.Description, resp.List.Description)
		assert.Equal(t, req.IsPublic, resp.List.IsPublic)
		assert.Equal(t, "test-request-id", resp.Response.RequestId)
		assert.Equal(t, "lists-service", resp.Response.Upstream)
		assert.Equal(t, "success", resp.Response.Status)
		mockRepo.AssertExpectations(t)
	})

	t.Run("ListNotFound", func(t *testing.T) {
		mockRepo := new(MockListsRepository)
		service := createTestListsService(t, mockRepo)

		userID := uuid.New()
		listID := uuid.New()
		ctx := createContextWithUserID(userID.String())

		req := &pb.UpdateListRequest{
			UserId:  "any-user-id",
			ListId:  listID.String(),
			Name:    "New Name",
			Request: &pb.BaseRequest{},
		}

		// Mock expectations - list not found
		mockRepo.On("GetList", mock.Anything, listID).Return(List{}, domain.ErrNotFound).Once()

		resp, err := service.UpdateList(ctx, req)

		assert.Error(t, err)
		assert.NotNil(t, resp)
		assert.False(t, resp.Success)
		assert.Equal(t, "list not found", resp.Message)
		grpcErr, ok := status.FromError(err)
		assert.True(t, ok)
		assert.Equal(t, codes.NotFound, grpcErr.Code())
		assert.Contains(t, grpcErr.Message(), "list not found")
		mockRepo.AssertExpectations(t)
	})

	t.Run("UnauthorizedUser", func(t *testing.T) {
		mockRepo := new(MockListsRepository)
		service := createTestListsService(t, mockRepo)

		userID := uuid.New()
		otherUserID := uuid.New()
		listID := uuid.New()
		cityID := uuid.New()
		ctx := createContextWithUserID(userID.String())

		existingList := List{
			ID:          listID,
			UserID:      otherUserID, // Different user
			Name:        "Old Name",
			Description: "Old Description",
			IsPublic:    false,
			IsItinerary: false,
			CityID:      cityID,
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}

		req := &pb.UpdateListRequest{
			UserId:  "any-user-id",
			ListId:  listID.String(),
			Name:    "New Name",
			Request: &pb.BaseRequest{},
		}

		// Mock expectations
		mockRepo.On("GetList", mock.Anything, listID).Return(existingList, nil).Once()

		resp, err := service.UpdateList(ctx, req)

		assert.Error(t, err)
		assert.NotNil(t, resp)
		assert.False(t, resp.Success)
		assert.Equal(t, "unauthorized", resp.Message)
		grpcErr, ok := status.FromError(err)
		assert.True(t, ok)
		assert.Equal(t, codes.PermissionDenied, grpcErr.Code())
		assert.Contains(t, grpcErr.Message(), "unauthorized")
		mockRepo.AssertExpectations(t)
	})

	t.Run("InvalidListIDFormat", func(t *testing.T) {
		mockRepo := new(MockListsRepository)
		service := createTestListsService(t, mockRepo)

		userID := uuid.New()
		ctx := createContextWithUserID(userID.String())

		req := &pb.UpdateListRequest{
			UserId:  "any-user-id",
			ListId:  "invalid-list-id",
			Name:    "New Name",
			Request: &pb.BaseRequest{},
		}

		resp, err := service.UpdateList(ctx, req)

		assert.Error(t, err)
		assert.NotNil(t, resp)
		assert.False(t, resp.Success)
		assert.Equal(t, "invalid list ID format", resp.Message)
		grpcErr, ok := status.FromError(err)
		assert.True(t, ok)
		assert.Equal(t, codes.InvalidArgument, grpcErr.Code())
		assert.Contains(t, grpcErr.Message(), "invalid list ID format")
		mockRepo.AssertExpectations(t)
	})

	t.Run("RepositoryUpdateError", func(t *testing.T) {
		mockRepo := new(MockListsRepository)
		service := createTestListsService(t, mockRepo)

		userID := uuid.New()
		listID := uuid.New()
		cityID := uuid.New()
		ctx := createContextWithUserID(userID.String())

		existingList := List{
			ID:          listID,
			UserID:      userID,
			Name:        "Old Name",
			Description: "Old Description",
			IsPublic:    false,
			IsItinerary: false,
			CityID:      cityID,
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}

		req := &pb.UpdateListRequest{
			UserId:  "any-user-id",
			ListId:  listID.String(),
			Name:    "New Name",
			Request: &pb.BaseRequest{},
		}

		// Mock expectations
		mockRepo.On("GetList", mock.Anything, listID).Return(existingList, nil).Once()
		mockRepo.On("UpdateList", mock.Anything, mock.AnythingOfType("List")).Return(errors.New("database error")).Once()

		resp, err := service.UpdateList(ctx, req)

		assert.Error(t, err)
		assert.NotNil(t, resp)
		assert.False(t, resp.Success)
		assert.Equal(t, "failed to update list", resp.Message)
		grpcErr, ok := status.FromError(err)
		assert.True(t, ok)
		assert.Equal(t, codes.Internal, grpcErr.Code())
		assert.Contains(t, grpcErr.Message(), "failed to update list")
		mockRepo.AssertExpectations(t)
	})
}

func TestListsService_DeleteList(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mockRepo := new(MockListsRepository)
		service := createTestListsService(t, mockRepo)

		userID := uuid.New()
		listID := uuid.New()
		cityID := uuid.New()
		ctx := createContextWithUserID(userID.String())

		existingList := List{
			ID:          listID,
			UserID:      userID,
			Name:        "My List",
			Description: "A list to delete",
			IsPublic:    false,
			IsItinerary: false,
			CityID:      cityID,
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}

		req := &pb.DeleteListRequest{
			UserId:  "any-user-id", // Will be overridden by auth
			ListId:  listID.String(),
			Request: &pb.BaseRequest{},
		}

		// Mock expectations
		mockRepo.On("GetList", mock.Anything, listID).Return(existingList, nil).Once()
		mockRepo.On("DeleteList", mock.Anything, listID).Return(nil).Once()

		// Call the gRPC method
		resp, err := service.DeleteList(ctx, req)

		// Assertions
		assert.NoError(t, err)
		assert.NotNil(t, resp)
		assert.True(t, resp.Success)
		assert.Equal(t, "list deleted successfully", resp.Message)
		assert.Equal(t, "test-request-id", resp.Response.RequestId)
		assert.Equal(t, "lists-service", resp.Response.Upstream)
		assert.Equal(t, "success", resp.Response.Status)
		mockRepo.AssertExpectations(t)
	})

	t.Run("ListNotFound", func(t *testing.T) {
		mockRepo := new(MockListsRepository)
		service := createTestListsService(t, mockRepo)

		userID := uuid.New()
		listID := uuid.New()
		ctx := createContextWithUserID(userID.String())

		req := &pb.DeleteListRequest{
			UserId:  "any-user-id",
			ListId:  listID.String(),
			Request: &pb.BaseRequest{},
		}

		// Mock expectations - list not found
		mockRepo.On("GetList", mock.Anything, listID).Return(List{}, domain.ErrNotFound).Once()

		resp, err := service.DeleteList(ctx, req)

		assert.Error(t, err)
		assert.NotNil(t, resp)
		assert.False(t, resp.Success)
		assert.Equal(t, "list not found", resp.Message)
		grpcErr, ok := status.FromError(err)
		assert.True(t, ok)
		assert.Equal(t, codes.NotFound, grpcErr.Code())
		assert.Contains(t, grpcErr.Message(), "list not found")
		mockRepo.AssertExpectations(t)
	})

	t.Run("UnauthorizedUser", func(t *testing.T) {
		mockRepo := new(MockListsRepository)
		service := createTestListsService(t, mockRepo)

		userID := uuid.New()
		otherUserID := uuid.New()
		listID := uuid.New()
		cityID := uuid.New()
		ctx := createContextWithUserID(userID.String())

		existingList := List{
			ID:          listID,
			UserID:      otherUserID, // Different user
			Name:        "My List",
			Description: "A list to delete",
			IsPublic:    false,
			IsItinerary: false,
			CityID:      cityID,
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}

		req := &pb.DeleteListRequest{
			UserId:  "any-user-id",
			ListId:  listID.String(),
			Request: &pb.BaseRequest{},
		}

		// Mock expectations
		mockRepo.On("GetList", mock.Anything, listID).Return(existingList, nil).Once()

		resp, err := service.DeleteList(ctx, req)

		assert.Error(t, err)
		assert.NotNil(t, resp)
		assert.False(t, resp.Success)
		assert.Equal(t, "unauthorized", resp.Message)
		grpcErr, ok := status.FromError(err)
		assert.True(t, ok)
		assert.Equal(t, codes.PermissionDenied, grpcErr.Code())
		assert.Contains(t, grpcErr.Message(), "unauthorized")
		mockRepo.AssertExpectations(t)
	})

	t.Run("InvalidListIDFormat", func(t *testing.T) {
		mockRepo := new(MockListsRepository)
		service := createTestListsService(t, mockRepo)

		userID := uuid.New()
		ctx := createContextWithUserID(userID.String())

		req := &pb.DeleteListRequest{
			UserId:  "any-user-id",
			ListId:  "invalid-list-id",
			Request: &pb.BaseRequest{},
		}

		resp, err := service.DeleteList(ctx, req)

		assert.Error(t, err)
		assert.NotNil(t, resp)
		assert.False(t, resp.Success)
		assert.Equal(t, "invalid list ID format", resp.Message)
		grpcErr, ok := status.FromError(err)
		assert.True(t, ok)
		assert.Equal(t, codes.InvalidArgument, grpcErr.Code())
		assert.Contains(t, grpcErr.Message(), "invalid list ID format")
		mockRepo.AssertExpectations(t)
	})

	t.Run("RepositoryDeleteError", func(t *testing.T) {
		mockRepo := new(MockListsRepository)
		service := createTestListsService(t, mockRepo)

		userID := uuid.New()
		listID := uuid.New()
		cityID := uuid.New()
		ctx := createContextWithUserID(userID.String())

		existingList := List{
			ID:          listID,
			UserID:      userID,
			Name:        "My List",
			Description: "A list to delete",
			IsPublic:    false,
			IsItinerary: false,
			CityID:      cityID,
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}

		req := &pb.DeleteListRequest{
			UserId:  "any-user-id",
			ListId:  listID.String(),
			Request: &pb.BaseRequest{},
		}

		// Mock expectations
		mockRepo.On("GetList", mock.Anything, listID).Return(existingList, nil).Once()
		mockRepo.On("DeleteList", mock.Anything, listID).Return(errors.New("database error")).Once()

		resp, err := service.DeleteList(ctx, req)

		assert.Error(t, err)
		assert.NotNil(t, resp)
		assert.False(t, resp.Success)
		assert.Equal(t, "failed to delete list", resp.Message)
		grpcErr, ok := status.FromError(err)
		assert.True(t, ok)
		assert.Equal(t, codes.Internal, grpcErr.Code())
		assert.Contains(t, grpcErr.Message(), "failed to delete list")
		mockRepo.AssertExpectations(t)
	})
}

func TestListsService_AddListItem(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mockRepo := new(MockListsRepository)
		service := createTestListsService(t, mockRepo)

		userID := uuid.New()
		listID := uuid.New()
		itemID := uuid.New()
		cityID := uuid.New()
		ctx := createContextWithUserID(userID.String())

		existingList := List{
			ID:          listID,
			UserID:      userID,
			Name:        "My List",
			Description: "A list",
			IsPublic:    false,
			IsItinerary: false,
			CityID:      cityID,
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}

		req := &pb.AddListItemRequest{
			UserId:             "any-user-id", // Will be overridden by auth
			ListId:             listID.String(),
			ItemId:             itemID.String(),
			ContentType:        pb.ContentType_CONTENT_TYPE_POI,
			Position:           1,
			Notes:              "Great place to visit",
			DayNumber:          2,
			DurationMinutes:    120,
			ItemAiDescription:  "AI generated description",
			Request:            &pb.BaseRequest{},
		}

		// Mock expectations
		mockRepo.On("GetList", mock.Anything, listID).Return(existingList, nil).Once()
		mockRepo.On("AddListItem", mock.Anything, mock.MatchedBy(func(item ListItem) bool {
			return item.ListID == listID &&
				item.ItemID == itemID &&
				item.ContentType == ContentTypePOI &&
				item.Position == int(req.Position) &&
				item.Notes == req.Notes &&
				*item.DayNumber == int(req.DayNumber) &&
				*item.Duration == int(req.DurationMinutes) &&
				item.ItemAIDescription == req.ItemAiDescription
		})).Return(nil).Once()

		// Call the gRPC method
		resp, err := service.AddListItem(ctx, req)

		// Assertions
		assert.NoError(t, err)
		assert.NotNil(t, resp)
		assert.True(t, resp.Success)
		assert.Equal(t, "list item added successfully", resp.Message)
		assert.NotNil(t, resp.Item)
		assert.Equal(t, listID.String(), resp.Item.ListId)
		assert.Equal(t, itemID.String(), resp.Item.ItemId)
		assert.Equal(t, pb.ContentType_CONTENT_TYPE_POI, resp.Item.ContentType)
		assert.Equal(t, req.Position, resp.Item.Position)
		assert.Equal(t, req.Notes, resp.Item.Notes)
		assert.Equal(t, "test-request-id", resp.Response.RequestId)
		assert.Equal(t, "lists-service", resp.Response.Upstream)
		assert.Equal(t, "success", resp.Response.Status)
		mockRepo.AssertExpectations(t)
	})

	t.Run("SuccessWithTimeSlot", func(t *testing.T) {
		mockRepo := new(MockListsRepository)
		service := createTestListsService(t, mockRepo)

		userID := uuid.New()
		listID := uuid.New()
		itemID := uuid.New()
		cityID := uuid.New()
		ctx := createContextWithUserID(userID.String())

		existingList := List{
			ID:          listID,
			UserID:      userID,
			Name:        "My List",
			Description: "A list",
			IsPublic:    false,
			IsItinerary: false,
			CityID:      cityID,
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}

		timeSlot := time.Now().Add(2 * time.Hour)
		timeSlotPb := timestamppb.New(timeSlot)

		req := &pb.AddListItemRequest{
			UserId:      "any-user-id",
			ListId:      listID.String(),
			ItemId:      itemID.String(),
			ContentType: pb.ContentType_CONTENT_TYPE_RESTAURANT,
			Position:    1,
			Notes:       "Great restaurant",
			TimeSlot:    timeSlotPb,
			Request:     &pb.BaseRequest{},
		}

		// Mock expectations
		mockRepo.On("GetList", mock.Anything, listID).Return(existingList, nil).Once()
		mockRepo.On("AddListItem", mock.Anything, mock.MatchedBy(func(item ListItem) bool {
			return item.ListID == listID &&
				item.ItemID == itemID &&
				item.ContentType == ContentTypeRestaurant &&
				item.Position == int(req.Position) &&
				item.Notes == req.Notes &&
				item.TimeSlot != nil &&
				item.TimeSlot.Equal(timeSlot)
		})).Return(nil).Once()

		// Call the gRPC method
		resp, err := service.AddListItem(ctx, req)

		// Assertions
		assert.NoError(t, err)
		assert.NotNil(t, resp)
		assert.True(t, resp.Success)
		assert.Equal(t, "list item added successfully", resp.Message)
		assert.NotNil(t, resp.Item)
		assert.Equal(t, pb.ContentType_CONTENT_TYPE_RESTAURANT, resp.Item.ContentType)
		assert.NotNil(t, resp.Item.TimeSlot)
		mockRepo.AssertExpectations(t)
	})

	t.Run("ListNotFound", func(t *testing.T) {
		mockRepo := new(MockListsRepository)
		service := createTestListsService(t, mockRepo)

		userID := uuid.New()
		listID := uuid.New()
		itemID := uuid.New()
		ctx := createContextWithUserID(userID.String())

		req := &pb.AddListItemRequest{
			UserId:      "any-user-id",
			ListId:      listID.String(),
			ItemId:      itemID.String(),
			ContentType: pb.ContentType_CONTENT_TYPE_POI,
			Position:    1,
			Request:     &pb.BaseRequest{},
		}

		// Mock expectations - list not found
		mockRepo.On("GetList", mock.Anything, listID).Return(List{}, domain.ErrNotFound).Once()

		resp, err := service.AddListItem(ctx, req)

		assert.Error(t, err)
		assert.NotNil(t, resp)
		assert.False(t, resp.Success)
		assert.Equal(t, "list not found", resp.Message)
		grpcErr, ok := status.FromError(err)
		assert.True(t, ok)
		assert.Equal(t, codes.NotFound, grpcErr.Code())
		assert.Contains(t, grpcErr.Message(), "list not found")
		mockRepo.AssertExpectations(t)
	})

	t.Run("UnauthorizedUser", func(t *testing.T) {
		mockRepo := new(MockListsRepository)
		service := createTestListsService(t, mockRepo)

		userID := uuid.New()
		otherUserID := uuid.New()
		listID := uuid.New()
		itemID := uuid.New()
		cityID := uuid.New()
		ctx := createContextWithUserID(userID.String())

		existingList := List{
			ID:          listID,
			UserID:      otherUserID, // Different user
			Name:        "My List",
			Description: "A list",
			IsPublic:    false,
			IsItinerary: false,
			CityID:      cityID,
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}

		req := &pb.AddListItemRequest{
			UserId:      "any-user-id",
			ListId:      listID.String(),
			ItemId:      itemID.String(),
			ContentType: pb.ContentType_CONTENT_TYPE_POI,
			Position:    1,
			Request:     &pb.BaseRequest{},
		}

		// Mock expectations
		mockRepo.On("GetList", mock.Anything, listID).Return(existingList, nil).Once()

		resp, err := service.AddListItem(ctx, req)

		assert.Error(t, err)
		assert.NotNil(t, resp)
		assert.False(t, resp.Success)
		assert.Equal(t, "unauthorized", resp.Message)
		grpcErr, ok := status.FromError(err)
		assert.True(t, ok)
		assert.Equal(t, codes.PermissionDenied, grpcErr.Code())
		assert.Contains(t, grpcErr.Message(), "unauthorized")
		mockRepo.AssertExpectations(t)
	})

	t.Run("InvalidListIDFormat", func(t *testing.T) {
		mockRepo := new(MockListsRepository)
		service := createTestListsService(t, mockRepo)

		userID := uuid.New()
		itemID := uuid.New()
		ctx := createContextWithUserID(userID.String())

		req := &pb.AddListItemRequest{
			UserId:      "any-user-id",
			ListId:      "invalid-list-id",
			ItemId:      itemID.String(),
			ContentType: pb.ContentType_CONTENT_TYPE_POI,
			Position:    1,
			Request:     &pb.BaseRequest{},
		}

		resp, err := service.AddListItem(ctx, req)

		assert.Error(t, err)
		assert.NotNil(t, resp)
		assert.False(t, resp.Success)
		assert.Equal(t, "invalid list ID format", resp.Message)
		grpcErr, ok := status.FromError(err)
		assert.True(t, ok)
		assert.Equal(t, codes.InvalidArgument, grpcErr.Code())
		assert.Contains(t, grpcErr.Message(), "invalid list ID format")
		mockRepo.AssertExpectations(t)
	})

	t.Run("InvalidItemIDFormat", func(t *testing.T) {
		mockRepo := new(MockListsRepository)
		service := createTestListsService(t, mockRepo)

		userID := uuid.New()
		listID := uuid.New()
		ctx := createContextWithUserID(userID.String())

		req := &pb.AddListItemRequest{
			UserId:      "any-user-id",
			ListId:      listID.String(),
			ItemId:      "invalid-item-id",
			ContentType: pb.ContentType_CONTENT_TYPE_POI,
			Position:    1,
			Request:     &pb.BaseRequest{},
		}

		resp, err := service.AddListItem(ctx, req)

		assert.Error(t, err)
		assert.NotNil(t, resp)
		assert.False(t, resp.Success)
		assert.Equal(t, "invalid item ID format", resp.Message)
		grpcErr, ok := status.FromError(err)
		assert.True(t, ok)
		assert.Equal(t, codes.InvalidArgument, grpcErr.Code())
		assert.Contains(t, grpcErr.Message(), "invalid item ID format")
		mockRepo.AssertExpectations(t)
	})

	t.Run("RepositoryAddError", func(t *testing.T) {
		mockRepo := new(MockListsRepository)
		service := createTestListsService(t, mockRepo)

		userID := uuid.New()
		listID := uuid.New()
		itemID := uuid.New()
		cityID := uuid.New()
		ctx := createContextWithUserID(userID.String())

		existingList := List{
			ID:          listID,
			UserID:      userID,
			Name:        "My List",
			Description: "A list",
			IsPublic:    false,
			IsItinerary: false,
			CityID:      cityID,
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}

		req := &pb.AddListItemRequest{
			UserId:      "any-user-id",
			ListId:      listID.String(),
			ItemId:      itemID.String(),
			ContentType: pb.ContentType_CONTENT_TYPE_POI,
			Position:    1,
			Request:     &pb.BaseRequest{},
		}

		// Mock expectations
		mockRepo.On("GetList", mock.Anything, listID).Return(existingList, nil).Once()
		mockRepo.On("AddListItem", mock.Anything, mock.AnythingOfType("ListItem")).Return(errors.New("database error")).Once()

		resp, err := service.AddListItem(ctx, req)

		assert.Error(t, err)
		assert.NotNil(t, resp)
		assert.False(t, resp.Success)
		assert.Equal(t, "failed to add list item", resp.Message)
		grpcErr, ok := status.FromError(err)
		assert.True(t, ok)
		assert.Equal(t, codes.Internal, grpcErr.Code())
		assert.Contains(t, grpcErr.Message(), "failed to add list item")
		mockRepo.AssertExpectations(t)
	})
}

func TestListsService_GetListItems(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mockRepo := new(MockListsRepository)
		service := createTestListsService(t, mockRepo)

		userID := uuid.New()
		listID := uuid.New()
		itemID1 := uuid.New()
		itemID2 := uuid.New()
		ctx := createContextWithUserID(userID.String())

		mockItems := []*ListItem{
			{
				ListID:      listID,
				ItemID:      itemID1,
				ContentType: ContentTypePOI,
				Position:    1,
				Notes:       "First item",
				CreatedAt:   time.Now(),
				UpdatedAt:   time.Now(),
			},
			{
				ListID:      listID,
				ItemID:      itemID2,
				ContentType: ContentTypeRestaurant,
				Position:    2,
				Notes:       "Second item",
				CreatedAt:   time.Now(),
				UpdatedAt:   time.Now(),
			},
		}

		req := &pb.GetListItemsRequest{
			UserId:  "any-user-id", // Will be overridden by auth
			ListId:  listID.String(),
			Request: &pb.BaseRequest{},
		}

		// Mock expectations
		mockRepo.On("GetListItems", mock.Anything, listID).Return(mockItems, nil).Once()

		// Call the gRPC method
		resp, err := service.GetListItems(ctx, req)

		// Assertions
		assert.NoError(t, err)
		assert.NotNil(t, resp)
		assert.Len(t, resp.Items, 2)
		assert.Equal(t, int32(2), resp.TotalCount)
		assert.Equal(t, mockItems[0].Notes, resp.Items[0].ListItem.Notes)
		assert.Equal(t, mockItems[1].Notes, resp.Items[1].ListItem.Notes)
		assert.Equal(t, pb.ContentType_CONTENT_TYPE_POI, resp.Items[0].ListItem.ContentType)
		assert.Equal(t, pb.ContentType_CONTENT_TYPE_RESTAURANT, resp.Items[1].ListItem.ContentType)
		assert.Equal(t, "test-request-id", resp.Response.RequestId)
		assert.Equal(t, "lists-service", resp.Response.Upstream)
		assert.Equal(t, "success", resp.Response.Status)
		mockRepo.AssertExpectations(t)
	})

	t.Run("EmptyList", func(t *testing.T) {
		mockRepo := new(MockListsRepository)
		service := createTestListsService(t, mockRepo)

		userID := uuid.New()
		listID := uuid.New()
		ctx := createContextWithUserID(userID.String())

		req := &pb.GetListItemsRequest{
			UserId:  "any-user-id",
			ListId:  listID.String(),
			Request: &pb.BaseRequest{},
		}

		// Mock expectations - empty list
		mockRepo.On("GetListItems", mock.Anything, listID).Return([]*ListItem{}, nil).Once()

		// Call the gRPC method
		resp, err := service.GetListItems(ctx, req)

		// Assertions
		assert.NoError(t, err)
		assert.NotNil(t, resp)
		assert.Len(t, resp.Items, 0)
		assert.Equal(t, int32(0), resp.TotalCount)
		mockRepo.AssertExpectations(t)
	})

	t.Run("InvalidListIDFormat", func(t *testing.T) {
		mockRepo := new(MockListsRepository)
		service := createTestListsService(t, mockRepo)

		userID := uuid.New()
		ctx := createContextWithUserID(userID.String())

		req := &pb.GetListItemsRequest{
			UserId:  "any-user-id",
			ListId:  "invalid-list-id",
			Request: &pb.BaseRequest{},
		}

		resp, err := service.GetListItems(ctx, req)

		assert.Error(t, err)
		assert.NotNil(t, resp)
		assert.Nil(t, resp.Items)
		grpcErr, ok := status.FromError(err)
		assert.True(t, ok)
		assert.Equal(t, codes.InvalidArgument, grpcErr.Code())
		assert.Contains(t, grpcErr.Message(), "invalid list ID format")
		mockRepo.AssertExpectations(t)
	})

	t.Run("RepositoryError", func(t *testing.T) {
		mockRepo := new(MockListsRepository)
		service := createTestListsService(t, mockRepo)

		userID := uuid.New()
		listID := uuid.New()
		ctx := createContextWithUserID(userID.String())

		req := &pb.GetListItemsRequest{
			UserId:  "any-user-id",
			ListId:  listID.String(),
			Request: &pb.BaseRequest{},
		}

		// Mock expectations - repository error
		mockRepo.On("GetListItems", mock.Anything, listID).Return(nil, errors.New("database error")).Once()

		resp, err := service.GetListItems(ctx, req)

		assert.Error(t, err)
		assert.NotNil(t, resp)
		assert.Nil(t, resp.Items)
		grpcErr, ok := status.FromError(err)
		assert.True(t, ok)
		assert.Equal(t, codes.Internal, grpcErr.Code())
		assert.Contains(t, grpcErr.Message(), "failed to get list items")
		mockRepo.AssertExpectations(t)
	})
}

func TestListsService_RemoveListItem(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mockRepo := new(MockListsRepository)
		service := createTestListsService(t, mockRepo)

		userID := uuid.New()
		listID := uuid.New()
		itemID := uuid.New()
		cityID := uuid.New()
		ctx := createContextWithUserID(userID.String())

		existingList := List{
			ID:          listID,
			UserID:      userID,
			Name:        "My List",
			Description: "A list",
			IsPublic:    false,
			IsItinerary: false,
			CityID:      cityID,
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}

		req := &pb.RemoveListItemRequest{
			UserId:  "any-user-id", // Will be overridden by auth
			ListId:  listID.String(),
			ItemId:  itemID.String(),
			Request: &pb.BaseRequest{},
		}

		// Mock expectations
		mockRepo.On("GetList", mock.Anything, listID).Return(existingList, nil).Once()
		mockRepo.On("DeleteListItemByID", mock.Anything, listID, itemID).Return(nil).Once()

		// Call the gRPC method
		resp, err := service.RemoveListItem(ctx, req)

		// Assertions
		assert.NoError(t, err)
		assert.NotNil(t, resp)
		assert.True(t, resp.Success)
		assert.Equal(t, "list item removed successfully", resp.Message)
		assert.Equal(t, "test-request-id", resp.Response.RequestId)
		assert.Equal(t, "lists-service", resp.Response.Upstream)
		assert.Equal(t, "success", resp.Response.Status)
		mockRepo.AssertExpectations(t)
	})

	t.Run("ListNotFound", func(t *testing.T) {
		mockRepo := new(MockListsRepository)
		service := createTestListsService(t, mockRepo)

		userID := uuid.New()
		listID := uuid.New()
		itemID := uuid.New()
		ctx := createContextWithUserID(userID.String())

		req := &pb.RemoveListItemRequest{
			UserId:  "any-user-id",
			ListId:  listID.String(),
			ItemId:  itemID.String(),
			Request: &pb.BaseRequest{},
		}

		// Mock expectations - list not found
		mockRepo.On("GetList", mock.Anything, listID).Return(List{}, domain.ErrNotFound).Once()

		resp, err := service.RemoveListItem(ctx, req)

		assert.Error(t, err)
		assert.NotNil(t, resp)
		assert.False(t, resp.Success)
		assert.Equal(t, "list not found", resp.Message)
		grpcErr, ok := status.FromError(err)
		assert.True(t, ok)
		assert.Equal(t, codes.NotFound, grpcErr.Code())
		assert.Contains(t, grpcErr.Message(), "list not found")
		mockRepo.AssertExpectations(t)
	})

	t.Run("UnauthorizedUser", func(t *testing.T) {
		mockRepo := new(MockListsRepository)
		service := createTestListsService(t, mockRepo)

		userID := uuid.New()
		otherUserID := uuid.New()
		listID := uuid.New()
		itemID := uuid.New()
		cityID := uuid.New()
		ctx := createContextWithUserID(userID.String())

		existingList := List{
			ID:          listID,
			UserID:      otherUserID, // Different user
			Name:        "My List",
			Description: "A list",
			IsPublic:    false,
			IsItinerary: false,
			CityID:      cityID,
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}

		req := &pb.RemoveListItemRequest{
			UserId:  "any-user-id",
			ListId:  listID.String(),
			ItemId:  itemID.String(),
			Request: &pb.BaseRequest{},
		}

		// Mock expectations
		mockRepo.On("GetList", mock.Anything, listID).Return(existingList, nil).Once()

		resp, err := service.RemoveListItem(ctx, req)

		assert.Error(t, err)
		assert.NotNil(t, resp)
		assert.False(t, resp.Success)
		assert.Equal(t, "unauthorized", resp.Message)
		grpcErr, ok := status.FromError(err)
		assert.True(t, ok)
		assert.Equal(t, codes.PermissionDenied, grpcErr.Code())
		assert.Contains(t, grpcErr.Message(), "unauthorized")
		mockRepo.AssertExpectations(t)
	})

	t.Run("InvalidListIDFormat", func(t *testing.T) {
		mockRepo := new(MockListsRepository)
		service := createTestListsService(t, mockRepo)

		userID := uuid.New()
		itemID := uuid.New()
		ctx := createContextWithUserID(userID.String())

		req := &pb.RemoveListItemRequest{
			UserId:  "any-user-id",
			ListId:  "invalid-list-id",
			ItemId:  itemID.String(),
			Request: &pb.BaseRequest{},
		}

		resp, err := service.RemoveListItem(ctx, req)

		assert.Error(t, err)
		assert.NotNil(t, resp)
		assert.False(t, resp.Success)
		assert.Equal(t, "invalid list ID format", resp.Message)
		grpcErr, ok := status.FromError(err)
		assert.True(t, ok)
		assert.Equal(t, codes.InvalidArgument, grpcErr.Code())
		assert.Contains(t, grpcErr.Message(), "invalid list ID format")
		mockRepo.AssertExpectations(t)
	})

	t.Run("InvalidItemIDFormat", func(t *testing.T) {
		mockRepo := new(MockListsRepository)
		service := createTestListsService(t, mockRepo)

		userID := uuid.New()
		listID := uuid.New()
		ctx := createContextWithUserID(userID.String())

		req := &pb.RemoveListItemRequest{
			UserId:  "any-user-id",
			ListId:  listID.String(),
			ItemId:  "invalid-item-id",
			Request: &pb.BaseRequest{},
		}

		resp, err := service.RemoveListItem(ctx, req)

		assert.Error(t, err)
		assert.NotNil(t, resp)
		assert.False(t, resp.Success)
		assert.Equal(t, "invalid item ID format", resp.Message)
		grpcErr, ok := status.FromError(err)
		assert.True(t, ok)
		assert.Equal(t, codes.InvalidArgument, grpcErr.Code())
		assert.Contains(t, grpcErr.Message(), "invalid item ID format")
		mockRepo.AssertExpectations(t)
	})

	t.Run("RepositoryDeleteError", func(t *testing.T) {
		mockRepo := new(MockListsRepository)
		service := createTestListsService(t, mockRepo)

		userID := uuid.New()
		listID := uuid.New()
		itemID := uuid.New()
		cityID := uuid.New()
		ctx := createContextWithUserID(userID.String())

		existingList := List{
			ID:          listID,
			UserID:      userID,
			Name:        "My List",
			Description: "A list",
			IsPublic:    false,
			IsItinerary: false,
			CityID:      cityID,
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}

		req := &pb.RemoveListItemRequest{
			UserId:  "any-user-id",
			ListId:  listID.String(),
			ItemId:  itemID.String(),
			Request: &pb.BaseRequest{},
		}

		// Mock expectations
		mockRepo.On("GetList", mock.Anything, listID).Return(existingList, nil).Once()
		mockRepo.On("DeleteListItemByID", mock.Anything, listID, itemID).Return(errors.New("database error")).Once()

		resp, err := service.RemoveListItem(ctx, req)

		assert.Error(t, err)
		assert.NotNil(t, resp)
		assert.False(t, resp.Success)
		assert.Equal(t, "failed to remove list item", resp.Message)
		grpcErr, ok := status.FromError(err)
		assert.True(t, ok)
		assert.Equal(t, codes.Internal, grpcErr.Code())
		assert.Contains(t, grpcErr.Message(), "failed to remove list item")
		mockRepo.AssertExpectations(t)
	})
}

// Test converter functions
func TestListsService_ConverterFunctions(t *testing.T) {
	t.Run("convertToPBList", func(t *testing.T) {
		userID := uuid.New()
		listID := uuid.New()
		cityID := uuid.New()
		parentListID := uuid.New()
		now := time.Now()

		list := &List{
			ID:           listID,
			UserID:       userID,
			Name:         "Test List",
			Description:  "Test Description",
			ImageURL:     "https://example.com/image.jpg",
			IsPublic:     true,
			IsItinerary:  false,
			ParentListID: &parentListID,
			CityID:       cityID,
			ViewCount:    10,
			SaveCount:    5,
			CreatedAt:    now,
			UpdatedAt:    now,
		}

		pbList := convertToPBList(list)

		assert.Equal(t, listID.String(), pbList.Id)
		assert.Equal(t, userID.String(), pbList.UserId)
		assert.Equal(t, "Test List", pbList.Name)
		assert.Equal(t, "Test Description", pbList.Description)
		assert.Equal(t, "https://example.com/image.jpg", pbList.ImageUrl)
		assert.True(t, pbList.IsPublic)
		assert.False(t, pbList.IsItinerary)
		assert.Equal(t, parentListID.String(), pbList.ParentListId)
		assert.Equal(t, cityID.String(), pbList.CityId)
		assert.Equal(t, int32(10), pbList.ViewCount)
		assert.Equal(t, int32(5), pbList.SaveCount)
		assert.Equal(t, now.Unix(), pbList.CreatedAt.Seconds)
		assert.Equal(t, now.Unix(), pbList.UpdatedAt.Seconds)
	})

	t.Run("convertToPBListItem", func(t *testing.T) {
		listID := uuid.New()
		itemID := uuid.New()
		sourceID := uuid.New()
		dayNumber := 2
		duration := 120
		timeSlot := time.Now().Add(2 * time.Hour)
		now := time.Now()

		item := &ListItem{
			ListID:                 listID,
			ItemID:                 itemID,
			ContentType:            ContentTypePOI,
			Position:               1,
			Notes:                  "Test notes",
			DayNumber:              &dayNumber,
			TimeSlot:               &timeSlot,
			Duration:               &duration,
			SourceLlmInteractionID: &sourceID,
			ItemAIDescription:      "AI description",
			CreatedAt:              now,
			UpdatedAt:              now,
		}

		pbItem := convertToPBListItem(item)

		assert.Equal(t, listID.String(), pbItem.ListId)
		assert.Equal(t, itemID.String(), pbItem.ItemId)
		assert.Equal(t, pb.ContentType_CONTENT_TYPE_POI, pbItem.ContentType)
		assert.Equal(t, int32(1), pbItem.Position)
		assert.Equal(t, "Test notes", pbItem.Notes)
		assert.Equal(t, int32(2), pbItem.DayNumber)
		assert.Equal(t, int32(120), pbItem.Duration)
		assert.Equal(t, sourceID.String(), pbItem.SourceLlmInteractionId)
		assert.Equal(t, "AI description", pbItem.ItemAiDescription)
		assert.Equal(t, timeSlot.Unix(), pbItem.TimeSlot.Seconds)
		assert.Equal(t, now.Unix(), pbItem.CreatedAt.Seconds)
		assert.Equal(t, now.Unix(), pbItem.UpdatedAt.Seconds)
	})

	t.Run("convertToPBContentType", func(t *testing.T) {
		assert.Equal(t, pb.ContentType_CONTENT_TYPE_POI, convertToPBContentType(ContentTypePOI))
		assert.Equal(t, pb.ContentType_CONTENT_TYPE_RESTAURANT, convertToPBContentType(ContentTypeRestaurant))
		assert.Equal(t, pb.ContentType_CONTENT_TYPE_HOTEL, convertToPBContentType(ContentTypeHotel))
		assert.Equal(t, pb.ContentType_CONTENT_TYPE_ITINERARY, convertToPBContentType(ContentTypeItinerary))
		assert.Equal(t, pb.ContentType_CONTENT_TYPE_UNSPECIFIED, convertToPBContentType("unknown"))
	})

	t.Run("convertFromPBContentType", func(t *testing.T) {
		assert.Equal(t, ContentTypePOI, convertFromPBContentType(pb.ContentType_CONTENT_TYPE_POI))
		assert.Equal(t, ContentTypeRestaurant, convertFromPBContentType(pb.ContentType_CONTENT_TYPE_RESTAURANT))
		assert.Equal(t, ContentTypeHotel, convertFromPBContentType(pb.ContentType_CONTENT_TYPE_HOTEL))
		assert.Equal(t, ContentTypeItinerary, convertFromPBContentType(pb.ContentType_CONTENT_TYPE_ITINERARY))
		assert.Equal(t, ContentTypePOI, convertFromPBContentType(pb.ContentType_CONTENT_TYPE_UNSPECIFIED))
	})
}