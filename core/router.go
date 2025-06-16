package core

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"fmt"
	"html/template"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
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
	config         Config
	env            string
	onReload       func()
	routes         []Route
	componentFiles []string
	templateCache  map[string]*template.Template
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

func (r *statusRecorder) Status() int {
	if r.status == 0 {
		return 200
	}
	return r.status
}

func NewRouter(config Config, ctx RuntimeContext) *Router {
	r := &Router{
		config:        config,
		env:           ctx.Env,
		onReload:      ctx.OnReload,
		templateCache: make(map[string]*template.Template),
	}

	r.loadRoutes()
	r.loadComponentFiles()

	if ctx.EnableWatch {
		go r.watchEverything()
	}

	return r
}

func (r *Router) loadComponentFiles() {
	var files []string
	filepath.Walk("components", func(path string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() && strings.HasSuffix(path, ".html") {
			files = append(files, path)
		}
		return nil
	})
	r.componentFiles = files
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
			renderErrorPage(w, r.config, r.env, http.StatusNotFound, "Page not found", req.URL.Path, r.componentFiles)
		}
	}

	if r.env == "dev" && shouldLogRequest(req.URL.Path) {
		duration := time.Since(start).Milliseconds()
		fmt.Printf("%s %d %dms\n", req.URL.Path, recorder.Status(), duration)
	}
}

var cacheLocks sync.Map

