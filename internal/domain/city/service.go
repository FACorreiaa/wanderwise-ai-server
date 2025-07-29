package city

import (
	"context"
	"errors"

	pb "github.com/FACorreiaa/loci-proto/modules/city/generated"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	codes2 "go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type Service struct {
	pb.UnimplementedCityServiceServer
	logger *zap.Logger
	repo   Repository
	pgpool *pgxpool.Pool
	tracer trace.Tracer
}

func NewService(ctx context.Context, repo Repository, pgpool *pgxpool.Pool, logger *zap.Logger) *Service {
	return &Service{
		logger: logger.With(zap.String("service", "city")),
		repo:   repo,
		pgpool: pgpool,
		tracer: otel.Tracer("CityService"),
	}
}

func (svc *Service) GetCities(ctx context.Context, req *pb.GetCitiesRequest) (*pb.GetCitiesResponse, error) {
	ctx, span := svc.tracer.Start(ctx, "CityService.GetCities", trace.WithAttributes(
		attribute.String("city.operation", "get_cities"),
		attribute.Int("city.limit", int(req.Limit)),
		attribute.Int("city.offset", int(req.Offset)),
		attribute.String("city.country_code", req.CountryCode),
		attribute.Bool("city.popular_only", req.PopularOnly),
	))
	defer span.End()

	svc.logger.Debug("Getting cities",
		zap.Int32("limit", req.Limit),
		zap.Int32("offset", req.Offset),
		zap.String("country_code", req.CountryCode),
		zap.Bool("popular_only", req.PopularOnly))

	var requestId string
	if req.Request != nil {
		requestId = req.Request.RequestId
	}

	// Set default limit if not provided
	limit := req.Limit
	if limit <= 0 {
		limit = 50 // Default limit
	}
	if limit > 100 {
		limit = 100 // Max limit
	}

	cities, err := svc.repo.GetAllCities(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes2.Error, "Failed to get cities")
		svc.logger.Error("Failed to get cities", zap.Error(err))
		return &pb.GetCitiesResponse{
			Cities:     nil,
			TotalCount: 0,
			Response: &pb.BaseResponse{
				RequestId: requestId,
				Upstream:  "city-service",
				Status:    "error",
			},
		}, status.Error(codes.Internal, "failed to get cities")
	}

	// Convert to protobuf and apply pagination
	offset := int(req.Offset)
	if offset < 0 {
		offset = 0
	}

	totalCount := int32(len(cities))

	// Apply pagination
	var pagedCities []CityDetail
	if offset < len(cities) {
		end := offset + int(limit)
		if end > len(cities) {
			end = len(cities)
		}
		pagedCities = cities[offset:end]
	}

	pbCities := make([]*pb.City, len(pagedCities))
	for i, city := range pagedCities {
		pbCities[i] = convertToPBCity(&city)
	}

	span.SetStatus(codes2.Ok, "Cities retrieved successfully")
	svc.logger.Info("Cities retrieved successfully",
		zap.Int32("total_count", totalCount),
		zap.Int("returned_count", len(pbCities)))

	return &pb.GetCitiesResponse{
		Cities:     pbCities,
		TotalCount: totalCount,
		Response: &pb.BaseResponse{
			RequestId: requestId,
			Upstream:  "city-service",
			Status:    "success",
		},
	}, nil
}

func (svc *Service) GetCity(ctx context.Context, req *pb.GetCityRequest) (*pb.GetCityResponse, error) {
	ctx, span := svc.tracer.Start(ctx, "CityService.GetCity", trace.WithAttributes(
		attribute.String("city.operation", "get_city"),
		attribute.String("city.city_id", req.CityId),
	))
	defer span.End()

	svc.logger.Debug("Getting city", zap.String("city_id", req.CityId))

	var requestId string
	if req.Request != nil {
		requestId = req.Request.RequestId
	}

	// Validate city ID
	if req.CityId == "" {
		span.RecordError(errors.New("city ID is required"))
		span.SetStatus(codes2.Error, "City ID is required")
		svc.logger.Error("City ID is required")
		return &pb.GetCityResponse{
			City: nil,
			Response: &pb.BaseResponse{
				RequestId: requestId,
				Upstream:  "city-service",
				Status:    "error",
			},
		}, status.Error(codes.InvalidArgument, "city ID is required")
	}

	cityUUID, err := uuid.Parse(req.CityId)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes2.Error, "Invalid city ID format")
		svc.logger.Error("Invalid city ID format", zap.Error(err))
		return &pb.GetCityResponse{
			City: nil,
			Response: &pb.BaseResponse{
				RequestId: requestId,
				Upstream:  "city-service",
				Status:    "error",
			},
		}, status.Error(codes.InvalidArgument, "invalid city ID format")
	}

	// Get all cities and find the one with matching ID
	cities, err := svc.repo.GetAllCities(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes2.Error, "Failed to get city")
		svc.logger.Error("Failed to get city", zap.Error(err))
		return &pb.GetCityResponse{
			City: nil,
			Response: &pb.BaseResponse{
				RequestId: requestId,
				Upstream:  "city-service",
				Status:    "error",
			},
		}, status.Error(codes.Internal, "failed to get city")
	}

	var foundCity *CityDetail
	for _, city := range cities {
		if city.ID == cityUUID {
			foundCity = &city
			break
		}
	}

	if foundCity == nil {
		span.SetStatus(codes2.Error, "City not found")
		svc.logger.Warn("City not found", zap.String("city_id", req.CityId))
		return &pb.GetCityResponse{
			City: nil,
			Response: &pb.BaseResponse{
				RequestId: requestId,
				Upstream:  "city-service",
				Status:    "error",
			},
		}, status.Error(codes.NotFound, "city not found")
	}

	span.SetStatus(codes2.Ok, "City retrieved successfully")
	svc.logger.Info("City retrieved successfully", zap.String("city_id", req.CityId))

	return &pb.GetCityResponse{
		City: convertToPBCity(foundCity),
		Response: &pb.BaseResponse{
			RequestId: requestId,
			Upstream:  "city-service",
			Status:    "success",
		},
	}, nil
}

