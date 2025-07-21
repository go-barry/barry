package cli

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestGetGoModuleName_Success(t *testing.T) {
	tmp := t.TempDir()
	goModPath := filepath.Join(tmp, "go.mod")
	err := os.WriteFile(goModPath, []byte("module github.com/my/module\n"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	os.Chdir(tmp)

	module, err := getGoModuleName()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if module != "github.com/my/module" {
		t.Errorf("expected module name, got %s", module)
	}
}

func TestGetGoModuleName_Missing(t *testing.T) {
	tmp := t.TempDir()
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	os.Chdir(tmp)

	_, err := getGoModuleName()
	if err == nil || !strings.Contains(err.Error(), "no such file or directory") {
		t.Errorf("expected file not found error, got %v", err)
	}
}

func TestGetGoModuleName_NoModuleLine(t *testing.T) {
	tmp := t.TempDir()
	_ = os.WriteFile(filepath.Join(tmp, "go.mod"), []byte("// no module line here"), 0644)

	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	os.Chdir(tmp)

	_, err := getGoModuleName()
	if err == nil || !strings.Contains(err.Error(), "module path not found") {
		t.Errorf("expected error about missing module path, got %v", err)
	}
}

func TestBuildCommand_WriteFileFails(t *testing.T) {
	originalWrite := osWriteFileFunc
	defer func() { osWriteFileFunc = originalWrite }()

	osWriteFileFunc = func(path string, data []byte, perm os.FileMode) error {
		return errors.New("mock write failure")
	}

	tmp := t.TempDir()
	_ = os.WriteFile(filepath.Join(tmp, "go.mod"), []byte("module github.com/test/build\n"), 0644)

	routeDir := filepath.Join(tmp, "routes", "demo")
	_ = os.MkdirAll(routeDir, 0755)
	_ = os.WriteFile(filepath.Join(routeDir, "index.server.go"), []byte("// dummy"), 0644)

	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	os.Chdir(tmp)

	err := BuildCommand.Action(nil)
	if err == nil || !strings.Contains(err.Error(), "failed to write wrapper") {
		t.Errorf("expected wrapper write error, got %v", err)
	}
}

func TestBuildCommand_CommandFails(t *testing.T) {
	originalExec := execCommand
	defer func() { execCommand = originalExec }()

	execCommand = func(name string, args ...string) *exec.Cmd {
		return exec.Command("false")
	}

	tmp := t.TempDir()
	_ = os.WriteFile(filepath.Join(tmp, "go.mod"), []byte("module github.com/test/execfail\n"), 0644)
	routePath := filepath.Join(tmp, "routes", "b")
	_ = os.MkdirAll(routePath, 0755)
	_ = os.WriteFile(filepath.Join(routePath, "index.server.go"), []byte("// dummy"), 0644)

	oldWD, _ := os.Getwd()
	defer os.Chdir(oldWD)
	_ = os.Chdir(tmp)

	err := BuildCommand.Action(nil)
	if err == nil || !strings.Contains(err.Error(), "failed to build plugin") {
		t.Errorf("expected build error, got %v", err)
	}
}

func TestBuildCommand_MkdirFails(t *testing.T) {
	originalMkdir := osMkdirAllFunc
	defer func() { osMkdirAllFunc = originalMkdir }()

	osMkdirAllFunc = func(path string, perm os.FileMode) error {
		return errors.New("mock mkdir fail")
	}

	tmp := t.TempDir()
	_ = os.WriteFile(filepath.Join(tmp, "go.mod"), []byte("module github.com/test/mkdirfail\n"), 0644)
	routePath := filepath.Join(tmp, "routes", "c")
	_ = os.MkdirAll(routePath, 0755)
	_ = os.WriteFile(filepath.Join(routePath, "index.server.go"), []byte("// dummy"), 0644)

	oldWD, _ := os.Getwd()
	defer os.Chdir(oldWD)
	_ = os.Chdir(tmp)

	err := BuildCommand.Action(nil)
	if err == nil || !strings.Contains(err.Error(), "failed to create wrapper directory") {
		t.Errorf("expected mkdir fail error, got: %v", err)
	}
}

func TestBuildCommand_Success(t *testing.T) {
	tmp := t.TempDir()
	modPath := filepath.Join(tmp, "go.mod")
	_ = os.WriteFile(modPath, []byte("module github.com/test/success\n"), 0644)

	routeDir := filepath.Join(tmp, "routes", "demo")
	_ = os.MkdirAll(routeDir, 0755)
	_ = os.WriteFile(filepath.Join(routeDir, "index.server.go"), []byte("// dummy"), 0644)

	originalWrite := osWriteFileFunc
	originalMkdir := osMkdirAllFunc
	originalExec := buildExecCommand
	defer func() {
		osWriteFileFunc = originalWrite
		osMkdirAllFunc = originalMkdir
		buildExecCommand = originalExec
	}()

	var capturedCmdArgs []string
	osWriteFileFunc = func(path string, data []byte, perm os.FileMode) error {
		return nil
	}
	osMkdirAllFunc = func(path string, perm os.FileMode) error {
		return nil
	}
	buildExecCommand = func(name string, args ...string) *exec.Cmd {
		capturedCmdArgs = append([]string{name}, args...)
		return exec.Command("true")
	}

	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	os.Chdir(tmp)

	err := BuildCommand.Action(nil)
	if err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
	if len(capturedCmdArgs) == 0 {
		t.Error("expected build command to run")
	}
}

func TestBuildCommand_ModuleNameFails(t *testing.T) {
	tmp := t.TempDir()

	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	_ = os.Chdir(tmp)

	err := BuildCommand.Action(nil)
	if err == nil || !strings.Contains(err.Error(), "failed to determine module name from go.mod") {
		t.Errorf("expected module name error, got: %v", err)
	}
}

func TestBuildCommand_SkipsNonIndexServerGoFiles(t *testing.T) {
	tmp := t.TempDir()
	_ = os.WriteFile(filepath.Join(tmp, "go.mod"), []byte("module github.com/test/skip\n"), 0644)

	routeDir := filepath.Join(tmp, "routes", "other")
	_ = os.MkdirAll(routeDir, 0755)
	_ = os.WriteFile(filepath.Join(routeDir, "not-index.server.go"), []byte("// dummy"), 0644)

	originalWrite := osWriteFileFunc
	originalMkdir := osMkdirAllFunc
	originalExec := buildExecCommand
	defer func() {
		osWriteFileFunc = originalWrite
		osMkdirAllFunc = originalMkdir
		buildExecCommand = originalExec
	}()

	osWriteFileFunc = func(path string, data []byte, perm os.FileMode) error {
		t.Errorf("os.WriteFile should not be called")
		return nil
	}
	osMkdirAllFunc = func(path string, perm os.FileMode) error {
		t.Errorf("os.MkdirAll should not be called")
		return nil
	}
	buildExecCommand = func(name string, args ...string) *exec.Cmd {
		t.Errorf("exec.Command should not be called")
		return exec.Command("true")
	}

	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	_ = os.Chdir(tmp)

	err := BuildCommand.Action(nil)
	if err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
}
