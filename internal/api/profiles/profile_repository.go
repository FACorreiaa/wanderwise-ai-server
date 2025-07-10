package profiles

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"

	"github.com/FACorreiaa/go-poi-au-suggestions/internal/types"
)

var _ Repository = (*RepositoryImpl)(nil)

// profilessRepo defines the contract for user data persistence.
type Repository interface {
	// GetSearchProfiles --- User Preference Profiles ---
	// GetSearchProfiles retrieves all preference profiles for a user
	GetSearchProfiles(ctx context.Context, userID uuid.UUID) ([]types.UserPreferenceProfileResponse, error)
	// GetSearchProfile retrieves a specific preference profile by ID
	GetSearchProfile(ctx context.Context, userID, profileID uuid.UUID) (*types.UserPreferenceProfileResponse, error)
	// GetDefaultSearchProfile retrieves the default preference profile for a user
	GetDefaultSearchProfile(ctx context.Context, userID uuid.UUID) (*types.UserPreferenceProfileResponse, error)
	// CreateSearchProfile creates a new preference profile for a user
	CreateSearchProfile(ctx context.Context, userID uuid.UUID, params types.CreateUserPreferenceProfileParams) (*types.UserPreferenceProfileResponse, error)
	// UpdateSearchProfile updates a preference profile
	UpdateSearchProfile(ctx context.Context, userID, profileID uuid.UUID, params types.UpdateSearchProfileParams) error
	// DeleteSearchProfile deletes a preference profile
	DeleteSearchProfile(ctx context.Context, userID, profileID uuid.UUID) error
	// SetDefaultSearchProfile sets a profile as the default for a user
	SetDefaultSearchProfile(ctx context.Context, userID, profileID uuid.UUID) error
}

type RepositoryImpl struct {
	logger *slog.Logger
	pgpool *pgxpool.Pool
}

func NewPostgresUserRepo(pgxpool *pgxpool.Pool, logger *slog.Logger) *RepositoryImpl {
	return &RepositoryImpl{
		logger: logger,
		pgpool: pgxpool,
	}
}

//SELECT upp.profile_name, upp.is_default, upp.search_radius_km,
//upp.preferred_time, upp.budget_level, upp.preferred_pace,
//upp.prefer_accessible_pois, prefer_outdoor_seating,
//upp.prefer_dog_friendly, upp.preferred_vibes,
//upp.preferred_transport, upp.dietary_needs,
//ucc.name, ucc.description ,ucc.active
//FROM user_preference_profiles upp
//JOIN user_custom_interests ucc ON ucc.user_id = upp.user_id
//WHERE upp.user_id = 'f835199b-7d87-4450-841c-b94fcf9706b0'
//ORDER BY upp.profile_name

// GetProfiles implements user.UserRepo.
func (r *RepositoryImpl) GetSearchProfiles(ctx context.Context, userID uuid.UUID) ([]types.UserPreferenceProfileResponse, error) {
	ctx, span := otel.Tracer("UserRepo").Start(ctx, "GetUserPreferenceProfiles", trace.WithAttributes(
		semconv.DBSystemPostgreSQL,
		attribute.String("db.sql.table", "user_preference_profiles"),
		attribute.String("db.user.id", userID.String()),
	))
	defer span.End()

	l := r.logger.With(slog.String("method", "GetUserPreferenceProfiles"), slog.String("userID", userID.String()))
	l.DebugContext(ctx, "Fetching user preference profiles")

	query := `
        SELECT id, user_id, profile_name, is_default, search_radius_km, preferred_time, 
               budget_level, preferred_pace, prefer_accessible_pois, prefer_outdoor_seating, 
               prefer_dog_friendly, preferred_vibes, preferred_transport, dietary_needs, 
               created_at, updated_at
        FROM user_preference_profiles
        WHERE user_id = $1
        ORDER BY is_default DESC, profile_name`

	rows, err := r.pgpool.Query(ctx, query, userID)
	if err != nil {
		l.ErrorContext(ctx, "Failed to query user preference profiles", slog.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "DB query failed")
		return nil, fmt.Errorf("database error fetching preference profiles: %w", err)
	}
	defer rows.Close()

	var profiles []types.UserPreferenceProfileResponse
	for rows.Next() {
		var p types.UserPreferenceProfileResponse
		err := rows.Scan(
			&p.ID, &p.UserID, &p.ProfileName, &p.IsDefault, &p.SearchRadiusKm, &p.PreferredTime,
			&p.BudgetLevel, &p.PreferredPace, &p.PreferAccessiblePOIs, &p.PreferOutdoorSeating,
			&p.PreferDogFriendly, &p.PreferredVibes, &p.PreferredTransport, &p.DietaryNeeds,
			&p.CreatedAt, &p.UpdatedAt,
		)
		if err != nil {
			l.ErrorContext(ctx, "Failed to scan preference profile row", slog.Any("error", err))
			span.RecordError(err)
			return nil, fmt.Errorf("database error scanning preference profile: %w", err)
		}
		profiles = append(profiles, p)
	}

	if err = rows.Err(); err != nil {
		l.ErrorContext(ctx, "Error iterating preference profile rows", slog.Any("error", err))
		span.RecordError(err)
		return nil, fmt.Errorf("database error reading preference profiles: %w", err)
	}

	l.DebugContext(ctx, "Fetched user preference profiles successfully", slog.Int("count", len(profiles)))
	span.SetStatus(codes.Ok, "Preference profiles fetched")
	return profiles, nil
}

