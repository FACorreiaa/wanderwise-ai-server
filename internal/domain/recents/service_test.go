package recents

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

	pb "github.com/FACorreiaa/loci-proto/modules/recents/generated"
)

// MockRecentsRepository implements the Repository interface for testing
type MockRecentsRepository struct {
	mock.Mock
}

func (m *MockRecentsRepository) GetUserRecentInteractions(ctx context.Context, userID uuid.UUID, page, limit int, filterOptions *RecentInteractionsFilter) (*RecentInteractionsResponse, error) {
	args := m.Called(ctx, userID, page, limit, filterOptions)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*RecentInteractionsResponse), args.Error(1)
}

func (m *MockRecentsRepository) GetCityPOIsByInteraction(ctx context.Context, userID uuid.UUID, cityName string) ([]POIDetailedInfo, error) {
	args := m.Called(ctx, userID, cityName)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]POIDetailedInfo), args.Error(1)
}

func (m *MockRecentsRepository) GetCityFavorites(ctx context.Context, userID uuid.UUID, cityName string) ([]POIDetailedInfo, error) {
	args := m.Called(ctx, userID, cityName)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]POIDetailedInfo), args.Error(1)
}

func (m *MockRecentsRepository) GetCityItinerariesByInteraction(ctx context.Context, userID uuid.UUID, cityName string) ([]UserSavedItinerary, error) {
	args := m.Called(ctx, userID, cityName)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]UserSavedItinerary), args.Error(1)
}

func (m *MockRecentsRepository) GetCityHotelsByInteraction(ctx context.Context, userID uuid.UUID, cityName string) ([]HotelDetailedInfo, error) {
	args := m.Called(ctx, userID, cityName)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]HotelDetailedInfo), args.Error(1)
}

func (m *MockRecentsRepository) GetCityRestaurantsByInteraction(ctx context.Context, userID uuid.UUID, cityName string) ([]RestaurantDetailedInfo, error) {
	args := m.Called(ctx, userID, cityName)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]RestaurantDetailedInfo), args.Error(1)
}

func createTestRecentsService(t *testing.T, mockRepo *MockRecentsRepository) *Service {
	logger := zap.NewNop()
	var pgpool *pgxpool.Pool = nil

	return NewService(context.Background(), mockRepo, pgpool, logger)
}

func createContextWithAuth(userID string) context.Context {
	ctx := context.Background()
	ctx = context.WithValue(ctx, "userID", userID)
	return ctx
}

