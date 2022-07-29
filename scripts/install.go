package scripts

import (
	"embed"
	"fmt"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"path/filepath"
)

//go:embed setup/setup.sh
//go:embed setup/config-template.toml
//go:embed setup/coriolis-snapshot-agent.service.sample
var setupContent embed.FS

const (
	setupTmpDir = "/tmp/setup"
)

func writeEmbeddedFile(filename string, mode fs.FileMode) error {
	fileContent, err := setupContent.ReadFile(filename)
	if err != nil {
		return err
	}

	return os.WriteFile(filepath.Join("/tmp", filename), fileContent, mode)
}

func RunInstall() {
	if err := os.MkdirAll(setupTmpDir, 0700); err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(setupTmpDir)

	if err := writeEmbeddedFile("setup/setup.sh", 0700); err != nil {
		log.Fatal(err)
	}

	if err := writeEmbeddedFile("setup/config-template.toml", 0400); err != nil {
		log.Fatal(err)
	}

	if err := writeEmbeddedFile("setup/coriolis-snapshot-agent.service.sample", 0600); err != nil {
		log.Fatal(err)
	}

	execPath, err := os.Executable()
	if err != nil {
		log.Fatalf("could not get the executable path: %q", err)
	}

	scriptCmd := fmt.Sprintf("%s %s %s", filepath.Join(setupTmpDir, "setup.sh"), "-e", execPath)

	cmd := exec.Command("sudo", "-E", "/bin/bash", "-c", scriptCmd)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Dir = setupTmpDir
	cmd.Run()
}
