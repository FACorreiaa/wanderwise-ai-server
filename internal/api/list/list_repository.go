package itineraryList

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"

	"github.com/FACorreiaa/go-poi-au-suggestions/internal/types"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Ensure RepositoryImpl implements the Repository interface
var _ Repository = (*RepositoryImpl)(nil)

// RepositoryImpl struct holds the logger and database connection pool
type RepositoryImpl struct {
	logger *slog.Logger
	pgpool *pgxpool.Pool
}

// Repository defines the interface for list and list item operations
type Repository interface {
	CreateList(ctx context.Context, list types.List) error
	GetList(ctx context.Context, listID uuid.UUID) (types.List, error)
	UpdateList(ctx context.Context, list types.List) error
	GetSubLists(ctx context.Context, parentListID uuid.UUID) ([]*types.List, error)
	GetListItems(ctx context.Context, listID uuid.UUID) ([]*types.ListItem, error)

	// Generic list item methods (support all content types)
	GetListItemByID(ctx context.Context, listID, itemID uuid.UUID) (types.ListItem, error)
	DeleteListItemByID(ctx context.Context, listID, itemID uuid.UUID) error

	// Legacy POI-specific methods (for backward compatibility)
	GetListItem(ctx context.Context, listID, poiID uuid.UUID) (types.ListItem, error)
	AddListItem(ctx context.Context, item types.ListItem) error
	UpdateListItem(ctx context.Context, item types.ListItem) error
	DeleteListItem(ctx context.Context, listID, poiID uuid.UUID) error // Adjusted signature
	DeleteList(ctx context.Context, listID uuid.UUID) error
	GetUserLists(ctx context.Context, userID uuid.UUID, isItinerary bool) ([]*types.List, error)
}

func NewRepository(pgxpool *pgxpool.Pool, logger *slog.Logger) *RepositoryImpl {
	return &RepositoryImpl{
		logger: logger,
		pgpool: pgxpool,
	}
}

// CreateList inserts a new list into the lists table
func (r *RepositoryImpl) CreateList(ctx context.Context, list types.List) error {
	query := `
        INSERT INTO lists (
            id, user_id, name, description, image_url, is_public, is_itinerary,
            parent_list_id, city_id, view_count, save_count, created_at, updated_at
        ) VALUES (
            $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13
        )
    `
	_, err := r.pgpool.Exec(ctx, query,
		list.ID, list.UserID, list.Name, list.Description, list.ImageURL, list.IsPublic, list.IsItinerary,
		list.ParentListID, list.CityID, list.ViewCount, list.SaveCount, list.CreatedAt, list.UpdatedAt,
	)
	if err != nil {
		r.logger.ErrorContext(ctx, "Failed to create list", slog.Any("error", err))
		return fmt.Errorf("failed to create list: %w", err)
	}
	return nil
}

// GetList retrieves a list by its ID from the lists table
func (r *RepositoryImpl) GetList(ctx context.Context, listID uuid.UUID) (types.List, error) {
	query := `
        SELECT id, user_id, name, description, image_url, is_public, is_itinerary,
               parent_list_id, city_id, view_count, save_count, created_at, updated_at
        FROM lists
        WHERE id = $1
    `
	row := r.pgpool.QueryRow(ctx, query, listID)
	var list types.List
	err := row.Scan(
		&list.ID, &list.UserID, &list.Name, &list.Description, &list.ImageURL, &list.IsPublic, &list.IsItinerary,
		&list.ParentListID, &list.CityID, &list.ViewCount, &list.SaveCount, &list.CreatedAt, &list.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return types.List{}, fmt.Errorf("list not found: %w", err)
		}
		r.logger.ErrorContext(ctx, "Failed to get list", slog.Any("error", err))
		return types.List{}, fmt.Errorf("failed to get list: %w", err)
	}
	return list, nil
}

