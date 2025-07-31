package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
	grpcCodes "google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "github.com/FACorreiaa/loci-proto/modules/auth/generated"

	"github.com/FACorreiaa/go-poi-au-suggestions/app/observability/metrics"
	"github.com/FACorreiaa/go-poi-au-suggestions/internal/types"
)

type Service struct {
	pb.UnimplementedAuthServiceServer
	logger *zap.Logger
	repo   Repository
	pgpool *pgxpool.Pool
	tracer trace.Tracer
}

func NewService(ctx context.Context, repo Repository, pgpool *pgxpool.Pool, logger *zap.Logger) *Service {
	return &Service{
		logger: logger.With(zap.String("service", "auth")),
		repo:   repo,
		pgpool: pgpool,
		tracer: otel.Tracer("AuthService"),
	}
}

func (svc *Service) Register(ctx context.Context, in *pb.RegisterRequest) (*pb.RegisterResponse, error) {
	ctx, span := svc.tracer.Start(ctx, "AuthService.Register", trace.WithAttributes(
		attribute.String("auth.operation", "register"),
		attribute.String("auth.username", in.Username),
		attribute.String("auth.email", in.Email),
	))
	defer span.End()

	// Record start time for metrics
	startTime := time.Now()
	defer func() {
		metrics.Get().RegisterDurationSeconds.Record(ctx, time.Since(startTime).Seconds())
	}()

	svc.logger.Info("User registration attempt",
		zap.String("username", in.Username),
		zap.String("email", in.Email))

	// Validation
	if strings.TrimSpace(in.Email) == "" || strings.TrimSpace(in.Username) == "" || strings.TrimSpace(in.Password) == "" {
		span.RecordError(types.ErrBadRequest)
		span.SetStatus(codes.Error, "Invalid input")
		metrics.Get().RegisterRequestsTotal.Add(ctx, 1, metric.WithAttributes(attribute.String("status", "error")))
		return nil, status.Error(grpcCodes.InvalidArgument, "username, email, and password are required")
	}

	// Hash password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Password hashing failed")
		svc.logger.Error("Failed to hash password", zap.Error(err))
		metrics.Get().RegisterRequestsTotal.Add(ctx, 1, metric.WithAttributes(attribute.String("status", "error")))
		return nil, status.Error(grpcCodes.Internal, "failed to process password")
	}

	// Register user
	userID, err := svc.repo.Register(ctx, in.Username, in.Email, string(hashedPassword))
	if err != nil {
		span.RecordError(err)
		if errors.Is(err, types.ErrConflict) {
			span.SetStatus(codes.Error, "User already exists")
			svc.logger.Warn("Registration failed: user already exists", zap.String("email", in.Email))
			metrics.Get().RegisterRequestsTotal.Add(ctx, 1, metric.WithAttributes(attribute.String("status", "conflict")))
			return nil, status.Error(grpcCodes.AlreadyExists, "user with this email already exists")
		}
		span.SetStatus(codes.Error, "Registration failed")
		svc.logger.Error("Registration failed", zap.Error(err))
		metrics.Get().RegisterRequestsTotal.Add(ctx, 1, metric.WithAttributes(attribute.String("status", "error")))
		return nil, status.Error(grpcCodes.Internal, "registration failed")
	}

	span.SetStatus(codes.Ok, "User registered successfully")
	svc.logger.Info("User registered successfully", zap.String("userID", userID))
	metrics.Get().RegisterRequestsTotal.Add(ctx, 1, metric.WithAttributes(attribute.String("status", "success")))

	// Note: Adjust field names based on actual protobuf definition
	return &pb.RegisterResponse{
		Success: true,
		Message: "Registration successful",
		// User field will be populated when UserAuth proto message is properly mapped
	}, nil
}

