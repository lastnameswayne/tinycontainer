package main

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/fatih/color"
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
	Username string
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
				err := export()
				if err != nil {

				}
				bold := color.New(color.Bold).SprintFunc()

				elapsed := time.Since(start)
				fmt.Printf("\n%s Export completed in %s\n", bold("Done!"), elapsed.Round(time.Millisecond))
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
				if ctx.Args().Len() < 1 {
					return fmt.Errorf("no script given")
				}

				start := time.Now()
				scriptPath := ctx.Args().First()
				err := run(scriptPath, username)
				if err != nil {
					return err
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
