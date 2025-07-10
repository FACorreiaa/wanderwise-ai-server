package appmiddleware

import "github.com/golang-jwt/jwt/v5"

type contextKey string

const UserIDKey contextKey = "userID"
const UserRoleKey contextKey = "userRole"

type Claims struct {
	UserID   string `json:"user_id"`
	Username string `json:"username"`
	Email    string `json:"email"`
	Role     string `json:"role"`
	Tenant   string `json:"tenant"`
	Scope    string `json:"scope"`
	StudioID string `json:"studio_id,omitempty"`
	jwt.RegisteredClaims
}

var JwtSecretKey = []byte("your-secret-key")
var JwtRefreshSecretKey = []byte("your-refresh-key")
