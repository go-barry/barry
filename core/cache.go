package core

import (
	"os"
	"path/filepath"
)

func GetCachedHTML(config Config, route string) ([]byte, bool) {
	cachePath := filepath.Join(config.OutputDir, route, "index.html")

	if _, err := os.Stat(cachePath); err != nil {
		return nil, false
	}

	content, err := os.ReadFile(cachePath)
	if err != nil {
		return nil, false
	}

	return content, true
}

func SaveCachedHTML(config Config, route string, html []byte) error {
	cacheDir := filepath.Join(config.OutputDir, route)
	cachePath := filepath.Join(cacheDir, "index.html")

	err := os.MkdirAll(cacheDir, os.ModePerm)
	if err != nil {
		return err
	}

	return os.WriteFile(cachePath, html, 0644)
}
