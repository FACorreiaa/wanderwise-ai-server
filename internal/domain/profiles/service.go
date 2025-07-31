package profiles

import (
	"context"
	"errors"
	"strings"

	pb "github.com/FACorreiaa/loci-proto/modules/profiles/generated"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	codes2 "go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	grpcCodes "google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/FACorreiaa/go-poi-au-suggestions/internal/domain"
)

type Service struct {
	pb.UnimplementedProfilesServiceServer
	logger *zap.Logger
	repo   Repository
	pgpool *pgxpool.Pool
	tracer trace.Tracer
}

func NewService(ctx context.Context, repo Repository, pgpool *pgxpool.Pool, logger *zap.Logger) *Service {
	return &Service{
		logger: logger.With(zap.String("service", "profiles")),
		repo:   repo,
		pgpool: pgpool,
		tracer: otel.Tracer("ProfilesService"),
	}
}

func (svc *Service) GetSearchProfiles(ctx context.Context, req *pb.GetSearchProfilesRequest) (*pb.GetSearchProfilesResponse, error) {
	ctx, span := svc.tracer.Start(ctx, "ProfilesService.GetSearchProfiles", trace.WithAttributes(
		attribute.String("profiles.operation", "get_search_profiles"),
		attribute.String("profiles.user_id", req.UserId),
	))
	defer span.End()

	userID, err := domain.CheckUserAuth(ctx)
	if err != nil {
		return nil, err
	}

	userUUID, err := uuid.Parse(userID)
	if err != nil {
		svc.logger.Error("Invalid user ID format", zap.Error(err))
		return &pb.GetSearchProfilesResponse{}, status.Error(codes.InvalidArgument, "invalid user ID format")
	}

	svc.logger.Debug("Getting search profiles", zap.String("userID", req.UserId))

	var requestId string
	if req.Request != nil {
		requestId = req.Request.RequestId
	}

	req.UserId = userID

	profiles, err := svc.repo.GetSearchProfiles(ctx, userUUID)
	if err != nil {
		span.RecordError(err)
		svc.logger.Error("Failed to get search profiles", zap.Error(err))
		return &pb.GetSearchProfilesResponse{
			Profiles:         nil,
			DefaultProfileId: "",
			Response: &pb.BaseResponse{
				RequestId: requestId,
				Upstream:  "profiles-service",
				Status:    "error",
			},
		}, status.Error(grpcCodes.Internal, "failed to get search profiles")
	}

	// Convert to protobuf and find default profile
	pbProfiles := make([]*pb.UserPreferenceProfile, len(profiles))
	var defaultProfileID string
	for i, profile := range profiles {
		pbProfiles[i] = convertToPBProfile(&profile)
		if profile.IsDefault {
			defaultProfileID = profile.ID.String()
		}
	}

	span.SetStatus(codes2.Ok, "Search profiles retrieved successfully")
	svc.logger.Info("Search profiles retrieved successfully", zap.String("userID", req.UserId), zap.Int("count", len(profiles)))

	return &pb.GetSearchProfilesResponse{
		Profiles:         pbProfiles,
		DefaultProfileId: defaultProfileID,
		Response: &pb.BaseResponse{
			RequestId: requestId,
			Upstream:  "profiles-service",
			Status:    "success",
		},
	}, nil
}

func (svc *Service) GetSearchProfile(ctx context.Context, req *pb.GetSearchProfileRequest) (*pb.GetSearchProfileResponse, error) {
	ctx, span := svc.tracer.Start(ctx, "ProfilesService.GetSearchProfile", trace.WithAttributes(
		attribute.String("profiles.operation", "get_search_profile"),
		attribute.String("profiles.user_id", req.UserId),
		attribute.String("profiles.profile_id", req.ProfileId),
	))
	defer span.End()

	userID, err := domain.CheckUserAuth(ctx)
	if err != nil {
		return nil, err
	}

	userUUID, err := uuid.Parse(userID)
	if err != nil {
		svc.logger.Error("Invalid user ID format", zap.Error(err))
		return &pb.GetSearchProfileResponse{}, status.Error(codes.InvalidArgument, "invalid user ID format")
	}

	svc.logger.Debug("Getting search profile", zap.String("userID", req.UserId), zap.String("profileID", req.ProfileId))

	var requestId string
	if req.Request != nil {
		requestId = req.Request.RequestId
	}

	req.UserId = userID

	// Validate input

	profileID, err := uuid.Parse(req.ProfileId)
	if err != nil {
		span.RecordError(err)
		//span.SetStatus(codes.Error, "Invalid profile ID format")
		svc.logger.Error("Invalid profile ID format", zap.Error(err))
		return &pb.GetSearchProfileResponse{
			Profile: nil,
			Response: &pb.BaseResponse{
				RequestId: requestId,
				Upstream:  "profiles-service",
				Status:    "error",
			},
		}, status.Error(grpcCodes.InvalidArgument, "invalid profile ID format")
	}

	profile, err := svc.repo.GetSearchProfile(ctx, userUUID, profileID)
	if err != nil {
		span.RecordError(err)
		if errors.Is(err, domain.ErrNotFound) {
			//span.SetStatus(codes.Error, "Profile not found")
			svc.logger.Warn("Search profile not found", zap.String("userID", req.UserId), zap.String("profileID", req.ProfileId))
			return &pb.GetSearchProfileResponse{
				Profile: nil,
				Response: &pb.BaseResponse{
					RequestId: requestId,
					Upstream:  "profiles-service",
					Status:    "error",
				},
			}, status.Error(grpcCodes.NotFound, "search profile not found")
		}
		//span.SetStatus(codes.Error, "Database error")
		svc.logger.Error("Failed to get search profile", zap.Error(err))
		return &pb.GetSearchProfileResponse{
			Profile: nil,
			Response: &pb.BaseResponse{
				RequestId: requestId,
				Upstream:  "profiles-service",
				Status:    "error",
			},
		}, status.Error(grpcCodes.Internal, "failed to get search profile")
	}

	//span.SetStatus(codes.Ok, "Search profile retrieved successfully")
	svc.logger.Info("Search profile retrieved successfully", zap.String("userID", req.UserId), zap.String("profileID", req.ProfileId))

	return &pb.GetSearchProfileResponse{
		Profile: convertToPBProfile(profile),
		Response: &pb.BaseResponse{
			RequestId: requestId,
			Upstream:  "profiles-service",
			Status:    "success",
		},
	}, nil
}

