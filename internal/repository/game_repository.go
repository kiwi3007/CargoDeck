package repository

import (
	"database/sql"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/kiwi3007/playerr/internal/domain"
)

// GameRepository is the SQLite-backed store for games, platforms and game files.
type GameRepository struct {
	db *sql.DB
}

func NewGameRepository(db *sql.DB) *GameRepository {
	return &GameRepository{db: db}
}

// ---- Platforms ----

func (r *GameRepository) GetAllPlatforms() ([]domain.Platform, error) {
	rows, err := r.db.Query(`SELECT Id, Name, Slug, Type, Icon FROM Platforms ORDER BY Id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var platforms []domain.Platform
	for rows.Next() {
		var p domain.Platform
		var icon sql.NullString
		if err := rows.Scan(&p.ID, &p.Name, &p.Slug, &p.Type, &icon); err != nil {
			return nil, err
		}
		if icon.Valid {
			p.Icon = &icon.String
		}
		platforms = append(platforms, p)
	}
	if platforms == nil {
		platforms = []domain.Platform{}
	}
	return platforms, rows.Err()
}

func (r *GameRepository) GetPlatformByID(id int) (*domain.Platform, error) {
	var p domain.Platform
	var icon sql.NullString
	err := r.db.QueryRow(`SELECT Id, Name, Slug, Type, Icon FROM Platforms WHERE Id = ?`, id).
		Scan(&p.ID, &p.Name, &p.Slug, &p.Type, &icon)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if icon.Valid {
		p.Icon = &icon.String
	}
	return &p, nil
}

// ---- Games ----

const gameSelectCols = `
    g.Id, g.Title, g.AlternativeTitle, g.Year, g.Overview, g.Storyline,
    g.PlatformId, g.Added,
    g.Images_CoverUrl, g.Images_CoverLargeUrl, g.Images_BackgroundUrl, g.Images_BannerUrl,
    g.Images_Screenshots, g.Images_Artworks,
    g.Genres, g.Developer, g.Publisher, g.ReleaseDate, g.Rating, g.RatingCount,
    g.Status, g.Monitored, g.Path, g.SizeOnDisk,
    g.IgdbId, g.SteamId, g.GogId, g.InstallPath, g.IsInstallable,
    g.ExecutablePath, g.IsExternal`

func scanGame(row interface {
	Scan(...any) error
}) (*domain.Game, error) {
	var g domain.Game
	var (
		altTitle        sql.NullString
		overview        sql.NullString
		storyline       sql.NullString
		added           string
		coverUrl        sql.NullString
		coverLargeUrl   sql.NullString
		backgroundUrl   sql.NullString
		bannerUrl       sql.NullString
		screenshots     sql.NullString
		artworks        sql.NullString
		genres          sql.NullString
		developer       sql.NullString
		publisher       sql.NullString
		releaseDate     sql.NullString
		rating          sql.NullFloat64
		ratingCount     sql.NullInt64
		path            sql.NullString
		sizeOnDisk      sql.NullInt64
		igdbId          sql.NullInt64
		steamId         sql.NullInt64
		gogId           sql.NullString
		installPath     sql.NullString
		isInstallable   int
		executablePath  sql.NullString
		isExternal      int
	)

	err := row.Scan(
		&g.ID, &g.Title, &altTitle, &g.Year, &overview, &storyline,
		&g.PlatformID, &added,
		&coverUrl, &coverLargeUrl, &backgroundUrl, &bannerUrl,
		&screenshots, &artworks,
		&genres, &developer, &publisher, &releaseDate, &rating, &ratingCount,
		&g.Status, &g.Monitored, &path, &sizeOnDisk,
		&igdbId, &steamId, &gogId, &installPath, &isInstallable,
		&executablePath, &isExternal,
	)
	if err != nil {
		return nil, err
	}

	// Nullable strings
	if altTitle.Valid {
		g.AlternativeTitle = &altTitle.String
	}
	if overview.Valid {
		g.Overview = &overview.String
	}
	if storyline.Valid {
		g.Storyline = &storyline.String
	}
	if path.Valid {
		g.Path = &path.String
	}
	if developer.Valid {
		g.Developer = &developer.String
	}
	if publisher.Valid {
		g.Publisher = &publisher.String
	}
	if gogId.Valid {
		g.GogID = &gogId.String
	}
	if installPath.Valid {
		g.InstallPath = &installPath.String
	}
	if executablePath.Valid {
		g.ExecutablePath = &executablePath.String
	}

	// Nullable numbers
	if sizeOnDisk.Valid {
		g.SizeOnDisk = &sizeOnDisk.Int64
	}
	if igdbId.Valid {
		v := int(igdbId.Int64)
		g.IgdbID = &v
	}
	if steamId.Valid {
		v := int(steamId.Int64)
		g.SteamID = &v
	}
	if rating.Valid {
		g.Rating = &rating.Float64
	}
	if ratingCount.Valid {
		v := int(ratingCount.Int64)
		g.RatingCount = &v
	}

	// Booleans stored as int
	g.IsInstallable = isInstallable != 0
	g.IsExternal = isExternal != 0
	g.Monitored = g.Monitored // already bool from scan

	// DateTime
	g.Added = parseFlexibleTime(added)
	if releaseDate.Valid && releaseDate.String != "" {
		t := parseFlexibleTime(releaseDate.String)
		g.ReleaseDate = &t
	}

	// Images
	g.Images = domain.GameImages{
		Screenshots: domain.JSONStringSliceFromDB(nullStringPtr(screenshots)),
		Artworks:    domain.JSONStringSliceFromDB(nullStringPtr(artworks)),
	}
	if coverUrl.Valid {
		g.Images.CoverUrl = &coverUrl.String
	}
	if coverLargeUrl.Valid {
		g.Images.CoverLargeUrl = &coverLargeUrl.String
	}
	if backgroundUrl.Valid {
		g.Images.BackgroundUrl = &backgroundUrl.String
	}
	if bannerUrl.Valid {
		g.Images.BannerUrl = &bannerUrl.String
	}

	// JSON slices
	g.Genres = domain.JSONStringSliceFromDB(nullStringPtr(genres))
	g.AvailablePlatforms = []string{}
	g.GameFiles = []domain.GameFile{}

	return &g, nil
}

func nullStringPtr(ns sql.NullString) *string {
	if ns.Valid {
		return &ns.String
	}
	return nil
}

func parseFlexibleTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	formats := []string{
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05Z",
		"2006-01-02T15:04:05.999999999Z",
		"2006-01-02",
		"0001-01-01 00:00:00",
	}
	for _, f := range formats {
		if t, err := time.Parse(f, s); err == nil {
			return t
		}
	}
	return time.Time{}
}

func (r *GameRepository) GetAllGames() ([]domain.Game, error) {
	query := fmt.Sprintf(`SELECT %s FROM Games g ORDER BY g.Title`, gameSelectCols)
	rows, err := r.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var games []domain.Game
	platformCache := map[int]*domain.Platform{}

	for rows.Next() {
		g, err := scanGame(rows)
		if err != nil {
			log.Printf("[Repo] Error scanning game row: %v", err)
			continue
		}
		p := r.resolvePlatform(g.PlatformID, platformCache)
		g.Platform = p

		files, _ := r.GetGameFiles(g.ID)
		g.GameFiles = files

		games = append(games, *g)
	}
	if games == nil {
		games = []domain.Game{}
	}
	return games, rows.Err()
}

func (r *GameRepository) GetGameByID(id int) (*domain.Game, error) {
	query := fmt.Sprintf(`SELECT %s FROM Games g WHERE g.Id = ?`, gameSelectCols)
	row := r.db.QueryRow(query, id)
	g, err := scanGame(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	p, _ := r.GetPlatformByID(g.PlatformID)
	g.Platform = p

	files, _ := r.GetGameFiles(g.ID)
	g.GameFiles = files

	return g, nil
}

func (r *GameRepository) CreateGame(g *domain.Game) (*domain.Game, error) {
	if g.Added.IsZero() {
		g.Added = time.Now()
	}

	result, err := r.db.Exec(`
        INSERT INTO Games (
            Title, AlternativeTitle, Year, Overview, Storyline, PlatformId, Added,
            Images_CoverUrl, Images_CoverLargeUrl, Images_BackgroundUrl, Images_BannerUrl,
            Images_Screenshots, Images_Artworks,
            Genres, Developer, Publisher, ReleaseDate, Rating, RatingCount,
            Status, Monitored, Path, SizeOnDisk,
            IgdbId, SteamId, GogId, InstallPath, IsInstallable,
            ExecutablePath, IsExternal
        ) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		g.Title, g.AlternativeTitle, g.Year, g.Overview, g.Storyline, g.PlatformID,
		formatTime(g.Added),
		g.Images.CoverUrl, g.Images.CoverLargeUrl, g.Images.BackgroundUrl, g.Images.BannerUrl,
		domain.JSONStringSlice(g.Images.Screenshots).ToDBString(),
		domain.JSONStringSlice(g.Images.Artworks).ToDBString(),
		domain.JSONStringSlice(g.Genres).ToDBString(),
		g.Developer, g.Publisher,
		formatTimePtr(g.ReleaseDate),
		g.Rating, g.RatingCount,
		int(g.Status), boolToInt(g.Monitored), g.Path, g.SizeOnDisk,
		g.IgdbID, g.SteamID, g.GogID, g.InstallPath, boolToInt(g.IsInstallable),
		g.ExecutablePath, boolToInt(g.IsExternal),
	)
	if err != nil {
		return nil, err
	}
	id, _ := result.LastInsertId()
	g.ID = int(id)
	return r.GetGameByID(g.ID)
}

