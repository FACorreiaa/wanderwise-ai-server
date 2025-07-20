package grpcratelimit

import (
	"context"

	"golang.org/x/time/rate"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

//// UnaryServerInterceptor returns a new unary server interceptor that performs request rate limiting.
//func UnaryServerInterceptor() grpc.UnaryServerInterceptor {
//	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
//		// TODO: implement rate limiting
//		return handler(ctx, req)
//	}
//}
//
//// StreamServerInterceptor returns a new streaming server interceptor that performs request rate limiting.
//func StreamServerInterceptor() grpc.StreamServerInterceptor {
//	return func(srv interface{}, stream grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
//		// TODO: implement rate limiting
//		return handler(srv, stream)
//	}
//}
//
//// UnaryClientInterceptor returns a new unary client interceptor that performs request rate limiting.
//func UnaryClientInterceptor() grpc.UnaryClientInterceptor {
//	return func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
//		// TODO: implement rate limiting
//		return invoker(ctx, method, req, reply, cc, opts...)
//	}
//}
//
//// StreamClientInterceptor returns a new streaming client interceptor that performs request rate limiting.
//func StreamClientInterceptor() grpc.StreamClientInterceptor {
//	return func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
//		// TODO: implement rate limiting
//		return streamer(ctx, desc, cc, method, opts...)
//	}
//}
//
//// Interceptors returns the unary and stream client interceptors.
//func Interceptors() (middleware.ClientInterceptor, middleware.ServerInterceptor) {
//	return middleware.ClientInterceptor{}, middleware.ServerInterceptor{}
//}

type RateLimiter struct {
	limiter *rate.Limiter
}

func NewRateLimiter(rps float64, burst int) *RateLimiter {
	return &RateLimiter{
		limiter: rate.NewLimiter(rate.Limit(rps), burst),
	}
}

func (rl *RateLimiter) UnaryServerInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		if !rl.limiter.Allow() {
			return nil, status.Errorf(
				codes.ResourceExhausted,
				"rate limit exceeded: %s",
				info.FullMethod,
			)
		}
		return handler(ctx, req)
	}
}

func (rl *RateLimiter) StreamServerInterceptor() grpc.StreamServerInterceptor {
	return func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		if !rl.limiter.Allow() {
			return status.Errorf(
				codes.ResourceExhausted,
				"rate limit exceeded: %s",
				info.FullMethod,
			)
		}
		return handler(srv, ss)
	}
}
