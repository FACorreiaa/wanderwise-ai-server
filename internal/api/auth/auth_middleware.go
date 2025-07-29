package auth

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time" // For validating exp claim

	"github.com/golang-jwt/jwt/v5"

	"github.com/FACorreiaa/go-poi-au-suggestions/config"
	"github.com/FACorreiaa/go-poi-au-suggestions/internal/api"
)

// Define typed context keys
type contextKey string

const UserIDKey contextKey = "userID"
const UserRoleKey contextKey = "userRole"
const UserPlanKey contextKey = "userPlan"
const UserSubStatusKey contextKey = "userSubStatus"

// Authenticate is middleware to validate JWT access tokens.
// It expects the JWT secret key to be passed in.
func Authenticate(logger *slog.Logger, jwtCfg config.JWTConfig) func(next http.Handler) http.Handler {
	secretKey := []byte(jwtCfg.SecretKey)
	if len(secretKey) == 0 {
		logger.Error("FATAL: JWT Secret Key is not configured!")
		panic("JWT Secret Key cannot be empty")
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			l := logger.With(slog.String("middleware", "Authenticate"))

			// Check if the request is for a public endpoint
			// Define public routes that do not require authentication
			// maybe change later
			publicRoutes := map[string]struct{}{
				"/api/v1/auth/register": {},
				"/api/v1/auth/login":    {},
				"/api/v1/auth/refresh":  {},
			}
			if _, isPublic := publicRoutes[r.URL.Path]; isPublic {
				l.DebugContext(ctx, "Skipping authentication for public route", slog.String("path", r.URL.Path))
				next.ServeHTTP(w, r)
				return
			}
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				l.WarnContext(ctx, "Missing Authorization header")
				api.ErrorResponse(w, r, http.StatusUnauthorized, "Authorization header required")
				return
			}

			headerParts := strings.Split(authHeader, " ")
			if len(headerParts) != 2 || strings.ToLower(headerParts[0]) != "bearer" {
				l.WarnContext(ctx, "Invalid Authorization header format")
				api.ErrorResponse(w, r, http.StatusUnauthorized, "Authorization header format must be Bearer {token}")
				return
			}
			tokenString := headerParts[1]

			claims := &Claims{}
			token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
				if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
					return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
				}
				return secretKey, nil
			})

			if err != nil {
				l.WarnContext(ctx, "Token parsing/validation failed", slog.Any("error", err))
				errMsg := "Invalid or expired token"
				if errors.Is(err, jwt.ErrTokenExpired) {
					errMsg = "Token has expired"
				} else if errors.Is(err, jwt.ErrTokenMalformed) {
					errMsg = "Malformed token"
				} else if errors.Is(err, jwt.ErrSignatureInvalid) {
					errMsg = "Invalid token signature"
				}
				api.ErrorResponse(w, r, http.StatusUnauthorized, errMsg)
				return
			}

			if !token.Valid {
				l.WarnContext(ctx, "Token marked as invalid or claims are nil")
				api.ErrorResponse(w, r, http.StatusUnauthorized, "Invalid token")
				return
			}

			now := time.Now()
			if claims.ExpiresAt == nil || now.Unix() > claims.ExpiresAt.Unix() {
				l.WarnContext(ctx, "Token expiration claim check failed", slog.Time("now", now), slog.Time("exp", claims.ExpiresAt.Time))
				api.ErrorResponse(w, r, http.StatusUnauthorized, "Token has expired")
				return
			}
			if claims.Issuer != jwtCfg.Issuer {
				l.WarnContext(ctx, "Token issuer mismatch", slog.String("expected", jwtCfg.Issuer), slog.String("actual", claims.Issuer))
				api.ErrorResponse(w, r, http.StatusUnauthorized, "Invalid token issuer")
				return
			}

			if jwtCfg.Audience != "" && !api.VerifyAudience(claims.Audience, jwtCfg.Audience) {
				l.WarnContext(ctx, "Token audience mismatch", slog.String("expected", jwtCfg.Audience), slog.Any("actual", claims.Audience))
				api.ErrorResponse(w, r, http.StatusUnauthorized, "Invalid token audience")
				return
			}

			ctx = context.WithValue(ctx, UserIDKey, claims.UserID)
			l.DebugContext(ctx, "Authentication successful, claims added to context", slog.String("userID", claims.UserID))
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// Helper functions to get claims from context
func GetUserIDFromContext(ctx context.Context) (string, bool) {
	userID, ok := ctx.Value(UserIDKey).(string)
	return userID, ok
}

