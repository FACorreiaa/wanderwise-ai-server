package statistics

import (
	"context"

	pb "github.com/FACorreiaa/loci-proto/modules/statistics/generated"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type Service struct {
	pb.UnimplementedStatisticsServiceServer
	logger *zap.Logger
	repo   Repository
	pgpool *pgxpool.Pool
	tracer trace.Tracer
}

func NewService(ctx context.Context, repo Repository, pgpool *pgxpool.Pool, logger *zap.Logger) *Service {
	return &Service{
		logger: logger.With(zap.String("service", "statistics")),
		repo:   repo,
		pgpool: pgpool,
		tracer: otel.Tracer("StatisticsService"),
	}
}

func (svc *Service) GetMainPageStatistics(ctx context.Context, request *pb.GetMainPageStatisticsRequest) (*pb.GetMainPageStatisticsResponse, error) {
	ctx, span := svc.tracer.Start(ctx, "StatisticsService.GetMainPageStatistics")
	defer span.End()

	requestId := "unknown"
	if request.Request != nil {
		requestId = request.Request.RequestId
	}

	svc.logger.Info("Getting main page statistics", zap.String("request_id", requestId))

	// Use system user ID for aggregate statistics
	systemUserID := uuid.MustParse("00000000-0000-0000-0000-000000000000")
	
	stats, err := svc.repo.GetMainPageStatistics(ctx, systemUserID)
	if err != nil {
		svc.logger.Error("Failed to get main page statistics", zap.Error(err), zap.String("request_id", requestId))
		return &pb.GetMainPageStatisticsResponse{
			Statistics: nil,
			Response: &pb.BaseResponse{
				RequestId: requestId,
				Upstream:  "statistics-service",
				Status:    "error",
			},
		}, status.Error(codes.Internal, "failed to get main page statistics")
	}

	svc.logger.Info("Successfully retrieved main page statistics", 
		zap.String("request_id", requestId),
		zap.Int64("total_users", stats.TotalUsersCount),
		zap.Int64("total_itineraries", stats.TotalItinerariesSaved),
		zap.Int64("total_pois", stats.TotalUniquePOIs))

	return &pb.GetMainPageStatisticsResponse{
		Statistics: &pb.MainPageStatistics{
			TotalPois:        stats.TotalUniquePOIs,
			TotalUsers:       stats.TotalUsersCount,
			TotalItineraries: stats.TotalItinerariesSaved,
		},
		Response: &pb.BaseResponse{
			RequestId: requestId,
			Upstream:  "statistics-service",
			Status:    "success",
		},
	}, nil
}

func (svc *Service) StreamMainPageStatistics(req *pb.StreamMainPageStatisticsRequest, stream grpc.ServerStreamingServer[pb.StatisticsEvent]) error {
	_, span := svc.tracer.Start(stream.Context(), "StatisticsService.StreamMainPageStatistics")
	defer span.End()

	requestId := "unknown"
	if req.Request != nil {
		requestId = req.Request.RequestId
	}

	svc.logger.Info("StreamMainPageStatistics not implemented yet", zap.String("request_id", requestId))
	
	return status.Error(codes.Unimplemented, "StreamMainPageStatistics not implemented yet")
}

func (svc *Service) GetDetailedPOIStatistics(ctx context.Context, req *pb.GetDetailedPOIStatisticsRequest) (*pb.GetDetailedPOIStatisticsResponse, error) {
	ctx, span := svc.tracer.Start(ctx, "StatisticsService.GetDetailedPOIStatistics")
	defer span.End()

	requestId := "unknown"
	if req.Request != nil {
		requestId = req.Request.RequestId
	}

	if req.UserId == "" {
		svc.logger.Error("User ID is required", zap.String("request_id", requestId))
		return &pb.GetDetailedPOIStatisticsResponse{
			Statistics: nil,
			Response: &pb.BaseResponse{
				RequestId: requestId,
				Upstream:  "statistics-service",
				Status:    "error",
			},
		}, status.Error(codes.InvalidArgument, "user_id is required")
	}

	userID, err := uuid.Parse(req.UserId)
	if err != nil {
		svc.logger.Error("Invalid user ID format", zap.Error(err), zap.String("request_id", requestId))
		return &pb.GetDetailedPOIStatisticsResponse{
			Statistics: nil,
			Response: &pb.BaseResponse{
				RequestId: requestId,
				Upstream:  "statistics-service",
				Status:    "error",
			},
		}, status.Error(codes.InvalidArgument, "invalid user_id format")
	}

	svc.logger.Info("Getting detailed POI statistics", 
		zap.String("request_id", requestId), 
		zap.String("user_id", req.UserId))

	stats, err := svc.repo.GetDetailedPOIStatistics(ctx, userID)
	if err != nil {
		svc.logger.Error("Failed to get detailed POI statistics", 
			zap.Error(err), 
			zap.String("request_id", requestId),
			zap.String("user_id", req.UserId))
		return &pb.GetDetailedPOIStatisticsResponse{
			Statistics: nil,
			Response: &pb.BaseResponse{
				RequestId: requestId,
				Upstream:  "statistics-service",
				Status:    "error",
			},
		}, status.Error(codes.Internal, "failed to get detailed POI statistics")
	}

	svc.logger.Info("Successfully retrieved detailed POI statistics",
		zap.String("request_id", requestId),
		zap.String("user_id", req.UserId),
		zap.Int64("general_pois", stats.GeneralPOIs),
		zap.Int64("suggested_pois", stats.SuggestedPOIs),
		zap.Int64("hotels", stats.Hotels),
		zap.Int64("restaurants", stats.Restaurants),
		zap.Int64("total_pois", stats.TotalPOIs))

	return &pb.GetDetailedPOIStatisticsResponse{
		Statistics: &pb.DetailedPOIStatistics{
			UserId:             req.UserId,
			TotalPoiSearches:   int32(stats.TotalPOIs),
			FavoritePoisCount:  int32(stats.GeneralPOIs + stats.SuggestedPOIs),
			VisitedCitiesCount: 0, // Not available in current domain model
		},
		Response: &pb.BaseResponse{
			RequestId: requestId,
			Upstream:  "statistics-service",
			Status:    "success",
		},
	}, nil
}

func (svc *Service) GetLandingPageStatistics(ctx context.Context, req *pb.GetLandingPageStatisticsRequest) (*pb.GetLandingPageStatisticsResponse, error) {
	ctx, span := svc.tracer.Start(ctx, "StatisticsService.GetLandingPageStatistics")
	defer span.End()

	requestId := "unknown"
	if req.Request != nil {
		requestId = req.Request.RequestId
	}

	if req.UserId == "" {
		svc.logger.Error("User ID is required", zap.String("request_id", requestId))
		return &pb.GetLandingPageStatisticsResponse{
			Statistics: nil,
			Response: &pb.BaseResponse{
				RequestId: requestId,
				Upstream:  "statistics-service",
				Status:    "error",
			},
		}, status.Error(codes.InvalidArgument, "user_id is required")
	}

	userID, err := uuid.Parse(req.UserId)
	if err != nil {
		svc.logger.Error("Invalid user ID format", zap.Error(err), zap.String("request_id", requestId))
		return &pb.GetLandingPageStatisticsResponse{
			Statistics: nil,
			Response: &pb.BaseResponse{
				RequestId: requestId,
				Upstream:  "statistics-service",
				Status:    "error",
			},
		}, status.Error(codes.InvalidArgument, "invalid user_id format")
	}

	svc.logger.Info("Getting landing page statistics", 
		zap.String("request_id", requestId), 
		zap.String("user_id", req.UserId))

	stats, err := svc.repo.LandingPageStatistics(ctx, userID)
	if err != nil {
		svc.logger.Error("Failed to get landing page statistics", 
			zap.Error(err), 
			zap.String("request_id", requestId),
			zap.String("user_id", req.UserId))
		return &pb.GetLandingPageStatisticsResponse{
			Statistics: nil,
			Response: &pb.BaseResponse{
				RequestId: requestId,
				Upstream:  "statistics-service",
				Status:    "error",
			},
		}, status.Error(codes.Internal, "failed to get landing page statistics")
	}

	svc.logger.Info("Successfully retrieved landing page statistics",
		zap.String("request_id", requestId),
		zap.String("user_id", req.UserId),
		zap.Int("saved_places", stats.SavedPlaces),
		zap.Int("itineraries", stats.Itineraries),
		zap.Int("cities_explored", stats.CitiesExplored),
		zap.Int("discoveries", stats.Discoveries))

	return &pb.GetLandingPageStatisticsResponse{
		Statistics: &pb.LandingPageUserStats{
			UserId:                          req.UserId,
			SearchesThisWeek:                int32(stats.Discoveries),        // Map discoveries to searches
			NewFavoritesThisWeek:            int32(stats.SavedPlaces),       // Map saved places to favorites
			ItinerariesCreatedThisMonth:     int32(stats.Itineraries),      // Direct mapping
			RecentlySearchedCities:          []string{},                    // Empty for now
		},
		Response: &pb.BaseResponse{
			RequestId: requestId,
			Upstream:  "statistics-service",
			Status:    "success",
		},
	}, nil
}

func (svc *Service) GetUserActivityAnalytics(ctx context.Context, req *pb.GetUserActivityAnalyticsRequest) (*pb.GetUserActivityAnalyticsResponse, error) {
	ctx, span := svc.tracer.Start(ctx, "StatisticsService.GetUserActivityAnalytics")
	defer span.End()

	requestId := "unknown"
	if req.Request != nil {
		requestId = req.Request.RequestId
	}

	svc.logger.Info("GetUserActivityAnalytics not implemented yet", zap.String("request_id", requestId))

	return &pb.GetUserActivityAnalyticsResponse{
		ActivityData: nil,
		Summary:      nil,
		Response: &pb.BaseResponse{
			RequestId: requestId,
			Upstream:  "statistics-service",
			Status:    "not_implemented",
		},
	}, status.Error(codes.Unimplemented, "GetUserActivityAnalytics not implemented yet")
}

func (svc *Service) GetSystemAnalytics(ctx context.Context, req *pb.GetSystemAnalyticsRequest) (*pb.GetSystemAnalyticsResponse, error) {
	ctx, span := svc.tracer.Start(ctx, "StatisticsService.GetSystemAnalytics")
	defer span.End()

	requestId := "unknown"
	if req.Request != nil {
		requestId = req.Request.RequestId
	}

	svc.logger.Info("GetSystemAnalytics not implemented yet", zap.String("request_id", requestId))

	return &pb.GetSystemAnalyticsResponse{
		Analytics: nil,
		Response: &pb.BaseResponse{
			RequestId: requestId,
			Upstream:  "statistics-service",
			Status:    "not_implemented",
		},
	}, status.Error(codes.Unimplemented, "GetSystemAnalytics not implemented yet")
}
