package indexer

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// ProwlarrClient searches via the Prowlarr unified API.
type ProwlarrClient struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

func NewProwlarrClient(baseURL, apiKey string) *ProwlarrClient {
	return &ProwlarrClient{
		baseURL:    strings.TrimRight(baseURL, "/"),
		apiKey:     apiKey,
		httpClient: &http.Client{Timeout: 60 * time.Second},
	}
}

// TestConnection checks Prowlarr health endpoint.
func (c *ProwlarrClient) TestConnection() bool {
	req, err := http.NewRequest(http.MethodGet, c.baseURL+"/api/v1/health", nil)
	if err != nil {
		return false
	}
	req.Header.Set("X-Api-Key", c.apiKey)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode < 300
}


// Search queries the Prowlarr /api/v1/search endpoint.
// Categories are intentionally NOT forwarded to Prowlarr — passing them causes
// Prowlarr to skip any indexer whose configured categories don't match exactly
// (e.g. NZB indexers mapped to "all" or different sub-categories).
// Prowlarr itself is already scoped to the right indexers by its own configuration.
func (c *ProwlarrClient) Search(query string, _ []int) ([]SearchResult, error) {
	params := url.Values{}
	params.Set("query", query)
	params.Set("type", "search")
	params.Set("limit", "1000")

	reqURL := c.baseURL + "/api/v1/search?" + params.Encode()
	log.Printf("[Prowlarr] Search URL: %s", reqURL)
	req, err := http.NewRequest(http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Api-Key", c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("prowlarr search: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("prowlarr search status %d", resp.StatusCode)
	}

	// Prowlarr may return XML (RSS/Newznab) or JSON
	trimmed := strings.TrimSpace(string(body))
	if strings.HasPrefix(trimmed, "<") {
		return parseProwlarrXML(body)
	}
	return parseProwlarrJSON(body)
}

// ---- JSON parsing ----

type prowlarrJSONResult struct {
	Title       string            `json:"title"`
	Guid        string            `json:"guid"`
	DownloadURL string            `json:"downloadUrl"`
	MagnetURL   string            `json:"magnetUrl"`
	InfoURL     string            `json:"infoUrl"`
	IndexerID   int               `json:"indexerId"`
	IndexerName string            `json:"indexer"`
	Size        int64             `json:"size"`
	Seeders     int               `json:"seeders"`
	Leechers    int               `json:"leechers"`
	PublishDate string            `json:"publishDate"`
	Protocol    string            `json:"protocol"`
	Categories  []prowlarrCatJSON `json:"categories"`
}

type prowlarrCatJSON struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

func parseProwlarrJSON(data []byte) ([]SearchResult, error) {
	var raw []prowlarrJSONResult
	if err := json.Unmarshal(data, &raw); err != nil {
		log.Printf("[Prowlarr] JSON decode error: %v — body snippet: %.200s", err, data)
		return nil, fmt.Errorf("prowlarr json decode: %w", err)
	}
	log.Printf("[Prowlarr] Raw results: %d", len(raw))
	indexerCounts := map[string]int{}
	for _, r := range raw {
		indexerCounts[r.IndexerName]++
	}
	for idx, cnt := range indexerCounts {
		log.Printf("[Prowlarr]   indexer=%q count=%d", idx, cnt)
	}

	results := make([]SearchResult, 0, len(raw))
	for _, r := range raw {
		sr := SearchResult{
			Title:       r.Title,
			Guid:        r.Guid,
			DownloadURL: r.DownloadURL,
			MagnetURL:   r.MagnetURL,
			InfoURL:     r.InfoURL,
			IndexerID:   r.IndexerID,
			IndexerName: r.IndexerName,
			Size:        r.Size,
			Seeders:     r.Seeders,
			Leechers:    r.Leechers,
			Protocol:    normalizeProtocol(r.Protocol, r.DownloadURL, r.Guid, r.IndexerName),
			Provider:    "Prowlarr",
		}
		if t, err := time.Parse(time.RFC3339, r.PublishDate); err == nil {
			sr.PublishDate = t
		} else {
			sr.PublishDate = time.Now()
		}
		for _, cat := range r.Categories {
			sr.Categories = append(sr.Categories, Category{ID: cat.ID, Name: cat.Name})
		}
		results = append(results, sr)
	}
	return results, nil
}

func normalizeProtocol(proto, dlURL, guid, indexer string) string {
	p := strings.ToLower(proto)
	if p == "nzb" || p == "usenet" {
		return "nzb"
	}
	// Detect NZB by URL or indexer name
	if strings.HasSuffix(strings.ToLower(dlURL), ".nzb") ||
		strings.HasSuffix(strings.ToLower(guid), ".nzb") ||
		strings.Contains(strings.ToLower(indexer), "nzb") {
		return "nzb"
	}
	return "torrent"
}

// ---- XML parsing (RSS/Newznab) ----

type rssRoot struct {
	XMLName xml.Name   `xml:"rss"`
	Channel rssChannel `xml:"channel"`
}

type rssChannel struct {
	Items []rssItem `xml:"item"`
}

type rssItem struct {
	Title     string      `xml:"title"`
	Guid      string      `xml:"guid"`
	Link      string      `xml:"link"`
	Comments  string      `xml:"comments"`
	PubDate   string      `xml:"pubDate"`
	Enclosure rssEnclosure `xml:"enclosure"`
	Attrs     []newznabAttr `xml:"http://www.newznab.com/DTD/2010/feeds/attributes/ attr"`
}

type rssEnclosure struct {
	URL    string `xml:"url,attr"`
	Length int64  `xml:"length,attr"`
	Type   string `xml:"type,attr"`
}

type newznabAttr struct {
	Name  string `xml:"name,attr"`
	Value string `xml:"value,attr"`
}

func parseProwlarrXML(data []byte) ([]SearchResult, error) {
	var root rssRoot
	if err := xml.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("prowlarr xml decode: %w", err)
	}

	results := make([]SearchResult, 0, len(root.Channel.Items))
	for _, item := range root.Channel.Items {
		sr := SearchResult{
			Title:       item.Title,
			Guid:        item.Guid,
			DownloadURL: item.Link,
			InfoURL:     item.Comments,
			Size:        item.Enclosure.Length,
			Protocol:    "torrent",
			Provider:    "Prowlarr",
		}
		if strings.EqualFold(item.Enclosure.Type, "application/x-nzb") {
			sr.Protocol = "nzb"
		}
		if t, err := time.Parse(time.RFC1123Z, item.PubDate); err == nil {
			sr.PublishDate = t
		} else if t, err := time.Parse(time.RFC1123, item.PubDate); err == nil {
			sr.PublishDate = t
		} else {
			sr.PublishDate = time.Now()
		}
		for _, attr := range item.Attrs {
			if attr.Name == "category" {
				sr.Categories = append(sr.Categories, Category{ID: 0, Name: attr.Value})
			}
		}
		results = append(results, sr)
	}
	return results, nil
}
