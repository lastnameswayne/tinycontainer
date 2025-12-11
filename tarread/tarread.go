package tarread

import (
	"archive/tar"
	"bytes"
	"crypto/tls"
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
	Key   string `json:"key"`
	Value []byte `json:"value"` // Base64 encoded string of the binary data
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

	for _, layer := range manifests[0].Layers {
		f, err := os.Open(filepath.Join(tempDir, layer))
		if err != nil {
			panic(err)
		}

		fmt.Println("layer", f.Name(), layer)
		err = readLayer(f, rootfsDir)
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
	// for _, file := range result {
	// 	sendFile(file, url)
	// }
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

		if header.Typeflag == tar.TypeDir {
			continue
		} else {
			// Handle files
			content := buf.Bytes()
			kv := KeyValue{
				Key:   filepathStr,
				Value: content,
			}
			result = append(result, kv)
		}
	}

	return result, nil
}

func readLayer(f *os.File, dstDir string) error {
	reader := tar.NewReader(f)
	defer f.Close()
	for {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("error reading tar: %v", err)

		}

		target := filepath.Join(dstDir, header.Name)

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0755); err != nil {
				return fmt.Errorf("mkdir error: %v", err)

			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return fmt.Errorf("mkdir error: %v", err)
			}

			outf, err := os.Create(target)
			if err != nil {
				return fmt.Errorf("create file error: %v", err)

			}

			if _, err := io.Copy(outf, reader); err != nil {
				outf.Close()
				return fmt.Errorf("copy file error: %v", err)
			}
			base := filepath.Base(header.Name)
			if strings.Contains(base, "python3.10") {
				fmt.Println(base, header.Name)
				stat, _ := outf.Stat()
				size := stat.Size()
				fmt.Println(size)
			}
			outf.Close()
		default:
		}
	}
	return nil

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

func sendFile(file KeyValue, url string) {
	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		},
	}
	// fmt.Println("sencding", file.Key, "of size", len(file.Value))

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
