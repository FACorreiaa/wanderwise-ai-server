package chat_prompt

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap/zaptest"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "github.com/FACorreiaa/loci-proto/modules/chat/generated"
	"github.com/FACorreiaa/go-poi-au-suggestions/internal/domain"
)

// MockRepository implements the Repository interface for testing
type MockRepository struct {
	mock.Mock
}

func (m *MockRepository) CreateSession(ctx context.Context, userID uuid.UUID, profileID uuid.UUID) (*LlmSession, error) {
	args := m.Called(ctx, userID, profileID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*LlmSession), args.Error(1)
}

func (m *MockRepository) GetSession(ctx context.Context, sessionID uuid.UUID) (*LlmSession, error) {
	args := m.Called(ctx, sessionID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*LlmSession), args.Error(1)
}

func (m *MockRepository) SaveInteraction(ctx context.Context, interaction LlmInteraction) (uuid.UUID, error) {
	args := m.Called(ctx, interaction)
	return args.Get(0).(uuid.UUID), args.Error(1)
}

func (m *MockRepository) GetInteractionsBySession(ctx context.Context, sessionID uuid.UUID) ([]LlmInteraction, error) {
	args := m.Called(ctx, sessionID)
	return args.Get(0).([]LlmInteraction), args.Error(1)
}

func (m *MockRepository) GetSessionsByUserIDPaginated(ctx context.Context, userID uuid.UUID, limit, offset int) ([]LlmSession, int, error) {
	args := m.Called(ctx, userID, limit, offset)
	return args.Get(0).([]LlmSession), args.Int(1), args.Error(2)
}

func (m *MockRepository) SaveBookmarkedItinerary(ctx context.Context, userID uuid.UUID, req BookmarkRequest) (uuid.UUID, error) {
	args := m.Called(ctx, userID, req)
	return args.Get(0).(uuid.UUID), args.Error(1)
}

func (m *MockRepository) GetSavedItinerariesPaginated(ctx context.Context, userID uuid.UUID, limit, offset int) ([]SavedItinerary, int, error) {
	args := m.Called(ctx, userID, limit, offset)
	return args.Get(0).([]SavedItinerary), args.Int(1), args.Error(2)
}

func (m *MockRepository) RemoveSavedItinerary(ctx context.Context, userID, itineraryID uuid.UUID) error {
	args := m.Called(ctx, userID, itineraryID)
	return args.Error(0)
}

func (m *MockRepository) GetLatestInteractionBySessionID(ctx context.Context, sessionID uuid.UUID) (*LlmInteraction, error) {
	args := m.Called(ctx, sessionID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*LlmInteraction), args.Error(1)
}

// MockStreamServer implements the streaming server interface for testing
type MockStreamServer struct {
	mock.Mock
	grpc.ServerStream
	events []pb.ChatEvent
}

func (m *MockStreamServer) Send(event *pb.ChatEvent) error {
	args := m.Called(event)
	m.events = append(m.events, *event)
	return args.Error(0)
}

func (m *MockStreamServer) Context() context.Context {
	args := m.Called()
	return args.Get(0).(context.Context)
}

func (m *MockStreamServer) GetEvents() []pb.ChatEvent {
	return m.events
}

// Test helper functions
func createTestChatService(t *testing.T) (*Service, *MockRepository) {
	logger := zaptest.NewLogger(t)
	mockRepo := &MockRepository{}
	
	service := &Service{
		logger: logger,
		repo:   mockRepo,
	}
	
	return service, mockRepo
}

