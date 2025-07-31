package poi

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	generativeAI "github.com/FACorreiaa/go-genai-sdk/lib"
	"github.com/patrickmn/go-cache"

	pb "github.com/FACorreiaa/loci-proto/modules/poi/generated"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/FACorreiaa/go-poi-au-suggestions/internal/domain"
)

type Service struct {
	pb.UnimplementedPOIServiceServer
	logger           *zap.Logger
	repo             Repository
	pgpool           *pgxpool.Pool
	tracer           trace.Tracer
	aiClient         *generativeAI.LLMChatClient
	embeddingService *generativeAI.EmbeddingService
	cache            *cache.Cache
	cacheTTL         time.Duration
}

func NewService(ctx context.Context, repo Repository, pgpool *pgxpool.Pool, logger *zap.Logger) *Service {
	return &Service{
		logger:   logger.With(zap.String("service", "poi")),
		repo:     repo,
		pgpool:   pgpool,
		tracer:   otel.Tracer("PoiService"),
		cache:    cache.New(5*time.Minute, 10*time.Minute),
		cacheTTL: 5 * time.Minute,
	}
}

func (svc *Service) GetPOIsByCity(ctx context.Context, req *pb.GetPOIsByCityRequest) (*pb.GetPOIsByCityResponse, error) {
	ctx, span := svc.tracer.Start(ctx, "POIService.GetPOIsByCity", trace.WithAttributes(
		attribute.String("poi.city_id", req.CityId),
	))
	defer span.End()

	if req.CityId == "" {
		return nil, status.Errorf(codes.InvalidArgument, "city ID is required")
	}

	cityUUID, err := uuid.Parse(req.CityId)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid city ID: %v", err)
	}

	svc.logger.Info("Getting POIs by city", zap.String("city_id", req.CityId))

	// Use the API service for business logic
	pois, err := svc.repo.GetPOIsByCityID(ctx, cityUUID)
	if err != nil {
		svc.logger.Error("Failed to get POIs by city", zap.Error(err))
		span.RecordError(err)
		return nil, status.Errorf(codes.Internal, "failed to get POIs by city: %v", err)
	}

	// Convert to protobuf format
	pbPOIs := make([]*pb.POIDetailedInfo, len(pois))
	for i, poi := range pois {
		pbPOIs[i] = convertPOIDetailedInfoToProto(poi)
	}

	svc.logger.Info("Successfully retrieved POIs by city",
		zap.String("city_id", req.CityId),
		zap.Int("count", len(pois)))

	span.SetAttributes(attribute.Int("results.count", len(pois)))

	return &pb.GetPOIsByCityResponse{
		Pois: pbPOIs,
	}, nil
}

func (svc *Service) SearchPOIs(ctx context.Context, req *pb.SearchPOIsRequest) (*pb.SearchPOIsResponse, error) {
	if req.Filter == nil {
		return nil, status.Errorf(codes.InvalidArgument, "filter is required")
	}
	if req.Filter.Location == nil {
		return nil, status.Errorf(codes.InvalidArgument, "location is required")
	}

	ctx, span := svc.tracer.Start(ctx, "POIService.SearchPOIs", trace.WithAttributes(
		attribute.Float64("poi.latitude", req.Filter.Location.Latitude),
		attribute.Float64("poi.longitude", req.Filter.Location.Longitude),
		attribute.Float64("poi.radius", req.Filter.RadiusMeters),
		attribute.String("poi.query", req.Filter.Query),
	))
	defer span.End()

	if err := ValidateLocationAndRadius(req.Filter.Location.Latitude, req.Filter.Location.Longitude, req.Filter.RadiusMeters); err != nil {
		return nil, err
	}

	svc.logger.Info("Searching POIs",
		zap.Float64("latitude", req.Filter.Location.Latitude),
		zap.Float64("longitude", req.Filter.Location.Longitude),
		zap.Float64("radius", req.Filter.RadiusMeters),
		zap.String("query", req.Filter.Query))

	// Build filter
	filter := POIFilter{
		Location: GeoPoint{
			Latitude:  req.Filter.Location.Latitude,
			Longitude: req.Filter.Location.Longitude,
		},
		Radius:   req.Filter.RadiusMeters,
		Category: "", // Use first category if available
	}
	if len(req.Filter.Categories) > 0 {
		filter.Category = req.Filter.Categories[0]
	}

	// Use the API service for business logic
	pois, err := svc.repo.SearchPOIs(ctx, filter)
	if err != nil {
		svc.logger.Error("Failed to search POIs", zap.Error(err))
		span.RecordError(err)
		return nil, status.Errorf(codes.Internal, "failed to search POIs: %v", err)
	}

	// Convert to protobuf format
	pbPOIs := make([]*pb.POIDetailedInfo, len(pois))
	for i, poi := range pois {
		pbPOIs[i] = convertPOIDetailedInfoToProto(poi)
	}

	svc.logger.Info("Successfully searched POIs",
		zap.Int("count", len(pois)),
		zap.String("query", req.Filter.Query))

	span.SetAttributes(attribute.Int("results.count", len(pois)))

	return &pb.SearchPOIsResponse{
		Pois:       pbPOIs,
		TotalCount: int32(len(pois)),
	}, nil
}

