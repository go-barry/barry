package cli

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/urfave/cli/v2"
)

//go:embed _starter/** _starter/**/*
var starterFS embed.FS

var InitCommand = &cli.Command{
	Name:  "init",
	Usage: "Create a new Barry project from the default starter",
	Action: func(c *cli.Context) error {
		targetDir, _ := os.Getwd()
		fmt.Println("🚀 Creating Barry project in:", targetDir)

		err := copyEmbeddedDir(starterFS, "_starter", targetDir)
		if err != nil {
			return fmt.Errorf("failed to create project: %w", err)
		}

		mainFile := filepath.Join(targetDir, "main.go")
		modFile := filepath.Join(targetDir, "go.mod")

		if _, err := os.Stat(mainFile); err == nil {
			if _, err := os.Stat(modFile); os.IsNotExist(err) {
				moduleName := filepath.Base(targetDir)
				fmt.Println("🔧 Initialising Go module:", moduleName)

				cmd := exec.Command("go", "mod", "init", moduleName)
				cmd.Stdout = os.Stdout
				cmd.Stderr = os.Stderr
				cmd.Dir = targetDir
				if err := cmd.Run(); err != nil {
					return fmt.Errorf("failed to run go mod init: %w", err)
				}

				cmd = exec.Command("go", "mod", "tidy")
				cmd.Stdout = os.Stdout
				cmd.Stderr = os.Stderr
				cmd.Dir = targetDir
				if err := cmd.Run(); err != nil {
					return fmt.Errorf("failed to run go mod tidy: %w", err)
				}
			}
		}

		fmt.Println("✅ Project created successfully.")
		fmt.Println("▶  Run: barry dev")
		return nil
	},
}

func copyEmbeddedDir(source fs.FS, sourceDir string, targetDir string) error {
	return fs.WalkDir(source, sourceDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(sourceDir, path)
		if err != nil {
			return err
		}

		if rel == "." {
			return nil
		}

		targetPath := filepath.Join(targetDir, rel)

		if d.IsDir() {
			return os.MkdirAll(targetPath, os.ModePerm)
		}

		data, err := fs.ReadFile(source, path)
		if err != nil {
			return err
		}

		if err := os.MkdirAll(filepath.Dir(targetPath), os.ModePerm); err != nil {
			return err
		}

		return os.WriteFile(targetPath, data, 0644)
	})
}
