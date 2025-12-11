package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"time"

	"github.com/lastnameswayne/tinycontainer/tarread"
	"github.com/urfave/cli/v2"
)

var server = "https://localhost:8443"

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
			Name: "run",
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
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}
