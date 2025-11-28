# Proto Schema Migration Guide

This directory contains Protocol Buffer (proto) schemas generated from your existing Go API models. These schemas are designed to facilitate migration to ConnectRPC (go-connect).

## Generated Proto Files

### Core Schemas

1. **common.proto** - Shared types and utilities
   - Response wrappers
   - Pagination structures
   - GeoPoint and RangeFilter helpers

2. **auth.proto** - Authentication & Authorization
   - Login/Register/Logout flows
   - Token management (access & refresh tokens)
   - Password/Email change operations
   - JWT Claims structure

3. **user.proto** - User Profile Management
   - UserProfile with stats, interests, badges
   - Profile update operations

4. **city.proto** - City Information
   - City details with coordinates
   - City search functionality
   - General city data (population, timezone, etc.)

5. **poi.proto** - Points of Interest
   - POI detailed information
   - Hotel and Restaurant specific models
   - POI search with filters

6. **interest.proto** - User Interests & Tags
   - Interest management
   - User interest associations
   - Preference levels

7. **profile.proto** - User Preference Profiles
   - Comprehensive preference system with enums
   - Domain-specific preferences (Accommodation, Dining, Activity, Itinerary)
   - Multiple preference profiles per user

8. **itinerary.proto** - Lists & Itineraries
   - List management (generic and itinerary-specific)
   - List items with multiple content types
   - Saved itineraries with markdown support
   - Bookmarking functionality

9. **chat.proto** - Chat Sessions & LLM Interactions
   - Chat session management
   - Conversation history
   - Streaming chat support
   - Session metrics (performance, content, engagement)
   - Recent interactions

10. **discover.proto** - Discovery Features
    - Trending discoveries
    - Featured collections
    - Category browsing

## Next Steps for Migration

### 1. Install Required Dependencies

```bash
# Install protoc compiler
brew install protobuf  # macOS
# or
apt-get install protobuf-compiler  # Linux

# Install Go plugins
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install connectrpc.com/connect/cmd/protoc-gen-connect-go@latest
```

### 2. Update go_package Option

Update the `go_package` option in each proto file to match your actual module path:

```protobuf
option go_package = "github.com/YOUR-ORG/go-ai-poi-server/gen/proto/PACKAGE";
```

### 3. Generate Go Code

Create a script to generate Go code from proto files:

```bash
#!/bin/bash
# generate.sh

PROTO_DIR="proto"
OUT_DIR="gen/proto"

mkdir -p $OUT_DIR

for proto_file in $PROTO_DIR/*.proto; do
  protoc \
    --proto_path=$PROTO_DIR \
    --go_out=$OUT_DIR \
    --go_opt=paths=source_relative \
    --connect-go_out=$OUT_DIR \
    --connect-go_opt=paths=source_relative \
    $proto_file
done
```

### 4. Implement Service Handlers

For each service defined in the proto files, you'll need to implement the handlers:

```go
// Example for AuthService
type authServiceHandler struct {
    authService *auth.Service
}

func (h *authServiceHandler) Login(
    ctx context.Context,
    req *connect.Request[authv1.LoginRequest],
) (*connect.Response[authv1.LoginResponse], error) {
    // Call your existing auth service
    result, err := h.authService.Login(ctx, req.Msg.Email, req.Msg.Password)
    if err != nil {
        return nil, err
    }
    
    return connect.NewResponse(&authv1.LoginResponse{
        AccessToken: result.AccessToken,
        RefreshToken: result.RefreshToken,
        Message: "Login successful",
    }), nil
}
```

### 5. Register Services with Connect

```go
import (
    "connectrpc.com/connect"
    authv1connect "your-module/gen/proto/auth/authconnect"
)

mux := http.NewServeMux()

// Register services
authHandler := &authServiceHandler{authService: authSvc}
path, handler := authv1connect.NewAuthServiceHandler(authHandler)
mux.Handle(path, handler)

// Add CORS and other middleware as needed
```

### 6. Migrate Gradually

You can migrate endpoint by endpoint:

1. Keep existing REST handlers
2. Add Connect handlers alongside
3. Test thoroughly
4. Switch clients over
5. Remove old REST handlers when ready

## Key Differences from REST

- **Typed requests/responses**: No more manual JSON marshaling
- **Streaming support**: Built-in bidirectional streaming
- **Code generation**: Client and server code generated from proto
- **HTTP/1.1 & HTTP/2**: Works with both protocols
- **gRPC compatible**: Can work with gRPC clients

## File Dependencies

The proto files have the following dependency structure:

```
common.proto (no dependencies)
├── auth.proto
├── user.proto
├── city.proto
├── interest.proto
│   └── profile.proto
│       ├── itinerary.proto
│       └── chat.proto
│           └── discover.proto
└── poi.proto
    ├── itinerary.proto
    └── chat.proto
```

Compile in dependency order or use the generation script above which handles it automatically.

## Additional Resources

- [Connect Documentation](https://connectrpc.com/docs/go/getting-started)
- [Protocol Buffers Guide](https://protobuf.dev/programming-guides/proto3/)
- [Connect vs gRPC](https://connectrpc.com/docs/introduction)
