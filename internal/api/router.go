package api

import (
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/kiwi3007/cargodeck/internal/agent"
	"github.com/kiwi3007/cargodeck/internal/config"
	"github.com/kiwi3007/cargodeck/internal/ddm"
	"github.com/kiwi3007/cargodeck/internal/manifest"
	"github.com/kiwi3007/cargodeck/internal/monitor"
	"github.com/kiwi3007/cargodeck/internal/repository"
	"github.com/kiwi3007/cargodeck/internal/scanner"
	"github.com/kiwi3007/cargodeck/internal/sse"
	"github.com/kiwi3007/cargodeck/internal/updater"
)

// Handler holds all dependencies for the API layer.
type Handler struct {
	repo          *repository.GameRepository
	cfg           *config.Service
	broker        *sse.Broker
	scanner       *scanner.Service
	importStatus  *monitor.ImportStatus
	processor     *monitor.Processor
	linkStore     *monitor.DownloadLinkStore
	agentRegistry *agent.Registry
	agentJobs     *agent.JobQueue
	agentBroker   *agent.AgentBroker
	manifest      *manifest.Service
	checker       *updater.Checker
	ddm           *ddm.Executor
	browsePending   sync.Map // requestId → chan agent.BrowseDirResult
	protonPending   sync.Map // requestId → chan agent.ListProtonResult
	installTempDirs sync.Map // gameID (int) → tempDir (string); cleaned up after agent job completes
}

func NewHandler(
	repo *repository.GameRepository,
	cfg *config.Service,
	broker *sse.Broker,
	scanner *scanner.Service,
	importStatus *monitor.ImportStatus,
	processor *monitor.Processor,
	linkStore *monitor.DownloadLinkStore,
	agentRegistry *agent.Registry,
	agentJobs *agent.JobQueue,
	agentBroker *agent.AgentBroker,
	manifestSvc *manifest.Service,
	checker *updater.Checker,
	ddmExec *ddm.Executor,
) *Handler {
	return &Handler{
		repo:          repo,
		cfg:           cfg,
		broker:        broker,
		scanner:       scanner,
		importStatus:  importStatus,
		processor:     processor,
		linkStore:     linkStore,
		agentRegistry: agentRegistry,
		agentJobs:     agentJobs,
		agentBroker:   agentBroker,
		manifest:      manifestSvc,
		checker:       checker,
		ddm:           ddmExec,
	}
}

