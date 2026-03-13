package domain

// DownloadState mirrors C# enum DownloadState
type DownloadState int

const (
	DownloadStateDownloading DownloadState = 0
	DownloadStatePaused      DownloadState = 1
	DownloadStateCompleted   DownloadState = 2
	DownloadStateError       DownloadState = 3
	DownloadStateQueued      DownloadState = 4
	DownloadStateChecking    DownloadState = 5
	DownloadStateDeleted     DownloadState = 6
	DownloadStateImporting   DownloadState = 7
	DownloadStateUnknown     DownloadState = 8
)

type DownloadStatus struct {
	ClientID     int           `json:"clientId"`
	ID           string        `json:"id"`
	Name         string        `json:"name"`
	Size         int64         `json:"size"`
	Progress     float64       `json:"progress"`
	State        DownloadState `json:"state"`
	Category     string        `json:"category"`
	DownloadPath string        `json:"downloadPath"`
	ClientName   string        `json:"clientName"`
	InfoHash     string        `json:"infoHash,omitempty"`
}

type DownloadClientConfig struct {
	ID                int    `json:"id"`
	Name              string `json:"name"`
	Implementation    string `json:"implementation"`
	Host              string `json:"host"`
	Port              int    `json:"port"`
	Username          string `json:"username,omitempty"`
	Password          string `json:"password,omitempty"`
	Category          string `json:"category,omitempty"`
	UrlBase           string `json:"urlBase,omitempty"`
	ApiKey            string `json:"apiKey,omitempty"`
	Enable            bool   `json:"enable"`
	UseSsl            bool   `json:"useSsl"`
	Priority          int    `json:"priority"`
	RemotePathMapping string `json:"remotePathMapping,omitempty"`
	LocalPathMapping  string `json:"localPathMapping,omitempty"`
}
