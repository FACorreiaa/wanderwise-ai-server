package auth

import (
	"context"
	"errors"

	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/crypto/bcrypt"

	"github.com/FACorreiaa/go-poi-au-suggestions/internal/types"
)

var _ AuthRepo = (*PostgresAuthRepo)(nil)

type AuthRepo interface {
	// GetUserByEmail fetches user details needed for validation/token generation.
	GetUserByEmail(ctx context.Context, email string) (*types.UserAuth, error)
	// GetUserByID fetches user details by ID.
	GetUserByID(ctx context.Context, userID string) (*types.UserAuth, error)
	// Register stores a new user with a HASHED password. Returns new user ID.
	Register(ctx context.Context, username, email, hashedPassword string) (string, error)
	// VerifyPassword checks if the given password matches the hash for the userID.
	VerifyPassword(ctx context.Context, userID, password string) error // Password is plain text here
	// UpdatePassword updates the user's HASHED password.
	UpdatePassword(ctx context.Context, userID, newHashedPassword string) error

	// provider specific methods for user management
	CreateUser(ctx context.Context, user *types.UserAuth) error
	CreateUserProvider(ctx context.Context, userID, provider, providerUserID string) error
	GetUserIDByProvider(ctx context.Context, provider, providerUserID string) (string, error)

	// --- Refresh Token Handling ---
	// StoreRefreshToken saves a new refresh token for a user.
	StoreRefreshToken(ctx context.Context, userID, token string, expiresAt time.Time) error
	// ValidateRefreshTokenAndGetUserID checks if a refresh token is valid and returns the user ID.
	ValidateRefreshTokenAndGetUserID(ctx context.Context, refreshToken string) (userID string, err error)
	// InvalidateRefreshToken marks a specific refresh token as revoked.
	InvalidateRefreshToken(ctx context.Context, refreshToken string) error
	// InvalidateAllUserRefreshTokens marks all tokens for a user as revoked.
	InvalidateAllUserRefreshTokens(ctx context.Context, userID string) error
}

type PostgresAuthRepo struct {
	logger *slog.Logger
	pgpool *pgxpool.Pool
}

func NewPostgresAuthRepo(pgxpool *pgxpool.Pool, logger *slog.Logger) *PostgresAuthRepo {
	return &PostgresAuthRepo{
		logger: logger,
		pgpool: pgxpool,
	}
}

// GetUserByEmail implements auth.AuthRepo.
func (r *PostgresAuthRepo) GetUserByEmail(ctx context.Context, email string) (*types.UserAuth, error) {
	var user types.UserAuth
	query := `SELECT id, username, email, password_hash FROM users WHERE email = $1 AND is_active = TRUE`
	err := r.pgpool.QueryRow(ctx, query, email).Scan(&user.ID, &user.Username, &user.Email, &user.Password)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("user with email %s not found: %w", email, types.ErrNotFound) // Use a domain error
		}
		r.logger.ErrorContext(ctx, "Error fetching user by email", slog.Any("error", err), slog.String("email", email))
		return nil, fmt.Errorf("database error fetching user: %w", err)
	}
	return &user, nil
}

// GetUserByID implements auth.AuthRepo.
func (r *PostgresAuthRepo) GetUserByID(ctx context.Context, userID string) (*types.UserAuth, error) {
	var user types.UserAuth
	// Select fields needed by token generation or other logic
	query := `SELECT id, username, email FROM users WHERE id = $1 AND is_active = TRUE`
	err := r.pgpool.QueryRow(ctx, query, userID).Scan(&user.ID, &user.Username, &user.Email)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("user with ID %s not found: %w", userID, types.ErrNotFound) // Use a domain error
		}
		r.logger.ErrorContext(ctx, "Error fetching user by ID", slog.Any("error", err), slog.String("userID", userID))
		return nil, fmt.Errorf("database error fetching user by ID: %w", err)
	}
	return &user, nil
}

// Register implements auth.AuthRepo. Expects HASHED password.
func (r *PostgresAuthRepo) Register(ctx context.Context, username, email, hashedPassword string) (string, error) {
	tracer := otel.Tracer("MyRESTAPI")

	// Start a span for the repository layer
	ctx, span := tracer.Start(ctx, "PostgresAuthRepo.Register", trace.WithAttributes(
		semconv.DBSystemPostgreSQL,
		attribute.String("db.system", "postgresql"),
		attribute.String("db.statement", "INSERT INTO users ..."),
	))
	defer span.End()

	// Record query start time
	//startTime := time.Now()

	var userID string
	query := `INSERT INTO users (username, email, password_hash, created_at) VALUES ($1, $2, $3, $4) RETURNING id`
	err := r.pgpool.QueryRow(ctx, query, username, email, hashedPassword, time.Now()).Scan(&userID)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Database error")
		//dbQueryErrorsTotal.Add(ctx, 1)
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return "", fmt.Errorf("email or username already exists: %w", types.ErrConflict)
		}
		r.logger.ErrorContext(ctx, "Error inserting user", slog.Any("error", err), slog.String("email", email))
		return "", fmt.Errorf("database error registering user: %w", err)
	}

	// Record query duration
	//duration := time.Since(startTime).Seconds()
	//dbQueryDurationSeconds.Record(ctx, duration)

	span.SetStatus(codes.Ok, "User inserted into database")
	return userID, nil
}