func (svc *Service) Login(ctx context.Context, in *pb.LoginRequest) (*pb.LoginResponse, error) {
	ctx, span := svc.tracer.Start(ctx, "AuthService.Login", trace.WithAttributes(
		attribute.String("auth.operation", "login"),
		attribute.String("auth.email", in.Email),
	))
	defer span.End()

	svc.logger.Info("User login attempt", zap.String("email", in.Email))

	// Validation
	if strings.TrimSpace(in.Email) == "" || strings.TrimSpace(in.Password) == "" {
		span.RecordError(types.ErrBadRequest)
		span.SetStatus(codes.Error, "Invalid input")
		return nil, status.Error(grpcCodes.InvalidArgument, "email and password are required")
	}

	// Get user by email
	user, err := svc.repo.GetUserByEmail(ctx, in.Email)
	if err != nil {
		span.RecordError(err)
		if errors.Is(err, types.ErrNotFound) {
			span.SetStatus(codes.Error, "User not found")
			svc.logger.Warn("Login failed: user not found", zap.String("email", in.Email))
			return nil, status.Error(grpcCodes.NotFound, "user not found")
		}
		span.SetStatus(codes.Error, "Database error")
		svc.logger.Error("Login failed: database error", zap.Error(err))
		return nil, status.Error(grpcCodes.Internal, "login failed")
	}

	// Verify password
	err = svc.repo.VerifyPassword(ctx, user.ID, in.Password)
	if err != nil {
		span.RecordError(err)
		if errors.Is(err, types.ErrUnauthenticated) {
			span.SetStatus(codes.Error, "Invalid credentials")
			svc.logger.Warn("Login failed: invalid password", zap.String("userID", user.ID))
			return nil, status.Error(grpcCodes.Unauthenticated, "invalid credentials")
		}
		span.SetStatus(codes.Error, "Password verification error")
		svc.logger.Error("Login failed: password verification error", zap.Error(err))
		return nil, status.Error(grpcCodes.Internal, "login failed")
	}

	// Generate tokens
	accessToken, refreshToken, err := svc.generateTokens(ctx, user)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Token generation failed")
		svc.logger.Error("Login failed: token generation error", zap.Error(err))
		return nil, status.Error(grpcCodes.Internal, "login failed")
	}

	// Store refresh token
	expiresAt := time.Now().Add(7 * 24 * time.Hour) // 7 days
	err = svc.repo.StoreRefreshToken(ctx, user.ID, refreshToken, expiresAt)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to store refresh token")
		svc.logger.Error("Login failed: refresh token storage error", zap.Error(err))
		return nil, status.Error(grpcCodes.Internal, "login failed")
	}

	span.SetStatus(codes.Ok, "User logged in successfully")
	svc.logger.Info("User logged in successfully", zap.String("userID", user.ID))

	return &pb.LoginResponse{
		Success:      true,
		Message:      "Login successful",
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresIn:    3600, // 1 hour
		// User: &pb.UserAuth{  // Will be populated when proto mapping is complete
		//	Id:       user.ID,
		//	Username: user.Username,
		//	Email:    user.Email,
		//	Role:     user.Role,
		// },
	}, nil
}

func (svc *Service) RefreshToken(ctx context.Context, in *pb.RefreshTokenRequest) (*pb.TokenResponse, error) {
	ctx, span := svc.tracer.Start(ctx, "AuthService.RefreshToken", trace.WithAttributes(
		attribute.String("auth.operation", "refresh_token"),
	))
	defer span.End()

	svc.logger.Info("Token refresh attempt")

	// Validation
	if strings.TrimSpace(in.RefreshToken) == "" {
		span.RecordError(types.ErrBadRequest)
		span.SetStatus(codes.Error, "Invalid input")
		return nil, status.Error(grpcCodes.InvalidArgument, "refresh token is required")
	}

	// Validate refresh token and get user ID
	userID, err := svc.repo.ValidateRefreshTokenAndGetUserID(ctx, in.RefreshToken)
	if err != nil {
		span.RecordError(err)
		if errors.Is(err, types.ErrUnauthenticated) {
			span.SetStatus(codes.Error, "Invalid or expired refresh token")
			svc.logger.Warn("Token refresh failed: invalid token")
			return nil, status.Error(grpcCodes.Unauthenticated, "invalid or expired refresh token")
		}
		span.SetStatus(codes.Error, "Token validation error")
		svc.logger.Error("Token refresh failed: validation error", zap.Error(err))
		return nil, status.Error(grpcCodes.Internal, "token refresh failed")
	}

	// Get user details
	user, err := svc.repo.GetUserByID(ctx, userID)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "User not found")
		svc.logger.Error("Token refresh failed: user not found", zap.String("userID", userID))
		return nil, status.Error(grpcCodes.NotFound, "user not found")
	}

	// Generate new tokens
	accessToken, newRefreshToken, err := svc.generateTokens(ctx, user)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Token generation failed")
		svc.logger.Error("Token refresh failed: token generation error", zap.Error(err))
		return nil, status.Error(grpcCodes.Internal, "token refresh failed")
	}

	// Invalidate old refresh token and store new one
	err = svc.repo.InvalidateRefreshToken(ctx, in.RefreshToken)
	if err != nil {
		svc.logger.Warn("Failed to invalidate old refresh token", zap.Error(err))
	}

	expiresAt := time.Now().Add(7 * 24 * time.Hour)
	err = svc.repo.StoreRefreshToken(ctx, userID, newRefreshToken, expiresAt)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to store new refresh token")
		svc.logger.Error("Token refresh failed: refresh token storage error", zap.Error(err))
		return nil, status.Error(grpcCodes.Internal, "token refresh failed")
	}

	span.SetStatus(codes.Ok, "Token refreshed successfully")
	svc.logger.Info("Token refreshed successfully", zap.String("userID", userID))

	return &pb.TokenResponse{
		AccessToken:  accessToken,
		RefreshToken: newRefreshToken,
		ExpiresIn:    3600,
	}, nil
}

