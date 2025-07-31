package chat_prompt

import (
	"context"
	"fmt"
	"os"
	"time"

	pb "github.com/FACorreiaa/loci-proto/modules/chat/generated"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/patrickmn/go-cache"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	generativeAI "github.com/FACorreiaa/go-genai-sdk/lib"

	"github.com/FACorreiaa/go-poi-au-suggestions/internal/domain"
	cityDomain "github.com/FACorreiaa/go-poi-au-suggestions/internal/domain/city"
	interestsDomain "github.com/FACorreiaa/go-poi-au-suggestions/internal/domain/interests"
	poiDomain "github.com/FACorreiaa/go-poi-au-suggestions/internal/domain/poi"
	profilesDomain "github.com/FACorreiaa/go-poi-au-suggestions/internal/domain/profiles"
	tagsDomain "github.com/FACorreiaa/go-poi-au-suggestions/internal/domain/tags"
)

func (svc *Service) startStreamProcessing(
	ctx context.Context,
	processFunc func(ctx context.Context, eventCh chan<- StreamEvent) error,
	eventCh chan<- StreamEvent,
) {
	defer func() {
		if r := recover(); r != nil {
			svc.logger.Error("Panic recovered in stream processing", zap.Any("panic", r))
			// Ensure the channel is not closed while trying to send to it
			select {
			case eventCh <- StreamEvent{Type: EventTypeError, Error: "internal server error"}:
			case <-ctx.Done():
			}
		}
		close(eventCh)
	}()

	if err := processFunc(ctx, eventCh); err != nil {
		// Log the error and send an error event
		svc.logger.Error("Error processing stream", zap.Error(err))
		select {
		case eventCh <- StreamEvent{Type: EventTypeError, Error: err.Error()}:
		case <-ctx.Done():
		}
	}
}

type IntentClassifier interface {
	Classify(ctx context.Context, message string) (IntentType, error)
}

type Service struct {
	pb.UnsafeChatServiceServer
	logger *zap.Logger
	repo   Repository
	pgpool *pgxpool.Pool
	tracer trace.Tracer

	// Business logic dependencies
	interestRepo      interestsDomain.Repository
	searchProfileRepo profilesDomain.Repository
	searchProfileSvc  *profilesDomain.Service
	tagsRepo          tagsDomain.Repository
	aiClient          *generativeAI.LLMChatClient
	embeddingService  *generativeAI.EmbeddingService
	cityRepo          cityDomain.Repository
	poiRepo           poiDomain.Repository
	cache             *cache.Cache

	// Events
	deadLetterCh     chan StreamEvent
	intentClassifier IntentClassifier
}

func NewService(
	ctx context.Context,
	repo Repository,
	pgpool *pgxpool.Pool,
	logger *zap.Logger,
	interestRepo interestsDomain.Repository,
	searchProfileRepo profilesDomain.Repository,
	searchProfileSvc *profilesDomain.Service,
	tagsRepo tagsDomain.Repository,
	cityRepo cityDomain.Repository,
	poiRepo poiDomain.Repository,
) (*Service, error) {
	// Initialize AI client
	apiKey := os.Getenv("GEMINI_API_KEY")
	aiClient, err := generativeAI.NewLLMChatClient(ctx, apiKey)
	if err != nil {
		logger.Error("Failed to create AI client", zap.Error(err))
		return nil, err
	}

	// Initialize embedding service - Note: requires slog logger, will be fixed later
	// For now, we'll set it to nil and add it when needed
	var embeddingService *generativeAI.EmbeddingService
	// TODO: Create slog wrapper or adapter for zap logger

	// Initialize cache
	cacheInstance := cache.New(24*time.Hour, 1*time.Hour)

	service := &Service{
		logger:            logger.With(zap.String("service", "chat_prompt")),
		repo:              repo,
		pgpool:            pgpool,
		tracer:            otel.Tracer("ChatPromptService"),
		interestRepo:      interestRepo,
		searchProfileRepo: searchProfileRepo,
		searchProfileSvc:  searchProfileSvc,
		tagsRepo:          tagsRepo,
		aiClient:          aiClient,
		embeddingService:  embeddingService,
		cityRepo:          cityRepo,
		poiRepo:           poiRepo,
		cache:             cacheInstance,
		deadLetterCh:      make(chan StreamEvent, 100),
		intentClassifier:  &SimpleIntentClassifier{},
	}

	go service.processDeadLetterQueue()

	return service, nil
}