func createTestSession() *LlmSession {
	return &LlmSession{
		ID:        uuid.New(),
		UserID:    uuid.New(),
		ProfileID: uuid.New(),
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
}

func createTestInteraction() LlmInteraction {
	return LlmInteraction{
		ID:              uuid.New(),
		SessionID:       uuid.New(),
		UserID:          uuid.New(),
		ProfileID:       uuid.New(),
		Prompt:          "Test prompt",
		ResponseText:    "Test response",
		ModelUsed:       "test-model",
		PromptTokens:    10,
		CompletionTokens: 20,
		TotalTokens:     30,
		Timestamp:       time.Now(),
	}
}

func createAuthContext(userID string) context.Context {
	return context.WithValue(context.Background(), domain.UserIDKey, userID)
}

// Test GetChatSessions
func TestService_GetChatSessions(t *testing.T) {
	service, mockRepo := createTestChatService(t)
	userID := uuid.New().String()

	tests := []struct {
		name        string
		ctx         context.Context
		request     *pb.GetChatSessionsRequest
		setupMock   func()
		expectError bool
		errorCode   codes.Code
	}{
		{
			name: "success",
			ctx:  createAuthContext(userID),
			request: &pb.GetChatSessionsRequest{
				Page:     1,
				PageSize: 10,
			},
			setupMock: func() {
				testSession := createTestSession()
				mockRepo.On("GetSessionsByUserIDPaginated", mock.Anything, 
					mock.AnythingOfType("uuid.UUID"), 10, 0).
					Return([]LlmSession{*testSession}, 1, nil)
			},
			expectError: false,
		},
		{
			name: "success with default pagination",
			ctx:  createAuthContext(userID),
			request: &pb.GetChatSessionsRequest{
				Page:     0, // Should default to 1
				PageSize: 0, // Should default to 10
			},
			setupMock: func() {
				testSession := createTestSession()
				mockRepo.On("GetSessionsByUserIDPaginated", mock.Anything, 
					mock.AnythingOfType("uuid.UUID"), 10, 0).
					Return([]LlmSession{*testSession}, 1, nil)
			},
			expectError: false,
		},
		{
			name: "unauthenticated",
			ctx:  context.Background(),
			request: &pb.GetChatSessionsRequest{
				Page:     1,
				PageSize: 10,
			},
			setupMock:   func() {},
			expectError: true,
			errorCode:   codes.Unauthenticated,
		},
		{
			name: "large page size gets clamped",
			ctx:  createAuthContext(userID),
			request: &pb.GetChatSessionsRequest{
				Page:     1,
				PageSize: 200, // Should be clamped to 100
			},
			setupMock: func() {
				testSession := createTestSession()
				mockRepo.On("GetSessionsByUserIDPaginated", mock.Anything, 
					mock.AnythingOfType("uuid.UUID"), 100, 0).
					Return([]LlmSession{*testSession}, 1, nil)
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupMock()
			
			resp, err := service.GetChatSessions(tt.ctx, tt.request)
			
			if tt.expectError {
				assert.Error(t, err)
				st, ok := status.FromError(err)
				assert.True(t, ok)
				assert.Equal(t, tt.errorCode, st.Code())
				assert.Nil(t, resp)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, resp)
				assert.GreaterOrEqual(t, len(resp.Sessions), 0)
			}
			
			mockRepo.AssertExpectations(t)
		})
	}
}

// Test SaveItinerary
func TestService_SaveItinerary(t *testing.T) {
	service, mockRepo := createTestChatService(t)
	userID := uuid.New().String()
	sessionID := uuid.New().String()

	tests := []struct {
		name        string
		ctx         context.Context
		request     *pb.SaveItineraryRequest
		setupMock   func()
		expectError bool
		errorCode   codes.Code
	}{
		{
			name: "success with session ID",
			ctx:  createAuthContext(userID),
			request: &pb.SaveItineraryRequest{
				Title:       "Test Itinerary",
				Description: "Test Description",
				SessionId:   sessionID,
			},
			setupMock: func() {
				savedID := uuid.New()
				mockRepo.On("SaveBookmarkedItinerary", mock.Anything, 
					mock.AnythingOfType("uuid.UUID"), mock.AnythingOfType("BookmarkRequest")).
					Return(savedID, nil)
			},
			expectError: false,
		},
		{
			name: "success with interaction ID",
			ctx:  createAuthContext(userID),
			request: &pb.SaveItineraryRequest{
				Title:         "Test Itinerary",
				Description:   "Test Description",
				InteractionId: uuid.New().String(),
			},
			setupMock: func() {
				savedID := uuid.New()
				mockRepo.On("SaveBookmarkedItinerary", mock.Anything, 
					mock.AnythingOfType("uuid.UUID"), mock.AnythingOfType("BookmarkRequest")).
					Return(savedID, nil)
			},
			expectError: false,
		},
		{
			name: "missing title",
			ctx:  createAuthContext(userID),
			request: &pb.SaveItineraryRequest{
				Title:     "",
				SessionId: sessionID,
			},
			setupMock:   func() {},
			expectError: true,
			errorCode:   codes.InvalidArgument,
		},
		{
			name: "missing session and interaction ID",
			ctx:  createAuthContext(userID),
			request: &pb.SaveItineraryRequest{
				Title:       "Test Itinerary",
				Description: "Test Description",
			},
			setupMock:   func() {},
			expectError: true,
			errorCode:   codes.InvalidArgument,
		},
		{
			name: "unauthenticated",
			ctx:  context.Background(),
			request: &pb.SaveItineraryRequest{
				Title:     "Test Itinerary",
				SessionId: sessionID,
			},
			setupMock:   func() {},
			expectError: true,
			errorCode:   codes.Unauthenticated,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupMock()
			
			resp, err := service.SaveItinerary(tt.ctx, tt.request)
			
			if tt.expectError {
				assert.Error(t, err)
				st, ok := status.FromError(err)
				assert.True(t, ok)
				assert.Equal(t, tt.errorCode, st.Code())
				assert.Nil(t, resp)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, resp)
				assert.True(t, resp.Success)
				assert.NotEmpty(t, resp.ItineraryId)
			}
			
			mockRepo.AssertExpectations(t)
		})
	}
}

