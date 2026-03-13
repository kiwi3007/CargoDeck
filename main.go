package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/kiwi3007/cargodeck/internal/agent"
	"github.com/kiwi3007/cargodeck/internal/api"
	"github.com/kiwi3007/cargodeck/internal/config"
	dbpkg "github.com/kiwi3007/cargodeck/internal/db"
	"github.com/kiwi3007/cargodeck/internal/manifest"
	"github.com/kiwi3007/cargodeck/internal/monitor"
	"github.com/kiwi3007/cargodeck/internal/repository"
	"github.com/kiwi3007/cargodeck/internal/scanner"
	"github.com/kiwi3007/cargodeck/internal/sse"
	"github.com/kiwi3007/cargodeck/internal/updater"
)

var version = "dev"

func main() {
	// ---- Resolve paths ----
	execDir, err := os.Executable()
	if err != nil {
		execDir = "."
	}
	execDir = filepath.Dir(execDir)

	// Allow override via env
	if d := os.Getenv("CARGODECK_CONFIG_DIR"); d != "" {
		execDir = d
	}

	contentRoot := config.FindConfigDir(execDir)
	cfg := config.NewService(contentRoot)

	// ---- Database ----
	dbPath := filepath.Join(cfg.Dir(), "cargodeck.db")
	db, err := dbpkg.Open(dbPath)
	if err != nil {
		log.Fatalf("[DB] Failed to open database: %v", err)
	}
	defer db.Close()

	// ---- Services ----
	repo := repository.NewGameRepository(db)
	broker := sse.NewBroker()
	scan := scanner.New(repo, broker)
	importStatus := monitor.NewImportStatus()
	linkStore := monitor.NewDownloadLinkStore(cfg.Dir())
	processor := monitor.NewProcessor(cfg, repo, broker, linkStore)
	dlMonitor := monitor.NewDownloadMonitor(cfg, importStatus, processor)

	// Ensure agent token is generated on first start
	cfg.LoadAgent()

	// ---- Agent components ----
	agentRegistry := agent.NewRegistry(cfg.Dir())
	agentJobs := agent.NewJobQueue()
	agentBroker := agent.NewAgentBroker()

	// ---- Manifest (Ludusavi save paths) ----
	manifestSvc := manifest.NewService(cfg.Dir())

	// ---- Update checker ----
	updateChecker := updater.NewChecker(repo, cfg, broker)

	// ---- API ----
	handler := api.NewHandler(repo, cfg, broker, scan, importStatus, processor, linkStore, agentRegistry, agentJobs, agentBroker, manifestSvc, updateChecker)
	router := handler.NewRouter()

	// ---- Static UI ----
	uiPath := findUIPath(contentRoot)
	if uiPath != "" {
		log.Printf("[UI] Serving static files from: %s", uiPath)
		router = withStaticFiles(router, uiPath)
	} else {
		log.Println("[UI] Warning: UI directory not found")
	}

	// ---- Server binding ----
	serverCfg := cfg.LoadServer()
	port := serverCfg.Port
	if p := os.Getenv("CARGODECK_PORT"); p != "" {
		fmt.Sscanf(p, "%d", &port)
	}
	bindIP := "127.0.0.1"
	if serverCfg.UseAllInterfaces || os.Getenv("DOTNET_RUNNING_IN_CONTAINER") == "true" {
		bindIP = "0.0.0.0"
	}
	if ip := os.Getenv("CARGODECK_IP"); ip != "" {
		bindIP = ip
	}

	addr, err := bindPort(bindIP, port)
	if err != nil {
		log.Fatalf("[Server] Cannot bind: %v", err)
	}

	srv := &http.Server{
		Addr:         addr,
		Handler:      router,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 0, // SSE needs no write timeout
		IdleTimeout:  120 * time.Second,
	}

	// ---- Start background services ----
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go dlMonitor.Run(ctx)
	go handler.RunQueueBroadcaster(ctx)
	go updateChecker.Run(ctx)

	// ---- Serve ----
	log.Printf("[Server] CargoDeck backend listening on http://%s", addr)

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("[Server] Fatal: %v", err)
		}
	}()

	// ---- Graceful shutdown ----
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("[Server] Shutting down...")
	cancel()
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	srv.Shutdown(shutdownCtx)
	log.Println("[Server] Stopped.")
}

// bindPort tries to listen on the preferred port, then falls back to 5003/5004/5005/dynamic.
func bindPort(ip string, preferred int) (string, error) {
	ports := []int{preferred, 5003, 5004, 5005, 0}
	for _, p := range ports {
		addr := fmt.Sprintf("%s:%d", ip, p)
		l, err := net.Listen("tcp", addr)
		if err != nil {
			continue
		}
		l.Close()
		if p == 0 {
			// dynamic: listen to get assigned port
			l2, err := net.Listen("tcp", fmt.Sprintf("%s:0", ip))
			if err != nil {
				return "", err
			}
			actual := l2.Addr().String()
			l2.Close()
			return actual, nil
		}
		return addr, nil
	}
	return "", fmt.Errorf("no available port found")
}

func findUIPath(contentRoot string) string {
	candidates := []string{
		filepath.Join(contentRoot, "_output", "UI"),
		filepath.Join(contentRoot, "UI"),
		"_output/UI",
	}
	for _, p := range candidates {
		if fi, err := os.Stat(p); err == nil && fi.IsDir() {
			return p
		}
	}
	return ""
}

func withStaticFiles(next http.Handler, uiPath string) http.Handler {
	fs := http.FileServer(http.Dir(uiPath))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// API and SSE routes go to the chi router
		if len(r.URL.Path) > 4 && r.URL.Path[:4] == "/api" {
			next.ServeHTTP(w, r)
			return
		}
		if r.URL.Path == "/health" {
			next.ServeHTTP(w, r)
			return
		}
		// Check if the file exists in the UI path
		fpath := filepath.Join(uiPath, r.URL.Path)
		if fi, err := os.Stat(fpath); err == nil && !fi.IsDir() {
			fs.ServeHTTP(w, r)
			return
		}
		// SPA fallback: serve index.html
		http.ServeFile(w, r, filepath.Join(uiPath, "index.html"))
	})
}
