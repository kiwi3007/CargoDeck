package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/kiwi3007/playerr/internal/agent"
	"github.com/kiwi3007/playerr/internal/config"
	"github.com/kiwi3007/playerr/internal/manifest"
	"github.com/kiwi3007/playerr/internal/monitor"
	"github.com/kiwi3007/playerr/internal/repository"
	"github.com/kiwi3007/playerr/internal/scanner"
	"github.com/kiwi3007/playerr/internal/sse"
	"github.com/kiwi3007/playerr/internal/updater"
)

// Handler holds all dependencies for the API layer.
type Handler struct {
	repo          *repository.GameRepository
	cfg           *config.Service
	broker        *sse.Broker
	scanner       *scanner.Service
	importStatus  *monitor.ImportStatus
	agentRegistry *agent.Registry
	agentJobs     *agent.JobQueue
	agentBroker   *agent.AgentBroker
	manifest      *manifest.Service
	checker       *updater.Checker
}

func NewHandler(
	repo *repository.GameRepository,
	cfg *config.Service,
	broker *sse.Broker,
	scanner *scanner.Service,
	importStatus *monitor.ImportStatus,
	agentRegistry *agent.Registry,
	agentJobs *agent.JobQueue,
	agentBroker *agent.AgentBroker,
	manifestSvc *manifest.Service,
	checker *updater.Checker,
) *Handler {
	return &Handler{
		repo:          repo,
		cfg:           cfg,
		broker:        broker,
		scanner:       scanner,
		importStatus:  importStatus,
		agentRegistry: agentRegistry,
		agentJobs:     agentJobs,
		agentBroker:   agentBroker,
		manifest:      manifestSvc,
		checker:       checker,
	}
}

// NewRouter creates the chi router with all routes registered.
func (h *Handler) NewRouter() http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(corsMiddleware)

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

		// Manual update check for a single game
		r.Post("/{id}/check-update", h.CheckGameUpdate)

		// Bulk update check for all games with a known version
		r.Post("/check-update", h.CheckAllUpdates)
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
	})

	// Agent API
	r.Route("/api/v3/agent", func(r chi.Router) {
		// Browser-accessible (no agent auth required)
		r.Get("/", h.ListAgents)
		r.Get("/binary", h.ServeAgentBinary)
		r.Get("/version", h.AgentVersion)
		r.Get("/setup.sh", h.ServeAgentSetupScript)

		// Agent-authenticated endpoints
		r.With(h.agentAuthMiddleware).Post("/register", h.RegisterAgent)
		r.With(h.agentAuthMiddleware).Get("/events", h.AgentEvents)
		r.With(h.agentAuthMiddleware).Post("/progress", h.AgentProgress)
		r.With(h.agentAuthMiddleware).Get("/artwork", h.GetArtworkURLs)

		// Browser dispatches install (no agent auth — same as all other browser endpoints)
		r.Get("/{agentId}/install-preview", h.GetInstallPreview)
		r.Post("/{agentId}/install", h.DispatchInstall)
		r.Post("/{agentId}/scan", h.DispatchScan)
		r.Post("/{agentId}/refresh-shortcuts", h.DispatchRefreshShortcuts)
		r.Delete("/{agentId}/game", h.DispatchDeleteGame)
		r.Post("/{agentId}/readlog", h.DispatchReadLog)
		r.Post("/{agentId}/regen-scripts", h.DispatchRegenScripts)
		r.Post("/{agentId}/change-exe", h.DispatchChangeExe)
		r.Post("/{agentId}/restart-steam", h.DispatchRestartSteam)
		r.Post("/{agentId}/restore-save", h.DispatchRestoreSave)

		// Agent-authenticated callbacks
		r.With(h.agentAuthMiddleware).Post("/{agentId}/games", h.ReportInstalledGames)
		r.With(h.agentAuthMiddleware).Post("/log", h.ReceiveAgentLog)
	})

	return r
}

func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	jsonOK(w, map[string]string{"status": "ok"})
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}
		next.ServeHTTP(w, r)
	})
}