func (svc *Service) GetDefaultSearchProfile(ctx context.Context, req *pb.GetDefaultSearchProfileRequest) (*pb.GetDefaultSearchProfileResponse, error) {
	ctx, span := svc.tracer.Start(ctx, "ProfilesService.GetDefaultSearchProfile", trace.WithAttributes(
		attribute.String("profiles.operation", "get_default_search_profile"),
		attribute.String("profiles.user_id", req.UserId),
	))
	defer span.End()

	svc.logger.Debug("Getting default search profile", zap.String("userID", req.UserId))

	userID, err := domain.CheckUserAuth(ctx)
	if err != nil {
		return nil, err
	}

	userUUID, err := uuid.Parse(userID)
	if err != nil {
		svc.logger.Error("Invalid user ID format", zap.Error(err))
		return &pb.GetDefaultSearchProfileResponse{}, status.Error(codes.InvalidArgument, "invalid user ID format")
	}

	var requestId string
	if req.Request != nil {
		requestId = req.Request.RequestId
	}

	req.UserId = userID

	profile, err := svc.repo.GetDefaultSearchProfile(ctx, userUUID)
	if err != nil {
		span.RecordError(err)
		if errors.Is(err, domain.ErrNotFound) {
			span.SetStatus(codes2.Error, "Default profile not found")
			svc.logger.Warn("Default search profile not found", zap.String("userID", req.UserId))
			return &pb.GetDefaultSearchProfileResponse{
				Profile: nil,
				Response: &pb.BaseResponse{
					RequestId: requestId,
					Upstream:  "profiles-service",
					Status:    "error",
				},
			}, status.Error(grpcCodes.NotFound, "default search profile not found")
		}
		span.SetStatus(codes2.Error, "Database error")
		svc.logger.Error("Failed to get default search profile", zap.Error(err))
		return &pb.GetDefaultSearchProfileResponse{
			Profile: nil,
			Response: &pb.BaseResponse{
				RequestId: requestId,
				Upstream:  "profiles-service",
				Status:    "error",
			},
		}, status.Error(grpcCodes.Internal, "failed to get default search profile")
	}

	span.SetStatus(codes2.Ok, "Default search profile retrieved successfully")
	svc.logger.Info("Default search profile retrieved successfully", zap.String("userID", req.UserId))

	return &pb.GetDefaultSearchProfileResponse{
		Profile: convertToPBProfile(profile),
		Response: &pb.BaseResponse{
			RequestId: requestId,
			Upstream:  "profiles-service",
			Status:    "success",
		},
	}, nil
}

func (svc *Service) CreateSearchProfile(ctx context.Context, req *pb.CreateSearchProfileRequest) (*pb.CreateSearchProfileResponse, error) {
	ctx, span := svc.tracer.Start(ctx, "ProfilesService.CreateSearchProfile", trace.WithAttributes(
		attribute.String("profiles.operation", "create_search_profile"),
		attribute.String("profiles.user_id", req.UserId),
	))
	defer span.End()

	userID, err := domain.CheckUserAuth(ctx)
	if err != nil {
		return nil, err
	}

	userUUID, err := uuid.Parse(userID)
	if err != nil {
		svc.logger.Error("Invalid user ID format", zap.Error(err))
		return &pb.CreateSearchProfileResponse{}, status.Error(codes.InvalidArgument, "invalid user ID format")
	}

	svc.logger.Debug("Creating search profile", zap.String("userID", req.UserId))

	var requestId string
	if req.Request != nil {
		requestId = req.Request.RequestId
	}

	// Convert protobuf to internal types
	params := convertPBToCreateParams(req.Profile)

	profile, err := svc.repo.CreateSearchProfile(ctx, userUUID, params)
	if err != nil {
		span.RecordError(err)
		if errors.Is(err, domain.ErrConflict) {
			span.SetStatus(codes2.Error, "Profile name already exists")
			svc.logger.Warn("Profile name already exists", zap.String("userID", req.UserId), zap.String("profileName", req.Profile.ProfileName))
			return &pb.CreateSearchProfileResponse{
				Success: false,
				Message: "profile name already exists",
				Profile: nil,
				Response: &pb.BaseResponse{
					RequestId: requestId,
					Upstream:  "profiles-service",
					Status:    "error",
				},
			}, status.Error(grpcCodes.AlreadyExists, "profile name already exists")
		}
		span.SetStatus(codes2.Error, "Database error")
		svc.logger.Error("Failed to create search profile", zap.Error(err))
		return &pb.CreateSearchProfileResponse{
			Success: false,
			Message: "failed to create search profile",
			Profile: nil,
			Response: &pb.BaseResponse{
				RequestId: requestId,
				Upstream:  "profiles-service",
				Status:    "error",
			},
		}, status.Error(grpcCodes.Internal, "failed to create search profile")
	}

	span.SetStatus(codes2.Ok, "Search profile created successfully")
	svc.logger.Info("Search profile created successfully", zap.String("userID", req.UserId), zap.String("profileID", profile.ID.String()))

	return &pb.CreateSearchProfileResponse{
		Success: true,
		Message: "search profile created successfully",
		Profile: convertToPBProfile(profile),
		Response: &pb.BaseResponse{
			RequestId: requestId,
			Upstream:  "profiles-service",
			Status:    "success",
		},
	}, nil
}

