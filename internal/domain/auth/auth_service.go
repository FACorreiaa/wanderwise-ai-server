// internal/auth/service.go
package auth

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/markbates/goth"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/crypto/bcrypt"

	"github.com/FACorreiaa/go-poi-au-suggestions/config"
	"github.com/FACorreiaa/go-poi-au-suggestions/internal/types"
)

// Ensure implementation satisfies the interface
var _ AuthService = (*AuthServiceImpl)(nil)

// AuthService defines the business logic contract.
type AuthService interface {
	Login(ctx context.Context, email, password string) (accessToken string, refreshToken string, err error)
	Logout(ctx context.Context, refreshToken string) error
	RefreshSession(ctx context.Context, refreshToken string) (accessToken string, newRefreshToken string, err error)
	Register(ctx context.Context, username, email, password, role string) error
	UpdatePassword(ctx context.Context, userID, oldPassword, newPassword string) error
	InvalidateAllUserRefreshTokens(ctx context.Context, userID string) error
	ValidateRefreshToken(ctx context.Context, refreshToken string) (string, error)
	GetUserByID(ctx context.Context, userID string) (*types.UserAuth, error)
	VerifyPassword(ctx context.Context, userID, password string) error
	GenerateTokens(ctx context.Context, user *types.UserAuth, sub *types.Subscription) (accessToken string, refreshToken string, err error)
	GetOrCreateUserFromProvider(ctx context.Context, provider string, providerUser goth.User) (*types.UserAuth, error)
}

// AuthServiceImpl provides the implementation for AuthService.
type AuthServiceImpl struct {
	logger *slog.Logger
	repo   AuthRepo // Use the interface
	cfg    *config.Config
}

// NewAuthService creates a new authentication service instance.
func NewAuthService(repo AuthRepo, cfg *config.Config, logger *slog.Logger) *AuthServiceImpl {
	// ... (nil checks and validation as before) ...
	return &AuthServiceImpl{logger: logger, repo: repo, cfg: cfg}
}

// Login validates credentials, generates tokens, stores refresh token.
func (s *AuthServiceImpl) Login(ctx context.Context, email, password string) (string, string, error) {
	l := s.logger.With(slog.String("method", "Login"), slog.String("email", email))
	l.DebugContext(ctx, "Attempting login")

	// 1. Fetch user by email (includes hash)
	user, err := s.repo.GetUserByEmail(ctx, email)
	if err != nil {
		l.WarnContext(ctx, "GetUserByEmail failed", slog.Any("error", err))
		// Don't reveal if user exists or password is wrong
		return "", "", fmt.Errorf("invalid credentials: %w", types.ErrUnauthenticated)
	}

	// 2. Compare submitted password with stored hash
	err = bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password))
	if err != nil {
		l.WarnContext(ctx, "Password comparison failed", slog.String("userID", user.ID))
		return "", "", fmt.Errorf("invalid credentials: %w", types.ErrUnauthenticated)
	}

	// --- Add types.Subscription Fetching Here Later ---
	// sub, err := s.subsRepo.GetCurrenttypes.SubscriptionByUserID(ctx, user.ID) ...
	// For now, create dummy/default sub info for token generation
	sub := &types.Subscription{Plan: "free", Status: "active"} // Placeholder

	// 3. Generate Tokens
	accessToken, refreshToken, err := s.GenerateTokens(ctx, user, sub) // Pass user and sub
	if err != nil {
		l.ErrorContext(ctx, "Failed to generate tokens", slog.String("userID", user.ID), slog.Any("error", err))
		return "", "", fmt.Errorf("internal error generating tokens: %w", err)
	}

	// 4. Store the new Refresh Token
	refreshTTL := s.getRefreshTTL()
	refreshExpiresAt := time.Now().Add(refreshTTL)
	err = s.repo.StoreRefreshToken(ctx, user.ID, refreshToken, refreshExpiresAt)
	if err != nil {
		l.ErrorContext(ctx, "Failed to store refresh token", slog.String("userID", user.ID), slog.Any("error", err))
		return "", "", fmt.Errorf("internal error storing session: %w", err)
	}

	l.InfoContext(ctx, "Login successful")
	return accessToken, refreshToken, nil
}

