package city

import (
	"context"
	"errors"
	"testing"

	pb "github.com/FACorreiaa/loci-proto/modules/city/generated"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// MockRepository implements the Repository interface for testing
type MockRepository struct {
	mock.Mock
}

func (m *MockRepository) SaveCity(ctx context.Context, city CityDetail) (uuid.UUID, error) {
	args := m.Called(ctx, city)
	return args.Get(0).(uuid.UUID), args.Error(1)
}

func (m *MockRepository) FindCityByNameAndCountry(ctx context.Context, city, country string) (*CityDetail, error) {
	args := m.Called(ctx, city, country)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*CityDetail), args.Error(1)
}

func (m *MockRepository) FindCityByFuzzyName(ctx context.Context, cityName string) (*CityDetail, error) {
	args := m.Called(ctx, cityName)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*CityDetail), args.Error(1)
}

func (m *MockRepository) GetCityIDByName(ctx context.Context, cityName string) (uuid.UUID, error) {
	args := m.Called(ctx, cityName)
	return args.Get(0).(uuid.UUID), args.Error(1)
}

func (m *MockRepository) GetAllCities(ctx context.Context) ([]CityDetail, error) {
	args := m.Called(ctx)
	return args.Get(0).([]CityDetail), args.Error(1)
}

func (m *MockRepository) FindSimilarCities(ctx context.Context, queryEmbedding []float32, limit int) ([]CityDetail, error) {
	args := m.Called(ctx, queryEmbedding, limit)
	return args.Get(0).([]CityDetail), args.Error(1)
}

func (m *MockRepository) UpdateCityEmbedding(ctx context.Context, cityID uuid.UUID, embedding []float32) error {
	args := m.Called(ctx, cityID, embedding)
	return args.Error(0)
}

func (m *MockRepository) GetCitiesWithoutEmbeddings(ctx context.Context, limit int) ([]CityDetail, error) {
	args := m.Called(ctx, limit)
	return args.Get(0).([]CityDetail), args.Error(1)
}

func (m *MockRepository) GetCity(ctx context.Context, lat, lon float64) (uuid.UUID, string, error) {
	args := m.Called(ctx, lat, lon)
	return args.Get(0).(uuid.UUID), args.Get(1).(string), args.Error(2)
}

// Helper function to create a test service
func createTestService() (*Service, *MockRepository) {
	mockRepo := &MockRepository{}
	logger := zap.NewNop()
	ctx := context.Background()
	service := NewService(ctx, mockRepo, &pgxpool.Pool{}, logger)
	return service, mockRepo
}

// Helper function to create sample city data
func createSampleCity() CityDetail {
	cityID := uuid.New()
	return CityDetail{
		ID:              cityID,
		Name:            "New York",
		Country:         "United States",
		StateProvince:   "New York",
		AiSummary:       "The city that never sleeps",
		CenterLatitude:  40.7128,
		CenterLongitude: -74.0060,
	}
}

func createSampleCities() []CityDetail {
	return []CityDetail{
		createSampleCity(),
		{
			ID:              uuid.New(),
			Name:            "Los Angeles",
			Country:         "United States",
			StateProvince:   "California",
			AiSummary:       "City of Angels",
			CenterLatitude:  34.0522,
			CenterLongitude: -118.2437,
		},
		{
			ID:              uuid.New(),
			Name:            "London",
			Country:         "United Kingdom",
			StateProvince:   "England",
			AiSummary:       "Historic capital of England",
			CenterLatitude:  51.5074,
			CenterLongitude: -0.1278,
		},
	}
}

