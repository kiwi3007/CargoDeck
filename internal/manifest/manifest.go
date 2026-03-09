// Package manifest downloads, caches, and resolves game save paths from the
// Ludusavi Manifest (https://github.com/mtkennerly/ludusavi-manifest).
// The manifest is sourced from PCGamingWiki and covers 10,000+ games.
package manifest

import (
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

const manifestURL = "https://raw.githubusercontent.com/mtkennerly/ludusavi-manifest/master/data/manifest.yaml"

// cacheMaxAge is how long before we re-download the manifest.
const cacheMaxAge = 7 * 24 * time.Hour

// FileRule describes one save-path entry in the manifest.
type FileRule struct {
	Tags []string   `yaml:"tags"`
	When []WhenRule `yaml:"when"`
}

// WhenRule filters a file rule by OS or store.
type WhenRule struct {
	Os    string `yaml:"os"`
	Store string `yaml:"store"`
}

// ManifestEntry is one game entry in the manifest.
type ManifestEntry struct {
	Files map[string]FileRule `yaml:"files"`
	Steam *struct {
		ID int `yaml:"id"`
	} `yaml:"steam"`
}

// Service manages the Ludusavi manifest.
type Service struct {
	cacheDir string
	mu       sync.RWMutex
	entries  map[string]ManifestEntry
	loaded   bool
}

// NewService creates a manifest service. cacheDir is where the yaml file is cached.
func NewService(cacheDir string) *Service {
	return &Service{cacheDir: cacheDir}
}

// cachePath returns the local cache file path.
func (s *Service) cachePath() string {
	return filepath.Join(s.cacheDir, "ludusavi-manifest.yaml")
}

// EnsureLoaded downloads (if needed) and parses the manifest.
// Safe to call multiple times; subsequent calls are no-ops.
func (s *Service) EnsureLoaded() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.loaded {
		return nil
	}

	data, err := s.loadData()
	if err != nil {
		return err
	}

	s.entries = make(map[string]ManifestEntry)
	if err := yaml.Unmarshal(data, &s.entries); err != nil {
		return err
	}
	s.loaded = true
	log.Printf("[Manifest] Loaded %d game entries", len(s.entries))
	return nil
}

// loadData returns manifest bytes, using cache if fresh enough.
func (s *Service) loadData() ([]byte, error) {
	path := s.cachePath()

	// Use cached file if fresh
	if fi, err := os.Stat(path); err == nil && time.Since(fi.ModTime()) < cacheMaxAge {
		log.Printf("[Manifest] Using cached manifest (%s old)", time.Since(fi.ModTime()).Round(time.Hour))
		return os.ReadFile(path)
	}

	// Download fresh copy
	log.Printf("[Manifest] Downloading manifest from %s...", manifestURL)
	resp, err := http.Get(manifestURL)
	if err != nil {
		// Fall back to stale cache if download fails
		if data, rerr := os.ReadFile(path); rerr == nil {
			log.Printf("[Manifest] Download failed, using stale cache: %v", err)
			return data, nil
		}
		return nil, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if err := os.MkdirAll(s.cacheDir, 0755); err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		log.Printf("[Manifest] Warning: could not cache manifest: %v", err)
	}
	log.Printf("[Manifest] Downloaded manifest (%d bytes)", len(data))
	return data, nil
}

// FindByTitle returns the manifest entry for a game title.
// Tries exact match, then case-insensitive, then fuzzy prefix.
func (s *Service) FindByTitle(title string) (ManifestEntry, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Exact match
	if e, ok := s.entries[title]; ok {
		return e, true
	}

	// Case-insensitive
	lower := strings.ToLower(title)
	for k, v := range s.entries {
		if strings.ToLower(k) == lower {
			return v, true
		}
	}
	return ManifestEntry{}, false
}

