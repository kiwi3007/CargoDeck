package api

import (
	"bytes"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"text/template"
	"time"
)

var validPlatforms = map[string]string{
	"linux-x64":   "playerr-agent",
	"linux-arm64": "playerr-agent",
	"win-x64":     "playerr-agent.exe",
	"osx-x64":     "playerr-agent",
	"osx-arm64":   "playerr-agent",
}

// ServeAgentBinary streams the agent binary for the requested platform.
// GET /api/v3/agent/binary?os={linux-x64|linux-arm64|win-x64|osx-x64|osx-arm64}
// No auth required — binary is not sensitive.
func (h *Handler) ServeAgentBinary(w http.ResponseWriter, r *http.Request) {
	platform := r.URL.Query().Get("os")
	binName, ok := validPlatforms[platform]
	if !ok {
		jsonErr(w, 400, "invalid os; use: linux-x64, linux-arm64, win-x64, osx-x64, osx-arm64")
		return
	}

	path := h.findAgentBinary(platform, binName)
	if path == "" {
		jsonErr(w, 404, fmt.Sprintf("agent binary for %s not found — run build_go.sh to generate binaries", platform))
		return
	}

	f, err := os.Open(path)
	if err != nil {
		jsonErr(w, 404, "binary not found")
		return
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		jsonErr(w, 500, "stat failed")
		return
	}

	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", binName))
	w.Header().Set("Content-Type", "application/octet-stream")
	http.ServeContent(w, r, binName, fi.ModTime(), f)
}

// AgentVersion returns the version string of the hosted agent binary.
// GET /api/v3/agent/version
// Agents call this on startup to determine if they need to self-update.
func (h *Handler) AgentVersion(w http.ResponseWriter, r *http.Request) {
	jsonOK(w, map[string]string{"version": h.agentVersion()})
}

// agentVersion reads the version.txt sidecar written by build_go.sh / Dockerfile.
// Falls back to "dev" if the file is not found.
func (h *Handler) agentVersion() string {
	contentRoot := filepath.Dir(h.cfg.Dir())
	candidates := []string{
		filepath.Join(contentRoot, "_output", "version.txt"),
		filepath.Join("/app/agents", "version.txt"),
		filepath.Join(filepath.Dir(contentRoot), "agents", "version.txt"),
	}
	for _, p := range candidates {
		if data, err := os.ReadFile(p); err == nil {
			if v := strings.TrimSpace(string(data)); v != "" {
				return v
			}
		}
	}
	return "dev"
}

