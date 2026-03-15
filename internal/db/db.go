package db

import (
	"database/sql"
	"embed"
	"fmt"
	"log"
	"strings"

	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrations embed.FS

// Open opens the SQLite database at dbPath and runs all migrations.
func Open(dbPath string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", dbPath+"?_journal_mode=WAL&_foreign_keys=on&_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	// SQLite: allow multiple concurrent reads; WAL handles writer concurrency
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)

	if err := runMigrations(db); err != nil {
		return nil, fmt.Errorf("migrations: %w", err)
	}
	addColumnIfMissing(db, "Games", "save_path", "TEXT")
	addColumnIfMissing(db, "Games", "current_version", "TEXT")
	addColumnIfMissing(db, "Games", "latest_version", "TEXT")
	addColumnIfMissing(db, "Games", "update_available", "INTEGER NOT NULL DEFAULT 0")
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS AgentGameLaunchArgs (
		GameId  INTEGER NOT NULL,
		AgentId TEXT    NOT NULL,
		LaunchArgs TEXT NOT NULL,
		PRIMARY KEY (GameId, AgentId)
	)`); err != nil {
		return nil, fmt.Errorf("create AgentGameLaunchArgs: %w", err)
	}
	addColumnIfMissing(db, "AgentGameLaunchArgs", "EnvVars", "TEXT NOT NULL DEFAULT ''")
	addColumnIfMissing(db, "AgentGameLaunchArgs", "ProtonPath", "TEXT NOT NULL DEFAULT ''")
	addColumnIfMissing(db, "AgentGameLaunchArgs", "UseSLS", "INTEGER NOT NULL DEFAULT 1")
	return db, nil
}

// addColumnIfMissing runs ALTER TABLE to add a column, silently ignoring "duplicate column name".
func addColumnIfMissing(db *sql.DB, table, col, def string) {
	_, err := db.Exec(fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", table, col, def))
	if err != nil && !strings.Contains(err.Error(), "duplicate column name") {
		log.Printf("[DB] addColumnIfMissing %s.%s: %v", table, col, err)
	}
}

func runMigrations(db *sql.DB) error {
	entries, err := migrations.ReadDir("migrations")
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		data, err := migrations.ReadFile("migrations/" + entry.Name())
		if err != nil {
			return err
		}
		if _, err := db.Exec(string(data)); err != nil {
			return fmt.Errorf("migration %s: %w", entry.Name(), err)
		}
		log.Printf("[DB] Applied migration: %s", entry.Name())
	}
	return nil
}
