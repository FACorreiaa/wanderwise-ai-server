package poi

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"go.uber.org/zap"
	"strings"
	"time"

	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"

	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/google/uuid"

	"github.com/jackc/pgx/v5/pgxpool"
)

var _ Repository = (*RepositoryImpl)(nil)

type Repository interface {
	SavePoi(ctx context.Context, poi POIDetailedInfo, cityID uuid.UUID) (uuid.UUID, error)
	FindPoiByNameAndCity(ctx context.Context, name string, cityID uuid.UUID) (*POIDetailedInfo, error)
	GetPOIsByCityAndDistance(ctx context.Context, cityID uuid.UUID, userLocation UserLocation) ([]POIDetailedInfo, error)
	GetPOIsByLocationAndDistance(ctx context.Context, lat, lon, radiusMeters float64) ([]POIDetailedInfo, error)
	GetPOIsByLocationAndDistanceWithCategory(ctx context.Context, lat, lon, radiusMeters float64, category string) ([]POIDetailedInfo, error)
	AddPoiToFavourites(ctx context.Context, userID, poiID uuid.UUID) (uuid.UUID, error)
	AddLLMPoiToFavourite(ctx context.Context, userID uuid.UUID, llmPoiID uuid.UUID) (uuid.UUID, error)
	RemovePoiFromFavourites(ctx context.Context, userID, poiID uuid.UUID) error

	RemoveLLMPoiFromFavourite(ctx context.Context, userID, llmPoiID uuid.UUID) error
	CheckPoiExists(ctx context.Context, poiID uuid.UUID) (bool, error)
	FindLLMPOIByNameAndCity(ctx context.Context, name, city string) (uuid.UUID, error)
	FindLLMPOIByName(ctx context.Context, name string) (uuid.UUID, error)
	GetFavouritePOIsByUserID(ctx context.Context, userID uuid.UUID) ([]POIDetailedInfo, error)
	GetFavouritePOIsByUserIDPaginated(ctx context.Context, userID uuid.UUID, limit, offset int) ([]POIDetailedInfo, int, error)
	GetPOIsByCityID(ctx context.Context, cityID uuid.UUID) ([]POIDetailedInfo, error)

	// POI details
	FindPOIDetails(ctx context.Context, cityID uuid.UUID, lat, lon float64, tolerance float64) (*POIDetailedInfo, error)
	SavePOIDetails(ctx context.Context, poi POIDetailedInfo, cityID uuid.UUID) (uuid.UUID, error)
	SearchPOIs(ctx context.Context, filter POIFilter) ([]POIDetailedInfo, error)

	// Vector similarity search methods
	FindSimilarPOIs(ctx context.Context, queryEmbedding []float32, limit int) ([]POIDetailedInfo, error)
	FindSimilarPOIsByCity(ctx context.Context, queryEmbedding []float32, cityID uuid.UUID, limit int) ([]POIDetailedInfo, error)
	SearchPOIsHybrid(ctx context.Context, filter POIFilter, queryEmbedding []float32, semanticWeight float64) ([]POIDetailedInfo, error)
	UpdatePOIEmbedding(ctx context.Context, poiID uuid.UUID, embedding []float32) error
	GetPOIsWithoutEmbeddings(ctx context.Context, limit int) ([]POIDetailedInfo, error)

	// Hotels
	FindHotelDetails(ctx context.Context, cityID uuid.UUID, lat, lon, tolerance float64) ([]HotelDetailedInfo, error)
	SaveHotelDetails(ctx context.Context, hotel HotelDetailedInfo, cityID uuid.UUID) (uuid.UUID, error)
	GetHotelByID(ctx context.Context, hotelID uuid.UUID) (*HotelDetailedInfo, error)
	// Restaurants
	FindRestaurantDetails(ctx context.Context, cityID uuid.UUID, lat, lon, tolerance float64, preferences *RestaurantUserPreferences) ([]RestaurantDetailedInfo, error)
	SaveRestaurantDetails(ctx context.Context, restaurant RestaurantDetailedInfo, cityID uuid.UUID) (uuid.UUID, error)
	GetRestaurantByID(ctx context.Context, restaurantID uuid.UUID) (*RestaurantDetailedInfo, error)

	GetItinerary(ctx context.Context, userID, itineraryID uuid.UUID) (*UserSavedItinerary, error)
	GetItineraries(ctx context.Context, userID uuid.UUID, page, pageSize int) ([]UserSavedItinerary, int, error)
	UpdateItinerary(ctx context.Context, userID uuid.UUID, itineraryID uuid.UUID, updates UpdateItineraryRequest) (*UserSavedItinerary, error)
	SaveItinerary(ctx context.Context, userID, cityID uuid.UUID) (uuid.UUID, error)
	SaveItineraryPOIs(ctx context.Context, itineraryID uuid.UUID, pois []POIDetailedInfo) error
	SavePOItoPointsOfInterest(ctx context.Context, poi POIDetailedInfo, cityID uuid.UUID) (uuid.UUID, error)
	CityExists(ctx context.Context, cityID uuid.UUID) (bool, error)

	// Distance
	CalculateDistancePostGIS(ctx context.Context, userLat, userLon, poiLat, poiLon float64) (float64, error)
	SaveLlmPoisToDatabase(ctx context.Context, userID uuid.UUID, pois []POIDetailedInfo, genAIResponse *GenAIResponse, llmInteractionID uuid.UUID) error
	SaveLlmInteraction(ctx context.Context, interaction *LlmInteraction) (uuid.UUID, error)
}

type RepositoryImpl struct {
	logger *zap.Logger
	pgpool *pgxpool.Pool
}

func NewRepository(pgxpool *pgxpool.Pool, logger *zap.Logger) *RepositoryImpl {
	return &RepositoryImpl{
		logger: logger,
		pgpool: pgxpool,
	}
}

func (r *RepositoryImpl) SavePoi(ctx context.Context, poi POIDetailedInfo, cityID uuid.UUID) (uuid.UUID, error) {
	tx, err := r.pgpool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return uuid.Nil, fmt.Errorf("failed to start transaction: %w", err)
	}

	// Validate coordinates
	if poi.Latitude < -90 || poi.Latitude > 90 || poi.Longitude < -180 || poi.Longitude > 180 {
		return uuid.Nil, fmt.Errorf("invalid coordinates: lat=%f, lon=%f", poi.Latitude, poi.Longitude)
	}
	if poi.Name == "" {
		return uuid.Nil, fmt.Errorf("POI name is required")
	}

	query := `
        INSERT INTO points_of_interest (
            name, description, location, city_id, poi_type, source, ai_summary
        ) VALUES (
            $1, $2, ST_SetSRID(ST_MakePoint($3, $4), 4326), $5, $6, $7, $8
        ) RETURNING id
    `
	var id uuid.UUID
	if err = tx.QueryRow(ctx, query,
		poi.Name, poi.DescriptionPOI, poi.Longitude, poi.Latitude, cityID,
		poi.Category, "loci_ai", poi.DescriptionPOI,
	).Scan(&id); err != nil {
		if err == pgx.ErrNoRows {
			if rollbackErr := tx.Rollback(ctx); rollbackErr != nil {
				r.logger.Error("Failed to rollback transaction", zap.Any("error", rollbackErr))
			}
			return uuid.Nil, nil
		}
		if rollbackErr := tx.Rollback(ctx); rollbackErr != nil {
			r.logger.Error("Failed to rollback transaction", zap.Any("error", rollbackErr))
		}
		return uuid.Nil, fmt.Errorf("failed to insert POI: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return uuid.Nil, fmt.Errorf("failed to commit transaction: %w", err)
	}
	// Log the successful insertion
	r.logger.Info("POI saved successfully", zap.String("name", poi.Name), zap.String("id", id.String()))

	return id, nil
}

func (r *RepositoryImpl) FindPoiByNameAndCity(ctx context.Context, name string, cityID uuid.UUID) (*POIDetailedInfo, error) {
	query := `
        SELECT name, description, ST_Y(location) as lat, ST_X(location) as lon, poi_type
        FROM points_of_interest
        WHERE name = $1 AND city_id = $2
    `
	var poi POIDetailedInfo
	if err := r.pgpool.QueryRow(ctx, query, name, cityID).Scan(
		&poi.Name, &poi.DescriptionPOI, &poi.Latitude, &poi.Longitude, &poi.Category,
	); err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to find POI: %w", err)
	}
	// Log the successful retrieval
	r.logger.Info("POI found successfully",
		zap.String("name", poi.Name),
		zap.Float64("latitude", poi.Latitude),
		zap.Float64("longitude", poi.Longitude),
		zap.String("cityID", cityID.String()))

	return &poi, nil
}

func (r *RepositoryImpl) GetPOIsByCityAndDistance(ctx context.Context, cityID uuid.UUID, userLocation UserLocation) ([]POIDetailedInfo, error) {
	userPoint := fmt.Sprintf("SRID=4326;POINT(%f %f)", userLocation.UserLon, userLocation.UserLat)
	query := `
        SELECT
            id, name,
            ST_X(location::geometry) AS longitude,
            ST_Y(location::geometry) AS latitude,
            poi_type AS category,
            ai_summary AS description_poi,
            ST_Distance(location::geography, ST_GeomFromText($1, 4326)::geography) AS distance
        FROM points_of_interest
        WHERE city_id = $2 AND ST_DWithin(location::geography, ST_GeomFromText($1, 4326)::geography, $3 * 1000)
        ORDER BY distance ASC
    `
	rows, err := r.pgpool.Query(ctx, query, userPoint, cityID, userLocation.SearchRadiusKm)
	if err != nil {
		return nil, fmt.Errorf("failed to query POIs: %w", err)
	}
	defer rows.Close()

	var pois []POIDetailedInfo
	for rows.Next() {
		var poi POIDetailedInfo
		err := rows.Scan(&poi.ID, &poi.Name, &poi.Longitude,
			&poi.Latitude, &poi.Category, &poi.DescriptionPOI, &poi.Distance)
		if err != nil {
			return nil, fmt.Errorf("failed to scan POI row: %w", err)
		}
		pois = append(pois, poi)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating POI rows: %w", err)
	}

	return pois, nil
}

func (r *RepositoryImpl) CheckPoiExists(ctx context.Context, poiID uuid.UUID) (bool, error) {
	var exists bool
	query := `SELECT EXISTS(SELECT 1 FROM points_of_interest WHERE id = $1)`
	err := r.pgpool.QueryRow(ctx, query, poiID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to query points_of_interest: %w", err)
	}
	return exists, nil
}

func (r *RepositoryImpl) AddPoiToFavourites(ctx context.Context, userID, poiID uuid.UUID) (uuid.UUID, error) {
	query := `
        INSERT INTO user_favorite_pois (user_id, poi_id)
		VALUES ($1, $2)
		ON CONFLICT (user_id, poi_id) DO UPDATE SET user_id = EXCLUDED.user_id
		RETURNING id
    `
	var id uuid.UUID
	err := r.pgpool.QueryRow(ctx, query, userID, poiID).Scan(&id)
	if err != nil {
		return uuid.Nil, fmt.Errorf("failed to add POI to favourites: %w", err)
	}
	return id, nil
}

func (r *RepositoryImpl) AddLLMPoiToFavourite(ctx context.Context, userID uuid.UUID, llmPoiID uuid.UUID) (uuid.UUID, error) {
	query := `
        INSERT INTO user_favorite_llm_pois (user_id, llm_poi_id)
        VALUES ($1, $2)
        ON CONFLICT (user_id, llm_poi_id) DO UPDATE SET user_id = EXCLUDED.user_id
		RETURNING id
    `
	var id uuid.UUID
	err := r.pgpool.QueryRow(ctx, query, userID, llmPoiID).Scan(&id)
	if err != nil {
		return uuid.Nil, fmt.Errorf("failed to insert into user_favorite_llm_pois: %w", err)
	}
	return id, nil
}

