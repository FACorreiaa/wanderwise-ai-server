package domain

import (
	"context"
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Domain specific errors for authentication and authorization.
var (
	ErrNotFound        = errors.New("requested item not found")
	ErrConflict        = errors.New("item already exists or conflict")
	ErrUnauthenticated = errors.New("authentication required or invalid credentials")
	ErrForbidden       = errors.New("action forbidden")
	ErrBadRequest      = errors.New("bad request")
)

func CheckUserAuth(ctx context.Context) (string, error) {
	userIDValue := ctx.Value("userID")
	if userIDValue == nil {
		return "", status.Error(codes.Unauthenticated, "userID is missing in metadata")
	}

	userID, ok := userIDValue.(string)
	if !ok {
		return "", status.Error(codes.Unauthenticated, "userID has invalid type in metadata")
	}

	if userID == "" {
		return "", status.Error(codes.Unauthenticated, "userID is empty in metadata")
	}

	return userID, nil
}
