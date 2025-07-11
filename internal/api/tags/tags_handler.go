package tags

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"

	"github.com/FACorreiaa/go-poi-au-suggestions/internal/api"
	"github.com/FACorreiaa/go-poi-au-suggestions/internal/api/auth"
	"github.com/FACorreiaa/go-poi-au-suggestions/internal/types"
)

var _ Handler = (*HandlerImpl)(nil)

type Handler interface {
	GetTags(w http.ResponseWriter, r *http.Request)
	GetTag(w http.ResponseWriter, r *http.Request)
	CreateTag(w http.ResponseWriter, r *http.Request)
	DeleteTag(w http.ResponseWriter, r *http.Request)
	UpdateTag(w http.ResponseWriter, r *http.Request)
}
type HandlerImpl struct {
	tagsService tagsService
	logger      *slog.Logger
}

// NewHandlerImpl creates a new user HandlerImpl instance.
func NewHandlerImpl(userService tagsService, logger *slog.Logger) *HandlerImpl {
	instanceAddress := fmt.Sprintf("%p", logger)
	slog.Info("Creating NewUserHandlerImpl", slog.String("logger_address", instanceAddress), slog.Bool("logger_is_nil", logger == nil))
	if logger == nil {
		panic("PANIC: Attempting to create UserHandlerImpl with nil logger!")
	}

	return &HandlerImpl{
		tagsService: userService,
		logger:      logger,
	}
}

