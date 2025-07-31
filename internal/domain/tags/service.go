package tags

import (
	"context"
	"time"

	pb "github.com/FACorreiaa/loci-proto/modules/tags/generated"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/FACorreiaa/go-poi-au-suggestions/app/observability/metrics"
	"github.com/FACorreiaa/go-poi-au-suggestions/internal/domain"
	"github.com/FACorreiaa/go-poi-au-suggestions/protocol/grpc/middleware/grpcrequest"
)

type Service struct {
	pb.UnimplementedTagsServiceServer
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
		tracer: otel.Tracer("AuthService"),
	}
}

func (svc *Service) GetTags(ctx context.Context, req *pb.GetTagsRequest) (*pb.GetTagsResponse, error) {
	ctx, span := svc.tracer.Start(ctx, "TagsService.GetTags", trace.WithAttributes(
		attribute.String("user.operation", "get_tags"),
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

	userUUID, err := uuid.Parse(userID)
	if err != nil {
		svc.logger.Error("Invalid user ID format", zap.Error(err))
		return &pb.GetTagsResponse{}, status.Error(codes.InvalidArgument, "invalid user ID format")
	}

	req.Request.RequestId = requestID
	req.UserId = userID

	startTime := time.Now()
	defer func() {
		metrics.Get().RegisterDurationSeconds.Record(ctx, time.Since(startTime).Seconds())
	}()

	svc.logger.Info("GetTags request received",
		zap.String("user_id", req.UserId))

	tags, err := svc.repo.GetAll(ctx, userUUID)
	if err != nil {
		svc.logger.Error("Failed to get tags", zap.Error(err))
		return &pb.GetTagsResponse{}, status.Error(codes.Internal, "failed to get tags")
	}

	protoTags := make([]*pb.PersonalTag, 0, len(tags))
	for _, tag := range tags {
		protoTag := svc.convertTagToProto(tag)
		protoTags = append(protoTags, protoTag)
	}

	protoResponse := &pb.GetTagsResponse{
		Tags: protoTags,
		Response: &pb.BaseResponse{
			RequestId: requestID,
			Status:    "success",
		},
	}

	return protoResponse, nil
}

func (svc *Service) GetTag(ctx context.Context, req *pb.GetTagRequest) (*pb.GetTagResponse, error) {
	ctx, span := svc.tracer.Start(ctx, "TagsService.GetTag", trace.WithAttributes(
		attribute.String("user.operation", "get_tag"),
		attribute.String("user.id", req.UserId),
		attribute.String("tag.id", req.TagId),
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

	userUUID, err := uuid.Parse(userID)
	if err != nil {
		svc.logger.Error("Invalid user ID format", zap.Error(err))
		return nil, status.Error(codes.InvalidArgument, "invalid user ID format")
	}

	tagUUID, err := uuid.Parse(req.TagId)
	if err != nil {
		svc.logger.Error("Invalid tag ID format", zap.Error(err))
		return nil, status.Error(codes.InvalidArgument, "invalid tag ID format")
	}

	req.Request.RequestId = requestID
	req.UserId = userID

	startTime := time.Now()
	defer func() {
		metrics.Get().RegisterDurationSeconds.Record(ctx, time.Since(startTime).Seconds())
	}()

	svc.logger.Info("GetTag request received",
		zap.String("user_id", req.UserId),
		zap.String("tag_id", req.TagId))

	tag, err := svc.repo.Get(ctx, userUUID, tagUUID)
	if err != nil {
		if err == domain.ErrNotFound {
			return nil, status.Error(codes.NotFound, "tag not found")
		}
		svc.logger.Error("Failed to get tag", zap.Error(err))
		return nil, status.Error(codes.Internal, "failed to get tag")
	}

	protoTag := svc.convertTagToProto(tag)

	response := &pb.GetTagResponse{
		Tag: protoTag,
		Response: &pb.BaseResponse{
			RequestId: requestID,
			Status:    "success",
		},
	}

	return response, nil
}

func (svc *Service) CreateTag(ctx context.Context, req *pb.CreateTagRequest) (*pb.CreateTagResponse, error) {
	ctx, span := svc.tracer.Start(ctx, "TagsService.CreateTag", trace.WithAttributes(
		attribute.String("user.operation", "create_tag"),
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

	userUUID, err := uuid.Parse(userID)
	if err != nil {
		svc.logger.Error("Invalid user ID format", zap.Error(err))
		return nil, status.Error(codes.InvalidArgument, "invalid user ID format")
	}

	if req.Tag == nil {
		return nil, status.Error(codes.InvalidArgument, "tag data is required")
	}

	req.Request.RequestId = requestID
	req.UserId = userID

	startTime := time.Now()
	defer func() {
		metrics.Get().RegisterDurationSeconds.Record(ctx, time.Since(startTime).Seconds())
	}()

	svc.logger.Info("CreateTag request received",
		zap.String("user_id", req.UserId),
		zap.String("tag_name", req.Tag.Name))

	params := CreatePersonalTagParams{
		Name:        req.Tag.Name,
		Description: req.Tag.Description,
		TagType:     req.Tag.TagType,
		Active:      &req.Tag.Active,
	}

	createdTag, err := svc.repo.Create(ctx, userUUID, params)
	if err != nil {
		svc.logger.Error("Failed to create tag", zap.Error(err))
		return nil, status.Error(codes.Internal, "failed to create tag")
	}

	protoTag := svc.convertPersonalTagToProto(createdTag)

	response := &pb.CreateTagResponse{
		Success: true,
		Message: "Tag created successfully",
		Tag:     protoTag,
		Response: &pb.BaseResponse{
			RequestId: requestID,
			Status:    "success",
		},
	}

	return response, nil
}

func (svc *Service) DeleteTag(ctx context.Context, req *pb.DeleteTagRequest) (*pb.DeleteTagResponse, error) {
	ctx, span := svc.tracer.Start(ctx, "TagsService.DeleteTag", trace.WithAttributes(
		attribute.String("user.operation", "delete_tag"),
		attribute.String("user.id", req.UserId),
		attribute.String("tag.id", req.TagId),
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

	userUUID, err := uuid.Parse(userID)
	if err != nil {
		svc.logger.Error("Invalid user ID format", zap.Error(err))
		return nil, status.Error(codes.InvalidArgument, "invalid user ID format")
	}

	tagUUID, err := uuid.Parse(req.TagId)
	if err != nil {
		svc.logger.Error("Invalid tag ID format", zap.Error(err))
		return nil, status.Error(codes.InvalidArgument, "invalid tag ID format")
	}

	req.Request.RequestId = requestID
	req.UserId = userID

	startTime := time.Now()
	defer func() {
		metrics.Get().RegisterDurationSeconds.Record(ctx, time.Since(startTime).Seconds())
	}()

	svc.logger.Info("DeleteTag request received",
		zap.String("user_id", req.UserId),
		zap.String("tag_id", req.TagId))

	err = svc.repo.Delete(ctx, userUUID, tagUUID)
	if err != nil {
		if err == domain.ErrNotFound {
			return nil, status.Error(codes.NotFound, "tag not found")
		}
		svc.logger.Error("Failed to delete tag", zap.Error(err))
		return nil, status.Error(codes.Internal, "failed to delete tag")
	}

	response := &pb.DeleteTagResponse{
		Success: true,
		Message: "Tag deleted successfully",
		Response: &pb.BaseResponse{
			RequestId: requestID,
			Status:    "success",
		},
	}

	return response, nil
}

func (svc *Service) UpdateTag(ctx context.Context, req *pb.UpdateTagRequest) (*pb.UpdateTagResponse, error) {
	ctx, span := svc.tracer.Start(ctx, "TagsService.UpdateTag", trace.WithAttributes(
		attribute.String("user.operation", "update_tag"),
		attribute.String("user.id", req.UserId),
		attribute.String("tag.id", req.TagId),
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

	userUUID, err := uuid.Parse(userID)
	if err != nil {
		svc.logger.Error("Invalid user ID format", zap.Error(err))
		return nil, status.Error(codes.InvalidArgument, "invalid user ID format")
	}

	tagUUID, err := uuid.Parse(req.TagId)
	if err != nil {
		svc.logger.Error("Invalid tag ID format", zap.Error(err))
		return nil, status.Error(codes.InvalidArgument, "invalid tag ID format")
	}

	if req.Tag == nil {
		return nil, status.Error(codes.InvalidArgument, "tag data is required")
	}

	req.Request.RequestId = requestID
	req.UserId = userID

	startTime := time.Now()
	defer func() {
		metrics.Get().RegisterDurationSeconds.Record(ctx, time.Since(startTime).Seconds())
	}()

	svc.logger.Info("UpdateTag request received",
		zap.String("user_id", req.UserId),
		zap.String("tag_id", req.TagId))

	params := UpdatePersonalTagParams{
		Name:        req.Tag.Name,
		Description: req.Tag.Description,
		TagType:     req.Tag.TagType,
		Active:      req.Tag.Active,
	}

	err = svc.repo.Update(ctx, userUUID, tagUUID, params)
	if err != nil {
		if err == domain.ErrNotFound {
			return nil, status.Error(codes.NotFound, "tag not found")
		}
		svc.logger.Error("Failed to update tag", zap.Error(err))
		return nil, status.Error(codes.Internal, "failed to update tag")
	}

	updatedTag, err := svc.repo.Get(ctx, userUUID, tagUUID)
	if err != nil {
		svc.logger.Error("Failed to fetch updated tag", zap.Error(err))
		return nil, status.Error(codes.Internal, "failed to fetch updated tag")
	}

	protoTag := svc.convertTagToProto(updatedTag)

	response := &pb.UpdateTagResponse{
		Success: true,
		Message: "Tag updated successfully",
		Tag:     protoTag,
		Response: &pb.BaseResponse{
			RequestId: requestID,
			Status:    "success",
		},
	}

	return response, nil
}