func (r *GameRepository) UpdateGame(id int, g *domain.Game) (*domain.Game, error) {
	_, err := r.db.Exec(`
        UPDATE Games SET
            Title=?, AlternativeTitle=?, Year=?, Overview=?, Storyline=?, PlatformId=?,
            Images_CoverUrl=?, Images_CoverLargeUrl=?, Images_BackgroundUrl=?, Images_BannerUrl=?,
            Images_Screenshots=?, Images_Artworks=?,
            Genres=?, Developer=?, Publisher=?, ReleaseDate=?, Rating=?, RatingCount=?,
            Status=?, Monitored=?, Path=?, SizeOnDisk=?,
            IgdbId=?, SteamId=?, GogId=?, InstallPath=?, IsInstallable=?,
            ExecutablePath=?, IsExternal=?
        WHERE Id=?`,
		g.Title, g.AlternativeTitle, g.Year, g.Overview, g.Storyline, g.PlatformID,
		g.Images.CoverUrl, g.Images.CoverLargeUrl, g.Images.BackgroundUrl, g.Images.BannerUrl,
		domain.JSONStringSlice(g.Images.Screenshots).ToDBString(),
		domain.JSONStringSlice(g.Images.Artworks).ToDBString(),
		domain.JSONStringSlice(g.Genres).ToDBString(),
		g.Developer, g.Publisher,
		formatTimePtr(g.ReleaseDate),
		g.Rating, g.RatingCount,
		int(g.Status), boolToInt(g.Monitored), g.Path, g.SizeOnDisk,
		g.IgdbID, g.SteamID, g.GogID, g.InstallPath, boolToInt(g.IsInstallable),
		g.ExecutablePath, boolToInt(g.IsExternal),
		id,
	)
	if err != nil {
		return nil, err
	}
	return r.GetGameByID(id)
}

