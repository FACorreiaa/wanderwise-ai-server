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
	"github.com/FACorreiaa/go-poi-au-suggestions/internal/types"
)

type IntentClassifier interface {
	Classify(ctx context.Context, message string) (types.IntentType, error)
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
	deadLetterCh     chan types.StreamEvent
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
		deadLetterCh:      make(chan types.StreamEvent, 100),
		intentClassifier:  &types.SimpleIntentClassifier{},
	}

	// Start background processes
	go service.processDeadLetterQueue()

	return service, nil
}

func (s *Service) processDeadLetterQueue() {
	for event := range s.deadLetterCh {
		s.logger.Warn("Processing dead letter event",
			zap.String("event_type", string(event.Type)),
			zap.Any("event_data", event.Data))
		// Handle dead letter events
	}
}