// GetProfile implements user.UserRepo.
func (r *RepositoryImpl) GetSearchProfile(ctx context.Context, userID, profileID uuid.UUID) (*types.UserPreferenceProfileResponse, error) {
	ctx, span := otel.Tracer("UserRepo").Start(ctx, "GetUserPreferenceProfile", trace.WithAttributes(
		semconv.DBSystemPostgreSQL,
		attribute.String("db.sql.table", "user_preference_profiles"),
		attribute.String("db.profile.id", profileID.String()),
	))
	defer span.End()

	l := r.logger.With(slog.String("method", "GetUserPreferenceProfile"), slog.String("profileID", profileID.String()))
	l.DebugContext(ctx, "Fetching user preference profile")

	query := `
        SELECT id, user_id, profile_name, is_default, search_radius_km, preferred_time, 
               budget_level, preferred_pace, prefer_accessible_pois, prefer_outdoor_seating, 
               prefer_dog_friendly, preferred_vibes, preferred_transport, dietary_needs, 
               created_at, updated_at
        FROM user_preference_profiles
        WHERE id = $1 AND user_id = $2`

	var p types.UserPreferenceProfileResponse
	err := r.pgpool.QueryRow(ctx, query, profileID, userID).Scan(
		&p.ID, &p.UserID, &p.ProfileName, &p.IsDefault, &p.SearchRadiusKm, &p.PreferredTime,
		&p.BudgetLevel, &p.PreferredPace, &p.PreferAccessiblePOIs, &p.PreferOutdoorSeating,
		&p.PreferDogFriendly, &p.PreferredVibes, &p.PreferredTransport, &p.DietaryNeeds,
		&p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		l.ErrorContext(ctx, "Failed to query user preference profile", slog.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "DB query failed")
		return nil, fmt.Errorf("preference profile not found: %w", types.ErrNotFound)
	}

	l.DebugContext(ctx, "Fetched user preference profile successfully")
	span.SetStatus(codes.Ok, "Preference profile fetched")
	return &p, nil
}

