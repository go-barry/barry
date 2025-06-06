package core

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go/format"
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
	"fmt"
	"net/http"
	"os"
	target "{{ .ImportPath }}"
)

func main() {
	r := &http.Request{}
	params := map[string]string{
		{{- range $k, $v := .Params }}
		"{{ $k }}": "{{ $v }}",
		{{- end }}
	}

	result, err := target.HandleRequest(r, params)
	if err != nil {
		fmt.Fprintln(os.Stderr, "barry-error:", err)
		os.Exit(1)
	}

	json.NewEncoder(os.Stdout).Encode(result)
}
`

func ExecuteServerFile(filePath string, params map[string]string) (map[string]interface{}, error) {
	modRoot, moduleName, err := findGoModRoot(filePath)
	if err != nil {
		return nil, fmt.Errorf("could not resolve go.mod: %w", err)
	}

	relPath, err := filepath.Rel(modRoot, filepath.Dir(filePath))
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

	var out, stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr

	err = cmd.Run()
	if err != nil {
		return nil, fmt.Errorf("exec error: %v stderr: %s", err, stderr.String())
	}

	var result map[string]interface{}
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
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
