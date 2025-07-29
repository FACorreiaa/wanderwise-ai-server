package broker

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	"github.com/FACorreiaa/go-poi-au-suggestions/config"
	profilesDomain "github.com/FACorreiaa/go-poi-au-suggestions/internal/domain/profiles"
)

type ProfilesServiceImpl struct {
	service *profilesDomain.Service
	logger  *zap.Logger
}

func NewProfilesService(cfg *config.Config, logger *zap.Logger, db *pgxpool.Pool) (Service, error) {
	repo := profilesDomain.NewRepository(db, logger)
	service := profilesDomain.NewService(context.Background(), repo, db, logger)

	return &ProfilesServiceImpl{
		service: service,
		logger:  logger,
	}, nil
}

func (s *ProfilesServiceImpl) Start(ctx context.Context) error {
	s.logger.Info("Tags gRPC service started")
	return nil
}

func (s *ProfilesServiceImpl) Stop(ctx context.Context) error {
	s.logger.Info("Tags gRPC service stopped")
	return nil
}

func (s *ProfilesServiceImpl) Health() error {
	// Could add database connectivity check here
	return nil
}

func (s *ProfilesServiceImpl) Type() ServiceType {
	return ProfilesService
}

func (s *ProfilesServiceImpl) GetGRPCService() *profilesDomain.Service {
	return s.service
}