func (r *RepositoryImpl) RemovePoiFromFavourites(ctx context.Context, userID, poiID uuid.UUID) error {
	query := `
		DELETE FROM user_favorite_pois
		WHERE user_id = $1 AND poi_id = $2
	`
	result, err := r.pgpool.Exec(ctx, query, userID, poiID)
	if err != nil {
		return fmt.Errorf("failed to remove POI from favourites: %w", err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("no favourite POI found to remove")
	}
	return nil
}

func (r *RepositoryImpl) RemoveLLMPoiFromFavourite(ctx context.Context, userID, llmPoiID uuid.UUID) error {
	// Try direct removal first
	query := `
		DELETE FROM user_favorite_llm_pois
		WHERE user_id = $1 AND llm_poi_id = $2
	`
	result, err := r.pgpool.Exec(ctx, query, userID, llmPoiID)
	if err != nil {
		return fmt.Errorf("failed to remove LLM POI from favourites: %w", err)
	}

	rowsAffected := result.RowsAffected()
	r.logger.Info("Delete query result", zap.Int64("rows_affected", rowsAffected))

	if rowsAffected == 0 {
		return fmt.Errorf("no favourite LLM POI found to remove")
	}
	return nil
}

func (r *RepositoryImpl) GetFavouritePOIsByUserID(ctx context.Context, userID uuid.UUID) ([]POIDetailedInfo, error) {
	query := `
		SELECT
    favorite_id,
    notes,
    added_at,
    id,
    name,
    longitude,
    latitude,
    category,
    description_poi,
    address,
    website,
    phone_number,
    opening_hours,
    rating,
    price_level,
    poi_source
FROM (
         -- Regular POI favorites
         SELECT
             ufp.id as favorite_id,
             ufp.notes,
             ufp.added_at,
             poi.id,
             poi.name,
             ST_X(poi.location) AS longitude,
             ST_Y(poi.location) AS latitude,
             poi.poi_type AS category,
             poi.description AS description_poi,
             poi.address,
             poi.website,
             poi.phone_number,
             poi.opening_hours,
             poi.average_rating as rating,
             poi.price_level::text as price_level,
             'regular' as poi_source
         FROM user_favorite_pois ufp
                  INNER JOIN points_of_interest poi ON ufp.poi_id = poi.id
         WHERE ufp.user_id = $1

         UNION ALL

         -- LLM POI favorites
         SELECT
             uflp.id as favorite_id,
             uflp.notes,
             uflp.added_at,
             llmsp.id,
             llmsp.name,
             llmsp.longitude,
             llmsp.latitude,
             llmsp.category,
             llmsp.description AS description_poi,
             llmsp.address,
             llmsp.website,
             llmsp.phone_number,
             llmsp.opening_hours,
             llmsp.rating,
             llmsp.price_level,
             'llm' as poi_source
         FROM user_favorite_llm_pois uflp
                  INNER JOIN llm_suggested_pois as llmsp ON uflp.llm_poi_id = llmsp.id
         WHERE uflp.user_id = $1
     ) combined_favorites
ORDER BY added_at DESC;
	`
	rows, err := r.pgpool.Query(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to query favourite POIs: %w", err)
	}
	defer rows.Close()
	var pois []POIDetailedInfo
	for rows.Next() {
		var poi POIDetailedInfo
		var favoriteID uuid.UUID
		var notes *string
		var addedAt time.Time
		var address, website, phoneNumber *string
		var openingHours *string
		var rating *float64
		var priceLevel *string
		var poiSource string

		err := rows.Scan(
			&favoriteID,         // favorite_id
			&notes,              // notes
			&addedAt,            // added_at
			&poi.ID,             // id
			&poi.Name,           // name
			&poi.Longitude,      // longitude
			&poi.Latitude,       // latitude
			&poi.Category,       // category
			&poi.DescriptionPOI, // description_poi
			&address,            // address
			&website,            // website
			&phoneNumber,        // phone_number
			&openingHours,       // opening_hours
			&rating,             // rating
			&priceLevel,         // price_level
			&poiSource,          // poi_source
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan favourite POI row: %w", err)
		}

		// Set optional fields
		if address != nil {
			poi.Address = *address
		}
		if website != nil {
			poi.Website = *website
		}
		if phoneNumber != nil {
			poi.PhoneNumber = *phoneNumber
		}
		//if openingHours != nil {
		//poi.OpeningHours = openingHours
		//}
		if rating != nil {
			poi.Rating = *rating
		}
		if priceLevel != nil {
			poi.PriceLevel = *priceLevel
		}

		pois = append(pois, poi)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating favourite POI rows: %w", err)
	}
	r.logger.Info("Favourite POIs retrieved successfully", zap.String("userID", userID.String()), zap.Int("count", len(pois)))
	return pois, nil
}

func (r *RepositoryImpl) GetFavouritePOIsByUserIDPaginated(ctx context.Context, userID uuid.UUID, limit, offset int) ([]POIDetailedInfo, int, error) {
	// First get the total count
	countQuery := `
		SELECT COUNT(*) FROM (
			SELECT 1 FROM user_favorite_pois ufp
			INNER JOIN points_of_interest poi ON ufp.poi_id = poi.id
			WHERE ufp.user_id = $1

			UNION ALL

			SELECT 1 FROM user_favorite_llm_pois uflp
			INNER JOIN llm_suggested_pois ON uflp.llm_poi_id = llm_suggested_pois.id
			WHERE uflp.user_id = $1
		) combined_count
	`

	var totalCount int
	err := r.pgpool.QueryRow(ctx, countQuery, userID).Scan(&totalCount)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count favourite POIs: %w", err)
	}

	// Then get the paginated results
	query := `
		SELECT
    favorite_id,
    notes,
    added_at,
    id,
    name,
    longitude,
    latitude,
    category,
    description_poi,
    address,
    website,
    phone_number,
    opening_hours,
    rating,
    price_level,
    poi_source
FROM (
         -- Regular POI favorites
         SELECT
             ufp.id as favorite_id,
             ufp.notes,
             ufp.added_at,
             poi.id,
             poi.name,
             ST_X(poi.location) AS longitude,
             ST_Y(poi.location) AS latitude,
             poi.poi_type AS category,
             poi.description AS description_poi,
             poi.address,
             poi.website,
             poi.phone_number,
             poi.opening_hours,
             poi.average_rating as rating,
             poi.price_level::text as price_level,
             'regular' as poi_source
         FROM user_favorite_pois ufp
                  INNER JOIN points_of_interest poi ON ufp.poi_id = poi.id
         WHERE ufp.user_id = $1

         UNION ALL

         -- LLM POI favorites
         SELECT
             uflp.id as favorite_id,
             uflp.notes,
             uflp.added_at,
             llmsp.id,
             llmsp.name,
             llmsp.longitude,
             llmsp.latitude,
             llmsp.category,
             llmsp.description AS description_poi,
             llmsp.address,
             llmsp.website,
             llmsp.phone_number,
             llmsp.opening_hours,
             llmsp.rating,
             llmsp.price_level,
             'llm' as poi_source
         FROM user_favorite_llm_pois uflp
                  INNER JOIN llm_suggested_pois llmsp ON uflp.llm_poi_id = llmsp.id
         WHERE uflp.user_id = $1
     ) combined_favorites
ORDER BY added_at DESC
LIMIT $2 OFFSET $3;
	`

	rows, err := r.pgpool.Query(ctx, query, userID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query favourite POIs: %w", err)
	}
	defer rows.Close()

	var pois []POIDetailedInfo
	for rows.Next() {
		var poi POIDetailedInfo
		var favoriteID uuid.UUID
		var notes *string
		var addedAt time.Time
		var descriptionPOI, address, website, phoneNumber *string
		var openingHours *string
		var rating *float64
		var priceLevel *string
		var poiSource string

		err := rows.Scan(
			&favoriteID,     // favorite_id
			&notes,          // notes
			&addedAt,        // added_at
			&poi.ID,         // id
			&poi.Name,       // name
			&poi.Longitude,  // longitude
			&poi.Latitude,   // latitude
			&poi.Category,   // category
			&descriptionPOI, // description_poi
			&address,        // address
			&website,        // website
			&phoneNumber,    // phone_number
			&openingHours,   // opening_hours
			&rating,         // rating
			&priceLevel,     // price_level
			&poiSource,      // poi_source
		)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to scan favourite POI row: %w", err)
		}

		// Set optional fields
		if descriptionPOI != nil {
			poi.DescriptionPOI = *descriptionPOI
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
		if rating != nil {
			poi.Rating = *rating
		}
		if priceLevel != nil {
			poi.PriceLevel = *priceLevel
		}

		pois = append(pois, poi)
	}
	if err = rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("error iterating favourite POI rows: %w", err)
	}

	r.logger.Info("Paginated favourite POIs retrieved successfully",
		zap.String("userID", userID.String()),
		zap.Int("count", len(pois)),
		zap.Int("total", totalCount),
		zap.Int("limit", limit),
		zap.Int("offset", offset))
	return pois, totalCount, nil
}

func (r *RepositoryImpl) GetPOIsByCityID(ctx context.Context, cityID uuid.UUID) ([]POIDetailedInfo, error) {
	query := `
		SELECT id, name, description, ST_X(location) AS longitude, ST_Y(location) AS latitude, poi_type
		FROM points_of_interest
		WHERE city_id = $1
	`
	rows, err := r.pgpool.Query(ctx, query, cityID)
	if err != nil {
		return nil, fmt.Errorf("failed to query POIs by city ID: %w", err)
	}
	defer rows.Close()

	var pois []POIDetailedInfo
	for rows.Next() {
		var poi POIDetailedInfo
		err := rows.Scan(&poi.ID, &poi.Name, &poi.DescriptionPOI, &poi.Longitude, &poi.Latitude, &poi.Category)
		if err != nil {
			return nil, fmt.Errorf("failed to scan POI row: %w", err)
		}
		pois = append(pois, poi)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating POI rows: %w", err)
	}

	r.logger.Info("POIs retrieved successfully by city ID", zap.String("cityID", cityID.String()), zap.Int("count", len(pois)))
	return pois, nil
}

func (r *RepositoryImpl) FindPOIDetails(ctx context.Context, cityID uuid.UUID, lat, lon float64, tolerance float64) (*POIDetailedInfo, error) {
	ctx, span := otel.Tracer("Repository").Start(ctx, "FindPOIDetailedInfos", trace.WithAttributes(
		attribute.String("city.id", cityID.String()),
		attribute.Float64("latitude", lat),
		attribute.Float64("longitude", lon),
	))
	defer span.End()

	query := `
        SELECT
            id, name, description, latitude, longitude, address, website, phone_number,
            opening_hours, price_range, category, tags, images, rating, llm_interaction_id
        FROM poi_details
        WHERE city_id = $1
        AND ST_DWithin(
            location::geography,
            ST_SetSRID(ST_MakePoint($2, $3)::geography, 4326),
            $4
        )
        LIMIT 1
    `
	row := r.pgpool.QueryRow(ctx, query, cityID, lon, lat, tolerance)

	var poi POIDetailedInfo
	var llmInteractionID uuid.NullUUID
	err := row.Scan(
		&poi.ID, &poi.Name, &poi.Description, &poi.Latitude, &poi.Longitude,
		&poi.Address, &poi.Website, &poi.PhoneNumber, &poi.OpeningHours,
		&poi.PriceRange, &poi.Category, &poi.Tags, &poi.Images, &poi.Rating,
		&llmInteractionID,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			span.SetStatus(codes.Ok, "No POI found")
			return nil, nil
		}
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to query POI details")
		return nil, fmt.Errorf("failed to query poi_details: %w", err)
	}

	if llmInteractionID.Valid {
		poi.LlmInteractionID = llmInteractionID.UUID
	}
	span.SetStatus(codes.Ok, "POI details found")
	return &poi, nil
}

