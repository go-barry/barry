package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/urfave/cli/v2"
)

func TestInfoCommand_WithStarterStructure(t *testing.T) {
	tmpDir := t.TempDir()

	outputDir := filepath.Join(tmpDir, "out")
	configContent := "outputDir: out\ncache: true\ndebugHeaders: true\ndebugLogs: true\n"
	err := os.WriteFile(filepath.Join(tmpDir, "barry.config.yml"), []byte(configContent), 0644)
	if err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	_ = os.MkdirAll(filepath.Join(tmpDir, "components"), 0755)
	_ = os.WriteFile(filepath.Join(tmpDir, "components/header.html"), []byte(`<header>Hi</header>`), 0644)

	routes := []string{"test", "docs", "docs/_slug"}
	for _, route := range routes {
		routeDir := filepath.Join(tmpDir, "routes", route)
		_ = os.MkdirAll(routeDir, 0755)
		_ = os.WriteFile(filepath.Join(routeDir, "index.html"), []byte(`<html>{{ define "content" }}Page{{ end }}</html>`), 0644)
	}

	_ = os.MkdirAll(filepath.Join(outputDir, "test"), 0755)
	_ = os.WriteFile(filepath.Join(outputDir, "test", "index.html"), []byte("<html>cached</html>"), 0644)

	origDir, _ := os.Getwd()
	_ = os.Chdir(tmpDir)
	t.Cleanup(func() {
		_ = os.Chdir(origDir)
		_ = os.RemoveAll(tmpDir)
	})

	app := &cli.App{Commands: []*cli.Command{InfoCommand}}

	var runErr error
	var output string
	output = captureOutput(func() {
		runErr = app.Run([]string{"barry", "info"})
	})

	if runErr != nil {
		t.Fatalf("expected no error, got: %v", runErr)
	}

	assertContains := func(label, content string) {
		if !strings.Contains(output, content) {
			t.Errorf("expected %s to contain %q", label, content)
		}
	}

	assertContains("output", "📁 Output Directory: out")
	assertContains("output", "🔁 Cache Enabled: true")
	assertContains("output", "🔁 Debug Headers Enabled: true")
	assertContains("output", "🔁 Debug Logs Enabled: true")
	assertContains("output", "🗂️  Routes Found: 3")
	assertContains("output", "📦 Components Found: 1")
	assertContains("output", "💾 Cached Pages: 1")
}
