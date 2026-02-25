package main

import (
	"archive/tar"
	"bytes"
	"crypto/sha1"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// Verbose controls logging output
var Verbose bool

func logf(format string, args ...any) {
	if Verbose {
		fmt.Printf(format, args...)
	}
}

func logln(args ...any) {
	if Verbose {
		fmt.Println(args...)
	}
}

// KeyValue represents the JSON structure for set requests
type KeyValue struct {
	Key       string `json:"key"`
	Value     []byte `json:"value"`
	Parent    string `json:"parent"`
	Name      string `json:"name"`
	IsDir     bool   `json:"is_dir"`
	Size      int64  `json:"size"`
	Mode      int64  `json:"mode"`
	ModTime   int64  `json:"mod_time"`
	Uid       int    `json:"uid"`
	Gid       int    `json:"gid"`
	LocalPath string `json:"-"` // on-disk path; content is loaded lazily on upload to not OOM the client.
}

type Symlink struct {
	Name     string // where the symlink EXISTS (the path of the symlink)
	Linkname string // what the symlink POINTS TO (the target path)
}

// SyncEntry is metadata sent to server for sync comparison
type SyncEntry struct {
	Key  string `json:"key"`
	Hash string `json:"hash"`
}

// SyncResponse contains keys that need uploading
type SyncResponse struct {
	NeedUpload []string `json:"need_upload"`
}

// computeHash computes SHA1 hash matching server's algorithm.
// We use the hash to figure out which of the file's the file server already has.
// If LocalPath is set, reads content from disk; otherwise uses Value.
func computeHash(kv KeyValue) string {
	h := sha1.New()
	if kv.IsDir {
		h.Write([]byte(kv.Key))
		return hex.EncodeToString(h.Sum(nil))
	}
	if kv.LocalPath != "" {
		f, err := os.Open(kv.LocalPath)
		if err != nil {
			return ""
		}
		defer f.Close()
		io.Copy(h, f)
	} else {
		h.Write(kv.Value)
	}
	return hex.EncodeToString(h.Sum(nil))
}

// ProgressFunc is called with (filesSent, totalFiles) during upload
type ProgressFunc func(sent, total int)

// extractImage extracts a docker image tarball into a list of files to upload.
// The caller must call os.RemoveAll on the returned tempDir when done.
func extractImage(tarfile string) ([]KeyValue, string, error) {
	tempDir, err := os.MkdirTemp("", "image-extract-")
	if err != nil {
		return nil, "", fmt.Errorf("create temp dir: %w", err)
	}

	tarFile, err := os.Open(tarfile)
	if err != nil {
		os.RemoveAll(tempDir)
		return nil, "", fmt.Errorf("open tarfile: %w", err)
	}
	readLayer(tarFile, tempDir)

	// manifest.json was extracted to tempDir by readLayer above
	manifestData, err := os.ReadFile(filepath.Join(tempDir, "manifest.json"))
	if err != nil {
		os.RemoveAll(tempDir)
		return nil, "", fmt.Errorf("read manifest: %w", err)
	}

	var manifests []Manifest
	if err := json.Unmarshal(manifestData, &manifests); err != nil {
		os.RemoveAll(tempDir)
		return nil, "", fmt.Errorf("cannot unmarshal manifest: %w", err)
	}
	if len(manifests) == 0 {
		os.RemoveAll(tempDir)
		return nil, "", fmt.Errorf("empty manifest.json in tarball")
	}

	logln(manifests[0].Layers)
	rootfsDir := filepath.Join(tempDir, "rootfs")
	if err := os.MkdirAll(rootfsDir, 0755); err != nil {
		os.RemoveAll(tempDir)
		return nil, "", fmt.Errorf("create rootfs dir: %w", err)
	}

	allSymlinks := []Symlink{}
	for _, layer := range manifests[0].Layers {
		f, err := os.Open(filepath.Join(tempDir, layer))
		if err != nil {
			os.RemoveAll(tempDir)
			return nil, "", fmt.Errorf("open layer %s: %w", layer, err)
		}

		logln("layer", f.Name(), layer)
		symlinks, err := readLayer(f, rootfsDir)
		allSymlinks = append(allSymlinks, symlinks...)

		f.Close()
		if err != nil {
			continue
		}
	}

	result, err := walkDirToEntries(rootfsDir)
	if err != nil {
		os.RemoveAll(tempDir)
		return nil, "", fmt.Errorf("walk rootfs: %w", err)
	}

	symlinkEntries, err := buildSymlinkEntries(rootfsDir, allSymlinks)
	if err != nil {
		os.RemoveAll(tempDir)
		return nil, "", fmt.Errorf("build symlink entries: %w", err)
	}

	result = append(result, symlinkEntries...)

	filteredResult := []KeyValue{}
	for _, file := range result {
		if !file.IsDir && file.LocalPath == "" {
			continue
		}

		if !strings.HasPrefix(file.Key, "app/") && file.Key != "app" {
			file.Key = "app/" + file.Key
			if file.Parent == "." {
				file.Parent = "app"
			} else {
				file.Parent = "app/" + file.Parent
			}
		}

		if strings.Contains(file.Key, "libstdc++") {
			logln("file", file.Key, file.Size, file.Parent)
		}

		filteredResult = append(filteredResult, file)
	}

	return filteredResult, tempDir, nil
}

// walkDirToEntries walks a directory and returns KeyValues with LocalPath set.
// No file content is loaded into memory.
func walkDirToEntries(dir string) ([]KeyValue, error) {
	result := []KeyValue{}
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		relPath, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		if relPath == "." {
			return nil
		}

		// skip Docker whiteout files
		if strings.HasPrefix(filepath.Base(relPath), ".wh.") {
			return nil
		}

		kv := KeyValue{
			Key:     filepath.Clean(relPath),
			Name:    filepath.Base(relPath),
			Parent:  filepath.Dir(relPath),
			IsDir:   info.IsDir(),
			Size:    info.Size(),
			Mode:    int64(info.Mode().Perm()),
			ModTime: info.ModTime().Unix(),
		}
		if !info.IsDir() {
			kv.LocalPath = path
		}
		result = append(result, kv)
		return nil
	})
	return result, err
}

