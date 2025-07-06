package recents

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/FACorreiaa/go-poi-au-suggestions/internal/types"
)

var _ Repository = (*RepositoryImpl)(nil)

type Repository interface {
	GetUserRecentInteractions(ctx context.Context, userID uuid.UUID, limit int) (*types.RecentInteractionsResponse, error)
	GetCityPOIsByInteraction(ctx context.Context, userID uuid.UUID, cityName string) ([]types.POIDetailedInfo, error)
	GetCityHotelsByInteraction(ctx context.Context, userID uuid.UUID, cityName string) ([]types.HotelDetailedInfo, error)
	GetCityRestaurantsByInteraction(ctx context.Context, userID uuid.UUID, cityName string) ([]types.RestaurantDetailedInfo, error)
	GetCityItinerariesByInteraction(ctx context.Context, userID uuid.UUID, cityName string) ([]types.UserSavedItinerary, error)
	GetCityFavorites(ctx context.Context, userID uuid.UUID, cityName string) ([]types.POIDetailedInfo, error)
}

type RepositoryImpl struct {
	pgpool *pgxpool.Pool
	logger *slog.Logger
}

func NewRepository(pgpool *pgxpool.Pool, logger *slog.Logger) *RepositoryImpl {
	return &RepositoryImpl{
		pgpool: pgpool,
		logger: logger,
	}
}

