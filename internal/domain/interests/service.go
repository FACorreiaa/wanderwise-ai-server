package interests

import (
	"context"

	pb "github.com/FACorreiaa/loci-proto/modules/interests/generated"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type Service struct {
	pb.UnimplementedInterestsServiceServer
	logger *zap.Logger
	repo   Repository
	pgpool *pgxpool.Pool
	tracer trace.Tracer
}

func NewService(ctx context.Context, repo Repository, pgpool *pgxpool.Pool, logger *zap.Logger) *Service {
	return &Service{
		logger: logger.With(zap.String("service", "tags")),
		repo:   repo,
		pgpool: pgpool,
		tracer: otel.Tracer("InterestsService"),
	}
}

func (svc *Service) GetAllInterests(ctx context.Context, req *pb.GetAllInterestsRequest) (*pb.GetAllInterestsResponse, error) {
	ctx, span := svc.tracer.Start(ctx, "GetAllInterests")
	defer span.End()

	svc.logger.Debug("Getting all interests")

	// Handle nil request
	var requestId string
	if req != nil && req.Request != nil {
		requestId = req.Request.RequestId
	}

	interests, err := svc.repo.GetAllInterests(ctx)
	if err != nil {
		svc.logger.Error("Failed to get all interests", zap.Error(err))
		return &pb.GetAllInterestsResponse{
			Interests: nil,
			Response: &pb.BaseResponse{
				RequestId: requestId,
				Upstream:  "interests-service",
				Status:    "error",
			},
		}, err
	}

	pbInterests := make([]*pb.Interest, len(interests))
	for i, interest := range interests {
		pbInterests[i] = convertToPBInterest(interest)
	}

	return &pb.GetAllInterestsResponse{
		Interests: pbInterests,
		Response: &pb.BaseResponse{
			RequestId: requestId,
			Upstream:  "interests-service",
			Status:    "success",
		},
	}, nil
}

func (svc *Service) CreateInterest(ctx context.Context, req *pb.CreateInterestRequest) (*pb.CreateInterestResponse, error) {
	ctx, span := svc.tracer.Start(ctx, "CreateInterest")
	defer span.End()

	// Handle nil request
	var requestId string
	if req != nil && req.Request != nil {
		requestId = req.Request.RequestId
	}

	if req == nil || req.Interest == nil {
		svc.logger.Error("Invalid request: missing interest data")
		return &pb.CreateInterestResponse{
			Success:  false,
			Message:  "Invalid request: missing interest data",
			Interest: nil,
			Response: &pb.BaseResponse{
				RequestId: requestId,
				Upstream:  "interests-service",
				Status:    "error",
			},
		}, nil
	}

	svc.logger.Debug("Creating interest", zap.String("name", req.Interest.Name))

	interest, err := svc.repo.CreateInterest(ctx, req.Interest.Name, &req.Interest.Description, req.Interest.Active, req.UserId)
	if err != nil {
		svc.logger.Error("Failed to create interest", zap.Error(err))
		return &pb.CreateInterestResponse{
			Success:  false,
			Message:  "Failed to create interest",
			Interest: nil,
			Response: &pb.BaseResponse{
				RequestId: requestId,
				Upstream:  "interests-service",
				Status:    "error",
			},
		}, err
	}

	return &pb.CreateInterestResponse{
		Success:  true,
		Message:  "Interest created successfully",
		Interest: convertToPBInterest(interest),
		Response: &pb.BaseResponse{
			RequestId: requestId,
			Upstream:  "interests-service",
			Status:    "success",
		},
	}, nil
}

func (svc *Service) UpdateInterest(ctx context.Context, req *pb.UpdateInterestRequest) (*pb.UpdateInterestResponse, error) {
	ctx, span := svc.tracer.Start(ctx, "UpdateInterest")
	defer span.End()

	// Handle nil request
	var requestId string
	if req != nil && req.Request != nil {
		requestId = req.Request.RequestId
	}

	if req == nil || req.Interest == nil {
		svc.logger.Error("Invalid request: missing interest data")
		return &pb.UpdateInterestResponse{
			Success:  false,
			Message:  "Invalid request: missing interest data",
			Interest: nil,
			Response: &pb.BaseResponse{
				RequestId: requestId,
				Upstream:  "interests-service",
				Status:    "error",
			},
		}, nil
	}

	svc.logger.Debug("Updating interest", zap.String("interest_id", req.InterestId))

	userID, err := uuid.Parse(req.UserId)
	if err != nil {
		svc.logger.Error("Invalid user ID", zap.Error(err))
		return &pb.UpdateInterestResponse{
			Success:  false,
			Message:  "Invalid user ID",
			Interest: nil,
			Response: &pb.BaseResponse{
				RequestId: requestId,
				Upstream:  "interests-service",
				Status:    "error",
			},
		}, err
	}

	interestID, err := uuid.Parse(req.InterestId)
	if err != nil {
		svc.logger.Error("Invalid interest ID", zap.Error(err))
		return &pb.UpdateInterestResponse{
			Success:  false,
			Message:  "Invalid interest ID",
			Interest: nil,
			Response: &pb.BaseResponse{
				RequestId: requestId,
				Upstream:  "interests-service",
				Status:    "error",
			},
		}, err
	}

	updateParams := UpdateinterestsParams{
		Name:        &req.Interest.Name,
		Description: &req.Interest.Description,
		Active:      &req.Interest.Active,
	}

	err = svc.repo.UpdateInterests(ctx, userID, interestID, updateParams)
	if err != nil {
		svc.logger.Error("Failed to update interest", zap.Error(err))
		return &pb.UpdateInterestResponse{
			Success:  false,
			Message:  "Failed to update interest",
			Interest: nil,
			Response: &pb.BaseResponse{
				RequestId: requestId,
				Upstream:  "interests-service",
				Status:    "error",
			},
		}, err
	}

	updatedInterest, err := svc.repo.GetInterest(ctx, interestID)
	if err != nil {
		svc.logger.Error("Failed to get updated interest", zap.Error(err))
		return &pb.UpdateInterestResponse{
			Success:  false,
			Message:  "Interest updated but failed to retrieve",
			Interest: nil,
			Response: &pb.BaseResponse{
				RequestId: requestId,
				Upstream:  "interests-service",
				Status:    "error",
			},
		}, err
	}

	return &pb.UpdateInterestResponse{
		Success:  true,
		Message:  "Interest updated successfully",
		Interest: convertToPBInterest(updatedInterest),
		Response: &pb.BaseResponse{
			RequestId: requestId,
			Upstream:  "interests-service",
			Status:    "success",
		},
	}, nil
}

func (svc *Service) RemoveInterest(ctx context.Context, req *pb.RemoveInterestRequest) (*pb.RemoveInterestResponse, error) {
	ctx, span := svc.tracer.Start(ctx, "RemoveInterest")
	defer span.End()

	// Handle nil request
	var requestId string
	if req != nil && req.Request != nil {
		requestId = req.Request.RequestId
	}

	if req == nil {
		svc.logger.Error("Invalid request: nil request")
		return &pb.RemoveInterestResponse{
			Success: false,
			Message: "Invalid request: nil request",
			Response: &pb.BaseResponse{
				RequestId: requestId,
				Upstream:  "interests-service",
				Status:    "error",
			},
		}, nil
	}

	svc.logger.Debug("Removing interest", zap.String("interest_id", req.InterestId))

	userID, err := uuid.Parse(req.UserId)
	if err != nil {
		svc.logger.Error("Invalid user ID", zap.Error(err))
		return &pb.RemoveInterestResponse{
			Success: false,
			Message: "Invalid user ID",
			Response: &pb.BaseResponse{
				RequestId: requestId,
				Upstream:  "interests-service",
				Status:    "error",
			},
		}, err
	}

	interestID, err := uuid.Parse(req.InterestId)
	if err != nil {
		svc.logger.Error("Invalid interest ID", zap.Error(err))
		return &pb.RemoveInterestResponse{
			Success: false,
			Message: "Invalid interest ID",
			Response: &pb.BaseResponse{
				RequestId: requestId,
				Upstream:  "interests-service",
				Status:    "error",
			},
		}, err
	}

	err = svc.repo.RemoveInterests(ctx, userID, interestID)
	if err != nil {
		svc.logger.Error("Failed to remove interest", zap.Error(err))
		return &pb.RemoveInterestResponse{
			Success: false,
			Message: "Failed to remove interest",
			Response: &pb.BaseResponse{
				RequestId: requestId,
				Upstream:  "interests-service",
				Status:    "error",
			},
		}, err
	}

	return &pb.RemoveInterestResponse{
		Success: true,
		Message: "Interest removed successfully",
		Response: &pb.BaseResponse{
			RequestId: requestId,
			Upstream:  "interests-service",
			Status:    "success",
		},
	}, nil
}

func convertToPBInterest(interest *Interest) *pb.Interest {
	pbInterest := &pb.Interest{
		Id:          interest.ID.String(),
		Name:        interest.Name,
		Description: "",
		Active:      false,
		Source:      interest.Source,
		CreatedAt:   timestamppb.New(interest.CreatedAt),
	}

	if interest.Description != nil {
		pbInterest.Description = *interest.Description
	}

	if interest.Active != nil {
		pbInterest.Active = *interest.Active
	}

	if interest.UpdatedAt != nil {
		pbInterest.UpdatedAt = timestamppb.New(*interest.UpdatedAt)
	}

	return pbInterest
}
