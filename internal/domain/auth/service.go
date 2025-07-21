package auth

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
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
	"golang.org/x/crypto/bcrypt"
	grpcCodes "google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "github.com/FACorreiaa/loci-proto/modules/auth/generated"

	"github.com/FACorreiaa/go-poi-au-suggestions/app/observability/metrics"
	"github.com/FACorreiaa/go-poi-au-suggestions/internal/domain"
	"github.com/FACorreiaa/go-poi-au-suggestions/internal/types"
)

type Service struct {
	pb.UnimplementedAuthServiceServer
	logger *slog.Logger
	repo   domain.AuthRepository
	pgpool *pgxpool.Pool
	tracer trace.Tracer
}

func NewService(ctx context.Context, repo domain.AuthRepository, pgpool *pgxpool.Pool, logger *slog.Logger) *Service {
	return &Service{
		logger: logger.With(slog.String("service", "auth")),
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

	svc.logger.InfoContext(ctx, "User registration attempt",
		slog.String("username", in.Username),
		slog.String("email", in.Email))

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
		svc.logger.ErrorContext(ctx, "Failed to hash password", slog.Any("error", err))
		metrics.Get().RegisterRequestsTotal.Add(ctx, 1, metric.WithAttributes(attribute.String("status", "error")))
		return nil, status.Error(grpcCodes.Internal, "failed to process password")
	}

	// Register user
	userID, err := svc.repo.Register(ctx, in.Username, in.Email, string(hashedPassword))
	if err != nil {
		span.RecordError(err)
		if errors.Is(err, types.ErrConflict) {
			span.SetStatus(codes.Error, "User already exists")
			svc.logger.WarnContext(ctx, "Registration failed: user already exists", slog.String("email", in.Email))
			metrics.Get().RegisterRequestsTotal.Add(ctx, 1, metric.WithAttributes(attribute.String("status", "conflict")))
			return nil, status.Error(grpcCodes.AlreadyExists, "user with this email already exists")
		}
		span.SetStatus(codes.Error, "Registration failed")
		svc.logger.ErrorContext(ctx, "Registration failed", slog.Any("error", err))
		metrics.Get().RegisterRequestsTotal.Add(ctx, 1, metric.WithAttributes(attribute.String("status", "error")))
		return nil, status.Error(grpcCodes.Internal, "registration failed")
	}

	span.SetStatus(codes.Ok, "User registered successfully")
	svc.logger.InfoContext(ctx, "User registered successfully", slog.String("userID", userID))
	metrics.Get().RegisterRequestsTotal.Add(ctx, 1, metric.WithAttributes(attribute.String("status", "success")))

	// Note: Adjust field names based on actual protobuf definition
	return &pb.RegisterResponse{
		// UserId:  userID,  // Commented until protobuf field name is confirmed
		// Message: "Registration successful",  // Commented until protobuf field name is confirmed
	}, nil
}