func (svc *Service) UpdateSearchProfile(ctx context.Context, req *pb.UpdateSearchProfileRequest) (*pb.UpdateSearchProfileResponse, error) {
	ctx, span := svc.tracer.Start(ctx, "ProfilesService.UpdateSearchProfile", trace.WithAttributes(
		attribute.String("profiles.operation", "update_search_profile"),
		attribute.String("profiles.user_id", req.UserId),
		attribute.String("profiles.profile_id", req.ProfileId),
	))
	defer span.End()

	userID, err := domain.CheckUserAuth(ctx)
	if err != nil {
		return nil, err
	}

	userUUID, err := uuid.Parse(userID)
	if err != nil {
		svc.logger.Error("Invalid user ID format", zap.Error(err))
		return &pb.UpdateSearchProfileResponse{}, status.Error(codes.InvalidArgument, "invalid user ID format")
	}

	svc.logger.Debug("Updating search profile", zap.String("userID", req.UserId), zap.String("profileID", req.ProfileId))

	var requestId string
	if req.Request != nil {
		requestId = req.Request.RequestId
	}

	req.UserId = userID

	profileID, err := uuid.Parse(req.ProfileId)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes2.Error, "Invalid profile ID format")
		svc.logger.Error("Invalid profile ID format", zap.Error(err))
		return &pb.UpdateSearchProfileResponse{
			Success: false,
			Message: "invalid profile ID format",
			Profile: nil,
			Response: &pb.BaseResponse{
				RequestId: requestId,
				Upstream:  "profiles-service",
				Status:    "error",
			},
		}, status.Error(grpcCodes.InvalidArgument, "invalid profile ID format")
	}

	// Convert protobuf to internal types
	params := convertPBToUpdateParams(req.Profile)

	err = svc.repo.UpdateSearchProfile(ctx, userUUID, profileID, params)
	if err != nil {
		span.RecordError(err)
		if errors.Is(err, domain.ErrNotFound) {
			span.SetStatus(codes2.Error, "Profile not found")
			svc.logger.Warn("Search profile not found for update", zap.String("userID", req.UserId), zap.String("profileID", req.ProfileId))
			return &pb.UpdateSearchProfileResponse{
				Success: false,
				Message: "search profile not found",
				Profile: nil,
				Response: &pb.BaseResponse{
					RequestId: requestId,
					Upstream:  "profiles-service",
					Status:    "error",
				},
			}, status.Error(grpcCodes.NotFound, "search profile not found")
		}
		if errors.Is(err, domain.ErrConflict) {
			span.SetStatus(codes2.Error, "Profile name already exists")
			svc.logger.Warn("Profile name already exists", zap.String("userID", req.UserId), zap.String("profileID", req.ProfileId))
			return &pb.UpdateSearchProfileResponse{
				Success: false,
				Message: "profile name already exists",
				Profile: nil,
				Response: &pb.BaseResponse{
					RequestId: requestId,
					Upstream:  "profiles-service",
					Status:    "error",
				},
			}, status.Error(grpcCodes.AlreadyExists, "profile name already exists")
		}
		span.SetStatus(codes2.Error, "Database error")
		svc.logger.Error("Failed to update search profile", zap.Error(err))
		return &pb.UpdateSearchProfileResponse{
			Success: false,
			Message: "failed to update search profile",
			Profile: nil,
			Response: &pb.BaseResponse{
				RequestId: requestId,
				Upstream:  "profiles-service",
				Status:    "error",
			},
		}, status.Error(grpcCodes.Internal, "failed to update search profile")
	}

	// Get updated profile to return
	updatedProfile, err := svc.repo.GetSearchProfile(ctx, userUUID, profileID)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes2.Error, "Failed to retrieve updated profile")
		svc.logger.Error("Failed to retrieve updated profile", zap.Error(err))
		return &pb.UpdateSearchProfileResponse{
			Success: true,
			Message: "search profile updated but failed to retrieve",
			Profile: nil,
			Response: &pb.BaseResponse{
				RequestId: requestId,
				Upstream:  "profiles-service",
				Status:    "partial_success",
			},
		}, nil
	}

	span.SetStatus(codes2.Ok, "Search profile updated successfully")
	svc.logger.Info("Search profile updated successfully", zap.String("userID", req.UserId), zap.String("profileID", req.ProfileId))

	return &pb.UpdateSearchProfileResponse{
		Success: true,
		Message: "search profile updated successfully",
		Profile: convertToPBProfile(updatedProfile),
		Response: &pb.BaseResponse{
			RequestId: requestId,
			Upstream:  "profiles-service",
			Status:    "success",
		},
	}, nil
}