//func (svc *Service) SearchPOIsSemantic(ctx context.Context, req *pb.SearchPOIsSemanticRequest) (*pb.SearchPOIsSemanticResponse, error) {
//	ctx, span := svc.tracer.Start(ctx, "POIService.SearchPOIsSemantic", trace.WithAttributes(
//		attribute.String("poi.query", req.Query),
//		attribute.Int("poi.limit", int(req.Limit)),
//	))
//	defer span.End()
//
//	if req.Query == "" {
//		return nil, status.Errorf(codes.InvalidArgument, "query is required")
//	}
//
//	limit := int(req.Limit)
//	if limit <= 0 {
//		limit = 10
//	}
//	if limit > 100 {
//		limit = 100
//	}
//
//	svc.logger.Info("Performing semantic POI search",
//		zap.String("query", req.Query),
//		zap.Int("limit", limit))
//
//	pois, err := svc.repo.FindSimilarPOIs(ctx, req.Query, limit)
//	if err != nil {
//		svc.logger.Error("Failed to perform semantic POI search", zap.Error(err))
//		span.RecordError(err)
//		return nil, status.Errorf(codes.Internal, "failed to perform semantic POI search: %v", err)
//	}
//
//	// Convert to protobuf format
//	pbPOIs := make([]*pb.POISemanticMatch, len(pois))
//	for i, poi := range pois {
//		pbPOIs[i] = &pb.POISemanticMatch{
//			Poi:             convertPOIDetailedInfoToProto(poi),
//			SimilarityScore: 1.0, // Default score, should come from semantic search
//			MatchReason:     "semantic similarity",
//		}
//	}
//
//	svc.logger.Info("Successfully performed semantic POI search",
//		zap.String("query", req.Query),
//		zap.Int("count", len(pois)))
//
//	span.SetAttributes(attribute.Int("results.count", len(pois)))
//
//	return &pb.SearchPOIsSemanticResponse{
//		Pois:       pbPOIs,
//		TotalCount: int32(len(pois)),
//	}, nil
//}
//
//func (svc *Service) SearchPOIsSemanticByCity(ctx context.Context, req *pb.SearchPOIsSemanticByCityRequest) (*pb.SearchPOIsSemanticResponse, error) {
//	ctx, span := svc.tracer.Start(ctx, "POIService.SearchPOIsSemanticByCity", trace.WithAttributes(
//		attribute.String("poi.query", req.Query),
//		attribute.String("poi.city_id", req.CityId),
//		attribute.Int("poi.limit", int(req.Limit)),
//	))
//	defer span.End()
//
//	if req.Query == "" {
//		return nil, status.Errorf(codes.InvalidArgument, "query is required")
//	}
//	if req.CityId == "" {
//		return nil, status.Errorf(codes.InvalidArgument, "city ID is required")
//	}
//
//	cityUUID, err := uuid.Parse(req.CityId)
//	if err != nil {
//		return nil, status.Errorf(codes.InvalidArgument, "invalid city ID: %v", err)
//	}
//
//	limit := int(req.Limit)
//	if limit <= 0 {
//		limit = 10
//	}
//	if limit > 100 {
//		limit = 100
//	}
//
//	svc.logger.Info("Performing semantic POI search by city",
//		zap.String("query", req.Query),
//		zap.String("city_id", req.CityId),
//		zap.Int("limit", limit))
//
//	// Use the API service for business logic
//	pois, err := svc.poiService.SearchPOIsSemanticByCity(ctx, req.Query, cityUUID, limit)
//	if err != nil {
//		svc.logger.Error("Failed to perform semantic POI search by city", zap.Error(err))
//		span.RecordError(err)
//		return nil, status.Errorf(codes.Internal, "failed to perform semantic POI search by city: %v", err)
//	}
//
//	// Convert to protobuf format
//	pbPOIs := make([]*pb.POISemanticMatch, len(pois))
//	for i, poi := range pois {
//		pbPOIs[i] = &pb.POISemanticMatch{
//			Poi:             convertPOIDetailedInfoToProto(poi),
//			SimilarityScore: 1.0, // Default score, should come from semantic search
//			MatchReason:     "semantic similarity by city",
//		}
//	}
//
//	svc.logger.Info("Successfully performed semantic POI search by city",
//		zap.String("query", req.Query),
//		zap.String("city_id", req.CityId),
//		zap.Int("count", len(pois)))
//
//	span.SetAttributes(attribute.Int("results.count", len(pois)))
//
//	return &pb.SearchPOIsSemanticResponse{
//		Pois:       pbPOIs,
//		TotalCount: int32(len(pois)),
//	}, nil
//}

func (svc *Service) SearchPOIsHybrid(ctx context.Context, req *pb.SearchPOIsHybridRequest) (*pb.SearchPOIsHybridResponse, error) {
	if req.SemanticQuery == "" {
		return nil, status.Errorf(codes.InvalidArgument, "semantic query is required")
	}
	if req.Filter == nil {
		return nil, status.Errorf(codes.InvalidArgument, "filter is required")
	}
	if req.Filter.Location == nil {
		return nil, status.Errorf(codes.InvalidArgument, "location is required")
	}

	ctx, span := svc.tracer.Start(ctx, "POIService.SearchPOIsHybrid", trace.WithAttributes(
		attribute.String("poi.query", req.SemanticQuery),
		attribute.Float64("poi.latitude", req.Filter.Location.Latitude),
		attribute.Float64("poi.longitude", req.Filter.Location.Longitude),
		attribute.Float64("poi.radius", req.Filter.RadiusMeters),
		attribute.Float64("poi.semantic_weight", req.SemanticWeight),
	))
	defer span.End()

	if err := ValidateLocationAndRadius(req.Filter.Location.Latitude, req.Filter.Location.Longitude, req.Filter.RadiusMeters); err != nil {
		return nil, err
	}
	if req.SemanticWeight < 0 || req.SemanticWeight > 1 {
		return nil, status.Errorf(codes.InvalidArgument, "semantic weight must be between 0 and 1")
	}

	svc.logger.Info("Performing hybrid POI search",
		zap.String("query", req.SemanticQuery),
		zap.Float64("latitude", req.Filter.Location.Latitude),
		zap.Float64("longitude", req.Filter.Location.Longitude),
		zap.Float64("radius", req.Filter.RadiusMeters),
		zap.Float64("semantic_weight", req.SemanticWeight))

	// Build filter
	filter := POIFilter{
		Location: GeoPoint{
			Latitude:  req.Filter.Location.Latitude,
			Longitude: req.Filter.Location.Longitude,
		},
		Radius:   req.Filter.RadiusMeters,
		Category: "", // Use first category if available
	}
	if len(req.Filter.Categories) > 0 {
		filter.Category = req.Filter.Categories[0]
	}

	queryEmbedding, err := svc.embeddingService.GenerateQueryEmbedding(ctx, req.SemanticQuery)
	if err != nil {
		svc.logger.Error("Failed to generate query embedding",
			zap.Any("error", err))
		span.RecordError(err)
		return nil, fmt.Errorf("failed to generate query embedding: %w", err)
	}

	pois, err := svc.repo.SearchPOIsHybrid(ctx, filter, queryEmbedding, req.SemanticWeight)
	if err != nil {
		svc.logger.Error("Failed to perform hybrid POI search", zap.Error(err))
		span.RecordError(err)
		return nil, status.Errorf(codes.Internal, "failed to perform hybrid POI search: %v", err)
	}

	// Convert to protobuf format
	pbPOIs := make([]*pb.POIHybridMatch, len(pois))
	for i, poi := range pois {
		pbPOIs[i] = &pb.POIHybridMatch{
			Poi:           convertPOIDetailedInfoToProto(poi),
			SpatialScore:  0.8, // Default scores - should come from hybrid search
			SemanticScore: 0.7,
			CombinedScore: 0.75,
		}
	}

	svc.logger.Info("Successfully performed hybrid POI search",
		zap.String("query", req.SemanticQuery),
		zap.Int("count", len(pois)))

	span.SetAttributes(attribute.Int("results.count", len(pois)))

	return &pb.SearchPOIsHybridResponse{
		Pois:       pbPOIs,
		TotalCount: int32(len(pois)),
	}, nil
}

