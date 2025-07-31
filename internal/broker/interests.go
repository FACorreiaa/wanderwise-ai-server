package broker

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	"github.com/FACorreiaa/go-poi-au-suggestions/config"
	interestsDomain "github.com/FACorreiaa/go-poi-au-suggestions/internal/domain/interests"
)

type InterestsServiceImpl struct {
	service *interestsDomain.Service
	logger  *zap.Logger
}

func NewInterestsService(cfg *config.Config, logger *zap.Logger, db *pgxpool.Pool) (Service, error) {
	repo := interestsDomain.NewRepositoryImpl(db, logger)
	service := interestsDomain.NewService(context.Background(), repo, db, logger)

	return &InterestsServiceImpl{
		service: service,
		logger:  logger,
	}, nil
}

func (s *InterestsServiceImpl) Start(ctx context.Context) error {
	s.logger.Info("Tags gRPC service started")
	return nil
}

func (s *InterestsServiceImpl) Stop(ctx context.Context) error {
	s.logger.Info("Tags gRPC service stopped")
	return nil
}

func (s *InterestsServiceImpl) Health() error {
	// Could add database connectivity check here
	return nil
}

func (s *InterestsServiceImpl) Type() ServiceType {
	return InterestsService
}

func (s *InterestsServiceImpl) GetGRPCService() *interestsDomain.Service {
	return s.service
}