// VerifyPassword implements auth.AuthRepo. Compares plain password to stored hash.
func (r *PostgresAuthRepo) VerifyPassword(ctx context.Context, userID, password string) error {
	var storedHash string
	query := `SELECT password_hash FROM users WHERE id = $1 AND is_active = TRUE`
	err := r.pgpool.QueryRow(ctx, query, userID).Scan(&storedHash)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("user not found: %w", types.ErrNotFound)
		}
		r.logger.ErrorContext(ctx, "Error fetching password hash for verification", slog.Any("error", err), slog.String("userID", userID))
		return fmt.Errorf("database error verifying password: %w", err)
	}

	err = bcrypt.CompareHashAndPassword([]byte(storedHash), []byte(password))
	if err != nil {
		// Don't log the actual password, but log the failure type
		l := r.logger.With(slog.String("userID", userID))
		if errors.Is(err, bcrypt.ErrMismatchedHashAndPassword) {
			l.WarnContext(ctx, "Password mismatch during verification")
			return fmt.Errorf("invalid password: %w", types.ErrUnauthenticated) // Specific error
		}
		l.ErrorContext(ctx, "Error comparing password hash", slog.Any("error", err))
		return fmt.Errorf("error during password comparison: %w", err)
	}
	return nil
}

// UpdatePassword implements auth.AuthRepo. Expects HASHED password.
func (r *PostgresAuthRepo) UpdatePassword(ctx context.Context, userID, newHashedPassword string) error {
	query := `UPDATE users SET password_hash = $1, updated_at = NOW() WHERE id = $2 AND is_active = TRUE`
	tag, err := r.pgpool.Exec(ctx, query, newHashedPassword, userID)
	if err != nil {
		r.logger.ErrorContext(ctx, "Error updating password hash", slog.Any("error", err), slog.String("userID", userID))
		return fmt.Errorf("database error updating password: %w", err)
	}
	if tag.RowsAffected() == 0 {
		r.logger.WarnContext(ctx, "User not found or no password change needed during update", slog.String("userID", userID))
		return fmt.Errorf("user not found or password unchanged: %w", types.ErrNotFound) // Or a different domain error
	}
	return nil
}

// StoreRefreshToken implements auth.AuthRepo.
func (r *PostgresAuthRepo) StoreRefreshToken(ctx context.Context, userID, token string, expiresAt time.Time) error {
	query := `INSERT INTO refresh_tokens (user_id, token, expires_at) VALUES ($1, $2, $3)`
	_, err := r.pgpool.Exec(ctx, query, userID, token, expiresAt)
	if err != nil {
		r.logger.ErrorContext(ctx, "Error storing refresh token", slog.Any("error", err), slog.String("userID", userID))
		return fmt.Errorf("database error storing refresh token: %w", err)
	}
	return nil
}

// ValidateRefreshTokenAndGetUserID implements auth.AuthRepo.
func (r *PostgresAuthRepo) ValidateRefreshTokenAndGetUserID(ctx context.Context, refreshToken string) (string, error) {
	var userID string
	var expiresAt time.Time
	var revokedAt *time.Time // Use pointer for nullable timestamp

	query := `SELECT user_id, expires_at, revoked_at FROM refresh_tokens WHERE token = $1`
	err := r.pgpool.QueryRow(ctx, query, refreshToken).Scan(&userID, &expiresAt, &revokedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", fmt.Errorf("refresh token not found: %w", types.ErrUnauthenticated)
		}
		r.logger.ErrorContext(ctx, "Error querying refresh token", slog.Any("error", err))
		return "", fmt.Errorf("database error validating refresh token: %w", err)
	}

	if revokedAt != nil {
		return "", fmt.Errorf("refresh token has been revoked: %w", types.ErrUnauthenticated)
	}
	if time.Now().After(expiresAt) {
		return "", fmt.Errorf("refresh token has expired: %w", types.ErrUnauthenticated)
	}

	return userID, nil // Return only the userID
}

// InvalidateRefreshToken implements auth.AuthRepo.
func (r *PostgresAuthRepo) InvalidateRefreshToken(ctx context.Context, refreshToken string) error {
	query := `UPDATE refresh_tokens SET revoked_at = NOW() WHERE token = $1 AND revoked_at IS NULL`
	tag, err := r.pgpool.Exec(ctx, query, refreshToken)
	if err != nil {
		r.logger.ErrorContext(ctx, "Error invalidating refresh token", slog.Any("error", err))
		return fmt.Errorf("database error invalidating token: %w", err)
	}
	if tag.RowsAffected() == 0 {
		r.logger.WarnContext(ctx, "Refresh token not found or already invalidated during invalidation attempt")
		// Depending on context (e.g., logout), this might not be a critical error
		// return fmt.Errorf("token not found or already revoked: %w", ErrNotFound)
	}
	return nil
}

