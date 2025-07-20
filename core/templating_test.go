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

func TestMinifyAsset_UnsupportedExtensionReturnsOriginal(t *testing.T) {
	result := MinifyAsset("prod", "/static/image.png", t.TempDir())
	if result != "/static/image.png" {
		t.Errorf("expected original path for unsupported extension, got %s", result)
	}
}

func TestMinifyAsset_AlreadyMinifiedReturnsOriginal(t *testing.T) {
	result := MinifyAsset("prod", "/static/app.min.js", t.TempDir())
	if result != "/static/app.min.js" {
		t.Errorf("expected original path for .min.js, got %s", result)
	}
}

func TestMinifyAsset_MissingSourceFileReturnsOriginal(t *testing.T) {
	result := MinifyAsset("prod", "/static/missing.css", t.TempDir())
	if result != "/static/missing.css" {
		t.Errorf("expected fallback on missing source file, got %s", result)
	}
}

func TestMinifyAsset_MinifyErrorReturnsOriginal(t *testing.T) {
	tmpCache := t.TempDir()
	publicDir := filepath.Join(".", "public")
	err := os.MkdirAll(publicDir, 0755)
	if err != nil {
		t.Fatalf("failed to create public dir: %v", err)
	}
	sourcePath := filepath.Join(publicDir, "broken.js")
	t.Cleanup(func() { _ = os.RemoveAll(publicDir) })

	err = os.WriteFile(sourcePath, []byte("function(){"), 0644)
	if err != nil {
		t.Fatalf("failed to write JS: %v", err)
	}

	result := MinifyAsset("prod", "/static/broken.js", tmpCache)

	if result != "/static/broken.js" {
		t.Errorf("expected fallback for minify error, got %s", result)
	}
}

func TestBarryTemplateFuncs_propsPanicsOnNonStringKey(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic on non-string key")
		}
	}()
	propsFunc := BarryTemplateFuncs("prod", ".")["props"].(func(...interface{}) map[string]interface{})
	propsFunc(123, "value")
}

func TestBarryTemplateFuncs_versionedPublicFallback(t *testing.T) {
	tmp := t.TempDir()

	publicDir := filepath.Join(".", "public")
	_ = os.MkdirAll(publicDir, 0755)
	t.Cleanup(func() { _ = os.RemoveAll("public") })

	filePath := filepath.Join(publicDir, "a.js")
	err := os.WriteFile(filePath, []byte("abc"), 0644)
	if err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	versioned := BarryTemplateFuncs("prod", tmp)["versioned"].(func(string) string)
	result := versioned("/static/a.js")

	if !strings.HasPrefix(result, "/static/a.js?v=") {
		t.Errorf("expected versioned path from public dir, got %s", result)
	}
}

func TestMinifyAsset_MkdirAllFails_ReturnsOriginal(t *testing.T) {
	tmp := t.TempDir()
	invalidDir := filepath.Join(tmp, "invalid-cache")

	_ = os.WriteFile(invalidDir, []byte("not a dir"), 0644)

	publicPath := filepath.Join("public", "test.css")
	_ = os.MkdirAll(filepath.Dir(publicPath), 0755)
	_ = os.WriteFile(publicPath, []byte("body { color: red; }"), 0644)
	t.Cleanup(func() { _ = os.RemoveAll("public") })

	result := MinifyAsset("prod", "/static/test.css", invalidDir)

	if result != "/static/test.css" {
		t.Errorf("expected fallback to original path, got %s", result)
	}
}

func TestMinifyAsset_WriteFileFails_ReturnsOriginal(t *testing.T) {
	tmp := t.TempDir()

	readonlyDir := filepath.Join(tmp, "static")
	_ = os.MkdirAll(readonlyDir, 0555)

	publicPath := filepath.Join("public", "blocked.css")
	_ = os.MkdirAll(filepath.Dir(publicPath), 0755)
	_ = os.WriteFile(publicPath, []byte("body { color: red; }"), 0644)
	t.Cleanup(func() { _ = os.RemoveAll("public") })

	result := MinifyAsset("prod", "/static/blocked.css", tmp)

	if result != "/static/blocked.css" {
		t.Errorf("expected fallback to original path, got %s", result)
	}
}

func TestBarryTemplateFuncs_minify(t *testing.T) {
	tmp := t.TempDir()

	publicPath := filepath.Join("public", "style.css")
	_ = os.MkdirAll(filepath.Dir(publicPath), 0755)
	_ = os.WriteFile(publicPath, []byte("body { color: blue; }"), 0644)
	t.Cleanup(func() { _ = os.RemoveAll("public") })

	minifyFunc := BarryTemplateFuncs("prod", tmp)["minify"].(func(string) string)
	result := minifyFunc("/static/style.css")

	if !strings.HasPrefix(result, "/static/style.min.css?v=") {
		t.Errorf("unexpected minify result: %s", result)
	}
}

func TestBarryTemplateFuncs_versionedSkipsNonStatic(t *testing.T) {
	versioned := BarryTemplateFuncs("prod", ".")["versioned"].(func(string) string)

	input := "/not-static/app.js"
	result := versioned(input)

	if result != input {
		t.Errorf("expected original path, got %s", result)
	}
}
