package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"time"

	"github.com/briandowns/spinner"
	"github.com/fatih/color"
)

func run(scriptPath, username string) error {
	green := color.New(color.FgGreen).SprintFunc()
	red := color.New(color.FgRed).SprintFunc()

	stat, err := os.Stat(scriptPath)
	if err != nil {
		fmt.Printf("%s File not found: %s\n", red("✗"), scriptPath)
		return fmt.Errorf("file not found %s", scriptPath)
	}
	if stat.IsDir() {
		fmt.Printf("%s Path is a directory: %s\n", red("✗"), scriptPath)
		return fmt.Errorf("this is a directory %s", scriptPath)
	}

	scriptName := path.Base(scriptPath)
	fmt.Printf("%s Initialized. Running %s as %s\n", green("✓"), scriptName, username)

	s := spinner.New(spinner.CharSets[14], 100*time.Millisecond)
	s.Suffix = " Uploading script to fileserver..."
	s.Start()

	file, err := os.ReadFile(scriptPath)
	if err != nil {
		s.Stop()
		fmt.Printf("%s Could not read file\n", red("✗"))
		return fmt.Errorf("could not read file")
	}
	withUsername := fmt.Sprintf("%s_app.py", username)
	keyval := KeyValue{
		Key:     fmt.Sprintf("%s/%s", _appDir, withUsername),
		Value:   file,
		Name:    withUsername,
		Parent:  _appDir,
		Size:    stat.Size(),
		Mode:    int64(stat.Mode().Perm()),
		ModTime: stat.ModTime().Unix(),
	}
	sendFileBatch([]KeyValue{keyval}, fileServerURL)

	s.Stop()
	fmt.Printf("%s Uploaded script to fileserver\n", green("✓"))
	fmt.Printf("├── 📦 Script: %s\n", scriptName)
	fmt.Printf("└── 👤 User: %s\n", username)

	s.Suffix = " Running in cloud container..."
	s.Start()

	runRequest := RunRequest{
		FileName: withUsername,
		Username: username,
	}
	marshalled, err := json.Marshal(runRequest)
	if err != nil {
		s.Stop()
		return err
	}

	request, err := http.NewRequest("POST", workerURL+"/run", bytes.NewBuffer(marshalled))
	if err != nil {
		s.Stop()
		return err
	}

	response := RunResponse{}
	resp, err := http.DefaultClient.Do(request)
	if err != nil {
		s.Stop()
		fmt.Printf("%s Failed to connect to container service\n", red("✗"))
		return err
	}
	defer resp.Body.Close()

	bodybytes, err := io.ReadAll(resp.Body)
	if err != nil {
		s.Stop()
		return err
	}
	if err := json.Unmarshal(bodybytes, &response); err != nil {
		s.Stop()
		return fmt.Errorf("invalid response from worker: %w\nbody: %s", err, string(bodybytes))
	}

	s.Stop()

	if response.Error != "" || response.ExitCode != 0 {
		fmt.Printf("%s Container execution failed (exit code %d)\n", red("✗"), response.ExitCode)
		if response.RunId > 0 {
			fmt.Printf("  View failed run at %s/run/%d\n", workerURL, response.RunId)
		}
		if response.Stdout != "" {
			fmt.Printf("\n%s\n", response.Stdout)
		}
		if response.Stderr != "" {
			fmt.Printf("\n%s\n", response.Stderr)
		}
		return fmt.Errorf("script execution failed with exit code %d", response.ExitCode)
	}

	if response.RunId > 0 {
		fmt.Printf("%s Container execution complete. View run at %s/run/%d\n", green("✓"), workerURL, response.RunId)
	} else {
		fmt.Printf("%s Container execution complete\n", green("✓"))
	}

	if response.Stdout != "" {
		fmt.Printf("\n%s\n", response.Stdout)
	}
	if response.Stderr != "" {
		fmt.Printf("\n%s\n", response.Stderr)
	}

	return nil
}
