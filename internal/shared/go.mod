module github.com/loci/shared

go 1.24.5

// Common dependencies used across all microservices
require (
    github.com/gin-gonic/gin v1.9.1
    github.com/swaggo/swag v1.16.2
    github.com/golang/mock v1.6.0
    github.com/stretchr/testify v1.8.4
    github.com/prometheus/client_golang v1.17.0
    github.com/opentracing/opentracing-go v1.2.0
    github.com/uber/jaeger-client-go v2.30.0+incompatible
    go.uber.org/zap v1.26.0
    github.com/spf13/viper v1.17.0
    gorm.io/gorm v1.25.5
    gorm.io/driver/postgres v1.5.4
    github.com/golang-jwt/jwt/v5 v5.1.0
    github.com/redis/go-redis/v9 v9.3.0
    golang.org/x/crypto v0.15.0
)