func (r *GameRepository) DeleteGame(id int) (bool, error) {
	res, err := r.db.Exec(`DELETE FROM Games WHERE Id = ?`, id)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

func (r *GameRepository) DeleteAllGames() error {
	_, err := r.db.Exec(`DELETE FROM Games`)
	return err
}

// ---- GameFiles ----

func (r *GameRepository) GetGameFiles(gameID int) ([]domain.GameFile, error) {
	rows, err := r.db.Query(`
        SELECT Id, GameId, RelativePath, Size, DateAdded, Quality, ReleaseGroup, Edition, Languages
        FROM GameFiles WHERE GameId = ? ORDER BY Id`, gameID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var files []domain.GameFile
	for rows.Next() {
		var f domain.GameFile
		var dateAdded string
		var quality, releaseGroup, edition, languages sql.NullString
		if err := rows.Scan(&f.ID, &f.GameID, &f.RelativePath, &f.Size, &dateAdded,
			&quality, &releaseGroup, &edition, &languages); err != nil {
			continue
		}
		f.DateAdded = parseFlexibleTime(dateAdded)
		if quality.Valid {
			f.Quality = &quality.String
		}
		if releaseGroup.Valid {
			f.ReleaseGroup = &releaseGroup.String
		}
		if edition.Valid {
			f.Edition = &edition.String
		}
		f.Languages = domain.JSONStringSliceFromDB(nullStringPtr(languages))
		files = append(files, f)
	}
	if files == nil {
		files = []domain.GameFile{}
	}
	return files, rows.Err()
}

func (r *GameRepository) CreateGameFile(f *domain.GameFile) (*domain.GameFile, error) {
	if f.DateAdded.IsZero() {
		f.DateAdded = time.Now()
	}
	res, err := r.db.Exec(`
        INSERT INTO GameFiles (GameId, RelativePath, Size, DateAdded, Quality, ReleaseGroup, Edition, Languages)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		f.GameID, f.RelativePath, f.Size, formatTime(f.DateAdded),
		f.Quality, f.ReleaseGroup, f.Edition,
		domain.JSONStringSlice(f.Languages).ToDBString(),
	)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	f.ID = int(id)
	return f, nil
}

func (r *GameRepository) DeleteGameFile(id int) error {
	_, err := r.db.Exec(`DELETE FROM GameFiles WHERE Id = ?`, id)
	return err
}

// ---- helpers ----

func (r *GameRepository) resolvePlatform(id int, cache map[int]*domain.Platform) *domain.Platform {
	if p, ok := cache[id]; ok {
		return p
	}
	p, err := r.GetPlatformByID(id)
	if err != nil {
		return nil
	}
	cache[id] = p
	return p
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func formatTime(t time.Time) string {
	return t.Format("2006-01-02 15:04:05")
}

func formatTimePtr(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := formatTime(*t)
	return &s
}

// GetGamesByPlatform returns games filtered by platform ID.
func (r *GameRepository) GetGamesByPlatform(platformID int) ([]domain.Game, error) {
	query := fmt.Sprintf(`SELECT %s FROM Games g WHERE g.PlatformId = ? ORDER BY g.Title`, gameSelectCols)
	rows, err := r.db.Query(query, platformID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var games []domain.Game
	p, _ := r.GetPlatformByID(platformID)

	for rows.Next() {
		g, err := scanGame(rows)
		if err != nil {
			continue
		}
		g.Platform = p
		files, _ := r.GetGameFiles(g.ID)
		g.GameFiles = files
		games = append(games, *g)
	}
	if games == nil {
		games = []domain.Game{}
	}
	return games, rows.Err()
}

// GetIgdbIds returns the set of IGDB IDs already in the library (for "isOwned" flagging).
func (r *GameRepository) GetIgdbIds() (map[int]struct{}, error) {
	rows, err := r.db.Query(`SELECT IgdbId FROM Games WHERE IgdbId IS NOT NULL`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := map[int]struct{}{}
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err == nil {
			result[id] = struct{}{}
		}
	}
	return result, rows.Err()
}

// SearchGames does a case-insensitive LIKE search on Title.
func (r *GameRepository) SearchGames(query string) ([]domain.Game, error) {
	q := fmt.Sprintf(`SELECT %s FROM Games g WHERE LOWER(g.Title) LIKE LOWER(?) ORDER BY g.Title`, gameSelectCols)
	rows, err := r.db.Query(q, "%"+strings.ToLower(query)+"%")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var games []domain.Game
	platformCache := map[int]*domain.Platform{}
	for rows.Next() {
		g, err := scanGame(rows)
		if err != nil {
			continue
		}
		g.Platform = r.resolvePlatform(g.PlatformID, platformCache)
		files, _ := r.GetGameFiles(g.ID)
		g.GameFiles = files
		games = append(games, *g)
	}
	if games == nil {
		games = []domain.Game{}
	}
	return games, rows.Err()
}
