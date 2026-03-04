package indexer

import "time"

// SearchResult is the unified result type returned by all indexer backends.
type SearchResult struct {
	Title       string     `json:"title"`
	Guid        string     `json:"guid"`
	DownloadURL string     `json:"downloadUrl"`
	MagnetURL   string     `json:"magnetUrl"`
	InfoURL     string     `json:"infoUrl"`
	IndexerID   int        `json:"indexerId"`
	IndexerName string     `json:"indexer"`
	Size        int64      `json:"size"`
	Seeders     int        `json:"seeders"`
	Leechers    int        `json:"leechers"`
	PublishDate time.Time  `json:"publishDate"`
	Protocol    string     `json:"protocol"` // "torrent" or "nzb"
	Provider    string     `json:"provider"`
	Categories  []Category `json:"categories"`
}

// Category mirrors the Prowlarr/Newznab category structure.
type Category struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}
