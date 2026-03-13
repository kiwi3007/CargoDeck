package agentclient

import (
	"bytes"
	"debug/pe"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// readEngineVersion attempts version detection using engine-specific metadata files.
// This is called before the PE exe fallback and covers GOG, Ren'Py, RPG Maker, Electron.
func readEngineVersion(dir string) string {
	if v := readGOGVersion(dir); v != "" {
		return v
	}
	if v := readRenpyVersion(dir); v != "" {
		return v
	}
	if v := readRPGMakerVersion(dir); v != "" {
		return v
	}
	if v := readPackageJSONVersion(dir); v != "" {
		return v
	}
	return ""
}

// readGOGVersion reads version from GOG's goggame-*.info JSON file.
func readGOGVersion(dir string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasPrefix(name, "goggame-") && strings.HasSuffix(name, ".info") {
			data, err := os.ReadFile(filepath.Join(dir, name))
			if err != nil {
				continue
			}
			if v := jsonStringField(data, "version"); v != "" {
				return v
			}
		}
	}
	return ""
}

// readRenpyVersion reads version from Ren'Py game/version.txt or renpy/version.txt.
func readRenpyVersion(dir string) string {
	for _, rel := range []string{
		filepath.Join("game", "version.txt"),
		filepath.Join("renpy", "version.txt"),
	} {
		data, err := os.ReadFile(filepath.Join(dir, rel))
		if err != nil {
			continue
		}
		// Format: "Ren'Py 8.1.3.23091102" or just "1.0.0"
		line := strings.TrimSpace(strings.SplitN(string(data), "\n", 2)[0])
		if line != "" {
			// Strip "Ren'Py <enginever>" prefix if present — that's the engine, not game
			if strings.HasPrefix(line, "Ren'Py ") || strings.HasPrefix(line, "RenPy ") {
				return ""
			}
			return line
		}
	}
	return ""
}

// readRPGMakerVersion reads gameVersion from RPG Maker MV/MZ Data/System.json.
func readRPGMakerVersion(dir string) string {
	for _, rel := range []string{
		filepath.Join("Data", "System.json"),
		filepath.Join("www", "data", "System.json"),
	} {
		data, err := os.ReadFile(filepath.Join(dir, rel))
		if err != nil {
			continue
		}
		if v := jsonStringField(data, "gameVersion"); v != "" {
			return v
		}
	}
	return ""
}

// readPackageJSONVersion reads version from an Electron/Node app's package.json.
// Checks resources/app/package.json, www/package.json, and root package.json.
// Also checks resources/app.asar if present.
func readPackageJSONVersion(dir string) string {
	for _, rel := range []string{
		filepath.Join("resources", "app", "package.json"),
		filepath.Join("www", "package.json"),
		"package.json",
	} {
		data, err := os.ReadFile(filepath.Join(dir, rel))
		if err != nil {
			continue
		}
		if v := jsonStringField(data, "version"); v != "" {
			return v
		}
	}
	// Try ASAR archive
	asarPath := filepath.Join(dir, "resources", "app.asar")
	if v := readASARVersion(asarPath); v != "" {
		return v
	}
	return ""
}

