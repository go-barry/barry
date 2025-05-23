package cli

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
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

		fmt.Println("✅ Project created successfully.")
		fmt.Println("▶  Next steps:")
		fmt.Println("   barry dev")
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

		targetPath := filepath.Join(targetDir, rel)

		if d.IsDir() {
			return os.MkdirAll(targetPath, os.ModePerm)
		}

		data, err := fs.ReadFile(source, path)
		if err != nil {
			fmt.Println("❌ Failed to read:", path)
			return err
		}

		if err := os.MkdirAll(filepath.Dir(targetPath), os.ModePerm); err != nil {
			return err
		}

		err = os.WriteFile(targetPath, data, 0644)
		if err != nil {
			fmt.Println("❌ Failed to write:", targetPath)
			return err
		}

		fmt.Println("📁 Created:", targetPath)
		return nil
	})
}
