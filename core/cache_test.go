package core

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSaveCachedHTMLAndGetCachedHTML(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := Config{OutputDir: tmpDir}
	route := "test-route"
	ext := "html"
	html := []byte("<html><body>Hello Barry!</body></html>")

	err := SaveCachedHTML(cfg, route, ext, html)
	if err != nil {
		t.Fatalf("SaveCachedHTML failed: %v", err)
	}

	htmlPath := filepath.Join(tmpDir, route, "index."+ext)
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

	cached, ok := GetCachedHTML(cfg, route, ext)
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
	ext := "html"

	data, ok := GetCachedHTML(cfg, route, ext)
	if ok {
		t.Errorf("Expected ok=false for missing file")
	}
	if data != nil {
		t.Errorf("Expected nil data for missing file")
	}
}

func TestSaveCachedHTML_CreateDirFails(t *testing.T) {
	tmpDir := t.TempDir()
	badPath := filepath.Join(tmpDir, "conflict")
	if err := os.WriteFile(badPath, []byte("file blocks dir"), 0644); err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	cfg := Config{OutputDir: badPath}
	err := SaveCachedHTML(cfg, "route", "html", []byte("<html></html>"))
	if err == nil || !strings.Contains(err.Error(), "failed to create cache directory") {
		t.Errorf("expected directory creation error, got: %v", err)
	}
}

func TestSaveCachedHTML_WriteHTMLFails(t *testing.T) {
	tmpDir := t.TempDir()
	routePath := filepath.Join(tmpDir, "readonly")
	if err := os.MkdirAll(routePath, 0400); err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	defer os.Chmod(routePath, 0755)

	cfg := Config{OutputDir: tmpDir}
	err := SaveCachedHTML(cfg, "readonly", "html", []byte("<html></html>"))
	if err == nil || !strings.Contains(err.Error(), "failed to write index.html") {
		t.Errorf("expected HTML write error, got: %v", err)
	}
}

func TestSaveCachedHTML_CreateGzipFails(t *testing.T) {
	tmpDir := t.TempDir()
	route := "gzipdir"
	routeDir := filepath.Join(tmpDir, route)

	if err := os.MkdirAll(routeDir, 0755); err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	htmlPath := filepath.Join(routeDir, "index.html")
	if err := os.WriteFile(htmlPath, []byte("<html>ok</html>"), 0644); err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	gzPath := filepath.Join(routeDir, "index.html.gz")
	if err := os.MkdirAll(gzPath, 0755); err != nil {
		t.Fatalf("failed to create dir at .gz path: %v", err)
	}

	cfg := Config{OutputDir: tmpDir}
	err := SaveCachedHTML(cfg, route, "html", []byte("<html>will fail gz create</html>"))

	if err == nil || !strings.Contains(err.Error(), "failed to create gzip file") {
		t.Errorf("Expected gzip create failure, got: %v", err)
	}
}

type failingWriter struct{}

func (f *failingWriter) Write(p []byte) (int, error) {
	return 0, fmt.Errorf("simulated write failure")
}
func (f *failingWriter) Close() error {
	return nil
}

func TestSaveCachedHTML_GzipWriteFails(t *testing.T) {
	originalFactory := gzipWriterFactory
	defer func() { gzipWriterFactory = originalFactory }()

	gzipWriterFactory = func(w io.Writer) io.WriteCloser {
		return &failingWriter{}
	}

	tmpDir := t.TempDir()
	cfg := Config{OutputDir: tmpDir}
	err := SaveCachedHTML(cfg, "write-error", "html", []byte("<html>failure</html>"))

	if err == nil || !strings.Contains(err.Error(), "failed to write gzipped index.html") {
		t.Errorf("Expected gzip write failure, got: %v", err)
	}
}
