package domain

import "github.com/golang-jwt/jwt/v5"

type Claims struct {
	UserID string `json:"userId"`
	Role   string `json:"role"`
	Scope  string `json:"scope"`
	jwt.RegisteredClaims
}

var JwtSecretKey = []byte("your-secret-key")
var JwtRefreshSecretKey = []byte("your-refresh-key")