func (svc *Service) DeleteSearchProfile(ctx context.Context, req *pb.DeleteSearchProfileRequest) (*pb.DeleteSearchProfileResponse, error) {
	ctx, span := svc.tracer.Start(ctx, "ProfilesService.DeleteSearchProfile", trace.WithAttributes(
		attribute.String("profiles.operation", "delete_search_profile"),
		attribute.String("profiles.user_id", req.UserId),
		attribute.String("profiles.profile_id", req.ProfileId),
	))
	defer span.End()

	userID, err := domain.CheckUserAuth(ctx)
	if err != nil {
		return nil, err
	}

	userUUID, err := uuid.Parse(userID)
	if err != nil {
		svc.logger.Error("Invalid user ID format", zap.Error(err))
		return &pb.DeleteSearchProfileResponse{}, status.Error(codes.InvalidArgument, "invalid user ID format")
	}

	svc.logger.Debug("Deleting search profile", zap.String("userID", req.UserId), zap.String("profileID", req.ProfileId))

	var requestId string
	if req.Request != nil {
		requestId = req.Request.RequestId
	}

	req.UserId = userID

	profileID, err := uuid.Parse(req.ProfileId)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes2.Error, "Invalid profile ID format")
		svc.logger.Error("Invalid profile ID format", zap.Error(err))
		return &pb.DeleteSearchProfileResponse{
			Success: false,
			Message: "invalid profile ID format",
			Response: &pb.BaseResponse{
				RequestId: requestId,
				Upstream:  "profiles-service",
				Status:    "error",
			},
		}, status.Error(grpcCodes.InvalidArgument, "invalid profile ID format")
	}

	err = svc.repo.DeleteSearchProfile(ctx, userUUID, profileID)
	if err != nil {
		span.RecordError(err)
		if errors.Is(err, domain.ErrNotFound) {
			span.SetStatus(codes2.Error, "Profile not found")
			svc.logger.Warn("Search profile not found for deletion", zap.String("userID", req.UserId), zap.String("profileID", req.ProfileId))
			return &pb.DeleteSearchProfileResponse{
				Success: false,
				Message: "search profile not found",
				Response: &pb.BaseResponse{
					RequestId: requestId,
					Upstream:  "profiles-service",
					Status:    "error",
				},
			}, status.Error(grpcCodes.NotFound, "search profile not found")
		}
		// Check for cannot delete default profile error
		if strings.Contains(err.Error(), "cannot delete default profile") {
			span.SetStatus(codes2.Error, "Cannot delete default profile")
			svc.logger.Warn("Attempted to delete default profile", zap.String("userID", req.UserId), zap.String("profileID", req.ProfileId))
			return &pb.DeleteSearchProfileResponse{
				Success: false,
				Message: "cannot delete default profile",
				Response: &pb.BaseResponse{
					RequestId: requestId,
					Upstream:  "profiles-service",
					Status:    "error",
				},
			}, status.Error(grpcCodes.FailedPrecondition, "cannot delete default profile")
		}
		span.SetStatus(codes2.Error, "Database error")
		svc.logger.Error("Failed to delete search profile", zap.Error(err))
		return &pb.DeleteSearchProfileResponse{
			Success: false,
			Message: "failed to delete search profile",
			Response: &pb.BaseResponse{
				RequestId: requestId,
				Upstream:  "profiles-service",
				Status:    "error",
			},
		}, status.Error(grpcCodes.Internal, "failed to delete search profile")
	}

	span.SetStatus(codes2.Ok, "Search profile deleted successfully")
	svc.logger.Info("Search profile deleted successfully", zap.String("userID", req.UserId), zap.String("profileID", req.ProfileId))

	return &pb.DeleteSearchProfileResponse{
		Success: true,
		Message: "search profile deleted successfully",
		Response: &pb.BaseResponse{
			RequestId: requestId,
			Upstream:  "profiles-service",
			Status:    "success",
		},
	}, nil
}

func (svc *Service) SetDefaultSearchProfile(ctx context.Context, req *pb.SetDefaultSearchProfileRequest) (*pb.SetDefaultSearchProfileResponse, error) {
	ctx, span := svc.tracer.Start(ctx, "ProfilesService.SetDefaultSearchProfile", trace.WithAttributes(
		attribute.String("profiles.operation", "set_default_search_profile"),
		attribute.String("profiles.user_id", req.UserId),
		attribute.String("profiles.profile_id", req.ProfileId),
	))
	defer span.End()

	userID, err := domain.CheckUserAuth(ctx)
	if err != nil {
		return nil, err
	}

	userUUID, err := uuid.Parse(userID)
	if err != nil {
		svc.logger.Error("Invalid user ID format", zap.Error(err))
		return &pb.SetDefaultSearchProfileResponse{}, status.Error(codes.InvalidArgument, "invalid user ID format")
	}

	svc.logger.Debug("Setting default search profile", zap.String("userID", req.UserId), zap.String("profileID", req.ProfileId))

	var requestId string
	if req.Request != nil {
		requestId = req.Request.RequestId
	}

	profileID, err := uuid.Parse(req.ProfileId)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes2.Error, "Invalid profile ID format")
		svc.logger.Error("Invalid profile ID format", zap.Error(err))
		return &pb.SetDefaultSearchProfileResponse{
			Success: false,
			Message: "invalid profile ID format",
			Response: &pb.BaseResponse{
				RequestId: requestId,
				Upstream:  "profiles-service",
				Status:    "error",
			},
		}, status.Error(grpcCodes.InvalidArgument, "invalid profile ID format")
	}

	err = svc.repo.SetDefaultSearchProfile(ctx, userUUID, profileID)
	if err != nil {
		span.RecordError(err)
		if errors.Is(err, domain.ErrNotFound) {
			span.SetStatus(codes2.Error, "Profile not found")
			svc.logger.Warn("Search profile not found for setting as default", zap.String("userID", req.UserId), zap.String("profileID", req.ProfileId))
			return &pb.SetDefaultSearchProfileResponse{
				Success: false,
				Message: "search profile not found",
				Response: &pb.BaseResponse{
					RequestId: requestId,
					Upstream:  "profiles-service",
					Status:    "error",
				},
			}, status.Error(grpcCodes.NotFound, "search profile not found")
		}
		span.SetStatus(codes2.Error, "Database error")
		svc.logger.Error("Failed to set default search profile", zap.Error(err))
		return &pb.SetDefaultSearchProfileResponse{
			Success: false,
			Message: "failed to set default search profile",
			Response: &pb.BaseResponse{
				RequestId: requestId,
				Upstream:  "profiles-service",
				Status:    "error",
			},
		}, status.Error(grpcCodes.Internal, "failed to set default search profile")
	}

	span.SetStatus(codes2.Ok, "Default search profile set successfully")
	svc.logger.Info("Default search profile set successfully", zap.String("userID", req.UserId), zap.String("profileID", req.ProfileId))

	return &pb.SetDefaultSearchProfileResponse{
		Success: true,
		Message: "default search profile set successfully",
		Response: &pb.BaseResponse{
			RequestId: requestId,
			Upstream:  "profiles-service",
			Status:    "success",
		},
	}, nil
}

