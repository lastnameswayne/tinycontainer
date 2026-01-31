package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

const _fileserverURL = "https://46.101.149.241:8443"

// getContentsFromFileServer only gets the filenames and metadata - not the actual binary value of the files in the directory.
func (d *Directory) getContentsFromFileServer() ([]ListEntry, error) {
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
	result := make([]ListEntry, len(entries))
	for i, e := range entries {
		result[i] = ListEntry{
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

func (d *Directory) getDataFromFileServer(name string) (KeyValue, error) {
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

func (d *Directory) getFileFromFileServer(name string) (*KeyValue, string, error) {
	entry, err := d.getDataFromFileServer(name)
	if err != nil {
		return nil, "", err
	}

	return &entry, entry.HashValue, nil
}
