package interests

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/FACorreiaa/go-poi-au-suggestions/internal/types"
)

// MockinterestsRepo is a mock implementation of interestsRepo
type MockinterestsRepo struct {
	mock.Mock
}

func (m *MockinterestsRepo) CreateInterest(ctx context.Context, name string, description *string, isActive bool, userID string) (*types.Interest, error) {
	args := m.Called(ctx, name, description, isActive, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*types.Interest), args.Error(1)
}

func (m *MockinterestsRepo) Removeinterests(ctx context.Context, userID uuid.UUID, interestID uuid.UUID) error {
	args := m.Called(ctx, userID, interestID)
	return args.Error(0)
}

func (m *MockinterestsRepo) GetAllInterests(ctx context.Context) ([]*types.Interest, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*types.Interest), args.Error(1)
}

func (m *MockinterestsRepo) Updateinterests(ctx context.Context, userID uuid.UUID, interestID uuid.UUID, params types.UpdateinterestsParams) error {
	args := m.Called(ctx, userID, interestID, params)
	return args.Error(0)
}

func (m *MockinterestsRepo) GetInterest(ctx context.Context, interestID uuid.UUID) (*types.Interest, error) {
	args := m.Called(ctx, interestID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*types.Interest), args.Error(1)
}

func (m *MockinterestsRepo) AddInterestToProfile(ctx context.Context, profileID uuid.UUID, interestID uuid.UUID) error {
	args := m.Called(ctx, profileID, interestID)
	return args.Error(0)
}

func (m *MockinterestsRepo) GetInterestsForProfile(ctx context.Context, profileID uuid.UUID) ([]*types.Interest, error) {
	args := m.Called(ctx, profileID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*types.Interest), args.Error(1)
}

func TestCreateInterest(t *testing.T) {
	// Setup
	mockRepo := new(MockinterestsRepo)
	logger := slog.Default()
	service := NewinterestsService(mockRepo, logger)
	ctx := context.Background()

	// Test data
	name := "Test Interest"
	description := "Test Description"
	isActive := true
	userID := "user123"
	active := isActive
	expectedInterest := &types.Interest{
		ID:          uuid.New(),
		Name:        name,
		Description: &description,
		Active:      &active,
		CreatedAt:   time.Now(),
		Source:      "test",
	}

	// Test cases
	tests := []struct {
		name          string
		setupMock     func()
		expectedError bool
	}{
		{
			name: "Success",
			setupMock: func() {
				mockRepo.On("CreateInterest", mock.Anything, name, &description, isActive, userID).Return(expectedInterest, nil)
			},
			expectedError: false,
		},
		{
			name: "Repository Error",
			setupMock: func() {
				mockRepo.On("CreateInterest", mock.Anything, name, &description, isActive, userID).Return(nil, errors.New("repository error"))
			},
			expectedError: true,
		},
	}

	// Run tests
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup mock
			tc.setupMock()

			// Call the method
			interest, err := service.CreateInterest(ctx, name, &description, isActive, userID)

			// Assertions
			if tc.expectedError {
				assert.Error(t, err)
				assert.Nil(t, interest)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, expectedInterest, interest)
			}

			// Verify mock
			mockRepo.AssertExpectations(t)
		})
	}
}

func TestRemoveinterests(t *testing.T) {
	// Setup
	mockRepo := new(MockinterestsRepo)
	logger := slog.Default()
	service := NewinterestsService(mockRepo, logger)
	ctx := context.Background()

	// Test data
	userID := uuid.New()
	interestID := uuid.New()

	// Test cases
	tests := []struct {
		name          string
		setupMock     func()
		expectedError bool
	}{
		{
			name: "Success",
			setupMock: func() {
				mockRepo.On("Removeinterests", mock.Anything, userID, interestID).Return(nil)
			},
			expectedError: false,
		},
		{
			name: "Repository Error",
			setupMock: func() {
				mockRepo.On("Removeinterests", mock.Anything, userID, interestID).Return(errors.New("repository error"))
			},
			expectedError: true,
		},
	}

	// Run tests
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup mock
			tc.setupMock()

			// Call the method
			err := service.Removeinterests(ctx, userID, interestID)

			// Assertions
			if tc.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			// Verify mock
			mockRepo.AssertExpectations(t)
		})
	}
}

func TestGetAllInterests(t *testing.T) {
	// Setup
	mockRepo := new(MockinterestsRepo)
	logger := slog.Default()
	service := NewinterestsService(mockRepo, logger)
	ctx := context.Background()

	// Test data
	active1 := true
	active2 := true
	now := time.Now()
	expectedInterests := []*types.Interest{
		{
			ID:        uuid.New(),
			Name:      "Interest 1",
			Active:    &active1,
			CreatedAt: now,
			Source:    "test",
		},
		{
			ID:        uuid.New(),
			Name:      "Interest 2",
			Active:    &active2,
			CreatedAt: now,
			Source:    "test",
		},
	}

	// Test cases
	tests := []struct {
		name          string
		setupMock     func()
		expectedError bool
	}{
		{
			name: "Success",
			setupMock: func() {
				mockRepo.On("GetAllInterests", mock.Anything).Return(expectedInterests, nil)
			},
			expectedError: false,
		},
		{
			name: "Repository Error",
			setupMock: func() {
				mockRepo.On("GetAllInterests", mock.Anything).Return(nil, errors.New("repository error"))
			},
			expectedError: true,
		},
	}

	// Run tests
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup mock
			tc.setupMock()

			// Call the method
			interests, err := service.GetAllInterests(ctx)

			// Assertions
			if tc.expectedError {
				assert.Error(t, err)
				assert.Nil(t, interests)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, expectedInterests, interests)
			}

			// Verify mock
			mockRepo.AssertExpectations(t)
		})
	}
}

func TestUpdateinterests(t *testing.T) {
	// Setup
	mockRepo := new(MockinterestsRepo)
	logger := slog.Default()
	service := NewinterestsService(mockRepo, logger)
	ctx := context.Background()

	// Test data
	userID := uuid.New()
	interestID := uuid.New()
	name := "Updated Interest"
	description := "Updated Description"
	active := true
	params := types.UpdateinterestsParams{
		Name:        &name,
		Description: &description,
		Active:      &active,
	}

	// Test cases
	tests := []struct {
		name          string
		setupMock     func()
		expectedError bool
	}{
		{
			name: "Success",
			setupMock: func() {
				mockRepo.On("Updateinterests", mock.Anything, userID, interestID, params).Return(nil)
			},
			expectedError: false,
		},
		{
			name: "Repository Error",
			setupMock: func() {
				mockRepo.On("Updateinterests", mock.Anything, userID, interestID, params).Return(errors.New("repository error"))
			},
			expectedError: true,
		},
	}

	// Run tests
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup mock
			tc.setupMock()

			// Call the method
			err := service.Updateinterests(ctx, userID, interestID, params)

			// Assertions
			if tc.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			// Verify mock
			mockRepo.AssertExpectations(t)
		})
	}
}