func (svc *Service) StartChatStream(req *pb.StartChatRequest, stream pb.ChatService_StartChatStreamServer) error {
	ctx, cancel := context.WithCancel(stream.Context())
	defer cancel()

	userID, err := domain.CheckUserAuth(ctx)
	if err != nil {
		return err
	}

	ctx, span := svc.tracer.Start(ctx, "ChatService.StartChatStream", trace.WithAttributes(
		attribute.String("chat.user_id", userID),
		attribute.String("chat.profile_id", req.ProfileId),
	))
	defer span.End()

	userUUID, err := uuid.Parse(userID)
	if err != nil {
		return status.Errorf(codes.InvalidArgument, "invalid user ID: %v", err)
	}

	profileUUID, err := uuid.Parse(req.ProfileId)
	if err != nil {
		return status.Errorf(codes.InvalidArgument, "invalid profile ID: %v", err)
	}

	return svc.handleStream(ctx, stream, func(eventCh chan<- StreamEvent) error {
		return svc.ProcessUnifiedChatMessageStream(ctx, userUUID, profileUUID, req.InitialMessage, req.Metadata, eventCh)
	})
}

func (svc *Service) handleStream(ctx context.Context, stream grpc.ServerStream, processFunc func(eventCh chan<- StreamEvent) error) error {
	eventCh := make(chan StreamEvent, 200)
	errCh := make(chan error, 1)

	go func() {
		defer close(errCh)
		defer close(eventCh)
		if err := processFunc(eventCh); err != nil {
			errCh <- err
		}
	}()

	for {
		select {
		case event, ok := <-eventCh:
			if !ok {
				// processFunc has finished
				return nil
			}
			pbEvent, err := svc.convertStreamEventToChatEvent(event)
			if err != nil {
				svc.logger.Error("Failed to convert stream event", zap.Error(err))
				continue // Or handle more gracefully
			}
			if err := stream.SendMsg(pbEvent); err != nil {
				svc.logger.Error("Failed to send message to stream", zap.Error(err))
				return err // Client disconnected or other stream error
			}

		case err := <-errCh:
			if err != nil {
				svc.logger.Error("Error during stream processing", zap.Error(err))
				// Decide if you want to send an error message to the client
				return err
			}

		case <-ctx.Done():
			svc.logger.Info("Stream cancelled by client")
			return ctx.Err()
		}
	}
}

func (svc *Service) ContinueChatStream(req *pb.ContinueChatRequest, stream pb.ChatService_ContinueChatStreamServer) error {
	ctx, cancel := context.WithCancel(stream.Context())
	defer cancel()

	userID, err := domain.CheckUserAuth(ctx)
	if err != nil {
		return err
	}

	ctx, span := svc.tracer.Start(ctx, "ChatService.ContinueChatStream", trace.WithAttributes(
		attribute.String("chat.user_id", userID),
		attribute.String("chat.session_id", req.SessionId),
	))
	defer span.End()

	sessionUUID, err := uuid.Parse(req.SessionId)
	if err != nil {
		return status.Errorf(codes.InvalidArgument, "invalid session ID: %v", err)
	}

	return svc.handleStream(ctx, stream, func(eventCh chan<- StreamEvent) error {
		return svc.ContinueSessionStreamed(ctx, sessionUUID, req.Message, nil, eventCh)
	})
}

