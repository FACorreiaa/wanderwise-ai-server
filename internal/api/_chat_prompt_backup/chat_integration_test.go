//go:build integration

package llmChat

import (
	"context"
	"log"
	"log/slog"
	"os"
	"testing"

	"github.com/FACorreiaa/go-poi-au-suggestions/internal/api/city"
	generativeAI "github.com/FACorreiaa/go-poi-au-suggestions/internal/api/generative_ai"
	"github.com/FACorreiaa/go-poi-au-suggestions/internal/api/interests"
	"github.com/FACorreiaa/go-poi-au-suggestions/internal/api/poi"
	"github.com/FACorreiaa/go-poi-au-suggestions/internal/api/profiles"
	"github.com/FACorreiaa/go-poi-au-suggestions/internal/api/tags"
	"github.com/FACorreiaa/go-poi-au-suggestions/internal/types"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var testChatDB *pgxpool.Pool
var testChatService LlmInteractiontService
var testChatRepo Repository

func TestMain(m *testing.M) {
	if err := godotenv.Load("../../../.env.test"); err != nil {
		log.Println("Warning: .env.test file not found for chat integration tests.")
	}

	dbURL := os.Getenv("TEST_DATABASE_URL")
	if dbURL == "" {
		log.Fatal("TEST_DATABASE_URL environment variable is not set for chat integration tests")
	}

	var err error
	config, err := pgxpool.ParseConfig(dbURL)
	if err != nil {
		log.Fatalf("Unable to parse TEST_DATABASE_URL: %v\n", err)
	}
	config.MaxConns = 5

	testChatDB, err = pgxpool.NewWithConfig(context.Background(), config)
	if err != nil {
		log.Fatalf("Unable to create connection pool for chat tests: %v\n", err)
	}
	defer testChatDB.Close()

	if err := testChatDB.Ping(context.Background()); err != nil {
		log.Fatalf("Unable to ping test database for chat tests: %v\n", err)
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	// Initialize dependencies
	testChatRepo = NewPostgresRepository(testChatDB, logger)
	interestRepo := interests.NewPostgresinterestsRepo(testChatDB, logger)
	profileRepo := profiles.NewPostgresprofilessRepo(testChatDB, logger)
	tagsRepo := tags.NewPostgrestagsRepo(testChatDB, logger)
	poiRepo := poi.NewPostgresPOIRepository(testChatDB, logger)
	cityRepo := city.NewPostgresCityRepository(testChatDB, logger)

	// Initialize AI client (may require API key)
	var aiClient *generativeAI.AIClient
	if os.Getenv("GOOGLE_GEMINI_API_KEY") != "" {
		aiClient, err = generativeAI.NewAIClient(context.Background())
		if err != nil {
			log.Printf("Warning: Could not initialize AI client for integration tests: %v", err)
		}
	}

	testChatService = NewServiceImpl(
		testChatRepo,
		interestRepo,
		profileRepo,
		tagsRepo,
		poiRepo,
		cityRepo,
		aiClient,
		logger,
	)

	exitCode := m.Run()
	os.Exit(exitCode)
}

func clearChatTables(t *testing.T) {
	t.Helper()
	_, err := testChatDB.Exec(context.Background(), "DELETE FROM llm_interaction_session")
	require.NoError(t, err, "Failed to clear llm_interaction_session table")
	_, err = testChatDB.Exec(context.Background(), "DELETE FROM user_saved_iteneraries")
	require.NoError(t, err, "Failed to clear user_saved_iteneraries table")
}

func createTestUserForChat(t *testing.T) uuid.UUID {
	t.Helper()
	userID := uuid.New()
	_, err := testChatDB.Exec(context.Background(),
		"INSERT INTO users (id, username, email, password_hash) VALUES ($1, $2, $3, $4) ON CONFLICT (id) DO NOTHING",
		userID, "chatuser", "chatuser@test.com", "hash")
	require.NoError(t, err)
	return userID
}

func createTestProfileForChat(t *testing.T, userID uuid.UUID) uuid.UUID {
	t.Helper()
	profileID := uuid.New()
	_, err := testChatDB.Exec(context.Background(),
		"INSERT INTO user_preference_profiles (id, user_id, profile_name, is_default) VALUES ($1, $2, $3, $4) ON CONFLICT (id) DO NOTHING",
		profileID, userID, "Test Profile", true)
	require.NoError(t, err)
	return profileID
}

func createTestCityForChat(t *testing.T) uuid.UUID {
	t.Helper()
	cityID := uuid.New()
	_, err := testChatDB.Exec(context.Background(),
		"INSERT INTO cities (id, name, country) VALUES ($1, $2, $3) ON CONFLICT (id) DO NOTHING",
		cityID, "Lisbon", "Portugal")
	require.NoError(t, err)
	return cityID
}

func TestServiceImpl_SaveItenerary_Integration(t *testing.T) {
	ctx := context.Background()
	clearChatTables(t)

	userID := createTestUserForChat(t)
	cityID := createTestCityForChat(t)

	t.Run("Save itinerary successfully", func(t *testing.T) {
		req := types.BookmarkRequest{
			Title:       "My Lisbon Trip",
			Description: "A wonderful trip to Lisbon",
			CityID:      cityID,
			Content:     "Day 1: Visit Belém Tower\nDay 2: Explore Alfama",
		}

		itineraryID, err := testChatService.SaveItenerary(ctx, userID, req)
		require.NoError(t, err)
		assert.NotEqual(t, uuid.Nil, itineraryID)

		// Verify in database
		var dbTitle, dbDescription string
		err = testChatDB.QueryRow(ctx,
			"SELECT title, description FROM user_saved_iteneraries WHERE id = $1",
			itineraryID).Scan(&dbTitle, &dbDescription)
		require.NoError(t, err)
		assert.Equal(t, req.Title, dbTitle)
		assert.Equal(t, req.Description, dbDescription)
	})
}

func TestServiceImpl_RemoveItenerary_Integration(t *testing.T) {
	ctx := context.Background()
	clearChatTables(t)

	userID := createTestUserForChat(t)
	cityID := createTestCityForChat(t)

	// Create test itinerary
	req := types.BookmarkRequest{
		Title:       "Trip to Remove",
		Description: "This will be removed",
		CityID:      cityID,
		Content:     "Test content",
	}

	itineraryID, err := testChatService.SaveItenerary(ctx, userID, req)
	require.NoError(t, err)

	t.Run("Remove existing itinerary", func(t *testing.T) {
		err := testChatService.RemoveItenerary(ctx, userID, itineraryID)
		require.NoError(t, err)

		// Verify removal
		var count int
		err = testChatDB.QueryRow(ctx,
			"SELECT COUNT(*) FROM user_saved_iteneraries WHERE id = $1",
			itineraryID).Scan(&count)
		require.NoError(t, err)
		assert.Equal(t, 0, count)
	})

	t.Run("Remove non-existent itinerary", func(t *testing.T) {
		nonExistentID := uuid.New()
		err := testChatService.RemoveItenerary(ctx, userID, nonExistentID)
		// Should handle gracefully or return specific error
		// Adjust assertion based on expected behavior
		require.NoError(t, err) // Assuming it handles gracefully
	})
}

func TestServiceImpl_StartNewSession_Integration(t *testing.T) {
	ctx := context.Background()
	clearChatTables(t)

	userID := createTestUserForChat(t)
	profileID := createTestProfileForChat(t, userID)
	createTestCityForChat(t)

	t.Run("Start new chat session", func(t *testing.T) {
		cityName := "Lisbon"
		message := "I want to explore Lisbon for 2 days"
		userLocation := &types.UserLocation{
			Latitude:  38.7223,
			Longitude: -9.1393,
		}

		sessionID, response, err := testChatService.StartNewSession(ctx, userID, profileID, cityName, message, userLocation)

		if os.Getenv("GOOGLE_GEMINI_API_KEY") == "" {
			// Skip AI-dependent tests if no API key
			t.Skip("Skipping AI-dependent test: GOOGLE_GEMINI_API_KEY not set")
		}

		require.NoError(t, err)
		assert.NotEqual(t, uuid.Nil, sessionID)
		assert.NotNil(t, response)

		// Verify session was saved
		var dbSessionID uuid.UUID
		err = testChatDB.QueryRow(ctx,
			"SELECT id FROM llm_interaction_session WHERE id = $1",
			sessionID).Scan(&dbSessionID)
		require.NoError(t, err)
		assert.Equal(t, sessionID, dbSessionID)
	})
}

func TestServiceImpl_ContinueSession_Integration(t *testing.T) {
	ctx := context.Background()
	clearChatTables(t)

	if os.Getenv("GOOGLE_GEMINI_API_KEY") == "" {
		t.Skip("Skipping AI-dependent test: GOOGLE_GEMINI_API_KEY not set")
	}

	userID := createTestUserForChat(t)
	profileID := createTestProfileForChat(t, userID)
	createTestCityForChat(t)

	// Start a session first
	cityName := "Lisbon"
	initialMessage := "I want to explore Lisbon"
	userLocation := &types.UserLocation{
		Latitude:  38.7223,
		Longitude: -9.1393,
	}

	sessionID, _, err := testChatService.StartNewSession(ctx, userID, profileID, cityName, initialMessage, userLocation)
	require.NoError(t, err)

	t.Run("Continue existing session", func(t *testing.T) {
		followUpMessage := "Tell me more about Belém Tower"

		response, err := testChatService.ContinueSession(ctx, sessionID, followUpMessage, userLocation)
		require.NoError(t, err)
		assert.NotNil(t, response)
	})

	t.Run("Continue non-existent session", func(t *testing.T) {
		nonExistentSessionID := uuid.New()
		followUpMessage := "This should fail"

		_, err := testChatService.ContinueSession(ctx, nonExistentSessionID, followUpMessage, userLocation)
		require.Error(t, err)
	})
}

func TestServiceImpl_GetIteneraryResponse_Integration(t *testing.T) {
	ctx := context.Background()
	clearChatTables(t)

	if os.Getenv("GOOGLE_GEMINI_API_KEY") == "" {
		t.Skip("Skipping AI-dependent test: GOOGLE_GEMINI_API_KEY not set")
	}

	userID := createTestUserForChat(t)
	profileID := createTestProfileForChat(t, userID)
	createTestCityForChat(t)

	t.Run("Get itinerary response", func(t *testing.T) {
		cityName := "Lisbon"
		userLocation := &types.UserLocation{
			Latitude:  38.7223,
			Longitude: -9.1393,
		}

		response, err := testChatService.GetIteneraryResponse(ctx, cityName, userID, profileID, userLocation)
		require.NoError(t, err)
		assert.NotNil(t, response)
		assert.NotEmpty(t, response.CityName)
	})
}

func TestServiceImpl_GetPOIDetailsResponse_Integration(t *testing.T) {
	ctx := context.Background()
	clearChatTables(t)

	userID := createTestUserForChat(t)
	createTestCityForChat(t)

	t.Run("Get POI details response", func(t *testing.T) {
		city := "Lisbon"
		lat := 38.7223
		lon := -9.1393

		response, err := testChatService.GetPOIDetailsResponse(ctx, userID, city, lat, lon)

		// This test depends on having POI data in the database
		// If no POIs exist, the service should handle gracefully
		if err != nil {
			// Verify it's a "no data" error rather than a system error
			assert.Contains(t, err.Error(), "no POI found")
		} else {
			assert.NotNil(t, response)
		}
	})
}

// Test intent classification
func TestSimpleIntentClassifier_Integration(t *testing.T) {
	classifier := &SimpleIntentClassifier{}
	ctx := context.Background()

	testCases := []struct {
		message      string
		expectedType types.IntentType
	}{
		{"I want to add a museum to my trip", types.IntentAddPOI},
		{"Please include the cathedral", types.IntentAddPOI},
		{"Remove the restaurant from my list", types.IntentRemovePOI},
		{"Delete this attraction", types.IntentRemovePOI},
		{"What time does the museum open?", types.IntentAskQuestion},
		{"Where is the best place to eat?", types.IntentAskQuestion},
		{"Change my itinerary to focus on art", types.IntentModifyItinerary},
	}

	for _, tc := range testCases {
		t.Run(tc.message, func(t *testing.T) {
			intent, err := classifier.Classify(ctx, tc.message)
			require.NoError(t, err)
			assert.Equal(t, tc.expectedType, intent)
		})
	}
}
