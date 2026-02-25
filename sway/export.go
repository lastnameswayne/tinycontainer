package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/briandowns/spinner"
	"github.com/fatih/color"
)

const _imageTar = "image.tar"

func export(verbose bool) error {
	Verbose = verbose
	green := color.New(color.FgGreen).SprintFunc()

	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("could not get working directory: %w", err)
	}
	imageName := "sway-" + filepath.Base(cwd)

	fmt.Println("This can take a few minutes...")
	s := spinner.New(spinner.CharSets[14], 100*time.Millisecond)
	s.Suffix = " Building docker image..."
	s.Start()

	buildCmd := exec.Command("docker", "buildx", "build", "--platform", "linux/amd64", "--tag", imageName, "--load", ".")
	buildCmd.Stderr = os.Stderr
	if err := buildCmd.Run(); err != nil {
		s.Stop()
		color.Red("✗ Build failed")
		log.Fatal(err)
	}
	s.Stop()
	fmt.Printf("%s Built docker image\n", green("✓"))

	s.Suffix = " Saving image to tarball..."
	s.Start()

	saveCmd := exec.Command("docker", "image", "save", imageName)
	outputFile, err := os.Create(_imageTar)
	if err != nil {
		s.Stop()
		log.Fatal("error", err)
	}
	defer outputFile.Close()
	defer os.Remove(_imageTar)
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
	files, tempDir, err := extractImage(_imageTar)
	if err != nil {
		s.Stop()
		return fmt.Errorf("extracting image: %w", err)
	}
	defer os.RemoveAll(tempDir)
	s.Stop()
	fmt.Printf("%s Extracted image (%d files)\n", green("✓"), len(files))

	s.Suffix = " Syncing with fileserver..."
	s.Start()
	toUpload := syncNewFiles(files, fileServerURL)
	s.Stop()
	fmt.Printf("%s Synced with fileserver — %d new files\n", green("✓"), len(toUpload))

	if len(toUpload) > 0 {
		s.Suffix = " Uploading to fileserver..."
		s.Start()
		uploadFiles(toUpload, fileServerURL, func(sent, total int) {
			pct := sent * 100 / total
			s.Suffix = fmt.Sprintf(" Uploading to fileserver... %d/%d files (%d%%)", sent, total, pct)
		})
		s.Stop()
		fmt.Printf("%s Uploaded %d files to fileserver\n", green("✓"), len(toUpload))
	}

	os.Remove(_imageTar)

	fmt.Printf("\n%s Ready for sway run!\n", green("✓"))

	return nil
}
