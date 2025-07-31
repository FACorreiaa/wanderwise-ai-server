package statistics

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	pb "github.com/FACorreiaa/loci-proto/modules/statistics/generated"
)

// MockStatisticsRepository implements the Repository interface for testing
type MockStatisticsRepository struct {
	mock.Mock
}

func (m *MockStatisticsRepository) GetMainPageStatistics(ctx context.Context, userID uuid.UUID) (*MainPageStatistics, error) {
	args := m.Called(ctx, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*MainPageStatistics), args.Error(1)
}

func (m *MockStatisticsRepository) GetDetailedPOIStatistics(ctx context.Context, userID uuid.UUID) (*DetailedPOIStatistics, error) {
	args := m.Called(ctx, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*DetailedPOIStatistics), args.Error(1)
}

func (m *MockStatisticsRepository) LandingPageStatistics(ctx context.Context, userID uuid.UUID) (*LandingPageUserStats, error) {
	args := m.Called(ctx, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*LandingPageUserStats), args.Error(1)
}

func createTestStatisticsService(t *testing.T, mockRepo *MockStatisticsRepository) *Service {
	logger := zap.NewNop()
	var pgpool *pgxpool.Pool = nil

	return NewService(context.Background(), mockRepo, pgpool, logger)
}

func TestStatisticsService_GetMainPageStatistics(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mockRepo := new(MockStatisticsRepository)
		service := createTestStatisticsService(t, mockRepo)

		expectedStats := &MainPageStatistics{
			TotalUsersCount:       1000,
			TotalItinerariesSaved: 5000,
			TotalUniquePOIs:       25000,
		}

		systemUserID := uuid.MustParse("00000000-0000-0000-0000-000000000000")
		req := &pb.GetMainPageStatisticsRequest{
			Request: &pb.BaseRequest{
				RequestId: "test-request-id",
			},
		}

		mockRepo.On("GetMainPageStatistics", mock.Anything, systemUserID).Return(expectedStats, nil).Once()

		ctx := context.Background()
		resp, err := service.GetMainPageStatistics(ctx, req)

		assert.NoError(t, err)
		assert.NotNil(t, resp)
		assert.NotNil(t, resp.Statistics)
		assert.Equal(t, int64(25000), resp.Statistics.TotalPois)
		assert.Equal(t, int64(1000), resp.Statistics.TotalUsers)
		assert.Equal(t, int64(5000), resp.Statistics.TotalItineraries)
		assert.Equal(t, "success", resp.Response.Status)
		assert.Equal(t, "statistics-service", resp.Response.Upstream)
		assert.Equal(t, "test-request-id", resp.Response.RequestId)
		mockRepo.AssertExpectations(t)
	})

	t.Run("RepositoryError", func(t *testing.T) {
		mockRepo := new(MockStatisticsRepository)
		service := createTestStatisticsService(t, mockRepo)

		systemUserID := uuid.MustParse("00000000-0000-0000-0000-000000000000")
		req := &pb.GetMainPageStatisticsRequest{
			Request: &pb.BaseRequest{
				RequestId: "test-request-id",
			},
		}

		mockRepo.On("GetMainPageStatistics", mock.Anything, systemUserID).Return(nil, errors.New("database error")).Once()

		ctx := context.Background()
		resp, err := service.GetMainPageStatistics(ctx, req)

		assert.Error(t, err)
		assert.NotNil(t, resp)
		assert.Nil(t, resp.Statistics)
		assert.Equal(t, "error", resp.Response.Status)

		grpcErr, ok := status.FromError(err)
		assert.True(t, ok)
		assert.Equal(t, codes.Internal, grpcErr.Code())
		assert.Contains(t, grpcErr.Message(), "failed to get main page statistics")
		mockRepo.AssertExpectations(t)
	})

	t.Run("NilRequest", func(t *testing.T) {
		mockRepo := new(MockStatisticsRepository)
		service := createTestStatisticsService(t, mockRepo)

		expectedStats := &MainPageStatistics{
			TotalUsersCount:       1000,
			TotalItinerariesSaved: 5000,
			TotalUniquePOIs:       25000,
		}

		systemUserID := uuid.MustParse("00000000-0000-0000-0000-000000000000")
		req := &pb.GetMainPageStatisticsRequest{
			Request: nil, // Nil request
		}

		mockRepo.On("GetMainPageStatistics", mock.Anything, systemUserID).Return(expectedStats, nil).Once()

		ctx := context.Background()
		resp, err := service.GetMainPageStatistics(ctx, req)

		assert.NoError(t, err)
		assert.NotNil(t, resp)
		assert.NotNil(t, resp.Statistics)
		assert.Equal(t, "unknown", resp.Response.RequestId) // Should default to "unknown"
		mockRepo.AssertExpectations(t)
	})
}