func (r *Router) serveStatic(htmlPath, serverPath string, w http.ResponseWriter, req *http.Request, params map[string]string, resolvedPath string) {
	if _, err := os.Stat(htmlPath); err != nil {
		renderErrorPage(w, r.config, r.env, http.StatusNotFound, "Page not found", req.URL.Path, r.componentFiles)
		return
	}

	routeKey := strings.TrimPrefix(resolvedPath, "/")

	if r.config.CacheEnabled {
		cacheDir := filepath.Join(r.config.OutputDir, routeKey)
		cachedFile := filepath.Join(cacheDir, "index.html")
		gzFile := cachedFile + ".gz"

		if r.env == "prod" && acceptsGzip(req) {
			if data, err := os.ReadFile(gzFile); err == nil {
				etag := generateETag(data)
				if match := req.Header.Get("If-None-Match"); match == etag {
					w.WriteHeader(http.StatusNotModified)
					return
				}
				w.Header().Set("ETag", etag)
				w.Header().Set("Content-Encoding", "gzip")
				w.Header().Set("Vary", "Accept-Encoding")
				w.Header().Set("Content-Type", "text/html")
				if r.config.DebugHeaders {
					w.Header().Set("X-Barry-Cache", "HIT")
				}
				w.Write(data)
				return
			}
		}

		if data, err := os.ReadFile(cachedFile); err == nil {
			etag := generateETag(data)
			if match := req.Header.Get("If-None-Match"); match == etag {
				w.WriteHeader(http.StatusNotModified)
				return
			}
			w.Header().Set("ETag", etag)
			w.Header().Set("Content-Type", "text/html")
			if r.config.DebugHeaders {
				w.Header().Set("X-Barry-Cache", "HIT")
			}
			w.Write(data)
			return
		}
	}

	data := map[string]interface{}{}
	if _, err := os.Stat(serverPath); err == nil {
		result, err := ExecuteServerFile(serverPath, params, r.env == "dev")
		if err != nil {
			if IsNotFoundError(err) {
				renderErrorPage(w, r.config, r.env, http.StatusNotFound, "Page not found", req.URL.Path, r.componentFiles)
				return
			}
			http.Error(w, "Server logic error: "+err.Error(), http.StatusInternalServerError)
			return
		}
		data = result
	}

	layoutPath := extractLayoutPath(htmlPath)

	var tmplFiles []string
	if layoutPath != "" {
		tmplFiles = append(tmplFiles, layoutPath)
	}
	tmplFiles = append(tmplFiles, htmlPath)
	tmplFiles = append(tmplFiles, r.componentFiles...)

	cacheKey := hashTemplateFiles(tmplFiles)
	tmpl, ok := r.templateCache[cacheKey]
	if !ok {
		tmpl = template.New("").Funcs(BarryTemplateFuncs(r.env, r.config.OutputDir))
		var err error
		tmpl, err = tmpl.ParseFiles(tmplFiles...)
		if err != nil {
			http.Error(w, "Template error: "+err.Error(), http.StatusInternalServerError)
			return
		}
		r.templateCache[cacheKey] = tmpl
	}

	var rendered bytes.Buffer
	if err := tmpl.ExecuteTemplate(&rendered, "layout", data); err != nil {
		http.Error(w, "Template execution error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	html := rendered.Bytes()

	if r.env == "dev" {
		html = bytes.Replace(html, []byte("</body>"), []byte(`
<script>
	if (typeof WebSocket !== "undefined") {
		const ws = new WebSocket("ws://" + location.host + "/__barry_reload");
		ws.onmessage = e => {
			if (e.data === "reload") location.reload();
		};
	}
</script>
</body>`), 1)
	}

	w.Header().Set("Content-Type", "text/html")
	if r.config.DebugHeaders {
		w.Header().Set("X-Barry-Cache", "MISS")
	}
	w.Write(html)

	if r.config.CacheEnabled {
		lock := getOrCreateLock(routeKey)
		go func(html []byte, key string, l *sync.Mutex) {
			l.Lock()
			defer l.Unlock()
			_ = SaveCachedHTML(r.config, key, html)
		}(html, routeKey, lock)
	}
}

func (r *Router) loadRoutes() {
	var routes []Route

	filepath.WalkDir("routes", func(path string, d os.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
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

		routes = append(routes, Route{
			URLPattern: regex,
			ParamKeys:  paramKeys,
			HTMLPath:   htmlPath,
			ServerPath: filepath.Join(path, "index.server.go"),
			FilePath:   path,
		})

		return nil
	})

	r.routes = routes
}

func renderErrorPage(w http.ResponseWriter, config Config, env string, status int, message, path string, componentFiles []string) {
	base := "routes/_error"
	statusFile := fmt.Sprintf("%s/%d.html", base, status)
	defaultFile := fmt.Sprintf("%s/index.html", base)

	context := map[string]interface{}{
		"Title":       fmt.Sprintf("%d - %s", status, message),
		"StatusCode":  status,
		"Message":     message,
		"Path":        path,
		"Description": message,
	}

	tryRender := func(file string) bool {
		layoutPath := extractLayoutPath(file)

		var tmplFiles []string
		if layoutPath != "" {
			tmplFiles = append(tmplFiles, layoutPath)
		}
		tmplFiles = append(tmplFiles, file)
		tmplFiles = append(tmplFiles, componentFiles...)

		tmpl := template.New("").Funcs(BarryTemplateFuncs(env, config.OutputDir))
		tmpl, err := tmpl.ParseFiles(tmplFiles...)
		if err != nil {
			fmt.Println("❌ Error parsing error page:", err)
			return false
		}

		writeStatusOnce(w, status)
		err = tmpl.ExecuteTemplate(w, "layout", context)
		if err != nil {
			fmt.Println("❌ Error executing error layout:", err)
			return false
		}
		return true
	}

	if tryRender(statusFile) || tryRender(defaultFile) {
		return
	}

	writeStatusOnce(w, status)
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

func acceptsGzip(r *http.Request) bool {
	return strings.Contains(r.Header.Get("Accept-Encoding"), "gzip")
}

func extractLayoutPath(file string) string {
	f, err := os.Open(file)
	if err != nil {
		return ""
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for i := 0; i < 50 && scanner.Scan(); i++ {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "<!-- layout:") && strings.HasSuffix(line, "-->") {
			return strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(line, "<!-- layout:"), "-->"))
		}
	}

	return ""
}

func writeStatusOnce(w http.ResponseWriter, status int) {
	if rw, ok := w.(*statusRecorder); ok && rw.status == 0 {
		rw.WriteHeader(status)
	}
}

func hashTemplateFiles(paths []string) string {
	h := sha256.New()
	for _, p := range paths {
		h.Write([]byte(p))
		if info, err := os.Stat(p); err == nil {
			mtime := info.ModTime().UnixNano()
			h.Write([]byte(fmt.Sprintf("%d", mtime)))
		}
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}

func generateETag(data []byte) string {
	hash := sha256.Sum256(data)
	return fmt.Sprintf(`W/"%x"`, hash[:8])
}

func getOrCreateLock(key string) *sync.Mutex {
	lock, ok := cacheLocks.Load(key)
	if ok {
		return lock.(*sync.Mutex)
	}
	mutex := &sync.Mutex{}
	actual, _ := cacheLocks.LoadOrStore(key, mutex)
	return actual.(*sync.Mutex)
}
