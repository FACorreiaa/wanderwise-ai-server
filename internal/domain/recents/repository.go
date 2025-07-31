package recents

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

var _ Repository = (*RepositoryImpl)(nil)

type Repository interface {
	GetUserRecentInteractions(ctx context.Context, userID uuid.UUID, page, limit int, filterOptions *RecentInteractionsFilter) (*RecentInteractionsResponse, error)
	GetCityPOIsByInteraction(ctx context.Context, userID uuid.UUID, cityName string) ([]POIDetailedInfo, error)
	GetCityHotelsByInteraction(ctx context.Context, userID uuid.UUID, cityName string) ([]HotelDetailedInfo, error)
	GetCityRestaurantsByInteraction(ctx context.Context, userID uuid.UUID, cityName string) ([]RestaurantDetailedInfo, error)
	GetCityItinerariesByInteraction(ctx context.Context, userID uuid.UUID, cityName string) ([]UserSavedItinerary, error)
	GetCityFavorites(ctx context.Context, userID uuid.UUID, cityName string) ([]POIDetailedInfo, error)
}

type RepositoryImpl struct {
	pgpool *pgxpool.Pool
	logger *zap.Logger
}

func NewRepository(pgpool *pgxpool.Pool, logger *zap.Logger) *RepositoryImpl {
	return &RepositoryImpl{
		pgpool: pgpool,
		logger: logger,
	}
}

