package core

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
)

func cleanupTestArtifacts() {
	_ = os.RemoveAll("routes")
	_ = os.RemoveAll("components")
	_ = os.Remove("layout.html")
}

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
		DebugLogs:    true,
		DebugHeaders: false,
	}

	return cfg, func() {
		os.RemoveAll("routes")
		os.RemoveAll("components")
		os.Remove("layout.html")
	}
}

type mockWatcher struct {
	events chan fsnotify.Event
	errors chan error
}

func (w *mockWatcher) Events() <-chan fsnotify.Event { return w.events }
func (w *mockWatcher) Errors() <-chan error          { return w.errors }
func (w *mockWatcher) Close() error {
	close(w.events)
	close(w.errors)
	return nil
}
func (w *mockWatcher) Add(_ string) error { return nil }

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
		DebugLogs:    true,
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

func TestRouter_EnqueuesCacheWrite(t *testing.T) {
	cfg, cleanup := setupRouterTestEnv(t)
	defer cleanup()

	cfg.CacheEnabled = true
	cfg.DebugLogs = true

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
		t.Errorf("expected 200 OK, got %d", res.StatusCode)
	}
}

func TestRouter_ServesFromGzipCache(t *testing.T) {
	cfg, cleanup := setupRouterTestEnv(t)
	defer cleanup()

	cfg.CacheEnabled = true
	cfg.DebugHeaders = true

	routeKey := "test"
	cacheDir := filepath.Join(cfg.OutputDir, routeKey)
	_ = os.MkdirAll(cacheDir, 0755)

	content := []byte("<html><body>Hello</body></html>")
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	_, _ = gw.Write(content)
	_ = gw.Close()

	_ = os.WriteFile(filepath.Join(cacheDir, "index.html.gz"), buf.Bytes(), 0644)

	router := NewRouter(cfg, RuntimeContext{Env: "prod"}).(*Router)
	router.routes = []Route{
		{
			URLPattern: regexp.MustCompile("^test$"),
			HTMLPath:   "routes/test/index.html",
			ServerPath: "routes/test/index.server.go",
			FilePath:   "routes/test",
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Accept-Encoding", "gzip")

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	res := rec.Result()
	if res.StatusCode != http.StatusOK {
		t.Errorf("expected 200 OK, got %d", res.StatusCode)
	}
	if res.Header.Get("Content-Encoding") != "gzip" {
		t.Errorf("expected gzip encoding, got: %s", res.Header.Get("Content-Encoding"))
	}
	if res.Header.Get("X-Barry-Cache") != "HIT" {
		t.Errorf("expected X-Barry-Cache: HIT, got %s", res.Header.Get("X-Barry-Cache"))
	}
}

func TestRenderErrorPage_Fallback(t *testing.T) {
	cfg := Config{
		OutputDir:    t.TempDir(),
		CacheEnabled: false,
		DebugLogs:    true,
		DebugHeaders: false,
	}

	router := NewRouter(cfg, RuntimeContext{Env: "dev"}).(*Router)

	rec := httptest.NewRecorder()
	router.renderErrorPage(rec, http.StatusInternalServerError, "Something broke", "/fail")

	if rec.Result().StatusCode != http.StatusInternalServerError {
		t.Errorf("expected 500 status code, got %d", rec.Result().StatusCode)
	}

	if !strings.Contains(rec.Body.String(), "500 - Something broke") {
		t.Errorf("expected fallback error message in body, got: %s", rec.Body.String())
	}
}

func TestRouter_ServerFileReturnsNotFoundError(t *testing.T) {
	cfg := Config{
		OutputDir:    t.TempDir(),
		CacheEnabled: false,
		DebugLogs:    true,
		DebugHeaders: false,
	}

	_ = os.MkdirAll("routes/fail", 0755)
	_ = os.WriteFile("routes/fail/index.html", []byte(`{{ define "content" }}Should not render{{ end }}`), 0644)
	_ = os.WriteFile("routes/fail/index.server.go", []byte(""), 0644)

	_ = os.MkdirAll("routes/_error", 0755)
	_ = os.WriteFile("routes/_error/index.html", []byte(`
<!-- layout: components/layouts/layout.html -->
{{ define "content" }}<h1>Error Layout: {{ .StatusCode }}</h1>{{ end }}
`), 0644)

	_ = os.MkdirAll("components/layouts", 0755)
	_ = os.WriteFile("components/layouts/layout.html", []byte(`
{{ define "layout" }}
<html><body>{{ template "content" . }}</body></html>
{{ end }}
`), 0644)

	t.Cleanup(func() {
		_ = os.RemoveAll("routes")
		_ = os.RemoveAll("components")
	})

	router := NewRouter(cfg, RuntimeContext{Env: "dev"}).(*Router)

	router.routes = []Route{{
		URLPattern: regexp.MustCompile("^fail$"),
		HTMLPath:   "routes/fail/index.html",
		ServerPath: "routes/fail/index.server.go",
		FilePath:   "routes/fail",
	}}

	original := ExecuteServerFile
	ExecuteServerFile = func(_ string, _ map[string]string, _ bool) (map[string]interface{}, error) {
		return nil, ErrNotFound
	}
	defer func() { ExecuteServerFile = original }()

	req := httptest.NewRequest(http.MethodGet, "/fail", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Result().StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Result().StatusCode)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "<h1>Error Layout: 404</h1>") {
		t.Errorf("expected error content, got: %s", body)
	}
}

func TestRouter_ServerFileReturnsGenericError(t *testing.T) {
	cfg := Config{
		OutputDir:    t.TempDir(),
		CacheEnabled: false,
		DebugLogs:    true,
		DebugHeaders: false,
	}

	_ = os.MkdirAll("routes/fail", 0755)
	_ = os.WriteFile("routes/fail/index.html", []byte(`{{ define "content" }}Should not render{{ end }}`), 0644)
	_ = os.WriteFile("routes/fail/index.server.go", []byte(""), 0644)

	_ = os.MkdirAll("routes/_error", 0755)
	_ = os.WriteFile("routes/_error/index.html", []byte(`
<!-- layout: components/layouts/layout.html -->
{{ define "content" }}<h1>Error Layout: {{ .StatusCode }}</h1>{{ end }}
`), 0644)

	_ = os.MkdirAll("components/layouts", 0755)
	_ = os.WriteFile("components/layouts/layout.html", []byte(`
{{ define "layout" }}
<html><body>{{ template "content" . }}</body></html>
{{ end }}
`), 0644)

	t.Cleanup(func() {
		_ = os.RemoveAll("routes")
		_ = os.RemoveAll("components")
	})

	router := NewRouter(cfg, RuntimeContext{Env: "dev"}).(*Router)

	router.routes = []Route{{
		URLPattern: regexp.MustCompile("^fail$"),
		HTMLPath:   "routes/fail/index.html",
		ServerPath: "routes/fail/index.server.go",
		FilePath:   "routes/fail",
	}}

	original := ExecuteServerFile
	ExecuteServerFile = func(_ string, _ map[string]string, _ bool) (map[string]interface{}, error) {
		return nil, ErrNotFound
	}
	defer func() { ExecuteServerFile = original }()

	req := httptest.NewRequest(http.MethodGet, "/fail", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Result().StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Result().StatusCode)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "<h1>Error Layout: 404</h1>") {
		t.Errorf("expected error content, got: %s", body)
	}
}

