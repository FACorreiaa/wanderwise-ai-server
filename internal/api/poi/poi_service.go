package poi

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	"google.golang.org/genai"

	"github.com/google/uuid"
	"github.com/patrickmn/go-cache"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	generativeAI "github.com/FACorreiaa/go-genai-sdk/lib"

	"github.com/FACorreiaa/go-poi-au-suggestions/internal/api/city"
	"github.com/FACorreiaa/go-poi-au-suggestions/internal/types"
)

var _ Service = (*ServiceImpl)(nil)

// Service defines the business logic contract for POI operations.
type Service interface {
	AddPoiToFavourites(ctx context.Context, userID, poiID uuid.UUID, isLLMGenerated bool) (uuid.UUID, error)
	RemovePoiFromFavourites(ctx context.Context, userID, poiID uuid.UUID, isLLMGenerated bool) error
	RemovePoiFromFavouritesByName(ctx context.Context, userID uuid.UUID, poiName string) error
	GetFavouritePOIsByUserID(ctx context.Context, userID uuid.UUID) ([]types.POIDetailedInfo, error)
	GetFavouritePOIsByUserIDPaginated(ctx context.Context, userID uuid.UUID, limit, offset int) ([]types.POIDetailedInfo, int, error)
	GetPOIsByCityID(ctx context.Context, cityID uuid.UUID) ([]types.POIDetailedInfo, error)

	// SearchPOIs Traditional search
	SearchPOIs(ctx context.Context, filter types.POIFilter) ([]types.POIDetailedInfo, error)

	// SearchPOIsSemantic Semantic search methods
	SearchPOIsSemantic(ctx context.Context, query string, limit int) ([]types.POIDetailedInfo, error)
	SearchPOIsSemanticByCity(ctx context.Context, query string, cityID uuid.UUID, limit int) ([]types.POIDetailedInfo, error)
	SearchPOIsHybrid(ctx context.Context, filter types.POIFilter, query string, semanticWeight float64) ([]types.POIDetailedInfo, error)
	GenerateEmbeddingForPOI(ctx context.Context, poiID uuid.UUID) error
	GenerateEmbeddingsForAllPOIs(ctx context.Context, batchSize int) error

	// GetItinerary Itinerary management
	GetItinerary(ctx context.Context, userID, itineraryID uuid.UUID) (*types.UserSavedItinerary, error)
	GetItineraries(ctx context.Context, userID uuid.UUID, page, pageSize int) (*types.PaginatedUserItinerariesResponse, error)
	UpdateItinerary(ctx context.Context, userID, itineraryID uuid.UUID, updates types.UpdateItineraryRequest) (*types.UserSavedItinerary, error)

	// GetGeneralPOIByDistance Discover Service
	GetGeneralPOIByDistance(ctx context.Context, userID uuid.UUID, lat, lon, distance float64) ([]types.POIDetailedInfo, error) //, categoryFilter string

	// GetNearbyRestaurants Domain-specific discover services
	GetNearbyRestaurants(ctx context.Context, userID uuid.UUID, lat, lon, distance float64, cuisineType, priceRange string) ([]types.POIDetailedInfo, error)
	GetNearbyActivities(ctx context.Context, userID uuid.UUID, lat, lon, distance float64, activityType, duration string) ([]types.POIDetailedInfo, error)
	GetNearbyHotels(ctx context.Context, userID uuid.UUID, lat, lon, distance float64, starRating, amenities string) ([]types.POIDetailedInfo, error)
	GetNearbyAttractions(ctx context.Context, userID uuid.UUID, lat, lon, distance float64, attractionType, isOutdoor string) ([]types.POIDetailedInfo, error)

	// FindOrCreateLLMPOI LLM POI management
	FindOrCreateLLMPOI(ctx context.Context, poiData *types.POIDetailedInfo) (uuid.UUID, error)
}

type ServiceImpl struct {
	logger           *slog.Logger
	poiRepository    Repository
	embeddingService *generativeAI.EmbeddingService
	aiClient         *generativeAI.LLMChatClient
	cityRepo         city.Repository
	cache            *cache.Cache
}

func NewServiceImpl(poiRepository Repository,
	embeddingService *generativeAI.EmbeddingService,
	cityRepo city.Repository,
	logger *slog.Logger) *ServiceImpl {
	apiKey := os.Getenv("GEMINI_API_KEY")
	aiClient, err := generativeAI.NewLLMChatClient(context.Background(), apiKey)
	if err != nil {
		logger.Error("Failed to initialize AI client", slog.Any("error", err))
		// For now, set to nil and handle gracefully in methods
		aiClient = nil
	}

	return &ServiceImpl{
		logger:           logger,
		poiRepository:    poiRepository,
		aiClient:         aiClient,
		cityRepo:         cityRepo,
		cache:            cache.New(5*time.Minute, 10*time.Minute),
		embeddingService: embeddingService,
	}
}
func (s *ServiceImpl) AddPoiToFavourites(ctx context.Context, userID, poiID uuid.UUID, isLLMGenerated bool) (uuid.UUID, error) {
	var id uuid.UUID
	if !isLLMGenerated {
		exists, err := s.poiRepository.CheckPoiExists(ctx, poiID)
		if err != nil {
			return uuid.Nil, fmt.Errorf("failed to check if POI exists: %w", err)
		}
		if !exists {
			return uuid.Nil, fmt.Errorf("POI with ID %s does not exist", poiID)
		}

		id, err = s.poiRepository.AddPoiToFavourites(ctx, userID, poiID)
		if err != nil {
			s.logger.Error("failed to add POI to favourites", "error", err)
			return uuid.Nil, err
		}
	} else {
		exists, err := s.poiRepository.CheckLlmPoiExists(ctx, poiID)
		if err != nil {
			return uuid.Nil, fmt.Errorf("failed to check if LLM POI exists: %w", err)
		}
		if !exists {
			return uuid.Nil, fmt.Errorf("LLM POI with ID %s does not exist", poiID)
		}
		// Insert into user_favorite_llm_pois
		id, err = s.poiRepository.AddLLMPoiToFavourite(ctx, userID, poiID)
		if err != nil {
			return uuid.Nil, fmt.Errorf("failed to insert favorite LLM POI: %w", err)
		}
	}

	return id, nil
}
func (s *ServiceImpl) RemovePoiFromFavourites(ctx context.Context, userID, poiID uuid.UUID, isLLMGenerated bool) error {
	if isLLMGenerated {
		err := s.poiRepository.RemoveLLMPoiFromFavourite(ctx, userID, poiID)
		if err != nil {
			s.logger.Error("failed to remove LLM POI from favourites", "error", err)
			return err
		}
	} else {
		err := s.poiRepository.RemovePoiFromFavourites(ctx, userID, poiID)
		if err != nil {
			s.logger.Error("failed to remove POI from favourites", "error", err)
			return err
		}
	}
	return nil
}

// RemovePoiFromFavouritesByName removes a favorite LLM POI by name as a fallback
func (s *ServiceImpl) RemovePoiFromFavouritesByName(ctx context.Context, userID uuid.UUID, poiName string) error {
	err := s.poiRepository.RemoveLLMPoiFromFavouriteByName(ctx, userID, poiName)
	if err != nil {
		s.logger.Error("failed to remove LLM POI from favourites by name", "error", err)
		return err
	}
	return nil
}
func (s *ServiceImpl) GetFavouritePOIsByUserID(ctx context.Context, userID uuid.UUID) ([]types.POIDetailedInfo, error) {
	pois, err := s.poiRepository.GetFavouritePOIsByUserID(ctx, userID)
	if err != nil {
		s.logger.Error("failed to get favourite POIs by user ID", "error", err)
		return nil, err
	}
	return pois, nil
}

