package tarread

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
	"strings"

	"path/filepath"
)

// KeyValue represents the JSON structure for set requests
type KeyValue struct {
	Key     string `json:"key"`      // Full path
	Value   []byte `json:"value"`    // Base64 encoded binary data
	Parent  string `json:"parent"`   // Parent directory path
	Name    string `json:"name"`     // Basename
	IsDir   bool   `json:"is_dir"`   // True if directory
	Size    int64  `json:"size"`     // File size in bytes
	Mode    int64  `json:"mode"`     // File permissions
	ModTime int64  `json:"mod_time"` // Last modified timestamp
	Uid     int    `json:"uid"`      // Owner user ID
	Gid     int    `json:"gid"`      // Owner group ID
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

// computeHash computes SHA1 hash matching server's algorithm
func computeHash(kv KeyValue) string {
	h := sha1.New()
	if kv.IsDir {
		h.Write([]byte(kv.Key))
	}
	h.Write(kv.Value)
	return hex.EncodeToString(h.Sum(nil))
}

func Export(tarfile string, url string) {
	tempDir, err := os.MkdirTemp("", "image-extract-")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(tempDir)
	tarFile, err := os.Open(tarfile)
	readLayer(tarFile, tempDir)

	//find manifest.json
	result, err := tarFileToEntries(tarfile)
	if err != nil {
		panic(err)
	}
	var manifest KeyValue
	for _, e := range result {
		if strings.Contains(e.Key, "manifest.json") {
			manifest = e
		}
	}

	var manifests []Manifest
	if err := json.Unmarshal(manifest.Value, &manifests); err != nil {
		log.Fatalf("Cannot unmarshal manifest: %v", err)
	}
	if len(manifests) == 0 {
		panic("err")
	}

	fmt.Println(manifests[0].Layers)
	rootfsDir := "/tmp/rootfs"
	if err := os.MkdirAll(rootfsDir, 0755); err != nil {
		log.Fatalf("Cannot create rootfsDir: %v", err)
	}

	allSymlinks := []Symlink{}
	for _, layer := range manifests[0].Layers {
		f, err := os.Open(filepath.Join(tempDir, layer))
		if err != nil {
			panic(err)
		}

		fmt.Println("layer", f.Name(), layer)
		symlinks, err := readLayer(f, rootfsDir)
		allSymlinks = append(allSymlinks, symlinks...)

		f.Close()
		if err != nil {
			continue
		}
	}

	finalTar := filepath.Join(tempDir, "final.tar")
	if err := createTarFromDir(rootfsDir, finalTar); err != nil {
		log.Fatalf("Failed to create final tar: %v", err)
	}

	result, err = tarFileToEntries(finalTar)
	if err != nil {
		panic(err)
	}

	symlinkEntries, err := buildSymlinkEntries(rootfsDir, allSymlinks)
	if err != nil {
		panic(err)
	}

	result = append(result, symlinkEntries...)

	filteredResult := []KeyValue{}
	for _, file := range result {
		if len(file.Value) == 0 && !file.IsDir {

			continue
		}

		// Prefix paths with "app/" if not already prefixed
		if !strings.HasPrefix(file.Key, "app/") && file.Key != "app" {
			file.Key = "app/" + file.Key
			if file.Parent == "." {
				file.Parent = "app"
			} else {
				file.Parent = "app/" + file.Parent
			}
		}

		if strings.Contains(file.Key, "libstdc++") {
			fmt.Println("file", file.Key, len(file.Value), file.Size, file.Parent)
		}

		filteredResult = append(filteredResult, file)
	}

	// Sync with server to find which files need uploading
	needUpload := SyncFiles(filteredResult, url)

	if len(needUpload) == 0 {
		fmt.Println("all files already up to date!")
		return
	}

	// Filter to only files that need uploading
	toUpload := make([]KeyValue, 0, len(needUpload))
	for _, f := range filteredResult {
		if _, ok := needUpload[f.Key]; ok {
			toUpload = append(toUpload, f)
		}
	}

	fmt.Printf("uploading %d files (skipping %d unchanged)\n", len(toUpload), len(filteredResult)-len(toUpload))

	batchSize := 1000
	for i := 0; i < len(toUpload); i = i + batchSize {
		start := i
		end := min(i+batchSize, len(toUpload))
		batch := toUpload[start:end]
		fmt.Println("sending batch...", len(batch))

		SendFileBatch(batch, url)
	}
}

func tarFileToEntries(path string) ([]KeyValue, error) {
	result := make([]KeyValue, 0)

	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}

	reader := tar.NewReader(f)
	for {
		header, err := reader.Next()
		if err == io.EOF {
			fmt.Println("END OF FILE")
			break
		}
		if err != nil {
			log.Printf("err %v", err)
			break
		}

		buf := bytes.NewBuffer(make([]byte, 0, header.Size))
		io.Copy(buf, reader)
		filepathStr := filepath.Clean(header.Name)
		// _, base := filepath.Split(filepathStr)

		// fmt.Println("filepath", filepathStr)
		// attr := attrFromHeader(header)

		isDir := header.Typeflag == tar.TypeDir
		content := []byte{}
		if !isDir {
			content = buf.Bytes()
		}
		kv := KeyValue{
			Key:     filepathStr,
			Value:   content,
			Name:    filepath.Base(header.Name),
			Parent:  filepath.Dir(header.Name),
			IsDir:   isDir,
			Size:    header.Size,
			Mode:    header.Mode,
			ModTime: header.ModTime.Unix(),
			Uid:     header.Uid,
			Gid:     header.Gid,
		}
		result = append(result, kv)
	}

	return result, nil
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
				fmt.Println(base, header.Name)
				stat, _ := outf.Stat()
				size := stat.Size()
				fmt.Println(size)
			}
			outf.Close()
		case tar.TypeSymlink, tar.TypeLink:
			name := filepath.Clean(header.Name) // normalize
			link := header.Linkname             // keep raw; resolve later with name context
			base := filepath.Base(name)

			if strings.Contains(base, "libstdc++") {
				fmt.Println("SYMLINK:", name, "->", link, "header", header.Name)
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
	// create a map from the symlink name to the real file name
	symlinkMap := map[string]string{}
	for _, symlink := range symlinks {
		symlinkMap[filepath.Clean(symlink.Name)] = symlink.Linkname
	}

	out := []KeyValue{}
	for _, symlink := range symlinks {
		// recursively look up in symlink map until you find the leaf

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

		file, err := os.ReadFile(path)
		if err != nil {
			continue
		}

		out = append(out, KeyValue{
			Key:     symlink.Name,
			Value:   file,
			Name:    filepath.Base(symlink.Name),
			Parent:  filepath.Dir(symlink.Name),
			Size:    stat.Size(),
			Mode:    int64(stat.Mode().Perm()),
			ModTime: stat.ModTime().Unix(),
		})
	}

	return out, nil
}

