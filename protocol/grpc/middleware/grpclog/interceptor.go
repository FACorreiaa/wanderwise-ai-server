package grpclog

import (
	"context"

	grpcZap "github.com/grpc-ecosystem/go-grpc-middleware/logging/zap"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/proto"

	"github.com/FACorreiaa/go-poi-au-suggestions/protocol/grpc/middleware"
)

func Interceptors(instance *zap.Logger) (middleware.ClientInterceptor, middleware.ServerInterceptor) {
	opt := []grpcZap.Option{
		grpcZap.WithLevels(codeToLevel),
	}

	grpcZap.ReplaceGrpcLoggerV2WithVerbosity(instance, int(zap.WarnLevel))

	// Wrap client interceptor
	clientInterceptor := middleware.ClientInterceptor{
		Unary:  logPayloadSizeUnaryClientInterceptor(instance, grpcZap.UnaryClientInterceptor(instance, opt...)),
		Stream: grpcZap.StreamClientInterceptor(instance, opt...), // Stream not modified for simplicity
	}

	// Wrap server interceptor
	serverInterceptor := middleware.ServerInterceptor{
		Unary:  logPayloadSizeUnaryServerInterceptor(instance, grpcZap.UnaryServerInterceptor(instance, opt...)),
		Stream: grpcZap.StreamServerInterceptor(instance, opt...), // Stream not modified for simplicity
	}

	return clientInterceptor, serverInterceptor
}

// codeToLevel translates a GRPC status code to a zap logging level
func codeToLevel(code codes.Code) zapcore.Level {
	// override OK to DebugLevel
	if code == codes.OK {
		return zap.DebugLevel
	}
	return grpcZap.DefaultCodeToLevel(code)
}

// logPayloadSizeUnaryServerInterceptor wraps the server-side interceptor to log payload sizes
func logPayloadSizeUnaryServerInterceptor(logger *zap.Logger, baseInterceptor grpc.UnaryServerInterceptor) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		// Calculate request payload size
		reqSize := getPayloadSize(req)
		logger.Debug("Server received request",
			zap.String("method", info.FullMethod),
			zap.Int("request_size_bytes", reqSize),
		)

		// Call the base interceptor (grpcZap)
		resp, err := baseInterceptor(ctx, req, info, handler)

		// Calculate response payload size
		respSize := getPayloadSize(resp)
		logger.Debug("Server sent response",
			zap.String("method", info.FullMethod),
			zap.Int("response_size_bytes", respSize),
			zap.Error(err),
		)

		return resp, err
	}
}

// logPayloadSizeUnaryClientInterceptor wraps the client-side interceptor to log payload sizes
func logPayloadSizeUnaryClientInterceptor(logger *zap.Logger, baseInterceptor grpc.UnaryClientInterceptor) grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, resp interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		// Calculate request payload size
		//reqSize := getPayloadSize(req)
		//logger.Debug("Client sent request",
		//	zap.String("method", method),
		//	zap.Int("request_size_bytes", reqSize),
		//)

		// Call the base interceptor (grpcZap)
		err := baseInterceptor(ctx, method, req, resp, cc, invoker, opts...)

		// Calculate response payload size
		respSize := getPayloadSize(resp)
		logger.Debug("Client received response",
			zap.String("method", method),
			zap.Int("response_size_bytes", respSize),
			zap.Error(err),
		)

		return err
	}
}

// getPayloadSize calculates the size of the gRPC payload
func getPayloadSize(msg interface{}) int {
	if protoMsg, ok := msg.(proto.Message); ok {
		if data, err := proto.Marshal(protoMsg); err == nil {
			return len(data)
		}
	}
	return 0
}
