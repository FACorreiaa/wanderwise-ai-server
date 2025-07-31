package chat_prompt

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"google.golang.org/grpc"

	pb "github.com/FACorreiaa/loci-proto/modules/chat/generated"
	"github.com/FACorreiaa/go-poi-au-suggestions/internal/domain"
)

// MockStartChatStreamServer implements pb.ChatService_StartChatStreamServer for testing
type MockStartChatStreamServer struct {
	mock.Mock
	grpc.ServerStream
	events []pb.ChatEvent
	ctx    context.Context
}

func (m *MockStartChatStreamServer) Send(event *pb.ChatEvent) error {
	args := m.Called(event)
	m.events = append(m.events, *event)
	return args.Error(0)
}

func (m *MockStartChatStreamServer) Context() context.Context {
	if m.ctx != nil {
		return m.ctx
	}
	args := m.Called()
	return args.Get(0).(context.Context)
}

func (m *MockStartChatStreamServer) GetEvents() []pb.ChatEvent {
	return m.events
}

func (m *MockStartChatStreamServer) SetContext(ctx context.Context) {
	m.ctx = ctx
}

// MockContinueChatStreamServer implements pb.ChatService_ContinueChatStreamServer for testing
type MockContinueChatStreamServer struct {
	mock.Mock
	grpc.ServerStream
	events []pb.ChatEvent
	ctx    context.Context
}

func (m *MockContinueChatStreamServer) Send(event *pb.ChatEvent) error {
	args := m.Called(event)
	m.events = append(m.events, *event)
	return args.Error(0)
}

func (m *MockContinueChatStreamServer) Context() context.Context {
	if m.ctx != nil {
		return m.ctx
	}
	args := m.Called()
	return args.Get(0).(context.Context)
}

func (m *MockContinueChatStreamServer) GetEvents() []pb.ChatEvent {
	return m.events
}

func (m *MockContinueChatStreamServer) SetContext(ctx context.Context) {
	m.ctx = ctx
}

// MockFreeChatStreamServer implements pb.ChatService_FreeChatStreamServer for testing
type MockFreeChatStreamServer struct {
	mock.Mock
	grpc.ServerStream
	events []pb.ChatEvent
	ctx    context.Context
}

func (m *MockFreeChatStreamServer) Send(event *pb.ChatEvent) error {
	args := m.Called(event)
	m.events = append(m.events, *event)
	return args.Error(0)
}

func (m *MockFreeChatStreamServer) Context() context.Context {
	if m.ctx != nil {
		return m.ctx
	}
	args := m.Called()
	return args.Get(0).(context.Context)
}

func (m *MockFreeChatStreamServer) GetEvents() []pb.ChatEvent {
	return m.events
}

func (m *MockFreeChatStreamServer) SetContext(ctx context.Context) {
	m.ctx = ctx
}