func GetUserRoleFromContext(ctx context.Context) (string, bool) {
	role, ok := ctx.Value(UserRoleKey).(string)
	return role, ok
}

func GetUserPlanFromContext(ctx context.Context) (string, bool) {
	plan, ok := ctx.Value(UserPlanKey).(string)
	return plan, ok
}

func GetUserSubStatusFromContext(ctx context.Context) (string, bool) {
	status, ok := ctx.Value(UserSubStatusKey).(string)
	return status, ok
}

// ValidateJWTToken validates a JWT token string and returns the claims
func ValidateJWTToken(tokenString string, jwtCfg config.JWTConfig) (*Claims, error) {
	secretKey := []byte(jwtCfg.SecretKey)
	
	claims := &Claims{}
	token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return secretKey, nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to parse token: %w", err)
	}

	if !token.Valid {
		return nil, errors.New("invalid token")
	}

	// Validate expiration
	if claims.ExpiresAt != nil && time.Now().After(claims.ExpiresAt.Time) {
		return nil, errors.New("token has expired")
	}

	// Validate issuer if configured
	if jwtCfg.Issuer != "" && claims.Issuer != jwtCfg.Issuer {
		return nil, errors.New("invalid token issuer")
	}

	// Validate audience if configured
	if jwtCfg.Audience != "" {
		validAudience := false
		for _, aud := range claims.Audience {
			if aud == jwtCfg.Audience {
				validAudience = true
				break
			}
		}
		if !validAudience {
			return nil, errors.New("invalid token audience")
		}
	}

	return claims, nil
}

// RequirePlanStatus checks if the user in the context has the required plan(s) and status.
// Runs AFTER the Authenticate middleware.
func RequirePlanStatus(logger *slog.Logger, allowedPlans []string, requiredStatus string) func(next http.Handler) http.Handler {
	// Convert allowedPlans to a map for faster lookup
	planMap := make(map[string]struct{}, len(allowedPlans))
	for _, p := range allowedPlans {
		planMap[p] = struct{}{}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			// Retrieve claims added by Authenticate middleware
			// Use your actual helper functions/context keys
			plan, planOk := GetUserPlanFromContext(ctx)          // e.g., "free", "premium_monthly"
			status, statusOk := GetUserSubStatusFromContext(ctx) // e.g., "active", "trialing"

			if !planOk || !statusOk {
				logger.ErrorContext(ctx, "Subscription claims missing from context", slog.String("plan_ok", fmt.Sprintf("%t", planOk)), slog.String("status_ok", fmt.Sprintf("%t", statusOk)))
				api.ErrorResponse(w, r, http.StatusInternalServerError, "Cannot determine subscription status") // Or forbidden
				return
			}

			// Check status first
			if status != requiredStatus {
				logger.WarnContext(ctx, "Subscription status check failed", slog.String("required_status", requiredStatus), slog.String("actual_status", status))
				api.ErrorResponse(w, r, http.StatusForbidden, fmt.Sprintf("Subscription status must be '%s'", requiredStatus))
				return
			}

			// Check if the user's plan is in the allowed list
			if _, allowed := planMap[plan]; !allowed {
				logger.WarnContext(ctx, "Subscription plan check failed", slog.Any("allowed_plans", allowedPlans), slog.String("actual_plan", plan))
				api.ErrorResponse(w, r, http.StatusForbidden, "Access denied for your current subscription plan.")
				return
			}

			// User has the required plan and status
			next.ServeHTTP(w, r)
		})
	}
}

// Add UserIDKey, UserRoleKey etc. used by Authenticate middleware
// Assume Authenticate middleware adds these values like:
// ctx = context.WithValue(ctx, UserPlanKey, claims.Plan)
// ctx = context.WithValue(ctx, UserSubStatusKey, claims.Status)
