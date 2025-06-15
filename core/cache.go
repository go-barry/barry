package core

import (
	"compress/gzip"
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

func SaveCachedHTML(config Config, routeKey string, html []byte) error {
	outDir := filepath.Join(config.OutputDir, routeKey)
	if err := os.MkdirAll(outDir, os.ModePerm); err != nil {
		return err
	}

	htmlPath := filepath.Join(outDir, "index.html")
	gzPath := htmlPath + ".gz"

	f, err := os.Create(gzPath)
	if err != nil {
		return err
	}
	defer f.Close()

	gz := gzip.NewWriter(f)
	defer gz.Close()

	_, err = gz.Write(html)
	return err
}
