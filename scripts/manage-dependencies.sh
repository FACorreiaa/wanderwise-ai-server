#!/bin/bash
# Dependency management script for Go AI POI microservices

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"
SHARED_DIR="$PROJECT_ROOT/shared"
DOMAINS_DIR="$PROJECT_ROOT/internal/domain"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

log() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Function to add a global dependency to shared module
add_global_dependency() {
    local package="$1"
    local version="$2"
    
    if [[ -z "$package" ]]; then
        error "Package name is required"
        echo "Usage: $0 add-global <package> [version]"
        exit 1
    fi
    
    log "Adding global dependency: $package"
    
    cd "$SHARED_DIR"
    if [[ -n "$version" ]]; then
        go get "$package@$version"
    else
        go get "$package"
    fi
    
    log "Updating workspace..."
    cd "$PROJECT_ROOT"
    go work sync
    
    log "Global dependency '$package' added successfully!"
}

# Function to remove a global dependency
remove_global_dependency() {
    local package="$1"
    
    if [[ -z "$package" ]]; then
        error "Package name is required"
        echo "Usage: $0 remove-global <package>"
        exit 1
    fi
    
    log "Removing global dependency: $package"
    
    cd "$SHARED_DIR"
    go mod edit -droprequire="$package"
    
    log "Updating workspace..."
    cd "$PROJECT_ROOT"
    go work sync
    
    log "Global dependency '$package' removed successfully!"
}

# Function to update all dependencies
update_dependencies() {
    log "Updating all dependencies in workspace..."
    
    cd "$PROJECT_ROOT"
    go get -u ./...
    go work sync
    
    log "All dependencies updated successfully!"
}

# Function to sync workspace
sync_workspace() {
    log "Syncing workspace dependencies..."
    
    cd "$PROJECT_ROOT"
    go work sync
    
    log "Workspace synced successfully!"
}

# Function to list global dependencies
list_global_dependencies() {
    log "Global dependencies in shared module:"
    
    cd "$SHARED_DIR"
    go list -m all | grep -v "^github.com/loci/shared$" | head -20
}

# Function to add service-specific dependency
add_service_dependency() {
    local service="$1"
    local package="$2"
    local version="$3"
    
    if [[ -z "$service" || -z "$package" ]]; then
        error "Service name and package name are required"
        echo "Usage: $0 add-service <service-name> <package> [version]"
        exit 1
    fi
    
    local service_dir="$DOMAINS_DIR/$service"
    
    if [[ ! -d "$service_dir" ]]; then
        error "Service directory not found: $service_dir"
        exit 1
    fi
    
    log "Adding dependency to $service: $package"
    
    cd "$service_dir"
    if [[ -n "$version" ]]; then
        go get "$package@$version"
    else
        go get "$package"
    fi
    
    log "Updating workspace..."
    cd "$PROJECT_ROOT"
    go work sync
    
    log "Dependency '$package' added to service '$service' successfully!"
}

# Function to rebuild base image
rebuild_base_image() {
    log "Rebuilding shared base image..."
    
    cd "$PROJECT_ROOT"
    docker build -f Dockerfile.base -t loci-base:latest .
    
    log "Base image rebuilt successfully!"
}

# Main command handling
case "$1" in
    "add-global")
        add_global_dependency "$2" "$3"
        ;;
    "remove-global")
        remove_global_dependency "$2"
        ;;
    "add-service")
        add_service_dependency "$2" "$3" "$4"
        ;;
    "update")
        update_dependencies
        ;;
    "sync")
        sync_workspace
        ;;
    "list")
        list_global_dependencies
        ;;
    "rebuild-base")
        rebuild_base_image
        ;;
    *)
        echo "Go AI POI Dependency Management Script"
        echo ""
        echo "Usage: $0 <command> [options]"
        echo ""
        echo "Commands:"
        echo "  add-global <package> [version]     Add a global dependency to shared module"
        echo "  remove-global <package>            Remove a global dependency from shared module"
        echo "  add-service <service> <package> [version]  Add dependency to specific service"
        echo "  update                             Update all dependencies"
        echo "  sync                               Sync workspace dependencies"
        echo "  list                               List global dependencies"
        echo "  rebuild-base                       Rebuild shared Docker base image"
        echo ""
        echo "Examples:"
        echo "  $0 add-global github.com/gin-gonic/gin v1.9.1"
        echo "  $0 add-service auth github.com/golang-jwt/jwt/v5"
        echo "  $0 list"
        echo "  $0 sync"
        ;;
esac