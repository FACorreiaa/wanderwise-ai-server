package recents

import (
	"context"
	"fmt"

	pb "github.com/FACorreiaa/loci-proto/modules/recents/generated"
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
	pb.UnsafeRecentsServiceServer
	logger *zap.Logger
	repo   Repository
	pgpool *pgxpool.Pool
	tracer trace.Tracer
}

func NewService(ctx context.Context, repo Repository, pgpool *pgxpool.Pool, logger *zap.Logger) *Service {
	return &Service{
		logger: logger.With(zap.String("service", "recents")),
		repo:   repo,
		pgpool: pgpool,
		tracer: otel.Tracer("RecentsService"),
	}
}

func (svc *Service) GetRecentInteractions(ctx context.Context, req *pb.GetRecentInteractionsRequest) (*pb.GetRecentInteractionsResponse, error) {
	// Check user authentication
	userID, err := domain.CheckUserAuth(ctx)
	if err != nil {
		return nil, err
	}

	// Extract search query from filter
	searchQuery := ""
	if req.Filter != nil {
		searchQuery = req.Filter.SearchQuery
	}

	ctx, span := svc.tracer.Start(ctx, "RecentsService.GetRecentInteractions", trace.WithAttributes(
		attribute.String("recents.user_id", userID),
		attribute.Int("limit", int(req.Limit)),
		attribute.Int("offset", int(req.Offset)),
		attribute.Bool("group_by_city", req.GroupByCity),
		attribute.String("search_query", searchQuery),
	))
	defer span.End()

	userUUID, err := uuid.Parse(userID)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid user ID: %v", err)
	}

	// Convert offset to page number
	limit := int(req.Limit)
	if limit < 1 {
		limit = 10
	}
	if limit > 100 {
		limit = 100
	}

	offset := int(req.Offset)
	if offset < 0 {
		offset = 0
	}

	page := (offset / limit) + 1

	// Create filter options from protobuf filter
	filterOptions := &RecentInteractionsFilter{
		SortBy:    "last_activity",
		SortOrder: "desc",
		Search:    searchQuery,
	}

	svc.logger.Info("Getting user recent interactions",
		zap.String("user_id", userID),
		zap.Int("page", page),
		zap.Int("limit", limit),
		zap.Bool("group_by_city", req.GroupByCity),
		zap.String("search_query", searchQuery))

	// Get recent interactions from repository
	response, err := svc.repo.GetUserRecentInteractions(ctx, userUUID, page, limit, filterOptions)
	if err != nil {
		svc.logger.Error("Failed to get recent interactions", zap.Error(err))
		span.RecordError(err)
		return nil, status.Errorf(codes.Internal, "failed to get recent interactions: %v", err)
	}

	// Build response based on GroupByCity flag
	if req.GroupByCity {
		// Return city summaries when grouping by city
		pbCitySummaries := make([]*pb.CityInteractionSummary, len(response.Cities))
		for i, city := range response.Cities {
			pbCitySummaries[i] = convertCityInteractionsToCitySummary(city)
		}

		svc.logger.Info("Successfully retrieved recent interactions grouped by city",
			zap.String("user_id", userID),
			zap.Int("cities_count", len(response.Cities)))

		span.SetAttributes(attribute.Int("results.cities", len(response.Cities)))

		return &pb.GetRecentInteractionsResponse{
			CitySummaries: pbCitySummaries,
			TotalCount:    int32(response.Total),
		}, nil
	} else {
		// Return flat list of interactions when not grouping
		var allInteractions []*pb.RecentInteraction
		for _, city := range response.Cities {
			for _, interaction := range city.Interactions {
				allInteractions = append(allInteractions, convertInteractionToProto(interaction))
			}
		}

		svc.logger.Info("Successfully retrieved recent interactions as flat list",
			zap.String("user_id", userID),
			zap.Int("interactions_count", len(allInteractions)))

		span.SetAttributes(attribute.Int("results.interactions", len(allInteractions)))

		return &pb.GetRecentInteractionsResponse{
			Interactions: allInteractions,
			TotalCount:   int32(response.Total),
		}, nil
	}
}