// Converter functions

func convertToPBProfile(profile *UserPreferenceProfileResponse) *pb.UserPreferenceProfile {
	pbProfile := &pb.UserPreferenceProfile{
		Id:                   profile.ID.String(),
		UserId:               profile.UserID.String(),
		ProfileName:          profile.ProfileName,
		IsDefault:            profile.IsDefault,
		SearchRadiusKm:       profile.SearchRadiusKm,
		PreferredTime:        convertToPBDayPreference(profile.PreferredTime),
		BudgetLevel:          int32(profile.BudgetLevel),
		PreferredPace:        convertToPBSearchPace(profile.PreferredPace),
		PreferAccessiblePois: profile.PreferAccessiblePOIs,
		PreferOutdoorSeating: profile.PreferOutdoorSeating,
		PreferDogFriendly:    profile.PreferDogFriendly,
		PreferredVibes:       profile.PreferredVibes,
		PreferredTransport:   convertToPBTransportPreference(profile.PreferredTransport),
		DietaryNeeds:         profile.DietaryNeeds,
		CreatedAt:            timestamppb.New(profile.CreatedAt),
		UpdatedAt:            timestamppb.New(profile.UpdatedAt),
	}

	// Convert interests
	if profile.Interests != nil {
		pbProfile.Interests = make([]*pb.InterestReference, len(profile.Interests))
		for i, interest := range profile.Interests {
			pbProfile.Interests[i] = &pb.InterestReference{
				Id:   interest.ID.String(),
				Name: interest.Name,
			}
		}
	}

	// Convert tags
	if profile.Tags != nil {
		pbProfile.Tags = make([]*pb.TagReference, len(profile.Tags))
		for i, tag := range profile.Tags {
			pbProfile.Tags[i] = &pb.TagReference{
				Id:      tag.ID.String(),
				Name:    tag.Name,
				TagType: tag.TagType,
			}
		}
	}

	// Convert user coordinates
	if profile.UserLatitude != nil {
		pbProfile.UserLatitude = *profile.UserLatitude
	}
	if profile.UserLongitude != nil {
		pbProfile.UserLongitude = *profile.UserLongitude
	}

	// Convert domain-specific preferences
	if profile.AccommodationPreferences != nil {
		pbProfile.AccommodationPreferences = convertToPBAccommodationPreferences(profile.AccommodationPreferences)
	}
	if profile.DiningPreferences != nil {
		pbProfile.DiningPreferences = convertToPBDiningPreferences(profile.DiningPreferences)
	}
	if profile.ActivityPreferences != nil {
		pbProfile.ActivityPreferences = convertToPBActivityPreferences(profile.ActivityPreferences)
	}
	if profile.ItineraryPreferences != nil {
		pbProfile.ItineraryPreferences = convertToPBItineraryPreferences(profile.ItineraryPreferences)
	}

	return pbProfile
}

func convertPBToCreateParams(pbProfile *pb.CreateUserPreferenceProfileParams) CreateUserPreferenceProfileParams {
	params := CreateUserPreferenceProfileParams{
		ProfileName: pbProfile.ProfileName,
	}

	// Convert optional fields
	if pbProfile.IsDefault {
		params.IsDefault = &pbProfile.IsDefault
	}
	if pbProfile.SearchRadiusKm != 0 {
		params.SearchRadiusKm = &pbProfile.SearchRadiusKm
	}
	if pbProfile.PreferredTime != pb.DayPreference_DAY_PREFERENCE_UNSPECIFIED {
		dayPref := convertFromPBDayPreference(pbProfile.PreferredTime)
		params.PreferredTime = &dayPref
	}
	if pbProfile.BudgetLevel != 0 {
		budgetLevel := int(pbProfile.BudgetLevel)
		params.BudgetLevel = &budgetLevel
	}
	if pbProfile.PreferredPace != pb.SearchPace_SEARCH_PACE_UNSPECIFIED {
		pace := convertFromPBSearchPace(pbProfile.PreferredPace)
		params.PreferredPace = &pace
	}
	if pbProfile.PreferAccessiblePois {
		params.PreferAccessiblePOIs = &pbProfile.PreferAccessiblePois
	}
	if pbProfile.PreferOutdoorSeating {
		params.PreferOutdoorSeating = &pbProfile.PreferOutdoorSeating
	}
	if pbProfile.PreferDogFriendly {
		params.PreferDogFriendly = &pbProfile.PreferDogFriendly
	}
	if len(pbProfile.PreferredVibes) > 0 {
		params.PreferredVibes = pbProfile.PreferredVibes
	}
	if pbProfile.PreferredTransport != pb.TransportPreference_TRANSPORT_PREFERENCE_UNSPECIFIED {
		transport := convertFromPBTransportPreference(pbProfile.PreferredTransport)
		params.PreferredTransport = &transport
	}
	if len(pbProfile.DietaryNeeds) > 0 {
		params.DietaryNeeds = pbProfile.DietaryNeeds
	}

	// Convert domain-specific preferences
	if pbProfile.AccommodationPreferences != nil {
		params.AccommodationPreferences = convertFromPBAccommodationPreferences(pbProfile.AccommodationPreferences)
	}
	if pbProfile.DiningPreferences != nil {
		params.DiningPreferences = convertFromPBDiningPreferences(pbProfile.DiningPreferences)
	}
	if pbProfile.ActivityPreferences != nil {
		params.ActivityPreferences = convertFromPBActivityPreferences(pbProfile.ActivityPreferences)
	}
	if pbProfile.ItineraryPreferences != nil {
		params.ItineraryPreferences = convertFromPBItineraryPreferences(pbProfile.ItineraryPreferences)
	}

	return params
}

