package broker

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	"github.com/FACorreiaa/go-poi-au-suggestions/config"
	poiDomain "github.com/FACorreiaa/go-poi-au-suggestions/internal/domain/poi"
)

// POIServiceImpl implements the Service interface for POI service
type POIServiceImpl struct {
	logger      *zap.Logger
	grpcService *poiDomain.Service
}

// NewPOIService creates a new POI service implementation
func NewPOIService(cfg *config.Config, logger *zap.Logger, db *pgxpool.Pool) (*POIServiceImpl, error) {
	// Create repository
	repo := poiDomain.NewRepository(db, logger)
	
	// Create domain service
	domainService := poiDomain.NewService(context.Background(), repo, db, logger)

	return &POIServiceImpl{
		logger:      logger.With(zap.String("service", "poi")),
		grpcService: domainService,
	}, nil
}

// Start starts the POI service
func (s *POIServiceImpl) Start(ctx context.Context) error {
	s.logger.Info("POI service started")
	return nil
}

// Stop stops the POI service
func (s *POIServiceImpl) Stop(ctx context.Context) error {
	s.logger.Info("POI service stopped")
	return nil
}

// Health checks the health of the POI service
func (s *POIServiceImpl) Health() error {
	return nil
}

// Type returns the service type for POI service
func (s *POIServiceImpl) Type() ServiceType {
	return POIService
}

// GetGRPCService returns the gRPC service instance
func (s *POIServiceImpl) GetGRPCService() *poiDomain.Service {
	return s.grpcService
}