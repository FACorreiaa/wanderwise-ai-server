package user

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/crypto/bcrypt"

	"github.com/FACorreiaa/go-poi-au-suggestions/internal/types"
)

var _ UserRepo = (*PostgresUserRepo)(nil)

// UserRepo defines the contract for user data persistence.
type UserRepo interface {
	// GetUserByID retrieves a user's full profile by their unique ID.
	// Returns types.ErrNotFound if the user doesn't exist or is inactive.
	GetUserByID(ctx context.Context, userID uuid.UUID) (*types.UserProfile, error)

	ChangePassword(ctx context.Context, email, oldPassword, newPassword string) error
	// UpdateProfile updates mutable fields on a user's profile.
	// It takes the userID and a struct containing only the fields to be updated (use pointers).
	// Returns types.ErrNotFound if the user doesn't exist.
	UpdateProfile(ctx context.Context, userID uuid.UUID, params types.UpdateProfileParams) error

	UpdateLastLogin(ctx context.Context, userID uuid.UUID) error

	// MarkEmailAsVerified sets the email_verified_at timestamp.
	MarkEmailAsVerified(ctx context.Context, userID uuid.UUID) error

	// DeactivateUser marks a user as inactive (soft delete).
	// This also invalidates all active sessions/tokens.
	DeactivateUser(ctx context.Context, userID uuid.UUID) error

	// ReactivateUser marks a user as active.
	ReactivateUser(ctx context.Context, userID uuid.UUID) error
}

type PostgresUserRepo struct {
	logger *slog.Logger
	pgpool *pgxpool.Pool
}

func NewPostgresUserRepo(pgxpool *pgxpool.Pool, logger *slog.Logger) *PostgresUserRepo {
	return &PostgresUserRepo{
		logger: logger,
		pgpool: pgxpool,
	}
}

func (r *PostgresUserRepo) ChangePassword(ctx context.Context, email, oldPassword, newPassword string) error {
	var userID, hashedPassword string
	err := r.pgpool.QueryRow(ctx,
		"SELECT id, password_hash FROM users WHERE email = $1",
		email).Scan(&userID, &hashedPassword)
	if err != nil {
		return fmt.Errorf("user not found: %w", err)
	}

	err = bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(oldPassword))
	if err != nil {
		return errors.New("invalid old password")
	}

	newHashedPassword, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("failed to hash new password: %w", err)
	}

	_, err = r.pgpool.Exec(ctx,
		"UPDATE users SET password_hash = $1, updated_at = $2 WHERE id = $3",
		string(newHashedPassword), time.Now(), userID)
	if err != nil {
		return fmt.Errorf("failed to update password: %w", err)
	}

	// Invalidate all refresh tokens
	_, err = r.pgpool.Exec(ctx,
		"UPDATE refresh_tokens SET revoked_at = $1 WHERE user_id = $2 AND revoked_at IS NULL",
		time.Now(), userID)
	if err != nil {
		fmt.Printf("Warning: failed to invalidate refresh tokens: %v\n", err)
	}

	return nil
}

func (r *PostgresUserRepo) GetUserByID(ctx context.Context, userID uuid.UUID) (*types.UserProfile, error) {
	var user types.UserProfile
	var interests, badges []string
	query := `
		SELECT id, username, firstname, lastname, phone, age, city, 
		       country, email, display_name, profile_image_url, 
		       email_verified_at, about_you, location, interests, badges,
		       places_visited, reviews_written, lists_created, followers, following,
		       is_active, last_login_at, theme, language, created_at, updated_at
		FROM users WHERE id = $1 AND is_active = TRUE
	`

	stats := &types.UserStats{}
	err := r.pgpool.QueryRow(ctx, query, userID).Scan(
		&user.ID,
		&user.Username,
		&user.Firstname,
		&user.Lastname,
		&user.PhoneNumber,
		&user.Age,
		&user.City,
		&user.Country,
		&user.Email,
		&user.DisplayName,
		&user.ProfileImageURL,
		&user.EmailVerifiedAt,
		&user.AboutYou,
		&user.Location,
		&interests,
		&badges,
		&stats.PlacesVisited,
		&stats.ReviewsWritten,
		&stats.ListsCreated,
		&stats.Followers,
		&stats.Following,
		&user.IsActive,
		&user.LastLoginAt,
		&user.Theme,
		&user.Language,
		&user.CreatedAt,
		&user.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("user not found: %w", err)
	}

	// Set additional fields for frontend compatibility
	user.Bio = user.AboutYou           // Map about_you to bio
	user.Avatar = user.ProfileImageURL // Map profile_image_url to avatar
	user.JoinedDate = user.CreatedAt   // Map created_at to joinedDate
	user.Interests = interests
	user.Badges = badges
	user.Stats = stats

	return &user, nil
}

