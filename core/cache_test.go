package core

import (
	"bytes"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestSaveCachedHTMLAndGetCachedHTML(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := Config{OutputDir: tmpDir}
	route := "test-route"
	html := []byte("<html><body>Hello Barry!</body></html>")

	err := SaveCachedHTML(cfg, route, html)
	if err != nil {
		t.Fatalf("SaveCachedHTML failed: %v", err)
	}

	htmlPath := filepath.Join(tmpDir, route, "index.html")
	data, err := os.ReadFile(htmlPath)
	if err != nil {
		t.Fatalf("Failed to read index.html: %v", err)
	}
	if !bytes.Equal(data, html) {
		t.Errorf("Cached HTML does not match original")
	}

	gzPath := htmlPath + ".gz"
	gzFile, err := os.Open(gzPath)
	if err != nil {
		t.Fatalf("Failed to read gzip file: %v", err)
	}
	defer gzFile.Close()

	gzReader, err := gzip.NewReader(gzFile)
	if err != nil {
		t.Fatalf("Failed to create gzip reader: %v", err)
	}
	defer gzReader.Close()

	unzipped, err := io.ReadAll(gzReader)
	if err != nil {
		t.Fatalf("Failed to read from gzip reader: %v", err)
	}

	if !bytes.Equal(unzipped, html) {
		t.Errorf("Gzipped content does not match original HTML")
	}

	cached, ok := GetCachedHTML(cfg, route)
	if !ok {
		t.Errorf("Expected to find cached HTML, got false")
	}
	if !bytes.Equal(cached, html) {
		t.Errorf("GetCachedHTML returned incorrect content")
	}
}

func TestGetCachedHTML_MissingFile(t *testing.T) {
	cfg := Config{OutputDir: t.TempDir()}
	route := "non-existent"

	data, ok := GetCachedHTML(cfg, route)
	if ok {
		t.Errorf("Expected ok=false for missing file")
	}
	if data != nil {
		t.Errorf("Expected nil data for missing file")
	}
}