func (svc *Service) GetCityInteractions(ctx context.Context, req *pb.GetCityInteractionsRequest) (*pb.GetCityInteractionsResponse, error) {
	// Check user authentication
	userID, err := domain.CheckUserAuth(ctx)
	if err != nil {
		return nil, err
	}

	ctx, span := svc.tracer.Start(ctx, "RecentsService.GetCityInteractions", trace.WithAttributes(
		attribute.String("recents.user_id", userID),
		attribute.String("city_name", req.CityName),
		attribute.Bool("include_details", req.IncludeDetails),
	))
	defer span.End()

	userUUID, err := uuid.Parse(userID)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid user ID: %v", err)
	}

	if req.CityName == "" {
		return nil, status.Errorf(codes.InvalidArgument, "city name is required")
	}

	svc.logger.Info("Getting city interactions for user",
		zap.String("user_id", userID),
		zap.String("city_name", req.CityName),
		zap.Bool("include_details", req.IncludeDetails))

	// Get city details using the repository pattern from API service
	defaultFilter := &RecentInteractionsFilter{
		SortBy:    "last_activity",
		SortOrder: "desc",
	}

	// Get recent interactions to find the city data
	recentResponse, err := svc.repo.GetUserRecentInteractions(ctx, userUUID, 1, 50, defaultFilter)
	if err != nil {
		svc.logger.Error("Failed to get recent interactions", zap.Error(err))
		span.RecordError(err)
		return nil, status.Errorf(codes.Internal, "failed to get recent interactions: %v", err)
	}

	// Find the city in recent interactions
	var cityInteractions *CityInteractions
	for _, city := range recentResponse.Cities {
		if city.CityName == req.CityName {
			cityInteractions = &city
			break
		}
	}

	if cityInteractions == nil {
		return nil, status.Errorf(codes.NotFound, "no interactions found for city: %s", req.CityName)
	}

	// Get additional data for city insights
	pois, _ := svc.repo.GetCityPOIsByInteraction(ctx, userUUID, req.CityName)
	favorites, _ := svc.repo.GetCityFavorites(ctx, userUUID, req.CityName)
	itineraries, _ := svc.repo.GetCityItinerariesByInteraction(ctx, userUUID, req.CityName)

	// Build the city interactions summary (protobuf format)
	var firstInteraction, lastInteraction *timestamppb.Timestamp
	if len(cityInteractions.Interactions) > 0 {
		// Interactions are sorted by created_at DESC, so first is latest, last is earliest
		lastInteraction = timestamppb.New(cityInteractions.Interactions[0].CreatedAt)
		firstInteraction = timestamppb.New(cityInteractions.Interactions[len(cityInteractions.Interactions)-1].CreatedAt)
	}

	// Build top categories from POIs
	topCategories := extractTopCategories(pois)

	pbCityInteractions := &pb.CityInteractions{
		CityId:             "", // We don't have city ID in our current structure
		CityName:           cityInteractions.CityName,
		Country:            "", // We don't have country in our current structure
		CountryCode:        "", // We don't have country code in our current structure
		TotalInteractions:  int32(len(cityInteractions.Interactions)),
		Searches:           int32(len(cityInteractions.Interactions)), // Approximation
		Favorites:          int32(len(favorites)),
		ItinerariesCreated: int32(len(itineraries)),
		PoisViewed:         int32(len(pois)),
		FirstInteraction:   firstInteraction,
		LastInteraction:    lastInteraction,
		TopCategories:      topCategories,
		// Preferences could be added later based on user behavior
	}

	// Build response
	response := &pb.GetCityInteractionsResponse{
		CityInteractions: pbCityInteractions,
	}

	// If details are requested, include the actual interactions
	if req.IncludeDetails {
		detailedInteractions := make([]*pb.RecentInteraction, len(cityInteractions.Interactions))
		for i, interaction := range cityInteractions.Interactions {
			detailedInteractions[i] = convertInteractionToProto(interaction)
		}
		response.DetailedInteractions = detailedInteractions
	}

	svc.logger.Info("Successfully retrieved city interactions",
		zap.String("user_id", userID),
		zap.String("city_name", req.CityName),
		zap.Int("interaction_count", len(cityInteractions.Interactions)),
		zap.Int("poi_count", len(pois)),
		zap.Int("favorite_count", len(favorites)),
		zap.Int("itinerary_count", len(itineraries)))

	span.SetAttributes(
		attribute.Int("results.interactions", len(cityInteractions.Interactions)),
		attribute.Int("results.pois", len(pois)),
		attribute.Int("results.favorites", len(favorites)),
		attribute.Int("results.itineraries", len(itineraries)),
	)

	return response, nil
}

