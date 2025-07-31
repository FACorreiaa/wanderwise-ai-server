package poi

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap/zaptest"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "github.com/FACorreiaa/loci-proto/modules/poi/generated"
	"github.com/FACorreiaa/go-poi-au-suggestions/internal/domain"
)

// MockRepository implements the Repository interface for testing
type MockRepository struct {
	mock.Mock
}

func (m *MockRepository) SavePoi(ctx context.Context, poi POIDetailedInfo, cityID uuid.UUID) (uuid.UUID, error) {
	args := m.Called(ctx, poi, cityID)
	return args.Get(0).(uuid.UUID), args.Error(1)
}

func (m *MockRepository) FindPoiByNameAndCity(ctx context.Context, name string, cityID uuid.UUID) (*POIDetailedInfo, error) {
	args := m.Called(ctx, name, cityID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*POIDetailedInfo), args.Error(1)
}

func (m *MockRepository) GetPOIsByCityAndDistance(ctx context.Context, cityID uuid.UUID, userLocation UserLocation) ([]POIDetailedInfo, error) {
	args := m.Called(ctx, cityID, userLocation)
	return args.Get(0).([]POIDetailedInfo), args.Error(1)
}

func (m *MockRepository) GetPOIsByLocationAndDistance(ctx context.Context, lat, lon, radiusMeters float64) ([]POIDetailedInfo, error) {
	args := m.Called(ctx, lat, lon, radiusMeters)
	return args.Get(0).([]POIDetailedInfo), args.Error(1)
}

func (m *MockRepository) GetPOIsByLocationAndDistanceWithCategory(ctx context.Context, lat, lon, radiusMeters float64, category string) ([]POIDetailedInfo, error) {
	args := m.Called(ctx, lat, lon, radiusMeters, category)
	return args.Get(0).([]POIDetailedInfo), args.Error(1)
}

func (m *MockRepository) AddPoiToFavourites(ctx context.Context, userID, poiID uuid.UUID) (uuid.UUID, error) {
	args := m.Called(ctx, userID, poiID)
	return args.Get(0).(uuid.UUID), args.Error(1)
}

func (m *MockRepository) AddLLMPoiToFavourite(ctx context.Context, userID uuid.UUID, llmPoiID uuid.UUID) (uuid.UUID, error) {
	args := m.Called(ctx, userID, llmPoiID)
	return args.Get(0).(uuid.UUID), args.Error(1)
}

func (m *MockRepository) RemovePoiFromFavourites(ctx context.Context, userID, poiID uuid.UUID) error {
	args := m.Called(ctx, userID, poiID)
	return args.Error(0)
}

func (m *MockRepository) RemoveLLMPoiFromFavourite(ctx context.Context, userID, llmPoiID uuid.UUID) error {
	args := m.Called(ctx, userID, llmPoiID)
	return args.Error(0)
}

func (m *MockRepository) CheckPoiExists(ctx context.Context, poiID uuid.UUID) (bool, error) {
	args := m.Called(ctx, poiID)
	return args.Bool(0), args.Error(1)
}

func (m *MockRepository) FindLLMPOIByNameAndCity(ctx context.Context, name, city string) (uuid.UUID, error) {
	args := m.Called(ctx, name, city)
	return args.Get(0).(uuid.UUID), args.Error(1)
}

func (m *MockRepository) FindLLMPOIByName(ctx context.Context, name string) (uuid.UUID, error) {
	args := m.Called(ctx, name)
	return args.Get(0).(uuid.UUID), args.Error(1)
}

func (m *MockRepository) GetFavouritePOIsByUserID(ctx context.Context, userID uuid.UUID) ([]POIDetailedInfo, error) {
	args := m.Called(ctx, userID)
	return args.Get(0).([]POIDetailedInfo), args.Error(1)
}

func (m *MockRepository) GetFavouritePOIsByUserIDPaginated(ctx context.Context, userID uuid.UUID, limit, offset int) ([]POIDetailedInfo, int, error) {
	args := m.Called(ctx, userID, limit, offset)
	return args.Get(0).([]POIDetailedInfo), args.Int(1), args.Error(2)
}

func (m *MockRepository) GetPOIsByCityID(ctx context.Context, cityID uuid.UUID) ([]POIDetailedInfo, error) {
	args := m.Called(ctx, cityID)
	return args.Get(0).([]POIDetailedInfo), args.Error(1)
}

