package tags

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
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

// Repository tagsRepo defines the contract for user data persistence.
type Repository interface {
	// GetAll --- Global Tags & User Avoid Tags ---
	// GetAll retrieves all global tags
	GetAll(ctx context.Context, userID uuid.UUID) ([]*types.Tags, error)

	// Get retrieves all avoid tags for a user
	Get(ctx context.Context, userID, tagID uuid.UUID) (*types.Tags, error)

	// Create adds an avoid tag for a user
	Create(ctx context.Context, userID uuid.UUID, params types.CreatePersonalTagParams) (*types.PersonalTag, error)

	// Delete removes an avoid tag for a user
	Delete(ctx context.Context, userID uuid.UUID, tagID uuid.UUID) error

	// Update updates on tag
	Update(ctx context.Context, userID, tagsID uuid.UUID, params types.UpdatePersonalTagParams) error

	// GetTagByName retrieves a tag by name.
	GetTagByName(ctx context.Context, name string) (*types.Tags, error)

	// LinkPersonalTagToProfile links a tag to a profile.
	LinkPersonalTagToProfile(ctx context.Context, userID, profileID uuid.UUID, tagID uuid.UUID) error

	// GetTagsForProfile retrieves all tags associated with a profile
	GetTagsForProfile(ctx context.Context, profileID uuid.UUID) ([]*types.Tags, error)
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

// GetAll implements user.UserRepo.
func (r *RepositoryImpl) GetAll(ctx context.Context, userID uuid.UUID) ([]*types.Tags, error) {
	ctx, span := otel.Tracer("UserRepo").Start(ctx, "GetAllGlobalTags", trace.WithAttributes(
		semconv.DBSystemPostgreSQL,
		attribute.String("db.sql.table", "global_tags"),
	))
	defer span.End()

	l := r.logger.With(slog.String("method", "GetAllGlobalTags"))
	l.DebugContext(ctx, "Fetching all active global tags")

	query := `
        SELECT
            g.id,
            g.name,
            g.description,
            g.tag_type,
            'global' AS source, 
			CASE WHEN 'global' = 'global' THEN false ELSE g.active END AS active,
            g.created_at        
        FROM global_tags g
        WHERE g.active = TRUE

        UNION ALL

        -- Select User Personal Tags
        SELECT
            upt.id,
            upt.name,
            NULL AS description, 
            upt.tag_type,
            'personal' AS source, 
			active,
            upt.created_at
        FROM user_personal_tags upt 
        WHERE upt.user_id = $1

        ORDER BY name`

	rows, err := r.pgpool.Query(ctx, query, userID)
	if err != nil {
		l.ErrorContext(ctx, "Failed to query global tags", slog.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "DB query failed")
		return nil, fmt.Errorf("database error fetching global tags: %w", err)
	}
	defer rows.Close()

	var tags []*types.Tags
	for rows.Next() {
		var t types.Tags
		err := rows.Scan(
			&t.ID,
			&t.Name,
			&t.Description,
			&t.TagType,
			&t.Source,
			&t.Active,
			&t.CreatedAt,
		)
		if err != nil {
			l.ErrorContext(ctx, "Failed to scan global tag row", slog.Any("error", err))
			span.RecordError(err)
			return nil, fmt.Errorf("database error scanning global tag: %w", err)
		}
		tags = append(tags, &t)
	}

	if err = rows.Err(); err != nil {
		l.ErrorContext(ctx, "Error iterating global tag rows", slog.Any("error", err))
		span.RecordError(err)
		return nil, fmt.Errorf("database error reading global tags: %w", err)
	}

	l.DebugContext(ctx, "Fetched all active global tags successfully", slog.Int("count", len(tags)))
	span.SetStatus(codes.Ok, "Global tags fetched")
	return tags, nil
}

// Get implements user.UserRepo.
func (r *RepositoryImpl) Get(ctx context.Context, userID, tagID uuid.UUID) (*types.Tags, error) {
	var tag types.Tags
	ctx, span := otel.Tracer("UserRepo").Start(ctx, "GetUserAvoidTags", trace.WithAttributes(
		semconv.DBSystemPostgreSQL,
		attribute.String("db.sql.table", "user_avoid_tags, global_tags"),
		attribute.String("db.user.id", userID.String()),
	))
	defer span.End()

	l := r.logger.With(slog.String("method", "GetUserAvoidTags"), slog.String("userID", userID.String()))
	l.DebugContext(ctx, "Fetching user avoid tags")

	query := `
    SELECT id, name, description, tag_type, source, created_at
    FROM (
        -- Select potential Global Tag
        SELECT
            g.id,
            g.name,
            g.description,
            g.tag_type,
            'global' AS source,
            g.created_at
        FROM global_tags g
        WHERE g.active = TRUE

        UNION ALL

        SELECT
            upt.id,
            upt.name,
            upt.description, -- Use upt.description here
            upt.tag_type,
            'personal' AS source,
            upt.created_at
        FROM user_personal_tags upt
        WHERE upt.user_id = $1 
    ) AS combined_tags
    WHERE combined_tags.id = $2 -- Filter the combined set by the specific tag_id LAST`

	err := r.pgpool.QueryRow(ctx, query, userID, tagID).Scan(
		&tag.ID,
		&tag.Name,
		&tag.Description,
		&tag.Source,
		&tag.TagType,
		&tag.CreatedAt,
	)
	if err != nil {
		l.ErrorContext(ctx, "Failed to query user avoid tags", slog.Any("error", err))
		span.SetStatus(codes.Error, "DB query failed")
		return nil, fmt.Errorf("database error fetching avoid tags: %w", err)
	}

	l.DebugContext(ctx, "Fetched user avoid tags successfully")
	span.SetStatus(codes.Ok, "Avoid tags fetched")
	return &tag, nil
}

// Create creates a new personal tag for a specific user.
func (r *RepositoryImpl) Create(ctx context.Context, userID uuid.UUID, params types.CreatePersonalTagParams) (*types.PersonalTag, error) {
	ctx, span := otel.Tracer("tagsRepo").Start(ctx, "CreatePersonalTag", trace.WithAttributes(
		semconv.DBSystemPostgreSQL,
		attribute.String("db.operation", "INSERT"),
		attribute.String("db.sql.table", "user_personal_tags"),
		attribute.String("db.user.id", userID.String()),
	))
	defer span.End()

	tx, err := r.pgpool.Begin(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Begin transaction failed")
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		if rollbackErr := tx.Rollback(ctx); rollbackErr != nil {
			r.logger.ErrorContext(ctx, "Failed to rollback transaction", slog.Any("error", rollbackErr))
		}
	}() // Rollback if commit is not successful

	l := r.logger.With(
		slog.String("method", "CreatePersonalTag"),
		slog.String("userID", userID.String()),
		slog.String("tagName", params.Name),
		slog.String("tagType", params.TagType),
	)
	l.DebugContext(ctx, "Creating user personal tag")

	newTagID := uuid.New()
	now := time.Now()

	tag := &types.PersonalTag{
		ID:          newTagID,
		UserID:      userID,
		Name:        params.Name,
		TagType:     params.TagType,
		Description: &params.Description,
		Source:      "personal",
		CreatedAt:   now,
	}

	query := `
        INSERT INTO user_personal_tags (id, user_id, name, tag_type, description, created_at)
        VALUES ($1, $2, $3, $4, $5, $6)
    `
	// Note: No ON CONFLICT here. We expect a unique constraint (user_id, name)
	// on the table and will handle the specific error if it occurs.

	_, err = tx.Exec(ctx, query, tag.ID, tag.UserID, tag.Name, tag.TagType, tag.Description, tag.CreatedAt)
	if err != nil {
		span.RecordError(err)

		l.ErrorContext(ctx, "Failed to insert user personal tag", slog.Any("error", err))
		span.SetStatus(codes.Error, "DB INSERT failed")
		return nil, fmt.Errorf("database error creating personal tag: %w", err)
	}

	err = tx.Commit(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Commit transaction failed")
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	l.InfoContext(ctx, "User personal tag created successfully", slog.String("tagID", tag.ID.String()))
	span.SetStatus(codes.Ok, "Personal tag created")
	return tag, nil
}

// Update updates the name and/or type of an existing personal tag for a specific user.
func (r *RepositoryImpl) Update(ctx context.Context, userID, tagsID uuid.UUID, params types.UpdatePersonalTagParams) error {
	ctx, span := otel.Tracer("tagsRepo").Start(ctx, "UpdatePersonalTag", trace.WithAttributes(
		semconv.DBSystemPostgreSQL,
		attribute.String("db.operation", "UPDATE"),
		attribute.String("db.sql.table", "user_personal_tags"),
	))
	defer span.End()

	tx, err := r.pgpool.Begin(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Begin transaction failed")
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		if rollbackErr := tx.Rollback(ctx); rollbackErr != nil {
			r.logger.ErrorContext(ctx, "Failed to rollback transaction", slog.Any("error", rollbackErr))
		}
	}()

	l := r.logger.With(
		slog.String("method", "UpdatePersonalTag"),
		slog.String("userID", userID.String()),
		slog.String("tagID", tagsID.String()),
		slog.String("newName", params.Name),
		slog.String("newTagType", params.TagType),
	)
	l.DebugContext(ctx, "Updating user personal tag")

	query := `
        UPDATE user_personal_tags
        SET name = $1, tag_type = $2, active = $3, updated_at = $4
        WHERE id = $5 AND user_id = $6
    `
	now := time.Now()

	cmdTag, err := tx.Exec(ctx, query, params.Name, params.TagType, params.Active, now, tagsID, userID)
	if err != nil {
		span.RecordError(err)
		l.ErrorContext(ctx, "Failed to update user personal tag", slog.Any("error", err))
		span.SetStatus(codes.Error, "DB UPDATE failed")
		return fmt.Errorf("database error updating personal tag: %w", err)
	}

	if cmdTag.RowsAffected() == 0 {
		l.WarnContext(ctx, "Attempted to update non-existent or unauthorized personal tag")
		span.SetStatus(codes.Error, "Tag not found or not owned by user")
		// It didn't exist OR didn't belong to the user, return NotFound
		return fmt.Errorf("personal tag not found or not owned by user: %w", types.ErrNotFound)
	}

	err = tx.Commit(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Commit transaction failed")
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	l.InfoContext(ctx, "User personal tag updated successfully")
	span.SetStatus(codes.Ok, "Personal tag updated")
	return nil
}

// Delete deletes a specific personal tag belonging to a user.
func (r *RepositoryImpl) Delete(ctx context.Context, userID uuid.UUID, tagID uuid.UUID) error {
	ctx, span := otel.Tracer("tagsRepo").Start(ctx, "DeletePersonalTag", trace.WithAttributes(
		semconv.DBSystemPostgreSQL,
		attribute.String("db.operation", "DELETE"),
		attribute.String("db.sql.table", "user_personal_tags"),
		attribute.String("db.user.id", userID.String()),
		attribute.String("db.tag.id", tagID.String()),
	))
	defer span.End()

	tx, err := r.pgpool.Begin(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Begin transaction failed")
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		if rollbackErr := tx.Rollback(ctx); rollbackErr != nil {
			r.logger.ErrorContext(ctx, "Failed to rollback transaction", slog.Any("error", rollbackErr))
		}
	}()

	l := r.logger.With(slog.String("method", "DeletePersonalTag"), slog.String("userID", userID.String()), slog.String("tagID", tagID.String()))
	l.DebugContext(ctx, "Deleting user personal tag")

	query := `
        DELETE FROM user_personal_tags
        WHERE id = $1 AND user_id = $2
    `
	cmdTag, err := tx.Exec(ctx, query, tagID, userID)
	if err != nil {
		// Note: DELETE typically won't cause unique or foreign key violations unless triggers exist.
		l.ErrorContext(ctx, "Failed to delete user personal tag", slog.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "DB DELETE failed")
		return fmt.Errorf("database error deleting personal tag: %w", err)
	}

	if cmdTag.RowsAffected() == 0 {
		l.WarnContext(ctx, "Attempted to delete non-existent or unauthorized personal tag")
		span.SetStatus(codes.Error, "Tag not found or not owned by user")
		// It didn't exist OR didn't belong to the user
		return fmt.Errorf("personal tag not found or not owned by user: %w", types.ErrNotFound)
	}

	err = tx.Commit(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Commit transaction failed")
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	l.InfoContext(ctx, "User personal tag deleted successfully")
	span.SetStatus(codes.Ok, "Personal tag deleted")
	return nil
}

// GetTagByName retrieves a tag by name.
func (r *RepositoryImpl) GetTagByName(ctx context.Context, name string) (*types.Tags, error) {
	ctx, span := otel.Tracer("UserRepo").Start(ctx, "GetTagByName", trace.WithAttributes(
		semconv.DBSystemPostgreSQL,
		attribute.String("db.operation", "SELECT"),
		attribute.String("db.sql.table", "global_tags"),
		attribute.String("tag.name", name),
	))
	defer span.End()

	l := r.logger.With(slog.String("method", "GetTagByName"), slog.String("name", name))
	l.DebugContext(ctx, "Fetching tag by name")

	query := `
        SELECT id, name, description, tag_type, created_at
        FROM global_tags
        WHERE name = $1 AND active = TRUE`

	var tag types.Tags
	err := r.pgpool.QueryRow(ctx, query, name).Scan(
		&tag.ID,
		&tag.Name,
		&tag.Description,
		&tag.TagType,
		&tag.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			l.WarnContext(ctx, "Tag not found", slog.String("name", name))
			span.SetStatus(codes.Error, "Tag not found")
			return nil, types.ErrNotFound
		}
		l.ErrorContext(ctx, "Failed to fetch tag by name", slog.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "DB query failed")
		return nil, fmt.Errorf("database error fetching tag: %w", err)
	}

	l.DebugContext(ctx, "Fetched tag by name successfully", slog.String("tagID", tag.ID.String()))
	span.SetStatus(codes.Ok, "Tag fetched")
	return &tag, nil
}

// LinkPersonalTagToProfile links a tag to a profile.
func (r *RepositoryImpl) LinkPersonalTagToProfile(ctx context.Context, userID, profileID, tagID uuid.UUID) error {
	ctx, span := otel.Tracer("UserRepo").Start(ctx, "AddTagToProfile", trace.WithAttributes(
		semconv.DBSystemPostgreSQL,
		attribute.String("db.operation", "INSERT"),
		attribute.String("db.sql.table", "user_personal_tags"),
		attribute.String("db.profile.id", profileID.String()),
		attribute.String("db.tag.id", tagID.String()),
	))
	defer span.End()

	l := r.logger.With(slog.String("method", "AddTagToProfile"), slog.String("profileID", profileID.String()), slog.String("tagID", tagID.String()))
	l.DebugContext(ctx, "Linking tag to profile")

	query := `UPDATE user_personal_tags
              SET profile_id = $1, updated_at = NOW()
              WHERE id = $2 AND user_id = $3` // Ensure ownership
	tag, err := r.pgpool.Exec(ctx, query, profileID, tagID, userID)
	if err != nil {
		l.ErrorContext(ctx, "Failed to link tag to profile", slog.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "DB INSERT failed")
		return fmt.Errorf("database error linking tag to profile: %w", err)
	}

	if tag.RowsAffected() == 0 {
		return fmt.Errorf("personal tag %s not found for user %s: %w", tagID, userID, types.ErrNotFound)
	}

	l.DebugContext(ctx, "Tag linked to profile successfully")
	span.SetStatus(codes.Ok, "Tag linked")
	return nil
}

// GetTagsForProfile retrieves all tags associated with a profile
func (r *RepositoryImpl) GetTagsForProfile(ctx context.Context, profileID uuid.UUID) ([]*types.Tags, error) {
	ctx, span := otel.Tracer("UserRepo").Start(ctx, "GetTagsForProfile", trace.WithAttributes(
		semconv.DBSystemPostgreSQL,
		attribute.String("db.operation", "SELECT"),
		attribute.String("db.sql.table", "global_tags"),
		attribute.String("db.profile.id", profileID.String()),
	))
	defer span.End()

	l := r.logger.With(slog.String("method", "GetTagsForProfile"), slog.String("profileID", profileID.String()))
	l.DebugContext(ctx, "Fetching tags for profile")

	query := `
        SELECT g.id, g.name, g.tag_type, g.description, g.created_at
        FROM global_tags g
        JOIN user_personal_tags upt ON g.id = upt.id
        WHERE upt.profile_id = $1`

	rows, err := r.pgpool.Query(ctx, query, profileID)
	if err != nil {
		l.ErrorContext(ctx, "Failed to query tags for profile", slog.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "DB query failed")
		return nil, fmt.Errorf("database error fetching tags for profile: %w", err)
	}
	defer rows.Close()

	var tags []*types.Tags
	for rows.Next() {
		var tag types.Tags
		err := rows.Scan(
			&tag.ID,
			&tag.Name,
			&tag.TagType,
			&tag.Description,
			&tag.CreatedAt,
			&tag.UpdatedAt,
		)
		if err != nil {
			l.ErrorContext(ctx, "Failed to scan tag row", slog.Any("error", err))
			span.RecordError(err)
			return nil, fmt.Errorf("database error scanning tag: %w", err)
		}
		tags = append(tags, &tag)
	}

	if err = rows.Err(); err != nil {
		l.ErrorContext(ctx, "Error iterating tag rows", slog.Any("error", err))
		span.RecordError(err)
		return nil, fmt.Errorf("database error reading tags: %w", err)
	}

	l.DebugContext(ctx, "Fetched tags for profile successfully", slog.Int("count", len(tags)))
	span.SetStatus(codes.Ok, "Tags fetched")
	return tags, nil
}
