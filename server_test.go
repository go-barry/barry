package barry

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-barry/barry/core"
)

type mockReloader struct{}

func (m *mockReloader) BroadcastReload() {}
func (m *mockReloader) Handler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("reload ok"))
}

func TestDetectMimeType(t *testing.T) {
	tests := map[string]string{
		"file.css":     "text/css",
		"script.js":    "application/javascript",
		"image.webp":   "image/webp",
		"icon.svg":     "image/svg+xml",
		"photo.png":    "image/png",
		"photo.jpeg":   "image/jpeg",
		"font.woff":    "font/woff",
		"font.woff2":   "font/woff2",
		"unknown.file": "application/octet-stream",
	}

	for filename, expected := range tests {
		t.Run(filename, func(t *testing.T) {
			mime := detectMimeType(filename)
			if mime != expected {
				t.Errorf("got %s, want %s", mime, expected)
			}
		})
	}
}

func TestAcceptsGzip(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Accept-Encoding", "gzip, deflate")

	if !acceptsGzip(req) {
		t.Error("expected true for Accept-Encoding with gzip")
	}

	req.Header.Set("Accept-Encoding", "br")
	if acceptsGzip(req) {
		t.Error("expected false for Accept-Encoding without gzip")
	}
}

func TestServeFileWithHeaders(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "test.txt")
	content := "Hello, Barry!"
	_ = os.WriteFile(filePath, []byte(content), 0644)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/static/test.txt", nil)

	serveFileWithHeaders(rec, req, filePath, "no-cache")

	resp := rec.Result()
	defer resp.Body.Close()

	if ct := resp.Header.Get("Content-Type"); ct != "application/octet-stream" {
		t.Errorf("unexpected content-type: %s", ct)
	}
	if cc := resp.Header.Get("Cache-Control"); cc != "no-cache" {
		t.Errorf("unexpected cache-control: %s", cc)
	}

	body, _ := io.ReadAll(resp.Body)
	if string(body) != content {
		t.Errorf("unexpected body: %s", body)
	}
}

func TestMakeStaticHandlerReturns404ForMissingFile(t *testing.T) {
	publicDir := t.TempDir()
	cacheDir := t.TempDir()

	handler := makeStaticHandler(publicDir, cacheDir)

	req := httptest.NewRequest(http.MethodGet, "/static/missing.txt", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)
	resp := rec.Result()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
}

func TestMakeStaticHandlerServesPublicFile(t *testing.T) {
	publicDir := t.TempDir()
	cacheDir := t.TempDir()

	testFile := "hello.txt"
	filePath := filepath.Join(publicDir, testFile)
	expected := "Hello from public!"
	_ = os.WriteFile(filePath, []byte(expected), 0644)

	handler := makeStaticHandler(publicDir, cacheDir)

	req := httptest.NewRequest(http.MethodGet, "/static/"+testFile, nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)
	resp := rec.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 OK, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	if string(body) != expected {
		t.Errorf("expected body %q, got %q", expected, body)
	}
}

func TestMakeStaticHandlerRejectsTraversal(t *testing.T) {
	publicDir := t.TempDir()
	cacheDir := t.TempDir()

	handler := makeStaticHandler(publicDir, cacheDir)

	req := httptest.NewRequest(http.MethodGet, "/static/../secrets.txt", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 Bad Request, got %d", rec.Code)
	}
}

func TestMakeStaticHandlerServesGzipFromCache(t *testing.T) {
	publicDir := t.TempDir()
	cacheDir := t.TempDir()

	fileName := "script.js"
	cachedFile := filepath.Join(cacheDir, fileName)
	gzipFile := cachedFile + ".gz"

	_ = os.WriteFile(gzipFile, []byte("gzipped content"), 0644)

	handler := makeStaticHandler(publicDir, cacheDir)

	req := httptest.NewRequest(http.MethodGet, "/static/"+fileName, nil)
	req.Header.Set("Accept-Encoding", "gzip")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)
	resp := rec.Result()
	defer resp.Body.Close()

	if resp.Header.Get("Content-Encoding") != "gzip" {
		t.Error("expected gzip Content-Encoding")
	}
	if resp.Header.Get("Vary") != "Accept-Encoding" {
		t.Error("expected Vary: Accept-Encoding header")
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 OK, got %d", resp.StatusCode)
	}
}