func (svc *Service) GetNearbyRecommendations(ctx context.Context, req *pb.GetNearbyRecommendationsRequest) (*pb.GetNearbyRecommendationsResponse, error) {
	// Check user authentication
	userID, err := domain.CheckUserAuth(ctx)
	if err != nil {
		return nil, err
	}

	if req.Location == nil {
		return nil, status.Errorf(codes.InvalidArgument, "location is required")
	}

	ctx, span := svc.tracer.Start(ctx, "POIService.GetNearbyRecommendations", trace.WithAttributes(
		attribute.String("poi.user_id", userID),
		attribute.Float64("poi.latitude", req.Location.Latitude),
		attribute.Float64("poi.longitude", req.Location.Longitude),
		attribute.Float64("poi.distance", req.RadiusMeters),
	))
	defer span.End()

	userUUID, err := uuid.Parse(userID)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid user ID: %v", err)
	}

	// Generate cache key
	cacheKey := generateFilteredPOICacheKey(req.Location.Latitude, req.Location.Longitude, req.Location.Limit, userUUID)

	// Check cache first and convert cached data to proto format
	if cached, found := svc.cache.Get(cacheKey); found {
		if pois, ok := cached.([]POIDetailedInfo); ok {
			svc.logger.Info("Serving POI recommendations from cache")

			// Convert cached POIs to protobuf format
			pbRecommendations := make([]*pb.POIRecommendation, len(pois))
			for i, poi := range pois {
				pbRecommendations[i] = &pb.POIRecommendation{
					Poi:                  convertPOIDetailedInfoToProto(poi),
					RecommendationScore:  0.8, // Default score
					RecommendationReason: "nearby location match",
					Tags:                 poi.Tags,
				}
			}

			return &pb.GetNearbyRecommendationsResponse{
				Recommendations: pbRecommendations,
			}, nil
		}
	}

	if err := ValidateLocationAndRadius(req.Location.Latitude, req.Location.Longitude, req.RadiusMeters); err != nil {
		return nil, err
	}

	svc.logger.Info("Getting nearby recommendations",
		zap.String("user_id", userID),
		zap.Float64("latitude", req.Location.Latitude),
		zap.Float64("longitude", req.Location.Longitude),
		zap.Float64("distance", req.RadiusMeters))

	// Get POIs from database
	pois, err := svc.repo.GetPOIsByLocationAndDistance(ctx, req.Location.Latitude, req.Location.Longitude, req.RadiusMeters)
	if err != nil {
		svc.logger.Error("Failed to get nearby recommendations", zap.Error(err))
		span.RecordError(err)
		return nil, status.Errorf(codes.Internal, "failed to get nearby recommendations: %v", err)
	}

	// Cache the results
	svc.cache.Set(cacheKey, pois, svc.cacheTTL)

	// Convert to protobuf format
	pbRecommendations := make([]*pb.POIRecommendation, len(pois))
	for i, poi := range pois {
		pbRecommendations[i] = &pb.POIRecommendation{
			Poi:                  convertPOIDetailedInfoToProto(poi),
			RecommendationScore:  0.8, // Default score
			RecommendationReason: "nearby location match",
			Tags:                 poi.Tags,
		}
	}

	svc.logger.Info("Successfully retrieved nearby recommendations",
		zap.String("user_id", userID),
		zap.Int("count", len(pois)))

	span.SetAttributes(attribute.Int("results.count", len(pois)))

	return &pb.GetNearbyRecommendationsResponse{
		Recommendations: pbRecommendations,
	}, nil
}

