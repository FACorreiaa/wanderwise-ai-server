package broker

import (
	"context"
	"sync"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"

	"github.com/FACorreiaa/go-poi-au-suggestions/config"
)

// ServiceType represents different service types
type ServiceType string

const (
	AuthService        ServiceType = "auth"
	UserService        ServiceType = "user"
	TagsService        ServiceType = "tags"
	InterestsService   ServiceType = "interests"
	ProfilesService    ServiceType = "profiles"
	CityService        ServiceType = "city"
	ListsService       ServiceType = "lists"
	ChatPromptService  ServiceType = "chat_prompt"
	// Additional services will be added here as needed
)

// Service represents a service in the broker
type Service interface {
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	Health() error
	Type() ServiceType
}

// Message represents a message between services
type Message struct {
	Type    string
	Payload interface{}
	From    ServiceType
	To      ServiceType
}

// MessageHandler handles messages
type MessageHandler func(msg *Message) error

// Broker manages services and message routing
type Broker struct {
	config      *config.Config
	logger      *zap.Logger
	db          *pgxpool.Pool
	registry    *prometheus.Registry
	services    map[ServiceType]Service
	handlers    map[string]MessageHandler
	messageChan chan *Message
	ctx         context.Context
	cancel      context.CancelFunc
	wg          sync.WaitGroup
	mu          sync.RWMutex
}

func NewBroker(cfg *config.Config, logger *zap.Logger, db *pgxpool.Pool, registry *prometheus.Registry) *Broker {
	ctx, cancel := context.WithCancel(context.Background())

	return &Broker{
		config:      cfg,
		logger:      logger,
		db:          db,
		registry:    registry,
		services:    make(map[ServiceType]Service),
		handlers:    make(map[string]MessageHandler),
		messageChan: make(chan *Message, 100),
		ctx:         ctx,
		cancel:      cancel,
	}
}

func (b *Broker) RegisterService(service Service) {
	b.mu.Lock()
	defer b.mu.Unlock()

	serviceType := service.Type()
	b.services[serviceType] = service
	b.logger.Info("Service registered", zap.String("type", string(serviceType)))
}

func (b *Broker) RegisterHandler(messageType string, handler MessageHandler) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.handlers[messageType] = handler
	b.logger.Info("Handler registered", zap.String("message_type", messageType))
}

// Start starts all registered services
func (b *Broker) Start(ctx context.Context) error {
	b.mu.RLock()
	services := make([]Service, 0, len(b.services))
	for _, service := range b.services {
		services = append(services, service)
	}
	b.mu.RUnlock()

	// Start message processor
	b.wg.Add(1)
	go b.processMessages()

	// Start all services
	for _, service := range services {
		b.wg.Add(1)
		go func(s Service) {
			defer b.wg.Done()
			if err := s.Start(ctx); err != nil {
				b.logger.Error("Service failed to start",
					zap.String("type", string(s.Type())),
					zap.Error(err))
			}
		}(service)
	}

	b.logger.Info("Broker started", zap.Int("services", len(services)))
	return nil
}

// Stop stops all services
func (b *Broker) Stop() error {
	b.cancel()

	b.mu.RLock()
	services := make([]Service, 0, len(b.services))
	for _, service := range b.services {
		services = append(services, service)
	}
	b.mu.RUnlock()

	// Stop all services
	for _, service := range services {
		if err := service.Stop(b.ctx); err != nil {
			b.logger.Error("Service failed to stop",
				zap.String("type", string(service.Type())),
				zap.Error(err))
		}
	}

	close(b.messageChan)
	b.wg.Wait()

	b.logger.Info("Broker stopped")
	return nil
}

// SendMessage sends a message through the broker
func (b *Broker) SendMessage(msg *Message) error {
	select {
	case b.messageChan <- msg:
		return nil
	case <-b.ctx.Done():
		return b.ctx.Err()
	}
}

// GetService returns a service by type
func (b *Broker) GetService(serviceType ServiceType) (Service, bool) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	service, exists := b.services[serviceType]
	return service, exists
}

// GetAllServices returns all registered services
func (b *Broker) GetAllServices() map[ServiceType]Service {
	b.mu.RLock()
	defer b.mu.RUnlock()

	services := make(map[ServiceType]Service, len(b.services))
	for k, v := range b.services {
		services[k] = v
	}
	return services
}

// HealthCheck checks health of all services
func (b *Broker) HealthCheck() map[ServiceType]error {
	b.mu.RLock()
	defer b.mu.RUnlock()

	results := make(map[ServiceType]error)
	for serviceType, service := range b.services {
		results[serviceType] = service.Health()
	}
	return results
}

// processMessages processes messages from the message channel
func (b *Broker) processMessages() {
	defer b.wg.Done()

	for {
		select {
		case msg, ok := <-b.messageChan:
			if !ok {
				return
			}

			b.mu.RLock()
			handler, exists := b.handlers[msg.Type]
			b.mu.RUnlock()

			if exists {
				if err := handler(msg); err != nil {
					b.logger.Error("Message handler failed",
						zap.String("message_type", msg.Type),
						zap.Error(err))
				}
			} else {
				b.logger.Warn("No handler for message type",
					zap.String("message_type", msg.Type))
			}

		case <-b.ctx.Done():
			return
		}
	}
}

// Config returns the broker configuration
func (b *Broker) Config() *config.Config {
	return b.config
}

// Logger returns the broker logger
func (b *Broker) Logger() *zap.Logger {
	return b.logger
}

// DB returns the database pool
func (b *Broker) DB() *pgxpool.Pool {
	return b.db
}

// Registry returns the prometheus registry
func (b *Broker) Registry() *prometheus.Registry {
	return b.registry
}
