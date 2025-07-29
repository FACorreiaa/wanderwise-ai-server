package broker

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	"github.com/FACorreiaa/go-poi-au-suggestions/config"
	cityDomain "github.com/FACorreiaa/go-poi-au-suggestions/internal/domain/city"
)

type CityServiceImpl struct {
	service *cityDomain.Service
	logger  *zap.Logger
}

func NewCityService(cfg *config.Config, logger *zap.Logger, db *pgxpool.Pool) (Service, error) {
	repo := cityDomain.NewCityRepository(db, logger)
	service := cityDomain.NewService(context.Background(), repo, db, logger)

	return &CityServiceImpl{
		service: service,
		logger:  logger,
	}, nil
}

func (s *CityServiceImpl) Start(ctx context.Context) error {
	s.logger.Info("City gRPC service started")
	return nil
}

func (s *CityServiceImpl) Stop(ctx context.Context) error {
	s.logger.Info("City gRPC service stopped")
	return nil
}

func (s *CityServiceImpl) Health() error {
	// Could add database connectivity check here
	return nil
}

func (s *CityServiceImpl) Type() ServiceType {
	return CityService
}

func (s *CityServiceImpl) GetGRPCService() *cityDomain.Service {
	return s.service
}