// GetTags godoc
// @Summary      Get All User Tags
// @Description  Retrieves all tags created by the authenticated user
// @Tags         User Tags
// @Accept       json
// @Produce      json
// @Success      200 {array} types.Tags "User Tags"
// @Failure      401 {object} types.Response "Unauthorized"
// @Failure      500 {object} types.Response "Internal Server Error"
// @Security     BearerAuth
// @Router       /user/tags [get].
func (u *HandlerImpl) GetTags(w http.ResponseWriter, r *http.Request) {
	ctx, span := otel.Tracer("GetTagsHandlerImpl").Start(r.Context(), "GetTags", trace.WithAttributes(
		semconv.HTTPRequestMethodKey.String(r.Method),
		semconv.HTTPRouteKey.String("/user/tags"),
	))
	defer span.End()

	l := u.logger.With(slog.String("HandlerImpl", "GetTags"))
	l.DebugContext(ctx, "Fetching user tags")

	userIDStr, ok := auth.GetUserIDFromContext(ctx)
	if !ok || userIDStr == "" {
		l.WarnContext(ctx, "User ID not found in context")
		span.SetStatus(codes.Error, "Authentication required")
		api.ErrorResponse(w, r, http.StatusUnauthorized, "Authentication required")
		return
	}

	userID, err := uuid.Parse(userIDStr)
	if err != nil { // Should ideally not happen if JWT sub is always valid UUID
		l.ErrorContext(ctx, "Invalid user ID format in context", slog.String("user_id_str", userIDStr), slog.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Invalid User ID in token")
		api.ErrorResponse(w, r, http.StatusInternalServerError, "Invalid user session")
		return
	}

	tags, err := u.tagsService.GetTags(ctx, userID)
	if err != nil {
		l.ErrorContext(ctx, "Failed to get user tags", slog.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to get user tags")
		api.ErrorResponse(w, r, http.StatusInternalServerError, "Failed to retrieve user tags")
		return
	}
	span.SetStatus(codes.Ok, "Tags retrieved successfully")
	api.WriteJSONResponse(w, r, http.StatusOK, tags)
}

// GetTag godoc
// @Summary      Get Specific User Tag
// @Description  Retrieves a specific tag by ID for the authenticated user
// @Tags         User Tags
// @Accept       json
// @Produce      json
// @Param        tagID path string true "Tag ID"
// @Success      200 {object} types.Tags "User Tag"
// @Failure      400 {object} types.Response "Invalid Input"
// @Failure      401 {object} types.Response "Unauthorized"
// @Failure      500 {object} types.Response "Internal Server Error"
// @Security     BearerAuth
// @Router       /user/tags/{tagID} [get]
func (u *HandlerImpl) GetTag(w http.ResponseWriter, r *http.Request) {
	ctx, span := otel.Tracer("GetTagHandlerImpl").Start(r.Context(), "GetTag", trace.WithAttributes(
		semconv.HTTPRequestMethodKey.String(r.Method),
		semconv.HTTPRouteKey.String("/user/tags"),
	))
	defer span.End()

	l := u.logger.With(slog.String("HandlerImpl", "GetTag"))
	l.DebugContext(ctx, "Fetching user tag")

	userIDStr, ok := auth.GetUserIDFromContext(ctx)
	if !ok || userIDStr == "" {
		l.ErrorContext(ctx, "User ID not found in context")
		span.SetStatus(codes.Error, "User ID not found in context")
		api.ErrorResponse(w, r, http.StatusUnauthorized, "Authentication required")
		return
	}

	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		l.ErrorContext(ctx, "Invalid user ID format", slog.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Invalid user ID format")
		api.ErrorResponse(w, r, http.StatusBadRequest, "Invalid user ID format")
		return
	}

	tagIDStr := chi.URLParam(r, "tagID")
	tagID, err := uuid.Parse(tagIDStr)
	if err != nil {
		l.WarnContext(ctx, "Invalid tag ID format", slog.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Invalid tag ID format")
		api.ErrorResponse(w, r, http.StatusBadRequest, "Invalid tag ID format")
		return
	}

	tag, err := u.tagsService.GetTag(ctx, userID, tagID)
	if err != nil {
		l.ErrorContext(ctx, "Failed to get user tag", slog.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to get user tag")
		api.ErrorResponse(w, r, http.StatusInternalServerError, "Failed to retrieve user tag")
		return
	}
	span.SetStatus(codes.Ok, "Tag retrieved successfully")
	api.WriteJSONResponse(w, r, http.StatusOK, tag)
}

// CreateTag godoc
// @Summary      Create User Tag
// @Description  Creates a new personal tag for the authenticated user
// @Tags         User Tags
// @Accept       json
// @Produce      json
// @Param        tag body types.CreatePersonalTagParams true "Tag creation details"
// @Success      200 {object} types.Tags "Created User Tag"
// @Failure      400 {object} types.Response "Invalid Input"
// @Failure      401 {object} types.Response "Unauthorized"
// @Failure      500 {object} types.Response "Internal Server Error"
// @Security     BearerAuth
// @Router       /user/tags [post]
func (u *HandlerImpl) CreateTag(w http.ResponseWriter, r *http.Request) {
	ctx, span := otel.Tracer("CreateTagHandlerImpl").Start(r.Context(), "CreateTag", trace.WithAttributes(
		semconv.HTTPRequestMethodKey.String(r.Method),
		semconv.HTTPRouteKey.String("/user/tags"),
	))
	defer span.End()

	l := u.logger.With(slog.String("HandlerImpl", "CreateTag"))
	l.DebugContext(ctx, "Creating user tag")

	userIDStr, ok := auth.GetUserIDFromContext(ctx)
	if !ok || userIDStr == "" {
		l.ErrorContext(ctx, "User ID not found in context")
		span.SetStatus(codes.Error, "User ID not found in context")
		api.ErrorResponse(w, r, http.StatusUnauthorized, "Authentication required")
		return
	}

	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		l.ErrorContext(ctx, "Invalid user ID format", slog.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Invalid user ID format")
		api.ErrorResponse(w, r, http.StatusBadRequest, "Invalid user ID format")
		return
	}

	var req types.CreatePersonalTagParams
	if err := api.DecodeJSONBody(w, r, &req); err != nil {
		l.WarnContext(ctx, "Failed to decode request", slog.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to decode request")
		api.ErrorResponse(w, r, http.StatusBadRequest, "Invalid request format")
		return
	}

	tag, err := u.tagsService.CreateTag(ctx, userID, req)
	if err != nil {
		l.ErrorContext(ctx, "Failed to create user tag", slog.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to create user tag")
		api.ErrorResponse(w, r, http.StatusInternalServerError, "Failed to create user tag")
		return
	}
	span.SetStatus(codes.Ok, "Tag created successfully")
	api.WriteJSONResponse(w, r, http.StatusOK, tag)
}

// DeleteTag godoc
// @Summary      Delete User Tag
// @Description  Deletes a specific tag for the authenticated user
// @Tags         User Tags
// @Accept       json
// @Produce      json
// @Param        tagID path string true "Tag ID to delete"
// @Success      200 "Tag deleted successfully"
// @Failure      400 {object} types.Response "Invalid Input"
// @Failure      401 {object} types.Response "Unauthorized"
// @Failure      404 {object} types.Response "Tag not found"
// @Failure      500 {object} types.Response "Internal Server Error"
// @Security     BearerAuth
// @Router       /user/tags/{tagID} [delete]
func (u *HandlerImpl) DeleteTag(w http.ResponseWriter, r *http.Request) {
	ctx, span := otel.Tracer("DeleteTagHandlerImpl").Start(r.Context(), "DeleteTag", trace.WithAttributes(
		semconv.HTTPRequestMethodKey.String(r.Method),
		semconv.HTTPRouteKey.String("/user/tags"),
	))
	defer span.End()

	l := u.logger.With(slog.String("HandlerImpl", "DeleteTag"))
	l.DebugContext(ctx, "Deleting user tag")

	userIDStr, ok := auth.GetUserIDFromContext(ctx)
	if !ok || userIDStr == "" {
		l.ErrorContext(ctx, "User ID not found in context")
		span.SetStatus(codes.Error, "User ID not found in context")
		api.ErrorResponse(w, r, http.StatusUnauthorized, "Authentication required")
		return
	}

	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		l.ErrorContext(ctx, "Invalid user ID format", slog.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Invalid user ID format")
		api.ErrorResponse(w, r, http.StatusBadRequest, "Invalid user ID format")
		return
	}

	tagIDStr := chi.URLParam(r, "tagID")
	tagID, err := uuid.Parse(tagIDStr)
	if err != nil {
		l.WarnContext(ctx, "Invalid tag ID format", slog.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Invalid tag ID format")
		api.ErrorResponse(w, r, http.StatusBadRequest, "Invalid tag ID format")
		return
	}

	err = u.tagsService.DeleteTag(ctx, userID, tagID)
	if err != nil {
		if errors.Is(err, types.ErrNotFound) {
			l.WarnContext(ctx, "Tag not found for deletion", slog.String("tagID", tagID.String()))
			span.SetStatus(codes.Error, "Tag not found")
			api.ErrorResponse(w, r, http.StatusNotFound, "Tag not found")
			return
		}
		l.ErrorContext(ctx, "Failed to delete user tag", slog.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to delete user tag")
		api.ErrorResponse(w, r, http.StatusInternalServerError, "Failed to delete user tag")
		return
	}
	span.SetStatus(codes.Ok, "Tag deleted successfully")
	api.WriteJSONResponse(w, r, http.StatusOK, nil)
}

// UpdateTag godoc
// @Summary      Update User Tag
// @Description  Updates a specific tag for the authenticated user
// @Tags         User Tags
// @Accept       json
// @Produce      json
// @Param        tagID path string true "Tag ID to update"
// @Param        tag body types.UpdatePersonalTagParams true "Tag update details"
// @Success      200 "Tag updated successfully"
// @Failure      400 {object} types.Response "Invalid Input"
// @Failure      401 {object} types.Response "Unauthorized"
// @Failure      404 {object} types.Response "Tag not found"
// @Failure      500 {object} types.Response "Internal Server Error"
// @Security     BearerAuth
// @Router       /user/tags/{tagID} [put]
func (u *HandlerImpl) UpdateTag(w http.ResponseWriter, r *http.Request) {
	ctx, span := otel.Tracer("UpdateTagHandlerImpl").Start(r.Context(), "UpdateTag", trace.WithAttributes(
		semconv.HTTPRequestMethodKey.String(r.Method),
		semconv.HTTPRouteKey.String("/user/tags"),
	))
	defer span.End()

	l := u.logger.With(slog.String("HandlerImpl", "UpdateTag"))
	l.DebugContext(ctx, "Updating user tag")

	userIDStr, ok := auth.GetUserIDFromContext(ctx)
	if !ok || userIDStr == "" {
		l.ErrorContext(ctx, "User ID not found in context")
		span.SetStatus(codes.Error, "User ID not found in context")
		api.ErrorResponse(w, r, http.StatusUnauthorized, "Authentication required")
		return
	}

	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		l.ErrorContext(ctx, "Invalid user ID format", slog.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Invalid user ID format")
		api.ErrorResponse(w, r, http.StatusBadRequest, "Invalid user ID format")
		return
	}

	tagIDStr := chi.URLParam(r, "tagID")
	tagID, err := uuid.Parse(tagIDStr)

	if err != nil {
		l.WarnContext(ctx, "Invalid tag ID format", slog.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Invalid tag ID format")
		api.ErrorResponse(w, r, http.StatusBadRequest, "Invalid tag ID format")
		return
	}

	var req types.UpdatePersonalTagParams
	if err = api.DecodeJSONBody(w, r, &req); err != nil {
		l.WarnContext(ctx, "Failed to decode request", slog.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to decode request")
		api.ErrorResponse(w, r, http.StatusBadRequest, "Invalid request format")
		return
	}

	err = u.tagsService.Update(ctx, userID, tagID, req)
	if err != nil {
		if errors.Is(err, types.ErrNotFound) {
			l.WarnContext(ctx, "Tag not found for update", slog.String("tagID", tagID.String()))
			span.SetStatus(codes.Error, "Tag not found")
			api.ErrorResponse(w, r, http.StatusNotFound, "Tag not found")
			return
		}
		l.ErrorContext(ctx, "Failed to update user tag", slog.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to update user tag")
		api.ErrorResponse(w, r, http.StatusInternalServerError, "Failed to update user tag")
		return
	}
	span.SetStatus(codes.Ok, "Tag updated successfully")
	api.WriteJSONResponse(w, r, http.StatusOK, nil)
}
