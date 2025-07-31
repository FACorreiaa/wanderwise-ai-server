package user

import (
	"context"
	"time"

	pb "github.com/FACorreiaa/loci-proto/modules/user/generated"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/FACorreiaa/go-poi-au-suggestions/app/observability/metrics"
	"github.com/FACorreiaa/go-poi-au-suggestions/internal/domain"
	"github.com/FACorreiaa/go-poi-au-suggestions/internal/types"
	"github.com/FACorreiaa/go-poi-au-suggestions/protocol/grpc/middleware/grpcrequest"
)

type Service struct {
	pb.UnimplementedUserServiceServer
	logger *zap.Logger
	repo   Repository
	pgpool *pgxpool.Pool
	tracer trace.Tracer
}

func NewService(ctx context.Context, repo Repository, pgpool *pgxpool.Pool, logger *zap.Logger) *Service {
	return &Service{
		logger: logger.With(zap.String("service", "auth")),
		repo:   repo,
		pgpool: pgpool,
		tracer: otel.Tracer("AuthService"),
	}
}

func (svc *Service) UpdateUserProfile(ctx context.Context, req *pb.UpdateUserProfileRequest) (*pb.UpdateUserProfileResponse, error) {
	ctx, span := svc.tracer.Start(ctx, "UserService.UpdateUserProfile", trace.WithAttributes(
		attribute.String("user.operation", "update_profile"),
		attribute.String("user.id", req.UserId),
	))
	defer span.End()

	requestID, ok := ctx.Value(grpcrequest.RequestIDKey{}).(string)
	if !ok {
		return nil, status.Error(codes.Internal, "request id not found in context")
	}

	if req.Request == nil {
		req.Request = &pb.BaseRequest{}
	}

	userID, err := domain.CheckUserAuth(ctx)
	if err != nil {
		return nil, err
	}

	req.Request.RequestId = requestID
	req.UserId = userID

	startTime := time.Now()
	defer func() {
		metrics.Get().RegisterDurationSeconds.Record(ctx, time.Since(startTime).Seconds())
	}()

	svc.logger.Info("User profile update attempt",
		zap.String("user_id", req.UserId))

	// Validate request
	if req.Profile == nil {
		return &pb.UpdateUserProfileResponse{
			Success: false,
			Message: "Profile data is required",
		}, status.Error(codes.InvalidArgument, "profile data is required")
	}

	// Convert protobuf profile to domain types
	params := convertPbProfileToUpdateParams(req.Profile, req.UpdateFields)

	// Parse userID to UUID
	userUUID, err := uuid.Parse(userID)
	if err != nil {
		svc.logger.Error("Invalid user ID format", zap.Error(err))
		return &pb.UpdateUserProfileResponse{
			Success: false,
			Message: "Invalid user ID format",
		}, status.Error(codes.InvalidArgument, "invalid user ID format")
	}

	err = svc.repo.UpdateProfile(ctx, userUUID, params)
	if err != nil {
		svc.logger.Error("Failed to update user profile",
			zap.String("user_id", userID),
			zap.Error(err))
		return &pb.UpdateUserProfileResponse{
			Success: false,
			Message: "Failed to update profile",
			Response: &pb.BaseResponse{
				Upstream:  "user-service",
				RequestId: requestID,
			},
		}, status.Error(codes.Internal, "failed to update profile")
	}

	// Fetch updated profile
	updatedProfile, err := svc.repo.GetUserByID(ctx, userUUID)
	if err != nil {
		svc.logger.Error("Failed to fetch updated profile",
			zap.String("user_id", userID),
			zap.Error(err))
		// Return success for the update but without the updated profile
		return &pb.UpdateUserProfileResponse{
			Success: true,
			Message: "Profile updated successfully",
		}, nil
	}

	// Convert domain profile to protobuf
	pbProfile := convertDomainProfileToPb(updatedProfile)

	svc.logger.Info("User profile updated successfully",
		zap.String("user_id", userID))

	span.SetAttributes(
		attribute.String("request.id", req.Request.RequestId),
		attribute.String("request.details", req.String()),
	)

	return &pb.UpdateUserProfileResponse{
		Success: true,
		Message: "Profile updated successfully",
		Profile: pbProfile,
		Response: &pb.BaseResponse{
			Upstream:  "user-service",
			RequestId: requestID,
		},
	}, nil
}