// FindBySteamID returns the entry with a matching Steam app ID.
func (s *Service) FindBySteamID(steamID int) (ManifestEntry, bool) {
	if steamID == 0 {
		return ManifestEntry{}, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, e := range s.entries {
		if e.Steam != nil && e.Steam.ID == steamID {
			return e, true
		}
	}
	return ManifestEntry{}, false
}

// ResolvePaths returns the list of save directories for a game.
// targetOS should be "linux", "windows", or "mac".
// home is the user's home directory on the target machine (the agent's home, not the server's).
// wineprefix is used to resolve Windows paths when running under Wine/Proton on Linux;
// when non-empty and targetOS != "windows", Windows-tagged paths are also resolved via the prefix.
func ResolvePaths(entry ManifestEntry, targetOS, wineprefix, home string) []string {
	if home == "" {
		home, _ = os.UserHomeDir()
	}
	paths := resolveForOS(entry, targetOS, wineprefix, home)
	if wineprefix != "" && targetOS != "windows" {
		// Also resolve Windows-tagged paths via the Wine prefix
		paths = append(paths, resolveForOS(entry, "windows", wineprefix, home)...)
	}
	return deduplicate(paths)
}

// resolveForOS resolves save paths for a single OS pass.
func resolveForOS(entry ManifestEntry, targetOS, wineprefix, home string) []string {
	var paths []string
	seen := map[string]bool{}

	for tmpl, rule := range entry.Files {
		// Only include save files (not config-only)
		if !hasSaveTag(rule.Tags) {
			continue
		}

		// Check OS applicability
		if !pathAppliesToOS(rule, targetOS) {
			continue
		}

		// Resolve path template
		resolved := resolveTemplate(tmpl, home, wineprefix)
		if resolved == "" {
			continue
		}

		// Strip glob suffixes to get the base directory
		dir := globBase(resolved)

		if !seen[dir] {
			seen[dir] = true
			paths = append(paths, dir)
		}
	}
	return paths
}

// deduplicate returns paths with duplicates removed, preserving order.
func deduplicate(paths []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, p := range paths {
		if !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	return out
}

// hasSaveTag returns true if "save" is in the tags list.
func hasSaveTag(tags []string) bool {
	for _, t := range tags {
		if strings.ToLower(t) == "save" {
			return true
		}
	}
	return false
}

// pathAppliesToOS checks if the file rule applies to the target OS.
// A rule applies if:
//   - It has no When conditions (applies to all)
//   - At least one When condition has a matching OS (or empty OS)
func pathAppliesToOS(rule FileRule, targetOS string) bool {
	if len(rule.When) == 0 {
		// No conditions — check if the path template implies an OS
		return true
	}
	for _, w := range rule.When {
		if w.Os == "" || strings.ToLower(w.Os) == targetOS {
			return true
		}
	}
	return false
}

// WinUserInPrefix finds the Wine/Proton user directory inside a prefix.
// Supports both Proton-style ({prefix}/pfx/drive_c) and Wine-style ({prefix}/drive_c).
// Returns the first non-Public user directory found, or "steamuser" as fallback.
func WinUserInPrefix(wineprefix string) string {
	// Try Proton-style first, then plain Wine-style
	pfxUsers := filepath.Join(wineprefix, "pfx", "drive_c", "users")
	if _, err := os.Stat(pfxUsers); os.IsNotExist(err) {
		pfxUsers = filepath.Join(wineprefix, "drive_c", "users")
	}
	winUser := "steamuser"
	if entries, err := os.ReadDir(pfxUsers); err == nil {
		for _, e := range entries {
			if e.IsDir() && e.Name() != "." && e.Name() != ".." && e.Name() != "Public" {
				winUser = e.Name()
				break
			}
		}
	}
	return winUser
}

// winDocumentsPath returns the path to the user's Documents folder inside a Wine prefix.
// Wine versions differ: some use "Documents" (newer Proton), others "My Documents" (older Wine).
// We detect which variant is the real directory (not the symlink) by checking both.
// If only one exists as a physical directory, use that. Otherwise default to "Documents".
func winDocumentsPath(userDir string) string {
	docs := filepath.Join(userDir, "Documents")
	myDocs := filepath.Join(userDir, "My Documents")

	docsReal, docsErr := filepath.EvalSymlinks(docs)
	myDocsReal, myDocsErr := filepath.EvalSymlinks(myDocs)

	switch {
	case docsErr == nil && myDocsErr != nil:
		// Only Documents exists
		return docs
	case docsErr != nil && myDocsErr == nil:
		// Only My Documents exists (older Wine setup)
		return myDocs
	case docsErr == nil && myDocsErr == nil:
		// Both exist — return the one that is NOT the symlink (i.e., the physical dir).
		// If they resolve to the same path, Documents wins.
		if docsReal == myDocsReal {
			return docs // same physical dir, either works
		}
		// Check which is a symlink
		if lfi, err := os.Lstat(docs); err == nil && lfi.Mode()&os.ModeSymlink != 0 {
			return myDocs // Documents is the symlink, My Documents is real
		}
		return docs
	default:
		// Neither exists yet — default to Documents (Proton standard)
		return docs
	}
}

// resolveTemplate replaces Ludusavi template variables with real paths.
func resolveTemplate(tmpl, home, wineprefix string) string {
	goos := runtime.GOOS

	// XDG / Linux paths
	xdgData := envOr("XDG_DATA_HOME", filepath.Join(home, ".local", "share"))
	xdgConfig := envOr("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	xdgCache := envOr("XDG_CACHE_HOME", filepath.Join(home, ".cache"))

	replacements := map[string]string{
		"<home>":         home,
		"<xdgData>":      xdgData,
		"<xdgConfig>":    xdgConfig,
		"<xdgCache>":     xdgCache,
		"<xdgDocuments>": filepath.Join(home, "Documents"),
		"<xdgDownload>":  filepath.Join(home, "Downloads"),
		"<xdgDesktop>":   filepath.Join(home, "Desktop"),
		"<xdgPictures>":  filepath.Join(home, "Pictures"),
		"<xdgVideos>":    filepath.Join(home, "Videos"),
		"<xdgMusic>":     filepath.Join(home, "Music"),
	}

	// Windows paths
	if goos == "windows" {
		appData := os.Getenv("APPDATA")
		localAppData := os.Getenv("LOCALAPPDATA")
		userProfile := os.Getenv("USERPROFILE")
		publicDir := os.Getenv("PUBLIC")
		if publicDir == "" {
			publicDir = `C:\Users\Public`
		}
		replacements["<winAppData>"] = appData
		replacements["<winLocalAppData>"] = localAppData
		replacements["<winDocuments>"] = filepath.Join(userProfile, "Documents")
		replacements["<winSavedGames>"] = filepath.Join(userProfile, "Saved Games")
		replacements["<winPublic>"] = publicDir
		replacements["<winDir>"] = os.Getenv("SystemRoot")
		replacements["<winProgramData>"] = os.Getenv("PROGRAMDATA")
	} else if wineprefix != "" {
		// Wine/Proton: resolve Windows paths inside the prefix.
		// Support both Proton-style ({prefix}/pfx/drive_c) and Wine-style ({prefix}/drive_c).
		driveC := filepath.Join(wineprefix, "pfx", "drive_c")
		if _, err := os.Stat(driveC); os.IsNotExist(err) {
			driveC = filepath.Join(wineprefix, "drive_c")
		}
		winUser := WinUserInPrefix(wineprefix)
		pfxUsers := filepath.Join(driveC, "users")
		userDir := filepath.Join(pfxUsers, winUser)
		replacements["<winAppData>"] = filepath.Join(userDir, "AppData", "Roaming")
		replacements["<winLocalAppData>"] = filepath.Join(userDir, "AppData", "Local")
		replacements["<winDocuments>"] = winDocumentsPath(userDir)
		replacements["<winSavedGames>"] = filepath.Join(userDir, "Saved Games")
		replacements["<winPublic>"] = filepath.Join(pfxUsers, "Public")
		replacements["<winDir>"] = filepath.Join(driveC, "windows")
		replacements["<winProgramData>"] = filepath.Join(driveC, "ProgramData")
	}

	result := tmpl
	for k, v := range replacements {
		if v == "" {
			continue
		}
		result = strings.ReplaceAll(result, k, v)
	}

	// If template vars remain (not resolved for this OS), skip
	if strings.Contains(result, "<") && strings.Contains(result, ">") {
		return ""
	}

	// Normalize path separators
	result = filepath.FromSlash(result)
	return result
}

// globBase strips glob wildcards from a path, returning the deepest non-glob parent.
func globBase(p string) string {
	parts := strings.Split(p, string(filepath.Separator))
	var clean []string
	for _, part := range parts {
		if strings.ContainsAny(part, "*?[") {
			break
		}
		clean = append(clean, part)
	}
	if len(clean) == 0 {
		return p
	}
	return filepath.Join(clean...)
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
