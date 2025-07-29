package profiles

import (
	"go.uber.org/zap"
)

type ProfileRepository struct {
	logger *zap.Logger
}

func NewProfileRepository(logger *zap.Logger) *ProfileRepository {
	return &ProfileRepository{
		logger: logger,
	}
}
