package broker

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	"github.com/FACorreiaa/go-poi-au-suggestions/config"
	userDomain "github.com/FACorreiaa/go-poi-au-suggestions/internal/domain/users"
)

type UserServiceImpl struct {
	service *userDomain.Service
	logger  *zap.Logger
}

func NewUserService(cfg *config.Config, logger *zap.Logger, db *pgxpool.Pool) (*UserServiceImpl, error) {
	userRepo := userDomain.NewPostgresUserRepo(db, logger)
	userService := userDomain.NewService(context.Background(), userRepo, db, logger)

	return &UserServiceImpl{
		service: userService,
		logger:  logger,
	}, nil
}

func (s *UserServiceImpl) Start(ctx context.Context) error {
	s.logger.Info("User gRPC service started")
	return nil
}

func (s *UserServiceImpl) Stop(ctx context.Context) error {
	s.logger.Info("User gRPC service stopped")
	return nil
}

func (s *UserServiceImpl) Health() error {
	return nil
}

func (s *UserServiceImpl) Type() ServiceType {
	return UserService
}

// GetGRPCService returns the gRPC service implementation
func (s *UserServiceImpl) GetGRPCService() *userDomain.Service {
	return s.service
}
