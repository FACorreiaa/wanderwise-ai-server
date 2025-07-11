package itinerarylist

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"github.com/FACorreiaa/go-poi-au-suggestions/internal/api"
	"github.com/FACorreiaa/go-poi-au-suggestions/internal/api/auth"
	"github.com/FACorreiaa/go-poi-au-suggestions/internal/types"
)

var _ Handler = (*HandlerImpl)(nil)

type Handler interface {
	CreateTopLevelListHandler(w http.ResponseWriter, r *http.Request)
	CreateItineraryForListHandler(w http.ResponseWriter, r *http.Request)
	GetListDetailsHandler(w http.ResponseWriter, r *http.Request)
	UpdateListDetailsHandler(w http.ResponseWriter, r *http.Request)
	DeleteListHandler(w http.ResponseWriter, r *http.Request)
	AddListItemHandler(w http.ResponseWriter, r *http.Request)
	UpdateListItemHandler(w http.ResponseWriter, r *http.Request)
	RemoveListItemHandler(w http.ResponseWriter, r *http.Request)
	// Legacy POI-specific handlers (for backward compatibility)
	AddPOIListItemHandler(w http.ResponseWriter, r *http.Request)
	UpdatePOIListItemHandler(w http.ResponseWriter, r *http.Request)
	RemovePOIListItemHandler(w http.ResponseWriter, r *http.Request)
	GetUserListsHandler(w http.ResponseWriter, r *http.Request)
}

type HandlerImpl struct {
	logger  *slog.Logger
	service Service
	// llmChatService *llmChat.ServiceImpl // If needed for AI list generation
}

func NewHandler(service Service, logger *slog.Logger /*, llmService *llmChat.ServiceImpl*/) *HandlerImpl {
	return &HandlerImpl{
		logger:  logger,
		service: service,
		// llmChatService: llmService,
	}
}

