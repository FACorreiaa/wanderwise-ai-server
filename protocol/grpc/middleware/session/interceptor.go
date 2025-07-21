package session

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	grpcCodes "google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/FACorreiaa/go-poi-au-suggestions/internal/domain"
)

// SessionType represents different user session types
type SessionType string

const (
	SessionTypeAnonymous SessionType = "anonymous" // Non-logged users - limited access
	SessionTypeUser      SessionType = "user"      // Logged users - standard access with rate limits
	SessionTypePremium   SessionType = "premium"   // Paying users - higher rate limits
)

// SessionContext contains user session information
type SessionContext struct {
	UserID      string      `json:"user_id,omitempty"`
	Role        string      `json:"role,omitempty"`
	SessionType SessionType `json:"session_type"`
	IsActive    bool        `json:"is_active"`
	Permissions []string    `json:"permissions,omitempty"`
	RateLimit   int         `json:"rate_limit"` // Requests per minute
	PageLimit   int         `json:"page_limit"` // Page views per session
	ExpiresAt   time.Time   `json:"expires_at,omitempty"`
}

// InterceptorSession creates a gRPC unary server interceptor for session management
func InterceptorSession(logger *slog.Logger) grpc.UnaryServerInterceptor {
	tracer := otel.Tracer("AuthInterceptor")

	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		ctx, span := tracer.Start(ctx, "AuthInterceptor.InterceptorSession", trace.WithAttributes(
			attribute.String("grpc.method", info.FullMethod),
		))
		defer span.End()

		logger := logger.With(
			slog.String("method", info.FullMethod),
			slog.String("interceptor", "session"),
		)

		// Define unauthenticated methods that don't require tokens
		unauthenticatedMethods := map[string]bool{
			"/fitSphere.auth.Auth/Register":         true,
			"/fitSphere.auth.Auth/Login":            true,
			"/fitSphere.auth.Auth/GetAllUsers":      true, // Public endpoint
			"/calculator.Calculator/GetUsersMacros": true, // Public with limits
			"/CalculatorService/GetUserMacros":      true,
			"/CalculatorService/GetUserMacrosAll":   true,
			"/health/check":                         true, // Health check
		}

		// Check if method requires authentication
		if unauthenticatedMethods[info.FullMethod] {
			// For public endpoints, still create anonymous session context
			sessionCtx := &SessionContext{
				SessionType: SessionTypeAnonymous,
				IsActive:    true,
				RateLimit:   10, // 10 requests per minute for anonymous
				PageLimit:   5,  // 5 page views per session
				ExpiresAt:   time.Now().Add(30 * time.Minute),
			}

			ctx = context.WithValue(ctx, "session", sessionCtx)
			span.SetAttributes(
				attribute.String("session.type", string(SessionTypeAnonymous)),
				attribute.Int("session.rate_limit", sessionCtx.RateLimit),
			)
			span.SetStatus(codes.Ok, "Anonymous session created")
			logger.InfoContext(ctx, "Anonymous session created for public endpoint")

			return handler(ctx, req)
		}

		// Extract metadata from gRPC context
		md, ok := metadata.FromIncomingContext(ctx)
		if !ok {
			span.RecordError(fmt.Errorf("missing metadata"))
			span.SetStatus(codes.Error, "Missing metadata")
			logger.WarnContext(ctx, "Request missing metadata")
			return nil, status.Error(grpcCodes.Unauthenticated, "missing context metadata")
		}

		// Try to extract authorization header
		authHeader := md["authorization"]
		if len(authHeader) == 0 {
			// No token provided - treat as anonymous with limited access
			sessionCtx := &SessionContext{
				SessionType: SessionTypeAnonymous,
				IsActive:    true,
				RateLimit:   5, // Very limited for no auth
				PageLimit:   3, // Very limited page views
				ExpiresAt:   time.Now().Add(15 * time.Minute),
			}

			ctx = context.WithValue(ctx, "session", sessionCtx)
			span.SetAttributes(
				attribute.String("session.type", string(SessionTypeAnonymous)),
				attribute.Int("session.rate_limit", sessionCtx.RateLimit),
			)
			span.SetStatus(codes.Ok, "Limited anonymous session created")
			logger.InfoContext(ctx, "Limited anonymous session created - no auth token")

			return handler(ctx, req)
		}

		// Extract and validate token
		tokenString := strings.TrimSpace(authHeader[0])
		if strings.HasPrefix(tokenString, "Bearer ") {
			tokenString = tokenString[7:] // Remove "Bearer " prefix
		}

		// Parse and validate JWT token
		claims := &domain.Claims{}
		token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
			// Validate signing method
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
			}
			return domain.JwtSecretKey, nil
		})

		if err != nil || !token.Valid {
			span.RecordError(err)
			span.SetStatus(codes.Error, "Invalid token")
			logger.WarnContext(ctx, "Invalid or expired token", slog.Any("error", err))

			// Token invalid - treat as anonymous with very limited access
			sessionCtx := &SessionContext{
				SessionType: SessionTypeAnonymous,
				IsActive:    false, // Mark as inactive due to invalid token
				RateLimit:   2,     // Very restrictive
				PageLimit:   1,     // Almost no page views
				ExpiresAt:   time.Now().Add(5 * time.Minute),
			}

			ctx = context.WithValue(ctx, "session", sessionCtx)
			span.SetAttributes(
				attribute.String("session.type", string(SessionTypeAnonymous)),
				attribute.Bool("session.is_active", false),
			)

			// For most methods, return error. For public methods, allow with restrictions
			if !unauthenticatedMethods[info.FullMethod] {
				return nil, status.Error(grpcCodes.Unauthenticated, "invalid or expired token")
			}

			logger.InfoContext(ctx, "Invalid token - proceeding with restricted anonymous session")
			return handler(ctx, req)
		}

		// Token is valid - determine session type based on user role and claims
		sessionType := determineSessionType(claims.Role, claims.Scope)
		sessionCtx := createSessionContext(claims, sessionType)

		// Add session context to request context
		ctx = context.WithValue(ctx, "session", sessionCtx)
		ctx = context.WithValue(ctx, "userID", claims.UserID)
		ctx = context.WithValue(ctx, "role", claims.Role)

		span.SetAttributes(
			attribute.String("session.type", string(sessionCtx.SessionType)),
			attribute.String("session.user_id", sessionCtx.UserID),
			attribute.String("session.role", sessionCtx.Role),
			attribute.Int("session.rate_limit", sessionCtx.RateLimit),
			attribute.Bool("session.is_active", sessionCtx.IsActive),
		)
		span.SetStatus(codes.Ok, "Authenticated session created")

		logger.InfoContext(ctx, "Authenticated session created",
			slog.String("user_id", claims.UserID),
			slog.String("role", claims.Role),
			slog.String("session_type", string(sessionType)),
			slog.Int("rate_limit", sessionCtx.RateLimit))

		return handler(ctx, req)
	}
}

