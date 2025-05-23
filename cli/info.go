package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/callumeddisford/barry/core"
	"github.com/urfave/cli/v2"
)

var InfoCommand = &cli.Command{
	Name:  "info",
	Usage: "Print project structure and cache summary",
	Action: func(c *cli.Context) error {
		config := core.LoadConfig("barry.config.yml")

		fmt.Println("📁 Output Directory:", config.OutputDir)
		fmt.Println("🔁 Cache Enabled:", config.CacheEnabled)
		fmt.Println("🔁 Debug Headers Enabled:", config.DebugHeaders)
		fmt.Println()

		componentCount := 0
		filepath.Walk("components", func(path string, info os.FileInfo, err error) error {
			if err == nil && !info.IsDir() && strings.HasSuffix(path, ".html") {
				componentCount++
			}
			return nil
		})

		routeCount := 0
		filepath.Walk("routes", func(path string, info os.FileInfo, err error) error {
			if err == nil && info.IsDir() {
				indexFile := filepath.Join(path, "index.html")
				if _, err := os.Stat(indexFile); err == nil {
					routeCount++
				}
			}
			return nil
		})

		cacheCount := 0
		filepath.Walk(config.OutputDir, func(path string, info os.FileInfo, err error) error {
			if err == nil && !info.IsDir() && strings.HasSuffix(path, ".html") {
				cacheCount++
			}
			return nil
		})

		fmt.Println("🗂️  Routes Found:", routeCount)
		fmt.Println("📦 Components Found:", componentCount)
		fmt.Println("💾 Cached Pages:", cacheCount)

		return nil
	},
}