func (m *MockRepository) FindPOIDetails(ctx context.Context, cityID uuid.UUID, lat, lon float64, tolerance float64) (*POIDetailedInfo, error) {
	args := m.Called(ctx, cityID, lat, lon, tolerance)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*POIDetailedInfo), args.Error(1)
}

func (m *MockRepository) SavePOIDetails(ctx context.Context, poi POIDetailedInfo, cityID uuid.UUID) (uuid.UUID, error) {
	args := m.Called(ctx, poi, cityID)
	return args.Get(0).(uuid.UUID), args.Error(1)
}

func (m *MockRepository) SearchPOIs(ctx context.Context, filter POIFilter) ([]POIDetailedInfo, error) {
	args := m.Called(ctx, filter)
	return args.Get(0).([]POIDetailedInfo), args.Error(1)
}

func (m *MockRepository) FindSimilarPOIs(ctx context.Context, queryEmbedding []float32, limit int) ([]POIDetailedInfo, error) {
	args := m.Called(ctx, queryEmbedding, limit)
	return args.Get(0).([]POIDetailedInfo), args.Error(1)
}

func (m *MockRepository) FindSimilarPOIsByCity(ctx context.Context, queryEmbedding []float32, cityID uuid.UUID, limit int) ([]POIDetailedInfo, error) {
	args := m.Called(ctx, queryEmbedding, cityID, limit)
	return args.Get(0).([]POIDetailedInfo), args.Error(1)
}

func (m *MockRepository) SearchPOIsHybrid(ctx context.Context, filter POIFilter, queryEmbedding []float32, semanticWeight float64) ([]POIDetailedInfo, error) {
	args := m.Called(ctx, filter, queryEmbedding, semanticWeight)
	return args.Get(0).([]POIDetailedInfo), args.Error(1)
}

func (m *MockRepository) UpdatePOIEmbedding(ctx context.Context, poiID uuid.UUID, embedding []float32) error {
	args := m.Called(ctx, poiID, embedding)
	return args.Error(0)
}

func (m *MockRepository) GetPOIsWithoutEmbeddings(ctx context.Context, limit int) ([]POIDetailedInfo, error) {
	args := m.Called(ctx, limit)
	return args.Get(0).([]POIDetailedInfo), args.Error(1)
}

func (m *MockRepository) GetItineraries(ctx context.Context, userID uuid.UUID, page, pageSize int) ([]UserSavedItinerary, int, error) {
	args := m.Called(ctx, userID, page, pageSize)
	return args.Get(0).([]UserSavedItinerary), args.Int(1), args.Error(2)
}

func (m *MockRepository) GetItinerary(ctx context.Context, userID, itineraryID uuid.UUID) (*UserSavedItinerary, error) {
	args := m.Called(ctx, userID, itineraryID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*UserSavedItinerary), args.Error(1)
}

func (m *MockRepository) UpdateItinerary(ctx context.Context, userID, itineraryID uuid.UUID, updates UpdateItineraryRequest) (*UserSavedItinerary, error) {
	args := m.Called(ctx, userID, itineraryID, updates)
	return args.Get(0).(*UserSavedItinerary), args.Error(1)
}

// Test helper functions
func createTestService(t *testing.T) (*Service, *MockRepository) {
	logger := zaptest.NewLogger(t)
	mockRepo := &MockRepository{}
	
	service := &Service{
		logger: logger,
		repo:   mockRepo,
	}
	
	return service, mockRepo
}

func createTestPOI() POIDetailedInfo {
	poiID := uuid.New()
	return POIDetailedInfo{
		ID:          poiID,
		Name:        "Test Restaurant",
		Description: "A great test restaurant",
		Category:    "restaurant",
		Latitude:    40.7128,
		Longitude:   -74.0060,
		City:        "New York",
		Address:     "123 Test St",
		Website:     "https://test.com",
		PhoneNumber: "+1234567890",
		Rating:      4.5,
		Tags:        []string{"italian", "fine-dining"},
		CreatedAt:   time.Now(),
	}
}

func createAuthContext(userID string) context.Context {
	return context.WithValue(context.Background(), domain.UserIDKey, userID)
}

