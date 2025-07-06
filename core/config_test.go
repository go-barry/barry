package core

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfigFromValidFile(t *testing.T) {
	tmp := t.TempDir()

	configYAML := `
outputDir: ./out
cache: true
debugHeaders: true
debugLogs: true
`
	configPath := filepath.Join(tmp, "barry.config.yml")
	err := os.WriteFile(configPath, []byte(configYAML), 0644)
	if err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	cfg := LoadConfig(configPath)

	if cfg.OutputDir != "./out" {
		t.Errorf("expected OutputDir './out', got %q", cfg.OutputDir)
	}
	if !cfg.CacheEnabled {
		t.Error("expected CacheEnabled to be true")
	}
	if !cfg.DebugHeaders {
		t.Error("expected DebugHeaders to be true")
	}
	if !cfg.DebugLogs {
		t.Error("expected DebugLogs to be true")
	}
}

func TestLoadConfigDefaultsWhenFileMissing(t *testing.T) {
	cfg := LoadConfig("nonexistent.yml")

	if cfg.OutputDir != "./cache" {
		t.Errorf("expected default OutputDir './cache', got %q", cfg.OutputDir)
	}
	if cfg.CacheEnabled {
		t.Error("expected CacheEnabled to be false")
	}
	if cfg.DebugHeaders {
		t.Error("expected DebugHeaders to be false")
	}
	if cfg.DebugLogs {
		t.Error("expected DebugLogs to be false")
	}
}

func TestLoadConfigDefaultsWhenOutputDirEmpty(t *testing.T) {
	tmp := t.TempDir()

	configYAML := `
cache: true
debugHeaders: true
debugLogs: true
`
	configPath := filepath.Join(tmp, "barry.config.yml")
	err := os.WriteFile(configPath, []byte(configYAML), 0644)
	if err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	cfg := LoadConfig(configPath)

	if cfg.OutputDir != "./cache" {
		t.Errorf("expected fallback OutputDir './cache', got %q", cfg.OutputDir)
	}
	if !cfg.CacheEnabled || !cfg.DebugHeaders || !cfg.DebugLogs {
		t.Error("expected true values for all booleans")
	}
}
