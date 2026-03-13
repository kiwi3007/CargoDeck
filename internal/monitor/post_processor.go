package monitor

import (
	"archive/zip"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/kiwi3007/cargodeck/internal/config"
	"github.com/kiwi3007/cargodeck/internal/domain"
	"github.com/kiwi3007/cargodeck/internal/indexer"
	"github.com/kiwi3007/cargodeck/internal/metadata/igdb"
	"github.com/kiwi3007/cargodeck/internal/repository"
	"github.com/kiwi3007/cargodeck/internal/sse"
)

// Processor runs the post-download pipeline: extract → clean → move → add to library.
type Processor struct {
	cfg       *config.Service
	repo      *repository.GameRepository
	broker    *sse.Broker
	linkStore *DownloadLinkStore
}

func NewProcessor(cfg *config.Service, repo *repository.GameRepository, broker *sse.Broker, linkStore *DownloadLinkStore) *Processor {
	return &Processor{cfg: cfg, repo: repo, broker: broker, linkStore: linkStore}
}

// Process runs the full pipeline for a completed download.
func (p *Processor) Process(d domain.DownloadStatus) {
	log.Printf("[PostDownload] Processing: %s at %s", d.Name, d.DownloadPath)

	settings := p.cfg.LoadPostDownload()

	isDir := false
	if fi, err := os.Stat(d.DownloadPath); err == nil {
		isDir = fi.IsDir()
	} else {
		log.Printf("[PostDownload] Path not found, skipping: %s", d.DownloadPath)
		return
	}

	// 1. Deep Clean (no server-side extraction — archives are stored compressed and extracted on-device)
	if settings.EnableDeepClean && isDir {
		p.deepClean(d.DownloadPath, settings.UnwantedExtensions)
	}

	// 3. Auto-Move + add to library
	if settings.EnableAutoMove {
		p.autoMove(d)
	}
}

// ---- Step 1: Extract archives ----

var archiveExts = map[string]bool{".zip": true, ".rar": true, ".7z": true}

func (p *Processor) extractArchives(dir string) {
	_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if !archiveExts[ext] {
			return nil
		}
		if isMultiPartNotFirst(filepath.Base(path)) {
			return nil
		}
		if extractArchive(path, dir) {
			log.Printf("[PostDownload] Extracted %s, removing archive", filepath.Base(path))
			_ = os.Remove(path)
		}
		return nil
	})
}

// isMultiPartNotFirst returns true for part2, part3… of a multi-part set.
func isMultiPartNotFirst(name string) bool {
	lower := strings.ToLower(name)
	if strings.Contains(lower, ".part") {
		return !strings.Contains(lower, ".part01.") && !strings.Contains(lower, ".part1.")
	}
	// Numerical split: .002, .003 etc.
	if matched, _ := regexp.MatchString(`\.\d{3}$`, lower); matched {
		return !strings.HasSuffix(lower, ".001")
	}
	return false
}

func extractArchive(src, destDir string) bool {
	ext := strings.ToLower(filepath.Ext(src))
	switch ext {
	case ".zip":
		return extractZip(src, destDir)
	case ".rar":
		tool := find7z()
		if tool == "" {
			log.Printf("[PostDownload] 7z not found, skipping %s", src)
			return false
		}
		return runExtractor(tool, "x", src, "-o"+destDir, "-y")
	case ".7z":
		tool := find7z()
		if tool == "" {
			log.Printf("[PostDownload] 7z not found, skipping %s", src)
			return false
		}
		return runExtractor(tool, "x", src, "-o"+destDir, "-y")
	}
	return false
}

func extractZip(src, destDir string) bool {
	r, err := zip.OpenReader(src)
	if err != nil {
		log.Printf("[PostDownload] zip open error: %v", err)
		return false
	}
	defer r.Close()

	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			continue
		}
		// Sanitise: strip path traversal
		name := filepath.Base(f.Name)
		dest := filepath.Join(destDir, name)
		if err := writeZipEntry(f, dest); err != nil {
			log.Printf("[PostDownload] zip extract error %s: %v", name, err)
		}
	}
	return true
}

func writeZipEntry(f *zip.File, dest string) error {
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()

	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, rc)
	return err
}