func (r *RepositoryImpl) SavePOIDetails(ctx context.Context, poi POIDetailedInfo, cityID uuid.UUID) (uuid.UUID, error) {
	ctx, span := otel.Tracer("Repository").Start(ctx, "SavePOIDetailedInfos", trace.WithAttributes(
		attribute.String("city.id", func() string {

			return "null"
		}()),
		attribute.String("poi.name", poi.Name),
	))
	defer span.End()

	// Validate coordinates
	if poi.Latitude < -90 || poi.Latitude > 90 || poi.Longitude < -180 || poi.Longitude > 180 {
		err := fmt.Errorf("invalid coordinates: lat=%f, lon=%f", poi.Latitude, poi.Longitude)
		span.RecordError(err)
		span.SetStatus(codes.Error, "Invalid coordinates")
		return uuid.Nil, err
	}

	// Check for duplicate POI by name and location (within 100m radius)
	// Updated to work without city constraint for discover endpoint
	duplicateCheckQuery := `
		SELECT id FROM poi_details
		WHERE LOWER(name) = LOWER($1)
		AND ST_DWithin(
			location::geography,
			ST_SetSRID(ST_MakePoint($2, $3)::geography, 4326),
			100
		)
		LIMIT 1
	`
	var existingID uuid.UUID
	err := r.pgpool.QueryRow(ctx, duplicateCheckQuery, poi.Name, poi.Longitude, poi.Latitude).Scan(&existingID)
	if err == nil {
		// Duplicate found
		r.logger.Info("POI already exists, skipping save",
			zap.String("poi_name", poi.Name),
			zap.String("existing_id", existingID.String()),
			zap.String("city_id", func() string {

				return "null"
			}()))
		span.SetAttributes(attribute.String("poi.existing_id", existingID.String()))
		span.SetStatus(codes.Ok, "POI already exists")
		return existingID, nil
	} else if err != pgx.ErrNoRows {
		// Unexpected error
		r.logger.Warn("Error checking for duplicate POI",
			zap.Any("error", err),
			zap.String("poi_name", poi.Name))
	}

	// Start a transaction to ensure both tables are updated atomically
	tx, err := r.pgpool.Begin(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to begin transaction")
		return uuid.Nil, fmt.Errorf("failed to begin transaction: %w", err)
	}

	poiID := uuid.New()

	// Insert into poi_details table
	POIDetailedInfosQuery := `
        INSERT INTO poi_details (
            id, city_id, name, description, latitude, longitude, location,
            address, website, phone_number, opening_hours, price_range, category,
            tags, images, rating, llm_interaction_id
        ) VALUES (
            $1, $2, $3, $4, $5, $6, ST_SetSRID(ST_MakePoint($7, $8), 4326),
            $9, $10, $11, $12, $13, $14, $15, $16, $17, $18
        )
    `
	_, err = tx.Exec(ctx, POIDetailedInfosQuery,
		poiID, cityID, poi.Name, poi.Description, poi.Latitude, poi.Longitude,
		poi.Longitude, poi.Latitude, // lon, lat for ST_MakePoint
		poi.Address, poi.Website, poi.PhoneNumber, poi.OpeningHours,
		poi.PriceRange, poi.Category, poi.Tags, poi.Images, poi.Rating,
		uuid.NullUUID{UUID: poi.LlmInteractionID, Valid: poi.LlmInteractionID != uuid.Nil},
	)
	if err != nil {
		if rollbackErr := tx.Rollback(ctx); rollbackErr != nil {
			r.logger.Error("Failed to rollback transaction", zap.Any("error", rollbackErr))
		}
		r.logger.Error("Failed to save POI details",
			zap.Any("error", err),
			zap.String("poi_name", poi.Name),
			zap.String("poi_id", poiID.String()),
			zap.String("city_id", func() string {

				return "null"
			}()))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to save poi_details")
		return uuid.Nil, fmt.Errorf("failed to save poi_details: %w", err)
	}

	// Convert price_range to price_level for points_of_interest
	var priceLevel *int
	if poi.PriceRange != "" {
		switch poi.PriceRange {
		case "€", "$", "free", "Free", "1":
			level := 1
			priceLevel = &level
		case "€€", "$$", "budget", "Budget", "2":
			level := 2
			priceLevel = &level
		case "€€€", "$$$", "moderate", "Moderate", "3":
			level := 3
			priceLevel = &level
		case "€€€€", "$$$$", "expensive", "Expensive", "4":
			level := 4
			priceLevel = &level
		case "luxury", "Luxury", "premium", "Premium", "5":
			level := 5
			priceLevel = &level
		default:
			r.logger.Warn("Unknown price range",
				zap.String("price_range", poi.PriceRange),
				zap.String("poi_name", poi.Name))
			// Default to level 2 (budget) for unknown price ranges
			level := 2
			priceLevel = &level
		}
	}

	// Insert into points_of_interest table
	poisQuery := `
        INSERT INTO points_of_interest (
            id, name, description, location, city_id, address, poi_type,
            website, phone_number, opening_hours, category, price_level,
            average_rating, source, ai_summary, tags
        ) VALUES (
            $1, $2, $3, ST_SetSRID(ST_MakePoint($4, $5), 4326), $6, $7, $8,
            $9, $10, $11, $12, $13, $14, $15, $16, $17
        )
    `
	_, err = tx.Exec(ctx, poisQuery,
		poiID, poi.Name, poi.Description,
		poi.Longitude, poi.Latitude, // lon, lat for ST_MakePoint
		cityID, poi.Address, poi.Category,
		poi.Website, poi.PhoneNumber, poi.OpeningHours,
		poi.Category, priceLevel, poi.Rating,
		"loci_ai", poi.Description, poi.Tags,
	)
	if err != nil {
		if rollbackErr := tx.Rollback(ctx); rollbackErr != nil {
			r.logger.Error("Failed to rollback transaction", zap.Any("error", rollbackErr))
		}
		r.logger.Error("Failed to save POI to points_of_interest",
			zap.Any("error", err),
			zap.String("poi_name", poi.Name),
			zap.String("poi_id", poiID.String()),
			zap.String("city_id", func() string {

				return "null"
			}()))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to save POI to points_of_interest")
		return uuid.Nil, fmt.Errorf("failed to save points_of_interest: %w", err)
	}

	// Commit the transaction
	err = tx.Commit(ctx)
	if err != nil {
		r.logger.Error("Failed to commit POI transaction",
			zap.Any("error", err),
			zap.String("poi_name", poi.Name),
			zap.String("poi_id", poiID.String()))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to commit transaction")
		return uuid.Nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	r.logger.Info("Successfully saved POI to database",
		zap.String("poi_name", poi.Name),
		zap.String("poi_id", poiID.String()),
		zap.String("city_id", func() string {

			return "null"
		}()),
		zap.Float64("latitude", poi.Latitude),
		zap.Float64("longitude", poi.Longitude))

	span.SetAttributes(attribute.String("poi.id", poiID.String()))
	span.SetStatus(codes.Ok, "POI details saved successfully to both tables")
	return poiID, nil
}

func (r *RepositoryImpl) FindHotelDetails(ctx context.Context, cityID uuid.UUID, lat, lon, tolerance float64) ([]HotelDetailedInfo, error) {
	ctx, span := otel.Tracer("HotelRepository").Start(ctx, "FindHotelDetails", trace.WithAttributes(
		attribute.String("city.id", cityID.String()),
		attribute.Float64("latitude", lat),
		attribute.Float64("longitude", lon),
	))
	defer span.End()

	query := `
        SELECT
            id, name, description, latitude, longitude, address, website, phone_number,
            opening_hours, price_range, category, tags, images, rating, llm_interaction_id
        FROM hotel_details
        WHERE city_id = $1
        AND ST_DWithin(
            location::geography,
            ST_SetSRID(ST_MakePoint($2, $3)::geography, 4326),
            $4
        )
    `
	rows, err := r.pgpool.Query(ctx, query, cityID, lon, lat, tolerance)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to query hotel details")
		return nil, fmt.Errorf("failed to query hotel_details: %w", err)
	}
	defer rows.Close()

	var hotels []HotelDetailedInfo
	for rows.Next() {
		var hotel HotelDetailedInfo
		var llmInteractionID uuid.NullUUID
		var website, phoneNumber, openingHours, priceRange *string
		err := rows.Scan(
			&hotel.ID, &hotel.Name, &hotel.Description, &hotel.Latitude, &hotel.Longitude,
			&hotel.Address, &website, &phoneNumber, &openingHours, &priceRange,
			&hotel.Category, &hotel.Tags, &hotel.Images, &hotel.Rating, &llmInteractionID,
		)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, "Failed to scan hotel details")
			return nil, fmt.Errorf("failed to scan hotel_details: %w", err)
		}
		hotel.Website = website
		hotel.PhoneNumber = phoneNumber
		hotel.OpeningHours = openingHours
		hotel.PriceRange = priceRange
		if llmInteractionID.Valid {
			hotel.LlmInteractionID = llmInteractionID.UUID
		}
		hotels = append(hotels, hotel)
	}
	if rows.Err() != nil {
		span.RecordError(rows.Err())
		span.SetStatus(codes.Error, "Failed to iterate hotel details")
		return nil, fmt.Errorf("failed to iterate hotel_details: %w", rows.Err())
	}

	span.SetStatus(codes.Ok, "Hotel details found")
	return hotels, nil
}

func (r *RepositoryImpl) SaveHotelDetails(ctx context.Context, hotel HotelDetailedInfo, cityID uuid.UUID) (uuid.UUID, error) {
	ctx, span := otel.Tracer("HotelRepository").Start(ctx, "SaveHotelDetails", trace.WithAttributes(
		attribute.String("city.id", cityID.String()),
		attribute.String("hotel.name", hotel.Name),
	))
	defer span.End()

	var openingHours *string
	if hotel.OpeningHours != nil && *hotel.OpeningHours != "" {
		// Verify it's valid JSON
		if json.Valid([]byte(*hotel.OpeningHours)) {
			openingHours = hotel.OpeningHours
		} else {
			// Log warning and set to nil if invalid
			r.logger.Warn("Invalid JSON for opening_hours, setting to NULL", zap.String("value", *hotel.OpeningHours))
			openingHours = nil
		}
	}

	query := `
        INSERT INTO hotel_details (
            id, city_id, name, description, latitude, longitude, location,
            address, website, phone_number, opening_hours, price_range, category,
            tags, images, rating, llm_interaction_id
        ) VALUES (
            $1, $2, $3, $4, $5, $6, ST_SetSRID(ST_MakePoint($7, $8), 4326),
            $9, $10, $11, $12, $13, $14, $15, $16, $17, $18
        )
        RETURNING id
    `
	var id uuid.UUID
	err := r.pgpool.QueryRow(ctx, query,
		uuid.New(), cityID, hotel.Name, hotel.Description, hotel.Latitude, hotel.Longitude,
		hotel.Longitude, hotel.Latitude, // lon, lat for ST_MakePoint
		hotel.Address, hotel.Website, hotel.PhoneNumber, openingHours,
		hotel.PriceRange, hotel.Category, hotel.Tags, hotel.Images, hotel.Rating,
		uuid.NullUUID{UUID: hotel.LlmInteractionID, Valid: hotel.LlmInteractionID != uuid.Nil},
	).Scan(&id)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to save hotel details")
		return uuid.Nil, fmt.Errorf("failed to save hotel_details: %w", err)
	}

	span.SetAttributes(attribute.String("hotel.id", id.String()))
	span.SetStatus(codes.Ok, "Hotel details saved successfully")
	return id, nil
}