func (s *AuthServiceImpl) Register(ctx context.Context, username, email, password, _ string) error {
	l := s.logger.With(slog.String("method", "Register"), slog.String("email", email))
	l.DebugContext(ctx, "Attempting registration")

	// Get the global tracer
	tracer := otel.Tracer("MyRESTAPI")

	// Start a child span for the service layer
	ctx, span := tracer.Start(ctx, "AuthService.Register", trace.WithAttributes(
		attribute.String("username", username),
		attribute.String("email", email),
	))
	defer span.End()

	// Hash the password
	hashedPasswordBytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		l.ErrorContext(ctx, "Failed to hash password", slog.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Password hashing failed")
		return fmt.Errorf("could not process password")
	}
	hashedPassword := string(hashedPasswordBytes)

	// Call repository to store user
	userID, err := s.repo.Register(ctx, username, email, hashedPassword)
	if err != nil {
		l.ErrorContext(ctx, "Repository registration failed", slog.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Repository registration failed")
		return fmt.Errorf("registration failed: %w", err)
	}

	l.InfoContext(ctx, "Registration successful", slog.String("userID", userID))
	span.SetStatus(codes.Ok, "User registered")
	return nil
}

// RefreshSession validates refresh token, generates new tokens, rotates refresh token.
func (s *AuthServiceImpl) RefreshSession(ctx context.Context, refreshToken string) (string, string, error) {
	l := s.logger.With(slog.String("method", "RefreshSession"))
	l.DebugContext(ctx, "Attempting token refresh")

	// 1. Validate refresh token and get User ID
	userID, err := s.repo.ValidateRefreshTokenAndGetUserID(ctx, refreshToken)
	if err != nil {
		l.WarnContext(ctx, "Refresh token validation failed", slog.Any("error", err))
		return "", "", fmt.Errorf("invalid or expired refresh token: %w", err)
	}

	// 2. Fetch full user details for new token claims
	user, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		l.ErrorContext(ctx, "Failed to get user details after refresh token validation", slog.String("userID", userID), slog.Any("error", err))
		// Invalidate the suspicious token?
		err = s.repo.InvalidateRefreshToken(ctx, refreshToken)
		if err != nil {
			return "", "", fmt.Errorf("invalid or expired refresh token: %w", err)
		}
		return "", "", fmt.Errorf("internal error retrieving user during refresh")
	}

	// --- Fetch types.Subscription Here Later ---
	sub := &types.Subscription{Plan: "free", Status: "active"} // Placeholder

	// 3. Generate NEW tokens
	newAccessToken, newRefreshToken, err := s.GenerateTokens(ctx, user, sub)
	if err != nil {
		l.ErrorContext(ctx, "Failed to generate new tokens", slog.String("userID", user.ID), slog.Any("error", err))
		return "", "", fmt.Errorf("internal error generating tokens: %w", err)
	}

	// 4. Store the NEW refresh token
	refreshTTL := s.getRefreshTTL()
	refreshExpiresAt := time.Now().Add(refreshTTL)
	err = s.repo.StoreRefreshToken(ctx, user.ID, newRefreshToken, refreshExpiresAt)
	if err != nil {
		l.ErrorContext(ctx, "Failed to store new refresh token", slog.String("userID", user.ID), slog.Any("error", err))
		return "", "", fmt.Errorf("internal error storing new session: %w", err)
	}

	// 5. Invalidate the OLD refresh token (Rotation)
	err = s.repo.InvalidateRefreshToken(ctx, refreshToken)
	if err != nil {
		l.WarnContext(ctx, "Failed to invalidate old refresh token during rotation", slog.String("userID", user.ID), slog.Any("error", err))
		// Log, but proceed since new tokens were issued
	}

	l.InfoContext(ctx, "Token refresh successful", slog.String("userID", user.ID))
	return newAccessToken, newRefreshToken, nil
}

