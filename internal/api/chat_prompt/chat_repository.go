package llmchat

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"

	"github.com/FACorreiaa/go-poi-au-suggestions/internal/types"
)

var _ Repository = (*RepositoryImpl)(nil)

type Repository interface {
	SaveInteraction(ctx context.Context, interaction types.LlmInteraction) (uuid.UUID, error)
	SaveLlmSuggestedPOIsBatch(ctx context.Context, pois []types.POIDetailedInfo, userID, searchProfileID, llmInteractionID, cityID uuid.UUID) error
	GetLlmSuggestedPOIsByInteractionSortedByDistance(ctx context.Context, llmInteractionID uuid.UUID, cityID uuid.UUID, userLocation types.UserLocation) ([]types.POIDetailedInfo, error)
	AddChatToBookmark(ctx context.Context, itinerary *types.UserSavedItinerary) (uuid.UUID, error)
	GetBookmarkedItineraries(ctx context.Context, userID uuid.UUID, page, limit int) (*types.PaginatedUserItinerariesResponse, error)
	RemoveChatFromBookmark(ctx context.Context, userID, itineraryID uuid.UUID) error
	GetInteractionByID(ctx context.Context, interactionID uuid.UUID) (*types.LlmInteraction, error)
	GetLatestInteractionBySessionID(ctx context.Context, sessionID uuid.UUID) (*types.LlmInteraction, error)

	// Session methods
	CreateSession(ctx context.Context, session types.ChatSession) error
	GetSession(ctx context.Context, sessionID uuid.UUID) (*types.ChatSession, error)
	GetUserChatSessions(ctx context.Context, userID uuid.UUID, page, limit int) (*types.ChatSessionsResponse, error)
	UpdateSession(ctx context.Context, session types.ChatSession) error
	AddMessageToSession(ctx context.Context, sessionID uuid.UUID, message types.ConversationMessage) error

	//
	SaveSinglePOI(ctx context.Context, poi types.POIDetailedInfo, userID, cityID uuid.UUID, llmInteractionID uuid.UUID) (uuid.UUID, error)
	GetPOIsBySessionSortedByDistance(ctx context.Context, sessionID, cityID uuid.UUID, userLocation types.UserLocation) ([]types.POIDetailedInfo, error)
	GetOrCreatePOI(ctx context.Context, tx pgx.Tx, POIDetailedInfo types.POIDetailedInfo, cityID uuid.UUID, sourceInteractionID uuid.UUID) (uuid.UUID, error)
	SaveItineraryPOIs(ctx context.Context, itineraryID uuid.UUID, pois []types.POIDetailedInfo) error

	// RAG
	//SaveInteractionWithEmbedding(ctx context.Context, interaction types.LlmInteraction, embedding []float32) (uuid.UUID, error)
	//FindSimilarInteractions(ctx context.Context, queryEmbedding []float32, limit int, threshold float32) ([]types.LlmInteraction, error)
}

type RepositoryImpl struct {
	logger *slog.Logger
	pgpool *pgxpool.Pool
}

func NewRepositoryImpl(pgxpool *pgxpool.Pool, logger *slog.Logger) *RepositoryImpl {
	return &RepositoryImpl{
		logger: logger,
		pgpool: pgxpool,
	}
}

func (r *RepositoryImpl) SaveInteraction(ctx context.Context, interaction types.LlmInteraction) (uuid.UUID, error) {
	ctx, span := otel.Tracer("LlmInteractionRepo").Start(ctx, "SaveInteraction", trace.WithAttributes(
		semconv.DBSystemKey.String(semconv.DBSystemPostgreSQL.Value.AsString()),
		attribute.String("db.operation", "INSERT_COMPLEX"),
		attribute.String("db.sql.table", "llm_interactions,itineraries,itinerary_pois"),
		attribute.String("user.id", interaction.UserID.String()),
		attribute.String("model.used", interaction.ModelUsed),
		attribute.Int("latency.ms", interaction.LatencyMs),
		attribute.String("city.name_from_interaction", interaction.CityName),
	))
	defer span.End()

	var err error
	tx, err := r.pgpool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to start transaction")
		return uuid.Nil, fmt.Errorf("failed to start transaction: %w", err)
	}
	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback(ctx)
			panic(p)
		}
		if err != nil {
			if rbErr := tx.Rollback(ctx); rbErr != nil {
				r.logger.ErrorContext(ctx, "Transaction rollback failed after error", "original_error", err, "rollback_error", rbErr)
				span.RecordError(fmt.Errorf("transaction rollback failed: %v (original error: %w)", rbErr, err))
			}
		}
	}()

	interactionQuery := `
        INSERT INTO llm_interactions (
            user_id, session_id, prompt, response, model_name, latency_ms, city_name
        ) VALUES ($1, $2, $3, $4, $5, $6, $7)
        RETURNING id
    `
	var interactionID uuid.UUID
	err = tx.QueryRow(ctx, interactionQuery,
		interaction.UserID,
		interaction.SessionID,
		interaction.Prompt,
		interaction.ResponseText,
		interaction.ModelUsed,
		interaction.LatencyMs,
		interaction.CityName,
	).Scan(&interactionID)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to insert llm_interaction")
		return uuid.Nil, fmt.Errorf("failed to insert llm_interaction: %w", err)
	}
	span.SetAttributes(attribute.String("llm_interaction.id", interactionID.String()))

	var cityID uuid.UUID
	if interaction.CityName != "" {
		cityQuery := `SELECT id FROM cities WHERE name = $1 LIMIT 1`
		err = tx.QueryRow(ctx, cityQuery, interaction.CityName).Scan(&cityID)
		if err != nil {
			if err == pgx.ErrNoRows {
				r.logger.WarnContext(ctx, "City not found in database, itinerary creation will be skipped", "city_name", interaction.CityName, "interaction_id", interactionID.String())
				span.AddEvent("City not found in database", trace.WithAttributes(attribute.String("city.name", interaction.CityName)))
				// err is pgx.ErrNoRows, so cityID remains uuid.Nil, processing continues correctly. Clear err.
				err = nil
			} else {
				span.RecordError(err)
				span.SetStatus(codes.Error, "Failed to get city_id")
				return interactionID, fmt.Errorf("failed to get city_id for city '%s': %w", interaction.CityName, err)
			}
		} else {
			span.SetAttributes(attribute.String("city.id", cityID.String()))
		}
	} else {
		r.logger.InfoContext(ctx, "interaction.CityName is empty, cannot determine city_id. Itinerary creation will be skipped.", "interaction_id", interactionID.String())
		span.AddEvent("interaction.CityName is empty")
	}

	var itineraryID uuid.UUID
	if cityID != uuid.Nil {
		itineraryQuery := `
	        INSERT INTO itineraries (user_id, city_id, source_llm_interaction_id)
	        VALUES ($1, $2, $3)
	        ON CONFLICT (user_id, city_id) DO UPDATE SET
	            updated_at = NOW(),
	            source_llm_interaction_id = EXCLUDED.source_llm_interaction_id
	        RETURNING id
	    `
		err = tx.QueryRow(ctx, itineraryQuery, interaction.UserID, cityID, interactionID).Scan(&itineraryID)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, "Failed to insert or update itinerary")
			return interactionID, fmt.Errorf("failed to insert or update itinerary: %w", err)
		}
		span.SetAttributes(attribute.String("itinerary.id", itineraryID.String()))
	}

	if itineraryID != uuid.Nil {
		var pois []types.POIDetailedInfo
		// Only parse POIs for itinerary/general responses, skip for domain-specific responses
		if strings.Contains(interaction.Prompt, "Unified Chat - Domain: dining") ||
			strings.Contains(interaction.Prompt, "Unified Chat - Domain: accommodation") ||
			strings.Contains(interaction.Prompt, "Unified Chat - Domain: activities") {
			// Skip POI parsing for domain-specific responses that don't contain POIs
			r.logger.DebugContext(ctx, "Skipping POI parsing for domain-specific response", "interaction_id", interactionID.String())
			span.AddEvent("Skipped POI parsing for domain-specific response")
		} else {
			pois, err = parsePOIsFromResponse(interaction.ResponseText, r.logger)
			if err != nil {
				span.RecordError(err)
				span.SetStatus(codes.Error, "Failed to parse POIs from response")
				return interactionID, fmt.Errorf("failed to parse POIs from response: %w", err)
			}
		}
		span.SetAttributes(attribute.Int("parsed_pois.count", len(pois)))

		if len(pois) > 0 {
			poiBatch := &pgx.Batch{}
			itineraryPoiInsertQuery := `
	            INSERT INTO itinerary_pois (itinerary_id, poi_id, order_index, ai_description)
	            VALUES ($1, $2, $3, $4)
	            ON CONFLICT (itinerary_id, poi_id) DO UPDATE SET
	                order_index = EXCLUDED.order_index,
	                ai_description = EXCLUDED.ai_description,
	                updated_at = NOW()
	        `
			for i, POIDetailedInfoFromLlm := range pois {
				var poiDBID uuid.UUID
				poiDBID, err = r.GetOrCreatePOI(ctx, tx, POIDetailedInfoFromLlm, cityID, interactionID)
				if err != nil {
					span.RecordError(err)
					span.SetStatus(codes.Error, "Failed to get or create POI")
					return interactionID, fmt.Errorf("failed to get or create POI '%s': %w", POIDetailedInfoFromLlm.Name, err)
				}
				poiBatch.Queue(itineraryPoiInsertQuery, itineraryID, poiDBID, i, POIDetailedInfoFromLlm.DescriptionPOI) // Assumes types.POIDetailedInfo has DescriptionPOI
			}

			if poiBatch.Len() > 0 {
				br := tx.SendBatch(ctx, poiBatch)
				for i := 0; i < poiBatch.Len(); i++ {
					_, execErr := br.Exec()
					if execErr != nil {
						err = fmt.Errorf("failed to insert itinerary_poi in batch (operation %d of %d for itinerary %s): %w", i+1, poiBatch.Len(), itineraryID.String(), execErr)
						if closeErr := br.Close(); closeErr != nil {
							r.logger.ErrorContext(ctx, "Failed to close batch for itinerary_pois after an exec error", "close_error", closeErr, "original_batch_error", err)
						}
						span.RecordError(err)
						span.SetStatus(codes.Error, "Failed to insert itinerary_poi in batch")
						return interactionID, err
					}
				}
				err = br.Close()
				if err != nil {
					span.RecordError(err)
					span.SetStatus(codes.Error, "Failed to close batch for itinerary_pois")
					return interactionID, fmt.Errorf("failed to close batch for itinerary_pois: %w", err)
				}
				span.SetAttributes(attribute.Int("itinerary_pois.inserted_or_updated.count", poiBatch.Len()))
			}
		}
	} else {
		if cityID != uuid.Nil {
			r.logger.WarnContext(ctx, "ItineraryID is Nil despite valid CityID, indicating itinerary insert/update issue.", "city_id", cityID.String(), "interaction_id", interactionID.String())
			span.AddEvent("ItineraryID is Nil despite valid CityID.")
		} else {
			r.logger.InfoContext(ctx, "Skipping itinerary_pois: itineraryID is Nil (likely city not found or CityName empty).", "interaction_id", interactionID.String())
			span.AddEvent("Skipping itinerary_pois: itineraryID is Nil.")
		}
	}

	err = tx.Commit(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to commit transaction")
		return uuid.Nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	span.SetStatus(codes.Ok, "Interaction and related entities saved successfully")
	return interactionID, nil
}