func TestService_GetCities(t *testing.T) {
	service, mockRepo := createTestService()
	ctx := context.Background()

	tests := []struct {
		name           string
		request        *pb.GetCitiesRequest
		mockSetup      func(*MockRepository)
		expectedError  bool
		expectedStatus codes.Code
		validateResp   func(*testing.T, *pb.GetCitiesResponse)
	}{
		{
			name: "successful_get_cities",
			request: &pb.GetCitiesRequest{
				Limit:       10,
				Offset:      0,
				CountryCode: "",
				PopularOnly: false,
				Request: &pb.BaseRequest{
					RequestId: "test-request-1",
				},
			},
			mockSetup: func(repo *MockRepository) {
				cities := createSampleCities()
				repo.On("GetAllCities", mock.Anything).Return(cities, nil)
			},
			expectedError: false,
			validateResp: func(t *testing.T, resp *pb.GetCitiesResponse) {
				assert.NotNil(t, resp)
				assert.Len(t, resp.Cities, 3)
				assert.Equal(t, int32(3), resp.TotalCount)
				assert.Equal(t, "success", resp.Response.Status)
				assert.Equal(t, "test-request-1", resp.Response.RequestId)
			},
		},
		{
			name: "successful_get_cities_with_pagination",
			request: &pb.GetCitiesRequest{
				Limit:       2,
				Offset:      1,
				CountryCode: "",
				PopularOnly: false,
				Request: &pb.BaseRequest{
					RequestId: "test-request-2",
				},
			},
			mockSetup: func(repo *MockRepository) {
				cities := createSampleCities()
				repo.On("GetAllCities", mock.Anything).Return(cities, nil)
			},
			expectedError: false,
			validateResp: func(t *testing.T, resp *pb.GetCitiesResponse) {
				assert.NotNil(t, resp)
				assert.Len(t, resp.Cities, 2) // Should return 2 cities (offset 1, limit 2)
				assert.Equal(t, int32(3), resp.TotalCount) // Total count should still be 3
				assert.Equal(t, "success", resp.Response.Status)
			},
		},
		{
			name: "default_limit_applied",
			request: &pb.GetCitiesRequest{
				Limit:       0, // Should use default limit of 50
				Offset:      0,
				CountryCode: "",
				PopularOnly: false,
			},
			mockSetup: func(repo *MockRepository) {
				cities := createSampleCities()
				repo.On("GetAllCities", mock.Anything).Return(cities, nil)
			},
			expectedError: false,
			validateResp: func(t *testing.T, resp *pb.GetCitiesResponse) {
				assert.NotNil(t, resp)
				assert.Len(t, resp.Cities, 3) // All cities should be returned
				assert.Equal(t, int32(3), resp.TotalCount)
			},
		},
		{
			name: "max_limit_enforced",
			request: &pb.GetCitiesRequest{
				Limit:       200, // Should be capped at 100
				Offset:      0,
				CountryCode: "",
				PopularOnly: false,
			},
			mockSetup: func(repo *MockRepository) {
				cities := createSampleCities()
				repo.On("GetAllCities", mock.Anything).Return(cities, nil)
			},
			expectedError: false,
			validateResp: func(t *testing.T, resp *pb.GetCitiesResponse) {
				assert.NotNil(t, resp)
				assert.Len(t, resp.Cities, 3) // All cities should be returned
				assert.Equal(t, int32(3), resp.TotalCount)
			},
		},
		{
			name: "repository_error",
			request: &pb.GetCitiesRequest{
				Limit:       10,
				Offset:      0,
				CountryCode: "",
				PopularOnly: false,
			},
			mockSetup: func(repo *MockRepository) {
				repo.On("GetAllCities", mock.Anything).Return([]CityDetail{}, errors.New("database error"))
			},
			expectedError:  true,
			expectedStatus: codes.Internal,
			validateResp: func(t *testing.T, resp *pb.GetCitiesResponse) {
				assert.NotNil(t, resp)
				assert.Nil(t, resp.Cities)
				assert.Equal(t, int32(0), resp.TotalCount)
				assert.Equal(t, "error", resp.Response.Status)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset mock
			mockRepo.ExpectedCalls = nil
			mockRepo.Calls = nil

			// Setup mock
			tt.mockSetup(mockRepo)

			// Call service method
			resp, err := service.GetCities(ctx, tt.request)

			// Validate error expectation
			if tt.expectedError {
				assert.Error(t, err)
				if tt.expectedStatus != codes.OK {
					st, ok := status.FromError(err)
					assert.True(t, ok)
					assert.Equal(t, tt.expectedStatus, st.Code())
				}
			} else {
				assert.NoError(t, err)
			}

			// Validate response
			if tt.validateResp != nil {
				tt.validateResp(t, resp)
			}

			// Verify mock expectations
			mockRepo.AssertExpectations(t)
		})
	}
}

