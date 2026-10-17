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
	if err != nil {
		return err
	}

	// Columns added for the write-up experiments. ALTER fails harmlessly if the column exists.
	for _, col := range []string{
		"first_output_ms INTEGER",
		"exit_ms INTEGER",
		"handler_ms INTEGER",
		"handler_start_us INTEGER",
		"negative_lookups INTEGER",
		"dropped_events INTEGER",
	} {
		DB.Exec("ALTER TABLE runs ADD COLUMN " + col)
	}

	_, err = DB.Exec(`
		CREATE TABLE IF NOT EXISTS file_events (
			run_id INTEGER NOT NULL,
			ts_us INTEGER NOT NULL,
			op TEXT NOT NULL,
			path TEXT NOT NULL,
			size_bytes INTEGER NOT NULL,
			tier TEXT NOT NULL,
			latency_us INTEGER NOT NULL,
			hit INTEGER NOT NULL,
			hedge INTEGER NOT NULL DEFAULT 0
		);
		CREATE INDEX IF NOT EXISTS file_events_run ON file_events(run_id);
	`)
	return err
}

// FileEvent is one FUSE Lookup/Open/Read, see filesystem/events.go.
// Hedge is 0 when no hedge fired, 1 when the hedge fired but the first request won,
// 2 when the hedged request won.
type FileEvent struct {
	RunID     int64
	TsUs      int64
	Op        string
	Path      string
	SizeBytes int64
	Tier      string
	LatencyUs int64
	Hit       bool
	Hedge     int
}

func InsertFileEvents(events []FileEvent) error {
	tx, err := DB.Begin()
	if err != nil {
		return err
	}
	stmt, err := tx.Prepare(`INSERT INTO file_events (run_id, ts_us, op, path, size_bytes, tier, latency_us, hit, hedge) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		tx.Rollback()
		return err
	}
	defer stmt.Close()
	for _, e := range events {
		if _, err := stmt.Exec(e.RunID, e.TsUs, e.Op, e.Path, e.SizeBytes, e.Tier, e.LatencyUs, e.Hit, e.Hedge); err != nil {
			tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

// StartRun inserts a placeholder row so file events can be tagged with the run id while
// the run is in progress. FinishRun fills in the rest.
func StartRun(filename string, startedAt time.Time, username string) (int64, error) {
	res, err := DB.Exec(`
		INSERT INTO runs (filename, started_at, duration_ms, exit_code, username)
		VALUES (?, ?, 0, -1, ?)
	`, filename, startedAt, username)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

type RunResult struct {
	DurationMs      int64
	FirstOutputMs   int64
	ExitMs          int64
	HandlerMs       int64
	HandlerStartUs  int64
	Stdout          string
	Stderr          string
	ExitCode        int
	MemoryHits      int64
	DiskHits        int64
	ServerFetches   int64
	NegativeLookups int64
	DroppedEvents   int64
}

func FinishRun(id int64, r RunResult) error {
	_, err := DB.Exec(`
		UPDATE runs SET duration_ms = ?, first_output_ms = ?, exit_ms = ?, handler_ms = ?, handler_start_us = ?, stdout = ?, stderr = ?, exit_code = ?,
			memory_cache_hits = ?, disk_cache_hits = ?, server_fetches = ?, negative_lookups = ?, dropped_events = ?
		WHERE id = ?
	`, r.DurationMs, r.FirstOutputMs, r.ExitMs, r.HandlerMs, r.HandlerStartUs, r.Stdout, r.Stderr, r.ExitCode,
		r.MemoryHits, r.DiskHits, r.ServerFetches, r.NegativeLookups, r.DroppedEvents, id)
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
	rows, err := DB.Query("SELECT id, filename, started_at, duration_ms, stdout, stderr, exit_code, memory_cache_hits, disk_cache_hits, server_fetches, username FROM runs WHERE exit_code != -1 ORDER BY id DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var runs []RunRecord
	for rows.Next() {
		var r RunRecord
		if err := rows.Scan(&r.ID, &r.Filename, &r.StartedAt, &r.DurationMs, &r.Stdout, &r.Stderr, &r.ExitCode, &r.MemoryCacheHits, &r.DiskCacheHits, &r.ServerFetches, &r.Username); err != nil {
			return nil, err
		}
		runs = append(runs, r)
	}
	return runs, rows.Err()
}