func TestMakeStaticHandlerServesNonGzipCacheFile(t *testing.T) {
	publicDir := t.TempDir()
	cacheDir := t.TempDir()

	fileName := "styles.css"
	cachedFile := filepath.Join(cacheDir, fileName)
	_ = os.WriteFile(cachedFile, []byte("cached css"), 0644)

	handler := makeStaticHandler(publicDir, cacheDir)

	req := httptest.NewRequest(http.MethodGet, "/static/"+fileName, nil)
	req.Header.Set("Accept-Encoding", "gzip")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)
	resp := rec.Result()
	defer resp.Body.Close()

	if resp.Header.Get("Content-Encoding") != "" {
		t.Error("did not expect Content-Encoding for non-gzip file")
	}

	body, _ := io.ReadAll(resp.Body)
	if string(body) != "cached css" {
		t.Errorf("unexpected body: %s", body)
	}
}

func TestSetupDevStaticRoutesFaviconAndRobots(t *testing.T) {
	publicDir := t.TempDir()
	faviconPath := filepath.Join(publicDir, "favicon.ico")
	robotsPath := filepath.Join(publicDir, "robots.txt")

	_ = os.WriteFile(faviconPath, []byte("icon"), 0644)
	_ = os.WriteFile(robotsPath, []byte("robots"), 0644)

	mux := http.NewServeMux()
	setupDevStaticRoutes(mux, publicDir)

	tests := []struct {
		path     string
		expected string
	}{
		{"/favicon.ico", "icon"},
		{"/robots.txt", "robots"},
	}

	for _, test := range tests {
		req := httptest.NewRequest(http.MethodGet, test.path, nil)
		rec := httptest.NewRecorder()

		mux.ServeHTTP(rec, req)
		resp := rec.Result()
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		if string(body) != test.expected {
			t.Errorf("expected %q, got %q", test.expected, body)
		}

		if cc := resp.Header.Get("Cache-Control"); cc != "no-store" {
			t.Errorf("expected Cache-Control: no-store for %s, got %s", test.path, cc)
		}
	}
}

func TestMakeStaticHandlerStripsQueryParams(t *testing.T) {
	publicDir := t.TempDir()
	cacheDir := t.TempDir()

	fileName := "main.js"
	_ = os.WriteFile(filepath.Join(publicDir, fileName), []byte("main js"), 0644)

	handler := makeStaticHandler(publicDir, cacheDir)

	req := httptest.NewRequest(http.MethodGet, "/static/main.js?v=1234", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)
	resp := rec.Result()
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if string(body) != "main js" {
		t.Errorf("expected file body to match, got %q", body)
	}
}

func TestBuildServerInDev(t *testing.T) {
	originalLoadConfig := core.LoadConfig
	originalNewRouter := core.NewRouter
	originalNewLiveReloader := core.NewLiveReloader

	defer func() {
		core.LoadConfig = originalLoadConfig
		core.NewRouter = originalNewRouter
		core.NewLiveReloader = originalNewLiveReloader
	}()

	core.LoadConfig = func(path string) *core.Config {
		return &core.Config{OutputDir: t.TempDir()}
	}

	core.NewRouter = func(c core.Config, ctx core.RuntimeContext) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, "router ok")
		})
	}

	core.NewLiveReloader = func() core.LiveReloaderInterface {
		return &mockReloader{}
	}

	cfg := RuntimeConfig{
		Env:         "dev",
		EnableCache: true,
		Port:        3001,
	}

	addr, handler := BuildServer(cfg)

	if addr != ":3001" {
		t.Errorf("expected :3001, got %s", addr)
	}

	req := httptest.NewRequest(http.MethodGet, "/__barry_reload", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 OK, got %d", rec.Code)
	}
	if body := rec.Body.String(); body != "reload ok" {
		t.Errorf("expected 'reload ok', got %q", body)
	}
}

