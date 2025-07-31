package broker

import (
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	"github.com/FACorreiaa/go-poi-au-suggestions/config"
)

// ServiceFactory creates and registers services
type ServiceFactory struct {
	broker *Broker
	cfg    *config.Config
	logger *zap.Logger
	db     *pgxpool.Pool
}

func NewServiceFactory(broker *Broker, cfg *config.Config, logger *zap.Logger, db *pgxpool.Pool) *ServiceFactory {
	return &ServiceFactory{
		broker: broker,
		cfg:    cfg,
		logger: logger,
		db:     db,
	}
}

func (f *ServiceFactory) CreateAndRegisterServices() error {
	// Create and register Auth Service
	authService, err := NewAuthService(f.cfg, f.logger, f.db)
	if err != nil {
		f.logger.Error("Failed to create auth service", zap.Error(err))
		return err
	}

	f.broker.RegisterService(authService)
	f.logger.Info("Auth service registered successfully")

	// Create and register User Service
	userService, err := NewUserService(f.cfg, f.logger, f.db)
	if err != nil {
		f.logger.Error("Failed to create user service", zap.Error(err))
		return err
	}

	f.broker.RegisterService(userService)
	f.logger.Info("User service registered successfully")

	tagsService, err := NewTagService(f.cfg, f.logger, f.db)
	if err != nil {
		f.logger.Error("Failed to create tag service", zap.Error(err))
		return err
	}

	f.broker.RegisterService(tagsService)
	f.logger.Info("Tag service registered successfully")

	interestsService, err := NewInterestsService(f.cfg, f.logger, f.db)
	if err != nil {
		f.logger.Error("Failed to create interest service", zap.Error(err))
		return err
	}

	f.broker.RegisterService(interestsService)
	f.logger.Info("Interest service registered successfully")

	profilesService, err := NewProfilesService(f.cfg, f.logger, f.db)
	if err != nil {
		f.logger.Error("Failed to create profiles service", zap.Error(err))
		return err
	}
	f.broker.RegisterService(profilesService)
	f.logger.Info("Profiles service registered successfully")

	cityService, err := NewCityService(f.cfg, f.logger, f.db)
	if err != nil {
		f.logger.Error("Failed to create city service", zap.Error(err))
		return err
	}
	f.broker.RegisterService(cityService)
	f.logger.Info("City service registered successfully")

	listsService, err := NewListsService(f.cfg, f.logger, f.db)
	if err != nil {
		f.logger.Error("Failed to create lists service", zap.Error(err))
		return err
	}
	f.broker.RegisterService(listsService)
	f.logger.Info("Lists service registered successfully")

	// Create and register Chat Prompt Service
	chatPromptService, err := NewChatPromptService(f.cfg, f.logger, f.db)
	if err != nil {
		f.logger.Error("Failed to create chat prompt service", zap.Error(err))
		return err
	}
	f.broker.RegisterService(chatPromptService)
	f.logger.Info("Chat prompt service registered successfully")

	// Create and register Recents Service
	recentsService, err := NewRecentsService(f.cfg, f.logger, f.db)
	if err != nil {
		f.logger.Error("Failed to create recents service", zap.Error(err))
		return err
	}
	f.broker.RegisterService(recentsService)
	f.logger.Info("Recents service registered successfully")

	// Create and register POI Service
	poiService, err := NewPOIService(f.cfg, f.logger, f.db)
	if err != nil {
		f.logger.Error("Failed to create POI service", zap.Error(err))
		return err
	}
	f.broker.RegisterService(poiService)
	f.logger.Info("POI service registered successfully")

	// Create and register Statistics Service
	statisticsService, err := NewStatisticsService(f.cfg, f.logger, f.db)
	if err != nil {
		f.logger.Error("Failed to create statistics service", zap.Error(err))
		return err
	}
	f.broker.RegisterService(statisticsService)
	f.logger.Info("Statistics service registered successfully")

	return nil
}
