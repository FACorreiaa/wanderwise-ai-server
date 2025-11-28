package discover

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/FACorreiaa/go-poi-au-suggestions/internal/types"
)

type Repository interface {
	// Get trending discoveries
	GetTrendingDiscoveries(ctx context.Context, limit int) ([]types.TrendingDiscovery, error)

	// Get featured collections
	GetFeaturedCollections(ctx context.Context, limit int) ([]types.FeaturedCollection, error)

	// Get user's recent discoveries
	GetRecentDiscoveriesByUserID(ctx context.Context, userID uuid.UUID, limit int) ([]types.ChatSession, error)

	// Get POIs by category
	GetPOIsByCategory(ctx context.Context, category string) ([]types.DiscoverResult, error)

	// Get trending searches today
	GetTrendingSearchesToday(ctx context.Context, limit int) ([]types.TrendingSearch, error)

	// Track a discover search
	TrackSearch(ctx context.Context, userID uuid.UUID, query, cityName, source string, resultCount int) error
}

type RepositoryImpl struct {
	db     *pgxpool.Pool
	logger *slog.Logger
}

func NewRepositoryImpl(db *pgxpool.Pool, logger *slog.Logger) *RepositoryImpl {
	return &RepositoryImpl{
		db:     db,
		logger: logger,
	}
}

// GetTrendingDiscoveries retrieves trending discoveries based on recent search activity
func (r *RepositoryImpl) GetTrendingDiscoveries(ctx context.Context, limit int) ([]types.TrendingDiscovery, error) {
	l := r.logger.With(slog.String("repository", "GetTrendingDiscoveries"))
	l.DebugContext(ctx, "Fetching trending discoveries", slog.Int("limit", limit))

	query := `
		SELECT
			city_name,
			COUNT(*) as search_count,
			MAX(created_at) as last_search
		FROM chat_sessions
		WHERE
			created_at >= NOW() - INTERVAL '7 days'
			AND city_name IS NOT NULL
			AND city_name != ''
		GROUP BY city_name
		ORDER BY search_count DESC, last_search DESC
		LIMIT $1
	`

	rows, err := r.db.Query(ctx, query, limit)
	if err != nil {
		l.ErrorContext(ctx, "Failed to query trending discoveries", slog.Any("error", err))
		return nil, fmt.Errorf("failed to query trending discoveries: %w", err)
	}
	defer rows.Close()

	var trending []types.TrendingDiscovery
	for rows.Next() {
		var t types.TrendingDiscovery
		var lastSearch time.Time
		err := rows.Scan(&t.CityName, &t.SearchCount, &lastSearch)
		if err != nil {
			l.ErrorContext(ctx, "Failed to scan trending discovery", slog.Any("error", err))
			continue
		}
		t.Emoji = getCityEmoji(t.CityName)
		trending = append(trending, t)
	}

	if err := rows.Err(); err != nil {
		l.ErrorContext(ctx, "Error iterating trending discoveries", slog.Any("error", err))
		return nil, fmt.Errorf("error iterating trending discoveries: %w", err)
	}

	l.InfoContext(ctx, "Successfully fetched trending discoveries", slog.Int("count", len(trending)))
	return trending, nil
}

