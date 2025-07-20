package clients

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/FACorreiaa/go-poi-au-suggestions/config"
)

// ServiceStatus represents the health status of a service
type ServiceStatus struct {
	Name      string
	URL       string
	Healthy   bool
	LastCheck time.Time
	Error     error
}

// ServiceRegistry manages service discovery and health monitoring
type ServiceRegistry struct {
	services map[string]*ServiceStatus
	client   *HTTPClient
	logger   *zap.Logger
	mu       sync.RWMutex
	config   *config.Config
}

// NewServiceRegistry creates a new service registry
func NewServiceRegistry(cfg *config.Config, logger *zap.Logger) *ServiceRegistry {
	return &ServiceRegistry{
		services: make(map[string]*ServiceStatus),
		client:   NewHTTPClient(cfg, logger),
		logger:   logger,
		config:   cfg,
	}
}

// InitializeServices registers all configured services
func (sr *ServiceRegistry) InitializeServices() {
	sr.mu.Lock()
	defer sr.mu.Unlock()

	services := map[string]string{
		"auth":       sr.config.UpstreamServices.Auth,
		"poi":        sr.config.UpstreamServices.POI,
		"chat":       sr.config.UpstreamServices.Chat,
		"lists":      sr.config.UpstreamServices.Lists,
		"users":      sr.config.UpstreamServices.Users,
		"admin":      sr.config.UpstreamServices.Admin,
		"city":       sr.config.UpstreamServices.City,
		"interests":  sr.config.UpstreamServices.Interests,
		"profiles":   sr.config.UpstreamServices.Profiles,
		"recents":    sr.config.UpstreamServices.Recents,
		"reviews":    sr.config.UpstreamServices.Reviews,
		"statistics": sr.config.UpstreamServices.Statistics,
		"tags":       sr.config.UpstreamServices.Tags,
	}

	for name, url := range services {
		if url != "" { // Only register services that are configured
			sr.services[name] = &ServiceStatus{
				Name:      name,
				URL:       url,
				Healthy:   false, // Will be updated by health checks
				LastCheck: time.Time{},
			}
		}
	}

	sr.logger.Info("Initialized service registry",
		zap.Int("service_count", len(sr.services)),
		zap.Strings("services", sr.getServiceNames()))
}

// StartHealthChecks begins periodic health checking of all services
func (sr *ServiceRegistry) StartHealthChecks(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Perform initial health check
	sr.performHealthChecks(ctx)

	for {
		select {
		case <-ctx.Done():
			sr.logger.Info("Stopping service health checks")
			return
		case <-ticker.C:
			sr.performHealthChecks(ctx)
		}
	}
}

// performHealthChecks checks the health of all registered services
func (sr *ServiceRegistry) performHealthChecks(ctx context.Context) {
	sr.mu.Lock()
	defer sr.mu.Unlock()

	for name, service := range sr.services {
		go func(name string, service *ServiceStatus) {
			healthy := sr.client.IsServiceHealthy(ctx, name)
			
			sr.mu.Lock()
			service.Healthy = healthy
			service.LastCheck = time.Now()
			if !healthy {
				service.Error = fmt.Errorf("health check failed")
			} else {
				service.Error = nil
			}
			sr.mu.Unlock()

			sr.logger.Debug("Service health check completed",
				zap.String("service", name),
				zap.String("url", service.URL),
				zap.Bool("healthy", healthy))
		}(name, service)
	}
}

// GetService returns the status of a specific service
func (sr *ServiceRegistry) GetService(name string) (*ServiceStatus, bool) {
	sr.mu.RLock()
	defer sr.mu.RUnlock()
	
	service, exists := sr.services[name]
	if !exists {
		return nil, false
	}
	
	// Return a copy to avoid race conditions
	return &ServiceStatus{
		Name:      service.Name,
		URL:       service.URL,
		Healthy:   service.Healthy,
		LastCheck: service.LastCheck,
		Error:     service.Error,
	}, true
}

// GetAllServices returns the status of all services
func (sr *ServiceRegistry) GetAllServices() map[string]*ServiceStatus {
	sr.mu.RLock()
	defer sr.mu.RUnlock()
	
	result := make(map[string]*ServiceStatus)
	for name, service := range sr.services {
		result[name] = &ServiceStatus{
			Name:      service.Name,
			URL:       service.URL,
			Healthy:   service.Healthy,
			LastCheck: service.LastCheck,
			Error:     service.Error,
		}
	}
	
	return result
}

// GetHealthyServices returns only the healthy services
func (sr *ServiceRegistry) GetHealthyServices() map[string]*ServiceStatus {
	sr.mu.RLock()
	defer sr.mu.RUnlock()
	
	result := make(map[string]*ServiceStatus)
	for name, service := range sr.services {
		if service.Healthy {
			result[name] = &ServiceStatus{
				Name:      service.Name,
				URL:       service.URL,
				Healthy:   service.Healthy,
				LastCheck: service.LastCheck,
				Error:     service.Error,
			}
		}
	}
	
	return result
}

// IsServiceAvailable checks if a service is available and healthy
func (sr *ServiceRegistry) IsServiceAvailable(name string) bool {
	sr.mu.RLock()
	defer sr.mu.RUnlock()
	
	service, exists := sr.services[name]
	if !exists {
		return false
	}
	
	// Consider service stale if last check was more than 2 minutes ago
	if time.Since(service.LastCheck) > 2*time.Minute {
		return false
	}
	
	return service.Healthy
}

// GetServiceURL returns the URL for a service
func (sr *ServiceRegistry) GetServiceURL(name string) (string, bool) {
	sr.mu.RLock()
	defer sr.mu.RUnlock()
	
	service, exists := sr.services[name]
	if !exists {
		return "", false
	}
	
	return service.URL, true
}

// getServiceNames returns a list of all registered service names
func (sr *ServiceRegistry) getServiceNames() []string {
	names := make([]string, 0, len(sr.services))
	for name := range sr.services {
		names = append(names, name)
	}
	return names
}

// GetServiceStats returns statistics about service health
func (sr *ServiceRegistry) GetServiceStats() map[string]interface{} {
	sr.mu.RLock()
	defer sr.mu.RUnlock()
	
	total := len(sr.services)
	healthy := 0
	unhealthy := 0
	stale := 0
	
	for _, service := range sr.services {
		if time.Since(service.LastCheck) > 2*time.Minute {
			stale++
		} else if service.Healthy {
			healthy++
		} else {
			unhealthy++
		}
	}
	
	return map[string]interface{}{
		"total":     total,
		"healthy":   healthy,
		"unhealthy": unhealthy,
		"stale":     stale,
		"health_percentage": func() float64 {
			if total == 0 {
				return 100.0
			}
			return float64(healthy) / float64(total) * 100.0
		}(),
	}
}