func (s *ServiceImpl) GetFavouritePOIsByUserIDPaginated(ctx context.Context, userID uuid.UUID, limit, offset int) ([]types.POIDetailedInfo, int, error) {
	pois, total, err := s.poiRepository.GetFavouritePOIsByUserIDPaginated(ctx, userID, limit, offset)
	if err != nil {
		s.logger.Error("failed to get paginated favourite POIs by user ID", "error", err)
		return nil, 0, err
	}
	return pois, total, nil
}
func (s *ServiceImpl) GetPOIsByCityID(ctx context.Context, cityID uuid.UUID) ([]types.POIDetailedInfo, error) {
	pois, err := s.poiRepository.GetPOIsByCityID(ctx, cityID)
	if err != nil {
		s.logger.Error("failed to get POIs by city ID", "error", err)
		return nil, err
	}
	return pois, nil
}

func (s *ServiceImpl) SearchPOIs(ctx context.Context, filter types.POIFilter) ([]types.POIDetailedInfo, error) {
	pois, err := s.poiRepository.SearchPOIs(ctx, filter)
	if err != nil {
		s.logger.Error("failed to search POIs", "error", err)
		return nil, err
	}
	return pois, nil
}

func (s *ServiceImpl) GetItinerary(ctx context.Context, userID, itineraryID uuid.UUID) (*types.UserSavedItinerary, error) {
	ctx, span := otel.Tracer("LlmInteractionService").Start(ctx, "GetItinerary")
	defer span.End()

	itinerary, err := s.poiRepository.GetItinerary(ctx, userID, itineraryID)
	if err != nil {
		s.logger.ErrorContext(ctx, "Repository failed to get itinerary", slog.Any("error", err))
		span.RecordError(err)
		return nil, fmt.Errorf("failed to get itinerary: %w", err)
	}
	if itinerary == nil {
		return nil, fmt.Errorf("itinerary not found")
	}

	span.SetStatus(codes.Ok, "Itinerary retrieved successfully")
	return itinerary, nil
}

func (s *ServiceImpl) GetItineraries(ctx context.Context, userID uuid.UUID, page, pageSize int) (*types.PaginatedUserItinerariesResponse, error) {
	_, span := otel.Tracer("LlmInteractionService").Start(ctx, "GetItineraries", trace.WithAttributes(
		attribute.String("user.id", userID.String()),
		attribute.Int("page", page),
		attribute.Int("page_size", pageSize),
	))
	defer span.End()

	s.logger.DebugContext(ctx, "Service: Getting itineraries for user", slog.String("userID", userID.String()))

	if page <= 0 {
		page = 1 // Default to page 1
	}
	if pageSize <= 0 {
		pageSize = 10 // Default page size
	}

	itineraries, totalRecords, err := s.poiRepository.GetItineraries(ctx, userID, page, pageSize)
	if err != nil {
		s.logger.ErrorContext(ctx, "Repository failed to get itineraries", slog.Any("error", err))
		span.RecordError(err)
		return nil, fmt.Errorf("failed to retrieve itineraries: %w", err)
	}

	span.SetAttributes(attribute.Int("itineraries.count", len(itineraries)), attribute.Int("total_records", totalRecords))
	span.SetStatus(codes.Ok, "Itineraries retrieved")

	return &types.PaginatedUserItinerariesResponse{
		Itineraries:  itineraries,
		TotalRecords: totalRecords,
		Page:         page,
		PageSize:     pageSize,
	}, nil
}

func (s *ServiceImpl) UpdateItinerary(ctx context.Context, userID, itineraryID uuid.UUID, updates types.UpdateItineraryRequest) (*types.UserSavedItinerary, error) {
	_, span := otel.Tracer("LlmInteractionService").Start(ctx, "UpdateItinerary", trace.WithAttributes(
		attribute.String("user.id", userID.String()),
		attribute.String("itinerary.id", itineraryID.String()),
	))
	defer span.End()

	s.logger.DebugContext(ctx, "Service: Updating itinerary", slog.String("userID", userID.String()), slog.String("itineraryID", itineraryID.String()), slog.Any("updates", updates))

	if updates.Title == nil && updates.Description == nil && updates.Tags == nil &&
		updates.EstimatedDurationDays == nil && updates.EstimatedCostLevel == nil &&
		updates.IsPublic == nil && updates.MarkdownContent == nil {
		span.AddEvent("No update fields provided.")
		s.logger.InfoContext(ctx, "No fields provided for itinerary update, fetching current.", slog.String("itineraryID", itineraryID.String()))
		return s.poiRepository.GetItinerary(ctx, userID, itineraryID) // Assumes GetItinerary checks ownership
	}

	updatedItinerary, err := s.poiRepository.UpdateItinerary(ctx, userID, itineraryID, updates)
	if err != nil {
		s.logger.ErrorContext(ctx, "Repository failed to update itinerary", slog.Any("error", err))
		span.RecordError(err)
		return nil, err // Propagate error (could be not found, or DB error)
	}

	span.SetStatus(codes.Ok, "Itinerary updated")
	return updatedItinerary, nil
}