func TestGetOrCreateCompileLock_CreatesNew(t *testing.T) {
	key := "some-unique-path"

	compileLocks.Delete(key)
	lock := getOrCreateCompileLock(key)
	if lock == nil {
		t.Fatal("expected mutex, got nil")
	}

	again := getOrCreateCompileLock(key)
	if lock != again {
		t.Error("expected same mutex instance")
	}
}

func TestRouter_WatchEverything_Starts(t *testing.T) {
	cfg, cleanup := setupRouterTestEnv(t)
	defer cleanup()

	router := NewRouter(cfg, RuntimeContext{
		Env:         "dev",
		EnableWatch: true,
		OnReload:    func() {},
	}).(*Router)

	_ = router

	go func() {
		time.Sleep(100 * time.Millisecond)
	}()

	// Nothing to assert - success is no panic
}

func TestStatusRecorder_DefaultStatus(t *testing.T) {
	rec := &statusRecorder{ResponseWriter: httptest.NewRecorder()}
	if got := rec.Status(); got != 200 {
		t.Errorf("expected 200, got %d", got)
	}
}

func TestRouter_LoadRoutes_ParsesDynamicParams(t *testing.T) {
	_ = os.MkdirAll("routes/posts/_id", 0755)
	_ = os.WriteFile("routes/posts/_id/index.html", []byte(`Hello`), 0644)
	t.Cleanup(func() {
		_ = os.RemoveAll("routes")
	})

	r := &Router{}
	r.loadRoutes()

	found := false
	for _, route := range r.routes {
		if route.URLPattern.MatchString("posts/123") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected route with dynamic param to be parsed")
	}
}

