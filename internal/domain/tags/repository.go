package tags

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

var _ Repository = (*RepositoryImpl)(nil)

// Repository tagsRepo defines the contract for user data persistence.
type Repository interface {
	GetAll(ctx context.Context, userID uuid.UUID) ([]*Tags, error)
	Get(ctx context.Context, userID, tagID uuid.UUID) (*Tags, error)
	Create(ctx context.Context, userID uuid.UUID, params CreatePersonalTagParams) (*PersonalTag, error)
	Delete(ctx context.Context, userID uuid.UUID, tagID uuid.UUID) error
	Update(ctx context.Context, userID, tagsID uuid.UUID, params UpdatePersonalTagParams) error
	GetTagByName(ctx context.Context, name string) (*Tags, error)
	LinkPersonalTagToProfile(ctx context.Context, userID, profileID uuid.UUID, tagID uuid.UUID) error
	GetTagsForProfile(ctx context.Context, profileID uuid.UUID) ([]*Tags, error)
}

type RepositoryImpl struct {
	logger *zap.Logger
	pgpool *pgxpool.Pool
}

func NewRepositoryImpl(pgxpool *pgxpool.Pool, logger *zap.Logger) *RepositoryImpl {
	return &RepositoryImpl{
		logger: logger,
		pgpool: pgxpool,
	}
}

// GetAll implements user.UserRepo.
func (r *RepositoryImpl) GetAll(ctx context.Context, userID uuid.UUID) ([]*Tags, error) {
	ctx, span := otel.Tracer("UserRepo").Start(ctx, "GetAllGlobalTags", trace.WithAttributes(
		semconv.DBSystemPostgreSQL,
		attribute.String("db.sql.table", "global_tags"),
	))
	defer span.End()

	l := r.logger.With(zap.String("method", "GetAllGlobalTags"))
	l.Debug("Fetching all active global tags")

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
		l.Error("Failed to query global tags", zap.Error(err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "DB query failed")
		return nil, fmt.Errorf("database error fetching global tags: %w", err)
	}
	defer rows.Close()

	var tags []*Tags
	for rows.Next() {
		var t Tags
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
			l.Error("Failed to scan global tag row", zap.Error(err))
			span.RecordError(err)
			return nil, fmt.Errorf("database error scanning global tag: %w", err)
		}
		tags = append(tags, &t)
	}

	if err = rows.Err(); err != nil {
		l.Error("Error iterating global tag rows", zap.Error(err))
		span.RecordError(err)
		return nil, fmt.Errorf("database error reading global tags: %w", err)
	}

	l.Debug("Fetched all active global tags successfully", zap.Int("count", len(tags)))
	span.SetStatus(codes.Ok, "Global tags fetched")
	return tags, nil
}

// Get implements user.UserRepo.
func (r *RepositoryImpl) Get(ctx context.Context, userID, tagID uuid.UUID) (*Tags, error) {
	var tag Tags
	ctx, span := otel.Tracer("UserRepo").Start(ctx, "GetUserAvoidTags", trace.WithAttributes(
		semconv.DBSystemPostgreSQL,
		attribute.String("db.sql.table", "user_avoid_tags, global_tags"),
		attribute.String("db.user.id", userID.String()),
	))
	defer span.End()

	l := r.logger.With(zap.String("method", "GetUserAvoidTags"), zap.String("userID", userID.String()))
	l.Debug("Fetching user avoid tags")

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
		l.Error("Failed to query user avoid tags", zap.Error(err))
		span.SetStatus(codes.Error, "DB query failed")
		return nil, fmt.Errorf("database error fetching avoid tags: %w", err)
	}

	l.Debug("Fetched user avoid tags successfully")
	span.SetStatus(codes.Ok, "Avoid tags fetched")
	return &tag, nil
}