// Logout invalidates the provided refresh token.
func (s *AuthServiceImpl) Logout(ctx context.Context, refreshToken string) error {
	l := s.logger.With(slog.String("method", "Logout"))
	l.DebugContext(ctx, "Attempting logout by invalidating refresh token")
	err := s.repo.InvalidateRefreshToken(ctx, refreshToken)
	if err != nil {
		l.ErrorContext(ctx, "Failed to invalidate refresh token", slog.Any("error", err))
		// Decide if this should be an error back to client
		// return fmt.Errorf("logout failed: %w", err)
	}
	l.InfoContext(ctx, "Logout successful (token invalidated)")
	return nil // Usually succeed logout even if invalidation had minor issues
}

// UpdatePassword verifies old password, hashes new one, updates, invalidates tokens.
func (s *AuthServiceImpl) UpdatePassword(ctx context.Context, userID, oldPassword, newPassword string) error {
	l := s.logger.With(slog.String("method", "UpdatePassword"), slog.String("userID", userID))
	l.DebugContext(ctx, "Attempting password update")

	// 1. Verify old password using the repository method
	err := s.repo.VerifyPassword(ctx, userID, oldPassword)
	if err != nil {
		l.WarnContext(ctx, "Old password verification failed", slog.Any("error", err))
		return fmt.Errorf("incorrect old password: %w", types.ErrUnauthenticated)
	}

	// 2. Hash the *new* password
	newHashedPasswordBytes, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		l.ErrorContext(ctx, "Failed to hash new password", slog.Any("error", err))
		return fmt.Errorf("could not process new password")
	}
	newHashedPassword := string(newHashedPasswordBytes)

	// 3. Call repository to update the stored hash
	err = s.repo.UpdatePassword(ctx, userID, newHashedPassword)
	if err != nil {
		l.ErrorContext(ctx, "Repository password update failed", slog.Any("error", err))
		return fmt.Errorf("failed to update password: %w", err)
	}

	// 4. Invalidate all refresh tokens for security
	err = s.InvalidateAllUserRefreshTokens(ctx, userID) // Call self to use logging etc.
	if err != nil {
		// Log as warning, password update succeeded but token invalidation failed
		l.WarnContext(ctx, "Failed to invalidate refresh tokens after password update", slog.Any("error", err))
	}

	l.InfoContext(ctx, "Password updated successfully")
	return nil
}

// InvalidateAllUserRefreshTokens invalidates all active refresh tokens for a user.
func (s *AuthServiceImpl) InvalidateAllUserRefreshTokens(ctx context.Context, userID string) error {
	l := s.logger.With(slog.String("method", "InvalidateAllUserRefreshTokens"), slog.String("userID", userID))
	l.DebugContext(ctx, "Invalidating all refresh tokens")
	err := s.repo.InvalidateAllUserRefreshTokens(ctx, userID)
	if err != nil {
		l.ErrorContext(ctx, "Failed to invalidate all refresh tokens", slog.Any("error", err))
		return fmt.Errorf("failed to invalidate tokens: %w", err)
	}
	l.InfoContext(ctx, "All refresh tokens invalidated")
	return nil
}

func (s *AuthServiceImpl) GetUserByID(ctx context.Context, userID string) (*types.UserAuth, error) {
	l := s.logger.With(slog.String("method", "GetUserByID"), slog.String("userID", userID))
	l.DebugContext(ctx, "Fetching user by ID")
	user, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		l.ErrorContext(ctx, "Failed to fetch user by ID", slog.Any("error", err))
		return nil, fmt.Errorf("failed to fetch user: %w", err)
	}
	l.InfoContext(ctx, "User fetched successfully")
	return user, nil
}

// --- Internal Helper: generateTokens ---
func (s *AuthServiceImpl) GenerateTokens(ctx context.Context, user *types.UserAuth, _ *types.Subscription) (accessToken string, refreshToken string, err error) {
	l := s.logger.With(slog.String("method", "generateTokens"), slog.String("userID", user.ID))

	// --- Access Token ---
	accessTTL := s.getAccessTTL()
	issuer := s.getIssuer()
	audience := s.getAudience()
	secretKeyBytes := []byte(s.getSecretKey())

	accessClaims := &types.Claims{ // Use your Claims struct
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(accessTTL)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Subject:   user.ID,
			Issuer:    issuer,
			Audience:  jwt.ClaimStrings{audience},
		},
		// Custom Claims
		UserID:   user.ID,
		Username: user.Username,
		Email:    user.Email,
		//types.SubscriptionPlan:   sub.Plan,   // Add from sub
		//types.SubscriptionStatus: sub.Status, // Add from sub
	}
	accessTokenJWT := jwt.NewWithClaims(jwt.SigningMethodHS256, accessClaims)
	accessToken, err = accessTokenJWT.SignedString(secretKeyBytes)
	if err != nil {
		l.ErrorContext(ctx, "Failed to sign access token", slog.Any("error", err))
		return "", "", fmt.Errorf("failed to sign access token: %w", err)
	}

	// --- Refresh Token ---
	refreshToken = uuid.NewString() // Simple UUID, stored in DB

	l.DebugContext(ctx, "Tokens generated successfully")
	return accessToken, refreshToken, nil
}