func (svc *Service) DiscoverRestaurants(ctx context.Context, req *pb.DiscoverRestaurantsRequest) (*pb.DiscoverRestaurantsResponse, error) {
	// Check user authentication
	userID, err := domain.CheckUserAuth(ctx)
	if err != nil {
		return nil, err
	}

	ctx, span := svc.tracer.Start(ctx, "POIService.DiscoverRestaurants", trace.WithAttributes(
		attribute.String("poi.user_id", userID),
		attribute.Float64("poi.latitude", float64(req.Location.Latitude)),
		attribute.Float64("poi.longitude", float64(req.Location.Longitude)),
		attribute.Float64("poi.distance", float64(req.RadiusMeters)),
	))
	defer span.End()

	userUUID, err := uuid.Parse(userID)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid user ID: %v", err)
	}
	cacheKey := fmt.Sprintf("restaurants_%f_%f_%f_%s_%s_%s", req.Location.Latitude,
		req.Location.Longitude,
		req.Location.Limit,
		userUUID,
		req.CuisineTypes,
		req.PriceRanges)

	if cached, found := svc.cache.Get(cacheKey); found {
		if pois, ok := cached.([]RestaurantDetailedInfo); ok {
			svc.logger.Info("Serving restaurants from cache")
			pbRestaurants := make([]*pb.RestaurantDetailedInfo, len(pois))
			for i, poi := range pois {
				pbRestaurants[i] = convertRestaurantDetailedInfoToProto(poi)
			}

			return &pb.DiscoverRestaurantsResponse{
				Restaurants: pbRestaurants,
			}, nil
		}
	}

	if err := ValidateLocationAndRadius(req.Location.Latitude, req.Location.Longitude, req.RadiusMeters); err != nil {
		return nil, err
	}

	svc.logger.Info("Discovering restaurants",
		zap.String("user_id", userID),
		zap.Float64("latitude", req.Location.Latitude),
		zap.Float64("longitude", req.Location.Longitude),
		zap.Float64("distance", req.RadiusMeters))

	// Use the API service for business logic
	restaurants, err := svc.repo.GetPOIsByLocationAndDistanceWithCategory(ctx, float64(req.Location.Latitude), float64(req.Location.Longitude), float64(req.RadiusMeters), "restaurant")
	if err != nil {
		svc.logger.Error("Failed to discover restaurants", zap.Error(err))
		span.RecordError(err)
		return nil, status.Errorf(codes.Internal, "failed to discover restaurants: %v", err)
	}

	// Convert to protobuf format
	pbRestaurants := make([]*pb.RestaurantDetailedInfo, len(restaurants))
	for i, restaurant := range restaurants {
		// Convert POIDetailedInfo to RestaurantDetailedInfo
		restaurantDetail := RestaurantDetailedInfo{
			ID:           restaurant.ID,
			City:         restaurant.City,
			Name:         restaurant.Name,
			Latitude:     restaurant.Latitude,
			Longitude:    restaurant.Longitude,
			Category:     restaurant.Category,
			Description:  restaurant.Description,
			Address:      &restaurant.Address,
			Website:      &restaurant.Website,
			PhoneNumber:  &restaurant.PhoneNumber,
			Rating:       restaurant.Rating,
			OpeningHours: restaurant.OpeningHours,
			Tags:         restaurant.Tags,
			PhotoUrls:    restaurant.PhotoUrls,
			IsVerified:   restaurant.IsVerified,
		}
		if restaurant.PriceLevel != "" {
			restaurantDetail.PriceLevel = &restaurant.PriceLevel
		}
		if restaurant.ReviewCount != nil && *restaurant.ReviewCount > 0 {
			reviewCountStr := strconv.Itoa(*restaurant.ReviewCount)
			restaurantDetail.ReviewCount = &reviewCountStr
		}
		pbRestaurants[i] = convertRestaurantDetailedInfoToProto(restaurantDetail)
	}

	svc.logger.Info("Successfully discovered restaurants",
		zap.String("user_id", userID),
		zap.Int("count", len(restaurants)))

	span.SetAttributes(attribute.Int("results.count", len(restaurants)))

	return &pb.DiscoverRestaurantsResponse{
		Restaurants: pbRestaurants,
	}, nil
}

func (svc *Service) DiscoverActivities(ctx context.Context, req *pb.DiscoverActivitiesRequest) (*pb.DiscoverActivitiesResponse, error) {
	// Check user authentication
	userID, err := domain.CheckUserAuth(ctx)
	if err != nil {
		return nil, err
	}

	ctx, span := svc.tracer.Start(ctx, "POIService.DiscoverActivities", trace.WithAttributes(
		attribute.String("poi.user_id", userID),
		attribute.Float64("poi.latitude", float64(req.Location.Latitude)),
		attribute.Float64("poi.longitude", float64(req.Location.Longitude)),
		attribute.Float64("poi.distance", float64(req.RadiusMeters)),
	))
	defer span.End()

	//userUUID, err := uuid.Parse(userID)
	//if err != nil {
	//	return nil, status.Errorf(codes.InvalidArgument, "invalid user ID: %v", err)
	//}

	if err := ValidateLocationAndRadius(req.Location.Latitude, req.Location.Longitude, req.RadiusMeters); err != nil {
		return nil, err
	}

	svc.logger.Info("Discovering activities",
		zap.String("user_id", userID),
		zap.Float64("latitude", req.Location.Latitude),
		zap.Float64("longitude", req.Location.Longitude),
		zap.Float64("distance", req.RadiusMeters))

	// Use the repository for business logic
	activities, err := svc.repo.GetPOIsByLocationAndDistanceWithCategory(ctx, float64(req.Location.Latitude), float64(req.Location.Longitude), float64(req.RadiusMeters), "activity")
	if err != nil {
		svc.logger.Error("Failed to discover activities", zap.Error(err))
		span.RecordError(err)
		return nil, status.Errorf(codes.Internal, "failed to discover activities: %v", err)
	}

	// Convert to protobuf format
	pbActivities := make([]*pb.POIDetailedInfo, len(activities))
	for i, activity := range activities {
		pbActivities[i] = convertPOIDetailedInfoToProto(activity)
	}

	svc.logger.Info("Successfully discovered activities",
		zap.String("user_id", userID),
		zap.Int("count", len(activities)))

	span.SetAttributes(attribute.Int("results.count", len(activities)))

	return &pb.DiscoverActivitiesResponse{
		Activities: pbActivities,
	}, nil
}

