package db

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRunLifecycleAndEvents(t *testing.T) {
	require.NoError(t, Init(filepath.Join(t.TempDir(), "runs.db")))
	// Init is idempotent (the ALTERs must not fail on a second start)
	require.NoError(t, Init(filepath.Join(t.TempDir(), "runs.db")))

	id, err := StartRun("u_app.py", time.Now(), "u")
	require.NoError(t, err)

	runs, err := GetAllRuns()
	require.NoError(t, err)
	require.Len(t, runs, 0, "in-progress runs are hidden from /stats")

	require.NoError(t, InsertFileEvents([]FileEvent{
		{RunID: id, TsUs: 1, Op: "lookup", Path: "app/a", SizeBytes: 10, Tier: "remote", LatencyUs: 5},
		{RunID: id, TsUs: 2, Op: "read", Path: "app/a", SizeBytes: 10, Tier: "mem", LatencyUs: 1, Hit: true},
	}))
	require.NoError(t, FinishRun(id, RunResult{DurationMs: 7, FirstOutputMs: 3, ExitMs: 6, HandlerMs: 8, Stdout: "hi"}))

	runs, err = GetAllRuns()
	require.NoError(t, err)
	require.Len(t, runs, 1)
	require.Equal(t, "hi", runs[0].Stdout)

	var n, remoteBytes int64
	require.NoError(t, DB.QueryRow(`SELECT COUNT(*), SUM(CASE WHEN tier='remote' THEN size_bytes END) FROM file_events WHERE run_id=?`, id).Scan(&n, &remoteBytes))
	require.Equal(t, int64(2), n)
	require.Equal(t, int64(10), remoteBytes)
}