func (r *PostgresUserRepo) UpdateProfile(ctx context.Context, userID uuid.UUID, params types.UpdateProfileParams) error {
	ctx, span := otel.Tracer("UserRepo").Start(ctx, "UpdateProfile", trace.WithAttributes(
		semconv.DBSystemPostgreSQL,
		attribute.String("db.operation", "UPDATE"),
		attribute.String("db.sql.table", "users"),
		attribute.String("db.user.id", userID.String()),
	))
	defer span.End()

	l := r.logger.With(slog.String("method", "UpdateProfile"), slog.String("userID", userID.String()))
	l.DebugContext(ctx, "Updating user profile", slog.Any("params", params)) // Log incoming params

	// Use squirrel or build query dynamically
	var setClauses []string
	var args []interface{}
	argID := 1 // Argument counter for placeholders ($1, $2, ...)

	// Check each field in params. If not nil, add to SET clause and args slice.
	if params.Username != nil {
		setClauses = append(setClauses, fmt.Sprintf("username = $%d", argID))
		args = append(args, *params.Username)
		argID++
		span.SetAttributes(attribute.Bool("update.username", true)) // Add trace attribute
	}
	if params.Email != nil {
		setClauses = append(setClauses, fmt.Sprintf("email = $%d", argID))
		args = append(args, *params.Email)
		argID++
		span.SetAttributes(attribute.Bool("update.email", true))
	}
	if params.DisplayName != nil {
		setClauses = append(setClauses, fmt.Sprintf("display_name = $%d", argID))
		args = append(args, *params.DisplayName)
		argID++
		span.SetAttributes(attribute.Bool("update.display_name", true))
	}
	if params.ProfileImageURL != nil {
		setClauses = append(setClauses, fmt.Sprintf("profile_image_url = $%d", argID))
		args = append(args, *params.ProfileImageURL)
		argID++
		span.SetAttributes(attribute.Bool("update.profile_image_url", true))
	}
	if params.Firstname != nil {
		setClauses = append(setClauses, fmt.Sprintf("firstname = $%d", argID))
		args = append(args, *params.Firstname)
		argID++
		span.SetAttributes(attribute.Bool("update.firstname", true))
	}
	if params.Lastname != nil {
		setClauses = append(setClauses, fmt.Sprintf("lastname = $%d", argID))
		args = append(args, *params.Lastname)
		argID++
		span.SetAttributes(attribute.Bool("update.lastname", true))
	}
	if params.Age != nil {
		setClauses = append(setClauses, fmt.Sprintf("age = $%d", argID))
		args = append(args, *params.Age)
		argID++
		span.SetAttributes(attribute.Bool("update.age", true))
	}
	if params.City != nil {
		setClauses = append(setClauses, fmt.Sprintf("city = $%d", argID))
		args = append(args, *params.City)
		argID++
		span.SetAttributes(attribute.Bool("update.city", true))
	}
	if params.Country != nil {
		setClauses = append(setClauses, fmt.Sprintf("country = $%d", argID))
		args = append(args, *params.Country)
		argID++
		span.SetAttributes(attribute.Bool("update.country", true))
	}
	if params.AboutYou != nil {
		setClauses = append(setClauses, fmt.Sprintf("about_you = $%d", argID))
		args = append(args, *params.AboutYou)
		argID++
		span.SetAttributes(attribute.Bool("update.about_you", true))
	}
	if params.Location != nil {
		setClauses = append(setClauses, fmt.Sprintf("location = $%d", argID))
		args = append(args, *params.Location)
		argID++
		span.SetAttributes(attribute.Bool("update.location", true))
	}
	if params.Interests != nil {
		setClauses = append(setClauses, fmt.Sprintf("interests = $%d", argID))
		args = append(args, *params.Interests)
		argID++
		span.SetAttributes(attribute.Bool("update.interests", true))
	}
	if params.PhoneNumber != nil {
		setClauses = append(setClauses, fmt.Sprintf("phone = $%d", argID))
		args = append(args, *params.PhoneNumber)
		argID++
		span.SetAttributes(attribute.Bool("update.phone", true))
	}
	if params.Badges != nil {
		setClauses = append(setClauses, fmt.Sprintf("badges = $%d", argID))
		args = append(args, *params.Badges)
		argID++
		span.SetAttributes(attribute.Bool("update.badges", true))
	}

	// If no fields were provided to update, return early (or error?)
	if len(setClauses) == 0 {
		l.WarnContext(ctx, "UpdateProfile called with no fields to update")
		span.SetStatus(codes.Ok, "No update fields provided") // Not an error, just no-op
		return nil                                            // Or return specific error types.ErrBadRequest("no update fields provided")
	}

	// Add updated_at clause (always update this if other fields change)
	setClauses = append(setClauses, fmt.Sprintf("updated_at = $%d", argID))
	args = append(args, time.Now())
	argID++

	// Final WHERE clause argument
	args = append(args, userID)

	// Construct the final query
	query := fmt.Sprintf("UPDATE users SET %s WHERE id = $%d AND is_active = TRUE",
		strings.Join(setClauses, ", "), // e.g., "username = $1, age = $2, updated_at = $3"
		argID,                          // The placeholder for userID
	)

	l.DebugContext(ctx, "Executing dynamic update query", slog.String("query", query), slog.Int("arg_count", len(args)))

	// Execute the dynamic query
	tag, err := r.pgpool.Exec(ctx, query, args...)
	if err != nil {
		// Add specific error checking (e.g., unique constraint violations on email/username if updated)
		l.ErrorContext(ctx, "Failed to execute update profile query", slog.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "DB UPDATE failed")
		return fmt.Errorf("database error updating profile: %w", err)
	}

	// Check if the user existed and was updated
	if tag.RowsAffected() == 0 {
		l.WarnContext(ctx, "User not found or no update occurred", slog.Int64("rows_affected", tag.RowsAffected()))
		span.SetStatus(codes.Error, "User not found or no change")
		// Check if user exists to differentiate "not found" vs "no effective change"
		var exists bool
		// Use a separate query or modify the UPDATE to return something on match
		checkErr := r.pgpool.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM users WHERE id = $1 AND is_active = TRUE)", userID).Scan(&exists)
		if checkErr == nil && !exists {
			return fmt.Errorf("user not found for update: %w", types.ErrNotFound)
		}
		// If user exists, maybe the provided values were the same as existing ones.
		// Or maybe user was inactive. Treat as not found for simplicity for now.
		return fmt.Errorf("user not found or update failed: %w", types.ErrNotFound)
	}

	l.InfoContext(ctx, "User profile updated successfully")
	span.SetStatus(codes.Ok, "Profile updated")
	return nil
}

