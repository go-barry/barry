package core

import (
	"html/template"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type Route struct {
	URLPattern *regexp.Regexp
	ParamKeys  []string
	HTMLPath   string
	ServerPath string
	FilePath   string
}

type Router struct {
	config Config
	routes []Route
}

func NewRouter(config Config) *Router {
	r := &Router{config: config}
	r.loadRoutes()
	return r
}

func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	path := strings.Trim(req.URL.Path, "/")

	if path == "" {
		r.serveStatic("routes/index.html", "routes/index.server.go", w, req, map[string]string{})
		return
	}

	for _, route := range r.routes {
		if matches := route.URLPattern.FindStringSubmatch(path); matches != nil {
			params := map[string]string{}
			for i, key := range route.ParamKeys {
				params[key] = matches[i+1]
			}
			r.serveStatic(route.HTMLPath, route.ServerPath, w, req, params)
			return
		}
	}

	http.NotFound(w, req)
}

func (r *Router) serveStatic(htmlPath, serverPath string, w http.ResponseWriter, req *http.Request, params map[string]string) {
	if _, err := os.Stat(htmlPath); err != nil {
		http.NotFound(w, req)
		return
	}

	data := map[string]interface{}{}

	if _, err := os.Stat(serverPath); err == nil {
		result, err := ExecuteServerFile(serverPath, params)
		if err != nil {
			http.Error(w, "Server logic error: "+err.Error(), http.StatusInternalServerError)
			return
		}
		data = result
	}

	tmpl, err := template.ParseFiles(htmlPath)
	if err != nil {
		http.Error(w, "Template error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	if r.config.DebugHeaders {
		w.Header().Set("X-Barry-Route", filepath.Base(htmlPath))
	}
	w.Header().Set("Content-Type", "text/html")
	tmpl.Execute(w, data)
}

func (r *Router) loadRoutes() {
	filepath.Walk("routes", func(path string, info os.FileInfo, err error) error {
		if err != nil || !info.IsDir() {
			return nil
		}

		htmlPath := filepath.Join(path, "index.html")
		if _, err := os.Stat(htmlPath); err != nil {
			return nil
		}

		rel := strings.TrimPrefix(path, "routes")
		parts := strings.Split(strings.Trim(rel, "/"), "/")
		paramKeys := []string{}
		pattern := ""

		for _, part := range parts {
			if strings.HasPrefix(part, "[") && strings.HasSuffix(part, "]") {
				key := part[1 : len(part)-1]
				paramKeys = append(paramKeys, key)
				pattern += "/([^/]+)"
			} else {
				pattern += "/" + part
			}
		}

		regex := regexp.MustCompile("^" + strings.TrimPrefix(pattern, "/") + "$")

		r.routes = append(r.routes, Route{
			URLPattern: regex,
			ParamKeys:  paramKeys,
			HTMLPath:   filepath.Join(path, "index.html"),
			ServerPath: filepath.Join(path, "index.server.go"),
			FilePath:   path,
		})

		return nil
	})
}
