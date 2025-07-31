# Broker Pattern Implementation

## Overview

This application implements a **Broker Pattern** to manage and coordinate services within a monolithic architecture. The broker acts as a central service registry and message router, providing a clean abstraction layer for service management and inter-service communication.

## Architecture

```
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│   Main App      │    │     Broker      │    │    Services     │
│                 │    │                 │    │                 │
│  ┌───────────┐  │    │  ┌───────────┐  │    │ ┌─────────────┐ │
│  │   Run()   │──┼────┼──│ Registry  │  │    │ │ AuthService │ │
│  └───────────┘  │    │  └───────────┘  │    │ └─────────────┘ │
│                 │    │                 │    │                 │
│  ┌───────────┐  │    │  ┌───────────┐  │    │ ┌─────────────┐ │
│  │gRPC Server│──┼────┼──│ Message   │  │    │ │Future       │ │
│  └───────────┘  │    │  │ Router    │  │    │ │Services     │ │
│                 │    │  └───────────┘  │    │ └─────────────┘ │
└─────────────────┘    └─────────────────┘    └─────────────────┘
```

## Components

### 1. Broker (`internal/broker/broker.go`)

The central coordinator that manages services and handles message routing.

**Key Features:**
- Service registration and discovery
- Health monitoring
- Message routing between services
- Lifecycle management (start/stop services)

```go
type Broker struct {
    config      *config.Config
    logger      *slog.Logger
    db          *pgxpool.Pool
    registry    *prometheus.Registry
    services    map[ServiceType]Service
    handlers    map[string]MessageHandler
    messageChan chan *Message
    // ... other fields
}
```

### 2. Service Interface (`internal/broker/broker.go`)

All services must implement this interface to be managed by the broker:

```go
type Service interface {
    Start(ctx context.Context) error
    Stop(ctx context.Context) error
    Health() error
    Type() ServiceType
}
```

### 3. Service Types

Currently supported service types:

```go
const (
    AuthService ServiceType = "auth"
    // Additional services can be added here
)
```

## Service Implementation Pattern

### Service Wrapper (`internal/broker/services.go`)

Each service has a wrapper that implements the broker's Service interface:

```go
type AuthServiceImpl struct {
    service *authDomain.Service  // The actual gRPC service
    logger  *slog.Logger
}

func (s *AuthServiceImpl) Start(ctx context.Context) error {
    s.logger.Info("Auth gRPC service started")
    return nil
}

func (s *AuthServiceImpl) GetGRPCService() *authDomain.Service {
    return s.service
}
```

### Service Factory (`internal/broker/services.go`)

Creates and registers services with the broker:

```go
func (f *ServiceFactory) CreateAndRegisterServices() error {
    // Create service instance
    authService, err := NewAuthService(f.cfg, f.logger, f.db)
    if err != nil {
        return err
    }
    
    // Register with broker
    f.broker.RegisterService(authService)
    return nil
}
```

## Usage Flow

### 1. Service Registration

```go
// In main.go
serviceFactory := broker.NewServiceFactory(serviceBroker, cfg, slogLogger, pool)
if err := serviceFactory.CreateAndRegisterServices(); err != nil {
    return nil, err
}
```

### 2. Service Discovery

```go
// In gRPC server setup
if authService, exists := app.Broker.GetService(broker.AuthService); exists {
    if authImpl, ok := authService.(*broker.AuthServiceImpl); ok {
        pb.RegisterAuthServiceServer(server, authImpl.GetGRPCService())
    }
}
```

### 3. Health Monitoring

```go
// Health check endpoint
healthCheck := app.Broker.HealthCheck()
for serviceType, err := range healthCheck {
    if err != nil {
        // Handle unhealthy service
    }
}
```

## Message Routing

The broker supports asynchronous message passing between services:

```go
type Message struct {
    Type    string
    Payload interface{}
    From    ServiceType
    To      ServiceType
}

// Send message
broker.SendMessage(&Message{
    Type:    "user.created",
    Payload: userEvent,
    From:    AuthService,
    To:      ProfileService,
})
```

## Benefits

### 1. **Centralized Service Management**
- Single point for service discovery
- Consistent lifecycle management
- Unified health monitoring

### 2. **Loose Coupling**
- Services don't directly reference each other
- Easy to add/remove services
- Clean separation of concerns

### 3. **Scalability**
- Can easily transition to microservices
- Message routing supports async communication
- Health monitoring enables automatic failover

### 4. **Observability**
- Centralized logging and metrics
- Service health dashboard
- Message tracing capabilities

## Configuration

The broker is configured through the main config system:

```yaml
# config.yaml
server:
  grpc_port: "9000"
  http_port: "8080"

repositories:
  postgres:
    host: "localhost"
    port: "5432"
    # ...
```

## API Endpoints

The broker exposes HTTP endpoints for monitoring:

- `GET /broker/services` - List all registered services
- `GET /broker/health` - Health status of all services
- `GET /health` - Application health check
- `GET /metrics` - Prometheus metrics

## Best Practices

### 1. Service Implementation
- Keep services stateless when possible
- Implement proper error handling in Health() method
- Use structured logging with service context

### 2. Message Design
- Use clear, semantic message types
- Keep payloads serializable
- Version your message schemas

### 3. Health Checks
- Check external dependencies (database, APIs)
- Return specific error information
- Implement circuit breaker patterns

## Future Enhancements

### 1. Service Discovery
- Dynamic service registration
- Service versioning
- Load balancing strategies

### 2. Message Features
- Message persistence
- Dead letter queues
- Message replay capabilities

### 3. Monitoring
- Distributed tracing
- Service dependency mapping
- Performance metrics

## Example: Adding a New Service

1. **Define Service Type:**
```go
const (
    AuthService    ServiceType = "auth"
    ProfileService ServiceType = "profile" // New service
)
```

2. **Implement Service:**
```go
type ProfileServiceImpl struct {
    service *profileDomain.Service
    logger  *slog.Logger
}

func (s *ProfileServiceImpl) Type() ServiceType {
    return ProfileService
}
```

3. **Register in Factory:**
```go
func (f *ServiceFactory) CreateAndRegisterServices() error {
    // ... existing services
    
    profileService, err := NewProfileService(f.cfg, f.logger, f.db)
    if err != nil {
        return err
    }
    f.broker.RegisterService(profileService)
    
    return nil
}
```

4. **Use in gRPC Server:**
```go
if profileService, exists := app.Broker.GetService(broker.ProfileService); exists {
    pb.RegisterProfileServiceServer(server, profileService.GetGRPCService())
}
```

## Conclusion

The Broker Pattern provides a robust foundation for service management in this monolithic application while maintaining the flexibility to evolve towards a microservices architecture. It promotes clean separation of concerns, centralized management, and observability across all services.