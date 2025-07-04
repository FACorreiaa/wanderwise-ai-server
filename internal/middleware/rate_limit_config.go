package middleware

import (
	"encoding/json"
	"fmt"
	"os"
)

// RateLimitSettings contains all rate limiting configurations
type RateLimitSettings struct {
	DefaultLimits map[RateLimitTier]RateLimitConfig `json:"default_limits"`
	LLMLimits     map[RateLimitTier]RateLimitConfig `json:"llm_limits"`
	Enabled       bool                             `json:"enabled"`
}

// LoadRateLimitConfig loads rate limit configuration from environment variables or config file
func LoadRateLimitConfig(configPath string) (*RateLimitSettings, error) {
	// Default configuration
	settings := &RateLimitSettings{
		DefaultLimits: DefaultRateLimits,
		LLMLimits:     LLMRateLimits,
		Enabled:       true,
	}

	// Try to load from config file if provided
	if configPath != "" {
		if err := loadFromFile(configPath, settings); err != nil {
			return nil, fmt.Errorf("failed to load rate limit config from file: %w", err)
		}
	}

	// Override with environment variables if present
	loadFromEnv(settings)

	return settings, nil
}

// loadFromFile loads configuration from a JSON file
func loadFromFile(path string, settings *RateLimitSettings) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	return json.Unmarshal(data, settings)
}

// loadFromEnv loads configuration from environment variables
func loadFromEnv(settings *RateLimitSettings) {
	// Check if rate limiting should be disabled
	if os.Getenv("RATE_LIMIT_DISABLED") == "true" {
		settings.Enabled = false
		return
	}

	// Override free tier limits
	if rpm := getEnvInt("RATE_LIMIT_FREE_RPM", 0); rpm > 0 {
		if settings.DefaultLimits[TierFree].RequestsPerMinute != rpm {
			config := settings.DefaultLimits[TierFree]
			config.RequestsPerMinute = rpm
			settings.DefaultLimits[TierFree] = config
		}
	}

	if rph := getEnvInt("RATE_LIMIT_FREE_RPH", 0); rph > 0 {
		config := settings.DefaultLimits[TierFree]
		config.RequestsPerHour = rph
		settings.DefaultLimits[TierFree] = config
	}

	if rpd := getEnvInt("RATE_LIMIT_FREE_RPD", 0); rpd > 0 {
		config := settings.DefaultLimits[TierFree]
		config.RequestsPerDay = rpd
		settings.DefaultLimits[TierFree] = config
	}

	// Override premium tier limits
	if rpm := getEnvInt("RATE_LIMIT_PREMIUM_RPM", 0); rpm > 0 {
		config := settings.DefaultLimits[TierPremium]
		config.RequestsPerMinute = rpm
		settings.DefaultLimits[TierPremium] = config
	}

	if rph := getEnvInt("RATE_LIMIT_PREMIUM_RPH", 0); rph > 0 {
		config := settings.DefaultLimits[TierPremium]
		config.RequestsPerHour = rph
		settings.DefaultLimits[TierPremium] = config
	}

	if rpd := getEnvInt("RATE_LIMIT_PREMIUM_RPD", 0); rpd > 0 {
		config := settings.DefaultLimits[TierPremium]
		config.RequestsPerDay = rpd
		settings.DefaultLimits[TierPremium] = config
	}

	// Override LLM limits for free tier
	if rpm := getEnvInt("RATE_LIMIT_LLM_FREE_RPM", 0); rpm > 0 {
		config := settings.LLMLimits[TierFree]
		config.RequestsPerMinute = rpm
		settings.LLMLimits[TierFree] = config
	}

	if rph := getEnvInt("RATE_LIMIT_LLM_FREE_RPH", 0); rph > 0 {
		config := settings.LLMLimits[TierFree]
		config.RequestsPerHour = rph
		settings.LLMLimits[TierFree] = config
	}

	if rpd := getEnvInt("RATE_LIMIT_LLM_FREE_RPD", 0); rpd > 0 {
		config := settings.LLMLimits[TierFree]
		config.RequestsPerDay = rpd
		settings.LLMLimits[TierFree] = config
	}

	// Override LLM limits for premium tier
	if rpm := getEnvInt("RATE_LIMIT_LLM_PREMIUM_RPM", 0); rpm > 0 {
		config := settings.LLMLimits[TierPremium]
		config.RequestsPerMinute = rpm
		settings.LLMLimits[TierPremium] = config
	}

	if rph := getEnvInt("RATE_LIMIT_LLM_PREMIUM_RPH", 0); rph > 0 {
		config := settings.LLMLimits[TierPremium]
		config.RequestsPerHour = rph
		settings.LLMLimits[TierPremium] = config
	}

	if rpd := getEnvInt("RATE_LIMIT_LLM_PREMIUM_RPD", 0); rpd > 0 {
		config := settings.LLMLimits[TierPremium]
		config.RequestsPerDay = rpd
		settings.LLMLimits[TierPremium] = config
	}
}

// getEnvInt gets an integer value from environment variable
func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := parseIntFromString(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}

// parseIntFromString safely parses an integer from string
func parseIntFromString(s string) (int, error) {
	var result int
	_, err := fmt.Sscanf(s, "%d", &result)
	return result, err
}

// SaveConfigTemplate saves a template configuration file
func SaveConfigTemplate(path string) error {
	settings := &RateLimitSettings{
		DefaultLimits: DefaultRateLimits,
		LLMLimits:     LLMRateLimits,
		Enabled:       true,
	}

	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}

// UpdateRateLimiter updates the rate limiter with new settings
func (rl *RateLimiter) UpdateConfig(settings *RateLimitSettings) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	// Clear existing buckets to apply new limits
	rl.buckets = make(map[string]*TokenBucket)
	
	// Update the global configurations
	DefaultRateLimits = settings.DefaultLimits
	LLMRateLimits = settings.LLMLimits

	rl.logger.Info("Rate limiter configuration updated",
		"default_limits", settings.DefaultLimits,
		"llm_limits", settings.LLMLimits,
		"enabled", settings.Enabled,
	)
}

// GetCurrentConfig returns the current rate limiting configuration
func GetCurrentConfig() *RateLimitSettings {
	return &RateLimitSettings{
		DefaultLimits: DefaultRateLimits,
		LLMLimits:     LLMRateLimits,
		Enabled:       true,
	}
}