// NewRouter creates the chi router with all routes registered.
func (h *Handler) NewRouter() http.Handler {
	r := chi.NewRouter()

	r.Use(filteredLogger)
	r.Use(middleware.Recoverer)
	r.Use(corsMiddleware)
	r.Use(securityHeaders)
	r.Use(h.uiAuthMiddleware)

	// Auth — challenge/verify are exempt from uiAuthMiddleware (bootstrap endpoints)
	// otp/generate requires a valid session (handled by uiAuthMiddleware, not exempt)
	r.Get("/api/v3/auth/required", h.GetAuthRequired)
	r.Get("/api/v3/auth/challenge", h.GetChallenge)
	r.Post("/api/v3/auth/verify", h.VerifyAuth)
	r.Post("/api/v3/auth/otp/generate", h.GenerateOTP)
	r.Get("/api/v3/auth/agent-challenge", h.GetAgentChallenge)

	// Health
	r.Get("/health", h.Health)

	// SSE (browser fan-out)
	r.Get("/api/v3/events", h.broker.ServeHTTP)

	// Games
	r.Route("/api/v3/game", func(r chi.Router) {
		r.Get("/", h.GetAllGames)
		r.Post("/", h.CreateGame)
		r.Delete("/all", h.DeleteAllGames)

		// Lookup (IGDB metadata search) — must come before /{id}
		r.Get("/lookup", h.GameLookup)
		r.Get("/lookup/igdb/{igdbId}", h.GameLookupByIgdbID)

		r.Get("/{id}", h.GetGameByID)
		r.Put("/{id}", h.UpdateGame)
		r.Delete("/{id}", h.DeleteGame)


		// Server-side file management (files physically on this server's game.Path)
		r.Get("/{id}/server-files", h.GetServerFiles)
		r.Delete("/{id}/server-file", h.DeleteServerFile)

		// File serving for agent downloads (requires agent auth)
		r.With(h.agentAuthMiddleware).Get("/{id}/file", h.ServeGameFile)

		// Install script download (browser, no auth needed)
		r.Get("/{id}/install-script", h.ServeInstallScript)

		// Manually trigger post-processor import pipeline
		r.Post("/{id}/import", h.ImportGame)

		// Manual update check for a single game
		r.Post("/{id}/check-update", h.CheckGameUpdate)

		// Re-fetch IGDB metadata (title, images, Steam ID, genres, etc.)
		r.Post("/{id}/refresh-metadata", h.RefreshGameMetadata)

		// Per-device launch arguments
		r.Get("/{id}/agent-launch-args", h.GetAgentLaunchArgs)
		r.Patch("/{id}/agent-launch-args", h.SetAgentLaunchArgs)

		// Bulk update check for all games with a known version
		r.Post("/check-update", h.CheckAllUpdates)

		// Steam manifest: store/retrieve steamtoolz ZIP data for manifest-based downloads
		r.Post("/{id}/steam-manifest", h.SetSteamManifest)
		r.Get("/{id}/steam-manifest-info", h.GetSteamManifestInfo)
		r.With(h.agentAuthMiddleware).Get("/{id}/steam-manifest-zip", h.ServeManifestZIP)
		r.Post("/{id}/fetch-manifest", h.FetchManifestFromMorrenus)
		r.Post("/{id}/steam-download", h.SteamDownload)
	})

	// Platforms
	r.Get("/api/v3/platform", h.GetAllPlatforms)
	r.Get("/api/v3/platform/{id}", h.GetPlatformByID)

	// Settings
	r.Route("/api/v3/settings", func(r chi.Router) {
		r.Get("/igdb", h.GetIgdb)
		r.Post("/igdb", h.SaveIgdb)
		r.Post("/igdb/test", h.TestIgdb)
		r.Delete("/igdb", h.DeleteIgdb)

		r.Get("/steam", h.GetSteam)
		r.Post("/steam", h.SaveSteam)
		r.Delete("/steam", h.DeleteSteam)
		r.Post("/steam/test", h.TestSteam)
		r.Post("/steam/sync", h.SyncSteam)

		r.Get("/steamgriddb", h.GetSteamGridDB)
		r.Post("/steamgriddb", h.SaveSteamGridDB)
		r.Delete("/steamgriddb", h.DeleteSteamGridDB)

		r.Get("/prowlarr", h.GetProwlarr)
		r.Post("/prowlarr", h.SaveProwlarr)

		r.Get("/jackett", h.GetJackett)
		r.Post("/jackett", h.SaveJackett)

		r.Get("/postdownload", h.GetPostDownload)
		r.Post("/postdownload", h.SavePostDownload)

		r.Get("/server", h.GetServer)
		r.Post("/server", h.SaveServer)

		// Agent settings (masked token, browser-accessible)
		r.Get("/agent", h.GetAgentSettings)

		r.Get("/discord", h.GetDiscord)
		r.Post("/discord", h.SaveDiscord)

		r.Get("/morrenus", h.GetMorrenus)
		r.Post("/morrenus", h.SaveMorrenus)
		r.Delete("/morrenus", h.DeleteMorrenus)
	})

	// Media (scan)
	r.Route("/api/v3/media", func(r chi.Router) {
		r.Get("/", h.GetMediaSettings)
		r.Post("/", h.SaveMediaSettings)
		r.Post("/scan", h.TriggerScan)
		r.Post("/scan/stop", h.StopScan)
		r.Get("/scan/status", h.GetScanStatus)
		r.Delete("/clean", h.CleanLibrary)
	})

	// Download clients
	r.Route("/api/v3/downloadclient", func(r chi.Router) {
		r.Get("/", h.GetDownloadClients)
		r.Post("/", h.CreateDownloadClient)
		r.Put("/{id}", h.UpdateDownloadClient)
		r.Delete("/{id}", h.DeleteDownloadClient)
		r.Get("/{id}", h.GetDownloadClient)

		r.Post("/test", h.TestDownloadClient)
		r.Post("/add", h.AddTorrent)

		r.Get("/queue", h.GetQueue)
		r.Delete("/queue/{clientId}/{downloadId}", h.DeleteDownload)
		r.Post("/queue/{clientId}/{downloadId}/pause", h.PauseDownload)
		r.Post("/queue/{clientId}/{downloadId}/resume", h.ResumeDownload)
	})

	// Hydra indexers
	r.Get("/api/v3/hydra", h.GetHydra)
	r.Post("/api/v3/hydra", h.SaveHydra)

	// Search (torrent/NZB indexers)
	r.Get("/api/v3/search", h.Search)
	r.Post("/api/v3/search/test", h.SearchTest)

	// Explore (folder browser)
	r.Get("/api/v3/explore", h.Explore)

	// Filesystem
	r.Get("/api/v3/filesystem", h.FilesystemList)
	r.Get("/api/v3/filesystem/folder", h.ListFolder)

	// Save snapshots
	r.Route("/api/v3/save", func(r chi.Router) {
		// Save path lookup (agent uses this to know what to watch/upload)
		r.Get("/paths", h.GetSavePaths)
		// Path info for browser display (includes source + detected paths)
		r.Get("/paths-info", h.GetSavePathsInfo)
		// Agent uploads snapshot (bearer token required)
		r.With(h.agentAuthMiddleware).Post("/snapshot", h.UploadSaveSnapshot)
		// Browser: list and delete snapshots
		r.Get("/{gameId}", h.ListSaveSnapshots)
		r.Delete("/{gameId}/{*}", h.DeleteSaveSnapshot)
		// Latest save across all agents (agent restore + browser info)
		r.With(h.agentAuthMiddleware).Get("/{gameId}/latest", h.ServeLatestSave)
		r.Get("/{gameId}/latest-info", h.GetLatestSaveInfo)
		// Custom save path override (supports agentId in body for per-device)
		r.Patch("/{gameId}/path", h.SetSavePath)
		// Per-device save path overrides (map of agentId → path)
		r.Get("/{gameId}/agent-paths", h.GetAgentSavePaths)
		// Conflict resolution: promote a specific agent's snapshot as globally latest
		r.Post("/{gameId}/promote-snapshot", h.PromoteAgentSnapshot)
	})

	// Agent API
	r.Route("/api/v3/agent", func(r chi.Router) {
		// Browser-accessible (no agent auth required)
		r.Get("/", h.ListAgents)
		r.Get("/binary", h.ServeAgentBinary)
		r.Get("/version", h.AgentVersion)
		r.Get("/setup.sh", h.ServeAgentSetupScript)

		// CHAP registration — no middleware needed; CHAP is validated inside handler
		r.Post("/register", h.RegisterAgent)
		r.With(h.agentAuthMiddleware).Get("/events", h.AgentEvents)
		r.With(h.agentAuthMiddleware).Post("/progress", h.AgentProgress)
		r.With(h.agentAuthMiddleware).Get("/artwork", h.GetArtworkURLs)

		// Browser: remove an agent permanently
		r.Delete("/{agentId}", h.DeleteAgent)

		// Browser dispatches install (no agent auth — same as all other browser endpoints)
		r.Get("/{agentId}/install-preview", h.GetInstallPreview)
		r.Post("/{agentId}/install", h.DispatchInstall)
		r.Post("/{agentId}/scan", h.DispatchScan)
		r.Post("/{agentId}/refresh-shortcuts", h.DispatchRefreshShortcuts)
		r.Delete("/{agentId}/game", h.DispatchDeleteGame)
		r.Post("/{agentId}/readlog", h.DispatchReadLog)
		r.Post("/{agentId}/readscript", h.DispatchReadScript)
		r.Post("/{agentId}/regen-scripts", h.DispatchRegenScripts)
		r.Post("/{agentId}/change-exe", h.DispatchChangeExe)
		r.Post("/{agentId}/restart-steam", h.DispatchRestartSteam)
		r.Post("/{agentId}/setup-slssteam", h.DispatchSetupSLSSteam)
		r.Post("/{agentId}/restore-save", h.DispatchRestoreSave)
		r.Post("/{agentId}/uninstall", h.DispatchUninstallAgent)
		r.Post("/{agentId}/upload-save", h.DispatchUploadSave)
		r.Post("/{agentId}/rename-prefix", h.DispatchRenamePrefix)
		// Agent-authenticated callbacks
		r.With(h.agentAuthMiddleware).Post("/{agentId}/games", h.ReportInstalledGames)
		r.With(h.agentAuthMiddleware).Post("/log", h.ReceiveAgentLog)
		r.With(h.agentAuthMiddleware).Post("/script", h.ReceiveAgentScript)
		r.With(h.agentAuthMiddleware).Post("/browse-result", h.ReceiveBrowseResult)
		r.With(h.agentAuthMiddleware).Post("/proton-result", h.ReceiveProtonResult)

		// Browser-facing: request a directory listing from an agent
		r.Get("/{agentId}/browse", h.BrowseAgentDir)

		// Browser-facing: list available Proton versions on an agent
		r.Get("/{agentId}/proton-versions", h.ListProtonVersions)
	})

	return r
}