func TestRouter_ServeStatic_MissingHTML(t *testing.T) {
	t.Cleanup(cleanupTestArtifacts)

	_ = os.MkdirAll("routes/_error", 0755)
	_ = os.WriteFile("routes/_error/404.html", []byte(`
<!-- layout: layout.html -->
{{ define "content" }}<h1>Error {{ .StatusCode }}</h1>{{ end }}
`), 0644)

	_ = os.WriteFile("layout.html", []byte(`
{{ define "layout" }}<html><body>{{ template "content" . }}</body></html>{{ end }}
`), 0644)

	_ = os.MkdirAll("components", 0755)

	cfg := Config{OutputDir: t.TempDir()}
	router := NewRouter(cfg, RuntimeContext{Env: "dev"}).(*Router)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/missing", nil)

	router.serveStatic("routes/missing/index.html", "routes/missing/index.server.go", rec, req, nil, "missing")

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

func TestRouter_ServeStatic_Gzip304Logging(t *testing.T) {
	t.Cleanup(cleanupTestArtifacts)

	cfg := Config{
		OutputDir:    t.TempDir(),
		CacheEnabled: true,
		DebugLogs:    true,
	}

	_ = os.MkdirAll(filepath.Join(cfg.OutputDir, "test"), 0755)
	_ = os.MkdirAll("routes/test", 0755)
	_ = os.MkdirAll("components", 0755)

	_ = os.WriteFile("layout.html", []byte(`
{{ define "layout" }}<html><body>{{ template "content" . }}</body></html>{{ end }}
`), 0644)
	_ = os.WriteFile("routes/test/index.html", []byte(`{{ define "content" }}<h1>Hello</h1>{{ end }}`), 0644)

	content := []byte("<html><body>Hello</body></html>")
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	_, _ = gw.Write(content)
	_ = gw.Close()
	_ = os.WriteFile(filepath.Join(cfg.OutputDir, "test", "index.html.gz"), buf.Bytes(), 0644)

	router := NewRouter(cfg, RuntimeContext{Env: "prod"}).(*Router)
	router.routes = []Route{{
		URLPattern: regexp.MustCompile("^test$"),
		HTMLPath:   "routes/test/index.html",
		ServerPath: "routes/test/index.server.go",
		FilePath:   "routes/test",
	}}

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	req.Header.Set("If-None-Match", generateETag(buf.Bytes()))

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotModified {
		t.Errorf("expected 304, got %d", rec.Code)
	}
}

func TestRouter_ServesFromHTMLCache_304(t *testing.T) {
	t.Cleanup(cleanupTestArtifacts)

	cfg := Config{
		OutputDir:    t.TempDir(),
		CacheEnabled: true,
		DebugLogs:    true,
		DebugHeaders: true,
	}

	routeKey := "test"
	cachedDir := filepath.Join(cfg.OutputDir, routeKey)
	_ = os.MkdirAll(cachedDir, 0755)

	content := []byte("<html><body>Hi</body></html>")
	etag := generateETag(content)
	_ = os.WriteFile(filepath.Join(cachedDir, "index.html"), content, 0644)

	_ = os.MkdirAll("routes/test", 0755)
	_ = os.WriteFile("routes/test/index.html", []byte(`{{ define "content" }}<h1>Ignored</h1>{{ end }}`), 0644)
	_ = os.WriteFile("layout.html", []byte(`{{ define "layout" }}<html><body>{{ template "content" . }}</body></html>{{ end }}`), 0644)
	_ = os.MkdirAll("components", 0755)

	router := NewRouter(cfg, RuntimeContext{Env: "prod"}).(*Router)
	router.routes = []Route{{
		URLPattern: regexp.MustCompile("^test$"),
		HTMLPath:   "routes/test/index.html",
		ServerPath: "routes/test/index.server.go",
		FilePath:   "routes/test",
	}}

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("If-None-Match", etag)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotModified {
		t.Errorf("expected 304, got %d", rec.Code)
	}
}

func TestRouter_ServesFromHTMLCache_WithHeaders(t *testing.T) {
	t.Cleanup(cleanupTestArtifacts)

	cfg := Config{
		OutputDir:    t.TempDir(),
		CacheEnabled: true,
		DebugLogs:    true,
		DebugHeaders: true,
	}

	routeKey := "test"
	cacheDir := filepath.Join(cfg.OutputDir, routeKey)
	_ = os.MkdirAll(cacheDir, 0755)

	content := []byte("<html><body>Hello Debug</body></html>")
	_ = os.WriteFile(filepath.Join(cacheDir, "index.html"), content, 0644)

	_ = os.MkdirAll("routes/test", 0755)
	_ = os.WriteFile("routes/test/index.html", []byte(`{{ define "content" }}<h1>Ignored</h1>{{ end }}`), 0644)
	_ = os.WriteFile("layout.html", []byte(`{{ define "layout" }}<html><body>{{ template "content" . }}</body></html>{{ end }}`), 0644)
	_ = os.MkdirAll("components", 0755)

	router := NewRouter(cfg, RuntimeContext{Env: "prod"}).(*Router)
	router.routes = []Route{{
		URLPattern: regexp.MustCompile("^test$"),
		HTMLPath:   "routes/test/index.html",
		ServerPath: "routes/test/index.server.go",
		FilePath:   "routes/test",
	}}

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	res := rec.Result()
	body, _ := io.ReadAll(res.Body)

	if res.StatusCode != http.StatusOK {
		t.Errorf("expected 200 OK, got %d", res.StatusCode)
	}
	if res.Header.Get("Content-Type") != "text/html" {
		t.Errorf("expected Content-Type text/html, got %s", res.Header.Get("Content-Type"))
	}
	if res.Header.Get("X-Barry-Cache") != "HIT" {
		t.Errorf("expected X-Barry-Cache header HIT, got %s", res.Header.Get("X-Barry-Cache"))
	}
	if !bytes.Contains(body, []byte("Hello Debug")) {
		t.Errorf("expected cached body content, got: %s", string(body))
	}
}

func TestRouter_CacheQueueFull_ImmediateWriteWithLogs(t *testing.T) {
	t.Cleanup(cleanupTestArtifacts)

	cfg := Config{
		OutputDir:    t.TempDir(),
		CacheEnabled: true,
		DebugLogs:    true,
	}

	_ = os.MkdirAll("routes/test", 0755)
	_ = os.WriteFile("routes/test/index.html", []byte(`<!-- layout: layout.html -->
{{ define "content" }}<h1>Hello Fallback</h1>{{ end }}`), 0644)

	_ = os.WriteFile("layout.html", []byte(`{{ define "layout" }}<html><body>{{ template "content" . }}</body></html>{{ end }}`), 0644)
	_ = os.MkdirAll("components", 0755)

	fullQueue := make(chan cacheWriteRequest, 1)
	fullQueue <- cacheWriteRequest{}

	originalQueue := cacheQueue
	cacheQueue = fullQueue
	defer func() { cacheQueue = originalQueue }()

	originalSave := SaveCachedHTMLFunc
	defer func() { SaveCachedHTMLFunc = originalSave }()
	SaveCachedHTMLFunc = func(_ Config, _ string, _ []byte) error {
		return nil
	}

	router := NewRouter(cfg, RuntimeContext{Env: "dev"}).(*Router)
	router.routes = []Route{{
		URLPattern: regexp.MustCompile("^test$"),
		HTMLPath:   "routes/test/index.html",
		ServerPath: "routes/test/index.server.go",
		FilePath:   "routes/test",
	}}

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	body := rec.Body.String()

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 OK, got %d", rec.Code)
	}
	if !strings.Contains(body, "<h1>Hello Fallback</h1>") {
		t.Errorf("expected rendered content, got: %s", body)
	}

	time.Sleep(200 * time.Millisecond)
}

