package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"playerr/internal/agent"
	"playerr/internal/config"
	"playerr/internal/monitor"
	"playerr/internal/repository"
	"playerr/internal/scanner"
	"playerr/internal/sse"
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
		r.Post("/{id}/play", h.PlayGame)
		r.Post("/{id}/install", h.InstallGame)
		r.Post("/{id}/uninstall", h.UninstallGame)
		r.Post("/{id}/shortcut", h.AddSteamShortcut)

		// File serving for agent downloads (requires agent auth)
		r.With(h.agentAuthMiddleware).Get("/{id}/file", h.ServeGameFile)

		// Install script download (browser, no auth needed)
		r.Get("/{id}/install-script", h.ServeInstallScript)
	})

	// Platforms
	r.Get("/api/v3/platform", h.GetAllPlatforms)
	r.Get("/api/v3/platform/{id}", h.GetPlatformByID)

	// Settings
	r.Route("/api/v3/settings", func(r chi.Router) {
		r.Get("/igdb", h.GetIgdb)
		r.Post("/igdb", h.SaveIgdb)
		r.Post("/igdb/test", h.TestIgdb)

		r.Get("/steam", h.GetSteam)
		r.Post("/steam", h.SaveSteam)

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

	// Agent API
	r.Route("/api/v3/agent", func(r chi.Router) {
		// Browser-accessible (no agent auth required)
		r.Get("/", h.ListAgents)

		// Agent-authenticated endpoints
		r.With(h.agentAuthMiddleware).Post("/register", h.RegisterAgent)
		r.With(h.agentAuthMiddleware).Get("/events", h.AgentEvents)
		r.With(h.agentAuthMiddleware).Post("/progress", h.AgentProgress)

		// Browser dispatches install (no agent auth — same as all other browser endpoints)
		r.Post("/{agentId}/install", h.DispatchInstall)
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
