package llmchat

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/FACorreiaa/go-poi-au-suggestions/internal/types"
)

type mockAIClient struct{}

type mockCityRepo struct{}

func (m *mockCityRepo) FindCityByNameAndCountry(_ context.Context, name, country string) (*types.CityDetail, error) {
	return &types.CityDetail{ID: uuid.New(), Name: name, Country: country}, nil
}

type mockPOIRepo struct {
	pois map[string]*types.POIDetailedInfo
}

func (m *mockPOIRepo) FindPOIDetailedInfos(_ context.Context, cityID uuid.UUID, lat, lon float64, tolerance float64) (*types.POIDetailedInfo, error) {
	key := fmt.Sprintf("%s:%.6f:%.6f", cityID.String(), lat, lon)
	if poi, exists := m.pois[key]; exists {
		return poi, nil
	}
	return nil, nil
}

func (m *mockPOIRepo) SavePOIDetailedInfos(_ context.Context, poi types.POIDetailedInfo, cityID uuid.UUID) (uuid.UUID, error) {
	key := fmt.Sprintf("%s:%.6f:%.6f", cityID.String(), poi.Latitude, poi.Longitude)
	m.pois[key] = &poi
	return uuid.New(), nil
}