// Test GetSavedItineraries
func TestService_GetSavedItineraries(t *testing.T) {
	service, mockRepo := createTestChatService(t)
	userID := uuid.New().String()

	tests := []struct {
		name        string
		ctx         context.Context
		request     *pb.GetSavedItinerariesRequest
		setupMock   func()
		expectError bool
		errorCode   codes.Code
	}{
		{
			name: "success",
			ctx:  createAuthContext(userID),
			request: &pb.GetSavedItinerariesRequest{
				Page:     1,
				PageSize: 10,
			},
			setupMock: func() {
				savedItinerary := SavedItinerary{
					ID:          uuid.New(),
					UserID:      uuid.MustParse(userID),
					Title:       "Test Itinerary",
					Description: sql.NullString{String: "Test Description", Valid: true},
					CreatedAt:   time.Now(),
				}
				mockRepo.On("GetSavedItinerariesPaginated", mock.Anything, 
					mock.AnythingOfType("uuid.UUID"), 10, 0).
					Return([]SavedItinerary{savedItinerary}, 1, nil)
			},
			expectError: false,
		},
		{
			name: "success with default pagination",
			ctx:  createAuthContext(userID),
			request: &pb.GetSavedItinerariesRequest{
				Page:     0, // Should default to 1
				PageSize: 0, // Should default to 10
			},
			setupMock: func() {
				mockRepo.On("GetSavedItinerariesPaginated", mock.Anything, 
					mock.AnythingOfType("uuid.UUID"), 10, 0).
					Return([]SavedItinerary{}, 0, nil)
			},
			expectError: false,
		},
		{
			name: "unauthenticated",
			ctx:  context.Background(),
			request: &pb.GetSavedItinerariesRequest{
				Page:     1,
				PageSize: 10,
			},
			setupMock:   func() {},
			expectError: true,
			errorCode:   codes.Unauthenticated,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupMock()
			
			resp, err := service.GetSavedItineraries(tt.ctx, tt.request)
			
			if tt.expectError {
				assert.Error(t, err)
				st, ok := status.FromError(err)
				assert.True(t, ok)
				assert.Equal(t, tt.errorCode, st.Code())
				assert.Nil(t, resp)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, resp)
				assert.GreaterOrEqual(t, len(resp.Itineraries), 0)
			}
			
			mockRepo.AssertExpectations(t)
		})
	}
}

// Test RemoveItinerary
func TestService_RemoveItinerary(t *testing.T) {
	service, mockRepo := createTestChatService(t)
	userID := uuid.New().String()
	itineraryID := uuid.New().String()

	tests := []struct {
		name        string
		ctx         context.Context
		request     *pb.RemoveItineraryRequest
		setupMock   func()
		expectError bool
		errorCode   codes.Code
	}{
		{
			name: "success",
			ctx:  createAuthContext(userID),
			request: &pb.RemoveItineraryRequest{
				ItineraryId: itineraryID,
			},
			setupMock: func() {
				mockRepo.On("RemoveSavedItinerary", mock.Anything, 
					mock.AnythingOfType("uuid.UUID"), mock.AnythingOfType("uuid.UUID")).
					Return(nil)
			},
			expectError: false,
		},
		{
			name: "missing itinerary ID",
			ctx:  createAuthContext(userID),
			request: &pb.RemoveItineraryRequest{
				ItineraryId: "",
			},
			setupMock:   func() {},
			expectError: true,
			errorCode:   codes.InvalidArgument,
		},
		{
			name: "invalid itinerary ID format",
			ctx:  createAuthContext(userID),
			request: &pb.RemoveItineraryRequest{
				ItineraryId: "invalid-uuid",
			},
			setupMock:   func() {},
			expectError: true,
			errorCode:   codes.InvalidArgument,
		},
		{
			name: "unauthenticated",
			ctx:  context.Background(),
			request: &pb.RemoveItineraryRequest{
				ItineraryId: itineraryID,
			},
			setupMock:   func() {},
			expectError: true,
			errorCode:   codes.Unauthenticated,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupMock()
			
			resp, err := service.RemoveItinerary(tt.ctx, tt.request)
			
			if tt.expectError {
				assert.Error(t, err)
				st, ok := status.FromError(err)
				assert.True(t, ok)
				assert.Equal(t, tt.errorCode, st.Code())
				assert.Nil(t, resp)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, resp)
				assert.True(t, resp.Success)
			}
			
			mockRepo.AssertExpectations(t)
		})
	}
}

// Test GetPOIDetails
func TestService_GetPOIDetails(t *testing.T) {
	service, _ := createTestChatService(t)

	tests := []struct {
		name        string
		ctx         context.Context
		request     *pb.GetPOIDetailsRequest
		expectError bool
		errorCode   codes.Code
	}{
		{
			name: "unimplemented method",
			ctx:  context.Background(),
			request: &pb.GetPOIDetailsRequest{
				PoiId: uuid.New().String(),
			},
			expectError: true,
			errorCode:   codes.Unimplemented,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := service.GetPOIDetails(tt.ctx, tt.request)
			
			if tt.expectError {
				assert.Error(t, err)
				st, ok := status.FromError(err)
				assert.True(t, ok)
				assert.Equal(t, tt.errorCode, st.Code())
				assert.Nil(t, resp)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, resp)
			}
		})
	}
}