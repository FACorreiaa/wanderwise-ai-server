package chat_prompt

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"go.uber.org/zap/zaptest"

	pb "github.com/FACorreiaa/loci-proto/modules/chat/generated"
	"github.com/FACorreiaa/go-poi-au-suggestions/internal/domain"
)

// ChatPromptIntegrationTestSuite provides integration tests for chat_prompt service
type ChatPromptIntegrationTestSuite struct {
	suite.Suite
	db      *pgxpool.Pool
	service *Service
	repo    *RepositoryImpl
	
	// Test data
	testUserID    uuid.UUID
	testProfileID uuid.UUID
	testSessionID uuid.UUID
}

func (suite *ChatPromptIntegrationTestSuite) SetupSuite() {
	// Skip integration tests if no database URL is provided
	dbURL := os.Getenv("TEST_DATABASE_URL")
	if dbURL == "" {
		suite.T().Skip("TEST_DATABASE_URL not set, skipping integration tests")
	}

	// Connect to test database
	ctx := context.Background()
	db, err := pgxpool.New(ctx, dbURL)
	require.NoError(suite.T(), err)
	
	// Test connection
	err = db.Ping(ctx)
	require.NoError(suite.T(), err)
	
	suite.db = db
	
	// Create logger
	logger := zaptest.NewLogger(suite.T())
	
	// Create repository
	suite.repo = NewRepository(db, logger)
	
	// Create service
	suite.service = &Service{
		logger: logger,
		repo:   suite.repo,
		pgpool: db,
	}
	
	// Create test data
	suite.testUserID = uuid.New()
	suite.testProfileID = uuid.New()
}

func (suite *ChatPromptIntegrationTestSuite) TearDownSuite() {
	if suite.db != nil {
		suite.cleanupTestData()
		suite.db.Close()
	}
}

func (suite *ChatPromptIntegrationTestSuite) SetupTest() {
	// Clean up any existing test data
	suite.cleanupTestData()
}

func (suite *ChatPromptIntegrationTestSuite) TearDownTest() {
	// Clean up test data after each test
	suite.cleanupTestData()
}

func (suite *ChatPromptIntegrationTestSuite) cleanupTestData() {
	ctx := context.Background()
	
	// Clean up in reverse dependency order
	queries := []string{
		"DELETE FROM llm_interactions WHERE user_id = $1",
		"DELETE FROM saved_itineraries WHERE user_id = $1", 
		"DELETE FROM llm_sessions WHERE user_id = $1",
	}
	
	for _, query := range queries {
		_, err := suite.db.Exec(ctx, query, suite.testUserID)
		if err != nil {
			suite.T().Logf("Warning: cleanup query failed: %v", err)
		}
	}
}

func (suite *ChatPromptIntegrationTestSuite) createAuthContext() context.Context {
	return context.WithValue(context.Background(), domain.UserIDKey, suite.testUserID.String())
}

func (suite *ChatPromptIntegrationTestSuite) createTestSession() (*LlmSession, error) {
	ctx := suite.createAuthContext()
	return suite.repo.CreateSession(ctx, suite.testUserID, suite.testProfileID)
}

func (suite *ChatPromptIntegrationTestSuite) TestGetChatSessions_Integration() {
	ctx := suite.createAuthContext()
	
	// Create test sessions
	session1, err := suite.createTestSession()
	require.NoError(suite.T(), err)
	
	session2, err := suite.createTestSession()
	require.NoError(suite.T(), err)
	
	suite.testSessionID = session1.ID
	
	// Test GetChatSessions
	req := &pb.GetChatSessionsRequest{
		Page:     1,
		PageSize: 10,
	}
	
	resp, err := suite.service.GetChatSessions(ctx, req)
	
	// Assertions
	assert.NoError(suite.T(), err)
	assert.NotNil(suite.T(), resp)
	assert.GreaterOrEqual(suite.T(), len(resp.Sessions), 2)
	assert.Equal(suite.T(), int32(2), resp.TotalCount)
	
	// Verify session data
	foundSessions := make(map[string]bool)
	for _, session := range resp.Sessions {
		foundSessions[session.SessionId] = true
		assert.Equal(suite.T(), suite.testUserID.String(), session.UserId)
		assert.Equal(suite.T(), suite.testProfileID.String(), session.ProfileId)
		assert.NotNil(suite.T(), session.CreatedAt)
	}
	
	assert.True(suite.T(), foundSessions[session1.ID.String()])
	assert.True(suite.T(), foundSessions[session2.ID.String()])
}