func TestRecentsService_GetRecentInteractions(t *testing.T) {
	t.Run("Success_FlatList", func(t *testing.T) {
		mockRepo := new(MockRecentsRepository)
		service := createTestRecentsService(t, mockRepo)

		userID := uuid.New()
		testTime := time.Now()

		interactions := []RecentInteraction{
			{
				ID:           uuid.New(),
				UserID:       userID,
				CityName:     "Paris",
				Prompt:       "Show me attractions in Paris",
				ResponseText: "Here are some attractions...",
				ModelUsed:    "gpt-4",
				LatencyMs:    1500,
				CreatedAt:    testTime,
			},
		}

		cityInteractions := CityInteractions{
			CityName:     "Paris",
			SessionID:    uuid.New(),
			Interactions: interactions,
			POICount:     5,
			LastActivity: testTime,
		}

		expectedResponse := &RecentInteractionsResponse{
			Cities: []CityInteractions{cityInteractions},
			Total:  1,
			Page:   1,
			Limit:  10,
		}

		req := &pb.GetRecentInteractionsRequest{
			Limit:       10,
			Offset:      0,
			GroupByCity: false,
			Filter: &pb.InteractionFilter{
				SearchQuery: "",
			},
		}

		mockRepo.On("GetUserRecentInteractions", mock.Anything, userID, 1, 10, mock.AnythingOfType("*recents.RecentInteractionsFilter")).
			Return(expectedResponse, nil).Once()

		ctx := createContextWithAuth(userID.String())
		resp, err := service.GetRecentInteractions(ctx, req)

		assert.NoError(t, err)
		assert.NotNil(t, resp)
		assert.Len(t, resp.Interactions, 1)
		assert.Empty(t, resp.CitySummaries)
		assert.Equal(t, int32(1), resp.TotalCount)
		assert.Equal(t, "Paris", resp.Interactions[0].CityName)
		mockRepo.AssertExpectations(t)
	})

	t.Run("Success_GroupByCity", func(t *testing.T) {
		mockRepo := new(MockRecentsRepository)
		service := createTestRecentsService(t, mockRepo)

		userID := uuid.New()
		testTime := time.Now()

		interactions := []RecentInteraction{
			{
				ID:           uuid.New(),
				UserID:       userID,
				CityName:     "Paris",
				Prompt:       "Show me attractions in Paris",
				ResponseText: "Here are some attractions...",
				ModelUsed:    "gpt-4",
				LatencyMs:    1500,
				CreatedAt:    testTime,
			},
		}

		cityInteractions := CityInteractions{
			CityName:     "Paris",
			SessionID:    uuid.New(),
			Interactions: interactions,
			POICount:     5,
			LastActivity: testTime,
		}

		expectedResponse := &RecentInteractionsResponse{
			Cities: []CityInteractions{cityInteractions},
			Total:  1,
			Page:   1,
			Limit:  10,
		}

		req := &pb.GetRecentInteractionsRequest{
			Limit:       10,
			Offset:      0,
			GroupByCity: true,
			Filter: &pb.InteractionFilter{
				SearchQuery: "Paris",
			},
		}

		mockRepo.On("GetUserRecentInteractions", mock.Anything, userID, 1, 10, mock.AnythingOfType("*recents.RecentInteractionsFilter")).
			Return(expectedResponse, nil).Once()

		ctx := createContextWithAuth(userID.String())
		resp, err := service.GetRecentInteractions(ctx, req)

		assert.NoError(t, err)
		assert.NotNil(t, resp)
		assert.Len(t, resp.CitySummaries, 1)
		assert.Empty(t, resp.Interactions)
		assert.Equal(t, int32(1), resp.TotalCount)
		assert.Equal(t, "Paris", resp.CitySummaries[0].CityName)
		assert.Equal(t, int32(1), resp.CitySummaries[0].InteractionCount)
		mockRepo.AssertExpectations(t)
	})

	t.Run("LimitValidation", func(t *testing.T) {
		mockRepo := new(MockRecentsRepository)
		service := createTestRecentsService(t, mockRepo)

		userID := uuid.New()

		req := &pb.GetRecentInteractionsRequest{
			Limit:       150, // Over limit of 100
			Offset:      0,
			GroupByCity: false,
		}

		expectedResponse := &RecentInteractionsResponse{
			Cities: []CityInteractions{},
			Total:  0,
			Page:   1,
			Limit:  100, // Should be capped at 100
		}

		mockRepo.On("GetUserRecentInteractions", mock.Anything, userID, 1, 100, mock.AnythingOfType("*recents.RecentInteractionsFilter")).
			Return(expectedResponse, nil).Once()

		ctx := createContextWithAuth(userID.String())
		resp, err := service.GetRecentInteractions(ctx, req)

		assert.NoError(t, err)
		assert.NotNil(t, resp)
		mockRepo.AssertExpectations(t)
	})

	t.Run("RepositoryError", func(t *testing.T) {
		mockRepo := new(MockRecentsRepository)
		service := createTestRecentsService(t, mockRepo)

		userID := uuid.New()

		req := &pb.GetRecentInteractionsRequest{
			Limit:       10,
			Offset:      0,
			GroupByCity: false,
		}

		mockRepo.On("GetUserRecentInteractions", mock.Anything, userID, 1, 10, mock.AnythingOfType("*recents.RecentInteractionsFilter")).
			Return(nil, errors.New("database error")).Once()

		ctx := createContextWithAuth(userID.String())
		resp, err := service.GetRecentInteractions(ctx, req)

		assert.Error(t, err)
		assert.Nil(t, resp)

		grpcErr, ok := status.FromError(err)
		assert.True(t, ok)
		assert.Equal(t, codes.Internal, grpcErr.Code())
		assert.Contains(t, grpcErr.Message(), "failed to get recent interactions")
		mockRepo.AssertExpectations(t)
	})

	t.Run("UnauthenticatedUser", func(t *testing.T) {
		mockRepo := new(MockRecentsRepository)
		service := createTestRecentsService(t, mockRepo)

		req := &pb.GetRecentInteractionsRequest{
			Limit:       10,
			Offset:      0,
			GroupByCity: false,
		}

		// Context without user authentication
		ctx := context.Background()
		resp, err := service.GetRecentInteractions(ctx, req)

		assert.Error(t, err)
		assert.Nil(t, resp)

		grpcErr, ok := status.FromError(err)
		assert.True(t, ok)
		assert.Equal(t, codes.Unauthenticated, grpcErr.Code())
		mockRepo.AssertExpectations(t)
	})
}

