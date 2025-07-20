package auth

import (
	"context"

	pb "github.com/FACorreiaa/fitme-protos/modules/user/generated"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/FACorreiaa/go-poi-au-suggestions/internal/domain"
)

type Service struct {
	pb.UnimplementedAuthServer
	ctx            context.Context
	repo           domain.AuthRepository
	pgpool         *pgxpool.Pool
	redis          *redis.Client
	SessionManager *SessionManager
}

func NewService(ctx context.Context, repo domain.AuthRepository,
	db *pgxpool.Pool,
	redis *redis.Client,
	sessionManager *SessionManager) *Service {
	return &Service{ctx: ctx, repo: repo, pgpool: db, redis: redis, SessionManager: sessionManager}
}

func (s *Service) Register(ctx context.Context, req *pb.RegisterRequest) (*pb.RegisterResponse, error) {
	return s.repo.Register(ctx, req)
}

func (s *Service) Login(ctx context.Context, req *pb.LoginRequest) (*pb.LoginResponse, error) {
	return s.repo.Login(ctx, req)
}

func (s *Service) Logout(ctx context.Context, req *pb.NilReq) (*pb.NilRes, error) {
	return s.repo.Logout(ctx, req)
}

func (s *Service) ChangePassword(ctx context.Context, req *pb.ChangePasswordRequest) (*pb.ChangePasswordResponse, error) {
	return s.repo.ChangePassword(ctx, req)
}

func (s *Service) ChangeEmail(ctx context.Context, req *pb.ChangeEmailRequest) (*pb.ChangeEmailResponse, error) {
	return s.repo.ChangeEmail(ctx, req)
}

func (s *Service) GetAllUsers(ctx context.Context, _ *pb.GetAllUsersRequest) (*pb.GetAllUsersResponse, error) {
	users, err := s.repo.GetAllUsers(ctx)
	if err != nil {
		return nil, err
	}
	return users, nil
}

func (s *Service) GetUserByID(ctx context.Context, req *pb.GetUserByIDRequest) (*pb.GetUserByIDResponse, error) {
	user, err := s.repo.GetUserByID(ctx, req)
	if err != nil {
		return nil, err
	}
	return user, nil
}

func (s *Service) DeleteUser(ctx context.Context, req *pb.DeleteUserRequest) (*pb.DeleteUserResponse, error) {
	return s.repo.DeleteUser(ctx, req)
}

func (s *Service) UpdateUser(ctx context.Context, req *pb.UpdateUserRequest) (*pb.UpdateUserResponse, error) {
	return s.repo.UpdateUser(ctx, req)
}

func (s *Service) InsertUser(ctx context.Context, req *pb.InsertUserRequest) (*pb.InsertUserResponse, error) {
	return s.repo.InsertUser(ctx, req)
}