func TestService_GetCity(t *testing.T) {
	service, mockRepo := createTestService()
	ctx := context.Background()

	sampleCity := createSampleCity()
	sampleCities := []CityDetail{sampleCity}

	tests := []struct {
		name           string
		request        *pb.GetCityRequest
		mockSetup      func(*MockRepository)
		expectedError  bool
		expectedStatus codes.Code
		validateResp   func(*testing.T, *pb.GetCityResponse)
	}{
		{
			name: "successful_get_city",
			request: &pb.GetCityRequest{
				CityId: sampleCity.ID.String(),
				Request: &pb.BaseRequest{
					RequestId: "test-request-1",
				},
			},
			mockSetup: func(repo *MockRepository) {
				repo.On("GetAllCities", mock.Anything).Return(sampleCities, nil)
			},
			expectedError: false,
			validateResp: func(t *testing.T, resp *pb.GetCityResponse) {
				assert.NotNil(t, resp)
				assert.NotNil(t, resp.City)
				assert.Equal(t, sampleCity.ID.String(), resp.City.Id)
				assert.Equal(t, sampleCity.Name, resp.City.Name)
				assert.Equal(t, "success", resp.Response.Status)
				assert.Equal(t, "test-request-1", resp.Response.RequestId)
			},
		},
		{
			name: "empty_city_id",
			request: &pb.GetCityRequest{
				CityId: "",
				Request: &pb.BaseRequest{
					RequestId: "test-request-2",
				},
			},
			mockSetup:      func(repo *MockRepository) {},
			expectedError:  true,
			expectedStatus: codes.InvalidArgument,
			validateResp: func(t *testing.T, resp *pb.GetCityResponse) {
				assert.NotNil(t, resp)
				assert.Nil(t, resp.City)
				assert.Equal(t, "error", resp.Response.Status)
			},
		},
		{
			name: "invalid_city_id_format",
			request: &pb.GetCityRequest{
				CityId: "invalid-uuid",
				Request: &pb.BaseRequest{
					RequestId: "test-request-3",
				},
			},
			mockSetup:      func(repo *MockRepository) {},
			expectedError:  true,
			expectedStatus: codes.InvalidArgument,
			validateResp: func(t *testing.T, resp *pb.GetCityResponse) {
				assert.NotNil(t, resp)
				assert.Nil(t, resp.City)
				assert.Equal(t, "error", resp.Response.Status)
			},
		},
		{
			name: "city_not_found",
			request: &pb.GetCityRequest{
				CityId: uuid.New().String(), // Different UUID
				Request: &pb.BaseRequest{
					RequestId: "test-request-4",
				},
			},
			mockSetup: func(repo *MockRepository) {
				repo.On("GetAllCities", mock.Anything).Return(sampleCities, nil)
			},
			expectedError:  true,
			expectedStatus: codes.NotFound,
			validateResp: func(t *testing.T, resp *pb.GetCityResponse) {
				assert.NotNil(t, resp)
				assert.Nil(t, resp.City)
				assert.Equal(t, "error", resp.Response.Status)
			},
		},
		{
			name: "repository_error",
			request: &pb.GetCityRequest{
				CityId: sampleCity.ID.String(),
				Request: &pb.BaseRequest{
					RequestId: "test-request-5",
				},
			},
			mockSetup: func(repo *MockRepository) {
				repo.On("GetAllCities", mock.Anything).Return([]CityDetail{}, errors.New("database error"))
			},
			expectedError:  true,
			expectedStatus: codes.Internal,
			validateResp: func(t *testing.T, resp *pb.GetCityResponse) {
				assert.NotNil(t, resp)
				assert.Nil(t, resp.City)
				assert.Equal(t, "error", resp.Response.Status)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset mock
			mockRepo.ExpectedCalls = nil
			mockRepo.Calls = nil

			// Setup mock
			tt.mockSetup(mockRepo)

			// Call service method
			resp, err := service.GetCity(ctx, tt.request)

			// Validate error expectation
			if tt.expectedError {
				assert.Error(t, err)
				if tt.expectedStatus != codes.OK {
					st, ok := status.FromError(err)
					assert.True(t, ok)
					assert.Equal(t, tt.expectedStatus, st.Code())
				}
			} else {
				assert.NoError(t, err)
			}

			// Validate response
			if tt.validateResp != nil {
				tt.validateResp(t, resp)
			}

			// Verify mock expectations
			mockRepo.AssertExpectations(t)
		})
	}
}

