package chat_prompt

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"go.uber.org/zap/zaptest"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/test/bufconn"

	pb "github.com/FACorreiaa/loci-proto/modules/chat/generated"
	"github.com/FACorreiaa/go-poi-au-suggestions/internal/domain"
)

// ChatPromptE2ETestSuite provides end-to-end tests for chat_prompt service
type ChatPromptE2ETestSuite struct {
	suite.Suite
	
	// gRPC server and client
	server   *grpc.Server
	client   pb.ChatServiceClient
	conn     *grpc.ClientConn
	listener *bufconn.Listener
	
	// Database and service
	db      *pgxpool.Pool
	service *Service
	repo    *RepositoryImpl
	
	// Test data
	testUserID    uuid.UUID
	testProfileID uuid.UUID
	authToken     string
}

const bufSize = 1024 * 1024

func (suite *ChatPromptE2ETestSuite) SetupSuite() {
	// Skip E2E tests if no database URL is provided
	dbURL := os.Getenv("TEST_DATABASE_URL")
	if dbURL == "" {
		suite.T().Skip("TEST_DATABASE_URL not set, skipping E2E tests")
	}

	// Setup database
	ctx := context.Background()
	db, err := pgxpool.New(ctx, dbURL)
	require.NoError(suite.T(), err)
	
	err = db.Ping(ctx)
	require.NoError(suite.T(), err)
	suite.db = db
	
	// Create test user data
	suite.testUserID = uuid.New()
	suite.testProfileID = uuid.New()
	suite.authToken = "test-auth-token"
	
	// Setup service
	logger := zaptest.NewLogger(suite.T())
	suite.repo = NewRepository(db, logger)
	suite.service = &Service{
		logger: logger,
		repo:   suite.repo,
		pgpool: db,
	}
	
	// Setup gRPC server
	suite.setupGRPCServer()
}

func (suite *ChatPromptE2ETestSuite) TearDownSuite() {
	if suite.conn != nil {
		suite.conn.Close()
	}
	if suite.server != nil {
		suite.server.Stop()
	}
	if suite.db != nil {
		suite.cleanupTestData()
		suite.db.Close()
	}
}

func (suite *ChatPromptE2ETestSuite) SetupTest() {
	suite.cleanupTestData()
}

func (suite *ChatPromptE2ETestSuite) TearDownTest() {
	suite.cleanupTestData()
}

