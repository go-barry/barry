package core

import (
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

var gzipWriterFactory = func(w io.Writer) io.WriteCloser {
	return gzip.NewWriter(w)
}

func GetCachedHTML(config Config, route, ext string) ([]byte, bool) {
	if ext == "" {
		ext = "html"
	}
	cachePath := filepath.Join(config.OutputDir, route, "index."+ext)
	content, err := os.ReadFile(cachePath)
	if err != nil {
		return nil, false
	}
	return content, true
}

func SaveCachedHTML(config Config, routeKey, ext string, data []byte) error {
	if ext == "" {
		ext = "html"
	}
	outDir := filepath.Join(config.OutputDir, routeKey)
	if err := os.MkdirAll(outDir, os.ModePerm); err != nil {
		return fmt.Errorf("failed to create cache directory: %w", err)
	}

	filename := "index." + ext
	filePath := filepath.Join(outDir, filename)
	gzPath := filePath + ".gz"

	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return fmt.Errorf("failed to write %s: %w", filename, err)
	}

	f, err := os.Create(gzPath)
	if err != nil {
		return fmt.Errorf("failed to create gzip file: %w", err)
	}
	defer f.Close()

	gz := gzipWriterFactory(f)
	defer gz.Close()

	if _, err := gz.Write(data); err != nil {
		return fmt.Errorf("failed to write gzipped %s: %w", filename, err)
	}

	return nil
}
