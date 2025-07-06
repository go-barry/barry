package core

import (
	"os"
	"path/filepath"
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

func TestExecuteServerFile_BasicSuccess(t *testing.T) {
	tmp := t.TempDir()

	// Set up go.mod in root of tmp
	goMod := []byte("module example.com/barrytest\n\ngo 1.20\n")
	if err := os.WriteFile(filepath.Join(tmp, "go.mod"), goMod, 0644); err != nil {
		t.Fatal(err)
	}

	// Set up nested route dir
	routeDir := filepath.Join(tmp, "routes", "foo")
	if err := os.MkdirAll(routeDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Write valid Go server file
	serverCode := `package foo

import "net/http"

func HandleRequest(r *http.Request, params map[string]string) (map[string]interface{}, error) {
	return map[string]interface{}{
		"foo": "bar",
	}, nil
}
`
	serverPath := filepath.Join(routeDir, "index.server.go")
	if err := os.WriteFile(serverPath, []byte(serverCode), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := ExecuteServerFile(serverPath, map[string]string{}, false)
	if err != nil {
		t.Fatalf("ExecuteServerFile failed: %v", err)
	}

	if val, ok := result["foo"]; !ok || val != "bar" {
		t.Errorf("expected result[\"foo\"] = \"bar\", got: %+v", result)
	}
}

func TestExecuteServerFile_NotFoundError(t *testing.T) {
	tmp := t.TempDir()

	goMod := []byte("module example.com/barrytest\n\ngo 1.20\n")
	_ = os.WriteFile(filepath.Join(tmp, "go.mod"), goMod, 0644)

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

	_, err := ExecuteServerFile(serverPath, map[string]string{}, false)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !IsNotFoundError(err) {
		t.Errorf("expected ErrNotFound, got: %v", err)
	}
}