// GetDefaultProfile implements user.UserRepo.
func (r *RepositoryImpl) GetDefaultSearchProfile(ctx context.Context, userID uuid.UUID) (*types.UserPreferenceProfileResponse, error) {
	ctx, span := otel.Tracer("UserRepo").Start(ctx, "GetDefaultUserPreferenceProfile", trace.WithAttributes(
		semconv.DBSystemPostgreSQL,
		attribute.String("db.sql.table", "user_preference_profiles"),
		attribute.String("db.user.id", userID.String()),
	))
	defer span.End()

	l := r.logger.With(slog.String("method", "GetDefaultUserPreferenceProfile"), slog.String("userID", userID.String()))
	l.DebugContext(ctx, "Fetching default user preference profile")

	query := `
        SELECT id, user_id, profile_name, is_default, search_radius_km, preferred_time, 
               budget_level, preferred_pace, prefer_accessible_pois, prefer_outdoor_seating, 
               prefer_dog_friendly, preferred_vibes, preferred_transport, dietary_needs, 
               created_at, updated_at
        FROM user_preference_profiles
        WHERE user_id = $1 AND is_default = TRUE`

	var p types.UserPreferenceProfileResponse
	err := r.pgpool.QueryRow(ctx, query, userID).Scan(
		&p.ID, &p.UserID, &p.ProfileName, &p.IsDefault, &p.SearchRadiusKm, &p.PreferredTime,
		&p.BudgetLevel, &p.PreferredPace, &p.PreferAccessiblePOIs, &p.PreferOutdoorSeating,
		&p.PreferDogFriendly, &p.PreferredVibes, &p.PreferredTransport, &p.DietaryNeeds,
		&p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		l.ErrorContext(ctx, "Failed to query default user preference profile", slog.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "DB query failed")
		return nil, fmt.Errorf("default preference profile not found: %w", types.ErrNotFound)
	}

	l.DebugContext(ctx, "Fetched default user preference profile successfully")
	span.SetStatus(codes.Ok, "Default preference profile fetched")
	return &p, nil
}

