package tags

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"testing"

	"github.com/FACorreiaa/go-poi-au-suggestions/internal/types" // Adjust path
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// MocktagsRepo is a mock implementation of tagsRepo
type MocktagsRepo struct {
	mock.Mock
}

func (m *MocktagsRepo) GetAll(ctx context.Context, userID uuid.UUID) ([]*types.Tags, error) {
	args := m.Called(ctx, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*types.Tags), args.Error(1)
}

func (m *MocktagsRepo) Get(ctx context.Context, userID, tagID uuid.UUID) (*types.Tags, error) {
	args := m.Called(ctx, userID, tagID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*types.Tags), args.Error(1)
}

func (m *MocktagsRepo) Create(ctx context.Context, userID uuid.UUID, params types.CreatePersonalTagParams) (*types.PersonalTag, error) {
	args := m.Called(ctx, userID, params)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*types.PersonalTag), args.Error(1)
}

func (m *MocktagsRepo) Delete(ctx context.Context, userID uuid.UUID, tagID uuid.UUID) error {
	args := m.Called(ctx, userID, tagID)
	return args.Error(0)
}

func (m *MocktagsRepo) Update(ctx context.Context, userID uuid.UUID, tagID uuid.UUID, params types.UpdatePersonalTagParams) error {
	args := m.Called(ctx, userID, tagID, params)
	return args.Error(0)
}

func (m *MocktagsRepo) GetTagByName(ctx context.Context, name string) (*types.Tags, error) {
	args := m.Called(ctx, name)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*types.Tags), args.Error(1)
}

func (m *MocktagsRepo) LinkPersonalTagToProfile(ctx context.Context, userID, profileID uuid.UUID, tagID uuid.UUID) error {
	args := m.Called(ctx, userID, profileID, tagID)
	return args.Error(0)
}

func (m *MocktagsRepo) GetTagsForProfile(ctx context.Context, profileID uuid.UUID) ([]*types.Tags, error) {
	args := m.Called(ctx, profileID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*types.Tags), args.Error(1)
}

// Helper to setup service with mock repository
func setuptagsServiceTest() (*tagsServiceImpl, *MocktagsRepo) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug})) // or io.Discard
	mockRepo := new(MocktagsRepo)
	service := NewtagsService(mockRepo, logger)
	return service, mockRepo
}

func TestTagsServiceImpl_GetTags(t *testing.T) {
	service, mockRepo := setuptagsServiceTest()
	ctx := context.Background()
	userID := uuid.New()

	t.Run("success - tags found", func(t *testing.T) {
		expectedTags := []*types.Tags{
			{ID: uuid.New(), Name: "Outdoors", TagType: "preference"},
			{ID: uuid.New(), Name: "Foodie", TagType: "preference"},
		}
		mockRepo.On("GetAll", mock.Anything, userID).Return(expectedTags, nil).Once()

		tags, err := service.GetTags(ctx, userID)
		require.NoError(t, err)
		assert.Equal(t, expectedTags, tags)
		mockRepo.AssertExpectations(t)
	})

	t.Run("success - no tags found", func(t *testing.T) {
		var expectedTags []*types.Tags
		mockRepo.On("GetAll", ctx, userID).Return(expectedTags, nil).Once()

		tags, err := service.GetTags(ctx, userID)
		require.NoError(t, err)
		assert.Empty(t, tags)
		mockRepo.AssertExpectations(t)
	})

	t.Run("repository error", func(t *testing.T) {
		repoErr := errors.New("db error fetching all tags")
		mockRepo.On("GetAll", mock.Anything, userID).Return(nil, repoErr).Once()

		_, err := service.GetTags(ctx, userID)
		require.Error(t, err)
		assert.True(t, errors.Is(err, repoErr))
		assert.Contains(t, err.Error(), "error fetching all global tags:")
		mockRepo.AssertExpectations(t)
	})
}

func TestTagsServiceImpl_GetTag(t *testing.T) {
	service, mockRepo := setuptagsServiceTest()
	ctx := context.Background()
	userID := uuid.New()
	tagID := uuid.New()

	t.Run("success", func(t *testing.T) {
		expectedTag := &types.Tags{ID: tagID, Name: "Specific Tag", TagType: "preference"}
		mockRepo.On("Get", mock.Anything, userID, tagID).Return(expectedTag, nil).Once()

		tag, err := service.GetTag(ctx, userID, tagID)
		require.NoError(t, err)
		assert.Equal(t, expectedTag, tag)
		mockRepo.AssertExpectations(t)
	})

	t.Run("repository error - not found", func(t *testing.T) {
		repoErr := errors.New("tag not found in repo") // Or types.ErrNotFound
		mockRepo.On("Get", mock.Anything, userID, tagID).Return(nil, repoErr).Once()

		_, err := service.GetTag(ctx, userID, tagID)
		require.Error(t, err)
		assert.True(t, errors.Is(err, repoErr))
		assert.Contains(t, err.Error(), "error fetching user avoid tags:") // Log message uses "avoid tags"
		mockRepo.AssertExpectations(t)
	})
}

