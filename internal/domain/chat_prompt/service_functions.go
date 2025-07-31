package chat_prompt

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	c "github.com/FACorreiaa/go-poi-au-suggestions/internal/domain/city"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

func (l *Service) SaveItenerary(ctx context.Context, userID uuid.UUID, req BookmarkRequest) (uuid.UUID, error) {
	var llmInteractionIDStr string
	if req.LlmInteractionID != nil {
		llmInteractionIDStr = req.LlmInteractionID.String()
	} else {
		llmInteractionIDStr = "nil"
	}

	ctx, span := otel.Tracer("LlmInteractionService").Start(ctx, "SaveItenerary", trace.WithAttributes(
		attribute.String("user.id", userID.String()),
		attribute.String("llm_interaction.id", llmInteractionIDStr),
		attribute.String("title", req.Title),
	))
	defer span.End()

	l.logger.Info("Attempting to bookmark interaction",
		zap.String("userID", userID.String()),
		zap.String("llmInteractionID", llmInteractionIDStr),
		zap.String("title", req.Title))

	var sourceInteractionID pgtype.UUID
	if req.LlmInteractionID != nil {
		sourceInteractionID = pgtype.UUID{
			Bytes: *req.LlmInteractionID,
			Valid: true,
		}
		l.logger.Info("Using provided LlmInteractionID for bookmark",
			zap.String("llmInteractionID", req.LlmInteractionID.String()))
	} else if req.SessionID != nil {
		latestInteraction, err := l.repo.GetLatestInteractionBySessionID(ctx, *req.SessionID)
		if err != nil || latestInteraction == nil {
			l.logger.Info("No interaction found for session, storing session ID without interaction reference",
				zap.String("sessionID", req.SessionID.String()),
				zap.Error(err))
			sourceInteractionID = pgtype.UUID{Valid: false}
		} else {
			sourceInteractionID = pgtype.UUID{
				Bytes: latestInteraction.ID,
				Valid: true,
			}
			l.logger.Info("Found latest interaction for session",
				zap.String("sessionID", req.SessionID.String()),
				zap.String("interactionID", latestInteraction.ID.String()))
		}
	} else {
		sourceInteractionID = pgtype.UUID{Valid: false}
		l.logger.Info("No LlmInteractionID or SessionID provided, bookmark will have no source reference")
	}

	var primaryCityID pgtype.UUID

	if req.PrimaryCityID != nil {
		primaryCityID = pgtype.UUID{
			Bytes: *req.PrimaryCityID,
			Valid: true,
		}
	} else if req.PrimaryCityName != "" {
		city, err := l.cityRepo.FindCityByNameAndCountry(ctx, req.PrimaryCityName, "")
		if err != nil {
			l.logger.Error("Failed to find city", zap.Error(err))
			span.RecordError(err)
			return uuid.Nil, fmt.Errorf("failed to find city: %w", err)
		}

		if city == nil {
			cityDetail := c.CityDetail{
				Name:      req.PrimaryCityName,
				Country:   "Unknown",
				AiSummary: "",
			}
			cityID, err := l.cityRepo.SaveCity(ctx, cityDetail)
			if err != nil {
				l.logger.Error("Failed to save city", zap.Error(err))
				span.RecordError(err)
				return uuid.Nil, fmt.Errorf("failed to save city: %w", err)
			}
			primaryCityID = pgtype.UUID{
				Bytes: cityID,
				Valid: true,
			}
			l.logger.Info("Created new city", zap.String("cityName", req.PrimaryCityName), zap.String("cityID", cityID.String()))
		} else {
			primaryCityID = pgtype.UUID{
				Bytes: city.ID,
				Valid: true,
			}
			l.logger.Info("Found existing city", zap.String("cityName", req.PrimaryCityName), zap.String("cityID", city.ID.String()))
		}
	} else {
		primaryCityID = pgtype.UUID{Valid: false}
	}

	var originalInteraction *LlmInteraction
	var err error
	if req.LlmInteractionID != nil {
		originalInteraction, err = l.repo.GetInteractionByID(ctx, *req.LlmInteractionID)
		if err != nil || originalInteraction == nil {
			l.logger.Error("Failed to fetch original LLM interaction", zap.Error(err))
			span.RecordError(err)
			return uuid.Nil, fmt.Errorf("could not retrieve original interaction: %w", err)
		}
	}

	var markdownContent string
	if originalInteraction != nil {
		markdownContent = originalInteraction.ResponseText
	} else {
		if req.Description != nil {
			markdownContent = *req.Description
		} else {
			markdownContent = ""
		}
	}

	var description sql.NullString
	if req.Description != nil {
		description.String = *req.Description
		description.Valid = true
	}
	isPublic := false
	if req.IsPublic != nil {
		isPublic = *req.IsPublic
	}

	var sessionID pgtype.UUID
	if req.SessionID != nil {
		sessionID = pgtype.UUID{
			Bytes: *req.SessionID,
			Valid: true,
		}
	} else {
		sessionID = pgtype.UUID{Valid: false}
	}

	newBookmark := &UserSavedItinerary{
		UserID:                 userID,
		SourceLlmInteractionID: sourceInteractionID,
		SessionID:              sessionID,
		PrimaryCityID:          primaryCityID,
		Title:                  req.Title,
		Description:            description,
		MarkdownContent:        markdownContent,
		Tags:                   req.Tags,
		IsPublic:               isPublic,
	}
	savedID, err := l.repo.AddChatToBookmark(ctx, newBookmark)
	if err != nil {
		span.RecordError(err)
		return uuid.Nil, err
	}

	l.logger.Info("Successfully saved bookmark to user_saved_itineraries",
		zap.String("savedID", savedID.String()),
		zap.String("title", req.Title))

	span.SetAttributes(attribute.String("saved_itinerary.id", savedID.String()))
	span.SetStatus(codes.Ok, "Bookmark saved successfully")
	return savedID, nil
}