// Create creates a new personal tag for a specific user.
func (r *RepositoryImpl) Create(ctx context.Context, userID uuid.UUID, params CreatePersonalTagParams) (*PersonalTag, error) {
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
			r.logger.Error("Failed to rollback transaction", zap.Error(rollbackErr))
		}
	}() // Rollback if commit is not successful

	l := r.logger.With(
		zap.String("method", "CreatePersonalTag"),
		zap.String("userID", userID.String()),
		zap.String("tagName", params.Name),
		zap.String("tagType", params.TagType),
	)
	l.Debug("Creating user personal tag")

	newTagID := uuid.New()
	now := time.Now()

	tag := &PersonalTag{
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

		l.Error("Failed to insert user personal tag", zap.Error(err))
		span.SetStatus(codes.Error, "DB INSERT failed")
		return nil, fmt.Errorf("database error creating personal tag: %w", err)
	}

	err = tx.Commit(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Commit transaction failed")
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	l.Info("User personal tag created successfully", zap.String("tagID", tag.ID.String()))
	span.SetStatus(codes.Ok, "Personal tag created")
	return tag, nil
}

// Update updates the name and/or type of an existing personal tag for a specific user.
func (r *RepositoryImpl) Update(ctx context.Context, userID, tagsID uuid.UUID, params UpdatePersonalTagParams) error {
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
			r.logger.Error("Failed to rollback transaction", zap.Error(rollbackErr))
		}
	}()

	l := r.logger.With(
		zap.String("method", "UpdatePersonalTag"),
		zap.String("userID", userID.String()),
		zap.String("tagID", tagsID.String()),
		zap.String("newName", params.Name),
		zap.String("newTagType", params.TagType),
	)
	l.Debug("Updating user personal tag")

	query := `
        UPDATE user_personal_tags
        SET name = $1, tag_type = $2, active = $3, updated_at = $4
        WHERE id = $5 AND user_id = $6
    `
	now := time.Now()

	cmdTag, err := tx.Exec(ctx, query, params.Name, params.TagType, params.Active, now, tagsID, userID)
	if err != nil {
		span.RecordError(err)
		l.Error("Failed to update user personal tag", zap.Error(err))
		span.SetStatus(codes.Error, "DB UPDATE failed")
		return fmt.Errorf("database error updating personal tag: %w", err)
	}

	if cmdTag.RowsAffected() == 0 {
		l.Warn("Attempted to update non-existent or unauthorized personal tag")
		span.SetStatus(codes.Error, "Tag not found or not owned by user")
		// It didn't exist OR didn't belong to the user, return NotFound
		return fmt.Errorf("personal tag not found or not owned by user: %w", err)
	}

	err = tx.Commit(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Commit transaction failed")
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	l.Info("User personal tag updated successfully")
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
			r.logger.Error("Failed to rollback transaction", zap.Error(rollbackErr))
		}
	}()

	l := r.logger.With(zap.String("method", "DeletePersonalTag"), zap.String("userID", userID.String()), zap.String("tagID", tagID.String()))
	l.Debug("Deleting user personal tag")

	query := `
        DELETE FROM user_personal_tags
        WHERE id = $1 AND user_id = $2
    `
	cmdTag, err := tx.Exec(ctx, query, tagID, userID)
	if err != nil {
		// Note: DELETE typically won't cause unique or foreign key violations unless triggers exist.
		l.Error("Failed to delete user personal tag", zap.Error(err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "DB DELETE failed")
		return fmt.Errorf("database error deleting personal tag: %w", err)
	}

	if cmdTag.RowsAffected() == 0 {
		l.Warn("Attempted to delete non-existent or unauthorized personal tag")
		span.SetStatus(codes.Error, "Tag not found or not owned by user")
	}

	err = tx.Commit(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Commit transaction failed")
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	l.Info("User personal tag deleted successfully")
	span.SetStatus(codes.Ok, "Personal tag deleted")
	return nil
}

// GetTagByName retrieves a tag by name.
func (r *RepositoryImpl) GetTagByName(ctx context.Context, name string) (*Tags, error) {
	ctx, span := otel.Tracer("UserRepo").Start(ctx, "GetTagByName", trace.WithAttributes(
		semconv.DBSystemPostgreSQL,
		attribute.String("db.operation", "SELECT"),
		attribute.String("db.sql.table", "global_tags"),
		attribute.String("tag.name", name),
	))
	defer span.End()

	l := r.logger.With(zap.String("method", "GetTagByName"), zap.String("name", name))
	l.Debug("Fetching tag by name")

	query := `
        SELECT id, name, description, tag_type, created_at
        FROM global_tags
        WHERE name = $1 AND active = TRUE`

	var tag Tags
	err := r.pgpool.QueryRow(ctx, query, name).Scan(
		&tag.ID,
		&tag.Name,
		&tag.Description,
		&tag.TagType,
		&tag.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			l.Warn("Tag not found", zap.String("name", name))
			span.SetStatus(codes.Error, "Tag not found")
			return nil, err
		}
		l.Error("Failed to fetch tag by name", zap.Error(err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "DB query failed")
		return nil, fmt.Errorf("database error fetching tag: %w", err)
	}

	l.Debug("Fetched tag by name successfully", zap.String("tagID", tag.ID.String()))
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

	l := r.logger.With(zap.String("method", "AddTagToProfile"), zap.String("profileID", profileID.String()), zap.String("tagID", tagID.String()))
	l.Debug("Linking tag to profile")

	query := `UPDATE user_personal_tags
              SET profile_id = $1, updated_at = NOW()
              WHERE id = $2 AND user_id = $3` // Ensure ownership
	tag, err := r.pgpool.Exec(ctx, query, profileID, tagID, userID)
	if err != nil {
		l.Error("Failed to link tag to profile", zap.Error(err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "DB INSERT failed")
		return fmt.Errorf("database error linking tag to profile: %w", err)
	}

	if tag.RowsAffected() == 0 {
		return fmt.Errorf("personal tag %s not found for user %s: %w", tagID, userID, err)
	}

	l.Debug("Tag linked to profile successfully")
	span.SetStatus(codes.Ok, "Tag linked")
	return nil
}

// GetTagsForProfile retrieves all tags associated with a profile
func (r *RepositoryImpl) GetTagsForProfile(ctx context.Context, profileID uuid.UUID) ([]*Tags, error) {
	ctx, span := otel.Tracer("UserRepo").Start(ctx, "GetTagsForProfile", trace.WithAttributes(
		semconv.DBSystemPostgreSQL,
		attribute.String("db.operation", "SELECT"),
		attribute.String("db.sql.table", "global_tags"),
		attribute.String("db.profile.id", profileID.String()),
	))
	defer span.End()

	l := r.logger.With(zap.String("method", "GetTagsForProfile"), zap.String("profileID", profileID.String()))
	l.Debug("Fetching tags for profile")

	query := `
        SELECT g.id, g.name, g.tag_type, g.description, g.created_at
        FROM global_tags g
        JOIN user_personal_tags upt ON g.id = upt.id
        WHERE upt.profile_id = $1`

	rows, err := r.pgpool.Query(ctx, query, profileID)
	if err != nil {
		l.Error("Failed to query tags for profile", zap.Error(err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "DB query failed")
		return nil, fmt.Errorf("database error fetching tags for profile: %w", err)
	}
	defer rows.Close()

	var tags []*Tags
	for rows.Next() {
		var tag Tags
		err := rows.Scan(
			&tag.ID,
			&tag.Name,
			&tag.TagType,
			&tag.Description,
			&tag.CreatedAt,
			&tag.UpdatedAt,
		)
		if err != nil {
			l.Error("Failed to scan tag row", zap.Error(err))
			span.RecordError(err)
			return nil, fmt.Errorf("database error scanning tag: %w", err)
		}
		tags = append(tags, &tag)
	}

	if err = rows.Err(); err != nil {
		l.Error("Error iterating tag rows", zap.Error(err))
		span.RecordError(err)
		return nil, fmt.Errorf("database error reading tags: %w", err)
	}

	l.Debug("Fetched tags for profile successfully", zap.Int("count", len(tags)))
	span.SetStatus(codes.Ok, "Tags fetched")
	return tags, nil
}
