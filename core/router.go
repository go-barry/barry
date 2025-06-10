package core

import (
	"bytes"
	"fmt"
	"html/template"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
)

type Route struct {
	URLPattern *regexp.Regexp
	ParamKeys  []string
	HTMLPath   string
	ServerPath string
	FilePath   string
}

type Router struct {
	config   Config
	env      string
	onReload func()
	routes   []Route
}

type RuntimeContext struct {
	Env         string
	EnableWatch bool
	OnReload    func()
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func NewRouter(config Config, ctx RuntimeContext) *Router {
	r := &Router{
		config:   config,
		env:      ctx.Env,
		onReload: ctx.OnReload,
	}
	r.loadRoutes()

	if ctx.EnableWatch {
		go r.watchEverything()
	}

	return r
}

func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	start := time.Now()
	path := strings.Trim(req.URL.Path, "/")

	recorder := &statusRecorder{ResponseWriter: w, status: 200}

	if path == "" {
		r.serveStatic("routes/index.html", "routes/index.server.go", recorder, req, map[string]string{}, "")
	} else {
		found := false
		for _, route := range r.routes {
			if matches := route.URLPattern.FindStringSubmatch(path); matches != nil {
				params := map[string]string{}
				for i, key := range route.ParamKeys {
					params[key] = matches[i+1]
				}
				r.serveStatic(route.HTMLPath, route.ServerPath, recorder, req, params, path)
				found = true
				break
			}
		}
		if !found {
			renderErrorPage(recorder, r.config, 404, "Page not found", req.URL.Path)
		}
	}

	if r.env == "dev" && shouldLogRequest(req.URL.Path) {
		duration := time.Since(start).Milliseconds()
		fmt.Printf("%s %d %dms\n", req.URL.Path, recorder.status, duration)
	}
}

func (r *Router) serveStatic(htmlPath, serverPath string, w http.ResponseWriter, req *http.Request, params map[string]string, resolvedPath string) {
	if _, err := os.Stat(htmlPath); err != nil {
		renderErrorPage(w, r.config, http.StatusNotFound, "Page not found", req.URL.Path)
		return
	}

	routeKey := strings.TrimPrefix(resolvedPath, "/")

	if r.config.CacheEnabled {
		if html, ok := GetCachedHTML(r.config, routeKey); ok {
			w.Header().Set("Content-Type", "text/html")
			if r.config.DebugHeaders {
				w.Header().Set("X-Barry-Cache", "HIT")
			}
			w.Write(html)
			return
		}
	}

	layoutPath := ""
	if content, err := os.ReadFile(htmlPath); err == nil {
		lines := strings.Split(string(content), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "<!-- layout:") && strings.HasSuffix(line, "-->") {
				layoutPath = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(line, "<!-- layout:"), "-->"))
				break
			}
		}
	}

	data := map[string]interface{}{}
	if _, err := os.Stat(serverPath); err == nil {
		result, err := ExecuteServerFile(serverPath, params, r.env == "dev")
		if err != nil {
			http.Error(w, "Server logic error: "+err.Error(), http.StatusInternalServerError)
			return
		}
		data = result
	}

	var componentFiles []string
	filepath.Walk("components", func(path string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() && strings.HasSuffix(path, ".html") {
			componentFiles = append(componentFiles, path)
		}
		return nil
	})

	tmplFiles := append([]string{htmlPath}, componentFiles...)
	if layoutPath != "" {
		tmplFiles = append([]string{layoutPath}, tmplFiles...)
	}

	tmpl := template.New("").Funcs(template.FuncMap{
		"props": func(values ...interface{}) map[string]interface{} {
			if len(values)%2 != 0 {
				panic("props must be called with even number of arguments")
			}
			m := make(map[string]interface{}, len(values)/2)
			for i := 0; i < len(values); i += 2 {
				key, ok := values[i].(string)
				if !ok {
					panic("props keys must be strings")
				}
				m[key] = values[i+1]
			}
			return m
		},
		"safeHTML": func(s interface{}) template.HTML {
			switch val := s.(type) {
			case template.HTML:
				return val
			case string:
				return template.HTML(val)
			default:
				return ""
			}
		},
	})
	tmpl, err := tmpl.ParseFiles(tmplFiles...)
	if err != nil {
		http.Error(w, "Template error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	var rendered bytes.Buffer
	err = tmpl.ExecuteTemplate(&rendered, "layout", data)
	if err != nil {
		http.Error(w, "Template execution error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	html := rendered.Bytes()

	if r.env == "dev" {
		liveReloadScript := `
<script>
	if (typeof WebSocket !== "undefined") {
		const ws = new WebSocket("ws://" + location.host + "/__barry_reload");
		ws.onmessage = e => {
			if (e.data === "reload") location.reload();
		};
	}
</script>
</body>`
		html = bytes.Replace(html, []byte("</body>"), []byte(liveReloadScript), 1)
	}

	if r.config.CacheEnabled {
		_ = SaveCachedHTML(r.config, routeKey, html)
	}

	w.Header().Set("Content-Type", "text/html")
	if r.config.DebugHeaders {
		w.Header().Set("X-Barry-Cache", "MISS")
	}
	w.Write(html)
}

func (r *Router) loadRoutes() {
	r.routes = []Route{}

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
			if strings.HasPrefix(part, "_") {
				key := part[1:]
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

func renderErrorPage(w http.ResponseWriter, config Config, status int, message, path string) {
	base := "routes/_error"
	statusFile := fmt.Sprintf("%s/%d.html", base, status)
	defaultFile := fmt.Sprintf("%s/index.html", base)

	context := map[string]interface{}{
		"StatusCode": status,
		"Message":    message,
		"Path":       path,
	}

	if _, err := os.Stat(statusFile); err == nil {
		tmpl, err := template.ParseFiles(statusFile)
		if err == nil {
			w.WriteHeader(status)
			tmpl.Execute(w, context)
			return
		}
	}

	if _, err := os.Stat(defaultFile); err == nil {
		tmpl, err := template.ParseFiles(defaultFile)
		if err == nil {
			w.WriteHeader(status)
			tmpl.Execute(w, context)
			return
		}
	}

	w.WriteHeader(status)
	w.Write([]byte(fmt.Sprintf("%d - %s", status, message)))
}

func (r *Router) watchEverything() {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return
	}
	defer watcher.Close()

	watchDirs := []string{"routes", "components", "public"}

	addDirs := func() {
		for _, base := range watchDirs {
			filepath.Walk(base, func(path string, info os.FileInfo, err error) error {
				if err == nil && info.IsDir() {
					_ = watcher.Add(path)
				}
				return nil
			})
		}
	}

	addDirs()

	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}

			if event.Op&(fsnotify.Create|fsnotify.Write|fsnotify.Remove|fsnotify.Rename) != 0 {
				r.loadRoutes()
				addDirs()
				if r.env == "dev" {
					println("🔄 Change detected:", event.Name)
					if r.onReload != nil {
						r.onReload()
					}
				}
			}
		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			println("❌ Watch error:", err.Error())
		}
	}
}

func shouldLogRequest(path string) bool {
	return !strings.HasPrefix(path, "/.well-known") &&
		!strings.HasPrefix(path, "/favicon.ico") &&
		!strings.HasPrefix(path, "/robots.txt")
}
