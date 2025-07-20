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
	"strconv"
	"strings"
	"text/template"
	"time"

	json "github.com/segmentio/encoding/json"
)

var nowFunc = time.Now
var formatSource = format.Source

var barryTmpDir = ".barry-tmp"
var errorNotFoundMsg = "barry-error: barry: not found"

var runnerTemplate = `package main

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"os"
	target "{{ .ImportPath }}"
)

func main() {
	log.SetOutput(os.Stderr)

	body := strings.NewReader({{ .Body | jsonMarshal }})
	r, _ := http.NewRequest("{{ .Method }}", "{{ .URL }}", body)

	for key, vals := range map[string][]string{
		{{- range $k, $v := .Headers }}
		"{{ $k }}": {{ $v | jsonMarshal }},
		{{- end }}
	} {
		for _, v := range vals {
			r.Header.Add(key, v)
		}
	}

	r.Host = "{{ .Host }}"
	r.RemoteAddr = "{{ .RemoteAddr }}"

	if err := r.ParseForm(); err != nil {
		log.Println("barry-error: failed to parse form:", err)
		os.Exit(1)
	}

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

var apiRunnerTemplate = `package main

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"os"
	target "{{ .ImportPath }}"
)

func main() {
	log.SetOutput(os.Stderr)

	body := strings.NewReader({{ .Body | jsonMarshal }})
	r, _ := http.NewRequest("{{ .Method }}", "{{ .URL }}", body)

	for key, vals := range map[string][]string{
		{{- range $k, $v := .Headers }}
		"{{ $k }}": {{ $v | jsonMarshal }},
		{{- end }}
	} {
		for _, v := range vals {
			r.Header.Add(key, v)
		}
	}

	r.Host = "{{ .Host }}"
	r.RemoteAddr = "{{ .RemoteAddr }}"

	if err := r.ParseForm(); err != nil {
		log.Println("barry-error: failed to parse form:", err)
		os.Exit(1)
	}

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

var templateFuncs = template.FuncMap{
	"jsonMarshal": func(v interface{}) string {
		switch val := v.(type) {
		case string:
			return strconv.Quote(val)
		case []string:
			var b strings.Builder
			b.WriteString("[]string{")
			for i, s := range val {
				if i > 0 {
					b.WriteString(", ")
				}
				b.WriteString(strconv.Quote(s))
			}
			b.WriteString("}")
			return b.String()
		default:
			b, _ := json.Marshal(v)
			return string(b)
		}
	},
}

type ExecContext struct {
	ImportPath string
	Params     map[string]string
	Method     string
	URL        string
	Headers    map[string][]string
	Body       string
	Host       string
	RemoteAddr string
}

type APIExecContext struct {
	ImportPath string
	Params     map[string]string
	Method     string
	URL        string
	Headers    map[string][]string
	Body       string
	Host       string
	RemoteAddr string
}

var ExecuteServerFile = func(filePath string, req *http.Request, params map[string]string, devMode bool) (map[string]interface{}, error) {
	return ExecuteServerFileWithTime(filePath, req, params, devMode, time.Now)
}

func ExecuteServerFileWithTime(filePath string, req *http.Request, params map[string]string, devMode bool, now func() time.Time) (map[string]interface{}, error) {
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

	bodyBytes, _ := io.ReadAll(req.Body)
	req.Body = io.NopCloser(bytes.NewReader(bodyBytes))

	ctx := ExecContext{
		ImportPath: importPath,
		Params:     params,
		Method:     req.Method,
		URL:        req.URL.String(),
		Headers:    req.Header,
		Body:       string(bodyBytes),
		Host:       req.Host,
		RemoteAddr: req.RemoteAddr,
	}

	var buf bytes.Buffer
	tmpl := template.Must(template.New("runner").Funcs(templateFuncs).Parse(runnerTemplate))
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
	cmd.Stderr = io.MultiWriter(os.Stderr, &errBuf)

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

	bodyBytes, _ := io.ReadAll(req.Body)
	req.Body = io.NopCloser(bytes.NewReader(bodyBytes))

	ctx := APIExecContext{
		ImportPath: importPath,
		Params:     params,
		Method:     req.Method,
		URL:        req.URL.String(),
		Headers:    req.Header,
		Body:       string(bodyBytes),
		Host:       req.Host,
		RemoteAddr: req.RemoteAddr,
	}

	var buf bytes.Buffer
	tmpl := template.Must(template.New("api-runner").Funcs(templateFuncs).Parse(apiRunnerTemplate))
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