func convertPBToUpdateParams(pbProfile *pb.UpdateSearchProfileParams) UpdateSearchProfileParams {
	params := UpdateSearchProfileParams{
		ProfileName: pbProfile.ProfileName,
	}

	// Convert optional fields
	if pbProfile.IsDefault {
		params.IsDefault = &pbProfile.IsDefault
	}
	if pbProfile.SearchRadiusKm != 0 {
		params.SearchRadiusKm = &pbProfile.SearchRadiusKm
	}
	if pbProfile.PreferredTime != pb.DayPreference_DAY_PREFERENCE_UNSPECIFIED {
		dayPref := convertFromPBDayPreference(pbProfile.PreferredTime)
		params.PreferredTime = &dayPref
	}
	if pbProfile.BudgetLevel != 0 {
		budgetLevel := int(pbProfile.BudgetLevel)
		params.BudgetLevel = &budgetLevel
	}
	if pbProfile.PreferredPace != pb.SearchPace_SEARCH_PACE_UNSPECIFIED {
		pace := convertFromPBSearchPace(pbProfile.PreferredPace)
		params.PreferredPace = &pace
	}
	if pbProfile.PreferAccessiblePois {
		params.PreferAccessiblePOIs = &pbProfile.PreferAccessiblePois
	}
	if pbProfile.PreferOutdoorSeating {
		params.PreferOutdoorSeating = &pbProfile.PreferOutdoorSeating
	}
	if pbProfile.PreferDogFriendly {
		params.PreferDogFriendly = &pbProfile.PreferDogFriendly
	}
	if len(pbProfile.PreferredVibes) > 0 {
		params.PreferredVibes = pbProfile.PreferredVibes
	}
	if pbProfile.PreferredTransport != pb.TransportPreference_TRANSPORT_PREFERENCE_UNSPECIFIED {
		transport := convertFromPBTransportPreference(pbProfile.PreferredTransport)
		params.PreferredTransport = &transport
	}
	if len(pbProfile.DietaryNeeds) > 0 {
		params.DietaryNeeds = pbProfile.DietaryNeeds
	}

	// Convert domain-specific preferences
	if pbProfile.AccommodationPreferences != nil {
		params.AccommodationPreferences = convertFromPBAccommodationPreferences(pbProfile.AccommodationPreferences)
	}
	if pbProfile.DiningPreferences != nil {
		params.DiningPreferences = convertFromPBDiningPreferences(pbProfile.DiningPreferences)
	}
	if pbProfile.ActivityPreferences != nil {
		params.ActivityPreferences = convertFromPBActivityPreferences(pbProfile.ActivityPreferences)
	}
	if pbProfile.ItineraryPreferences != nil {
		params.ItineraryPreferences = convertFromPBItineraryPreferences(pbProfile.ItineraryPreferences)
	}

	return params
}

// Enum converters
func convertToPBDayPreference(pref DayPreference) pb.DayPreference {
	switch pref {
	case DayPreferenceAny:
		return pb.DayPreference_DAY_PREFERENCE_ANY
	case DayPreferenceDay:
		return pb.DayPreference_DAY_PREFERENCE_DAY
	case DayPreferenceNight:
		return pb.DayPreference_DAY_PREFERENCE_NIGHT
	default:
		return pb.DayPreference_DAY_PREFERENCE_UNSPECIFIED
	}
}

func convertFromPBDayPreference(pref pb.DayPreference) DayPreference {
	switch pref {
	case pb.DayPreference_DAY_PREFERENCE_ANY:
		return DayPreferenceAny
	case pb.DayPreference_DAY_PREFERENCE_DAY:
		return DayPreferenceDay
	case pb.DayPreference_DAY_PREFERENCE_NIGHT:
		return DayPreferenceNight
	default:
		return DayPreferenceAny
	}
}

func convertToPBSearchPace(pace SearchPace) pb.SearchPace {
	switch pace {
	case SearchPaceAny:
		return pb.SearchPace_SEARCH_PACE_ANY
	case SearchPaceRelaxed:
		return pb.SearchPace_SEARCH_PACE_RELAXED
	case SearchPaceModerate:
		return pb.SearchPace_SEARCH_PACE_MODERATE
	case SearchPaceFast:
		return pb.SearchPace_SEARCH_PACE_FAST
	default:
		return pb.SearchPace_SEARCH_PACE_UNSPECIFIED
	}
}

func convertFromPBSearchPace(pace pb.SearchPace) SearchPace {
	switch pace {
	case pb.SearchPace_SEARCH_PACE_ANY:
		return SearchPaceAny
	case pb.SearchPace_SEARCH_PACE_RELAXED:
		return SearchPaceRelaxed
	case pb.SearchPace_SEARCH_PACE_MODERATE:
		return SearchPaceModerate
	case pb.SearchPace_SEARCH_PACE_FAST:
		return SearchPaceFast
	default:
		return SearchPaceAny
	}
}

