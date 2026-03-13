package domain

import (
	"encoding/json"
	"strings"
	"time"
)

// GameStatus mirrors C# enum GameStatus (integer values must match)
type GameStatus int

const (
	GameStatusTBA               GameStatus = 0
	GameStatusAnnounced         GameStatus = 1
	GameStatusReleased          GameStatus = 2
	GameStatusDownloading       GameStatus = 3
	GameStatusDownloaded        GameStatus = 4
	GameStatusMissing           GameStatus = 5
	GameStatusInstallerDetected GameStatus = 6
)

// PlatformType mirrors C# enum PlatformType
type PlatformType int

const (
	PlatformTypePC           PlatformType = 0
	PlatformTypeMacOS        PlatformType = 1
	PlatformTypePlayStation  PlatformType = 2
	PlatformTypePlayStation2 PlatformType = 3
	PlatformTypePlayStation3 PlatformType = 4
	PlatformTypePlayStation4 PlatformType = 5
	PlatformTypePlayStation5 PlatformType = 6
	PlatformTypePSP          PlatformType = 7
	PlatformTypePSVita       PlatformType = 8
	PlatformTypeXbox         PlatformType = 9
	PlatformTypeXbox360      PlatformType = 10
	PlatformTypeXboxOne      PlatformType = 11
	PlatformTypeXboxSeriesX  PlatformType = 12
	PlatformTypeNintendo64   PlatformType = 13
	PlatformTypeGameCube     PlatformType = 14
	PlatformTypeWii          PlatformType = 15
	PlatformTypeWiiU         PlatformType = 16
	PlatformTypeSwitch       PlatformType = 17
	PlatformTypeGameBoy      PlatformType = 18
	PlatformTypeGameBoyAdv   PlatformType = 19
	PlatformTypeNintendoDS   PlatformType = 20
	PlatformTypeNintendo3DS  PlatformType = 21
	PlatformTypeSegaGenesis  PlatformType = 22
	PlatformTypeDreamCast    PlatformType = 23
	PlatformTypeArcade       PlatformType = 24
	PlatformTypeOther        PlatformType = 25
)

type Platform struct {
	ID   int          `json:"id"`
	Name string       `json:"name"`
	Slug string       `json:"slug"`
	Type PlatformType `json:"type"`
	Icon *string      `json:"icon,omitempty"`
}

type GameImages struct {
	CoverUrl        *string  `json:"coverUrl,omitempty"`
	CoverLargeUrl   *string  `json:"coverLargeUrl,omitempty"`
	BackgroundUrl   *string  `json:"backgroundUrl,omitempty"`
	BannerUrl       *string  `json:"bannerUrl,omitempty"`
	Screenshots     []string `json:"screenshots"`
	Artworks        []string `json:"artworks"`
}

type GameFile struct {
	ID            int       `json:"id"`
	GameID        int       `json:"gameId"`
	RelativePath  string    `json:"relativePath"`
	Size          int64     `json:"size"`
	DateAdded     time.Time `json:"dateAdded"`
	Quality       *string   `json:"quality,omitempty"`
	ReleaseGroup  *string   `json:"releaseGroup,omitempty"`
	Edition       *string   `json:"edition,omitempty"`
	Languages     []string  `json:"languages"`
}

type Game struct {
	ID               int        `json:"id"`
	Title            string     `json:"title"`
	AlternativeTitle *string    `json:"alternativeTitle,omitempty"`
	Year             int        `json:"year"`
	Overview         *string    `json:"overview,omitempty"`
	Storyline        *string    `json:"storyline,omitempty"`
	PlatformID       int        `json:"platformId"`
	Platform         *Platform  `json:"platform,omitempty"`
	Added            time.Time  `json:"added"`
	Images           GameImages `json:"images"`
	Genres           []string   `json:"genres"`
	AvailablePlatforms []string `json:"availablePlatforms"`
	Developer        *string    `json:"developer,omitempty"`
	Publisher        *string    `json:"publisher,omitempty"`
	ReleaseDate      *time.Time `json:"releaseDate,omitempty"`
	Rating           *float64   `json:"rating,omitempty"`
	RatingCount      *int       `json:"ratingCount,omitempty"`
	Status           GameStatus `json:"status"`
	Monitored        bool       `json:"monitored"`
	Path             *string    `json:"path,omitempty"`
	SizeOnDisk       *int64     `json:"sizeOnDisk,omitempty"`
	GameFiles        []GameFile `json:"gameFiles"`
	IgdbID           *int       `json:"igdbId,omitempty"`
	SteamID          *int       `json:"steamId,omitempty"`
	GogID            *string    `json:"gogId,omitempty"`
	InstallPath      *string    `json:"installPath,omitempty"`
	IsInstallable    bool       `json:"isInstallable"`
	ExecutablePath   *string    `json:"executablePath,omitempty"`
	IsExternal       bool       `json:"isExternal"`
	IsOwned          bool       `json:"isOwned"`
	SavePath         string     `json:"savePath,omitempty"`
	CurrentVersion   string     `json:"currentVersion,omitempty"`
	LatestVersion    string     `json:"latestVersion,omitempty"`
	UpdateAvailable  bool       `json:"updateAvailable"`
}

// JSONStringSlice wraps []string for SQLite JSON column round-trips.
type JSONStringSlice []string

func (s JSONStringSlice) ToDBString() string {
	if len(s) == 0 {
		return "[]"
	}
	b, _ := json.Marshal([]string(s))
	return string(b)
}

func JSONStringSliceFromDB(v *string) []string {
	if v == nil || *v == "" {
		return []string{}
	}
	val := strings.TrimSpace(*v)
	if val == "" || val == "null" {
		return []string{}
	}
	var result []string
	if err := json.Unmarshal([]byte(val), &result); err != nil {
		return []string{}
	}
	return result
}
