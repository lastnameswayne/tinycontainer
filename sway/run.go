package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"

	"github.com/lastnameswayne/tinycontainer/tarread"
)

func run(scriptPath, username string) error {
	stat, err := os.Stat(scriptPath)
	if err != nil {
		return fmt.Errorf("file not found %s", scriptPath)
	}
	if stat.IsDir() {
		return fmt.Errorf("this is a directory %s", scriptPath)
	}

	file, err := os.ReadFile(scriptPath)
	if err != nil {
		return fmt.Errorf("could not read file")
	}
	scriptName := path.Base(scriptPath)
	withUsername := fmt.Sprintf("%s_%s", username, scriptName)
	keyval := tarread.KeyValue{
		Key:     fmt.Sprintf("%s/%s", _appDir, withUsername),
		Value:   file,
		Name:    withUsername,
		Parent:  _appDir,
		Size:    stat.Size(),
		Mode:    int64(stat.Mode().Perm()),
		ModTime: stat.ModTime().Unix(),
	}
	tarread.SendFileBatch([]tarread.KeyValue{keyval}, _publicFileServer)
	runRequest := RunRequest{
		FileName: withUsername,
		Username: username,
	}
	marshalled, err := json.Marshal(runRequest)
	if err != nil {
		return err
	}

	request, err := http.NewRequest("POST", "http://167.71.54.99:8444/run", bytes.NewBuffer(marshalled))
	if err != nil {
		return err
	}

	response := RunResponse{}
	resp, err := http.DefaultClient.Do(request)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	bodybytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	json.Unmarshal(bodybytes, &response)

	if response.Error != "" || response.ExitCode != 0 {
		fmt.Printf("Could not run script %s. Error:", scriptPath)
		fmt.Println(response.ExitCode, response.Error)
	} else {
		fmt.Println(response.Stdout)
	}
	return nil
}