func (suite *ChatPromptE2ETestSuite) setupGRPCServer() {
	// Create in-memory listener
	suite.listener = bufconn.Listen(bufSize)
	
	// Create gRPC server
	suite.server = grpc.NewServer()
	
	// Register service
	pb.RegisterChatServiceServer(suite.server, suite.service)
	
	// Start server
	go func() {
		if err := suite.server.Serve(suite.listener); err != nil {
			suite.T().Logf("Server error: %v", err)
		}
	}()
	
	// Create client connection
	ctx := context.Background()
	conn, err := grpc.DialContext(ctx, "bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return suite.listener.Dial()
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(suite.T(), err)
	
	suite.conn = conn
	suite.client = pb.NewChatServiceClient(conn)
}

func (suite *ChatPromptE2ETestSuite) cleanupTestData() {
	ctx := context.Background()
	
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

func (suite *ChatPromptE2ETestSuite) createAuthContext() context.Context {
	md := metadata.New(map[string]string{
		"authorization": "Bearer " + suite.authToken,
	})
	ctx := metadata.NewOutgoingContext(context.Background(), md)
	return context.WithValue(ctx, domain.UserIDKey, suite.testUserID.String())
}

func (suite *ChatPromptE2ETestSuite) TestGetChatSessions_E2E() {
	ctx := suite.createAuthContext()
	
	// First, create some test sessions directly in the database
	session1, err := suite.repo.CreateSession(ctx, suite.testUserID, suite.testProfileID)
	require.NoError(suite.T(), err)
	
	session2, err := suite.repo.CreateSession(ctx, suite.testUserID, suite.testProfileID)
	require.NoError(suite.T(), err)
	
	// Test the gRPC call
	req := &pb.GetChatSessionsRequest{
		Page:     1,
		PageSize: 10,
	}
	
	resp, err := suite.client.GetChatSessions(ctx, req)
	
	// Assertions
	assert.NoError(suite.T(), err)
	assert.NotNil(suite.T(), resp)
	assert.GreaterOrEqual(suite.T(), len(resp.Sessions), 2)
	assert.Equal(suite.T(), int32(2), resp.TotalCount)
	
	// Verify response structure
	for _, session := range resp.Sessions {
		assert.NotEmpty(suite.T(), session.SessionId)
		assert.Equal(suite.T(), suite.testUserID.String(), session.UserId)
		assert.Equal(suite.T(), suite.testProfileID.String(), session.ProfileId)
		assert.NotNil(suite.T(), session.CreatedAt)
		assert.NotNil(suite.T(), session.UpdatedAt)
	}
	
	// Verify specific sessions exist
	sessionIDs := make(map[string]bool)
	for _, session := range resp.Sessions {
		sessionIDs[session.SessionId] = true
	}
	assert.True(suite.T(), sessionIDs[session1.ID.String()])
	assert.True(suite.T(), sessionIDs[session2.ID.String()])
}

func (suite *ChatPromptE2ETestSuite) TestSaveAndGetItineraries_E2E() {
	ctx := suite.createAuthContext()
	
	// Create a test session
	session, err := suite.repo.CreateSession(ctx, suite.testUserID, suite.testProfileID)
	require.NoError(suite.T(), err)
	
	// Add some interactions to the session
	interaction := LlmInteraction{
		ID:               uuid.New(),
		SessionID:        session.ID,
		UserID:           suite.testUserID,
		ProfileID:        suite.testProfileID,
		Prompt:           "Plan a visit to the High Line in NYC",
		ResponseText:     "The High Line is an elevated linear park...",
		ModelUsed:        "test-model",
		PromptTokens:     12,
		CompletionTokens: 75,
		TotalTokens:      87,
		Timestamp:        time.Now(),
	}
	
	_, err = suite.repo.SaveInteraction(ctx, interaction)
	require.NoError(suite.T(), err)
	
	// Test SaveItinerary
	saveReq := &pb.SaveItineraryRequest{
		Title:       "High Line Adventure",
		Description: "Exploring the elevated park in Manhattan",
		SessionId:   session.ID.String(),
	}
	
	saveResp, err := suite.client.SaveItinerary(ctx, saveReq)
	
	// Assertions for save
	assert.NoError(suite.T(), err)
	assert.NotNil(suite.T(), saveResp)
	assert.True(suite.T(), saveResp.Success)
	assert.NotEmpty(suite.T(), saveResp.ItineraryId)
	assert.Equal(suite.T(), "Itinerary saved successfully", saveResp.Message)
	
	// Test GetSavedItineraries
	getReq := &pb.GetSavedItinerariesRequest{
		Page:     1,
		PageSize: 10,
	}
	
	getResp, err := suite.client.GetSavedItineraries(ctx, getReq)
	
	// Assertions for get
	assert.NoError(suite.T(), err)
	assert.NotNil(suite.T(), getResp)
	assert.Equal(suite.T(), 1, len(getResp.Itineraries))
	assert.Equal(suite.T(), int32(1), getResp.TotalCount)
	
	savedItinerary := getResp.Itineraries[0]
	assert.Equal(suite.T(), saveResp.ItineraryId, savedItinerary.Id)
	assert.Equal(suite.T(), "High Line Adventure", savedItinerary.Title)
	assert.Equal(suite.T(), "Exploring the elevated park in Manhattan", savedItinerary.Description)
	assert.Equal(suite.T(), suite.testUserID.String(), savedItinerary.UserId)
	assert.NotNil(suite.T(), savedItinerary.CreatedAt)
}

func (suite *ChatPromptE2ETestSuite) TestRemoveItinerary_E2E() {
	ctx := suite.createAuthContext()
	
	// Setup: Create session and save itinerary
	session, err := suite.repo.CreateSession(ctx, suite.testUserID, suite.testProfileID)
	require.NoError(suite.T(), err)
	
	saveReq := &pb.SaveItineraryRequest{
		Title:       "Central Park Picnic",
		Description: "Perfect spots for a picnic in Central Park",
		SessionId:   session.ID.String(),
	}
	
	saveResp, err := suite.client.SaveItinerary(ctx, saveReq)
	require.NoError(suite.T(), err)
	
	// Verify itinerary exists
	getReq := &pb.GetSavedItinerariesRequest{Page: 1, PageSize: 10}
	getResp, err := suite.client.GetSavedItineraries(ctx, getReq)
	require.NoError(suite.T(), err)
	require.Equal(suite.T(), 1, len(getResp.Itineraries))
	
	// Test RemoveItinerary
	removeReq := &pb.RemoveItineraryRequest{
		ItineraryId: saveResp.ItineraryId,
	}
	
	removeResp, err := suite.client.RemoveItinerary(ctx, removeReq)
	
	// Assertions
	assert.NoError(suite.T(), err)
	assert.NotNil(suite.T(), removeResp)
	assert.True(suite.T(), removeResp.Success)
	assert.Equal(suite.T(), "Itinerary removed successfully", removeResp.Message)
	
	// Verify itinerary was removed
	getResp, err = suite.client.GetSavedItineraries(ctx, getReq)
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), 0, len(getResp.Itineraries))
	assert.Equal(suite.T(), int32(0), getResp.TotalCount)
}