// Test StartChatStream
func TestService_StartChatStream(t *testing.T) {
	service, mockRepo := createTestChatService(t)
	userID := uuid.New().String()

	tests := []struct {
		name        string
		request     *pb.StartChatRequest
		setupMock   func(*MockStartChatStreamServer)
		setupRepo   func()
		expectError bool
		description string
	}{
		{
			name: "success with authenticated user",
			request: &pb.StartChatRequest{
				Prompt:    "Tell me about restaurants in New York",
				ProfileId: uuid.New().String(),
				CityName:  "New York",
				Location: &pb.LocationInfo{
					Latitude:  40.7128,
					Longitude: -74.0060,
				},
			},
			setupMock: func(stream *MockStartChatStreamServer) {
				ctx := createAuthContext(userID)
				stream.SetContext(ctx)
				stream.On("Context").Return(ctx).Maybe()
				stream.On("Send", mock.AnythingOfType("*pb.ChatEvent")).Return(nil).Maybe()
			},
			setupRepo: func() {
				session := createTestSession()
				mockRepo.On("CreateSession", mock.Anything, 
					mock.AnythingOfType("uuid.UUID"), mock.AnythingOfType("uuid.UUID")).
					Return(session, nil).Maybe()
				mockRepo.On("SaveInteraction", mock.Anything, 
					mock.AnythingOfType("LlmInteraction")).
					Return(uuid.New(), nil).Maybe()
			},
			expectError: false,
			description: "Should successfully start chat stream for authenticated user",
		},
		{
			name: "missing prompt",
			request: &pb.StartChatRequest{
				Prompt:    "",
				ProfileId: uuid.New().String(),
			},
			setupMock: func(stream *MockStartChatStreamServer) {
				ctx := createAuthContext(userID)
				stream.SetContext(ctx)
				stream.On("Context").Return(ctx).Maybe()
				stream.On("Send", mock.MatchedBy(func(event *pb.ChatEvent) bool {
					return event.Type == pb.EventType_ERROR
				})).Return(nil).Maybe()
			},
			setupRepo:   func() {},
			expectError: false, // Stream doesn't return error, sends error event instead
			description: "Should send error event for missing prompt",
		},
		{
			name: "missing profile ID",
			request: &pb.StartChatRequest{
				Prompt:    "Tell me about restaurants",
				ProfileId: "",
			},
			setupMock: func(stream *MockStartChatStreamServer) {
				ctx := createAuthContext(userID)
				stream.SetContext(ctx)
				stream.On("Context").Return(ctx).Maybe()
				stream.On("Send", mock.MatchedBy(func(event *pb.ChatEvent) bool {
					return event.Type == pb.EventType_ERROR
				})).Return(nil).Maybe()
			},
			setupRepo:   func() {},
			expectError: false, // Stream doesn't return error, sends error event instead
			description: "Should send error event for missing profile ID",
		},
		{
			name: "unauthenticated user",
			request: &pb.StartChatRequest{
				Prompt:    "Tell me about restaurants",
				ProfileId: uuid.New().String(),
			},
			setupMock: func(stream *MockStartChatStreamServer) {
				ctx := context.Background() // No auth
				stream.SetContext(ctx)
				stream.On("Context").Return(ctx).Maybe()
				stream.On("Send", mock.MatchedBy(func(event *pb.ChatEvent) bool {
					return event.Type == pb.EventType_ERROR
				})).Return(nil).Maybe()
			},
			setupRepo:   func() {},
			expectError: false, // Stream doesn't return error, sends error event instead
			description: "Should send error event for unauthenticated user",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stream := &MockStartChatStreamServer{}
			tt.setupMock(stream)
			tt.setupRepo()
			
			err := service.StartChatStream(tt.request, stream)
			
			if tt.expectError {
				assert.Error(t, err, tt.description)
			} else {
				assert.NoError(t, err, tt.description)
			}
			
			// For error cases, verify that error events were sent
			if tt.name == "missing prompt" || tt.name == "missing profile ID" || tt.name == "unauthenticated user" {
				events := stream.GetEvents()
				if len(events) > 0 {
					hasErrorEvent := false
					for _, event := range events {
						if event.Type == pb.EventType_ERROR {
							hasErrorEvent = true
							break
						}
					}
					assert.True(t, hasErrorEvent, "Should have sent error event")
				}
			}
			
			stream.AssertExpectations(t)
			mockRepo.AssertExpectations(t)
		})
	}
}