func TestRouter_TemplateParseError(t *testing.T) {
	t.Cleanup(cleanupTestArtifacts)

	cfg := Config{
		OutputDir: t.TempDir(),
	}

	_ = os.MkdirAll("routes/test", 0755)
	_ = os.WriteFile("routes/test/index.html", []byte(`{{ define "content" }}{{ .Oops }}`), 0644)
	_ = os.WriteFile("layout.html", []byte(`{{ define "layout" }}<html>{{ template "content" . }}</html>{{ end }}`), 0644)
	_ = os.MkdirAll("components", 0755)

	router := NewRouter(cfg, RuntimeContext{Env: "dev"}).(*Router)
	router.routes = []Route{{
		URLPattern: regexp.MustCompile("^test$"),
		HTMLPath:   "routes/test/index.html",
		ServerPath: "routes/test/index.server.go",
		FilePath:   "routes/test",
	}}

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Template error") {
		t.Errorf("expected template parse error message, got: %s", rec.Body.String())
	}
}

func TestRouter_ServerFile_GenericErrorNoTemplate(t *testing.T) {
	t.Cleanup(cleanupTestArtifacts)

	cfg := Config{
		OutputDir:    t.TempDir(),
		CacheEnabled: false,
		DebugLogs:    true,
		DebugHeaders: true,
	}

	_ = os.MkdirAll("routes/fail", 0755)
	_ = os.WriteFile("routes/fail/index.html", []byte(`{{ define "content" }}Hello{{ end }}`), 0644)
	_ = os.WriteFile("routes/fail/index.server.go", []byte("// dummy"), 0644)
	_ = os.MkdirAll("components", 0755)

	router := NewRouter(cfg, RuntimeContext{Env: "dev"}).(*Router)
	router.routes = []Route{{
		URLPattern: regexp.MustCompile("^fail$"),
		HTMLPath:   "routes/fail/index.html",
		ServerPath: "routes/fail/index.server.go",
		FilePath:   "routes/fail",
	}}

	original := ExecuteServerFile
	ExecuteServerFile = func(_ string, _ map[string]string, _ bool) (map[string]interface{}, error) {
		return nil, errors.New("kaboom")
	}
	defer func() { ExecuteServerFile = original }()

	req := httptest.NewRequest(http.MethodGet, "/fail", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Server logic error: kaboom") {
		t.Errorf("expected server error message, got: %s", body)
	}
}