func runExtractor(cmd string, args ...string) bool {
	if _, err := exec.LookPath(cmd); err != nil {
		log.Printf("[PostDownload] %s not found in PATH", cmd)
		return false
	}
	c := exec.Command(cmd, args...)
	c.Stdout = io.Discard
	c.Stderr = io.Discard
	if err := c.Run(); err != nil {
		log.Printf("[PostDownload] %s error: %v", cmd, err)
		return false
	}
	return true
}

func find7z() string {
	for _, name := range []string{"7z", "7za", "7zz"} {
		if p, err := exec.LookPath(name); err == nil {
			return p
		}
	}
	return ""
}

// ---- Step 2: Deep clean ----

func (p *Processor) deepClean(dir string, unwanted []string) {
	if len(unwanted) == 0 {
		return
	}
	set := map[string]bool{}
	for _, ext := range unwanted {
		set[strings.ToLower(ext)] = true
	}

	_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if set[strings.ToLower(filepath.Ext(path))] {
			log.Printf("[PostDownload] Deleting unwanted: %s", filepath.Base(path))
			_ = os.Remove(path)
		}
		return nil
	})
}

// ---- Step 3: Auto-move to library ----

var validGameExts = map[string]bool{
	".nsp": true, ".xci": true, ".pkg": true, ".iso": true,
	".exe": true, ".zip": true, ".rar": true, ".7z": true,
}

func (p *Processor) autoMove(d domain.DownloadStatus) {
	media := p.cfg.LoadMedia()
	libraryRoot := media.DestinationPath
	if libraryRoot == "" {
		libraryRoot = media.FolderPath
	}
	if libraryRoot == "" {
		log.Println("[PostDownload] Auto-move skipped: no library path configured")
		return
	}
	if _, err := os.Stat(libraryRoot); err != nil {
		log.Printf("[PostDownload] Auto-move skipped: library path not found: %s", libraryRoot)
		return
	}

	// Resolve a clean game name (try IGDB, fall back to release name)
	containerName, igdbID := p.resolveGameName(d.Name)
	// Parse version from the raw release name before cleaning strips it.
	version := indexer.ParseVersionFromTitle(d.Name)

	isDir := false
	if fi, err := os.Stat(d.DownloadPath); err == nil {
		isDir = fi.IsDir()
	}

	if isDir {
		p.moveDirectory(d, libraryRoot, containerName, igdbID, version)
	} else {
		p.moveSingleFile(d, libraryRoot, containerName, igdbID, version)
	}
}


func (p *Processor) resolveGameName(releaseName string) (string, *int) {
	clean := cleanReleaseName(releaseName)

	igdbCfg := p.cfg.LoadIgdb()
	if !igdbCfg.IsConfigured() {
		return sanitizeFilename(clean), nil
	}

	client := igdb.NewClient(igdbCfg.ClientId, igdbCfg.ClientSecret)
	games, err := client.SearchGames(clean, nil)
	if err != nil || len(games) == 0 {
		log.Printf("[PostDownload] No IGDB match for %q, using release name", clean)
		return sanitizeFilename(clean), nil
	}

	id := games[0].ID
	log.Printf("[PostDownload] Resolved %q → %q (igdb:%d)", releaseName, games[0].Name, id)
	return sanitizeFilename(games[0].Name), &id
}

func (p *Processor) moveDirectory(d domain.DownloadStatus, libraryRoot, containerName string, igdbID *int, version string) {
	srcDir := d.DownloadPath
	originalName := filepath.Base(srcDir)
	gameDir := filepath.Join(libraryRoot, containerName)

	// Build destination: library/CleanName/OriginalReleaseName/
	destBase := filepath.Join(gameDir, originalName)

	// If destination already exists this download was processed before (e.g. after a restart).
	// Skip re-copying and just ensure the library record is up to date.
	if _, err := os.Stat(destBase); err == nil {
		log.Printf("[PostDownload] Destination already exists, skipping copy: %s", destBase)
		p.addGameToLibrary(d.ID, containerName, gameDir, installerStatusFromDir(srcDir), igdbID, version)
		p.broker.Publish("LIBRARY_UPDATED", "{}")
		return
	}

	hasGameFile := false
	err := filepath.WalkDir(srcDir, func(path string, de os.DirEntry, err error) error {
		if err != nil || de.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(srcDir, path)
		dest := filepath.Join(destBase, rel)
		if err := linkOrCopyFile(path, dest); err != nil {
			log.Printf("[PostDownload] Link/copy failed %s: %v", rel, err)
			return nil
		}
		if validGameExts[strings.ToLower(filepath.Ext(path))] {
			hasGameFile = true
		}
		return nil
	})
	if err != nil {
		log.Printf("[PostDownload] Walk error: %v", err)
	}

	// Source dir is intentionally left intact so the torrent client can continue seeding.

	if hasGameFile {
		p.addGameToLibrary(d.ID, containerName, gameDir, installerStatusFromDir(srcDir), igdbID, version)
		p.broker.Publish("LIBRARY_UPDATED", "{}")
	}
}