func createSymlinkMapFromLayer(result []KeyValue, symlinks []Symlink) []KeyValue {
	keyValues := []KeyValue{}
	for _, symlink := range symlinks {
		for _, val := range result {
			if strings.Contains(symlink.Linkname, "ld-linux-x86-64") && strings.Contains(val.Key, "ld-linux-x86-64") {
				fmt.Println(symlink.Linkname, symlink.Name, val.Key)
			}
			if symlink.Linkname != val.Key {
				continue
			}
			fmt.Print("found", symlink.Name, len(val.Value))
			keyValue := KeyValue{
				Key:   symlink.Name,
				Value: val.Value,
			}

			keyValues = append(keyValues, keyValue)
		}

	}

	return keyValues

}

func createTarFromDir(dir string, out string) error {
	outFile, err := os.Create(out)
	if err != nil {
		return err
	}
	tw := tar.NewWriter(outFile)
	defer tw.Close()

	return filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
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

		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}

		header.Name = relPath
		if err := tw.WriteHeader(header); err != nil {
			return err
		}

		if !info.IsDir() {
			f, err := os.Open(path)
			if err != nil {
				return err
			}
			if _, err := io.Copy(tw, f); err != nil {
				f.Close()
				return err
			}
			f.Close()
		}

		return nil
	})

}

type Manifest struct {
	Config   string   `json:"Config"`
	RepoTags []string `json:"RepoTags"`
	Layers   []string `json:"Layers"`
}

func SendFile(file KeyValue, url string) {
	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		},
	}

	req, err := http.NewRequest("PUT", url+"/upload", bytes.NewReader(file.Value))
	if err != nil {
		log.Fatalf("Error creating HTTP request: %v", err)
	}

	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("X-File-Name", file.Key)

	resp, err := client.Do(req)
	if err != nil {
		log.Fatalf("Error sending HTTP request: %v", err)
	}

	// Close the response body
	defer resp.Body.Close()
}

// SyncFiles sends file hashes to server and returns set of keys that need uploading
func SyncFiles(files []KeyValue, url string) map[string]struct{} {
	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		},
	}

	// Build sync entries with hashes
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

	fmt.Println("syncing", len(files), "files with server...")
	resp, err := client.Do(req)
	if err != nil {
		log.Fatalf("Error sending HTTP request: %v", err)
	}
	defer resp.Body.Close()

	var syncResp SyncResponse
	if err := json.NewDecoder(resp.Body).Decode(&syncResp); err != nil {
		log.Fatalf("Error decoding sync response: %v", err)
	}

	// Convert to set for O(1) lookup
	needUpload := make(map[string]struct{}, len(syncResp.NeedUpload))
	for _, key := range syncResp.NeedUpload {
		needUpload[key] = struct{}{}
	}

	fmt.Printf("server says %d files need upload\n", len(needUpload))
	return needUpload
}

func SendFileBatch(files []KeyValue, url string) {
	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		},
	}

	batchFiles, err := json.Marshal(files)
	if err != nil {
		log.Fatalf("Error marshalling batch files: %v", err)
	}

	req, err := http.NewRequest("PUT", url+"/batch-upload", bytes.NewReader(batchFiles))
	if err != nil {
		log.Fatalf("Error creating HTTP request: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")

	fmt.Println("sending to", url+"/batch-upload")
	resp, err := client.Do(req)
	if err != nil {
		log.Fatalf("Error sending HTTP request: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	fmt.Println("response status:", resp.StatusCode, "body:", string(body))
}