func (l *Service) RemoveItenerary(ctx context.Context, userID, itineraryID uuid.UUID) error {
	ctx, span := otel.Tracer("LlmInteractionService").Start(ctx, "RemoveItenerary", trace.WithAttributes(
		attribute.String("user.id", userID.String()),
		attribute.String("itinerary.id", itineraryID.String()),
	))
	defer span.End()

	l.logger.Info("Attempting to remove chat from bookmark",
		zap.String("itineraryID", itineraryID.String()))

	if err := l.repo.RemoveChatFromBookmark(ctx, userID, itineraryID); err != nil {
		l.logger.Error("Failed to remove chat from bookmark", zap.Error(err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to remove chat from bookmark")
		return fmt.Errorf("failed to remove chat from bookmark: %w", err)
	}

	l.logger.Info("Successfully removed chat from bookmark", zap.String("itineraryID", itineraryID.String()))
	span.SetStatus(codes.Ok, "Itinerary removed successfully")
	return nil
}

func (l *Service) saveCityInteraction(ctx context.Context, interaction LlmInteraction) (uuid.UUID, error) {
	ctx, span := otel.Tracer("LlmInteractionService").Start(ctx, "saveCityInteraction")
	defer span.End()

	if interaction.LatencyMs == 0 {
		interaction.LatencyMs = int(time.Since(interaction.Timestamp).Milliseconds())
	}
	if interaction.ModelUsed == "" {
		interaction.ModelUsed = model
	}

	interactionID, err := l.repo.SaveInteraction(ctx, interaction)
	if err != nil {
		span.RecordError(err)
		l.logger.Warn("Failed to save LLM interaction", zap.Error(err))
		return uuid.Nil, fmt.Errorf("failed to save interaction: %w", err)
	}

	span.SetAttributes(attribute.String("interaction.id", interactionID.String()))
	return interactionID, nil
}