// CreateProfile implements user.UserRepo.
func (r *RepositoryImpl) CreateSearchProfile(ctx context.Context, userID uuid.UUID, params types.CreateUserPreferenceProfileParams) (*types.UserPreferenceProfileResponse, error) {
	ctx, span := otel.Tracer("UserRepo").Start(ctx, "CreateUserPreferenceProfile", trace.WithAttributes(
		semconv.DBSystemPostgreSQL,
		attribute.String("db.operation", "INSERT"),
		attribute.String("db.sql.table", "user_preference_profiles"),
		attribute.String("db.user.id", userID.String()),
	))
	defer span.End()

	tx, err := r.pgpool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}

	l := r.logger.With(slog.String("method", "CreateUserPreferenceProfile"), slog.String("userID", userID.String()))
	l.DebugContext(ctx, "Creating user preference profile", slog.String("profileName", params.ProfileName))

	// Set default values for optional parameters (as before)
	isDefault := false
	if params.IsDefault != nil {
		isDefault = *params.IsDefault
	}
	searchRadiusKm := 5.0
	if params.SearchRadiusKm != nil {
		searchRadiusKm = *params.SearchRadiusKm
	}
	preferredTime := types.DayPreferenceAny
	if params.PreferredTime != nil {
		preferredTime = *params.PreferredTime
	}
	budgetLevel := 0
	if params.BudgetLevel != nil {
		budgetLevel = *params.BudgetLevel
	}
	preferredPace := types.SearchPaceAny
	if params.PreferredPace != nil {
		preferredPace = *params.PreferredPace
	}
	preferAccessiblePOIs := false
	if params.PreferAccessiblePOIs != nil {
		preferAccessiblePOIs = *params.PreferAccessiblePOIs
	}
	preferOutdoorSeating := false
	if params.PreferOutdoorSeating != nil {
		preferOutdoorSeating = *params.PreferOutdoorSeating
	}
	preferDogFriendly := false
	if params.PreferDogFriendly != nil {
		preferDogFriendly = *params.PreferDogFriendly
	}
	preferredVibes := params.PreferredVibes
	if preferredVibes == nil {
		preferredVibes = []string{}
	}
	preferredTransport := types.TransportPreferenceAny
	if params.PreferredTransport != nil {
		preferredTransport = *params.PreferredTransport
	}
	dietaryNeeds := params.DietaryNeeds
	if dietaryNeeds == nil {
		dietaryNeeds = []string{}
	}

	if isDefault {
		query := "UPDATE user_preference_profiles SET is_default = FALSE WHERE user_id = $1 AND id != $2"
		_, err := tx.Exec(ctx, query, userID, uuid.Nil) // uuid.Nil as placeholder; will be updated after insert
		if err != nil {
			err = tx.Rollback(ctx)
			if err != nil {
				return nil, fmt.Errorf("failed to rollback transaction: %w", err)
			}
			l.ErrorContext(ctx, "Failed to reset existing default profiles", slog.Any("error", err))
			span.RecordError(err)
			span.SetStatus(codes.Error, "Failed to reset defaults")
			return nil, fmt.Errorf("failed to reset existing default profiles: %w", err)
		}
	}

	// Insert base profile
	var p types.UserPreferenceProfileResponse
	query := `
        INSERT INTO user_preference_profiles (
            user_id, profile_name, is_default, search_radius_km, preferred_time, 
            budget_level, preferred_pace, prefer_accessible_pois, prefer_outdoor_seating, 
            prefer_dog_friendly, preferred_vibes, preferred_transport, dietary_needs
        ) VALUES (
            $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13
        ) RETURNING id, user_id, profile_name, is_default, search_radius_km, preferred_time, 
                   budget_level, preferred_pace, prefer_accessible_pois, prefer_outdoor_seating, 
                   prefer_dog_friendly, preferred_vibes, preferred_transport, dietary_needs, 
                   created_at, updated_at`
	err = tx.QueryRow(ctx, query,
		userID, params.ProfileName, isDefault, searchRadiusKm, preferredTime,
		budgetLevel, preferredPace, preferAccessiblePOIs, preferOutdoorSeating,
		preferDogFriendly, preferredVibes, preferredTransport, dietaryNeeds,
	).Scan(
		&p.ID, &p.UserID, &p.ProfileName, &p.IsDefault, &p.SearchRadiusKm, &p.PreferredTime,
		&p.BudgetLevel, &p.PreferredPace, &p.PreferAccessiblePOIs, &p.PreferOutdoorSeating,
		&p.PreferDogFriendly, &p.PreferredVibes, &p.PreferredTransport, &p.DietaryNeeds,
		&p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		err = tx.Rollback(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to rollback transaction: %w", err)
		}
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" { // Unique violation
			l.WarnContext(ctx, "Profile name already exists for this user", slog.Any("error", err))
			span.RecordError(err)
			span.SetStatus(codes.Error, "Profile name conflict")
			return nil, fmt.Errorf("profile name already exists: %w", types.ErrConflict)
		}
		l.ErrorContext(ctx, "Failed to create user preference profile", slog.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "DB INSERT failed")
		return nil, fmt.Errorf("database error creating preference profile: %w", err)
	}

	// Insert domain-specific preferences if provided
	if params.AccommodationPreferences != nil {
		accommodationJSON, err := json.Marshal(params.AccommodationPreferences)
		if err != nil {
			err = tx.Rollback(ctx)
			if err != nil {
				return nil, fmt.Errorf("failed to rollback transaction: %w", err)
			}
			l.ErrorContext(ctx, "Failed to marshal accommodation preferences", slog.Any("error", err))
			return nil, fmt.Errorf("failed to marshal accommodation preferences: %w", err)
		}
		query = `
            INSERT INTO user_accommodation_preferences (user_preference_profile_id, accommodation_filters)
            VALUES ($1, $2)`
		_, err = tx.Exec(ctx, query, p.ID, accommodationJSON)
		if err != nil {
			if rollbackErr := tx.Rollback(ctx); rollbackErr != nil {
				l.ErrorContext(ctx, "Failed to rollback transaction", slog.Any("error", rollbackErr))
			}
			l.ErrorContext(ctx, "Failed to insert accommodation preferences", slog.Any("error", err))
			return nil, fmt.Errorf("failed to insert accommodation preferences: %w", err)
		}
	}

	if params.DiningPreferences != nil {
		diningJSON, err := json.Marshal(params.DiningPreferences)
		if err != nil {
			if rollbackErr := tx.Rollback(ctx); rollbackErr != nil {
				l.ErrorContext(ctx, "Failed to rollback transaction", slog.Any("error", rollbackErr))
			}
			l.ErrorContext(ctx, "Failed to marshal dining preferences", slog.Any("error", err))
			return nil, fmt.Errorf("failed to marshal dining preferences: %w", err)
		}
		query = `
            INSERT INTO user_dining_preferences (user_preference_profile_id, dining_filters)
            VALUES ($1, $2)`
		_, err = tx.Exec(ctx, query, p.ID, diningJSON)
		if err != nil {
			if rollbackErr := tx.Rollback(ctx); rollbackErr != nil {
				l.ErrorContext(ctx, "Failed to rollback transaction", slog.Any("error", rollbackErr))
			}
			l.ErrorContext(ctx, "Failed to insert dining preferences", slog.Any("error", err))
			return nil, fmt.Errorf("failed to insert dining preferences: %w", err)
		}
	}

	if params.ActivityPreferences != nil {
		activityJSON, err := json.Marshal(params.ActivityPreferences)
		if err != nil {
			if rollbackErr := tx.Rollback(ctx); rollbackErr != nil {
				l.ErrorContext(ctx, "Failed to rollback transaction", slog.Any("error", rollbackErr))
			}
			l.ErrorContext(ctx, "Failed to marshal activity preferences", slog.Any("error", err))
			return nil, fmt.Errorf("failed to marshal activity preferences: %w", err)
		}
		query = `
            INSERT INTO user_activity_preferences (user_preference_profile_id, activity_filters)
            VALUES ($1, $2)`
		_, err = tx.Exec(ctx, query, p.ID, activityJSON)
		if err != nil {
			if rollbackErr := tx.Rollback(ctx); rollbackErr != nil {
				l.ErrorContext(ctx, "Failed to rollback transaction", slog.Any("error", rollbackErr))
			}
			l.ErrorContext(ctx, "Failed to insert activity preferences", slog.Any("error", err))
			return nil, fmt.Errorf("failed to insert activity preferences: %w", err)
		}
	}

	if params.ItineraryPreferences != nil {
		itineraryJSON, err := json.Marshal(params.ItineraryPreferences)
		if err != nil {
			if rollbackErr := tx.Rollback(ctx); rollbackErr != nil {
				l.ErrorContext(ctx, "Failed to rollback transaction", slog.Any("error", rollbackErr))
			}
			l.ErrorContext(ctx, "Failed to marshal itinerary preferences", slog.Any("error", err))
			return nil, fmt.Errorf("failed to marshal itinerary preferences: %w", err)
		}
		query = `
            INSERT INTO user_itinerary_preferences (user_preference_profile_id, itinerary_filters)
            VALUES ($1, $2)`
		_, err = tx.Exec(ctx, query, p.ID, itineraryJSON)
		if err != nil {
			if rollbackErr := tx.Rollback(ctx); rollbackErr != nil {
				l.ErrorContext(ctx, "Failed to rollback transaction", slog.Any("error", rollbackErr))
			}
			l.ErrorContext(ctx, "Failed to insert itinerary preferences", slog.Any("error", err))
			return nil, fmt.Errorf("failed to insert itinerary preferences: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	l.InfoContext(ctx, "User preference profile created successfully", slog.String("profileID", p.ID.String()))
	span.SetStatus(codes.Ok, "Preference profile created")
	return &p, nil
}

// UpdateProfile implements user.UserRepo.
func (r *RepositoryImpl) UpdateSearchProfile(ctx context.Context, userID, profileID uuid.UUID, params types.UpdateSearchProfileParams) error {
	ctx, span := otel.Tracer("UserRepo").Start(ctx, "UpdateUserPreferenceProfile", trace.WithAttributes(
		semconv.DBSystemPostgreSQL,
		attribute.String("db.operation", "UPDATE"),
		attribute.String("db.sql.table", "user_preference_profiles"),
		attribute.String("db.profile.id", profileID.String()),
	))
	defer span.End()

	l := r.logger.With(slog.String("method", "UpdateUserPreferenceProfile"), slog.String("profileID", profileID.String()))
	l.DebugContext(ctx, "Updating user preference profile")

	// Begin transaction to update profile and domain preferences atomically
	tx, err := r.pgpool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	updateBuilder := squirrel.Update("user_preference_profiles").
		PlaceholderFormat(squirrel.Dollar).
		Where(squirrel.Eq{"id": profileID, "user_id": userID})

	var hasUpdates bool

	if params.ProfileName != "" {
		updateBuilder = updateBuilder.Set("profile_name", params.ProfileName)
		hasUpdates = true
	}
	if params.IsDefault != nil {
		updateBuilder = updateBuilder.Set("is_default", *params.IsDefault)
		hasUpdates = true
	}
	if params.SearchRadiusKm != nil {
		updateBuilder = updateBuilder.Set("search_radius_km", *params.SearchRadiusKm)
		hasUpdates = true
	}
	if params.PreferredTime != nil {
		updateBuilder = updateBuilder.Set("preferred_time", *params.PreferredTime)
		hasUpdates = true
	}
	if params.BudgetLevel != nil {
		updateBuilder = updateBuilder.Set("budget_level", *params.BudgetLevel)
		hasUpdates = true
	}
	if params.PreferredPace != nil {
		updateBuilder = updateBuilder.Set("preferred_pace", *params.PreferredPace)
		hasUpdates = true
	}
	if params.PreferAccessiblePOIs != nil {
		updateBuilder = updateBuilder.Set("prefer_accessible_pois", *params.PreferAccessiblePOIs)
		hasUpdates = true
	}
	if params.PreferOutdoorSeating != nil {
		updateBuilder = updateBuilder.Set("prefer_outdoor_seating", *params.PreferOutdoorSeating)
		hasUpdates = true
	}
	if params.PreferDogFriendly != nil {
		updateBuilder = updateBuilder.Set("prefer_dog_friendly", *params.PreferDogFriendly)
		hasUpdates = true
	}
	if params.PreferredVibes != nil {
		updateBuilder = updateBuilder.Set("preferred_vibes", params.PreferredVibes)
		hasUpdates = true
	}
	if params.PreferredTransport != nil {
		updateBuilder = updateBuilder.Set("preferred_transport", *params.PreferredTransport)
		hasUpdates = true
	}
	if params.DietaryNeeds != nil {
		updateBuilder = updateBuilder.Set("dietary_needs", params.DietaryNeeds)
		hasUpdates = true
	}

	// Update main profile if there are changes
	if hasUpdates {
		updateBuilder = updateBuilder.Set("updated_at", time.Now())

		query, args, err := updateBuilder.ToSql()
		if err != nil {
			if rollbackErr := tx.Rollback(ctx); rollbackErr != nil {
			l.ErrorContext(ctx, "Failed to rollback transaction", slog.Any("rollback_error", rollbackErr))
		}
			return fmt.Errorf("failed to build update query: %w", err)
		}

		tag, err := tx.Exec(ctx, query, args...)
		if err != nil {
			if rollbackErr := tx.Rollback(ctx); rollbackErr != nil {
			l.ErrorContext(ctx, "Failed to rollback transaction", slog.Any("rollback_error", rollbackErr))
		}
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" { // Unique violation
				l.WarnContext(ctx, "Profile name already exists for this user", slog.Any("error", err))
				span.RecordError(err)
				span.SetStatus(codes.Error, "Profile name conflict")
				return fmt.Errorf("profile name already exists: %w", types.ErrConflict)
			}
			l.ErrorContext(ctx, "Failed to update user preference profile", slog.Any("error", err))
			span.RecordError(err)
			span.SetStatus(codes.Error, "DB UPDATE failed")
			return fmt.Errorf("database error updating preference profile: %w", err)
		}

		if tag.RowsAffected() == 0 {
			if rollbackErr := tx.Rollback(ctx); rollbackErr != nil {
			l.ErrorContext(ctx, "Failed to rollback transaction", slog.Any("rollback_error", rollbackErr))
		}
			err := fmt.Errorf("preference profile not found: %w", types.ErrNotFound)
			l.WarnContext(ctx, "Attempted to update non-existent preference profile")
			span.RecordError(err)
			span.SetStatus(codes.Error, "Profile not found")
			return err
		}
	}

	// Update domain-specific preferences if provided
	if params.AccommodationPreferences != nil {
		if err := r.updateAccommodationPreferencesInTx(ctx, tx, profileID, params.AccommodationPreferences); err != nil {
			if rollbackErr := tx.Rollback(ctx); rollbackErr != nil {
			l.ErrorContext(ctx, "Failed to rollback transaction", slog.Any("rollback_error", rollbackErr))
		}
			l.ErrorContext(ctx, "Failed to update accommodation preferences", slog.Any("error", err))
			return fmt.Errorf("failed to update accommodation preferences: %w", err)
		}
	}

	if params.DiningPreferences != nil {
		if err := r.updateDiningPreferencesInTx(ctx, tx, profileID, params.DiningPreferences); err != nil {
			if rollbackErr := tx.Rollback(ctx); rollbackErr != nil {
			l.ErrorContext(ctx, "Failed to rollback transaction", slog.Any("rollback_error", rollbackErr))
		}
			l.ErrorContext(ctx, "Failed to update dining preferences", slog.Any("error", err))
			return fmt.Errorf("failed to update dining preferences: %w", err)
		}
	}

	if params.ActivityPreferences != nil {
		if err := r.updateActivityPreferencesInTx(ctx, tx, profileID, params.ActivityPreferences); err != nil {
			if rollbackErr := tx.Rollback(ctx); rollbackErr != nil {
			l.ErrorContext(ctx, "Failed to rollback transaction", slog.Any("rollback_error", rollbackErr))
		}
			l.ErrorContext(ctx, "Failed to update activity preferences", slog.Any("error", err))
			return fmt.Errorf("failed to update activity preferences: %w", err)
		}
	}

	if params.ItineraryPreferences != nil {
		if err := r.updateItineraryPreferencesInTx(ctx, tx, profileID, params.ItineraryPreferences); err != nil {
			if rollbackErr := tx.Rollback(ctx); rollbackErr != nil {
			l.ErrorContext(ctx, "Failed to rollback transaction", slog.Any("rollback_error", rollbackErr))
		}
			l.ErrorContext(ctx, "Failed to update itinerary preferences", slog.Any("error", err))
			return fmt.Errorf("failed to update itinerary preferences: %w", err)
		}
	}

	// Commit transaction
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	l.InfoContext(ctx, "User preference profile and domain preferences updated successfully")
	span.SetStatus(codes.Ok, "Preference profile updated")
	return nil
}

// DeleteProfile implements user.UserRepo.
func (r *RepositoryImpl) DeleteSearchProfile(ctx context.Context, userID, profileID uuid.UUID) error {
	ctx, span := otel.Tracer("UserRepo").Start(ctx, "DeleteUserPreferenceProfile", trace.WithAttributes(
		semconv.DBSystemPostgreSQL,
		attribute.String("db.operation", "DELETE"),
		attribute.String("db.sql.table", "user_preference_profiles"),
		attribute.String("db.profile.id", profileID.String()),
	))
	defer span.End()

	l := r.logger.With(slog.String("method", "DeleteUserPreferenceProfile"), slog.String("profileID", profileID.String()))
	l.DebugContext(ctx, "Deleting user preference profile")

	// First check if this is the default profile
	var isDefault bool
	err := r.pgpool.QueryRow(ctx, "SELECT is_default FROM user_preference_profiles WHERE id = $1 AND user_id = $2", profileID, userID).Scan(&isDefault)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			err := fmt.Errorf("preference profile not found: %w", types.ErrNotFound)
			l.WarnContext(ctx, "Attempted to delete non-existent preference profile")
			span.RecordError(err)
			span.SetStatus(codes.Error, "Profile not found")
			return err
		}
		l.ErrorContext(ctx, "Failed to check if profile is default", slog.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "DB query failed")
		return fmt.Errorf("database error checking profile: %w", err)
	}

	if isDefault {
		err := errors.New("cannot delete default profile")
		l.WarnContext(ctx, "Attempted to delete default preference profile")
		span.RecordError(err)
		span.SetStatus(codes.Error, "Cannot delete default profile")
		return err
	}

	// Delete the profile
	tag, err := r.pgpool.Exec(ctx, "DELETE FROM user_preference_profiles WHERE id = $1 AND user_id = $2", profileID, userID)
	if err != nil {
		l.ErrorContext(ctx, "Failed to delete user preference profile", slog.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "DB DELETE failed")
		return fmt.Errorf("database error deleting preference profile: %w", err)
	}

	if tag.RowsAffected() == 0 {
		// This should not happen since we already checked if the profile exists
		err := fmt.Errorf("preference profile not found: %w", types.ErrNotFound)
		l.WarnContext(ctx, "Attempted to delete non-existent preference profile")
		span.RecordError(err)
		span.SetStatus(codes.Error, "Profile not found")
		return err
	}

	l.InfoContext(ctx, "User preference profile deleted successfully")
	span.SetStatus(codes.Ok, "Preference profile deleted")
	return nil
}