func (suite *ChatPromptIntegrationTestSuite) TestGetChatSessions_Pagination() {
	ctx := suite.createAuthContext()
	
	// Create multiple test sessions
	var sessions []*LlmSession
	for i := 0; i < 5; i++ {
		session, err := suite.createTestSession()
		require.NoError(suite.T(), err)
		sessions = append(sessions, session)
	}
	
	// Test first page
	req := &pb.GetChatSessionsRequest{
		Page:     1,
		PageSize: 2,
	}
	resp, err := suite.service.GetChatSessions(ctx, req)
	
	assert.NoError(suite.T(), err)
	assert.NotNil(suite.T(), resp)
	assert.Equal(suite.T(), 2, len(resp.Sessions))
	assert.Equal(suite.T(), int32(5), resp.TotalCount)
	assert.Equal(suite.T(), int32(1), resp.Page)
	assert.Equal(suite.T(), int32(2), resp.PageSize)
	
	// Test second page
	req.Page = 2
	resp, err = suite.service.GetChatSessions(ctx, req)
	
	assert.NoError(suite.T(), err)
	assert.NotNil(suite.T(), resp)
	assert.Equal(suite.T(), 2, len(resp.Sessions))
	assert.Equal(suite.T(), int32(5), resp.TotalCount)
	assert.Equal(suite.T(), int32(2), resp.Page)
}

func (suite *ChatPromptIntegrationTestSuite) TestSaveItinerary_Integration() {
	ctx := suite.createAuthContext()
	
	// Create a test session first
	session, err := suite.createTestSession()
	require.NoError(suite.T(), err)
	
	// Create a test interaction in the session
	interaction := LlmInteraction{
		ID:               uuid.New(),
		SessionID:        session.ID,
		UserID:           suite.testUserID,
		ProfileID:        suite.testProfileID,
		Prompt:           "Plan a day trip to Central Park",
		ResponseText:     "Here's a great itinerary for Central Park...",
		ModelUsed:        "test-model",
		PromptTokens:     15,
		CompletionTokens: 100,
		TotalTokens:      115,
		Timestamp:        time.Now(),
	}
	
	_, err = suite.repo.SaveInteraction(ctx, interaction)
	require.NoError(suite.T(), err)
	
	// Test SaveItinerary with session ID
	req := &pb.SaveItineraryRequest{
		Title:       "Central Park Day Trip",
		Description: "A wonderful day exploring Central Park",
		SessionId:   session.ID.String(),
	}
	
	resp, err := suite.service.SaveItinerary(ctx, req)
	
	// Assertions
	assert.NoError(suite.T(), err)
	assert.NotNil(suite.T(), resp)
	assert.True(suite.T(), resp.Success)
	assert.NotEmpty(suite.T(), resp.ItineraryId)
	assert.Equal(suite.T(), "Itinerary saved successfully", resp.Message)
	
	// Verify the itinerary was actually saved
	itineraryUUID, err := uuid.Parse(resp.ItineraryId)
	require.NoError(suite.T(), err)
	
	// Query the database to verify
	var count int
	err = suite.db.QueryRow(ctx, 
		"SELECT COUNT(*) FROM saved_itineraries WHERE id = $1 AND user_id = $2", 
		itineraryUUID, suite.testUserID).Scan(&count)
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), 1, count)
}

func (suite *ChatPromptIntegrationTestSuite) TestSaveItinerary_WithInteractionID() {
	ctx := suite.createAuthContext()
	
	// Create a test session and interaction
	session, err := suite.createTestSession()
	require.NoError(suite.T(), err)
	
	interaction := LlmInteraction{
		ID:               uuid.New(),
		SessionID:        session.ID,
		UserID:           suite.testUserID,
		ProfileID:        suite.testProfileID,
		Prompt:           "Plan a food tour of Little Italy",
		ResponseText:     "Here's an amazing food tour...",
		ModelUsed:        "test-model",
		PromptTokens:     20,
		CompletionTokens: 150,
		TotalTokens:      170,
		Timestamp:        time.Now(),
	}
	
	interactionID, err := suite.repo.SaveInteraction(ctx, interaction)
	require.NoError(suite.T(), err)
	
	// Test SaveItinerary with interaction ID
	req := &pb.SaveItineraryRequest{
		Title:         "Little Italy Food Tour",
		Description:   "Explore the best food in Little Italy",
		InteractionId: interactionID.String(),
	}
	
	resp, err := suite.service.SaveItinerary(ctx, req)
	
	// Assertions
	assert.NoError(suite.T(), err)
	assert.NotNil(suite.T(), resp)
	assert.True(suite.T(), resp.Success)
	assert.NotEmpty(suite.T(), resp.ItineraryId)
}