func TestRouter_ServeStatic_MissingLayout_LogsWarning(t *testing.T) {
	t.Cleanup(cleanupTestArtifacts)

	cfg := Config{
		OutputDir:    t.TempDir(),
		CacheEnabled: false,
		DebugLogs:    true,
		DebugHeaders: false,
	}

	_ = os.MkdirAll("routes/test", 0755)
	_ = os.WriteFile("routes/test/index.html", []byte(`<!-- layout: missing-layout.html -->
{{ define "content" }}<h1>Hello</h1>{{ end }}`), 0644)
	_ = os.MkdirAll("components", 0755)

	router := NewRouter(cfg, RuntimeContext{Env: "dev"}).(*Router)
	router.routes = []Route{{
		URLPattern: regexp.MustCompile("^test$"),
		HTMLPath:   "routes/test/index.html",
		ServerPath: "routes/test/index.server.go",
		FilePath:   "routes/test",
	}}

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 due to missing layout, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, `"layout" is undefined`) {
		t.Errorf("expected template execution error for missing layout, got: %s", body)
	}
}

func TestRouter_ServeStatic_SetsMissHeader(t *testing.T) {
	t.Cleanup(cleanupTestArtifacts)

	cfg := Config{
		OutputDir:    t.TempDir(),
		CacheEnabled: false,
		DebugLogs:    true,
		DebugHeaders: true,
	}

	_ = os.MkdirAll("routes/test", 0755)
	_ = os.WriteFile("routes/test/index.html", []byte(`<!-- layout: layout.html -->
{{ define "content" }}<h1>Hello Miss</h1>{{ end }}`), 0644)
	_ = os.WriteFile("layout.html", []byte(`{{ define "layout" }}<html><body>{{ template "content" . }}</body></html>{{ end }}`), 0644)
	_ = os.MkdirAll("components", 0755)

	router := NewRouter(cfg, RuntimeContext{Env: "dev"}).(*Router)
	router.routes = []Route{{
		URLPattern: regexp.MustCompile("^test$"),
		HTMLPath:   "routes/test/index.html",
		ServerPath: "routes/test/index.server.go",
		FilePath:   "routes/test",
	}}

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	res := rec.Result()
	if res.StatusCode != http.StatusOK {
		t.Errorf("expected 200 OK, got %d", res.StatusCode)
	}

	if got := res.Header.Get("X-Barry-Cache"); got != "MISS" {
		t.Errorf("expected X-Barry-Cache header to be MISS, got: %q", got)
	}
}