func (svc *Service) SearchCities(ctx context.Context, req *pb.SearchCitiesRequest) (*pb.SearchCitiesResponse, error) {
	ctx, span := svc.tracer.Start(ctx, "CityService.SearchCities", trace.WithAttributes(
		attribute.String("city.operation", "search_cities"),
		attribute.String("city.query", req.Query),
		attribute.String("city.country_code", req.CountryCode),
		attribute.Bool("city.fuzzy_search", req.FuzzySearch),
	))
	defer span.End()

	svc.logger.Debug("Searching cities",
		zap.String("query", req.Query),
		zap.String("country_code", req.CountryCode),
		zap.Bool("fuzzy_search", req.FuzzySearch))

	var requestId string
	if req.Request != nil {
		requestId = req.Request.RequestId
	}

	// Validate query
	if req.Query == "" {
		span.RecordError(errors.New("query is required"))
		span.SetStatus(codes2.Error, "Query is required")
		svc.logger.Error("Query is required")
		return &pb.SearchCitiesResponse{
			Results:    nil,
			TotalCount: 0,
			Response: &pb.BaseResponse{
				RequestId: requestId,
				Upstream:  "city-service",
				Status:    "error",
			},
		}, status.Error(codes.InvalidArgument, "query is required")
	}

	var city *CityDetail
	var err error

	if req.FuzzySearch {
		city, err = svc.repo.FindCityByFuzzyName(ctx, req.Query)
	} else {
		city, err = svc.repo.FindCityByNameAndCountry(ctx, req.Query, req.CountryCode)
	}

	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes2.Error, "Failed to search cities")
		svc.logger.Error("Failed to search cities", zap.Error(err))
		return &pb.SearchCitiesResponse{
			Results:    nil,
			TotalCount: 0,
			Response: &pb.BaseResponse{
				RequestId: requestId,
				Upstream:  "city-service",
				Status:    "error",
			},
		}, status.Error(codes.Internal, "failed to search cities")
	}

	var results []*pb.CitySearchResult
	var totalCount int32 = 0

	if city != nil {
		results = []*pb.CitySearchResult{
			{
				City:           convertToPBCity(city),
				RelevanceScore: 1.0, // Exact match gets full score
				MatchReason:    "exact_name_match",
			},
		}
		totalCount = 1
	}

	span.SetStatus(codes2.Ok, "Cities search completed")
	svc.logger.Info("Cities search completed",
		zap.String("query", req.Query),
		zap.Int32("total_count", totalCount))

	return &pb.SearchCitiesResponse{
		Results:    results,
		TotalCount: totalCount,
		Metadata: &pb.SearchMetadata{
			QueryTimeMs:       0.0, // Could be calculated if needed
			SearchMethod:      "name_search",
			FuzzyMatchingUsed: req.FuzzySearch,
		},
		Response: &pb.BaseResponse{
			RequestId: requestId,
			Upstream:  "city-service",
			Status:    "success",
		},
	}, nil
}

