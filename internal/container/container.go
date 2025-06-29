package container

import (
	"context"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	database "github.com/FACorreiaa/go-poi-au-suggestions/app/db"
	"github.com/FACorreiaa/go-poi-au-suggestions/config"
	"github.com/FACorreiaa/go-poi-au-suggestions/internal/api/auth"
	llmChat "github.com/FACorreiaa/go-poi-au-suggestions/internal/api/chat_prompt"
	"github.com/FACorreiaa/go-poi-au-suggestions/internal/api/city"
	generativeAI "github.com/FACorreiaa/go-poi-au-suggestions/internal/api/generative_ai"
	"github.com/FACorreiaa/go-poi-au-suggestions/internal/api/interests"
	itineraryList "github.com/FACorreiaa/go-poi-au-suggestions/internal/api/list"
	"github.com/FACorreiaa/go-poi-au-suggestions/internal/api/poi"
	"github.com/FACorreiaa/go-poi-au-suggestions/internal/api/profiles"
	"github.com/FACorreiaa/go-poi-au-suggestions/internal/api/recents"
	"github.com/FACorreiaa/go-poi-au-suggestions/internal/api/statistics"
	"github.com/FACorreiaa/go-poi-au-suggestions/internal/api/tags"
	"github.com/FACorreiaa/go-poi-au-suggestions/internal/api/user"
)

// Container holds all application dependencies
type Container struct {
	Config                    *config.Config
	Logger                    *slog.Logger
	Pool                      *pgxpool.Pool
	AuthHandler               *auth.HandlerImpl
	UserHandler               *user.HandlerImpl
	InterestHandler           *interests.HandlerImpl
	TagsHandler               *tags.HandlerImpl
	SearchProfileHandler      *profiles.HandlerImpl
	LLMInteractionHandlerImpl *llmChat.HandlerImpl
	POIHandler                *poi.HandlerImpl
	ItineraryListHandler      *itineraryList.HandlerImpl
	CityHandler               *city.Handler
	RecentsHandler            *recents.HandlerImpl
	StatisticsHandler         *statistics.HandlerImpl
	// Add other HandlerImpls, services, and repositories as needed
}

// NewContainer initializes and returns a new dependency container
func NewContainer(cfg *config.Config, logger *slog.Logger) (*Container, error) {
	// Initialize database
	dbConfig, err := database.NewDatabaseConfig(cfg, logger)
	if err != nil {
		logger.Error("Failed to generate database config", slog.Any("error", err))
		return nil, err
	}

	pool, err := database.Init(dbConfig.ConnectionURL, logger)
	if err != nil {
		logger.Error("Failed to initialize database pool", slog.Any("error", err))
		return nil, err
	}

	// Initialize repositories
	authRepo := auth.NewPostgresAuthRepo(pool, logger)

	// Initialize services
	authService := auth.NewAuthService(authRepo, cfg, logger)

	// Initialize HandlerImpls
	authHandlerImpl := auth.NewAuthHandlerImpl(authService, logger)

	//
	userRepo := user.NewPostgresUserRepo(pool, logger)
	userService := user.NewUserService(userRepo, logger)
	userHandlerImpl := user.NewHandlerImpl(userService, logger)

	interestsRepo := interests.NewRepositoryImpl(pool, logger)
	interestsService := interests.NewinterestsService(interestsRepo, logger)
	HandlerImpl := interests.NewHandlerImpl(interestsService, logger)

	tagsRepo := tags.NewRepositoryImpl(pool, logger)
	tagsService := tags.NewtagsService(tagsRepo, logger)
	tagsHandler := tags.NewHandlerImpl(tagsService, logger)

	profilessRepo := profiles.NewPostgresUserRepo(pool, logger)
	profilessService := profiles.NewUserProfilesService(profilessRepo, interestsRepo, tagsRepo, logger)
	profilessHandlerImpl := profiles.NewUserHandlerImpl(profilessService, logger)
	// Create and return the container

	// city repository, service, and handler
	cityRepo := city.NewCityRepository(pool, logger)
	cityService := city.NewCityService(cityRepo, logger)
	cityHandler := city.NewCityHandler(cityService, logger)

	poiRepo := poi.NewRepository(pool, logger)
	// initialise the LLM interaction service
	llmInteractionRepo := llmChat.NewRepositoryImpl(pool, logger)
	llmInteractionService := llmChat.NewLlmInteractiontService(interestsRepo,
		profilessRepo,
		profilessService,
		tagsRepo,
		llmInteractionRepo,
		cityRepo,
		poiRepo,
		logger)
	llmInteractionHandlerImpl := llmChat.NewLLMHandlerImpl(llmInteractionService, logger)

	embeddingService, _ := generativeAI.NewEmbeddingService(context.Background(), logger)
	poiRepository := poi.NewRepository(pool, logger)
	poiService := poi.NewServiceImpl(poiRepository, embeddingService, cityRepo, logger)
	poiHandler := poi.NewHandlerImpl(poiService, logger)

	itineraryListRepository := itineraryList.NewRepository(pool, logger)
	itineraryLisrService := itineraryList.NewServiceImpl(itineraryListRepository, logger)
	itineraryListHandler := itineraryList.NewHandler(itineraryLisrService, logger)

	// Initialize recents components
	recentsRepository := recents.NewRepository(pool, logger)
	recentsService := recents.NewService(recentsRepository, logger)
	recentsHandler := recents.NewHandler(recentsService, logger)

	// Initialize statistics components
	statisticsRepository := statistics.NewRepository(logger, pool)
	statisticsHandler := statistics.NewHandler(statisticsRepository, logger, cfg.JWT)

	return &Container{
		Config:                    cfg,
		Logger:                    logger,
		Pool:                      pool,
		AuthHandler:               authHandlerImpl,
		UserHandler:               userHandlerImpl,
		InterestHandler:           HandlerImpl,
		TagsHandler:               tagsHandler,
		SearchProfileHandler:      profilessHandlerImpl,
		LLMInteractionHandlerImpl: llmInteractionHandlerImpl,
		POIHandler:                poiHandler,
		ItineraryListHandler:      itineraryListHandler,
		CityHandler:               cityHandler,
		RecentsHandler:            recentsHandler,
		StatisticsHandler:         statisticsHandler,
		// Add other HandlerImpls, services, and repositories as needed
	}, nil
}

// Close releases all resources held by the container
func (c *Container) Close() {
	if c.Pool != nil {
		c.Pool.Close()
	}
}

// WaitForDB waits for the database to be ready
func (c *Container) WaitForDB(ctx context.Context) bool {
	return database.WaitForDB(ctx, c.Pool, c.Logger)
}

// RunMigrations runs database migrations
func (c *Container) RunMigrations(connectionURL string) error {
	return database.RunMigrations(connectionURL, c.Logger)
}