func (s *AuthServiceImpl) VerifyPassword(ctx context.Context, userID, password string) error {
	return s.repo.VerifyPassword(ctx, userID, password)
}

// --- Internal Helpers for Config with Defaults ---
func (s *AuthServiceImpl) getAccessTTL() time.Duration {
	if s.cfg != nil && s.cfg.JWT.AccessTokenTTL > 0 {
		return s.cfg.JWT.AccessTokenTTL
	}
	s.logger.Warn("JWT AccessTokenTTL not configured, using default 15m")
	return 15 * time.Minute
}
func (s *AuthServiceImpl) getRefreshTTL() time.Duration {
	if s.cfg != nil && s.cfg.JWT.RefreshTokenTTL > 0 {
		return s.cfg.JWT.RefreshTokenTTL
	}
	s.logger.Warn("JWT RefreshTokenTTL not configured, using default 7d")
	return 7 * 24 * time.Hour
}
func (s *AuthServiceImpl) getIssuer() string {
	if s.cfg != nil && s.cfg.JWT.Issuer != "" {
		return s.cfg.JWT.Issuer
	}
	return "Loci" // Default
}
func (s *AuthServiceImpl) getAudience() string {
	if s.cfg != nil && s.cfg.JWT.Audience != "" {
		return s.cfg.JWT.Audience
	}
	return "Loci-app" // Default
}
func (s *AuthServiceImpl) getSecretKey() string {
	// Already checked for empty in NewAuthService
	return s.cfg.JWT.SecretKey
}

func (s *AuthServiceImpl) ValidateRefreshToken(ctx context.Context, refreshToken string) (string, error) {
	// Assuming repo has a method to validate refresh tokens
	userID, err := s.repo.ValidateRefreshTokenAndGetUserID(ctx, refreshToken)
	if err != nil {
		return "", err
	}
	return userID, nil
}

// implement much later

// Implement a dummy for now if needed for compilation
type dummySubsRepo struct{}

func (d *dummySubsRepo) GetCurrentSubscriptionByUserID(_ context.Context, _ string) (*types.Subscription, error) {
	return &types.Subscription{Plan: "free", Status: "active"}, nil // Always return free/active
}
func (d *dummySubsRepo) CreateDefaultSubscription(_ context.Context, _ string) error {
	return nil // Do nothing
}
func NewDummySubsRepo() types.SubscriptionRepository { return &dummySubsRepo{} }

// provider
func (s *AuthServiceImpl) GetOrCreateUserFromProvider(ctx context.Context, provider string, providerUser goth.User) (*types.UserAuth, error) {
	// Check if the user exists based on provider and provider_user_id
	userID, err := s.repo.GetUserIDByProvider(ctx, provider, providerUser.UserID)
	if err == nil {
		// User exists, retrieve them
		return s.repo.GetUserByID(ctx, userID)
	}

	// Check if the email is already taken
	existingUser, err := s.repo.GetUserByEmail(ctx, providerUser.Email)
	if err == nil && existingUser != nil {
		return nil, types.ErrConflict
	}

	// Create a new user
	newUser := &types.UserAuth{
		Username: providerUser.Name,
		Email:    providerUser.Email,
		Role:     "user", // Default role
	}
	err = s.repo.CreateUser(ctx, newUser)
	if err != nil {
		return nil, err
	}

	// Link the provider
	err = s.repo.CreateUserProvider(ctx, newUser.ID, provider, providerUser.UserID)
	if err != nil {
		return nil, err
	}

	return newUser, nil
}