func (suite *ChatPromptIntegrationTestSuite) TestGetSavedItineraries_Integration() {
	ctx := suite.createAuthContext()
	
	// Create test sessions and save some itineraries
	session1, err := suite.createTestSession()
	require.NoError(suite.T(), err)
	
	session2, err := suite.createTestSession()
	require.NoError(suite.T(), err)
	
	// Save first itinerary
	req1 := &pb.SaveItineraryRequest{
		Title:       "Brooklyn Bridge Walk",
		Description: "A scenic walk across Brooklyn Bridge",
		SessionId:   session1.ID.String(),
	}
	resp1, err := suite.service.SaveItinerary(ctx, req1)
	require.NoError(suite.T(), err)
	
	// Save second itinerary
	req2 := &pb.SaveItineraryRequest{
		Title:       "Museum Hopping",
		Description: "Visit the best museums in NYC",
		SessionId:   session2.ID.String(),
	}
	resp2, err := suite.service.SaveItinerary(ctx, req2)
	require.NoError(suite.T(), err)
	
	// Test GetSavedItineraries
	getReq := &pb.GetSavedItinerariesRequest{
		Page:     1,
		PageSize: 10,
	}
	
	getResp, err := suite.service.GetSavedItineraries(ctx, getReq)
	
	// Assertions
	assert.NoError(suite.T(), err)
	assert.NotNil(suite.T(), getResp)
	assert.Equal(suite.T(), 2, len(getResp.Itineraries))
	assert.Equal(suite.T(), int32(2), getResp.TotalCount)
	
	// Verify itinerary data
	foundItineraries := make(map[string]*pb.SavedItinerary)
	for _, itinerary := range getResp.Itineraries {
		foundItineraries[itinerary.Id] = itinerary
		assert.Equal(suite.T(), suite.testUserID.String(), itinerary.UserId)
		assert.NotNil(suite.T(), itinerary.CreatedAt)
	}
	
	itinerary1 := foundItineraries[resp1.ItineraryId]
	assert.NotNil(suite.T(), itinerary1)
	assert.Equal(suite.T(), "Brooklyn Bridge Walk", itinerary1.Title)
	assert.Equal(suite.T(), "A scenic walk across Brooklyn Bridge", itinerary1.Description)
	
	itinerary2 := foundItineraries[resp2.ItineraryId]
	assert.NotNil(suite.T(), itinerary2)
	assert.Equal(suite.T(), "Museum Hopping", itinerary2.Title)
	assert.Equal(suite.T(), "Visit the best museums in NYC", itinerary2.Description)
}

func (suite *ChatPromptIntegrationTestSuite) TestRemoveItinerary_Integration() {
	ctx := suite.createAuthContext()
	
	// Create a test session and save an itinerary
	session, err := suite.createTestSession()
	require.NoError(suite.T(), err)
	
	saveReq := &pb.SaveItineraryRequest{
		Title:       "Times Square Visit",
		Description: "Experience the energy of Times Square",
		SessionId:   session.ID.String(),
	}
	saveResp, err := suite.service.SaveItinerary(ctx, saveReq)
	require.NoError(suite.T(), err)
	
	// Verify itinerary exists
	getReq := &pb.GetSavedItinerariesRequest{Page: 1, PageSize: 10}
	getResp, err := suite.service.GetSavedItineraries(ctx, getReq)
	require.NoError(suite.T(), err)
	require.Equal(suite.T(), 1, len(getResp.Itineraries))
	
	// Test RemoveItinerary
	removeReq := &pb.RemoveItineraryRequest{
		ItineraryId: saveResp.ItineraryId,
	}
	removeResp, err := suite.service.RemoveItinerary(ctx, removeReq)
	
	// Assertions
	assert.NoError(suite.T(), err)
	assert.NotNil(suite.T(), removeResp)
	assert.True(suite.T(), removeResp.Success)
	assert.Equal(suite.T(), "Itinerary removed successfully", removeResp.Message)
	
	// Verify itinerary was actually removed
	getResp, err = suite.service.GetSavedItineraries(ctx, getReq)
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), 0, len(getResp.Itineraries))
	assert.Equal(suite.T(), int32(0), getResp.TotalCount)
}

