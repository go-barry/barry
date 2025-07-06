package cli

import (
	"bytes"
	"os"
	"path/filepath"
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

func TestCheckCommand_ParseError(t *testing.T) {
	tempDir := "testdata_parse_error"
	_ = os.RemoveAll(tempDir)
	if err := os.Mkdir(tempDir, 0755); err != nil {
		t.Fatalf("Failed to create test directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	routesDir := filepath.Join(tempDir, "routes", "bad")
	if err := os.MkdirAll(routesDir, 0755); err != nil {
		t.Fatalf("Failed to create routes dir: %v", err)
	}
	brokenHTML := `{{ define "layout" }} {{ if }} {{ end }}`
	if err := os.WriteFile(filepath.Join(routesDir, "index.html"), []byte(brokenHTML), 0644); err != nil {
		t.Fatalf("Failed to write index.html: %v", err)
	}

	layoutPath := filepath.Join(tempDir, "routes", "layout.html")
	validLayout := `{{ define "layout" }}Valid Layout{{ end }}`
	if err := os.WriteFile(layoutPath, []byte(validLayout), 0644); err != nil {
		t.Fatalf("Failed to write layout.html: %v", err)
	}

	originalStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe failed: %v", err)
	}
	os.Stdout = w

	originalCWD, _ := os.Getwd()
	defer os.Chdir(originalCWD)
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("chdir failed: %v", err)
	}

	app := &cli.App{
		Commands: []*cli.Command{CheckCommand},
		ExitErrHandler: func(c *cli.Context, err error) {
		},
	}

	appErr := app.Run([]string{"cli", "check"})

	w.Close()
	os.Stdout = originalStdout

	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	output := buf.String()

	if !strings.Contains(output, "❌ /bad → parse error:") {
		t.Errorf("expected parse error, got:\n%s", output)
	}

	exitErr, ok := appErr.(cli.ExitCoder)
	if !ok || exitErr.ExitCode() != 1 {
		t.Fatalf("expected cli.Exit code 1, got: %v", appErr)
	}
}

func TestCheckCommand_ExecError(t *testing.T) {
	tempDir := "testdata_exec_error"
	_ = os.RemoveAll(tempDir)
	if err := os.Mkdir(tempDir, 0755); err != nil {
		t.Fatalf("Failed to create test directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	routesDir := filepath.Join(tempDir, "routes", "bad")
	if err := os.MkdirAll(routesDir, 0755); err != nil {
		t.Fatalf("Failed to create routes dir: %v", err)
	}

	indexHTML := `<!-- layout: routes/bad/layout.html -->`
	if err := os.WriteFile(filepath.Join(routesDir, "index.html"), []byte(indexHTML), 0644); err != nil {
		t.Fatalf("Failed to write index.html: %v", err)
	}

	brokenLayout := `{{ define "not-layout" }}This compiles, but won't be executed{{ end }}`
	if err := os.WriteFile(filepath.Join(routesDir, "layout.html"), []byte(brokenLayout), 0644); err != nil {
		t.Fatalf("Failed to write layout.html: %v", err)
	}

	originalStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe failed: %v", err)
	}
	os.Stdout = w

	originalCWD, _ := os.Getwd()
	defer os.Chdir(originalCWD)
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("chdir failed: %v", err)
	}

	app := &cli.App{
		Commands: []*cli.Command{CheckCommand},
		ExitErrHandler: func(c *cli.Context, err error) {
		},
	}

	appErr := app.Run([]string{"cli", "check"})

	w.Close()
	os.Stdout = originalStdout

	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	output := buf.String()

	if !strings.Contains(output, "❌ /bad → exec error:") {
		t.Errorf("expected exec error message for /bad, got:\n%s", output)
	}

	exitErr, ok := appErr.(cli.ExitCoder)
	if !ok || exitErr.ExitCode() != 1 {
		t.Fatalf("expected cli.Exit code 1, got: %v", appErr)
	}
}
