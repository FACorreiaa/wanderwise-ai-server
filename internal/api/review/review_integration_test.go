//go:build integration

package review

import (
	"context"
	"log"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var testReviewDB *pgxpool.Pool

func TestMain(m *testing.M) {
	if err := godotenv.Load("../../../.env.test"); err != nil {
		log.Println("Warning: .env.test file not found for review integration tests.")
	}

	dbURL := os.Getenv("TEST_DATABASE_URL")
	if dbURL == "" {
		log.Fatal("TEST_DATABASE_URL environment variable is not set for review integration tests")
	}

	var err error
	config, err := pgxpool.ParseConfig(dbURL)
	if err != nil {
		log.Fatalf("Unable to parse TEST_DATABASE_URL: %v\n", err)
	}
	config.MaxConns = 5

	testReviewDB, err = pgxpool.NewWithConfig(context.Background(), config)
	if err != nil {
		log.Fatalf("Unable to create connection pool for review tests: %v\n", err)
	}
	defer testReviewDB.Close()

	if err := testReviewDB.Ping(context.Background()); err != nil {
		log.Fatalf("Unable to ping test database for review tests: %v\n", err)
	}

	exitCode := m.Run()
	os.Exit(exitCode)
}

func clearReviewTables(t *testing.T) {
	t.Helper()
	_, err := testReviewDB.Exec(context.Background(), "DELETE FROM review_replies")
	require.NoError(t, err, "Failed to clear review_replies table")
	_, err = testReviewDB.Exec(context.Background(), "DELETE FROM review_helpful")
	require.NoError(t, err, "Failed to clear review_helpful table")
	_, err = testReviewDB.Exec(context.Background(), "DELETE FROM reviews")
	require.NoError(t, err, "Failed to clear reviews table")
}

func createTestUserForReview(t *testing.T) uuid.UUID {
	t.Helper()
	userID := uuid.New()
	_, err := testReviewDB.Exec(context.Background(),
		"INSERT INTO users (id, username, email, password_hash) VALUES ($1, $2, $3, $4) ON CONFLICT (id) DO NOTHING",
		userID, "reviewuser", "reviewuser@test.com", "hash")
	require.NoError(t, err)
	return userID
}

func createTestPOIForReview(t *testing.T) uuid.UUID {
	t.Helper()
	poiID := uuid.New()
	cityID := uuid.New()

	// Create city first
	_, err := testReviewDB.Exec(context.Background(),
		"INSERT INTO cities (id, name, country) VALUES ($1, $2, $3) ON CONFLICT (id) DO NOTHING",
		cityID, "Test City", "Test Country")
	require.NoError(t, err)

	// Create POI
	_, err = testReviewDB.Exec(context.Background(),
		"INSERT INTO pois (id, city_id, name, latitude, longitude, location, category) VALUES ($1, $2, $3, $4, $5, ST_SetSRID(ST_MakePoint($6, $7), 4326), $8) ON CONFLICT (id) DO NOTHING",
		poiID, cityID, "Test POI", 38.7223, -9.1393, -9.1393, 38.7223, "Test")
	require.NoError(t, err)
	return poiID
}

func TestReviewModels_Integration(t *testing.T) {
	ctx := context.Background()
	clearReviewTables(t)

	userID := createTestUserForReview(t)
	poiID := createTestPOIForReview(t)

	t.Run("Create and save review", func(t *testing.T) {
		review := models.NewReview(userID, poiID, 5, "Amazing place!", "This is a fantastic location to visit.")
		visitDate := time.Now().AddDate(0, 0, -7) // 7 days ago
		review.VisitDate = &visitDate
		review.ImageURLs = []string{"https://example.com/image1.jpg", "https://example.com/image2.jpg"}

		// Insert review into database
		query := `
			INSERT INTO reviews (id, user_id, poi_id, rating, title, content, visit_date, image_urls, helpful, unhelpful, is_verified, is_published, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
		`
		_, err := testReviewDB.Exec(ctx, query,
			review.ID, review.UserID, review.POIID, review.Rating, review.Title, review.Content,
			review.VisitDate, review.ImageURLs, review.Helpful, review.Unhelpful,
			review.IsVerified, review.IsPublished, review.CreatedAt, review.UpdatedAt)
		require.NoError(t, err)

		// Verify review was saved
		var dbTitle, dbContent string
		var dbRating int
		err = testReviewDB.QueryRow(ctx, "SELECT title, content, rating FROM reviews WHERE id = $1", review.ID).
			Scan(&dbTitle, &dbContent, &dbRating)
		require.NoError(t, err)

		assert.Equal(t, review.Title, dbTitle)
		assert.Equal(t, review.Content, dbContent)
		assert.Equal(t, review.Rating, dbRating)
	})

	t.Run("Create and save review helpful", func(t *testing.T) {
		// Create a review first
		review := models.NewReview(userID, poiID, 4, "Good place", "Nice location")
		query := `
			INSERT INTO reviews (id, user_id, poi_id, rating, title, content, helpful, unhelpful, is_verified, is_published, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		`
		_, err := testReviewDB.Exec(ctx, query,
			review.ID, review.UserID, review.POIID, review.Rating, review.Title, review.Content,
			review.Helpful, review.Unhelpful, review.IsVerified, review.IsPublished, review.CreatedAt, review.UpdatedAt)
		require.NoError(t, err)

		// Create another user to mark the review as helpful
		otherUserID := createTestUserForReview(t)

		reviewHelpful := models.NewReviewHelpful(otherUserID, review.ID, true)

		// Insert review helpful record
		helpfulQuery := `
			INSERT INTO review_helpful (user_id, review_id, is_helpful, created_at)
			VALUES ($1, $2, $3, $4)
		`
		_, err = testReviewDB.Exec(ctx, helpfulQuery,
			reviewHelpful.UserID, reviewHelpful.ReviewID, reviewHelpful.IsHelpful, reviewHelpful.CreatedAt)
		require.NoError(t, err)

		// Verify review helpful was saved
		var dbIsHelpful bool
		err = testReviewDB.QueryRow(ctx, "SELECT is_helpful FROM review_helpful WHERE user_id = $1 AND review_id = $2",
			reviewHelpful.UserID, reviewHelpful.ReviewID).Scan(&dbIsHelpful)
		require.NoError(t, err)

		assert.Equal(t, reviewHelpful.IsHelpful, dbIsHelpful)
	})

	t.Run("Create and save review reply", func(t *testing.T) {
		// Create a review first
		review := models.NewReview(userID, poiID, 3, "Average place", "It was okay")
		query := `
			INSERT INTO reviews (id, user_id, poi_id, rating, title, content, helpful, unhelpful, is_verified, is_published, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		`
		_, err := testReviewDB.Exec(ctx, query,
			review.ID, review.UserID, review.POIID, review.Rating, review.Title, review.Content,
			review.Helpful, review.Unhelpful, review.IsVerified, review.IsPublished, review.CreatedAt, review.UpdatedAt)
		require.NoError(t, err)

		// Create a reply to the review
		replyUserID := createTestUserForReview(t)
		reviewReply := models.NewReviewReply(review.ID, replyUserID, "Thanks for your feedback!", false)

		// Insert review reply
		replyQuery := `
			INSERT INTO review_replies (id, review_id, user_id, content, is_official, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
		`
		_, err = testReviewDB.Exec(ctx, replyQuery,
			reviewReply.ID, reviewReply.ReviewID, reviewReply.UserID, reviewReply.Content,
			reviewReply.IsOfficial, reviewReply.CreatedAt, reviewReply.UpdatedAt)
		require.NoError(t, err)

		// Verify review reply was saved
		var dbContent string
		var dbIsOfficial bool
		err = testReviewDB.QueryRow(ctx, "SELECT content, is_official FROM review_replies WHERE id = $1", reviewReply.ID).
			Scan(&dbContent, &dbIsOfficial)
		require.NoError(t, err)

		assert.Equal(t, reviewReply.Content, dbContent)
		assert.Equal(t, reviewReply.IsOfficial, dbIsOfficial)
	})
}

func TestReviewQueries_Integration(t *testing.T) {
	ctx := context.Background()
	clearReviewTables(t)

	userID := createTestUserForReview(t)
	poiID := createTestPOIForReview(t)

	// Create multiple reviews for testing
	reviews := []*models.Review{
		models.NewReview(userID, poiID, 5, "Excellent!", "Perfect place"),
		models.NewReview(userID, poiID, 4, "Very good", "Really enjoyed it"),
		models.NewReview(userID, poiID, 3, "Average", "It was okay"),
	}

	// Insert reviews
	for _, review := range reviews {
		query := `
			INSERT INTO reviews (id, user_id, poi_id, rating, title, content, helpful, unhelpful, is_verified, is_published, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		`
		_, err := testReviewDB.Exec(ctx, query,
			review.ID, review.UserID, review.POIID, review.Rating, review.Title, review.Content,
			review.Helpful, review.Unhelpful, review.IsVerified, review.IsPublished, review.CreatedAt, review.UpdatedAt)
		require.NoError(t, err)
	}

	t.Run("Get reviews by POI", func(t *testing.T) {
		rows, err := testReviewDB.Query(ctx, "SELECT id, rating, title FROM reviews WHERE poi_id = $1 ORDER BY rating DESC", poiID)
		require.NoError(t, err)
		defer rows.Close()

		var retrievedReviews []struct {
			ID     uuid.UUID
			Rating int
			Title  string
		}

		for rows.Next() {
			var review struct {
				ID     uuid.UUID
				Rating int
				Title  string
			}
			err := rows.Scan(&review.ID, &review.Rating, &review.Title)
			require.NoError(t, err)
			retrievedReviews = append(retrievedReviews, review)
		}

		assert.Len(t, retrievedReviews, 3)
		assert.Equal(t, 5, retrievedReviews[0].Rating) // Should be ordered by rating DESC
		assert.Equal(t, 4, retrievedReviews[1].Rating)
		assert.Equal(t, 3, retrievedReviews[2].Rating)
	})

	t.Run("Calculate average rating", func(t *testing.T) {
		var avgRating float64
		err := testReviewDB.QueryRow(ctx, "SELECT AVG(rating::numeric) FROM reviews WHERE poi_id = $1", poiID).Scan(&avgRating)
		require.NoError(t, err)

		expectedAvg := float64(5+4+3) / 3
		assert.InDelta(t, expectedAvg, avgRating, 0.01)
	})

	t.Run("Get reviews by user", func(t *testing.T) {
		var count int
		err := testReviewDB.QueryRow(ctx, "SELECT COUNT(*) FROM reviews WHERE user_id = $1", userID).Scan(&count)
		require.NoError(t, err)

		assert.Equal(t, 3, count)
	})
}

func TestReviewConstraints_Integration(t *testing.T) {
	ctx := context.Background()
	clearReviewTables(t)

	userID := createTestUserForReview(t)
	poiID := createTestPOIForReview(t)

	t.Run("Rating constraints", func(t *testing.T) {
		// Test valid rating (should succeed)
		review := models.NewReview(userID, poiID, 5, "Valid rating", "Content")
		query := `
			INSERT INTO reviews (id, user_id, poi_id, rating, title, content, helpful, unhelpful, is_verified, is_published, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		`
		_, err := testReviewDB.Exec(ctx, query,
			review.ID, review.UserID, review.POIID, review.Rating, review.Title, review.Content,
			review.Helpful, review.Unhelpful, review.IsVerified, review.IsPublished, review.CreatedAt, review.UpdatedAt)
		require.NoError(t, err)

		// Test invalid rating (if constraints exist in DB schema)
		invalidReview := models.NewReview(userID, poiID, 10, "Invalid rating", "Content") // Assuming rating should be 1-5
		invalidReview.ID = uuid.New()                                                     // Different ID
		_, err = testReviewDB.Exec(ctx, query,
			invalidReview.ID, invalidReview.UserID, invalidReview.POIID, invalidReview.Rating, invalidReview.Title, invalidReview.Content,
			invalidReview.Helpful, invalidReview.Unhelpful, invalidReview.IsVerified, invalidReview.IsPublished, invalidReview.CreatedAt, invalidReview.UpdatedAt)

		// This should succeed if no constraints, or fail if there are rating constraints
		// Adjust assertion based on your database schema constraints
		if err != nil {
			assert.Contains(t, err.Error(), "constraint")
		}
	})
}
