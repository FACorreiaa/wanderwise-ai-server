package llmChat

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/FACorreiaa/go-poi-au-suggestions/internal/types"
)

type mockAIClient struct{}

type mockCityRepo struct{}

func (m *mockCityRepo) FindCityByNameAndCountry(ctx context.Context, name, country string) (*types.CityDetail, error) {
	return &types.CityDetail{ID: uuid.New(), Name: name}, nil
}

type mockPOIRepo struct {
	pois map[string]*types.POIDetailedInfo
}

func (m *mockPOIRepo) FindPOIDetailedInfos(ctx context.Context, cityID uuid.UUID, lat, lon float64, tolerance float64) (*types.POIDetailedInfo, error) {
	key := fmt.Sprintf("%s:%.6f:%.6f", cityID.String(), lat, lon)
	if poi, exists := m.pois[key]; exists {
		return poi, nil
	}
	return nil, nil
}

func (m *mockPOIRepo) SavePOIDetailedInfos(ctx context.Context, poi types.POIDetailedInfo, cityID uuid.UUID) (uuid.UUID, error) {
	key := fmt.Sprintf("%s:%.6f:%.6f", cityID.String(), poi.Latitude, poi.Longitude)
	m.pois[key] = &poi
	return uuid.New(), nil
}

func (m *mockPOIRepo) FindPoiByNameAndCity(ctx context.Context, name string, cityID uuid.UUID) (*types.POIDetailedInfo, error) {
	return nil, nil
}

func (m *mockPOIRepo) SavePoi(ctx context.Context, poi types.POIDetailedInfo, cityID uuid.UUID) (uuid.UUID, error) {
	return uuid.New(), nil
}

type mockLlmRepo struct{}

func (m *mockLlmRepo) SaveInteraction(ctx context.Context, interaction types.LlmInteraction) (uuid.UUID, error) {
	return uuid.New(), nil
}

// func TestGetPOIDetailedInfosResponse_Database(t *testing.T) {
// 	logger := slog.New(slog.NewTextHandlerImpl(os.Stdout, nil))
// 	poiRepo := &mockPOIRepo{pois: make(map[string]*types.POIDetailedInfo)}
// 	service := &ServiceImpl{
// 		logger:             logger,
// 		aiClient:           &mockAIClient{},
// 		cityRepo:           &mockCityRepo{},
// 		poiRepo:            poiRepo,
// 		llmInteractionRepo: &mockLlmRepo{},
// 		cache:              cache.New(1*time.Hour, 10*time.Minute),
// 	}

// 	ctx := context.Background()
// 	userID := uuid.New()
// 	city := "Berlin"
// 	lat := 52.5200
// 	lon := 13.4050

// 	// First call: AI and database save
// 	poi, err := service.GetPOIDetailedInfosResponse(ctx, userID, city, lat, lon)
// 	assert.NoError(t, err)
// 	assert.Equal(t, "Cafe Berlin", poi.Name)

// 	// Second call: database hit
// 	poi2, err := service.GetPOIDetailedInfosResponse(ctx, userID, city, lat, lon)
// 	assert.NoError(t, err)
// 	assert.Equal(t, poi.Name, poi2.Name)

// 	// Third call: cache hit
// 	start := time.Now()
// 	poi3, err := service.GetPOIDetailedInfosResponse(ctx, userID, city, lat, lon)
// 	assert.NoError(t, err)
// 	assert.Equal(t, poi.Name, poi3.Name)
// 	assert.Less(t, time.Since(start).Milliseconds(), int64(10)) // Near-instant
// }
