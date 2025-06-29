package profiles

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/sync/errgroup"

	"github.com/FACorreiaa/go-poi-au-suggestions/internal/api/interests"
	"github.com/FACorreiaa/go-poi-au-suggestions/internal/api/tags"
	"github.com/FACorreiaa/go-poi-au-suggestions/internal/types"
)

// Ensure implementation satisfies the interface
var _ Service = (*ServiceImpl)(nil)

// profilessService defines the business logic contract for user operations.
type Service interface {
	//GetSearchProfiles User  Profiles
	GetSearchProfiles(ctx context.Context, userID uuid.UUID) ([]types.UserPreferenceProfileResponse, error)
	GetSearchProfile(ctx context.Context, userID, profileID uuid.UUID) (*types.UserPreferenceProfileResponse, error)
	GetDefaultSearchProfile(ctx context.Context, userID uuid.UUID) (*types.UserPreferenceProfileResponse, error)
	CreateSearchProfile(ctx context.Context, userID uuid.UUID, params types.CreateUserPreferenceProfileParams) (*types.UserPreferenceProfileResponse, error)
	UpdateSearchProfile(ctx context.Context, userID, profileID uuid.UUID, params types.UpdateSearchProfileParams) error
	DeleteSearchProfile(ctx context.Context, userID, profileID uuid.UUID) error
	SetDefaultSearchProfile(ctx context.Context, userID, profileID uuid.UUID) error
	
	// Domain preferences are now handled in the main UpdateSearchProfile method
}

// ServiceImpl provides the implementation for UserService.
type ServiceImpl struct {
	logger   *slog.Logger
	prefRepo Repository
	intRepo  interests.Repository
	tagRepo  tags.Repository
}

func NewUserProfilesService(prefRepo Repository, intRepo interests.Repository, tagRepo tags.Repository, logger *slog.Logger) *ServiceImpl {
	return &ServiceImpl{
		prefRepo: prefRepo,
		intRepo:  intRepo,
		tagRepo:  tagRepo,
		logger:   logger,
	}
}

