package igdb

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	twitchTokenURL = "https://id.twitch.tv/oauth2/token"
	igdbBaseURL    = "https://api.igdb.com/v4"
	igdbFields     = "name, summary, storyline, cover.image_id, screenshots.image_id, artworks.image_id, first_release_date, total_rating, total_rating_count, genres.name, involved_companies.company.name, involved_companies.developer, involved_companies.publisher, external_games.category, external_games.uid, platforms.name, platforms.abbreviation"
)

// ImageSize represents IGDB image size identifiers.
type ImageSize string

const (
	SizeCoverBig      ImageSize = "cover_big"
	SizeScreenshotHuge ImageSize = "screenshot_huge"
	SizeHD            ImageSize = "720p"
)

// ImageURL returns a full IGDB image URL for the given image_id and size.
func ImageURL(imageID string, size ImageSize) string {
	if imageID == "" {
		return ""
	}
	return fmt.Sprintf("https://images.igdb.com/igdb/image/upload/t_%s/%s.jpg", size, imageID)
}

// Client is a thread-safe IGDB API client.
type Client struct {
	clientID     string
	clientSecret string
	httpClient   *http.Client

	mu          sync.Mutex
	accessToken string
	tokenExpiry time.Time
}

func NewClient(clientID, clientSecret string) *Client {
	return &Client{
		clientID:     clientID,
		clientSecret: clientSecret,
		httpClient:   &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *Client) ensureToken() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.accessToken != "" && time.Now().Before(c.tokenExpiry) {
		return nil
	}

	params := url.Values{}
	params.Set("client_id", c.clientID)
	params.Set("client_secret", c.clientSecret)
	params.Set("grant_type", "client_credentials")

	resp, err := c.httpClient.PostForm(twitchTokenURL, params)
	if err != nil {
		return fmt.Errorf("igdb auth: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("igdb auth status %d: %s", resp.StatusCode, body)
	}

	var tok struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tok); err != nil {
		return fmt.Errorf("igdb auth decode: %w", err)
	}

	c.accessToken = tok.AccessToken
	c.tokenExpiry = time.Now().Add(time.Duration(tok.ExpiresIn-60) * time.Second)
	return nil
}

func (c *Client) post(endpoint, body string) ([]byte, error) {
	if err := c.ensureToken(); err != nil {
		return nil, err
	}

	req, err := http.NewRequest(http.MethodPost, igdbBaseURL+endpoint, strings.NewReader(body))
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	tok := c.accessToken
	c.mu.Unlock()

	req.Header.Set("Client-ID", c.clientID)
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Content-Type", "text/plain")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("igdb request: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("igdb %s status %d: %s", endpoint, resp.StatusCode, data)
	}
	return data, nil
}

// SearchGames searches IGDB by title, optionally filtered by platform ID.
func (c *Client) SearchGames(query string, platformID *int) ([]Game, error) {
	var body string
	if platformID != nil {
		body = fmt.Sprintf(`fields %s; search %q; where platforms = (%d) & version_parent = null; limit 100;`, igdbFields, query, *platformID)
	} else {
		body = fmt.Sprintf(`fields %s; search %q; where version_parent = null; limit 100;`, igdbFields, query)
	}

	data, err := c.post("/games", body)
	if err != nil {
		return nil, err
	}
	var games []Game
	if err := json.Unmarshal(data, &games); err != nil {
		return nil, fmt.Errorf("igdb search decode: %w", err)
	}
	return games, nil
}

// GetGamesByIds fetches full metadata for a list of IGDB game IDs.
func (c *Client) GetGamesByIds(ids []int) ([]Game, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	parts := make([]string, len(ids))
	for i, id := range ids {
		parts[i] = fmt.Sprintf("%d", id)
	}
	body := fmt.Sprintf(`fields %s; where id = (%s); limit 50;`, igdbFields, strings.Join(parts, ","))

	data, err := c.post("/games", body)
	if err != nil {
		return nil, err
	}
	var games []Game
	if err := json.Unmarshal(data, &games); err != nil {
		return nil, fmt.Errorf("igdb getbyids decode: %w", err)
	}
	return games, nil
}

// TestConnection verifies the IGDB credentials by fetching a token.
func (c *Client) TestConnection() error {
	// Force a fresh token fetch
	c.mu.Lock()
	c.accessToken = ""
	c.mu.Unlock()
	return c.ensureToken()
}