// GetFeaturedCollections retrieves featured collections
func (r *RepositoryImpl) GetFeaturedCollections(ctx context.Context, limit int) ([]types.FeaturedCollection, error) {
	l := r.logger.With(slog.String("repository", "GetFeaturedCollections"))
	l.DebugContext(ctx, "Fetching featured collections", slog.Int("limit", limit))

	query := `
		SELECT
			category,
			COUNT(*) as item_count,
			MAX(created_at) as last_updated
		FROM poi_detailed_info
		WHERE
			rating >= 4.0
			AND category IS NOT NULL
			AND category != ''
		GROUP BY category
		ORDER BY item_count DESC
		LIMIT $1
	`

	rows, err := r.db.Query(ctx, query, limit)
	if err != nil {
		l.ErrorContext(ctx, "Failed to query featured collections", slog.Any("error", err))
		return nil, fmt.Errorf("failed to query featured collections: %w", err)
	}
	defer rows.Close()

	var featured []types.FeaturedCollection
	for rows.Next() {
		var f types.FeaturedCollection
		var lastUpdated time.Time
		err := rows.Scan(&f.Category, &f.ItemCount, &lastUpdated)
		if err != nil {
			l.ErrorContext(ctx, "Failed to scan featured collection", slog.Any("error", err))
			continue
		}
		f.Title = getCategoryTitle(f.Category)
		f.Emoji = getCategoryEmoji(f.Category)
		featured = append(featured, f)
	}

	if err := rows.Err(); err != nil {
		l.ErrorContext(ctx, "Error iterating featured collections", slog.Any("error", err))
		return nil, fmt.Errorf("error iterating featured collections: %w", err)
	}

	l.InfoContext(ctx, "Successfully fetched featured collections", slog.Int("count", len(featured)))
	return featured, nil
}

// GetRecentDiscoveriesByUserID retrieves user's recent discover searches
func (r *RepositoryImpl) GetRecentDiscoveriesByUserID(ctx context.Context, userID uuid.UUID, limit int) ([]types.ChatSession, error) {
	l := r.logger.With(slog.String("repository", "GetRecentDiscoveriesByUserID"))
	l.DebugContext(ctx, "Fetching recent discoveries",
		slog.String("user_id", userID.String()),
		slog.Int("limit", limit))

	query := `
		SELECT
			id,
			user_id,
			profile_id,
			city_name,
			conversation_history,
			session_context,
			created_at,
			updated_at,
			expires_at,
			status
		FROM chat_sessions
		WHERE
			user_id = $1
		ORDER BY created_at DESC
		LIMIT $2
	`

	rows, err := r.db.Query(ctx, query, userID, limit)
	if err != nil {
		l.ErrorContext(ctx, "Failed to query recent discoveries", slog.Any("error", err))
		return nil, fmt.Errorf("failed to query recent discoveries: %w", err)
	}
	defer rows.Close()

	var sessions []types.ChatSession
	for rows.Next() {
		var session types.ChatSession
		var conversationHistory []byte
		var sessionContext []byte
		var profileID sql.NullString

		err := rows.Scan(
			&session.ID,
			&session.UserID,
			&profileID,
			&session.CityName,
			&conversationHistory,
			&sessionContext,
			&session.CreatedAt,
			&session.UpdatedAt,
			&session.ExpiresAt,
			&session.Status,
		)
		if err != nil {
			l.ErrorContext(ctx, "Failed to scan chat session", slog.Any("error", err))
			continue
		}

		if profileID.Valid {
			if pid, err := uuid.Parse(profileID.String); err == nil {
				session.ProfileID = pid
			}
		}

		// Parse conversation history JSON
		if len(conversationHistory) > 0 {
			// You may need to implement JSON parsing here based on your types
			// For now, we'll leave it empty
			session.ConversationHistory = []types.ConversationMessage{}
		}

		// Parse session context JSON
		if len(sessionContext) > 0 {
			// You may need to implement JSON parsing here based on your types
			session.SessionContext = types.SessionContext{}
		}

		sessions = append(sessions, session)
	}

	if err := rows.Err(); err != nil {
		l.ErrorContext(ctx, "Error iterating recent discoveries", slog.Any("error", err))
		return nil, fmt.Errorf("error iterating recent discoveries: %w", err)
	}

	l.InfoContext(ctx, "Successfully fetched recent discoveries",
		slog.String("user_id", userID.String()),
		slog.Int("count", len(sessions)))
	return sessions, nil
}