// installerStatusFromDir returns InstallerDetected if any top-level filename in dir
// looks like an installer, otherwise Downloaded.
func installerStatusFromDir(dir string) domain.GameStatus {
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		lower := strings.ToLower(e.Name())
		if strings.Contains(lower, "setup") || strings.Contains(lower, "install") {
			return domain.GameStatusInstallerDetected
		}
	}
	return domain.GameStatusDownloaded
}

func (p *Processor) moveSingleFile(d domain.DownloadStatus, libraryRoot, containerName string, igdbID *int, version string) {
	src := d.DownloadPath
	ext := strings.ToLower(filepath.Ext(src))
	if !validGameExts[ext] {
		log.Printf("[PostDownload] Not a recognised game file, skipping: %s", src)
		return
	}

	gameDir := filepath.Join(libraryRoot, containerName)
	dest := filepath.Join(gameDir, filepath.Base(src))
	if err := linkOrCopyFile(src, dest); err != nil {
		log.Printf("[PostDownload] Link/copy failed: %v", err)
		return
	}

	baseLower := strings.ToLower(filepath.Base(src))
	status := domain.GameStatusDownloaded
	if strings.Contains(baseLower, "setup") || strings.Contains(baseLower, "install") {
		status = domain.GameStatusInstallerDetected
	}
	p.addGameToLibrary(d.ID, containerName, gameDir, status, igdbID, version)
	p.broker.Publish("LIBRARY_UPDATED", "{}")
}

// addGameToLibrary associates a game directory with an existing game record (matched by
// download link, IGDB ID, or title) or creates a new one.
// downloadID is the client-assigned ID (e.g. qBittorrent infohash) used to look up the
// game the user originally triggered the download from.
func (p *Processor) addGameToLibrary(downloadID, title, gameDir string, status domain.GameStatus, igdbID *int, version string) {
	// Priority 1: use the stored link (game the user clicked "download" on).
	var existing *domain.Game
	if p.linkStore != nil && downloadID != "" {
		if gameID, ok := p.linkStore.Get(downloadID); ok {
			if g, err := p.repo.GetGameByID(gameID); err == nil && g != nil {
				existing = g
				log.Printf("[PostDownload] Matched download %q to linked game %q (id:%d)", downloadID, g.Title, g.ID)
			}
		}
	}

	// Priority 2: IGDB ID match, then title match.
	if existing == nil && igdbID != nil {
		existing, _ = p.repo.GetGameByIgdbID(*igdbID)
	}
	if existing == nil {
		existing, _ = p.repo.GetGameByTitle(title)
	}
	if existing != nil {
		existing.Path = &gameDir
		existing.Status = status
		if _, err := p.repo.UpdateGame(existing.ID, existing); err != nil {
			log.Printf("[PostDownload] Failed to update %q (id:%d): %v", existing.Title, existing.ID, err)
		} else {
			log.Printf("[PostDownload] Associated download with existing game %q (id:%d)", existing.Title, existing.ID)
		}
		if version != "" {
			_ = p.repo.UpdateGameVersion(existing.ID, version)
			log.Printf("[PostDownload] Set version %q for %q", version, existing.Title)
		}
		return
	}

	// Skip if this directory is already tracked.
	if existing, err := p.repo.GetGameByPath(filepath.Clean(gameDir)); err == nil && existing != nil {
		return
	}

	game := domain.Game{
		Title:     title,
		Path:      &gameDir,
		Added:     time.Now(),
		Status:    status,
		Monitored: true,
	}

	// Attach IGDB metadata if we have an ID
	if igdbID != nil {
		igdbCfg := p.cfg.LoadIgdb()
		if igdbCfg.IsConfigured() {
			client := igdb.NewClient(igdbCfg.ClientId, igdbCfg.ClientSecret)
			if games, err := client.GetGamesByIds([]int{*igdbID}); err == nil && len(games) > 0 {
				g := games[0]
				id := g.ID
				game.IgdbID = &id
				if g.Summary != "" {
					game.Overview = &g.Summary
				}
				for _, genre := range g.Genres {
					game.Genres = append(game.Genres, genre.Name)
				}
				if g.Cover != nil {
					u := fmt.Sprintf("https://images.igdb.com/igdb/image/upload/t_cover_big/%s.jpg", g.Cover.ImageID)
					game.Images.CoverUrl = &u
				}
				if g.FirstReleaseDate != nil {
					t := time.Unix(*g.FirstReleaseDate, 0).UTC()
					game.ReleaseDate = &t
					y := t.Year()
					game.Year = y
				}
			}
		}
	}

	if created, err := p.repo.CreateGame(&game); err != nil {
		log.Printf("[PostDownload] Failed to add %q to library: %v", title, err)
	} else {
		log.Printf("[PostDownload] Added %q to library", title)
		if version != "" && created != nil {
			_ = p.repo.UpdateGameVersion(created.ID, version)
			log.Printf("[PostDownload] Set version %q for %q", version, title)
		}
	}
}

