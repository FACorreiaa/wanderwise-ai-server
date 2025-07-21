package domain

import (
	"context"
	"time"
)

type UserAuth struct {
	ID        string    `json:"id" example:"d290f1ee-6c54-4b01-90e6-d701748f0851"` // Unique identifier (UUID).
	Username  string    `json:"username" example:"johndoe"`                        // Optional unique username.
	Email     string    `json:"email" example:"john.doe@example.com"`              // Unique email address used for login.
	Password  string    `json:"-"`                                                 // Hashed password (never exposed).
	Role      string    `json:"role" example:"user"`                               // User role (e.g., 'user', 'admin').
	CreatedAt time.Time `json:"created_at"`                                        // Timestamp when the user was created.
	UpdatedAt time.Time `json:"updated_at"`                                        // Timestamp when the user was last updated.
}

type AuthRepository interface {
	GetUserByEmail(ctx context.Context, email string) (*UserAuth, error)
	GetUserByID(ctx context.Context, userID string) (*UserAuth, error)
	Register(ctx context.Context, username, email, hashedPassword string) (string, error)
	VerifyPassword(ctx context.Context, userID, password string) error // Password is plain text here
	UpdatePassword(ctx context.Context, userID, newHashedPassword string) error

	CreateUser(ctx context.Context, user *UserAuth) error
	CreateUserProvider(ctx context.Context, userID, provider, providerUserID string) error
	GetUserIDByProvider(ctx context.Context, provider, providerUserID string) (string, error)

	StoreRefreshToken(ctx context.Context, userID, token string, expiresAt time.Time) error
	ValidateRefreshTokenAndGetUserID(ctx context.Context, refreshToken string) (userID string, err error)
	InvalidateRefreshToken(ctx context.Context, refreshToken string) error
	InvalidateAllUserRefreshTokens(ctx context.Context, userID string) error
}