func TestTagsServiceImpl_CreateTag(t *testing.T) {
	service, mockRepo := setuptagsServiceTest()
	ctx := context.Background()
	userID := uuid.New()
	desc := "Loves spicy food"
	params := types.CreatePersonalTagParams{
		Name:        "SpicyLover",
		Description: desc,
		TagType:     "preference",
	}
	expectedPersonalTag := &types.PersonalTag{
		ID:          uuid.New(),
		UserID:      userID,
		Name:        params.Name,
		Description: &desc,
		TagType:     "preference",
	}

	t.Run("success", func(t *testing.T) {
		mockRepo.On("Create", mock.Anything, userID, params).Return(expectedPersonalTag, nil).Once()

		tag, err := service.CreateTag(ctx, userID, params)
		require.NoError(t, err)
		assert.Equal(t, expectedPersonalTag, tag)
		mockRepo.AssertExpectations(t)
	})

	t.Run("repository error", func(t *testing.T) {
		repoErr := errors.New("db error creating tag")
		mockRepo.On("Create", mock.Anything, userID, params).Return(nil, repoErr).Once()

		_, err := service.CreateTag(ctx, userID, params)
		require.Error(t, err)
		assert.True(t, errors.Is(err, repoErr))
		assert.Contains(t, err.Error(), "error adding user avoid tag:")
		mockRepo.AssertExpectations(t)
	})
}

func TestTagsServiceImpl_DeleteTag(t *testing.T) {
	service, mockRepo := setuptagsServiceTest()
	ctx := context.Background()
	userID := uuid.New()
	tagID := uuid.New()

	t.Run("success", func(t *testing.T) {
		mockRepo.On("Delete", mock.Anything, userID, tagID).Return(nil).Once()

		err := service.DeleteTag(ctx, userID, tagID)
		require.NoError(t, err)
		mockRepo.AssertExpectations(t)
	})

	t.Run("repository error", func(t *testing.T) {
		repoErr := errors.New("db error deleting tag")
		mockRepo.On("Delete", mock.Anything, userID, tagID).Return(repoErr).Once()

		err := service.DeleteTag(ctx, userID, tagID)
		require.Error(t, err)
		assert.True(t, errors.Is(err, repoErr))
		assert.Contains(t, err.Error(), "error removing user avoid tag:")
		mockRepo.AssertExpectations(t)
	})
}

func TestTagsServiceImpl_Update(t *testing.T) {
	service, mockRepo := setuptagsServiceTest()
	ctx := context.Background()
	userID := uuid.New()
	tagID := uuid.New()
	newName := "Updated Tag Name"
	newDesc := "Updated description for tag"
	params := types.UpdatePersonalTagParams{
		Name:        newName,
		Description: newDesc,
		TagType:     "preference",
	}

	t.Run("success", func(t *testing.T) {
		mockRepo.On("Update", mock.Anything, userID, tagID, params).Return(nil).Once()

		err := service.Update(ctx, userID, tagID, params)
		require.NoError(t, err)
		mockRepo.AssertExpectations(t)
	})

	t.Run("repository error", func(t *testing.T) {
		repoErr := errors.New("db error updating tag")
		mockRepo.On("Update", mock.Anything, userID, tagID, params).Return(repoErr).Once()

		err := service.Update(ctx, userID, tagID, params)
		require.Error(t, err)
		assert.True(t, errors.Is(err, repoErr))
		assert.Contains(t, err.Error(), "error updating user avoid tag:")
		mockRepo.AssertExpectations(t)
	})

	t.Run("no fields to update (service passes to repo)", func(t *testing.T) {
		emptyParams := types.UpdatePersonalTagParams{}
		mockRepo.On("Update", ctx, userID, tagID, emptyParams).Return(nil).Once()

		err := service.Update(ctx, userID, tagID, emptyParams)
		require.NoError(t, err) // Assuming repo handles empty updates gracefully
		mockRepo.AssertExpectations(t)
	})
}
