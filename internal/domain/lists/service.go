package lists

import (
	"context"
	"errors"
	"time"

	pb "github.com/FACorreiaa/loci-proto/modules/list/generated"
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
	"github.com/FACorreiaa/go-poi-au-suggestions/protocol/grpc/middleware/grpcrequest"
)

type Service struct {
	pb.UnimplementedListServiceServer
	logger *zap.Logger
	repo   Repository
	pgpool *pgxpool.Pool
	tracer trace.Tracer
}

func NewService(ctx context.Context, repo Repository, pgpool *pgxpool.Pool, logger *zap.Logger) *Service {
	return &Service{
		logger: logger.With(zap.String("service", "lists")),
		repo:   repo,
		pgpool: pgpool,
		tracer: otel.Tracer("ListsService"),
	}
}

func (svc *Service) CreateList(ctx context.Context, req *pb.CreateListRequest) (*pb.CreateListResponse, error) {
	ctx, span := svc.tracer.Start(ctx, "ListsService.CreateList", trace.WithAttributes(
		attribute.String("lists.operation", "create_list"),
		attribute.String("lists.user_id", req.UserId),
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

	svc.logger.Info("CreateList request received", zap.String("user_id", req.UserId))

	// Validate input
	if req.Name == "" {
		return &pb.CreateListResponse{
			Success: false,
			Message: "list name is required",
		}, status.Error(codes.InvalidArgument, "list name is required")
	}

	userUUID, err := uuid.Parse(userID)
	if err != nil {
		svc.logger.Error("Invalid user ID format", zap.Error(err))
		return &pb.CreateListResponse{
			Success: false,
			Message: "invalid user ID format",
		}, status.Error(codes.InvalidArgument, "invalid user ID format")
	}

	// Convert protobuf to domain types
	list := List{
		ID:          uuid.New(),
		UserID:      userUUID,
		Name:        req.Name,
		Description: req.Description,
		IsPublic:    req.IsPublic,
		IsItinerary: req.IsItinerary,
		ViewCount:   0,
		SaveCount:   0,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	if req.CityId != "" {
		cityID, err := uuid.Parse(req.CityId)
		if err != nil {
			svc.logger.Error("Invalid city ID format", zap.Error(err))
			return &pb.CreateListResponse{
				Success: false,
				Message: "invalid city ID format",
			}, status.Error(codes.InvalidArgument, "invalid city ID format")
		}
		list.CityID = cityID
	}

	err = svc.repo.CreateList(ctx, list)
	if err != nil {
		svc.logger.Error("Failed to create list", zap.Error(err))
		return &pb.CreateListResponse{
			Success: false,
			Message: "failed to create list",
		}, status.Error(codes.Internal, "failed to create list")
	}

	svc.logger.Info("List created successfully", zap.String("user_id", req.UserId), zap.String("list_id", list.ID.String()))

	return &pb.CreateListResponse{
		Success: true,
		Message: "list created successfully",
		List:    convertToPBList(&list),
		Response: &pb.BaseResponse{
			RequestId: requestID,
			Upstream:  "lists-service",
			Status:    "success",
		},
	}, nil
}

func (svc *Service) GetList(ctx context.Context, req *pb.GetListRequest) (*pb.GetListResponse, error) {
	ctx, span := svc.tracer.Start(ctx, "ListsService.GetList", trace.WithAttributes(
		attribute.String("lists.operation", "get_list"),
		attribute.String("lists.list_id", req.ListId),
	))
	defer span.End()

	requestID, ok := ctx.Value(grpcrequest.RequestIDKey{}).(string)
	if !ok {
		return nil, status.Error(codes.Internal, "request id not found in context")
	}

	if req.Request == nil {
		req.Request = &pb.BaseRequest{}
	}

	req.Request.RequestId = requestID

	startTime := time.Now()
	defer func() {
		metrics.Get().RegisterDurationSeconds.Record(ctx, time.Since(startTime).Seconds())
	}()

	svc.logger.Info("GetList request received", zap.String("list_id", req.ListId))

	listID, err := uuid.Parse(req.ListId)
	if err != nil {
		svc.logger.Error("Invalid list ID format", zap.Error(err))
		return &pb.GetListResponse{
			List: nil,
		}, status.Error(codes.InvalidArgument, "invalid list ID format")
	}

	list, err := svc.repo.GetList(ctx, listID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			svc.logger.Warn("List not found", zap.String("list_id", req.ListId))
			return &pb.GetListResponse{
				List: nil,
			}, status.Error(codes.NotFound, "list not found")
		}
		svc.logger.Error("Failed to get list", zap.Error(err))
		return &pb.GetListResponse{
			List: nil,
		}, status.Error(codes.Internal, "failed to get list")
	}

	// Get list items if requested
	var listItems []*ListItem
	if req.IncludeDetailedItems {
		listItems, err = svc.repo.GetListItems(ctx, listID)
		if err != nil {
			svc.logger.Error("Failed to get list items", zap.Error(err))
			return &pb.GetListResponse{
				List: nil,
			}, status.Error(codes.Internal, "failed to get list items")
		}
	}

	svc.logger.Info("List retrieved successfully", zap.String("list_id", req.ListId))

	// Convert to pb.ListWithDetailedItems
	pbListWithItems := &pb.ListWithDetailedItems{
		List: convertToPBList(&list),
	}

	if req.IncludeDetailedItems && listItems != nil {
		pbItems := make([]*pb.ListItemWithContent, len(listItems))
		for i, item := range listItems {
			pbItems[i] = &pb.ListItemWithContent{
				ListItem: convertToPBListItem(item),
				// Note: Content details (POI/Restaurant/Hotel) would need additional repository calls
			}
		}
		pbListWithItems.Items = pbItems
	}

	return &pb.GetListResponse{
		List: pbListWithItems,
		Response: &pb.BaseResponse{
			RequestId: requestID,
			Upstream:  "lists-service",
			Status:    "success",
		},
	}, nil
}

func (svc *Service) GetLists(ctx context.Context, req *pb.GetListsRequest) (*pb.GetListsResponse, error) {
	ctx, span := svc.tracer.Start(ctx, "ListsService.GetLists", trace.WithAttributes(
		attribute.String("lists.operation", "get_lists"),
		attribute.String("lists.user_id", req.UserId),
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

	svc.logger.Info("GetLists request received", zap.String("user_id", req.UserId))

	userUUID, err := uuid.Parse(userID)
	if err != nil {
		svc.logger.Error("Invalid user ID format", zap.Error(err))
		return &pb.GetListsResponse{
			Lists: nil,
		}, status.Error(codes.InvalidArgument, "invalid user ID format")
	}

	// For now, we'll get all lists (both regular and itineraries)
	// You might want to add filtering based on req parameters
	regularLists, err := svc.repo.GetUserLists(ctx, userUUID, false)
	if err != nil {
		svc.logger.Error("Failed to get regular lists", zap.Error(err))
		return &pb.GetListsResponse{
			Lists: nil,
		}, status.Error(codes.Internal, "failed to get regular lists")
	}

	itineraryLists, err := svc.repo.GetUserLists(ctx, userUUID, true)
	if err != nil {
		svc.logger.Error("Failed to get itinerary lists", zap.Error(err))
		return &pb.GetListsResponse{
			Lists: nil,
		}, status.Error(codes.Internal, "failed to get itinerary lists")
	}

	// Combine all lists
	allLists := append(regularLists, itineraryLists...)

	pbLists := make([]*pb.ListWithItems, len(allLists))
	for i, list := range allLists {
		pbList := &pb.ListWithItems{
			List: convertToPBList(list),
		}

		// Include items if requested
		if req.IncludeItems {
			listItems, err := svc.repo.GetListItems(ctx, list.ID)
			if err != nil {
				svc.logger.Warn("Failed to get items for list", zap.String("list_id", list.ID.String()), zap.Error(err))
			} else {
				pbItems := make([]*pb.ListItem, len(listItems))
				for j, item := range listItems {
					pbItems[j] = convertToPBListItem(item)
				}
				pbList.Items = pbItems
			}
		}

		pbLists[i] = pbList
	}

	svc.logger.Info("Lists retrieved successfully", zap.String("user_id", req.UserId), zap.Int("count", len(allLists)))

	return &pb.GetListsResponse{
		Lists:      pbLists,
		TotalCount: int32(len(allLists)),
		Response: &pb.BaseResponse{
			RequestId: requestID,
			Upstream:  "lists-service",
			Status:    "success",
		},
	}, nil
}

func (svc *Service) UpdateList(ctx context.Context, req *pb.UpdateListRequest) (*pb.UpdateListResponse, error) {
	ctx, span := svc.tracer.Start(ctx, "ListsService.UpdateList", trace.WithAttributes(
		attribute.String("lists.operation", "update_list"),
		attribute.String("lists.list_id", req.ListId),
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

	startTime := time.Now()
	defer func() {
		metrics.Get().RegisterDurationSeconds.Record(ctx, time.Since(startTime).Seconds())
	}()

	svc.logger.Info("UpdateList request received", zap.String("list_id", req.ListId))

	listID, err := uuid.Parse(req.ListId)
	if err != nil {
		svc.logger.Error("Invalid list ID format", zap.Error(err))
		return &pb.UpdateListResponse{
			Success: false,
			Message: "invalid list ID format",
		}, status.Error(codes.InvalidArgument, "invalid list ID format")
	}

	userUUID, err := uuid.Parse(userID)
	if err != nil {
		svc.logger.Error("Invalid user ID format", zap.Error(err))
		return &pb.UpdateListResponse{
			Success: false,
			Message: "invalid user ID format",
		}, status.Error(codes.InvalidArgument, "invalid user ID format")
	}

	// Get existing list to verify ownership and for updating
	existingList, err := svc.repo.GetList(ctx, listID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			svc.logger.Warn("List not found for update", zap.String("list_id", req.ListId))
			return &pb.UpdateListResponse{
				Success: false,
				Message: "list not found",
			}, status.Error(codes.NotFound, "list not found")
		}
		svc.logger.Error("Failed to get list for update", zap.Error(err))
		return &pb.UpdateListResponse{
			Success: false,
			Message: "failed to get list for update",
		}, status.Error(codes.Internal, "failed to get list for update")
	}

	// Verify ownership
	if existingList.UserID != userUUID {
		svc.logger.Warn("Unauthorized attempt to update list", zap.String("user_id", userID), zap.String("list_id", req.ListId))
		return &pb.UpdateListResponse{
			Success: false,
			Message: "unauthorized",
		}, status.Error(codes.PermissionDenied, "unauthorized")
	}

	// Update fields if provided
	if req.Name != "" {
		existingList.Name = req.Name
	}
	if req.Description != "" {
		existingList.Description = req.Description
	}
	if req.ImageUrl != "" {
		existingList.ImageURL = req.ImageUrl
	}
	existingList.IsPublic = req.IsPublic
	if req.CityId != "" {
		cityID, err := uuid.Parse(req.CityId)
		if err != nil {
			svc.logger.Error("Invalid city ID format", zap.Error(err))
			return &pb.UpdateListResponse{
				Success: false,
				Message: "invalid city ID format",
			}, status.Error(codes.InvalidArgument, "invalid city ID format")
		}
		existingList.CityID = cityID
	}
	existingList.UpdatedAt = time.Now()

	err = svc.repo.UpdateList(ctx, existingList)
	if err != nil {
		svc.logger.Error("Failed to update list", zap.Error(err))
		return &pb.UpdateListResponse{
			Success: false,
			Message: "failed to update list",
		}, status.Error(codes.Internal, "failed to update list")
	}

	svc.logger.Info("List updated successfully", zap.String("list_id", req.ListId))

	return &pb.UpdateListResponse{
		Success: true,
		Message: "list updated successfully",
		List:    convertToPBList(&existingList),
		Response: &pb.BaseResponse{
			RequestId: requestID,
			Upstream:  "lists-service",
			Status:    "success",
		},
	}, nil
}

func (svc *Service) DeleteList(ctx context.Context, req *pb.DeleteListRequest) (*pb.DeleteListResponse, error) {
	ctx, span := svc.tracer.Start(ctx, "ListsService.DeleteList", trace.WithAttributes(
		attribute.String("lists.operation", "delete_list"),
		attribute.String("lists.list_id", req.ListId),
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

	startTime := time.Now()
	defer func() {
		metrics.Get().RegisterDurationSeconds.Record(ctx, time.Since(startTime).Seconds())
	}()

	svc.logger.Info("DeleteList request received", zap.String("list_id", req.ListId))

	listID, err := uuid.Parse(req.ListId)
	if err != nil {
		svc.logger.Error("Invalid list ID format", zap.Error(err))
		return &pb.DeleteListResponse{
			Success: false,
			Message: "invalid list ID format",
		}, status.Error(codes.InvalidArgument, "invalid list ID format")
	}

	userUUID, err := uuid.Parse(userID)
	if err != nil {
		svc.logger.Error("Invalid user ID format", zap.Error(err))
		return &pb.DeleteListResponse{
			Success: false,
			Message: "invalid user ID format",
		}, status.Error(codes.InvalidArgument, "invalid user ID format")
	}

	// Verify ownership before deletion
	list, err := svc.repo.GetList(ctx, listID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			svc.logger.Warn("List not found for deletion", zap.String("list_id", req.ListId))
			return &pb.DeleteListResponse{
				Success: false,
				Message: "list not found",
			}, status.Error(codes.NotFound, "list not found")
		}
		svc.logger.Error("Failed to get list for deletion", zap.Error(err))
		return &pb.DeleteListResponse{
			Success: false,
			Message: "failed to get list for deletion",
		}, status.Error(codes.Internal, "failed to get list for deletion")
	}

	if list.UserID != userUUID {
		svc.logger.Warn("Unauthorized attempt to delete list", zap.String("user_id", userID), zap.String("list_id", req.ListId))
		return &pb.DeleteListResponse{
			Success: false,
			Message: "unauthorized",
		}, status.Error(codes.PermissionDenied, "unauthorized")
	}

	err = svc.repo.DeleteList(ctx, listID)
	if err != nil {
		svc.logger.Error("Failed to delete list", zap.Error(err))
		return &pb.DeleteListResponse{
			Success: false,
			Message: "failed to delete list",
		}, status.Error(codes.Internal, "failed to delete list")
	}

	svc.logger.Info("List deleted successfully", zap.String("list_id", req.ListId))

	return &pb.DeleteListResponse{
		Success: true,
		Message: "list deleted successfully",
		Response: &pb.BaseResponse{
			RequestId: requestID,
			Upstream:  "lists-service",
			Status:    "success",
		},
	}, nil
}

func (svc *Service) AddListItem(ctx context.Context, req *pb.AddListItemRequest) (*pb.AddListItemResponse, error) {
	ctx, span := svc.tracer.Start(ctx, "ListsService.AddListItem", trace.WithAttributes(
		attribute.String("lists.operation", "add_list_item"),
		attribute.String("lists.list_id", req.ListId),
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

	startTime := time.Now()
	defer func() {
		metrics.Get().RegisterDurationSeconds.Record(ctx, time.Since(startTime).Seconds())
	}()

	svc.logger.Info("AddListItem request received", zap.String("list_id", req.ListId))

	listID, err := uuid.Parse(req.ListId)
	if err != nil {
		svc.logger.Error("Invalid list ID format", zap.Error(err))
		return &pb.AddListItemResponse{
			Success: false,
			Message: "invalid list ID format",
		}, status.Error(codes.InvalidArgument, "invalid list ID format")
	}

	itemID, err := uuid.Parse(req.ItemId)
	if err != nil {
		svc.logger.Error("Invalid item ID format", zap.Error(err))
		return &pb.AddListItemResponse{
			Success: false,
			Message: "invalid item ID format",
		}, status.Error(codes.InvalidArgument, "invalid item ID format")
	}

	userUUID, err := uuid.Parse(userID)
	if err != nil {
		svc.logger.Error("Invalid user ID format", zap.Error(err))
		return &pb.AddListItemResponse{
			Success: false,
			Message: "invalid user ID format",
		}, status.Error(codes.InvalidArgument, "invalid user ID format")
	}

	// Verify list ownership
	list, err := svc.repo.GetList(ctx, listID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			svc.logger.Warn("List not found for adding item", zap.String("list_id", req.ListId))
			return &pb.AddListItemResponse{
				Success: false,
				Message: "list not found",
			}, status.Error(codes.NotFound, "list not found")
		}
		svc.logger.Error("Failed to get list for adding item", zap.Error(err))
		return &pb.AddListItemResponse{
			Success: false,
			Message: "failed to get list for adding item",
		}, status.Error(codes.Internal, "failed to get list for adding item")
	}

	if list.UserID != userUUID {
		svc.logger.Warn("Unauthorized attempt to add item to list", zap.String("user_id", userID), zap.String("list_id", req.ListId))
		return &pb.AddListItemResponse{
			Success: false,
			Message: "unauthorized",
		}, status.Error(codes.PermissionDenied, "unauthorized")
	}

	// Convert protobuf to domain types
	listItem := ListItem{
		ListID:      listID,
		ItemID:      itemID,
		ContentType: convertFromPBContentType(req.ContentType),
		Position:    int(req.Position),
		Notes:       req.Notes,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	if req.DayNumber > 0 {
		dayNum := int(req.DayNumber)
		listItem.DayNumber = &dayNum
	}

	if req.TimeSlot != nil {
		timeSlot := req.TimeSlot.AsTime()
		listItem.TimeSlot = &timeSlot
	}

	if req.DurationMinutes > 0 {
		duration := int(req.DurationMinutes)
		listItem.Duration = &duration
	}

	if req.SourceLlmInteractionId != "" {
		sourceID, err := uuid.Parse(req.SourceLlmInteractionId)
		if err == nil {
			listItem.SourceLlmInteractionID = &sourceID
		}
	}

	if req.ItemAiDescription != "" {
		listItem.ItemAIDescription = req.ItemAiDescription
	}

	err = svc.repo.AddListItem(ctx, listItem)
	if err != nil {
		svc.logger.Error("Failed to add list item", zap.Error(err))
		return &pb.AddListItemResponse{
			Success: false,
			Message: "failed to add list item",
		}, status.Error(codes.Internal, "failed to add list item")
	}

	svc.logger.Info("List item added successfully", zap.String("list_id", req.ListId), zap.String("item_id", req.ItemId))

	return &pb.AddListItemResponse{
		Success: true,
		Message: "list item added successfully",
		Item:    convertToPBListItem(&listItem),
		Response: &pb.BaseResponse{
			RequestId: requestID,
			Upstream:  "lists-service",
			Status:    "success",
		},
	}, nil
}

func (svc *Service) GetListItems(ctx context.Context, req *pb.GetListItemsRequest) (*pb.GetListItemsResponse, error) {
	ctx, span := svc.tracer.Start(ctx, "ListsService.GetListItems", trace.WithAttributes(
		attribute.String("lists.operation", "get_list_items"),
		attribute.String("lists.list_id", req.ListId),
	))
	defer span.End()

	requestID, ok := ctx.Value(grpcrequest.RequestIDKey{}).(string)
	if !ok {
		return nil, status.Error(codes.Internal, "request id not found in context")
	}

	if req.Request == nil {
		req.Request = &pb.BaseRequest{}
	}

	req.Request.RequestId = requestID

	startTime := time.Now()
	defer func() {
		metrics.Get().RegisterDurationSeconds.Record(ctx, time.Since(startTime).Seconds())
	}()

	svc.logger.Info("GetListItems request received", zap.String("list_id", req.ListId))

	listID, err := uuid.Parse(req.ListId)
	if err != nil {
		svc.logger.Error("Invalid list ID format", zap.Error(err))
		return &pb.GetListItemsResponse{
			Items: nil,
		}, status.Error(codes.InvalidArgument, "invalid list ID format")
	}

	items, err := svc.repo.GetListItems(ctx, listID)
	if err != nil {
		svc.logger.Error("Failed to get list items", zap.Error(err))
		return &pb.GetListItemsResponse{
			Items: nil,
		}, status.Error(codes.Internal, "failed to get list items")
	}

	pbItems := make([]*pb.ListItemWithContent, len(items))
	for i, item := range items {
		pbItems[i] = &pb.ListItemWithContent{
			ListItem: convertToPBListItem(item),
			// Note: Content details (POI/Restaurant/Hotel) would need additional repository calls
		}
	}

	svc.logger.Info("List items retrieved successfully", zap.String("list_id", req.ListId), zap.Int("count", len(items)))

	return &pb.GetListItemsResponse{
		Items:      pbItems,
		TotalCount: int32(len(items)),
		Response: &pb.BaseResponse{
			RequestId: requestID,
			Upstream:  "lists-service",
			Status:    "success",
		},
	}, nil
}

func (svc *Service) RemoveListItem(ctx context.Context, req *pb.RemoveListItemRequest) (*pb.RemoveListItemResponse, error) {
	ctx, span := svc.tracer.Start(ctx, "ListsService.RemoveListItem", trace.WithAttributes(
		attribute.String("lists.operation", "remove_list_item"),
		attribute.String("lists.list_id", req.ListId),
		attribute.String("lists.item_id", req.ItemId),
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

	startTime := time.Now()
	defer func() {
		metrics.Get().RegisterDurationSeconds.Record(ctx, time.Since(startTime).Seconds())
	}()

	svc.logger.Info("RemoveListItem request received", zap.String("list_id", req.ListId), zap.String("item_id", req.ItemId))

	listID, err := uuid.Parse(req.ListId)
	if err != nil {
		svc.logger.Error("Invalid list ID format", zap.Error(err))
		return &pb.RemoveListItemResponse{
			Success: false,
			Message: "invalid list ID format",
		}, status.Error(codes.InvalidArgument, "invalid list ID format")
	}

	itemID, err := uuid.Parse(req.ItemId)
	if err != nil {
		svc.logger.Error("Invalid item ID format", zap.Error(err))
		return &pb.RemoveListItemResponse{
			Success: false,
			Message: "invalid item ID format",
		}, status.Error(codes.InvalidArgument, "invalid item ID format")
	}

	userUUID, err := uuid.Parse(userID)
	if err != nil {
		svc.logger.Error("Invalid user ID format", zap.Error(err))
		return &pb.RemoveListItemResponse{
			Success: false,
			Message: "invalid user ID format",
		}, status.Error(codes.InvalidArgument, "invalid user ID format")
	}

	// Verify list ownership
	list, err := svc.repo.GetList(ctx, listID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			svc.logger.Warn("List not found for removing item", zap.String("list_id", req.ListId))
			return &pb.RemoveListItemResponse{
				Success: false,
				Message: "list not found",
			}, status.Error(codes.NotFound, "list not found")
		}
		svc.logger.Error("Failed to get list for removing item", zap.Error(err))
		return &pb.RemoveListItemResponse{
			Success: false,
			Message: "failed to get list for removing item",
		}, status.Error(codes.Internal, "failed to get list for removing item")
	}

	if list.UserID != userUUID {
		svc.logger.Warn("Unauthorized attempt to remove item from list", zap.String("user_id", userID), zap.String("list_id", req.ListId))
		return &pb.RemoveListItemResponse{
			Success: false,
			Message: "unauthorized",
		}, status.Error(codes.PermissionDenied, "unauthorized")
	}

	err = svc.repo.DeleteListItemByID(ctx, listID, itemID)
	if err != nil {
		svc.logger.Error("Failed to remove list item", zap.Error(err))
		return &pb.RemoveListItemResponse{
			Success: false,
			Message: "failed to remove list item",
		}, status.Error(codes.Internal, "failed to remove list item")
	}

	svc.logger.Info("List item removed successfully", zap.String("list_id", req.ListId), zap.String("item_id", req.ItemId))

	return &pb.RemoveListItemResponse{
		Success: true,
		Message: "list item removed successfully",
		Response: &pb.BaseResponse{
			RequestId: requestID,
			Upstream:  "lists-service",
			Status:    "success",
		},
	}, nil
}

// Converter functions
func convertToPBList(list *List) *pb.List {
	pbList := &pb.List{
		Id:          list.ID.String(),
		UserId:      list.UserID.String(),
		Name:        list.Name,
		Description: list.Description,
		ImageUrl:    list.ImageURL,
		IsPublic:    list.IsPublic,
		IsItinerary: list.IsItinerary,
		CityId:      list.CityID.String(),
		ViewCount:   int32(list.ViewCount),
		SaveCount:   int32(list.SaveCount),
		CreatedAt:   timestamppb.New(list.CreatedAt),
		UpdatedAt:   timestamppb.New(list.UpdatedAt),
	}

	if list.ParentListID != nil {
		pbList.ParentListId = list.ParentListID.String()
	}

	return pbList
}

func convertToPBListItem(item *ListItem) *pb.ListItem {
	pbItem := &pb.ListItem{
		ListId:                 item.ListID.String(),
		ItemId:                 item.ItemID.String(),
		ContentType:            convertToPBContentType(item.ContentType),
		Position:               int32(item.Position),
		Notes:                  item.Notes,
		ItemAiDescription:      item.ItemAIDescription,
		CreatedAt:              timestamppb.New(item.CreatedAt),
		UpdatedAt:              timestamppb.New(item.UpdatedAt),
		SourceLlmInteractionId: "",
	}

	if item.DayNumber != nil {
		pbItem.DayNumber = int32(*item.DayNumber)
	}

	if item.TimeSlot != nil {
		pbItem.TimeSlot = timestamppb.New(*item.TimeSlot)
	}

	if item.Duration != nil {
		pbItem.Duration = int32(*item.Duration)
	}

	if item.SourceLlmInteractionID != nil {
		pbItem.SourceLlmInteractionId = item.SourceLlmInteractionID.String()
	}

	return pbItem
}

func convertToPBContentType(contentType ContentType) pb.ContentType {
	switch contentType {
	case ContentTypePOI:
		return pb.ContentType_CONTENT_TYPE_POI
	case ContentTypeRestaurant:
		return pb.ContentType_CONTENT_TYPE_RESTAURANT
	case ContentTypeHotel:
		return pb.ContentType_CONTENT_TYPE_HOTEL
	case ContentTypeItinerary:
		return pb.ContentType_CONTENT_TYPE_ITINERARY
	default:
		return pb.ContentType_CONTENT_TYPE_UNSPECIFIED
	}
}

func convertFromPBContentType(contentType pb.ContentType) ContentType {
	switch contentType {
	case pb.ContentType_CONTENT_TYPE_POI:
		return ContentTypePOI
	case pb.ContentType_CONTENT_TYPE_RESTAURANT:
		return ContentTypeRestaurant
	case pb.ContentType_CONTENT_TYPE_HOTEL:
		return ContentTypeHotel
	case pb.ContentType_CONTENT_TYPE_ITINERARY:
		return ContentTypeItinerary
	default:
		return ContentTypePOI // default fallback
	}
}