// readASARVersion extracts package.json from an Electron ASAR archive and reads "version".
// ASAR format: [uint32=4][uint32=H][uint32=H][H bytes: JSON file index][file data...]
// The JSON file index is {"files":{"package.json":{"offset":"0","size":N},...}}
func readASARVersion(asarPath string) string {
	f, err := os.Open(asarPath)
	if err != nil {
		return ""
	}
	defer f.Close()

	// Read 16-byte header: 4×uint32 little-endian
	var hdr [16]byte
	if _, err := f.Read(hdr[:]); err != nil {
		return ""
	}
	// hdr[0..3] = 4 (pickle header size marker)
	// hdr[4..7] = archive header size (includes the next 2 uint32s)
	// hdr[8..11] = same value again
	// hdr[12..15] = JSON header size
	jsonSize := binary.LittleEndian.Uint32(hdr[12:])
	if jsonSize == 0 || jsonSize > 4*1024*1024 {
		return "" // sanity limit 4MB
	}
	jsonData := make([]byte, jsonSize)
	if _, err := f.Read(jsonData); err != nil {
		return ""
	}

	// Find package.json entry: {"offset":"<N>","size":<M>}
	needle := []byte(`"package.json"`)
	idx := bytes.Index(jsonData, needle)
	if idx < 0 {
		return ""
	}
	entry := jsonData[idx+len(needle):]

	offsetStr := jsonStringField(entry, "offset")
	sizeVal := jsonIntField(entry, "size")
	if offsetStr == "" || sizeVal <= 0 || sizeVal > 1024*1024 {
		return ""
	}
	fileOffset := int64(0)
	for _, ch := range offsetStr {
		if ch < '0' || ch > '9' {
			return ""
		}
		fileOffset = fileOffset*10 + int64(ch-'0')
	}

	// Data section starts after 16-byte header + jsonSize bytes
	dataStart := int64(16) + int64(jsonSize)
	pkgData := make([]byte, sizeVal)
	if _, err := f.ReadAt(pkgData, dataStart+fileOffset); err != nil {
		return ""
	}
	return jsonStringField(pkgData, "version")
}

// jsonStringField extracts a simple string value from JSON like {"key": "value"}.
// Returns "" if the key is not found or value is not a string.
func jsonStringField(data []byte, key string) string {
	needle := []byte(`"` + key + `"`)
	idx := bytes.Index(data, needle)
	if idx < 0 {
		return ""
	}
	rest := data[idx+len(needle):]
	// Skip whitespace and colon
	i := 0
	for i < len(rest) && (rest[i] == ' ' || rest[i] == '\t' || rest[i] == ':' || rest[i] == '\n' || rest[i] == '\r') {
		i++
	}
	if i >= len(rest) || rest[i] != '"' {
		return ""
	}
	i++ // skip opening quote
	var buf []byte
	for i < len(rest) {
		ch := rest[i]
		if ch == '"' {
			break
		}
		if ch == '\\' && i+1 < len(rest) {
			i++
			buf = append(buf, rest[i])
		} else {
			buf = append(buf, ch)
		}
		i++
	}
	return strings.TrimSpace(string(buf))
}

// readExeVersion attempts to determine a game version from an installed game exe.
// It first tries engine-specific known-safe locations, then falls back to the
// PE ProductVersion resource (skipped for engines where it reflects the runtime,
// not the game, e.g. LÖVE, Unreal).
func readExeVersion(exePath string) string {
	dir := filepath.Dir(exePath)
	stem := strings.TrimSuffix(filepath.Base(exePath), filepath.Ext(exePath))

	// --- Engine-specific known-safe version sources ---

	// LÖVE: exe is the LÖVE runtime. ProductVersion = LÖVE version, not game version.
	// The game's own version lives in conf.lua / main.lua inside the .love archive,
	// which we don't parse. Skip PE entirely.
	if fileExists(filepath.Join(dir, "love.dll")) ||
		fileExists(filepath.Join(dir, stem+".love")) {
		return ""
	}

	// Unreal Engine: version is in Build/Build.version (JSON) relative to the exe.
	// Common structures: {game}/Binaries/Win64/{name}.exe
	//   → Build.version at {game}/Build/Build.version
	//   → or {dir}/../../Build/Build.version
	for _, rel := range []string{
		filepath.Join("..", "..", "Build", "Build.version"),
		filepath.Join("..", "Build", "Build.version"),
		"Build.version",
	} {
		if v := readUnrealBuildVersion(filepath.Join(dir, rel)); v != "" {
			return v
		}
	}

	// Unity: {stem}_Data/ directory present.
	// Unity's ProductVersion in the exe is set by the developer and is usually the
	// game version, BUT many developers leave it blank or set it to "1.0". We try it
	// below via the normal PE path — it's safer than skipping entirely.
	// Detect and skip only if ProductVersion looks like a Unity engine version (e.g. "2022.3.1").

	// --- PE ProductVersion fallback ---
	return readPEProductVersion(exePath)
}

