package api

import (
	"bytes"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"text/template"
	"time"
	"unicode"
)

const installScriptTmpl = `#!/usr/bin/env bash
# Playerr Remote Install Script
# Game:      {{.GameTitle}}
# Generated: {{.GeneratedAt}}
# Server:    {{.ServerURL}}
set -euo pipefail

GAME_ID={{.GameID}}
GAME_TITLE={{.GameTitleQuoted}}
INSTALL_DIR="${HOME}/Games/${GAME_TITLE}"
SERVER={{.ServerURLQuoted}}
TOKEN={{.TokenQuoted}}

echo "[CargoDeck] Installing: ${GAME_TITLE}"
echo "[CargoDeck] Destination: ${INSTALL_DIR}"
mkdir -p "${INSTALL_DIR}"
cd "${INSTALL_DIR}"

# ---- Download files ----
{{range .Files}}echo "[CargoDeck] Downloading: {{.}}"
mkdir -p "$(dirname "{{.}}")"
curl -L --continue-at - \
  -H "Authorization: Bearer ${TOKEN}" \
  -o {{shellQuote .}} \
  "${SERVER}/api/v3/game/${GAME_ID}/file?path={{urlEncode .}}"
{{end}}
echo "[CargoDeck] Download complete."

# ---- Run installer if found ----
INSTALLER=""
for f in setup*.exe Setup*.exe install*.exe Install*.exe; do
  if [ -f "$f" ]; then
    INSTALLER="$f"
    break
  fi
done

if [ -n "${INSTALLER}" ]; then
  echo "[CargoDeck] Found installer: ${INSTALLER}"

  STEAM_ROOT="${HOME}/.local/share/Steam"
  [ -d "${HOME}/.steam/steam" ] && STEAM_ROOT="${HOME}/.steam/steam"
  PROTON_BIN=""

  for dir in "${STEAM_ROOT}/compatibilitytools.d" "${STEAM_ROOT}/steamapps/common"; do
    if [ -d "$dir" ]; then
      while IFS= read -r -d '' p; do
        [ -f "$p" ] && PROTON_BIN="$p" && break 2
      done < <(find "$dir" -maxdepth 2 -name "proton" -print0 2>/dev/null | sort -rz)
    fi
  done

  COMPAT_DATA="${HOME}/.steam/steam/steamapps/compatdata/playerr_{{.GameID}}"
  mkdir -p "${COMPAT_DATA}"

  if [ -n "${PROTON_BIN}" ]; then
    echo "[CargoDeck] Running via Proton: ${PROTON_BIN}"
    STEAM_COMPAT_DATA_PATH="${COMPAT_DATA}" \
    STEAM_COMPAT_CLIENT_INSTALL_PATH="${STEAM_ROOT}" \
    "${PROTON_BIN}" run "${INSTALLER}"
  elif command -v wine &>/dev/null; then
    echo "[CargoDeck] Running via Wine"
    wine "${INSTALLER}"
  else
    echo "[CargoDeck] WARNING: No Proton or Wine found. Run manually: ${INSTALL_DIR}/${INSTALLER}"
  fi
fi

# ---- Create Steam shortcut via Playerr ----
echo "[CargoDeck] Creating Steam shortcut..."
curl -s -X POST "${SERVER}/api/v3/game/${GAME_ID}/shortcut" \
  || echo "[CargoDeck] WARNING: Could not create Steam shortcut."

echo "[CargoDeck] Done. Files are in: ${INSTALL_DIR}"
`

type scriptData struct {
	GameID          int
	GameTitle       string
	GameTitleQuoted string
	ServerURL       string
	ServerURLQuoted string
	TokenQuoted     string
	Files           []string
	GeneratedAt     string
}

var scriptFuncMap = template.FuncMap{
	"shellQuote": shellQuote,
	"urlEncode":  url.QueryEscape,
}

func (h *Handler) ServeInstallScript(w http.ResponseWriter, r *http.Request) {
	id, err := paramInt(r, "id")
	if err != nil {
		jsonErr(w, 400, "invalid id")
		return
	}

	game, err := h.repo.GetGameByID(id)
	if err != nil || game == nil {
		jsonErr(w, 404, "game not found")
		return
	}

	var files []string
	for _, gf := range game.GameFiles {
		files = append(files, gf.RelativePath)
	}
	if len(files) == 0 {
		jsonErr(w, 400, "game has no tracked files (run a media scan first)")
		return
	}

	cfg := h.cfg.LoadAgent()
	serverURL := resolveServerURL(r)

	// Strip newlines/carriage returns — they would break shell script context.
	safeTitle := strings.NewReplacer("\n", " ", "\r", " ").Replace(game.Title)

	data := scriptData{
		GameID:          id,
		GameTitle:       safeTitle,
		GameTitleQuoted: shellQuote(safeTitle),
		ServerURL:       serverURL,
		ServerURLQuoted: shellQuote(serverURL),
		TokenQuoted:     shellQuote(cfg.Token),
		Files:           files,
		GeneratedAt:     time.Now().UTC().Format(time.RFC3339),
	}

	tmpl, err := template.New("install").Funcs(scriptFuncMap).Parse(installScriptTmpl)
	if err != nil {
		jsonErr(w, 500, "template error: "+err.Error())
		return
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		jsonErr(w, 500, "render error: "+err.Error())
		return
	}

	filename := fmt.Sprintf("install-%s.sh", safeName(game.Title))
	w.Header().Set("Content-Type", "text/x-sh")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	w.WriteHeader(http.StatusOK)
	w.Write(buf.Bytes())
}

func safeName(title string) string {
	var b strings.Builder
	for _, ch := range title {
		if unicode.IsLetter(ch) || unicode.IsDigit(ch) || ch == '-' || ch == '_' {
			b.WriteRune(ch)
		} else if ch == ' ' {
			b.WriteRune('-')
		}
	}
	s := b.String()
	if s == "" {
		return "game"
	}
	return strings.ToLower(s)
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}
