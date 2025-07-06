package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-barry/barry/core"
	"github.com/urfave/cli/v2"
)

func TestCleanCommand_CleansOutputDir(t *testing.T) {
	tmpDir := t.TempDir()

	dummyFile := filepath.Join(tmpDir, "index.html")
	if err := os.WriteFile(dummyFile, []byte("cached!"), 0644); err != nil {
		t.Fatal(err)
	}

	overrideLoadConfig(tmpDir, func() {
		app := &cli.App{
			Commands: []*cli.Command{CleanCommand},
		}
		err := app.Run([]string{"cmd", "clean"})
		if err != nil {
			t.Fatalf("clean command failed: %v", err)
		}

		if _, err := os.Stat(dummyFile); !os.IsNotExist(err) {
			t.Errorf("expected file to be deleted, but still exists: %s", dummyFile)
		}
	})
}

func TestCleanCommand_CleansSubroute(t *testing.T) {
	tmpDir := t.TempDir()
	subDir := filepath.Join(tmpDir, "foo/bar")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatal(err)
	}
	subFile := filepath.Join(subDir, "index.html")
	_ = os.WriteFile(subFile, []byte("route data"), 0644)

	overrideLoadConfig(tmpDir, func() {
		app := &cli.App{
			Commands: []*cli.Command{CleanCommand},
		}
		err := app.Run([]string{"cmd", "clean", "foo/bar"})
		if err != nil {
			t.Fatalf("clean command failed: %v", err)
		}

		if _, err := os.Stat(subDir); !os.IsNotExist(err) {
			t.Errorf("expected subroute directory to be deleted, but it exists")
		}
	})
}

func TestCleanCommand_NoOpOnNonexistentDir(t *testing.T) {
	tmpDir := t.TempDir()
	overrideLoadConfig(filepath.Join(tmpDir, "does-not-exist"), func() {
		app := &cli.App{
			Commands: []*cli.Command{CleanCommand},
		}
		err := app.Run([]string{"cmd", "clean"})
		if err != nil {
			t.Fatalf("expected no error for nonexistent dir, got: %v", err)
		}
	})
}

func TestCleanCommand_ErrIfNotDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	file := filepath.Join(tmpDir, "notadir")
	_ = os.WriteFile(file, []byte("I'm a file"), 0644)

	overrideLoadConfig(file, func() {
		app := &cli.App{
			Commands: []*cli.Command{CleanCommand},
		}
		err := app.Run([]string{"cmd", "clean"})
		if err == nil || err.Error() != fmt.Sprintf("not a directory: %s", file) {
			t.Errorf("expected 'not a directory' error, got: %v", err)
		}
	})
}

func TestCleanCommand_ErrIfStatFails(t *testing.T) {
	app := &cli.App{
		Commands: []*cli.Command{CleanCommand},
	}

	overrideLoadConfig("/hopefully/invalid/\x00", func() {
		err := app.Run([]string{"cmd", "clean"})
		if err == nil {
			t.Fatal("expected error due to stat failure, got nil")
		}
	})
}

func TestCleanCommand_ErrIfRemoveFails(t *testing.T) {
	tmpDir := t.TempDir()
	protectedDir := filepath.Join(tmpDir, "locked")

	if err := os.Mkdir(protectedDir, 0755); err != nil {
		t.Fatal(err)
	}

	file := filepath.Join(protectedDir, "file.html")
	if err := os.WriteFile(file, []byte("cached"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := os.Chmod(protectedDir, 0400); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(protectedDir, 0755)

	overrideLoadConfig(protectedDir, func() {
		app := &cli.App{
			Commands: []*cli.Command{CleanCommand},
		}
		err := app.Run([]string{"cmd", "clean"})
		if err == nil || !strings.Contains(err.Error(), "failed to clean cache") {
			t.Errorf("expected clean error, got: %v", err)
		}
	})
}

func overrideLoadConfig(outputDir string, testFn func()) {
	orig := core.LoadConfig
	core.LoadConfig = func(_ string) *core.Config {
		return &core.Config{OutputDir: outputDir}
	}
	defer func() { core.LoadConfig = orig }()
	testFn()
}
