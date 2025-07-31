package broker

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	"github.com/FACorreiaa/go-poi-au-suggestions/config"
	recentsDomain "github.com/FACorreiaa/go-poi-au-suggestions/internal/domain/recents"
)

// RecentsServiceImpl implements Service for gRPC recents service
type RecentsServiceImpl struct {
	service *recentsDomain.Service
	logger  *zap.Logger
}

func NewRecentsService(cfg *config.Config, logger *zap.Logger, db *pgxpool.Pool) (*RecentsServiceImpl, error) {
	// Create recents domain repository
	recentsRepo := recentsDomain.NewRepository(db, logger)

	// Create domain service
	recentsService := recentsDomain.NewService(context.Background(), recentsRepo, db, logger)

	return &RecentsServiceImpl{
		service: recentsService,
		logger:  logger,
	}, nil
}

func (s *RecentsServiceImpl) Start(ctx context.Context) error {
	s.logger.Info("Recents gRPC service started")
	return nil
}

func (s *RecentsServiceImpl) Stop(ctx context.Context) error {
	s.logger.Info("Recents gRPC service stopped")
	return nil
}

func (s *RecentsServiceImpl) Health() error {
	// Could add database connectivity check here
	return nil
}

func (s *RecentsServiceImpl) Type() ServiceType {
	return RecentsService
}

// GetGRPCService returns the gRPC service implementation
func (s *RecentsServiceImpl) GetGRPCService() *recentsDomain.Service {
	return s.service
}