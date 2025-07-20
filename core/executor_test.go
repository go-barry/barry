package core

import (
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestFindGoModRoot(t *testing.T) {
	tmpDir := t.TempDir()

	goMod := []byte("module example.com/testmod\n")
	goModPath := filepath.Join(tmpDir, "go.mod")
	if err := os.WriteFile(goModPath, goMod, 0644); err != nil {
		t.Fatal(err)
	}

	subDir := filepath.Join(tmpDir, "nested", "more")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatal(err)
	}

	startPath := filepath.Join(subDir, "index.server.go")
	if err := os.WriteFile(startPath, []byte("// dummy file"), 0644); err != nil {
		t.Fatal(err)
	}

	modRoot, modName, err := findGoModRoot(startPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if modRoot != tmpDir {
		t.Errorf("expected modRoot to be %s, got %s", tmpDir, modRoot)
	}
	if modName != "example.com/testmod" {
		t.Errorf("expected module name 'example.com/testmod', got %s", modName)
	}
}

func TestExecuteServerFile_GoModNotFound(t *testing.T) {
	tmp := t.TempDir()

	serverPath := filepath.Join(tmp, "index.server.go")
	_ = os.WriteFile(serverPath, []byte("// dummy"), 0644)

	_, err := ExecuteServerFile(serverPath, nil, map[string]string{}, false)
	if err == nil || !strings.Contains(err.Error(), "could not resolve go.mod") {
		t.Fatalf("expected go.mod resolution error, got: %v", err)
	}
}

func TestExecuteServerFile_BasicSuccess(t *testing.T) {
	tmp := t.TempDir()

	goMod := []byte("module example.com/barrytest\n\ngo 1.20\n")
	_ = os.WriteFile(filepath.Join(tmp, "go.mod"), goMod, 0644)

	routeDir := filepath.Join(tmp, "routes", "foo")
	_ = os.MkdirAll(routeDir, 0755)

	serverCode := `package foo

import "net/http"

func HandleRequest(r *http.Request, params map[string]string) (map[string]interface{}, error) {
	return map[string]interface{}{
		"foo": "bar",
	}, nil
}
`
	serverPath := filepath.Join(routeDir, "index.server.go")
	_ = os.WriteFile(serverPath, []byte(serverCode), 0644)

	req := httptest.NewRequest(http.MethodGet, "/", nil)

	result, err := ExecuteServerFile(serverPath, req, map[string]string{}, false)
	if err != nil {
		t.Fatalf("ExecuteServerFile failed: %v", err)
	}

	if val, ok := result["foo"]; !ok || val != "bar" {
		t.Errorf(`expected result["foo"] = "bar", got: %+v`, result)
	}
}

func TestExecuteServerFile_NotFoundError(t *testing.T) {
	tmp := t.TempDir()
	_ = os.WriteFile(filepath.Join(tmp, "go.mod"), []byte("module example.com/barrytest\n\ngo 1.20\n"), 0644)

	routeDir := filepath.Join(tmp, "routes", "404test")
	_ = os.MkdirAll(routeDir, 0755)

	serverCode := `package test

import (
	"errors"
	"net/http"
)

func HandleRequest(r *http.Request, params map[string]string) (map[string]interface{}, error) {
	return nil, errors.New("barry: not found")
}
`
	serverPath := filepath.Join(routeDir, "index.server.go")
	_ = os.WriteFile(serverPath, []byte(serverCode), 0644)

	req := httptest.NewRequest(http.MethodGet, "/", nil)

	_, err := ExecuteServerFile(serverPath, req, map[string]string{}, false)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !IsNotFoundError(err) {
		t.Errorf("expected ErrNotFound, got: %v", err)
	}
}

func TestExecuteServerFile_MkdirAllFails(t *testing.T) {
	tmp := t.TempDir()
	fixedTime := time.Date(2025, 7, 6, 12, 0, 0, 0, time.UTC)

	_ = os.WriteFile(filepath.Join(tmp, "go.mod"), []byte("module example.com/barrytest\n"), 0644)
	routeDir := filepath.Join(tmp, "routes")
	_ = os.MkdirAll(routeDir, 0755)

	serverPath := filepath.Join(routeDir, "index.server.go")
	_ = os.WriteFile(serverPath, []byte(`package routes; import "net/http"; func HandleRequest(r *http.Request, p map[string]string)(map[string]interface{}, error){ return nil, nil }`), 0644)

	absPath, _ := filepath.Abs(serverPath)
	hash := sha256.Sum256([]byte(absPath + fixedTime.String()))
	runDir := filepath.Join(tmp, ".barry-tmp", fmt.Sprintf("%x", hash[:8]))

	_ = os.MkdirAll(filepath.Dir(runDir), 0755)
	_ = os.WriteFile(runDir, []byte("I am a file, not a dir"), 0644)

	req := httptest.NewRequest(http.MethodGet, "/", nil)

	_, err := ExecuteServerFileWithTime(serverPath, req, map[string]string{}, false, func() time.Time { return fixedTime })
	if err == nil || !strings.Contains(err.Error(), "could not create temp dir") {
		t.Fatalf("expected temp dir creation error, got: %v", err)
	}
}

func TestExecuteServerFile_WriteFileFails(t *testing.T) {
	tmp := t.TempDir()
	fixedTime := time.Date(2025, 7, 6, 12, 0, 0, 0, time.UTC)

	_ = os.WriteFile(filepath.Join(tmp, "go.mod"), []byte("module example.com/barrytest\n"), 0644)
	routeDir := filepath.Join(tmp, "routes", "failwrite")
	_ = os.MkdirAll(routeDir, 0755)

	serverPath := filepath.Join(routeDir, "index.server.go")
	_ = os.WriteFile(serverPath, []byte(`package failwrite; import "net/http"; func HandleRequest(r *http.Request, p map[string]string)(map[string]interface{}, error){ return nil, nil }`), 0644)

	absPath, _ := filepath.Abs(serverPath)
	hash := sha256.Sum256([]byte(absPath + fixedTime.String()))
	runDir := filepath.Join(tmp, ".barry-tmp", fmt.Sprintf("%x", hash[:8]))
	_ = os.MkdirAll(runDir, 0755)
	_ = os.Chmod(runDir, 0500)

	t.Cleanup(func() {
		_ = os.Chmod(runDir, 0755)
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)

	_, err := ExecuteServerFileWithTime(serverPath, req, map[string]string{}, false, func() time.Time { return fixedTime })
	if err == nil || !strings.Contains(err.Error(), "could not write temp file") {
		t.Fatalf("expected write file error, got: %v", err)
	}
}

func TestFindGoModRoot_NoModuleLine(t *testing.T) {
	tmpDir := t.TempDir()
	_ = os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte("// no module line\n"), 0644)
	file := filepath.Join(tmpDir, "somefile.go")
	_ = os.WriteFile(file, []byte("// dummy"), 0644)

	_, _, err := findGoModRoot(file)
	if err == nil || !strings.Contains(err.Error(), "go.mod not found") {
		t.Fatalf("expected go.mod not found error, got: %v", err)
	}
}

func TestExecuteServerFile_BadJSONOutput(t *testing.T) {
	tmp := t.TempDir()
	_ = os.WriteFile(filepath.Join(tmp, "go.mod"), []byte("module example.com/jsonfail\n"), 0644)

	routeDir := filepath.Join(tmp, "routes", "badjson")
	_ = os.MkdirAll(routeDir, 0755)
	serverPath := filepath.Join(routeDir, "index.server.go")
	code := `package badjson
import ("net/http"; "fmt")
func HandleRequest(r *http.Request, params map[string]string) (map[string]interface{}, error) {
	fmt.Println("not json")
	return nil, nil
}`
	_ = os.WriteFile(serverPath, []byte(code), 0644)

	req := httptest.NewRequest(http.MethodGet, "/", nil)

	_, err := ExecuteServerFile(serverPath, req, map[string]string{}, false)
	if err == nil || !strings.Contains(err.Error(), "json decode error") {
		t.Fatalf("expected JSON decode error, got: %v", err)
	}
}

func TestExecuteServerFile_TemplateExecutionFails(t *testing.T) {
	orig := runnerTemplate
	defer func() { runnerTemplate = orig }()
	runnerTemplate = `{{ .DoesNotExist }}`

	tmp := t.TempDir()
	_ = os.WriteFile(filepath.Join(tmp, "go.mod"), []byte("module example.com/test\n"), 0644)

	serverPath := filepath.Join(tmp, "index.server.go")
	_ = os.WriteFile(serverPath, []byte(`package test; import "net/http"; func HandleRequest(r *http.Request, _ map[string]string) (map[string]interface{}, error) { return nil, nil }`), 0644)

	req := httptest.NewRequest(http.MethodGet, "/", nil)

	_, err := ExecuteServerFile(serverPath, req, map[string]string{}, false)
	if err == nil || !strings.Contains(err.Error(), "template execution error") {
		t.Fatalf("expected template execution error, got: %v", err)
	}
}

func TestExecuteServerFile_DevModeStderrMultiWriter(t *testing.T) {
	tmp := t.TempDir()
	_ = os.WriteFile(filepath.Join(tmp, "go.mod"), []byte("module example.com/devmode\n"), 0644)

	routeDir := filepath.Join(tmp, "routes", "dm")
	_ = os.MkdirAll(routeDir, 0755)

	serverPath := filepath.Join(routeDir, "index.server.go")
	_ = os.WriteFile(serverPath, []byte(`package dm; import "net/http"; func HandleRequest(r *http.Request, p map[string]string)(map[string]interface{}, error){ return map[string]interface{}{"x": 1}, nil }`), 0644)

	req := httptest.NewRequest(http.MethodGet, "/", nil)

	_, err := ExecuteServerFile(serverPath, req, map[string]string{}, true)
	if err != nil {
		t.Fatalf("expected success in dev mode, got: %v", err)
	}
}

func TestExecuteServerFile_CommandFails(t *testing.T) {
	tmp := t.TempDir()
	_ = os.WriteFile(filepath.Join(tmp, "go.mod"), []byte("module example.com/boom\n"), 0644)

	routeDir := filepath.Join(tmp, "routes", "boom")
	_ = os.MkdirAll(routeDir, 0755)

	serverPath := filepath.Join(routeDir, "index.server.go")
	_ = os.WriteFile(serverPath, []byte(`package boom; import "net/http"; func HandleRequest(r *http.Request, p map[string]string)(map[string]interface{}, error){ return nil, nil }`), 0644)

	orig := runnerTemplate
	defer func() { runnerTemplate = orig }()
	runnerTemplate = `package main; import "bad/import/path" // bad

func main() {}
`
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	_, err := ExecuteServerFile(serverPath, req, map[string]string{}, false)
	if err == nil || !strings.Contains(err.Error(), "exec error") {
		t.Fatalf("expected exec error, got: %v", err)
	}
}

func TestFindGoModRoot_ReadFileFails(t *testing.T) {
	tmpDir := t.TempDir()
	goModPath := filepath.Join(tmpDir, "go.mod")
	_ = os.WriteFile(goModPath, []byte("module example.com/test"), 0000)

	file := filepath.Join(tmpDir, "main.go")
	_ = os.WriteFile(file, []byte("// dummy"), 0644)

	_, _, err := findGoModRoot(file)
	if err == nil || !strings.Contains(err.Error(), "failed to read go.mod") {
		t.Fatalf("expected read error, got: %v", err)
	}
}

func TestExecuteAPIFile_JSONOutput(t *testing.T) {
	tmp := t.TempDir()

	_ = os.WriteFile(filepath.Join(tmp, "go.mod"), []byte("module example.com/testapi\n"), 0644)

	dir := filepath.Join(tmp, "api", "hello")
	_ = os.MkdirAll(dir, 0755)

	code := `package hello

import "net/http"

func HandleAPI(r *http.Request, p map[string]string) (interface{}, error) {
	return map[string]interface{}{
		"message": "hi",
	}, nil
}`

	path := filepath.Join(dir, "index.go")
	_ = os.WriteFile(path, []byte(code), 0644)

	req := httptest.NewRequest("GET", "/?x=1", nil)

	result, err := ExecuteAPIFile(path, req, map[string]string{}, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(string(result), "hi") {
		t.Errorf("expected JSON output with 'hi', got: %s", result)
	}
}

func TestExecuteAPIFile_GoModNotFound(t *testing.T) {
	tmp := t.TempDir()

	serverPath := filepath.Join(tmp, "index.go")
	_ = os.WriteFile(serverPath, []byte("// dummy"), 0644)

	req, _ := http.NewRequest("GET", "/", nil)
	_, err := ExecuteAPIFile(serverPath, req, map[string]string{}, false)
	if err == nil || !strings.Contains(err.Error(), "could not resolve go.mod") {
		t.Fatalf("expected go.mod resolution error, got: %v", err)
	}
}

func TestExecuteAPIFile_TemplateExecutionFails(t *testing.T) {
	orig := apiRunnerTemplate
	defer func() { apiRunnerTemplate = orig }()

	apiRunnerTemplate = `{{ .DoesNotExist }}`

	tmp := t.TempDir()
	_ = os.WriteFile(filepath.Join(tmp, "go.mod"), []byte("module example.com/broken\n"), 0644)

	path := filepath.Join(tmp, "api", "bad")
	_ = os.MkdirAll(path, 0755)
	_ = os.WriteFile(filepath.Join(path, "index.go"), []byte("package bad"), 0644)

	req := httptest.NewRequest("GET", "/", nil)

	_, err := ExecuteAPIFile(filepath.Join(path, "index.go"), req, map[string]string{}, false)
	if err == nil || !strings.Contains(err.Error(), "template execution error") {
		t.Fatalf("expected template error, got: %v", err)
	}
}

func TestExecuteAPIFile_CommandFails(t *testing.T) {
	tmp := t.TempDir()
	_ = os.WriteFile(filepath.Join(tmp, "go.mod"), []byte("module example.com/badcmd\n"), 0644)

	path := filepath.Join(tmp, "api", "crash")
	_ = os.MkdirAll(path, 0755)

	code := `package crash

import "net/http"

func HandleAPI(r *http.Request, p map[string]string) (interface{}, error) {
	panic("fail")
}`
	apiFile := filepath.Join(path, "index.go")
	_ = os.WriteFile(apiFile, []byte(code), 0644)

	req := httptest.NewRequest("GET", "/", nil)

	_, err := ExecuteAPIFile(apiFile, req, map[string]string{}, false)
	if err == nil || !strings.Contains(err.Error(), "exec error") {
		t.Fatalf("expected exec error, got: %v", err)
	}
}

func TestExecuteAPIFile_NotFoundError(t *testing.T) {
	tmp := t.TempDir()
	_ = os.WriteFile(filepath.Join(tmp, "go.mod"), []byte("module example.com/404api\n"), 0644)

	path := filepath.Join(tmp, "api", "missing")
	_ = os.MkdirAll(path, 0755)

	code := `package missing

import (
	"errors"
	"net/http"
)

func HandleAPI(r *http.Request, p map[string]string) (interface{}, error) {
	return nil, errors.New("barry: not found")
}`
	apiFile := filepath.Join(path, "index.go")
	_ = os.WriteFile(apiFile, []byte(code), 0644)

	req := httptest.NewRequest("GET", "/", nil)

	_, err := ExecuteAPIFile(apiFile, req, map[string]string{}, false)
	if err == nil || !IsNotFoundError(err) {
		t.Fatalf("expected not found error, got: %v", err)
	}
}

func TestExecuteAPIFile_CannotResolveRelativeImport(t *testing.T) {
	tmp := t.TempDir()

	_ = os.WriteFile(filepath.Join(tmp, "go.mod"), []byte("module example.com/irrelevant\n"), 0644)
	apiPath := filepath.Join(tmp, "api", "foo")
	_ = os.MkdirAll(apiPath, 0755)
	apiFile := filepath.Join(apiPath, "index.go")
	_ = os.WriteFile(apiFile, []byte(`package foo; import "net/http"; func HandleAPI(r *http.Request, _ map[string]string)(interface{}, error){ return nil, nil }`), 0644)

	orig := findGoModRoot
	defer func() { findGoModRoot = orig }()
	findGoModRoot = func(_ string) (string, string, error) {
		return string([]byte{0x7f}), "irrelevant", nil
	}

	req, _ := http.NewRequest("GET", "/", nil)
	_, err := ExecuteAPIFile(apiFile, req, nil, false)
	if err == nil || !strings.Contains(err.Error(), "cannot resolve relative import path") {
		t.Fatalf("expected relative import error, got: %v", err)
	}
}

func TestExecuteServerFile_UnformattedSource(t *testing.T) {
	tmp := t.TempDir()
	_ = os.WriteFile(filepath.Join(tmp, "go.mod"), []byte("module example.com/badformat\n"), 0644)

	routeDir := filepath.Join(tmp, "routes", "ugly")
	_ = os.MkdirAll(routeDir, 0755)

	serverPath := filepath.Join(routeDir, "index.server.go")
	_ = os.WriteFile(serverPath, []byte(`package ugly

import "net/http"

func HandleRequest(r *http.Request, p map[string]string) (map[string]interface{}, error) {
return map[string]interface{}{ "ok":true}, nil
}
`), 0644)

	orig := runnerTemplate
	defer func() { runnerTemplate = orig }()
	runnerTemplate = `package main
import("encoding/json";"log";"net/http";"os"
target "{{ .ImportPath }}")
func main(){
log.SetOutput(os.Stderr)
r:=&http.Request{}
params:=map[string]string{}
result,err:=target.HandleRequest(r,params)
if err!=nil{
log.Println("barry-error:",err)
os.Exit(1)}
json.NewEncoder(os.Stdout).Encode(result)}`

	req := httptest.NewRequest(http.MethodGet, "/", nil)

	result, err := ExecuteServerFile(serverPath, req, map[string]string{}, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val := result["ok"]; val != true {
		t.Errorf("expected ok = true, got: %v", result)
	}
}

func TestExecuteServerFile_ResolveRelativeImportFails(t *testing.T) {
	orig := findGoModRoot
	defer func() { findGoModRoot = orig }()

	tmp := t.TempDir()
	serverPath := filepath.Join(tmp, "index.server.go")
	_ = os.WriteFile(serverPath, []byte("// dummy"), 0644)

	findGoModRoot = func(_ string) (string, string, error) {
		return string([]byte{0x00}), "example.com/test", nil
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)

	_, err := ExecuteServerFile(serverPath, req, map[string]string{}, false)
	if err == nil || !strings.Contains(err.Error(), "cannot resolve relative import path") {
		t.Fatalf("expected relative import error, got: %v", err)
	}
}

func TestExecuteServerFile_FallbackOnFormatError(t *testing.T) {
	tmp := t.TempDir()
	_ = os.WriteFile(filepath.Join(tmp, "go.mod"), []byte("module example.com/fmtfail\n"), 0644)

	serverPath := filepath.Join(tmp, "index.server.go")
	_ = os.WriteFile(serverPath, []byte(`package fmtfail; import "net/http"; func HandleRequest(r *http.Request, _ map[string]string) (map[string]interface{}, error) { return map[string]interface{}{"ok": true}, nil }`), 0644)

	orig := runnerTemplate
	defer func() { runnerTemplate = orig }()

	runnerTemplate = `package main
	func main() {
		this is !not valid go
	}`

	req := httptest.NewRequest(http.MethodGet, "/", nil)

	_, err := ExecuteServerFile(serverPath, req, map[string]string{}, false)
	if err == nil || !strings.Contains(err.Error(), "exec error") {
		t.Fatalf("expected exec error due to bad formatting, got: %v", err)
	}
}

func TestExecuteAPIFile_MkdirAllFails(t *testing.T) {
	tmp := t.TempDir()
	_ = os.WriteFile(filepath.Join(tmp, "go.mod"), []byte("module example.com/testapi\n"), 0644)

	apiPath := filepath.Join(tmp, "api", "test")
	_ = os.MkdirAll(apiPath, 0755)
	apiFile := filepath.Join(apiPath, "index.go")
	_ = os.WriteFile(apiFile, []byte(`package test; import "net/http"; func HandleAPI(r *http.Request, _ map[string]string)(interface{}, error){ return nil, nil }`), 0644)

	req := httptest.NewRequest("GET", "/", nil)

	fixedTime := time.Date(2025, 7, 15, 12, 0, 0, 0, time.UTC)
	origNow := nowFunc
	nowFunc = func() time.Time { return fixedTime }
	defer func() { nowFunc = origNow }()

	absPath, _ := filepath.Abs(apiFile)
	hash := sha256.Sum256([]byte(absPath + req.Method + fixedTime.String()))
	runDir := filepath.Join(tmp, ".barry-tmp", fmt.Sprintf("%x", hash[:8]))

	_ = os.MkdirAll(filepath.Dir(runDir), 0755)
	_ = os.WriteFile(runDir, []byte("I am a file, not a dir"), 0644)

	_, err := ExecuteAPIFile(apiFile, req, nil, false)
	if err == nil || !strings.Contains(err.Error(), "could not create temp dir") {
		t.Fatalf("expected mkdir error, got: %v", err)
	}
}

func TestExecuteAPIFile_DevModeTrueTriggersMultiWriter(t *testing.T) {
	tmp := t.TempDir()
	_ = os.WriteFile(filepath.Join(tmp, "go.mod"), []byte("module example.com/devapi\n"), 0644)

	apiPath := filepath.Join(tmp, "api", "dev")
	_ = os.MkdirAll(apiPath, 0755)
	apiFile := filepath.Join(apiPath, "index.go")
	_ = os.WriteFile(apiFile, []byte(`package dev; import "net/http"; func HandleAPI(r *http.Request, p map[string]string) (interface{}, error) { return map[string]interface{}{"dev": true}, nil }`), 0644)

	req := httptest.NewRequest("GET", "/", nil)

	_, err := ExecuteAPIFile(apiFile, req, nil, true)
	if err != nil {
		t.Fatalf("unexpected error in devMode: %v", err)
	}
}

func TestExecuteAPIFile_FormatFailsButCompiles(t *testing.T) {
	tmp := t.TempDir()
	_ = os.WriteFile(filepath.Join(tmp, "go.mod"), []byte("module example.com/apiunformatted\n"), 0644)

	apiPath := filepath.Join(tmp, "api", "ugly")
	_ = os.MkdirAll(apiPath, 0755)

	apiFile := filepath.Join(apiPath, "index.go")
	_ = os.WriteFile(apiFile, []byte(`package ugly; import "net/http"; func HandleAPI(r *http.Request, p map[string]string)(interface{}, error){ return map[string]interface{}{"ugly":true}, nil }`), 0644)

	origTemplate := apiRunnerTemplate
	defer func() { apiRunnerTemplate = origTemplate }()
	apiRunnerTemplate = `package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	target "{{ .ImportPath }}"
)

func main() {
	log.SetOutput(os.Stderr)
	r, _ := http.NewRequest("{{ .Method }}", "{{ .URL }}", nil)
	params := map[string]string{}
	result, err := target.HandleAPI(r, params)
	if err != nil {
		log.Println("barry-error:", err)
		os.Exit(1)
	}
	json.NewEncoder(os.Stdout).Encode(result)
}`

	origFormat := formatSource
	formatSource = func(_ []byte) ([]byte, error) {
		return nil, fmt.Errorf("format failed!")
	}
	defer func() { formatSource = origFormat }()

	req := httptest.NewRequest("GET", "/?x=1", nil)

	result, err := ExecuteAPIFile(apiFile, req, nil, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(string(result), `"ugly":true`) {
		t.Errorf("expected result to contain 'ugly': %s", result)
	}
}

func TestExecuteAPIFile_WriteFileFails(t *testing.T) {
	tmp := t.TempDir()
	_ = os.WriteFile(filepath.Join(tmp, "go.mod"), []byte("module example.com/writefailapi\n"), 0644)

	apiPath := filepath.Join(tmp, "api", "failwrite")
	_ = os.MkdirAll(apiPath, 0755)

	apiFile := filepath.Join(apiPath, "index.go")
	_ = os.WriteFile(apiFile, []byte(`package failwrite; import "net/http"; func HandleAPI(r *http.Request, _ map[string]string)(interface{}, error){ return nil, nil }`), 0644)

	req := httptest.NewRequest("GET", "/", nil)

	fixedTime := time.Date(2025, 7, 15, 12, 0, 0, 0, time.UTC)
	origNow := nowFunc
	nowFunc = func() time.Time { return fixedTime }
	defer func() { nowFunc = origNow }()

	absPath, _ := filepath.Abs(apiFile)
	hash := sha256.Sum256([]byte(absPath + req.Method + fixedTime.String()))
	runDir := filepath.Join(tmp, ".barry-tmp", fmt.Sprintf("%x", hash[:8]))
	tmpFile := filepath.Join(runDir, "main.go")

	_ = os.MkdirAll(runDir, 0755)
	_ = os.WriteFile(tmpFile, []byte("cannot overwrite"), 0444)

	t.Cleanup(func() { _ = os.Chmod(runDir, 0755) })

	_, err := ExecuteAPIFile(apiFile, req, nil, false)
	if err == nil || !strings.Contains(err.Error(), "could not write temp file") {
		t.Fatalf("expected write file error, got: %v", err)
	}
}
