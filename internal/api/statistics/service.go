package statistics

import (
	"context"
	"log/slog"

	"github.com/FACorreiaa/go-poi-au-suggestions/internal/types"
	uuid "github.com/vgarvardt/pgx-google-uuid/v5"
)

var _ Service = (*ServiceImpl)(nil)

type Service interface {
	GetMainPageStatistics(ctx context.Context, userID uuid.UUID) (*types.MainPageStatistics, error)
	GetDetailedPOIStatistics(ctx context.Context, userID uuid.UUID) (*types.DetailedPOIStatistics, error)
}

type ServiceImpl struct {
	repo   Repository
	logger *slog.Logger
}

func NewService(repo Repository, logger *slog.Logger) *ServiceImpl {
	return &ServiceImpl{
		repo:   repo,
		logger: logger,
	}
}

func (s *ServiceImpl) GetMainPageStatistics(ctx context.Context, userID uuid.UUID) (*types.MainPageStatistics, error) {
	l := s.logger.With(slog.String("method", "GetMainPageStatistics"))
	stats, err := s.repo.GetMainPageStatistics(ctx, userID)
	if err != nil {
		l.ErrorContext(ctx, "Failed to get main page statistics", "error", err)
		return nil, err
	}

	l.InfoContext(ctx, "Successfully retrieved main page statistics")
	return stats, nil
}

func (s *ServiceImpl) GetDetailedPOIStatistics(ctx context.Context, userID uuid.UUID) (*types.DetailedPOIStatistics, error) {
	l := s.logger.With(slog.String("method", "GetDetailedPOIStatistics"))
	stats, err := s.repo.GetDetailedPOIStatistics(ctx, userID)
	if err != nil {
		l.ErrorContext(ctx, "Failed to get detailed POI statistics", "error", err)
		return nil, err
	}

	l.InfoContext(ctx, "Successfully retrieved detailed POI statistics")
	return stats, nil
}
