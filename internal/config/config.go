package config

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"

	"github.com/kiwi3007/playerr/internal/domain"
)

// cryptoRandRead is a var so tests can override it.
var cryptoRandRead = rand.Read

// Service handles reading/writing all config/*.json files.
type Service struct {
	dir string
}

func NewService(contentRoot string) *Service {
	dir := filepath.Join(contentRoot, "config")
	if err := os.MkdirAll(dir, 0755); err != nil {
		log.Printf("[Config] Warning: could not create config dir: %v", err)
	}
	log.Printf("[Config] Using directory: %s", dir)
	return &Service{dir: dir}
}

func (s *Service) Dir() string { return s.dir }

// ---- helpers ----

func load[T any](path string, def T) T {
	data, err := os.ReadFile(path)
	if err != nil {
		return def
	}
	var v T
	if err := json.Unmarshal(data, &v); err != nil {
		log.Printf("[Config] Error parsing %s: %v", path, err)
		return def
	}
	return v
}

func save(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

// ---- Prowlarr ----

type ProwlarrSettings struct {
	Url    string `json:"url"`
	ApiKey string `json:"apiKey"`
}

func (s *Service) LoadProwlarr() ProwlarrSettings {
	return load(filepath.Join(s.dir, "prowlarr.json"), ProwlarrSettings{})
}
func (s *Service) SaveProwlarr(v ProwlarrSettings) error {
	return save(filepath.Join(s.dir, "prowlarr.json"), v)
}

// ---- Jackett ----

type JackettSettings struct {
	Url    string `json:"url"`
	ApiKey string `json:"apiKey"`
}

func (s *Service) LoadJackett() JackettSettings {
	return load(filepath.Join(s.dir, "jackett.json"), JackettSettings{})
}
func (s *Service) SaveJackett(v JackettSettings) error {
	return save(filepath.Join(s.dir, "jackett.json"), v)
}

// ---- IGDB ----

type IgdbSettings struct {
	ClientId     string `json:"clientId"`
	ClientSecret string `json:"clientSecret"`
}

func (s IgdbSettings) IsConfigured() bool {
	return s.ClientId != "" && s.ClientSecret != ""
}

func (s *Service) LoadIgdb() IgdbSettings {
	cfg := load(filepath.Join(s.dir, "igdb.json"), IgdbSettings{})
	if cfg.ClientId == "" {
		cfg.ClientId = os.Getenv("IGDB_CLIENT_ID")
	}
	if cfg.ClientSecret == "" {
		cfg.ClientSecret = os.Getenv("IGDB_CLIENT_SECRET")
	}
	return cfg
}
func (s *Service) SaveIgdb(v IgdbSettings) error {
	return save(filepath.Join(s.dir, "igdb.json"), v)
}

// ---- SteamGridDB ----

type SteamGridDBSettings struct {
	ApiKey string `json:"apiKey"`
}

func (s SteamGridDBSettings) IsConfigured() bool { return s.ApiKey != "" }

func (s *Service) LoadSteamGridDB() SteamGridDBSettings {
	return load(filepath.Join(s.dir, "steamgriddb.json"), SteamGridDBSettings{})
}
func (s *Service) SaveSteamGridDB(v SteamGridDBSettings) error {
	return save(filepath.Join(s.dir, "steamgriddb.json"), v)
}

// ---- Steam ----

type SteamSettings struct {
	ApiKey  string `json:"apiKey"`
	SteamId string `json:"steamId"`
}

func (s SteamSettings) IsConfigured() bool {
	return s.ApiKey != "" && s.SteamId != ""
}

func (s *Service) LoadSteam() SteamSettings {
	return load(filepath.Join(s.dir, "steam.json"), SteamSettings{})
}
func (s *Service) SaveSteam(v SteamSettings) error {
	return save(filepath.Join(s.dir, "steam.json"), v)
}

// ---- Media ----

type MediaSettings struct {
	FolderPath      string `json:"folderPath"`
	DownloadPath    string `json:"downloadPath"`
	DestinationPath string `json:"destinationPath"`
}

func (s *Service) LoadMedia() MediaSettings {
	m := load(filepath.Join(s.dir, "media.json"), MediaSettings{})

	home, _ := os.UserHomeDir()
	docs := filepath.Join(home, "Documents")
	downloads := filepath.Join(home, "Downloads")

	if m.FolderPath == "" {
		m.FolderPath = filepath.Join(docs, "Playerr", "Games")
	}
	if m.DownloadPath == "" {
		m.DownloadPath = filepath.Join(downloads, "Playerr")
	}
	if m.DestinationPath == "" {
		m.DestinationPath = filepath.Join(docs, "Playerr", "Library")
	}

	for _, p := range []string{m.FolderPath, m.DownloadPath, m.DestinationPath} {
		_ = os.MkdirAll(p, 0755)
	}
	return m
}
func (s *Service) SaveMedia(v MediaSettings) error {
	return save(filepath.Join(s.dir, "media.json"), v)
}

// ---- Post-Download ----

type PostDownloadSettings struct {
	EnableAutoMove      bool     `json:"enableAutoMove"`
	EnableAutoExtract   bool     `json:"enableAutoExtract"`
	EnableDeepClean     bool     `json:"enableDeepClean"`
	EnableAutoRename    bool     `json:"enableAutoRename"`
	MonitorIntervalSecs int      `json:"monitorIntervalSeconds"`
	UnwantedExtensions  []string `json:"unwantedExtensions"`
}

func defaultPostDownload() PostDownloadSettings {
	return PostDownloadSettings{
		EnableAutoMove:      true,
		EnableAutoExtract:   true,
		EnableDeepClean:     true,
		EnableAutoRename:    true,
		MonitorIntervalSecs: 60,
		UnwantedExtensions:  []string{".txt", ".nfo", ".url"},
	}
}

func (s *Service) LoadPostDownload() PostDownloadSettings {
	cfg := load(filepath.Join(s.dir, "postdownload.json"), defaultPostDownload())
	if cfg.MonitorIntervalSecs == 0 {
		cfg.MonitorIntervalSecs = 60
	}
	return cfg
}
func (s *Service) SavePostDownload(v PostDownloadSettings) error {
	return save(filepath.Join(s.dir, "postdownload.json"), v)
}

// ---- Download Clients ----

func (s *Service) LoadDownloadClients() []domain.DownloadClientConfig {
	return load(filepath.Join(s.dir, "downloadclients.json"), []domain.DownloadClientConfig{})
}
func (s *Service) SaveDownloadClients(v []domain.DownloadClientConfig) error {
	return save(filepath.Join(s.dir, "downloadclients.json"), v)
}

// ---- Hydra Indexers ----

type HydraConfig struct {
	ID      int    `json:"id"`
	Name    string `json:"name"`
	Url     string `json:"url"`
	ApiKey  string `json:"apiKey"`
	Enable  bool   `json:"enable"`
	Categories []int `json:"categories,omitempty"`
}

func (s *Service) LoadHydra() []HydraConfig {
	return load(filepath.Join(s.dir, "hydra.json"), []HydraConfig{})
}
func (s *Service) SaveHydra(v []HydraConfig) error {
	return save(filepath.Join(s.dir, "hydra.json"), v)
}

// ---- Server ----

type ServerSettings struct {
	Port             int    `json:"port"`
	UseAllInterfaces bool   `json:"useAllInterfaces"`
	UIPassword       string `json:"uiPassword"`
}

func defaultServer() ServerSettings {
	return ServerSettings{Port: 5002, UseAllInterfaces: false}
}

func (s *Service) LoadServer() ServerSettings {
	cfg := load(filepath.Join(s.dir, "server.json"), defaultServer())
	if cfg.Port == 0 {
		cfg.Port = 5002
	}
	return cfg
}
func (s *Service) SaveServer(v ServerSettings) error {
	return save(filepath.Join(s.dir, "server.json"), v)
}

// ---- Agent ----

type AgentSettings struct {
	Token string `json:"token"`
}

func (s *Service) LoadAgent() AgentSettings {
	cfg := load(filepath.Join(s.dir, "agent.json"), AgentSettings{})
	if cfg.Token == "" {
		cfg.Token = generateToken()
		_ = s.SaveAgent(cfg)
	}
	return cfg
}

func (s *Service) SaveAgent(v AgentSettings) error {
	return save(filepath.Join(s.dir, "agent.json"), v)
}

func generateToken() string {
	b := make([]byte, 16)
	if _, err := cryptoRandRead(b); err != nil {
		// Fallback to a fixed string (shouldn't happen)
		return "changeme-please-restart"
	}
	return fmt.Sprintf("%x", b)
}

// ---- Discord ----

type DiscordSettings struct {
	WebhookURL        string `json:"webhookUrl"`
	CheckIntervalHours int   `json:"checkIntervalHours"`
}

func defaultDiscord() DiscordSettings {
	return DiscordSettings{CheckIntervalHours: 24}
}

func (s *Service) LoadDiscord() DiscordSettings {
	cfg := load(filepath.Join(s.dir, "discord.json"), defaultDiscord())
	if cfg.CheckIntervalHours == 0 {
		cfg.CheckIntervalHours = 24
	}
	return cfg
}
func (s *Service) SaveDiscord(v DiscordSettings) error {
	return save(filepath.Join(s.dir, "discord.json"), v)
}

// ---- Utility ----

// FindConfigDir walks up from execDir looking for a config/ folder.
func FindConfigDir(execDir string) string {
	candidate := execDir
	for i := 0; i < 10; i++ {
		check := filepath.Join(candidate, "config")
		if _, err := os.Stat(check); err == nil {
			return candidate
		}
		parent := filepath.Dir(candidate)
		if parent == candidate {
			break
		}
		candidate = parent
	}

	// Fallback: use OS application data dir
	var appData string
	switch runtime.GOOS {
	case "windows":
		appData = os.Getenv("APPDATA")
	case "darwin":
		home, _ := os.UserHomeDir()
		appData = filepath.Join(home, "Library", "Application Support")
	default:
		home, _ := os.UserHomeDir()
		appData = filepath.Join(home, ".config")
	}
	if appData != "" {
		return filepath.Join(appData, "Playerr")
	}
	return execDir
}
