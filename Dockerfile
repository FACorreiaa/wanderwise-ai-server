# Build stage
FROM golang:1.25-alpine AS builder

# Install air for hot reloading
RUN go install github.com/air-verse/air@latest

WORKDIR /app

# Copy go mod and sum files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o main .

# Final stage
FROM alpine:latest

# Install ca-certificates for HTTPS requests
RUN apk --no-cache add ca-certificates

WORKDIR /root/

# Copy the binary from builder
COPY --from=builder /app/main .

# Copy any config files or static assets if needed
COPY --from=builder /app/.env* ./

# Expose port (adjust as needed based on your app configuration)
EXPOSE 8080

# Run the binary
CMD ["./main"]