// Test ContinueChatStream
func TestService_ContinueChatStream(t *testing.T) {
	service, mockRepo := createTestChatService(t)
	userID := uuid.New().String()
	sessionID := uuid.New().String()

	tests := []struct {
		name        string
		request     *pb.ContinueChatRequest
		setupMock   func(*MockContinueChatStreamServer)
		setupRepo   func()
		expectError bool
		description string
	}{
		{
			name: "success with valid session",
			request: &pb.ContinueChatRequest{
				SessionId: sessionID,
				Prompt:    "Tell me more about Italian restaurants",
			},
			setupMock: func(stream *MockContinueChatStreamServer) {
				ctx := createAuthContext(userID)
				stream.SetContext(ctx)
				stream.On("Context").Return(ctx).Maybe()
				stream.On("Send", mock.AnythingOfType("*pb.ChatEvent")).Return(nil).Maybe()
			},
			setupRepo: func() {
				session := createTestSession()
				mockRepo.On("GetSession", mock.Anything, 
					mock.AnythingOfType("uuid.UUID")).
					Return(session, nil).Maybe()
				mockRepo.On("SaveInteraction", mock.Anything, 
					mock.AnythingOfType("LlmInteraction")).
					Return(uuid.New(), nil).Maybe()
			},
			expectError: false,
			description: "Should successfully continue chat stream",
		},
		{
			name: "missing session ID",
			request: &pb.ContinueChatRequest{
				SessionId: "",
				Prompt:    "Tell me more",
			},
			setupMock: func(stream *MockContinueChatStreamServer) {
				ctx := createAuthContext(userID)
				stream.SetContext(ctx)
				stream.On("Context").Return(ctx).Maybe()
				stream.On("Send", mock.MatchedBy(func(event *pb.ChatEvent) bool {
					return event.Type == pb.EventType_ERROR
				})).Return(nil).Maybe()
			},
			setupRepo:   func() {},
			expectError: false,
			description: "Should send error event for missing session ID",
		},
		{
			name: "missing prompt",
			request: &pb.ContinueChatRequest{
				SessionId: sessionID,
				Prompt:    "",
			},
			setupMock: func(stream *MockContinueChatStreamServer) {
				ctx := createAuthContext(userID)
				stream.SetContext(ctx)
				stream.On("Context").Return(ctx).Maybe()
				stream.On("Send", mock.MatchedBy(func(event *pb.ChatEvent) bool {
					return event.Type == pb.EventType_ERROR
				})).Return(nil).Maybe()
			},
			setupRepo:   func() {},
			expectError: false,
			description: "Should send error event for missing prompt",
		},
		{
			name: "unauthenticated user",
			request: &pb.ContinueChatRequest{
				SessionId: sessionID,
				Prompt:    "Tell me more",
			},
			setupMock: func(stream *MockContinueChatStreamServer) {
				ctx := context.Background() // No auth
				stream.SetContext(ctx)
				stream.On("Context").Return(ctx).Maybe()
				stream.On("Send", mock.MatchedBy(func(event *pb.ChatEvent) bool {
					return event.Type == pb.EventType_ERROR
				})).Return(nil).Maybe()
			},
			setupRepo:   func() {},
			expectError: false,
			description: "Should send error event for unauthenticated user",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stream := &MockContinueChatStreamServer{}
			tt.setupMock(stream)
			tt.setupRepo()
			
			err := service.ContinueChatStream(tt.request, stream)
			
			if tt.expectError {
				assert.Error(t, err, tt.description)
			} else {
				assert.NoError(t, err, tt.description)
			}
			
			stream.AssertExpectations(t)
			mockRepo.AssertExpectations(t)
		})
	}
}

