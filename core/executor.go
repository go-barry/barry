package core

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"text/template"
)

type ExecContext struct {
	UserCode string
	Params   map[string]string
}

const runnerTemplate = `package main

import (
	"encoding/json"
	"net/http"
	"os"
	"fmt"
)

{{ .UserCode }}

func main() {
	r := &http.Request{}
	params := map[string]string{
		{{- range $k, $v := .Params }}
		"{{ $k }}": "{{ $v }}",
		{{- end }}
	}

	result, err := HandleRequest(r, params)
	if err != nil {
		fmt.Fprintln(os.Stderr, "barry-error:", err)
		os.Exit(1)
	}

	json.NewEncoder(os.Stdout).Encode(result)
}
`

func ExecuteServerFile(filePath string, params map[string]string) (map[string]interface{}, error) {
	cleanCode, err := extractFunctionsFromFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to extract functions: %v", err)
	}

	ctx := ExecContext{
		UserCode: cleanCode,
		Params:   params,
	}

	var buf bytes.Buffer
	tmpl := template.Must(template.New("runner").Parse(runnerTemplate))
	tmpl.Execute(&buf, ctx)

	tmpDir, err := os.MkdirTemp("", "barry_exec")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmpDir)

	tmpFile := filepath.Join(tmpDir, "main.go")
	os.WriteFile(tmpFile, buf.Bytes(), 0644)

	cmd := exec.Command("go", "run", tmpFile)
	var out, stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr

	err = cmd.Run()
	if err != nil {
		return nil, fmt.Errorf("exec error: %v\nstderr: %s", err, stderr.String())
	}

	var result map[string]interface{}
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		return nil, fmt.Errorf("json decode error: %v", err)
	}

	return result, nil
}

func extractFunctionsFromFile(filePath string) (string, error) {
	fs := token.NewFileSet()

	node, err := parser.ParseFile(fs, filePath, nil, parser.ParseComments)
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer

	for _, decl := range node.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok {
			err := printer.Fprint(&buf, fs, fn)
			if err != nil {
				return "", err
			}
			buf.WriteString("\n\n")
		}
	}

	return buf.String(), nil
}