func TestService_SearchCities(t *testing.T) {
	service, mockRepo := createTestService()
	ctx := context.Background()

	sampleCity := createSampleCity()

	tests := []struct {
		name           string
		request        *pb.SearchCitiesRequest
		mockSetup      func(*MockRepository)
		expectedError  bool
		expectedStatus codes.Code
		validateResp   func(*testing.T, *pb.SearchCitiesResponse)
	}{
		{
			name: "successful_exact_search",
			request: &pb.SearchCitiesRequest{
				Query:       "New York",
				CountryCode: "US",
				FuzzySearch: false,
				Request: &pb.BaseRequest{
					RequestId: "test-request-1",
				},
			},
			mockSetup: func(repo *MockRepository) {
				repo.On("FindCityByNameAndCountry", mock.Anything, "New York", "US").Return(&sampleCity, nil)
			},
			expectedError: false,
			validateResp: func(t *testing.T, resp *pb.SearchCitiesResponse) {
				assert.NotNil(t, resp)
				assert.Len(t, resp.Results, 1)
				assert.Equal(t, int32(1), resp.TotalCount)
				assert.Equal(t, sampleCity.Name, resp.Results[0].City.Name)
				assert.Equal(t, float64(1.0), resp.Results[0].RelevanceScore)
				assert.Equal(t, "exact_name_match", resp.Results[0].MatchReason)
				assert.Equal(t, "success", resp.Response.Status)
				assert.NotNil(t, resp.Metadata)
				assert.Equal(t, "name_search", resp.Metadata.SearchMethod)
				assert.False(t, resp.Metadata.FuzzyMatchingUsed)
			},
		},
		{
			name: "successful_fuzzy_search",
			request: &pb.SearchCitiesRequest{
				Query:       "New Yrok", // Typo
				CountryCode: "",
				FuzzySearch: true,
				Request: &pb.BaseRequest{
					RequestId: "test-request-2",
				},
			},
			mockSetup: func(repo *MockRepository) {
				repo.On("FindCityByFuzzyName", mock.Anything, "New Yrok").Return(&sampleCity, nil)
			},
			expectedError: false,
			validateResp: func(t *testing.T, resp *pb.SearchCitiesResponse) {
				assert.NotNil(t, resp)
				assert.Len(t, resp.Results, 1)
				assert.Equal(t, int32(1), resp.TotalCount)
				assert.Equal(t, sampleCity.Name, resp.Results[0].City.Name)
				assert.True(t, resp.Metadata.FuzzyMatchingUsed)
				assert.Equal(t, "success", resp.Response.Status)
			},
		},
		{
			name: "empty_query",
			request: &pb.SearchCitiesRequest{
				Query:       "",
				CountryCode: "US",
				FuzzySearch: false,
				Request: &pb.BaseRequest{
					RequestId: "test-request-3",
				},
			},
			mockSetup:      func(repo *MockRepository) {},
			expectedError:  true,
			expectedStatus: codes.InvalidArgument,
			validateResp: func(t *testing.T, resp *pb.SearchCitiesResponse) {
				assert.NotNil(t, resp)
				assert.Nil(t, resp.Results)
				assert.Equal(t, int32(0), resp.TotalCount)
				assert.Equal(t, "error", resp.Response.Status)
			},
		},
		{
			name: "city_not_found",
			request: &pb.SearchCitiesRequest{
				Query:       "Nonexistent City",
				CountryCode: "US",
				FuzzySearch: false,
				Request: &pb.BaseRequest{
					RequestId: "test-request-4",
				},
			},
			mockSetup: func(repo *MockRepository) {
				repo.On("FindCityByNameAndCountry", mock.Anything, "Nonexistent City", "US").Return(nil, nil)
			},
			expectedError: false,
			validateResp: func(t *testing.T, resp *pb.SearchCitiesResponse) {
				assert.NotNil(t, resp)
				assert.Len(t, resp.Results, 0)
				assert.Equal(t, int32(0), resp.TotalCount)
				assert.Equal(t, "success", resp.Response.Status)
			},
		},
		{
			name: "repository_error_exact_search",
			request: &pb.SearchCitiesRequest{
				Query:       "New York",
				CountryCode: "US",
				FuzzySearch: false,
				Request: &pb.BaseRequest{
					RequestId: "test-request-5",
				},
			},
			mockSetup: func(repo *MockRepository) {
				repo.On("FindCityByNameAndCountry", mock.Anything, "New York", "US").Return(nil, errors.New("database error"))
			},
			expectedError:  true,
			expectedStatus: codes.Internal,
			validateResp: func(t *testing.T, resp *pb.SearchCitiesResponse) {
				assert.NotNil(t, resp)
				assert.Nil(t, resp.Results)
				assert.Equal(t, int32(0), resp.TotalCount)
				assert.Equal(t, "error", resp.Response.Status)
			},
		},
		{
			name: "repository_error_fuzzy_search",
			request: &pb.SearchCitiesRequest{
				Query:       "New York",
				CountryCode: "",
				FuzzySearch: true,
				Request: &pb.BaseRequest{
					RequestId: "test-request-6",
				},
			},
			mockSetup: func(repo *MockRepository) {
				repo.On("FindCityByFuzzyName", mock.Anything, "New York").Return(nil, errors.New("database error"))
			},
			expectedError:  true,
			expectedStatus: codes.Internal,
			validateResp: func(t *testing.T, resp *pb.SearchCitiesResponse) {
				assert.NotNil(t, resp)
				assert.Nil(t, resp.Results)
				assert.Equal(t, int32(0), resp.TotalCount)
				assert.Equal(t, "error", resp.Response.Status)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset mock
			mockRepo.ExpectedCalls = nil
			mockRepo.Calls = nil

			// Setup mock
			tt.mockSetup(mockRepo)

			// Call service method
			resp, err := service.SearchCities(ctx, tt.request)

			// Validate error expectation
			if tt.expectedError {
				assert.Error(t, err)
				if tt.expectedStatus != codes.OK {
					st, ok := status.FromError(err)
					assert.True(t, ok)
					assert.Equal(t, tt.expectedStatus, st.Code())
				}
			} else {
				assert.NoError(t, err)
			}

			// Validate response
			if tt.validateResp != nil {
				tt.validateResp(t, resp)
			}

			// Verify mock expectations
			mockRepo.AssertExpectations(t)
		})
	}
}

