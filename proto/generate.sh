#!/bin/bash

# Proto generation script for go-connect migration
# This script generates Go code from proto files

set -e

PROTO_DIR="proto"
OUT_DIR="gen/proto"

echo "🔧 Generating Go code from proto files..."

# Create output directory
mkdir -p $OUT_DIR

# List of proto files in dependency order
PROTO_FILES=(
    "common.proto"
    "auth.proto"
    "user.proto"
    "city.proto"
    "interest.proto"
    "poi.proto"
    "profile.proto"
    "itinerary.proto"
    "chat.proto"
    "discover.proto"
)

# Generate code for each proto file
for proto_file in "${PROTO_FILES[@]}"; do
    echo "📝 Generating $proto_file..."
    
    protoc \
        --proto_path=$PROTO_DIR \
        --go_out=$OUT_DIR \
        --go_opt=paths=source_relative \
        --connect-go_out=$OUT_DIR \
        --connect-go_opt=paths=source_relative \
        $PROTO_DIR/$proto_file
done

echo "✅ Code generation complete! Output in $OUT_DIR"
echo ""
echo "Next steps:"
echo "1. Update go_package options in proto files to match your module"
echo "2. Run this script again to regenerate with correct paths"
echo "3. Implement service handlers (see MIGRATION_GUIDE.md)"
echo "4. Register services with Connect in your main.go"