// GetUserRecentInteractions fetches recent interactions grouped by city
func (r *RepositoryImpl) GetUserRecentInteractions(ctx context.Context, userID uuid.UUID, limit int) (*types.RecentInteractionsResponse, error) {
	ctx, span := otel.Tracer("RecentsRepository").Start(ctx, "GetUserRecentInteractions", trace.WithAttributes(
		attribute.String("user_id", userID.String()),
		attribute.Int("limit", limit),
	))
	defer span.End()

	l := r.logger.With(slog.String("method", "GetUserRecentInteractions"))

	query := `
		SELECT DISTINCT
			l.city_name,
			MAX(l.created_at) as last_activity,
			COUNT(*) as interaction_count,
			l.session_id,
			CASE 
				WHEN l.city_name IS NOT NULL AND l.city_name != '' 
				THEN 'Trip to ' || l.city_name
				ELSE 'Travel Planning'
			END as title
		FROM llm_interactions l 
		WHERE user_id = $1 
			AND city_name != '' 
			AND city_name IS NOT NULL
		GROUP BY l.city_name, l.session_id 
		ORDER BY last_activity DESC 
		LIMIT $2
	`

	rows, err := r.pgpool.Query(ctx, query, userID, limit)
	if err != nil {
		l.ErrorContext(ctx, "Failed to query recent interactions", slog.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Database query failed")
		return nil, fmt.Errorf("failed to query recent interactions: %w", err)
	}
	defer rows.Close()

	var cities []types.CityInteractions
	for rows.Next() {
		var cityName string
		var lastActivity time.Time
		var interactionCount int
		var sessionID uuid.UUID
		var title string

		err := rows.Scan(&cityName, &lastActivity, &interactionCount, &sessionID, &title)
		if err != nil {
			l.ErrorContext(ctx, "Failed to scan city row", slog.Any("error", err))
			continue
		}

		// Get detailed interactions for this city
		interactions, err := r.getCityInteractions(ctx, userID, cityName)
		if err != nil {
			l.WarnContext(ctx, "Failed to get interactions for city",
				slog.String("city", cityName),
				slog.Any("error", err))
			continue
		}

		// Count POIs for this city
		poiCount, err := r.getCityPOICount(ctx, userID, cityName)
		if err != nil {
			l.WarnContext(ctx, "Failed to get POI count for city",
				slog.String("city", cityName),
				slog.Any("error", err))
			poiCount = 0
		}

		cities = append(cities, types.CityInteractions{
			CityName:     cityName,
			Interactions: interactions,
			POICount:     poiCount,
			LastActivity: lastActivity,
			SessionID:    sessionID,
			Title:        title,
		})
	}

	if err := rows.Err(); err != nil {
		l.ErrorContext(ctx, "Error iterating rows", slog.Any("error", err))
		span.RecordError(err)
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	total := len(cities)
	l.InfoContext(ctx, "Successfully retrieved recent interactions",
		slog.Int("cities_count", total),
		slog.String("user_id", userID.String()))

	span.SetAttributes(attribute.Int("results.cities", total))
	span.SetStatus(codes.Ok, "Recent interactions retrieved")

	return &types.RecentInteractionsResponse{
		Cities: cities,
		Total:  total,
	}, nil
}

// getCityInteractions gets recent interactions for a specific city
func (r *RepositoryImpl) getCityInteractions(ctx context.Context, userID uuid.UUID, cityName string) ([]types.RecentInteraction, error) {
	query := `
		SELECT 
			id,
			user_id,
			city_name,
			city_id,
			prompt,
			response,
			model_name,
			latency_ms,
			created_at
		FROM llm_interactions 
		WHERE user_id = $1 AND city_name = $2 
		ORDER BY created_at DESC 
		LIMIT 5
	`

	rows, err := r.pgpool.Query(ctx, query, userID, cityName)
	if err != nil {
		return nil, fmt.Errorf("failed to query city interactions: %w", err)
	}
	defer rows.Close()

	var interactions []types.RecentInteraction
	for rows.Next() {
		var interaction types.RecentInteraction
		var cityID *uuid.UUID
		var responseText *string

		err := rows.Scan(
			&interaction.ID,
			&interaction.UserID,
			&interaction.CityName,
			&cityID,
			&interaction.Prompt,
			&responseText,
			&interaction.ModelUsed,
			&interaction.LatencyMs,
			&interaction.CreatedAt,
		)
		if err != nil {
			r.logger.WarnContext(ctx, "Failed to scan interaction row", slog.Any("error", err))
			continue
		}

		interaction.CityID = cityID
		if responseText != nil {
			interaction.ResponseText = *responseText
		}

		interactions = append(interactions, interaction)
	}

	return interactions, nil
}

// getCityPOICount counts POIs for a city from user interactions
func (r *RepositoryImpl) getCityPOICount(ctx context.Context, userID uuid.UUID, cityName string) (int, error) {
	query := `
		SELECT COUNT(DISTINCT pd.id)
		FROM poi_details pd
		JOIN llm_interactions li ON pd.llm_interaction_id = li.id
		WHERE li.user_id = $1 AND li.city_name = $2
	`

	var count int
	err := r.pgpool.QueryRow(ctx, query, userID, cityName).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count POIs: %w", err)
	}

	return count, nil
}

// GetCityPOIsByInteraction gets all POIs for a city from user's interactions
func (r *RepositoryImpl) GetCityPOIsByInteraction(ctx context.Context, userID uuid.UUID, cityName string) ([]types.POIDetailedInfo, error) {
	ctx, span := otel.Tracer("RecentsRepository").Start(ctx, "GetCityPOIsByInteraction", trace.WithAttributes(
		attribute.String("user_id", userID.String()),
		attribute.String("city_name", cityName),
	))
	defer span.End()

	l := r.logger.With(slog.String("method", "GetCityPOIsByInteraction"))

	query := `
		SELECT DISTINCT 
			pd.id,
			pd.name,
			pd.latitude,
			pd.longitude,
			pd.description,
			pd.address,
			pd.website,
			pd.phone_number,
			pd.opening_hours,
			pd.price_range,
			pd.category,
			pd.tags,
			pd.images,
			pd.rating,
			pd.created_at
		FROM poi_details pd
		JOIN llm_interactions li ON pd.llm_interaction_id = li.id
		WHERE li.user_id = $1 AND li.city_name = $2
		ORDER BY pd.created_at DESC
	`

	rows, err := r.pgpool.Query(ctx, query, userID, cityName)
	if err != nil {
		l.ErrorContext(ctx, "Failed to query city POIs", slog.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Database query failed")
		return nil, fmt.Errorf("failed to query city POIs: %w", err)
	}
	defer rows.Close()

	var pois []types.POIDetailedInfo
	for rows.Next() {
		var poi types.POIDetailedInfo
		var description, address, website, phoneNumber, openingHours, priceRange, category *string
		var tags, images []string

		err := rows.Scan(
			&poi.ID,
			&poi.Name,
			&poi.Latitude,
			&poi.Longitude,
			&description,
			&address,
			&website,
			&phoneNumber,
			&openingHours,
			&priceRange,
			&category,
			&tags,
			&images,
			&poi.Rating,
			&poi.CreatedAt,
		)
		if err != nil {
			l.WarnContext(ctx, "Failed to scan POI row", slog.Any("error", err))
			continue
		}

		// Handle nullable fields
		if description != nil {
			poi.Description = *description
		}
		if address != nil {
			poi.Address = *address
		}
		if website != nil {
			poi.Website = *website
		}
		if phoneNumber != nil {
			poi.PhoneNumber = *phoneNumber
		}
		if openingHours != nil {
			poi.OpeningHours = map[string]string{"general": *openingHours}
		}
		if priceRange != nil {
			poi.PriceRange = *priceRange
		}
		if category != nil {
			poi.Category = *category
		}
		poi.Tags = tags
		poi.Images = images

		pois = append(pois, poi)
	}

	if err := rows.Err(); err != nil {
		l.ErrorContext(ctx, "Error iterating POI rows", slog.Any("error", err))
		span.RecordError(err)
		return nil, fmt.Errorf("error iterating POI rows: %w", err)
	}

	l.InfoContext(ctx, "Successfully retrieved city POIs",
		slog.String("city_name", cityName),
		slog.Int("poi_count", len(pois)))

	span.SetAttributes(attribute.Int("results.pois", len(pois)))
	span.SetStatus(codes.Ok, "City POIs retrieved")

	return pois, nil
}

// GetCityHotelsByInteraction gets all hotels for a city from user's interactions
func (r *RepositoryImpl) GetCityHotelsByInteraction(ctx context.Context, userID uuid.UUID, cityName string) ([]types.HotelDetailedInfo, error) {
	ctx, span := otel.Tracer("RecentsRepository").Start(ctx, "GetCityHotelsByInteraction", trace.WithAttributes(
		attribute.String("user_id", userID.String()),
		attribute.String("city_name", cityName),
	))
	defer span.End()

	l := r.logger.With(slog.String("method", "GetCityHotelsByInteraction"))

	query := `
		SELECT DISTINCT 
			hd.id,
			hd.name,
			hd.latitude,
			hd.longitude,
			hd.category,
			hd.description,
			hd.address,
			hd.website,
			hd.phone_number,
			hd.price_range,
			hd.tags,
			hd.images,
			hd.rating,
			hd.llm_interaction_id
		FROM hotel_details hd
		JOIN llm_interactions li ON hd.llm_interaction_id = li.id
		WHERE li.user_id = $1 AND li.city_name = $2
		ORDER BY hd.created_at DESC
	`

	rows, err := r.pgpool.Query(ctx, query, userID, cityName)
	if err != nil {
		l.ErrorContext(ctx, "Failed to query city hotels", slog.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Database query failed")
		return nil, fmt.Errorf("failed to query city hotels: %w", err)
	}
	defer rows.Close()

	var hotels []types.HotelDetailedInfo
	for rows.Next() {
		var hotel types.HotelDetailedInfo
		var category, description, address, website, phoneNumber, priceRange *string
		var tags, images []string

		err := rows.Scan(
			&hotel.ID,
			&hotel.Name,
			&hotel.Latitude,
			&hotel.Longitude,
			&category,
			&description,
			&address,
			&website,
			&phoneNumber,
			&priceRange,
			&tags,
			&images,
			&hotel.Rating,
			&hotel.LlmInteractionID,
		)
		if err != nil {
			l.WarnContext(ctx, "Failed to scan hotel row", slog.Any("error", err))
			continue
		}

		// Handle nullable fields
		if category != nil {
			hotel.Category = *category
		}
		if description != nil {
			hotel.Description = *description
		}
		if address != nil {
			hotel.Address = *address
		}
		if website != nil {
			hotel.Website = website
		}
		if phoneNumber != nil {
			hotel.PhoneNumber = phoneNumber
		}
		if priceRange != nil {
			hotel.PriceRange = priceRange
		}
		hotel.Tags = tags
		hotel.Images = images
		hotel.City = cityName

		hotels = append(hotels, hotel)
	}

	if err := rows.Err(); err != nil {
		l.ErrorContext(ctx, "Error iterating hotel rows", slog.Any("error", err))
		span.RecordError(err)
		return nil, fmt.Errorf("error iterating hotel rows: %w", err)
	}

	l.InfoContext(ctx, "Successfully retrieved city hotels",
		slog.String("city_name", cityName),
		slog.Int("hotel_count", len(hotels)))

	span.SetAttributes(attribute.Int("results.hotels", len(hotels)))
	span.SetStatus(codes.Ok, "City hotels retrieved")

	return hotels, nil
}

// GetCityRestaurantsByInteraction gets all restaurants for a city from user's interactions
func (r *RepositoryImpl) GetCityRestaurantsByInteraction(ctx context.Context, userID uuid.UUID, cityName string) ([]types.RestaurantDetailedInfo, error) {
	ctx, span := otel.Tracer("RecentsRepository").Start(ctx, "GetCityRestaurantsByInteraction", trace.WithAttributes(
		attribute.String("user_id", userID.String()),
		attribute.String("city_name", cityName),
	))
	defer span.End()

	l := r.logger.With(slog.String("method", "GetCityRestaurantsByInteraction"))

	query := `
		SELECT DISTINCT 
			rd.id,
			rd.name,
			rd.latitude,
			rd.longitude,
			rd.category,
			rd.description,
			rd.address,
			rd.website,
			rd.phone_number,
			rd.price_level,
			rd.cuisine_type,
			rd.tags,
			rd.images,
			rd.rating,
			rd.llm_interaction_id
		FROM restaurant_details rd
		JOIN llm_interactions li ON rd.llm_interaction_id = li.id
		WHERE li.user_id = $1 AND li.city_name = $2
		ORDER BY rd.created_at DESC
	`

	rows, err := r.pgpool.Query(ctx, query, userID, cityName)
	if err != nil {
		l.ErrorContext(ctx, "Failed to query city restaurants", slog.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Database query failed")
		return nil, fmt.Errorf("failed to query city restaurants: %w", err)
	}
	defer rows.Close()

	var restaurants []types.RestaurantDetailedInfo
	for rows.Next() {
		var restaurant types.RestaurantDetailedInfo
		var category, description *string
		var address, website, phoneNumber, priceLevel, cuisineType *string
		var tags, images []string

		err := rows.Scan(
			&restaurant.ID,
			&restaurant.Name,
			&restaurant.Latitude,
			&restaurant.Longitude,
			&category,
			&description,
			&address,
			&website,
			&phoneNumber,
			&priceLevel,
			&cuisineType,
			&tags,
			&images,
			&restaurant.Rating,
			&restaurant.LlmInteractionID,
		)
		if err != nil {
			l.WarnContext(ctx, "Failed to scan restaurant row", slog.Any("error", err))
			continue
		}

		// Handle nullable fields
		if category != nil {
			restaurant.Category = *category
		}
		if description != nil {
			restaurant.Description = *description
		}
		restaurant.Address = address
		restaurant.Website = website
		restaurant.PhoneNumber = phoneNumber
		restaurant.PriceLevel = priceLevel
		restaurant.CuisineType = cuisineType
		restaurant.Tags = tags
		restaurant.Images = images
		restaurant.City = cityName

		restaurants = append(restaurants, restaurant)
	}

	if err := rows.Err(); err != nil {
		l.ErrorContext(ctx, "Error iterating restaurant rows", slog.Any("error", err))
		span.RecordError(err)
		return nil, fmt.Errorf("error iterating restaurant rows: %w", err)
	}

	l.InfoContext(ctx, "Successfully retrieved city restaurants",
		slog.String("city_name", cityName),
		slog.Int("restaurant_count", len(restaurants)))

	span.SetAttributes(attribute.Int("results.restaurants", len(restaurants)))
	span.SetStatus(codes.Ok, "City restaurants retrieved")

	return restaurants, nil
}

// GetCityItinerariesByInteraction gets all saved itineraries for a city from user's interactions
func (r *RepositoryImpl) GetCityItinerariesByInteraction(ctx context.Context, userID uuid.UUID, cityName string) ([]types.UserSavedItinerary, error) {
	ctx, span := otel.Tracer("RecentsRepository").Start(ctx, "GetCityItinerariesByInteraction", trace.WithAttributes(
		attribute.String("user_id", userID.String()),
		attribute.String("city_name", cityName),
	))
	defer span.End()

	l := r.logger.With(slog.String("method", "GetCityItinerariesByInteraction"))

	query := `
		SELECT DISTINCT 
			usi.id,
			usi.user_id,
			usi.source_llm_interaction_id,
			usi.session_id,
			usi.primary_city_id,
			usi.title,
			usi.description,
			usi.markdown_content,
			usi.tags,
			usi.estimated_duration_days,
			usi.estimated_cost_level,
			usi.is_public,
			usi.created_at,
			usi.updated_at
		FROM user_saved_itineraries usi
		JOIN llm_interactions li ON (usi.source_llm_interaction_id = li.id OR usi.session_id IN (
			SELECT DISTINCT session_id FROM llm_interactions WHERE user_id = $1 AND city_name = $2
		))
		WHERE usi.user_id = $1 
		ORDER BY usi.created_at DESC
	`

	rows, err := r.pgpool.Query(ctx, query, userID, cityName)
	if err != nil {
		l.ErrorContext(ctx, "Failed to query city itineraries", slog.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Database query failed")
		return nil, fmt.Errorf("failed to query city itineraries: %w", err)
	}
	defer rows.Close()

	var itineraries []types.UserSavedItinerary
	for rows.Next() {
		var itinerary types.UserSavedItinerary

		err := rows.Scan(
			&itinerary.ID,
			&itinerary.UserID,
			&itinerary.SourceLlmInteractionID,
			&itinerary.SessionID,
			&itinerary.PrimaryCityID,
			&itinerary.Title,
			&itinerary.Description,
			&itinerary.MarkdownContent,
			&itinerary.Tags,
			&itinerary.EstimatedDurationDays,
			&itinerary.EstimatedCostLevel,
			&itinerary.IsPublic,
			&itinerary.CreatedAt,
			&itinerary.UpdatedAt,
		)
		if err != nil {
			l.WarnContext(ctx, "Failed to scan itinerary row", slog.Any("error", err))
			continue
		}

		itineraries = append(itineraries, itinerary)
	}

	if err := rows.Err(); err != nil {
		l.ErrorContext(ctx, "Error iterating itinerary rows", slog.Any("error", err))
		span.RecordError(err)
		return nil, fmt.Errorf("error iterating itinerary rows: %w", err)
	}

	l.InfoContext(ctx, "Successfully retrieved city itineraries",
		slog.String("city_name", cityName),
		slog.Int("itinerary_count", len(itineraries)))

	span.SetAttributes(attribute.Int("results.itineraries", len(itineraries)))
	span.SetStatus(codes.Ok, "City itineraries retrieved")

	return itineraries, nil
}

// GetCityFavorites gets all favorite POIs for a city (both regular and LLM POIs)
func (r *RepositoryImpl) GetCityFavorites(ctx context.Context, userID uuid.UUID, cityName string) ([]types.POIDetailedInfo, error) {
	ctx, span := otel.Tracer("RecentsRepository").Start(ctx, "GetCityFavorites", trace.WithAttributes(
		attribute.String("user_id", userID.String()),
		attribute.String("city_name", cityName),
	))
	defer span.End()

	l := r.logger.With(slog.String("method", "GetCityFavorites"))

	// Query both regular POI favorites and LLM POI favorites
	query := `
		-- Regular POI favorites
		SELECT DISTINCT 
			pd.id,
			pd.name,
			pd.latitude,
			pd.longitude,
			pd.description,
			pd.address,
			pd.website,
			pd.phone_number,
			pd.opening_hours,
			pd.price_range,
			pd.category,
			pd.tags,
			pd.images,
			pd.rating,
			pd.created_at
		FROM poi_details pd
		JOIN user_favorite_pois ufp ON pd.id = ufp.poi_id
		JOIN cities c ON pd.city_id = c.id
		WHERE ufp.user_id = $1 AND LOWER(c.name) = LOWER($2)
		
		UNION
		
		-- LLM POI favorites
		SELECT DISTINCT 
			lp.id,
			lp.name,
			lp.latitude,
			lp.longitude,
			lp.description,
			lp.address,
			lp.website,
			lp.phone_number,
			lp.opening_hours,
			lp.price_range,
			lp.category,
			lp.tags,
			lp.images,
			lp.rating,
			lp.created_at
		FROM llm_poi lp
		JOIN user_favorite_llm_pois uflp ON lp.id = uflp.llm_poi_id
		JOIN llm_interactions li ON lp.llm_interaction_id = li.id
		WHERE uflp.user_id = $1 AND LOWER(li.city_name) = LOWER($2)
		
		ORDER BY created_at DESC
	`

	rows, err := r.pgpool.Query(ctx, query, userID, cityName)
	if err != nil {
		l.ErrorContext(ctx, "Failed to query city favorites", slog.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Database query failed")
		return nil, fmt.Errorf("failed to query city favorites: %w", err)
	}
	defer rows.Close()

	var pois []types.POIDetailedInfo
	for rows.Next() {
		var poi types.POIDetailedInfo
		var description, address, website, phoneNumber, openingHours, priceRange, category *string
		var tags, images []string

		err := rows.Scan(
			&poi.ID,
			&poi.Name,
			&poi.Latitude,
			&poi.Longitude,
			&description,
			&address,
			&website,
			&phoneNumber,
			&openingHours,
			&priceRange,
			&category,
			&tags,
			&images,
			&poi.Rating,
			&poi.CreatedAt,
		)
		if err != nil {
			l.WarnContext(ctx, "Failed to scan favorite POI row", slog.Any("error", err))
			continue
		}

		// Handle nullable fields
		if description != nil {
			poi.Description = *description
		}
		if address != nil {
			poi.Address = *address
		}
		if website != nil {
			poi.Website = *website
		}
		if phoneNumber != nil {
			poi.PhoneNumber = *phoneNumber
		}
		if openingHours != nil {
			poi.OpeningHours = map[string]string{"general": *openingHours}
		}
		if priceRange != nil {
			poi.PriceRange = *priceRange
		}
		if category != nil {
			poi.Category = *category
		}
		poi.Tags = tags
		poi.Images = images

		pois = append(pois, poi)
	}

	if err := rows.Err(); err != nil {
		l.ErrorContext(ctx, "Error iterating favorite POI rows", slog.Any("error", err))
		span.RecordError(err)
		return nil, fmt.Errorf("error iterating favorite POI rows: %w", err)
	}

	l.InfoContext(ctx, "Successfully retrieved city favorites",
		slog.String("city_name", cityName),
		slog.Int("favorite_count", len(pois)))

	span.SetAttributes(attribute.Int("results.favorites", len(pois)))
	span.SetStatus(codes.Ok, "City favorites retrieved")

	return pois, nil
}
