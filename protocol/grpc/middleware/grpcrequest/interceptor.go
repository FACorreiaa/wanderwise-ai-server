package grpcrequest

import (
	"context"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

type RequestIDKey struct{}

func RequestIDMiddleware() grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (resp interface{}, err error) {
		requestID := uuid.New().String()

		ctx = context.WithValue(ctx, RequestIDKey{}, requestID)

		md := metadata.Pairs("request-id", requestID)
		err = grpc.SendHeader(ctx, md)
		if err != nil {
			return nil, err
		}

		resp, err = handler(ctx, req)

		return resp, err
	}
}
