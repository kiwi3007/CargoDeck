package scanner

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/kiwi3007/playerr/internal/domain"
	"github.com/kiwi3007/playerr/internal/repository"
)

// Service scans media folders and adds games to the repository.
type Service struct {
	repo   *repository.GameRepository
	broker EventBroker

	mu            sync.Mutex
	scanning      atomic.Bool
	cancelScan    context.CancelFunc
	lastGameFound atomic.Value
	gamesAdded    atomic.Int64
}

// EventBroker is implemented by sse.Broker.
type EventBroker interface {
	Publish(event, data string)
}

func New(repo *repository.GameRepository, broker EventBroker) *Service {
	s := &Service{repo: repo, broker: broker}
	s.lastGameFound.Store("")
	return s
}

func (s *Service) IsScanning() bool      { return s.scanning.Load() }
func (s *Service) LastGameFound() string { return s.lastGameFound.Load().(string) }
func (s *Service) GamesAdded() int       { return int(s.gamesAdded.Load()) }

func (s *Service) StopScan() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancelScan != nil {
		s.cancelScan()
		s.cancelScan = nil
	}
}

func (s *Service) Scan(folderPath, platform string) {
	if !s.scanning.CompareAndSwap(false, true) {
		log.Println("[Scanner] Already scanning")
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	s.mu.Lock()
	s.cancelScan = cancel
	s.mu.Unlock()

	s.gamesAdded.Store(0)
	s.lastGameFound.Store("")

	go func() {
		defer func() {
			s.scanning.Store(false)
			s.broker.Publish("LIBRARY_UPDATED", "scan_complete")
			log.Printf("[Scanner] Scan complete. Added %d games.", s.GamesAdded())
		}()

		s.doScan(ctx, folderPath, platform)
	}()
}

func (s *Service) doScan(ctx context.Context, folderPath, platformHint string) {
	// Resolve game folders to scan
	var roots []string
	if folderPath != "" {
		roots = []string{folderPath}
	} else {
		// Get all platform paths from repo (could expand later)
		roots = []string{folderPath}
	}

	for _, root := range roots {
		if root == "" {
			continue
		}
		if ctx.Err() != nil {
			return
		}
		s.scanFolder(ctx, root, platformHint)
	}
}

func (s *Service) scanFolder(ctx context.Context, root, platformHint string) {
	entries, err := os.ReadDir(root)
	if err != nil {
		log.Printf("[Scanner] Cannot read dir %s: %v", root, err)
		return
	}

	// Get existing games to avoid duplicates
	existing, _ := s.repo.GetAllGames()
	existingPaths := map[string]bool{}
	for _, g := range existing {
		if g.Path != nil {
			existingPaths[filepath.Clean(*g.Path)] = true
		}
	}

	for _, entry := range entries {
		if ctx.Err() != nil {
			return
		}
		if !entry.IsDir() {
			continue
		}

		gamePath := filepath.Join(root, entry.Name())
		if existingPaths[filepath.Clean(gamePath)] {
			continue
		}

		gameName := cleanGameName(entry.Name())
		log.Printf("[Scanner] Found potential game: %s", gameName)

		s.lastGameFound.Store(gameName)

		// Detect platform from path/hint
		platformID := resolvePlatformID(platformHint, gamePath)

		// Calculate size
		size := dirSize(gamePath)

		// Try to detect executable
		exe := findExecutable(gamePath)
		status := domain.GameStatusMissing
		if exe != "" {
			status = domain.GameStatusReleased
		}

		g := &domain.Game{
			Title:      gameName,
			PlatformID: platformID,
			Added:      time.Now(),
			Status:     status,
			Monitored:  true,
			Path:       &gamePath,
			SizeOnDisk: &size,
		}
		if exe != "" {
			g.ExecutablePath = &exe
		}

		if _, err := s.repo.CreateGame(g); err != nil {
			log.Printf("[Scanner] Error creating game %s: %v", gameName, err)
			continue
		}

		s.gamesAdded.Add(1)
		s.broker.Publish("LIBRARY_UPDATED", gameName)
	}
}

func (s *Service) CleanLibrary() error {
	games, err := s.repo.GetAllGames()
	if err != nil {
		return err
	}
	for _, g := range games {
		if g.Path == nil || *g.Path == "" {
			continue
		}
		if _, err := os.Stat(*g.Path); os.IsNotExist(err) {
			log.Printf("[Scanner] Removing missing game: %s (%s)", g.Title, *g.Path)
			s.repo.DeleteGame(g.ID)
		}
	}
	return nil
}

// ---- helpers ----

func cleanGameName(name string) string {
	// Remove common tags like [v1.0], (2023), etc.
	s := name
	// Replace underscores/dots with spaces
	s = strings.ReplaceAll(s, "_", " ")
	s = strings.ReplaceAll(s, ".", " ")
	return strings.TrimSpace(s)
}

func resolvePlatformID(hint, path string) int {
	h := strings.ToLower(hint)
	p := strings.ToLower(path)
	switch {
	case h == "switch" || strings.Contains(p, "switch"):
		return 130
	case h == "pc" || h == "windows" || strings.Contains(p, "windows") || strings.Contains(p, "pc"):
		return 6
	case h == "ps4" || strings.Contains(p, "ps4"):
		return 48
	case h == "ps5" || strings.Contains(p, "ps5"):
		return 167
	default:
		return 6 // Default to PC
	}
}

func findExecutable(root string) string {
	// Look for common game executables in root and one level deep
	exePatterns := []string{"*.exe", "*.app"}
	for _, pat := range exePatterns {
		matches, _ := filepath.Glob(filepath.Join(root, pat))
		for _, m := range matches {
			base := strings.ToLower(filepath.Base(m))
			// Exclude obvious non-game executables
			if strings.Contains(base, "unins") ||
				strings.Contains(base, "setup") ||
				strings.Contains(base, "install") ||
				strings.Contains(base, "redist") ||
				strings.Contains(base, "directx") ||
				strings.Contains(base, "vcredist") {
				continue
			}
			return m
		}
	}
	return ""
}

func dirSize(path string) int64 {
	var size int64
	_ = filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() {
			size += info.Size()
		}
		return nil
	})
	return size
}