func (svc *Service) FreeChatStream(req *pb.FreeChatRequest, stream pb.ChatService_FreeChatStreamServer) error {
	ctx, cancel := context.WithCancel(stream.Context())
	defer cancel()

	ctx, span := svc.tracer.Start(ctx, "ChatService.FreeChatStream", trace.WithAttributes(
		attribute.String("chat.message", req.Message),
	))
	defer span.End()

	return svc.handleStream(ctx, stream, func(eventCh chan<- StreamEvent) error {
		return svc.ProcessUnifiedChatMessageStreamFree(ctx, "", req.Message, nil, eventCh)
	})
}

func (svc *Service) GetChatSessions(ctx context.Context, req *pb.GetChatSessionsRequest) (*pb.GetChatSessionsResponse, error) {
	userID, err := domain.CheckUserAuth(ctx)
	if err != nil {
		return nil, err
	}

	ctx, span := svc.tracer.Start(ctx, "ChatService.GetChatSessions", trace.WithAttributes(
		attribute.String("chat.user_id", userID),
		attribute.Int("limit", int(req.Limit)),
		attribute.Int("offset", int(req.Offset)),
	))
	defer span.End()

	userUUID, err := uuid.Parse(userID)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid user ID: %v", err)
	}

	// Convert offset to page number (assuming offset-based pagination)
	limit := int(req.Limit)
	if limit < 1 {
		limit = 10
	}
	if limit > 50 {
		limit = 50
	}

	offset := int(req.Offset)
	if offset < 0 {
		offset = 0
	}

	page := (offset / limit) + 1

	response, err := svc.GetUserChatSessions(ctx, userUUID, page, limit)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get chat sessions: %v", err)
	}

	// Convert domain response to protobuf
	pbSessions := make([]*pb.ChatSession, len(response.Sessions))
	for i, session := range response.Sessions {
		pbSessions[i] = &pb.ChatSession{
			Id:           session.ID.String(),
			UserId:       userID,
			Title:        session.CityName, // Use city name as title for now
			CreatedAt:    timestamppb.New(session.CreatedAt),
			UpdatedAt:    timestamppb.New(session.UpdatedAt),
			MessageCount: 0, // Will need to be calculated if needed
		}
	}

	return &pb.GetChatSessionsResponse{
		Sessions:   pbSessions,
		TotalCount: int32(response.Total),
	}, nil
}

func (svc *Service) SaveItinerary(ctx context.Context, req *pb.SaveItineraryRequest) (*pb.SaveItineraryResponse, error) {
	userID, err := domain.CheckUserAuth(ctx)
	if err != nil {
		return nil, err
	}

	ctx, span := svc.tracer.Start(ctx, "ChatService.SaveItinerary", trace.WithAttributes(
		attribute.String("chat.user_id", userID),
		attribute.String("itinerary.title", req.Title),
	))
	defer span.End()

	userUUID, err := uuid.Parse(userID)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid user ID: %v", err)
	}

	if req.Title == "" {
		return nil, status.Errorf(codes.InvalidArgument, "title is required")
	}

	// Create the bookmark request from protobuf
	var description *string
	if req.Description != "" {
		description = &req.Description
	}

	bookmarkReq := BookmarkRequest{
		Title:       req.Title,
		Description: description,
	}

	// The protobuf only has SessionId, so use that
	if req.SessionId != "" {
		sessionUUID, err := uuid.Parse(req.SessionId)
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "invalid session_id: %v", err)
		}
		bookmarkReq.SessionID = &sessionUUID
	} else {
		return nil, status.Errorf(codes.InvalidArgument, "session_id is required")
	}

	itineraryID, err := svc.SaveItenerary(ctx, userUUID, bookmarkReq)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to save itinerary: %v", err)
	}

	return &pb.SaveItineraryResponse{
		ItineraryId: itineraryID.String(),
		Success:     true,
		Message:     "Itinerary saved successfully",
	}, nil
}