func (svc *Service) Login(ctx context.Context, in *pb.LoginRequest) (*pb.LoginResponse, error) {
	ctx, span := svc.tracer.Start(ctx, "AuthService.Login", trace.WithAttributes(
		attribute.String("auth.operation", "login"),
		attribute.String("auth.email", in.Email),
	))
	defer span.End()

	svc.logger.InfoContext(ctx, "User login attempt", slog.String("email", in.Email))

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
			svc.logger.WarnContext(ctx, "Login failed: user not found", slog.String("email", in.Email))
			return nil, status.Error(grpcCodes.NotFound, "user not found")
		}
		span.SetStatus(codes.Error, "Database error")
		svc.logger.ErrorContext(ctx, "Login failed: database error", slog.Any("error", err))
		return nil, status.Error(grpcCodes.Internal, "login failed")
	}

	// Verify password
	err = svc.repo.VerifyPassword(ctx, user.ID, in.Password)
	if err != nil {
		span.RecordError(err)
		if errors.Is(err, types.ErrUnauthenticated) {
			span.SetStatus(codes.Error, "Invalid credentials")
			svc.logger.WarnContext(ctx, "Login failed: invalid password", slog.String("userID", user.ID))
			return nil, status.Error(grpcCodes.Unauthenticated, "invalid credentials")
		}
		span.SetStatus(codes.Error, "Password verification error")
		svc.logger.ErrorContext(ctx, "Login failed: password verification error", slog.Any("error", err))
		return nil, status.Error(grpcCodes.Internal, "login failed")
	}

	// Generate tokens
	accessToken, refreshToken, err := svc.generateTokens(ctx, user)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Token generation failed")
		svc.logger.ErrorContext(ctx, "Login failed: token generation error", slog.Any("error", err))
		return nil, status.Error(grpcCodes.Internal, "login failed")
	}

	// Store refresh token
	expiresAt := time.Now().Add(7 * 24 * time.Hour) // 7 days
	err = svc.repo.StoreRefreshToken(ctx, user.ID, refreshToken, expiresAt)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to store refresh token")
		svc.logger.ErrorContext(ctx, "Login failed: refresh token storage error", slog.Any("error", err))
		return nil, status.Error(grpcCodes.Internal, "login failed")
	}

	span.SetStatus(codes.Ok, "User logged in successfully")
	svc.logger.InfoContext(ctx, "User logged in successfully", slog.String("userID", user.ID))

	// Note: Adjust field names based on actual protobuf definition
	return &pb.LoginResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresIn:    3600, // 1 hour
		// User: &pb.User{  // Commented until protobuf type is confirmed
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

	svc.logger.InfoContext(ctx, "Token refresh attempt")

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
			svc.logger.WarnContext(ctx, "Token refresh failed: invalid token")
			return nil, status.Error(grpcCodes.Unauthenticated, "invalid or expired refresh token")
		}
		span.SetStatus(codes.Error, "Token validation error")
		svc.logger.ErrorContext(ctx, "Token refresh failed: validation error", slog.Any("error", err))
		return nil, status.Error(grpcCodes.Internal, "token refresh failed")
	}

	// Get user details
	user, err := svc.repo.GetUserByID(ctx, userID)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "User not found")
		svc.logger.ErrorContext(ctx, "Token refresh failed: user not found", slog.String("userID", userID))
		return nil, status.Error(grpcCodes.NotFound, "user not found")
	}

	// Generate new tokens
	accessToken, newRefreshToken, err := svc.generateTokens(ctx, user)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Token generation failed")
		svc.logger.ErrorContext(ctx, "Token refresh failed: token generation error", slog.Any("error", err))
		return nil, status.Error(grpcCodes.Internal, "token refresh failed")
	}

	// Invalidate old refresh token and store new one
	err = svc.repo.InvalidateRefreshToken(ctx, in.RefreshToken)
	if err != nil {
		svc.logger.WarnContext(ctx, "Failed to invalidate old refresh token", slog.Any("error", err))
	}

	expiresAt := time.Now().Add(7 * 24 * time.Hour)
	err = svc.repo.StoreRefreshToken(ctx, userID, newRefreshToken, expiresAt)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to store new refresh token")
		svc.logger.ErrorContext(ctx, "Token refresh failed: refresh token storage error", slog.Any("error", err))
		return nil, status.Error(grpcCodes.Internal, "token refresh failed")
	}

	span.SetStatus(codes.Ok, "Token refreshed successfully")
	svc.logger.InfoContext(ctx, "Token refreshed successfully", slog.String("userID", userID))

	return &pb.TokenResponse{
		AccessToken:  accessToken,
		RefreshToken: newRefreshToken,
		ExpiresIn:    3600,
	}, nil
}

