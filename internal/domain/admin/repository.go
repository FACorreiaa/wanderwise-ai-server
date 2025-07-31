package admin

import (
	"context"

	"github.com/google/uuid"

	"github.com/FACorreiaa/go-poi-au-suggestions/internal/types"
)

type AdminRepo interface {
	// GetUserByID retrieves a user's full profile by their unique ID.
	GetUserByID(ctx context.Context, userID uuid.UUID) (*types.UserProfile, error)
	// UpdateProfile updates mutable fields on a user's profile.
	// It takes the userID and a struct containing only the fields to be updated (use pointers).
	UpdateProfile(ctx context.Context, userID uuid.UUID, params types.UpdateProfileParams) error
	// DeactivateUser marks a user as inactive (soft delete).
	// Consider if this should also invalidate sessions/tokens.
	DeactivateUser(ctx context.Context, userID uuid.UUID) error
	// ReactivateUser marks a user as active.
	ReactivateUser(ctx context.Context, userID uuid.UUID) error
	// GetAllInterests retrieves all available interests for selection in the UI.
	GetAllInterests(ctx context.Context) ([]types.Interest, error)
	// GetUserPreferences retrieves the list of interests associated with a user.
	GetUserPreferences(ctx context.Context, userID uuid.UUID) ([]types.Interest, error)
	// SetUserPreferences atomically replaces a user's current interests with the provided list of interest IDs.
	SetUserPreferences(ctx context.Context, userID uuid.UUID, interestIDs []uuid.UUID) error

	// --- Other Potential Methods ---

	// FindUsers searches/filters users (e.g., for admin panels). Requires pagination/filtering parameters.
	// ListUsers(ctx context.Context, filterParams ListUsersParams) ([]User, int, error) // Returns users, total count, error

	// DeleteUser performs a hard delete (use with caution).
	// DeleteUser(ctx context.Context, userID uuid.UUID) error
}
