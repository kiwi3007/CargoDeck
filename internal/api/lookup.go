package api

import (
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/kiwi3007/playerr/internal/domain"
	"github.com/kiwi3007/playerr/internal/metadata/igdb"
)

// platformKeyToIgdbID maps frontend platformKey strings to IGDB platform IDs.
var platformKeyToIgdbID = map[string]int{
	"nintendo_switch": 130,
	"ps4":             48,
	"ps3":             9,
	"ps5":             167,
	"pc_windows":      6,
	"macos":           14,
}

// GameLookup handles GET /api/v3/game/lookup?term=&platformKey=&lang=
func (h *Handler) GameLookup(w http.ResponseWriter, r *http.Request) {
	term := r.URL.Query().Get("term")
	if strings.TrimSpace(term) == "" {
		jsonErr(w, 400, "term is required")
		return
	}

	igdbCfg := h.cfg.LoadIgdb()
	if !igdbCfg.IsConfigured() {
		// Return empty list if IGDB not configured
		jsonOK(w, []any{})
		return
	}

	client := igdb.NewClient(igdbCfg.ClientId, igdbCfg.ClientSecret)

	platformKey := r.URL.Query().Get("platformKey")
	var platformID *int
	if id, ok := platformKeyToIgdbID[strings.ToLower(platformKey)]; ok {
		platformID = &id
	}

	games, err := client.SearchGames(term, platformID)
	if err != nil {
		log.Printf("[Lookup] IGDB search error: %v", err)
		jsonErr(w, 500, err.Error())
		return
	}

	lang := r.URL.Query().Get("lang")

	// Check which IGDB IDs are already in the library
	ownedIds, _ := h.repo.GetIgdbIds()

	results := make([]any, 0, len(games))
	for _, g := range games {
		mapped := mapIgdbGame(g, lang)
		if _, owned := ownedIds[g.ID]; owned {
			mapped["isOwned"] = true
		}
		results = append(results, mapped)
	}

	jsonOK(w, results)
}

// GameLookupByIgdbID handles GET /api/v3/game/lookup/igdb/{igdbId}
func (h *Handler) GameLookupByIgdbID(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "igdbId")
	igdbID, err := strconv.Atoi(idStr)
	if err != nil {
		jsonErr(w, 400, "invalid igdbId")
		return
	}

	igdbCfg := h.cfg.LoadIgdb()
	if !igdbCfg.IsConfigured() {
		jsonErr(w, 503, "IGDB not configured")
		return
	}

	client := igdb.NewClient(igdbCfg.ClientId, igdbCfg.ClientSecret)
	games, err := client.GetGamesByIds([]int{igdbID})
	if err != nil {
		log.Printf("[Lookup] IGDB getbyid error: %v", err)
		jsonErr(w, 500, err.Error())
		return
	}
	if len(games) == 0 {
		jsonErr(w, 404, "not found")
		return
	}

	lang := r.URL.Query().Get("lang")
	jsonOK(w, mapIgdbGame(games[0], lang))
}

// RefreshGameMetadata re-fetches IGDB data for a game and updates metadata fields
// (title, overview, genres, rating, images, Steam ID, developer, publisher, release date).
// POST /api/v3/game/{id}/refresh-metadata
func (h *Handler) RefreshGameMetadata(w http.ResponseWriter, r *http.Request) {
	id, err := paramInt(r, "id")
	if err != nil {
		jsonErr(w, 400, "invalid id")
		return
	}

	game, err := h.repo.GetGameByID(id)
	if err != nil {
		jsonErr(w, 404, "game not found")
		return
	}

	if game.IgdbID == nil || *game.IgdbID == 0 {
		jsonErr(w, 400, "game has no IGDB ID — search for it first to link it")
		return
	}

	igdbCfg := h.cfg.LoadIgdb()
	if !igdbCfg.IsConfigured() {
		jsonErr(w, 503, "IGDB not configured")
		return
	}

	client := igdb.NewClient(igdbCfg.ClientId, igdbCfg.ClientSecret)
	games, err := client.GetGamesByIds([]int{*game.IgdbID})
	if err != nil {
		log.Printf("[Lookup] RefreshGameMetadata IGDB error: %v", err)
		jsonErr(w, 500, err.Error())
		return
	}
	if len(games) == 0 {
		jsonErr(w, 404, "IGDB returned no result for this game ID")
		return
	}

	g := games[0]
	applyIgdbToGame(g, game)

	updated, err := h.repo.UpdateGame(id, game)
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	log.Printf("[Lookup] Refreshed metadata for game %d (%s) steamId=%v", id, game.Title, game.SteamID)
	jsonOK(w, updated)
}

