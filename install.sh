#!/bin/bash
#
# Mac Profile Sync Installer
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/JoshuaSeidel/mac-profile-sync/main/install.sh | bash
#
# Or with a specific version:
#   curl -fsSL https://raw.githubusercontent.com/JoshuaSeidel/mac-profile-sync/main/install.sh | bash -s -- v1.0.0
#

set -e

REPO="JoshuaSeidel/mac-profile-sync"
INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"
BINARY_NAME="mac-profile-sync"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

info() {
    echo -e "${BLUE}==>${NC} $1"
}

success() {
    echo -e "${GREEN}==>${NC} $1"
}

warn() {
    echo -e "${YELLOW}==>${NC} $1"
}

error() {
    echo -e "${RED}Error:${NC} $1"
    exit 1
}

# Detect OS and architecture
detect_platform() {
    OS=$(uname -s | tr '[:upper:]' '[:lower:]')
    ARCH=$(uname -m)

    case "$OS" in
        darwin)
            OS="darwin"
            ;;
        *)
            error "Unsupported operating system: $OS. Mac Profile Sync only supports macOS."
            ;;
    esac

    case "$ARCH" in
        x86_64|amd64)
            ARCH="amd64"
            ;;
        arm64|aarch64)
            ARCH="arm64"
            ;;
        *)
            error "Unsupported architecture: $ARCH"
            ;;
    esac

    PLATFORM="${OS}_${ARCH}"
    info "Detected platform: $PLATFORM"
}

