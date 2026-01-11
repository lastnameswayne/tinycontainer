package db

import (
	"database/sql"
	"time"

	_ "modernc.org/sqlite"
)

var DB *sql.DB

func Init(path string) error {
	var err error
	DB, err = sql.Open("sqlite", path)
	if err != nil {
		return err
	}

	_, err = DB.Exec(`
		CREATE TABLE IF NOT EXISTS runs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			filename TEXT NOT NULL,
			started_at DATETIME NOT NULL,
			duration_ms INTEGER NOT NULL,
			stdout TEXT,
			stderr TEXT,
			exit_code INTEGER NOT NULL,
			memory_cache_hits INTEGER,
			disk_cache_hits INTEGER,
			server_fetches INTEGER
		)
	`)
	return err
}

func LogRun(filename string, startedAt time.Time, durationMs int64,
	stdout, stderr string, exitCode int,
	memoryHits, diskHits, serverFetches int64) error {

	_, err := DB.Exec(`
		INSERT INTO runs (filename, started_at, duration_ms, stdout, stderr, exit_code, memory_cache_hits, disk_cache_hits, server_fetches)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, filename, startedAt, durationMs, stdout, stderr, exitCode, memoryHits, diskHits, serverFetches)
	return err
}