func (svc *Service) GetSavedItineraries(ctx context.Context, req *pb.GetSavedItinerariesRequest) (*pb.GetSavedItinerariesResponse, error) {
	userID, err := domain.CheckUserAuth(ctx)
	if err != nil {
		return nil, err
	}

	ctx, span := svc.tracer.Start(ctx, "ChatService.GetSavedItineraries", trace.WithAttributes(
		attribute.String("chat.user_id", userID),
		attribute.Int("limit", int(req.Limit)),
		attribute.Int("offset", int(req.Offset)),
	))
	defer span.End()

	userUUID, err := uuid.Parse(userID)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid user ID: %v", err)
	}

	// Convert offset to page number
	limit := int(req.Limit)
	if limit < 1 {
		limit = 10
	}
	if limit > 100 {
		limit = 100
	}

	offset := int(req.Offset)
	if offset < 0 {
		offset = 0
	}

	page := (offset / limit) + 1

	response, err := svc.GetBookmarkedItineraries(ctx, userUUID, page, limit)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get saved itineraries: %v", err)
	}

	// Convert domain response to protobuf
	pbItineraries := make([]*pb.UserSavedItinerary, len(response.Itineraries))
	for i, itinerary := range response.Itineraries {
		description := ""
		if itinerary.Description.Valid {
			description = itinerary.Description.String
		}

		pbItineraries[i] = &pb.UserSavedItinerary{
			Id:            itinerary.ID.String(),
			UserId:        userID,
			Title:         itinerary.Title,
			Description:   description,
			ItineraryData: "", // JSON data would go here if available
			CreatedAt:     timestamppb.New(itinerary.CreatedAt),
			UpdatedAt:     timestamppb.New(itinerary.UpdatedAt),
		}
	}

	return &pb.GetSavedItinerariesResponse{
		Itineraries: pbItineraries,
		TotalCount:  int32(response.TotalRecords),
	}, nil
}

func (svc *Service) RemoveItinerary(ctx context.Context, req *pb.RemoveItineraryRequest) (*pb.RemoveItineraryResponse, error) {
	userID, err := domain.CheckUserAuth(ctx)
	if err != nil {
		return nil, err
	}

	ctx, span := svc.tracer.Start(ctx, "ChatService.RemoveItinerary", trace.WithAttributes(
		attribute.String("chat.user_id", userID),
		attribute.String("itinerary.id", req.ItineraryId),
	))
	defer span.End()

	userUUID, err := uuid.Parse(userID)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid user ID: %v", err)
	}

	itineraryUUID, err := uuid.Parse(req.ItineraryId)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid itinerary ID: %v", err)
	}

	err = svc.RemoveItenerary(ctx, userUUID, itineraryUUID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to remove itinerary: %v", err)
	}

	return &pb.RemoveItineraryResponse{
		Success: true,
		Message: "Itinerary removed successfully",
	}, nil
}

func (svc *Service) GetPOIDetails(ctx context.Context, req *pb.GetPOIDetailsRequest) (*pb.GetPOIDetailsResponse, error) {
	userID, err := domain.CheckUserAuth(ctx)
	if err != nil {
		return nil, err
	}

	ctx, span := svc.tracer.Start(ctx, "ChatService.GetPOIDetails", trace.WithAttributes(
		attribute.String("chat.user_id", userID),
		attribute.String("poi.poi_id", req.PoiId),
		attribute.Bool("include_reviews", req.IncludeReviews),
		attribute.Bool("include_photos", req.IncludePhotos),
	))
	defer span.End()

	userUUID, err := uuid.Parse(userID)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid user ID: %v", err)
	}

	if req.PoiId == "" {
		return nil, status.Errorf(codes.InvalidArgument, "poi_id is required")
	}

	poiUUID, err := uuid.Parse(req.PoiId)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid poi_id: %v", err)
	}

	// Note: The existing GetPOIDetailedInfosResponse method expects city name and coordinates
	// For now, we'll return a placeholder implementation since the API signature doesn't match
	// In a real implementation, you'd need a method that takes POI ID
	_ = userUUID
	_ = poiUUID

	return &pb.GetPOIDetailsResponse{
		Poi: &pb.POIDetailedInfo{
			Id:          req.PoiId,
			Name:        "Placeholder POI",
			Description: "This is a placeholder implementation. The actual implementation would fetch POI details by ID.",
		},
	}, nil
}