func TestStatisticsService_GetDetailedPOIStatistics(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mockRepo := new(MockStatisticsRepository)
		service := createTestStatisticsService(t, mockRepo)

		userID := uuid.New()
		expectedStats := &DetailedPOIStatistics{
			GeneralPOIs:   100,
			SuggestedPOIs: 50,
			Hotels:        20,
			Restaurants:   80,
			TotalPOIs:     250,
		}

		req := &pb.GetDetailedPOIStatisticsRequest{
			UserId: userID.String(),
			Request: &pb.BaseRequest{
				RequestId: "test-request-id",
			},
		}

		mockRepo.On("GetDetailedPOIStatistics", mock.Anything, userID).Return(expectedStats, nil).Once()

		ctx := context.Background()
		resp, err := service.GetDetailedPOIStatistics(ctx, req)

		assert.NoError(t, err)
		assert.NotNil(t, resp)
		assert.NotNil(t, resp.Statistics)
		assert.Equal(t, userID.String(), resp.Statistics.UserId)
		assert.Equal(t, int32(250), resp.Statistics.TotalPoiSearches)
		assert.Equal(t, int32(150), resp.Statistics.FavoritePoisCount) // GeneralPOIs + SuggestedPOIs
		assert.Equal(t, int32(0), resp.Statistics.VisitedCitiesCount)  // Not available in current domain model
		assert.Equal(t, "success", resp.Response.Status)
		mockRepo.AssertExpectations(t)
	})

	t.Run("EmptyUserID", func(t *testing.T) {
		mockRepo := new(MockStatisticsRepository)
		service := createTestStatisticsService(t, mockRepo)

		req := &pb.GetDetailedPOIStatisticsRequest{
			UserId: "", // Empty user ID
			Request: &pb.BaseRequest{
				RequestId: "test-request-id",
			},
		}

		ctx := context.Background()
		resp, err := service.GetDetailedPOIStatistics(ctx, req)

		assert.Error(t, err)
		assert.NotNil(t, resp)
		assert.Nil(t, resp.Statistics)
		assert.Equal(t, "error", resp.Response.Status)

		grpcErr, ok := status.FromError(err)
		assert.True(t, ok)
		assert.Equal(t, codes.InvalidArgument, grpcErr.Code())
		assert.Contains(t, grpcErr.Message(), "user_id is required")
		mockRepo.AssertExpectations(t)
	})

	t.Run("InvalidUserIDFormat", func(t *testing.T) {
		mockRepo := new(MockStatisticsRepository)
		service := createTestStatisticsService(t, mockRepo)

		req := &pb.GetDetailedPOIStatisticsRequest{
			UserId: "invalid-uuid", // Invalid UUID format
			Request: &pb.BaseRequest{
				RequestId: "test-request-id",
			},
		}

		ctx := context.Background()
		resp, err := service.GetDetailedPOIStatistics(ctx, req)

		assert.Error(t, err)
		assert.NotNil(t, resp)
		assert.Nil(t, resp.Statistics)
		assert.Equal(t, "error", resp.Response.Status)

		grpcErr, ok := status.FromError(err)
		assert.True(t, ok)
		assert.Equal(t, codes.InvalidArgument, grpcErr.Code())
		assert.Contains(t, grpcErr.Message(), "invalid user_id format")
		mockRepo.AssertExpectations(t)
	})

	t.Run("RepositoryError", func(t *testing.T) {
		mockRepo := new(MockStatisticsRepository)
		service := createTestStatisticsService(t, mockRepo)

		userID := uuid.New()
		req := &pb.GetDetailedPOIStatisticsRequest{
			UserId: userID.String(),
			Request: &pb.BaseRequest{
				RequestId: "test-request-id",
			},
		}

		mockRepo.On("GetDetailedPOIStatistics", mock.Anything, userID).Return(nil, errors.New("database error")).Once()

		ctx := context.Background()
		resp, err := service.GetDetailedPOIStatistics(ctx, req)

		assert.Error(t, err)
		assert.NotNil(t, resp)
		assert.Nil(t, resp.Statistics)
		assert.Equal(t, "error", resp.Response.Status)

		grpcErr, ok := status.FromError(err)
		assert.True(t, ok)
		assert.Equal(t, codes.Internal, grpcErr.Code())
		assert.Contains(t, grpcErr.Message(), "failed to get detailed POI statistics")
		mockRepo.AssertExpectations(t)
	})
}