func TestRouter_UsesCachedTemplate(t *testing.T) {
	t.Cleanup(cleanupTestArtifacts)

	cfg := Config{
		OutputDir:    t.TempDir(),
		CacheEnabled: false,
		DebugLogs:    true,
		DebugHeaders: false,
	}

	_ = os.MkdirAll("routes/test", 0755)
	_ = os.WriteFile("routes/test/index.html", []byte(`<!-- layout: layout.html -->
{{ define "content" }}<h1>Hello Cache</h1>{{ end }}`), 0644)
	_ = os.WriteFile("layout.html", []byte(`{{ define "layout" }}<html><body>{{ template "content" . }}</body></html>{{ end }}`), 0644)
	_ = os.MkdirAll("components", 0755)

	router := NewRouter(cfg, RuntimeContext{Env: "dev"}).(*Router)
	router.routes = []Route{{
		URLPattern: regexp.MustCompile("^test$"),
		HTMLPath:   "routes/test/index.html",
		ServerPath: "routes/test/index.server.go",
		FilePath:   "routes/test",
	}}

	req1 := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec1 := httptest.NewRecorder()
	router.ServeHTTP(rec1, req1)

	if rec1.Code != http.StatusOK {
		t.Fatalf("first request failed: got %d", rec1.Code)
	}

	req2 := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec2 := httptest.NewRecorder()
	router.ServeHTTP(rec2, req2)

	if rec2.Code != http.StatusOK {
		t.Errorf("second request failed: got %d", rec2.Code)
	}
	if !strings.Contains(rec2.Body.String(), "Hello Cache") {
		t.Errorf("expected cached template content, got: %s", rec2.Body.String())
	}
}

func TestRouter_CacheQueueFull_ImmediateWriteErrorLogs(t *testing.T) {
	t.Cleanup(cleanupTestArtifacts)

	cfg := Config{
		OutputDir:    t.TempDir(),
		CacheEnabled: true,
		DebugLogs:    true,
	}

	_ = os.MkdirAll("routes/test", 0755)
	_ = os.WriteFile("routes/test/index.html", []byte(`<!-- layout: layout.html -->
{{ define "content" }}<h1>Cache Fail</h1>{{ end }}`), 0644)
	_ = os.WriteFile("layout.html", []byte(`{{ define "layout" }}<html><body>{{ template "content" . }}</body></html>{{ end }}`), 0644)
	_ = os.MkdirAll("components", 0755)

	fullQueue := make(chan cacheWriteRequest, 1)
	fullQueue <- cacheWriteRequest{}
	originalQueue := cacheQueue
	cacheQueue = fullQueue
	defer func() { cacheQueue = originalQueue }()

	originalSave := SaveCachedHTMLFunc
	defer func() { SaveCachedHTMLFunc = originalSave }()
	SaveCachedHTMLFunc = func(_ Config, _ string, _ []byte) error {
		return errors.New("disk full")
	}

	router := NewRouter(cfg, RuntimeContext{Env: "dev"}).(*Router)
	router.routes = []Route{{
		URLPattern: regexp.MustCompile("^test$"),
		HTMLPath:   "routes/test/index.html",
		ServerPath: "routes/test/index.server.go",
		FilePath:   "routes/test",
	}}

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	time.Sleep(100 * time.Millisecond)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 OK, got %d", rec.Code)
	}
}

