package core

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"go/format"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"
	"time"

	json "github.com/segmentio/encoding/json"
)

var nowFunc = time.Now
var formatSource = format.Source

var (
	runnerTemplate = `package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	target "{{ .ImportPath }}"
)

func main() {
	log.SetOutput(os.Stderr)

	r := &http.Request{}
	params := map[string]string{
		{{- range $k, $v := .Params }}
		"{{ $k }}": "{{ $v }}",
		{{- end }}
	}

	result, err := target.HandleRequest(r, params)
	if err != nil {
		log.Println("barry-error:", err)
		os.Exit(1)
	}

	json.NewEncoder(os.Stdout).Encode(result)
}
`

	barryTmpDir      = ".barry-tmp"
	errorPrefix      = "barry-error:"
	errorNotFoundMsg = "barry-error: barry: not found"
)

var apiRunnerTemplate = `package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	target "{{ .ImportPath }}"
)

func main() {
	log.SetOutput(os.Stderr)

	r, _ := http.NewRequest("{{ .Method }}", "/?{{ .QueryString }}", nil)
	params := map[string]string{
		{{- range $k, $v := .Params }}
		"{{ $k }}": "{{ $v }}",
		{{- end }}
	}

	result, err := target.HandleAPI(r, params)
	if err != nil {
		log.Println("barry-error:", err)
		os.Exit(1)
	}

	json.NewEncoder(os.Stdout).Encode(result)
}
`

type ExecContext struct {
	ImportPath string
	Params     map[string]string
}

var ExecuteServerFile = func(filePath string, params map[string]string, devMode bool) (map[string]interface{}, error) {
	return ExecuteServerFileWithTime(filePath, params, devMode, time.Now)
}

func ExecuteServerFileWithTime(filePath string, params map[string]string, devMode bool, now func() time.Time) (map[string]interface{}, error) {
	absPath, _ := filepath.Abs(filePath)

	modRoot, moduleName, err := findGoModRoot(absPath)
	if err != nil {
		return nil, fmt.Errorf("could not resolve go.mod: %w", err)
	}

	relPath, err := filepath.Rel(modRoot, filepath.Dir(absPath))
	if err != nil {
		return nil, fmt.Errorf("cannot resolve relative import path: %w", err)
	}

	importPath := filepath.ToSlash(filepath.Join(moduleName, relPath))

	ctx := ExecContext{
		ImportPath: importPath,
		Params:     params,
	}

	var buf bytes.Buffer
	tmpl := template.Must(template.New("runner").Parse(runnerTemplate))
	if err := tmpl.Execute(&buf, ctx); err != nil {
		return nil, fmt.Errorf("template execution error: %w", err)
	}

	formatted, err := formatSource(buf.Bytes())
	if err != nil {
		formatted = buf.Bytes()
	}

	tmpRoot := filepath.Join(modRoot, barryTmpDir)

	hash := sha256.Sum256([]byte(absPath + now().String()))
	runDir := filepath.Join(tmpRoot, fmt.Sprintf("%x", hash[:8]))
	if err := os.MkdirAll(runDir, os.ModePerm); err != nil {
		return nil, fmt.Errorf("could not create temp dir: %w", err)
	}

	tmpFile := filepath.Join(runDir, "main.go")
	if err := os.WriteFile(tmpFile, formatted, 0644); err != nil {
		return nil, fmt.Errorf("could not write temp file: %w", err)
	}

	cmd := exec.Command("go", "run", tmpFile)
	cmd.Dir = modRoot

	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	if devMode {
		cmd.Stderr = io.MultiWriter(os.Stderr, &errBuf)
	} else {
		cmd.Stderr = &errBuf
	}

	err = cmd.Run()

	_ = os.RemoveAll(runDir)

	if err != nil {
		errText := errBuf.String()
		if strings.Contains(errText, errorNotFoundMsg) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("exec error: %v\nstderr: %s", err, errText)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(outBuf.Bytes(), &result); err != nil {
		return nil, fmt.Errorf("json decode error: %w", err)
	}

	return result, nil
}

var ExecuteAPIFile = func(filePath string, req *http.Request, params map[string]string, devMode bool) ([]byte, error) {
	absPath, _ := filepath.Abs(filePath)

	modRoot, moduleName, err := findGoModRoot(absPath)
	if err != nil {
		return nil, fmt.Errorf("could not resolve go.mod: %w", err)
	}

	relPath, err := filepath.Rel(modRoot, filepath.Dir(absPath))
	if err != nil {
		return nil, fmt.Errorf("cannot resolve relative import path: %w", err)
	}

	importPath := filepath.ToSlash(filepath.Join(moduleName, relPath))

	ctx := struct {
		ImportPath  string
		Params      map[string]string
		Method      string
		QueryString string
	}{
		ImportPath:  importPath,
		Params:      params,
		Method:      req.Method,
		QueryString: req.URL.RawQuery,
	}

	var buf bytes.Buffer
	tmpl := template.Must(template.New("api-runner").Parse(apiRunnerTemplate))
	if err := tmpl.Execute(&buf, ctx); err != nil {
		return nil, fmt.Errorf("template execution error: %w", err)
	}

	formatted, err := formatSource(buf.Bytes())
	if err != nil {
		formatted = buf.Bytes()
	}

	tmpRoot := filepath.Join(modRoot, barryTmpDir)
	hash := sha256.Sum256([]byte(absPath + req.Method + nowFunc().String()))
	runDir := filepath.Join(tmpRoot, fmt.Sprintf("%x", hash[:8]))

	if err := os.MkdirAll(runDir, os.ModePerm); err != nil {
		return nil, fmt.Errorf("could not create temp dir: %w", err)
	}

	tmpFile := filepath.Join(runDir, "main.go")
	if err := os.WriteFile(tmpFile, formatted, 0644); err != nil {
		return nil, fmt.Errorf("could not write temp file: %w", err)
	}

	cmd := exec.Command("go", "run", tmpFile)
	cmd.Dir = modRoot

	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	if devMode {
		cmd.Stderr = io.MultiWriter(os.Stderr, &errBuf)
	} else {
		cmd.Stderr = &errBuf
	}

	err = cmd.Run()
	_ = os.RemoveAll(runDir)

	if err != nil {
		errText := errBuf.String()
		if strings.Contains(errText, errorNotFoundMsg) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("exec error: %v\nstderr: %s", err, errText)
	}

	return outBuf.Bytes(), nil
}

var findGoModRoot = func(startPath string) (string, string, error) {
	dir := filepath.Dir(startPath)
	for {
		modPath := filepath.Join(dir, "go.mod")
		if _, err := os.Stat(modPath); err == nil {
			data, err := os.ReadFile(modPath)
			if err != nil {
				return "", "", fmt.Errorf("failed to read go.mod: %w", err)
			}
			lines := strings.Split(string(data), "\n")
			for _, line := range lines {
				if strings.HasPrefix(line, "module ") {
					module := strings.TrimSpace(strings.TrimPrefix(line, "module "))
					return dir, module, nil
				}
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", "", fmt.Errorf("go.mod not found starting from %s", startPath)
}
