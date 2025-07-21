package domain

import (
	"context"

	pb "github.com/FACorreiaa/loci-proto/modules/auth/generated"
	"github.com/golang-jwt/jwt/v5"
	"google.golang.org/grpc"
)

type Claims struct {
	UserID string `json:"userId"`
	Role   string `json:"role"`
	Scope  string `json:"scope"`
	jwt.RegisteredClaims
}

var JwtSecretKey = []byte("your-secret-key")
var JwtRefreshSecretKey = []byte("your-refresh-key")

type AuthService interface {
	Register(ctx context.Context, req *pb.RegisterRequest) (*pb.RegisterResponse, error)
	Login(ctx context.Context, req *pb.LoginRequest) (*pb.LoginResponse, error)
	RefreshToken(ctx context.Context, req *pb.RefreshTokenRequest) (*pb.TokenResponse, error)
	Logout(ctx context.Context, in *pb.LogoutRequest) (*pb.LogoutResponse, error)
	ValidateSession(ctx context.Context, in *pb.ValidateSessionRequest) (*pb.ValidateSessionResponse, error)
	UpdatePassword(ctx context.Context, in *pb.UpdatePasswordRequest, opts ...grpc.CallOption) (*pb.UpdatePasswordResponse, error)
	GoogleLogin(ctx context.Context, in *pb.GoogleLoginRequest, opts ...grpc.CallOption) (*pb.GoogleLoginResponse, error)
	GoogleCallback(ctx context.Context, in *pb.GoogleCallbackRequest, opts ...grpc.CallOption) (*pb.LoginResponse, error)
}