// syncNewFiles syncs with the server and returns only the files that need uploading.
func syncNewFiles(files []KeyValue, url string) []KeyValue {
	needUpload := syncFiles(files, url)

	toUpload := make([]KeyValue, 0, len(needUpload))
	for _, f := range files {
		if _, ok := needUpload[f.Key]; ok {
			toUpload = append(toUpload, f)
		}
	}
	return toUpload
}

// uploadFiles uploads files in batches, calling onProgress after each batch.
func uploadFiles(files []KeyValue, url string, onProgress ProgressFunc) {
	batchSize := 100 // files per batch
	sent := 0
	if onProgress != nil {
		onProgress(0, len(files))
	}
	for i := 0; i < len(files); i = i + batchSize {
		end := min(i+batchSize, len(files))
		batch := files[i:end]
		logln("sending batch...", len(batch))

		sendFileBatch(batch, url)
		sent += len(batch)
		if onProgress != nil {
			onProgress(sent, len(files))
		}
	}
}

func readLayer(f *os.File, dstDir string) ([]Symlink, error) {
	symlinks := []Symlink{}
	reader := tar.NewReader(f)
	defer f.Close()
	for {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("error reading tar: %v", err)
		}

		target := filepath.Join(dstDir, header.Name)

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0755); err != nil {
				return nil, fmt.Errorf("mkdir error: %v", err)
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return nil, fmt.Errorf("mkdir error: %v", err)
			}

			outf, err := os.Create(target)
			if err != nil {
				return nil, fmt.Errorf("create file error: %v", err)
			}

			if _, err := io.Copy(outf, reader); err != nil {
				outf.Close()
				return nil, fmt.Errorf("copy file error: %v", err)
			}
			base := filepath.Base(header.Name)
			if strings.Contains(base, "libstdc++") {
				logln(base, header.Name)
				stat, _ := outf.Stat()
				size := stat.Size()
				logln(size)
			}
			outf.Close()
		case tar.TypeSymlink, tar.TypeLink:
			name := filepath.Clean(header.Name)
			link := header.Linkname
			base := filepath.Base(name)

			if strings.Contains(base, "libstdc++") {
				logln("SYMLINK:", name, "->", link, "header", header.Name)
			}

			symlinks = append(symlinks, Symlink{
				Name:     name,
				Linkname: link,
			})

		default:
		}
	}
	return symlinks, nil
}

