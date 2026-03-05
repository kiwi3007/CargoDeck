# Playerr v0.4.9
- **New Feature**: Remote install via agent binary (playerr-agent). Run the agent on a remote device (e.g. Steam Deck) and install games to it directly from the Playerr web UI.
- **New Feature**: Install jobs are pushed via SSE (no polling). The agent is battery-friendly: only one goroutine runs at idle.
- **New Feature**: File serving with HTTP Range support for resumable downloads from agent.
- **New Feature**: Bash install script fallback (Settings -> Agents -> Download Script) for devices without the agent binary.
- **New Feature**: Automatic Proton detection on Steam Deck (GE-Proton preferred, official Proton fallback).
- **New Feature**: Agents tab in Settings showing connected devices, masked auth token, and download link.
- **New Feature**: Real-time install progress bar in game details view.
- **Improved**: Proton/Wine launcher logic extracted into shared package (used by both server and agent).
- **Security**: Agent endpoints protected by auto-generated Bearer token stored in config/agent.json.

# Playerr v0.4.8
- **New Feature**: Support for **Flood** as a dedicated download client.
- **Improved**: rTorrent compatibility fix (XML-RPC commands).
- **Improved**: Scanner logic for Scene releases and better title cleaning.
- **Improved**: Torrent search result persistence (no more losing results when going back).
- **New Feature**: Version selector for games with multiple installers.
- **UI**: Dedicated "Clients" tab in Settings.
- **UI**: Added URL Base suggestion for rTorrent.

# Playerr v0.4.7
- **New Feature**: Custom IP/Port Configuration (Settings -> Advanced).
- **New Feature**: Allow Remote Access (Bind to 0.0.0.0).
- **New Feature**: Windows System Tray support (Minimize to Tray).
- **UI**: Added "Advanced" settings tab.
- **Fix**: Resolved issue with "blank window" when using custom ports or remote access in standalone mode.
- **Refactor**: Re-architected translation system for stability (Special thanks to **Gabi** for the architecture advice).