func (r *RepositoryImpl) GetHotelByID(ctx context.Context, hotelID uuid.UUID) (*HotelDetailedInfo, error) {
	ctx, span := otel.Tracer("HotelRepository").Start(ctx, "GetHotelByID", trace.WithAttributes(
		attribute.String("hotel.id", hotelID.String()),
	))
	defer span.End()

	query := `
		SELECT
			id, name, description, latitude, longitude, address, website, phone_number,
			opening_hours, price_range, category, tags, images, rating, llm_interaction_id
		FROM hotel_details
		WHERE id = $1
	`
	row := r.pgpool.QueryRow(ctx, query, hotelID)

	var hotel HotelDetailedInfo
	var llmInteractionID uuid.NullUUID
	err := row.Scan(
		&hotel.ID, &hotel.Name, &hotel.Description, &hotel.Latitude, &hotel.Longitude,
		&hotel.Address, &hotel.Website, &hotel.PhoneNumber, &hotel.OpeningHours,
		&hotel.PriceRange, &hotel.Category, &hotel.Tags, &hotel.Images, &hotel.Rating,
		&llmInteractionID,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			span.SetStatus(codes.Ok, "No hotel found")
			return nil, nil
		}
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to query hotel details by ID")
		return nil, fmt.Errorf("failed to query hotel_details by ID: %w", err)
	}

	if llmInteractionID.Valid {
		hotel.LlmInteractionID = llmInteractionID.UUID
	}
	span.SetStatus(codes.Ok, "Hotel details found by ID")
	return &hotel, nil
}

func (r *RepositoryImpl) FindRestaurantDetails(ctx context.Context, cityID uuid.UUID, lat, lon, tolerance float64, preferences *RestaurantUserPreferences) ([]RestaurantDetailedInfo, error) {
	ctx, span := otel.Tracer("RestaurantRepository").Start(ctx, "FindRestaurantDetails")
	defer span.End()

	query := `
        SELECT
            id, name, description, latitude, longitude, address, website, phone_number,
            opening_hours, price_level, category, tags, images, rating, cuisine_type, llm_interaction_id
        FROM restaurant_details
        WHERE city_id = $1
        AND ST_DWithin(
            location::geography,
            ST_SetSRID(ST_MakePoint($2, $3)::geography, 4326),
            $4
        )
    `
	args := []interface{}{cityID, lon, lat, tolerance}
	if preferences != nil {
		if preferences.PreferredCuisine != "" {
			query += ` AND cuisine_type = $5`
			args = append(args, preferences.PreferredCuisine)
		}
		if preferences.PreferredPriceRange != "" {
			query += fmt.Sprintf(` AND price_level = $%d`, len(args)+1)
			args = append(args, preferences.PreferredPriceRange)
		}
	}

	rows, err := r.pgpool.Query(ctx, query, args...)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to query restaurants")
		return nil, fmt.Errorf("failed to query restaurant_details: %w", err)
	}
	defer rows.Close()

	var restaurants []RestaurantDetailedInfo
	for rows.Next() {
		var r RestaurantDetailedInfo
		var llmID uuid.NullUUID
		err := rows.Scan(&r.ID, &r.Name, &r.Description, &r.Latitude, &r.Longitude, &r.Address,
			&r.Website, &r.PhoneNumber, &r.OpeningHours, &r.PriceLevel, &r.Category,
			&r.Tags, &r.Images, &r.Rating, &r.CuisineType, &llmID)
		if err != nil {
			span.RecordError(err)
			return nil, fmt.Errorf("failed to scan restaurant: %w", err)
		}
		if llmID.Valid {
			r.LlmInteractionID = llmID.UUID
		}
		restaurants = append(restaurants, r)
	}
	span.SetStatus(codes.Ok, "Restaurants found")
	return restaurants, nil
}

func (r *RepositoryImpl) SaveRestaurantDetails(ctx context.Context, restaurant RestaurantDetailedInfo, cityID uuid.UUID) (uuid.UUID, error) {
	ctx, span := otel.Tracer("RestaurantRepository").Start(ctx, "SaveRestaurantDetails", trace.WithAttributes(
		attribute.String("restaurant.name", restaurant.Name),
		attribute.String("city.id", cityID.String()),
	))
	defer span.End()

	// Normalize opening_hours
	var openingHoursJSON sql.NullString // Use sql.NullString for JSONB to handle NULL correctly
	if restaurant.OpeningHours != nil && len(restaurant.OpeningHours) > 0 {
		if hoursJSON, err := json.Marshal(restaurant.OpeningHours); err == nil {
			openingHoursJSON.String = string(hoursJSON)
			openingHoursJSON.Valid = true
		} else {
			r.logger.Warn("Failed to marshal opening_hours, setting to NULL",
				zap.Error(err),
				zap.String("restaurant_name", restaurant.Name))
			// openingHoursJSON remains invalid, which inserts NULL
		}
	}

	// Normalize price_level and cuisine_type (using sql.NullString is safer for text fields that can be null)
	var priceLevel sql.NullString
	if restaurant.PriceLevel != nil && *restaurant.PriceLevel != "" {
		priceLevel.String = *restaurant.PriceLevel
		priceLevel.Valid = true
	}

	var cuisineType sql.NullString
	if restaurant.CuisineType != nil && *restaurant.CuisineType != "" {
		cuisineType.String = *restaurant.CuisineType
		cuisineType.Valid = true
	}

	// Handle nullable text fields from restaurant struct
	var address sql.NullString
	if restaurant.Address != nil {
		address.String = *restaurant.Address
		address.Valid = true
	}
	var website sql.NullString
	if restaurant.Website != nil {
		website.String = *restaurant.Website
		website.Valid = true
	}
	var phoneNumber sql.NullString
	if restaurant.PhoneNumber != nil {
		phoneNumber.String = *restaurant.PhoneNumber
		phoneNumber.Valid = true
	}
	var category sql.NullString
	if restaurant.Category != "" { // Assuming Category is not a pointer in the struct
		category.String = restaurant.Category
		category.Valid = true
	}

	query := `
        INSERT INTO restaurant_details (
            id, city_id, name, description, latitude, longitude, location,
            address, website, phone_number, opening_hours, price_level, category,
            cuisine_type, tags, images, rating, llm_interaction_id
        ) VALUES (
            $1, $2, $3, $4, $5, $6, ST_SetSRID(ST_MakePoint($7, $8), 4326),
            $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19 -- Added $19
        ) RETURNING id
    `
	var id uuid.UUID
	err := r.pgpool.QueryRow(ctx, query,
		restaurant.ID,
		cityID,                      // $2: city_id
		restaurant.Name,             // $3: name
		restaurant.Description,      // $4: description
		restaurant.Latitude,         // $5: latitude
		restaurant.Longitude,        // $6: longitude
		restaurant.Longitude,        // $7: location (longitude for ST_MakePoint)
		restaurant.Latitude,         // $8: location (latitude for ST_MakePoint)
		address,                     // $9: address (sql.NullString)
		website,                     // $10: website (sql.NullString)
		phoneNumber,                 // $11: phone_number (sql.NullString)
		openingHoursJSON,            // $12: opening_hours (sql.NullString representing JSON)
		priceLevel,                  // $13: price_level (sql.NullString)
		category,                    // $14: category (sql.NullString)
		cuisineType,                 // $15: cuisine_type (sql.NullString)
		restaurant.Tags,             // $16: tags (TEXT[])
		restaurant.Images,           // $17: images (TEXT[])
		restaurant.Rating,           // $18: rating (DOUBLE PRECISION)
		restaurant.LlmInteractionID, // $19: llm_interaction_id (UUID)
	).Scan(&id)

	if err != nil {
		r.logger.Error("Failed to save restaurant details",
			zap.Any("error", err),
			zap.String("restaurant_name", restaurant.Name),
			zap.String("city_id", cityID.String()))
		span.RecordError(err)
		span.SetStatus(codes.Error, "DB INSERT failed")
		return uuid.Nil, fmt.Errorf("failed to save restaurant_details: %w", err)
	}

	// If the `id` scanned back is different from `restaurant.ID` (which it will be if you used uuid.New() in the query's $1)
	// and you need the database-generated ID, then `id` is what you want.
	// If you want to ensure the ID from the service layer (which was already in restaurant.ID) is used and is the PK,
	// then you should pass restaurant.ID as $1. My correction above assumes you pass restaurant.ID as $1.

	span.SetAttributes(attribute.String("db.restaurant.id", id.String())) // Log the ID returned by the DB
	span.SetStatus(codes.Ok, "Restaurant saved")
	return id, nil
}

func (r *RepositoryImpl) GetRestaurantByID(ctx context.Context, restaurantID uuid.UUID) (*RestaurantDetailedInfo, error) {
	ctx, span := otel.Tracer("RestaurantRepository").Start(ctx, "GetRestaurantByID")
	defer span.End()

	query := `
        SELECT
            id, name, description, latitude, longitude, address, website, phone_number,
            opening_hours, price_level, category, tags, images, rating, cuisine_type, llm_interaction_id
        FROM restaurant_details
        WHERE id = $1
    `
	var restaurant RestaurantDetailedInfo
	var llmID uuid.NullUUID
	err := r.pgpool.QueryRow(ctx, query, restaurantID).Scan(&restaurant.ID, &restaurant.Name,
		&restaurant.Description, &restaurant.Latitude,
		&restaurant.Longitude, &restaurant.Address,
		&restaurant.Website, &restaurant.PhoneNumber,
		&restaurant.OpeningHours, &restaurant.PriceLevel,
		&restaurant.Category, &restaurant.Tags,
		&restaurant.Images, &restaurant.Rating,
		&restaurant.CuisineType, &llmID)
	if err != nil {
		if err == pgx.ErrNoRows {
			span.SetStatus(codes.Ok, "Restaurant not found")
			return nil, nil
		}
		span.RecordError(err)
		return nil, fmt.Errorf("failed to get restaurant: %w", err)
	}
	if llmID.Valid {
		restaurant.LlmInteractionID = llmID.UUID
	}
	span.SetStatus(codes.Ok, "Restaurant found")
	return &restaurant, nil
}