func TestRecentsService_GetCityInteractions(t *testing.T) {
	t.Run("Success_WithDetails", func(t *testing.T) {
		mockRepo := new(MockRecentsRepository)
		service := createTestRecentsService(t, mockRepo)

		userID := uuid.New()
		testTime := time.Now()
		cityName := "Paris"

		interactions := []RecentInteraction{
			{
				ID:           uuid.New(),
				UserID:       userID,
				CityName:     cityName,
				Prompt:       "Show me attractions in Paris",
				ResponseText: "Here are some attractions...",
				ModelUsed:    "gpt-4",
				LatencyMs:    1500,
				CreatedAt:    testTime,
			},
		}

		cityInteractions := CityInteractions{
			CityName:     cityName,
			SessionID:    uuid.New(),
			Interactions: interactions,
			POICount:     5,
			LastActivity: testTime,
		}

		expectedResponse := &RecentInteractionsResponse{
			Cities: []CityInteractions{cityInteractions},
			Total:  1,
		}

		pois := []POIDetailedInfo{
			{
				ID:       uuid.New(),
				City:     cityName,
				Name:     "Eiffel Tower",
				Category: "Attraction",
				Rating:   4.5,
			},
		}

		favorites := []POIDetailedInfo{
			{
				ID:       uuid.New(),
				City:     cityName,
				Name:     "Louvre Museum",
				Category: "Museum",
				Rating:   4.8,
			},
		}

		itineraries := []UserSavedItinerary{
			{
				ID:     uuid.New(),
				UserID: userID,
				Title:  "Paris 3-day itinerary",
			},
		}

		req := &pb.GetCityInteractionsRequest{
			CityName:       cityName,
			IncludeDetails: true,
		}

		mockRepo.On("GetUserRecentInteractions", mock.Anything, userID, 1, 50, mock.AnythingOfType("*recents.RecentInteractionsFilter")).
			Return(expectedResponse, nil).Once()
		mockRepo.On("GetCityPOIsByInteraction", mock.Anything, userID, cityName).
			Return(pois, nil).Once()
		mockRepo.On("GetCityFavorites", mock.Anything, userID, cityName).
			Return(favorites, nil).Once()
		mockRepo.On("GetCityItinerariesByInteraction", mock.Anything, userID, cityName).
			Return(itineraries, nil).Once()

		ctx := createContextWithAuth(userID.String())
		resp, err := service.GetCityInteractions(ctx, req)

		assert.NoError(t, err)
		assert.NotNil(t, resp)
		assert.NotNil(t, resp.CityInteractions)
		assert.Equal(t, cityName, resp.CityInteractions.CityName)
		assert.Equal(t, int32(1), resp.CityInteractions.TotalInteractions)
		assert.Equal(t, int32(1), resp.CityInteractions.Favorites)
		assert.Equal(t, int32(1), resp.CityInteractions.ItinerariesCreated)
		assert.Equal(t, int32(1), resp.CityInteractions.PoisViewed)
		assert.Len(t, resp.DetailedInteractions, 1)
		mockRepo.AssertExpectations(t)
	})

	t.Run("Success_WithoutDetails", func(t *testing.T) {
		mockRepo := new(MockRecentsRepository)
		service := createTestRecentsService(t, mockRepo)

		userID := uuid.New()
		testTime := time.Now()
		cityName := "Paris"

		interactions := []RecentInteraction{
			{
				ID:           uuid.New(),
				UserID:       userID,
				CityName:     cityName,
				Prompt:       "Show me attractions in Paris",
				ResponseText: "Here are some attractions...",
				ModelUsed:    "gpt-4",
				LatencyMs:    1500,
				CreatedAt:    testTime,
			},
		}

		cityInteractions := CityInteractions{
			CityName:     cityName,
			SessionID:    uuid.New(),
			Interactions: interactions,
			POICount:     5,
			LastActivity: testTime,
		}

		expectedResponse := &RecentInteractionsResponse{
			Cities: []CityInteractions{cityInteractions},
			Total:  1,
		}

		req := &pb.GetCityInteractionsRequest{
			CityName:       cityName,
			IncludeDetails: false,
		}

		mockRepo.On("GetUserRecentInteractions", mock.Anything, userID, 1, 50, mock.AnythingOfType("*recents.RecentInteractionsFilter")).
			Return(expectedResponse, nil).Once()
		mockRepo.On("GetCityPOIsByInteraction", mock.Anything, userID, cityName).
			Return([]POIDetailedInfo{}, nil).Once()
		mockRepo.On("GetCityFavorites", mock.Anything, userID, cityName).
			Return([]POIDetailedInfo{}, nil).Once()
		mockRepo.On("GetCityItinerariesByInteraction", mock.Anything, userID, cityName).
			Return([]UserSavedItinerary{}, nil).Once()

		ctx := createContextWithAuth(userID.String())
		resp, err := service.GetCityInteractions(ctx, req)

		assert.NoError(t, err)
		assert.NotNil(t, resp)
		assert.NotNil(t, resp.CityInteractions)
		assert.Equal(t, cityName, resp.CityInteractions.CityName)
		assert.Empty(t, resp.DetailedInteractions) // No details requested
		mockRepo.AssertExpectations(t)
	})

	t.Run("EmptyCityName", func(t *testing.T) {
		mockRepo := new(MockRecentsRepository)
		service := createTestRecentsService(t, mockRepo)

		userID := uuid.New()

		req := &pb.GetCityInteractionsRequest{
			CityName:       "", // Empty city name
			IncludeDetails: false,
		}

		ctx := createContextWithAuth(userID.String())
		resp, err := service.GetCityInteractions(ctx, req)

		assert.Error(t, err)
		assert.Nil(t, resp)

		grpcErr, ok := status.FromError(err)
		assert.True(t, ok)
		assert.Equal(t, codes.InvalidArgument, grpcErr.Code())
		assert.Contains(t, grpcErr.Message(), "city name is required")
		mockRepo.AssertExpectations(t)
	})

	t.Run("CityNotFound", func(t *testing.T) {
		mockRepo := new(MockRecentsRepository)
		service := createTestRecentsService(t, mockRepo)

		userID := uuid.New()
		cityName := "NonexistentCity"

		expectedResponse := &RecentInteractionsResponse{
			Cities: []CityInteractions{}, // No cities found
			Total:  0,
		}

		req := &pb.GetCityInteractionsRequest{
			CityName:       cityName,
			IncludeDetails: false,
		}

		mockRepo.On("GetUserRecentInteractions", mock.Anything, userID, 1, 50, mock.AnythingOfType("*recents.RecentInteractionsFilter")).
			Return(expectedResponse, nil).Once()

		ctx := createContextWithAuth(userID.String())
		resp, err := service.GetCityInteractions(ctx, req)

		assert.Error(t, err)
		assert.Nil(t, resp)

		grpcErr, ok := status.FromError(err)
		assert.True(t, ok)
		assert.Equal(t, codes.NotFound, grpcErr.Code())
		assert.Contains(t, grpcErr.Message(), "no interactions found for city")
		mockRepo.AssertExpectations(t)
	})

	t.Run("RepositoryError", func(t *testing.T) {
		mockRepo := new(MockRecentsRepository)
		service := createTestRecentsService(t, mockRepo)

		userID := uuid.New()
		cityName := "Paris"

		req := &pb.GetCityInteractionsRequest{
			CityName:       cityName,
			IncludeDetails: false,
		}

		mockRepo.On("GetUserRecentInteractions", mock.Anything, userID, 1, 50, mock.AnythingOfType("*recents.RecentInteractionsFilter")).
			Return(nil, errors.New("database error")).Once()

		ctx := createContextWithAuth(userID.String())
		resp, err := service.GetCityInteractions(ctx, req)

		assert.Error(t, err)
		assert.Nil(t, resp)

		grpcErr, ok := status.FromError(err)
		assert.True(t, ok)
		assert.Equal(t, codes.Internal, grpcErr.Code())
		assert.Contains(t, grpcErr.Message(), "failed to get recent interactions")
		mockRepo.AssertExpectations(t)
	})

	t.Run("UnauthenticatedUser", func(t *testing.T) {
		mockRepo := new(MockRecentsRepository)
		service := createTestRecentsService(t, mockRepo)

		req := &pb.GetCityInteractionsRequest{
			CityName:       "Paris",
			IncludeDetails: false,
		}

		// Context without user authentication
		ctx := context.Background()
		resp, err := service.GetCityInteractions(ctx, req)

		assert.Error(t, err)
		assert.Nil(t, resp)

		grpcErr, ok := status.FromError(err)
		assert.True(t, ok)
		assert.Equal(t, codes.Unauthenticated, grpcErr.Code())
		mockRepo.AssertExpectations(t)
	})
}