func (r *RepositoryImpl) SaveLlmSuggestedPOIsBatch(ctx context.Context, pois []types.POIDetailedInfo, userID, searchProfileID, llmInteractionID, cityID uuid.UUID) error {
	ctx, span := otel.Tracer("LlmInteractionRepo").Start(ctx, "SaveLlmSuggestedPOIsBatch", trace.WithAttributes(
		semconv.DBSystemPostgreSQL,
		attribute.String("db.operation", "INSERT"),
		attribute.String("db.sql.table", "llm_suggested_pois"),
		attribute.String("user.id", userID.String()),
		attribute.String("search_profile.id", searchProfileID.String()),
		attribute.String("llm_interaction.id", llmInteractionID.String()),
		attribute.String("city.id", cityID.String()),
		attribute.Int("pois.count", len(pois)),
	))
	defer span.End()

	r.logger.InfoContext(ctx, "SaveLlmSuggestedPOIsBatch - About to save batch",
		slog.String("llm_interaction_id", llmInteractionID.String()),
		slog.String("user_id", userID.String()),
		slog.String("city_id", cityID.String()),
		slog.Int("poi_count", len(pois)))

	// Verify the llm_interaction_id exists before trying to insert POIs
	var exists bool
	checkQuery := `SELECT EXISTS(SELECT 1 FROM llm_interactions WHERE id = $1)`
	err := r.pgpool.QueryRow(ctx, checkQuery, llmInteractionID).Scan(&exists)
	if err != nil {
		r.logger.ErrorContext(ctx, "Failed to check if llm_interaction exists", slog.Any("error", err))
		return fmt.Errorf("failed to check if llm_interaction exists: %w", err)
	}
	if !exists {
		r.logger.ErrorContext(ctx, "llm_interaction_id does not exist in database",
			slog.String("llm_interaction_id", llmInteractionID.String()))
		return fmt.Errorf("llm_interaction_id %s does not exist in database", llmInteractionID.String())
	}
	r.logger.InfoContext(ctx, "llm_interaction_id exists, proceeding with POI batch insert")

	batch := &pgx.Batch{}
	query := `
        INSERT INTO llm_suggested_pois 
            (user_id, search_profile_id, llm_interaction_id, city_id, 
             name, description_poi, location)
        VALUES 
            ($1, $2, $3, $4, $5, $6, ST_SetSRID(ST_MakePoint($7, $8), 4326))
    `

	for _, poi := range pois {
		batch.Queue(query,
			userID, searchProfileID, llmInteractionID, cityID,
			poi.Name, poi.DescriptionPOI, poi.Longitude, poi.Latitude, // Lon, Lat order for ST_MakePoint
		)
	}

	br := r.pgpool.SendBatch(ctx, batch)
	defer br.Close()

	for i := 0; i < len(pois); i++ {
		_, err := br.Exec()
		if err != nil {
			// Consider how to handle partial failures. Log and continue, or return error?
			span.RecordError(err)
			span.SetStatus(codes.Error, fmt.Sprintf("Failed to execute batch insert for POI %d", i))
			return fmt.Errorf("failed to execute batch insert for llm_suggested_poi %d: %w", i, err)
		}
	}

	span.SetStatus(codes.Ok, "POIs batch saved successfully")
	return nil
}

func (r *RepositoryImpl) GetLlmSuggestedPOIsByInteractionSortedByDistance(
	ctx context.Context, llmInteractionID uuid.UUID, cityID uuid.UUID, userLocation types.UserLocation,
) ([]types.POIDetailedInfo, error) {
	ctx, span := otel.Tracer("LlmInteractionRepo").Start(ctx, "GetLlmSuggestedPOIsByInteractionSortedByDistance", trace.WithAttributes(
		semconv.DBSystemPostgreSQL,
		attribute.String("db.operation", "SELECT"),
		attribute.String("db.sql.table", "llm_suggested_pois"),
		attribute.String("llm_interaction.id", llmInteractionID.String()),
		attribute.String("city.id", cityID.String()),
		attribute.Float64("user.latitude", userLocation.UserLat),
		attribute.Float64("user.longitude", userLocation.UserLon),
	))
	defer span.End()

	userPoint := fmt.Sprintf("SRID=4326;POINT(%f %f)", userLocation.UserLon, userLocation.UserLat)

	// Ensure cityID filter is applied if cityID is not Nil
	// We filter by llm_interaction_id, so city_id might be redundant if interaction is specific to a city context
	// But adding it for robustness if an interaction could span POIs from different "requested" cities (unlikely for current setup).
	query := `
        SELECT 
            id, 
            name, 
            description_poi,
            ST_X(location::geometry) AS longitude, 
            ST_Y(location::geometry) AS latitude, 
            ST_Distance(location::geography, ST_GeomFromText($1, 4326)::geography) AS distance
        FROM llm_suggested_pois
        WHERE llm_interaction_id = $2 `

	args := []interface{}{userPoint, llmInteractionID}
	argCounter := 3

	if cityID != uuid.Nil {
		query += fmt.Sprintf("AND city_id = $%d ", argCounter)
		args = append(args, cityID)
		argCounter++
	}

	query += "ORDER BY distance ASC"

	rows, err := r.pgpool.Query(ctx, query, args...)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to query sorted POIs")
		return nil, fmt.Errorf("failed to query sorted llm_suggested_pois: %w", err)
	}
	defer rows.Close()

	var resultPois []types.POIDetailedInfo
	for rows.Next() {
		var p types.POIDetailedInfo
		var descr sql.NullString // Handle nullable fields from DB
		// var cat sql.NullString
		// var addr sql.NullString
		// var web sql.NullString
		// var openH sql.NullString

		err := rows.Scan(
			&p.ID, &p.Name, &descr,
			&p.Longitude, &p.Latitude,
			&p.Distance, // Ensure your types.POIDetailedInfo has Distance field
		)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, "Failed to scan POI row")
			return nil, fmt.Errorf("failed to scan llm_suggested_poi row: %w", err)
		}
		p.DescriptionPOI = descr.String
		//p.Category = cat.String

		resultPois = append(resultPois, p)
	}

	if err = rows.Err(); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Error iterating POI rows")
		return nil, fmt.Errorf("error iterating llm_suggested_poi rows: %w", err)
	}

	span.SetAttributes(attribute.Int("pois.count", len(resultPois)))
	span.SetStatus(codes.Ok, "POIs retrieved successfully")
	return resultPois, nil
}

