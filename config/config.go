package config

import (
	"bytes"
	_ "embed"
	"fmt"
	"time"

	"github.com/spf13/viper"
)

//go:embed config.yml
var embeddedConfig []byte

type JWTConfig struct {
	// SecretKey is the secret used for signing HS256 tokens. REQUIRED.
	// Load this from environment variables or a secure secrets manager.
	SecretKey string `mapstructure:"secret"` // e.g., JWT_SECRET_KEY env var

	// Issuer identifies the principal that issued the JWT. Recommended.
	Issuer string `mapstructure:"issuer"` // e.g., "Loci" or your domain

	// Audience identifies the recipients that the JWT is intended for. Recommended.
	// Often the frontend URL or API identifier.
	Audience string `mapstructure:"audience"` // e.g., "Loci-app"

	// AccessTokenTTL defines the duration for which access tokens are valid. REQUIRED.
	AccessTokenTTL time.Duration `mapstructure:"accessTokenTTL"` // e.g., "15m", "1h"

	// RefreshTokenTTL defines the duration for which refresh tokens are valid. REQUIRED.
	RefreshTokenTTL time.Duration `mapstructure:"refreshTokenTTL"` // e.g., "7d", "30d"
}

type RateLimitConfig struct {
	// Enable/disable rate limiting
	Enabled bool `mapstructure:"enabled"`
	
	// Anonymous users (not logged in)
	AnonymousRequests struct {
		PerMinute int `mapstructure:"perMinute"`
		PerHour   int `mapstructure:"perHour"`
		PerDay    int `mapstructure:"perDay"`
	} `mapstructure:"anonymous"`
	
	// Logged in users (free tier)
	LoggedInUsers struct {
		PerMinute int `mapstructure:"perMinute"`
		PerHour   int `mapstructure:"perHour"`
		PerDay    int `mapstructure:"perDay"`
	} `mapstructure:"loggedIn"`
	
	// Paying users (premium tier)
	PayingUsers struct {
		PerMinute int `mapstructure:"perMinute"`
		PerHour   int `mapstructure:"perHour"`
		PerDay    int `mapstructure:"perDay"`
	} `mapstructure:"paying"`
}

type Config struct {
	Mode         string          `mapstructure:"mode"`
	Dotenv       string          `mapstructure:"dotenv"`
	JWT          JWTConfig       `mapstructure:"jwt"`
	RateLimit    RateLimitConfig `mapstructure:"rateLimit"`
	HandlerImpls struct {
		ExternalAPI struct {
			Port      string `mapstrucutre:"port"`
			CertFile  string `mapstructure:"certFile"`
			KeyFile   string `mapstructure:"keyFile"`
			EnableTLS bool   `mapstracture:"enableTLS"`
		} `mapstructure:"externalAPI"`
		Pprof struct {
			Port      string `mapstructure:"port"`
			CertFile  string `mapstructure:"certFile"`
			KeyFile   string `mapstructure:"keyFile"`
			EnableTLS bool   `mapstructure:"enableTLS"`
		}
		Prometheus struct {
			Port      string `mapstructure:"port"`
			CertFile  string `mapstructure:"certFile"`
			KeyFile   string `mapstructure:"keyFile"`
			EnableTLS bool   `mapstructure:"enableTLS"`
		}
	} `mapstructure:"HandlerImpls"`
	Repositories struct {
		Postgres struct {
			Host              string `mapstructure:"host"`
			Password          string `mapstructure:"password"`
			Port              string `mapstructure:"port"`
			Username          string `mapstructure:"username"`
			DB                string `mapstructure:"db"`
			SSLMODE           string `mapstructure:"SSLMODE"`
			MAXCONWAITINGTIME int    `mapstructure:"MAXCONWAITINGTIME"`
		} `mapstructure:"postgres"`
		Redis struct {
			Host     string `mapstructure:"host"`
			Port     string `mapstructure:"port"`
			Password string `mapstructure:"password"`
			DB       int    `mapstructure:"db"`
		} `mapstructure:"redis"`
	}
	Server struct {
		HTTPPort string        `mapstructure:"HTTPPort"`
		GrpcPort string        `mapstructure:"GrpcPort"`
		Timeout  time.Duration `mapstructure:"HTTPTimeout"`
	} `mapstructure:"server"`
	
	UpstreamServices struct {
		Auth       string `mapstructure:"auth"`
		POI        string `mapstructure:"poi"`
		Chat       string `mapstructure:"chat"`
		Lists      string `mapstructure:"lists"`
		Users      string `mapstructure:"users"`
		Admin      string `mapstructure:"admin"`
		City       string `mapstructure:"city"`
		Interests  string `mapstructure:"interests"`
		Profiles   string `mapstructure:"profiles"`
		Recents    string `mapstructure:"recents"`
		Reviews    string `mapstructure:"reviews"`
		Statistics string `mapstructure:"statistics"`
		Tags       string `mapstructure:"tags"`
	} `mapstructure:"upstreamServices"`
}

func InitConfig() (*Config, error) {
	var config Config
	v := viper.New()

	// Add file-based config paths
	v.AddConfigPath(".")
	v.AddConfigPath("config")
	v.AddConfigPath("/app/config")
	v.AddConfigPath("/usr/local/bin")
	v.AddConfigPath("/usr/local/bin/inkme")
	v.AutomaticEnv()

	v.SetConfigName("config")
	v.SetConfigType("yml")

	// Try to load file-based config
	err := v.ReadInConfig()
	if err != nil {
		fmt.Printf("Warning: Failed to find file-based config: %s. Falling back to embedded config.\n", err)
		if err = v.ReadConfig(bytes.NewReader(embeddedConfig)); err != nil {
			return nil, fmt.Errorf("failed to read embedded config: %s", err)
		}
	}

	// Unmarshal the config into the Config struct
	if err = v.Unmarshal(&config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %s", err)
	}

	if config.JWT.SecretKey == "" {
		return nil, fmt.Errorf("JWT_SECRET_KEY environment variable is required")
	}
	if config.JWT.AccessTokenTTL == 0 {
		return nil, fmt.Errorf("JWT_ACCESS_TOKEN_TTL environment variable is required and must be a valid duration (e.g., 15m)")
	}
	if config.JWT.RefreshTokenTTL == 0 {
		return nil, fmt.Errorf("JWT_REFRESH_TOKEN_TTL environment variable is required and must be a valid duration (e.g., 168h)")
	}

	// Handle rate limiting environment variable override
	if rateLimitEnabled := v.GetString("RATE_LIMIT_ENABLED"); rateLimitEnabled != "" {
		switch rateLimitEnabled {
		case "true", "1", "yes", "on":
			config.RateLimit.Enabled = true
		case "false", "0", "no", "off":
			config.RateLimit.Enabled = false
		}
	}

	fmt.Println("Successfully loaded app configs...")
	return &config, nil
}
