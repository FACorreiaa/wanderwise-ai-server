package profiles

import (
	"context"

	"go.uber.org/zap"
)

// ProfileRepository defines the interface for interacting with user profiles.
// It is responsible for abstracting the underlying data source and providing
// a clean API for profile-related operations.
type ProfileRepository interface {
	// CreateProfile creates a new user profile in the data source.
	CreateProfile(ctx context.Context, profile *Profile) error
	// GetProfile retrieves a user profile by its ID.
	GetProfile(ctx context.Context, id string) (*Profile, error)
	// UpdateProfile updates an existing user profile.
	UpdateProfile(ctx context.Context, profile *Profile) error
	// DeleteProfile removes a user profile from the data source.
	DeleteProfile(ctx context.Context, id string) error
}

// zapProfileRepository is an implementation of ProfileRepository that uses
// a zap logger for logging.
// It is responsible for logging all profile-related operations.
type zapProfileRepository struct {
	logger *zap.Logger
}

// NewZapProfileRepository creates a new zapProfileRepository.
// It takes a zap logger as a dependency and returns a new zapProfileRepository.
func NewZapProfileRepository(logger *zap.Logger) ProfileRepository {
	return &zapProfileRepository{
		logger: logger,
	}
}

// CreateProfile creates a new user profile in the data source.
// It logs the creation of the profile.
func (r *zapProfileRepository) CreateProfile(ctx context.Context, profile *Profile) error {
	r.logger.Info("creating profile", zap.Any("profile", profile))
	return nil
}

// GetProfile retrieves a user profile by its ID.
// It logs the retrieval of the profile.
func (r *zapProfileRepository) GetProfile(ctx context.Context, id string) (*Profile, error) {
	r.logger.Info("getting profile", zap.String("id", id))
	return nil, nil
}

// UpdateProfile updates an existing user profile.
// It logs the update of the profile.
func (r *zapProfileRepository) UpdateProfile(ctx context.Context, profile *Profile) error {
	r.logger.Info("updating profile", zap.Any("profile", profile))
	return nil
}

// DeleteProfile removes a user profile from the data source.
// It logs the deletion of the profile.
func (r *zapProfileRepository) DeleteProfile(ctx context.Context, id string) error {
	r.logger.Info("deleting profile", zap.String("id", id))
	return nil
}
