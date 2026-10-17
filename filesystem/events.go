package main

import (
	"log"
	"os"
	"sync/atomic"
	"time"

	"github.com/lastnameswayne/tinycontainer/db"
)

// Per-file event logging for the write-up experiments. Every FUSE Lookup/Open/Read
// emits one event. Events go through a buffered channel and are inserted in batches
// by a single goroutine so the FUSE handlers never wait on SQLite.
// Set SWAY_EVENTS=0 to turn logging off (used to measure its overhead).

const (
	tierMem      = "mem"
	tierDisk     = "disk"
	tierRemote   = "remote"
	tierNegative = "negative"
	tierError    = "error"
)

// currentRunID is the runs.id of the run in progress, 0 between runs.
// Assumes one run at a time, which is how the experiments drive the worker.
var currentRunID atomic.Int64

var eventsEnabled = os.Getenv("SWAY_EVENTS") != "0"

var eventCh = make(chan db.FileEvent, 1<<16)

// droppedEvents counts events lost because the channel was full.
var droppedEvents atomic.Int64

// emitEvent records one file event. hit means the fileserver was not contacted.
func emitEvent(op, path string, sizeBytes int64, tier string, start time.Time, hit bool, hedge int) {
	if !eventsEnabled {
		return
	}
	now := time.Now()
	ev := db.FileEvent{
		RunID:     currentRunID.Load(),
		TsUs:      start.UnixMicro(),
		Op:        op,
		Path:      path,
		SizeBytes: sizeBytes,
		Tier:      tier,
		LatencyUs: now.Sub(start).Microseconds(),
		Hit:       hit,
		Hedge:     hedge,
	}
	select {
	case eventCh <- ev:
	default:
		droppedEvents.Add(1)
	}
}

// startEventWriter drains eventCh into SQLite in batches.
func startEventWriter() {
	go func() {
		const maxBatch = 2000
		batch := make([]db.FileEvent, 0, maxBatch)
		ticker := time.NewTicker(200 * time.Millisecond)
		defer ticker.Stop()
		flush := func() {
			if len(batch) == 0 || db.DB == nil {
				batch = batch[:0]
				return
			}
			if err := db.InsertFileEvents(batch); err != nil {
				log.Printf("error inserting file events: %v", err)
			}
			batch = batch[:0]
		}
		for {
			select {
			case ev := <-eventCh:
				batch = append(batch, ev)
				if len(batch) >= maxBatch {
					flush()
				}
			case <-ticker.C:
				flush()
			}
		}
	}()
}

// waitForEventDrain blocks until every event emitted so far has been written, so a
// run's events are complete by the time /run returns.
func waitForEventDrain() {
	deadline := time.Now().Add(10 * time.Second)
	for len(eventCh) > 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	// one more tick so the final partial batch is flushed
	time.Sleep(250 * time.Millisecond)
}