func convertToPBTransportPreference(transport TransportPreference) pb.TransportPreference {
	switch transport {
	case TransportPreferenceAny:
		return pb.TransportPreference_TRANSPORT_PREFERENCE_ANY
	case TransportPreferenceWalk:
		return pb.TransportPreference_TRANSPORT_PREFERENCE_WALK
	case TransportPreferencePublic:
		return pb.TransportPreference_TRANSPORT_PREFERENCE_PUBLIC
	case TransportPreferenceCar:
		return pb.TransportPreference_TRANSPORT_PREFERENCE_CAR
	default:
		return pb.TransportPreference_TRANSPORT_PREFERENCE_UNSPECIFIED
	}
}

func convertFromPBTransportPreference(transport pb.TransportPreference) TransportPreference {
	switch transport {
	case pb.TransportPreference_TRANSPORT_PREFERENCE_ANY:
		return TransportPreferenceAny
	case pb.TransportPreference_TRANSPORT_PREFERENCE_WALK:
		return TransportPreferenceWalk
	case pb.TransportPreference_TRANSPORT_PREFERENCE_PUBLIC:
		return TransportPreferencePublic
	case pb.TransportPreference_TRANSPORT_PREFERENCE_CAR:
		return TransportPreferenceCar
	default:
		return TransportPreferenceAny
	}
}

// Domain-specific preference converters
func convertToPBAccommodationPreferences(prefs *AccommodationPreferences) *pb.AccommodationPreferences {
	pbPrefs := &pb.AccommodationPreferences{
		Id:                      prefs.ID.String(),
		UserPreferenceProfileId: prefs.UserPreferenceID.String(),
		AccommodationType:       prefs.AccommodationType,
		Amenities:               prefs.Amenities,
		RoomType:                prefs.RoomType,
		ChainPreference:         prefs.ChainPreference,
		CancellationPolicy:      prefs.CancellationPolicy,
		BookingFlexibility:      prefs.BookingFlexibility,
		CreatedAt:               timestamppb.New(prefs.CreatedAt),
		UpdatedAt:               timestamppb.New(prefs.UpdatedAt),
	}

	if prefs.StarRating != nil {
		pbPrefs.StarRating = &pb.RangeFilter{}
		if prefs.StarRating.Min != nil {
			pbPrefs.StarRating.Min = *prefs.StarRating.Min
		}
		if prefs.StarRating.Max != nil {
			pbPrefs.StarRating.Max = *prefs.StarRating.Max
		}
	}

	if prefs.PriceRangePerNight != nil {
		pbPrefs.PriceRangePerNight = &pb.RangeFilter{}
		if prefs.PriceRangePerNight.Min != nil {
			pbPrefs.PriceRangePerNight.Min = *prefs.PriceRangePerNight.Min
		}
		if prefs.PriceRangePerNight.Max != nil {
			pbPrefs.PriceRangePerNight.Max = *prefs.PriceRangePerNight.Max
		}
	}

	return pbPrefs
}

func convertFromPBAccommodationPreferences(pbPrefs *pb.AccommodationPreferences) *AccommodationPreferences {
	prefs := &AccommodationPreferences{
		AccommodationType:  pbPrefs.AccommodationType,
		Amenities:          pbPrefs.Amenities,
		RoomType:           pbPrefs.RoomType,
		ChainPreference:    pbPrefs.ChainPreference,
		CancellationPolicy: pbPrefs.CancellationPolicy,
		BookingFlexibility: pbPrefs.BookingFlexibility,
	}

	if pbPrefs.StarRating != nil {
		prefs.StarRating = &RangeFilter{}
		if pbPrefs.StarRating.Min != 0 {
			prefs.StarRating.Min = &pbPrefs.StarRating.Min
		}
		if pbPrefs.StarRating.Max != 0 {
			prefs.StarRating.Max = &pbPrefs.StarRating.Max
		}
	}

	if pbPrefs.PriceRangePerNight != nil {
		prefs.PriceRangePerNight = &RangeFilter{}
		if pbPrefs.PriceRangePerNight.Min != 0 {
			prefs.PriceRangePerNight.Min = &pbPrefs.PriceRangePerNight.Min
		}
		if pbPrefs.PriceRangePerNight.Max != 0 {
			prefs.PriceRangePerNight.Max = &pbPrefs.PriceRangePerNight.Max
		}
	}

	return prefs
}

func convertToPBDiningPreferences(prefs *DiningPreferences) *pb.DiningPreferences {
	pbPrefs := &pb.DiningPreferences{
		Id:                      prefs.ID.String(),
		UserPreferenceProfileId: prefs.UserPreferenceID.String(),
		CuisineTypes:            prefs.CuisineTypes,
		MealTypes:               prefs.MealTypes,
		ServiceStyle:            prefs.ServiceStyle,
		DietaryNeeds:            prefs.DietaryNeeds,
		AllergenFree:            prefs.AllergenFree,
		MichelinRated:           prefs.MichelinRated,
		LocalRecommendations:    prefs.LocalRecommendations,
		ChainVsLocal:            prefs.ChainVsLocal,
		OrganicPreference:       prefs.OrganicPreference,
		OutdoorSeatingPreferred: prefs.OutdoorSeatingPref,
		CreatedAt:               timestamppb.New(prefs.CreatedAt),
		UpdatedAt:               timestamppb.New(prefs.UpdatedAt),
	}

	if prefs.PriceRangePerPerson != nil {
		pbPrefs.PriceRangePerPerson = &pb.RangeFilter{}
		if prefs.PriceRangePerPerson.Min != nil {
			pbPrefs.PriceRangePerPerson.Min = *prefs.PriceRangePerPerson.Min
		}
		if prefs.PriceRangePerPerson.Max != nil {
			pbPrefs.PriceRangePerPerson.Max = *prefs.PriceRangePerPerson.Max
		}
	}

	return pbPrefs
}

