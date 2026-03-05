package launcher

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// ShortcutEntry mirrors the fields Steam stores in shortcuts.vdf.
type ShortcutEntry struct {
	AppID         uint32
	AppName       string
	Exe           string
	StartDir      string
	Icon          string
	LaunchOptions string
}

// ShortcutAppID computes the non-Steam game appid using Steam's formula:
// CRC32(exe + name) | 0x80000000
func ShortcutAppID(name, exe string) uint32 {
	return crc32.ChecksumIEEE([]byte(exe+name)) | 0x80000000
}

// FindSteamUserConfigDir returns the first <userdata>/<id>/config dir found.
func FindSteamUserConfigDir() string {
	home, _ := os.UserHomeDir()
	roots := []string{
		filepath.Join(home, ".local", "share", "Steam", "userdata"),
		filepath.Join(home, ".steam", "steam", "userdata"),
		// Windows paths
		filepath.Join(os.Getenv("ProgramFiles(x86)"), "Steam", "userdata"),
		filepath.Join(os.Getenv("LOCALAPPDATA"), "Steam", "userdata"),
	}
	for _, root := range roots {
		entries, err := os.ReadDir(root)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			cfgDir := filepath.Join(root, e.Name(), "config")
			if fi, err := os.Stat(cfgDir); err == nil && fi.IsDir() {
				return cfgDir
			}
		}
	}
	return ""
}

// AddSteamShortcut writes (or replaces) a non-Steam shortcut in shortcuts.vdf.
// Returns the appid of the added shortcut.
func AddSteamShortcut(entry ShortcutEntry) (uint32, error) {
	cfgDir := FindSteamUserConfigDir()
	if cfgDir == "" {
		return 0, fmt.Errorf("Steam userdata directory not found")
	}

	vdfPath := filepath.Join(cfgDir, "shortcuts.vdf")

	var existing []ShortcutEntry
	if data, err := os.ReadFile(vdfPath); err == nil {
		existing = parseShortcutsVDF(data)
	}

	if entry.AppID == 0 {
		entry.AppID = ShortcutAppID(entry.AppName, entry.Exe)
	}

	replaced := false
	for i, e := range existing {
		if e.AppName == entry.AppName || e.Exe == entry.Exe {
			existing[i] = entry
			replaced = true
			break
		}
	}
	if !replaced {
		existing = append(existing, entry)
	}

	data := buildShortcutsVDF(existing)
	if err := os.WriteFile(vdfPath, data, 0644); err != nil {
		return 0, fmt.Errorf("write shortcuts.vdf: %w", err)
	}
	log.Printf("[Shortcut] Wrote shortcut %q (appid=%d) to %s", entry.AppName, entry.AppID, vdfPath)
	return entry.AppID, nil
}

// ---- Binary VDF parser ----

func parseShortcutsVDF(data []byte) []ShortcutEntry {
	var entries []ShortcutEntry
	pos := 0

	if pos >= len(data) || data[pos] != 0x00 {
		return entries
	}
	pos++
	pos = skipCString(data, pos) // skip "shortcuts"

	for pos < len(data) {
		if data[pos] == 0x08 {
			break
		}
		if data[pos] != 0x00 {
			break
		}
		pos++
		pos = skipCString(data, pos)

		entry := ShortcutEntry{}
		for pos < len(data) && data[pos] != 0x08 {
			typeByte := data[pos]
			pos++
			key := readCString(data, &pos)
			switch typeByte {
			case 0x02:
				if pos+4 <= len(data) {
					val := binary.LittleEndian.Uint32(data[pos : pos+4])
					pos += 4
					if strings.EqualFold(key, "appid") {
						entry.AppID = val
					}
				}
			case 0x01:
				val := readCString(data, &pos)
				switch strings.ToLower(key) {
				case "appname":
					entry.AppName = val
				case "exe":
					entry.Exe = val
				case "startdir":
					entry.StartDir = val
				case "icon":
					entry.Icon = val
				case "launchoptions":
					entry.LaunchOptions = val
				}
			case 0x00:
				pos = skipMap(data, pos)
			}
		}
		if pos < len(data) && data[pos] == 0x08 {
			pos++
		}
		entries = append(entries, entry)
	}
	return entries
}

func skipMap(data []byte, pos int) int {
	for pos < len(data) && data[pos] != 0x08 {
		typeByte := data[pos]
		pos++
		pos = skipCString(data, pos)
		switch typeByte {
		case 0x01:
			pos = skipCString(data, pos)
		case 0x02:
			pos += 4
		case 0x00:
			pos = skipMap(data, pos)
		}
	}
	if pos < len(data) {
		pos++
	}
	return pos
}

func readCString(data []byte, pos *int) string {
	start := *pos
	for *pos < len(data) && data[*pos] != 0x00 {
		*pos++
	}
	s := string(data[start:*pos])
	if *pos < len(data) {
		*pos++
	}
	return s
}

func skipCString(data []byte, pos int) int {
	for pos < len(data) && data[pos] != 0x00 {
		pos++
	}
	if pos < len(data) {
		pos++
	}
	return pos
}

// ---- Binary VDF builder ----

func buildShortcutsVDF(entries []ShortcutEntry) []byte {
	var buf bytes.Buffer

	buf.WriteByte(0x00)
	writeCString(&buf, "shortcuts")

	for i, e := range entries {
		buf.WriteByte(0x00)
		writeCString(&buf, strconv.Itoa(i))

		writeVDFInt32(&buf, "appid", e.AppID)
		writeVDFStr(&buf, "AppName", e.AppName)
		writeVDFStr(&buf, "Exe", e.Exe)
		writeVDFStr(&buf, "StartDir", e.StartDir)
		writeVDFStr(&buf, "icon", e.Icon)
		writeVDFStr(&buf, "ShortcutPath", "")
		writeVDFStr(&buf, "LaunchOptions", e.LaunchOptions)
		writeVDFInt32(&buf, "IsHidden", 0)
		writeVDFInt32(&buf, "AllowDesktopConfig", 1)
		writeVDFInt32(&buf, "AllowOverlay", 1)
		writeVDFInt32(&buf, "OpenVR", 0)
		writeVDFInt32(&buf, "Devkit", 0)
		writeVDFStr(&buf, "DevkitGameID", "")
		writeVDFInt32(&buf, "DevkitOverrideAppID", 0)
		writeVDFInt32(&buf, "LastPlayTime", 0)
		writeVDFStr(&buf, "FlatpakAppID", "")
		writeVDFStr(&buf, "sortas", "")

		buf.WriteByte(0x00)
		writeCString(&buf, "tags")
		buf.WriteByte(0x08)

		buf.WriteByte(0x08)
	}

	buf.WriteByte(0x08)
	buf.WriteByte(0x08)
	return buf.Bytes()
}

func writeCString(buf *bytes.Buffer, s string) {
	buf.WriteString(s)
	buf.WriteByte(0x00)
}

func writeVDFStr(buf *bytes.Buffer, key, val string) {
	buf.WriteByte(0x01)
	writeCString(buf, key)
	writeCString(buf, val)
}

func writeVDFInt32(buf *bytes.Buffer, key string, val uint32) {
	buf.WriteByte(0x02)
	writeCString(buf, key)
	b := make([]byte, 4)
	binary.LittleEndian.PutUint32(b, val)
	buf.Write(b)
}