func TestStatisticsService_GetLandingPageStatistics(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mockRepo := new(MockStatisticsRepository)
		service := createTestStatisticsService(t, mockRepo)

		userID := uuid.New()
		expectedStats := &LandingPageUserStats{
			SavedPlaces:    25,
			Itineraries:    10,
			CitiesExplored: 5,
			Discoveries:    30,
		}

		req := &pb.GetLandingPageStatisticsRequest{
			UserId: userID.String(),
			Request: &pb.BaseRequest{
				RequestId: "test-request-id",
			},
		}

		mockRepo.On("LandingPageStatistics", mock.Anything, userID).Return(expectedStats, nil).Once()

		ctx := context.Background()
		resp, err := service.GetLandingPageStatistics(ctx, req)

		assert.NoError(t, err)
		assert.NotNil(t, resp)
		assert.NotNil(t, resp.Statistics)
		assert.Equal(t, userID.String(), resp.Statistics.UserId)
		assert.Equal(t, int32(30), resp.Statistics.SearchesThisWeek)            // Maps to Discoveries
		assert.Equal(t, int32(25), resp.Statistics.NewFavoritesThisWeek)       // Maps to SavedPlaces
		assert.Equal(t, int32(10), resp.Statistics.ItinerariesCreatedThisMonth) // Direct mapping
		assert.Empty(t, resp.Statistics.RecentlySearchedCities)                 // Empty for now
		assert.Equal(t, "success", resp.Response.Status)
		mockRepo.AssertExpectations(t)
	})

	t.Run("EmptyUserID", func(t *testing.T) {
		mockRepo := new(MockStatisticsRepository)
		service := createTestStatisticsService(t, mockRepo)

		req := &pb.GetLandingPageStatisticsRequest{
			UserId: "", // Empty user ID
			Request: &pb.BaseRequest{
				RequestId: "test-request-id",
			},
		}

		ctx := context.Background()
		resp, err := service.GetLandingPageStatistics(ctx, req)

		assert.Error(t, err)
		assert.NotNil(t, resp)
		assert.Nil(t, resp.Statistics)
		assert.Equal(t, "error", resp.Response.Status)

		grpcErr, ok := status.FromError(err)
		assert.True(t, ok)
		assert.Equal(t, codes.InvalidArgument, grpcErr.Code())
		assert.Contains(t, grpcErr.Message(), "user_id is required")
		mockRepo.AssertExpectations(t)
	})

	t.Run("InvalidUserIDFormat", func(t *testing.T) {
		mockRepo := new(MockStatisticsRepository)
		service := createTestStatisticsService(t, mockRepo)

		req := &pb.GetLandingPageStatisticsRequest{
			UserId: "invalid-uuid", // Invalid UUID format
			Request: &pb.BaseRequest{
				RequestId: "test-request-id",
			},
		}

		ctx := context.Background()
		resp, err := service.GetLandingPageStatistics(ctx, req)

		assert.Error(t, err)
		assert.NotNil(t, resp)
		assert.Nil(t, resp.Statistics)
		assert.Equal(t, "error", resp.Response.Status)

		grpcErr, ok := status.FromError(err)
		assert.True(t, ok)
		assert.Equal(t, codes.InvalidArgument, grpcErr.Code())
		assert.Contains(t, grpcErr.Message(), "invalid user_id format")
		mockRepo.AssertExpectations(t)
	})

	t.Run("RepositoryError", func(t *testing.T) {
		mockRepo := new(MockStatisticsRepository)
		service := createTestStatisticsService(t, mockRepo)

		userID := uuid.New()
		req := &pb.GetLandingPageStatisticsRequest{
			UserId: userID.String(),
			Request: &pb.BaseRequest{
				RequestId: "test-request-id",
			},
		}

		mockRepo.On("LandingPageStatistics", mock.Anything, userID).Return(nil, errors.New("database error")).Once()

		ctx := context.Background()
		resp, err := service.GetLandingPageStatistics(ctx, req)

		assert.Error(t, err)
		assert.NotNil(t, resp)
		assert.Nil(t, resp.Statistics)
		assert.Equal(t, "error", resp.Response.Status)

		grpcErr, ok := status.FromError(err)
		assert.True(t, ok)
		assert.Equal(t, codes.Internal, grpcErr.Code())
		assert.Contains(t, grpcErr.Message(), "failed to get landing page statistics")
		mockRepo.AssertExpectations(t)
	})
}