func (svc *Service) DiscoverHotels(ctx context.Context, req *pb.DiscoverHotelsRequest) (*pb.DiscoverHotelsResponse, error) {
	// Check user authentication
	userID, err := domain.CheckUserAuth(ctx)
	if err != nil {
		return nil, err
	}

	ctx, span := svc.tracer.Start(ctx, "POIService.DiscoverHotels", trace.WithAttributes(
		attribute.String("poi.user_id", userID),
		attribute.Float64("poi.latitude", float64(req.Location.Latitude)),
		attribute.Float64("poi.longitude", float64(req.Location.Longitude)),
		attribute.Float64("poi.distance", float64(req.RadiusMeters)),
	))
	defer span.End()

	//userUUID, err := uuid.Parse(userID)
	//if err != nil {
	//	return nil, status.Errorf(codes.InvalidArgument, "invalid user ID: %v", err)
	//}

	if err := ValidateLocationAndRadius(req.Location.Latitude, req.Location.Longitude, req.RadiusMeters); err != nil {
		return nil, err
	}

	svc.logger.Info("Discovering hotels",
		zap.String("user_id", userID),
		zap.Float64("latitude", req.Location.Latitude),
		zap.Float64("longitude", req.Location.Longitude),
		zap.Float64("distance", req.RadiusMeters))

	// Use the repository for business logic
	hotels, err := svc.repo.GetPOIsByLocationAndDistanceWithCategory(ctx, float64(req.Location.Latitude), float64(req.Location.Longitude), float64(req.RadiusMeters), "hotel")
	if err != nil {
		svc.logger.Error("Failed to discover hotels", zap.Error(err))
		span.RecordError(err)
		return nil, status.Errorf(codes.Internal, "failed to discover hotels: %v", err)
	}

	// Convert to protobuf format
	pbHotels := make([]*pb.HotelDetailedInfo, len(hotels))
	for i, hotel := range hotels {
		pbHotels[i] = convertPOIToHotelDetailedInfoProto(hotel)
	}

	svc.logger.Info("Successfully discovered hotels",
		zap.String("user_id", userID),
		zap.Int("count", len(hotels)))

	span.SetAttributes(attribute.Int("results.count", len(hotels)))

	return &pb.DiscoverHotelsResponse{
		Hotels: pbHotels,
	}, nil
}

func (svc *Service) DiscoverAttractions(ctx context.Context, req *pb.DiscoverAttractionsRequest) (*pb.DiscoverAttractionsResponse, error) {
	// Check user authentication
	userID, err := domain.CheckUserAuth(ctx)
	if err != nil {
		return nil, err
	}

	ctx, span := svc.tracer.Start(ctx, "POIService.DiscoverAttractions", trace.WithAttributes(
		attribute.String("poi.user_id", userID),
		attribute.Float64("poi.latitude", float64(req.Location.Latitude)),
		attribute.Float64("poi.longitude", float64(req.Location.Longitude)),
		attribute.Float64("poi.distance", float64(req.RadiusMeters)),
	))
	defer span.End()

	//userUUID, err := uuid.Parse(userID)
	//if err != nil {
	//	return nil, status.Errorf(codes.InvalidArgument, "invalid user ID: %v", err)
	//}

	if err := ValidateLocationAndRadius(req.Location.Latitude, req.Location.Longitude, req.RadiusMeters); err != nil {
		return nil, err
	}

	svc.logger.Info("Discovering attractions",
		zap.String("user_id", userID),
		zap.Float64("latitude", req.Location.Latitude),
		zap.Float64("longitude", req.Location.Longitude),
		zap.Float64("distance", req.RadiusMeters))

	// Use the repository for business logic
	attractions, err := svc.repo.GetPOIsByLocationAndDistanceWithCategory(ctx, float64(req.Location.Latitude), float64(req.Location.Longitude), float64(req.RadiusMeters), "attraction")
	if err != nil {
		svc.logger.Error("Failed to discover attractions", zap.Error(err))
		span.RecordError(err)
		return nil, status.Errorf(codes.Internal, "failed to discover attractions: %v", err)
	}

	// Convert to protobuf format
	pbAttractions := make([]*pb.POIDetailedInfo, len(attractions))
	for i, attraction := range attractions {
		pbAttractions[i] = convertPOIDetailedInfoToProto(attraction)
	}

	svc.logger.Info("Successfully discovered attractions",
		zap.String("user_id", userID),
		zap.Int("count", len(attractions)))

	span.SetAttributes(attribute.Int("results.count", len(attractions)))

	return &pb.DiscoverAttractionsResponse{
		Attractions: pbAttractions,
	}, nil
}

func (svc *Service) AddToFavorites(ctx context.Context, req *pb.AddToFavoritesRequest) (*pb.AddToFavoritesResponse, error) {
	// Check user authentication
	userID, err := domain.CheckUserAuth(ctx)
	if err != nil {
		return nil, err
	}

	ctx, span := svc.tracer.Start(ctx, "POIService.AddToFavorites", trace.WithAttributes(
		attribute.String("poi.user_id", userID),
		attribute.String("poi.poi_id", req.PoiId),
		attribute.Bool("poi.is_llm_generated", req.IsLlmPoi),
	))
	defer span.End()

	userUUID, err := uuid.Parse(userID)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid user ID: %v", err)
	}

	poiUUID, err := uuid.Parse(req.PoiId)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid POI ID: %v", err)
	}

	svc.logger.Info("Adding POI to favorites",
		zap.String("user_id", userID),
		zap.String("poi_id", req.PoiId),
		zap.Bool("is_llm_generated", req.IsLlmPoi))

	// Use the API service for business logic
	favoriteID, err := svc.repo.AddPoiToFavourites(ctx, userUUID, poiUUID)
	if err != nil {
		svc.logger.Error("Failed to add POI to favorites", zap.Error(err))
		span.RecordError(err)
		return nil, status.Errorf(codes.Internal, "failed to add POI to favorites: %v", err)
	}

	svc.logger.Info("Successfully added POI to favorites",
		zap.String("user_id", userID),
		zap.String("poi_id", req.PoiId),
		zap.String("favorite_id", favoriteID.String()))

	span.SetAttributes(attribute.String("favorite.id", favoriteID.String()))

	return &pb.AddToFavoritesResponse{
		Success:    true,
		Message:    "POI added to favorites successfully",
		FavoriteId: favoriteID.String(),
	}, nil
}