// UpdateLastLogin implements user.UserRepo.
func (r *PostgresUserRepo) UpdateLastLogin(ctx context.Context, userID uuid.UUID) error {
	ctx, span := otel.Tracer("UserRepo").Start(ctx, "UpdateLastLogin", trace.WithAttributes(
		semconv.DBSystemPostgreSQL,
		attribute.String("db.operation", "UPDATE"),
		attribute.String("db.sql.table", "users"),
		attribute.String("db.user.id", userID.String()),
	))
	defer span.End()

	l := r.logger.With(slog.String("method", "UpdateLastLogin"), slog.String("userID", userID.String()))
	l.DebugContext(ctx, "Updating user last login timestamp")

	query := `
        UPDATE users 
        SET last_login_at = $1, updated_at = $1
        WHERE id = $2`

	now := time.Now()
	tag, err := r.pgpool.Exec(ctx, query, now, userID)
	if err != nil {
		l.ErrorContext(ctx, "Failed to update user last login timestamp", slog.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "DB UPDATE failed")
		return fmt.Errorf("database error updating last login: %w", err)
	}

	if tag.RowsAffected() == 0 {
		err := fmt.Errorf("user not found: %w", types.ErrNotFound)
		l.WarnContext(ctx, "Attempted to update last login for non-existent user")
		span.RecordError(err)
		span.SetStatus(codes.Error, "User not found")
		return err
	}

	l.InfoContext(ctx, "User last login timestamp updated successfully")
	span.SetStatus(codes.Ok, "Last login updated")
	return nil
}