func TestStatisticsService_StreamMainPageStatistics(t *testing.T) {
	t.Run("NotImplemented", func(t *testing.T) {
		mockRepo := new(MockStatisticsRepository)
		service := createTestStatisticsService(t, mockRepo)

		req := &pb.StreamMainPageStatisticsRequest{
			Request: &pb.BaseRequest{
				RequestId: "test-request-id",
			},
		}

		// Create a mock streaming server
		mockStream := &MockStatisticsStream{}

		err := service.StreamMainPageStatistics(req, mockStream)

		assert.Error(t, err)
		grpcErr, ok := status.FromError(err)
		assert.True(t, ok)
		assert.Equal(t, codes.Unimplemented, grpcErr.Code())
		assert.Contains(t, grpcErr.Message(), "StreamMainPageStatistics not implemented yet")
		mockRepo.AssertExpectations(t)
	})
}

func TestStatisticsService_GetUserActivityAnalytics(t *testing.T) {
	t.Run("NotImplemented", func(t *testing.T) {
		mockRepo := new(MockStatisticsRepository)
		service := createTestStatisticsService(t, mockRepo)

		req := &pb.GetUserActivityAnalyticsRequest{
			Request: &pb.BaseRequest{
				RequestId: "test-request-id",
			},
		}

		ctx := context.Background()
		resp, err := service.GetUserActivityAnalytics(ctx, req)

		assert.Error(t, err)
		assert.NotNil(t, resp)
		assert.Nil(t, resp.ActivityData)
		assert.Nil(t, resp.Summary)
		assert.Equal(t, "not_implemented", resp.Response.Status)

		grpcErr, ok := status.FromError(err)
		assert.True(t, ok)
		assert.Equal(t, codes.Unimplemented, grpcErr.Code())
		assert.Contains(t, grpcErr.Message(), "GetUserActivityAnalytics not implemented yet")
		mockRepo.AssertExpectations(t)
	})
}

func TestStatisticsService_GetSystemAnalytics(t *testing.T) {
	t.Run("NotImplemented", func(t *testing.T) {
		mockRepo := new(MockStatisticsRepository)
		service := createTestStatisticsService(t, mockRepo)

		req := &pb.GetSystemAnalyticsRequest{
			Request: &pb.BaseRequest{
				RequestId: "test-request-id",
			},
		}

		ctx := context.Background()
		resp, err := service.GetSystemAnalytics(ctx, req)

		assert.Error(t, err)
		assert.NotNil(t, resp)
		assert.Nil(t, resp.Analytics)
		assert.Equal(t, "not_implemented", resp.Response.Status)

		grpcErr, ok := status.FromError(err)
		assert.True(t, ok)
		assert.Equal(t, codes.Unimplemented, grpcErr.Code())
		assert.Contains(t, grpcErr.Message(), "GetSystemAnalytics not implemented yet")
		mockRepo.AssertExpectations(t)
	})
}

// MockStatisticsStream is a mock implementation for gRPC streaming
type MockStatisticsStream struct {
	mock.Mock
}

func (m *MockStatisticsStream) Send(event *pb.StatisticsEvent) error {
	args := m.Called(event)
	return args.Error(0)
}

func (m *MockStatisticsStream) Context() context.Context {
	return context.Background()
}

func (m *MockStatisticsStream) SendMsg(msg interface{}) error {
	args := m.Called(msg)
	return args.Error(0)
}

func (m *MockStatisticsStream) RecvMsg(msg interface{}) error {
	args := m.Called(msg)
	return args.Error(0)
}

func (m *MockStatisticsStream) SetHeader(md metadata.MD) error {
	args := m.Called(md)
	return args.Error(0)
}

func (m *MockStatisticsStream) SendHeader(md metadata.MD) error {
	args := m.Called(md)
	return args.Error(0)
}

func (m *MockStatisticsStream) SetTrailer(md metadata.MD) {
	m.Called(md)
}