// Test GetPOIsByCity
func TestService_GetPOIsByCity(t *testing.T) {
	service, mockRepo := createTestService(t)

	tests := []struct {
		name        string
		request     *pb.GetPOIsByCityRequest
		setupMock   func()
		expectError bool
		errorCode   codes.Code
	}{
		{
			name: "success",
			request: &pb.GetPOIsByCityRequest{
				CityId: uuid.New().String(),
			},
			setupMock: func() {
				testPOI := createTestPOI()
				mockRepo.On("GetPOIsByCityID", mock.Anything, mock.AnythingOfType("uuid.UUID")).
					Return([]POIDetailedInfo{testPOI}, nil)
			},
			expectError: false,
		},
		{
			name: "empty city ID",
			request: &pb.GetPOIsByCityRequest{
				CityId: "",
			},
			setupMock:   func() {},
			expectError: true,
			errorCode:   codes.InvalidArgument,
		},
		{
			name: "invalid city ID format",
			request: &pb.GetPOIsByCityRequest{
				CityId: "invalid-uuid",
			},
			setupMock:   func() {},
			expectError: true,
			errorCode:   codes.InvalidArgument,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupMock()
			
			resp, err := service.GetPOIsByCity(context.Background(), tt.request)
			
			if tt.expectError {
				assert.Error(t, err)
				st, ok := status.FromError(err)
				assert.True(t, ok)
				assert.Equal(t, tt.errorCode, st.Code())
				assert.Nil(t, resp)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, resp)
				assert.NotEmpty(t, resp.Pois)
			}
			
			mockRepo.AssertExpectations(t)
		})
	}
}

// Test SearchPOIs
func TestService_SearchPOIs(t *testing.T) {
	service, mockRepo := createTestService(t)

	tests := []struct {
		name        string
		request     *pb.SearchPOIsRequest
		setupMock   func()
		expectError bool
		errorCode   codes.Code
	}{
		{
			name: "success",
			request: &pb.SearchPOIsRequest{
				Filter: &pb.POIFilter{
					Location: &pb.GeoPoint{
						Latitude:  40.7128,
						Longitude: -74.0060,
					},
					RadiusMeters: 1000,
					Query:        "restaurant",
				},
			},
			setupMock: func() {
				testPOI := createTestPOI()
				mockRepo.On("SearchPOIs", mock.Anything, mock.AnythingOfType("POIFilter")).
					Return([]POIDetailedInfo{testPOI}, nil)
			},
			expectError: false,
		},
		{
			name: "missing filter",
			request: &pb.SearchPOIsRequest{
				Filter: nil,
			},
			setupMock:   func() {},
			expectError: true,
			errorCode:   codes.InvalidArgument,
		},
		{
			name: "missing location",
			request: &pb.SearchPOIsRequest{
				Filter: &pb.POIFilter{
					Location: nil,
				},
			},
			setupMock:   func() {},
			expectError: true,
			errorCode:   codes.InvalidArgument,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupMock()
			
			resp, err := service.SearchPOIs(context.Background(), tt.request)
			
			if tt.expectError {
				assert.Error(t, err)
				st, ok := status.FromError(err)
				assert.True(t, ok)
				assert.Equal(t, tt.errorCode, st.Code())
				assert.Nil(t, resp)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, resp)
				assert.NotEmpty(t, resp.Pois)
			}
			
			mockRepo.AssertExpectations(t)
		})
	}
}

// Test GetNearbyRecommendations
func TestService_GetNearbyRecommendations(t *testing.T) {
	service, mockRepo := createTestService(t)
	userID := uuid.New().String()

	tests := []struct {
		name        string
		ctx         context.Context
		request     *pb.GetNearbyRecommendationsRequest
		setupMock   func()
		expectError bool
		errorCode   codes.Code
	}{
		{
			name: "success",
			ctx:  createAuthContext(userID),
			request: &pb.GetNearbyRecommendationsRequest{
				Location: &pb.LocationRequest{
					Latitude:  40.7128,
					Longitude: -74.0060,
				},
				RadiusMeters: 1000,
			},
			setupMock: func() {
				testPOI := createTestPOI()
				mockRepo.On("GetPOIsByLocationAndDistance", mock.Anything, 40.7128, -74.0060, 1000.0).
					Return([]POIDetailedInfo{testPOI}, nil)
			},
			expectError: false,
		},
		{
			name: "missing location",
			ctx:  createAuthContext(userID),
			request: &pb.GetNearbyRecommendationsRequest{
				Location: nil,
			},
			setupMock:   func() {},
			expectError: true,
			errorCode:   codes.InvalidArgument,
		},
		{
			name: "unauthenticated",
			ctx:  context.Background(),
			request: &pb.GetNearbyRecommendationsRequest{
				Location: &pb.LocationRequest{
					Latitude:  40.7128,
					Longitude: -74.0060,
				},
			},
			setupMock:   func() {},
			expectError: true,
			errorCode:   codes.Unauthenticated,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupMock()
			
			resp, err := service.GetNearbyRecommendations(tt.ctx, tt.request)
			
			if tt.expectError {
				assert.Error(t, err)
				st, ok := status.FromError(err)
				assert.True(t, ok)
				assert.Equal(t, tt.errorCode, st.Code())
				assert.Nil(t, resp)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, resp)
			}
			
			mockRepo.AssertExpectations(t)
		})
	}
}

