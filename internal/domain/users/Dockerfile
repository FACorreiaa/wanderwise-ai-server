# Multi-stage Dockerfile for Users Service
# Supports production builds, development with Air hot reload, and debugging with Delve

# Base stage with Go and dependencies
FROM golang:1.24.5-alpine AS base

LABEL maintainer="a11199"
LABEL description="Users Service - User management and profiles microservice"

RUN apk add --no-cache git ca-certificates tzdata bash
WORKDIR /app

# Copy go workspace files from root
COPY ../../../go.work ../../../go.work.sum ./
COPY ../../../go.mod ../../../go.sum ./

# Copy domain specific go.mod
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download && go mod verify

# Development stage with Air for hot reload
FROM base AS dev
RUN go install github.com/air-verse/air@latest
COPY . .
EXPOSE 8005 2345 6060
CMD ["air", "-c", ".air.toml"]

# Debug stage with Delve debugger
FROM base AS debug
RUN go install github.com/go-delve/delve/cmd/dlv@latest
COPY . .
EXPOSE 8005 2345 6060
CMD ["dlv", "debug", "--headless", "--listen=:2345", "--api-version=2", "--accept-multiclient", "--log", "./cmd/main.go"]

# Build stage for production
FROM base AS builder
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o users-service -ldflags="-w -s -X main.version=$(date +%Y%m%d-%H%M%S)" ./cmd/main.go

# Production stage
FROM alpine:3.20 AS prod
RUN apk --no-cache add ca-certificates tzdata
WORKDIR /app

# Copy binary
COPY --from=builder /app/users-service /usr/bin/users-service

# Create non-root user for security
RUN addgroup -g 1001 -S appgroup && \
    adduser -u 1001 -S appuser -G appgroup
USER appuser

EXPOSE 8005
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD wget --no-verbose --tries=1 --spider http://localhost:8005/health || exit 1

CMD ["users-service"]