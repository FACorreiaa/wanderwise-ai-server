package recents

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/FACorreiaa/go-poi-au-suggestions/internal/types"
)

var _ Service = (*ServiceImpl)(nil)

type Service interface {
	GetUserRecentInteractions(ctx context.Context, userID uuid.UUID, limit int) (*types.RecentInteractionsResponse, error)
	GetCityDetailsForUser(ctx context.Context, userID uuid.UUID, cityName string) (*types.CityInteractions, error)
}

type ServiceImpl struct {
	repo   Repository
	logger *slog.Logger
}

func NewService(repo Repository, logger *slog.Logger) *ServiceImpl {
	return &ServiceImpl{
		repo:   repo,
		logger: logger,
	}
}

// GetUserRecentInteractions retrieves recent interactions for a user
func (s *ServiceImpl) GetUserRecentInteractions(ctx context.Context, userID uuid.UUID, limit int) (*types.RecentInteractionsResponse, error) {
	ctx, span := otel.Tracer("RecentsService").Start(ctx, "GetUserRecentInteractions", trace.WithAttributes(
		attribute.String("user_id", userID.String()),
		attribute.Int("limit", limit),
	))
	defer span.End()

	l := s.logger.With(slog.String("method", "GetUserRecentInteractions"))

	// Validate limit
	if limit <= 0 {
		limit = 10
	}
	if limit > 50 {
		limit = 50
	}

	l.InfoContext(ctx, "Getting user recent interactions",
		slog.String("user_id", userID.String()),
		slog.Int("limit", limit))

	// Get recent interactions from repository
	response, err := s.repo.GetUserRecentInteractions(ctx, userID, limit)
	if err != nil {
		l.ErrorContext(ctx, "Failed to get recent interactions", slog.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to get recent interactions")
		return nil, fmt.Errorf("failed to get recent interactions: %w", err)
	}

	l.InfoContext(ctx, "Successfully retrieved recent interactions",
		slog.String("user_id", userID.String()),
		slog.Int("cities_count", len(response.Cities)))

	span.SetAttributes(attribute.Int("results.cities", len(response.Cities)))
	span.SetStatus(codes.Ok, "Recent interactions retrieved")

	return response, nil
}

// GetCityDetailsForUser retrieves detailed information for a specific city
func (s *ServiceImpl) GetCityDetailsForUser(ctx context.Context, userID uuid.UUID, cityName string) (*types.CityInteractions, error) {
	ctx, span := otel.Tracer("RecentsService").Start(ctx, "GetCityDetailsForUser", trace.WithAttributes(
		attribute.String("user_id", userID.String()),
		attribute.String("city_name", cityName),
	))
	defer span.End()

	l := s.logger.With(slog.String("method", "GetCityDetailsForUser"))

	if cityName == "" {
		err := fmt.Errorf("city name is required")
		l.ErrorContext(ctx, "City name is required")
		span.RecordError(err)
		span.SetStatus(codes.Error, "City name is required")
		return nil, err
	}

	l.InfoContext(ctx, "Getting city details for user",
		slog.String("user_id", userID.String()),
		slog.String("city_name", cityName))

	// Get recent interactions to find the city data
	recentResponse, err := s.repo.GetUserRecentInteractions(ctx, userID, 50) // Get more to find the city
	if err != nil {
		l.ErrorContext(ctx, "Failed to get recent interactions", slog.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to get recent interactions")
		return nil, fmt.Errorf("failed to get recent interactions: %w", err)
	}

	// Find the city in recent interactions
	var cityInteractions *types.CityInteractions
	for _, city := range recentResponse.Cities {
		if city.CityName == cityName {
			cityInteractions = &city
			break
		}
	}

	if cityInteractions == nil {
		err := fmt.Errorf("no interactions found for city: %s", cityName)
		l.WarnContext(ctx, "No interactions found for city", slog.String("city_name", cityName))
		span.SetStatus(codes.Error, "No interactions found")
		return nil, err
	}

	interactions := cityInteractions.Interactions
	poiCount := cityInteractions.POICount

	// Get POIs for the city
	pois, err := s.repo.GetCityPOIsByInteraction(ctx, userID, cityName)
	if err != nil {
		l.WarnContext(ctx, "Failed to get POIs for city",
			slog.String("city_name", cityName),
			slog.Any("error", err))
		pois = []types.POIDetailedInfo{} // Set to empty slice if we can't get POIs
	}

	// Get hotels for the city
	hotels, err := s.repo.GetCityHotelsByInteraction(ctx, userID, cityName)
	if err != nil {
		l.WarnContext(ctx, "Failed to get hotels for city",
			slog.String("city_name", cityName),
			slog.Any("error", err))
		hotels = []types.HotelDetailedInfo{} // Set to empty slice if we can't get hotels
	}

	// Get restaurants for the city
	restaurants, err := s.repo.GetCityRestaurantsByInteraction(ctx, userID, cityName)
	if err != nil {
		l.WarnContext(ctx, "Failed to get restaurants for city",
			slog.String("city_name", cityName),
			slog.Any("error", err))
		restaurants = []types.RestaurantDetailedInfo{} // Set to empty slice if we can't get restaurants
	}

	// Get saved itineraries for the city
	itineraries, err := s.repo.GetCityItinerariesByInteraction(ctx, userID, cityName)
	if err != nil {
		l.WarnContext(ctx, "Failed to get itineraries for city",
			slog.String("city_name", cityName),
			slog.Any("error", err))
		itineraries = []types.UserSavedItinerary{} // Set to empty slice if we can't get itineraries
	}

	// Get favorite POIs for the city
	favorites, err := s.repo.GetCityFavorites(ctx, userID, cityName)
	if err != nil {
		l.WarnContext(ctx, "Failed to get favorites for city",
			slog.String("city_name", cityName),
			slog.Any("error", err))
		favorites = []types.POIDetailedInfo{} // Set to empty slice if we can't get favorites
	}

	// Enrich interactions with POI/hotel/restaurant data
	for i := range interactions {
		interactions[i].POIs = convertPOIsToDetail(pois)
		interactions[i].Hotels = hotels
		interactions[i].Restaurants = restaurants
	}

	// Get the last activity timestamp
	var lastActivity time.Time
	if len(interactions) > 0 {
		lastActivity = interactions[0].CreatedAt // Interactions are ordered by created_at DESC
	}

	cityDetails := &types.CityInteractions{
		CityName:          cityName,
		Interactions:      interactions,
		POICount:          poiCount,
		LastActivity:      lastActivity,
		SavedItineraries:  itineraries,
		FavoritePOIs:      favorites,
		TotalInteractions: len(interactions),
		TotalFavorites:    len(favorites),
		TotalItineraries:  len(itineraries),
	}

	l.InfoContext(ctx, "Successfully retrieved city details",
		slog.String("user_id", userID.String()),
		slog.String("city_name", cityName),
		slog.Int("interaction_count", len(interactions)),
		slog.Int("poi_count", len(pois)),
		slog.Int("hotel_count", len(hotels)),
		slog.Int("restaurant_count", len(restaurants)),
		slog.Int("itinerary_count", len(itineraries)),
		slog.Int("favorite_count", len(favorites)))

	span.SetAttributes(
		attribute.Int("results.interactions", len(interactions)),
		attribute.Int("results.pois", len(pois)),
		attribute.Int("results.hotels", len(hotels)),
		attribute.Int("results.restaurants", len(restaurants)),
		attribute.Int("results.itineraries", len(itineraries)),
		attribute.Int("results.favorites", len(favorites)),
	)
	span.SetStatus(codes.Ok, "City details retrieved")

	return cityDetails, nil
}

// Helper function to convert POIDetailedInfo to POIDetail for consistency with existing types
func convertPOIsToDetail(detailedPOIs []types.POIDetailedInfo) []types.POIDetailedInfo {
	var pois []types.POIDetailedInfo
	for _, poi := range detailedPOIs {
		detail := types.POIDetailedInfo{
			ID:               poi.ID,
			LlmInteractionID: poi.LlmInteractionID,
			City:             poi.City,
			CityID:           poi.CityID,
			Name:             poi.Name,
			Latitude:         poi.Latitude,
			Longitude:        poi.Longitude,
			Category:         poi.Category,
			DescriptionPOI:   poi.Description,
			Address:          poi.Address,
			Website:          poi.Website,
			Distance:         poi.Distance,
		}
		pois = append(pois, detail)
	}
	return pois
}
