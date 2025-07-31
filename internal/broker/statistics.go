package broker

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	"github.com/FACorreiaa/go-poi-au-suggestions/config"
	statisticsDomain "github.com/FACorreiaa/go-poi-au-suggestions/internal/domain/statistics"
)

type StatisticsServiceImpl struct {
	service *statisticsDomain.Service
	logger  *zap.Logger
}

// Compile-time check to ensure StatisticsServiceImpl implements Service
var _ Service = (*StatisticsServiceImpl)(nil)

func NewStatisticsService(cfg *config.Config, logger *zap.Logger, db *pgxpool.Pool) (Service, error) {
	repo := statisticsDomain.NewRepository(logger, db)
	service := statisticsDomain.NewService(context.Background(), repo, db, logger)

	return &StatisticsServiceImpl{
		service: service,
		logger:  logger,
	}, nil
}

func (s *StatisticsServiceImpl) Start(ctx context.Context) error {
	s.logger.Info("Statistics gRPC service started")
	return nil
}

func (s *StatisticsServiceImpl) Stop(ctx context.Context) error {
	s.logger.Info("Statistics gRPC service stopped")
	return nil
}

func (s *StatisticsServiceImpl) Health() error {
	// Could add database connectivity check here
	return nil
}

func (s *StatisticsServiceImpl) Type() ServiceType {
	return StatisticsService
}

func (s *StatisticsServiceImpl) GetGRPCService() *statisticsDomain.Service {
	return s.service
}