// findAgentBinary searches for the agent binary in known locations.
func (h *Handler) findAgentBinary(platform, binName string) string {
	// Derive content root from config dir (cfg.Dir() = .../config/)
	contentRoot := filepath.Dir(h.cfg.Dir())

	candidates := []string{
		// Dev / local builds
		filepath.Join(contentRoot, "_output", platform, binName),
		// Docker image path (set by Dockerfile)
		filepath.Join("/app/agents", platform, binName),
		// Alongside the server binary
		filepath.Join(filepath.Dir(contentRoot), "agents", platform, binName),
	}

	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// ---- Setup script ----

const agentSetupTmpl = `#!/usr/bin/env bash
# Playerr Agent Setup Script
# Generated: {{.GeneratedAt}}
# Server:    {{.ServerURL}}
#
# Usage:  bash setup-agent.sh [device-name]
#   or:   curl -fsSL '{{.ServerURL}}/api/v3/agent/setup.sh' | bash
#
set -euo pipefail

PLAYERR_SERVER={{.ServerURLQuoted}}
PLAYERR_TOKEN={{.TokenQuoted}}
PLAYERR_NAME="${1:-$(hostname 2>/dev/null || echo playerr-device)}"

echo ""
echo "╔══════════════════════════════════════╗"
echo "║      Playerr Agent Setup             ║"
echo "╚══════════════════════════════════════╝"
echo " Server:  ${PLAYERR_SERVER}"
echo " Device:  ${PLAYERR_NAME}"
echo ""

# ── Detect OS / architecture ──────────────────────────────────────────────────
OS_TYPE="$(uname -s 2>/dev/null || echo Linux)"
ARCH="$(uname -m 2>/dev/null || echo x86_64)"

case "${ARCH}" in
  x86_64|amd64)   ARCH_LABEL="x64"  ;;
  aarch64|arm64)  ARCH_LABEL="arm64" ;;
  *)  echo "Unsupported architecture: ${ARCH}"; exit 1 ;;
esac

case "${OS_TYPE}" in
  Linux)  PLATFORM="linux-${ARCH_LABEL}"; BIN_EXT="" ;;
  Darwin) PLATFORM="osx-${ARCH_LABEL}";   BIN_EXT="" ;;
  MINGW*|CYGWIN*|MSYS*) PLATFORM="win-x64"; BIN_EXT=".exe" ;;
  *)  echo "Unsupported OS: ${OS_TYPE}"; exit 1 ;;
esac

INSTALL_DIR="${HOME}/.config/playerr-agent"
BINARY="${INSTALL_DIR}/playerr-agent${BIN_EXT}"

echo "[1/4] Creating install directory: ${INSTALL_DIR}"
mkdir -p "${INSTALL_DIR}"

# ── Download binary ───────────────────────────────────────────────────────────
echo "[2/4] Downloading agent binary (${PLATFORM})..."
if command -v curl &>/dev/null; then
  curl -fL --progress-bar \
    "${PLAYERR_SERVER}/api/v3/agent/binary?os=${PLATFORM}" \
    -o "${BINARY}"
elif command -v wget &>/dev/null; then
  wget -q --show-progress \
    "${PLAYERR_SERVER}/api/v3/agent/binary?os=${PLATFORM}" \
    -O "${BINARY}"
else
  echo "Error: curl or wget is required"; exit 1
fi
chmod +x "${BINARY}"
echo "      ✓ $(du -sh "${BINARY}" 2>/dev/null | cut -f1 || echo "ok")"

# ── Quick connectivity test ───────────────────────────────────────────────────
echo "[3/4] Testing connection to server..."
if "${BINARY}" --server "${PLAYERR_SERVER}" --token "${PLAYERR_TOKEN}" --name "${PLAYERR_NAME}" --test-connection 2>/dev/null; then
  echo "      ✓ Connected"
else
  echo "      ⚠ Could not verify connection (agent will retry automatically)"
fi

# ── Install service ───────────────────────────────────────────────────────────
echo "[4/4] Installing autostart service..."

if [ "${OS_TYPE}" = "Linux" ] && command -v systemctl &>/dev/null; then
  # systemd user service
  SVCDIR="${HOME}/.config/systemd/user"
  mkdir -p "${SVCDIR}"

  cat > "${SVCDIR}/playerr-agent.service" << SVCEOF
[Unit]
Description=Playerr Agent
After=network-online.target
Wants=network-online.target

[Service]
ExecStart=${BINARY} --server ${PLAYERR_SERVER} --token ${PLAYERR_TOKEN} --name ${PLAYERR_NAME}
Restart=on-failure
RestartSec=15
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=default.target
SVCEOF

  systemctl --user daemon-reload
  systemctl --user enable playerr-agent
  systemctl --user restart playerr-agent 2>/dev/null || systemctl --user start playerr-agent

  # Enable linger so the service survives logout / reboot without login
  loginctl enable-linger "$(id -un)" 2>/dev/null \
    || echo "      Note: run 'sudo loginctl enable-linger $(id -un)' for autostart on boot"

  sleep 2
  echo ""
  echo "── Service Status ─────────────────────────────────────────────────────"
  systemctl --user status playerr-agent --no-pager -l || true

elif [ "${OS_TYPE}" = "Darwin" ]; then
  # launchd user agent
  PLIST="${HOME}/Library/LaunchAgents/com.playerr.agent.plist"
  mkdir -p "${HOME}/Library/LaunchAgents"

  cat > "${PLIST}" << PLISTEOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key><string>com.playerr.agent</string>
  <key>ProgramArguments</key>
  <array>
    <string>${BINARY}</string>
    <string>--server</string><string>${PLAYERR_SERVER}</string>
    <string>--token</string><string>${PLAYERR_TOKEN}</string>
    <string>--name</string><string>${PLAYERR_NAME}</string>
  </array>
  <key>RunAtLoad</key><true/>
  <key>KeepAlive</key><true/>
  <key>StandardOutPath</key><string>${INSTALL_DIR}/agent.log</string>
  <key>StandardErrorPath</key><string>${INSTALL_DIR}/agent.log</string>
</dict>
</plist>
PLISTEOF

  launchctl unload "${PLIST}" 2>/dev/null || true
  launchctl load -w "${PLIST}"
  echo "      ✓ Loaded as launchd agent (auto-restarts on crash/reboot)"

else
  # Fallback: write a run script
  cat > "${INSTALL_DIR}/run.sh" << RUNEOF
#!/bin/bash
exec "${BINARY}" --server "${PLAYERR_SERVER}" --token "${PLAYERR_TOKEN}" --name "${PLAYERR_NAME}"
RUNEOF
  chmod +x "${INSTALL_DIR}/run.sh"
  echo "      ⚠ No systemd/launchd found."
  echo "        Run manually: ${INSTALL_DIR}/run.sh"
fi

echo ""
echo "══════════════════════════════════════════════════════════════════════"
echo "  ✓ Playerr Agent installed!"
echo "    Binary:  ${BINARY}"
echo "    Server:  ${PLAYERR_SERVER}"
if [ "${OS_TYPE}" = "Linux" ] && command -v systemctl &>/dev/null; then
  echo "    Logs:    journalctl --user -u playerr-agent -f"
  echo "    Manage:  systemctl --user {status|stop|restart} playerr-agent"
elif [ "${OS_TYPE}" = "Darwin" ]; then
  echo "    Logs:    tail -f ${INSTALL_DIR}/agent.log"
fi
echo "══════════════════════════════════════════════════════════════════════"
echo ""
`

type setupScriptData struct {
	ServerURL       string
	ServerURLQuoted string
	TokenQuoted     string
	GeneratedAt     string
}

// ServeAgentSetupScript generates a one-shot setup script with the server URL
// and token pre-embedded.
// GET /api/v3/agent/setup.sh
// No auth required — the script itself contains the token, accessible only
// to someone who already has the settings page open.
func (h *Handler) ServeAgentSetupScript(w http.ResponseWriter, r *http.Request) {
	cfg := h.cfg.LoadAgent()
	serverURL := resolveServerURL(r)

	data := setupScriptData{
		ServerURL:       serverURL,
		ServerURLQuoted: shellQuote(serverURL),
		TokenQuoted:     shellQuote(cfg.Token),
		GeneratedAt:     time.Now().UTC().Format(time.RFC3339),
	}

	tmpl, err := template.New("setup").Parse(agentSetupTmpl)
	if err != nil {
		jsonErr(w, 500, "template error")
		return
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		jsonErr(w, 500, "render error")
		return
	}

	w.Header().Set("Content-Type", "text/x-sh; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="setup-playerr-agent.sh"`)
	w.WriteHeader(http.StatusOK)
	w.Write(buf.Bytes())
}
