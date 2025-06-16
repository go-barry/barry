package core

import (
	"compress/gzip"
	"fmt"
	"os"
	"path/filepath"
)

func GetCachedHTML(config Config, route string) ([]byte, bool) {
	cachePath := filepath.Join(config.OutputDir, route, "index.html")

	content, err := os.ReadFile(cachePath)
	if err != nil {
		return nil, false
	}
	return content, true
}

func SaveCachedHTML(config Config, routeKey string, html []byte) error {
	outDir := filepath.Join(config.OutputDir, routeKey)
	if err := os.MkdirAll(outDir, os.ModePerm); err != nil {
		return fmt.Errorf("failed to create cache directory: %w", err)
	}

	htmlPath := filepath.Join(outDir, "index.html")
	gzPath := htmlPath + ".gz"

	f, err := os.Create(gzPath)
	if err != nil {
		return fmt.Errorf("failed to create gzip file: %w", err)
	}
	defer f.Close()

	gz := gzip.NewWriter(f)
	defer gz.Close()

	if _, err := gz.Write(html); err != nil {
		return fmt.Errorf("failed to write gzipped html: %w", err)
	}

	return nil
}