// determineSessionType determines the session type based on user role and token scope
func determineSessionType(role, scope string) SessionType {
	// Check if user has premium/paid subscription
	if strings.Contains(strings.ToLower(role), "premium") ||
		strings.Contains(strings.ToLower(role), "paid") ||
		strings.Contains(strings.ToLower(scope), "premium") {
		return SessionTypePremium
	}

	// Check if user has admin privileges (treat as premium)
	if strings.Contains(strings.ToLower(role), "admin") {
		return SessionTypePremium
	}

	// Default to regular user session
	return SessionTypeUser
}

// createSessionContext creates a session context based on claims and session type
func createSessionContext(claims *domain.Claims, sessionType SessionType) *SessionContext {
	sessionCtx := &SessionContext{
		UserID:      claims.UserID,
		Role:        claims.Role,
		SessionType: sessionType,
		IsActive:    true,
		ExpiresAt:   claims.ExpiresAt.Time,
	}

	// Set permissions and limits based on session type
	switch sessionType {
	case SessionTypePremium:
		sessionCtx.RateLimit = 1000  // High rate limit
		sessionCtx.PageLimit = 10000 // Very high page views
		sessionCtx.Permissions = []string{
			"read", "write", "delete", "admin", "premium_features",
		}

	case SessionTypeUser:
		sessionCtx.RateLimit = 100  // Standard rate limit
		sessionCtx.PageLimit = 1000 // Standard page views
		sessionCtx.Permissions = []string{
			"read", "write",
		}

	case SessionTypeAnonymous:
		sessionCtx.RateLimit = 10 // Low rate limit
		sessionCtx.PageLimit = 50 // Limited page views
		sessionCtx.Permissions = []string{
			"read",
		}
	}

	return sessionCtx
}

// GetSessionFromContext extracts session context from the gRPC context
func GetSessionFromContext(ctx context.Context) (*SessionContext, bool) {
	session, ok := ctx.Value("session").(*SessionContext)
	return session, ok
}

// HasPermission checks if the current session has the required permission
func HasPermission(ctx context.Context, requiredPermission string) bool {
	session, ok := GetSessionFromContext(ctx)
	if !ok {
		return false
	}

	for _, perm := range session.Permissions {
		if perm == requiredPermission {
			return true
		}
	}
	return false
}

// IsSessionActive checks if the current session is active and not expired
func IsSessionActive(ctx context.Context) bool {
	session, ok := GetSessionFromContext(ctx)
	if !ok {
		return false
	}

	return session.IsActive && time.Now().Before(session.ExpiresAt)
}

// GetRateLimit returns the rate limit for the current session
func GetRateLimit(ctx context.Context) int {
	session, ok := GetSessionFromContext(ctx)
	if !ok {
		return 1 // Very restrictive default
	}
	return session.RateLimit
}

// GetPageLimit returns the page view limit for the current session
func GetPageLimit(ctx context.Context) int {
	session, ok := GetSessionFromContext(ctx)
	if !ok {
		return 1 // Very restrictive default
	}
	return session.PageLimit
}
