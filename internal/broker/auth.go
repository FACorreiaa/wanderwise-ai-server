package broker

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	"github.com/FACorreiaa/go-poi-au-suggestions/config"
	authDomain "github.com/FACorreiaa/go-poi-au-suggestions/internal/domain/auth"
)

// AuthServiceImpl implements Service for gRPC authentication
type AuthServiceImpl struct {
	service *authDomain.Service
	logger  *zap.Logger
}

func NewAuthService(cfg *config.Config, logger *zap.Logger, db *pgxpool.Pool) (*AuthServiceImpl, error) {
	authRepo := authDomain.NewPostgresAuthRepo(db, logger)
	authService := authDomain.NewService(context.Background(), authRepo, db, logger)

	return &AuthServiceImpl{
		service: authService,
		logger:  logger,
	}, nil
}

func (s *AuthServiceImpl) Start(ctx context.Context) error {
	s.logger.Info("Auth gRPC service started")
	return nil
}

func (s *AuthServiceImpl) Stop(ctx context.Context) error {
	s.logger.Info("Auth gRPC service stopped")
	return nil
}

func (s *AuthServiceImpl) Health() error {
	// Could add database connectivity check here
	return nil
}

func (s *AuthServiceImpl) Type() ServiceType {
	return AuthService
}

// GetGRPCService returns the gRPC service implementation
func (s *AuthServiceImpl) GetGRPCService() *authDomain.Service {
	return s.service
}
