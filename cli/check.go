package cli

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/go-barry/barry/core"
	"github.com/urfave/cli/v2"
)

var CheckCommand = &cli.Command{
	Name:  "check",
	Usage: "Validate templates, components, and layouts",
	Action: func(c *cli.Context) error {
		var failed bool

		var components []string
		filepath.Walk("components", func(path string, info os.FileInfo, err error) error {
			if err == nil && !info.IsDir() && strings.HasSuffix(path, ".html") {
				components = append(components, path)
			}
			return nil
		})

		filepath.Walk("routes", func(path string, info os.FileInfo, err error) error {
			if err != nil || !info.IsDir() {
				return nil
			}

			htmlPath := filepath.Join(path, "index.html")
			if _, err := os.Stat(htmlPath); err != nil {
				return nil
			}

			layoutPath := ""
			if content, err := os.ReadFile(htmlPath); err == nil {
				lines := strings.Split(string(content), "\n")
				for _, line := range lines {
					line = strings.TrimSpace(line)
					if strings.HasPrefix(line, "<!-- layout:") && strings.HasSuffix(line, "-->") {
						layoutPath = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(line, "<!-- layout:"), "-->"))
						break
					}
				}
			}

			files := append([]string{htmlPath}, components...)
			if layoutPath != "" {
				files = append([]string{layoutPath}, files...)
			}

			rel, _ := filepath.Rel("routes", path)
			if rel == "." {
				rel = "/"
			} else {
				rel = "/" + rel
			}

			var tmpl *template.Template
			tmpl = template.New(filepath.Base(files[0])).Funcs(core.BarryTemplateFuncs("dev", "cache"))
			tmpl, err = tmpl.ParseFiles(files...)

			if err != nil {
				failed = true
				fmt.Printf("❌ %s → parse error: %v\n", rel, err)
				return nil
			}

			var buf bytes.Buffer
			err = tmpl.ExecuteTemplate(&buf, "layout", map[string]interface{}{})
			if err != nil {
				failed = true
				fmt.Printf("❌ %s → exec error: %v\n", rel, err)
			} else {
				fmt.Printf("✅ %s\n", rel)
			}

			return nil
		})

		if failed {
			return cli.Exit("some templates failed to compile", 1)
		}

		fmt.Println("✅ All templates validated successfully.")
		return nil
	},
}