func (r *RepositoryImpl) SearchPOIs(ctx context.Context, filter POIFilter) ([]POIDetailedInfo, error) {
	ctx, span := otel.Tracer("Repository").Start(ctx, "SearchPOIs", trace.WithAttributes(
		attribute.Float64("location.latitude", filter.Location.Latitude),
		attribute.Float64("location.longitude", filter.Location.Longitude),
		attribute.Float64("radius", filter.Radius),
		attribute.String("category", filter.Category),
	))
	defer span.End()

	l := r.logger.With(zap.String("method", "SearchPOIs"))

	// Base query using PostGIS for geospatial filtering
	query := `
        SELECT
            id,
            name,
            description,
            ST_X(location::geometry) AS longitude,
            ST_Y(location::geometry) AS latitude,
            category,
            ST_Distance(
                location,
                ST_SetSRID(ST_MakePoint($1, $2), 4326)::geography
            ) AS distance_meters
        FROM points_of_interest
        WHERE ST_DWithin(
            location,
            ST_SetSRID(ST_MakePoint($1, $2), 4326)::geography,
            $3
        )
    `
	args := []interface{}{
		filter.Location.Longitude, // $1
		filter.Location.Latitude,  // $2
		filter.Radius * 1000,      // $3 (convert km to meters for ST_DWithin)
	}

	// Add category filter if provided
	if filter.Category != "" {
		query += ` AND category = $4`
		args = append(args, filter.Category) // $4
	}

	// Order by distance
	query += ` ORDER BY distance_meters ASC`

	l.Debug("Executing POI search query", zap.String("query", query), zap.Any("args", args))

	// Execute query
	rows, err := r.pgpool.Query(ctx, query, args...)
	if err != nil {
		l.Error("Failed to query POIs", zap.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Database query failed")
		return nil, fmt.Errorf("failed to search points_of_interest: %w", err)
	}
	defer rows.Close()

	// Collect results
	var pois []POIDetailedInfo
	for rows.Next() {
		var poi POIDetailedInfo
		var distanceMeters float64
		var description sql.NullString // Handle NULL description

		err := rows.Scan(
			&poi.ID,
			&poi.Name,
			&description,
			&poi.Longitude,
			&poi.Latitude,
			&poi.Category,
			&distanceMeters,
		)
		if err != nil {
			l.Error("Failed to scan POI row", zap.Any("error", err))
			span.RecordError(err)
			return nil, fmt.Errorf("failed to scan POI row: %w", err)
		}

		// Set description if valid
		if description.Valid {
			poi.DescriptionPOI = description.String
		}

		// Convert distance from meters to kilometers
		poi.Distance = distanceMeters / 1000

		pois = append(pois, poi)
	}

	// Check for errors during row iteration
	if err = rows.Err(); err != nil {
		l.Error("Error iterating POI rows", zap.Any("error", err))
		span.RecordError(err)
		return nil, fmt.Errorf("error iterating POI rows: %w", err)
	}

	// Log and set span status
	if len(pois) == 0 {
		l.Info("No POIs found")
		span.SetStatus(codes.Ok, "No POIs found")
	} else {
		l.Info("POIs found", zap.Int("count", len(pois)))
		span.SetStatus(codes.Ok, "POIs found")
	}

	return pois, nil
}

func (r *RepositoryImpl) GetItinerary(ctx context.Context, userID, itineraryID uuid.UUID) (*UserSavedItinerary, error) {
	ctx, span := otel.Tracer("LlmInteractionRepo").Start(ctx, "GetItinerary", trace.WithAttributes(
		semconv.DBSystemPostgreSQL,
		attribute.String("db.operation", "SELECT"),
		attribute.String("db.sql.table", "user_saved_itineraries"),
		attribute.String("user.id", userID.String()),
		attribute.String("itinerary.id", itineraryID.String()),
	))
	defer span.End()

	query := `
		SELECT
			id, user_id, source_llm_interaction_id, session_id, primary_city_id, title, description,
			markdown_content, tags, estimated_duration_days, estimated_cost_level, is_public
		FROM user_saved_itineraries
		WHERE id = $1 AND user_id = $2
	`
	row := r.pgpool.QueryRow(ctx, query, itineraryID, userID)

	var itinerary UserSavedItinerary
	if err := row.Scan(
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
	); err != nil {
		if err == pgx.ErrNoRows {
			err = fmt.Errorf("no itinerary found with ID %s for user %s", itineraryID, userID)
			span.RecordError(err)
			return nil, err
		}
		span.RecordError(err)
		return nil, fmt.Errorf("failed to scan user_saved_itineraries row: %w", err)
	}

	return &itinerary, nil
}

func (r *RepositoryImpl) GetItineraries(ctx context.Context, userID uuid.UUID, page, pageSize int) ([]UserSavedItinerary, int, error) {
	ctx, span := otel.Tracer("LlmInteractionRepo").Start(ctx, "GetItineraries", trace.WithAttributes(
		semconv.DBSystemPostgreSQL,
		attribute.String("db.operation", "SELECT"),
		attribute.String("db.sql.table", "user_saved_itineraries"),
		attribute.String("user.id", userID.String()),
		attribute.Int("page", page),
		attribute.Int("page_size", pageSize),
	))
	defer span.End()

	offset := (page - 1) * pageSize
	query := `
		SELECT
			id, user_id, source_llm_interaction_id, session_id, primary_city_id, title, description,
			markdown_content, tags, estimated_duration_days, estimated_cost_level, is_public
		FROM user_saved_itineraries
		WHERE user_id = $1
		LIMIT $2 OFFSET $3
	`
	rows, err := r.pgpool.Query(ctx, query, userID, pageSize, offset)
	if err != nil {
		span.RecordError(err)
		return nil, 0, fmt.Errorf("failed to query user_saved_itineraries: %w", err)
	}
	defer rows.Close()

	var itineraries []UserSavedItinerary
	for rows.Next() {
		var itinerary UserSavedItinerary
		if err := rows.Scan(
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
		); err != nil {
			if err == pgx.ErrNoRows {
				continue // No more rows to scan
			}
			return nil, 0, fmt.Errorf("failed to scan user_saved_itineraries row: %w", err)
		}
		itineraries = append(itineraries, itinerary)
	}

	if err = rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("error iterating user_saved_itineraries rows: %w", err)
	}

	countQuery := `
		SELECT COUNT(*) FROM user_saved_itineraries WHERE user_id = $1
	`
	var totalRecords int
	if err := r.pgpool.QueryRow(ctx, countQuery, userID).Scan(&totalRecords); err != nil {
		span.RecordError(err)
		return nil, 0, fmt.Errorf("failed to count user_saved_itineraries: %w", err)
	}
	span.SetAttributes(
		attribute.Int("total_records", totalRecords),
		attribute.Int("itineraries.count", len(itineraries)),
	)
	span.SetStatus(codes.Ok, "Itineraries retrieved successfully")
	return itineraries, totalRecords, nil
}

func (r *RepositoryImpl) UpdateItinerary(ctx context.Context, userID uuid.UUID, itineraryID uuid.UUID, updates UpdateItineraryRequest) (*UserSavedItinerary, error) {
	ctx, span := otel.Tracer("LlmInteractionRepo").Start(ctx, "UpdateItinerary", trace.WithAttributes(
		semconv.DBSystemPostgreSQL,
		attribute.String("db.operation", "UPDATE"),
		attribute.String("db.sql.table", "user_saved_itineraries"),
		attribute.String("user.id", userID.String()),
		attribute.String("itinerary.id", itineraryID.String()),
	))
	defer span.End()

	setClauses := []string{}
	args := []interface{}{}
	argCount := 1 // Start arg counter for $1, $2, ...

	if updates.Title != nil {
		setClauses = append(setClauses, fmt.Sprintf("title = $%d", argCount))
		args = append(args, *updates.Title)
		argCount++
		span.SetAttributes(attribute.Bool("update.title", true))
	}
	if updates.Description != nil {
		setClauses = append(setClauses, fmt.Sprintf("description = $%d", argCount))
		if *updates.Description == "" {
			args = append(args, sql.NullString{Valid: false})
		} else {
			args = append(args, sql.NullString{String: *updates.Description, Valid: true})
		}
		argCount++
		span.SetAttributes(attribute.Bool("update.description", true))
	}
	if updates.Tags != nil {
		setClauses = append(setClauses, fmt.Sprintf("tags = $%d", argCount))
		args = append(args, updates.Tags)
		argCount++
		span.SetAttributes(attribute.Bool("update.tags", true))
	}
	if updates.EstimatedDurationDays != nil {
		setClauses = append(setClauses, fmt.Sprintf("estimated_duration_days = $%d", argCount))
		args = append(args, sql.NullInt32{Int32: *updates.EstimatedDurationDays, Valid: true})
		argCount++
		span.SetAttributes(attribute.Bool("update.duration", true))
	}
	if updates.EstimatedCostLevel != nil {
		setClauses = append(setClauses, fmt.Sprintf("estimated_cost_level = $%d", argCount))
		args = append(args, sql.NullInt32{Int32: *updates.EstimatedCostLevel, Valid: true})
		argCount++
		span.SetAttributes(attribute.Bool("update.cost", true))
	}
	if updates.IsPublic != nil {
		setClauses = append(setClauses, fmt.Sprintf("is_public = $%d", argCount))
		args = append(args, *updates.IsPublic)
		argCount++
		span.SetAttributes(attribute.Bool("update.is_public", true))
	}
	if updates.MarkdownContent != nil {
		setClauses = append(setClauses, fmt.Sprintf("markdown_content = $%d", argCount))
		args = append(args, *updates.MarkdownContent)
		argCount++
		span.SetAttributes(attribute.Bool("update.markdown", true))
	}

	if len(setClauses) == 0 {
		span.AddEvent("No fields provided for update.")
		return nil, fmt.Errorf("no fields to update for itinerary %s", itineraryID)
	}

	// Always update the updated_at timestamp
	setClauses = append(setClauses, fmt.Sprintf("updated_at = $%d", argCount))
	args = append(args, time.Now())
	argCount++

	// Store the current argCount for the WHERE clause
	whereIDPlaceholder := argCount
	args = append(args, itineraryID)
	argCount++
	userIDPlaceholder := argCount
	args = append(args, userID)

	query := fmt.Sprintf(`
        UPDATE user_saved_itineraries
        SET %s
        WHERE id = $%d AND user_id = $%d
        RETURNING id, user_id, source_llm_interaction_id, primary_city_id, title, description,
                  markdown_content, tags, estimated_duration_days, estimated_cost_level, is_public,
                  created_at, updated_at
    `, strings.Join(setClauses, ", "), whereIDPlaceholder, userIDPlaceholder)

	r.logger.Debug("Executing UpdateItinerary query", zap.String("query", query), zap.Any("args_count", len(args)))

	var updatedItinerary UserSavedItinerary
	err := r.pgpool.QueryRow(ctx, query, args...).Scan(
		&updatedItinerary.ID,
		&updatedItinerary.UserID,
		&updatedItinerary.SourceLlmInteractionID,
		&updatedItinerary.PrimaryCityID,
		&updatedItinerary.Title,
		&updatedItinerary.Description,
		&updatedItinerary.MarkdownContent,
		&updatedItinerary.Tags,
		&updatedItinerary.EstimatedDurationDays,
		&updatedItinerary.EstimatedCostLevel,
		&updatedItinerary.IsPublic,
		&updatedItinerary.CreatedAt,
		&updatedItinerary.UpdatedAt,
	)

	if err != nil {
		if err == pgx.ErrNoRows {
			notFoundErr := fmt.Errorf("itinerary with ID %s not found for user %s or does not exist", itineraryID, userID)
			span.RecordError(notFoundErr)
			span.SetStatus(codes.Error, "Itinerary not found or not owned by user")
			return nil, notFoundErr
		}
		span.RecordError(err)
		span.SetStatus(codes.Error, "DB UPDATE failed")
		r.logger.Error("Failed to update itinerary", zap.Any("error", err))
		return nil, fmt.Errorf("failed to update user_saved_itineraries: %w", err)
	}

	span.SetStatus(codes.Ok, "Itinerary updated successfully")
	return &updatedItinerary, nil
}

func (r *RepositoryImpl) SaveItinerary(ctx context.Context, userID, cityID uuid.UUID) (uuid.UUID, error) {
	ctx, span := otel.Tracer("LlmInteractionRepo").Start(ctx, "SaveItinerary")
	defer span.End()

	query := `
        INSERT INTO itineraries (user_id, city_id, created_at, updated_at)
        VALUES ($1, $2, NOW(), NOW())
        RETURNING id
    `
	var itineraryID uuid.UUID
	err := r.pgpool.QueryRow(ctx, query, userID, cityID).Scan(&itineraryID)
	if err != nil {
		span.RecordError(err)
		return uuid.Nil, fmt.Errorf("failed to save itinerary: %w", err)
	}
	span.SetAttributes(attribute.String("itinerary.id", itineraryID.String()))
	return itineraryID, nil
}

func (r *RepositoryImpl) SavePOItoPointsOfInterest(ctx context.Context, poi POIDetailedInfo, cityID uuid.UUID) (uuid.UUID, error) {
	ctx, span := otel.Tracer("LlmInteractionRepo").Start(ctx, "SavePOItoPointsOfInterest")
	defer span.End()

	// Check if POI exists
	queryCheck := `
        SELECT id FROM points_of_interest
        WHERE name = $1 AND city_id = $2
    `
	var poiID uuid.UUID
	err := r.pgpool.QueryRow(ctx, queryCheck, poi.Name, cityID).Scan(&poiID)
	if err == nil {
		return poiID, nil // POI already exists
	}
	if err != pgx.ErrNoRows {
		span.RecordError(err)
		return uuid.Nil, fmt.Errorf("failed to check POI existence: %w", err)
	}

	// Insert new POI
	queryInsert := `
        INSERT INTO points_of_interest (id, city_id, name, latitude, longitude, category)
        VALUES ($1, $2, $3, $4, $5, $6)
        RETURNING id
    `
	poiID = uuid.New()
	err = r.pgpool.QueryRow(ctx, queryInsert, poiID, cityID, poi.Name, poi.Latitude, poi.Longitude, poi.Category).Scan(&poiID)
	if err != nil {
		span.RecordError(err)
		return uuid.Nil, fmt.Errorf("failed to save POI to points_of_interest: %w", err)
	}
	span.SetAttributes(attribute.String("poi.id", poiID.String()))
	return poiID, nil
}

type ItineraryPOISource struct {
	pois        []POIDetailedInfo
	itineraryID uuid.UUID
	idx         int
}