# Get the version to install (always fetches latest unless specified)
get_version() {
    if [ -n "$1" ]; then
        VERSION="$1"
        info "Installing specified version: $VERSION"
    else
        info "Fetching latest version..."
        VERSION=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name"' | sed -E 's/.*"([^"]+)".*/\1/')
        if [ -z "$VERSION" ]; then
            error "Could not determine latest version. Please specify a version manually."
        fi
        info "Latest version: $VERSION"
    fi
}

# Download and install
install_binary() {
    DOWNLOAD_URL="https://github.com/${REPO}/releases/download/${VERSION}/${BINARY_NAME}-${VERSION}-${PLATFORM}.tar.gz"

    info "Downloading from: $DOWNLOAD_URL"

    # Create temp directory
    TMP_DIR=$(mktemp -d)
    trap "rm -rf $TMP_DIR" EXIT

    # Download
    if ! curl -fsSL "$DOWNLOAD_URL" -o "$TMP_DIR/archive.tar.gz"; then
        error "Failed to download $DOWNLOAD_URL"
    fi

    # Extract
    info "Extracting..."
    tar -xzf "$TMP_DIR/archive.tar.gz" -C "$TMP_DIR"

    # Find the binary
    BINARY_PATH=$(find "$TMP_DIR" -name "${BINARY_NAME}*" -type f -perm +111 2>/dev/null | head -1)
    if [ -z "$BINARY_PATH" ]; then
        BINARY_PATH="$TMP_DIR/${BINARY_NAME}-${PLATFORM}"
    fi

    if [ ! -f "$BINARY_PATH" ]; then
        error "Binary not found in archive"
    fi

    # Install
    info "Installing to $INSTALL_DIR..."

    # Check if we need sudo
    if [ -w "$INSTALL_DIR" ]; then
        mv "$BINARY_PATH" "$INSTALL_DIR/$BINARY_NAME"
        chmod +x "$INSTALL_DIR/$BINARY_NAME"
    else
        warn "Requesting sudo access to install to $INSTALL_DIR"
        sudo mv "$BINARY_PATH" "$INSTALL_DIR/$BINARY_NAME"
        sudo chmod +x "$INSTALL_DIR/$BINARY_NAME"
    fi

    success "Installed $BINARY_NAME to $INSTALL_DIR/$BINARY_NAME"
}

# Create default config
setup_config() {
    CONFIG_DIR="$HOME/.mac-profile-sync"

    if [ ! -d "$CONFIG_DIR" ]; then
        info "Creating config directory at $CONFIG_DIR"
        mkdir -p "$CONFIG_DIR"
    fi

    if [ ! -f "$CONFIG_DIR/config.yaml" ]; then
        info "Creating default configuration..."
        HOSTNAME_VAL=$(hostname -s 2>/dev/null || echo "My-Mac")
        cat > "$CONFIG_DIR/config.yaml" << EOF
# Mac Profile Sync Configuration

# Device identification
device:
  name: "$HOSTNAME_VAL"

# Folders to sync
folders:
  - path: ~/Desktop
    enabled: true
  - path: ~/Documents
    enabled: true

# Sync settings
sync:
  conflict_resolution: "newest_wins"  # newest_wins | keep_both | prompt
  ignore_patterns:
    - ".DS_Store"
    - "*.tmp"
    - ".git"
    - "node_modules"
    - ".Trash"

# Network settings
network:
  port: 9876
  use_discovery: true
  manual_peers: []

# Security
security:
  require_pairing: true
  encryption: true
EOF

        success "Created default configuration at $CONFIG_DIR/config.yaml"
    else
        info "Configuration already exists at $CONFIG_DIR/config.yaml"
    fi
}

# Create launchd plist for auto-start
setup_launchd() {
    PLIST_DIR="$HOME/Library/LaunchAgents"
    PLIST_FILE="$PLIST_DIR/com.github.joshuaseidel.mac-profile-sync.plist"

    echo ""
    read -p "Would you like to set up auto-start on login? [y/N] " -n 1 -r
    echo ""

    if [[ $REPLY =~ ^[Yy]$ ]]; then
        mkdir -p "$PLIST_DIR"

        cat > "$PLIST_FILE" << EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.github.joshuaseidel.mac-profile-sync</string>
    <key>ProgramArguments</key>
    <array>
        <string>$INSTALL_DIR/$BINARY_NAME</string>
        <string>daemon</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
    <key>StandardOutPath</key>
    <string>$HOME/.mac-profile-sync/stdout.log</string>
    <key>StandardErrorPath</key>
    <string>$HOME/.mac-profile-sync/stderr.log</string>
</dict>
</plist>
EOF

        success "Created launchd plist at $PLIST_FILE"
        info "To start the daemon now, run:"
        echo "    launchctl load $PLIST_FILE"
        echo ""
        info "To stop the daemon, run:"
        echo "    launchctl unload $PLIST_FILE"
    fi
}

# Print next steps
print_next_steps() {
    echo ""
    echo "╔═══════════════════════════════════════════════════════════════╗"
    echo "║                   Installation Complete!                       ║"
    echo "╚═══════════════════════════════════════════════════════════════╝"
    echo ""
    echo -e "${GREEN}Next Steps:${NC}"
    echo ""
    echo -e "${YELLOW}1. Configure your sync settings:${NC}"
    echo "   Edit ~/.mac-profile-sync/config.yaml to customize:"
    echo "   - Device name (how this Mac appears to peers)"
    echo "   - Folders to sync (Desktop, Documents, etc.)"
    echo "   - Conflict resolution strategy"
    echo ""
    echo -e "${YELLOW}2. Start syncing:${NC}"
    echo "   Option A - Interactive TUI:"
    echo "     ${BLUE}mac-profile-sync${NC}"
    echo ""
    echo "   Option B - Background daemon:"
    echo "     ${BLUE}mac-profile-sync daemon${NC}"
    echo ""
    echo -e "${YELLOW}3. On your other Mac:${NC}"
    echo "   Run the same installer and start the app."
    echo "   Both Macs will auto-discover each other via Bonjour."
    echo ""
    echo -e "${YELLOW}4. To add a peer manually:${NC}"
    echo "   In the TUI, press 3 (Peers), then 'a' to add."
    echo "   Enter the other Mac's IP:port (e.g., 192.168.1.100:9876)"
    echo ""
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo ""
    echo -e "${GREEN}Quick Reference:${NC}"
    echo "  mac-profile-sync              Launch interactive TUI"
    echo "  mac-profile-sync daemon       Run as background service"
    echo "  mac-profile-sync status       Show current sync status"
    echo "  mac-profile-sync add ~/path   Add a folder to sync"
    echo "  mac-profile-sync peers        List discovered peers"
    echo ""
    echo -e "${GREEN}TUI Navigation:${NC}"
    echo "  Tab / Shift+Tab   Switch between views"
    echo "  1-4               Jump to view (Dashboard/Folders/Peers/Settings)"
    echo "  ↑↓                Navigate within view"
    echo "  q                 Quit"
    echo ""
    echo -e "${GREEN}Configuration:${NC} ~/.mac-profile-sync/config.yaml"
    echo -e "${GREEN}Logs:${NC}          ~/.mac-profile-sync/stderr.log"
    echo ""
    echo "For more help: mac-profile-sync --help"
    echo ""
}

# Main
main() {
    echo ""
    echo "╔═══════════════════════════════════════╗"
    echo "║       Mac Profile Sync Installer      ║"
    echo "╚═══════════════════════════════════════╝"
    echo ""

    detect_platform
    get_version "$1"
    install_binary
    setup_config
    setup_launchd
    print_next_steps
}

main "$1"
