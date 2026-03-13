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

	// Scorer fields — populated by internal/scorer after search results are collected.
	Score           int      `json:"score"`
	ScorePct        int      `json:"scorePct"`
	TitleScore      int      `json:"titleScore"`
	SourceName      string   `json:"sourceName,omitempty"`
	SourceTier      int      `json:"sourceTier"`
	DetectedVersion string   `json:"detectedVersion,omitempty"`
	DetectedLangs   []string `json:"detectedLangs,omitempty"`
	ReleaseType     string   `json:"releaseType,omitempty"`
	InstallMethod   string   `json:"installMethod,omitempty"`
	SizeWarning     string   `json:"sizeWarning,omitempty"`
}

// Category mirrors the Prowlarr/Newznab category structure.
type Category struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}
