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
				cmd := exec.Command("docker build --tag hello-py .")
				if err := cmd.Run(); err != nil {
					log.Fatal(err)
				}
				cmd = exec.Command("docker image save hello-py >test.tar")
				if err := cmd.Run(); err != nil {
					log.Fatal(err)
				}
				fmt.Println("sending to fileserver")
				tarread.Export("test.tar", "135.181.157.206")

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
