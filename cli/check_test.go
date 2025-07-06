package cli

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/urfave/cli/v2"
)

func TestCheckCommand_StarterDirectory(t *testing.T) {
	err := os.Chdir("_starter")
	if err != nil {
		t.Fatalf("Failed to chdir to _starter: %v", err)
	}
	defer os.Chdir("..")

	originalStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("Failed to create pipe: %v", err)
	}
	os.Stdout = w

	app := &cli.App{
		Commands: []*cli.Command{CheckCommand},
	}
	_ = app.Run([]string{"cli", "check"})

	w.Close()
	os.Stdout = originalStdout

	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	output := buf.String()

	if !strings.Contains(output, "✅ /") {
		t.Errorf("expected success marker with route name, got:\n%s", output)
	}
	if !strings.Contains(output, "All templates validated successfully.") {
		t.Errorf("expected final success message, got:\n%s", output)
	}
}
