# Mac Profile Sync

A Go-based application that keeps folders (Desktop, Documents, etc.) synchronized between two Macs in real-time using filesystem watching, with Bonjour auto-discovery and configurable conflict resolution.

## Features

- **Real-time sync** - Uses filesystem watching to detect and sync changes instantly
- **Bonjour/mDNS discovery** - Automatically discovers other Macs on the network
- **Interactive TUI** - Terminal interface for configuration and control
- **Conflict resolution** - Choose between newest-wins, keep-both, or manual resolution
- **Sync direction** - Bidirectional, send-only, or receive-only modes
- **Exclude directories** - Prevent specific directories from syncing
- **Secure** - TLS encryption for file transfers
- **Configurable** - YAML-based configuration with sensible defaults

## Installation

### Quick Install (Recommended)

```bash
curl -fsSL https://raw.githubusercontent.com/JoshuaSeidel/mac-profile-sync/main/install.sh | bash
```

### Install Specific Version

```bash
curl -fsSL https://raw.githubusercontent.com/JoshuaSeidel/mac-profile-sync/main/install.sh | bash -s -- v1.2.0
```

### Build from Source

```bash
git clone https://github.com/JoshuaSeidel/mac-profile-sync.git
cd mac-profile-sync
make build
```

## Quick Start

1. **Configure using the TUI:**
   ```bash
   mac-profile-sync tui
   ```

2. **In the TUI:**
   - Press `2` to go to Folders view
   - Press `a` to add folders to sync
   - Press `e` to exclude directories
   - Press `1` to go to Dashboard
   - Press `s` to start sync

3. **Run the daemon:**
   ```bash
   mac-profile-sync
   ```

4. **Repeat on your other Mac** - they will auto-discover each other via Bonjour.

## Usage

### Run Daemon (Default)

```bash
mac-profile-sync
```

Runs the sync daemon. **Note:** Sync must be enabled first using the TUI. If sync is disabled, the daemon will exit immediately.

### Launch TUI for Configuration

```bash
mac-profile-sync tui
```

Opens the interactive TUI for:
- Adding/removing sync folders
- Excluding directories
- Managing peers
- Changing settings
- Starting/stopping sync

### Other Commands

```bash
# Check sync status
mac-profile-sync status

# Add a folder to sync
mac-profile-sync add ~/Projects

# Remove a folder from sync
mac-profile-sync remove ~/Projects

# List discovered peers
mac-profile-sync peers

# Show version
mac-profile-sync version
```

## TUI Interface

### Navigation

| Key | Action |
|-----|--------|
| `Tab` / `Shift+Tab` | Switch between views |
| `1` | Dashboard view |
| `2` | Folder management |
| `3` | Peers view |
| `4` | Settings |
| `q` | Quit |
| `↑/↓` | Navigate within view |

### Dashboard View

| Key | Action |
|-----|--------|
| `s` | Start/stop sync |

### Folders View

| Key | Action |
|-----|--------|
| `a` | Add sync folder |
| `e` | Exclude directory |
| `Enter/Space` | Toggle folder sync |
| `x` | Remove folder/exclusion |

### Peers View

| Key | Action |
|-----|--------|
| `a` | Add manual peer |
| `x` | Remove peer |

## Syncing Between Different Usernames

Mac Profile Sync supports syncing between Macs with **different usernames**. For example:
- Mac 1: `/Users/john/Desktop`
- Mac 2: `/Users/johnny/Desktop`

The app uses folder names (like "Desktop", "Documents") to match folders between machines, not the full path. This allows seamless syncing even when usernames differ.

### Home Directory Syncing

You can sync your entire home directory by adding `~` or your home path:

```bash
# In TUI, press 'a' in Folders view and enter:
~
```

When syncing home directories, the following are **ignored by default** to prevent issues:
- `Library` - macOS system/application data
- `Applications` - Application bundles
- `.Trash` - Trash folder
- `.cache`, `.local`, `.config` - Application caches
- `Caches`, `CachedData`, `Cache` - Various cache folders
- Node/Rust/Go/Python package managers (`.npm`, `.cargo`, `.rustup`, etc.)
- IDE state (`.vscode`, `.idea`)
- Build artifacts (`build`, `dist`, `target`, `node_modules`)

Add additional exclusions in the TUI (press `e` in Folders view) or edit `~/.mac-profile-sync/config.yaml`.

## Configuration

Configuration is stored at `~/.mac-profile-sync/config.yaml`:

```yaml
# Device identification
device:
  name: "MacBook-Pro"

# Folders to sync
folders:
  - path: ~/Desktop
    enabled: true
  - path: ~/Documents
    enabled: true

# Sync settings
sync:
  enabled: false                          # Must be enabled via TUI
  direction: "bidirectional"              # bidirectional | send_only | receive_only
  conflict_resolution: "newest_wins"      # newest_wins | keep_both | prompt
  ignore_patterns:
    - ".DS_Store"
    - "*.tmp"
    - ".git"
    - "node_modules"
    - ".Trash"
  exclude_dirs: []                        # e.g., ["~/Documents/Private"]

# Network settings
network:
  port: 9876
  use_discovery: true
  manual_peers: []                        # e.g., ["192.168.1.100:9876"]

# Security
security:
  require_pairing: true
  encryption: true
```

### Sync Direction Modes

| Mode | Description |
|------|-------------|
| `bidirectional` | Sync files both ways (default) |
| `send_only` | Only send files to peers, never receive |
| `receive_only` | Only receive files from peers, never send |

### Conflict Resolution Strategies

| Strategy | Description |
|----------|-------------|
| `newest_wins` | Automatically keep the most recently modified version |
| `keep_both` | Keep both versions, renaming the local file |
| `prompt` | Show a TUI prompt to manually resolve each conflict |

## Auto-Start on Login

The installer can optionally set up auto-start. To manually configure:

```bash
# Create launchd plist
cat > ~/Library/LaunchAgents/com.github.joshuaseidel.mac-profile-sync.plist << EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.github.joshuaseidel.mac-profile-sync</string>
    <key>ProgramArguments</key>
    <array>
        <string>/usr/local/bin/mac-profile-sync</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
</dict>
</plist>
EOF

# Load the daemon
launchctl load ~/Library/LaunchAgents/com.github.joshuaseidel.mac-profile-sync.plist

# Unload the daemon
launchctl unload ~/Library/LaunchAgents/com.github.joshuaseidel.mac-profile-sync.plist
```

**Note:** Remember to enable sync using `mac-profile-sync tui` before setting up auto-start.

## Network Requirements

- Both Macs must be on the same local network
- Port 9876 (default) must be accessible
- For Bonjour discovery: mDNS/Bonjour must be enabled (default on macOS)

## Troubleshooting

### Sync Not Starting

1. Ensure sync is enabled: Run `mac-profile-sync tui`, press `s` on Dashboard
2. Check that at least one folder is configured and enabled

### Peers Not Discovering

1. Ensure both Macs are on the same network
2. Check firewall settings allow port 9876
3. Try using manual peers in TUI (press `3`, then `a`)

### Files Not Syncing

1. Check logs: `~/.mac-profile-sync/stderr.log`
2. Run with verbose logging: `mac-profile-sync -v`
3. Verify folders exist and are accessible
4. Check if the path is in exclude_dirs

## Development

```bash
# Build
make build

# Run
make run

# Test
make test

# Format code
make fmt

# Lint
make lint
```

## License

MIT License - see LICENSE file for details.