func TestService_GetCityStatistics(t *testing.T) {
	service, mockRepo := createTestService()
	ctx := context.Background()

	sampleCity := createSampleCity()
	sampleCities := []CityDetail{sampleCity}

	tests := []struct {
		name           string
		request        *pb.GetCityStatisticsRequest
		mockSetup      func(*MockRepository)
		expectedError  bool
		expectedStatus codes.Code
		validateResp   func(*testing.T, *pb.GetCityStatisticsResponse)
	}{
		{
			name: "successful_get_city_statistics",
			request: &pb.GetCityStatisticsRequest{
				CityId:        sampleCity.ID.String(),
				IncludeTrends: false,
				Request: &pb.BaseRequest{
					RequestId: "test-request-1",
				},
			},
			mockSetup: func(repo *MockRepository) {
				repo.On("GetAllCities", mock.Anything).Return(sampleCities, nil)
			},
			expectedError: false,
			validateResp: func(t *testing.T, resp *pb.GetCityStatisticsResponse) {
				assert.NotNil(t, resp)
				assert.NotNil(t, resp.Statistics)
				assert.Equal(t, sampleCity.ID.String(), resp.Statistics.CityId)
				assert.Equal(t, int32(0), resp.Statistics.TotalPois)
				assert.Equal(t, int32(0), resp.Statistics.TotalRestaurants)
				assert.Equal(t, int32(0), resp.Statistics.TotalHotels)
				assert.Equal(t, int32(0), resp.Statistics.TotalAttractions)
				assert.Equal(t, int32(0), resp.Statistics.UserVisits)
				assert.Equal(t, int32(0), resp.Statistics.SavedItineraries)
				assert.Equal(t, float64(0.0), resp.Statistics.AverageRating)
				assert.NotNil(t, resp.Statistics.LastUpdated)
				assert.Equal(t, "success", resp.Response.Status)
				assert.Equal(t, "test-request-1", resp.Response.RequestId)
			},
		},
		{
			name: "empty_city_id",
			request: &pb.GetCityStatisticsRequest{
				CityId:        "",
				IncludeTrends: false,
				Request: &pb.BaseRequest{
					RequestId: "test-request-2",
				},
			},
			mockSetup:      func(repo *MockRepository) {},
			expectedError:  true,
			expectedStatus: codes.InvalidArgument,
			validateResp: func(t *testing.T, resp *pb.GetCityStatisticsResponse) {
				assert.NotNil(t, resp)
				assert.Nil(t, resp.Statistics)
				assert.Equal(t, "error", resp.Response.Status)
			},
		},
		{
			name: "invalid_city_id_format",
			request: &pb.GetCityStatisticsRequest{
				CityId:        "invalid-uuid",
				IncludeTrends: false,
				Request: &pb.BaseRequest{
					RequestId: "test-request-3",
				},
			},
			mockSetup:      func(repo *MockRepository) {},
			expectedError:  true,
			expectedStatus: codes.InvalidArgument,
			validateResp: func(t *testing.T, resp *pb.GetCityStatisticsResponse) {
				assert.NotNil(t, resp)
				assert.Nil(t, resp.Statistics)
				assert.Equal(t, "error", resp.Response.Status)
			},
		},
		{
			name: "city_not_found",
			request: &pb.GetCityStatisticsRequest{
				CityId:        uuid.New().String(), // Different UUID
				IncludeTrends: false,
				Request: &pb.BaseRequest{
					RequestId: "test-request-4",
				},
			},
			mockSetup: func(repo *MockRepository) {
				repo.On("GetAllCities", mock.Anything).Return(sampleCities, nil)
			},
			expectedError:  true,
			expectedStatus: codes.NotFound,
			validateResp: func(t *testing.T, resp *pb.GetCityStatisticsResponse) {
				assert.NotNil(t, resp)
				assert.Nil(t, resp.Statistics)
				assert.Equal(t, "error", resp.Response.Status)
			},
		},
		{
			name: "repository_error",
			request: &pb.GetCityStatisticsRequest{
				CityId:        sampleCity.ID.String(),
				IncludeTrends: false,
				Request: &pb.BaseRequest{
					RequestId: "test-request-5",
				},
			},
			mockSetup: func(repo *MockRepository) {
				repo.On("GetAllCities", mock.Anything).Return([]CityDetail{}, errors.New("database error"))
			},
			expectedError:  true,
			expectedStatus: codes.Internal,
			validateResp: func(t *testing.T, resp *pb.GetCityStatisticsResponse) {
				assert.NotNil(t, resp)
				assert.Nil(t, resp.Statistics)
				assert.Equal(t, "error", resp.Response.Status)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset mock
			mockRepo.ExpectedCalls = nil
			mockRepo.Calls = nil

			// Setup mock
			tt.mockSetup(mockRepo)

			// Call service method
			resp, err := service.GetCityStatistics(ctx, tt.request)

			// Validate error expectation
			if tt.expectedError {
				assert.Error(t, err)
				if tt.expectedStatus != codes.OK {
					st, ok := status.FromError(err)
					assert.True(t, ok)
					assert.Equal(t, tt.expectedStatus, st.Code())
				}
			} else {
				assert.NoError(t, err)
			}

			// Validate response
			if tt.validateResp != nil {
				tt.validateResp(t, resp)
			}

			// Verify mock expectations
			mockRepo.AssertExpectations(t)
		})
	}
}