// SetDefaultProfile implements user.UserRepo.
func (r *RepositoryImpl) SetDefaultSearchProfile(ctx context.Context, userID, profileID uuid.UUID) error {
	ctx, span := otel.Tracer("UserRepo").Start(ctx, "SetDefaultUserPreferenceProfile", trace.WithAttributes(
		semconv.DBSystemPostgreSQL,
		attribute.String("db.operation", "UPDATE"),
		attribute.String("db.sql.table", "user_preference_profiles"),
		attribute.String("db.profile.id", profileID.String()),
	))
	defer span.End()

	l := r.logger.With(slog.String("method", "SetDefaultUserPreferenceProfile"), slog.String("profileID", profileID.String()))
	l.DebugContext(ctx, "Setting profile as default")

	// First get the user ID for this profile
	err := r.pgpool.QueryRow(ctx, "SELECT user_id FROM user_preference_profiles WHERE user_id = $1", userID).Scan(&userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			err := fmt.Errorf("preference profile not found: %w", types.ErrNotFound)
			l.WarnContext(ctx, "Attempted to set non-existent profile as default")
			span.RecordError(err)
			span.SetStatus(codes.Error, "Profile not found")
			return err
		}
		l.ErrorContext(ctx, "Failed to get user ID for profile", slog.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "DB query failed")
		return fmt.Errorf("database error getting profile: %w", err)
	}

	// Begin a transaction
	tx, err := r.pgpool.Begin(ctx)
	if err != nil {
		l.ErrorContext(ctx, "Failed to begin transaction", slog.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "DB transaction failed")
		return fmt.Errorf("database error beginning transaction: %w", err)
	}

	// First, set all profiles for this user to not be default
	_, err = tx.Exec(ctx, "UPDATE user_preference_profiles SET is_default = FALSE WHERE user_id = $1", userID)
	if err != nil {
		if rollbackErr := tx.Rollback(ctx); rollbackErr != nil {
			l.ErrorContext(ctx, "Failed to rollback transaction", slog.Any("rollback_error", rollbackErr))
		}
		l.ErrorContext(ctx, "Failed to reset default profiles", slog.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "DB UPDATE failed")
		return fmt.Errorf("database error resetting default profiles: %w", err)
	}

	// Then set the specified profile as default
	tag, err := tx.Exec(ctx, "UPDATE user_preference_profiles SET is_default = TRUE WHERE id = $1 AND user_id = $2", profileID, userID)
	if err != nil {
		if rollbackErr := tx.Rollback(ctx); rollbackErr != nil {
			l.ErrorContext(ctx, "Failed to rollback transaction", slog.Any("rollback_error", rollbackErr))
		}
		l.ErrorContext(ctx, "Failed to set profile as default", slog.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "DB UPDATE failed")
		return fmt.Errorf("database error setting default profile: %w", err)
	}

	if tag.RowsAffected() == 0 {
		if rollbackErr := tx.Rollback(ctx); rollbackErr != nil {
			l.ErrorContext(ctx, "Failed to rollback transaction", slog.Any("rollback_error", rollbackErr))
		}
		// This should not happen since we already checked if the profile exists
		err := fmt.Errorf("preference profile not found: %w", types.ErrNotFound)
		l.WarnContext(ctx, "Attempted to set non-existent profile as default")
		span.RecordError(err)
		span.SetStatus(codes.Error, "Profile not found")
		return err
	}

	// Commit the transaction
	err = tx.Commit(ctx)
	if err != nil {
		l.ErrorContext(ctx, "Failed to commit transaction", slog.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "DB transaction commit failed")
		return fmt.Errorf("database error committing transaction: %w", err)
	}

	l.InfoContext(ctx, "User preference profile set as default successfully")
	span.SetStatus(codes.Ok, "Profile set as default")
	return nil
}