// GetUserRecentInteractions fetches recent interactions grouped by city
func (r *RepositoryImpl) GetUserRecentInteractions(ctx context.Context, userID uuid.UUID, page, limit int, filterOptions *RecentInteractionsFilter) (*RecentInteractionsResponse, error) {
	ctx, span := otel.Tracer("RecentsRepository").Start(ctx, "GetUserRecentInteractions", trace.WithAttributes(
		attribute.String("user_id", userID.String()),
		attribute.Int("page", page),
		attribute.Int("limit", limit),
		attribute.String("sort_by", filterOptions.SortBy),
		attribute.String("sort_order", filterOptions.SortOrder),
		attribute.String("search", filterOptions.Search),
	))
	defer span.End()

	l := r.logger.With(zap.String("method", "GetUserRecentInteractions"))

	// Build WHERE clause with filters
	whereConditions := []string{"user_id = $1", "city_name != ''", "city_name IS NOT NULL"}
	args := []interface{}{userID}
	argIndex := 2

	// Add search filter
	if filterOptions.Search != "" {
		whereConditions = append(whereConditions, fmt.Sprintf("LOWER(city_name) LIKE LOWER($%d)", argIndex))
		args = append(args, "%"+filterOptions.Search+"%")
		argIndex++
	}

	// Build HAVING clause for interaction count filters
	havingConditions := []string{}
	if filterOptions.MinInteractions >= 0 {
		havingConditions = append(havingConditions, fmt.Sprintf("COUNT(*) >= %d", filterOptions.MinInteractions))
	}
	if filterOptions.MaxInteractions >= 0 {
		havingConditions = append(havingConditions, fmt.Sprintf("COUNT(*) <= %d", filterOptions.MaxInteractions))
	}

	// Build ORDER BY clause
	var orderBy string
	switch filterOptions.SortBy {
	case "city_name":
		orderBy = "city_name"
	case "interaction_count":
		orderBy = "interaction_count"
	case "poi_count":
		orderBy = "poi_count"
	default:
		orderBy = "last_activity"
	}

	if filterOptions.SortOrder == "asc" {
		orderBy += " ASC"
	} else {
		orderBy += " DESC"
	}

	// Build the main query - we need a subquery to properly aggregate by city
	subquery := fmt.Sprintf(`
        SELECT 
            l.city_name,
            MAX(l.created_at) as last_activity,
            COUNT(*) as interaction_count,
            (
                SELECT session_id 
                FROM llm_interactions llmi 
                WHERE llmi.user_id = l.user_id 
                  AND llmi.city_name = l.city_name 
                ORDER BY llmi.created_at DESC 
                LIMIT 1
            ) as session_id,
            CASE 
                WHEN l.city_name IS NOT NULL AND l.city_name != '' 
                THEN 'Trip to ' || l.city_name
                ELSE 'Travel Planning'
            END as title,
            COALESCE((
                SELECT COUNT(DISTINCT pd.id)
                FROM poi_details pd
                JOIN llm_interactions li ON pd.llm_interaction_id = li.id
                WHERE li.user_id = l.user_id AND li.city_name = l.city_name
            ), 0) as poi_count
        FROM llm_interactions l 
        WHERE %s
        GROUP BY l.city_name, l.user_id
        %s
    `, strings.Join(whereConditions, " AND "),
		func() string {
			if len(havingConditions) > 0 {
				return "HAVING " + strings.Join(havingConditions, " AND ")
			}
			return ""
		}())

	query := fmt.Sprintf(`
        SELECT 
            city_name,
            last_activity,
            interaction_count,
            session_id,
            title,
            poi_count
        FROM (%s) as city_data
        ORDER BY %s 
        LIMIT $%d OFFSET $%d
    `, subquery, orderBy, argIndex, argIndex+1)

	args = append(args, limit, (page-1)*limit)
	l.Info("Executing query",
		zap.String("query", query),
		zap.Any("params", args))

	rows, err := r.pgpool.Query(ctx, query, args...)
	if err != nil {
		l.Error("Failed to query recent interactions", zap.Error(err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Database query failed")
		return nil, fmt.Errorf("failed to query recent interactions: %w", err)
	}
	defer rows.Close()

	var cities []CityInteractions
	for rows.Next() {
		var cityName string
		var lastActivity time.Time
		var interactionCount int
		var sessionID uuid.UUID
		var title string
		var poiCount int

		err := rows.Scan(&cityName, &lastActivity, &interactionCount, &sessionID, &title, &poiCount)
		if err != nil {
			l.Error("Failed to scan city row", zap.Error(err))
			continue
		}

		interactions, err := r.getCityInteractions(ctx, userID, cityName)
		if err != nil {
			l.Warn("Failed to get interactions for city",
				zap.String("city", cityName),
				zap.Error(err))
			continue
		}

		// POI count is now included in the main query

		cities = append(cities, CityInteractions{
			CityName:     cityName,
			Interactions: interactions,
			SessionIDs:   []uuid.UUID{sessionID},
			POICount:     poiCount,
			LastActivity: lastActivity,
			SessionID:    sessionID,
			Title:        title,
		})
	}

	if err := rows.Err(); err != nil {
		l.Error("Error iterating rows", zap.Error(err))
		span.RecordError(err)
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	// Build count query with same filters - count cities, not sessions
	countSubquery := fmt.Sprintf(`
        SELECT 
            l.city_name,
            COUNT(*) as interaction_count
        FROM llm_interactions l
        WHERE %s
        GROUP BY l.city_name, l.user_id
        %s
    `, strings.Join(whereConditions, " AND "),
		func() string {
			if len(havingConditions) > 0 {
				return "HAVING " + strings.Join(havingConditions, " AND ")
			}
			return ""
		}())

	// For count, we need to count the results from the grouped query
	countWrapperQuery := fmt.Sprintf("SELECT COUNT(*) FROM (%s) as grouped_results", countSubquery)

	// Use the same args except for limit and offset
	countArgs := args[:len(args)-2]

	var total int
	err = r.pgpool.QueryRow(ctx, countWrapperQuery, countArgs...).Scan(&total)
	if err != nil {
		l.Error("Failed to query total recent interactions count", zap.Error(err))
		// Handle the error - you could return an error or default to 0
		return nil, fmt.Errorf("failed to get total count: %w", err)
	}

	l.Info("Successfully retrieved recent interactions",
		zap.Int("cities_count", total),
		zap.String("user_id", userID.String()))

	span.SetAttributes(attribute.Int("results.cities", total))
	span.SetStatus(codes.Ok, "Recent interactions retrieved")

	return &RecentInteractionsResponse{
		Cities: cities,
		Total:  total,
	}, nil
}

// getCityInteractions gets recent interactions for a specific city
func (r *RepositoryImpl) getCityInteractions(ctx context.Context, userID uuid.UUID, cityName string) ([]RecentInteraction, error) {
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

	var interactions []RecentInteraction
	for rows.Next() {
		var interaction RecentInteraction
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
			r.logger.Warn("Failed to scan interaction row", zap.Error(err))
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

// GetCityPOIsByInteraction gets all POIs for a city from user's interactions
func (r *RepositoryImpl) GetCityPOIsByInteraction(ctx context.Context, userID uuid.UUID, cityName string) ([]POIDetailedInfo, error) {
	ctx, span := otel.Tracer("RecentsRepository").Start(ctx, "GetCityPOIsByInteraction", trace.WithAttributes(
		attribute.String("user_id", userID.String()),
		attribute.String("city_name", cityName),
	))
	defer span.End()

	l := r.logger.With(zap.String("method", "GetCityPOIsByInteraction"))

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
		l.Error("Failed to query city POIs", zap.Error(err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Database query failed")
		return nil, fmt.Errorf("failed to query city POIs: %w", err)
	}
	defer rows.Close()

	var pois []POIDetailedInfo
	for rows.Next() {
		var poi POIDetailedInfo
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
			l.Warn("Failed to scan POI row", zap.Error(err))
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
		l.Error("Error iterating POI rows", zap.Error(err))
		span.RecordError(err)
		return nil, fmt.Errorf("error iterating POI rows: %w", err)
	}

	l.Info("Successfully retrieved city POIs",
		zap.String("city_name", cityName),
		zap.Int("poi_count", len(pois)))

	span.SetAttributes(attribute.Int("results.pois", len(pois)))
	span.SetStatus(codes.Ok, "City POIs retrieved")

	return pois, nil
}

// GetCityHotelsByInteraction gets all hotels for a city from user's interactions
func (r *RepositoryImpl) GetCityHotelsByInteraction(ctx context.Context, userID uuid.UUID, cityName string) ([]HotelDetailedInfo, error) {
	ctx, span := otel.Tracer("RecentsRepository").Start(ctx, "GetCityHotelsByInteraction", trace.WithAttributes(
		attribute.String("user_id", userID.String()),
		attribute.String("city_name", cityName),
	))
	defer span.End()

	l := r.logger.With(zap.String("method", "GetCityHotelsByInteraction"))

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
		l.Error("Failed to query city hotels", zap.Error(err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Database query failed")
		return nil, fmt.Errorf("failed to query city hotels: %w", err)
	}
	defer rows.Close()

	var hotels []HotelDetailedInfo
	for rows.Next() {
		var hotel HotelDetailedInfo
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
			l.Warn("Failed to scan hotel row", zap.Error(err))
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
		l.Error("Error iterating hotel rows", zap.Error(err))
		span.RecordError(err)
		return nil, fmt.Errorf("error iterating hotel rows: %w", err)
	}

	l.Info("Successfully retrieved city hotels",
		zap.String("city_name", cityName),
		zap.Int("hotel_count", len(hotels)))

	span.SetAttributes(attribute.Int("results.hotels", len(hotels)))
	span.SetStatus(codes.Ok, "City hotels retrieved")

	return hotels, nil
}

// GetCityRestaurantsByInteraction gets all restaurants for a city from user's interactions
func (r *RepositoryImpl) GetCityRestaurantsByInteraction(ctx context.Context, userID uuid.UUID, cityName string) ([]RestaurantDetailedInfo, error) {
	ctx, span := otel.Tracer("RecentsRepository").Start(ctx, "GetCityRestaurantsByInteraction", trace.WithAttributes(
		attribute.String("user_id", userID.String()),
		attribute.String("city_name", cityName),
	))
	defer span.End()

	l := r.logger.With(zap.String("method", "GetCityRestaurantsByInteraction"))

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
		l.Error("Failed to query city restaurants", zap.Error(err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Database query failed")
		return nil, fmt.Errorf("failed to query city restaurants: %w", err)
	}
	defer rows.Close()

	var restaurants []RestaurantDetailedInfo
	for rows.Next() {
		var restaurant RestaurantDetailedInfo
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
			l.Warn("Failed to scan restaurant row", zap.Error(err))
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
		l.Error("Error iterating restaurant rows", zap.Error(err))
		span.RecordError(err)
		return nil, fmt.Errorf("error iterating restaurant rows: %w", err)
	}

	l.Info("Successfully retrieved city restaurants",
		zap.String("city_name", cityName),
		zap.Int("restaurant_count", len(restaurants)))

	span.SetAttributes(attribute.Int("results.restaurants", len(restaurants)))
	span.SetStatus(codes.Ok, "City restaurants retrieved")

	return restaurants, nil
}

// GetCityItinerariesByInteraction gets all saved itineraries for a city from user's interactions
func (r *RepositoryImpl) GetCityItinerariesByInteraction(ctx context.Context, userID uuid.UUID, cityName string) ([]UserSavedItinerary, error) {
	ctx, span := otel.Tracer("RecentsRepository").Start(ctx, "GetCityItinerariesByInteraction", trace.WithAttributes(
		attribute.String("user_id", userID.String()),
		attribute.String("city_name", cityName),
	))
	defer span.End()

	l := r.logger.With(zap.String("method", "GetCityItinerariesByInteraction"))

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
		l.Error("Failed to query city itineraries", zap.Error(err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Database query failed")
		return nil, fmt.Errorf("failed to query city itineraries: %w", err)
	}
	defer rows.Close()

	var itineraries []UserSavedItinerary
	for rows.Next() {
		var itinerary UserSavedItinerary

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
			l.Warn("Failed to scan itinerary row", zap.Error(err))
			continue
		}

		itineraries = append(itineraries, itinerary)
	}

	if err := rows.Err(); err != nil {
		l.Error("Error iterating itinerary rows", zap.Error(err))
		span.RecordError(err)
		return nil, fmt.Errorf("error iterating itinerary rows: %w", err)
	}

	l.Info("Successfully retrieved city itineraries",
		zap.String("city_name", cityName),
		zap.Int("itinerary_count", len(itineraries)))

	span.SetAttributes(attribute.Int("results.itineraries", len(itineraries)))
	span.SetStatus(codes.Ok, "City itineraries retrieved")

	return itineraries, nil
}

// GetCityFavorites gets all favorite POIs for a city (both regular and LLM POIs)
func (r *RepositoryImpl) GetCityFavorites(ctx context.Context, userID uuid.UUID, cityName string) ([]POIDetailedInfo, error) {
	ctx, span := otel.Tracer("RecentsRepository").Start(ctx, "GetCityFavorites", trace.WithAttributes(
		attribute.String("user_id", userID.String()),
		attribute.String("city_name", cityName),
	))
	defer span.End()

	l := r.logger.With(zap.String("method", "GetCityFavorites"))

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
		l.Error("Failed to query city favorites", zap.Error(err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Database query failed")
		return nil, fmt.Errorf("failed to query city favorites: %w", err)
	}
	defer rows.Close()

	var pois []POIDetailedInfo
	for rows.Next() {
		var poi POIDetailedInfo
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
			l.Warn("Failed to scan favorite POI row", zap.Error(err))
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
		l.Error("Error iterating favorite POI rows", zap.Error(err))
		span.RecordError(err)
		return nil, fmt.Errorf("error iterating favorite POI rows: %w", err)
	}

	l.Info("Successfully retrieved city favorites",
		zap.String("city_name", cityName),
		zap.Int("favorite_count", len(pois)))

	span.SetAttributes(attribute.Int("results.favorites", len(pois)))
	span.SetStatus(codes.Ok, "City favorites retrieved")

	return pois, nil
}