func convertFromPBDiningPreferences(pbPrefs *pb.DiningPreferences) *DiningPreferences {
	prefs := &DiningPreferences{
		CuisineTypes:         pbPrefs.CuisineTypes,
		MealTypes:            pbPrefs.MealTypes,
		ServiceStyle:         pbPrefs.ServiceStyle,
		DietaryNeeds:         pbPrefs.DietaryNeeds,
		AllergenFree:         pbPrefs.AllergenFree,
		MichelinRated:        pbPrefs.MichelinRated,
		LocalRecommendations: pbPrefs.LocalRecommendations,
		ChainVsLocal:         pbPrefs.ChainVsLocal,
		OrganicPreference:    pbPrefs.OrganicPreference,
		OutdoorSeatingPref:   pbPrefs.OutdoorSeatingPreferred,
	}

	if pbPrefs.PriceRangePerPerson != nil {
		prefs.PriceRangePerPerson = &RangeFilter{}
		if pbPrefs.PriceRangePerPerson.Min != 0 {
			prefs.PriceRangePerPerson.Min = &pbPrefs.PriceRangePerPerson.Min
		}
		if pbPrefs.PriceRangePerPerson.Max != 0 {
			prefs.PriceRangePerPerson.Max = &pbPrefs.PriceRangePerPerson.Max
		}
	}

	return prefs
}

func convertToPBActivityPreferences(prefs *ActivityPreferences) *pb.ActivityPreferences {
	return &pb.ActivityPreferences{
		Id:                       prefs.ID.String(),
		UserPreferenceProfileId:  prefs.UserPreferenceID.String(),
		ActivityCategories:       prefs.ActivityCategories,
		PhysicalActivityLevel:    prefs.PhysicalActivityLevel,
		IndoorOutdoorPreference:  prefs.IndoorOutdoorPref,
		CulturalImmersionLevel:   prefs.CulturalImmersionLevel,
		MustSeeVsHiddenGems:      prefs.MustSeeVsHiddenGems,
		EducationalPreference:    prefs.EducationalPreference,
		PhotographyOpportunities: prefs.PhotoOpportunities,
		SeasonSpecificActivities: prefs.SeasonSpecific,
		AvoidCrowds:              prefs.AvoidCrowds,
		LocalEventsInterest:      prefs.LocalEventsInterest,
		CreatedAt:                timestamppb.New(prefs.CreatedAt),
		UpdatedAt:                timestamppb.New(prefs.UpdatedAt),
	}
}

func convertFromPBActivityPreferences(pbPrefs *pb.ActivityPreferences) *ActivityPreferences {
	return &ActivityPreferences{
		ActivityCategories:     pbPrefs.ActivityCategories,
		PhysicalActivityLevel:  pbPrefs.PhysicalActivityLevel,
		IndoorOutdoorPref:      pbPrefs.IndoorOutdoorPreference,
		CulturalImmersionLevel: pbPrefs.CulturalImmersionLevel,
		MustSeeVsHiddenGems:    pbPrefs.MustSeeVsHiddenGems,
		EducationalPreference:  pbPrefs.EducationalPreference,
		PhotoOpportunities:     pbPrefs.PhotographyOpportunities,
		SeasonSpecific:         pbPrefs.SeasonSpecificActivities,
		AvoidCrowds:            pbPrefs.AvoidCrowds,
		LocalEventsInterest:    pbPrefs.LocalEventsInterest,
	}
}

func convertToPBItineraryPreferences(prefs *ItineraryPreferences) *pb.ItineraryPreferences {
	return &pb.ItineraryPreferences{
		Id:                      prefs.ID.String(),
		UserPreferenceProfileId: prefs.UserPreferenceID.String(),
		PlanningStyle:           prefs.PlanningStyle,
		PreferredPace:           prefs.PreferredPace,
		TimeFlexibility:         prefs.TimeFlexibility,
		MorningVsEvening:        prefs.MorningVsEvening,
		WeekendVsWeekday:        prefs.WeekendVsWeekday,
		PreferredSeasons:        prefs.PreferredSeasons,
		AvoidPeakSeason:         prefs.AvoidPeakSeason,
		AdventureVsRelaxation:   prefs.AdventureVsRelaxation,
		SpontaneousVsPlanned:    prefs.SpontaneousVsPlanned,
		CreatedAt:               timestamppb.New(prefs.CreatedAt),
		UpdatedAt:               timestamppb.New(prefs.UpdatedAt),
	}
}

func convertFromPBItineraryPreferences(pbPrefs *pb.ItineraryPreferences) *ItineraryPreferences {
	return &ItineraryPreferences{
		PlanningStyle:         pbPrefs.PlanningStyle,
		PreferredPace:         pbPrefs.PreferredPace,
		TimeFlexibility:       pbPrefs.TimeFlexibility,
		MorningVsEvening:      pbPrefs.MorningVsEvening,
		WeekendVsWeekday:      pbPrefs.WeekendVsWeekday,
		PreferredSeasons:      pbPrefs.PreferredSeasons,
		AvoidPeakSeason:       pbPrefs.AvoidPeakSeason,
		AdventureVsRelaxation: pbPrefs.AdventureVsRelaxation,
		SpontaneousVsPlanned:  pbPrefs.SpontaneousVsPlanned,
	}
}
