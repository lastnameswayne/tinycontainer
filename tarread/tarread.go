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
	"path/filepath"
)

// KeyValue represents the JSON structure for set requests
type KeyValue struct {
	Key   string `json:"key"`
	Value []byte `json:"value"` // Base64 encoded string of the binary data
}

func Export(tarfile string, url string) {
	result := make([]KeyValue, 0)

	f, err := os.Open(tarfile)
	if err != nil {
		panic(err)
	}

	reader := tar.NewReader(f)
	for {
		fmt.Println("here")
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

		fmt.Println("filepath", filepathStr)
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

	for _, file := range result {
		fmt.Println(file.Key)

		// Create a new HTTP client
		client := &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					InsecureSkipVerify: true,
				},
			},
		}

		// Marshal the KeyValue struct into JSON
		jsonData, err := json.Marshal(file)
		if err != nil {
			log.Fatalf("Error encoding JSON: %v", err)
		}

		// Create a new HTTP request
		fmt.Println("sending req", jsonData)
		req, err := http.NewRequest("POST", url+"/upload", bytes.NewBuffer(jsonData))
		if err != nil {
			log.Fatalf("Error creating HTTP request: %v", err)
		}

		// Set the content type to application/json
		req.Header.Set("Content-Type", "application/json")

		// Send the HTTP request
		resp, err := client.Do(req)
		if err != nil {
			log.Fatalf("Error sending HTTP request: %v", err)
		}

		// Close the response body
		defer resp.Body.Close()
	}
}
