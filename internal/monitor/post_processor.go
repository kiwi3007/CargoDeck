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

	"github.com/kiwi3007/playerr/internal/config"
	"github.com/kiwi3007/playerr/internal/domain"
	"github.com/kiwi3007/playerr/internal/metadata/igdb"
	"github.com/kiwi3007/playerr/internal/repository"
	"github.com/kiwi3007/playerr/internal/sse"
)

// Processor runs the post-download pipeline: extract → clean → move → add to library.
type Processor struct {
	cfg    *config.Service
	repo   *repository.GameRepository
	broker *sse.Broker
}

func NewProcessor(cfg *config.Service, repo *repository.GameRepository, broker *sse.Broker) *Processor {
	return &Processor{cfg: cfg, repo: repo, broker: broker}
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

	// 1. Auto-Extract
	if settings.EnableAutoExtract && isDir {
		p.extractArchives(d.DownloadPath)
	}

	// 2. Deep Clean
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

	isDir := false
	if fi, err := os.Stat(d.DownloadPath); err == nil {
		isDir = fi.IsDir()
	}

	if isDir {
		p.moveDirectory(d, libraryRoot, containerName, igdbID)
	} else {
		p.moveSingleFile(d, libraryRoot, containerName, igdbID)
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

func (p *Processor) moveDirectory(d domain.DownloadStatus, libraryRoot, containerName string, igdbID *int) {
	srcDir := d.DownloadPath
	originalName := filepath.Base(srcDir)

	// Build destination: library/CleanName/OriginalReleaseName/
	destBase := filepath.Join(libraryRoot, containerName, originalName)

	var firstGameFile string
	err := filepath.WalkDir(srcDir, func(path string, de os.DirEntry, err error) error {
		if err != nil || de.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(srcDir, path)
		dest := filepath.Join(destBase, rel)
		if err := moveFile(path, dest); err != nil {
			log.Printf("[PostDownload] Move failed %s: %v", rel, err)
			return nil
		}
		if firstGameFile == "" && validGameExts[strings.ToLower(filepath.Ext(path))] {
			firstGameFile = dest
		}
		return nil
	})
	if err != nil {
		log.Printf("[PostDownload] Walk error: %v", err)
	}

	// Remove empty source dir
	_ = os.RemoveAll(srcDir)

	if firstGameFile != "" {
		p.addGameToLibrary(containerName, firstGameFile, igdbID)
		p.broker.Publish("LIBRARY_UPDATED", "{}")
	}
}

func (p *Processor) moveSingleFile(d domain.DownloadStatus, libraryRoot, containerName string, igdbID *int) {
	src := d.DownloadPath
	ext := strings.ToLower(filepath.Ext(src))
	if !validGameExts[ext] {
		log.Printf("[PostDownload] Not a recognised game file, skipping: %s", src)
		return
	}

	dest := filepath.Join(libraryRoot, containerName, filepath.Base(src))
	if err := moveFile(src, dest); err != nil {
		log.Printf("[PostDownload] Move failed: %v", err)
		return
	}

	p.addGameToLibrary(containerName, dest, igdbID)
	p.broker.Publish("LIBRARY_UPDATED", "{}")
}

// addGameToLibrary associates the downloaded file with an existing game (matched by IGDB ID)
// or creates a new Game record if no match is found.
func (p *Processor) addGameToLibrary(title, filePath string, igdbID *int) {
	dir := filepath.Dir(filePath)

	baseLower := strings.ToLower(filepath.Base(filePath))
	status := domain.GameStatusDownloaded
	if strings.Contains(baseLower, "setup") || strings.Contains(baseLower, "install") {
		status = domain.GameStatusInstallerDetected
	}

	// Try to find an existing library entry: first by IGDB ID, then by title.
	var existing *domain.Game
	if igdbID != nil {
		existing, _ = p.repo.GetGameByIgdbID(*igdbID)
	}
	if existing == nil {
		existing, _ = p.repo.GetGameByTitle(title)
	}
	if existing != nil {
		existing.Path = &dir
		existing.ExecutablePath = &filePath
		existing.Status = status
		if _, err := p.repo.UpdateGame(existing.ID, existing); err != nil {
			log.Printf("[PostDownload] Failed to update %q (id:%d): %v", existing.Title, existing.ID, err)
		} else {
			log.Printf("[PostDownload] Associated download with existing game %q (id:%d)", existing.Title, existing.ID)
		}
		return
	}

	// Skip if this exact path is already tracked.
	if games, err := p.repo.GetAllGames(); err == nil {
		for _, g := range games {
			if g.ExecutablePath != nil && *g.ExecutablePath == filePath {
				return
			}
		}
	}

	game := domain.Game{
		Title:          title,
		ExecutablePath: &filePath,
		Path:           &dir,
		Added:          time.Now(),
		Status:         status,
		Monitored:      true,
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

	if _, err := p.repo.CreateGame(&game); err != nil {
		log.Printf("[PostDownload] Failed to add %q to library: %v", title, err)
	} else {
		log.Printf("[PostDownload] Added %q to library", title)
	}
}

// ---- Helpers ----

// moveFile moves src to dst, creating parent dirs. Falls back to copy+delete.
func moveFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}
	// Try rename first (same filesystem — instant)
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
	// Cross-filesystem: copy then delete
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

	if _, err = io.Copy(out, in); err != nil {
		return err
	}
	in.Close()
	return os.Remove(src)
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
