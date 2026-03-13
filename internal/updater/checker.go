package updater

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/kiwi3007/playerr/internal/config"
	"github.com/kiwi3007/playerr/internal/domain"
	"github.com/kiwi3007/playerr/internal/indexer"
	"github.com/kiwi3007/playerr/internal/repository"
	"github.com/kiwi3007/playerr/internal/sse"
)

// Checker periodically searches indexers for newer versions of installed games.
type Checker struct {
	repo   *repository.GameRepository
	cfg    *config.Service
	broker *sse.Broker
}

func NewChecker(repo *repository.GameRepository, cfg *config.Service, broker *sse.Broker) *Checker {
	return &Checker{repo: repo, cfg: cfg, broker: broker}
}

// Run is the background goroutine entry point.
func (c *Checker) Run(ctx context.Context) {
	discord := c.cfg.LoadDiscord()
	interval := time.Duration(discord.CheckIntervalHours) * time.Hour
	if interval < time.Minute {
		interval = 24 * time.Hour
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Also run once at startup after a short delay to not block start-up
	select {
	case <-time.After(2 * time.Minute):
		c.CheckAll()
	case <-ctx.Done():
		return
	}

	for {
		// Re-read interval in case settings changed
		discord = c.cfg.LoadDiscord()
		newInterval := time.Duration(discord.CheckIntervalHours) * time.Hour
		if newInterval < time.Minute {
			newInterval = 24 * time.Hour
		}
		if newInterval != interval {
			interval = newInterval
			ticker.Reset(interval)
		}

		select {
		case <-ticker.C:
			c.CheckAll()
		case <-ctx.Done():
			return
		}
	}
}

// CheckAll checks every game that has a current_version set.
func (c *Checker) CheckAll() {
	games, err := c.repo.GetMonitoredGamesWithVersion()
	if err != nil {
		log.Printf("[Updater] GetMonitoredGamesWithVersion: %v", err)
		return
	}
	log.Printf("[Updater] Checking %d games for updates", len(games))
	for _, g := range games {
		if err := c.CheckGame(g); err != nil {
			log.Printf("[Updater] CheckGame %q: %v", g.Title, err)
		}
	}
}

// CheckGame searches indexers for a newer version of a single game.
func (c *Checker) CheckGame(game domain.Game) error {
	results, err := c.search(game.Title)
	if err != nil {
		return fmt.Errorf("search: %w", err)
	}

	bestCandidate := ""
	for _, r := range results {
		v := indexer.ParseVersionFromTitle(r.Title)
		if v == "" {
			continue
		}
		if bestCandidate == "" || indexer.IsNewerVersion(bestCandidate, v) {
			bestCandidate = v
		}
	}

	if bestCandidate != "" && indexer.IsNewerVersion(game.CurrentVersion, bestCandidate) {
		// New version found
		if err := c.repo.UpdateGameUpdateInfo(game.ID, bestCandidate, true); err != nil {
			return fmt.Errorf("UpdateGameUpdateInfo: %w", err)
		}
		log.Printf("[Updater] Update available for %q: %s → %s", game.Title, game.CurrentVersion, bestCandidate)

		// SSE notification
		payload, _ := json.Marshal(map[string]any{
			"id":             game.ID,
			"title":          game.Title,
			"currentVersion": game.CurrentVersion,
			"latestVersion":  bestCandidate,
		})
		c.broker.Publish("GAME_UPDATE_AVAILABLE", string(payload))

		// Discord webhook
		discord := c.cfg.LoadDiscord()
		if discord.WebhookURL != "" {
			go c.sendDiscord(discord.WebhookURL, game.Title, game.CurrentVersion, bestCandidate)
		}
	} else if bestCandidate == "" && game.UpdateAvailable {
		// No version found in results — reset the flag (game may have been re-installed at latest)
		_ = c.repo.UpdateGameUpdateInfo(game.ID, "", false)
	}
	return nil
}

// search queries Prowlarr (or falls back to Jackett) and returns up to 10 results.
func (c *Checker) search(title string) ([]indexer.SearchResult, error) {
	ctx := context.Background()
	prowlarr := c.cfg.LoadProwlarr()
	if prowlarr.Url != "" && prowlarr.ApiKey != "" {
		client := indexer.NewProwlarrClient(prowlarr.Url, prowlarr.ApiKey)
		results, err := client.Search(ctx, title, nil)
		if err == nil {
			return cap10(results), nil
		}
		log.Printf("[Updater] Prowlarr search error: %v", err)
	}

	jackett := c.cfg.LoadJackett()
	if jackett.Url != "" && jackett.ApiKey != "" {
		client := indexer.NewJackettClient(jackett.Url, jackett.ApiKey)
		results, err := client.Search(ctx, title, nil)
		if err == nil {
			return cap10(results), nil
		}
		log.Printf("[Updater] Jackett search error: %v", err)
	}

	return nil, fmt.Errorf("no indexer configured")
}

func cap10(results []indexer.SearchResult) []indexer.SearchResult {
	if len(results) > 10 {
		return results[:10]
	}
	return results
}

type discordEmbed struct {
	Title       string              `json:"title"`
	Description string              `json:"description"`
	Color       int                 `json:"color"`
	Footer      map[string]string   `json:"footer"`
}

func (c *Checker) sendDiscord(webhookURL, gameTitle, installed, latest string) {
	body := map[string]any{
		"embeds": []discordEmbed{
			{
				Title:       fmt.Sprintf("Update Available: %s", gameTitle),
				Description: fmt.Sprintf("**Installed:** v%s\n**Available:** v%s\n\nA newer release was found on your indexers.", installed, latest),
				Color:       5814783,
				Footer:      map[string]string{"text": "Playerr"},
			},
		},
	}
	data, err := json.Marshal(body)
	if err != nil {
		return
	}
	resp, err := http.Post(webhookURL, "application/json", bytes.NewReader(data)) //nolint:noctx
	if err != nil {
		log.Printf("[Updater] Discord webhook error: %v", err)
		return
	}
	resp.Body.Close()
}
