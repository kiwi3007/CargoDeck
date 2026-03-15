package repository

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/kiwi3007/cargodeck/internal/domain"
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
    g.ExecutablePath, g.IsExternal, g.save_path,
    g.current_version, g.latest_version, g.update_available`

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
		monitored        int
		isInstallable    int
		executablePath   sql.NullString
		isExternal       int
		savePath         sql.NullString
		currentVersion   sql.NullString
		latestVersion    sql.NullString
		updateAvailable  int
	)

	err := row.Scan(
		&g.ID, &g.Title, &altTitle, &g.Year, &overview, &storyline,
		&g.PlatformID, &added,
		&coverUrl, &coverLargeUrl, &backgroundUrl, &bannerUrl,
		&screenshots, &artworks,
		&genres, &developer, &publisher, &releaseDate, &rating, &ratingCount,
		&g.Status, &monitored, &path, &sizeOnDisk,
		&igdbId, &steamId, &gogId, &installPath, &isInstallable,
		&executablePath, &isExternal, &savePath,
		&currentVersion, &latestVersion, &updateAvailable,
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
	g.Monitored = monitored != 0
	if savePath.Valid {
		g.SavePath = savePath.String
	}
	if currentVersion.Valid {
		g.CurrentVersion = currentVersion.String
	}
	if latestVersion.Valid {
		g.LatestVersion = latestVersion.String
	}
	g.UpdateAvailable = updateAvailable != 0

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

// UpdateGameSavePath sets (or clears) the custom save path for a game.
func (r *GameRepository) UpdateGameSavePath(id int, path string) error {
	var val interface{}
	if path != "" {
		val = path
	}
	_, err := r.db.Exec(`UPDATE Games SET save_path = ? WHERE Id = ?`, val, id)
	return err
}

// ---- Per-device (per-agent) save path overrides ----

// parseSavePaths handles both the legacy plain-string format and the new
// JSON-array format (["path1","path2"]) stored in the SavePath column.
func parseSavePaths(raw string) []string {
	if strings.HasPrefix(raw, "[") {
		var paths []string
		if json.Unmarshal([]byte(raw), &paths) == nil {
			var out []string
			for _, p := range paths {
				if p != "" {
					out = append(out, p)
				}
			}
			return out
		}
	}
	if raw != "" {
		return []string{raw}
	}
	return nil
}

// GetAgentSavePath returns the custom save paths for a specific agent+game.
func (r *GameRepository) GetAgentSavePath(gameID int, agentID string) ([]string, error) {
	var raw string
	err := r.db.QueryRow(
		`SELECT SavePath FROM AgentGameSavePaths WHERE GameId = ? AND AgentId = ?`,
		gameID, agentID,
	).Scan(&raw)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return parseSavePaths(raw), nil
}

// SetAgentSavePath upserts custom save paths for an agent+game. Empty slice deletes.
func (r *GameRepository) SetAgentSavePath(gameID int, agentID string, paths []string) error {
	if len(paths) == 0 {
		_, err := r.db.Exec(
			`DELETE FROM AgentGameSavePaths WHERE GameId = ? AND AgentId = ?`,
			gameID, agentID,
		)
		return err
	}
	data, _ := json.Marshal(paths)
	_, err := r.db.Exec(
		`INSERT INTO AgentGameSavePaths(GameId, AgentId, SavePath) VALUES(?,?,?)
		 ON CONFLICT(GameId, AgentId) DO UPDATE SET SavePath = excluded.SavePath`,
		gameID, agentID, string(data),
	)
	return err
}

// GetAllAgentSavePaths returns all per-device save path overrides for a game.
// Returns a map of agentId → []savePath.
func (r *GameRepository) GetAllAgentSavePaths(gameID int) (map[string][]string, error) {
	rows, err := r.db.Query(
		`SELECT AgentId, SavePath FROM AgentGameSavePaths WHERE GameId = ?`,
		gameID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := map[string][]string{}
	for rows.Next() {
		var agentID, raw string
		if err := rows.Scan(&agentID, &raw); err == nil {
			result[agentID] = parseSavePaths(raw)
		}
	}
	return result, rows.Err()
}

// AgentRunSettings holds per-device run configuration for a game.
type AgentRunSettings struct {
	LaunchArgs string `json:"launchArgs"`
	EnvVars    string `json:"envVars"`
	ProtonPath string `json:"protonPath"`
	UseSLS     bool   `json:"useSLS"`
}

// GetAgentRunSettings returns the per-device run settings for an agent+game.
func (r *GameRepository) GetAgentRunSettings(gameID int, agentID string) (AgentRunSettings, error) {
	var s AgentRunSettings
	var useSLS int
	err := r.db.QueryRow(
		`SELECT LaunchArgs, COALESCE(EnvVars,''), COALESCE(ProtonPath,''), COALESCE(UseSLS,1) FROM AgentGameLaunchArgs WHERE GameId = ? AND AgentId = ?`,
		gameID, agentID,
	).Scan(&s.LaunchArgs, &s.EnvVars, &s.ProtonPath, &useSLS)
	if err == sql.ErrNoRows {
		return AgentRunSettings{UseSLS: true}, nil
	}
	s.UseSLS = useSLS != 0
	return s, err
}

// SetAgentRunSettings upserts run settings for an agent+game. Deletes the row if all fields are at defaults.
func (r *GameRepository) SetAgentRunSettings(gameID int, agentID string, s AgentRunSettings) error {
	if s.LaunchArgs == "" && s.EnvVars == "" && s.ProtonPath == "" && s.UseSLS {
		_, err := r.db.Exec(
			`DELETE FROM AgentGameLaunchArgs WHERE GameId = ? AND AgentId = ?`,
			gameID, agentID,
		)
		return err
	}
	useSLS := 0
	if s.UseSLS {
		useSLS = 1
	}
	_, err := r.db.Exec(
		`INSERT INTO AgentGameLaunchArgs(GameId, AgentId, LaunchArgs, EnvVars, ProtonPath, UseSLS) VALUES(?,?,?,?,?,?)
		 ON CONFLICT(GameId, AgentId) DO UPDATE SET LaunchArgs = excluded.LaunchArgs, EnvVars = excluded.EnvVars, ProtonPath = excluded.ProtonPath, UseSLS = excluded.UseSLS`,
		gameID, agentID, s.LaunchArgs, s.EnvVars, s.ProtonPath, useSLS,
	)
	return err
}

// GetAllAgentRunSettings returns all per-device run settings for a game (agentId → settings).
func (r *GameRepository) GetAllAgentRunSettings(gameID int) (map[string]AgentRunSettings, error) {
	rows, err := r.db.Query(
		`SELECT AgentId, LaunchArgs, COALESCE(EnvVars,''), COALESCE(ProtonPath,''), COALESCE(UseSLS,1) FROM AgentGameLaunchArgs WHERE GameId = ?`,
		gameID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := map[string]AgentRunSettings{}
	for rows.Next() {
		var agentID string
		var s AgentRunSettings
		var useSLS int
		if err := rows.Scan(&agentID, &s.LaunchArgs, &s.EnvVars, &s.ProtonPath, &useSLS); err == nil {
			s.UseSLS = useSLS != 0
			result[agentID] = s
		}
	}
	return result, rows.Err()
}

// GetAgentLaunchArgs returns the launch args string for an agent+game (legacy helper).
func (r *GameRepository) GetAgentLaunchArgs(gameID int, agentID string) (string, error) {
	s, err := r.GetAgentRunSettings(gameID, agentID)
	return s.LaunchArgs, err
}

// UpdateGamePath sets the Path field for a game (used after a completed Steam depot download).
func (r *GameRepository) UpdateGamePath(id int, path string) error {
	_, err := r.db.Exec(`UPDATE Games SET Path = ? WHERE Id = ?`, path, id)
	return err
}

// UpdateGameVersion sets the current_version field (called when agent reports installed version).
func (r *GameRepository) UpdateGameVersion(id int, version string) error {
	_, err := r.db.Exec(`UPDATE Games SET current_version = ? WHERE Id = ?`, version, id)
	return err
}

// UpdateGameUpdateInfo sets latest_version and update_available flag.
func (r *GameRepository) UpdateGameUpdateInfo(id int, latestVersion string, updateAvailable bool) error {
	_, err := r.db.Exec(
		`UPDATE Games SET latest_version = ?, update_available = ? WHERE Id = ?`,
		latestVersion, boolToInt(updateAvailable), id,
	)
	return err
}

// GetMonitoredGamesWithVersion returns games that have current_version set (eligible for update checks).
func (r *GameRepository) GetMonitoredGamesWithVersion() ([]domain.Game, error) {
	query := fmt.Sprintf(
		`SELECT %s FROM Games g WHERE g.current_version IS NOT NULL AND g.current_version != '' ORDER BY g.Title`,
		gameSelectCols,
	)
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
			continue
		}
		g.Platform = r.resolvePlatform(g.PlatformID, platformCache)
		games = append(games, *g)
	}
	if games == nil {
		games = []domain.Game{}
	}
	return games, rows.Err()
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
func (r *GameRepository) GetGameByIgdbID(igdbID int) (*domain.Game, error) {
	query := fmt.Sprintf(`SELECT %s FROM Games g WHERE g.IgdbId = ? LIMIT 1`, gameSelectCols)
	row := r.db.QueryRow(query, igdbID)
	g, err := scanGame(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return g, nil
}

// GetGameByTitle returns the first library game matching the title (case-insensitive).
func (r *GameRepository) GetGameByTitle(title string) (*domain.Game, error) {
	query := fmt.Sprintf(`SELECT %s FROM Games g WHERE LOWER(g.Title) = LOWER(?) LIMIT 1`, gameSelectCols)
	row := r.db.QueryRow(query, title)
	g, err := scanGame(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return g, nil
}

// GetGameByPath returns the first game whose Path exactly matches the given directory.
func (r *GameRepository) GetGameByPath(path string) (*domain.Game, error) {
	query := fmt.Sprintf(`SELECT %s FROM Games g WHERE g.Path = ? LIMIT 1`, gameSelectCols)
	row := r.db.QueryRow(query, path)
	g, err := scanGame(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return g, nil
}

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
