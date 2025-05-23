package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/callumeddisford/barry/core"
	"github.com/urfave/cli/v2"
)

var CleanCommand = &cli.Command{
	Name:      "clean",
	Usage:     "Delete cached HTML from the output directory (default: outputDir in barry.config.yml)",
	ArgsUsage: "[route (optional)]",
	Action: func(c *cli.Context) error {
		config := core.LoadConfig("barry.config.yml")
		target := config.OutputDir

		if c.Args().Len() > 0 {
			route := c.Args().Get(0)
			route = strings.TrimPrefix(route, "/")
			target = filepath.Join(config.OutputDir, route)
		}

		info, err := os.Stat(target)
		if err != nil {
			if os.IsNotExist(err) {
				fmt.Println("🧼 Nothing to clean:", target)
				return nil
			}
			return fmt.Errorf("failed to access path: %w", err)
		}

		if !info.IsDir() {
			return fmt.Errorf("not a directory: %s", target)
		}

		fmt.Println("🧹 Cleaning:", target)
		err = os.RemoveAll(target)
		if err != nil {
			return fmt.Errorf("failed to clean cache: %w", err)
		}

		fmt.Println("✅ Done.")
		return nil
	},
}
