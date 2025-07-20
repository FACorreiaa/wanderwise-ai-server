//go:build integration

package auth

import (
	"context"
	"log"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/FACorreiaa/go-poi-au-suggestions/config"
	"github.com/FACorreiaa/go-poi-au-suggestions/internal/types"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var testAuthDB *pgxpool.Pool
var testAuthService AuthService
var testAuthRepo AuthRepo

func TestMain(m *testing.M) {
	if err := godotenv.Load("../../../.env.test"); err != nil {
		log.Println("Warning: .env.test file not found for auth integration tests.")
	}

	dbURL := os.Getenv("TEST_DATABASE_URL")
	if dbURL == "" {
		log.Fatal("TEST_DATABASE_URL environment variable is not set for auth integration tests")
	}

	var err error
	config, err := pgxpool.ParseConfig(dbURL)
	if err != nil {
		log.Fatalf("Unable to parse TEST_DATABASE_URL: %v\n", err)
	}
	config.MaxConns = 5

	testAuthDB, err = pgxpool.NewWithConfig(context.Background(), config)
	if err != nil {
		log.Fatalf("Unable to create connection pool for auth tests: %v\n", err)
	}
	defer testAuthDB.Close()

	if err := testAuthDB.Ping(context.Background()); err != nil {
		log.Fatalf("Unable to ping test database for auth tests: %v\n", err)
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	testAuthRepo = NewPostgresAuthRepo(testAuthDB, logger)
	
	cfg := &config.Config{
		JWT: config.JWTConfig{
			SecretKey:       "test-secret-key-for-integration-tests",
			AccessTokenTTL:  15 * time.Minute,
			RefreshTokenTTL: 7 * 24 * time.Hour,
			Issuer:          "test-issuer",
			Audience:        "test-audience",
		},
	}
	
	testAuthService = NewAuthService(testAuthRepo, cfg, logger)

	exitCode := m.Run()
	os.Exit(exitCode)
}

func clearAuthTables(t *testing.T) {
	t.Helper()
	_, err := testAuthDB.Exec(context.Background(), "DELETE FROM refresh_tokens")
	require.NoError(t, err, "Failed to clear refresh_tokens table")
	_, err = testAuthDB.Exec(context.Background(), "DELETE FROM user_providers")
	require.NoError(t, err, "Failed to clear user_providers table")
	_, err = testAuthDB.Exec(context.Background(), "DELETE FROM users WHERE email LIKE '%@authtest.com'")
	require.NoError(t, err, "Failed to clear test users")
}

func TestAuthServiceImpl_Register_Integration(t *testing.T) {
	ctx := context.Background()
	clearAuthTables(t)

	t.Run("Register new user successfully", func(t *testing.T) {
		username := "testuser"
		email := "testuser@authtest.com"
		password := "password123"
		role := "user"

		err := testAuthService.Register(ctx, username, email, password, role)
		require.NoError(t, err)

		// Verify user was created in database
		var dbUsername, dbEmail string
		err = testAuthDB.QueryRow(ctx, "SELECT username, email FROM users WHERE email = $1", email).Scan(&dbUsername, &dbEmail)
		require.NoError(t, err)
		assert.Equal(t, username, dbUsername)
		assert.Equal(t, email, dbEmail)
	})

	t.Run("Register user with duplicate email", func(t *testing.T) {
		username := "testuser2"
		email := "duplicate@authtest.com"
		password := "password123"
		role := "user"

		// First registration
		err := testAuthService.Register(ctx, username, email, password, role)
		require.NoError(t, err)

		// Attempt duplicate registration
		err = testAuthService.Register(ctx, "differentuser", email, password, role)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "registration failed")
	})
}

func TestAuthServiceImpl_Login_Integration(t *testing.T) {
	ctx := context.Background()
	clearAuthTables(t)

	// Setup test user
	username := "loginuser"
	email := "loginuser@authtest.com"
	password := "password123"
	role := "user"

	err := testAuthService.Register(ctx, username, email, password, role)
	require.NoError(t, err)

	t.Run("Login with valid credentials", func(t *testing.T) {
		accessToken, refreshToken, err := testAuthService.Login(ctx, email, password)
		require.NoError(t, err)
		assert.NotEmpty(t, accessToken)
		assert.NotEmpty(t, refreshToken)

		// Verify refresh token was stored
		var storedToken string
		err = testAuthDB.QueryRow(ctx, "SELECT token FROM refresh_tokens WHERE token = $1", refreshToken).Scan(&storedToken)
		require.NoError(t, err)
		assert.Equal(t, refreshToken, storedToken)
	})

	t.Run("Login with invalid email", func(t *testing.T) {
		_, _, err := testAuthService.Login(ctx, "nonexistent@authtest.com", password)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid credentials")
	})

	t.Run("Login with invalid password", func(t *testing.T) {
		_, _, err := testAuthService.Login(ctx, email, "wrongpassword")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid credentials")
	})
}

func TestAuthServiceImpl_RefreshSession_Integration(t *testing.T) {
	ctx := context.Background()
	clearAuthTables(t)

	// Setup test user and login to get refresh token
	username := "refreshuser"
	email := "refreshuser@authtest.com"
	password := "password123"
	role := "user"

	err := testAuthService.Register(ctx, username, email, password, role)
	require.NoError(t, err)

	_, refreshToken, err := testAuthService.Login(ctx, email, password)
	require.NoError(t, err)

	t.Run("Refresh with valid token", func(t *testing.T) {
		newAccessToken, newRefreshToken, err := testAuthService.RefreshSession(ctx, refreshToken)
		require.NoError(t, err)
		assert.NotEmpty(t, newAccessToken)
		assert.NotEmpty(t, newRefreshToken)
		assert.NotEqual(t, refreshToken, newRefreshToken)

		// Verify old token was invalidated
		var count int
		err = testAuthDB.QueryRow(ctx, "SELECT COUNT(*) FROM refresh_tokens WHERE token = $1 AND is_active = true", refreshToken).Scan(&count)
		require.NoError(t, err)
		assert.Equal(t, 0, count)

		// Verify new token exists
		err = testAuthDB.QueryRow(ctx, "SELECT COUNT(*) FROM refresh_tokens WHERE token = $1 AND is_active = true", newRefreshToken).Scan(&count)
		require.NoError(t, err)
		assert.Equal(t, 1, count)
	})

	t.Run("Refresh with invalid token", func(t *testing.T) {
		invalidToken := uuid.NewString()
		_, _, err := testAuthService.RefreshSession(ctx, invalidToken)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid or expired refresh token")
	})
}

func TestAuthServiceImpl_Logout_Integration(t *testing.T) {
	ctx := context.Background()
	clearAuthTables(t)

	// Setup test user and login
	username := "logoutuser"
	email := "logoutuser@authtest.com"
	password := "password123"
	role := "user"

	err := testAuthService.Register(ctx, username, email, password, role)
	require.NoError(t, err)

	_, refreshToken, err := testAuthService.Login(ctx, email, password)
	require.NoError(t, err)

	t.Run("Logout with valid token", func(t *testing.T) {
		err := testAuthService.Logout(ctx, refreshToken)
		require.NoError(t, err)

		// Verify token was invalidated
		var isActive bool
		err = testAuthDB.QueryRow(ctx, "SELECT is_active FROM refresh_tokens WHERE token = $1", refreshToken).Scan(&isActive)
		require.NoError(t, err)
		assert.False(t, isActive)
	})

	t.Run("Logout with invalid token", func(t *testing.T) {
		invalidToken := uuid.NewString()
		err := testAuthService.Logout(ctx, invalidToken)
		// Logout should succeed even with invalid token (idempotent)
		require.NoError(t, err)
	})
}

func TestAuthServiceImpl_UpdatePassword_Integration(t *testing.T) {
	ctx := context.Background()
	clearAuthTables(t)

	// Setup test user
	username := "passworduser"
	email := "passworduser@authtest.com"
	password := "oldpassword123"
	role := "user"

	err := testAuthService.Register(ctx, username, email, password, role)
	require.NoError(t, err)

	// Get user ID
	user, err := testAuthRepo.GetUserByEmail(ctx, email)
	require.NoError(t, err)
	userID := user.ID

	// Login to create refresh tokens
	_, refreshToken, err := testAuthService.Login(ctx, email, password)
	require.NoError(t, err)

	t.Run("Update password with valid old password", func(t *testing.T) {
		newPassword := "newpassword456"
		err := testAuthService.UpdatePassword(ctx, userID, password, newPassword)
		require.NoError(t, err)

		// Verify old refresh tokens were invalidated
		var count int
		err = testAuthDB.QueryRow(ctx, "SELECT COUNT(*) FROM refresh_tokens WHERE user_id = $1 AND is_active = true", userID).Scan(&count)
		require.NoError(t, err)
		assert.Equal(t, 0, count)

		// Verify can login with new password
		_, _, err = testAuthService.Login(ctx, email, newPassword)
		require.NoError(t, err)

		// Verify cannot login with old password
		_, _, err = testAuthService.Login(ctx, email, password)
		require.Error(t, err)
	})

	t.Run("Update password with invalid old password", func(t *testing.T) {
		err := testAuthService.UpdatePassword(ctx, userID, "wrongpassword", "newpassword789")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "incorrect old password")
	})
}