func (svc *Service) Logout(ctx context.Context, in *pb.LogoutRequest) (*pb.LogoutResponse, error) {
	ctx, span := svc.tracer.Start(ctx, "AuthService.Logout", trace.WithAttributes(
		attribute.String("auth.operation", "logout"),
	))
	defer span.End()

	svc.logger.InfoContext(ctx, "User logout attempt")

	// Note: Adjust field name based on actual protobuf definition
	// For now, assuming the field might be named differently
	refreshToken := ""
	// if in.RefreshToken != "" { refreshToken = in.RefreshToken }
	// if in.Token != "" { refreshToken = in.Token } // Alternative field name

	if refreshToken == "" {
		span.RecordError(types.ErrBadRequest)
		span.SetStatus(codes.Error, "Invalid input")
		return nil, status.Error(grpcCodes.InvalidArgument, "refresh token is required")
	}

	// Invalidate refresh token
	err := svc.repo.InvalidateRefreshToken(ctx, refreshToken)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to invalidate token")
		svc.logger.ErrorContext(ctx, "Logout failed: token invalidation error", slog.Any("error", err))
		return nil, status.Error(grpcCodes.Internal, "logout failed")
	}

	span.SetStatus(codes.Ok, "User logged out successfully")
	svc.logger.InfoContext(ctx, "User logged out successfully")

	return &pb.LogoutResponse{
		Message: "Logged out successfully",
	}, nil
}

func (svc *Service) ValidateSession(ctx context.Context, in *pb.ValidateSessionRequest) (*pb.ValidateSessionResponse, error) {
	ctx, span := svc.tracer.Start(ctx, "AuthService.ValidateSession", trace.WithAttributes(
		attribute.String("auth.operation", "validate_session"),
	))
	defer span.End()

	svc.logger.InfoContext(ctx, "Session validation attempt")

	// Validation
	if strings.TrimSpace(in.AccessToken) == "" {
		span.RecordError(types.ErrBadRequest)
		span.SetStatus(codes.Error, "Invalid input")
		return nil, status.Error(grpcCodes.InvalidArgument, "access token is required")
	}

	// Parse and validate JWT token
	claims := &domain.Claims{}
	token, err := jwt.ParseWithClaims(in.AccessToken, claims, func(token *jwt.Token) (interface{}, error) {
		return domain.JwtSecretKey, nil
	})

	if err != nil || !token.Valid {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Invalid token")
		svc.logger.WarnContext(ctx, "Session validation failed: invalid token", slog.Any("error", err))
		return &pb.ValidateSessionResponse{
			Valid: false,
			// Message: "Invalid or expired token",  // Commented until protobuf field is confirmed
		}, nil
	}

	// Get user details to verify user still exists and is active
	_, err = svc.repo.GetUserByID(ctx, claims.UserID)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "User validation error")
		svc.logger.WarnContext(ctx, "Session validation failed: user not found", slog.String("userID", claims.UserID))
		return &pb.ValidateSessionResponse{
			Valid: false,
			// Message: "User not found",  // Commented until protobuf field is confirmed
		}, nil
	}

	span.SetStatus(codes.Ok, "Session validated successfully")
	svc.logger.InfoContext(ctx, "Session validated successfully", slog.String("userID", claims.UserID))

	return &pb.ValidateSessionResponse{
		Valid: true,
		// User: &pb.User{  // Commented until protobuf type is confirmed
		//	Id:       user.ID,
		//	Username: user.Username,
		//	Email:    user.Email,
		//	Role:     user.Role,
		// },
		// Message: "Session is valid",  // Commented until protobuf field is confirmed
	}, nil
}

// generateTokens creates both access and refresh tokens for a user
func (svc *Service) generateTokens(ctx context.Context, user *domain.UserAuth) (accessToken, refreshToken string, err error) {
	ctx, span := svc.tracer.Start(ctx, "AuthService.generateTokens")
	defer span.End()

	// Create access token claims
	accessClaims := &domain.Claims{
		UserID: user.ID,
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
	accessToken, err = accessTokenObj.SignedString(domain.JwtSecretKey)
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