// convertStreamEventToChatEvent converts domain StreamEvent to protobuf ChatEvent
func (svc *Service) convertStreamEventToChatEvent(event StreamEvent) (*pb.ChatEvent, error) {
	fmt.Printf("DEBUG: Converting event - Type: %s, Message: %q, Data: %v\n", event.Type, event.Message, event.Data)

	pbEvent := &pb.ChatEvent{
		EventType: event.Type,
		Data:      event.Message,
		SessionId: "", // Will be set from event data if available
		Timestamp: timestamppb.New(event.Timestamp),
	}

	// Extract session ID from event data if available
	if data, ok := event.Data.(map[string]interface{}); ok {
		if sessionID, exists := data["session_id"].(string); exists {
			pbEvent.SessionId = sessionID
		}
	}

	// Set payload based on event type
	switch event.Type {
	case EventTypeError:
		pbEvent.Payload = &pb.ChatEvent_Error{
			Error: &pb.ErrorEvent{
				Code:    "PROCESSING_ERROR",
				Message: event.Error,
				Details: map[string]string{
					"event_id": event.EventID,
				},
			},
		}
	case EventTypeComplete:
		// For complete events, set the Data field to the full response
		if data, ok := event.Data.(map[string]interface{}); ok {
			if fullResponse, exists := data["full_response"].(string); exists {
				pbEvent.Data = fullResponse
			}
		}

		pbEvent.Payload = &pb.ChatEvent_Complete{
			Complete: &pb.CompleteEvent{
				SessionId:   pbEvent.SessionId,
				CompletedAt: timestamppb.New(event.Timestamp),
			},
		}
	case EventTypeMessage, EventTypeChunk:
		content := event.Message
		if content == "" && event.Data != nil {
			// For chunk events, the content might be in Data field
			if dataStr, ok := event.Data.(string); ok {
				content = dataStr
			} else if dataMap, ok := event.Data.(map[string]interface{}); ok {
				// New format: chunk is in the "chunk" field
				if chunkStr, exists := dataMap["chunk"].(string); exists {
					content = chunkStr
				}
			}
		}

		pbEvent.Payload = &pb.ChatEvent_Message{
			Message: &pb.ChatMessage{
				Id:        event.EventID,
				SessionId: pbEvent.SessionId,
				Content:   content,
				Role:      "assistant",
				CreatedAt: timestamppb.New(event.Timestamp),
			},
		}
	case EventTypeStart, EventTypeProgress:
		pbEvent.Payload = &pb.ChatEvent_Thinking{
			Thinking: &pb.ThinkingEvent{
				Message: event.Message,
			},
		}
	case EventTypeCityData:
		// Try to convert city data to CityResponse
		if data, ok := event.Data.(map[string]interface{}); ok {
			pbEvent.Payload = &pb.ChatEvent_CityResponse{
				CityResponse: &pb.CityResponse{
					Name:        getStringFromMap(data, "city"),
					Country:     getStringFromMap(data, "country"),
					Description: getStringFromMap(data, "description"),
					// Add more fields as needed based on your domain types
				},
			}
		}
	case EventTypeItinerary:
		// Try to convert itinerary data to ItineraryResponse
		if data, ok := event.Data.(map[string]interface{}); ok {
			pbEvent.Payload = &pb.ChatEvent_ItineraryResponse{
				ItineraryResponse: &pb.ItineraryResponse{
					Title:       getStringFromMap(data, "title"),
					Description: getStringFromMap(data, "description"),
					// Add more fields as needed based on your domain types
				},
			}
		}
	default:
		// For other event types, use the thinking event as default
		pbEvent.Payload = &pb.ChatEvent_Thinking{
			Thinking: &pb.ThinkingEvent{
				Message: event.Message,
			},
		}
	}

	return pbEvent, nil
}

// Helper function to safely extract strings from map
func getStringFromMap(data map[string]interface{}, key string) string {
	if value, exists := data[key]; exists {
		if str, ok := value.(string); ok {
			return str
		}
	}
	return ""
}