func (ips *ItineraryPOISource) Next() bool {
	ips.idx++
	return ips.idx < len(ips.pois)
}

func (ips *ItineraryPOISource) Values() ([]interface{}, error) {
	poi := ips.pois[ips.idx]
	return []interface{}{ips.itineraryID, poi.ID, ips.idx, poi.DescriptionPOI}, nil
}

func (ips *ItineraryPOISource) Err() error {
	return nil
}

func (r *RepositoryImpl) SaveItineraryPOIs(ctx context.Context, itineraryID uuid.UUID, pois []POIDetailedInfo) error {
	ctx, span := otel.Tracer("LlmInteractionRepo").Start(ctx, "SaveItineraryPOIs")
	defer span.End()

	for i := range pois {
		poiID, err := r.SavePOItoPointsOfInterest(ctx, pois[i], pois[i].CityID) // Assume CityID is added to POIDetailedInfo or passed separately
		if err != nil {
			span.RecordError(err)
			return fmt.Errorf("failed to ensure POI in points_of_interest: %w", err)
		}
		pois[i].ID = poiID
	}

	source := &ItineraryPOISource{
		pois:        pois,
		itineraryID: itineraryID,
		idx:         -1,
	}

	_, err := r.pgpool.CopyFrom(
		ctx,
		pgx.Identifier{"itinerary_pois"},
		[]string{"itinerary_id", "poi_id", "order_index", "ai_description"},
		source,
	)

	if err != nil {
		span.RecordError(err)
		return fmt.Errorf("failed to save itinerary POIs: %w", err)
	}

	span.SetAttributes(attribute.Int("pois.count", len(pois)))
	return nil
}

func (r *RepositoryImpl) CityExists(ctx context.Context, cityID uuid.UUID) (bool, error) {
	query := `SELECT EXISTS(SELECT 1 FROM cities WHERE id = $1)`
	var exists bool
	err := r.pgpool.QueryRow(ctx, query, cityID).Scan(&exists) // Assuming r.db is your database connection
	if err != nil {
		return false, fmt.Errorf("failed to check city existence: %w", err)
	}
	return exists, nil
}