// Test AddToFavorites
func TestService_AddToFavorites(t *testing.T) {
	service, mockRepo := createTestService(t)
	userID := uuid.New().String()
	poiID := uuid.New().String()
	favoriteID := uuid.New()

	tests := []struct {
		name        string
		ctx         context.Context
		request     *pb.AddToFavoritesRequest
		setupMock   func()
		expectError bool
		errorCode   codes.Code
	}{
		{
			name: "success",
			ctx:  createAuthContext(userID),
			request: &pb.AddToFavoritesRequest{
				PoiId:    poiID,
				IsLlmPoi: false,
			},
			setupMock: func() {
				mockRepo.On("AddPoiToFavourites", mock.Anything, 
					mock.AnythingOfType("uuid.UUID"), mock.AnythingOfType("uuid.UUID")).
					Return(favoriteID, nil)
			},
			expectError: false,
		},
		{
			name: "invalid POI ID",
			ctx:  createAuthContext(userID),
			request: &pb.AddToFavoritesRequest{
				PoiId:    "invalid-uuid",
				IsLlmPoi: false,
			},
			setupMock:   func() {},
			expectError: true,
			errorCode:   codes.InvalidArgument,
		},
		{
			name: "unauthenticated",
			ctx:  context.Background(),
			request: &pb.AddToFavoritesRequest{
				PoiId:    poiID,
				IsLlmPoi: false,
			},
			setupMock:   func() {},
			expectError: true,
			errorCode:   codes.Unauthenticated,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupMock()
			
			resp, err := service.AddToFavorites(tt.ctx, tt.request)
			
			if tt.expectError {
				assert.Error(t, err)
				st, ok := status.FromError(err)
				assert.True(t, ok)
				assert.Equal(t, tt.errorCode, st.Code())
				assert.Nil(t, resp)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, resp)
				assert.True(t, resp.Success)
				assert.NotEmpty(t, resp.FavoriteId)
			}
			
			mockRepo.AssertExpectations(t)
		})
	}
}

// Test GetFavorites
func TestService_GetFavorites(t *testing.T) {
	service, mockRepo := createTestService(t)
	userID := uuid.New().String()

	tests := []struct {
		name        string
		ctx         context.Context
		request     *pb.GetFavoritesRequest
		setupMock   func()
		expectError bool
		errorCode   codes.Code
	}{
		{
			name: "success",
			ctx:  createAuthContext(userID),
			request: &pb.GetFavoritesRequest{
				Limit:  10,
				Offset: 0,
			},
			setupMock: func() {
				testPOI := createTestPOI()
				mockRepo.On("GetFavouritePOIsByUserIDPaginated", mock.Anything, 
					mock.AnythingOfType("uuid.UUID"), 10, 0).
					Return([]POIDetailedInfo{testPOI}, 1, nil)
			},
			expectError: false,
		},
		{
			name: "unauthenticated",
			ctx:  context.Background(),
			request: &pb.GetFavoritesRequest{
				Limit:  10,
				Offset: 0,
			},
			setupMock:   func() {},
			expectError: true,
			errorCode:   codes.Unauthenticated,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupMock()
			
			resp, err := service.GetFavorites(tt.ctx, tt.request)
			
			if tt.expectError {
				assert.Error(t, err)
				st, ok := status.FromError(err)
				assert.True(t, ok)
				assert.Equal(t, tt.errorCode, st.Code())
				assert.Nil(t, resp)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, resp)
			}
			
			mockRepo.AssertExpectations(t)
		})
	}
}