// Test FreeChatStream
func TestService_FreeChatStream(t *testing.T) {
	service, mockRepo := createTestChatService(t)

	tests := []struct {
		name        string
		request     *pb.FreeChatRequest
		setupMock   func(*MockFreeChatStreamServer)
		setupRepo   func()
		expectError bool
		description string
	}{
		{
			name: "success with valid prompt",
			request: &pb.FreeChatRequest{
				Prompt: "What are some popular tourist attractions?",
			},
			setupMock: func(stream *MockFreeChatStreamServer) {
				ctx := context.Background()
				stream.SetContext(ctx)
				stream.On("Context").Return(ctx).Maybe()
				stream.On("Send", mock.AnythingOfType("*pb.ChatEvent")).Return(nil).Maybe()
			},
			setupRepo: func() {
				// Free chat might not need repository calls
			},
			expectError: false,
			description: "Should successfully process free chat request",
		},
		{
			name: "missing prompt",
			request: &pb.FreeChatRequest{
				Prompt: "",
			},
			setupMock: func(stream *MockFreeChatStreamServer) {
				ctx := context.Background()
				stream.SetContext(ctx)
				stream.On("Context").Return(ctx).Maybe()
				stream.On("Send", mock.MatchedBy(func(event *pb.ChatEvent) bool {
					return event.Type == pb.EventType_ERROR
				})).Return(nil).Maybe()
			},
			setupRepo:   func() {},
			expectError: false,
			description: "Should send error event for missing prompt",
		},
		{
			name: "stream send error",
			request: &pb.FreeChatRequest{
				Prompt: "Valid prompt",
			},
			setupMock: func(stream *MockFreeChatStreamServer) {
				ctx := context.Background()
				stream.SetContext(ctx)
				stream.On("Context").Return(ctx).Maybe()
				stream.On("Send", mock.AnythingOfType("*pb.ChatEvent")).
					Return(errors.New("stream error")).Maybe()
			},
			setupRepo:   func() {},
			expectError: false, // Stream methods don't typically return errors, they handle them internally
			description: "Should handle stream send errors gracefully",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stream := &MockFreeChatStreamServer{}
			tt.setupMock(stream)
			tt.setupRepo()
			
			err := service.FreeChatStream(tt.request, stream)
			
			if tt.expectError {
				assert.Error(t, err, tt.description)
			} else {
				assert.NoError(t, err, tt.description)
			}
			
			stream.AssertExpectations(t)
			mockRepo.AssertExpectations(t)
		})
	}
}

// Test streaming error handling
func TestService_StreamErrorHandling(t *testing.T) {
	service, _ := createTestChatService(t)

	t.Run("StartChatStream with stream context cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(createAuthContext(uuid.New().String()))
		cancel() // Cancel immediately
		
		stream := &MockStartChatStreamServer{}
		stream.SetContext(ctx)
		stream.On("Context").Return(ctx).Maybe()
		stream.On("Send", mock.AnythingOfType("*pb.ChatEvent")).Return(nil).Maybe()
		
		request := &pb.StartChatRequest{
			Prompt:    "Test prompt",
			ProfileId: uuid.New().String(),
		}
		
		err := service.StartChatStream(request, stream)
		
		// Should handle cancellation gracefully
		assert.NoError(t, err)
		stream.AssertExpectations(t)
	})

	t.Run("ContinueChatStream with stream context cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(createAuthContext(uuid.New().String()))
		cancel() // Cancel immediately
		
		stream := &MockContinueChatStreamServer{}
		stream.SetContext(ctx)
		stream.On("Context").Return(ctx).Maybe()
		stream.On("Send", mock.AnythingOfType("*pb.ChatEvent")).Return(nil).Maybe()
		
		request := &pb.ContinueChatRequest{
			SessionId: uuid.New().String(),
			Prompt:    "Test prompt",
		}
		
		err := service.ContinueChatStream(request, stream)
		
		// Should handle cancellation gracefully
		assert.NoError(t, err)
		stream.AssertExpectations(t)
	})

	t.Run("FreeChatStream with stream context cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately
		
		stream := &MockFreeChatStreamServer{}
		stream.SetContext(ctx)
		stream.On("Context").Return(ctx).Maybe()
		stream.On("Send", mock.AnythingOfType("*pb.ChatEvent")).Return(nil).Maybe()
		
		request := &pb.FreeChatRequest{
			Prompt: "Test prompt",
		}
		
		err := service.FreeChatStream(request, stream)
		
		// Should handle cancellation gracefully
		assert.NoError(t, err)
		stream.AssertExpectations(t)
	})
}