func (suite *ChatPromptE2ETestSuite) TestFreeChatStream_E2E() {
	ctx := context.Background() // Free chat doesn't require auth
	
	req := &pb.FreeChatRequest{
		Prompt: "What are some popular tourist attractions in New York?",
	}
	
	stream, err := suite.client.FreeChatStream(ctx, req)
	require.NoError(suite.T(), err)
	
	// Read events from stream
	var events []*pb.ChatEvent
	eventCount := 0
	maxEvents := 10 // Prevent infinite loop
	
	for eventCount < maxEvents {
		event, err := stream.Recv()
		if err != nil {
			// Stream ended or error occurred
			break
		}
		
		events = append(events, event)
		eventCount++
		
		// Break on session complete or error
		if event.Type == pb.EventType_SESSION_COMPLETE || event.Type == pb.EventType_ERROR {
			break
		}
	}
	
	// Assertions
	assert.Greater(suite.T(), len(events), 0, "Should receive at least one event")
	
	// Check event types are valid
	for _, event := range events {
		assert.True(suite.T(), event.Type >= pb.EventType_SESSION_START && event.Type <= pb.EventType_ERROR,
			"Event type should be valid")
	}
	
	// Should have at least a session start event
	hasSessionStart := false
	for _, event := range events {
		if event.Type == pb.EventType_SESSION_START {
			hasSessionStart = true
			break
		}
	}
	assert.True(suite.T(), hasSessionStart, "Should have session start event")
}

func (suite *ChatPromptE2ETestSuite) TestStartChatStream_E2E() {
	// Note: This test would require proper authentication setup
	// For now, we'll test the error case for unauthenticated requests
	
	ctx := context.Background() // No auth context
	
	req := &pb.StartChatRequest{
		Prompt:    "Plan a day in Brooklyn",
		ProfileId: suite.testProfileID.String(),
		CityName:  "New York",
		Location: &pb.LocationInfo{
			Latitude:  40.6782,
			Longitude: -73.9442,
		},
	}
	
	stream, err := suite.client.StartChatStream(ctx, req)
	require.NoError(suite.T(), err)
	
	// Should receive an error event for unauthenticated request
	event, err := stream.Recv()
	
	if err == nil {
		// If we got an event, it should be an error event
		assert.Equal(suite.T(), pb.EventType_ERROR, event.Type)
		assert.Contains(suite.T(), event.Data, "authentication") // Should mention auth error
	}
	// If err != nil, the stream was closed due to auth error, which is also expected
}

func (suite *ChatPromptE2ETestSuite) TestContinueChatStream_E2E() {
	// Similar to StartChatStream, test the error case
	ctx := context.Background()
	
	req := &pb.ContinueChatRequest{
		SessionId: uuid.New().String(),
		Prompt:    "Tell me more about Brooklyn",
	}
	
	stream, err := suite.client.ContinueChatStream(ctx, req)
	require.NoError(suite.T(), err)
	
	// Should receive an error event for unauthenticated request
	event, err := stream.Recv()
	
	if err == nil {
		assert.Equal(suite.T(), pb.EventType_ERROR, event.Type)
		assert.Contains(suite.T(), event.Data, "authentication")
	}
}

func (suite *ChatPromptE2ETestSuite) TestGetPOIDetails_E2E() {
	ctx := suite.createAuthContext()
	
	req := &pb.GetPOIDetailsRequest{
		PoiId: uuid.New().String(),
	}
	
	resp, err := suite.client.GetPOIDetails(ctx, req)
	
	// Should return unimplemented error
	assert.Error(suite.T(), err)
	assert.Nil(suite.T(), resp)
	assert.Contains(suite.T(), err.Error(), "unimplemented")
}