func TestRouter_ServesIndexAtRoot(t *testing.T) {
	t.Cleanup(cleanupTestArtifacts)

	cfg := Config{
		OutputDir:    t.TempDir(),
		CacheEnabled: false,
		DebugLogs:    true,
		DebugHeaders: false,
	}

	_ = os.MkdirAll("routes", 0755)
	_ = os.WriteFile("routes/index.html", []byte(`<!-- layout: layout.html -->
{{ define "content" }}<h1>Home Page</h1>{{ end }}`), 0644)
	_ = os.WriteFile("layout.html", []byte(`{{ define "layout" }}<html><body>{{ template "content" . }}</body></html>{{ end }}`), 0644)
	_ = os.MkdirAll("components", 0755)

	router := NewRouter(cfg, RuntimeContext{Env: "dev"}).(*Router)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 OK, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Home Page") {
		t.Errorf("expected Home Page content, got: %s", rec.Body.String())
	}
}

func TestRouter_ParsesAndInjectsParams(t *testing.T) {
	t.Cleanup(cleanupTestArtifacts)

	cfg := Config{
		OutputDir:    t.TempDir(),
		CacheEnabled: false,
		DebugLogs:    true,
		DebugHeaders: false,
	}

	_ = os.MkdirAll("routes/posts/_id", 0755)
	_ = os.WriteFile("routes/posts/_id/index.html", []byte(`<!-- layout: layout.html -->
{{ define "content" }}<h1>Post ID: {{ .id }}</h1>{{ end }}`), 0644)
	_ = os.WriteFile("routes/posts/_id/index.server.go", []byte("// mock server logic"), 0644)

	_ = os.WriteFile("layout.html", []byte(`{{ define "layout" }}<html><body>{{ template "content" . }}</body></html>{{ end }}`), 0644)
	_ = os.MkdirAll("components", 0755)

	original := ExecuteServerFile
	ExecuteServerFile = func(_ string, params map[string]string, _ bool) (map[string]interface{}, error) {
		out := map[string]interface{}{}
		for k, v := range params {
			out[k] = v
		}
		return out, nil
	}
	defer func() { ExecuteServerFile = original }()

	router := NewRouter(cfg, RuntimeContext{Env: "dev"}).(*Router)
	router.routes = []Route{{
		URLPattern: regexp.MustCompile("^posts/([^/]+)$"),
		ParamKeys:  []string{"id"},
		HTMLPath:   "routes/posts/_id/index.html",
		ServerPath: "routes/posts/_id/index.server.go",
		FilePath:   "routes/posts/_id",
	}}

	req := httptest.NewRequest(http.MethodGet, "/posts/abc123", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 OK, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "Post ID: abc123") {
		t.Errorf("expected param injected, got: %s", body)
	}
}

func TestRouter_WatchEverything_NewWatcherFails(t *testing.T) {
	cfg, cleanup := setupRouterTestEnv(t)
	defer cleanup()

	original := newWatcher
	newWatcher = func() (*fsnotify.Watcher, error) {
		return nil, errors.New("failed to create watcher")
	}
	defer func() { newWatcher = original }()

	_ = NewRouter(cfg, RuntimeContext{
		Env:         "dev",
		EnableWatch: true,
		OnReload:    func() {},
	})

	time.Sleep(50 * time.Millisecond)
}