func (suite *ChatPromptIntegrationTestSuite) TestCompleteWorkflow_Integration() {
	ctx := suite.createAuthContext()
	
	// Step 1: Create a session (simulating StartChatStream result)
	session, err := suite.createTestSession()
	require.NoError(suite.T(), err)
	
	// Step 2: Simulate some interactions
	interactions := []LlmInteraction{
		{
			ID:               uuid.New(),
			SessionID:        session.ID,
			UserID:           suite.testUserID,
			ProfileID:        suite.testProfileID,
			Prompt:           "I want to explore SoHo",
			ResponseText:     "SoHo is a great neighborhood for shopping and art galleries...",
			ModelUsed:        "test-model",
			PromptTokens:     10,
			CompletionTokens: 50,
			TotalTokens:      60,
			Timestamp:        time.Now(),
		},
		{
			ID:               uuid.New(),
			SessionID:        session.ID,
			UserID:           suite.testUserID,
			ProfileID:        suite.testProfileID,
			Prompt:           "What restaurants are recommended in SoHo?",
			ResponseText:     "Here are some excellent restaurants in SoHo...",
			ModelUsed:        "test-model",
			PromptTokens:     15,
			CompletionTokens: 80,
			TotalTokens:      95,
			Timestamp:        time.Now().Add(5 * time.Minute),
		},
	}
	
	for _, interaction := range interactions {
		_, err := suite.repo.SaveInteraction(ctx, interaction)
		require.NoError(suite.T(), err)
	}
	
	// Step 3: Get chat sessions and verify data
	getSessionsReq := &pb.GetChatSessionsRequest{Page: 1, PageSize: 10}
	sessionsResp, err := suite.service.GetChatSessions(ctx, getSessionsReq)
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), 1, len(sessionsResp.Sessions))
	
	// Step 4: Save the session as an itinerary
	saveReq := &pb.SaveItineraryRequest{
		Title:       "SoHo Exploration",
		Description: "A complete guide to exploring SoHo",
		SessionId:   session.ID.String(),
	}
	saveResp, err := suite.service.SaveItinerary(ctx, saveReq)
	require.NoError(suite.T(), err)
	assert.True(suite.T(), saveResp.Success)
	
	// Step 5: Get saved itineraries and verify
	getItinerariesReq := &pb.GetSavedItinerariesRequest{Page: 1, PageSize: 10}
	itinerariesResp, err := suite.service.GetSavedItineraries(ctx, getItinerariesReq)
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), 1, len(itinerariesResp.Itineraries))
	
	savedItinerary := itinerariesResp.Itineraries[0]
	assert.Equal(suite.T(), "SoHo Exploration", savedItinerary.Title)
	assert.Equal(suite.T(), "A complete guide to exploring SoHo", savedItinerary.Description)
	
	// Step 6: Remove the itinerary
	removeReq := &pb.RemoveItineraryRequest{
		ItineraryId: saveResp.ItineraryId,
	}
	removeResp, err := suite.service.RemoveItinerary(ctx, removeReq)
	require.NoError(suite.T(), err)
	assert.True(suite.T(), removeResp.Success)
	
	// Step 7: Verify itinerary was removed
	itinerariesResp, err = suite.service.GetSavedItineraries(ctx, getItinerariesReq)
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), 0, len(itinerariesResp.Itineraries))
}

func (suite *ChatPromptIntegrationTestSuite) TestConcurrentAccess() {
	ctx := suite.createAuthContext()
	
	// Test concurrent session creation
	const numSessions = 10
	sessionChan := make(chan *LlmSession, numSessions)
	errorChan := make(chan error, numSessions)
	
	for i := 0; i < numSessions; i++ {
		go func() {
			session, err := suite.createTestSession()
			if err != nil {
				errorChan <- err
				return
			}
			sessionChan <- session
		}()
	}
	
	// Collect results
	var sessions []*LlmSession
	for i := 0; i < numSessions; i++ {
		select {
		case session := <-sessionChan:
			sessions = append(sessions, session)
		case err := <-errorChan:
			suite.T().Errorf("Concurrent session creation failed: %v", err)
		case <-time.After(10 * time.Second):
			suite.T().Fatal("Timeout waiting for concurrent session creation")
		}
	}
	
	assert.Equal(suite.T(), numSessions, len(sessions))
	
	// Verify all sessions are unique
	sessionIDs := make(map[uuid.UUID]bool)
	for _, session := range sessions {
		assert.False(suite.T(), sessionIDs[session.ID], "Duplicate session ID found")
		sessionIDs[session.ID] = true
	}
}

// TestChatPromptIntegrationSuite runs the integration test suite
func TestChatPromptIntegrationSuite(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration tests in short mode")
	}
	
	suite.Run(t, new(ChatPromptIntegrationTestSuite))
}