// MarkEmailAsVerified implements user.UserRepo.
func (r *PostgresUserRepo) MarkEmailAsVerified(ctx context.Context, userID uuid.UUID) error {
	ctx, span := otel.Tracer("UserRepo").Start(ctx, "MarkEmailAsVerified", trace.WithAttributes(
		semconv.DBSystemPostgreSQL,
		attribute.String("db.operation", "UPDATE"),
		attribute.String("db.sql.table", "users"),
		attribute.String("db.user.id", userID.String()),
	))
	defer span.End()

	l := r.logger.With(slog.String("method", "MarkEmailAsVerified"), slog.String("userID", userID.String()))
	l.DebugContext(ctx, "Marking user email as verified")

	query := `
        UPDATE users 
        SET email_verified_at = $1, updated_at = $1
        WHERE id = $2 AND email_verified_at IS NULL`

	now := time.Now()
	tag, err := r.pgpool.Exec(ctx, query, now, userID)
	if err != nil {
		l.ErrorContext(ctx, "Failed to mark user email as verified", slog.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "DB UPDATE failed")
		return fmt.Errorf("database error marking email as verified: %w", err)
	}

	if tag.RowsAffected() == 0 {
		// Check if the user exists
		var exists bool
		err := r.pgpool.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM users WHERE id = $1)", userID).Scan(&exists)
		if err != nil {
			l.ErrorContext(ctx, "Failed to check if user exists", slog.Any("error", err))
			span.RecordError(err)
			span.SetStatus(codes.Error, "DB query failed")
			return fmt.Errorf("database error checking user existence: %w", err)
		}

		if !exists {
			err := fmt.Errorf("user not found: %w", types.ErrNotFound)
			l.WarnContext(ctx, "Attempted to mark email as verified for non-existent user")
			span.RecordError(err)
			span.SetStatus(codes.Error, "User not found")
			return err
		}

		// User exists but email is already verified
		l.InfoContext(ctx, "User email already verified")
		span.SetStatus(codes.Ok, "Email already verified")
		return nil
	}

	l.InfoContext(ctx, "User email marked as verified successfully")
	span.SetStatus(codes.Ok, "Email verified")
	return nil
}

