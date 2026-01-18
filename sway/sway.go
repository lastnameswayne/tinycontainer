package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path"
	"time"

	"github.com/lastnameswayne/tinycontainer/tarread"
	"github.com/urfave/cli/v2"
)

var server = "https://localhost:8443"

const _publicFileServer = "https://46.101.149.241:8443"
const _appDir = "app"

type RunResponse struct {
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	ExitCode int    `json:"exit_code"`
	Error    string `json:"error,omitempty"`
}

type RunRequest struct {
	FileName string
}

func main() {
	app := &cli.App{
		Name:  "sway",
		Usage: "run a container in the cloud",
		Action: func(*cli.Context) error {
			fmt.Println("hello world")
			return nil
		},
	}

	app.Commands = []*cli.Command{
		{
			Name: "export",
			Action: func(ctx *cli.Context) error {
				start := time.Now()
				fmt.Println("building docker image and generating tar ball...")
				buildCmd := exec.Command("docker", "buildx", "build", "--platform", "linux/amd64", "--tag", "hello-py", "--load", ".")
				saveCmd := exec.Command("docker", "image", "save", "hello-py")
				outputFile, err := os.Create("test.tar")
				if err != nil {
					log.Fatal("error", err)
				}
				defer outputFile.Close()
				saveCmd.Stdout = outputFile
				if err := buildCmd.Run(); err != nil {
					log.Fatal("build", err)
				}
				if err := saveCmd.Run(); err != nil {
					log.Fatal(err)
				}
				fmt.Println("sending to fileserver")
				tarread.Export("test.tar", "https://46.101.149.241:8443")

				fmt.Println("starting worker...")

				//could ssh into worker
				//we need to run
				// sudo runc run <container-id>

				timeElapsed := time.Now().UnixMilli() - start.UnixMilli()
				fmt.Printf("took %d ms", timeElapsed)
				return nil
			},
		},
		{
			Name: "run",
			Action: func(ctx *cli.Context) error {
				username := os.Getenv("SWAY_USERNAME")
				if username == "" {
					return fmt.Errorf("SWAY_USERNAME not set. Run:\n\n  export SWAY_USERNAME=yourname\n")
				}

				start := time.Now()

				if ctx.Args().Len() < 1 {
					return fmt.Errorf("no script given")
				}
				scriptPath := ctx.Args().First()

				//first: seed fileserver with script
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

				timeElapsed := time.Now().UnixMilli() - start.UnixMilli()
				fmt.Printf("took %d ms", timeElapsed)

				return nil
			},
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}
