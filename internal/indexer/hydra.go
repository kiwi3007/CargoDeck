package indexer

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// HydraClient fetches a static JSON source and filters client-side.
type HydraClient struct {
	name       string
	sourceURL  string
	httpClient *http.Client
}

func NewHydraClient(name, sourceURL string) *HydraClient {
	return &HydraClient{
		name:       name,
		sourceURL:  sourceURL,
		httpClient: &http.Client{Timeout: 60 * time.Second},
	}
}

// Search downloads the Hydra JSON source and filters by query.
func (c *HydraClient) Search(query string) ([]SearchResult, error) {
	resp, err := c.httpClient.Get(c.sourceURL)
	if err != nil {
		return nil, fmt.Errorf("hydra fetch %s: %w", c.name, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var root json.RawMessage
	if err := json.Unmarshal(body, &root); err != nil {
		return nil, fmt.Errorf("hydra json: %w", err)
	}

	// Support both {"downloads": [...]} and [...]
	var items []json.RawMessage
	var wrapper struct {
		Downloads []json.RawMessage `json:"downloads"`
	}
	if err := json.Unmarshal(root, &wrapper); err == nil && len(wrapper.Downloads) > 0 {
		items = wrapper.Downloads
	} else {
		if err := json.Unmarshal(root, &items); err != nil {
			return nil, fmt.Errorf("hydra items: %w", err)
		}
	}

	normalizedQuery := strings.ToLower(query)
	var results []SearchResult

	for _, raw := range items {
		var item struct {
			Title      string   `json:"title"`
			URIs       []string `json:"uris"`
			UploadDate string   `json:"uploadDate"`
			FileSize   string   `json:"fileSize"`
		}
		if err := json.Unmarshal(raw, &item); err != nil {
			continue
		}
		if item.Title == "" {
			continue
		}
		if !strings.Contains(strings.ToLower(item.Title), normalizedQuery) {
			continue
		}

		var magnet, dlURL string
		for _, uri := range item.URIs {
			if strings.HasPrefix(uri, "magnet:") {
				magnet = uri
			} else if strings.HasPrefix(uri, "http") {
				dlURL = uri
			}
		}

		pubDate := time.Now()
		if item.UploadDate != "" {
			if t, err := time.Parse("2006-01-02", item.UploadDate); err == nil {
				pubDate = t
			}
		}

		guid := magnet
		if guid == "" {
			guid = dlURL
		}

		results = append(results, SearchResult{
			Title:       item.Title,
			Guid:        guid,
			DownloadURL: dlURL,
			MagnetURL:   magnet,
			InfoURL:     c.sourceURL,
			IndexerName: c.name,
			Size:        parseHydraSize(item.FileSize),
			Protocol:    "torrent",
			Provider:    "Hydra",
			PublishDate: pubDate,
		})
	}
	return results, nil
}

func parseHydraSize(s string) int64 {
	if s == "" {
		return 0
	}
	upper := strings.ToUpper(s)
	var multiplier float64 = 1
	switch {
	case strings.Contains(upper, "GB"):
		multiplier = 1024 * 1024 * 1024
	case strings.Contains(upper, "MB"):
		multiplier = 1024 * 1024
	case strings.Contains(upper, "KB"):
		multiplier = 1024
	}
	numStr := strings.TrimFunc(s, func(r rune) bool {
		return !unicode.IsDigit(r) && r != '.'
	})
	// Keep only leading numeric part
	for i, c := range numStr {
		if !unicode.IsDigit(c) && c != '.' {
			numStr = numStr[:i]
			break
		}
	}
	if v, err := strconv.ParseFloat(numStr, 64); err == nil {
		return int64(v * multiplier)
	}
	return 0
}
