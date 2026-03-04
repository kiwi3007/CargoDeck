package igdb

// Game is an IGDB game record returned by the API.
type Game struct {
	ID               int                `json:"id"`
	Name             string             `json:"name"`
	Summary          string             `json:"summary"`
	Storyline        string             `json:"storyline"`
	Cover            *Image             `json:"cover"`
	Screenshots      []Image            `json:"screenshots"`
	Artworks         []Image            `json:"artworks"`
	Genres           []Genre            `json:"genres"`
	FirstReleaseDate *int64             `json:"first_release_date"`
	Rating           *float64           `json:"total_rating"`
	RatingCount      *int               `json:"total_rating_count"`
	InvolvedCompanies []InvolvedCompany `json:"involved_companies"`
	ExternalGames    []ExternalGame     `json:"external_games"`
	Platforms        []Platform         `json:"platforms"`
}

type Image struct {
	ImageID string `json:"image_id"`
}

type Genre struct {
	Name string `json:"name"`
}

type InvolvedCompany struct {
	Company   Company `json:"company"`
	Developer bool    `json:"developer"`
	Publisher bool    `json:"publisher"`
}

type Company struct {
	Name string `json:"name"`
}

type ExternalGame struct {
	Category int    `json:"category"` // 1 = Steam
	UID      string `json:"uid"`
}

type Platform struct {
	Name         string `json:"name"`
	Abbreviation string `json:"abbreviation"`
}
