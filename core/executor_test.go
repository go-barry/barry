package core

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
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

func Test_jsonMarshalFunc_string_and_slice(t *testing.T) {
	jsonFunc, ok := templateFuncs["jsonMarshal"].(func(interface{}) string)
	if !ok {
		t.Fatal("jsonMarshal function not found or has wrong type")
	}

	str := jsonFunc("hello")
	if str != strconv.Quote("hello") {
		t.Errorf("expected quoted string, got %s", str)
	}

	slice := jsonFunc([]string{"a", "b"})
	expected := `[]string{"a", "b"}`
	if slice != expected {
		t.Errorf("expected %s, got %s", expected, slice)
	}

	obj := jsonFunc(map[string]int{"count": 42})
	if !strings.Contains(obj, `"count":42`) {
		t.Errorf("expected JSON string, got %s", obj)
	}
}

func TestExecuteAPIFileWithSubprocess_JSONEncode(t *testing.T) {
	original := ExecuteServerFileWithSubprocessFunc
	defer func() { ExecuteServerFileWithSubprocessFunc = original }()

	ExecuteServerFileWithSubprocessFunc = func(fp string, r *http.Request, p map[string]string) (map[string]interface{}, error) {
		return map[string]interface{}{"status": "ok"}, nil
	}

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	params := map[string]string{"foo": "bar"}

	resp, err := ExecuteAPIFileWithSubprocess("fake.go", req, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(string(resp), "ok") {
		t.Errorf("expected response to contain 'ok', got %s", string(resp))
	}
}

func TestExecuteServerFileWithSubprocess_missingGoMod(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	_, err := ExecuteServerFileWithSubprocess("/nonexistent/path/to/server.go", req, nil)
	if err == nil || !strings.Contains(err.Error(), "could not resolve go.mod") {
		t.Errorf("expected go.mod error, got %v", err)
	}
}

func TestExecuteServerFileWithSubprocess_invalidImportPath(t *testing.T) {
	tmpDir := t.TempDir()
	goModPath := filepath.Join(tmpDir, "go.mod")
	os.WriteFile(goModPath, []byte("module example.com/testmod\n"), 0644)

	badPath := filepath.Join(tmpDir, "nested", "handler.go")
	os.MkdirAll(filepath.Dir(badPath), 0755)
	os.WriteFile(badPath, []byte("package main"), 0644)

	req := httptest.NewRequest(http.MethodPost, "/some/path", strings.NewReader("body=data"))
	_, err := ExecuteServerFileWithSubprocess(badPath, req, map[string]string{"id": "123"})

	if err == nil {
		t.Errorf("expected error due to 'go run' failure, got nil")
	}
}

func TestExecuteServerFile_pluginReturnsResult(t *testing.T) {
	original := LoadPluginAndCallFunc
	defer func() { LoadPluginAndCallFunc = original }()

	LoadPluginAndCallFunc = func(fp string, r *http.Request, p map[string]string) (map[string]interface{}, error) {
		return map[string]interface{}{"plugin": "value"}, nil
	}

	req := httptest.NewRequest(http.MethodGet, "/plugin", nil)
	result, err := ExecuteServerFile("dummy.go", req, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result["plugin"] != "value" {
		t.Errorf("expected plugin result, got %v", result)
	}
}

func TestExecuteServerFile_pluginReturnsError(t *testing.T) {
	original := LoadPluginAndCallFunc
	defer func() { LoadPluginAndCallFunc = original }()

	LoadPluginAndCallFunc = func(fp string, r *http.Request, p map[string]string) (map[string]interface{}, error) {
		return nil, errors.New("plugin failed")
	}

	req := httptest.NewRequest(http.MethodGet, "/plugin", nil)
	_, err := ExecuteServerFile("dummy.go", req, nil)
	if err == nil || !strings.Contains(err.Error(), "plugin failed") {
		t.Errorf("expected plugin failure, got %v", err)
	}
}

func TestExecuteServerFile_pluginNotFound_fallbackToSubprocess(t *testing.T) {
	originalPlugin := LoadPluginAndCallFunc
	originalSub := ExecuteServerFileWithSubprocessFunc
	defer func() {
		LoadPluginAndCallFunc = originalPlugin
		ExecuteServerFileWithSubprocessFunc = originalSub
	}()

	LoadPluginAndCallFunc = func(fp string, r *http.Request, p map[string]string) (map[string]interface{}, error) {
		return nil, ErrPluginNotFound
	}

	ExecuteServerFileWithSubprocessFunc = func(fp string, r *http.Request, p map[string]string) (map[string]interface{}, error) {
		return map[string]interface{}{"fallback": true}, nil
	}

	req := httptest.NewRequest(http.MethodGet, "/fallback", nil)
	result, err := ExecuteServerFile("dummy.go", req, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result["fallback"] != true {
		t.Errorf("expected fallback result, got %v", result)
	}
}

func TestExecuteAPIFile_pluginReturnsResult(t *testing.T) {
	original := LoadPluginAndCallFunc
	defer func() { LoadPluginAndCallFunc = original }()

	LoadPluginAndCallFunc = func(fp string, r *http.Request, p map[string]string) (map[string]interface{}, error) {
		return map[string]interface{}{"hello": "world"}, nil
	}

	req := httptest.NewRequest(http.MethodGet, "/api", nil)
	out, err := ExecuteAPIFile("dummy.go", req, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(string(out), "world") {
		t.Errorf("expected json with world, got %s", string(out))
	}
}

func TestExecuteAPIFile_pluginError(t *testing.T) {
	original := LoadPluginAndCallFunc
	defer func() { LoadPluginAndCallFunc = original }()

	LoadPluginAndCallFunc = func(fp string, r *http.Request, p map[string]string) (map[string]interface{}, error) {
		return nil, errors.New("broken")
	}

	req := httptest.NewRequest(http.MethodGet, "/api", nil)
	_, err := ExecuteAPIFile("dummy.go", req, nil)
	if err == nil || !strings.Contains(err.Error(), "broken") {
		t.Errorf("expected error, got %v", err)
	}
}

func TestExecuteAPIFile_pluginNotFound_fallback(t *testing.T) {
	originalPlugin := LoadPluginAndCallFunc
	originalSub := ExecuteAPIFileWithSubprocessFunc
	defer func() {
		LoadPluginAndCallFunc = originalPlugin
		ExecuteAPIFileWithSubprocessFunc = originalSub
	}()

	LoadPluginAndCallFunc = func(fp string, r *http.Request, p map[string]string) (map[string]interface{}, error) {
		return nil, ErrPluginNotFound
	}

	ExecuteAPIFileWithSubprocessFunc = func(fp string, r *http.Request, p map[string]string) ([]byte, error) {
		return []byte(`{"fallback":true}`), nil
	}

	req := httptest.NewRequest(http.MethodGet, "/fallback", nil)
	out, err := ExecuteAPIFile("dummy.go", req, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(string(out), `"fallback":true`) {
		t.Errorf("expected fallback JSON, got %s", string(out))
	}
}

func TestExecuteServerFileWithSubprocess_filepathRelFails(t *testing.T) {
	originalFindMod := findGoModRoot
	originalRel := filepathRelFunc
	defer func() {
		findGoModRoot = originalFindMod
		filepathRelFunc = originalRel
	}()

	findGoModRoot = func(startPath string) (string, string, error) {
		return "/any/path", "example.com/test", nil
	}

	filepathRelFunc = func(basepath, targpath string) (string, error) {
		return "", errors.New("rel failed")
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	_, err := ExecuteServerFileWithSubprocess("/tmp/index.server.go", req, nil)
	if err == nil || !strings.Contains(err.Error(), "cannot resolve relative import path") {
		t.Errorf("expected relative path error, got %v", err)
	}
}

func TestExecuteServerFileWithSubprocess_templateExecuteError(t *testing.T) {
	originalFindMod := findGoModRoot
	originalTemplate := runnerTemplate
	defer func() {
		findGoModRoot = originalFindMod
		runnerTemplate = originalTemplate
	}()

	tmp := t.TempDir()
	_ = os.WriteFile(filepath.Join(tmp, "go.mod"), []byte("module example.com/badtemplate\n"), 0644)

	dir := filepath.Join(tmp, "routes", "errtemplate")
	_ = os.MkdirAll(dir, 0755)

	serverPath := filepath.Join(dir, "index.server.go")
	_ = os.WriteFile(serverPath, []byte(`package errtemplate; import "net/http"; func HandleRequest(r *http.Request, _ map[string]string)(map[string]interface{}, error){ return map[string]interface{}{"ok":true}, nil }`), 0644)

	findGoModRoot = func(startPath string) (string, string, error) {
		return tmp, "example.com/badtemplate", nil
	}

	runnerTemplate = `{{ .DoesNotExist.Method }}`

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	_, err := ExecuteServerFileWithSubprocess(serverPath, req, nil)

	if err == nil || !strings.Contains(err.Error(), "template execution error") {
		t.Errorf("expected template execution error, got %v", err)
	}
}

func TestExecuteServerFileWithSubprocess_formatSourceFails(t *testing.T) {
	originalFindMod := findGoModRoot
	originalTemplate := runnerTemplate
	defer func() {
		findGoModRoot = originalFindMod
		runnerTemplate = originalTemplate
	}()

	tmp := t.TempDir()
	_ = os.WriteFile(filepath.Join(tmp, "go.mod"), []byte("module example.com/badformat\n"), 0644)

	dir := filepath.Join(tmp, "routes", "badformat")
	_ = os.MkdirAll(dir, 0755)

	serverPath := filepath.Join(dir, "index.server.go")
	_ = os.WriteFile(serverPath, []byte(`package badformat; import "net/http"; func HandleRequest(r *http.Request, _ map[string]string)(map[string]interface{}, error){ return map[string]interface{}{"ok":true}, nil }`), 0644)

	findGoModRoot = func(startPath string) (string, string, error) {
		return tmp, "example.com/badformat", nil
	}

	runnerTemplate = `package main func BAD {}`

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	_, err := ExecuteServerFileWithSubprocess(serverPath, req, nil)

	if err == nil || !strings.Contains(err.Error(), "exec error") {
		t.Errorf("expected subprocess exec error due to unformatted code, got: %v", err)
	}
}

func TestExecuteServerFileWithSubprocess_mkdirFails(t *testing.T) {
	originalMod := findGoModRoot
	originalMkdir := osMkdirAll
	defer func() {
		findGoModRoot = originalMod
		osMkdirAll = originalMkdir
	}()

	tmp := t.TempDir()
	_ = os.WriteFile(filepath.Join(tmp, "go.mod"), []byte("module example.com/mkdirfail\n"), 0644)
	dir := filepath.Join(tmp, "routes", "failmkdir")
	_ = os.MkdirAll(dir, 0755)
	serverPath := filepath.Join(dir, "index.server.go")
	_ = os.WriteFile(serverPath, []byte(`package failmkdir; import "net/http"; func HandleRequest(r *http.Request, _ map[string]string)(map[string]interface{}, error){ return nil, nil }`), 0644)

	findGoModRoot = func(startPath string) (string, string, error) {
		return tmp, "example.com/mkdirfail", nil
	}

	osMkdirAll = func(path string, perm os.FileMode) error {
		return errors.New("mkdir blocked")
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	_, err := ExecuteServerFileWithSubprocess(serverPath, req, nil)
	if err == nil || !strings.Contains(err.Error(), "could not create temp dir") {
		t.Errorf("expected mkdir error, got: %v", err)
	}
}

func TestExecuteServerFileWithSubprocess_writeFileFails(t *testing.T) {
	originalMod := findGoModRoot
	originalWrite := osWriteFile
	defer func() {
		findGoModRoot = originalMod
		osWriteFile = originalWrite
	}()

	tmp := t.TempDir()
	_ = os.WriteFile(filepath.Join(tmp, "go.mod"), []byte("module example.com/writefail\n"), 0644)
	dir := filepath.Join(tmp, "routes", "failwrite")
	_ = os.MkdirAll(dir, 0755)
	serverPath := filepath.Join(dir, "index.server.go")
	_ = os.WriteFile(serverPath, []byte(`package failwrite; import "net/http"; func HandleRequest(r *http.Request, _ map[string]string)(map[string]interface{}, error){ return nil, nil }`), 0644)

	findGoModRoot = func(startPath string) (string, string, error) {
		return tmp, "example.com/writefail", nil
	}

	osWriteFile = func(name string, data []byte, perm os.FileMode) error {
		if strings.HasSuffix(name, "main.go") {
			return errors.New("write blocked")
		}
		return os.WriteFile(name, data, perm)
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	_, err := ExecuteServerFileWithSubprocess(serverPath, req, nil)
	if err == nil || !strings.Contains(err.Error(), "could not write temp file") {
		t.Errorf("expected write error, got: %v", err)
	}
}

func TestExecuteServerFileWithSubprocess_ExecReturnsNotFoundError(t *testing.T) {
	originalMod := findGoModRoot
	defer func() { findGoModRoot = originalMod }()

	tmp := t.TempDir()
	_ = os.WriteFile(filepath.Join(tmp, "go.mod"), []byte("module example.com/errnotfound\n"), 0644)
	dir := filepath.Join(tmp, "routes", "notfound")
	_ = os.MkdirAll(dir, 0755)

	goFile := filepath.Join(dir, "index.server.go")
	_ = os.WriteFile(goFile, []byte(`package notfound; import "net/http"; func HandleRequest(r *http.Request, _ map[string]string)(map[string]interface{}, error){ return nil, nil }`), 0644)

	findGoModRoot = func(startPath string) (string, string, error) {
		return tmp, "example.com/errnotfound", nil
	}

	runnerTemplate = `package main
import (
	"log"
	"os"
)
func main() {
	log.SetOutput(os.Stderr)
	log.Println("barry-error: barry: not found")
	os.Exit(1)
}`

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	_, err := ExecuteServerFileWithSubprocess(goFile, req, nil)

	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got: %v", err)
	}
}

func TestExecuteServerFileWithSubprocess_JSONDecodeError(t *testing.T) {
	originalMod := findGoModRoot
	defer func() { findGoModRoot = originalMod }()

	tmp := t.TempDir()
	_ = os.WriteFile(filepath.Join(tmp, "go.mod"), []byte("module example.com/badjson\n"), 0644)
	dir := filepath.Join(tmp, "routes", "badjson")
	_ = os.MkdirAll(dir, 0755)

	goFile := filepath.Join(dir, "index.server.go")
	_ = os.WriteFile(goFile, []byte(`package badjson; import "net/http"; func HandleRequest(r *http.Request, _ map[string]string)(map[string]interface{}, error){ return nil, nil }`), 0644)

	findGoModRoot = func(startPath string) (string, string, error) {
		return tmp, "example.com/badjson", nil
	}

	runnerTemplate = `package main
import (
	"fmt"
)
func main() {
	fmt.Print("this is not json")
}`

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	_, err := ExecuteServerFileWithSubprocess(goFile, req, nil)

	if err == nil || !strings.Contains(err.Error(), "json decode error") {
		t.Errorf("expected json decode error, got: %v", err)
	}
}

func TestExecuteServerFileWithSubprocess_Success(t *testing.T) {
	originalMod := findGoModRoot
	defer func() { findGoModRoot = originalMod }()
	runnerTemplate = defaultRunnerTemplate

	tmp := t.TempDir()

	_ = os.WriteFile(filepath.Join(tmp, "go.mod"), []byte("module example.com/success\n"), 0644)

	dir := filepath.Join(tmp, "routes", "ok")
	_ = os.MkdirAll(dir, 0755)

	goFile := filepath.Join(dir, "index.server.go")
	code := `
package ok

import "net/http"

func HandleRequest(r *http.Request, _ map[string]string) (map[string]interface{}, error) {
	return map[string]interface{}{"msg": "success"}, nil
}
`
	_ = os.WriteFile(goFile, []byte(code), 0644)

	findGoModRoot = func(startPath string) (string, string, error) {
		return tmp, "example.com/success", nil
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	result, err := ExecuteServerFileWithSubprocess(goFile, req, nil)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result["msg"] != "success" {
		t.Errorf("expected result[msg] to be 'success', got %v", result["msg"])
	}
}

func TestExecuteAPIFileWithSubprocess_SubprocessFails(t *testing.T) {
	original := ExecuteServerFileWithSubprocessFunc
	defer func() { ExecuteServerFileWithSubprocessFunc = original }()

	expectedErr := errors.New("subprocess failed")

	ExecuteServerFileWithSubprocessFunc = func(fp string, r *http.Request, p map[string]string) (map[string]interface{}, error) {
		return nil, expectedErr
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	_, err := ExecuteAPIFileWithSubprocess("fake.go", req, nil)

	if !errors.Is(err, expectedErr) {
		t.Errorf("expected subprocess error to be returned, got: %v", err)
	}
}

func TestFindGoModRoot_ReadFails(t *testing.T) {
	original := osReadFile
	defer func() { osReadFile = original }()

	tmp := t.TempDir()
	goModPath := filepath.Join(tmp, "go.mod")

	_ = os.WriteFile(goModPath, []byte("module test"), 0644)

	osReadFile = func(path string) ([]byte, error) {
		if strings.HasSuffix(path, "go.mod") {
			return nil, errors.New("simulated read failure")
		}
		return original(path)
	}

	file := filepath.Join(tmp, "nested", "main.go")
	_ = os.MkdirAll(filepath.Dir(file), 0755)
	_ = os.WriteFile(file, []byte("// dummy"), 0644)

	_, _, err := findGoModRoot(file)
	if err == nil || !strings.Contains(err.Error(), "failed to read go.mod") {
		t.Errorf("expected read failure error, got %v", err)
	}
}