func (svc *Service) GetCityStatistics(ctx context.Context, req *pb.GetCityStatisticsRequest) (*pb.GetCityStatisticsResponse, error) {
	ctx, span := svc.tracer.Start(ctx, "CityService.GetCityStatistics", trace.WithAttributes(
		attribute.String("city.operation", "get_city_statistics"),
		attribute.String("city.city_id", req.CityId),
	))
	defer span.End()

	svc.logger.Debug("Getting city statistics", zap.String("city_id", req.CityId))

	var requestId string
	if req.Request != nil {
		requestId = req.Request.RequestId
	}

	// Validate city ID
	if req.CityId == "" {
		span.RecordError(errors.New("city ID is required"))
		span.SetStatus(codes2.Error, "City ID is required")
		svc.logger.Error("City ID is required")
		return &pb.GetCityStatisticsResponse{
			Statistics: nil,
			Response: &pb.BaseResponse{
				RequestId: requestId,
				Upstream:  "city-service",
				Status:    "error",
			},
		}, status.Error(codes.InvalidArgument, "city ID is required")
	}

	cityUUID, err := uuid.Parse(req.CityId)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes2.Error, "Invalid city ID format")
		svc.logger.Error("Invalid city ID format", zap.Error(err))
		return &pb.GetCityStatisticsResponse{
			Statistics: nil,
			Response: &pb.BaseResponse{
				RequestId: requestId,
				Upstream:  "city-service",
				Status:    "error",
			},
		}, status.Error(codes.InvalidArgument, "invalid city ID format")
	}

	// Get city first to verify it exists
	cities, err := svc.repo.GetAllCities(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes2.Error, "Failed to get city")
		svc.logger.Error("Failed to get city", zap.Error(err))
		return &pb.GetCityStatisticsResponse{
			Statistics: nil,
			Response: &pb.BaseResponse{
				RequestId: requestId,
				Upstream:  "city-service",
				Status:    "error",
			},
		}, status.Error(codes.Internal, "failed to get city")
	}

	var foundCity *CityDetail
	for _, city := range cities {
		if city.ID == cityUUID {
			foundCity = &city
			break
		}
	}

	if foundCity == nil {
		span.SetStatus(codes2.Error, "City not found")
		svc.logger.Warn("City not found", zap.String("city_id", req.CityId))
		return &pb.GetCityStatisticsResponse{
			Statistics: nil,
			Response: &pb.BaseResponse{
				RequestId: requestId,
				Upstream:  "city-service",
				Status:    "error",
			},
		}, status.Error(codes.NotFound, "city not found")
	}

	// Create mock statistics for now
	statistics := &pb.CityStatistics{
		CityId:           req.CityId,
		TotalPois:        0, // Would need to query POI repository
		TotalRestaurants: 0, // Would need to query restaurants repository
		TotalHotels:      0, // Would need to query hotels repository
		TotalAttractions: 0, // Would need to query attractions repository
		UserVisits:       0, // Would need to query user visits
		SavedItineraries: 0, // Would need to query saved itineraries
		AverageRating:    0.0,
		PoiByCategory:    []*pb.CategoryCount{}, // Would need to query POI categories
		LastUpdated:      timestamppb.Now(),
	}

	span.SetStatus(codes2.Ok, "City statistics retrieved successfully")
	svc.logger.Info("City statistics retrieved successfully", zap.String("city_id", req.CityId))

	return &pb.GetCityStatisticsResponse{
		Statistics: statistics,
		Response: &pb.BaseResponse{
			RequestId: requestId,
			Upstream:  "city-service",
			Status:    "success",
		},
	}, nil
}

// Helper function to convert internal CityDetail to protobuf City
func convertToPBCity(city *CityDetail) *pb.City {
	return &pb.City{
		Id:              city.ID.String(),
		Name:            city.Name,
		Country:         city.Country,
		CountryCode:     "", // Not available in current schema
		StateProvince:   city.StateProvince,
		Latitude:        city.CenterLatitude,
		Longitude:       city.CenterLongitude,
		Timezone:        "",         // Not available in current schema
		Population:      int64(0),   // Not available in current schema
		Currency:        "",         // Not available in current schema
		Languages:       []string{}, // Not available in current schema
		Description:     city.AiSummary,
		Highlights:      []string{}, // Not available in current schema
		Climate:         "",         // Not available in current schema
		BestTimeToVisit: "",         // Not available in current schema
		TopAttractions:  []string{}, // Not available in current schema
		Metadata: &pb.CityMetadata{
			ImageUrl:             "",         // Not available in current schema
			ImageGallery:         []string{}, // Not available in current schema
			OfficialWebsite:      "",         // Not available in current schema
			TourismWebsite:       "",         // Not available in current schema
			IsCapital:            false,      // Not available in current schema
			IsPopularDestination: false,      // Could be determined based on popularity metrics
			SafetyRating:         "",         // Not available in current schema
			CostOfLivingIndex:    0.0,        // Not available in current schema
			WalkabilityScore:     "",         // Not available in current schema
			TransportOptions:     []string{}, // Not available in current schema
		},
		CreatedAt: timestamppb.Now(), // Not available in current schema
		UpdatedAt: timestamppb.Now(), // Not available in current schema
	}
}
