package barry

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

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
