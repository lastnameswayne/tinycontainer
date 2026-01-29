package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"time"

	"github.com/briandowns/spinner"
	"github.com/fatih/color"
	"github.com/lastnameswayne/tinycontainer/tarread"
)

func export(verbose bool) error {
	tarread.Verbose = verbose
	green := color.New(color.FgGreen).SprintFunc()

	s := spinner.New(spinner.CharSets[14], 100*time.Millisecond)
	s.Suffix = " Building docker image..."
	s.Start()

	buildCmd := exec.Command("docker", "buildx", "build", "--platform", "linux/amd64", "--tag", "hello-py", "--load", ".")
	if err := buildCmd.Run(); err != nil {
		s.Stop()
		color.Red("✗ Build failed")
		log.Fatal("build", err)
	}
	s.Stop()
	fmt.Printf("%s Building docker image\n", green("✓"))

	s.Suffix = " Saving image to tarball..."
	s.Start()

	saveCmd := exec.Command("docker", "image", "save", "hello-py")
	outputFile, err := os.Create("test.tar")
	if err != nil {
		s.Stop()
		log.Fatal("error", err)
	}
	defer outputFile.Close()
	saveCmd.Stdout = outputFile
	if err := saveCmd.Run(); err != nil {
		s.Stop()
		color.Red("✗ Save failed")
		log.Fatal(err)
	}
	s.Stop()
	fmt.Printf("%s Saved tarball\n", green("✓"))

	s.Suffix = " Uploading to fileserver..."
	s.Start()
	tarread.Export("test.tar", "https://46.101.149.241:8443")
	s.Stop()
	fmt.Printf("%s Uploaded to fileserver\n", green("✓"))

	os.Remove("test.tar")

	return nil
}
