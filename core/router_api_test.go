package core

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func mockRouter(t *testing.T, apiDir string) *Router {
	t.Helper()

	cfg := Config{}
	ctx := RuntimeContext{Env: "dev"}
	r := &Router{
		config: cfg,
		env:    ctx.Env,
	}
	r.loadApiRoutes()
	return r
}

func TestLoadApiRoutes_BasicRoute(t *testing.T) {
	tmp := t.TempDir()

	_ = os.WriteFile(filepath.Join(tmp, "go.mod"), []byte("module example.com/mock\n"), 0644)

	apiPath := filepath.Join(tmp, "api", "hello")
	_ = os.MkdirAll(apiPath, 0755)

	file := filepath.Join(apiPath, "index.go")
	_ = os.WriteFile(file, []byte("// test"), 0644)

	wd, _ := os.Getwd()
	defer os.Chdir(wd)
	_ = os.Chdir(tmp)

	r := &Router{}
	r.loadApiRoutes()

	if len(r.apiRoutes) != 1 {
		t.Fatalf("expected 1 API route, got %d", len(r.apiRoutes))
	}

	route := r.apiRoutes[0]

	expectedAbs, _ := filepath.Abs(file)
	actualAbs, _ := filepath.Abs(route.ServerPath)

	expectedResolved, _ := filepath.EvalSymlinks(expectedAbs)
	actualResolved, _ := filepath.EvalSymlinks(actualAbs)

	if expectedResolved != actualResolved {
		t.Errorf("expected ServerPath to be %s, got %s", expectedResolved, actualResolved)
	}

	if route.URLPattern == nil {
		t.Errorf("expected compiled URL pattern")
	}
	if route.Method != "ANY" {
		t.Errorf("expected method ANY, got %s", route.Method)
	}
}

func TestLoadApiRoutes_WithParam(t *testing.T) {
	tmp := t.TempDir()

	_ = os.WriteFile(filepath.Join(tmp, "go.mod"), []byte("module example.com/mock\n"), 0644)

	apiPath := filepath.Join(tmp, "api", "user", "_id")
	_ = os.MkdirAll(apiPath, 0755)

	file := filepath.Join(apiPath, "index.go")
	_ = os.WriteFile(file, []byte("// test"), 0644)

	wd, _ := os.Getwd()
	defer os.Chdir(wd)
	_ = os.Chdir(tmp)

	r := &Router{}
	r.loadApiRoutes()

	if len(r.apiRoutes) != 1 {
		t.Fatalf("expected 1 API route, got %d", len(r.apiRoutes))
	}

	route := r.apiRoutes[0]
	if len(route.ParamKeys) != 1 || route.ParamKeys[0] != "id" {
		t.Errorf("expected param key 'id', got %+v", route.ParamKeys)
	}

	if !route.URLPattern.MatchString("user/123") {
		t.Errorf("URLPattern did not match expected format")
	}
}

var _origExecuteAPIFile = ExecuteAPIFile

func TestHandleAPI_ReturnsJSON(t *testing.T) {
	defer func() { ExecuteAPIFile = _origExecuteAPIFile }()

	ExecuteAPIFile = func(path string, req *http.Request, params map[string]string, devMode bool) ([]byte, error) {
		return []byte(`{"hello":"world"}`), nil
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/hello", nil)

	r := &Router{env: "dev"}
	route := ApiRoute{ServerPath: "fake/path"}

	r.handleAPI(rec, req, route, map[string]string{})

	resp := rec.Result()
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	if !strings.Contains(string(body), `"hello":"world"`) {
		t.Errorf("unexpected body: %s", body)
	}
}

func TestHandleAPI_NotFound(t *testing.T) {
	defer func() { ExecuteAPIFile = _origExecuteAPIFile }()

	ExecuteAPIFile = func(path string, req *http.Request, params map[string]string, devMode bool) ([]byte, error) {
		return nil, ErrNotFound
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/missing", nil)

	r := &Router{env: "dev"}
	route := ApiRoute{ServerPath: "fake/path"}

	r.handleAPI(rec, req, route, map[string]string{})

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Not Found") {
		t.Errorf("expected 'Not Found' message")
	}
}

func TestHandleAPI_InternalError(t *testing.T) {
	defer func() { ExecuteAPIFile = _origExecuteAPIFile }()

	ExecuteAPIFile = func(path string, req *http.Request, params map[string]string, devMode bool) ([]byte, error) {
		return nil, fmt.Errorf("boom")
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/error", nil)

	r := &Router{env: "dev"}
	route := ApiRoute{ServerPath: "fake/path"}

	r.handleAPI(rec, req, route, map[string]string{})

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Server error") {
		t.Errorf("expected 'Server error' in response")
	}
}