func (svc *Service) RemoveFromFavorites(ctx context.Context, req *pb.RemoveFromFavoritesRequest) (*pb.RemoveFromFavoritesResponse, error) {
	// Check user authentication
	userID, err := domain.CheckUserAuth(ctx)
	if err != nil {
		return nil, err
	}

	ctx, span := svc.tracer.Start(ctx, "POIService.RemoveFromFavorites", trace.WithAttributes(
		attribute.String("poi.user_id", userID),
		attribute.String("poi.poi_id", req.PoiId),
		attribute.Bool("poi.is_llm_generated", req.IsLlmPoi),
	))
	defer span.End()

	userUUID, err := uuid.Parse(userID)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid user ID: %v", err)
	}

	poiUUID, err := uuid.Parse(req.PoiId)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid POI ID: %v", err)
	}

	svc.logger.Info("Removing POI from favorites",
		zap.String("user_id", userID),
		zap.String("poi_id", req.PoiId),
		zap.Bool("is_llm_generated", req.IsLlmPoi))

	// Use the repository for business logic
	err = svc.repo.RemovePoiFromFavourites(ctx, userUUID, poiUUID)
	if err != nil {
		svc.logger.Error("Failed to remove POI from favorites", zap.Error(err))
		span.RecordError(err)
		return nil, status.Errorf(codes.Internal, "failed to remove POI from favorites: %v", err)
	}

	svc.logger.Info("Successfully removed POI from favorites",
		zap.String("user_id", userID),
		zap.String("poi_id", req.PoiId))

	return &pb.RemoveFromFavoritesResponse{
		Success: true,
		Message: "POI removed from favorites successfully",
	}, nil
}

func (svc *Service) GetFavorites(ctx context.Context, req *pb.GetFavoritesRequest) (*pb.GetFavoritesResponse, error) {
	// Check user authentication
	userID, err := domain.CheckUserAuth(ctx)
	if err != nil {
		return nil, err
	}

	ctx, span := svc.tracer.Start(ctx, "POIService.GetFavorites", trace.WithAttributes(
		attribute.String("poi.user_id", userID),
		attribute.Int("poi.limit", int(req.Limit)),
		attribute.Int("poi.offset", int(req.Offset)),
	))
	defer span.End()

	userUUID, err := uuid.Parse(userID)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid user ID: %v", err)
	}

	// Set defaults
	limit := int(req.Limit)
	if limit <= 0 {
		limit = 10
	}
	if limit > 100 {
		limit = 100
	}

	offset := int(req.Offset)
	if offset < 0 {
		offset = 0
	}

	svc.logger.Info("Getting user favorites",
		zap.String("user_id", userID),
		zap.Int("limit", limit),
		zap.Int("offset", offset))

	// Use the repository for business logic
	favoritePOIs, total, err := svc.repo.GetFavouritePOIsByUserIDPaginated(ctx, userUUID, limit, offset)
	if err != nil {
		svc.logger.Error("Failed to get favorite POIs", zap.Error(err))
		span.RecordError(err)
		return nil, status.Errorf(codes.Internal, "failed to get favorite POIs: %v", err)
	}

	// Convert to protobuf format
	pbPOIs := make([]*pb.POIDetailedInfo, len(favoritePOIs))
	for i, poi := range favoritePOIs {
		pbPOIs[i] = convertPOIDetailedInfoToProto(poi)
	}

	svc.logger.Info("Successfully retrieved favorite POIs",
		zap.String("user_id", userID),
		zap.Int("count", len(favoritePOIs)),
		zap.Int("total", total))

	span.SetAttributes(
		attribute.Int("results.count", len(favoritePOIs)),
		attribute.Int("results.total", total),
	)

	return &pb.GetFavoritesResponse{
		Favorites:  pbPOIs,
		TotalCount: int32(total),
		Limit:      int32(limit),
		Offset:     int32(offset),
	}, nil
}

func (svc *Service) GetItineraries(ctx context.Context, req *pb.GetItinerariesRequest) (*pb.GetItinerariesResponse, error) {
	// Check user authentication
	userID, err := domain.CheckUserAuth(ctx)
	if err != nil {
		return nil, err
	}

	ctx, span := svc.tracer.Start(ctx, "POIService.GetItineraries", trace.WithAttributes(
		attribute.String("poi.user_id", userID),
		attribute.Int("poi.page", int(req.Page)),
		attribute.Int("poi.page_size", int(req.PageSize)),
	))
	defer span.End()

	userUUID, err := uuid.Parse(userID)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid user ID: %v", err)
	}

	page := int(req.Page)
	if page <= 0 {
		page = 1
	}
	pageSize := int(req.PageSize)
	if pageSize <= 0 {
		pageSize = 10
	}
	if pageSize > 100 {
		pageSize = 100
	}

	svc.logger.Info("Getting user itineraries",
		zap.String("user_id", userID),
		zap.Int("page", page),
		zap.Int("page_size", pageSize))

	// Use the repository for business logic
	itineraries, totalRecords, err := svc.repo.GetItineraries(ctx, userUUID, page, pageSize)
	if err != nil {
		svc.logger.Error("Failed to get itineraries", zap.Error(err))
		span.RecordError(err)
		return nil, status.Errorf(codes.Internal, "failed to get itineraries: %v", err)
	}

	// Convert to protobuf format
	pbItineraries := make([]*pb.UserItinerary, len(itineraries))
	for i, itinerary := range itineraries {
		pbItineraryResponse := convertUserSavedItineraryToProto(itinerary)
		pbItineraries[i] = pbItineraryResponse.Itinerary
	}

	svc.logger.Info("Successfully retrieved itineraries",
		zap.String("user_id", userID))

	return &pb.GetItinerariesResponse{
		Itineraries: pbItineraries,
		TotalCount:  int32(totalRecords),
		Page:        int32(page),
		PageSize:    int32(pageSize),
	}, nil
}

