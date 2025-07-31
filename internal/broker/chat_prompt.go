package broker

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	"github.com/FACorreiaa/go-poi-au-suggestions/config"
	chatPromptDomain "github.com/FACorreiaa/go-poi-au-suggestions/internal/domain/chat_prompt"
	cityDomain "github.com/FACorreiaa/go-poi-au-suggestions/internal/domain/city"
	interestsDomain "github.com/FACorreiaa/go-poi-au-suggestions/internal/domain/interests"
	poiDomain "github.com/FACorreiaa/go-poi-au-suggestions/internal/domain/poi"
	profilesDomain "github.com/FACorreiaa/go-poi-au-suggestions/internal/domain/profiles"
	tagsDomain "github.com/FACorreiaa/go-poi-au-suggestions/internal/domain/tags"
)

// ChatPromptServiceImpl implements Service for gRPC chat prompt service
type ChatPromptServiceImpl struct {
	service *chatPromptDomain.Service
	logger  *zap.Logger
}

func NewChatPromptService(cfg *config.Config, logger *zap.Logger, db *pgxpool.Pool) (*ChatPromptServiceImpl, error) {
	// Create all required domain repositories
	chatPromptRepo := chatPromptDomain.NewRepositoryImpl(db, logger)
	interestRepo := interestsDomain.NewRepositoryImpl(db, logger)
	profilesRepo := profilesDomain.NewRepository(db, logger)
	tagsRepo := tagsDomain.NewRepositoryImpl(db, logger)
	cityRepo := cityDomain.NewCityRepository(db, logger)
	poiRepo := poiDomain.NewRepository(db, logger)

	// Create domain services
	profilesService := profilesDomain.NewService(context.Background(), profilesRepo, db, logger)

	// Create domain service with all injected dependencies
	chatPromptService, err := chatPromptDomain.NewService(
		context.Background(),
		chatPromptRepo,
		db,
		logger,
		interestRepo,
		profilesRepo,
		profilesService,
		tagsRepo,
		cityRepo,
		poiRepo,
	)
	if err != nil {
		logger.Error("Failed to create chat prompt domain service", zap.Error(err))
		return nil, err
	}

	return &ChatPromptServiceImpl{
		service: chatPromptService,
		logger:  logger,
	}, nil
}

func (s *ChatPromptServiceImpl) Start(ctx context.Context) error {
	s.logger.Info("ChatPrompt gRPC service started")
	return nil
}

func (s *ChatPromptServiceImpl) Stop(ctx context.Context) error {
	s.logger.Info("ChatPrompt gRPC service stopped")
	return nil
}

func (s *ChatPromptServiceImpl) Health() error {
	// Could add database connectivity check here
	return nil
}

func (s *ChatPromptServiceImpl) Type() ServiceType {
	return ChatPromptService
}

// GetGRPCService returns the gRPC service implementation
func (s *ChatPromptServiceImpl) GetGRPCService() *chatPromptDomain.Service {
	return s.service
}
