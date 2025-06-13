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

func MinifyAsset(env, path string) string {
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
	min := filepath.Join("public", fmt.Sprintf("%s.min%s", name, ext))
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

	_ = os.WriteFile(min, minified, 0644)

	if f, err := os.Create(minGz); err == nil {
		gz := gzip.NewWriter(f)
		gz.Write(minified)
		gz.Close()
		f.Close()
	}

	h := md5.New()
	h.Write(minified)
	hash := hex.EncodeToString(h.Sum(nil))[:6]

	return fmt.Sprintf("/static/%s.min%s?v=%s", name, ext, hash)
}

func BarryTemplateFuncs(env string) template.FuncMap {
	return template.FuncMap{
		"minify": func(path string) string {
			return MinifyAsset(env, path)
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
	}
}