func (svc *Service) GetItinerary(ctx context.Context, req *pb.GetItineraryRequest) (*pb.GetItineraryResponse, error) {
	// Check user authentication
	userID, err := domain.CheckUserAuth(ctx)
	if err != nil {
		return nil, err
	}

	ctx, span := svc.tracer.Start(ctx, "POIService.GetItinerary", trace.WithAttributes(
		attribute.String("poi.user_id", userID),
		attribute.String("poi.itinerary_id", req.ItineraryId),
	))
	defer span.End()

	userUUID, err := uuid.Parse(userID)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid user ID: %v", err)
	}

	itineraryUUID, err := uuid.Parse(req.ItineraryId)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid itinerary ID: %v", err)
	}

	svc.logger.Info("Getting itinerary",
		zap.String("user_id", userID),
		zap.String("itinerary_id", req.ItineraryId))

	// Use the repository for business logic
	itinerary, err := svc.repo.GetItinerary(ctx, userUUID, itineraryUUID)
	if err != nil {
		svc.logger.Error("Failed to get itinerary", zap.Error(err))
		span.RecordError(err)
		return nil, status.Errorf(codes.Internal, "failed to get itinerary: %v", err)
	}

	if itinerary == nil {
		return nil, status.Errorf(codes.NotFound, "itinerary not found")
	}

	// Convert to protobuf format
	pbItinerary := convertUserSavedItineraryToProto(*itinerary)

	svc.logger.Info("Successfully retrieved itinerary",
		zap.String("user_id", userID),
		zap.String("itinerary_id", req.ItineraryId))

	return &pb.GetItineraryResponse{
		Itinerary: pbItinerary.Itinerary,
	}, nil
}

func (svc *Service) UpdateItinerary(ctx context.Context, req *pb.UpdateItineraryRequest) (*pb.UpdateItineraryResponse, error) {
	// Check user authentication
	userID, err := domain.CheckUserAuth(ctx)
	if err != nil {
		return nil, err
	}

	ctx, span := svc.tracer.Start(ctx, "POIService.UpdateItinerary", trace.WithAttributes(
		attribute.String("poi.user_id", userID),
		attribute.String("poi.itinerary_id", req.ItineraryId),
	))
	defer span.End()

	userUUID, err := uuid.Parse(userID)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid user ID: %v", err)
	}

	itineraryUUID, err := uuid.Parse(req.ItineraryId)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid itinerary ID: %v", err)
	}

	svc.logger.Info("Updating itinerary",
		zap.String("user_id", userID),
		zap.String("itinerary_id", req.ItineraryId))

	// Build update request from protobuf
	updates := UpdateItineraryRequest{}
	if req.Title != "" {
		updates.Title = &req.Title
	}
	if req.Description != "" {
		updates.Description = &req.Description
	}
	//if req.Tags != nil && len(req.Tags.Tags) > 0 {
	//	updates.Tags = &req.Tags.Tags
	//}
	//if req.EstimatedDurationDays != nil {
	//	updates.EstimatedDurationDays = (*int)(req.EstimatedDurationDays)
	//}
	//if req.EstimatedCostLevel != nil {
	//	updates.EstimatedCostLevel = req.EstimatedCostLevel
	//}
	//if req.IsPublic != nil {
	//	updates.IsPublic = req.IsPublic
	//}
	if req.MarkdownContent != "" {
		updates.MarkdownContent = &req.MarkdownContent
	}

	// Use the repository for business logic
	updatedItinerary, err := svc.repo.UpdateItinerary(ctx, userUUID, itineraryUUID, updates)
	if err != nil {
		svc.logger.Error("Failed to update itinerary", zap.Error(err))
		span.RecordError(err)
		return nil, status.Errorf(codes.Internal, "failed to update itinerary: %v", err)
	}

	// Convert to protobuf format
	pbItinerary := convertUserSavedItineraryToProto(*updatedItinerary)

	svc.logger.Info("Successfully updated itinerary",
		zap.String("user_id", userID),
		zap.String("itinerary_id", req.ItineraryId))

	return &pb.UpdateItineraryResponse{
		Itinerary: pbItinerary.Itinerary,
	}, nil
}

func (svc *Service) GenerateEmbeddings(ctx context.Context, req *pb.GenerateEmbeddingsRequest) (*pb.GenerateEmbeddingsResponse, error) {
	ctx, span := svc.tracer.Start(ctx, "POIService.GenerateEmbeddings", trace.WithAttributes(
		attribute.Int("poi.batch_size", int(req.BatchSize)),
	))
	defer span.End()

	batchSize := int(req.BatchSize)
	if batchSize <= 0 {
		batchSize = 10
	}
	if batchSize > 100 {
		batchSize = 100
	}

	svc.logger.Info("Generating embeddings for POIs",
		zap.Int("batch_size", batchSize))

	// Use the repository for business logic
	err := svc.GenerateEmbeddingsForAllPOIs(ctx, batchSize)
	if err != nil {
		svc.logger.Error("Failed to generate embeddings", zap.Error(err))
		span.RecordError(err)
		return nil, status.Errorf(codes.Internal, "failed to generate embeddings: %v", err)
	}

	svc.logger.Info("Successfully generated embeddings for POIs")

	return &pb.GenerateEmbeddingsResponse{
		Success: true,
		Message: "Embeddings generated successfully",
	}, nil
}

