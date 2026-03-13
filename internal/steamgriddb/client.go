package steamgriddb

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const baseURL = "https://www.steamgriddb.com/api/v2"

type Client struct {
	apiKey string
	http   *http.Client
}

func NewClient(apiKey string) *Client {
	return &Client{apiKey: apiKey, http: &http.Client{Timeout: 20 * time.Second}}
}

func (c *Client) get(path string, out any) error {
	req, err := http.NewRequest(http.MethodGet, baseURL+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("steamgriddb %s: %d %s", path, resp.StatusCode, body)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// SearchGame returns the SteamGridDB game ID for the best match.
func (c *Client) SearchGame(name string) (int, error) {
	var result struct {
		Data []struct {
			ID   int    `json:"id"`
			Name string `json:"name"`
		} `json:"data"`
	}
	if err := c.get("/search/autocomplete/"+url.PathEscape(name), &result); err != nil {
		return 0, err
	}
	if len(result.Data) == 0 {
		return 0, fmt.Errorf("no results for %q", name)
	}
	return result.Data[0].ID, nil
}

type imageResult struct {
	Data []struct {
		URL string `json:"url"`
	} `json:"data"`
}

func (c *Client) fetchFirstURL(path string) (string, error) {
	var result imageResult
	if err := c.get(path, &result); err != nil {
		return "", err
	}
	if len(result.Data) == 0 {
		return "", fmt.Errorf("no images at %s", path)
	}
	return result.Data[0].URL, nil
}

func (c *Client) download(imageURL, destPath string) error {
	resp, err := c.http.Get(imageURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		return err
	}
	f, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, resp.Body)
	return err
}

func ext(imageURL string) string {
	u := strings.Split(imageURL, "?")[0]
	e := filepath.Ext(u)
	if e == "" {
		return ".png"
	}
	return e
}

// ImageURLs holds the SteamGridDB CDN URLs for each artwork type.
type ImageURLs struct {
	Portrait  string `json:"portrait"`  // 600×900 cover
	Landscape string `json:"landscape"` // 920×430 wide capsule
	Hero      string `json:"hero"`      // 1920×620 hero banner
	Logo      string `json:"logo"`
	Icon      string `json:"icon"` // square icon (for shortcut icon field)
}

// GetImageURLs resolves SteamGridDB CDN URLs for the game without downloading.
// Callers (e.g. agents) can download the images themselves.
func (c *Client) GetImageURLs(gameName string) (*ImageURLs, error) {
	sgdbID, err := c.SearchGame(gameName)
	if err != nil {
		return nil, err
	}
	urls := &ImageURLs{}
	if u, err := c.fetchFirstURL(fmt.Sprintf("/grids/game/%d?dimensions=600x900", sgdbID)); err == nil {
		urls.Portrait = u
	}
	// Prefer 920x430 (modern Steam wide capsule); fall back to 460x215
	if u, err := c.fetchFirstURL(fmt.Sprintf("/grids/game/%d?dimensions=920x430", sgdbID)); err == nil {
		urls.Landscape = u
	} else if u, err := c.fetchFirstURL(fmt.Sprintf("/grids/game/%d?dimensions=460x215", sgdbID)); err == nil {
		urls.Landscape = u
	}
	if u, err := c.fetchFirstURL(fmt.Sprintf("/heroes/game/%d", sgdbID)); err == nil {
		urls.Hero = u
	}
	if u, err := c.fetchFirstURL(fmt.Sprintf("/logos/game/%d", sgdbID)); err == nil {
		urls.Logo = u
	}
	if u, err := c.fetchFirstURL(fmt.Sprintf("/icons/game/%d", sgdbID)); err == nil {
		urls.Icon = u
	}
	return urls, nil
}

// FetchImages downloads grid/hero/logo images into gridDir, named by Steam appId convention.
// Non-fatal: individual image failures are logged but don't return an error.
func (c *Client) FetchImages(gameName string, appID uint32, gridDir string) error {
	sgdbID, err := c.SearchGame(gameName)
	if err != nil {
		return fmt.Errorf("search: %w", err)
	}

	idStr := fmt.Sprintf("%d", appID)

	// Portrait cover (600x900) → {appId}p.ext
	if imgURL, err := c.fetchFirstURL(fmt.Sprintf("/grids/game/%d?dimensions=600x900", sgdbID)); err == nil {
		_ = c.download(imgURL, filepath.Join(gridDir, idStr+"p"+ext(imgURL)))
	}

	// Wide capsule (920x430 preferred, 460x215 fallback) → {appId}.ext
	if imgURL, err := c.fetchFirstURL(fmt.Sprintf("/grids/game/%d?dimensions=920x430", sgdbID)); err == nil {
		_ = c.download(imgURL, filepath.Join(gridDir, idStr+ext(imgURL)))
	} else if imgURL, err := c.fetchFirstURL(fmt.Sprintf("/grids/game/%d?dimensions=460x215", sgdbID)); err == nil {
		_ = c.download(imgURL, filepath.Join(gridDir, idStr+ext(imgURL)))
	}

	// Hero (1920x620) → {appId}_hero.ext
	if imgURL, err := c.fetchFirstURL(fmt.Sprintf("/heroes/game/%d", sgdbID)); err == nil {
		_ = c.download(imgURL, filepath.Join(gridDir, idStr+"_hero"+ext(imgURL)))
	}

	// Logo → {appId}_logo.ext
	if imgURL, err := c.fetchFirstURL(fmt.Sprintf("/logos/game/%d", sgdbID)); err == nil {
		_ = c.download(imgURL, filepath.Join(gridDir, idStr+"_logo"+ext(imgURL)))
	}

	return nil
}