// GetSubLists retrieves all sub-lists with a given parent_list_id
func (r *RepositoryImpl) GetSubLists(ctx context.Context, parentListID uuid.UUID) ([]*types.List, error) {
	query := `
        SELECT id, user_id, name, description, image_url, is_public, is_itinerary,
               parent_list_id, city_id, view_count, save_count, created_at, updated_at
        FROM lists
        WHERE parent_list_id = $1
    `
	rows, err := r.pgpool.Query(ctx, query, parentListID)
	if err != nil {
		r.logger.ErrorContext(ctx, "Failed to get sub-lists", slog.Any("error", err))
		return nil, fmt.Errorf("failed to get sub-lists: %w", err)
	}
	defer rows.Close()

	var subLists []*types.List
	for rows.Next() {
		var list types.List
		err := rows.Scan(
			&list.ID, &list.UserID, &list.Name, &list.Description, &list.ImageURL, &list.IsPublic, &list.IsItinerary,
			&list.ParentListID, &list.CityID, &list.ViewCount, &list.SaveCount, &list.CreatedAt, &list.UpdatedAt,
		)
		if err != nil {
			r.logger.ErrorContext(ctx, "Failed to scan sub-list", slog.Any("error", err))
			return nil, fmt.Errorf("failed to scan sub-list: %w", err)
		}
		subLists = append(subLists, &list)
	}
	if err = rows.Err(); err != nil {
		r.logger.ErrorContext(ctx, "Error iterating sub-list rows", slog.Any("error", err))
		return nil, fmt.Errorf("error iterating sub-list rows: %w", err)
	}
	return subLists, nil
}

// GetListItems retrieves all items associated with a specific list, ordered by position
func (r *RepositoryImpl) GetListItems(ctx context.Context, listID uuid.UUID) ([]*types.ListItem, error) {
	query := `
        SELECT list_id, poi_id, position, notes, day_number, time_slot, duration, created_at, updated_at
        FROM list_items
        WHERE list_id = $1
        ORDER BY position
    `
	rows, err := r.pgpool.Query(ctx, query, listID)
	if err != nil {
		r.logger.ErrorContext(ctx, "Failed to get list items", slog.Any("error", err))
		return nil, fmt.Errorf("failed to get list items: %w", err)
	}
	defer rows.Close()

	var items []*types.ListItem
	for rows.Next() {
		var item types.ListItem
		var dayNumber sql.NullInt32
		var timeSlot sql.NullTime
		var duration sql.NullInt32
		err := rows.Scan(
			&item.ListID, &item.ItemID, &item.Position, &item.Notes,
			&dayNumber, &timeSlot, &duration, &item.CreatedAt, &item.UpdatedAt,
		)
		if err != nil {
			r.logger.ErrorContext(ctx, "Failed to scan list item", slog.Any("error", err))
			return nil, fmt.Errorf("failed to scan list item: %w", err)
		}
		if dayNumber.Valid {
			dn := int(dayNumber.Int32)
			item.DayNumber = &dn
		}
		if timeSlot.Valid {
			item.TimeSlot = &timeSlot.Time
		}
		if duration.Valid {
			dur := int(duration.Int32)
			item.Duration = &dur
		}
		items = append(items, &item)
	}
	if err = rows.Err(); err != nil {
		r.logger.ErrorContext(ctx, "Error iterating list item rows", slog.Any("error", err))
		return nil, fmt.Errorf("error iterating list item rows: %w", err)
	}
	return items, nil
}

// AddListItem inserts a new item into the list_items table (supports both legacy and new structure)
func (r *RepositoryImpl) AddListItem(ctx context.Context, item types.ListItem) error {
	query := `
        INSERT INTO list_items (
            list_id, item_id, content_type, position, notes, day_number, time_slot, 
            duration, source_llm_interaction_id, item_ai_description, created_at, updated_at
        ) VALUES (
            $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12
        )
    `
	_, err := r.pgpool.Exec(ctx, query,
		item.ListID, item.ItemID, item.ContentType, item.Position, item.Notes,
		item.DayNumber, item.TimeSlot, item.Duration, item.SourceLlmInteractionID,
		item.ItemAIDescription, item.CreatedAt, item.UpdatedAt,
	)
	if err != nil {
		r.logger.ErrorContext(ctx, "Failed to add list item", slog.Any("error", err))
		return fmt.Errorf("failed to add list item: %w", err)
	}
	return nil
}

