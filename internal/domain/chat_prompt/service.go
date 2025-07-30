package chat_prompt

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	pb "github.com/FACorreiaa/loci-proto/modules/chat/generated"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/patrickmn/go-cache"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
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
	"github.com/FACorreiaa/go-poi-au-suggestions/internal/types"
)

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
	ctx := stream.Context()

	userID, err := domain.CheckUserAuth(ctx)
	if err != nil {
		return err
	}

	ctx, span := svc.tracer.Start(ctx, "ChatService.StartChatStream", trace.WithAttributes(
		attribute.String("chat.operation", "start_chat_stream"),
		attribute.String("chat.user_id", userID),
		attribute.String("chat.profile_id", req.ProfileId),
		attribute.String("chat.initial_message", req.InitialMessage),
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

	req.UserId = userID

	eventCh := make(chan StreamEvent, 200)

	// Create a cancellable context with timeout for the streaming goroutine
	streamCtx, cancelStream := context.WithTimeout(ctx, 20*time.Second) // 20 second max
	defer cancelStream()                                                // Cancel when function exits

	go func() {
		defer func() {
			if r := recover(); r != nil {
				svc.logger.Error("Panic in StartChatStream", zap.Any("panic", r))
				select {
				case eventCh <- StreamEvent{
					Type:      EventTypeError,
					Error:     "Internal server error",
					Timestamp: time.Now(),
					EventID:   uuid.New().String(),
				}:
				case <-ctx.Done():
				}
			}
		}()

		_ = req.ContextType

		cityName := ""
		var userLocation *UserLocation

		if req.Metadata != nil {
			if city, exists := req.Metadata["city_name"]; exists {
				cityName = city
			}
			if lat, exists := req.Metadata["user_lat"]; exists {
				if lon, exists := req.Metadata["user_lon"]; exists {
					if latFloat, err := strconv.ParseFloat(lat, 64); err == nil {
						if lonFloat, err := strconv.ParseFloat(lon, 64); err == nil {
							userLocation = &UserLocation{
								UserLat: latFloat,
								UserLon: lonFloat,
							}
						}
					}
				}
			}
		}

		var typesUserLocation *types.UserLocation
		if userLocation != nil {
			typesUserLocation = &types.UserLocation{
				UserLat: userLocation.UserLat,
				UserLon: userLocation.UserLon,
			}
		}

		// Call the domain service's own LLM processing method
		fmt.Printf("DEBUG: Starting LLM stream for user %s, message: %s\n", userID, req.InitialMessage)

		// Convert types.UserLocation to domain UserLocation if needed
		var domainUserLocation *UserLocation
		if typesUserLocation != nil {
			domainUserLocation = &UserLocation{
				UserLat: typesUserLocation.UserLat,
				UserLon: typesUserLocation.UserLon,
			}
		}

		// Create a wrapper channel with debug logging
		wrappedEventCh := make(chan StreamEvent, 100)

		// Start a goroutine to forward events with debug logging
		go func() {
			defer func() {
				close(wrappedEventCh)
				fmt.Printf("DEBUG: Closed wrappedEventCh\n")
			}()

			fmt.Printf("DEBUG: Starting ProcessUnifiedChatMessageStream\n")

			// Call the domain service's own stream processing method
			err := svc.ProcessUnifiedChatMessageStream(
				streamCtx, // Use cancellable context
				userUUID,
				profileUUID,
				cityName,
				req.InitialMessage,
				domainUserLocation,
				wrappedEventCh,
			)

			fmt.Printf("DEBUG: ProcessUnifiedChatMessageStream completed with error: %v\n", err)

			if err != nil && streamCtx.Err() == nil {
				svc.logger.Error("Failed to process unified chat message stream", zap.Error(err))
				select {
				case wrappedEventCh <- StreamEvent{
					Type:      EventTypeError,
					Error:     err.Error(),
					Timestamp: time.Now(),
					EventID:   uuid.New().String(),
				}:
				case <-streamCtx.Done():
				}
			}
		}()

		// Forward events from wrapped channel to main eventCh with debug logging
		fmt.Printf("DEBUG: Starting event forwarding loop\n")
		go func() {
			defer close(eventCh)
			for {
				select {
				case event, ok := <-wrappedEventCh:
					if !ok {
						fmt.Printf("DEBUG: wrappedEventCh closed, ending forwarding\n")
						return
					}

					// Debug print to see LLM output in terminal
					fmt.Printf("DEBUG LLM OUTPUT: Type=%s, Data=%s, Error=%s\n", event.Type, event.Data, event.Error)

					select {
					case eventCh <- event:
					case <-streamCtx.Done():
						fmt.Printf("DEBUG: Context cancelled during event forwarding\n")
						return
					}

				case <-streamCtx.Done():
					fmt.Printf("DEBUG: Context cancelled, stopping event forwarding\n")
					return
				}
			}
		}()
	}()

	// Stream events back to client
	for {
		select {
		case event, ok := <-eventCh:
			if !ok {
				return nil // Channel closed, streaming complete
			}

			pbEvent, err := svc.convertStreamEventToChatEvent(event)
			if err != nil {
				svc.logger.Error("Failed to convert stream event to chat event", zap.Error(err))
				continue
			}

			if err := stream.Send(pbEvent); err != nil {
				svc.logger.Error("Failed to send chat event", zap.Error(err))
				// Cancel the stream context to stop background processing
				cancelStream()

				// Check if it's a client disconnect
				if status.Code(err) == codes.Canceled || status.Code(err) == codes.Unavailable {
					svc.logger.Info("Client disconnected during streaming")
					return nil
				}
				return err
			}

			if event.Type == EventTypeComplete || event.Type == EventTypeError {
				return nil
			}

		case <-ctx.Done():
			svc.logger.Info("Client disconnected from chat stream")
			return ctx.Err()
		}
	}
}

func (svc *Service) ContinueChatStream(req *pb.ContinueChatRequest, stream pb.ChatService_ContinueChatStreamServer) error {
	// Implementation placeholder - similar pattern to StartChatStream
	return status.Error(codes.Unimplemented, "ContinueChatStream not yet implemented")
}

func (svc *Service) FreeChatStream(req *pb.FreeChatRequest, stream pb.ChatService_FreeChatStreamServer) error {
	// Implementation placeholder - similar pattern to StartChatStream
	return status.Error(codes.Unimplemented, "FreeChatStream not yet implemented")
}

func (svc *Service) GetChatSessions(ctx context.Context, req *pb.GetChatSessionsRequest) (*pb.GetChatSessionsResponse, error) {
	return &pb.GetChatSessionsResponse{}, nil
}

func (svc *Service) SaveItinerary(ctx context.Context, req *pb.SaveItineraryRequest) (*pb.SaveItineraryResponse, error) {
	return &pb.SaveItineraryResponse{}, nil
}

func (svc *Service) GetSavedItineraries(ctx context.Context, req *pb.GetSavedItinerariesRequest) (*pb.GetSavedItinerariesResponse, error) {
	return &pb.GetSavedItinerariesResponse{}, nil
}

func (svc *Service) RemoveItinerary(ctx context.Context, req *pb.RemoveItineraryRequest) (*pb.RemoveItineraryResponse, error) {
	return &pb.RemoveItineraryResponse{}, nil
}

func (svc *Service) GetPOIDetails(ctx context.Context, req *pb.GetPOIDetailsRequest) (*pb.GetPOIDetailsResponse, error) {
	return &pb.GetPOIDetailsResponse{}, nil
}

// convertStreamEventToChatEvent converts domain StreamEvent to protobuf ChatEvent
func (svc *Service) convertStreamEventToChatEvent(event StreamEvent) (*pb.ChatEvent, error) {
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
		pbEvent.Payload = &pb.ChatEvent_Complete{
			Complete: &pb.CompleteEvent{
				SessionId:   pbEvent.SessionId,
				CompletedAt: timestamppb.New(event.Timestamp),
			},
		}
	case EventTypeMessage, EventTypeChunk:
		pbEvent.Payload = &pb.ChatEvent_Message{
			Message: &pb.ChatMessage{
				Id:        event.EventID,
				SessionId: pbEvent.SessionId,
				Content:   event.Message,
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