// CreateTopLevelListHandler godoc
// @Summary      Create Top Level List
// @Description  Creates a new top-level list or itinerary for the authenticated user
// @Tags         Lists
// @Accept       json
// @Produce      json
// @Param        list body types.CreateListRequest true "List creation details"
// @Success      201 {object} interface{} "List created successfully"
// @Failure      400 {object} types.Response "Invalid Input"
// @Failure      401 {object} types.Response "Authentication required"
// @Failure      500 {object} types.Response "Internal Server Error"
// @Security     BearerAuth
// @Router       /lists [post]
func (h *HandlerImpl) CreateTopLevelListHandler(w http.ResponseWriter, r *http.Request) {
	ctx, span := otel.Tracer("ItineraryListHandler").Start(r.Context(), "CreateTopLevelList")
	defer span.End()
	l := h.logger.With(slog.String("handler", "CreateTopLevelListHandler"))

	userIDStr, ok := auth.GetUserIDFromContext(ctx)
	if !ok || userIDStr == "" {
		l.ErrorContext(ctx, "User ID not found in context")
		span.SetStatus(codes.Error, "Unauthorized - User ID missing")
		api.ErrorResponse(w, r, http.StatusUnauthorized, "Authentication required")
		return
	}
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		l.ErrorContext(ctx, "Invalid user ID format", slog.String("userID_str", userIDStr), slog.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Invalid User ID format")
		api.ErrorResponse(w, r, http.StatusBadRequest, "Invalid user ID format")
		return
	}
	span.SetAttributes(attribute.String("user.id", userID.String()))

	var req types.CreateListRequest
	if err := api.DecodeJSONBody(w, r, &req); err != nil {
		l.ErrorContext(ctx, "Failed to decode or validate request", slog.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Bad request")
		return // ErrorResponse handled by DecodeJSONBody
	}
	span.SetAttributes(attribute.String("list.name", req.Name), attribute.Bool("list.is_itinerary", req.IsItinerary))

	l.DebugContext(ctx, "Attempting to create top-level list", slog.String("name", req.Name))
	list, err := h.service.CreateTopLevelList(ctx, userID, req.Name, req.Description, req.CityID, req.IsItinerary, req.IsPublic)
	if err != nil {
		l.ErrorContext(ctx, "Service failed to create top-level list", slog.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to create list")
		api.ErrorResponse(w, r, http.StatusInternalServerError, "Failed to create list: "+err.Error())
		return
	}

	l.InfoContext(ctx, "Top-level list created successfully", slog.String("list_id", list.ID.String()))
	span.SetAttributes(attribute.String("list.id", list.ID.String()))
	span.SetStatus(codes.Ok, "List created")
	api.WriteJSONResponse(w, r, http.StatusCreated, list)
}

func (h *HandlerImpl) CreateItineraryForListHandler(w http.ResponseWriter, r *http.Request) {
	ctx, span := otel.Tracer("ItineraryListHandler").Start(r.Context(), "CreateItineraryForList")
	defer span.End()
	l := h.logger.With(slog.String("handler", "CreateItineraryForListHandler"))

	userIDStr, ok := auth.GetUserIDFromContext(ctx)
	if !ok || userIDStr == "" {
		l.ErrorContext(ctx, "User ID not found in context")
		span.SetStatus(codes.Error, "Unauthorized - User ID missing")
		api.ErrorResponse(w, r, http.StatusUnauthorized, "Authentication required")
		return
	}
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		l.ErrorContext(ctx, "Invalid user ID format", slog.String("userID_str", userIDStr), slog.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Invalid User ID format")
		api.ErrorResponse(w, r, http.StatusBadRequest, "Invalid user ID format")
		return
	}
	span.SetAttributes(attribute.String("user.id", userID.String()))

	parentListIDStr := chi.URLParam(r, "parentListID")
	parentListID, err := uuid.Parse(parentListIDStr)
	if err != nil {
		l.ErrorContext(ctx, "Invalid parent list ID format", slog.String("parentListID_str", parentListIDStr), slog.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Invalid Parent List ID format")
		api.ErrorResponse(w, r, http.StatusBadRequest, "Invalid parent list ID format")
		return
	}
	span.SetAttributes(attribute.String("parent_list.id", parentListID.String()))

	var req types.CreateItineraryForListRequest
	if err := api.DecodeJSONBody(w, r, &req); err != nil {
		l.ErrorContext(ctx, "Failed to decode or validate request", slog.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Bad request")
		return
	}
	span.SetAttributes(attribute.String("itinerary.name", req.Name))

	l.DebugContext(ctx, "Attempting to create itinerary for list", slog.String("name", req.Name))
	itinerary, err := h.service.CreateItineraryForList(ctx, userID, parentListID, req.Name, req.Description, req.IsPublic)
	if err != nil {
		l.ErrorContext(ctx, "Service failed to create itinerary", slog.Any("error", err))
		span.RecordError(err)
		if strings.Contains(err.Error(), "not found") {
			span.SetStatus(codes.Error, "Parent list not found")
			api.ErrorResponse(w, r, http.StatusNotFound, "Parent list not found")
		} else if strings.Contains(err.Error(), "does not own") || strings.Contains(err.Error(), "cannot add itinerary") {
			span.SetStatus(codes.Error, "Forbidden")
			api.ErrorResponse(w, r, http.StatusForbidden, err.Error())
		} else {
			span.SetStatus(codes.Error, "Failed to create itinerary")
			api.ErrorResponse(w, r, http.StatusInternalServerError, "Failed to create itinerary: "+err.Error())
		}
		return
	}

	l.InfoContext(ctx, "Itinerary created successfully", slog.String("itinerary_id", itinerary.ID.String()))
	span.SetAttributes(attribute.String("itinerary.id", itinerary.ID.String()))
	span.SetStatus(codes.Ok, "Itinerary created")
	api.WriteJSONResponse(w, r, http.StatusCreated, itinerary)
}

// GetListDetailsHandler godoc
// @Summary      Get List Details
// @Description  Retrieves detailed information about a specific list including items
// @Tags         Lists
// @Accept       json
// @Produce      json
// @Param        listID path string true "List ID"
// @Success      200 {object} interface{} "List details"
// @Failure      400 {object} types.Response "Invalid Input"
// @Failure      401 {object} types.Response "Authentication required"
// @Failure      403 {object} types.Response "Access denied"
// @Failure      404 {object} types.Response "List not found"
// @Failure      500 {object} types.Response "Internal Server Error"
// @Security     BearerAuth
// @Router       /lists/{listID} [get]
func (h *HandlerImpl) GetListDetailsHandler(w http.ResponseWriter, r *http.Request) {
	ctx, span := otel.Tracer("ItineraryListHandler").Start(r.Context(), "GetListDetails")
	defer span.End()
	l := h.logger.With(slog.String("handler", "GetListDetailsHandler"))

	userIDStr, ok := auth.GetUserIDFromContext(ctx)
	if !ok || userIDStr == "" {
		l.ErrorContext(ctx, "User ID not found in context")
		span.SetStatus(codes.Error, "Unauthorized - User ID missing")
		api.ErrorResponse(w, r, http.StatusUnauthorized, "Authentication required")
		return
	}
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		l.ErrorContext(ctx, "Invalid user ID format", slog.String("userID_str", userIDStr), slog.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Invalid User ID format")
		api.ErrorResponse(w, r, http.StatusBadRequest, "Invalid user ID format")
		return
	}
	span.SetAttributes(attribute.String("user.id", userID.String()))

	listIDStr := chi.URLParam(r, "listID")
	listID, err := uuid.Parse(listIDStr)
	if err != nil {
		l.ErrorContext(ctx, "Invalid list ID format", slog.String("listID_str", listIDStr), slog.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Invalid List ID format")
		api.ErrorResponse(w, r, http.StatusBadRequest, "Invalid list ID format")
		return
	}
	span.SetAttributes(attribute.String("list.id", listID.String()))

	l.DebugContext(ctx, "Attempting to get list details")
	listDetails, err := h.service.GetListDetails(ctx, listID, userID)
	if err != nil {
		l.ErrorContext(ctx, "Service failed to get list details", slog.Any("error", err))
		span.RecordError(err)
		if strings.Contains(err.Error(), "not found") {
			span.SetStatus(codes.Error, "List not found")
			api.ErrorResponse(w, r, http.StatusNotFound, "List not found")
		} else if strings.Contains(err.Error(), "access denied") {
			span.SetStatus(codes.Error, "Forbidden")
			api.ErrorResponse(w, r, http.StatusForbidden, "Access denied")
		} else {
			span.SetStatus(codes.Error, "Failed to get list details")
			api.ErrorResponse(w, r, http.StatusInternalServerError, "Failed to get list details: "+err.Error())
		}
		return
	}

	l.InfoContext(ctx, "List details fetched successfully", slog.String("list_name", listDetails.List.Name))
	span.SetStatus(codes.Ok, "List details fetched")
	api.WriteJSONResponse(w, r, http.StatusOK, listDetails)
}

func (h *HandlerImpl) UpdateListDetailsHandler(w http.ResponseWriter, r *http.Request) {
	ctx, span := otel.Tracer("ItineraryListHandler").Start(r.Context(), "UpdateListDetails")
	defer span.End()
	l := h.logger.With(slog.String("handler", "UpdateListDetailsHandler"))

	userIDStr, ok := auth.GetUserIDFromContext(ctx)
	if !ok || userIDStr == "" {
		l.ErrorContext(ctx, "User ID not found in context")
		span.SetStatus(codes.Error, "Unauthorized - User ID missing")
		api.ErrorResponse(w, r, http.StatusUnauthorized, "Authentication required")
		return
	}
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		l.ErrorContext(ctx, "Invalid user ID format", slog.String("userID_str", userIDStr), slog.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Invalid User ID format")
		api.ErrorResponse(w, r, http.StatusBadRequest, "Invalid user ID format")
		return
	}
	span.SetAttributes(attribute.String("user.id", userID.String()))

	listIDStr := chi.URLParam(r, "listID")
	listID, err := uuid.Parse(listIDStr)
	if err != nil {
		l.ErrorContext(ctx, "Invalid list ID format", slog.String("listID_str", listIDStr), slog.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Invalid List ID format")
		api.ErrorResponse(w, r, http.StatusBadRequest, "Invalid list ID format")
		return
	}
	span.SetAttributes(attribute.String("list.id", listID.String()))

	var req types.UpdateListRequest
	if err := api.DecodeJSONBody(w, r, &req); err != nil {
		l.ErrorContext(ctx, "Failed to decode or validate request", slog.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Bad request")
		return
	}

	l.DebugContext(ctx, "Attempting to update list details")
	updatedList, err := h.service.UpdateListDetails(ctx, listID, userID, req)
	if err != nil {
		l.ErrorContext(ctx, "Service failed to update list details", slog.Any("error", err))
		span.RecordError(err)
		if strings.Contains(err.Error(), "not found") {
			span.SetStatus(codes.Error, "List not found")
			api.ErrorResponse(w, r, http.StatusNotFound, "List not found")
		} else if strings.Contains(err.Error(), "does not own") {
			span.SetStatus(codes.Error, "Forbidden")
			api.ErrorResponse(w, r, http.StatusForbidden, "You do not own this list")
		} else {
			span.SetStatus(codes.Error, "Failed to update list")
			api.ErrorResponse(w, r, http.StatusInternalServerError, "Failed to update list: "+err.Error())
		}
		return
	}

	l.InfoContext(ctx, "List details updated successfully", slog.String("list_name", updatedList.Name))
	span.SetStatus(codes.Ok, "List updated")
	api.WriteJSONResponse(w, r, http.StatusOK, updatedList)
}

func (h *HandlerImpl) DeleteListHandler(w http.ResponseWriter, r *http.Request) {
	ctx, span := otel.Tracer("ItineraryListHandler").Start(r.Context(), "DeleteList")
	defer span.End()
	l := h.logger.With(slog.String("handler", "DeleteListHandler"))

	userIDStr, ok := auth.GetUserIDFromContext(ctx)
	if !ok || userIDStr == "" {
		l.ErrorContext(ctx, "User ID not found in context")
		span.SetStatus(codes.Error, "Unauthorized - User ID missing")
		api.ErrorResponse(w, r, http.StatusUnauthorized, "Authentication required")
		return
	}
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		l.ErrorContext(ctx, "Invalid user ID format", slog.String("userID_str", userIDStr), slog.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Invalid User ID format")
		api.ErrorResponse(w, r, http.StatusBadRequest, "Invalid user ID format")
		return
	}
	span.SetAttributes(attribute.String("user.id", userID.String()))

	listIDStr := chi.URLParam(r, "listID")
	listID, err := uuid.Parse(listIDStr)
	if err != nil {
		l.ErrorContext(ctx, "Invalid list ID format", slog.String("listID_str", listIDStr), slog.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Invalid List ID format")
		api.ErrorResponse(w, r, http.StatusBadRequest, "Invalid list ID format")
		return
	}
	span.SetAttributes(attribute.String("list.id", listID.String()))

	l.DebugContext(ctx, "Attempting to delete list")
	err = h.service.DeleteUserList(ctx, listID, userID)
	if err != nil {
		l.ErrorContext(ctx, "Service failed to delete list", slog.Any("error", err))
		span.RecordError(err)
		if strings.Contains(err.Error(), "not found") {
			span.SetStatus(codes.Error, "List not found")
			api.ErrorResponse(w, r, http.StatusNotFound, "List not found")
		} else if strings.Contains(err.Error(), "does not own") {
			span.SetStatus(codes.Error, "Forbidden")
			api.ErrorResponse(w, r, http.StatusForbidden, "You do not own this list")
		} else {
			span.SetStatus(codes.Error, "Failed to delete list")
			api.ErrorResponse(w, r, http.StatusInternalServerError, "Failed to delete list: "+err.Error())
		}
		return
	}

	l.InfoContext(ctx, "List deleted successfully")
	span.SetStatus(codes.Ok, "List deleted")
	w.WriteHeader(http.StatusNoContent)
}

// Generic list item handlers that support different content types

// AddListItemHandler godoc
// @Summary      Add Item to List
// @Description  Adds an item (POI, itinerary, etc.) to a specific list
// @Tags         Lists
// @Accept       json
// @Produce      json
// @Param        listID path string true "List ID"
// @Param        item body types.AddListItemRequest true "Item to add to the list"
// @Success      201 {object} interface{} "Item added to list successfully"
// @Failure      400 {object} types.Response "Invalid Input"
// @Failure      401 {object} types.Response "Authentication required"
// @Failure      403 {object} types.Response "Access denied"
// @Failure      404 {object} types.Response "List or item not found"
// @Failure      500 {object} types.Response "Internal Server Error"
// @Security     BearerAuth
// @Router       /lists/{listID}/items [post]
func (h *HandlerImpl) AddListItemHandler(w http.ResponseWriter, r *http.Request) {
	ctx, span := otel.Tracer("ItineraryListHandler").Start(r.Context(), "AddListItem")
	defer span.End()
	l := h.logger.With(slog.String("handler", "AddListItemHandler"))

	userIDStr, ok := auth.GetUserIDFromContext(ctx)
	if !ok || userIDStr == "" {
		l.ErrorContext(ctx, "User ID not found in context")
		span.SetStatus(codes.Error, "Unauthorized - User ID missing")
		api.ErrorResponse(w, r, http.StatusUnauthorized, "Authentication required")
		return
	}
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		l.ErrorContext(ctx, "Invalid user ID format", slog.String("userID_str", userIDStr), slog.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Invalid User ID format")
		api.ErrorResponse(w, r, http.StatusBadRequest, "Invalid user ID format")
		return
	}
	span.SetAttributes(attribute.String("user.id", userID.String()))

	listIDStr := chi.URLParam(r, "listID")
	listID, err := uuid.Parse(listIDStr)
	if err != nil {
		l.ErrorContext(ctx, "Invalid list ID format", slog.String("listID_str", listIDStr), slog.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Invalid List ID format")
		api.ErrorResponse(w, r, http.StatusBadRequest, "Invalid list ID format")
		return
	}
	span.SetAttributes(attribute.String("list.id", listID.String()))

	var req types.AddListItemRequest
	if err := api.DecodeJSONBody(w, r, &req); err != nil {
		l.ErrorContext(ctx, "Failed to decode or validate request", slog.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Bad request")
		return
	}
	span.SetAttributes(
		attribute.String("item.id", req.ItemID.String()),
		attribute.String("content.type", string(req.ContentType)),
	)

	l.DebugContext(ctx, "Attempting to add item to list",
		slog.String("content_type", string(req.ContentType)),
		slog.String("item_id", req.ItemID.String()),
	)

	listItem, err := h.service.AddListItem(ctx, userID, listID, req)
	if err != nil {
		l.ErrorContext(ctx, "Service failed to add item to list", slog.Any("error", err))
		span.RecordError(err)
		if strings.Contains(err.Error(), "not found") {
			span.SetStatus(codes.Error, "Resource not found")
			api.ErrorResponse(w, r, http.StatusNotFound, err.Error())
		} else if strings.Contains(err.Error(), "does not own") {
			span.SetStatus(codes.Error, "Forbidden")
			api.ErrorResponse(w, r, http.StatusForbidden, err.Error())
		} else {
			span.SetStatus(codes.Error, "Failed to add item")
			api.ErrorResponse(w, r, http.StatusInternalServerError, "Failed to add item to list: "+err.Error())
		}
		return
	}

	l.InfoContext(ctx, "Item added to list successfully",
		slog.String("item_id", req.ItemID.String()),
		slog.String("content_type", string(req.ContentType)),
	)
	span.SetStatus(codes.Ok, "Item added")
	api.WriteJSONResponse(w, r, http.StatusCreated, listItem)
}

func (h *HandlerImpl) UpdateListItemHandler(w http.ResponseWriter, r *http.Request) {
	ctx, span := otel.Tracer("ItineraryListHandler").Start(r.Context(), "UpdateListItem")
	defer span.End()
	l := h.logger.With(slog.String("handler", "UpdateListItemHandler"))

	userIDStr, ok := auth.GetUserIDFromContext(ctx)
	if !ok || userIDStr == "" {
		l.ErrorContext(ctx, "User ID not found in context")
		span.SetStatus(codes.Error, "Unauthorized - User ID missing")
		api.ErrorResponse(w, r, http.StatusUnauthorized, "Authentication required")
		return
	}
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		l.ErrorContext(ctx, "Invalid user ID format", slog.String("userID_str", userIDStr), slog.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Invalid User ID format")
		api.ErrorResponse(w, r, http.StatusBadRequest, "Invalid user ID format")
		return
	}
	span.SetAttributes(attribute.String("user.id", userID.String()))

	listIDStr := chi.URLParam(r, "listID")
	listID, err := uuid.Parse(listIDStr)
	if err != nil {
		l.ErrorContext(ctx, "Invalid list ID format", slog.String("listID_str", listIDStr), slog.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Invalid List ID format")
		api.ErrorResponse(w, r, http.StatusBadRequest, "Invalid list ID format")
		return
	}
	span.SetAttributes(attribute.String("list.id", listID.String()))

	itemIDStr := chi.URLParam(r, "itemID")
	itemID, err := uuid.Parse(itemIDStr)
	if err != nil {
		l.ErrorContext(ctx, "Invalid item ID format", slog.String("itemID_str", itemIDStr), slog.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Invalid Item ID format")
		api.ErrorResponse(w, r, http.StatusBadRequest, "Invalid item ID format")
		return
	}
	span.SetAttributes(attribute.String("item.id", itemID.String()))

	var req types.UpdateListItemRequest
	if err := api.DecodeJSONBody(w, r, &req); err != nil {
		l.ErrorContext(ctx, "Failed to decode or validate request", slog.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Bad request")
		return
	}

	l.DebugContext(ctx, "Attempting to update item in list")
	updatedItem, err := h.service.UpdateListItem(ctx, userID, listID, itemID, req)
	if err != nil {
		l.ErrorContext(ctx, "Service failed to update item in list", slog.Any("error", err))
		span.RecordError(err)
		if strings.Contains(err.Error(), "not found") {
			span.SetStatus(codes.Error, "Resource not found")
			api.ErrorResponse(w, r, http.StatusNotFound, "Item or list not found")
		} else if strings.Contains(err.Error(), "does not own") {
			span.SetStatus(codes.Error, "Forbidden")
			api.ErrorResponse(w, r, http.StatusForbidden, "You do not own this list")
		} else {
			span.SetStatus(codes.Error, "Failed to update item")
			api.ErrorResponse(w, r, http.StatusInternalServerError, "Failed to update item: "+err.Error())
		}
		return
	}

	l.InfoContext(ctx, "Item in list updated successfully")
	span.SetStatus(codes.Ok, "Item updated")
	api.WriteJSONResponse(w, r, http.StatusOK, updatedItem)
}

func (h *HandlerImpl) RemoveListItemHandler(w http.ResponseWriter, r *http.Request) {
	ctx, span := otel.Tracer("ItineraryListHandler").Start(r.Context(), "RemoveListItem")
	defer span.End()
	l := h.logger.With(slog.String("handler", "RemoveListItemHandler"))

	userIDStr, ok := auth.GetUserIDFromContext(ctx)
	if !ok || userIDStr == "" {
		l.ErrorContext(ctx, "User ID not found in context")
		span.SetStatus(codes.Error, "Unauthorized - User ID missing")
		api.ErrorResponse(w, r, http.StatusUnauthorized, "Authentication required")
		return
	}
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		l.ErrorContext(ctx, "Invalid user ID format", slog.String("userID_str", userIDStr), slog.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Invalid User ID format")
		api.ErrorResponse(w, r, http.StatusBadRequest, "Invalid user ID format")
		return
	}
	span.SetAttributes(attribute.String("user.id", userID.String()))

	listIDStr := chi.URLParam(r, "listID")
	listID, err := uuid.Parse(listIDStr)
	if err != nil {
		l.ErrorContext(ctx, "Invalid list ID format", slog.String("listID_str", listIDStr), slog.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Invalid List ID format")
		api.ErrorResponse(w, r, http.StatusBadRequest, "Invalid list ID format")
		return
	}
	span.SetAttributes(attribute.String("list.id", listID.String()))

	itemIDStr := chi.URLParam(r, "itemID")
	itemID, err := uuid.Parse(itemIDStr)
	if err != nil {
		l.ErrorContext(ctx, "Invalid item ID format", slog.String("itemID_str", itemIDStr), slog.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Invalid Item ID format")
		api.ErrorResponse(w, r, http.StatusBadRequest, "Invalid item ID format")
		return
	}
	span.SetAttributes(attribute.String("item.id", itemID.String()))

	l.DebugContext(ctx, "Attempting to remove item from list")
	err = h.service.RemoveListItem(ctx, userID, listID, itemID)
	if err != nil {
		l.ErrorContext(ctx, "Service failed to remove item from list", slog.Any("error", err))
		span.RecordError(err)
		if strings.Contains(err.Error(), "not found") {
			span.SetStatus(codes.Error, "Resource not found")
			api.ErrorResponse(w, r, http.StatusNotFound, "Item or list not found")
		} else if strings.Contains(err.Error(), "does not own") {
			span.SetStatus(codes.Error, "Forbidden")
			api.ErrorResponse(w, r, http.StatusForbidden, "You do not own this list")
		} else {
			span.SetStatus(codes.Error, "Failed to remove item")
			api.ErrorResponse(w, r, http.StatusInternalServerError, "Failed to remove item: "+err.Error())
		}
		return
	}

	l.InfoContext(ctx, "Item removed from list successfully")
	span.SetStatus(codes.Ok, "Item removed")
	w.WriteHeader(http.StatusNoContent)
}

// Legacy POI-specific handlers (for backward compatibility)

func (h *HandlerImpl) AddPOIListItemHandler(w http.ResponseWriter, r *http.Request) {
	ctx, span := otel.Tracer("ItineraryListHandler").Start(r.Context(), "AddPOIListItem")
	defer span.End()
	l := h.logger.With(slog.String("handler", "AddPOIListItemHandler"))

	userIDStr, ok := auth.GetUserIDFromContext(ctx)
	if !ok || userIDStr == "" {
		l.ErrorContext(ctx, "User ID not found in context")
		span.SetStatus(codes.Error, "Unauthorized - User ID missing")
		api.ErrorResponse(w, r, http.StatusUnauthorized, "Authentication required")
		return
	}
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		l.ErrorContext(ctx, "Invalid user ID format", slog.String("userID_str", userIDStr), slog.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Invalid User ID format")
		api.ErrorResponse(w, r, http.StatusBadRequest, "Invalid user ID format")
		return
	}
	span.SetAttributes(attribute.String("user.id", userID.String()))

	itineraryIDStr := chi.URLParam(r, "itineraryID")
	itineraryID, err := uuid.Parse(itineraryIDStr)
	if err != nil {
		l.ErrorContext(ctx, "Invalid itinerary ID format", slog.String("itineraryID_str", itineraryIDStr), slog.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Invalid Itinerary ID format")
		api.ErrorResponse(w, r, http.StatusBadRequest, "Invalid itinerary ID format")
		return
	}
	span.SetAttributes(attribute.String("itinerary.id", itineraryID.String()))

	var req types.AddListItemRequest
	if err := api.DecodeJSONBody(w, r, &req); err != nil {
		l.ErrorContext(ctx, "Failed to decode or validate request", slog.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Bad request")
		return
	}
	span.SetAttributes(attribute.String("poi.id", req.ItemID.String()))

	l.DebugContext(ctx, "Attempting to add POI to itinerary")
	listItem, err := h.service.AddPOIListItem(ctx, userID, itineraryID, req.ItemID, req)
	if err != nil {
		l.ErrorContext(ctx, "Service failed to add POI to itinerary", slog.Any("error", err))
		span.RecordError(err)
		if strings.Contains(err.Error(), "not found") {
			span.SetStatus(codes.Error, "Resource not found")
			api.ErrorResponse(w, r, http.StatusNotFound, err.Error())
		} else if strings.Contains(err.Error(), "does not own") || strings.Contains(err.Error(), "not an itinerary") {
			span.SetStatus(codes.Error, "Forbidden")
			api.ErrorResponse(w, r, http.StatusForbidden, err.Error())
		} else {
			span.SetStatus(codes.Error, "Failed to add POI")
			api.ErrorResponse(w, r, http.StatusInternalServerError, "Failed to add POI to itinerary: "+err.Error())
		}
		return
	}

	l.InfoContext(ctx, "POI added to itinerary successfully", slog.String("poi_id", req.ItemID.String()))
	span.SetStatus(codes.Ok, "POI added")
	api.WriteJSONResponse(w, r, http.StatusCreated, listItem)
}

func (h *HandlerImpl) UpdatePOIListItemHandler(w http.ResponseWriter, r *http.Request) {
	ctx, span := otel.Tracer("ItineraryListHandler").Start(r.Context(), "UpdatePOIListItem")
	defer span.End()
	l := h.logger.With(slog.String("handler", "UpdatePOIListItemHandler"))

	userIDStr, ok := auth.GetUserIDFromContext(ctx)
	if !ok || userIDStr == "" {
		l.ErrorContext(ctx, "User ID not found in context")
		span.SetStatus(codes.Error, "Unauthorized - User ID missing")
		api.ErrorResponse(w, r, http.StatusUnauthorized, "Authentication required")
		return
	}
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		l.ErrorContext(ctx, "Invalid user ID format", slog.String("userID_str", userIDStr), slog.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Invalid User ID format")
		api.ErrorResponse(w, r, http.StatusBadRequest, "Invalid user ID format")
		return
	}
	span.SetAttributes(attribute.String("user.id", userID.String()))

	itineraryIDStr := chi.URLParam(r, "itineraryID")
	itineraryID, err := uuid.Parse(itineraryIDStr)
	if err != nil {
		l.ErrorContext(ctx, "Invalid itinerary ID format", slog.String("itineraryID_str", itineraryIDStr), slog.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Invalid Itinerary ID format")
		api.ErrorResponse(w, r, http.StatusBadRequest, "Invalid itinerary ID format")
		return
	}
	span.SetAttributes(attribute.String("itinerary.id", itineraryID.String()))

	poiIDStr := chi.URLParam(r, "poiID")
	poiID, err := uuid.Parse(poiIDStr)
	if err != nil {
		l.ErrorContext(ctx, "Invalid POI ID format", slog.String("poiID_str", poiIDStr), slog.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Invalid POI ID format")
		api.ErrorResponse(w, r, http.StatusBadRequest, "Invalid POI ID format")
		return
	}
	span.SetAttributes(attribute.String("poi.id", poiID.String()))

	var req types.UpdateListItemRequest
	if err := api.DecodeJSONBody(w, r, &req); err != nil {
		l.ErrorContext(ctx, "Failed to decode or validate request", slog.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Bad request")
		return
	}

	l.DebugContext(ctx, "Attempting to update POI in itinerary")
	updatedItem, err := h.service.UpdatePOIListItem(ctx, userID, itineraryID, poiID, req)
	if err != nil {
		l.ErrorContext(ctx, "Service failed to update POI in itinerary", slog.Any("error", err))
		span.RecordError(err)
		if strings.Contains(err.Error(), "not found") {
			span.SetStatus(codes.Error, "Resource not found")
			api.ErrorResponse(w, r, http.StatusNotFound, "Item or itinerary not found")
		} else if strings.Contains(err.Error(), "does not own") {
			span.SetStatus(codes.Error, "Forbidden")
			api.ErrorResponse(w, r, http.StatusForbidden, "You do not own this itinerary")
		} else {
			span.SetStatus(codes.Error, "Failed to update item")
			api.ErrorResponse(w, r, http.StatusInternalServerError, "Failed to update item: "+err.Error())
		}
		return
	}

	l.InfoContext(ctx, "POI in itinerary updated successfully")
	span.SetStatus(codes.Ok, "Item updated")
	api.WriteJSONResponse(w, r, http.StatusOK, updatedItem)
}

func (h *HandlerImpl) RemovePOIListItemHandler(w http.ResponseWriter, r *http.Request) {
	ctx, span := otel.Tracer("ItineraryListHandler").Start(r.Context(), "RemovePOIListItem")
	defer span.End()
	l := h.logger.With(slog.String("handler", "RemovePOIListItemHandler"))

	userIDStr, ok := auth.GetUserIDFromContext(ctx)
	if !ok || userIDStr == "" {
		l.ErrorContext(ctx, "User ID not found in context")
		span.SetStatus(codes.Error, "Unauthorized - User ID missing")
		api.ErrorResponse(w, r, http.StatusUnauthorized, "Authentication required")
		return
	}
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		l.ErrorContext(ctx, "Invalid user ID format", slog.String("userID_str", userIDStr), slog.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Invalid User ID format")
		api.ErrorResponse(w, r, http.StatusBadRequest, "Invalid user ID format")
		return
	}
	span.SetAttributes(attribute.String("user.id", userID.String()))

	itineraryIDStr := chi.URLParam(r, "itineraryID")
	itineraryID, err := uuid.Parse(itineraryIDStr)
	if err != nil {
		l.ErrorContext(ctx, "Invalid itinerary ID format", slog.String("itineraryID_str", itineraryIDStr), slog.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Invalid Itinerary ID format")
		api.ErrorResponse(w, r, http.StatusBadRequest, "Invalid itinerary ID format")
		return
	}
	span.SetAttributes(attribute.String("itinerary.id", itineraryID.String()))

	poiIDStr := chi.URLParam(r, "poiID")
	poiID, err := uuid.Parse(poiIDStr)
	if err != nil {
		l.ErrorContext(ctx, "Invalid POI ID format", slog.String("poiID_str", poiIDStr), slog.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Invalid POI ID format")
		api.ErrorResponse(w, r, http.StatusBadRequest, "Invalid POI ID format")
		return
	}
	span.SetAttributes(attribute.String("poi.id", poiID.String()))

	l.DebugContext(ctx, "Attempting to remove POI from itinerary")
	err = h.service.RemovePOIListItem(ctx, userID, itineraryID, poiID)
	if err != nil {
		l.ErrorContext(ctx, "Service failed to remove POI from itinerary", slog.Any("error", err))
		span.RecordError(err)
		if strings.Contains(err.Error(), "not found") {
			span.SetStatus(codes.Error, "Resource not found")
			api.ErrorResponse(w, r, http.StatusNotFound, "Item or itinerary not found")
		} else if strings.Contains(err.Error(), "does not own") {
			span.SetStatus(codes.Error, "Forbidden")
			api.ErrorResponse(w, r, http.StatusForbidden, "You do not own this itinerary")
		} else {
			span.SetStatus(codes.Error, "Failed to remove item")
			api.ErrorResponse(w, r, http.StatusInternalServerError, "Failed to remove item: "+err.Error())
		}
		return
	}

	l.InfoContext(ctx, "POI removed from itinerary successfully")
	span.SetStatus(codes.Ok, "Item removed")
	w.WriteHeader(http.StatusNoContent)
}

// GetUserListsHandler godoc
// @Summary      Get User Lists
// @Description  Retrieves all lists for the authenticated user, optionally filtered by type (itinerary or collection)
// @Tags         Lists
// @Accept       json
// @Produce      json
// @Param        type query string false "List type filter ('itinerary' or 'collection')"
// @Success      200 {array} interface{} "User lists"
// @Failure      400 {object} types.Response "Invalid Input"
// @Failure      401 {object} types.Response "Authentication required"
// @Failure      500 {object} types.Response "Internal Server Error"
// @Security     BearerAuth
// @Router       /lists [get]
func (h *HandlerImpl) GetUserListsHandler(w http.ResponseWriter, r *http.Request) {
	ctx, span := otel.Tracer("ItineraryListHandler").Start(r.Context(), "GetUserLists")
	defer span.End()
	l := h.logger.With(slog.String("handler", "GetUserListsHandler"))

	userIDStr, ok := auth.GetUserIDFromContext(ctx)
	if !ok || userIDStr == "" {
		l.ErrorContext(ctx, "User ID not found in context")
		span.SetStatus(codes.Error, "Unauthorized - User ID missing")
		api.ErrorResponse(w, r, http.StatusUnauthorized, "Authentication required")
		return
	}
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		l.ErrorContext(ctx, "Invalid user ID format", slog.String("userID_str", userIDStr), slog.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Invalid User ID format")
		api.ErrorResponse(w, r, http.StatusBadRequest, "Invalid user ID format")
		return
	}
	span.SetAttributes(attribute.String("user.id", userID.String()))

	listType := r.URL.Query().Get("type")
	var isItinerary bool
	if strings.ToLower(listType) == "itinerary" {
		isItinerary = true
	} else if strings.ToLower(listType) == "collection" {
		isItinerary = false
	} else if listType != "" {
		l.WarnContext(ctx, "Invalid list type provided", slog.String("list_type", listType))
		span.SetStatus(codes.Error, "Invalid list type")
		api.ErrorResponse(w, r, http.StatusBadRequest, "Invalid list type. Use 'itinerary' or 'collection'.")
		return
	}
	// If listType is empty, isItinerary defaults to false (collections)
	span.SetAttributes(attribute.Bool("filter.is_itinerary", isItinerary))

	l.DebugContext(ctx, "Attempting to get user lists", slog.Bool("is_itinerary", isItinerary))
	lists, err := h.service.GetUserLists(ctx, userID, isItinerary)
	if err != nil {
		l.ErrorContext(ctx, "Service failed to get user lists", slog.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to get lists")
		api.ErrorResponse(w, r, http.StatusInternalServerError, "Failed to retrieve lists: "+err.Error())
		return
	}

	l.InfoContext(ctx, "User lists fetched successfully", slog.Int("count", len(lists)))
	span.SetStatus(codes.Ok, "User lists fetched")
	api.WriteJSONResponse(w, r, http.StatusOK, lists)
}
