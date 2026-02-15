package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"time"

	"github.com/briandowns/spinner"
	"github.com/fatih/color"
)

func export(verbose bool) error {
	Verbose = verbose
	green := color.New(color.FgGreen).SprintFunc()
	url := "https://46.101.149.241:8443"

	fmt.Println("This can take a few minutes...")
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
	fmt.Printf("%s Built docker image\n", green("✓"))

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

	s.Suffix = " Extracting image..."
	s.Start()
	files := extractImage("test.tar")
	s.Stop()
	fmt.Printf("%s Extracted image (%d files)\n", green("✓"), len(files))

	s.Suffix = " Syncing with fileserver..."
	s.Start()
	toUpload := syncNewFiles(files, url)
	s.Stop()
	fmt.Printf("%s Synced with fileserver — %d new files\n", green("✓"), len(toUpload))

	if len(toUpload) > 0 {
		s.Suffix = " Uploading to fileserver..."
		s.Start()
		uploadFiles(toUpload, url, func(sent, total int) {
			pct := sent * 100 / total
			s.Suffix = fmt.Sprintf(" Uploading to fileserver... %d/%d files (%d%%)", sent, total, pct)
		})
		s.Stop()
		fmt.Printf("%s Uploaded %d files to fileserver\n", green("✓"), len(toUpload))
	}

	os.Remove("test.tar")

	fmt.Printf("\n%s Ready for sway run!\n", green("✓"))

	return nil
}
