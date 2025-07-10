package appmiddleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"
)

// Authenticate extracts JWT from Authorization header, validates it,
// and adds userID and role to the request context.
func Authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 1. Get the Authorization header
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			http.Error(w, "Authorization header required", http.StatusUnauthorized)
			return
		}

		// 2. Validate format "Bearer <token>"
		headerParts := strings.Split(authHeader, " ")
		if len(headerParts) != 2 || strings.ToLower(headerParts[0]) != "bearer" {
			http.Error(w, "Authorization header format must be Bearer {token}", http.StatusUnauthorized)
			return
		}
		tokenString := headerParts[1]

		// 3. Parse and validate the token
		claims := &Claims{}
		token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
			// Ensure the signing method is what you expect (optional but recommended)
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, jwt.ErrSignatureInvalid // Or a more specific error
			}
			return JwtSecretKey, nil // Use your secret key
		})

		// 4. Handle parsing errors or invalid token
		if err != nil {
			if err == jwt.ErrSignatureInvalid {
				http.Error(w, "Invalid token signature", http.StatusUnauthorized)
				return
			}
			// Log the detailed error for debugging?
			// logger.ErrorContext(r.Context(), "Token validation error", slog.Any("error", err))
			http.Error(w, "Invalid or expired token", http.StatusUnauthorized) // More generic error to client
			return
		}

		if !token.Valid {
			http.Error(w, "Invalid token", http.StatusUnauthorized)
			return
		}

		// 5. Token is valid, add claims to context
		// Use specific context keys to avoid collisions
		ctx := context.WithValue(r.Context(), UserIDKey, claims.UserID)
		ctx = context.WithValue(ctx, UserRoleKey, claims.Role)

		// 6. Create a new request with the updated context and call the next HandlerImpl
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// Helper functions to retrieve values (optional but good practice)
func GetUserIDFromContext(ctx context.Context) (string, bool) { // Adjust type if UserID isn't string
	userID, ok := ctx.Value(UserIDKey).(string) // Adjust type assertion
	return userID, ok
}

func GetUserRoleFromContext(ctx context.Context) (string, bool) { // Adjust type if Role isn't string
	role, ok := ctx.Value(UserRoleKey).(string) // Adjust type assertion
	return role, ok
}