func TestConvertToPBCity(t *testing.T) {
	tests := []struct {
		name     string
		input    CityDetail
		validate func(*testing.T, *pb.City)
	}{
		{
			name: "complete_city_conversion",
			input: CityDetail{
				ID:              uuid.MustParse("123e4567-e89b-12d3-a456-426614174000"),
				Name:            "New York",
				Country:         "United States",
				StateProvince:   "New York",
				AiSummary:       "The city that never sleeps",
				CenterLatitude:  40.7128,
				CenterLongitude: -74.0060,
			},
			validate: func(t *testing.T, city *pb.City) {
				assert.Equal(t, "123e4567-e89b-12d3-a456-426614174000", city.Id)
				assert.Equal(t, "New York", city.Name)
				assert.Equal(t, "United States", city.Country)
				assert.Equal(t, "New York", city.StateProvince)
				assert.Equal(t, "The city that never sleeps", city.Description)
				assert.Equal(t, float64(40.7128), city.Latitude)
				assert.Equal(t, float64(-74.0060), city.Longitude)
				assert.NotNil(t, city.Metadata)
				assert.NotNil(t, city.CreatedAt)
				assert.NotNil(t, city.UpdatedAt)
				
				// Check that unset fields have appropriate defaults
				assert.Equal(t, "", city.CountryCode)
				assert.Equal(t, "", city.Timezone)
				assert.Equal(t, int64(0), city.Population)
				assert.Equal(t, "", city.Currency)
				assert.Empty(t, city.Languages)
				assert.Empty(t, city.Highlights)
				assert.Equal(t, "", city.Climate)
				assert.Equal(t, "", city.BestTimeToVisit)
				assert.Empty(t, city.TopAttractions)
			},
		},
		{
			name: "minimal_city_conversion",
			input: CityDetail{
				ID:              uuid.MustParse("987fcdeb-51a2-43d1-9f12-345678901234"),
				Name:            "Test City",
				Country:         "Test Country",
				StateProvince:   "",
				AiSummary:       "",
				CenterLatitude:  0.0,
				CenterLongitude: 0.0,
			},
			validate: func(t *testing.T, city *pb.City) {
				assert.Equal(t, "987fcdeb-51a2-43d1-9f12-345678901234", city.Id)
				assert.Equal(t, "Test City", city.Name)
				assert.Equal(t, "Test Country", city.Country)
				assert.Equal(t, "", city.StateProvince)
				assert.Equal(t, "", city.Description)
				assert.Equal(t, float64(0.0), city.Latitude)
				assert.Equal(t, float64(0.0), city.Longitude)
				assert.NotNil(t, city.Metadata)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertToPBCity(&tt.input)
			assert.NotNil(t, result)
			tt.validate(t, result)
		})
	}
}

func TestService_NewService(t *testing.T) {
	ctx := context.Background()
	mockRepo := &MockRepository{}
	logger := zap.NewNop()
	pgpool := &pgxpool.Pool{}

	service := NewService(ctx, mockRepo, pgpool, logger)

	assert.NotNil(t, service)
	assert.Equal(t, mockRepo, service.repo)
	assert.Equal(t, pgpool, service.pgpool)
	assert.NotNil(t, service.logger)
	assert.NotNil(t, service.tracer)
}

// Benchmark tests
func BenchmarkConvertToPBCity(b *testing.B) {
	city := createSampleCity()
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		convertToPBCity(&city)
	}
}

func BenchmarkService_GetCities(b *testing.B) {
	service, mockRepo := createTestService()
	ctx := context.Background()
	
	cities := createSampleCities()
	mockRepo.On("GetAllCities", mock.Anything).Return(cities, nil)
	
	request := &pb.GetCitiesRequest{
		Limit:       10,
		Offset:      0,
		CountryCode: "",
		PopularOnly: false,
	}
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = service.GetCities(ctx, request)
	}
}