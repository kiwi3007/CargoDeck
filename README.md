# CargoDeck

**Self-hosted game library manager & remote installer.**
Designed to make installing non-Steam games to the Steam Deck as painless as possible

[![License: GPL v3](https://img.shields.io/badge/License-GPLv3-blue.svg?style=for-the-badge)](https://www.gnu.org/licenses/gpl-3.0)
[![Docker](https://img.shields.io/badge/Docker-amd64%20%2F%20arm64-2496ed?style=for-the-badge&logo=docker)](https://github.com/kiwi3007/CargoDeck/pkgs/container/cargodeck)

---

## AI Disclaimer

This project was initially forked from [Maikboarder/Playerr](https://github.com/Maikboarder/Playerr) and all further development has been with heavy AI assistance. I am making this public because it has been useful to me and could be useful to others. It would my advice that this application is not exposed to the internet.

## What is CargoDeck?

CargoDeck is a self-hosted game library manager focused on manual download of games. It connects to your indexers and download clients, automatically processes completed downloads, and can push games directly to remote devices such as the Steam Deck over the network.

---

## Features

### Indexer Search
Search Prowlarr from a single interface and send directly to your download client of choice.

### Game Library
Add games my scanning your games directory or manually searching for games from IGDB.

### Automatic Processing
Automatically scans, extracts, and imports game files from your download clients.

### Download Clients
Native integration with qBittorrent, Transmission, Deluge, Flood, rTorrent, SABnzbd, and NZBGet.

### Remote Devices & Agents
Install and manage games on remote machines over the network via lightweight agents. Supports Steam Deck, HTPCs, and any Linux or Windows machine. Agents connect back to the server and receive install jobs automatically.

### Wine & Proton
Auto-detects the best available runner on each device (Steam Proton, system Wine). Generates launch scripts, applies crack files, and adds Steam shortcuts automatically.

### Save Sync
Watches game save directories in real-time using inotify. Syncs snapshots to the server on file change or game exit. Retains the last 10 snapshots per device per game, with conflict detection and restore-suppression to avoid false positives.

### Update Tracking
Monitors indexers for newer versions of games already in your library. Parses version numbers from release names, PE binaries, GOG info files, and engine-specific metadata.

---

## Installation

### Docker (Recommended)

```yaml
services:
  cargodeck:
    image: ghcr.io/kiwi3007/cargodeck:latest
    container_name: cargodeck
    restart: unless-stopped
    ports:
      - "2727:2727"
    volumes:
      - ./config:/config
      - /your/games:/media/games
      - /your/downloads:/media/downloads
    environment:
      - CARGODECK_PORT=2727
      - CARGODECK_IP=0.0.0.0
      - CARGODECK_CONFIG_DIR=/config
```

Run with:
```bash
docker compose up -d
```

Access the UI at `http://your-ip:2727`.

### Build from Source

**Requirements:** Go 1.24+, Node.js 18+

```bash
git clone https://github.com/kiwi3007/CargoDeck.git
cd CargoDeck

# Build frontend
npm install && npm run build

# Build backend (Linux)
go build -o _output/linux-x64/cargodeck .

# Or use the build script for all platforms
./build_go.sh
```

---

## Agent Setup

Agents are lightweight binaries that run on remote devices and receive install jobs from the server.
### Linux / Steam Deck / macOS

Go to **Settings → Agents**, generate a one-time token (valid 15 minutes), and copy the command shown in the client.

Run the installer in your terminal on your remote device. To make this easier on Steam Deck, you can visit the UI in the browser and copy the command from there.

This will:
- Auto-detect the OS and architecture
- Download the correct agent binary to `~/.config/playerr-agent/`
- Install and start a **systemd user service** (Linux) or **launchd agent** (macOS) so the agent restarts automatically on reboot

To check status on Linux:
```bash
systemctl --user status playerr-agent
journalctl --user -u playerr-agent -f
```

### Windows

Windows does not support the one-liner. Instead:

1. In the UI go to **Settings → Agents** and download the `win-x64` agent binary
2. Move `playerr-agent.exe` somewhere permanent, e.g. `C:\playerr-agent\`
3. Open Command Prompt and run:

```cmd
playerr-agent.exe --server http://your-server:2727 --token YOUR_TOKEN --name my-pc
```

To run automatically on login, create a scheduled task:

```powershell
$action = New-ScheduledTaskAction -Execute "C:\playerr-agent\playerr-agent.exe" `
  -Argument "--server http://your-server:2727 --token YOUR_TOKEN --name my-pc"
$trigger = New-ScheduledTaskTrigger -AtLogOn
Register-ScheduledTask -TaskName "CargoDeck Agent" -Action $action -Trigger $trigger -RunLevel Highest
```


The agent registers itself on first run, stores a session token, and reconnects automatically if the server restarts.

---

## Configuration

Config files are stored in the config directory (default: `./config/`). Override with the `CARGODECK_CONFIG_DIR` environment variable.

| File | Purpose |
|------|---------|
| `appsettings.json` | Server port and bind address |
| `igdb.json` | IGDB API credentials (required for metadata) |
| `hydra.json` | Indexer configuration |
| `prowlarr.json` / `jackett.json` | Prowlarr / Jackett connection |
| `media.json` | Library, download, and destination paths |
| `playerr.db` | SQLite database |

---

## License

Released under the GNU General Public License v3.0. See `LICENSE` for details.

See `DISCLAIMER.md` for the legal notice regarding third-party content and user responsibility.

---

*Forked from [Maikboarder/Playerr](https://github.com/Maikboarder/Playerr). Developed with the assistance of AI.*
