package cli

import (
	"embed"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/urfave/cli/v2"
)

//go:embed _starter/**
var testFS embed.FS

func TestCopyEmbeddedDir(t *testing.T) {
	tmpDir := t.TempDir()

	err := copyEmbeddedDir(testFS, "_starter", tmpDir)
	if err != nil {
		t.Fatalf("unexpected error copying embedded dir: %v", err)
	}

	err = fs.WalkDir(testFS, "_starter", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel("_starter", path)
		if err != nil {
			return err
		}
		dest := filepath.Join(tmpDir, rel)
		if _, err := os.Stat(dest); err != nil {
			t.Errorf("expected file %s to exist, but got error: %v", rel, err)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected walk error: %v", err)
	}
}

func TestInitCommand_RunSuccess(t *testing.T) {
	tmpDir := t.TempDir()

	oldWd, _ := os.Getwd()
	defer os.Chdir(oldWd)
	_ = os.Chdir(tmpDir)

	starterFS = testFS

	execCommand = fakeExecCommandSuccess
	defer func() { execCommand = exec.Command }()

	app := &cli.App{
		Commands: []*cli.Command{InitCommand},
	}

	err := app.Run([]string{"cmd", "init"})
	if err != nil {
		t.Fatalf("init command failed: %v", err)
	}

	expectedFiles := []string{
		"main.go",
	}

	for _, f := range expectedFiles {
		if _, err := os.Stat(filepath.Join(tmpDir, f)); err != nil {
			t.Errorf("expected file %s to exist, but got error: %v", f, err)
		}
	}
}

func TestInitCommand_SkipsGoModInitIfAlreadyExists(t *testing.T) {
	tmpDir := t.TempDir()
	_ = os.Chdir(tmpDir)

	_ = os.WriteFile(filepath.Join(tmpDir, "main.go"), []byte("package main"), 0644)
	_ = os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte("module testmod"), 0644)

	starterFS = testFS
	execCommand = fakeExecCommandShouldNotRun(t)
	defer func() { execCommand = exec.Command }()

	app := &cli.App{
		Commands: []*cli.Command{InitCommand},
	}

	err := app.Run([]string{"cmd", "init"})
	if err != nil {
		t.Fatalf("init command failed: %v", err)
	}
}

var execCommand = exec.Command

func fakeExecCommandSuccess(command string, args ...string) *exec.Cmd {
	cs := []string{"-test.run=TestHelperProcess", "--", command}
	cs = append(cs, args...)
	cmd := exec.Command(os.Args[0], cs...)
	cmd.Env = append(os.Environ(), "GO_TEST_PROCESS=1")
	return cmd
}

func fakeExecCommandShouldNotRun(t *testing.T) func(string, ...string) *exec.Cmd {
	return func(command string, args ...string) *exec.Cmd {
		t.Fatalf("unexpected call to exec.Command: %s %v", command, args)
		return nil
	}
}

func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_TEST_PROCESS") != "1" {
		return
	}
	os.Exit(0)
}
