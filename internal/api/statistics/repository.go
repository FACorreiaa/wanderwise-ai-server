package statistics

import (
	"context"
	"log/slog"

	"github.com/FACorreiaa/go-poi-au-suggestions/internal/types"
	"github.com/jackc/pgx/v5/pgxpool"
	uuid "github.com/vgarvardt/pgx-google-uuid/v5"
)

var _ Repository = (*RepositoryImpl)(nil)

type Repository interface {
	// GetMainPageStatistics retrieves the main page statistics.
	GetMainPageStatistics(ctx context.Context, userID uuid.UUID) (*types.MainPageStatistics, error)
	// GetDetailedPOIStatistics retrieves detailed POI statistics by type.
	GetDetailedPOIStatistics(ctx context.Context, userID uuid.UUID) (*types.DetailedPOIStatistics, error)
}

type RepositoryImpl struct {
	logger *slog.Logger
	pgpool *pgxpool.Pool
}

func NewRepository(logger *slog.Logger, pgpool *pgxpool.Pool) *RepositoryImpl {
	return &RepositoryImpl{
		logger: logger,
		pgpool: pgpool,
	}
}

func (r *RepositoryImpl) GetMainPageStatistics(ctx context.Context, userID uuid.UUID) (*types.MainPageStatistics, error) {
	r.logger.InfoContext(ctx, "Getting main page statistics for user")

	query := `
		WITH user_unique_pois AS (
			-- POIs from poi_details
			SELECT DISTINCT 
				pd.name,
				pd.latitude,
				pd.longitude,
				'poi_details' as source_table
			FROM poi_details pd
			JOIN llm_interactions li ON pd.llm_interaction_id = li.id
			WHERE li.user_id = $1
			
			UNION
			
			-- POIs from llm_suggested_pois  
			SELECT DISTINCT
				lsp.name,
				lsp.latitude,
				lsp.longitude,
				'llm_suggested_pois' as source_table
			FROM llm_suggested_pois lsp
			WHERE lsp.user_id = $1
			
			UNION
			
			-- Hotels
			SELECT DISTINCT
				hd.name,
				hd.latitude,
				hd.longitude,
				'hotel_details' as source_table
			FROM hotel_details hd
			JOIN llm_interactions li ON hd.llm_interaction_id = li.id
			WHERE li.user_id = $1
			
			UNION
			
			-- Restaurants
			SELECT DISTINCT
				rd.name,
				rd.latitude,
				rd.longitude,
				'restaurant_details' as source_table
			FROM restaurant_details rd
			JOIN llm_interactions li ON rd.llm_interaction_id = li.id
			WHERE li.user_id = $1
		),
		user_itineraries AS (
			-- Count saved itineraries for the user
			SELECT COUNT(*) as itinerary_count
			FROM user_saved_itineraries usi
			WHERE usi.user_id = $1
		),
		total_users AS (
			-- Count total active users in the system
			SELECT COUNT(*) as user_count
			FROM users u
			WHERE u.is_active = true
		)
		SELECT
			(SELECT user_count FROM total_users) AS total_users_count,
			(SELECT itinerary_count FROM user_itineraries) AS total_itineraries_created,
			COUNT(*) AS total_unique_pois
		FROM user_unique_pois;
	`

	var stats types.MainPageStatistics

	err := r.pgpool.QueryRow(ctx, query, userID).Scan(
		&stats.TotalUsersCount,
		&stats.TotalItinerariesCreated,
		&stats.TotalUniquePOIs,
	)

	if err != nil {
		r.logger.ErrorContext(ctx, "failed to get main page statistics", slog.Any("error", err))
		return nil, err
	}

	r.logger.InfoContext(ctx, "Successfully retrieved main page statistics",
		slog.Int64("total_users", stats.TotalUsersCount),
		slog.Int64("user_itineraries", stats.TotalItinerariesCreated),
		slog.Int64("unique_pois", stats.TotalUniquePOIs))

	return &stats, nil
}

func (r *RepositoryImpl) GetDetailedPOIStatistics(ctx context.Context, userID uuid.UUID) (*types.DetailedPOIStatistics, error) {
	r.logger.InfoContext(ctx, "Getting detailed POI statistics for user")

	query := `
		WITH poi_breakdown AS (
			-- POIs from poi_details
			SELECT 'general_poi' as poi_type, COUNT(*) as count
			FROM poi_details pd
			JOIN llm_interactions li ON pd.llm_interaction_id = li.id
			WHERE li.user_id = $1
			
			UNION ALL
			
			-- POIs from llm_suggested_pois  
			SELECT 'suggested_poi' as poi_type, COUNT(*) as count
			FROM llm_suggested_pois lsp
			WHERE lsp.user_id = $1
			
			UNION ALL
			
			-- Hotels
			SELECT 'hotel' as poi_type, COUNT(*) as count
			FROM hotel_details hd
			JOIN llm_interactions li ON hd.llm_interaction_id = li.id
			WHERE li.user_id = $1
			
			UNION ALL
			
			-- Restaurants
			SELECT 'restaurant' as poi_type, COUNT(*) as count
			FROM restaurant_details rd
			JOIN llm_interactions li ON rd.llm_interaction_id = li.id
			WHERE li.user_id = $1
		)
		SELECT 
			COALESCE(SUM(CASE WHEN poi_type = 'general_poi' THEN count END), 0) as general_pois,
			COALESCE(SUM(CASE WHEN poi_type = 'suggested_poi' THEN count END), 0) as suggested_pois,
			COALESCE(SUM(CASE WHEN poi_type = 'hotel' THEN count END), 0) as hotels,
			COALESCE(SUM(CASE WHEN poi_type = 'restaurant' THEN count END), 0) as restaurants,
			COALESCE(SUM(count), 0) as total_pois
		FROM poi_breakdown;
	`

	var stats types.DetailedPOIStatistics

	err := r.pgpool.QueryRow(ctx, query, userID).Scan(
		&stats.GeneralPOIs,
		&stats.SuggestedPOIs,
		&stats.Hotels,
		&stats.Restaurants,
		&stats.TotalPOIs,
	)

	if err != nil {
		r.logger.ErrorContext(ctx, "failed to get detailed POI statistics", slog.Any("error", err))
		return nil, err
	}

	r.logger.InfoContext(ctx, "Successfully retrieved detailed POI statistics",
		slog.Int64("general_pois", stats.GeneralPOIs),
		slog.Int64("suggested_pois", stats.SuggestedPOIs),
		slog.Int64("hotels", stats.Hotels),
		slog.Int64("restaurants", stats.Restaurants),
		slog.Int64("total_pois", stats.TotalPOIs))

	return &stats, nil
}