// GetSearchProfiles retrieves all preference profiles for a user.
func (s *ServiceImpl) GetSearchProfiles(ctx context.Context, userID uuid.UUID) ([]types.UserPreferenceProfileResponse, error) {
	ctx, span := otel.Tracer("UserService").Start(ctx, "GetSearchProfiles", trace.WithAttributes(
		attribute.String("user.id", userID.String()),
	))
	defer span.End()

	l := s.logger.With(slog.String("method", "GetSearchProfiles"), slog.String("userID", userID.String()))
	l.DebugContext(ctx, "Fetching user preference profiles")

	profiles, err := s.prefRepo.GetSearchProfiles(ctx, userID)
	if err != nil {
		l.ErrorContext(ctx, "Failed to fetch user preference profiles", slog.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to fetch user preference profiles")
		return nil, fmt.Errorf("error fetching user preference profiles: %w", err)
	}

	l.InfoContext(ctx, "User preference profiles fetched successfully", slog.Int("count", len(profiles)))
	span.SetStatus(codes.Ok, "User preference profiles fetched successfully")
	return profiles, nil
}

// GetSearchProfile retrieves a specific preference profile by ID.
func (s *ServiceImpl) GetSearchProfile(ctx context.Context, userID, profileID uuid.UUID) (*types.UserPreferenceProfileResponse, error) {
	ctx, span := otel.Tracer("UserService").Start(ctx, "GetSearchProfile", trace.WithAttributes(
		attribute.String("profile.id", profileID.String()),
	))
	defer span.End()

	l := s.logger.With(slog.String("method", "GetSearchProfile"), slog.String("profileID", profileID.String()))
	l.DebugContext(ctx, "Fetching user preference profile")

	profile, err := s.prefRepo.GetSearchProfile(ctx, userID, profileID)
	if err != nil {
		l.ErrorContext(ctx, "Failed to fetch user preference profile", slog.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to fetch user preference profile")
		return nil, fmt.Errorf("error fetching user preference profile: %w", err)
	}

	l.InfoContext(ctx, "User preference profile fetched successfully")
	span.SetStatus(codes.Ok, "User preference profile fetched successfully")
	return profile, nil
}

// GetDefaultSearchProfile retrieves the default preference profile for a user.
func (s *ServiceImpl) GetDefaultSearchProfile(ctx context.Context, userID uuid.UUID) (*types.UserPreferenceProfileResponse, error) {
	ctx, span := otel.Tracer("UserService").Start(ctx, "GetDefaultSearchProfile", trace.WithAttributes(
		attribute.String("user.id", userID.String()),
	))
	defer span.End()

	l := s.logger.With(slog.String("method", "GetDefaultSearchProfile"), slog.String("userID", userID.String()))
	l.DebugContext(ctx, "Fetching default user preference profile")

	profile, err := s.prefRepo.GetDefaultSearchProfile(ctx, userID)
	if err != nil {
		l.ErrorContext(ctx, "Failed to fetch default user preference profile", slog.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to fetch default user preference profile")
		return nil, fmt.Errorf("error fetching default user preference profile: %w", err)
	}

	l.InfoContext(ctx, "Default user preference profile fetched successfully")
	span.SetStatus(codes.Ok, "Default user preference profile fetched successfully")
	return profile, nil
}

// CreateSearchProfileCC TODO fix Create profile interests and tags
func (s *ServiceImpl) CreateSearchProfileCC(ctx context.Context, userID uuid.UUID, params types.CreateUserPreferenceProfileParams) (*types.UserPreferenceProfileResponse, error) { // Return the richer response type

	ctx, span := otel.Tracer("PreferenceService").Start(ctx, "CreateSearchProfile")
	defer span.End()

	l := s.logger.With(slog.String("method", "CreateSearchProfile"), slog.String("userID", userID.String()), slog.String("profileName", params.ProfileName))
	l.DebugContext(ctx, "Attempting to create profile and link associations")

	// --- 1. Validate input further if needed (e.g., check if profile name is empty) ---
	if params.ProfileName == "" {
		return nil, fmt.Errorf("%w: profile name cannot be empty", types.ErrBadRequest)
	}

	tx, err := s.prefRepo.(*RepositoryImpl).pgpool.Begin(ctx)
	if err != nil {
		l.ErrorContext(ctx, "Failed to begin transaction", slog.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Transaction begin failed")
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// --- 2. Create the base profile ---
	// NOTE: The repo method CreateSearchProfile should ONLY insert into user_preference_profiles
	// and return the core profile data. It should NOT handle tags/interests.
	createdProfileCore, err := s.prefRepo.CreateSearchProfile(ctx, userID, params)
	if err != nil {
		l.ErrorContext(ctx, "Failed to create base profile in repo", slog.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Repo failed creating profile")
		// Map repo errors (like ErrConflict) if applicable
		return nil, fmt.Errorf("failed to create profile core: %w", err)
	}
	profileID := createdProfileCore.ID
	l.InfoContext(ctx, "Base profile created successfully", slog.String("profileID", profileID.String()))

	// --- 3. Link Interests and Avoid Tags Concurrently ---
	g, childCtx := errgroup.WithContext(ctx)

	// Link Interests
	if len(params.Interests) > 0 {
		interestIDs := params.Interests // Capture loop variable
		g.Go(func() error {
			l.DebugContext(childCtx, "Linking interests to profile", slog.Int("count", len(interestIDs)), slog.String("profileID", profileID.String()))
			for _, interestID := range interestIDs {
				linkErr := s.intRepo.AddInterestToProfile(childCtx, profileID, interestID)
				if linkErr != nil {
					// Log the specific error but potentially continue linking others
					l.ErrorContext(childCtx, "Failed to link interest to profile", slog.String("interestID", interestID.String()), slog.Any("error", linkErr))
					return fmt.Errorf("failed linking interest %s: %w", interestID, linkErr) // Causes errgroup to cancel
				}
			}
			return nil // Success for this goroutine (unless an error was returned above)
		})
	}

	// Link Avoid Tags
	if len(params.Tags) > 0 {
		tagIDs := params.Tags // Capture loop variable
		g.Go(func() error {
			l.DebugContext(childCtx, "Linking avoid tags to profile", slog.Int("count", len(tagIDs)), slog.String("profileID", profileID.String()))
			for _, tagID := range tagIDs {
				linkErr := s.tagRepo.LinkPersonalTagToProfile(childCtx, userID, profileID, tagID)
				if linkErr != nil {
					l.ErrorContext(childCtx, "Failed to link avoid tag to profile", slog.String("tagID", tagID.String()), slog.Any("error", linkErr))
					return fmt.Errorf("failed linking avoid tag %s: %w", tagID, linkErr) // Causes errgroup to cancel
				}
			}
			return nil // Success for this goroutine
		})
	}

	// Wait for linking operations
	if err := g.Wait(); err != nil {
		l.ErrorContext(ctx, "Error occurred during interest/tag association", slog.Any("error", err))
		// Profile was created, but associations might be incomplete.
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed associating items")
		// Return the created profile data along with the association error?
		// Or consider rolling back the profile creation (requires full transaction management)?
		// Returning the error indicating partial success is one option.
		return createdProfileCore, fmt.Errorf("profile created, but failed associating items: %w", err)
	}

	// --- 4. Fetch Associated Data for Response (Optional but good UX) ---
	// After successful creation and linking, fetch the linked data to return the full response object.
	// Can also run these concurrently.
	gResp, respCtx := errgroup.WithContext(ctx)
	var fetchedInterests []*types.Interest
	var fetchedTags []*types.Tags

	gResp.Go(func() error {
		var fetchErr error
		fetchedInterests, fetchErr = s.intRepo.GetAllInterests(respCtx)
		l.DebugContext(respCtx, "Fetched interests for response", slog.Int("count", len(fetchedInterests)), slog.Any("error", fetchErr)) // Log count and error
		return fetchErr                                                                                                                  // Return error if fetching fails
	})

	gResp.Go(func() error {
		var fetchErr error
		fetchedTags, fetchErr = s.tagRepo.GetAll(respCtx, userID)
		l.DebugContext(respCtx, "Fetched tags for response", slog.Int("count", len(fetchedTags)), slog.Any("error", fetchErr)) // Log count and error
		return fetchErr
	})

	if err = gResp.Wait(); err != nil {
		l.ErrorContext(ctx, "Error occurred fetching associated data for response", slog.Any("error", err))
		return createdProfileCore, nil
	}

	if err = tx.Commit(ctx); err != nil {
		l.ErrorContext(ctx, "Failed to commit transaction", slog.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Transaction commit failed")
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	// --- 5. Assemble Final Response ---
	fullResponse := &types.UserPreferenceProfileResponse{
		// Copy fields from createdProfileCore
		ID:                   createdProfileCore.ID,
		UserID:               createdProfileCore.UserID,
		ProfileName:          createdProfileCore.ProfileName,
		IsDefault:            createdProfileCore.IsDefault,
		SearchRadiusKm:       createdProfileCore.SearchRadiusKm,
		PreferredTime:        createdProfileCore.PreferredTime,
		BudgetLevel:          createdProfileCore.BudgetLevel,
		PreferredPace:        createdProfileCore.PreferredPace,
		PreferAccessiblePOIs: createdProfileCore.PreferAccessiblePOIs,
		PreferOutdoorSeating: createdProfileCore.PreferOutdoorSeating,
		PreferDogFriendly:    createdProfileCore.PreferDogFriendly,
		PreferredVibes:       createdProfileCore.PreferredVibes,
		PreferredTransport:   createdProfileCore.PreferredTransport,
		DietaryNeeds:         createdProfileCore.DietaryNeeds,
		CreatedAt:            createdProfileCore.CreatedAt,
		UpdatedAt:            createdProfileCore.UpdatedAt,
		Interests:            fetchedInterests,
		Tags:                 fetchedTags,
	}

	l.InfoContext(ctx, "Successfully created profile and processed associations")
	span.SetStatus(codes.Ok, "Profile created with associations")
	return fullResponse, nil
}

// CreateSearchProfile userProfiles/service.go
func (s *ServiceImpl) CreateSearchProfile(ctx context.Context, userID uuid.UUID, p types.CreateUserPreferenceProfileParams) (*types.UserPreferenceProfileResponse, error) {
	ctx, span := otel.Tracer("ProfilesService").Start(ctx, "CreateSearchProfile")
	defer span.End()

	// If this is the first profile for the user, set it as default
	profiles, err := s.prefRepo.GetSearchProfiles(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("error checking existing profiles: %w", err)
	}
	if len(profiles) == 0 {
		defaultValue := true
		p.IsDefault = &defaultValue
	}

	return s.prefRepo.CreateSearchProfile(ctx, userID, p)
}

// DeleteSearchProfile deletes a preference profile.
func (s *ServiceImpl) DeleteSearchProfile(ctx context.Context, userID, profileID uuid.UUID) error {
	ctx, span := otel.Tracer("UserService").Start(ctx, "DeleteSearchProfile", trace.WithAttributes(
		attribute.String("profile.id", profileID.String()),
	))
	defer span.End()

	l := s.logger.With(slog.String("method", "DeleteSearchProfile"), slog.String("profileID", profileID.String()))
	l.DebugContext(ctx, "Deleting user preference profile")

	err := s.prefRepo.DeleteSearchProfile(ctx, userID, profileID)
	if err != nil {
		l.ErrorContext(ctx, "Failed to delete user preference profile", slog.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to delete user preference profile")
		return fmt.Errorf("error deleting user preference profile: %w", err)
	}

	l.InfoContext(ctx, "User preference profile deleted successfully")
	span.SetStatus(codes.Ok, "User preference profile deleted successfully")
	return nil
}

// SetDefaultSearchProfile sets a profile as the default for a user.
func (s *ServiceImpl) SetDefaultSearchProfile(ctx context.Context, userID, profileID uuid.UUID) error {
	ctx, span := otel.Tracer("UserService").Start(ctx, "SetDefaultSearchProfile", trace.WithAttributes(
		attribute.String("profile.id", profileID.String()),
	))
	defer span.End()

	l := s.logger.With(slog.String("method", "SetDefaultSearchProfile"), slog.String("profileID", profileID.String()))
	l.DebugContext(ctx, "Setting profile as default")

	err := s.prefRepo.SetDefaultSearchProfile(ctx, userID, profileID)
	if err != nil {
		l.ErrorContext(ctx, "Failed to set default user preference profile", slog.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to set default user preference profile")
		return fmt.Errorf("error setting default user preference profile: %w", err)
	}

	l.InfoContext(ctx, "User preference profile set as default successfully")
	span.SetStatus(codes.Ok, "User preference profile set as default successfully")
	return nil
}

// UpdateSearchProfile implements profilessService.
func (s *ServiceImpl) UpdateSearchProfile(ctx context.Context, userID, profileID uuid.UUID, params types.UpdateSearchProfileParams) error {
	ctx, span := otel.Tracer("UserService").Start(ctx, "UpdateSearchProfile", trace.WithAttributes(
		attribute.String("profile.id", profileID.String()),
	))
	defer span.End()

	l := s.logger.With(slog.String("method", "SetDefaultSearchProfile"), slog.String("profileID", profileID.String()))
	l.DebugContext(ctx, "Setting profile as default")

	if err := s.prefRepo.UpdateSearchProfile(ctx, userID, profileID, params); err != nil {
		l.ErrorContext(ctx, "Failed to set default user preference profile", slog.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to set default user preference profile")
		return fmt.Errorf("error setting default user preference profile: %w", err)
	}

	l.InfoContext(ctx, "User search profile updated successfully")
	span.SetStatus(codes.Ok, "User preference profile updated successfully")
	return nil
}

