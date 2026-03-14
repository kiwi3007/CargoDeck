package ddm

import (
	"archive/zip"
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/kiwi3007/cargodeck/internal/sse"
)

// JobStatus mirrors the agent job status values for consistency.
type JobStatus string

const (
	JobQueued      JobStatus = "queued"
	JobDownloading JobStatus = "downloading"
	JobInstalling  JobStatus = "installing"
	JobDone        JobStatus = "done"
	JobFailed      JobStatus = "failed"
)

// ProgressEvent is emitted via SSE (event name: STEAM_DOWNLOAD_PROGRESS).
type ProgressEvent struct {
	JobID   string    `json:"jobId"`
	GameID  int       `json:"gameId"`
	Status  JobStatus `json:"status"`
	Message string    `json:"message"`
	Percent int       `json:"percent"`
}

// DepotEntry holds the manifest data for a single Steam depot.
type DepotEntry struct {
	DepotID     int
	DepotKey    string // hex encryption key
	ManifestGID string // manifest version ID
	AppToken    int64
}

// Config holds executor configuration.
type Config struct {
	BinPath string
}

// DefaultConfig returns a Config populated from the environment or the Docker-bundled path.
func DefaultConfig() Config {
	bin := os.Getenv("PLAYERR_DDM_BIN")
	if bin == "" {
		bin = "/app/depotdownloadermod/DepotDownloader"
	}
	return Config{BinPath: bin}
}

// Executor runs DepotDownloaderMod jobs on the server.
type Executor struct {
	cfg        Config
	broker     *sse.Broker
	mu         sync.Mutex
	activeJobs map[int]string // gameID → jobID
}

// NewExecutor creates a new Executor.
func NewExecutor(cfg Config, broker *sse.Broker) *Executor {
	return &Executor{
		cfg:        cfg,
		broker:     broker,
		activeJobs: make(map[int]string),
	}
}

// IsAvailable returns true if the DDMod binary exists at the configured path.
func (e *Executor) IsAvailable() bool {
	_, err := os.Stat(e.cfg.BinPath)
	return err == nil
}

// ActiveJob returns the running jobID for a game, if any.
func (e *Executor) ActiveJob(gameID int) (string, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	id, ok := e.activeJobs[gameID]
	return id, ok
}

// Download starts an async DepotDownloaderMod download for the given game.
// outputDir is where game files are written (e.g. the configured download path).
// manifestDir is the steam-manifests/{gameID} directory containing manifests.zip.
// Returns the jobID immediately; progress events are emitted via SSE.
func (e *Executor) Download(
	gameID int,
	appID int,
	gameTitle string,
	outputDir string,
	manifestDir string,
	depots []DepotEntry,
) (string, error) {
	e.mu.Lock()
	if existing, running := e.activeJobs[gameID]; running {
		e.mu.Unlock()
		return "", fmt.Errorf("download already in progress (job %s)", existing)
	}
	jobID := fmt.Sprintf("ddm-%d-%d", gameID, time.Now().UnixMilli())
	e.activeJobs[gameID] = jobID
	e.mu.Unlock()

	go e.run(gameID, jobID, appID, gameTitle, outputDir, manifestDir, depots)
	return jobID, nil
}

