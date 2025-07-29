package broker

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	"github.com/FACorreiaa/go-poi-au-suggestions/config"
	listsDomain "github.com/FACorreiaa/go-poi-au-suggestions/internal/domain/lists"
)

type ListsServiceImpl struct {
	service *listsDomain.Service
	logger  *zap.Logger
}

func (s *ListsServiceImpl) Start(ctx context.Context) error {
	s.logger.Info("Lists gRPC service started")
	return nil
}

func (s *ListsServiceImpl) Stop(ctx context.Context) error {
	s.logger.Info("Lists gRPC service stopped")
	return nil
}

func (s *ListsServiceImpl) Health() error {
	// Could add database connectivity check here
	return nil
}

func (s *ListsServiceImpl) Type() ServiceType {
	return ListsService
}

func (s *ListsServiceImpl) GetGRPCService() *listsDomain.Service {
	return s.service
}

func NewListsService(cfg *config.Config, logger *zap.Logger, db *pgxpool.Pool) (*ListsServiceImpl, error) {
	listsRepo := listsDomain.NewRepository(db, logger)
	listsService := listsDomain.NewService(context.Background(), listsRepo, db, logger)

	return &ListsServiceImpl{
		service: listsService,
		logger:  logger,
	}, nil
}