func TestAuthServiceImpl_GetUserByID_Integration(t *testing.T) {
	ctx := context.Background()
	clearAuthTables(t)

	// Setup test user
	username := "getusertest"
	email := "getusertest@authtest.com"
	password := "password123"
	role := "user"

	err := testAuthService.Register(ctx, username, email, password, role)
	require.NoError(t, err)

	// Get user ID
	user, err := testAuthRepo.GetUserByEmail(ctx, email)
	require.NoError(t, err)
	userID := user.ID

	t.Run("Get existing user", func(t *testing.T) {
		retrievedUser, err := testAuthService.GetUserByID(ctx, userID)
		require.NoError(t, err)
		require.NotNil(t, retrievedUser)
		assert.Equal(t, userID, retrievedUser.ID)
		assert.Equal(t, username, retrievedUser.Username)
		assert.Equal(t, email, retrievedUser.Email)
	})

	t.Run("Get non-existent user", func(t *testing.T) {
		nonExistentID := uuid.NewString()
		_, err := testAuthService.GetUserByID(ctx, nonExistentID)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to fetch user")
	})
}

func TestAuthServiceImpl_InvalidateAllUserRefreshTokens_Integration(t *testing.T) {
	ctx := context.Background()
	clearAuthTables(t)

	// Setup test user
	username := "invalidateuser"
	email := "invalidateuser@authtest.com"
	password := "password123"
	role := "user"

	err := testAuthService.Register(ctx, username, email, password, role)
	require.NoError(t, err)

	// Get user ID
	user, err := testAuthRepo.GetUserByEmail(ctx, email)
	require.NoError(t, err)
	userID := user.ID

	// Create multiple refresh tokens
	_, refreshToken1, err := testAuthService.Login(ctx, email, password)
	require.NoError(t, err)
	_, refreshToken2, err := testAuthService.Login(ctx, email, password)
	require.NoError(t, err)

	t.Run("Invalidate all user refresh tokens", func(t *testing.T) {
		err := testAuthService.InvalidateAllUserRefreshTokens(ctx, userID)
		require.NoError(t, err)

		// Verify all tokens were invalidated
		var count int
		err = testAuthDB.QueryRow(ctx, "SELECT COUNT(*) FROM refresh_tokens WHERE user_id = $1 AND is_active = true", userID).Scan(&count)
		require.NoError(t, err)
		assert.Equal(t, 0, count)

		// Verify tokens cannot be used for refresh
		_, _, err = testAuthService.RefreshSession(ctx, refreshToken1)
		require.Error(t, err)
		_, _, err = testAuthService.RefreshSession(ctx, refreshToken2)
		require.Error(t, err)
	})
}