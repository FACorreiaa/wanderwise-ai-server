package chat_prompt

import (
	"context"
	"os"
	"time"

	pb "github.com/FACorreiaa/loci-proto/modules/chat/generated"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/patrickmn/go-cache"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"

	generativeAI "github.com/FACorreiaa/go-genai-sdk/lib"

	cityDomain "github.com/FACorreiaa/go-poi-au-suggestions/internal/domain/city"
	interestsDomain "github.com/FACorreiaa/go-poi-au-suggestions/internal/domain/interests"
	poiDomain "github.com/FACorreiaa/go-poi-au-suggestions/internal/domain/poi"
	profilesDomain "github.com/FACorreiaa/go-poi-au-suggestions/internal/domain/profiles"
	tagsDomain "github.com/FACorreiaa/go-poi-au-suggestions/internal/domain/tags"
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

func (svc *Service) StartChatStream(ctx context.Context, req *pb.StartChatRequest) (*pb.ChatEvent, error) {
	return nil, nil
}

func (svc *Service) ContinueChatStream(ctx context.Context, req *pb.ContinueChatRequest) (*pb.ChatEvent, error) {
	return nil, nil
}

func (svc *Service) FreeChatStream(ctx context.Context, req *pb.FreeChatRequest) (*pb.ChatEvent, error) {
	return nil, nil
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