// FindSimilarPOIs finds POIs similar to the provided query embedding using cosine similarity
func (r *RepositoryImpl) FindSimilarPOIs(ctx context.Context, queryEmbedding []float32, limit int) ([]POIDetailedInfo, error) {
	ctx, span := otel.Tracer("Repository").Start(ctx, "FindSimilarPOIs", trace.WithAttributes(
		attribute.Int("embedding.dimension", len(queryEmbedding)),
		attribute.Int("limit", limit),
	))
	defer span.End()

	l := r.logger.With(zap.String("method", "FindSimilarPOIs"))

	// Convert []float32 to pgvector format string
	embeddingStr := fmt.Sprintf("[%v]", strings.Join(func() []string {
		strs := make([]string, len(queryEmbedding))
		for i, v := range queryEmbedding {
			strs[i] = fmt.Sprintf("%f", v)
		}
		return strs
	}(), ","))

	query := `
        SELECT
            id,
            name,
            description,
            ST_X(location::geometry) AS longitude,
            ST_Y(location::geometry) AS latitude,
            poi_type AS category,
            1 - (embedding <=> $1::vector) AS similarity_score
        FROM points_of_interest
        WHERE embedding IS NOT NULL
        ORDER BY embedding <=> $1::vector
        LIMIT $2
    `

	l.Debug("Executing similarity search query",
		zap.String("query", query),
		zap.Int("embedding_dim", len(queryEmbedding)),
		zap.Int("limit", limit))

	rows, err := r.pgpool.Query(ctx, query, embeddingStr, limit)
	if err != nil {
		l.Error("Failed to query similar POIs", zap.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Database query failed")
		return nil, fmt.Errorf("failed to search similar POIs: %w", err)
	}
	defer rows.Close()

	var pois []POIDetailedInfo
	for rows.Next() {
		var poi POIDetailedInfo
		var similarityScore float64
		var description sql.NullString

		err := rows.Scan(
			&poi.ID,
			&poi.Name,
			&description,
			&poi.Longitude,
			&poi.Latitude,
			&poi.Category,
			&similarityScore,
		)
		if err != nil {
			l.Error("Failed to scan similar POI row", zap.Any("error", err))
			span.RecordError(err)
			return nil, fmt.Errorf("failed to scan similar POI row: %w", err)
		}

		if description.Valid {
			poi.DescriptionPOI = description.String
		}

		// Store similarity score in distance field for now (could add dedicated field)
		poi.Distance = similarityScore

		pois = append(pois, poi)
	}

	if err = rows.Err(); err != nil {
		l.Error("Error iterating similar POI rows", zap.Any("error", err))
		span.RecordError(err)
		return nil, fmt.Errorf("error iterating similar POI rows: %w", err)
	}

	l.Info("Similar POIs found", zap.Int("count", len(pois)))
	span.SetAttributes(attribute.Int("results.count", len(pois)))
	span.SetStatus(codes.Ok, "Similar POIs found")

	return pois, nil
}

// FindSimilarPOIsByCity finds POIs similar to the provided query embedding within a specific city
func (r *RepositoryImpl) FindSimilarPOIsByCity(ctx context.Context, queryEmbedding []float32, cityID uuid.UUID, limit int) ([]POIDetailedInfo, error) {
	ctx, span := otel.Tracer("Repository").Start(ctx, "FindSimilarPOIsByCity", trace.WithAttributes(
		attribute.String("city.id", cityID.String()),
		attribute.Int("embedding.dimension", len(queryEmbedding)),
		attribute.Int("limit", limit),
	))
	defer span.End()

	l := r.logger.With(zap.String("method", "FindSimilarPOIsByCity"))

	// Convert []float32 to pgvector format string
	embeddingStr := fmt.Sprintf("[%v]", strings.Join(func() []string {
		strs := make([]string, len(queryEmbedding))
		for i, v := range queryEmbedding {
			strs[i] = fmt.Sprintf("%f", v)
		}
		return strs
	}(), ","))

	query := `
        SELECT
            id,
            name,
            description,
            ST_X(location::geometry) AS longitude,
            ST_Y(location::geometry) AS latitude,
            poi_type AS category,
            1 - (embedding <=> $1::vector) AS similarity_score
        FROM points_of_interest
        WHERE embedding IS NOT NULL AND city_id = $2
        ORDER BY embedding <=> $1::vector
        LIMIT $3
    `

	l.Debug("Executing city-specific similarity search",
		zap.String("city_id", cityID.String()),
		zap.Int("embedding_dim", len(queryEmbedding)),
		zap.Int("limit", limit))

	rows, err := r.pgpool.Query(ctx, query, embeddingStr, cityID, limit)
	if err != nil {
		l.Error("Failed to query similar POIs by city", zap.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Database query failed")
		return nil, fmt.Errorf("failed to search similar POIs by city: %w", err)
	}
	defer rows.Close()

	var pois []POIDetailedInfo
	for rows.Next() {
		var poi POIDetailedInfo
		var similarityScore float64
		var description sql.NullString

		err := rows.Scan(
			&poi.ID,
			&poi.Name,
			&description,
			&poi.Longitude,
			&poi.Latitude,
			&poi.Category,
			&similarityScore,
		)
		if err != nil {
			l.Error("Failed to scan similar POI row", zap.Any("error", err))
			span.RecordError(err)
			return nil, fmt.Errorf("failed to scan similar POI row: %w", err)
		}

		if description.Valid {
			poi.DescriptionPOI = description.String
		}

		poi.Distance = similarityScore
		poi.CityID = cityID

		pois = append(pois, poi)
	}

	if err = rows.Err(); err != nil {
		l.Error("Error iterating similar POI rows", zap.Any("error", err))
		span.RecordError(err)
		return nil, fmt.Errorf("error iterating similar POI rows: %w", err)
	}

	l.Info("Similar POIs by city found",
		zap.String("city_id", cityID.String()),
		zap.Int("count", len(pois)))
	span.SetAttributes(
		attribute.String("city.id", cityID.String()),
		attribute.Int("results.count", len(pois)),
	)
	span.SetStatus(codes.Ok, "Similar POIs by city found")

	return pois, nil
}

// SearchPOIsHybrid combines spatial filtering with semantic similarity search
func (r *RepositoryImpl) SearchPOIsHybrid(ctx context.Context, filter POIFilter, queryEmbedding []float32, semanticWeight float64) ([]POIDetailedInfo, error) {
	ctx, span := otel.Tracer("Repository").Start(ctx, "SearchPOIsHybrid", trace.WithAttributes(
		attribute.Float64("location.latitude", filter.Location.Latitude),
		attribute.Float64("location.longitude", filter.Location.Longitude),
		attribute.Float64("radius", filter.Radius),
		attribute.String("category", filter.Category),
		attribute.Float64("semantic.weight", semanticWeight),
		attribute.Int("embedding.dimension", len(queryEmbedding)),
	))
	defer span.End()

	l := r.logger.With(zap.String("method", "SearchPOIsHybrid"))

	// Convert []float32 to pgvector format string
	embeddingStr := fmt.Sprintf("[%v]", strings.Join(func() []string {
		strs := make([]string, len(queryEmbedding))
		for i, v := range queryEmbedding {
			strs[i] = fmt.Sprintf("%f", v)
		}
		return strs
	}(), ","))

	// Hybrid search combining spatial distance and semantic similarity
	query := `
        SELECT
            id,
            name,
            description,
            ST_X(location::geometry) AS longitude,
            ST_Y(location::geometry) AS latitude,
            poi_type AS category,
            ST_Distance(
                location,
                ST_SetSRID(ST_MakePoint($1, $2), 4326)::geography
            ) AS distance_meters,
            CASE
                WHEN embedding IS NOT NULL THEN 1 - (embedding <=> $6::vector)
                ELSE 0
            END AS similarity_score,
            -- Hybrid score: weighted combination of spatial proximity and semantic similarity
            CASE
                WHEN embedding IS NOT NULL THEN
                    (1 - $5) * (1 / (1 + ST_Distance(location, ST_SetSRID(ST_MakePoint($1, $2), 4326)::geography) / 1000)) +
                    $5 * (1 - (embedding <=> $6::vector))
                ELSE
                    (1 - $5) * (1 / (1 + ST_Distance(location, ST_SetSRID(ST_MakePoint($1, $2), 4326)::geography) / 1000))
            END AS hybrid_score
        FROM points_of_interest
        WHERE ST_DWithin(
            location,
            ST_SetSRID(ST_MakePoint($1, $2), 4326)::geography,
            $3
        )
    `

	args := []interface{}{
		filter.Location.Longitude, // $1
		filter.Location.Latitude,  // $2
		filter.Radius * 1000,      // $3 (convert km to meters)
	}

	// Add category filter if provided
	argIndex := 4
	if filter.Category != "" {
		query += fmt.Sprintf(` AND poi_type = $%d`, argIndex)
		args = append(args, filter.Category)
		_ = argIndex + 1 // argIndex incremented but not used after this point
	}

	// Add semantic weight and embedding (adjust indexes based on whether category was added)
	args = append(args, semanticWeight) // semantic weight
	args = append(args, embeddingStr)   // embedding

	// Order by hybrid score (descending)
	query += ` ORDER BY hybrid_score DESC`

	l.Debug("Executing hybrid search query",
		zap.String("query", query),
		zap.Any("args_count", len(args)),
		zap.Float64("semantic_weight", semanticWeight))

	rows, err := r.pgpool.Query(ctx, query, args...)
	if err != nil {
		l.Error("Failed to execute hybrid search", zap.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Database query failed")
		return nil, fmt.Errorf("failed to execute hybrid POI search: %w", err)
	}
	defer rows.Close()

	var pois []POIDetailedInfo
	for rows.Next() {
		var poi POIDetailedInfo
		var distanceMeters, similarityScore, hybridScore float64
		var description sql.NullString

		err := rows.Scan(
			&poi.ID,
			&poi.Name,
			&description,
			&poi.Longitude,
			&poi.Latitude,
			&poi.Category,
			&distanceMeters,
			&similarityScore,
			&hybridScore,
		)
		if err != nil {
			l.Error("Failed to scan hybrid search POI row", zap.Any("error", err))
			span.RecordError(err)
			return nil, fmt.Errorf("failed to scan hybrid search POI row: %w", err)
		}

		if description.Valid {
			poi.DescriptionPOI = description.String
		}

		// Store the actual distance in meters converted to km
		poi.Distance = distanceMeters / 1000

		pois = append(pois, poi)
	}

	if err = rows.Err(); err != nil {
		l.Error("Error iterating hybrid search POI rows", zap.Any("error", err))
		span.RecordError(err)
		return nil, fmt.Errorf("error iterating hybrid search POI rows: %w", err)
	}

	l.Info("Hybrid search POIs found",
		zap.Int("count", len(pois)),
		zap.Float64("semantic_weight", semanticWeight))
	span.SetAttributes(
		attribute.Int("results.count", len(pois)),
		attribute.Float64("semantic.weight", semanticWeight),
	)
	span.SetStatus(codes.Ok, "Hybrid search completed")

	return pois, nil
}

// UpdatePOIEmbedding updates the embedding vector for a specific POI
func (r *RepositoryImpl) UpdatePOIEmbedding(ctx context.Context, poiID uuid.UUID, embedding []float32) error {
	ctx, span := otel.Tracer("Repository").Start(ctx, "UpdatePOIEmbedding", trace.WithAttributes(
		attribute.String("poi.id", poiID.String()),
		attribute.Int("embedding.dimension", len(embedding)),
	))
	defer span.End()

	l := r.logger.With(zap.String("method", "UpdatePOIEmbedding"))

	// Convert []float32 to pgvector format string
	embeddingStr := fmt.Sprintf("[%v]", strings.Join(func() []string {
		strs := make([]string, len(embedding))
		for i, v := range embedding {
			strs[i] = fmt.Sprintf("%f", v)
		}
		return strs
	}(), ","))

	query := `
        UPDATE points_of_interest
        SET embedding = $1::vector, embedding_generated_at = NOW()
        WHERE id = $2
    `

	result, err := r.pgpool.Exec(ctx, query, embeddingStr, poiID)
	if err != nil {
		l.Error("Failed to update POI embedding",
			zap.Any("error", err),
			zap.String("poi_id", poiID.String()))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Database update failed")
		return fmt.Errorf("failed to update POI embedding: %w", err)
	}

	if result.RowsAffected() == 0 {
		err := fmt.Errorf("no POI found with ID %s", poiID.String())
		l.Warn("No POI found for embedding update", zap.String("poi_id", poiID.String()))
		span.RecordError(err)
		span.SetStatus(codes.Error, "POI not found")
		return err
	}

	l.Info("POI embedding updated successfully",
		zap.String("poi_id", poiID.String()),
		zap.Int("embedding_dimension", len(embedding)))
	span.SetAttributes(
		attribute.String("poi.id", poiID.String()),
		attribute.Int("embedding.dimension", len(embedding)),
	)
	span.SetStatus(codes.Ok, "POI embedding updated")

	return nil
}

// GetPOIsWithoutEmbeddings retrieves POIs that don't have embeddings generated yet
func (r *RepositoryImpl) GetPOIsWithoutEmbeddings(ctx context.Context, limit int) ([]POIDetailedInfo, error) {
	ctx, span := otel.Tracer("Repository").Start(ctx, "GetPOIsWithoutEmbeddings", trace.WithAttributes(
		attribute.Int("limit", limit),
	))
	defer span.End()

	l := r.logger.With(zap.String("method", "GetPOIsWithoutEmbeddings"))

	query := `
        SELECT
            id,
            name,
            description,
            ST_X(location::geometry) AS longitude,
            ST_Y(location::geometry) AS latitude,
            poi_type AS category,
            city_id
        FROM points_of_interest
        WHERE embedding IS NULL
        ORDER BY created_at ASC
        LIMIT $1
    `

	rows, err := r.pgpool.Query(ctx, query, limit)
	if err != nil {
		l.Error("Failed to query POIs without embeddings", zap.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Database query failed")
		return nil, fmt.Errorf("failed to query POIs without embeddings: %w", err)
	}
	defer rows.Close()

	var pois []POIDetailedInfo
	for rows.Next() {
		var poi POIDetailedInfo
		var description sql.NullString

		err := rows.Scan(
			&poi.ID,
			&poi.Name,
			&description,
			&poi.Longitude,
			&poi.Latitude,
			&poi.Category,
			&poi.CityID,
		)
		if err != nil {
			l.Error("Failed to scan POI without embedding row", zap.Any("error", err))
			span.RecordError(err)
			return nil, fmt.Errorf("failed to scan POI without embedding row: %w", err)
		}

		if description.Valid {
			poi.DescriptionPOI = description.String
		}

		pois = append(pois, poi)
	}

	if err = rows.Err(); err != nil {
		l.Error("Error iterating POI without embedding rows", zap.Any("error", err))
		span.RecordError(err)
		return nil, fmt.Errorf("error iterating POI without embedding rows: %w", err)
	}

	l.Info("POIs without embeddings found", zap.Int("count", len(pois)))
	span.SetAttributes(attribute.Int("results.count", len(pois)))
	span.SetStatus(codes.Ok, "POIs without embeddings retrieved")

	return pois, nil
}

// GetPOIsByLocationAndDistance retrieves POIs within a specified radius from a given location using PostGIS
func (r *RepositoryImpl) GetPOIsByLocationAndDistance(ctx context.Context, lat, lon, radiusMeters float64) ([]POIDetailedInfo, error) {
	ctx, span := otel.Tracer("POIRepository").Start(ctx, "GetPOIsByLocationAndDistance", trace.WithAttributes(
		attribute.Float64("location.lat", lat),
		attribute.Float64("location.lon", lon),
		attribute.Float64("radius.meters", radiusMeters),
	))
	defer span.End()

	l := r.logger.With(zap.String("method", "GetPOIsByLocationAndDistance"))

	// Build the query with optional category filter
	baseQuery := `
					SELECT
						id,
						name,
						description,
						longitude,
						latitude,
						category,
						address,
						website,
						phone_number,
						opening_hours,
						poi_type,
						price_level,
						rating,
						ROUND(CAST(distance_meters / 1000.0 AS numeric), 2) as distance,
						city_id,
						COALESCE(tags, '{}') as tags,
						COALESCE(rating_count, 0) as rating_count,
						COALESCE(is_sponsored, false) as is_sponsored
					FROM (
						SELECT
							id,
							name,
							COALESCE(description, '') as description,
							ST_X(location) as longitude,
							ST_Y(location) as latitude,
							COALESCE(category, '') as category,
							COALESCE(address, '') as address,
							COALESCE(website, '') as website,
							COALESCE(phone_number, '') as phone_number,
							opening_hours,
							COALESCE(poi_type, '') as poi_type,
							price_level,
							COALESCE(average_rating, 0) as rating,
							ST_Distance(location::geography, ST_SetSRID(ST_MakePoint($1, $2), 4326)::geography) as distance_meters,
							city_id,
							tags,
							rating_count,
							is_sponsored
						FROM points_of_interest
						WHERE ST_DWithin(
							location::geography,
							ST_SetSRID(ST_MakePoint($1, $2), 4326)::geography,
							$3
						)
					) sub
					ORDER BY distance ASC LIMIT 50
				`

	var args []interface{}
	args = append(args, lon, lat, radiusMeters) // $1, $2, $3

	l.Debug("Executing POI distance query",
		zap.String("query", baseQuery),
		zap.Float64("lat", lat),
		zap.Float64("lon", lon),
		zap.Float64("radius_meters", radiusMeters))

	rows, err := r.pgpool.Query(ctx, baseQuery, args...)
	if err != nil {
		l.Error("Failed to query POIs by location and distance", zap.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Database query failed")
		return nil, fmt.Errorf("failed to query POIs by location and distance: %w", err)
	}
	defer rows.Close()

	var pois []POIDetailedInfo
	for rows.Next() {
		var poi POIDetailedInfo
		var description, address, website, phoneNumber, poiType sql.NullString
		var openingHours sql.NullString // JSONB can be scanned as string
		var priceLevel sql.NullInt32
		var rating sql.NullFloat64
		var cityID sql.NullString
		var tagsRaw []byte // Postgres array of text
		var ratingCount sql.NullInt32
		var isSponsored sql.NullBool

		err := rows.Scan(
			&poi.ID,
			&poi.Name,
			&description,
			&poi.Longitude,
			&poi.Latitude,
			&poi.Category,
			&address,
			&website,
			&phoneNumber,
			&openingHours,
			&poiType,
			&priceLevel,
			&rating,
			&poi.Distance, // Already calculated in km
			&cityID,
			&tagsRaw,
			&ratingCount,
			&isSponsored,
		)
		if err != nil {
			l.Error("Failed to scan POI row", zap.Any("error", err))
			span.RecordError(err)
			return nil, fmt.Errorf("failed to scan POI row: %w", err)
		}

		// Set optional fields
		if description.Valid {
			poi.Description = description.String
		}
		if address.Valid {
			poi.Address = address.String
		}
		if website.Valid {
			poi.Website = website.String
		}
		if phoneNumber.Valid {
			poi.PhoneNumber = phoneNumber.String
		}
		if rating.Valid {
			poi.Rating = rating.Float64
		}
		if priceLevel.Valid {
			// Convert price level to string format
			switch priceLevel.Int32 {
			case 1:
				poi.PriceLevel = "€"
			case 2:
				poi.PriceLevel = "€€"
			case 3:
				poi.PriceLevel = "€€€"
			case 4:
				poi.PriceLevel = "€€€€"
			default:
				poi.PriceLevel = "Free"
			}
		} else {
			poi.PriceLevel = "Free"
		}

		// Process tags array from PostgreSQL
		if tagsRaw != nil {
			// Parse PostgreSQL array format: {tag1,tag2,tag3}
			tagsStr := string(tagsRaw)
			if tagsStr != "{}" && len(tagsStr) > 2 {
				// Remove braces and split by commas
				tagsStr = strings.Trim(tagsStr, "{}")
				if tagsStr != "" {
					poi.Tags = strings.Split(tagsStr, ",")
					// Clean up quotes and spaces
					for i, tag := range poi.Tags {
						poi.Tags[i] = strings.Trim(strings.Trim(tag, `"`), " ")
					}
				}
			}
		}

		// Calculate popularity from rating count and sponsored status
		popularityScore := 0
		if ratingCount.Valid {
			popularityScore = int(ratingCount.Int32)
		}
		if isSponsored.Valid && isSponsored.Bool {
			popularityScore += 50 // Boost sponsored items
		}
		// Map popularity score to 1-10 scale for display
		if popularityScore > 100 {
			poi.Priority = 10
		} else if popularityScore > 0 {
			poi.Priority = (popularityScore / 10) + 1
		} else {
			poi.Priority = 1
		}

		pois = append(pois, poi)
	}

	if err = rows.Err(); err != nil {
		l.Error("Error iterating POI rows", zap.Any("error", err))
		span.RecordError(err)
		return nil, fmt.Errorf("error iterating POI rows: %w", err)
	}

	l.Info("POIs by location and distance found",
		zap.Int("count", len(pois)),
		zap.Float64("radius_km", radiusMeters/1000))
	span.SetAttributes(attribute.Int("results.count", len(pois)))
	span.SetStatus(codes.Ok, "POIs by location and distance retrieved")

	return pois, nil
}

// GetPOIsByLocationAndDistanceWithCategory retrieves POIs within a specified radius from a given location filtered by category
func (r *RepositoryImpl) GetPOIsByLocationAndDistanceWithCategory(ctx context.Context, lat, lon, radiusMeters float64, category string) ([]POIDetailedInfo, error) {
	ctx, span := otel.Tracer("POIRepository").Start(ctx, "GetPOIsByLocationAndDistanceWithCategory", trace.WithAttributes(
		attribute.Float64("location.lat", lat),
		attribute.Float64("location.lon", lon),
		attribute.Float64("radius.meters", radiusMeters),
		attribute.String("category", category),
	))
	defer span.End()

	l := r.logger.With(zap.String("method", "GetPOIsByLocationAndDistanceWithCategory"))

	// Build the query with category filter
	baseQuery := `
					SELECT
						id,
						name,
						description,
						longitude,
						latitude,
						category,
						address,
						website,
						phone_number,
						opening_hours,
						poi_type,
						price_level,
						rating,
						ROUND(CAST(distance_meters / 1000.0 AS numeric), 2) as distance,
						city_id,
						COALESCE(tags, '{}') as tags,
						COALESCE(rating_count, 0) as rating_count,
						COALESCE(is_sponsored, false) as is_sponsored
					FROM (
						SELECT
							id,
							name,
							COALESCE(description, '') as description,
							ST_X(location) as longitude,
							ST_Y(location) as latitude,
							COALESCE(category, '') as category,
							COALESCE(address, '') as address,
							COALESCE(website, '') as website,
							COALESCE(phone_number, '') as phone_number,
							opening_hours,
							COALESCE(poi_type, '') as poi_type,
							price_level,
							COALESCE(average_rating, 0) as rating,
							ST_Distance(location::geography, ST_SetSRID(ST_MakePoint($1, $2), 4326)::geography) as distance_meters,
							city_id,
							tags,
							rating_count,
							is_sponsored
						FROM points_of_interest
						WHERE ST_DWithin(
							location::geography,
							ST_SetSRID(ST_MakePoint($1, $2), 4326)::geography,
							$3
						)
						AND LOWER(category) = LOWER($4)
					) sub
					ORDER BY distance ASC LIMIT 50
				`

	var args []interface{}
	args = append(args, lon, lat, radiusMeters, category) // $1, $2, $3, $4

	l.Debug("Executing POI distance query with category filter",
		zap.String("query", baseQuery),
		zap.Float64("lat", lat),
		zap.Float64("lon", lon),
		zap.Float64("radius_meters", radiusMeters),
		zap.String("category", category))

	rows, err := r.pgpool.Query(ctx, baseQuery, args...)
	if err != nil {
		l.Error("Failed to query POIs by location, distance and category", zap.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Database query failed")
		return nil, fmt.Errorf("failed to query POIs by location, distance and category: %w", err)
	}
	defer rows.Close()

	var pois []POIDetailedInfo
	for rows.Next() {
		var poi POIDetailedInfo
		var description, address, website, phoneNumber, poiType sql.NullString
		var openingHours sql.NullString // JSONB can be scanned as string
		var priceLevel sql.NullInt32
		var rating sql.NullFloat64
		var cityID sql.NullString
		var tagsRaw []byte // Postgres array of text
		var ratingCount sql.NullInt32
		var isSponsored sql.NullBool

		err := rows.Scan(
			&poi.ID,
			&poi.Name,
			&description,
			&poi.Longitude,
			&poi.Latitude,
			&poi.Category,
			&address,
			&website,
			&phoneNumber,
			&openingHours,
			&poiType,
			&priceLevel,
			&rating,
			&poi.Distance,
			&cityID,
			&tagsRaw,
			&ratingCount,
			&isSponsored,
		)
		if err != nil {
			l.Error("Failed to scan POI row", zap.Any("error", err))
			span.RecordError(err)
			span.SetStatus(codes.Error, "Row scan failed")
			return nil, fmt.Errorf("failed to scan POI row: %w", err)
		}

		// Handle nullable fields
		if description.Valid {
			poi.DescriptionPOI = description.String
		}
		if address.Valid {
			poi.Address = address.String
		}
		if website.Valid {
			poi.Website = website.String
		}
		if phoneNumber.Valid {
			poi.PhoneNumber = phoneNumber.String
		}
		if openingHours.Valid {
			var hoursSlice []string
			if err := json.Unmarshal([]byte(openingHours.String), &hoursSlice); err == nil {
				poi.OpeningHours = hoursSlice
			} else {
				// Fallback: treat as single string
				poi.OpeningHours = []string{openingHours.String}
			}
		}
		if poiType.Valid {
			poi.Category = poiType.String
		}
		if priceLevel.Valid {
			poi.PriceLevel = fmt.Sprintf("%d", priceLevel.Int32)
		}
		if rating.Valid {
			poi.Rating = rating.Float64
		}
		if cityID.Valid {
			poi.City = cityID.String
		}

		// Parse tags from JSON array
		if len(tagsRaw) > 0 {
			var tags []string
			err := json.Unmarshal(tagsRaw, &tags)
			if err != nil {
				l.Warn("Failed to parse tags", zap.Any("error", err))
				poi.Tags = []string{}
			} else {
				poi.Tags = tags
			}
		} else {
			poi.Tags = []string{}
		}

		pois = append(pois, poi)
	}

	if err = rows.Err(); err != nil {
		l.Error("Row iteration error", zap.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Row iteration failed")
		return nil, fmt.Errorf("row iteration error: %w", err)
	}

	l.Info("POIs by location, distance and category found",
		zap.Int("count", len(pois)),
		zap.Float64("radius_km", radiusMeters/1000),
		zap.String("category", category))
	span.SetAttributes(attribute.Int("results.count", len(pois)))
	span.SetStatus(codes.Ok, "POIs by location, distance and category retrieved")

	return pois, nil
}

func (r *RepositoryImpl) SaveLlmInteraction(ctx context.Context, interaction *LlmInteraction) (uuid.UUID, error) {
	ctx, span := otel.Tracer("POIRepository").Start(ctx, "SaveLlmInteraction")
	defer span.End()

	l := r.logger.With(zap.String("method", "SaveLlmInteraction"))

	query := `
		INSERT INTO llm_interactions (user_id, model_name, prompt, response, latitude, longitude, distance)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id
	`

	var id uuid.UUID
	err := r.pgpool.QueryRow(ctx, query, interaction.UserID, interaction.ModelName, interaction.Prompt, interaction.Response, interaction.Latitude, interaction.Longitude, interaction.Distance).Scan(&id)
	if err != nil {
		l.Error("Failed to save LLM interaction", zap.Any("error", err))
		span.RecordError(err)
		return uuid.Nil, fmt.Errorf("failed to save LLM interaction: %w", err)
	}

	l.Info("Successfully saved LLM interaction", zap.String("id", id.String()))
	span.SetStatus(codes.Ok, "LLM interaction saved successfully")
	return id, nil
}

func (r *RepositoryImpl) SaveLlmPoisToDatabase(ctx context.Context, userID uuid.UUID, pois []POIDetailedInfo, _ *GenAIResponse, llmInteractionID uuid.UUID) error {
	ctx, span := otel.Tracer("POIRepository").Start(ctx, "SaveLlmPoisToDatabase", trace.WithAttributes(
		attribute.Int("poi.count", len(pois)),
	))
	defer span.End()

	l := r.logger.With(zap.String("method", "SaveLlmPoisToDatabase"))

	if len(pois) == 0 {
		l.Info("No LLM POIs to save.")
		return nil
	}

	tx, err := r.pgpool.Begin(ctx)
	if err != nil {
		l.Error("Failed to begin transaction for saving LLM POIs", zap.Any("error", err))
		span.RecordError(err)
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		if rollbackErr := tx.Rollback(ctx); rollbackErr != nil {
			l.Error("Failed to rollback transaction", zap.Any("error", rollbackErr))
		}
	}() // Rollback on error

	stmt, err := tx.Prepare(ctx, "insert_llm_poi", `
        INSERT INTO llm_suggested_pois (id, user_id, llm_interaction_id, name, latitude, longitude, category, description_poi, distance, location)
        VALUES ($1, $2, $3, $4::TEXT, $5, $6, $7, $8, $9, ST_SetSRID(ST_MakePoint($6, $5), 4326))
        ON CONFLICT (name, latitude, longitude) DO NOTHING
    `)
	if err != nil {
		l.Error("Failed to prepare statement for LLM POI insertion", zap.Any("error", err))
		span.RecordError(err)
		return fmt.Errorf("failed to prepare statement: %w", err)
	}

	for _, poi := range pois {
		// Validate POI data
		if poi.Name == "" {
			l.Warn("POI has empty or nil name, skipping", zap.String("poi_name", poi.Name))
			continue
		}
		if poi.Latitude == 0 || poi.Longitude == 0 {
			l.Warn("POI has invalid coordinates, skipping", zap.String("poi_name", poi.Name))
			continue
		}

		// Log parameter values for debugging
		l.Debug("Inserting POI",
			zap.String("poi_name", poi.Name),
			zap.Float64("latitude", poi.Latitude),
			zap.Float64("longitude", poi.Longitude),
			zap.String("category", poi.Category),
			zap.String("description", poi.Description),
			zap.Float64("distance", poi.Distance))

		_, err := tx.Exec(ctx, stmt.Name, poi.ID, userID, llmInteractionID, poi.Name, poi.Latitude, poi.Longitude, poi.Category, poi.Description, poi.Distance)
		if err != nil {
			l.Error("Failed to insert LLM POI", zap.Any("error", err), zap.String("poi_name", poi.Name))
			span.RecordError(err)
			return fmt.Errorf("failed to insert LLM POI: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		l.Error("Failed to commit transaction for saving LLM POIs", zap.Any("error", err))
		span.RecordError(err)
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	l.Info("Successfully saved LLM POIs to database", zap.Int("count", len(pois)))
	span.SetStatus(codes.Ok, "LLM POIs saved successfully")
	return nil
}

// CalculateDistancePostGIS calculateDistancePostGIS computes the distance between two points using PostGIS (in meters)
func (r *RepositoryImpl) CalculateDistancePostGIS(ctx context.Context, userLat, userLon, poiLat, poiLon float64) (float64, error) {
	query := `
        SELECT ST_Distance(
            ST_SetSRID(ST_MakePoint($1, $2), 4326)::geography,
            ST_SetSRID(ST_MakePoint($3, $4), 4326)::geography
        ) AS distance;
    `
	var distance float64
	err := r.pgpool.QueryRow(ctx, query, userLon, userLat, poiLon, poiLat).Scan(&distance)
	if err != nil {
		return 0, fmt.Errorf("failed to calculate distance with PostGIS: %w", err)
	}
	return distance, nil
}

// FindLLMPOIByNameAndCity finds an existing LLM POI by name and city
func (r *RepositoryImpl) FindLLMPOIByNameAndCity(ctx context.Context, name, city string) (uuid.UUID, error) {
	ctx, span := otel.Tracer("POIRepository").Start(ctx, "FindLLMPOIByNameAndCity")
	defer span.End()

	query := `
		SELECT id
		FROM llm_suggested_pois
		WHERE LOWER(name) = LOWER($1) AND LOWER(city_name) = LOWER($2)
		LIMIT 1
	`

	var id uuid.UUID
	err := r.pgpool.QueryRow(ctx, query, name, city).Scan(&id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return uuid.Nil, fmt.Errorf("LLM POI not found")
		}
		return uuid.Nil, fmt.Errorf("failed to find LLM POI: %w", err)
	}

	span.SetAttributes(attribute.String("poi.name", name), attribute.String("poi.city", city))
	return id, nil
}

// FindLLMPOIByName finds an existing LLM POI by name across all cities
func (r *RepositoryImpl) FindLLMPOIByName(ctx context.Context, name string) (uuid.UUID, error) {
	ctx, span := otel.Tracer("POIRepository").Start(ctx, "FindLLMPOIByName")
	defer span.End()

	query := `
		SELECT id
		FROM llm_suggested_pois
		WHERE LOWER(name) = LOWER($1)
		LIMIT 1
	`

	var id uuid.UUID
	err := r.pgpool.QueryRow(ctx, query, name).Scan(&id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return uuid.Nil, fmt.Errorf("LLM POI not found")
		}
		return uuid.Nil, fmt.Errorf("failed to find LLM POI: %w", err)
	}

	span.SetAttributes(attribute.String("poi.name", name))
	return id, nil
}