// DeactivateUser implements user.UserRepo.
func (r *PostgresUserRepo) DeactivateUser(ctx context.Context, userID uuid.UUID) error {
	ctx, span := otel.Tracer("UserRepo").Start(ctx, "DeactivateUser", trace.WithAttributes(
		semconv.DBSystemPostgreSQL,
		attribute.String("db.operation", "UPDATE"),
		attribute.String("db.sql.table", "users"),
		attribute.String("db.user.id", userID.String()),
	))
	defer span.End()

	l := r.logger.With(slog.String("method", "DeactivateUser"), slog.String("userID", userID.String()))
	l.DebugContext(ctx, "Deactivating user")

	// Begin a transaction
	tx, err := r.pgpool.Begin(ctx)
	if err != nil {
		l.ErrorContext(ctx, "Failed to begin transaction", slog.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "DB transaction failed")
		return fmt.Errorf("database error beginning transaction: %w", err)
	}

	// First, check if the user exists and is active
	var isActive bool
	err = tx.QueryRow(ctx, "SELECT is_active FROM users WHERE id = $1", userID).Scan(&isActive)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			_ = tx.Rollback(ctx)
			l.WarnContext(ctx, "Attempted to deactivate non-existent user")
			span.RecordError(err)
			span.SetStatus(codes.Error, "User not found")
			return fmt.Errorf("user not found: %w", types.ErrNotFound)
		}
		_ = tx.Rollback(ctx)
		l.ErrorContext(ctx, "Failed to check user active status", slog.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "DB query failed")
		return fmt.Errorf("database error checking user status: %w", err)
	}

	if !isActive {
		_ = tx.Rollback(ctx)
		l.InfoContext(ctx, "User is already inactive")
		span.SetStatus(codes.Ok, "User already inactive")
		return nil
	}

	// Deactivate the user
	_, err = tx.Exec(ctx, "UPDATE users SET is_active = FALSE, updated_at = $1 WHERE id = $2", time.Now(), userID)
	if err != nil {
		_ = tx.Rollback(ctx)
		l.ErrorContext(ctx, "Failed to deactivate user", slog.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "DB UPDATE failed")
		return fmt.Errorf("database error deactivating user: %w", err)
	}

	// Invalidate all refresh tokens
	_, err = tx.Exec(ctx, "UPDATE refresh_tokens SET revoked_at = $1 WHERE user_id = $2 AND revoked_at IS NULL", time.Now(), userID)
	if err != nil {
		_ = tx.Rollback(ctx)
		l.ErrorContext(ctx, "Failed to invalidate refresh tokens", slog.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "DB UPDATE failed")
		return fmt.Errorf("database error invalidating refresh tokens: %w", err)
	}

	// Invalidate all sessions
	_, err = tx.Exec(ctx, "UPDATE sessions SET invalidated_at = $1 WHERE user_id = $2 AND invalidated_at IS NULL", time.Now(), userID)
	if err != nil {
		_ = tx.Rollback(ctx)
		l.ErrorContext(ctx, "Failed to invalidate sessions", slog.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "DB UPDATE failed")
		return fmt.Errorf("database error invalidating sessions: %w", err)
	}

	// Commit the transaction
	err = tx.Commit(ctx)
	if err != nil {
		l.ErrorContext(ctx, "Failed to commit transaction", slog.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "DB transaction commit failed")
		return fmt.Errorf("database error committing transaction: %w", err)
	}

	l.InfoContext(ctx, "User deactivated successfully")
	span.SetStatus(codes.Ok, "User deactivated")
	return nil
}

// ReactivateUser implements user.UserRepo.
func (r *PostgresUserRepo) ReactivateUser(ctx context.Context, userID uuid.UUID) error {
	ctx, span := otel.Tracer("UserRepo").Start(ctx, "ReactivateUser", trace.WithAttributes(
		semconv.DBSystemPostgreSQL,
		attribute.String("db.operation", "UPDATE"),
		attribute.String("db.sql.table", "users"),
		attribute.String("db.user.id", userID.String()),
	))
	defer span.End()

	l := r.logger.With(slog.String("method", "ReactivateUser"), slog.String("userID", userID.String()))
	l.DebugContext(ctx, "Reactivating user")

	// Check if the user exists and is inactive
	var isActive bool
	err := r.pgpool.QueryRow(ctx, "SELECT is_active FROM users WHERE id = $1", userID).Scan(&isActive)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			l.WarnContext(ctx, "Attempted to reactivate non-existent user")
			span.RecordError(err)
			span.SetStatus(codes.Error, "User not found")
			return fmt.Errorf("user not found: %w", types.ErrNotFound)
		}
		l.ErrorContext(ctx, "Failed to check user active status", slog.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "DB query failed")
		return fmt.Errorf("database error checking user status: %w", err)
	}

	if isActive {
		l.InfoContext(ctx, "User is already active")
		span.SetStatus(codes.Ok, "User already active")
		return nil
	}

	// Reactivate the user
	_, err = r.pgpool.Exec(ctx, "UPDATE users SET is_active = TRUE, updated_at = $1 WHERE id = $2", time.Now(), userID)
	if err != nil {
		l.ErrorContext(ctx, "Failed to reactivate user", slog.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "DB UPDATE failed")
		return fmt.Errorf("database error reactivating user: %w", err)
	}

	l.InfoContext(ctx, "User reactivated successfully")
	span.SetStatus(codes.Ok, "User reactivated")
	return nil
}
