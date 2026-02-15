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
			server_fetches INTEGER,
			username TEXT
		)
	`)
	return err
}

func LogRun(filename string, startedAt time.Time, durationMs int64,
	stdout, stderr string, exitCode int,
	memoryHits, diskHits, serverFetches int64, username string) (int64, error) {

	res, err := DB.Exec(`
		INSERT INTO runs (filename, started_at, duration_ms, stdout, stderr, exit_code, memory_cache_hits, disk_cache_hits, server_fetches, username)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, filename, startedAt, durationMs, stdout, stderr, exitCode, memoryHits, diskHits, serverFetches, username)
	if err != nil {
		return 0, err
	}

	return res.LastInsertId()
}

type RunRecord struct {
	ID              int64     `json:"id"`
	Filename        string    `json:"filename"`
	StartedAt       time.Time `json:"started_at"`
	DurationMs      int64     `json:"duration_ms"`
	Stdout          string    `json:"stdout"`
	Stderr          string    `json:"stderr"`
	ExitCode        int       `json:"exit_code"`
	MemoryCacheHits int64     `json:"memory_cache_hits"`
	DiskCacheHits   int64     `json:"disk_cache_hits"`
	ServerFetches   int64     `json:"server_fetches"`
	Username        string    `json:"username"`
}

func GetAllRuns() ([]RunRecord, error) {
	rows, err := DB.Query("SELECT id, filename, started_at, duration_ms, stdout, stderr, exit_code, memory_cache_hits, disk_cache_hits, server_fetches FROM runs ORDER BY id DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var runs []RunRecord
	for rows.Next() {
		var r RunRecord
		if err := rows.Scan(&r.ID, &r.Filename, &r.StartedAt, &r.DurationMs, &r.Stdout, &r.Stderr, &r.ExitCode, &r.MemoryCacheHits, &r.DiskCacheHits, &r.ServerFetches); err != nil {
			return nil, err
		}
		runs = append(runs, r)
	}
	return runs, rows.Err()
}