// DeleteListItem deletes a specific item from the list_items table using list_id and poi_id
func (r *RepositoryImpl) DeleteListItem(ctx context.Context, listID, poiID uuid.UUID) error {
	query := `DELETE FROM list_items WHERE list_id = $1 AND poi_id = $2`
	result, err := r.pgpool.Exec(ctx, query, listID, poiID)
	if err != nil {
		r.logger.ErrorContext(ctx, "Failed to delete list item", slog.Any("error", err))
		return fmt.Errorf("failed to delete list item: %w", err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("no list item found for list_id %s and poi_id %s", listID, poiID)
	}
	return nil
}

// DeleteList deletes a list by its ID from the lists table
func (r *RepositoryImpl) DeleteList(ctx context.Context, listID uuid.UUID) error {
	query := `DELETE FROM lists WHERE id = $1`
	result, err := r.pgpool.Exec(ctx, query, listID)
	if err != nil {
		r.logger.ErrorContext(ctx, "Failed to delete list", slog.Any("error", err))
		return fmt.Errorf("failed to delete list: %w", err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("no list found with ID %s", listID)
	}
	return nil
}

// UpdateList updates a list in the lists table
func (r *RepositoryImpl) UpdateList(ctx context.Context, list types.List) error {
	query := `
        UPDATE lists
        SET name = $1, description = $2, image_url = $3, is_public = $4, 
            city_id = $5, updated_at = $6
        WHERE id = $7
    `
	result, err := r.pgpool.Exec(ctx, query,
		list.Name, list.Description, list.ImageURL, list.IsPublic,
		list.CityID, list.UpdatedAt, list.ID,
	)
	if err != nil {
		r.logger.ErrorContext(ctx, "Failed to update list", slog.Any("error", err))
		return fmt.Errorf("failed to update list: %w", err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("no list found with ID %s", list.ID)
	}
	return nil
}

// GetListItem retrieves a specific item from the list_items table using list_id and poi_id
func (r *RepositoryImpl) GetListItem(ctx context.Context, listID, poiID uuid.UUID) (types.ListItem, error) {
	query := `
        SELECT list_id, poi_id, position, notes, day_number, time_slot, duration, created_at, updated_at
        FROM list_items
        WHERE list_id = $1 AND poi_id = $2
    `
	row := r.pgpool.QueryRow(ctx, query, listID, poiID)
	var item types.ListItem
	var dayNumber sql.NullInt32
	var timeSlot sql.NullTime
	var duration sql.NullInt32
	err := row.Scan(
		&item.ListID, &item.ItemID, &item.Position, &item.Notes,
		&dayNumber, &timeSlot, &duration, &item.CreatedAt, &item.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return types.ListItem{}, fmt.Errorf("list item not found: %w", err)
		}
		r.logger.ErrorContext(ctx, "Failed to get list item", slog.Any("error", err))
		return types.ListItem{}, fmt.Errorf("failed to get list item: %w", err)
	}
	if dayNumber.Valid {
		dn := int(dayNumber.Int32)
		item.DayNumber = &dn
	}
	if timeSlot.Valid {
		item.TimeSlot = &timeSlot.Time
	}
	if duration.Valid {
		dur := int(duration.Int32)
		item.Duration = &dur
	}
	return item, nil
}

// UpdateListItem updates an item in the list_items table (supports new generic structure)
func (r *RepositoryImpl) UpdateListItem(ctx context.Context, item types.ListItem) error {
	query := `
        UPDATE list_items
        SET item_id = $1, content_type = $2, position = $3, notes = $4, day_number = $5, 
            time_slot = $6, duration = $7, source_llm_interaction_id = $8, 
            item_ai_description = $9, updated_at = $10
        WHERE list_id = $11 AND item_id = $12
    `
	result, err := r.pgpool.Exec(ctx, query,
		item.ItemID, item.ContentType, item.Position, item.Notes, item.DayNumber,
		item.TimeSlot, item.Duration, item.SourceLlmInteractionID, item.ItemAIDescription,
		item.UpdatedAt, item.ListID, item.ItemID,
	)
	if err != nil {
		r.logger.ErrorContext(ctx, "Failed to update list item", slog.Any("error", err))
		return fmt.Errorf("failed to update list item: %w", err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("no list item found for list_id %s and item_id %s", item.ListID, item.ItemID)
	}
	return nil
}

// GetUserLists retrieves all lists for a user, optionally filtered by isItinerary
func (r *RepositoryImpl) GetUserLists(ctx context.Context, userID uuid.UUID, isItinerary bool) ([]*types.List, error) {
	query := `
        SELECT id, user_id, name, description, image_url, is_public, is_itinerary,
               parent_list_id, city_id, view_count, save_count, created_at, updated_at
        FROM lists
        WHERE user_id = $1 AND is_itinerary = $2
        ORDER BY created_at DESC
    `
	rows, err := r.pgpool.Query(ctx, query, userID, isItinerary)
	if err != nil {
		r.logger.ErrorContext(ctx, "Failed to get user lists", slog.Any("error", err))
		return nil, fmt.Errorf("failed to get user lists: %w", err)
	}
	defer rows.Close()

	var lists []*types.List
	for rows.Next() {
		var list types.List
		err := rows.Scan(
			&list.ID, &list.UserID, &list.Name, &list.Description, &list.ImageURL, &list.IsPublic, &list.IsItinerary,
			&list.ParentListID, &list.CityID, &list.ViewCount, &list.SaveCount, &list.CreatedAt, &list.UpdatedAt,
		)
		if err != nil {
			r.logger.ErrorContext(ctx, "Failed to scan list", slog.Any("error", err))
			return nil, fmt.Errorf("failed to scan list: %w", err)
		}
		lists = append(lists, &list)
	}
	if err = rows.Err(); err != nil {
		r.logger.ErrorContext(ctx, "Error iterating list rows", slog.Any("error", err))
		return nil, fmt.Errorf("error iterating list rows: %w", err)
	}
	return lists, nil
}

// Generic list item methods (support all content types)

// GetListItemByID retrieves a specific item from a list using generic item_id
func (r *RepositoryImpl) GetListItemByID(ctx context.Context, listID, itemID uuid.UUID) (types.ListItem, error) {
	query := `
        SELECT list_id, item_id, content_type, position, notes, day_number, 
               time_slot, duration, source_llm_interaction_id, item_ai_description, 
               created_at, updated_at
        FROM list_items 
        WHERE list_id = $1 AND item_id = $2
    `
	var item types.ListItem
	err := r.pgpool.QueryRow(ctx, query, listID, itemID).Scan(
		&item.ListID, &item.ItemID, &item.ContentType, &item.Position, &item.Notes,
		&item.DayNumber, &item.TimeSlot, &item.Duration, &item.SourceLlmInteractionID,
		&item.ItemAIDescription, &item.CreatedAt, &item.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return types.ListItem{}, fmt.Errorf("no list item found for list_id %s and item_id %s", listID, itemID)
		}
		r.logger.ErrorContext(ctx, "Failed to get list item by ID", slog.Any("error", err))
		return types.ListItem{}, fmt.Errorf("failed to get list item: %w", err)
	}
	return item, nil
}

// DeleteListItemByID deletes a specific item from a list using generic item_id
func (r *RepositoryImpl) DeleteListItemByID(ctx context.Context, listID, itemID uuid.UUID) error {
	query := `DELETE FROM list_items WHERE list_id = $1 AND item_id = $2`
	result, err := r.pgpool.Exec(ctx, query, listID, itemID)
	if err != nil {
		r.logger.ErrorContext(ctx, "Failed to delete list item by ID", slog.Any("error", err))
		return fmt.Errorf("failed to delete list item: %w", err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("no list item found for list_id %s and item_id %s", listID, itemID)
	}
	return nil
}
