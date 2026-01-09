#!/bin/bash
#
# repo-search update script
# Updates repo-search to the latest version from GitHub
#

set -e

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
NC='\033[0m'

# Installation paths
INSTALL_PREFIX="${REPO_SEARCH_PREFIX:-$HOME/.local}"
BIN_DIR="$INSTALL_PREFIX/bin"
SHARE_DIR="$INSTALL_PREFIX/share/repo-search"

# Source repo location (where repo-search was cloned)
SOURCE_DIR="${REPO_SEARCH_SOURCE:-$HOME/dev/repo-search}"

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

echo -e "${CYAN}Updating repo-search...${NC}"
echo ""

# Check if source directory exists
if [[ ! -d "$SOURCE_DIR" ]]; then
    error "Source directory not found: $SOURCE_DIR"
    info "Set REPO_SEARCH_SOURCE to the location of your repo-search clone"
    info "Or clone it:"
    info "  git clone https://github.com/brian-lai/repo-search.git $SOURCE_DIR"
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
git fetch origin main

# Check if there are updates
LOCAL=$(git rev-parse HEAD)
REMOTE=$(git rev-parse origin/main)

if [[ "$LOCAL" == "$REMOTE" ]]; then
    success "Already up to date ($OLD_COMMIT)"
    exit 0
fi

# Pull changes
git pull origin main
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

cp dist/repo-search "$BIN_DIR/repo-search-mcp"
cp dist/repo-search-index "$BIN_DIR/repo-search-index"
cp scripts/repo-search-wrapper.sh "$BIN_DIR/repo-search"
chmod +x "$BIN_DIR/repo-search" "$BIN_DIR/repo-search-mcp" "$BIN_DIR/repo-search-index"

# Install templates if they exist
if [[ -d "templates" ]]; then
    cp templates/* "$SHARE_DIR/templates/" 2>/dev/null || true
fi

success "Installed"
echo ""

echo -e "${GREEN}Update complete!${NC}"
echo ""
echo "Run 'repo-search doctor' to verify the installation."