func TestStart_CallsListenAndServe(t *testing.T) {
	called := false
	var gotAddr string
	var gotHandler http.Handler

	original := ListenAndServe
	ListenAndServe = func(addr string, handler http.Handler) error {
		called = true
		gotAddr = addr
		gotHandler = handler
		return nil
	}
	defer func() { ListenAndServe = original }()

	core.LoadConfig = func(path string) *core.Config {
		return &core.Config{OutputDir: t.TempDir()}
	}
	core.NewRouter = func(c core.Config, ctx core.RuntimeContext) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, "ok")
		})
	}
	core.NewLiveReloader = func() core.LiveReloaderInterface {
		return &mockReloader{}
	}

	cfg := RuntimeConfig{
		Env:         "dev",
		EnableCache: true,
		Port:        4321,
	}
	Start(cfg)

	if !called {
		t.Fatal("expected ListenAndServe to be called")
	}
	if gotAddr != ":4321" {
		t.Errorf("expected addr ':4321', got %q", gotAddr)
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	gotHandler.ServeHTTP(rec, req)
	if rec.Body.String() != "ok" {
		t.Errorf("expected handler to respond with 'ok', got %q", rec.Body.String())
	}
}

func TestDevStaticRoutes_FileServerAddsNoStore(t *testing.T) {
	publicDir := t.TempDir()
	_ = os.WriteFile(filepath.Join(publicDir, "test.js"), []byte("content"), 0644)

	mux := http.NewServeMux()
	setupDevStaticRoutes(mux, publicDir)

	req := httptest.NewRequest(http.MethodGet, "/static/test.js", nil)
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Body.String() != "content" {
		t.Errorf("expected file content, got %q", rec.Body.String())
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "no-store" {
		t.Errorf("expected 'no-store', got %q", cc)
	}
}

func TestBuildServerInProd(t *testing.T) {
	publicDir := "public"
	_ = os.MkdirAll(publicDir, 0755)
	_ = os.WriteFile(filepath.Join(publicDir, "favicon.ico"), []byte("icon"), 0644)
	_ = os.WriteFile(filepath.Join(publicDir, "robots.txt"), []byte("robots"), 0644)

	t.Cleanup(func() {
		_ = os.RemoveAll(publicDir)
	})

	core.LoadConfig = func(path string) *core.Config {
		return &core.Config{OutputDir: t.TempDir()}
	}
	core.NewRouter = func(c core.Config, ctx core.RuntimeContext) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, "router")
		})
	}

	cfg := RuntimeConfig{Env: "prod", EnableCache: false, Port: 1234}
	addr, handler := BuildServer(cfg)

	if addr != ":1234" {
		t.Errorf("expected :1234, got %s", addr)
	}

	tests := []struct {
		path     string
		expected string
	}{
		{"/favicon.ico", "icon"},
		{"/robots.txt", "robots"},
	}

	for _, test := range tests {
		req := httptest.NewRequest(http.MethodGet, test.path, nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		body := rec.Body.String()
		if body != test.expected {
			t.Errorf("for %s: expected %q, got %q", test.path, test.expected, body)
		}
	}
}

func TestStart_ExitsOnServerFailure(t *testing.T) {
	var exited bool
	var exitCode int
	var stderr string

	originalExit := Exit
	originalListenAndServe := ListenAndServe
	defer func() {
		Exit = originalExit
		ListenAndServe = originalListenAndServe
	}()

	Exit = func(code int) {
		exited = true
		exitCode = code
	}

	ListenAndServe = func(addr string, handler http.Handler) error {
		return fmt.Errorf("simulated server failure")
	}

	r, w, _ := os.Pipe()
	stdErrBackup := os.Stderr
	os.Stderr = w

	cfg := RuntimeConfig{
		Env:         "prod",
		EnableCache: false,
		Port:        1234,
	}
	Start(cfg)

	_ = w.Close()
	os.Stderr = stdErrBackup
	buf, _ := io.ReadAll(r)
	stderr = string(buf)

	if !exited {
		t.Fatal("expected Exit to be called")
	}
	if exitCode != 1 {
		t.Errorf("expected exit code 1, got %d", exitCode)
	}
	if !strings.Contains(stderr, "❌ Server failed: simulated server failure") {
		t.Errorf("unexpected stderr output: %q", stderr)
	}
}
