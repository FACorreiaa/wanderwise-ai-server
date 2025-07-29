package broker

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	"github.com/FACorreiaa/go-poi-au-suggestions/config"
	tagsDomain "github.com/FACorreiaa/go-poi-au-suggestions/internal/domain/tags"
)

type TagServiceImpl struct {
	service *tagsDomain.Service
	logger  *zap.Logger
}

func (s *TagServiceImpl) Start(ctx context.Context) error {
	s.logger.Info("Tags gRPC service started")
	return nil
}

func (s *TagServiceImpl) Stop(ctx context.Context) error {
	s.logger.Info("Tags gRPC service stopped")
	return nil
}

func (s *TagServiceImpl) Health() error {
	// Could add database connectivity check here
	return nil
}

func (s *TagServiceImpl) Type() ServiceType {
	return TagsService
}

func (s *TagServiceImpl) GetGRPCService() *tagsDomain.Service {
	return s.service
}

func NewTagService(cfg *config.Config, logger *zap.Logger, db *pgxpool.Pool) (*TagServiceImpl, error) {
	tagsRepo := tagsDomain.NewRepositoryImpl(db, logger)
	tagsService := tagsDomain.NewService(context.Background(), tagsRepo, db, logger)

	return &TagServiceImpl{
		service: tagsService,
		logger:  logger,
	}, nil
}
