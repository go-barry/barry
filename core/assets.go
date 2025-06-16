package core

import (
	"bytes"
	"compress/gzip"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"strings"

	"github.com/tdewolff/minify/v2"
	mincss "github.com/tdewolff/minify/v2/css"
	minjs "github.com/tdewolff/minify/v2/js"
)

func MinifyAsset(env, path string, cacheDir string) string {
	if env != "prod" {
		return path
	}

	ext := filepath.Ext(path)
	base := filepath.Base(path)
	name := strings.TrimSuffix(base, ext)

	if ext != ".css" && ext != ".js" {
		return path
	}

	if strings.Contains(name, ".min") {
		return path
	}

	publicPath := strings.TrimPrefix(path, "/static/")
	src := filepath.Join("public", publicPath)
	min := filepath.Join(cacheDir, "static", fmt.Sprintf("%s.min%s", name, ext))
	minGz := min + ".gz"

	original, err := os.ReadFile(src)
	if err != nil {
		return path
	}

	m := minify.New()
	m.AddFunc("text/css", mincss.Minify)
	m.AddFunc("application/javascript", minjs.Minify)

	var buf bytes.Buffer
	var minifyErr error

	switch ext {
	case ".css":
		minifyErr = m.Minify("text/css", &buf, bytes.NewReader(original))
	case ".js":
		minifyErr = m.Minify("application/javascript", &buf, bytes.NewReader(original))
	}

	if minifyErr != nil {
		return path
	}

	minified := buf.Bytes()

	if err := os.MkdirAll(filepath.Dir(min), os.ModePerm); err != nil {
		return path
	}

	if f, err := os.Create(minGz); err == nil {
		defer f.Close()
		gz := gzip.NewWriter(f)
		if _, err := gz.Write(minified); err == nil {
			_ = gz.Close()
		}
	}

	h := md5.New()
	h.Write(minified)
	hash := hex.EncodeToString(h.Sum(nil))[:6]

	var out strings.Builder
	fmt.Fprintf(&out, "/static/%s.min%s?v=%s", name, ext, hash)
	return out.String()
}

func BarryTemplateFuncs(env, cacheDir string) template.FuncMap {
	return template.FuncMap{
		"minify": func(path string) string {
			return MinifyAsset(env, path, cacheDir)
		},
		"props": func(values ...interface{}) map[string]interface{} {
			if len(values)%2 != 0 {
				panic("props must be called with even number of arguments")
			}
			m := make(map[string]interface{}, len(values)/2)
			for i := 0; i < len(values); i += 2 {
				key, ok := values[i].(string)
				if !ok {
					panic("props keys must be strings")
				}
				m[key] = values[i+1]
			}
			return m
		},
		"safeHTML": func(s interface{}) template.HTML {
			switch val := s.(type) {
			case template.HTML:
				return val
			case string:
				return template.HTML(val)
			default:
				return ""
			}
		},
		"versioned": func(path string) string {
			if !strings.HasPrefix(path, "/static/") {
				return path
			}

			rel := strings.TrimPrefix(path, "/static/")
			locations := []string{
				filepath.Join("public", rel),
				filepath.Join(cacheDir, "static", rel),
			}

			for _, file := range locations {
				if content, err := os.ReadFile(file); err == nil {
					h := md5.New()
					h.Write(content)
					hash := hex.EncodeToString(h.Sum(nil))[:6]
					var out strings.Builder
					fmt.Fprintf(&out, "/static/%s?v=%s", rel, hash)
					return out.String()
				}
			}

			return path
		},
	}
}