func (svc *Service) Logout(ctx context.Context, in *pb.LogoutRequest) (*pb.LogoutResponse, error) {
	ctx, span := svc.tracer.Start(ctx, "AuthService.Logout", trace.WithAttributes(
		attribute.String("auth.operation", "logout"),
		attribute.String("auth.user_id", in.UserId),
	))
	defer span.End()

	svc.logger.Info("User logout attempt", zap.String("userID", in.UserId))

	// Validation
	if strings.TrimSpace(in.UserId) == "" {
		span.RecordError(types.ErrBadRequest)
		span.SetStatus(codes.Error, "Invalid input")
		return nil, status.Error(grpcCodes.InvalidArgument, "user ID is required")
	}

	// Invalidate all refresh tokens for the user
	err := svc.repo.InvalidateAllUserRefreshTokens(ctx, in.UserId)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to invalidate tokens")
		svc.logger.Error("Logout failed: token invalidation error", zap.Error(err))
		return nil, status.Error(grpcCodes.Internal, "logout failed")
	}

	span.SetStatus(codes.Ok, "User logged out successfully")
	svc.logger.Info("User logged out successfully", zap.String("userID", in.UserId))

	return &pb.LogoutResponse{
		Success: true,
		Message: "Logged out successfully",
	}, nil
}

func (svc *Service) ValidateSession(ctx context.Context, in *pb.ValidateSessionRequest) (*pb.ValidateSessionResponse, error) {
	ctx, span := svc.tracer.Start(ctx, "AuthService.ValidateSession", trace.WithAttributes(
		attribute.String("auth.operation", "validate_session"),
	))
	defer span.End()

	svc.logger.Info("Session validation attempt")

	// Validation
	if strings.TrimSpace(in.AccessToken) == "" {
		span.RecordError(types.ErrBadRequest)
		span.SetStatus(codes.Error, "Invalid input")
		return nil, status.Error(grpcCodes.InvalidArgument, "access token is required")
	}

	// Parse and validate JWT token
	claims := &Claims{}
	token, err := jwt.ParseWithClaims(in.AccessToken, claims, func(token *jwt.Token) (interface{}, error) {
		return JwtSecretKey, nil
	})

	if err != nil || !token.Valid {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Invalid token")
		svc.logger.Warn("Session validation failed: invalid token", zap.Error(err))
		return &pb.ValidateSessionResponse{
			Valid: false,
		}, nil
	}

	// Get user details to verify user still exists and is active
	_, err = svc.repo.GetUserByID(ctx, claims.UserID)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "User validation error")
		svc.logger.Warn("Session validation failed: user not found", zap.String("userID", claims.UserID))
		return &pb.ValidateSessionResponse{
			Valid: false,
		}, nil
	}

	span.SetStatus(codes.Ok, "Session validated successfully")
	svc.logger.Info("Session validated successfully", zap.String("userID", claims.UserID))

	return &pb.ValidateSessionResponse{
		Valid:     true,
		UserId:    claims.UserID,
		Email:     claims.Email,
		Role:      claims.Role,
		ExpiresAt: claims.ExpiresAt.Unix(),
	}, nil
}

// generateTokens creates both access and refresh tokens for a user
func (svc *Service) generateTokens(ctx context.Context, user *UserAuth) (accessToken, refreshToken string, err error) {
	ctx, span := svc.tracer.Start(ctx, "AuthService.generateTokens")
	defer span.End()

	// Create access token claims
	accessClaims := &Claims{
		UserID: user.ID,
		Email:  user.Email,
		Role:   user.Role,
		Scope:  "access",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(15 * time.Minute)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			NotBefore: jwt.NewNumericDate(time.Now()),
			Issuer:    "go-ai-poi-server",
			Subject:   user.ID,
			Audience:  []string{"go-ai-poi-client"},
		},
	}

	// Generate access token
	accessTokenObj := jwt.NewWithClaims(jwt.SigningMethodHS256, accessClaims)
	accessToken, err = accessTokenObj.SignedString(JwtSecretKey)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to sign access token")
		return "", "", fmt.Errorf("failed to generate access token: %w", err)
	}

	// Generate refresh token (simple UUID)
	refreshToken = uuid.New().String()

	span.SetStatus(codes.Ok, "Tokens generated successfully")
	return accessToken, refreshToken, nil
}
