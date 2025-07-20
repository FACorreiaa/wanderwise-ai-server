package middleware

import (
	"log"
	"sync"
	"time"

	"context"

	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/stats"
	"google.golang.org/protobuf/proto"
)

type ClientHandler struct {
	Handler stats.Handler
}

type ServerHandler struct {
	Handler stats.Handler
}

var (
	Log      *zap.Logger
	onceInit sync.Once
)

func initializeLogger() {
	// Initialize Zap logger
	// add adapter later
	Log, _ = zap.NewProduction()
	defer func(Log *zap.Logger) {
		err := Log.Sync()
		if err != nil {
			zap.Error(err)
			log.Fatal(err)
		}
	}(Log) // Flushes buffer, if any
}

func KeepaliveEnforcementPolicy() keepalive.EnforcementPolicy {
	return keepalive.EnforcementPolicy{
		MinTime:             10 * time.Second,
		PermitWithoutStream: true,
	}
}

func KeepAliveServerParams() keepalive.ServerParameters {
	return keepalive.ServerParameters{
		Time:    10 * time.Second,
		Timeout: 10 * time.Second,
	}
}

type ClientInterceptor struct {
	Unary  grpc.UnaryClientInterceptor
	Stream grpc.StreamClientInterceptor
}

func (cl *ClientInterceptor) UnaryClientInterceptor(
	ctx context.Context,
	method string,
	req interface{},
	reply interface{},
	cc *grpc.ClientConn,
	invoker grpc.UnaryInvoker,
	opts ...grpc.CallOption,
) error {
	onceInit.Do(initializeLogger)

	opts = append(opts,
		grpc.MaxCallRecvMsgSize(1024*1024*4), // 4 MB max receive size
		grpc.MaxCallSendMsgSize(1024*1024*4), // 4 MB max send size
		grpc.UseCompressor("gzip"),           // Enable gzip compression
		grpc.WaitForReady(true),              // Wait for server to be ready
		// grpc.PerRPCCredentials(creds),      // Uncomment for per-RPC credentials
	)

	// Logic before invoking the invoker
	start := time.Now()
	// Calls the invoker to execute RPC

	//reqSize := getMessageSize(req)

	err := invoker(ctx, method, req, reply, cc, opts...)
	// Logic after invoking the invoker

	//respSize := getMessageSize(reply)

	Log.Info("Invoked RPC method",
		zap.String("method", method),
		zap.Duration("duration", time.Since(start)),
		//zap.Int("request_size", reqSize),
		//zap.Int("response_size", respSize),
		zap.Error(err),
	)
	return err
}

type ServerInterceptor struct {
	Unary  grpc.UnaryServerInterceptor
	Stream grpc.StreamServerInterceptor
}

// UnaryServerInterceptor is a middleware for gRPC server requests.
func (si *ServerInterceptor) UnaryServerInterceptor(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	log.Printf("Server request method: %s", info.FullMethod)
	// TODO fill logic
	resp, err := handler(ctx, req)
	return resp, err
}

func getMessageSize(msg interface{}) int {
	// Check if the message can be marshaled to proto
	if protoMsg, ok := msg.(proto.Message); ok {
		// Marshal the proto message and measure its size
		data, err := proto.Marshal(protoMsg)
		if err == nil {
			return len(data)
		}
		// Log error if marshalling fails
		Log.Warn("Failed to marshal proto message", zap.Error(err))
	}
	// Return 0 if the message isn't a proto message or failed to marshal
	return 0
}