func (suite *ChatPromptE2ETestSuite) TestErrorHandling_E2E() {
	ctx := suite.createAuthContext()
	
	// Test invalid UUID formats
	t := suite.T()
	
	t.Run("SaveItinerary with invalid session ID", func(t *testing.T) {
		req := &pb.SaveItineraryRequest{
			Title:     "Test Itinerary",
			SessionId: "invalid-uuid",
		}
		
		resp, err := suite.client.SaveItinerary(ctx, req)
		assert.Error(t, err)
		assert.Nil(t, resp)
		assert.Contains(t, err.Error(), "invalid")
	})
	
	t.Run("RemoveItinerary with invalid ID", func(t *testing.T) {
		req := &pb.RemoveItineraryRequest{
			ItineraryId: "invalid-uuid",
		}
		
		resp, err := suite.client.RemoveItinerary(ctx, req)
		assert.Error(t, err)
		assert.Nil(t, resp)
		assert.Contains(t, err.Error(), "invalid")
	})
	
	t.Run("SaveItinerary with missing title", func(t *testing.T) {
		req := &pb.SaveItineraryRequest{
			Title:     "",
			SessionId: uuid.New().String(),
		}
		
		resp, err := suite.client.SaveItinerary(ctx, req)
		assert.Error(t, err)
		assert.Nil(t, resp)
		assert.Contains(t, err.Error(), "title")
	})
}

func (suite *ChatPromptE2ETestSuite) TestPaginationBoundaries_E2E() {
	ctx := suite.createAuthContext()
	
	// Create multiple sessions for pagination testing
	numSessions := 15
	for i := 0; i < numSessions; i++ {
		_, err := suite.repo.CreateSession(ctx, suite.testUserID, suite.testProfileID)
		require.NoError(suite.T(), err)
	}
	
	// Test first page
	req := &pb.GetChatSessionsRequest{
		Page:     1,
		PageSize: 10,
	}
	resp, err := suite.client.GetChatSessions(ctx, req)
	
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), 10, len(resp.Sessions))
	assert.Equal(suite.T(), int32(15), resp.TotalCount)
	assert.Equal(suite.T(), int32(1), resp.Page)
	assert.Equal(suite.T(), int32(10), resp.PageSize)
	
	// Test second page
	req.Page = 2
	resp, err = suite.client.GetChatSessions(ctx, req)
	
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), 5, len(resp.Sessions)) // Remaining sessions
	assert.Equal(suite.T(), int32(15), resp.TotalCount)
	assert.Equal(suite.T(), int32(2), resp.Page)
	
	// Test beyond available pages
	req.Page = 10
	resp, err = suite.client.GetChatSessions(ctx, req)
	
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), 0, len(resp.Sessions))
	assert.Equal(suite.T(), int32(15), resp.TotalCount)
}

func (suite *ChatPromptE2ETestSuite) TestConcurrentRequests_E2E() {
	ctx := suite.createAuthContext()
	
	// Test concurrent SaveItinerary requests
	const numConcurrent = 5
	
	// Create a session first
	session, err := suite.repo.CreateSession(ctx, suite.testUserID, suite.testProfileID)
	require.NoError(suite.T(), err)
	
	// Channel to collect results
	type result struct {
		resp *pb.SaveItineraryResponse
		err  error
	}
	results := make(chan result, numConcurrent)
	
	// Launch concurrent requests
	for i := 0; i < numConcurrent; i++ {
		go func(idx int) {
			req := &pb.SaveItineraryRequest{
				Title:       fmt.Sprintf("Concurrent Itinerary %d", idx),
				Description: fmt.Sprintf("Description for itinerary %d", idx),
				SessionId:   session.ID.String(),
			}
			
			resp, err := suite.client.SaveItinerary(ctx, req)
			results <- result{resp: resp, err: err}
		}(i)
	}
	
	// Collect results
	successCount := 0
	for i := 0; i < numConcurrent; i++ {
		select {
		case res := <-results:
			if res.err == nil && res.resp != nil && res.resp.Success {
				successCount++
			} else if res.err != nil {
				suite.T().Logf("Concurrent request failed: %v", res.err)
			}
		case <-time.After(30 * time.Second):
			suite.T().Fatal("Timeout waiting for concurrent requests")
		}
	}
	
	assert.Equal(suite.T(), numConcurrent, successCount, "All concurrent requests should succeed")
	
	// Verify all itineraries were saved
	getReq := &pb.GetSavedItinerariesRequest{Page: 1, PageSize: 10}
	getResp, err := suite.client.GetSavedItineraries(ctx, getReq)
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), numConcurrent, len(getResp.Itineraries))
}

// TestChatPromptE2ESuite runs the E2E test suite
func TestChatPromptE2ESuite(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping E2E tests in short mode")
	}
	
	// Check if we're in CI environment or have specific E2E flag
	if os.Getenv("RUN_E2E_TESTS") == "" {
		t.Skip("E2E tests disabled. Set RUN_E2E_TESTS=1 to enable")
	}
	
	suite.Run(t, new(ChatPromptE2ETestSuite))
}