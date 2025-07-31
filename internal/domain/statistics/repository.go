package statistics

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
)

var _ Repository = (*RepositoryImpl)(nil)

type Repository interface {
	GetMainPageStatistics(ctx context.Context, userID uuid.UUID) (*MainPageStatistics, error)
	GetDetailedPOIStatistics(ctx context.Context, userID uuid.UUID) (*DetailedPOIStatistics, error)
	LandingPageStatistics(ctx context.Context, userID uuid.UUID) (*LandingPageUserStats, error)
}

type RepositoryImpl struct {
	logger *zap.Logger
	pgpool *pgxpool.Pool
}

func NewRepository(logger *zap.Logger, pgpool *pgxpool.Pool) *RepositoryImpl {
	return &RepositoryImpl{
		logger: logger,
		pgpool: pgpool,
	}
}

func (r *RepositoryImpl) GetMainPageStatistics(ctx context.Context, userID uuid.UUID) (*MainPageStatistics, error) {
	r.logger.Info("Getting main page statistics for user", zap.String("user_id", userID.String()))

	// Check if this is a request for aggregate statistics (system user)
	systemUserID := uuid.MustParse("00000000-0000-0000-0000-000000000000")
	var query string
	var args []interface{}
	if userID == systemUserID {
		query = `
			WITH all_unique_pois AS (
				-- POIs from poi_details (all users)
				SELECT DISTINCT
					pd.name,
					pd.latitude,
					pd.longitude,
					'poi_details' as source_table
				FROM poi_details pd
				JOIN llm_interactions li ON pd.llm_interaction_id = li.id

				UNION

				-- POIs from llm_suggested_pois (all users)
				SELECT DISTINCT
					lsp.name,
					lsp.latitude,
					lsp.longitude,
					'llm_suggested_pois' as source_table
				FROM llm_suggested_pois lsp

				UNION

				-- POIs from hotel_details (all users)
				SELECT DISTINCT
					hd.name,
					hd.latitude,
					hd.longitude,
					'hotel_details' as source_table
				FROM hotel_details hd
				JOIN llm_interactions li ON hd.llm_interaction_id = li.id

				UNION

				-- POIs from restaurant_details (all users)
				SELECT DISTINCT
					rd.name,
					rd.latitude,
					rd.longitude,
					'restaurant_details' as source_table
				FROM restaurant_details rd
				JOIN llm_interactions li ON rd.llm_interaction_id = li.id
			),
			total_itineraries AS (
				-- Count all saved itineraries across all users
				SELECT COUNT(*) as itinerary_count
				FROM user_saved_itineraries usi
			),
			total_users AS (
				-- Count total active users in the system
				SELECT COUNT(*) as user_count
				FROM users u
				WHERE u.is_active = true
			)
			SELECT
				(SELECT user_count FROM total_users) AS total_users_count,
				(SELECT itinerary_count FROM total_itineraries) AS total_itineraries_saved,
				COUNT(*) AS total_unique_pois
			FROM all_unique_pois;
		`
	} else {
		query = `
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
				-- Count saved/bookmarked itineraries for the user
				SELECT COUNT(*) as saved_itinerary_count
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
				(SELECT saved_itinerary_count FROM user_itineraries) AS total_itineraries_saved,
				COUNT(*) AS total_unique_pois
			FROM user_unique_pois;
		`
		args = append(args, userID)
	}

	var stats MainPageStatistics

	err := r.pgpool.QueryRow(ctx, query, args...).Scan(
		&stats.TotalUsersCount,
		&stats.TotalItinerariesSaved,
		&stats.TotalUniquePOIs,
	)

	if err != nil {
		r.logger.Error("failed to get main page statistics", zap.Error(err), zap.String("user_id", userID.String()))
		return nil, err
	}

	r.logger.Info("Successfully retrieved main page statistics",
		zap.String("user_id", userID.String()),
		zap.Int64("total_users", stats.TotalUsersCount),
		zap.Int64("user_itineraries", stats.TotalItinerariesSaved),
		zap.Int64("unique_pois", stats.TotalUniquePOIs))

	return &stats, nil
}

func (r *RepositoryImpl) GetDetailedPOIStatistics(ctx context.Context, userID uuid.UUID) (*DetailedPOIStatistics, error) {
	r.logger.Info("Getting detailed POI statistics for user", zap.String("user_id", userID.String()))

	query := `
		SELECT
			COUNT(DISTINCT pd.id) as general_pois,
			COUNT(DISTINCT lsp.id) as suggested_pois,
			COUNT(DISTINCT hd.id) as hotels,
			COUNT(DISTINCT rd.id) as restaurants,
			(COUNT(DISTINCT pd.id) + COUNT(DISTINCT lsp.id) + COUNT(DISTINCT hd.id) + COUNT(DISTINCT rd.id)) as total_pois
		FROM llm_interactions li
		LEFT JOIN poi_details pd ON li.id = pd.llm_interaction_id AND li.user_id = $1
		LEFT JOIN llm_suggested_pois lsp ON li.user_id = lsp.user_id
		LEFT JOIN hotel_details hd ON li.id = hd.llm_interaction_id AND li.user_id = $1
		LEFT JOIN restaurant_details rd ON li.id = rd.llm_interaction_id AND li.user_id = $1
		WHERE li.user_id = $1
	`

	var stats DetailedPOIStatistics

	err := r.pgpool.QueryRow(ctx, query, userID).Scan(
		&stats.GeneralPOIs,
		&stats.SuggestedPOIs,
		&stats.Hotels,
		&stats.Restaurants,
		&stats.TotalPOIs,
	)

	if err != nil {
		r.logger.Error("failed to get detailed POI statistics", zap.Error(err), zap.String("user_id", userID.String()))
		return nil, err
	}

	r.logger.Info("Successfully retrieved detailed POI statistics",
		zap.String("user_id", userID.String()),
		zap.Int64("general_pois", stats.GeneralPOIs),
		zap.Int64("suggested_pois", stats.SuggestedPOIs),
		zap.Int64("hotels", stats.Hotels),
		zap.Int64("restaurants", stats.Restaurants),
		zap.Int64("total_pois", stats.TotalPOIs))

	return &stats, nil
}

func (r *RepositoryImpl) LandingPageStatistics(ctx context.Context, userID uuid.UUID) (*LandingPageUserStats, error) {
	r.logger.Info("Getting LandingPageStatistics", zap.String("user_id", userID.String()))

	query := `
	SELECT
    (SELECT COUNT(*) FROM user_favorite_llm_pois WHERE user_id = $1) AS saved_places,
    (SELECT COUNT(*) FROM itineraries WHERE user_id = $1) AS itineraries,
    (SELECT COUNT(DISTINCT city_id) FROM itineraries WHERE user_id = $1) AS cities_explored,
    (SELECT COUNT(*) FROM chat_sessions WHERE user_id = $1) AS discoveries;
	`

	var stats LandingPageUserStats

	err := r.pgpool.QueryRow(ctx, query, userID).Scan(
		&stats.SavedPlaces,
		&stats.Itineraries,
		&stats.CitiesExplored,
		&stats.Discoveries,
	)

	if err != nil {
		r.logger.Error("failed to get detailed POI statistics", zap.Error(err), zap.String("user_id", userID.String()))
		return nil, err
	}

	r.logger.Info("Successfully retrieved user Stats",
		zap.String("user_id", userID.String()),
		zap.Int("saved_places", stats.SavedPlaces),
		zap.Int("itineraries", stats.Itineraries),
		zap.Int("cities_explored", stats.CitiesExplored),
		zap.Int("discoveries", stats.Discoveries))

	return &stats, nil
}