// applyIgdbToGame copies IGDB fields onto an existing domain.Game in place.
// Only overwrites fields that IGDB actually returned (non-zero/non-empty).
func applyIgdbToGame(g igdb.Game, game *domain.Game) {
	if g.Name != "" {
		game.Title = g.Name
	}
	if g.Summary != "" {
		s := g.Summary
		game.Overview = &s
	}
	if len(g.Genres) > 0 {
		genres := make([]string, 0, len(g.Genres))
		for _, genre := range g.Genres {
			genres = append(genres, genre.Name)
		}
		game.Genres = genres
	}
	if g.Rating != nil {
		game.Rating = g.Rating
	}
	if g.RatingCount != nil {
		game.RatingCount = g.RatingCount
	}
	if g.FirstReleaseDate != nil {
		t := time.Unix(*g.FirstReleaseDate, 0).UTC()
		game.ReleaseDate = &t
		y := t.Year()
		game.Year = y
	}
	for _, ic := range g.InvolvedCompanies {
		if ic.Developer && game.Developer == nil {
			name := ic.Company.Name
			game.Developer = &name
		}
		if ic.Publisher && game.Publisher == nil {
			name := ic.Company.Name
			game.Publisher = &name
		}
	}
	// Steam ID from external_games (category 1 = Steam)
	for _, eg := range g.ExternalGames {
		if eg.Category == 1 {
			if sid, err := strconv.Atoi(eg.UID); err == nil {
				game.SteamID = &sid
			}
			break
		}
	}
	// Images
	if g.Cover != nil {
		coverURL := igdb.ImageURL(g.Cover.ImageID, igdb.SizeCoverBig)
		coverLargeURL := igdb.ImageURL(g.Cover.ImageID, igdb.SizeHD)
		game.Images.CoverUrl = &coverURL
		game.Images.CoverLargeUrl = &coverLargeURL
	}
	screenshots := make([]string, 0, len(g.Screenshots))
	for _, s := range g.Screenshots {
		screenshots = append(screenshots, igdb.ImageURL(s.ImageID, igdb.SizeScreenshotHuge))
	}
	if len(screenshots) > 0 {
		game.Images.Screenshots = screenshots
	}
	artworks := make([]string, 0, len(g.Artworks))
	for _, a := range g.Artworks {
		artworks = append(artworks, igdb.ImageURL(a.ImageID, igdb.SizeHD))
	}
	if len(artworks) > 0 {
		game.Images.Artworks = artworks
		game.Images.BackgroundUrl = &artworks[0]
	} else if len(screenshots) > 0 {
		game.Images.BackgroundUrl = &screenshots[0]
	}
}

// mapIgdbGame converts an igdb.Game to the same Game shape the frontend expects.
func mapIgdbGame(g igdb.Game, lang string) map[string]any {
	coverURL := ""
	coverLargeURL := ""
	if g.Cover != nil {
		coverURL = igdb.ImageURL(g.Cover.ImageID, igdb.SizeCoverBig)
		coverLargeURL = igdb.ImageURL(g.Cover.ImageID, igdb.SizeHD)
	}

	screenshots := []string{}
	for _, s := range g.Screenshots {
		screenshots = append(screenshots, igdb.ImageURL(s.ImageID, igdb.SizeScreenshotHuge))
	}

	artworks := []string{}
	for _, a := range g.Artworks {
		artworks = append(artworks, igdb.ImageURL(a.ImageID, igdb.SizeHD))
	}

	bgURL := ""
	if len(artworks) > 0 {
		bgURL = artworks[0]
	} else if len(screenshots) > 0 {
		bgURL = screenshots[0]
	}

	genres := []string{}
	for _, genre := range g.Genres {
		genres = append(genres, genre.Name)
	}

	platforms := []string{}
	for _, p := range g.Platforms {
		if p.Abbreviation != "" {
			platforms = append(platforms, p.Abbreviation)
		} else {
			platforms = append(platforms, p.Name)
		}
	}

	var developer, publisher *string
	for _, ic := range g.InvolvedCompanies {
		if ic.Developer && developer == nil {
			name := ic.Company.Name
			developer = &name
		}
		if ic.Publisher && publisher == nil {
			name := ic.Company.Name
			publisher = &name
		}
	}

	var releaseDate *string
	var year *int
	if g.FirstReleaseDate != nil {
		t := time.Unix(*g.FirstReleaseDate, 0).UTC()
		s := t.Format(time.RFC3339)
		releaseDate = &s
		y := t.Year()
		year = &y
	}

	// Steam ID from external_games (category 1 = Steam)
	var steamID *int
	for _, eg := range g.ExternalGames {
		if eg.Category == 1 {
			if id, err := strconv.Atoi(eg.UID); err == nil {
				steamID = &id
			}
			break
		}
	}

	_ = lang // lang-based genre translation omitted for simplicity

	m := map[string]any{
		"id":                 0, // not yet in library
		"igdbId":            g.ID,
		"title":             g.Name,
		"overview":          g.Summary,
		"storyline":         g.Storyline,
		"genres":            genres,
		"availablePlatforms": platforms,
		"developer":         developer,
		"publisher":         publisher,
		"releaseDate":       releaseDate,
		"year":              year,
		"rating":            g.Rating,
		"ratingCount":       g.RatingCount,
		"steamId":           steamID,
		"status":            domain.GameStatusTBA,
		"monitored":         false,
		"isOwned":           false,
		"images": map[string]any{
			"coverUrl":      coverURL,
			"coverLargeUrl": coverLargeURL,
			"backgroundUrl": bgURL,
			"bannerUrl":     "",
			"screenshots":   screenshots,
			"artworks":      artworks,
		},
	}
	return m
}
