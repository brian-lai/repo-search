#!/bin/bash
#
# codetect update script
# Updates codetect to the latest version from GitHub
#

set -e

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
NC='\033[0m'

# Installation paths
INSTALL_PREFIX="${CODETECT_PREFIX:-$HOME/.local}"
BIN_DIR="$INSTALL_PREFIX/bin"
SHARE_DIR="$INSTALL_PREFIX/share/codetect"

# Source repo location (where codetect was cloned)
SOURCE_DIR="${CODETECT_SOURCE:-$HOME/dev/codetect}"

success() {
    echo -e "${GREEN}✓${NC} $1"
}

warn() {
    echo -e "${YELLOW}!${NC} $1"
}

error() {
    echo -e "${RED}✗${NC} $1"
}

info() {
    echo -e "  $1"
}

echo -e "${CYAN}Updating codetect...${NC}"
echo ""

# Check if source directory exists
if [[ ! -d "$SOURCE_DIR" ]]; then
    error "Source directory not found: $SOURCE_DIR"
    info "Set CODETECT_SOURCE to the location of your codetect clone"
    info "Or clone it:"
    info "  git clone https://github.com/brian-lai/codetect.git $SOURCE_DIR"
    exit 1
fi

# Change to source directory
cd "$SOURCE_DIR"

# Check if it's a git repo
if [[ ! -d ".git" ]]; then
    error "Not a git repository: $SOURCE_DIR"
    exit 1
fi

# Get current version
OLD_COMMIT=$(git rev-parse --short HEAD)

# Pull latest
echo "Fetching latest changes..."
git fetch origin --tags

# Find latest version tag
LATEST_VERSION=$(git tag -l 'v*' | sort -V | tail -n1)

if [[ -z "$LATEST_VERSION" ]]; then
    error "No version tags found"
    exit 1
fi

# Check if we're already on the latest version
CURRENT_TAG=$(git describe --tags --exact-match 2>/dev/null || echo "")
if [[ "$CURRENT_TAG" == "$LATEST_VERSION" ]]; then
    success "Already on latest version: $LATEST_VERSION"
    exit 0
fi

# Checkout latest version tag
echo "Updating to $LATEST_VERSION..."
git checkout "$LATEST_VERSION"
NEW_COMMIT=$(git rev-parse --short HEAD)

success "Updated: $OLD_COMMIT → $NEW_COMMIT"
echo ""

# Show what changed
echo "Changes:"
git log --oneline "$OLD_COMMIT".."$NEW_COMMIT" | head -10
echo ""

# Build
echo "Building..."
make build
success "Build complete"
echo ""

# Install binaries
echo "Installing to $BIN_DIR..."
mkdir -p "$BIN_DIR" "$SHARE_DIR/templates"

cp dist/codetect "$BIN_DIR/codetect-mcp"
cp dist/codetect-index "$BIN_DIR/codetect-index"
cp scripts/codetect-wrapper.sh "$BIN_DIR/codetect"
chmod +x "$BIN_DIR/codetect" "$BIN_DIR/codetect-mcp" "$BIN_DIR/codetect-index"

# Install templates if they exist
if [[ -d "templates" ]]; then
    cp templates/* "$SHARE_DIR/templates/" 2>/dev/null || true
fi

success "Installed"
echo ""

echo -e "${GREEN}Update complete!${NC}"
echo ""
echo "Run 'codetect doctor' to verify the installation."