func (svc *Service) GetUserProfile(ctx context.Context, req *pb.GetUserProfileRequest) (*pb.GetUserProfileResponse, error) {
	ctx, span := svc.tracer.Start(ctx, "UserService.UpdateUserProfile", trace.WithAttributes(
		attribute.String("user.operation", "update_profile"),
		attribute.String("user.id", req.UserId),
	))
	defer span.End()

	requestID, ok := ctx.Value(grpcrequest.RequestIDKey{}).(string)
	if !ok {
		return nil, status.Error(codes.Internal, "request id not found in context")
	}

	if req.Request == nil {
		req.Request = &pb.BaseRequest{}
	}

	userID, err := domain.CheckUserAuth(ctx)
	if err != nil {
		return nil, err
	}

	req.Request.RequestId = requestID
	req.UserId = userID

	startTime := time.Now()
	defer func() {
		metrics.Get().RegisterDurationSeconds.Record(ctx, time.Since(startTime).Seconds())
	}()

	svc.logger.Info("User profile update attempt",
		zap.String("user_id", req.UserId))

	if req.UserId == "" {
		return &pb.GetUserProfileResponse{
			Response: &pb.BaseResponse{
				Upstream:  "user-service",
				RequestId: requestID,
			},
		}, status.Error(codes.Unauthenticated, "userID is missing in metadata")
	}

	userUUID, err := uuid.Parse(userID)
	if err != nil {
		svc.logger.Error("Invalid user ID format", zap.Error(err))
		return &pb.GetUserProfileResponse{}, status.Error(codes.InvalidArgument, "invalid user ID format")
	}

	updatedProfile, err := svc.repo.GetUserByID(ctx, userUUID)
	if err != nil {
		svc.logger.Error("Failed to fetch updated profile",
			zap.String("user_id", userID),
			zap.Error(err))
		// Return success for the update but without the updated profile
		return &pb.GetUserProfileResponse{}, nil
	}

	pbProfile := convertDomainProfileToPb(updatedProfile)

	return &pb.GetUserProfileResponse{
		Profile: pbProfile,
		Response: &pb.BaseResponse{
			Upstream:  "user-service",
			RequestId: requestID,
		},
	}, nil
}

// convertPbProfileToUpdateParams converts protobuf UserProfile to domain UpdateProfileParams
// Uses updateFields as a field mask to determine which fields should be updated
func convertPbProfileToUpdateParams(profile *pb.UserProfile, updateFields []string) types.UpdateProfileParams {
	params := types.UpdateProfileParams{}

	// Create a map for faster field lookup
	fieldMap := make(map[string]bool)
	for _, field := range updateFields {
		fieldMap[field] = true
	}

	// Only set fields that are in the update mask or if no mask is provided (update all)
	if len(updateFields) == 0 || fieldMap["username"] {
		if profile.Username != "" {
			params.Username = &profile.Username
		}
	}

	if len(updateFields) == 0 || fieldMap["email"] {
		if profile.Email != "" {
			params.Email = &profile.Email
		}
	}

	if len(updateFields) == 0 || fieldMap["first_name"] {
		if profile.FirstName != "" {
			params.Firstname = &profile.FirstName
		}
	}

	if len(updateFields) == 0 || fieldMap["last_name"] {
		if profile.LastName != "" {
			params.Lastname = &profile.LastName
		}
	}

	if len(updateFields) == 0 || fieldMap["phone"] {
		if profile.Phone != "" {
			params.PhoneNumber = &profile.Phone
		}
	}

	if len(updateFields) == 0 || fieldMap["bio"] {
		if profile.Bio != "" {
			params.AboutYou = &profile.Bio
		}
	}

	if len(updateFields) == 0 || fieldMap["avatar_url"] {
		if profile.AvatarUrl != "" {
			params.ProfileImageURL = &profile.AvatarUrl
		}
	}

	if len(updateFields) == 0 || fieldMap["location"] {
		if profile.Location != "" {
			params.Location = &profile.Location
		}
	}

	return params
}

// convertDomainProfileToPb converts domain UserProfile to protobuf UserProfile
func convertDomainProfileToPb(profile *types.UserProfile) *pb.UserProfile {
	pbProfile := &pb.UserProfile{
		Id:            profile.ID.String(),
		Email:         profile.Email,
		EmailVerified: profile.EmailVerifiedAt != nil,
		CreatedAt:     timestamppb.New(profile.CreatedAt),
		UpdatedAt:     timestamppb.New(profile.UpdatedAt),
	}

	// Handle optional string fields
	if profile.Username != nil {
		pbProfile.Username = *profile.Username
	}
	if profile.Firstname != nil {
		pbProfile.FirstName = *profile.Firstname
	}
	if profile.Lastname != nil {
		pbProfile.LastName = *profile.Lastname
	}
	if profile.PhoneNumber != nil {
		pbProfile.Phone = *profile.PhoneNumber
	}
	if profile.AboutYou != nil {
		pbProfile.Bio = *profile.AboutYou
	}
	if profile.ProfileImageURL != nil {
		pbProfile.AvatarUrl = *profile.ProfileImageURL
	}
	if profile.Location != nil {
		pbProfile.Location = *profile.Location
	}
	if profile.Language != nil {
		pbProfile.Language = *profile.Language
	}

	// Handle phone verification
	pbProfile.PhoneVerified = profile.PhoneNumber != nil && *profile.PhoneNumber != ""

	return pbProfile
}