func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	jsonOK(w, map[string]string{"status": "ok"})
}

// filteredLogger is a minimal request logger that skips high-frequency polling
// endpoints to keep Docker logs readable.
var silencedPaths = map[string]bool{
	"/api/v3/media/scan/status": true,
	"/api/v3/agent/progress":    true,
}

func filteredLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if silencedPaths[r.URL.Path] {
			next.ServeHTTP(w, r)
			return
		}
		start := time.Now()
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		next.ServeHTTP(ww, r)
		log.Printf("%s %s %d %s", r.Method, r.URL.Path, ww.Status(), time.Since(start))
	})
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" && isAllowedOrigin(origin, r.Host) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// isAllowedOrigin returns true if the CORS origin should be reflected.
// Allows same-host and localhost (for dev webpack server).
func isAllowedOrigin(origin, serverHost string) bool {
	originHost := originToHost(origin)
	if originHost == serverHost {
		return true
	}
	bare := originHost
	if i := strings.LastIndex(bare, ":"); i > strings.LastIndex(bare, "]") {
		bare = bare[:i]
	}
	return bare == "localhost" || bare == "127.0.0.1" || bare == "::1"
}

// originToHost strips the scheme from an Origin header value.
func originToHost(origin string) string {
	if i := strings.Index(origin, "://"); i >= 0 {
		rest := origin[i+3:]
		if j := strings.IndexByte(rest, '/'); j >= 0 {
			return rest[:j]
		}
		return rest
	}
	return origin
}
