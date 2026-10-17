package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"time"
)

const _defaultFileserverURL = "https://46.101.149.241:8443"

func getFileserverURL() string {
	if v := os.Getenv("FILESERVER_URL"); v != "" {
		return v
	}
	return _defaultFileserverURL
}

var ErrNotFoundOnFileServer = fmt.Errorf("NOT FOUND ON FILESERVER")

// listEntry is a lightweight entry for directory listings (no file content)
type listEntry struct {
	Key       string `json:"key"`
	HashValue string `json:"hash_value"`
	Name      string `json:"name"`
	IsDir     bool   `json:"is_dir"`
	Size      int64  `json:"size"`
	Mode      int64  `json:"mode"`
	ModTime   int64  `json:"mod_time"`
}

// getContentsFromFileServer only gets the filenames and metadata - not the actual binary value of the files in the directory.
func (d *Directory) getContentsFromFileServer() ([]listEntry, error) {
	requestUrl := fmt.Sprintf("%s/fetch?filepath=%s/", d.rootFS.fileserverURL, url.QueryEscape(d.path))

	req, err := http.NewRequest("GET", requestUrl, nil)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}

	resp, err := d.rootFS.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error sending request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrNotFoundOnFileServer
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	var entries []KeyValue
	if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
		return nil, fmt.Errorf("error decoding response: %w", err)
	}

	// Convert KeyValue to ListEntry
	result := make([]listEntry, len(entries))
	for i, e := range entries {
		result[i] = listEntry{
			Key:       e.Key,
			HashValue: e.HashValue,
			Name:      e.Name,
			IsDir:     e.IsDir,
			Size:      e.Size,
			Mode:      e.Mode,
			ModTime:   e.ModTime,
		}
	}
	return result, nil
}

// hedgeAfter is how long a single-file fetch may be outstanding before a duplicate
// request is sent (hedged request). Whichever response arrives first is used and the
// other is cancelled. Set with SWAY_HEDGE_MS; 0 or unset turns hedging off.
var hedgeAfter = func() time.Duration {
	ms, err := strconv.Atoi(os.Getenv("SWAY_HEDGE_MS"))
	if err != nil || ms <= 0 {
		return 0
	}
	return time.Duration(ms) * time.Millisecond
}()

// getEntryFromFileServer fetches one file (content included). The returned int
// describes hedging: 0 no hedge sent, 1 hedge sent but the first request won,
// 2 the hedged request won.
func (d *Directory) getEntryFromFileServer(name string) (KeyValue, int, error) {
	path := d.path
	requestUrl := fmt.Sprintf("%s/fetch?filepath=%s", d.rootFS.fileserverURL, url.QueryEscape(path+"/"+name))
	log.Printf("fetching %s", requestUrl)

	if hedgeAfter <= 0 {
		entry, err := d.fetchEntry(context.Background(), requestUrl)
		return entry, 0, err
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	type result struct {
		entry  KeyValue
		err    error
		hedged bool
	}
	results := make(chan result, 2)
	send := func(hedged bool) {
		go func() {
			entry, err := d.fetchEntry(ctx, requestUrl)
			results <- result{entry, err, hedged}
		}()
	}

	send(false)
	outstanding := 1
	fired := false
	timer := time.NewTimer(hedgeAfter)
	defer timer.Stop()

	for {
		select {
		case <-timer.C:
			fired = true
			outstanding++
			send(true)
		case r := <-results:
			outstanding--
			// a transport error on one copy is not final while the other is still in flight
			if r.err != nil && r.err != ErrNotFoundOnFileServer && outstanding > 0 {
				continue
			}
			hedge := 0
			if fired {
				hedge = 1
				if r.hedged {
					hedge = 2
				}
			}
			return r.entry, hedge, r.err
		}
	}
}

func (d *Directory) fetchEntry(ctx context.Context, requestUrl string) (KeyValue, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", requestUrl, nil)
	if err != nil {
		return KeyValue{}, fmt.Errorf("error creating request: %w", err)
	}

	resp, err := d.rootFS.client.Do(req)
	if err != nil {
		return KeyValue{}, fmt.Errorf("error sending request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return KeyValue{}, ErrNotFoundOnFileServer
	}

	if resp.StatusCode != http.StatusOK {
		return KeyValue{}, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	var entry KeyValue
	if err := json.NewDecoder(resp.Body).Decode(&entry); err != nil {
		return KeyValue{}, fmt.Errorf("error decoding response: %w", err)
	}

	return entry, nil
}