// GetPOIsByCategory retrieves POIs by category
func (r *RepositoryImpl) GetPOIsByCategory(ctx context.Context, category string) ([]types.DiscoverResult, error) {
	l := r.logger.With(slog.String("repository", "GetPOIsByCategory"))
	l.DebugContext(ctx, "Fetching POIs by category", slog.String("category", category))

	query := `
		SELECT
			id,
			name,
			latitude,
			longitude,
			category,
			description_poi,
			address,
			website,
			phone_number,
			price_level,
			rating,
			tags
		FROM poi_detailed_info
		WHERE
			LOWER(category) = LOWER($1)
			AND rating >= 3.5
		ORDER BY rating DESC, name ASC
		LIMIT 20
	`

	rows, err := r.db.Query(ctx, query, category)
	if err != nil {
		l.ErrorContext(ctx, "Failed to query POIs by category", slog.Any("error", err))
		return nil, fmt.Errorf("failed to query POIs by category: %w", err)
	}
	defer rows.Close()

	var results []types.DiscoverResult
	for rows.Next() {
		var result types.DiscoverResult
		var website sql.NullString
		var phoneNumber sql.NullString
		var tags []string

		err := rows.Scan(
			&result.ID,
			&result.Name,
			&result.Latitude,
			&result.Longitude,
			&result.Category,
			&result.Description,
			&result.Address,
			&website,
			&phoneNumber,
			&result.PriceLevel,
			&result.Rating,
			&tags,
		)
		if err != nil {
			l.ErrorContext(ctx, "Failed to scan POI", slog.Any("error", err))
			continue
		}

		if website.Valid {
			result.Website = &website.String
		}
		if phoneNumber.Valid {
			result.PhoneNumber = &phoneNumber.String
		}
		result.Tags = tags

		results = append(results, result)
	}

	if err := rows.Err(); err != nil {
		l.ErrorContext(ctx, "Error iterating POIs", slog.Any("error", err))
		return nil, fmt.Errorf("error iterating POIs: %w", err)
	}

	l.InfoContext(ctx, "Successfully fetched POIs by category",
		slog.String("category", category),
		slog.Int("count", len(results)))
	return results, nil
}

// Helper functions

func getCityEmoji(cityName string) string {
	cityEmojiMap := map[string]string{
		"lisbon":     "🇵🇹",
		"porto":      "🇵🇹",
		"paris":      "🇫🇷",
		"london":     "🇬🇧",
		"tokyo":      "🇯🇵",
		"new york":   "🗽",
		"barcelona":  "🇪🇸",
		"amsterdam":  "🇳🇱",
		"rome":       "🇮🇹",
		"berlin":     "🇩🇪",
		"singapore":  "🇸🇬",
		"dubai":      "🇦🇪",
		"sydney":     "🇦🇺",
		"san francisco": "🌉",
	}

	if emoji, ok := cityEmojiMap[cityName]; ok {
		return emoji
	}
	return "🌍" // Default globe emoji
}

func getCategoryTitle(category string) string {
	titleMap := map[string]string{
		"restaurant":    "Top Restaurants",
		"hotel":         "Best Hotels",
		"activity":      "Popular Activities",
		"attraction":    "Must-See Attractions",
		"museum":        "Museums & Galleries",
		"park":          "Parks & Gardens",
		"beach":         "Beautiful Beaches",
		"nightlife":     "Nightlife Spots",
		"shopping":      "Shopping Destinations",
		"cultural":      "Cultural Experiences",
		"market":        "Local Markets",
		"adventure":     "Adventure Activities",
	}

	if title, ok := titleMap[category]; ok {
		return title
	}
	return fmt.Sprintf("Best %ss", category)
}

func getCategoryEmoji(category string) string {
	emojiMap := map[string]string{
		"restaurant":    "🍽️",
		"hotel":         "🏨",
		"activity":      "🎯",
		"attraction":    "🏛️",
		"museum":        "🎨",
		"park":          "🌳",
		"beach":         "🏖️",
		"nightlife":     "🌃",
		"shopping":      "🛍️",
		"cultural":      "🎭",
		"market":        "🏪",
		"adventure":     "⛰️",
		"cafe":          "☕",
		"bar":           "🍺",
		"entertainment": "🎪",
	}

	if emoji, ok := emojiMap[category]; ok {
		return emoji
	}
	return "📍"
}

