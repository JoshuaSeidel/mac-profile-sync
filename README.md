# Mac Profile Sync

A Go-based TUI application that keeps folders (Desktop, Documents, etc.) synchronized between two Macs in real-time using filesystem watching, with Bonjour auto-discovery and configurable conflict resolution.

## Features

- **Real-time sync** - Uses filesystem watching to detect and sync changes instantly
- **Bonjour/mDNS discovery** - Automatically discovers other Macs on the network
- **Beautiful TUI** - Interactive terminal interface for managing sync
- **Conflict resolution** - Choose between newest-wins, keep-both, or manual resolution
- **Secure** - TLS encryption for file transfers
- **Configurable** - YAML-based configuration with sensible defaults

## Installation

### Quick Install (Recommended)

```bash
curl -fsSL https://raw.githubusercontent.com/JoshuaSeidel/mac-profile-sync/main/install.sh | bash
```

### Install Specific Version

```bash
curl -fsSL https://raw.githubusercontent.com/JoshuaSeidel/mac-profile-sync/main/install.sh | bash -s -- v1.0.0
```

### Build from Source

```bash
git clone https://github.com/JoshuaSeidel/mac-profile-sync.git
cd mac-profile-sync
make build
```

## Usage

### Start the TUI Application

```bash
mac-profile-sync
```

### Run as Background Daemon

```bash
mac-profile-sync daemon
```

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

```
╭─────────────────── Mac Profile Sync ────────────────────╮
│                                                         │
│  Status: ● Connected                                    │
│  Peer: MacBook-Air (192.168.1.105)                     │
│                                                         │
│  ┌─────────────────────────────────────────────────┐   │
│  │ Synced Folders                           [a]dd  │   │
│  ├─────────────────────────────────────────────────┤   │
│  │ ✓ ~/Desktop                          12 files   │   │
│  │ ✓ ~/Documents                        847 files  │   │
│  │ ○ ~/Pictures                         disabled   │   │
│  └─────────────────────────────────────────────────┘   │
│                                                         │
│  Recent Activity:                                       │
│  → Synced report.docx                     2s ago       │
│  → Synced screenshot.png                  5s ago       │
│  ← Received budget.xlsx                   1m ago       │
│                                                         │
│  [d]ashboard [f]olders [s]ettings [q]uit              │
╰─────────────────────────────────────────────────────────╯
```

### Keyboard Shortcuts

| Key | Action |
|-----|--------|
| `d` | Dashboard view |
| `f` | Folder management |
| `c` | Conflict resolution |
| `s` | Settings |
| `q` | Quit |
| `↑/↓` | Navigate |
| `Enter` | Select/Toggle |
| `a` | Add folder |
| `x` | Remove folder |

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
  - path: ~/Pictures
    enabled: false

# Sync settings
sync:
  conflict_resolution: "newest_wins"  # newest_wins | keep_both | prompt
  ignore_patterns:
    - ".DS_Store"
    - "*.tmp"
    - ".git"
    - "node_modules"

# Network settings
network:
  port: 9876
  use_discovery: true
  manual_peers: []  # e.g., ["192.168.1.100:9876"]

# Security
security:
  require_pairing: true
  encryption: true
```

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
        <string>daemon</string>
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

## Network Requirements

- Both Macs must be on the same local network
- Port 9876 (default) must be accessible
- For Bonjour discovery: mDNS/Bonjour must be enabled (default on macOS)

## Troubleshooting

### Peers Not Discovering

1. Ensure both Macs are on the same network
2. Check firewall settings allow port 9876
3. Try using manual peers in config:
   ```yaml
   network:
     manual_peers:
       - "192.168.1.100:9876"
   ```

### Sync Not Working

1. Check logs: `~/.mac-profile-sync/stderr.log`
2. Run with verbose logging: `mac-profile-sync -v`
3. Verify folders exist and are accessible

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