// readUnrealBuildVersion reads the MajorVersion/MinorVersion/PatchVersion fields
// from an Unreal Engine Build.version JSON file. Returns "" if not an Unreal file.
func readUnrealBuildVersion(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	// Simple key scan — avoid importing encoding/json for this tiny struct.
	major := jsonIntField(data, "MajorVersion")
	minor := jsonIntField(data, "MinorVersion")
	patch := jsonIntField(data, "PatchVersion")
	if major < 0 {
		return ""
	}
	if patch > 0 {
		return fmt.Sprintf("%d.%d.%d", major, minor, patch)
	}
	return fmt.Sprintf("%d.%d", major, minor)
}

// jsonIntField extracts a simple integer value from JSON like {"Key": 123}.
// Returns -1 if the key is not found.
func jsonIntField(data []byte, key string) int {
	needle := []byte(`"` + key + `"`)
	idx := bytes.Index(data, needle)
	if idx < 0 {
		return -1
	}
	rest := data[idx+len(needle):]
	// Skip whitespace and colon
	i := 0
	for i < len(rest) && (rest[i] == ' ' || rest[i] == '\t' || rest[i] == ':') {
		i++
	}
	n := 0
	found := false
	for i < len(rest) && rest[i] >= '0' && rest[i] <= '9' {
		n = n*10 + int(rest[i]-'0')
		i++
		found = true
	}
	if !found {
		return -1
	}
	return n
}

// readPEProductVersion reads ProductVersion from a Windows PE exe's .rsrc section.
// Returns "" if not parseable or if the version looks like a Unity engine version.
func readPEProductVersion(exePath string) string {
	f, err := pe.Open(exePath)
	if err != nil {
		return ""
	}
	defer f.Close()

	var rsrcData []byte
	for _, s := range f.Sections {
		if s.Name == ".rsrc" {
			rsrcData, _ = s.Data()
			break
		}
	}
	if rsrcData == nil {
		return ""
	}

	v := parseVersionFromRSRC(rsrcData)

	// Reject versions that look like Unity engine versions (year-based, e.g. "2022.3.1.0")
	if len(v) >= 4 && v[:2] == "20" && v[4] == '.' {
		return ""
	}

	return v
}

// parseVersionFromRSRC searches for VS_FIXEDFILEINFO magic and returns ProductVersion.
// Falls back to FileVersion only if ProductVersion is all zeros.
//
// VS_FIXEDFILEINFO layout (little-endian DWORDs):
//
//	+0  dwSignature       0xFEEF04BD
//	+4  dwStrucVersion
//	+8  dwFileVersionMS
//	+12 dwFileVersionLS
//	+16 dwProductVersionMS   major<<16 | minor
//	+20 dwProductVersionLS   patch<<16 | build
func parseVersionFromRSRC(data []byte) string {
	magic := []byte{0xBD, 0x04, 0xEF, 0xFE}
	idx := bytes.Index(data, magic)
	if idx < 0 || idx+24 > len(data) {
		return ""
	}

	productMS := binary.LittleEndian.Uint32(data[idx+16:])
	productLS := binary.LittleEndian.Uint32(data[idx+20:])

	major := productMS >> 16
	minor := productMS & 0xFFFF
	patch := productLS >> 16
	build := productLS & 0xFFFF

	// Fall back to FileVersion if ProductVersion is all zeros
	if major == 0 && minor == 0 {
		fileMS := binary.LittleEndian.Uint32(data[idx+8:])
		fileLS := binary.LittleEndian.Uint32(data[idx+12:])
		major = fileMS >> 16
		minor = fileMS & 0xFFFF
		patch = fileLS >> 16
		build = fileLS & 0xFFFF
	}

	if major == 0 && minor == 0 {
		return ""
	}
	if build > 0 {
		return fmt.Sprintf("%d.%d.%d.%d", major, minor, patch, build)
	}
	if patch > 0 {
		return fmt.Sprintf("%d.%d.%d", major, minor, patch)
	}
	return fmt.Sprintf("%d.%d", major, minor)
}

func fileExists(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && !fi.IsDir()
}