func (svc *Service) RecordInteraction(ctx context.Context, req *pb.RecordInteractionRequest) (*pb.RecordInteractionResponse, error) {
	return nil, nil
}

func (svc *Service) GetInteractionHistory(ctx context.Context, req *pb.GetInteractionHistoryRequest) (*pb.GetInteractionHistoryResponse, error) {
	return nil, nil
}

func (svc *Service) GetFrequentPlaces(ctx context.Context, req *pb.GetFrequentPlacesRequest) (*pb.GetFrequentPlacesResponse, error) {
	return nil, nil
}

// Conversion functions

func convertCityInteractionsToCitySummary(city CityInteractions) *pb.CityInteractionSummary {
	pbInteractions := make([]*pb.RecentInteraction, len(city.Interactions))
	for i, interaction := range city.Interactions {
		pbInteractions[i] = convertInteractionToProto(interaction)
	}

	return &pb.CityInteractionSummary{
		CityId:             "", // We don't have city ID in our current structure
		CityName:           city.CityName,
		Country:            "", // We don't have country in our current structure
		InteractionCount:   int32(len(city.Interactions)),
		LatestInteraction:  timestamppb.New(city.LastActivity),
		RecentInteractions: pbInteractions,
	}
}

// convertCityInteractionsToProto is deprecated - use convertCityInteractionsToCitySummary instead
// Kept for compatibility but should not be used with new protobuf schema

func convertInteractionToProto(interaction RecentInteraction) *pb.RecentInteraction {
	var cityID string
	if interaction.CityID != nil {
		cityID = interaction.CityID.String()
	}

	// Create metadata from our internal fields
	metadata := make(map[string]string)
	if interaction.ModelUsed != "" {
		metadata["model_used"] = interaction.ModelUsed
	}
	if interaction.LatencyMs > 0 {
		metadata["latency_ms"] = fmt.Sprintf("%d", interaction.LatencyMs)
	}
	if interaction.ResponseText != "" {
		metadata["response_text"] = interaction.ResponseText
	}

	return &pb.RecentInteraction{
		Id:              interaction.ID.String(),
		UserId:          interaction.UserID.String(),
		InteractionType: pb.InteractionType_INTERACTION_TYPE_SEARCH, // Default to search
		EntityType:      "search",                                   // Our interactions are primarily searches
		EntityName:      interaction.CityName,                       // Use city name as entity name
		Description:     interaction.Prompt,                         // Use prompt as description
		CityId:          cityID,
		CityName:        interaction.CityName,
		Country:         "", // We don't have country in our current structure
		Metadata:        metadata,
		CreatedAt:       timestamppb.New(interaction.CreatedAt),
	}
}

// convertPOIsToProto is deprecated - POIDetails type doesn't exist in current protobuf schema
// Individual POI data is now included in RecentInteraction messages

// convertHotelsToProto is deprecated - HotelDetails type doesn't exist in current protobuf schema
// Hotel data is now included in RecentInteraction messages

// convertRestaurantsToProto is deprecated - RestaurantDetails type doesn't exist in current protobuf schema
// Restaurant data is now included in RecentInteraction messages

// convertItinerariesToProto is deprecated - SavedItinerary type doesn't exist in current protobuf schema
// Itinerary data is tracked separately in the CityInteractions summary

func convertOpeningHoursToProto(openingHours map[string]string) map[string]string {
	if openingHours == nil {
		return make(map[string]string)
	}
	return openingHours
}

// extractTopCategories extracts the most common categories from POIs
func extractTopCategories(pois []POIDetailedInfo) []string {
	categoryCount := make(map[string]int)
	for _, poi := range pois {
		if poi.Category != "" {
			categoryCount[poi.Category]++
		}
	}

	// Get top 5 categories
	var topCategories []string
	for category := range categoryCount {
		topCategories = append(topCategories, category)
		if len(topCategories) >= 5 {
			break
		}
	}

	return topCategories
}
