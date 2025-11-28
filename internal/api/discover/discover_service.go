package discover

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/google/uuid"

	"github.com/FACorreiaa/go-poi-au-suggestions/internal/types"
)

type Service interface {
	// Get all discover page data (trending, featured, recent)
	GetDiscoverPageData(ctx context.Context, userID uuid.UUID, limit int) (*types.DiscoverPageData, error)

	// Get trending discoveries
	GetTrendingDiscoveries(ctx context.Context, limit int) ([]types.TrendingDiscovery, error)

	// Get featured collections
	GetFeaturedCollections(ctx context.Context, limit int) ([]types.FeaturedCollection, error)

	// Get user's recent discoveries
	GetRecentDiscoveries(ctx context.Context, userID uuid.UUID, limit int) ([]types.ChatSession, error)

	// Get category results
	GetCategoryResults(ctx context.Context, category string) ([]types.DiscoverResult, error)
}

type ServiceImpl struct {
	repo   Repository
	logger *slog.Logger
}

func NewServiceImpl(repo Repository, logger *slog.Logger) *ServiceImpl {
	return &ServiceImpl{
		repo:   repo,
		logger: logger,
	}
}

// GetDiscoverPageData retrieves all data needed for the discover page
func (s *ServiceImpl) GetDiscoverPageData(ctx context.Context, userID uuid.UUID, limit int) (*types.DiscoverPageData, error) {
	l := s.logger.With(slog.String("service", "GetDiscoverPageData"))
	l.DebugContext(ctx, "Getting discover page data", slog.Int("limit", limit))

	pageData := &types.DiscoverPageData{}

	// Get trending discoveries
	trending, err := s.repo.GetTrendingDiscoveries(ctx, limit)
	if err != nil {
		l.WarnContext(ctx, "Failed to get trending discoveries", slog.Any("error", err))
		// Don't fail the entire request, just set empty array
		trending = []types.TrendingDiscovery{}
	}
	pageData.Trending = trending

	// Get featured collections
	featured, err := s.repo.GetFeaturedCollections(ctx, limit)
	if err != nil {
		l.WarnContext(ctx, "Failed to get featured collections", slog.Any("error", err))
		// Don't fail the entire request, just set empty array
		featured = []types.FeaturedCollection{}
	}
	pageData.Featured = featured

	// Get trending searches today
	trendingSearches, err := s.repo.GetTrendingSearchesToday(ctx, limit)
	if err != nil {
		l.WarnContext(ctx, "Failed to get trending searches", slog.Any("error", err))
		// Don't fail the entire request, just set empty array
		trendingSearches = []types.TrendingSearch{}
	}
	pageData.TrendingSearches = trendingSearches

	// Get recent discoveries if user is authenticated
	if userID != uuid.Nil {
		recent, err := s.repo.GetRecentDiscoveriesByUserID(ctx, userID, limit)
		if err != nil {
			l.WarnContext(ctx, "Failed to get recent discoveries", slog.Any("error", err))
			// Don't fail the entire request, just set empty array
			recent = []types.ChatSession{}
		}
		pageData.RecentDiscoveries = recent
	} else {
		pageData.RecentDiscoveries = []types.ChatSession{}
	}

	l.InfoContext(ctx, "Successfully retrieved discover page data",
		slog.Int("trending_count", len(pageData.Trending)),
		slog.Int("featured_count", len(pageData.Featured)),
		slog.Int("trending_searches_count", len(pageData.TrendingSearches)),
		slog.Int("recent_count", len(pageData.RecentDiscoveries)),
	)

	return pageData, nil
}

// GetTrendingDiscoveries retrieves currently trending discoveries
func (s *ServiceImpl) GetTrendingDiscoveries(ctx context.Context, limit int) ([]types.TrendingDiscovery, error) {
	l := s.logger.With(slog.String("service", "GetTrendingDiscoveries"))
	l.DebugContext(ctx, "Getting trending discoveries", slog.Int("limit", limit))

	trending, err := s.repo.GetTrendingDiscoveries(ctx, limit)
	if err != nil {
		l.ErrorContext(ctx, "Failed to get trending discoveries", slog.Any("error", err))
		return nil, fmt.Errorf("failed to get trending discoveries: %w", err)
	}

	l.InfoContext(ctx, "Successfully retrieved trending discoveries", slog.Int("count", len(trending)))
	return trending, nil
}

// GetFeaturedCollections retrieves featured collections
func (s *ServiceImpl) GetFeaturedCollections(ctx context.Context, limit int) ([]types.FeaturedCollection, error) {
	l := s.logger.With(slog.String("service", "GetFeaturedCollections"))
	l.DebugContext(ctx, "Getting featured collections", slog.Int("limit", limit))

	featured, err := s.repo.GetFeaturedCollections(ctx, limit)
	if err != nil {
		l.ErrorContext(ctx, "Failed to get featured collections", slog.Any("error", err))
		return nil, fmt.Errorf("failed to get featured collections: %w", err)
	}

	l.InfoContext(ctx, "Successfully retrieved featured collections", slog.Int("count", len(featured)))
	return featured, nil
}

// GetRecentDiscoveries retrieves user's recent discoveries
func (s *ServiceImpl) GetRecentDiscoveries(ctx context.Context, userID uuid.UUID, limit int) ([]types.ChatSession, error) {
	l := s.logger.With(slog.String("service", "GetRecentDiscoveries"))
	l.DebugContext(ctx, "Getting recent discoveries",
		slog.String("user_id", userID.String()),
		slog.Int("limit", limit))

	recent, err := s.repo.GetRecentDiscoveriesByUserID(ctx, userID, limit)
	if err != nil {
		l.ErrorContext(ctx, "Failed to get recent discoveries", slog.Any("error", err))
		return nil, fmt.Errorf("failed to get recent discoveries: %w", err)
	}

	l.InfoContext(ctx, "Successfully retrieved recent discoveries",
		slog.String("user_id", userID.String()),
		slog.Int("count", len(recent)))
	return recent, nil
}

// GetCategoryResults retrieves results for a specific category
func (s *ServiceImpl) GetCategoryResults(ctx context.Context, category string) ([]types.DiscoverResult, error) {
	l := s.logger.With(slog.String("service", "GetCategoryResults"))
	l.DebugContext(ctx, "Getting category results", slog.String("category", category))

	// Validate category
	validCategories := map[string]bool{
		"restaurant":    true,
		"hotel":         true,
		"activity":      true,
		"attraction":    true,
		"museum":        true,
		"park":          true,
		"beach":         true,
		"nightlife":     true,
		"shopping":      true,
		"cultural":      true,
		"market":        true,
		"adventure":     true,
		"cafe":          true,
		"bar":           true,
		"entertainment": true,
	}

	if !validCategories[category] {
		return nil, fmt.Errorf("invalid category: %s", category)
	}

	results, err := s.repo.GetPOIsByCategory(ctx, category)
	if err != nil {
		l.ErrorContext(ctx, "Failed to get category results", slog.Any("error", err))
		return nil, fmt.Errorf("failed to get category results: %w", err)
	}

	l.InfoContext(ctx, "Successfully retrieved category results",
		slog.String("category", category),
		slog.Int("count", len(results)))
	return results, nil
}
