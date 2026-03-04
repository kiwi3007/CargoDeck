package indexer

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// JackettClient searches via the Jackett /api/v2.0/indexers/all/results endpoint.
type JackettClient struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

func NewJackettClient(baseURL, apiKey string) *JackettClient {
	return &JackettClient{
		baseURL:    strings.TrimRight(baseURL, "/"),
		apiKey:     apiKey,
		httpClient: &http.Client{Timeout: 60 * time.Second},
	}
}

// TestConnection does a lightweight caps check.
func (c *JackettClient) TestConnection() bool {
	u := fmt.Sprintf("%s/api/v2.0/indexers/all/results?apikey=%s&t=caps", c.baseURL, c.apiKey)
	resp, err := c.httpClient.Get(u)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode < 300
}

// Search queries Jackett and normalises results to SearchResult.
func (c *JackettClient) Search(query string, categories []int) ([]SearchResult, error) {
	params := url.Values{}
	params.Set("apikey", c.apiKey)
	params.Set("Query", query)
	if len(categories) > 0 {
		cats := make([]string, len(categories))
		for i, cat := range categories {
			cats[i] = fmt.Sprintf("%d", cat)
		}
		params.Set("Category", strings.Join(cats, ","))
	}

	reqURL := fmt.Sprintf("%s/api/v2.0/indexers/all/results?%s", c.baseURL, params.Encode())
	resp, err := c.httpClient.Get(reqURL)
	if err != nil {
		return nil, fmt.Errorf("jackett search: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("jackett search status %d", resp.StatusCode)
	}

	var root struct {
		Results []jackettResult `json:"Results"`
	}
	if err := json.Unmarshal(body, &root); err != nil {
		return nil, fmt.Errorf("jackett decode: %w", err)
	}

	results := make([]SearchResult, 0, len(root.Results))
	for _, r := range root.Results {
		dl := r.Link
		if r.MagnetURI != "" {
			dl = r.MagnetURI
		}
		leechers := 0
		if r.Peers > r.Seeders {
			leechers = r.Peers - r.Seeders
		}
		results = append(results, SearchResult{
			Title:       r.Title,
			Guid:        r.Guid,
			DownloadURL: dl,
			MagnetURL:   r.MagnetURI,
			InfoURL:     r.Guid,
			IndexerName: r.Tracker,
			Size:        r.Size,
			Seeders:     r.Seeders,
			Leechers:    leechers,
			PublishDate: r.PublishDate,
			Protocol:    "torrent",
			Provider:    "Jackett",
		})
	}
	return results, nil
}

type jackettResult struct {
	Title       string    `json:"Title"`
	Guid        string    `json:"Guid"`
	Link        string    `json:"Link"`
	MagnetURI   string    `json:"MagnetUri"`
	Tracker     string    `json:"Tracker"`
	Size        int64     `json:"Size"`
	PublishDate time.Time `json:"PublishDate"`
	Seeders     int       `json:"Seeders"`
	Peers       int       `json:"Peers"`
}