func (r *RepositoryImpl) AddChatToBookmark(ctx context.Context, itinerary *types.UserSavedItinerary) (uuid.UUID, error) {
	ctx, span := otel.Tracer("LlmInteractionRepo").Start(ctx, "AddChatToBookmark", trace.WithAttributes(
		semconv.DBSystemPostgreSQL,
		attribute.String("db.operation", "INSERT"),
		attribute.String("db.sql.table", "user_saved_itineraries"),
		attribute.String("user.id", itinerary.UserID.String()),
		attribute.String("title", itinerary.Title),
	))
	defer span.End()

	tx, err := r.pgpool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to start transaction")
		return uuid.Nil, fmt.Errorf("failed to start transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	query := `
		INSERT INTO user_saved_itineraries (
			user_id, source_llm_interaction_id, session_id, primary_city_id, title, description,
			markdown_content, tags, estimated_duration_days, estimated_cost_level, is_public
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		RETURNING id
	`
	var savedItineraryID uuid.UUID
	if err := tx.QueryRow(ctx, query,
		&itinerary.UserID,
		&itinerary.SourceLlmInteractionID,
		&itinerary.SessionID,
		&itinerary.PrimaryCityID,
		&itinerary.Title,
		&itinerary.Description,
		&itinerary.MarkdownContent,
		&itinerary.Tags,
		&itinerary.EstimatedDurationDays,
		&itinerary.EstimatedCostLevel,
		&itinerary.IsPublic,
	).Scan(&savedItineraryID); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to insert itinerary")
		return uuid.Nil, fmt.Errorf("failed to insert user_saved_itineraries: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to commit transaction")
		return uuid.Nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Debug: Log what was saved for debugging bookmark removal issues
	r.logger.InfoContext(ctx, "Successfully saved bookmark to database",
		slog.String("saved_itinerary_id", savedItineraryID.String()),
		slog.String("user_id", itinerary.UserID.String()),
		slog.String("title", itinerary.Title),
		slog.String("session_id", func() string {
			if itinerary.SessionID.Valid {
				return uuid.UUID(itinerary.SessionID.Bytes).String()
			}
			return "null"
		}()),
		slog.String("source_llm_interaction_id", func() string {
			if itinerary.SourceLlmInteractionID.Valid {
				return uuid.UUID(itinerary.SourceLlmInteractionID.Bytes).String()
			}
			return "null"
		}()))

	span.SetAttributes(attribute.String("saved_itinerary.id", savedItineraryID.String()))
	span.SetStatus(codes.Ok, "Itinerary saved successfully")
	return savedItineraryID, nil
}

func (r *RepositoryImpl) GetInteractionByID(ctx context.Context, interactionID uuid.UUID) (*types.LlmInteraction, error) {
	ctx, span := otel.Tracer("LlmInteractionRepo").Start(ctx, "GetInteractionByID", trace.WithAttributes(
		semconv.DBSystemPostgreSQL,
		attribute.String("db.operation", "SELECT"),
		attribute.String("db.sql.table", "llm_interactions"),
		attribute.String("interaction.id", interactionID.String()),
	))
	defer span.End()

	query := `
		SELECT 
			id, user_id, prompt, response, model_name, latency_ms,
			prompt_tokens, completion_tokens, total_tokens,
			request_payload, response_payload
		FROM llm_interactions
		WHERE id = $1
	`
	row := r.pgpool.QueryRow(ctx, query, interactionID)

	var interaction types.LlmInteraction

	nullPromptTokens := sql.NullInt64{}
	nullCompletionTokens := sql.NullInt64{}
	nullTotalTokens := sql.NullInt64{}
	nullRequestPayload := sql.NullString{}
	nullResponsePayload := sql.NullString{}

	if err := row.Scan(
		&interaction.ID,
		&interaction.UserID,
		&interaction.Prompt,
		&interaction.ResponseText,
		&interaction.ModelUsed,
		&interaction.LatencyMs,
		&nullPromptTokens,
		&nullCompletionTokens,
		&nullTotalTokens,
		&nullRequestPayload,
		&nullResponsePayload,
	); err != nil {
		if err == pgx.ErrNoRows {
			span.SetStatus(codes.Error, "Interaction not found")
			return nil, fmt.Errorf("no interaction found with ID %s", interactionID)
		}
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to scan interaction row")
		return nil, fmt.Errorf("failed to scan llm_interaction row: %w", err)
	}

	span.SetAttributes(
		attribute.String("user.id", interaction.UserID.String()),
		attribute.String("model.used", interaction.ModelUsed),
		attribute.Int("latency.ms", interaction.LatencyMs),
	)
	span.SetStatus(codes.Ok, "Interaction retrieved successfully")
	return &interaction, nil
}

func (r *RepositoryImpl) GetLatestInteractionBySessionID(ctx context.Context, sessionID uuid.UUID) (*types.LlmInteraction, error) {
	ctx, span := otel.Tracer("LlmInteractionRepo").Start(ctx, "GetLatestInteractionBySessionID", trace.WithAttributes(
		semconv.DBSystemPostgreSQL,
		attribute.String("db.operation", "SELECT"),
		attribute.String("db.sql.table", "llm_interactions"),
		attribute.String("session.id", sessionID.String()),
	))
	defer span.End()

	query := `
		SELECT 
			id, user_id, session_id, prompt, response, model_name, latency_ms,
			prompt_tokens, completion_tokens, total_tokens,
			request_payload, response_payload, city_name, created_at
		FROM llm_interactions
		WHERE session_id = $1
		ORDER BY created_at DESC
		LIMIT 1
	`
	row := r.pgpool.QueryRow(ctx, query, sessionID)

	var interaction types.LlmInteraction

	nullPromptTokens := sql.NullInt64{}
	nullCompletionTokens := sql.NullInt64{}
	nullTotalTokens := sql.NullInt64{}
	nullRequestPayload := sql.NullString{}
	nullResponsePayload := sql.NullString{}
	nullCityName := sql.NullString{}
	nullSessionID := uuid.NullUUID{}

	if err := row.Scan(
		&interaction.ID,
		&interaction.UserID,
		&nullSessionID,
		&interaction.Prompt,
		&interaction.ResponseText,
		&interaction.ModelUsed,
		&interaction.LatencyMs,
		&nullPromptTokens,
		&nullCompletionTokens,
		&nullTotalTokens,
		&nullRequestPayload,
		&nullResponsePayload,
		&nullCityName,
		&interaction.Timestamp,
	); err != nil {
		if err == pgx.ErrNoRows {
			span.SetStatus(codes.Error, "No interactions found for session")
			return nil, fmt.Errorf("no interactions found for session ID %s", sessionID)
		}
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to scan interaction row")
		return nil, fmt.Errorf("failed to scan llm_interaction row: %w", err)
	}

	// Handle nullable fields
	if nullSessionID.Valid {
		interaction.SessionID = nullSessionID.UUID
	}
	if nullCityName.Valid {
		interaction.CityName = nullCityName.String
	}
	if nullPromptTokens.Valid {
		interaction.PromptTokens = int(nullPromptTokens.Int64)
	}
	if nullCompletionTokens.Valid {
		interaction.CompletionTokens = int(nullCompletionTokens.Int64)
	}
	if nullTotalTokens.Valid {
		interaction.TotalTokens = int(nullTotalTokens.Int64)
	}

	span.SetAttributes(
		attribute.String("user.id", interaction.UserID.String()),
		attribute.String("session.id", interaction.SessionID.String()),
		attribute.String("model.used", interaction.ModelUsed),
		attribute.Int("latency.ms", interaction.LatencyMs),
	)
	span.SetStatus(codes.Ok, "Latest interaction retrieved successfully")
	return &interaction, nil
}

func (r *RepositoryImpl) RemoveChatFromBookmark(ctx context.Context, userID, itineraryID uuid.UUID) error {
	ctx, span := otel.Tracer("LlmInteractionRepo").Start(ctx, "RemoveChatFromBookmark", trace.WithAttributes(
		semconv.DBSystemPostgreSQL,
		attribute.String("db.operation", "DELETE"),
		attribute.String("db.sql.table", "user_saved_itineraries"),
		attribute.String("user.id", userID.String()),
		attribute.String("itinerary.id", itineraryID.String()),
	))
	defer span.End()

	tx, err := r.pgpool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to start transaction")
		return fmt.Errorf("failed to start transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	query := `
		DELETE FROM user_saved_itineraries
		WHERE id = $1 AND user_id = $2
	`
	_, err = tx.Exec(ctx, query, itineraryID, userID)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to delete itinerary")
		return fmt.Errorf("failed to delete user_saved_itinerary with ID %s: %w", itineraryID, err)
	}

	//if tag.RowsAffected() == 0 {
	//	// Debug: List all itineraries for this user to help with debugging
	//	r.logger.InfoContext(ctx, "Attempting to remove non-existent itinerary, debugging available itineraries",
	//		slog.String("requested_itinerary_id", itineraryID.String()),
	//		slog.String("user_id", userID.String()))
	//
	//	debugQuery := `SELECT id, title, session_id, source_llm_interaction_id, created_at FROM user_saved_itineraries WHERE user_id = $1 ORDER BY created_at DESC LIMIT 10`
	//	rows, debugErr := r.pgpool.Query(ctx, debugQuery, userID)
	//	if debugErr != nil {
	//		r.logger.ErrorContext(ctx, "Failed to execute debug query", slog.Any("error", debugErr))
	//	} else {
	//		defer rows.Close()
	//		count := 0
	//		for rows.Next() {
	//			var id uuid.UUID
	//			var title string
	//			var createdAt time.Time
	//			var sessionIDStr, sourceIDStr sql.NullString
	//
	//			if scanErr := rows.Scan(&id, &title, &sessionIDStr, &sourceIDStr, &createdAt); scanErr == nil {
	//				count++
	//				r.logger.InfoContext(ctx, "Found existing itinerary",
	//					slog.Int("index", count),
	//					slog.String("id", id.String()),
	//					slog.String("title", title),
	//					slog.String("session_id", func() string {
	//						if sessionIDStr.Valid {
	//							return sessionIDStr.String
	//						}
	//						return "null"
	//					}()),
	//					slog.String("source_llm_interaction_id", func() string {
	//						if sourceIDStr.Valid {
	//							return sourceIDStr.String
	//						}
	//						return "null"
	//					}()),
	//					slog.Time("created_at", createdAt))
	//			} else {
	//				r.logger.WarnContext(ctx, "Failed to scan debug row", slog.Any("error", scanErr))
	//			}
	//		}
	//		if count == 0 {
	//			r.logger.InfoContext(ctx, "No itineraries found for user")
	//		} else {
	//			r.logger.InfoContext(ctx, "Debug query completed", slog.Int("total_found", count))
	//		}
	//	}
	//
	//	// Check if the itinerary exists but belongs to a different user
	//	var existsForOtherUser bool
	//	checkQuery := `SELECT EXISTS(SELECT 1 FROM user_saved_itineraries WHERE id = $1)`
	//	checkErr := r.pgpool.QueryRow(ctx, checkQuery, itineraryID).Scan(&existsForOtherUser)
	//	if checkErr != nil {
	//		r.logger.ErrorContext(ctx, "Failed to check if itinerary exists for other user", slog.Any("error", checkErr))
	//	}
	//
	//	if existsForOtherUser {
	//		err := fmt.Errorf("itinerary with ID %s exists but belongs to a different user (attempted by user %s)", itineraryID, userID)
	//		r.logger.WarnContext(ctx, "Attempted to delete itinerary belonging to different user",
	//			slog.String("itineraryID", itineraryID.String()),
	//			slog.String("userID", userID.String()))
	//		span.RecordError(err)
	//		span.SetStatus(codes.Error, "Itinerary belongs to different user")
	//		return err
	//	} else {
	//		// Itinerary doesn't exist - this is actually OK for idempotent DELETE operations
	//		// The desired outcome (itinerary not existing) is achieved
	//		r.logger.InfoContext(ctx, "Attempted to delete non-existent itinerary - treating as successful (idempotent)",
	//			slog.String("itineraryID", itineraryID.String()),
	//			slog.String("userID", userID.String()))
	//		span.SetStatus(codes.Ok, "Itinerary already deleted (idempotent operation)")
	//		return nil
	//	}
	//}

	if err := tx.Commit(ctx); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to commit transaction")
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	span.SetStatus(codes.Ok, "Itinerary removed successfully")
	return nil
}

func (r *RepositoryImpl) GetBookmarkedItineraries(ctx context.Context, userID uuid.UUID, page, limit int) (*types.PaginatedUserItinerariesResponse, error) {
	ctx, span := otel.Tracer("LlmInteractionRepo").Start(ctx, "GetBookmarkedItineraries", trace.WithAttributes(
		semconv.DBSystemPostgreSQL,
		attribute.String("db.operation", "SELECT"),
		attribute.String("db.sql.table", "user_saved_itineraries"),
		attribute.String("user.id", userID.String()),
		attribute.Int("page", page),
		attribute.Int("limit", limit),
	))
	defer span.End()

	// Calculate offset
	offset := (page - 1) * limit

	// Get total count
	var totalCount int
	countQuery := `SELECT COUNT(*) FROM user_saved_itineraries WHERE user_id = $1`
	err := r.pgpool.QueryRow(ctx, countQuery, userID).Scan(&totalCount)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to count bookmarked itineraries")
		return nil, fmt.Errorf("failed to count bookmarked itineraries: %w", err)
	}

	// Get paginated results
	query := `
		SELECT 
			id, user_id, source_llm_interaction_id, session_id, primary_city_id, 
			title, description, markdown_content, tags, estimated_duration_days, 
			estimated_cost_level, is_public, created_at, updated_at
		FROM user_saved_itineraries 
		WHERE user_id = $1 
		ORDER BY created_at DESC 
		LIMIT $2 OFFSET $3
	`

	rows, err := r.pgpool.Query(ctx, query, userID, limit, offset)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to query bookmarked itineraries")
		return nil, fmt.Errorf("failed to query bookmarked itineraries: %w", err)
	}
	defer rows.Close()

	var itineraries []types.UserSavedItinerary
	for rows.Next() {
		var itinerary types.UserSavedItinerary
		var tags []string

		err := rows.Scan(
			&itinerary.ID,
			&itinerary.UserID,
			&itinerary.SourceLlmInteractionID,
			&itinerary.SessionID,
			&itinerary.PrimaryCityID,
			&itinerary.Title,
			&itinerary.Description,
			&itinerary.MarkdownContent,
			&tags,
			&itinerary.EstimatedDurationDays,
			&itinerary.EstimatedCostLevel,
			&itinerary.IsPublic,
			&itinerary.CreatedAt,
			&itinerary.UpdatedAt,
		)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, "Failed to scan itinerary row")
			return nil, fmt.Errorf("failed to scan itinerary row: %w", err)
		}

		itinerary.Tags = tags
		itineraries = append(itineraries, itinerary)
	}

	if err := rows.Err(); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Row iteration error")
		return nil, fmt.Errorf("row iteration error: %w", err)
	}

	response := &types.PaginatedUserItinerariesResponse{
		Itineraries:  itineraries,
		TotalRecords: totalCount,
		Page:         page,
		PageSize:     limit,
	}

	span.SetStatus(codes.Ok, "Bookmarked itineraries retrieved successfully")
	return response, nil
}

// sessions
func (r *RepositoryImpl) CreateSession(ctx context.Context, session types.ChatSession) error {
	tx, err := r.pgpool.Begin(ctx)
	if err != nil {
		r.logger.ErrorContext(ctx, "Failed to begin transaction for session creation", slog.Any("error", err))
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	query := `
        INSERT INTO chat_sessions (
            id, user_id, profile_id, city_name, current_itinerary, conversation_history, session_context,
            created_at, updated_at, expires_at, status
        ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
    `
	itineraryJSON, _ := json.Marshal(session.CurrentItinerary)
	historyJSON, _ := json.Marshal(session.ConversationHistory)
	contextJSON, _ := json.Marshal(session.SessionContext)

	_, err = tx.Exec(ctx, query, session.ID, session.UserID, session.ProfileID, session.CityName,
		itineraryJSON, historyJSON, contextJSON, session.CreatedAt, session.UpdatedAt, session.ExpiresAt, session.Status)
	if err != nil {
		r.logger.ErrorContext(ctx, "Failed to create session", slog.Any("error", err))
		return fmt.Errorf("failed to create session: %w", err)
	}

	if err = tx.Commit(ctx); err != nil {
		r.logger.ErrorContext(ctx, "Failed to commit session creation transaction", slog.Any("error", err))
		return fmt.Errorf("failed to commit session creation: %w", err)
	}

	return nil
}

// GetSession retrieves a session by ID
func (r *RepositoryImpl) GetSession(ctx context.Context, sessionID uuid.UUID) (*types.ChatSession, error) {
	query := `
        SELECT id, user_id, profile_id, city_name, current_itinerary, conversation_history, session_context,
               created_at, updated_at, expires_at, status
        FROM chat_sessions WHERE id = $1
    `
	row := r.pgpool.QueryRow(ctx, query, sessionID)

	var session types.ChatSession
	var itineraryJSON, historyJSON, contextJSON []byte
	err := row.Scan(&session.ID, &session.UserID, &session.ProfileID, &session.CityName,
		&itineraryJSON, &historyJSON, &contextJSON, &session.CreatedAt, &session.UpdatedAt, &session.ExpiresAt, &session.Status)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("session %s not found", sessionID)
		}
		r.logger.ErrorContext(ctx, "Failed to get session", slog.Any("error", err))
		return nil, fmt.Errorf("failed to get session: %w", err)
	}

	err = json.Unmarshal(itineraryJSON, &session.CurrentItinerary)
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(historyJSON, &session.ConversationHistory)
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(contextJSON, &session.SessionContext)
	if err != nil {
		return nil, err
	}
	return &session, nil
}

// GetUserChatSessions retrieves paginated chat history from LLM interactions grouped by session/city, ordered by most recent first
func (r *RepositoryImpl) GetUserChatSessions(ctx context.Context, userID uuid.UUID, page, limit int) (*types.ChatSessionsResponse, error) {
	ctx, span := otel.Tracer("LlmInteractionRepo").Start(ctx, "GetUserChatSessions", trace.WithAttributes(
		semconv.DBSystemKey.String(semconv.DBSystemPostgreSQL.Value.AsString()),
		attribute.String("db.operation", "SELECT"),
		attribute.String("db.sql.table", "llm_interactions"),
		attribute.String("user.id", userID.String()),
		attribute.Int("page", page),
		attribute.Int("limit", limit),
	))
	defer span.End()

	r.logger.InfoContext(ctx, "Getting paginated user chat sessions",
		slog.String("user_id", userID.String()),
		slog.Int("page", page),
		slog.Int("limit", limit))

	// Calculate offset
	offset := (page - 1) * limit

	// Main query with pagination
	query := `
        WITH grouped_interactions AS (
            SELECT 
                COALESCE(session_id::text, city_name || '_' || DATE(created_at)) as session_key,
                user_id,
                city_name,
                MIN(created_at) as first_interaction,
                MAX(created_at) as last_interaction,
                COUNT(*) as interaction_count,
                -- Performance metrics aggregation
                AVG(latency_ms)::int as avg_latency_ms,
                SUM(total_tokens) as total_tokens,
                SUM(prompt_tokens) as total_prompt_tokens,
                SUM(completion_tokens) as total_completion_tokens,
                SUM(latency_ms) as total_latency_ms,
                array_agg(DISTINCT model_name) FILTER (WHERE model_name IS NOT NULL) as models_used,
                json_agg(
                    json_build_object(
                        'id', id,
                        'prompt', prompt,
                        'response', response,
                        'created_at', created_at,
                        'city_name', city_name,
                        'session_id', session_id,
                        'model_name', model_name,
                        'latency_ms', latency_ms,
                        'total_tokens', total_tokens,
                        'prompt_tokens', prompt_tokens,
                        'completion_tokens', completion_tokens
                    ) ORDER BY created_at
                ) as interactions
            FROM llm_interactions 
            WHERE user_id = $1 AND prompt IS NOT NULL
            GROUP BY session_key, user_id, city_name
        )
        SELECT 
            session_key,
            user_id,
            city_name,
            first_interaction,
            last_interaction,
            interaction_count,
            avg_latency_ms,
            total_tokens,
            total_prompt_tokens,
            total_completion_tokens,
            total_latency_ms,
            models_used,
            interactions
        FROM grouped_interactions
        ORDER BY last_interaction DESC
        LIMIT $2 OFFSET $3
    `

	// Count query for total records
	countQuery := `
        WITH grouped_interactions AS (
            SELECT 
                COALESCE(session_id::text, city_name || '_' || DATE(created_at)) as session_key,
                user_id,
                city_name
            FROM llm_interactions 
            WHERE user_id = $1 AND prompt IS NOT NULL
            GROUP BY session_key, user_id, city_name
        )
        SELECT COUNT(*) FROM grouped_interactions
    `

	// Execute count query first
	var total int
	err := r.pgpool.QueryRow(ctx, countQuery, userID).Scan(&total)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to get total count")
		r.logger.ErrorContext(ctx, "Failed to get total chat sessions count", slog.Any("error", err))
		return nil, fmt.Errorf("failed to get total count: %w", err)
	}

	// Execute main query
	rows, err := r.pgpool.Query(ctx, query, userID, limit, offset)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to query LLM interactions")
		r.logger.ErrorContext(ctx, "Failed to get user chat sessions from LLM interactions", slog.Any("error", err), slog.String("user_id", userID.String()))
		return nil, fmt.Errorf("failed to get user chat sessions: %w", err)
	}
	defer rows.Close()

	var sessions []types.ChatSession
	for rows.Next() {
		var sessionKey, cityName string
		var userIDFromDB uuid.UUID
		var firstInteraction, lastInteraction time.Time
		var interactionCount int
		var avgLatencyMs, totalTokens, totalPromptTokens, totalCompletionTokens, totalLatencyMs sql.NullInt64
		var modelsUsed []string
		var interactionsJSON string

		err := rows.Scan(
			&sessionKey, &userIDFromDB, &cityName, &firstInteraction, &lastInteraction, &interactionCount,
			&avgLatencyMs, &totalTokens, &totalPromptTokens, &totalCompletionTokens, &totalLatencyMs,
			&modelsUsed, &interactionsJSON,
		)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, "Failed to scan LLM interaction row")
			r.logger.ErrorContext(ctx, "Failed to scan LLM interaction row", slog.Any("error", err))
			return nil, fmt.Errorf("failed to scan LLM interaction row: %w", err)
		}

		var interactions []map[string]interface{}
		if err := json.Unmarshal([]byte(interactionsJSON), &interactions); err != nil {
			r.logger.WarnContext(ctx, "Failed to parse interactions JSON", slog.Any("error", err))
			continue
		}

		var conversationHistory []types.ConversationMessage
		var totalPOIs, totalHotels, totalRestaurants int
		var citiesCovered []string
		var hasItinerary bool
		var dominantCategories []string

		for _, interaction := range interactions {
			if prompt, ok := interaction["prompt"].(string); ok && prompt != "" {
				conversationHistory = append(conversationHistory, types.ConversationMessage{
					Role:      "user",
					Content:   prompt,
					Timestamp: parseTimeFromInterface(interaction["created_at"]),
				})
			}
			if response, ok := interaction["response"].(string); ok {
				if response == "" {
					response = fmt.Sprintf("I provided recommendations for %s", cityName)
				} else {
					// Count content items from response for metrics
					contentCounts := countContentFromResponse(response)
					totalPOIs += contentCounts.POIs
					totalHotels += contentCounts.Hotels
					totalRestaurants += contentCounts.Restaurants
					if contentCounts.HasItinerary {
						hasItinerary = true
					}
					dominantCategories = append(dominantCategories, contentCounts.Categories...)

					// Convert JSON response to human-readable format
					response = formatResponseForDisplay(response, cityName)
				}
				conversationHistory = append(conversationHistory, types.ConversationMessage{
					Role:      "assistant",
					Content:   response,
					Timestamp: parseTimeFromInterface(interaction["created_at"]),
				})
			}
		}

		// Calculate enriched metrics
		performanceMetrics := types.SessionPerformanceMetrics{
			AvgResponseTimeMs: int(avgLatencyMs.Int64),
			TotalTokens:       int(totalTokens.Int64),
			PromptTokens:      int(totalPromptTokens.Int64),
			CompletionTokens:  int(totalCompletionTokens.Int64),
			ModelsUsed:        modelsUsed,
			TotalLatencyMs:    int(totalLatencyMs.Int64),
		}

		// Calculate unique cities covered
		citiesMap := make(map[string]bool)
		citiesMap[cityName] = true
		for _, city := range citiesCovered {
			citiesMap[city] = true
		}
		uniqueCities := make([]string, 0, len(citiesMap))
		for city := range citiesMap {
			uniqueCities = append(uniqueCities, city)
		}

		// Calculate complexity score (1-10)
		complexityScore := calculateComplexityScore(totalPOIs, totalHotels, totalRestaurants, len(conversationHistory), hasItinerary)

		contentMetrics := types.SessionContentMetrics{
			TotalPOIs:          totalPOIs,
			TotalHotels:        totalHotels,
			TotalRestaurants:   totalRestaurants,
			CitiesCovered:      uniqueCities,
			HasItinerary:       hasItinerary,
			ComplexityScore:    complexityScore,
			DominantCategories: uniqueStringSlice(dominantCategories),
		}

		// Calculate engagement metrics
		userMsgCount, assistantMsgCount := countMessagesByRole(conversationHistory)
		conversationDuration := lastInteraction.Sub(firstInteraction)
		avgMsgLength := calculateAverageMessageLength(conversationHistory)
		engagementLevel := calculateEngagementLevel(len(conversationHistory), conversationDuration, complexityScore)

		engagementMetrics := types.SessionEngagementMetrics{
			MessageCount:          len(conversationHistory),
			ConversationDuration:  conversationDuration,
			UserMessageCount:      userMsgCount,
			AssistantMessageCount: assistantMsgCount,
			AvgMessageLength:      avgMsgLength,
			EngagementLevel:       engagementLevel,
		}

		sessionID := uuid.NewSHA1(uuid.NameSpaceOID, []byte(sessionKey))
		session := types.ChatSession{
			ID:                  sessionID,
			UserID:              userIDFromDB,
			CityName:            cityName,
			ConversationHistory: conversationHistory,
			CreatedAt:           firstInteraction,
			UpdatedAt:           lastInteraction,
			Status:              "active",
			PerformanceMetrics:  performanceMetrics,
			ContentMetrics:      contentMetrics,
			EngagementMetrics:   engagementMetrics,
		}
		sessions = append(sessions, session)
	}

	if err = rows.Err(); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Error iterating through LLM interaction rows")
		r.logger.ErrorContext(ctx, "Error iterating through LLM interaction rows", slog.Any("error", err))
		return nil, fmt.Errorf("error iterating through LLM interaction rows: %w", err)
	}

	// Calculate hasMore
	hasMore := (page * limit) < total

	response := &types.ChatSessionsResponse{
		Sessions: sessions,
		Total:    total,
		Page:     page,
		Limit:    limit,
		HasMore:  hasMore,
	}

	r.logger.InfoContext(ctx, "Successfully retrieved paginated chat sessions",
		slog.String("user_id", userID.String()),
		slog.Int("sessions_count", len(sessions)),
		slog.Int("total", total),
		slog.Int("page", page),
		slog.Int("limit", limit),
		slog.Bool("has_more", hasMore))

	span.SetAttributes(
		attribute.Int("sessions.count", len(sessions)),
		attribute.Int("sessions.total", total),
		attribute.Int("response.page", page),
		attribute.Int("response.limit", limit),
		attribute.Bool("response.has_more", hasMore),
	)
	span.SetStatus(codes.Ok, "Chat sessions retrieved successfully")
	return response, nil
}

// Helper function to parse time from interface{}
func parseTimeFromInterface(timeInterface interface{}) time.Time {
	switch t := timeInterface.(type) {
	case time.Time:
		return t
	case string:
		if parsed, err := time.Parse(time.RFC3339, t); err == nil {
			return parsed
		}
	}
	return time.Now()
}

// Helper function to format JSON response for human-readable display
func formatResponseForDisplay(response, cityName string) string {
	// Handle responses with prefixed tags like [itinerary], [city_data], etc.
	cleanedResponse := response

	// Remove common LLM response prefixes
	prefixPatterns := []string{
		`\[itinerary\]\s*`,
		`\[city_data\]\s*`,
		`\[restaurants\]\s*`,
		`\[hotels\]\s*`,
		`\[activities\]\s*`,
		`\[pois\]\s*`,
		`\[general_pois\]\s*`,
		`\[personalized_pois\]\s*`,
	}

	for _, pattern := range prefixPatterns {
		re := regexp.MustCompile(`(?i)^` + pattern)
		cleanedResponse = re.ReplaceAllString(cleanedResponse, "")
	}

	// Remove markdown code blocks if present
	cleanedResponse = regexp.MustCompile("(?s)```json\\s*(.*)\\s*```").ReplaceAllString(cleanedResponse, "$1")
	cleanedResponse = strings.TrimSpace(cleanedResponse)

	// First, check if cleaned response is valid JSON
	if !json.Valid([]byte(cleanedResponse)) {
		// If not JSON, return as-is (might be already formatted text)
		return response
	}

	// Try to parse as GeneralCityData first (for [city_data] responses)
	var generalCity types.GeneralCityData
	if err := json.Unmarshal([]byte(cleanedResponse), &generalCity); err == nil && generalCity.City != "" {
		return formatCityDataResponse(generalCity)
	}

	// Try to parse as AiCityResponse (most common format)
	var cityResponse types.AiCityResponse
	if err := json.Unmarshal([]byte(cleanedResponse), &cityResponse); err == nil {
		// Check if it's a valid itinerary response (either has POIs or itinerary data)
		if len(cityResponse.PointsOfInterest) > 0 || cityResponse.AIItineraryResponse.ItineraryName != "" || len(cityResponse.AIItineraryResponse.PointsOfInterest) > 0 {
			return formatItineraryResponse(cityResponse, cityName)
		}
	}

	// Try to parse as hotel array
	var hotels []types.HotelDetailedInfo
	if err := json.Unmarshal([]byte(cleanedResponse), &hotels); err == nil && len(hotels) > 0 {
		return formatHotelResponse(hotels, cityName)
	}

	// Try to parse as restaurant array
	var restaurants []types.RestaurantDetailedInfo
	if err := json.Unmarshal([]byte(cleanedResponse), &restaurants); err == nil && len(restaurants) > 0 {
		return formatRestaurantResponse(restaurants, cityName)
	}

	// Try to parse as POI array
	var pois []types.POIDetailedInfo
	if err := json.Unmarshal([]byte(cleanedResponse), &pois); err == nil && len(pois) > 0 {
		return formatPOIResponse(pois, cityName)
	}

	// Try to extract meaningful information from malformed JSON or text
	cleanedLower := strings.ToLower(cleanedResponse)

	// Check if it contains city information
	if strings.Contains(cleanedLower, "city") || strings.Contains(cleanedLower, "country") {
		return fmt.Sprintf("I found information about %s and prepared some details for you!", cityName)
	}

	// Check for content type indicators
	if strings.Contains(cleanedLower, "hotel") || strings.Contains(cleanedLower, "accommodation") {
		return fmt.Sprintf("I found some excellent hotel options in %s for you!", cityName)
	}

	if strings.Contains(cleanedLower, "restaurant") || strings.Contains(cleanedLower, "dining") {
		return fmt.Sprintf("I discovered some amazing restaurants in %s for you!", cityName)
	}

	if strings.Contains(cleanedLower, "poi") || strings.Contains(cleanedLower, "attraction") || strings.Contains(cleanedLower, "point") {
		return fmt.Sprintf("I found some exciting places to visit in %s for you!", cityName)
	}

	if strings.Contains(cleanedLower, "itinerary") || strings.Contains(cleanedLower, "plan") {
		return fmt.Sprintf("I created a personalized travel plan for %s!", cityName)
	}

	// If we can't determine the content type, return a generic message
	return fmt.Sprintf("I provided personalized recommendations for %s. Here are some great options I found for you!", cityName)
}

// Format itinerary response to readable text
func formatItineraryResponse(response types.AiCityResponse, cityName string) string {
	// Determine which POI list to use and total count
	var totalPOIs int
	var firstPOIName string

	// Check both POI arrays and get the total count
	if len(response.PointsOfInterest) > 0 {
		totalPOIs += len(response.PointsOfInterest)
		firstPOIName = getFirstPOIName(response.PointsOfInterest)
	}

	if len(response.AIItineraryResponse.PointsOfInterest) > 0 {
		totalPOIs += len(response.AIItineraryResponse.PointsOfInterest)
		if firstPOIName == "" {
			firstPOIName = getFirstPOIName(response.AIItineraryResponse.PointsOfInterest)
		}
	}

	// If we have an itinerary name, use it
	if response.AIItineraryResponse.ItineraryName != "" {
		if totalPOIs > 0 {
			return fmt.Sprintf("I created a personalized itinerary called '%s' for %s with %d amazing places to visit, including %s and more!",
				response.AIItineraryResponse.ItineraryName,
				cityName,
				totalPOIs,
				firstPOIName)
		} else {
			return fmt.Sprintf("I created a personalized itinerary called '%s' for %s with great recommendations!",
				response.AIItineraryResponse.ItineraryName,
				cityName)
		}
	}

	// Fallback to generic response
	if totalPOIs > 0 {
		return fmt.Sprintf("I found %d great places to visit in %s, including %s. Perfect for your trip!",
			totalPOIs,
			cityName,
			firstPOIName)
	}

	return fmt.Sprintf("I provided personalized recommendations for %s. Here are some great options I found for you!", cityName)
}

// Format hotel response to readable text
func formatHotelResponse(hotels []types.HotelDetailedInfo, cityName string) string {
	if len(hotels) == 0 {
		return fmt.Sprintf("I searched for hotels in %s for you.", cityName)
	}

	return fmt.Sprintf("I found %d excellent hotel%s in %s, including %s and other great options that match your preferences!",
		len(hotels),
		pluralize(len(hotels)),
		cityName,
		hotels[0].Name)
}

// Format restaurant response to readable text
func formatRestaurantResponse(restaurants []types.RestaurantDetailedInfo, cityName string) string {
	if len(restaurants) == 0 {
		return fmt.Sprintf("I searched for restaurants in %s for you.", cityName)
	}

	return fmt.Sprintf("I discovered %d fantastic restaurant%s in %s, starting with %s and many more delicious options!",
		len(restaurants),
		pluralize(len(restaurants)),
		cityName,
		restaurants[0].Name)
}

// Format POI response to readable text
func formatPOIResponse(pois []types.POIDetailedInfo, cityName string) string {
	if len(pois) == 0 {
		return fmt.Sprintf("I searched for activities in %s for you.", cityName)
	}

	return fmt.Sprintf("I found %d exciting place%s to visit in %s, including %s and other amazing spots you'll love!",
		len(pois),
		pluralize(len(pois)),
		cityName,
		pois[0].Name)
}

// Format city data response to readable text
func formatCityDataResponse(cityData types.GeneralCityData) string {
	result := fmt.Sprintf("Let me tell you about %s, %s! ", cityData.City, cityData.Country)

	if cityData.Description != "" {
		result += cityData.Description + " "
	}

	// Add additional details if available
	details := make([]string, 0)

	if cityData.Population != "" {
		details = append(details, fmt.Sprintf("population of %s", cityData.Population))
	}

	if cityData.Weather != "" {
		details = append(details, fmt.Sprintf("weather: %s", cityData.Weather))
	}

	if cityData.Language != "" {
		details = append(details, fmt.Sprintf("language: %s", cityData.Language))
	}

	if len(details) > 0 {
		result += "Key details: " + strings.Join(details, ", ") + ". "
	}

	if cityData.Attractions != "" {
		result += fmt.Sprintf("Notable attractions include: %s. ", cityData.Attractions)
	}

	if cityData.History != "" {
		result += fmt.Sprintf("History: %s", cityData.History)
	}

	return strings.TrimSpace(result)
}

// Helper functions
func getFirstPOIName(pois []types.POIDetailedInfo) string {
	if len(pois) > 0 {
		return pois[0].Name
	}
	return "some amazing attractions"
}

func pluralize(count int) string {
	if count == 1 {
		return ""
	}
	return "s"
}

// UpdateSession updates an existing session
func (r *RepositoryImpl) UpdateSession(ctx context.Context, session types.ChatSession) error {
	query := `
        UPDATE chat_sessions SET current_itinerary = $2, conversation_history = $3, session_context = $4,
                                 updated_at = $5, expires_at = $6, status = $7
        WHERE id = $1
    `
	itineraryJSON, _ := json.Marshal(session.CurrentItinerary)
	historyJSON, _ := json.Marshal(session.ConversationHistory)
	contextJSON, _ := json.Marshal(session.SessionContext)

	_, err := r.pgpool.Exec(ctx, query, session.ID, itineraryJSON, historyJSON, contextJSON,
		session.UpdatedAt, session.ExpiresAt, session.Status)
	if err != nil {
		r.logger.ErrorContext(ctx, "Failed to update session", slog.Any("error", err))
		return fmt.Errorf("failed to update session: %w", err)
	}
	return nil
}

// AddMessageToSession appends a message to the session's conversation history
func (r *RepositoryImpl) AddMessageToSession(ctx context.Context, sessionID uuid.UUID, message types.ConversationMessage) error {
	session, err := r.GetSession(ctx, sessionID)
	if err != nil {
		return err
	}
	session.ConversationHistory = append(session.ConversationHistory, message)
	session.UpdatedAt = time.Now()
	return r.UpdateSession(ctx, *session)
}

func (r *RepositoryImpl) SaveSinglePOI(ctx context.Context, poi types.POIDetailedInfo, userID, cityID, llmInteractionID uuid.UUID) (uuid.UUID, error) {
	ctx, span := otel.Tracer("LlmInteractionRepo").Start(ctx, "SaveSinglePOI", trace.WithAttributes(
		attribute.String("poi.name", poi.Name), /* ... */
	))
	defer span.End()

	// Validate coordinates before attempting to use them.
	if poi.Latitude < -90 || poi.Latitude > 90 || poi.Longitude < -180 || poi.Longitude > 180 {
		// Or if they are exactly 0,0 and that's considered invalid from LLM
		err := fmt.Errorf("invalid coordinates for POI %s: lat %f, lon %f", poi.Name, poi.Latitude, poi.Longitude)
		span.RecordError(err)
		return uuid.Nil, err
	}

	tx, err := r.pgpool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return uuid.Nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// If poi.ID is already set (e.g., from LLM or previous step), use it. Otherwise, generate new.
	recordID := poi.ID
	if recordID == uuid.Nil {
		recordID = uuid.New()
	}

	// Columns: id, user_id, city_id, llm_interaction_id, name, latitude, longitude, location, category, description_poi (10 columns)
	// Values: $1, $2, $3, $4, $5, $6, $7, ST_MakePoint($7, $6), $8, $9 (10 value expressions for 9 placeholders + ST_MakePoint)
	// Corrected: 9 distinct columns from poiData + 1 for location, then id for the record.
	// Order of columns in INSERT INTO: id, user_id, city_id, llm_interaction_id, name, latitude, longitude, location, category, description_poi
	// Placeholders:                $1,    $2,      $3,      $4,                 $5,   $6,       $7,        ST_MakePoint($7,$6), $8, $9
	query := `
        INSERT INTO llm_suggested_pois (
            id, user_id, city_id, llm_interaction_id, name, 
            latitude, longitude, "location", -- Ensure "location" is quoted if it's a reserved keyword or mixed case
            category, description_poi 
            -- Removed distance from INSERT list
        ) VALUES (
            $1, $2, $3, $4, $5, 
            $6, $7, ST_SetSRID(ST_MakePoint($7, $6), 4326), -- Longitude ($7) first, then Latitude ($6)
            $8, $9
        )
        RETURNING id
    `
	// Arguments should be:
	// $1: recordID (for llm_suggested_pois.id)
	// $2: userID
	// $3: cityID
	// $4: llmInteractionID
	// $5: poi.Name
	// $6: poi.Latitude  (for the latitude column)
	// $7: poi.Longitude (for the longitude column AND for ST_MakePoint's X)
	// $8: poi.Category
	// $9: poi.DescriptionPOI

	var returnedID uuid.UUID
	err = tx.QueryRow(ctx, query,
		recordID,         // $1: id
		userID,           // $2: user_id
		cityID,           // $3: city_id
		llmInteractionID, // $4: llm_interaction_id
		poi.Name,         // $5: name
		poi.Latitude,     // $6: latitude column value
		poi.Longitude,    // $7: longitude column value (also used as X in ST_MakePoint)
		// ST_MakePoint will use $7 (poi.Longitude) as X and $6 (poi.Latitude) as Y
		poi.Category,       // $8: category
		poi.DescriptionPOI, // $9: description_poi
	).Scan(&returnedID)

	if err != nil {
		r.logger.ErrorContext(ctx, "Failed to insert llm_suggested_poi", slog.Any("error", err), slog.String("query", query), slog.String("name", poi.Name))
		span.RecordError(err)
		return uuid.Nil, fmt.Errorf("failed to save llm_suggested_poi: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		span.RecordError(err)
		return uuid.Nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	r.logger.Info("LLM Suggested POI saved successfully", slog.String("id", returnedID.String()))
	return returnedID, nil
}

func (r *RepositoryImpl) GetPOIsBySessionSortedByDistance(ctx context.Context, sessionID, cityID uuid.UUID, userLocation types.UserLocation) ([]types.POIDetailedInfo, error) {

	query := `
        SELECT id, name, latitude, longitude, category, description_poi, 
               ST_Distance(
                   ST_SetSRID(ST_MakePoint($2, $3), 4326)::geography,
                   location::geography  -- Use the actual geometry column for distance
               ) AS distance
        FROM llm_suggested_pois  -- Assuming this is the correct table to query for session POIs
        WHERE city_id = $1 
        -- Add AND llm_interaction_id IN (SELECT ...) if POIs are tied to specific interactions of the session
        ORDER BY distance ASC;
    `
	rows, err := r.pgpool.Query(ctx, query, cityID, userLocation.UserLon, userLocation.UserLat)
	if err != nil {
		return nil, fmt.Errorf("failed to query POIs for session: %w", err)
	}
	defer rows.Close()

	var pois []types.POIDetailedInfo
	for rows.Next() {
		var p types.POIDetailedInfo
		var lat, lon, dist sql.NullFloat64 // Use nullable types
		var cat, desc sql.NullString

		// Adjust scan to match selected columns and their nullability
		err := rows.Scan(&p.ID, &p.Name, &lat, &lon, &cat, &desc, &dist)
		if err != nil {
			return nil, fmt.Errorf("failed to scan POI for session: %w", err)
		}

		if lat.Valid {
			p.Latitude = lat.Float64
		}
		if lon.Valid {
			p.Longitude = lon.Float64
		}
		if cat.Valid {
			p.Category = cat.String
		}
		if desc.Valid {
			p.DescriptionPOI = desc.String
		}
		if dist.Valid {
			p.Distance = dist.Float64
		}

		pois = append(pois, p)
	}
	return pois, rows.Err()
}

// type POIDetailedInfo struct {
// 	Name        string  `json:"name"`
// 	Latitude    float64 `json:"latitude"`
// 	Longitude   float64 `json:"longitude"`
// 	Category    string  `json:"category"`
// 	Description string  `json:"description"`
// }

// type LlmApiResponseData struct {
// 	GeneralCityData struct {
// 		City            string  `json:"city"`
// 		Country         string  `json:"country"`
// 		Description     string  `json:"description"`
// 		CenterLatitude  float64 `json:"center_latitude"`
// 		CenterLongitude float64 `json:"center_longitude"`
// 		// Add other fields from general_city_data if you need them
// 		// Population       string  `json:"population,omitempty"`
// 		// Area             string  `json:"area,omitempty"`
// 		// Timezone         string  `json:"timezone,omitempty"`
// 		// Language         string  `json:"language,omitempty"`
// 		// Weather          string  `json:"weather,omitempty"`
// 		// Attractions      string  `json:"attractions,omitempty"`
// 		// History          string  `json:"history,omitempty"`
// 	} `json:"general_city_data"`

// 	PointsOfInterest []types.POIDetailedInfo `json:"points_of_interest"` // <--- ADD THIS FIELD for general POIs

// 	ItineraryResponse struct {
// 		ItineraryName      string            `json:"itinerary_name"`
// 		OverallDescription string            `json:"overall_description"`
// 		PointsOfInterest   []types.POIDetailedInfo `json:"points_of_interest"` // This is for itinerary_response.points_of_interest
// 	} `json:"itinerary_response"`
// }

// type LlmApiResponse struct {
// 	SessionID string             `json:"session_id"` // Capture the top-level session_id
// 	Data      LlmApiResponseData `json:"data"`
// 	// Note: The JSON also has a "session_id" inside "data".
// 	// If you need that too, you'd add it to LlmApiResponseData:
// 	// SessionIDInsideData string `json:"session_id,omitempty"`
// }

func parsePOIsFromResponse(responseText string, logger *slog.Logger) ([]types.POIDetailedInfo, error) {
	cleanedResponse := cleanJSONResponse(responseText)

	// Debug logging to see the actual cleaned response
	logger.Debug("parsePOIsFromResponse: Cleaned response debug",
		"originalLength", len(responseText),
		"cleanedLength", len(cleanedResponse),
		"cleanedPreview", cleanedResponse[:min(500, len(cleanedResponse))],
		"cleanedSuffix", func() string {
			start := len(cleanedResponse) - 200
			if start < 0 {
				start = 0
			}
			return cleanedResponse[start:]
		}())

	// Check if this looks like an itinerary response instead of a POI response
	if strings.Contains(cleanedResponse, "itinerary_name") {
		logger.Debug("parsePOIsFromResponse: Response appears to be an itinerary, not POI data")
		return []types.POIDetailedInfo{}, nil
	}

	// First try to parse as unified chat response format with "data" wrapper
	var unifiedResponse struct {
		Data types.AiCityResponse `json:"data"`
	}
	err := json.Unmarshal([]byte(cleanedResponse), &unifiedResponse)
	if err == nil {
		// Collect POIs from both general points_of_interest and itinerary points_of_interest
		var allPOIs []types.POIDetailedInfo
		if unifiedResponse.Data.PointsOfInterest != nil {
			allPOIs = append(allPOIs, unifiedResponse.Data.PointsOfInterest...)
		}
		if unifiedResponse.Data.AIItineraryResponse.PointsOfInterest != nil {
			allPOIs = append(allPOIs, unifiedResponse.Data.AIItineraryResponse.PointsOfInterest...)
		}
		if len(allPOIs) > 0 {
			logger.Debug("parsePOIsFromResponse: Parsed as unified chat response", "poiCount", len(allPOIs))
			return allPOIs, nil
		}
	} else if err != nil {
		logger.Debug("parsePOIsFromResponse: Failed to parse as unified response", "error", err.Error())
	}

	// Second, try to parse as a full AiCityResponse (for legacy responses)
	var parsedResponse types.AiCityResponse
	err = json.Unmarshal([]byte(cleanedResponse), &parsedResponse)
	if err == nil && parsedResponse.PointsOfInterest != nil {
		logger.Debug("parsePOIsFromResponse: Parsed as AiCityResponse", "poiCount", len(parsedResponse.PointsOfInterest))
		return parsedResponse.PointsOfInterest, nil
	} else if err != nil {
		logger.Debug("parsePOIsFromResponse: Failed to parse as AiCityResponse", "error", err.Error())
	}

	// Third, try to parse as a single POI (for individual POI additions)
	var singlePOI types.POIDetailedInfo
	err = json.Unmarshal([]byte(cleanedResponse), &singlePOI)
	if err == nil && singlePOI.Name != "" {
		logger.Debug("parsePOIsFromResponse: Parsed as single POI", "poiName", singlePOI.Name)
		return []types.POIDetailedInfo{singlePOI}, nil
	}

	// If all fail, log the error and return empty
	logger.Warn("parsePOIsFromResponse: Could not parse response as unified chat, AiCityResponse, or single POI",
		"error", err,
		"cleanedResponseLength", len(cleanedResponse),
		"responsePreview", cleanedResponse[:min(200, len(cleanedResponse))])
	return []types.POIDetailedInfo{}, nil
}

func (r *RepositoryImpl) GetOrCreatePOI(ctx context.Context, tx pgx.Tx, POIDetailedInfo types.POIDetailedInfo, cityID uuid.UUID, sourceInteractionID uuid.UUID) (uuid.UUID, error) {
	var poiDBID uuid.UUID
	findPoiQuery := `SELECT id FROM points_of_interest WHERE name = $1 AND city_id = $2 LIMIT 1`
	err := tx.QueryRow(ctx, findPoiQuery, POIDetailedInfo.Name, cityID).Scan(&poiDBID)

	if err == pgx.ErrNoRows {
		createPoiQuery := `
            INSERT INTO points_of_interest (name, city_id, location, category, description)
            VALUES ($1, $2, ST_SetSRID(ST_MakePoint($3, $4), 4326), $5, $6) RETURNING id`
		err = tx.QueryRow(ctx, createPoiQuery,
			POIDetailedInfo.Name,
			cityID,
			POIDetailedInfo.Latitude,
			POIDetailedInfo.Longitude,
			POIDetailedInfo.Category,
			POIDetailedInfo.DescriptionPOI, // Assumes types.POIDetailedInfo has DescriptionPOI from JSON
		).Scan(&poiDBID)
		if err != nil {
			r.logger.ErrorContext(ctx, "GetOrCreatePOI: Failed to insert new POI", "error", err, "poi_name", POIDetailedInfo.Name)
			return uuid.Nil, fmt.Errorf("GetOrCreatePOI: failed to insert new POI '%s': %w", POIDetailedInfo.Name, err)
		}
	} else if err != nil {
		r.logger.ErrorContext(ctx, "GetOrCreatePOI: Failed to query existing POI", "error", err, "poi_name", POIDetailedInfo.Name)
		return uuid.Nil, fmt.Errorf("GetOrCreatePOI: failed to query existing POI '%s': %w", POIDetailedInfo.Name, err)
	}
	return poiDBID, nil
}

func (r *RepositoryImpl) SaveItineraryPOIs(ctx context.Context, itineraryID uuid.UUID, pois []types.POIDetailedInfo) error {
	batch := &pgx.Batch{}
	for i, poi := range pois {
		query := `
            INSERT INTO itinerary_pois (itinerary_id, poi_id, order_index, ai_description)
            VALUES ($1, $2, $3, $4)
            ON CONFLICT (itinerary_id, poi_id) DO UPDATE SET
                order_index = EXCLUDED.order_index,
                ai_description = EXCLUDED.ai_description,
                updated_at = NOW()
        `
		batch.Queue(query, itineraryID, poi.ID, i, poi.DescriptionPOI)
	}

	br := r.pgpool.SendBatch(ctx, batch)
	defer br.Close()

	for i := 0; i < len(pois); i++ {
		_, err := br.Exec()
		if err != nil {
			return fmt.Errorf("failed to execute batch insert for itinerary_poi %d: %w", i, err)
		}
	}

	return nil
}

// func (r *RepositoryImpl) SaveInteractionWithEmbedding(ctx context.Context, interaction types.LlmInteraction, embedding []float32) (uuid.UUID, error) {
// 	ctx, span := otel.Tracer("LlmInteractionRepo").Start(ctx, "SaveInteractionWithEmbedding", trace.WithAttributes(
// 		semconv.DBSystemPostgreSQL,
// 		attribute.String("db.operation", "INSERT_COMPLEX"),
// 		attribute.String("db.sql.table", "llm_interactions,itineraries,itinerary_pois"),
// 		attribute.String("user.id", interaction.UserID.String()),
// 		attribute.String("model.used", interaction.ModelUsed),
// 		attribute.Int("latency.ms", interaction.LatencyMs),
// 		attribute.String("city.name", interaction.CityName),
// 	))
// 	defer span.End()

// 	var err error
// 	tx, err := r.pgpool.BeginTx(ctx, pgx.TxOptions{})
// 	if err != nil {
// 		span.RecordError(err)
// 		span.SetStatus(codes.Error, "Failed to start transaction")
// 		return uuid.Nil, fmt.Errorf("failed to start transaction: %w", err)
// 	}
// 	defer func() {
// 		if p := recover(); p != nil {
// 			_ = tx.Rollback(ctx)
// 			panic(p)
// 		}
// 		if err != nil {
// 			if rbErr := tx.Rollback(ctx); rbErr != nil {
// 				r.logger.ErrorContext(ctx, "Transaction rollback failed", slog.Any("error", rbErr))
// 			}
// 		}
// 	}()

// 	// Convert embedding to pgvector format
// 	vectorParam := pgvector.NewVector(embedding)

// 	interactionQuery := `
//         INSERT INTO llm_interactions (
//             user_id, prompt, response, model_name, latency_ms, city_name, prompt_embedding
//         ) VALUES ($1, $2, $3, $4, $5, $6, $7)
//         RETURNING id
//     `
// 	var interactionID uuid.UUID
// 	err = tx.QueryRow(ctx, interactionQuery,
// 		interaction.UserID,
// 		interaction.Prompt,
// 		interaction.ResponseText,
// 		interaction.ModelUsed,
// 		interaction.LatencyMs,
// 		interaction.CityName,
// 		vectorParam,
// 	).Scan(&interactionID)
// 	if err != nil {
// 		span.RecordError(err)
// 		span.SetStatus(codes.Error, "Failed to insert llm_interaction")
// 		return uuid.Nil, fmt.Errorf("failed to insert llm_interaction: %w", err)
// 	}
// 	span.SetAttributes(attribute.String("llm_interaction.id", interactionID.String()))

// 	// Existing itinerary and POI logic remains unchanged
// 	var cityID uuid.UUID
// 	if interaction.CityName != "" {
// 		cityQuery := `SELECT id FROM cities WHERE name = $1 LIMIT 1`
// 		err = tx.QueryRow(ctx, cityQuery, interaction.CityName).Scan(&cityID)
// 		if err != nil && err != pgx.ErrNoRows {
// 			span.RecordError(err)
// 			return interactionID, fmt.Errorf("failed to get city_id: %w", err)
// 		}
// 	}

// 	var itineraryID uuid.UUID
// 	if cityID != uuid.Nil {
// 		itineraryQuery := `
//             INSERT INTO itineraries (user_id, city_id, source_llm_interaction_id)
//             VALUES ($1, $2, $3)
//             ON CONFLICT (user_id, city_id) DO UPDATE SET
//                 updated_at = NOW(),
//                 source_llm_interaction_id = EXCLUDED.source_llm_interaction_id
//             RETURNING id
//         `
// 		err = tx.QueryRow(ctx, itineraryQuery, interaction.UserID, cityID, interactionID).Scan(&itineraryID)
// 		if err != nil {
// 			span.RecordError(err)
// 			return interactionID, fmt.Errorf("failed to insert itinerary: %w", err)
// 		}
// 	}

// 	if itineraryID != uuid.Nil {
// 		var pois []types.POIDetailedInfo
// 		pois, err = parsePOIsFromResponse(interaction.ResponseText, r.logger)
// 		if err != nil {
// 			span.RecordError(err)
// 			return interactionID, fmt.Errorf("failed to parse POIs: %w", err)
// 		}

// 		if len(pois) > 0 {
// 			poiBatch := &pgx.Batch{}
// 			itineraryPoiInsertQuery := `
//                 INSERT INTO itinerary_pois (itinerary_id, poi_id, order_index, ai_description)
//                 VALUES ($1, $2, $3, $4)
//                 ON CONFLICT (itinerary_id, poi_id) DO UPDATE SET
//                     order_index = EXCLUDED.order_index,
//                     ai_description = EXCLUDED.ai_description,
//                     updated_at = NOW()
//             `
// 			for i, POIDetailedInfo := range pois {
// 				var poiDBID uuid.UUID
// 				poiDBID, err = r.GetOrCreatePOI(ctx, tx, POIDetailedInfo, cityID, interactionID)
// 				if err != nil {
// 					span.RecordError(err)
// 					return interactionID, fmt.Errorf("failed to get or create POI: %w", err)
// 				}
// 				poiBatch.Queue(itineraryPoiInsertQuery, itineraryID, poiDBID, i, POIDetailedInfo.DescriptionPOI)
// 			}

// 			if poiBatch.Len() > 0 {
// 				br := tx.SendBatch(ctx, poiBatch)
// 				for i := 0; i < poiBatch.Len(); i++ {
// 					_, execErr := br.Exec()
// 					if execErr != nil {
// 						err = fmt.Errorf("failed to insert itinerary_poi: %w", execErr)
// 						br.Close()
// 						return interactionID, err
// 					}
// 				}
// 				br.Close()
// 			}
// 		}
// 	}

// 	err = tx.Commit(ctx)
// 	if err != nil {
// 		span.RecordError(err)
// 		return uuid.Nil, fmt.Errorf("failed to commit transaction: %w", err)
// 	}

// 	span.SetStatus(codes.Ok, "Interaction saved successfully")
// 	return interactionID, nil
// }

// ContentCounts represents counts of different content types found in responses
type ContentCounts struct {
	POIs         int
	Hotels       int
	Restaurants  int
	HasItinerary bool
	Categories   []string
}

// countContentFromResponse analyzes a response to count different content types
func countContentFromResponse(response string) ContentCounts {
	counts := ContentCounts{
		Categories: make([]string, 0),
	}

	// Try to parse as JSON first
	var jsonData map[string]interface{}
	if err := json.Unmarshal([]byte(response), &jsonData); err == nil {
		// Handle JSON response
		if pois, ok := jsonData["points_of_interest"].([]interface{}); ok {
			counts.POIs = len(pois)
			counts.Categories = append(counts.Categories, "attractions")
		}
		if hotels, ok := jsonData["hotels"].([]interface{}); ok {
			counts.Hotels = len(hotels)
			counts.Categories = append(counts.Categories, "accommodation")
		}
		if restaurants, ok := jsonData["restaurants"].([]interface{}); ok {
			counts.Restaurants = len(restaurants)
			counts.Categories = append(counts.Categories, "dining")
		}
		if _, ok := jsonData["itinerary_response"]; ok {
			counts.HasItinerary = true
			counts.Categories = append(counts.Categories, "itinerary")
		}
		if _, ok := jsonData["itinerary_name"]; ok {
			counts.HasItinerary = true
			counts.Categories = append(counts.Categories, "itinerary")
		}
	} else {
		// Handle text response with pattern matching
		lowerResponse := strings.ToLower(response)

		// Count mentions of different content types
		if strings.Contains(lowerResponse, "hotel") || strings.Contains(lowerResponse, "accommodation") {
			counts.Hotels = 1
			counts.Categories = append(counts.Categories, "accommodation")
		}
		if strings.Contains(lowerResponse, "restaurant") || strings.Contains(lowerResponse, "dining") {
			counts.Restaurants = 1
			counts.Categories = append(counts.Categories, "dining")
		}
		if strings.Contains(lowerResponse, "attraction") || strings.Contains(lowerResponse, "visit") || strings.Contains(lowerResponse, "see") {
			counts.POIs = 1
			counts.Categories = append(counts.Categories, "attractions")
		}
		if strings.Contains(lowerResponse, "itinerary") || strings.Contains(lowerResponse, "plan") || strings.Contains(lowerResponse, "schedule") {
			counts.HasItinerary = true
			counts.Categories = append(counts.Categories, "itinerary")
		}
	}

	return counts
}

// calculateComplexityScore calculates a complexity score from 1-10 based on session content
func calculateComplexityScore(pois, hotels, restaurants, messageCount int, hasItinerary bool) int {
	score := 1

	// Base score from content count
	totalContent := pois + hotels + restaurants
	if totalContent > 20 {
		score += 3
	} else if totalContent > 10 {
		score += 2
	} else if totalContent > 5 {
		score += 1
	}

	// Bonus for having itinerary
	if hasItinerary {
		score += 2
	}

	// Bonus for message count (engagement)
	if messageCount > 20 {
		score += 2
	} else if messageCount > 10 {
		score += 1
	}

	// Bonus for content diversity
	contentTypes := 0
	if pois > 0 {
		contentTypes++
	}
	if hotels > 0 {
		contentTypes++
	}
	if restaurants > 0 {
		contentTypes++
	}
	if contentTypes >= 3 {
		score += 2
	} else if contentTypes >= 2 {
		score += 1
	}

	// Cap at 10
	if score > 10 {
		score = 10
	}

	return score
}

// countMessagesByRole counts messages by user and assistant roles
func countMessagesByRole(messages []types.ConversationMessage) (userCount, assistantCount int) {
	for _, msg := range messages {
		if msg.Role == "user" {
			userCount++
		} else if msg.Role == "assistant" {
			assistantCount++
		}
	}
	return
}

// calculateAverageMessageLength calculates the average length of all messages
func calculateAverageMessageLength(messages []types.ConversationMessage) int {
	if len(messages) == 0 {
		return 0
	}

	totalLength := 0
	for _, msg := range messages {
		totalLength += len(msg.Content)
	}

	return totalLength / len(messages)
}

// calculateEngagementLevel determines engagement level based on metrics
func calculateEngagementLevel(messageCount int, duration time.Duration, complexityScore int) string {
	score := 0

	// Message count factor
	if messageCount > 15 {
		score += 3
	} else if messageCount > 8 {
		score += 2
	} else if messageCount > 3 {
		score += 1
	}

	// Duration factor (more than 10 minutes indicates engagement)
	if duration > 30*time.Minute {
		score += 3
	} else if duration > 10*time.Minute {
		score += 2
	} else if duration > 2*time.Minute {
		score += 1
	}

	// Complexity factor
	if complexityScore >= 8 {
		score += 2
	} else if complexityScore >= 5 {
		score += 1
	}

	// Determine level
	if score >= 6 {
		return "high"
	} else if score >= 3 {
		return "medium"
	}
	return "low"
}

// uniqueStringSlice removes duplicates from a string slice
func uniqueStringSlice(slice []string) []string {
	unique := make(map[string]bool)
	result := make([]string, 0)

	for _, item := range slice {
		if !unique[item] && item != "" {
			unique[item] = true
			result = append(result, item)
		}
	}

	return result
}