func TestRouter_getLayoutPath_ScannerError(t *testing.T) {
	t.Cleanup(cleanupTestArtifacts)

	cfg := Config{
		OutputDir: t.TempDir(),
	}

	_ = os.MkdirAll("routes/test", 0755)
	longLine := strings.Repeat("a", bufio.MaxScanTokenSize+10)
	_ = os.WriteFile("routes/test/index.html", []byte(longLine), 0644)

	router := NewRouter(cfg, RuntimeContext{Env: "dev"}).(*Router)

	path := router.getLayoutPath("routes/test/index.html")

	if path != "" {
		t.Errorf("expected empty layout path due to scanner error, got: %q", path)
	}
}

func TestRouter_ServeStatic_MissingLayoutWithDebugLogs(t *testing.T) {
	t.Cleanup(cleanupTestArtifacts)

	cfg := Config{
		OutputDir:    t.TempDir(),
		CacheEnabled: false,
		DebugLogs:    true,
	}

	_ = os.MkdirAll("routes/test", 0755)
	_ = os.WriteFile("routes/test/index.html", []byte(`<!-- layout: missing-layout.html -->
{{ define "content" }}<h1>Should Fail</h1>{{ end }}`), 0644)

	_ = os.MkdirAll("components", 0755)

	router := NewRouter(cfg, RuntimeContext{Env: "dev"}).(*Router)
	router.routes = []Route{{
		URLPattern: regexp.MustCompile("^test$"),
		HTMLPath:   "routes/test/index.html",
		ServerPath: "routes/test/index.server.go",
		FilePath:   "routes/test",
	}}

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 due to missing layout, got %d", rec.Code)
	}

	if !strings.Contains(rec.Body.String(), "Template execution error") {
		t.Errorf("expected template execution error, got: %s", rec.Body.String())
	}
}

func TestRouter_RenderErrorPage_MissingLayoutFallback(t *testing.T) {
	t.Cleanup(cleanupTestArtifacts)

	cfg := Config{
		OutputDir:    t.TempDir(),
		CacheEnabled: false,
	}

	_ = os.MkdirAll("routes/_error", 0755)

	_ = os.WriteFile("routes/_error/404.html", []byte(`<!-- layout: does-not-exist.html -->
{{ define "layout" }}<html><body>{{ template "content" . }}</body></html>{{ end }}
{{ define "content" }}<h1>Error 404 Override</h1>{{ end }}`), 0644)

	_ = os.WriteFile("routes/_error/index.html", []byte(`<!-- layout: does-not-exist.html -->
{{ define "layout" }}<html><body>{{ template "content" . }}</body></html>{{ end }}
{{ define "content" }}<h1>Error Fallback</h1>{{ end }}`), 0644)

	_ = os.MkdirAll("components", 0755)

	router := NewRouter(cfg, RuntimeContext{Env: "dev"}).(*Router)

	rec := httptest.NewRecorder()
	router.renderErrorPage(rec, http.StatusNotFound, "Missing layout", "/bad")

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404 fallback, got %d", rec.Code)
	}

	if !strings.Contains(rec.Body.String(), "Error Fallback") &&
		!strings.Contains(rec.Body.String(), "Error 404 Override") {
		t.Errorf("expected fallback content, got: %s", rec.Body.String())
	}
}

func TestRouter_RenderErrorPage_TemplateParseError(t *testing.T) {
	t.Cleanup(cleanupTestArtifacts)

	cfg := Config{
		OutputDir:    t.TempDir(),
		CacheEnabled: false,
	}

	_ = os.MkdirAll("routes/_error", 0755)
	_ = os.WriteFile("routes/_error/index.html", []byte(`<!-- layout: layout.html -->
{{ define "layout" }}<html><body>{{ template "content" . }}</body></html>{{ end }}
{{ define "content" }}<h1>{{ .Message </h1>{{ end }}`), 0644)

	_ = os.MkdirAll("components", 0755)

	router := NewRouter(cfg, RuntimeContext{Env: "dev"}).(*Router)

	rec := httptest.NewRecorder()
	router.renderErrorPage(rec, http.StatusNotFound, "Invalid template syntax", "/broken")

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 due to parse error, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "Template error") {
		t.Errorf("expected template parse error message, got: %s", body)
	}
}