// InvalidateAllUserRefreshTokens implements auth.AuthRepo.
func (r *PostgresAuthRepo) InvalidateAllUserRefreshTokens(ctx context.Context, userID string) error {
	query := `UPDATE refresh_tokens SET revoked_at = NOW() WHERE user_id = $1 AND revoked_at IS NULL`
	_, err := r.pgpool.Exec(ctx, query, userID)
	if err != nil {
		r.logger.ErrorContext(ctx, "Error invalidating all refresh tokens for user", slog.Any("error", err), slog.String("userID", userID))
		return fmt.Errorf("database error invalidating tokens: %w", err)
	}
	// Log how many were invalidated? (tag.RowsAffected())
	return nil
}

// provider specific methods for user management
// GetUserIDByProvider retrieves the user ID associated with a provider and provider_user_id
func (r *PostgresAuthRepo) GetUserIDByProvider(ctx context.Context, provider, providerUserID string) (string, error) {
	ctx, span := otel.Tracer("UserRepository").Start(ctx, "GetUserIDByProvider",
		trace.WithAttributes(
			attribute.String("provider", provider),
			attribute.String("provider_user_id", providerUserID),
		))
	defer span.End()

	l := r.logger.With(slog.String("method", "GetUserIDByProvider"))

	var userID string
	query := `SELECT user_id FROM user_providers WHERE provider = $1 AND provider_user_id = $2`
	err := r.pgpool.QueryRow(ctx, query, provider, providerUserID).Scan(&userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			l.InfoContext(ctx, "No user found for provider", slog.String("provider", provider), slog.String("provider_user_id", providerUserID))
			span.SetStatus(codes.Ok, "No user found")
			return "", nil // No user found, not an error
		}
		l.ErrorContext(ctx, "Failed to query user by provider", slog.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Database query failed")
		return "", err
	}

	l.InfoContext(ctx, "User found for provider", slog.String("user_id", userID))
	span.SetStatus(codes.Ok, "User found")
	return userID, nil
}

// CreateUser creates a new user in the database
func (r *PostgresAuthRepo) CreateUser(ctx context.Context, user *types.UserAuth) error {
	ctx, span := otel.Tracer("UserRepository").Start(ctx, "CreateUser",
		trace.WithAttributes(
			attribute.String("email", user.Email),
			attribute.String("username", user.Username),
		))
	defer span.End()

	l := r.logger.With(slog.String("method", "CreateUser"))

	query := `
        INSERT INTO users (id, username, email, role, created_at)
        VALUES (gen_random_uuid(), $1, $2, $3, $4)
        RETURNING id, created_at
    `
	var id string
	var createdAt time.Time
	err := r.pgpool.QueryRow(ctx, query, user.Username, user.Email, user.Role, time.Now()).Scan(&id, &createdAt)
	if err != nil {
		if pgErr, ok := err.(*pgconn.PgError); ok && pgErr.Code == "23505" { // Unique violation (email)
			l.WarnContext(ctx, "Email already exists", slog.String("email", user.Email))
			span.RecordError(err)
			span.SetStatus(codes.Error, "Email conflict")
			return types.ErrConflict
		}
		l.ErrorContext(ctx, "Failed to create user", slog.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Database insert failed")
		return err
	}

	// Update user struct with returned values
	user.ID = id
	user.CreatedAt = createdAt

	l.InfoContext(ctx, "User created successfully", slog.String("user_id", user.ID))
	span.SetStatus(codes.Ok, "User created")
	return nil
}

// CreateUserProvider links a user to an OAuth provider
func (r *PostgresAuthRepo) CreateUserProvider(ctx context.Context, userID, provider, providerUserID string) error {
	ctx, span := otel.Tracer("UserRepository").Start(ctx, "CreateUserProvider",
		trace.WithAttributes(
			attribute.String("user_id", userID),
			attribute.String("provider", provider),
			attribute.String("provider_user_id", providerUserID),
		))
	defer span.End()

	l := r.logger.With(slog.String("method", "CreateUserProvider"))

	query := `
        INSERT INTO user_providers (user_id, provider, provider_user_id, created_at)
        VALUES ($1, $2, $3, $4)
    `
	_, err := r.pgpool.Exec(ctx, query, userID, provider, providerUserID, time.Now())
	if err != nil {
		if pgErr, ok := err.(*pgconn.PgError); ok && pgErr.Code == "23505" { // Unique violation (provider+provider_user_id)
			l.WarnContext(ctx, "Provider link already exists", slog.String("provider", provider), slog.String("provider_user_id", providerUserID))
			span.RecordError(err)
			span.SetStatus(codes.Error, "Provider link conflict")
			return types.ErrConflict
		}
		l.ErrorContext(ctx, "Failed to create provider link", slog.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Database insert failed")
		return err
	}

	l.InfoContext(ctx, "Provider linked successfully", slog.String("user_id", userID), slog.String("provider", provider))
	span.SetStatus(codes.Ok, "Provider linked")
	return nil
}