func TestRecentsService_RecordInteraction(t *testing.T) {
	t.Run("NotImplemented", func(t *testing.T) {
		mockRepo := new(MockRecentsRepository)
		service := createTestRecentsService(t, mockRepo)

		req := &pb.RecordInteractionRequest{}

		ctx := context.Background()
		resp, err := service.RecordInteraction(ctx, req)

		assert.NoError(t, err)
		assert.Nil(t, resp)
		mockRepo.AssertExpectations(t)
	})
}

func TestRecentsService_GetInteractionHistory(t *testing.T) {
	t.Run("NotImplemented", func(t *testing.T) {
		mockRepo := new(MockRecentsRepository)
		service := createTestRecentsService(t, mockRepo)

		req := &pb.GetInteractionHistoryRequest{}

		ctx := context.Background()
		resp, err := service.GetInteractionHistory(ctx, req)

		assert.NoError(t, err)
		assert.Nil(t, resp)
		mockRepo.AssertExpectations(t)
	})
}

func TestRecentsService_GetFrequentPlaces(t *testing.T) {
	t.Run("NotImplemented", func(t *testing.T) {
		mockRepo := new(MockRecentsRepository)
		service := createTestRecentsService(t, mockRepo)

		req := &pb.GetFrequentPlacesRequest{}

		ctx := context.Background()
		resp, err := service.GetFrequentPlaces(ctx, req)

		assert.NoError(t, err)
		assert.Nil(t, resp)
		mockRepo.AssertExpectations(t)
	})
}