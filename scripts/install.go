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

func writeEmbeddedFile(filename string, destination string, mode fs.FileMode) error {
	fileContent, err := setupContent.ReadFile(filename)
	if err != nil {
		return err
	}

	filePath := filepath.Join(destination, filename)
	dirName := filepath.Dir(filePath)
	if _, err := os.Stat(dirName); err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		if err := os.MkdirAll(dirName, 0750); err != nil {
			return err
		}
	}
	return os.WriteFile(filePath, fileContent, mode)
}

func RunInstall() {
	dir, err := os.MkdirTemp("", "snapshot-agent")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(dir)

	if err := writeEmbeddedFile("setup/setup.sh", dir, 0700); err != nil {
		log.Fatal(err)
	}

	if err := writeEmbeddedFile("setup/config-template.toml", dir, 0400); err != nil {
		log.Fatal(err)
	}

	if err := writeEmbeddedFile("setup/coriolis-snapshot-agent.service.sample", dir, 0600); err != nil {
		log.Fatal(err)
	}

	execPath, err := os.Executable()
	if err != nil {
		log.Fatalf("could not get the executable path: %q", err)
	}

	scriptPath := filepath.Join(dir, "setup/setup.sh")
	scriptDir := filepath.Dir(scriptPath)
	scriptCmd := fmt.Sprintf("%s %s %s", scriptPath, "-e", execPath)

	cmd := exec.Command("sudo", "-E", "/bin/bash", "-c", scriptCmd)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Dir = scriptDir
	cmd.Run()
}