func (e *Executor) run(
	gameID int,
	jobID string,
	appID int,
	gameTitle string,
	outputDir string,
	manifestDir string,
	depots []DepotEntry,
) {
	defer func() {
		e.mu.Lock()
		delete(e.activeJobs, gameID)
		e.mu.Unlock()
	}()

	publish := func(status JobStatus, msg string, pct int) {
		evt := ProgressEvent{JobID: jobID, GameID: gameID, Status: status, Message: msg, Percent: pct}
		data, _ := json.Marshal(evt)
		e.broker.Publish("STEAM_DOWNLOAD_PROGRESS", string(data))
		log.Printf("[DDM] job=%s status=%s pct=%d: %s", jobID, status, pct, msg)
	}

	publish(JobQueued, "Starting DDMod download...", 0)

	if _, err := os.Stat(e.cfg.BinPath); err != nil {
		publish(JobFailed, "DDM binary not found at "+e.cfg.BinPath, 0)
		return
	}

	// Game files land in {outputDir}/{sanitized title}/
	gameDir := filepath.Join(outputDir, sanitizeTitle(gameTitle))
	if err := os.MkdirAll(gameDir, 0o755); err != nil {
		publish(JobFailed, "Cannot create game dir: "+err.Error(), 0)
		return
	}

	// Temp dir for extracting manifests.zip
	tmpDir, err := os.MkdirTemp("", "cargodeck-ddm-*")
	if err != nil {
		publish(JobFailed, "Temp dir error: "+err.Error(), 0)
		return
	}
	defer os.RemoveAll(tmpDir)

	// Extract manifests.zip into tmpDir
	publish(JobDownloading, "Extracting manifest files...", 2)
	zipPath := filepath.Join(manifestDir, "manifests.zip")
	zipData, err := os.ReadFile(zipPath)
	if err != nil {
		publish(JobFailed, "Cannot read manifests.zip: "+err.Error(), 2)
		return
	}
	if err := extractZIP(zipData, tmpDir); err != nil {
		publish(JobFailed, "Extract manifests: "+err.Error(), 2)
		return
	}

	// Write depotkeys.txt  (one line per depot: "depotId;hexKey")
	keysFile := filepath.Join(tmpDir, "depotkeys.txt")
	var sb strings.Builder
	for _, d := range depots {
		fmt.Fprintf(&sb, "%d;%s\n", d.DepotID, d.DepotKey)
	}
	if err := os.WriteFile(keysFile, []byte(sb.String()), 0o644); err != nil {
		publish(JobFailed, "Write depotkeys: "+err.Error(), 3)
		return
	}

	// Only run DDMod for depots that have a manifest GID and a corresponding .manifest file
	var realDepots []DepotEntry
	for _, d := range depots {
		if d.ManifestGID == "" {
			continue
		}
		mf := filepath.Join(tmpDir, fmt.Sprintf("%d_%s.manifest", d.DepotID, d.ManifestGID))
		if _, err := os.Stat(mf); err == nil {
			realDepots = append(realDepots, d)
		} else {
			log.Printf("[DDM] No manifest file for depot %d, skipping", d.DepotID)
		}
	}
	if len(realDepots) == 0 {
		publish(JobFailed, "No depots with manifest files found in ZIP", 3)
		return
	}

	total := len(realDepots)
	for i, entry := range realDepots {
		pctBase := 5 + (85 * i / total)
		manifestFile := filepath.Join(tmpDir, fmt.Sprintf("%d_%s.manifest", entry.DepotID, entry.ManifestGID))
		publish(JobDownloading, fmt.Sprintf("Downloading depot %d (%d/%d)...", entry.DepotID, i+1, total), pctBase)

		args := []string{
			"-app", fmt.Sprintf("%d", appID),
			"-depot", fmt.Sprintf("%d", entry.DepotID),
			"-manifest", entry.ManifestGID,
			"-manifestfile", manifestFile,
			"-depotkeys", keysFile,
			"-dir", gameDir,
		}
		if entry.AppToken != 0 {
			args = append(args, "-apptoken", fmt.Sprintf("%d", entry.AppToken))
		}

		if err := e.runStreaming(args, publish); err != nil {
			return // error already published
		}
	}

	// Write ACF manifest so Steam (and the agent installer) recognises the game
	publish(JobInstalling, "Writing Steam manifest...", 92)
	acfPath := filepath.Join(outputDir, fmt.Sprintf("appmanifest_%d.acf", appID))
	if err := writeACF(acfPath, appID, gameTitle, gameDir); err != nil {
		publish(JobFailed, "Write ACF failed: "+err.Error(), 92)
		return
	}

	publish(JobDone, "Download complete", 100)
}

func (e *Executor) runStreaming(args []string, publish func(JobStatus, string, int)) error {
	cmd := exec.Command(e.cfg.BinPath, args...)
	pr, pw := io.Pipe()
	cmd.Stdout = pw
	cmd.Stderr = pw

	if err := cmd.Start(); err != nil {
		pw.Close()
		pr.Close()
		publish(JobFailed, "DDMod start error: "+err.Error(), 0)
		return err
	}

	// Stream output lines to progress events
	go func() {
		scanner := bufio.NewScanner(pr)
		for scanner.Scan() {
			log.Printf("[DDM] %s", scanner.Text())
			publish(JobDownloading, scanner.Text(), 10)
		}
	}()

	runErr := cmd.Wait()
	pw.Close()
	if runErr != nil {
		publish(JobFailed, "DDMod exited with error: "+runErr.Error(), 0)
		return runErr
	}
	return nil
}

// extractZIP unpacks the contents of a ZIP archive into destDir.
// Only the base filename is used (no directory structure from the archive).
func extractZIP(data []byte, destDir string) error {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return err
	}
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		outPath := filepath.Join(destDir, filepath.Base(f.Name))
		rc, err := f.Open()
		if err != nil {
			return err
		}
		b, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			return err
		}
		if err := os.WriteFile(outPath, b, 0o644); err != nil {
			return err
		}
	}
	return nil
}

// writeACF writes a minimal Steam appmanifest_{appId}.acf file.
// The ACF is placed in outputDir (the same directory as the game folder),
// so both the server library and the agent installer can find it.
func writeACF(path string, appID int, gameTitle, gameDir string) error {
	folderName := filepath.Base(gameDir)
	content := fmt.Sprintf(`"AppState"
{
	"appid"		"%d"
	"Universe"	"1"
	"name"		"%s"
	"StateFlags"	"4"
	"installdir"	"%s"
	"LastUpdated"	"%d"
	"SizeOnDisk"	"0"
	"buildid"	"0"
	"InstalledDepots"
	{
	}
}
`, appID, gameTitle, folderName, time.Now().Unix())
	log.Printf("[DDM] Writing ACF: %s", path)
	return os.WriteFile(path, []byte(content), 0o644)
}

func sanitizeTitle(s string) string {
	r := strings.NewReplacer(
		"/", "-", ":", "-", "\\", "-",
		"*", "-", "?", "", "\"", "",
		"<", "", ">", "", "|", "",
	)
	return strings.TrimSpace(r.Replace(s))
}