// GetTrendingSearchesToday retrieves the most searched queries today
func (r *RepositoryImpl) GetTrendingSearchesToday(ctx context.Context, limit int) ([]types.TrendingSearch, error) {
	l := r.logger.With(slog.String("repository", "GetTrendingSearchesToday"))
	l.DebugContext(ctx, "Fetching trending searches today", slog.Int("limit", limit))

	query := `
		SELECT
			query,
			city_name,
			COUNT(*) as search_count,
			MAX(created_at) as last_searched
		FROM discover_searches
		WHERE
			created_at >= NOW() - INTERVAL '24 hours'
			AND query IS NOT NULL
			AND query != ''
			AND city_name IS NOT NULL
			AND city_name != ''
		GROUP BY query, city_name
		ORDER BY search_count DESC, last_searched DESC
		LIMIT $1
	`

	rows, err := r.db.Query(ctx, query, limit)
	if err != nil {
		l.ErrorContext(ctx, "Failed to query trending searches", slog.Any("error", err))
		return nil, fmt.Errorf("failed to query trending searches: %w", err)
	}
	defer rows.Close()

	var searches []types.TrendingSearch
	for rows.Next() {
		var search types.TrendingSearch
		var lastSearched time.Time

		err := rows.Scan(
			&search.Query,
			&search.CityName,
			&search.SearchCount,
			&lastSearched,
		)
		if err != nil {
			l.ErrorContext(ctx, "Failed to scan trending search", slog.Any("error", err))
			continue
		}

		// Format last searched as human-readable time
		search.LastSearched = formatTimeAgo(lastSearched)
		searches = append(searches, search)
	}

	if err := rows.Err(); err != nil {
		l.ErrorContext(ctx, "Error iterating trending searches", slog.Any("error", err))
		return nil, fmt.Errorf("error iterating trending searches: %w", err)
	}

	l.InfoContext(ctx, "Successfully fetched trending searches", slog.Int("count", len(searches)))
	return searches, nil
}

// TrackSearch records a discover search for trending analysis
func (r *RepositoryImpl) TrackSearch(ctx context.Context, userID uuid.UUID, query, cityName, source string, resultCount int) error {
	l := r.logger.With(slog.String("repository", "TrackSearch"))

	insertQuery := `
		INSERT INTO discover_searches (
			user_id,
			query,
			city_name,
			search_type,
			result_count,
			source,
			created_at
		) VALUES ($1, $2, $3, $4, $5, $6, NOW())
	`

	// Use NULL for anonymous users
	var userIDParam interface{}
	if userID == uuid.Nil {
		userIDParam = nil
	} else {
		userIDParam = userID
	}

	_, err := r.db.Exec(ctx, insertQuery, userIDParam, query, cityName, "discover", resultCount, source)
	if err != nil {
		l.ErrorContext(ctx, "Failed to track search",
			slog.Any("error", err),
			slog.String("query", query),
			slog.String("city", cityName))
		// Don't fail the request if tracking fails
		return nil
	}

	l.DebugContext(ctx, "Search tracked successfully",
		slog.String("query", query),
		slog.String("city", cityName),
		slog.String("source", source),
		slog.Int("result_count", resultCount))

	return nil
}

// formatTimeAgo formats a time as a human-readable "time ago" string
func formatTimeAgo(t time.Time) string {
	duration := time.Since(t)

	switch {
	case duration < time.Minute:
		return "just now"
	case duration < time.Hour:
		minutes := int(duration.Minutes())
		if minutes == 1 {
			return "1 minute ago"
		}
		return fmt.Sprintf("%d minutes ago", minutes)
	case duration < 24*time.Hour:
		hours := int(duration.Hours())
		if hours == 1 {
			return "1 hour ago"
		}
		return fmt.Sprintf("%d hours ago", hours)
	case duration < 7*24*time.Hour:
		days := int(duration.Hours() / 24)
		if days == 1 {
			return "1 day ago"
		}
		return fmt.Sprintf("%d days ago", days)
	default:
		return t.Format("Jan 2")
	}
}