func buildSymlinkEntries(rootfsDir string, symlinks []Symlink) ([]KeyValue, error) {
	symlinkMap := map[string]string{}
	for _, symlink := range symlinks {
		symlinkMap[filepath.Clean(symlink.Name)] = symlink.Linkname
	}

	out := []KeyValue{}
	for _, symlink := range symlinks {
		resolvedName := filepath.Clean(symlink.Name)
		for {
			innerName, ok := symlinkMap[resolvedName]
			if !ok {
				break
			}
			innerName = filepath.Clean(innerName)
			isAbsolute := strings.HasPrefix(innerName, "/")
			if isAbsolute {
				trimmed := strings.TrimPrefix(innerName, "/")
				innerName = filepath.Clean(trimmed)
			} else {
				linkDir := filepath.Dir(resolvedName)
				joined := filepath.Join(linkDir, innerName)
				innerName = filepath.Clean(joined)
			}

			resolvedName = innerName
		}

		path := filepath.Join(rootfsDir, resolvedName)

		stat, err := os.Stat(path)
		if err != nil {
			continue
		}

		if stat.IsDir() {
			continue
		}

		out = append(out, KeyValue{
			Key:       symlink.Name,
			LocalPath: path,
			Name:      filepath.Base(symlink.Name),
			Parent:    filepath.Dir(symlink.Name),
			Size:      stat.Size(),
			Mode:      int64(stat.Mode().Perm()),
			ModTime:   stat.ModTime().Unix(),
		})
	}

	return out, nil
}

type Manifest struct {
	Config   string   `json:"Config"`
	RepoTags []string `json:"RepoTags"`
	Layers   []string `json:"Layers"`
}

// syncFiles sends file hashes to server and returns set of keys that need uploading
func syncFiles(files []KeyValue, url string) map[string]struct{} {
	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		},
	}

	entries := make([]SyncEntry, len(files))
	for i, f := range files {
		entries[i] = SyncEntry{
			Key:  f.Key,
			Hash: computeHash(f),
		}
	}

	data, err := json.Marshal(entries)
	if err != nil {
		log.Fatalf("Error marshalling sync entries: %v", err)
	}

	req, err := http.NewRequest("POST", url+"/sync", bytes.NewReader(data))
	if err != nil {
		log.Fatalf("Error creating HTTP request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	logln("syncing", len(files), "files with server...")
	resp, err := client.Do(req)
	if err != nil {
		log.Fatalf("Error sending HTTP request: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var syncResp SyncResponse
	if err := json.Unmarshal(body, &syncResp); err != nil {
		log.Fatalf("Error decoding sync response (status %d): %v\nbody: %s", resp.StatusCode, err, string(body))
	}

	needUpload := make(map[string]struct{}, len(syncResp.NeedUpload))
	for _, key := range syncResp.NeedUpload {
		needUpload[key] = struct{}{}
	}

	logf("server says %d files need upload\n", len(needUpload))
	return needUpload
}

func sendFileBatch(files []KeyValue, url string) {
	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		},
	}

	// Load content from disk for files with a LocalPath (deferred from extraction)
	loaded := make([]KeyValue, 0, len(files))
	for _, f := range files {
		if f.LocalPath != "" && !f.IsDir {
			content, err := os.ReadFile(f.LocalPath)
			if err != nil {
				log.Printf("Warning: could not read %s: %v", f.LocalPath, err)
				continue
			}
			f.Value = content
		}
		loaded = append(loaded, f)
	}

	batchFiles, err := json.Marshal(loaded)
	if err != nil {
		log.Fatalf("Error marshalling batch files: %v", err)
	}

	req, err := http.NewRequest("PUT", url+"/batch-upload", bytes.NewReader(batchFiles))
	if err != nil {
		log.Fatalf("Error creating HTTP request: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")

	logln("sending to", url+"/batch-upload")
	resp, err := client.Do(req)
	if err != nil {
		log.Fatalf("Error sending HTTP request: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	logln("response status:", resp.StatusCode, "body:", string(body))
}
