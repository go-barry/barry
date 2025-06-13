package core

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go/format"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"
)

type ExecContext struct {
	ImportPath string
	Params     map[string]string
}

const runnerTemplate = `package main

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

func ExecuteServerFile(filePath string, params map[string]string, devMode bool) (map[string]interface{}, error) {
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
	tmpl.Execute(&buf, ctx)

	formatted, err := format.Source(buf.Bytes())
	if err != nil {
		formatted = buf.Bytes()
	}

	tmpDir := filepath.Join(modRoot, ".barry-tmp")
	os.MkdirAll(tmpDir, os.ModePerm)

	tmpFile := filepath.Join(tmpDir, "main.go")
	os.WriteFile(tmpFile, formatted, 0644)

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
	if err != nil {
		errText := errBuf.String()
		if strings.Contains(errText, "barry-error: barry: not found") {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("exec error: %v\nstderr: %s", err, errText)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(outBuf.Bytes(), &result); err != nil {
		return nil, fmt.Errorf("json decode error: %v", err)
	}

	return result, nil
}

func findGoModRoot(startPath string) (string, string, error) {
	dir := filepath.Dir(startPath)
	for {
		modPath := filepath.Join(dir, "go.mod")
		if _, err := os.Stat(modPath); err == nil {
			data, _ := os.ReadFile(modPath)
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
