package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

var _fileserverURL = "https://46.101.149.241:8443"

var ErrNotFoundOnFileServer = fmt.Errorf("NOT FOUND ON FILESERVER")

// listEntry is a lightweight entry for directory listings (no file content)
type listEntry struct {
	Key       string `json:"key"`
	HashValue string `json:"hash_value"`
	Name      string `json:"name"`
	IsDir     bool   `json:"is_dir"`
	Size      int64  `json:"size"`
	Mode      int64  `json:"mode"`
}

// getContentsFromFileServer only gets the filenames and metadata - not the actual binary value of the files in the directory.
func (d *Directory) getContentsFromFileServer() ([]listEntry, error) {
	requestUrl := fmt.Sprintf("%s/fetch?filepath=%s/", _fileserverURL, url.QueryEscape(d.path))

	req, err := http.NewRequest("GET", requestUrl, nil)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}

	resp, err := d.fs.client.Do(req)
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
		}
	}
	return result, nil
}

func (d *Directory) getEntryFromFileServer(name string) (KeyValue, error) {
	path := d.path
	requestUrl := fmt.Sprintf("%s/fetch?filepath=%s", _fileserverURL, url.QueryEscape(path+"/"+name))
	fmt.Println("CALLING URL WITH", requestUrl)

	req, err := http.NewRequest("GET", requestUrl, nil)
	if err != nil {
		return KeyValue{}, fmt.Errorf("error creating request: %w", err)
	}

	resp, err := d.fs.client.Do(req)
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