// SearchPOIsSemantic performs semantic search for POIs using natural language queries
func (s *ServiceImpl) SearchPOIsSemantic(ctx context.Context, query string, limit int) ([]types.POIDetailedInfo, error) {
	ctx, span := otel.Tracer("POIService").Start(ctx, "SearchPOIsSemantic", trace.WithAttributes(
		attribute.String("query", query),
		attribute.Int("limit", limit),
	))
	defer span.End()

	l := s.logger.With(slog.String("method", "SearchPOIsSemantic"))

	if s.embeddingService == nil {
		err := fmt.Errorf("embedding service not available")
		l.ErrorContext(ctx, "Embedding service not initialized", slog.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Embedding service not available")
		return nil, err
	}

	// Generate embedding for the query
	queryEmbedding, err := s.embeddingService.GenerateQueryEmbedding(ctx, query)
	if err != nil {
		l.ErrorContext(ctx, "Failed to generate query embedding",
			slog.Any("error", err),
			slog.String("query", query))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to generate query embedding")
		return nil, fmt.Errorf("failed to generate query embedding: %w", err)
	}

	// Search for similar POIs
	pois, err := s.poiRepository.FindSimilarPOIs(ctx, queryEmbedding, limit)
	if err != nil {
		l.ErrorContext(ctx, "Failed to find similar POIs", slog.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to find similar POIs")
		return nil, fmt.Errorf("failed to find similar POIs: %w", err)
	}

	l.InfoContext(ctx, "Semantic search completed",
		slog.String("query", query),
		slog.Int("results", len(pois)))
	span.SetAttributes(
		attribute.String("query", query),
		attribute.Int("results.count", len(pois)),
	)
	span.SetStatus(codes.Ok, "Semantic search completed")

	return pois, nil
}

// SearchPOIsSemanticByCity performs semantic search for POIs within a specific city
func (s *ServiceImpl) SearchPOIsSemanticByCity(ctx context.Context, query string, cityID uuid.UUID, limit int) ([]types.POIDetailedInfo, error) {
	ctx, span := otel.Tracer("POIService").Start(ctx, "SearchPOIsSemanticByCity", trace.WithAttributes(
		attribute.String("query", query),
		attribute.String("city.id", cityID.String()),
		attribute.Int("limit", limit),
	))
	defer span.End()

	l := s.logger.With(slog.String("method", "SearchPOIsSemanticByCity"))

	if s.embeddingService == nil {
		err := fmt.Errorf("embedding service not available")
		l.ErrorContext(ctx, "Embedding service not initialized", slog.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Embedding service not available")
		return nil, err
	}

	// Generate embedding for the query
	queryEmbedding, err := s.embeddingService.GenerateQueryEmbedding(ctx, query)
	if err != nil {
		l.ErrorContext(ctx, "Failed to generate query embedding",
			slog.Any("error", err),
			slog.String("query", query))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to generate query embedding")
		return nil, fmt.Errorf("failed to generate query embedding: %w", err)
	}

	// Search for similar POIs in the specified city
	pois, err := s.poiRepository.FindSimilarPOIsByCity(ctx, queryEmbedding, cityID, limit)
	if err != nil {
		l.ErrorContext(ctx, "Failed to find similar POIs by city", slog.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to find similar POIs by city")
		return nil, fmt.Errorf("failed to find similar POIs by city: %w", err)
	}

	l.InfoContext(ctx, "Semantic search by city completed",
		slog.String("query", query),
		slog.String("city_id", cityID.String()),
		slog.Int("results", len(pois)))
	span.SetAttributes(
		attribute.String("query", query),
		attribute.String("city.id", cityID.String()),
		attribute.Int("results.count", len(pois)),
	)
	span.SetStatus(codes.Ok, "Semantic search by city completed")

	return pois, nil
}

// SearchPOIsHybrid performs hybrid search combining spatial and semantic similarity
func (s *ServiceImpl) SearchPOIsHybrid(ctx context.Context, filter types.POIFilter, query string, semanticWeight float64) ([]types.POIDetailedInfo, error) {
	ctx, span := otel.Tracer("POIService").Start(ctx, "SearchPOIsHybrid", trace.WithAttributes(
		attribute.String("query", query),
		attribute.Float64("semantic.weight", semanticWeight),
		attribute.Float64("location.latitude", filter.Location.Latitude),
		attribute.Float64("location.longitude", filter.Location.Longitude),
		attribute.Float64("radius", filter.Radius),
	))
	defer span.End()

	l := s.logger.With(slog.String("method", "SearchPOIsHybrid"))

	if s.embeddingService == nil {
		err := fmt.Errorf("embedding service not available")
		l.ErrorContext(ctx, "Embedding service not initialized", slog.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Embedding service not available")
		return nil, err
	}

	// Validate semantic weight
	if semanticWeight < 0 || semanticWeight > 1 {
		err := fmt.Errorf("semantic weight must be between 0 and 1, got: %f", semanticWeight)
		l.ErrorContext(ctx, "Invalid semantic weight", slog.Float64("semantic_weight", semanticWeight))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Invalid semantic weight")
		return nil, err
	}

	// Generate embedding for the query
	queryEmbedding, err := s.embeddingService.GenerateQueryEmbedding(ctx, query)
	if err != nil {
		l.ErrorContext(ctx, "Failed to generate query embedding",
			slog.Any("error", err),
			slog.String("query", query))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to generate query embedding")
		return nil, fmt.Errorf("failed to generate query embedding: %w", err)
	}

	// Perform hybrid search
	pois, err := s.poiRepository.SearchPOIsHybrid(ctx, filter, queryEmbedding, semanticWeight)
	if err != nil {
		l.ErrorContext(ctx, "Failed to perform hybrid search", slog.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to perform hybrid search")
		return nil, fmt.Errorf("failed to perform hybrid search: %w", err)
	}

	l.InfoContext(ctx, "Hybrid search completed",
		slog.String("query", query),
		slog.Float64("semantic_weight", semanticWeight),
		slog.Int("results", len(pois)))
	span.SetAttributes(
		attribute.String("query", query),
		attribute.Float64("semantic.weight", semanticWeight),
		attribute.Int("results.count", len(pois)),
	)
	span.SetStatus(codes.Ok, "Hybrid search completed")

	return pois, nil
}

// GenerateEmbeddingForPOI generates and stores embedding for a specific POI
func (s *ServiceImpl) GenerateEmbeddingForPOI(ctx context.Context, poiID uuid.UUID) error {
	ctx, span := otel.Tracer("POIService").Start(ctx, "GenerateEmbeddingForPOI", trace.WithAttributes(
		attribute.String("poi.id", poiID.String()),
	))
	defer span.End()

	l := s.logger.With(slog.String("method", "GenerateEmbeddingForPOI"))

	if s.embeddingService == nil {
		err := fmt.Errorf("embedding service not available")
		l.ErrorContext(ctx, "Embedding service not initialized", slog.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Embedding service not available")
		return err
	}

	// Get POI details to generate embedding
	pois, err := s.poiRepository.GetPOIsWithoutEmbeddings(ctx, 1)
	if err != nil {
		l.ErrorContext(ctx, "Failed to get POI details", slog.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to get POI details")
		return fmt.Errorf("failed to get POI details: %w", err)
	}

	if len(pois) == 0 {
		l.InfoContext(ctx, "No POI found for embedding generation", slog.String("poi_id", poiID.String()))
		span.SetStatus(codes.Ok, "No POI found")
		return fmt.Errorf("POI not found or already has embedding")
	}

	poi := pois[0]

	// Generate embedding using POI information
	embedding, err := s.embeddingService.GeneratePOIEmbedding(ctx, poi.Name, poi.DescriptionPOI, poi.Category)
	if err != nil {
		l.ErrorContext(ctx, "Failed to generate POI embedding",
			slog.Any("error", err),
			slog.String("poi_id", poiID.String()))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to generate POI embedding")
		return fmt.Errorf("failed to generate POI embedding: %w", err)
	}

	// Update POI with generated embedding
	err = s.poiRepository.UpdatePOIEmbedding(ctx, poiID, embedding)
	if err != nil {
		l.ErrorContext(ctx, "Failed to update POI embedding",
			slog.Any("error", err),
			slog.String("poi_id", poiID.String()))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to update POI embedding")
		return fmt.Errorf("failed to update POI embedding: %w", err)
	}

	l.InfoContext(ctx, "POI embedding generated and stored successfully",
		slog.String("poi_id", poiID.String()),
		slog.String("poi_name", poi.Name))
	span.SetAttributes(
		attribute.String("poi.id", poiID.String()),
		attribute.String("poi.name", poi.Name),
	)
	span.SetStatus(codes.Ok, "POI embedding generated")

	return nil
}

// GenerateEmbeddingsForAllPOIs generates embeddings for all POIs that don't have them
func (s *ServiceImpl) GenerateEmbeddingsForAllPOIs(ctx context.Context, batchSize int) error {
	ctx, span := otel.Tracer("POIService").Start(ctx, "GenerateEmbeddingsForAllPOIs", trace.WithAttributes(
		attribute.Int("batch.size", batchSize),
	))
	defer span.End()

	l := s.logger.With(slog.String("method", "GenerateEmbeddingsForAllPOIs"))

	if s.embeddingService == nil {
		err := fmt.Errorf("embedding service not available")
		l.ErrorContext(ctx, "Embedding service not initialized", slog.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Embedding service not available")
		return err
	}

	if batchSize <= 0 {
		batchSize = 10 // Default batch size
	}

	totalProcessed := 0
	totalErrors := 0

	for {
		// Get batch of POIs without embeddings
		pois, err := s.poiRepository.GetPOIsWithoutEmbeddings(ctx, batchSize)
		if err != nil {
			l.ErrorContext(ctx, "Failed to get POIs without embeddings", slog.Any("error", err))
			span.RecordError(err)
			span.SetStatus(codes.Error, "Failed to get POIs without embeddings")
			return fmt.Errorf("failed to get POIs without embeddings: %w", err)
		}

		if len(pois) == 0 {
			// No more POIs to process
			break
		}

		l.InfoContext(ctx, "Processing batch of POIs", slog.Int("batch_size", len(pois)))

		// Process each POI in the batch
		for _, poi := range pois {
			// Generate embedding
			embedding, err := s.embeddingService.GeneratePOIEmbedding(ctx, poi.Name, poi.DescriptionPOI, poi.Category)
			if err != nil {
				l.ErrorContext(ctx, "Failed to generate embedding for POI",
					slog.Any("error", err),
					slog.String("poi_id", poi.ID.String()),
					slog.String("poi_name", poi.Name))
				totalErrors++
				continue
			}

			// Update POI with embedding
			err = s.poiRepository.UpdatePOIEmbedding(ctx, poi.ID, embedding)
			if err != nil {
				l.ErrorContext(ctx, "Failed to update POI embedding",
					slog.Any("error", err),
					slog.String("poi_id", poi.ID.String()),
					slog.String("poi_name", poi.Name))
				totalErrors++
				continue
			}

			totalProcessed++
			l.DebugContext(ctx, "POI embedding generated successfully",
				slog.String("poi_id", poi.ID.String()),
				slog.String("poi_name", poi.Name))
		}

		// Break if we processed fewer POIs than the batch size (end of data)
		if len(pois) < batchSize {
			break
		}
	}

	l.InfoContext(ctx, "Batch embedding generation completed",
		slog.Int("total_processed", totalProcessed),
		slog.Int("total_errors", totalErrors))
	span.SetAttributes(
		attribute.Int("total.processed", totalProcessed),
		attribute.Int("total.errors", totalErrors),
	)

	if totalErrors > 0 {
		span.SetStatus(codes.Error, fmt.Sprintf("Completed with %d errors", totalErrors))
		return fmt.Errorf("embedding generation completed with %d errors out of %d total POIs", totalErrors, totalProcessed+totalErrors)
	}

	span.SetStatus(codes.Ok, "All POI embeddings generated successfully")
	return nil
}

func (s *ServiceImpl) GetGeneralPOIByDistance(ctx context.Context, userID uuid.UUID, lat, lon, distance float64) ([]types.POIDetailedInfo, error) {
	ctx, span := otel.Tracer("LlmInteractionService").Start(ctx, "GetGeneralPOIByDistance")
	defer span.End()

	cacheKey := generateFilteredPOICacheKey(lat, lon, distance, userID)
	span.SetAttributes(attribute.String("cache.key", cacheKey))

	if cached, found := s.cache.Get(cacheKey); found {
		if pois, ok := cached.([]types.POIDetailedInfo); ok {
			s.logger.InfoContext(ctx, "Serving POIs from cache", "key", cacheKey)
			return pois, nil
		}
	}

	s.logger.InfoContext(ctx, "Cache miss. Querying POIs from database.", "lat", lat, "lon", lon, "distance_m", distance)
	poisFromDB, err := s.poiRepository.GetPOIsByLocationAndDistance(ctx, lat, lon, distance)
	if err == nil && len(poisFromDB) > 0 {
		for i := range poisFromDB {
			poisFromDB[i].Source = "points_of_interest"
		}
		s.cache.Set(cacheKey, poisFromDB, cache.DefaultExpiration)
		return poisFromDB, nil
	}

	s.logger.InfoContext(ctx, "No POIs found in database, falling back to LLM generation")
	span.AddEvent("database_miss_fallback_to_llm")

	genAIResponse, err := s.generatePOIsFromLLM(ctx, userID, lat, lon, distance)
	if err != nil {
		span.RecordError(err)
		return nil, err
	}

	enrichedPOIs := s.enrichAndFilterLLMResponse(genAIResponse.GeneralPOI, lat, lon, distance)
	for i := range enrichedPOIs {
		enrichedPOIs[i].Source = "llm_suggested_pois"
	}

	if len(enrichedPOIs) > 0 {
		interaction := &types.LlmInteraction{
			UserID:    userID,
			ModelName: genAIResponse.ModelName,
			Prompt:    genAIResponse.Prompt,
			Response:  genAIResponse.Response,
			Latitude:  &lat,
			Longitude: &lon,
			Distance:  &distance,
		}

		llmInteractionID, err := s.poiRepository.SaveLlmInteraction(ctx, interaction)
		if err != nil {
			s.logger.ErrorContext(ctx, "Failed to save LLM interaction", slog.Any("error", err))
			return nil, err
		}

		// Synchronous save to ensure POIs are available immediately
		if err := s.poiRepository.SaveLlmPoisToDatabase(ctx, userID, enrichedPOIs, genAIResponse, llmInteractionID); err != nil {
			s.logger.WarnContext(ctx, "Failed to save LLM POIs to database", slog.Any("error", err))
		}
	}

	s.cache.Set(cacheKey, enrichedPOIs, cache.DefaultExpiration)
	span.SetStatus(codes.Ok, "POIs generated via LLM and cached")
	return enrichedPOIs, nil
}

func (s *ServiceImpl) generatePOIsFromLLM(ctx context.Context, userID uuid.UUID, lat, lon, distance float64) (*types.GenAIResponse, error) {
	resultCh := make(chan types.GenAIResponse, 1)
	var wg sync.WaitGroup
	wg.Add(1)
	go s.getGeneralPOIByDistance(&wg, ctx, userID, lat, lon, distance, resultCh, &genai.GenerateContentConfig{
		Temperature:     genai.Ptr[float32](0.7),
		MaxOutputTokens: 16384,
	})
	wg.Wait()
	close(resultCh)

	result := <-resultCh
	if result.Err != nil {
		return nil, result.Err
	}
	return &result, nil
}

func (s *ServiceImpl) getGeneralPOIByDistance(wg *sync.WaitGroup,
	ctx context.Context,
	userID uuid.UUID,
	lat, lon, distance float64,
	resultCh chan<- types.GenAIResponse,
	config *genai.GenerateContentConfig) {
	ctx, span := otel.Tracer("LlmInteractionService").Start(ctx, "GenerateGeneralPOIWorker", trace.WithAttributes(
		attribute.Float64("latitude", lat),
		attribute.Float64("longitude", lon),
		attribute.Float64("distance.km", distance),
		attribute.String("user.id", userID.String())))

	defer span.End()
	defer wg.Done()

	prompt := getGeneralPOIByDistance(lat, lon, distance)
	span.SetAttributes(attribute.Int("prompt.length", len(prompt)))

	if s.aiClient == nil {
		err := fmt.Errorf("AI client is not available - check API key configuration")
		span.RecordError(err)
		span.SetStatus(codes.Error, "AI client unavailable")
		resultCh <- types.GenAIResponse{Err: err}
		return
	}

	startTime := time.Now()
	response, err := s.aiClient.GenerateResponse(ctx, prompt, config)
	latencyMs := int(time.Since(startTime).Milliseconds())
	span.SetAttributes(attribute.Int("response.latency_ms", latencyMs))

	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to generate general POIs")
		resultCh <- types.GenAIResponse{Err: fmt.Errorf("failed to generate general POIs: %w", err)}
		return
	}

	var txt string
	for _, candidate := range response.Candidates {
		if candidate.Content != nil && len(candidate.Content.Parts) > 0 {
			txt = candidate.Content.Parts[0].Text
			break
		}
	}
	if txt == "" {
		err := fmt.Errorf("no valid general POI content from AI")
		span.RecordError(err)
		span.SetStatus(codes.Error, "Empty response from AI")
		resultCh <- types.GenAIResponse{Err: err}
		return
	}
	span.SetAttributes(attribute.Int("response.length", len(txt)))

	cleanTxt := cleanJSONResponse(txt)
	var poiData struct {
		PointsOfInterest []types.POIDetailedInfo `json:"points_of_interest"`
	}
	if err := json.Unmarshal([]byte(cleanTxt), &poiData); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to parse general POI JSON")
		resultCh <- types.GenAIResponse{Err: fmt.Errorf("failed to parse general POI JSON: %w", err)}
		return
	}

	fmt.Println(cleanTxt)

	span.SetAttributes(attribute.Int("pois.count", len(poiData.PointsOfInterest)))
	span.SetStatus(codes.Ok, "General POIs generated successfully")
	resultCh <- types.GenAIResponse{
		GeneralPOI: poiData.PointsOfInterest,
		ModelName:  s.aiClient.ModelName,
		Prompt:     prompt,
		Response:   cleanTxt,
	}
}

// FindOrCreateLLMPOI finds an existing LLM POI by name or creates a new one
func (s *ServiceImpl) FindOrCreateLLMPOI(ctx context.Context, poiData *types.POIDetailedInfo) (uuid.UUID, error) {
	ctx, span := otel.Tracer("POIService").Start(ctx, "FindOrCreateLLMPOI")
	defer span.End()

	if poiData == nil {
		return uuid.Nil, fmt.Errorf("POI data cannot be nil")
	}

	// First, try to find existing POI by name and city
	existingID, err := s.poiRepository.FindLLMPOIByNameAndCity(ctx, poiData.Name, poiData.City)
	if err == nil && existingID != uuid.Nil {
		s.logger.InfoContext(ctx, "Found existing LLM POI", "name", poiData.Name, "id", existingID)
		span.SetAttributes(attribute.String("operation", "found_existing"))
		return existingID, nil
	}

	// If not found, create new LLM POI
	newID, err := s.poiRepository.CreateLLMPOI(ctx, poiData)
	if err != nil {
		s.logger.ErrorContext(ctx, "Failed to create LLM POI", "error", err, "name", poiData.Name)
		span.RecordError(err)
		return uuid.Nil, fmt.Errorf("failed to create LLM POI: %w", err)
	}

	s.logger.InfoContext(ctx, "Created new LLM POI", "name", poiData.Name, "id", newID)
	span.SetAttributes(attribute.String("operation", "created_new"))
	return newID, nil
}

// FindLLMPOIByName finds an LLM POI by name, searching across all cities
func (s *ServiceImpl) FindLLMPOIByName(ctx context.Context, poiName string) (uuid.UUID, error) {
	ctx, span := otel.Tracer("POIService").Start(ctx, "FindLLMPOIByName", trace.WithAttributes(
		attribute.String("poi.name", poiName),
	))
	defer span.End()

	// For removal purposes, we need to find the POI by name
	// Since we don't have city context, we'll search by name only
	// This could be enhanced later to include city context if needed
	return s.poiRepository.FindLLMPOIByName(ctx, poiName)
}

// GetNearbyRestaurants get nearby restaurants with optional filters
func (s *ServiceImpl) GetNearbyRestaurants(ctx context.Context, userID uuid.UUID, lat, lon, distance float64, cuisineType, priceRange string) ([]types.POIDetailedInfo, error) {
	ctx, span := otel.Tracer("POIService").Start(ctx, "GetNearbyRestaurants", trace.WithAttributes(
		attribute.Float64("location.lat", lat),
		attribute.Float64("location.lon", lon),
		attribute.Float64("distance", distance),
		attribute.String("cuisine_type", cuisineType),
		attribute.String("price_range", priceRange),
	))
	defer span.End()

	// Build cache key with domain-specific filters
	cacheKey := fmt.Sprintf("restaurants_%f_%f_%f_%s_%s_%s", lat, lon, distance, userID.String(), cuisineType, priceRange)

	if cached, found := s.cache.Get(cacheKey); found {
		if pois, ok := cached.([]types.POIDetailedInfo); ok {
			s.logger.InfoContext(ctx, "Serving restaurants from cache", "key", cacheKey)
			return pois, nil
		}
	}

	s.logger.InfoContext(ctx, "Querying restaurants from database", "lat", lat, "lon", lon, "distance", distance)

	// Get restaurants from database with filters
	restaurants, err := s.poiRepository.GetPOIsByLocationAndDistanceWithCategory(ctx, lat, lon, distance, "restaurant")
	if err == nil && len(restaurants) > 0 {
		// Apply domain-specific filters
		filteredRestaurants := s.filterRestaurants(restaurants, cuisineType, priceRange)

		// Mark as database source
		for i := range filteredRestaurants {
			filteredRestaurants[i].Source = "points_of_interest"
		}

		s.cache.Set(cacheKey, filteredRestaurants, cache.DefaultExpiration)
		return filteredRestaurants, nil
	}

	s.logger.InfoContext(ctx, "No restaurants found in database, falling back to LLM generation")

	// Generate restaurants using LLM with domain-specific prompt
	genAIResponse, err := s.generateRestaurantsFromLLM(ctx, userID, lat, lon, distance, cuisineType, priceRange)
	if err != nil {
		span.RecordError(err)
		return nil, err
	}

	enrichedRestaurants := s.enrichAndFilterLLMResponse(genAIResponse.GeneralPOI, lat, lon, distance)
	for i := range enrichedRestaurants {
		enrichedRestaurants[i].Source = "llm_suggested_pois"
	}

	s.cache.Set(cacheKey, enrichedRestaurants, cache.DefaultExpiration)
	return enrichedRestaurants, nil
}

// GetNearbyActivities get nearby activities with optional filters
func (s *ServiceImpl) GetNearbyActivities(ctx context.Context, userID uuid.UUID, lat, lon, distance float64, activityType, duration string) ([]types.POIDetailedInfo, error) {
	ctx, span := otel.Tracer("POIService").Start(ctx, "GetNearbyActivities", trace.WithAttributes(
		attribute.Float64("location.lat", lat),
		attribute.Float64("location.lon", lon),
		attribute.Float64("distance", distance),
		attribute.String("activity_type", activityType),
		attribute.String("duration", duration),
	))
	defer span.End()

	// Build cache key with domain-specific filters
	cacheKey := fmt.Sprintf("activities_%f_%f_%f_%s_%s_%s", lat, lon, distance, userID.String(), activityType, duration)

	if cached, found := s.cache.Get(cacheKey); found {
		if pois, ok := cached.([]types.POIDetailedInfo); ok {
			s.logger.InfoContext(ctx, "Serving activities from cache", "key", cacheKey)
			return pois, nil
		}
	}

	s.logger.InfoContext(ctx, "Querying activities from database", "lat", lat, "lon", lon, "distance", distance)

	// Get activities from database with filters
	activities, err := s.poiRepository.GetPOIsByLocationAndDistanceWithCategory(ctx, lat, lon, distance, "activity")
	if err == nil && len(activities) > 0 {
		// Apply domain-specific filters
		filteredActivities := s.filterActivities(activities, activityType, duration)

		// Mark as database source
		for i := range filteredActivities {
			filteredActivities[i].Source = "points_of_interest"
		}

		s.cache.Set(cacheKey, filteredActivities, cache.DefaultExpiration)
		return filteredActivities, nil
	}

	s.logger.InfoContext(ctx, "No activities found in database, falling back to LLM generation")

	// Generate activities using LLM with domain-specific prompt
	genAIResponse, err := s.generateActivitiesFromLLM(ctx, userID, lat, lon, distance, activityType, duration)
	if err != nil {
		span.RecordError(err)
		return nil, err
	}

	enrichedActivities := s.enrichAndFilterLLMResponse(genAIResponse.GeneralPOI, lat, lon, distance)
	for i := range enrichedActivities {
		enrichedActivities[i].Source = "llm_suggested_pois"
	}

	s.cache.Set(cacheKey, enrichedActivities, cache.DefaultExpiration)
	return enrichedActivities, nil
}

// GetNearbyHotels get nearby hotels with optional filters
func (s *ServiceImpl) GetNearbyHotels(ctx context.Context, userID uuid.UUID, lat, lon, distance float64, starRating, amenities string) ([]types.POIDetailedInfo, error) {
	ctx, span := otel.Tracer("POIService").Start(ctx, "GetNearbyHotels", trace.WithAttributes(
		attribute.Float64("location.lat", lat),
		attribute.Float64("location.lon", lon),
		attribute.Float64("distance", distance),
		attribute.String("star_rating", starRating),
		attribute.String("amenities", amenities),
	))
	defer span.End()

	// Build cache key with domain-specific filters
	cacheKey := fmt.Sprintf("hotels_%f_%f_%f_%s_%s_%s", lat, lon, distance, userID.String(), starRating, amenities)

	if cached, found := s.cache.Get(cacheKey); found {
		if pois, ok := cached.([]types.POIDetailedInfo); ok {
			s.logger.InfoContext(ctx, "Serving hotels from cache", "key", cacheKey)
			return pois, nil
		}
	}

	s.logger.InfoContext(ctx, "Querying hotels from database", "lat", lat, "lon", lon, "distance", distance)

	// Get hotels from database with filters
	hotels, err := s.poiRepository.GetPOIsByLocationAndDistanceWithCategory(ctx, lat, lon, distance, "hotel")
	if err == nil && len(hotels) > 0 {
		// Apply domain-specific filters
		filteredHotels := s.filterHotels(hotels, starRating, amenities)

		// Mark as database source
		for i := range filteredHotels {
			filteredHotels[i].Source = "points_of_interest"
		}

		s.cache.Set(cacheKey, filteredHotels, cache.DefaultExpiration)
		return filteredHotels, nil
	}

	s.logger.InfoContext(ctx, "No hotels found in database, falling back to LLM generation")

	// Generate hotels using LLM with domain-specific prompt
	genAIResponse, err := s.generateHotelsFromLLM(ctx, userID, lat, lon, distance, starRating, amenities)
	if err != nil {
		span.RecordError(err)
		return nil, err
	}

	enrichedHotels := s.enrichAndFilterLLMResponse(genAIResponse.GeneralPOI, lat, lon, distance)
	for i := range enrichedHotels {
		enrichedHotels[i].Source = "llm_suggested_pois"
	}

	s.cache.Set(cacheKey, enrichedHotels, cache.DefaultExpiration)
	return enrichedHotels, nil
}

// GetNearbyAttractions get nearby attractions with optional filters
func (s *ServiceImpl) GetNearbyAttractions(ctx context.Context, userID uuid.UUID, lat, lon, distance float64, attractionType, isOutdoor string) ([]types.POIDetailedInfo, error) {
	ctx, span := otel.Tracer("POIService").Start(ctx, "GetNearbyAttractions", trace.WithAttributes(
		attribute.Float64("location.lat", lat),
		attribute.Float64("location.lon", lon),
		attribute.Float64("distance", distance),
		attribute.String("attraction_type", attractionType),
		attribute.String("is_outdoor", isOutdoor),
	))
	defer span.End()

	// Build cache key with domain-specific filters
	cacheKey := fmt.Sprintf("attractions_%f_%f_%f_%s_%s_%s", lat, lon, distance, userID.String(), attractionType, isOutdoor)

	if cached, found := s.cache.Get(cacheKey); found {
		if pois, ok := cached.([]types.POIDetailedInfo); ok {
			s.logger.InfoContext(ctx, "Serving attractions from cache", "key", cacheKey)
			return pois, nil
		}
	}

	s.logger.InfoContext(ctx, "Querying attractions from database", "lat", lat, "lon", lon, "distance", distance)

	// Get attractions from database with filters
	attractions, err := s.poiRepository.GetPOIsByLocationAndDistanceWithCategory(ctx, lat, lon, distance, "attraction")
	if err == nil && len(attractions) > 0 {
		// Apply domain-specific filters
		filteredAttractions := s.filterAttractions(attractions, attractionType, isOutdoor)

		// Mark as database source
		for i := range filteredAttractions {
			filteredAttractions[i].Source = "points_of_interest"
		}

		s.cache.Set(cacheKey, filteredAttractions, cache.DefaultExpiration)
		return filteredAttractions, nil
	}

	s.logger.InfoContext(ctx, "No attractions found in database, falling back to LLM generation")

	// Generate attractions using LLM with domain-specific prompt
	genAIResponse, err := s.generateAttractionsFromLLM(ctx, userID, lat, lon, distance, attractionType, isOutdoor)
	if err != nil {
		span.RecordError(err)
		return nil, err
	}

	enrichedAttractions := s.enrichAndFilterLLMResponse(genAIResponse.GeneralPOI, lat, lon, distance)
	for i := range enrichedAttractions {
		enrichedAttractions[i].Source = "llm_suggested_pois"
	}

	s.cache.Set(cacheKey, enrichedAttractions, cache.DefaultExpiration)
	return enrichedAttractions, nil
}

// Helper functions for domain-specific filtering
func (s *ServiceImpl) filterRestaurants(restaurants []types.POIDetailedInfo, cuisineType, priceRange string) []types.POIDetailedInfo {
	if cuisineType == "" && priceRange == "" {
		return restaurants
	}

	filtered := make([]types.POIDetailedInfo, 0)
	for _, restaurant := range restaurants {
		// Filter by cuisine type
		if cuisineType != "" && restaurant.Category != cuisineType {
			continue
		}
		// Filter by price range
		if priceRange != "" && restaurant.PriceLevel != priceRange {
			continue
		}
		filtered = append(filtered, restaurant)
	}
	return filtered
}

func (s *ServiceImpl) filterActivities(activities []types.POIDetailedInfo, activityType, duration string) []types.POIDetailedInfo {
	if activityType == "" && duration == "" {
		return activities
	}

	filtered := make([]types.POIDetailedInfo, 0)
	for _, activity := range activities {
		// Filter by activity type
		if activityType != "" && activity.Category != activityType {
			continue
		}
		// Filter by duration (using description as proxy for duration since TimeToSpend field doesn't exist)
		if duration != "" && !strings.Contains(strings.ToLower(activity.Description), strings.ToLower(duration)) {
			continue
		}
		filtered = append(filtered, activity)
	}
	return filtered
}

func (s *ServiceImpl) filterHotels(hotels []types.POIDetailedInfo, starRating, amenities string) []types.POIDetailedInfo {
	if starRating == "" && amenities == "" {
		return hotels
	}

	filtered := make([]types.POIDetailedInfo, 0)
	for _, hotel := range hotels {
		// Filter by star rating
		if starRating != "" && hotel.PriceLevel != starRating {
			continue
		}
		// Filter by amenities (basic string matching)
		if amenities != "" {
			if !strings.Contains(strings.ToLower(hotel.Amenities), strings.ToLower(amenities)) {
				continue
			}
		}
		filtered = append(filtered, hotel)
	}
	return filtered
}

func (s *ServiceImpl) filterAttractions(attractions []types.POIDetailedInfo, attractionType, isOutdoor string) []types.POIDetailedInfo {
	if attractionType == "" && isOutdoor == "" {
		return attractions
	}

	filtered := make([]types.POIDetailedInfo, 0)
	for _, attraction := range attractions {
		// Filter by attraction type
		if attractionType != "" && attraction.Category != attractionType {
			continue
		}
		// Filter by outdoor/indoor (basic tag matching)
		if isOutdoor != "" {
			hasOutdoorTag := false
			for _, tag := range attraction.Tags {
				if (isOutdoor == "true" && tag == "outdoor") || (isOutdoor == "false" && tag == "indoor") {
					hasOutdoorTag = true
					break
				}
			}
			if !hasOutdoorTag {
				continue
			}
		}
		filtered = append(filtered, attraction)
	}
	return filtered
}

// TODO
// generateRestaurantsFromLLM
func (s *ServiceImpl) generateRestaurantsFromLLM(ctx context.Context, userID uuid.UUID, lat, lon, distance float64, cuisineType, priceRange string) (*types.GenAIResponse, error) {
	resultCh := make(chan types.GenAIResponse, 1)
	var wg sync.WaitGroup
	wg.Add(1)
	go s.getGeneralRestaurantByDistance(&wg, ctx, userID, lat, lon, distance, resultCh, &genai.GenerateContentConfig{
		Temperature:     genai.Ptr[float32](0.7),
		MaxOutputTokens: 16384,
	})
	wg.Wait()
	close(resultCh)

	result := <-resultCh
	if result.Err != nil {
		return nil, result.Err
	}
	return &result, nil
}

func (s *ServiceImpl) getGeneralRestaurantByDistance(wg *sync.WaitGroup,
	ctx context.Context,
	userID uuid.UUID,
	lat, lon, distance float64,
	resultCh chan<- types.GenAIResponse,
	config *genai.GenerateContentConfig) {
	ctx, span := otel.Tracer("LlmInteractionService").Start(ctx, "GenerateGeneralPOIWorker", trace.WithAttributes(
		attribute.Float64("latitude", lat),
		attribute.Float64("longitude", lon),
		attribute.Float64("distance.km", distance),
		attribute.String("user.id", userID.String())))

	defer span.End()
	defer wg.Done()

	userLocation := types.UserLocation{
		UserLat:        lat,
		UserLon:        lon,
		SearchRadiusKm: distance,
	}
	prompt := getRestaurantsNearbyPrompt(userLocation)
	span.SetAttributes(attribute.Int("prompt.length", len(prompt)))

	if s.aiClient == nil {
		err := fmt.Errorf("AI client is not available - check API key configuration")
		span.RecordError(err)
		span.SetStatus(codes.Error, "AI client unavailable")
		resultCh <- types.GenAIResponse{Err: err}
		return
	}

	startTime := time.Now()
	response, err := s.aiClient.GenerateResponse(ctx, prompt, config)
	latencyMs := int(time.Since(startTime).Milliseconds())
	span.SetAttributes(attribute.Int("response.latency_ms", latencyMs))

	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to generate general POIs")
		resultCh <- types.GenAIResponse{Err: fmt.Errorf("failed to generate general POIs: %w", err)}
		return
	}

	var txt string
	for _, candidate := range response.Candidates {
		if candidate.Content != nil && len(candidate.Content.Parts) > 0 {
			txt = candidate.Content.Parts[0].Text
			break
		}
	}
	if txt == "" {
		err := fmt.Errorf("no valid general POI content from AI")
		span.RecordError(err)
		span.SetStatus(codes.Error, "Empty response from AI")
		resultCh <- types.GenAIResponse{Err: err}
		return
	}
	span.SetAttributes(attribute.Int("response.length", len(txt)))

	cleanTxt := cleanJSONResponse(txt)
	var poiData struct {
		PointsOfInterest []types.POIDetailedInfo `json:"points_of_interest"`
	}
	if err := json.Unmarshal([]byte(cleanTxt), &poiData); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to parse general POI JSON")
		resultCh <- types.GenAIResponse{Err: fmt.Errorf("failed to parse general POI JSON: %w", err)}
		return
	}

	fmt.Println(cleanTxt)

	span.SetAttributes(attribute.Int("pois.count", len(poiData.PointsOfInterest)))
	span.SetStatus(codes.Ok, "General POIs generated successfully")
	resultCh <- types.GenAIResponse{
		GeneralPOI: poiData.PointsOfInterest,
		ModelName:  s.aiClient.ModelName,
		Prompt:     prompt,
		Response:   cleanTxt,
	}
}

func (s *ServiceImpl) generateActivitiesFromLLM(ctx context.Context, userID uuid.UUID, lat, lon, distance float64, activityType, duration string) (*types.GenAIResponse, error) {
	resultCh := make(chan types.GenAIResponse, 1)
	var wg sync.WaitGroup
	wg.Add(1)
	go s.getGeneralActivitiesByDistance(&wg, ctx, userID, lat, lon, distance, resultCh, &genai.GenerateContentConfig{
		Temperature:     genai.Ptr[float32](0.7),
		MaxOutputTokens: 16384,
	})
	wg.Wait()
	close(resultCh)

	result := <-resultCh
	if result.Err != nil {
		return nil, result.Err
	}
	return &result, nil
}

func (s *ServiceImpl) getGeneralActivitiesByDistance(wg *sync.WaitGroup,
	ctx context.Context,
	userID uuid.UUID,
	lat, lon, distance float64,
	resultCh chan<- types.GenAIResponse,
	config *genai.GenerateContentConfig) {
	ctx, span := otel.Tracer("LlmInteractionService").Start(ctx, "GenerateGeneralPOIWorker", trace.WithAttributes(
		attribute.Float64("latitude", lat),
		attribute.Float64("longitude", lon),
		attribute.Float64("distance.km", distance),
		attribute.String("user.id", userID.String())))

	defer span.End()
	defer wg.Done()

	userLocation := types.UserLocation{
		UserLat:        lat,
		UserLon:        lon,
		SearchRadiusKm: distance,
	}
	prompt := getActivitiesNearbyPrompt(userLocation)
	span.SetAttributes(attribute.Int("prompt.length", len(prompt)))

	if s.aiClient == nil {
		err := fmt.Errorf("AI client is not available - check API key configuration")
		span.RecordError(err)
		span.SetStatus(codes.Error, "AI client unavailable")
		resultCh <- types.GenAIResponse{Err: err}
		return
	}

	startTime := time.Now()
	response, err := s.aiClient.GenerateResponse(ctx, prompt, config)
	latencyMs := int(time.Since(startTime).Milliseconds())
	span.SetAttributes(attribute.Int("response.latency_ms", latencyMs))

	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to generate general POIs")
		resultCh <- types.GenAIResponse{Err: fmt.Errorf("failed to generate general POIs: %w", err)}
		return
	}

	var txt string
	for _, candidate := range response.Candidates {
		if candidate.Content != nil && len(candidate.Content.Parts) > 0 {
			txt = candidate.Content.Parts[0].Text
			break
		}
	}
	if txt == "" {
		err := fmt.Errorf("no valid general POI content from AI")
		span.RecordError(err)
		span.SetStatus(codes.Error, "Empty response from AI")
		resultCh <- types.GenAIResponse{Err: err}
		return
	}
	span.SetAttributes(attribute.Int("response.length", len(txt)))

	cleanTxt := cleanJSONResponse(txt)
	var poiData struct {
		PointsOfInterest []types.POIDetailedInfo `json:"points_of_interest"`
	}
	if err := json.Unmarshal([]byte(cleanTxt), &poiData); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to parse general POI JSON")
		resultCh <- types.GenAIResponse{Err: fmt.Errorf("failed to parse general POI JSON: %w", err)}
		return
	}

	fmt.Println(cleanTxt)

	span.SetAttributes(attribute.Int("pois.count", len(poiData.PointsOfInterest)))
	span.SetStatus(codes.Ok, "General POIs generated successfully")
	resultCh <- types.GenAIResponse{
		GeneralPOI: poiData.PointsOfInterest,
		ModelName:  s.aiClient.ModelName,
		Prompt:     prompt,
		Response:   cleanTxt,
	}
}

func (s *ServiceImpl) generateHotelsFromLLM(ctx context.Context, userID uuid.UUID, lat, lon, distance float64, starRating, amenities string) (*types.GenAIResponse, error) {
	resultCh := make(chan types.GenAIResponse, 1)
	var wg sync.WaitGroup
	wg.Add(1)
	go s.getGeneralHotelsByDistance(&wg, ctx, userID, lat, lon, distance, resultCh, &genai.GenerateContentConfig{
		Temperature:     genai.Ptr[float32](0.7),
		MaxOutputTokens: 16384,
	})
	wg.Wait()
	close(resultCh)

	result := <-resultCh
	if result.Err != nil {
		return nil, result.Err
	}
	return &result, nil
}

func (s *ServiceImpl) getGeneralHotelsByDistance(wg *sync.WaitGroup,
	ctx context.Context,
	userID uuid.UUID,
	lat, lon, distance float64,
	resultCh chan<- types.GenAIResponse,
	config *genai.GenerateContentConfig) {
	ctx, span := otel.Tracer("LlmInteractionService").Start(ctx, "GenerateGeneralPOIWorker", trace.WithAttributes(
		attribute.Float64("latitude", lat),
		attribute.Float64("longitude", lon),
		attribute.Float64("distance.km", distance),
		attribute.String("user.id", userID.String())))

	defer span.End()
	defer wg.Done()

	userLocation := types.UserLocation{
		UserLat:        lat,
		UserLon:        lon,
		SearchRadiusKm: distance,
	}
	prompt := getHotelsNeabyPrompt(userLocation)
	span.SetAttributes(attribute.Int("prompt.length", len(prompt)))

	if s.aiClient == nil {
		err := fmt.Errorf("AI client is not available - check API key configuration")
		span.RecordError(err)
		span.SetStatus(codes.Error, "AI client unavailable")
		resultCh <- types.GenAIResponse{Err: err}
		return
	}

	startTime := time.Now()
	response, err := s.aiClient.GenerateResponse(ctx, prompt, config)
	latencyMs := int(time.Since(startTime).Milliseconds())
	span.SetAttributes(attribute.Int("response.latency_ms", latencyMs))

	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to generate general POIs")
		resultCh <- types.GenAIResponse{Err: fmt.Errorf("failed to generate general POIs: %w", err)}
		return
	}

	var txt string
	for _, candidate := range response.Candidates {
		if candidate.Content != nil && len(candidate.Content.Parts) > 0 {
			txt = candidate.Content.Parts[0].Text
			break
		}
	}
	if txt == "" {
		err := fmt.Errorf("no valid general POI content from AI")
		span.RecordError(err)
		span.SetStatus(codes.Error, "Empty response from AI")
		resultCh <- types.GenAIResponse{Err: err}
		return
	}
	span.SetAttributes(attribute.Int("response.length", len(txt)))

	cleanTxt := cleanJSONResponse(txt)
	var poiData struct {
		PointsOfInterest []types.POIDetailedInfo `json:"points_of_interest"`
	}
	if err := json.Unmarshal([]byte(cleanTxt), &poiData); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to parse general POI JSON")
		resultCh <- types.GenAIResponse{Err: fmt.Errorf("failed to parse general POI JSON: %w", err)}
		return
	}

	fmt.Println(cleanTxt)

	span.SetAttributes(attribute.Int("pois.count", len(poiData.PointsOfInterest)))
	span.SetStatus(codes.Ok, "General POIs generated successfully")
	resultCh <- types.GenAIResponse{
		GeneralPOI: poiData.PointsOfInterest,
		ModelName:  s.aiClient.ModelName,
		Prompt:     prompt,
		Response:   cleanTxt,
	}
}

func (s *ServiceImpl) generateAttractionsFromLLM(ctx context.Context, userID uuid.UUID, lat, lon, distance float64, attractionType, isOutdoor string) (*types.GenAIResponse, error) {
	resultCh := make(chan types.GenAIResponse, 1)
	var wg sync.WaitGroup
	wg.Add(1)
	go s.getGeneralAttractionsByDistance(&wg, ctx, userID, lat, lon, distance, resultCh, &genai.GenerateContentConfig{
		Temperature:     genai.Ptr[float32](0.7),
		MaxOutputTokens: 16384,
	})
	wg.Wait()
	close(resultCh)

	result := <-resultCh
	if result.Err != nil {
		return nil, result.Err
	}
	return &result, nil
}

func (s *ServiceImpl) getGeneralAttractionsByDistance(wg *sync.WaitGroup,
	ctx context.Context,
	userID uuid.UUID,
	lat, lon, distance float64,
	resultCh chan<- types.GenAIResponse,
	config *genai.GenerateContentConfig) {
	ctx, span := otel.Tracer("LlmInteractionService").Start(ctx, "GenerateGeneralPOIWorker", trace.WithAttributes(
		attribute.Float64("latitude", lat),
		attribute.Float64("longitude", lon),
		attribute.Float64("distance.km", distance),
		attribute.String("user.id", userID.String())))

	defer span.End()
	defer wg.Done()

	userLocation := types.UserLocation{
		UserLat:        lat,
		UserLon:        lon,
		SearchRadiusKm: distance,
	}
	prompt := getAttractionsNeabyPrompt(userLocation)
	span.SetAttributes(attribute.Int("prompt.length", len(prompt)))

	if s.aiClient == nil {
		err := fmt.Errorf("AI client is not available - check API key configuration")
		span.RecordError(err)
		span.SetStatus(codes.Error, "AI client unavailable")
		resultCh <- types.GenAIResponse{Err: err}
		return
	}

	startTime := time.Now()
	response, err := s.aiClient.GenerateResponse(ctx, prompt, config)
	latencyMs := int(time.Since(startTime).Milliseconds())
	span.SetAttributes(attribute.Int("response.latency_ms", latencyMs))

	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to generate general POIs")
		resultCh <- types.GenAIResponse{Err: fmt.Errorf("failed to generate general POIs: %w", err)}
		return
	}

	var txt string
	for _, candidate := range response.Candidates {
		if candidate.Content != nil && len(candidate.Content.Parts) > 0 {
			txt = candidate.Content.Parts[0].Text
			break
		}
	}
	if txt == "" {
		err := fmt.Errorf("no valid general POI content from AI")
		span.RecordError(err)
		span.SetStatus(codes.Error, "Empty response from AI")
		resultCh <- types.GenAIResponse{Err: err}
		return
	}
	span.SetAttributes(attribute.Int("response.length", len(txt)))

	cleanTxt := cleanJSONResponse(txt)
	var poiData struct {
		PointsOfInterest []types.POIDetailedInfo `json:"points_of_interest"`
	}
	if err := json.Unmarshal([]byte(cleanTxt), &poiData); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to parse general POI JSON")
		resultCh <- types.GenAIResponse{Err: fmt.Errorf("failed to parse general POI JSON: %w", err)}
		return
	}

	fmt.Println(cleanTxt)

	span.SetAttributes(attribute.Int("pois.count", len(poiData.PointsOfInterest)))
	span.SetStatus(codes.Ok, "General POIs generated successfully")
	resultCh <- types.GenAIResponse{
		GeneralPOI: poiData.PointsOfInterest,
		ModelName:  s.aiClient.ModelName,
		Prompt:     prompt,
		Response:   cleanTxt,
	}
}