// ---- Helpers ----

// linkOrCopyFile hardlinks src to dst (same filesystem, instant, no extra disk use).
// Falls back to a plain copy if the hardlink fails (e.g. cross-filesystem).
// The source is never deleted so the torrent client can continue seeding.
func linkOrCopyFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}
	// Try hardlink first — zero extra space, instant
	if err := os.Link(src, dst); err == nil {
		return nil
	}
	// Fallback: copy without deleting source
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}

var (
	bracketRE  = regexp.MustCompile(`[\[\(][^\]\)]*[\]\)]`)
	multiSpaceRE = regexp.MustCompile(`\s{2,}`)
)

var noiseWords = map[string]bool{
	"setup": true, "install": true, "installer": true, "gog": true,
	"repack": true, "fitgirl": true, "dodi": true, "cracked": true,
	"steamrip": true, "portable": true, "iso": true, "bin": true,
	"codex": true, "skidrow": true, "reloaded": true, "plaza": true,
	"cpy": true, "tenoke": true, "rune": true, "goldberg": true,
	"empress": true, "p2p": true, "fairlight": true, "flt": true,
	"prophet": true, "kaos": true, "elamigos": true, "dlc": true,
	"update": true, "upd": true, "collection": true, "edition": true,
	"goty": true, "remastered": true, "remake": true, "definitive": true,
	"nsp": true, "xci": true, "nsz": true, "xcz": true, "pkg": true,
	"zip": true, "rar": true, "7z": true,
	// Language codes used in GOG / scene release names
	"eng": true, "enu": true, "multi": true, "multi2": true, "multi3": true,
	"en": true, "de": true, "fr": true, "ru": true, "pl": true,
	"es": true, "it": true, "pt": true, "nl": true, "cs": true,
}

// versionTagRE strips dotted version strings like v1.63, v2.0.1 before the dot→space pass,
// so "v1.63" is removed as a unit rather than leaving the fragment "63" behind.
var versionTagRE = regexp.MustCompile(`(?i)\bv\d+[\.\d]*[a-z]?\b`)

func cleanReleaseName(s string) string {
	// Strip bracketed content: [FitGirl Repack], (v1.2), etc.
	s = bracketRE.ReplaceAllString(s, " ")
	// Strip version tags while dots are still intact so "v1.63" is removed as a unit.
	s = versionTagRE.ReplaceAllString(s, " ")
	// Replace separators with spaces; + is used in GOG-style names (e.g. "Game + ENG + 1]").
	s = strings.NewReplacer(".", " ", "_", " ", "-", " ", "+", " ").Replace(s)

	words := strings.Fields(s)
	kept := words[:0]
	for _, w := range words {
		// Strip any residual bracket chars left by incomplete pairs (e.g. "1]" → "1").
		w = strings.Trim(w, "[](){}")
		if w == "" {
			continue
		}
		lower := strings.ToLower(w)
		if noiseWords[lower] {
			continue // skip noise words but keep scanning
		}
		kept = append(kept, w)
	}

	result := strings.Join(kept, " ")
	result = multiSpaceRE.ReplaceAllString(result, " ")
	return strings.TrimSpace(result)
}

var invalidFileChars = regexp.MustCompile(`[<>:"/\\|?*\x00-\x1f]`)

func sanitizeFilename(s string) string {
	s = invalidFileChars.ReplaceAllString(s, "_")
	return strings.TrimSpace(s)
}
