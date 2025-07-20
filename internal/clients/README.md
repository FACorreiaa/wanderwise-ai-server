# Microservices Client Library

This package provides HTTP client functionality for inter-service communication in the Go AI POI microservices architecture.

## Overview

The client library includes:

1. **HTTPClient** - HTTP client for making requests to other microservices
2. **ServiceRegistry** - Service discovery and health monitoring
3. **Example Usage** - Demonstrates how to use the clients

## Features

### HTTP Client (`http_client.go`)
- Type-safe methods for calling each microservice
- Automatic JSON serialization/deserialization
- Request/response logging
- Context propagation (correlation IDs, request IDs)
- Error handling and retries
- Timeout management

### Service Registry (`service_registry.go`)
- Service discovery from configuration
- Health checking with configurable intervals
- Service status monitoring
- Statistics and health metrics
- Thread-safe operations

## Configuration

Services are configured in `config.yml` under `upstreamServices`:

```yaml
upstreamServices:
  auth: "http://auth-service:8001"
  poi: "http://poi-service:8002"
  chat: "http://chat-service:8003"
  lists: "http://lists-service:8004"
  users: "http://users-service:8005"
  admin: "http://admin-service:8006"
  city: "http://city-service:8007"
  interests: "http://interests-service:8008"
  profiles: "http://profiles-service:8009"
  recents: "http://recents-service:8010"
  reviews: "http://reviews-service:8011"
  statistics: "http://statistics-service:8012"
  tags: "http://tags-service:8013"
```

## Usage Examples

### Basic Service Call

```go
// Initialize client
httpClient := clients.NewHTTPClient(cfg, logger)

// Call a service
resp, err := httpClient.CallUsersService(ctx, clients.ServiceRequest{
    Method: "GET",
    Path:   "/users/123",
    Headers: map[string]string{
        "Accept": "application/json",
    },
})
```

### Service Registry

```go
// Initialize service registry
registry := clients.NewServiceRegistry(cfg, logger)
registry.InitializeServices()

// Start health checking
go registry.StartHealthChecks(ctx, 30*time.Second)

// Check if a service is available
if registry.IsServiceAvailable("auth") {
    // Service is healthy and available
}

// Get service statistics
stats := registry.GetServiceStats()
fmt.Printf("Healthy services: %d/%d\n", stats["healthy"], stats["total"])
```

### Complex Service Interactions

See `example_usage.go` for examples of:
- Calling multiple services in sequence
- Handling errors and fallbacks
- Coordinated operations across services
- Authentication flows

## Available Services

| Service | Port | Purpose |
|---------|------|---------|
| auth | 8001 | Authentication and authorization |
| poi | 8002 | Points of interest management |
| chat | 8003 | AI chat and recommendations |
| lists | 8004 | User lists and collections |
| users | 8005 | User management |
| admin | 8006 | Administrative functions |
| city | 8007 | City and location data |
| interests | 8008 | User interests and preferences |
| profiles | 8009 | User profiles |
| recents | 8010 | Recent activity tracking |
| reviews | 8011 | Reviews and ratings |
| statistics | 8012 | Analytics and statistics |
| tags | 8013 | Tagging system |

## Monitoring Endpoints

The main service exposes these endpoints for monitoring the service registry:

- `GET /services` - List all registered services and their status
- `GET /services/stats` - Get service health statistics
- `GET /services/healthy` - List only healthy services

## Best Practices

1. **Always use context** for request cancellation and timeouts
2. **Handle errors gracefully** - services may be temporarily unavailable
3. **Add correlation IDs** for request tracing across services
4. **Use circuit breakers** for resilience (can be added to the client)
5. **Monitor service health** regularly
6. **Log service interactions** for debugging and monitoring

## Error Handling

The client automatically handles:
- Network errors and timeouts
- JSON marshaling/unmarshaling errors
- HTTP status code validation
- Service unavailability

Custom error handling can be implemented by checking the response status code and body.

## Future Enhancements

- Circuit breaker pattern implementation
- Retry logic with exponential backoff
- Load balancing across multiple service instances
- gRPC client support
- Request/response caching
- Metrics and tracing integration