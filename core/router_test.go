package core

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func setupRouterTestEnv(t *testing.T) (Config, func()) {
	routesDir := filepath.Join("routes", "test")
	if err := os.MkdirAll(routesDir, 0755); err != nil {
		t.Fatal(err)
	}
	html := `<!-- layout: layout.html -->
{{ define "content" }}<h1>Hello</h1>{{ end }}`
	if err := os.WriteFile(filepath.Join(routesDir, "index.html"), []byte(html), 0644); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile("layout.html", []byte(`{{ define "layout" }}<html><body>{{ template "content" . }}</body></html>{{ end }}`), 0644); err != nil {
		t.Fatal(err)
	}

	errorDir := filepath.Join("routes", "_error")
	if err := os.MkdirAll(errorDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(errorDir, "index.html"), []byte(`<!-- layout: layout.html -->
{{ define "content" }}<h1>Error</h1>{{ end }}`), 0644); err != nil {
		t.Fatal(err)
	}

	if err := os.MkdirAll("components", 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join("components", "header.html"), []byte(`<header>Hi</header>`), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := Config{
		OutputDir:    t.TempDir(),
		CacheEnabled: false,
		DebugLogs:    false,
		DebugHeaders: false,
	}

	return cfg, func() {
		os.RemoveAll("routes")
		os.RemoveAll("components")
		os.Remove("layout.html")
	}
}

func TestRouter_ServesMatchingRoute(t *testing.T) {
	cfg, cleanup := setupRouterTestEnv(t)
	defer cleanup()

	router := NewRouter(cfg, RuntimeContext{Env: "dev"}).(*Router)

	router.routes = []Route{
		{
			URLPattern: regexp.MustCompile("^test$"),
			HTMLPath:   "routes/test/index.html",
			ServerPath: "routes/test/index.server.go",
			FilePath:   "routes/test",
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)
	res := rec.Result()

	if res.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", res.StatusCode)
	}

	body, _ := io.ReadAll(res.Body)
	if !strings.Contains(string(body), "<h1>Hello</h1>") {
		t.Errorf("expected rendered content, got: %s", string(body))
	}
}

func TestRouter_Returns404ForUnknownRoute(t *testing.T) {
	cfg := Config{
		OutputDir:    t.TempDir(),
		CacheEnabled: false,
		DebugLogs:    false,
		DebugHeaders: false,
	}

	_ = os.WriteFile("layout.html", []byte(`{{ define "layout" }}<html><body>{{ template "content" . }}</body></html>{{ end }}`), 0644)

	errorDir := filepath.Join("routes", "_error")
	_ = os.MkdirAll(errorDir, 0755)
	_ = os.WriteFile(filepath.Join(errorDir, "404.html"), []byte(`<!-- layout: layout.html -->
{{ define "content" }}<h1>Error</h1>{{ end }}`), 0644)

	_ = os.MkdirAll("components", 0755)

	t.Cleanup(func() {
		_ = os.RemoveAll("routes")
		_ = os.RemoveAll("components")
		_ = os.Remove("layout.html")
	})

	router := NewRouter(cfg, RuntimeContext{Env: "dev"})

	rec := httptest.NewRecorder()
	wrapped := &statusRecorder{ResponseWriter: rec}

	req := httptest.NewRequest(http.MethodGet, "/not-found", nil)
	router.ServeHTTP(wrapped, req)

	if wrapped.Status() != http.StatusNotFound {
		t.Errorf("expected 404, got %d", wrapped.Status())
	}

	body := rec.Body.String()
	if !strings.Contains(body, "<h1>Error</h1>") {
		t.Errorf("expected error page content, got: %s", body)
	}
}

func TestRouter_ParseLayoutDirective(t *testing.T) {
	cfg, cleanup := setupRouterTestEnv(t)
	defer cleanup()

	router := NewRouter(cfg, RuntimeContext{Env: "dev"}).(*Router)
	layout := router.getLayoutPath("routes/test/index.html")

	if layout != "layout.html" {
		t.Errorf("expected layout.html, got %q", layout)
	}
}

func TestGenerateETag_ConsistentHash(t *testing.T) {
	data := []byte("<html>Hi</html>")
	tag1 := generateETag(data)
	tag2 := generateETag(data)

	if tag1 != tag2 {
		t.Errorf("ETag hash inconsistent: %s vs %s", tag1, tag2)
	}
}