// Conversion functions
func convertPOIDetailedInfoToProto(poi POIDetailedInfo) *pb.POIDetailedInfo {
	pbPOI := &pb.POIDetailedInfo{
		Id:          poi.ID.String(),
		Name:        poi.Name,
		Description: poi.Description,
		Category:    poi.Category,
		Latitude:    float64(poi.Latitude),
		Longitude:   float64(poi.Longitude),
		Address:     poi.Address,
		CityName:    poi.City,
		Phone:       poi.PhoneNumber,
		Website:     poi.Website,
		Rating:      float64(poi.Rating),
		PriceRange:  poi.PriceLevel,
		Tags:        convertStringSliceToTagsProto(poi.Tags),
		//Source:      poi.Source,
		CreatedAt: timestamppb.New(poi.CreatedAt),
	}

	// Handle ReviewCount safely
	if poi.ReviewCount != nil {
		pbPOI.ReviewCount = int32(*poi.ReviewCount)
	}

	// Handle IsVerified safely
	if poi.IsVerified != nil {
		pbPOI.IsVerified = *poi.IsVerified
	}

	// Handle PhotoUrls safely
	if poi.PhotoUrls != nil {
		photoUrls := make([]string, len(poi.PhotoUrls))
		for i, url := range poi.PhotoUrls {
			if url != nil {
				photoUrls[i] = *url
			}
		}
		pbPOI.Photos = photoUrls
	}

	// Handle OpeningHours ([]string type)
	if poi.OpeningHours != nil && len(poi.OpeningHours) > 0 {
		pbPOI.OpeningHours = poi.OpeningHours
	}

	return pbPOI
}

func convertRestaurantDetailedInfoToProto(restaurant RestaurantDetailedInfo) *pb.RestaurantDetailedInfo {
	poi := &pb.POIDetailedInfo{
		Id:          restaurant.ID.String(),
		Name:        restaurant.Name,
		Description: restaurant.Description,
		Category:    restaurant.Category,
		Latitude:    float64(restaurant.Latitude),
		Longitude:   float64(restaurant.Longitude),
		CityName:    restaurant.City,
		Rating:      float64(restaurant.Rating),
		Tags:        convertStringSliceToTagsProto(restaurant.Tags),
		IsVerified:  restaurant.IsVerified != nil && *restaurant.IsVerified,
	}

	// Handle optional pointer fields
	if restaurant.Address != nil {
		poi.Address = *restaurant.Address
	}
	if restaurant.Website != nil {
		poi.Website = *restaurant.Website
	}
	if restaurant.PhoneNumber != nil {
		poi.Phone = *restaurant.PhoneNumber
	}
	if restaurant.PriceLevel != nil {
		poi.PriceRange = *restaurant.PriceLevel
	}
	if restaurant.ReviewCount != nil {
		if reviewCount, err := strconv.Atoi(*restaurant.ReviewCount); err == nil {
			poi.ReviewCount = int32(reviewCount)
		}
	}
	if restaurant.OpeningHours != nil && len(restaurant.OpeningHours) > 0 {
		poi.OpeningHours = restaurant.OpeningHours
	}
	if restaurant.PhotoUrls != nil {
		poi.Photos = make([]string, len(restaurant.PhotoUrls))
		for i, url := range restaurant.PhotoUrls {
			if url != nil {
				poi.Photos[i] = *url
			}
		}
	}

	result := &pb.RestaurantDetailedInfo{
		Poi: poi,
	}

	// Handle restaurant-specific optional fields
	if restaurant.CuisineType != nil {
		result.CuisineType = *restaurant.CuisineType
	}

	return result
}

func convertPOIToHotelDetailedInfoProto(poi POIDetailedInfo) *pb.HotelDetailedInfo {
	hotelInfo := &pb.HotelDetailedInfo{
		Poi: convertPOIDetailedInfoToProto(poi),
	}

	// Extract hotel-specific information from POI data
	if poi.StarRating != "" {
		if starRating, err := strconv.Atoi(poi.StarRating); err == nil {
			hotelInfo.StarRating = int32(starRating)
		}
	}

	// Parse amenities from the poi.Amenities string
	if poi.Amenities != "" {
		// Split amenities string by comma and clean up
		amenitiesList := strings.Split(poi.Amenities, ",")
		for i, amenity := range amenitiesList {
			amenitiesList[i] = strings.TrimSpace(amenity)
		}
		hotelInfo.Amenities = amenitiesList
	}

	// Set property type based on category
	hotelInfo.PropertyType = poi.Category

	return hotelInfo
}

func convertUserSavedItineraryToProto(itinerary UserSavedItinerary) *pb.GetItineraryResponse {
	pbItinerary := &pb.UserItinerary{
		Id:              itinerary.ID.String(),
		UserId:          itinerary.UserID.String(),
		Title:           itinerary.Title,
		Tags:            convertStringSliceToTagsProto(itinerary.Tags),
		IsPublic:        itinerary.IsPublic,
		MarkdownContent: itinerary.MarkdownContent,
		CreatedAt:       timestamppb.New(itinerary.CreatedAt),
		UpdatedAt:       timestamppb.New(itinerary.UpdatedAt),
	}

	// Handle nullable Description
	if itinerary.Description.Valid {
		pbItinerary.Description = itinerary.Description.String
	}

	// Handle nullable EstimatedDurationDays
	if itinerary.EstimatedDurationDays.Valid {
		pbItinerary.EstimatedRepeatedDays = strconv.Itoa(int(itinerary.EstimatedDurationDays.Int32))
	}

	// Handle nullable EstimatedCostLevel
	if itinerary.EstimatedCostLevel.Valid {
		pbItinerary.EstimatedCostLevel_11 = itinerary.EstimatedCostLevel.Int32
	}

	return &pb.GetItineraryResponse{
		Itinerary: pbItinerary,
	}
}

func convertStringSliceToTagsProto(tags []string) []*pb.Tags {
	if tags == nil {
		return nil
	}

	pbTags := make([]*pb.Tags, len(tags))
	for i, tag := range tags {
		pbTags[i] = &pb.Tags{
			Name: tag,
		}
	}
	return pbTags
}
