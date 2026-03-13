-- Playerr initial schema — fully idempotent (CREATE TABLE IF NOT EXISTS / INSERT OR IGNORE)
-- Mirrors the .NET EF Core schema including all manually-applied ALTER TABLE columns.

CREATE TABLE IF NOT EXISTS Platforms (
    Id   INTEGER PRIMARY KEY,
    Name TEXT    NOT NULL DEFAULT '',
    Slug TEXT    NOT NULL DEFAULT '',
    Type INTEGER NOT NULL DEFAULT 0,
    Icon TEXT
);

CREATE TABLE IF NOT EXISTS Games (
    Id               INTEGER PRIMARY KEY AUTOINCREMENT,
    Title            TEXT    NOT NULL DEFAULT '',
    AlternativeTitle TEXT,
    Year             INTEGER NOT NULL DEFAULT 0,
    Overview         TEXT,
    Storyline        TEXT,
    PlatformId       INTEGER NOT NULL DEFAULT 0,
    Added            TEXT    NOT NULL DEFAULT '0001-01-01 00:00:00',
    -- Images (EF Core owned-type flattened columns)
    Images_CoverUrl        TEXT,
    Images_CoverLargeUrl   TEXT,
    Images_BackgroundUrl   TEXT,
    Images_BannerUrl       TEXT,
    Images_Screenshots     TEXT,  -- JSON array
    Images_Artworks        TEXT,  -- JSON array
    -- Other fields
    Genres           TEXT,   -- JSON array
    Developer        TEXT,
    Publisher        TEXT,
    ReleaseDate      TEXT,
    Rating           REAL,
    RatingCount      INTEGER,
    Status           INTEGER NOT NULL DEFAULT 0,
    Monitored        INTEGER NOT NULL DEFAULT 0,
    Path             TEXT,
    SizeOnDisk       INTEGER,
    IgdbId           INTEGER,
    SteamId          INTEGER,
    GogId            TEXT,
    InstallPath      TEXT,
    IsInstallable    INTEGER NOT NULL DEFAULT 0,
    ExecutablePath   TEXT,
    IsExternal       INTEGER NOT NULL DEFAULT 0,
    save_path        TEXT,
    current_version  TEXT,
    latest_version   TEXT,
    update_available INTEGER NOT NULL DEFAULT 0,
    FOREIGN KEY (PlatformId) REFERENCES Platforms(Id)
);

CREATE TABLE IF NOT EXISTS GameFiles (
    Id           INTEGER PRIMARY KEY AUTOINCREMENT,
    GameId       INTEGER NOT NULL,
    RelativePath TEXT    NOT NULL DEFAULT '',
    Size         INTEGER NOT NULL DEFAULT 0,
    DateAdded    TEXT    NOT NULL DEFAULT '0001-01-01 00:00:00',
    Quality      TEXT,
    ReleaseGroup TEXT,
    Edition      TEXT,
    Languages    TEXT,  -- JSON array
    FOREIGN KEY (GameId) REFERENCES Games(Id) ON DELETE CASCADE
);

-- Per-device (per-agent) save path overrides.
-- Takes priority over the global game.save_path when set.
CREATE TABLE IF NOT EXISTS AgentGameSavePaths (
    GameId   INTEGER NOT NULL,
    AgentId  TEXT    NOT NULL,
    SavePath TEXT    NOT NULL,
    PRIMARY KEY (GameId, AgentId),
    FOREIGN KEY (GameId) REFERENCES Games(Id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS IX_GameFiles_GameId ON GameFiles(GameId);
CREATE INDEX IF NOT EXISTS IX_Games_PlatformId ON Games(PlatformId);

-- Default platforms (INSERT OR IGNORE to be idempotent)
INSERT OR IGNORE INTO Platforms(Id, Name, Slug, Type) VALUES
    (6,   'PC (Microsoft Windows)', 'pc',           0),
    (3,   'Linux',                  'linux',         0),
    (14,  'Mac',                    'mac',           1),
    (7,   'PlayStation',            'ps1',           2),
    (8,   'PlayStation 2',          'ps2',           3),
    (9,   'PlayStation 3',          'ps3',           4),
    (48,  'PlayStation 4',          'ps4',           5),
    (130, 'Nintendo Switch',        'switch',        17),
    (167, 'PlayStation 5',          'ps5',           6),
    (169, 'Xbox Series X|S',        'xbox-series-x', 12),
    (38,  'PlayStation Portable',   'psp',           7);
