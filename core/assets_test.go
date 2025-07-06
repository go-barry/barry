package core

import (
	"html/template"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTempFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestMinifyAsset_NonProdReturnsSamePath(t *testing.T) {
	path := "/static/style.css"
	result := MinifyAsset("dev", path, t.TempDir())
	if result != path {
		t.Errorf("expected same path in dev mode, got %s", result)
	}
}

func TestMinifyAsset_ProdMinifiesAndCaches(t *testing.T) {
	tmpCache := t.TempDir()

	publicDir := filepath.Join(".", "public")
	err := os.MkdirAll(publicDir, 0755)
	if err != nil {
		t.Fatalf("failed to create public dir: %v", err)
	}

	sourcePath := filepath.Join(publicDir, "example.css")
	err = os.WriteFile(sourcePath, []byte("body { color: red; }"), 0644)
	if err != nil {
		t.Fatalf("failed to write source CSS file: %v", err)
	}

	t.Cleanup(func() {
		_ = os.RemoveAll(publicDir)
	})

	result := MinifyAsset("prod", "/static/example.css", tmpCache)

	if !strings.HasPrefix(result, "/static/example.min.css?v=") {
		t.Errorf("unexpected minified path: %s", result)
	}

	minifiedFile := filepath.Join(tmpCache, "static", "example.min.css")
	gzippedFile := minifiedFile + ".gz"

	if _, err := os.Stat(minifiedFile); err != nil {
		t.Errorf("expected minified file to exist: %s", minifiedFile)
	}

	if _, err := os.Stat(gzippedFile); err != nil {
		t.Errorf("expected gzipped file to exist: %s", gzippedFile)
	}
}

func TestBarryTemplateFuncs_props(t *testing.T) {
	propsFunc := BarryTemplateFuncs("dev", ".")["props"].(func(...interface{}) map[string]interface{})

	result := propsFunc("name", "Callum", "role", "Engineer")

	if result["name"] != "Callum" || result["role"] != "Engineer" {
		t.Errorf("unexpected props map: %+v", result)
	}
}

func TestBarryTemplateFuncs_propsPanicsOnOddArgs(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic on odd number of args")
		}
	}()
	propsFunc := BarryTemplateFuncs("dev", ".")["props"].(func(...interface{}) map[string]interface{})
	propsFunc("name", "Callum", "missingValue")
}

func TestBarryTemplateFuncs_safeHTML(t *testing.T) {
	safe := BarryTemplateFuncs("dev", ".")["safeHTML"].(func(interface{}) template.HTML)

	if safe("<b>test</b>") != template.HTML("<b>test</b>") {
		t.Error("string input failed")
	}

	if safe(template.HTML("<i>safe</i>")) != template.HTML("<i>safe</i>") {
		t.Error("template.HTML input failed")
	}

	if safe(123) != template.HTML("") {
		t.Error("unexpected non-string should return empty")
	}
}

func TestBarryTemplateFuncs_versioned(t *testing.T) {
	tmp := t.TempDir()
	path := "static/script.js"
	fullPath := filepath.Join(tmp, path)

	err := os.MkdirAll(filepath.Dir(fullPath), 0755)
	if err != nil {
		t.Fatal(err)
	}

	err = os.WriteFile(fullPath, []byte("console.log('hello')"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	versioned := BarryTemplateFuncs("prod", tmp)["versioned"].(func(string) string)
	result := versioned("/" + path)

	if !strings.HasPrefix(result, "/static/script.js?v=") {
		t.Errorf("unexpected versioned path: %s", result)
	}
}

func TestBarryTemplateFuncs_versionedFallback(t *testing.T) {
	versioned := BarryTemplateFuncs("prod", ".")["versioned"].(func(string) string)

	input := "/static/missing.js"
	result := versioned(input)

	if result != input {
		t.Errorf("expected fallback to original path, got %s", result)
	}
}
