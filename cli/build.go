package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/urfave/cli/v2"
)

var buildExecCommand = exec.Command
var osWriteFileFunc = os.WriteFile
var osMkdirAllFunc = os.MkdirAll

func getGoModuleName() (string, error) {
	data, err := os.ReadFile("go.mod")
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "module ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "module ")), nil
		}
	}
	return "", fmt.Errorf("module path not found in go.mod")
}

var BuildCommand = &cli.Command{
	Name:  "build",
	Usage: "Compile all .server.go files into .so plugins for production use",
	Action: func(c *cli.Context) error {
		modName, err := getGoModuleName()
		if err != nil {
			return fmt.Errorf("failed to determine module name from go.mod: %w", err)
		}

		for _, root := range []string{"routes", "api"} {
			err = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
				if err != nil || d.IsDir() {
					return nil
				}
				if filepath.Base(path) != "index.server.go" {
					return nil
				}

				dir := filepath.Dir(path)
				pluginOut := filepath.Join(dir, "index.server.so")

				relImport := strings.TrimPrefix(dir, root)
				relImport = strings.TrimPrefix(relImport, string(filepath.Separator))
				relImport = filepath.ToSlash(filepath.Join(root, relImport))
				importPath := fmt.Sprintf("%s/%s", modName, relImport)

				tmpFile := filepath.Join(".barry-tmp", relImport, "plugin_wrapper.go")

				if err := osMkdirAllFunc(filepath.Dir(tmpFile), os.ModePerm); err != nil {
					return fmt.Errorf("failed to create wrapper directory: %w", err)
				}

				wrapper := fmt.Sprintf(`package main

import user "%s"

import (
	"net/http"
)

func HandleRequest(r *http.Request, p map[string]string) (map[string]interface{}, error) {
	return user.HandleRequest(r, p)
}
`, importPath)

				if err := osWriteFileFunc(tmpFile, []byte(wrapper), 0644); err != nil {
					return fmt.Errorf("failed to write wrapper for %s: %w", path, err)
				}

				cmd := buildExecCommand("go", "build", "-buildmode=plugin", "-o", pluginOut, tmpFile)
				cmd.Dir = "."
				cmd.Stdout = os.Stdout
				cmd.Stderr = os.Stderr
				cwd, _ := os.Getwd()
				fmt.Println("📦 Building from:", cwd)
				fmt.Println("🔧 Building:", pluginOut)
				if err := cmd.Run(); err != nil {
					return fmt.Errorf("failed to build plugin for %s: %w", path, err)
				}
				return nil
			})

			if err != nil {
				return err
			}
		}

		fmt.Println("✅ All plugins built successfully.")
		return nil
	},
}
