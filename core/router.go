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
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

type Route struct {
	URLPattern   *regexp.Regexp
	ParamKeys    []string
	ParamRawKeys []string
	HTMLPath     string
	ServerPath   string
	FilePath     string
}

type Router struct {
	config         Config
	env            string
	onReload       func()
	routes         []Route
	apiRoutes      []ApiRoute
	componentFiles []string
	templateCache  sync.Map
	layoutCache    sync.Map
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

func (r *statusRecorder) Flush() {
	if f, ok := r.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

type cacheWriteRequest struct {
	Config   Config
	RouteKey string
	HTML     []byte
	Lock     *sync.Mutex
	Ext      string
}

var cacheLocks sync.Map
var compileLocks sync.Map
var cacheQueue = make(chan cacheWriteRequest, 100)
var SaveCachedHTMLFunc = SaveCachedHTML
var newWatcher = fsnotify.NewWatcher

func init() {
	go func() {
		for req := range cacheQueue {
			safeHTML := make([]byte, len(req.HTML))
			copy(safeHTML, req.HTML)

			req.Lock.Lock()
			_ = SaveCachedHTMLFunc(req.Config, req.RouteKey, req.Ext, safeHTML)
			req.Lock.Unlock()
		}
	}()
}

var NewRouter = func(config Config, ctx RuntimeContext) http.Handler {
	r := &Router{
		config:   config,
		env:      ctx.Env,
		onReload: ctx.OnReload,
	}

	var wg sync.WaitGroup
	wg.Add(3)
	go func() {
		defer wg.Done()
		r.loadRoutes()
	}()
	go func() {
		defer wg.Done()
		r.loadComponentFiles()
	}()
	go func() {
		defer wg.Done()
		r.loadApiRoutes()
	}()
	wg.Wait()

	if ctx.EnableWatch {
		go r.watchEverything()
	}

	return r
}

func (r *Router) loadRoutes() {
	routes := []Route{}

	_ = filepath.WalkDir("routes", func(path string, d os.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return nil
		}

		if strings.HasPrefix(filepath.Base(path), "_error") {
			return filepath.SkipDir
		}

		htmlPath := filepath.Join(path, "index.html")
		xmlPath := filepath.Join(path, "index.xml")

		hasHTML := fileExists(htmlPath)
		hasXML := fileExists(xmlPath)

		if !hasHTML && !hasXML {
			return nil
		}

		rel := strings.TrimPrefix(path, "routes")
		parts := strings.Split(strings.Trim(rel, "/"), "/")
		paramKeys := []string{}
		paramRawKeys := []string{}
		pattern := ""

		for _, part := range parts {
			if strings.HasPrefix(part, "_") {
				rawKey := part[1:]
				cleanKey := strings.TrimSuffix(rawKey, filepath.Ext(rawKey))

				paramRawKeys = append(paramRawKeys, rawKey)
				paramKeys = append(paramKeys, cleanKey)
				pattern += "/([^/]+)"
			} else {
				pattern += "/" + part
			}
		}

		regex := regexp.MustCompile("^" + strings.TrimPrefix(pattern, "/") + "$")

		routes = append(routes, Route{
			URLPattern:   regex,
			ParamKeys:    paramKeys,
			ParamRawKeys: paramRawKeys,
			HTMLPath:     choose(htmlPath, xmlPath),
			ServerPath:   filepath.Join(path, "index.server.go"),
			FilePath:     path,
		})

		return nil
	})

	sort.SliceStable(routes, func(i, j int) bool {
		isDynamic := func(parts []string) bool {
			for _, part := range parts {
				if strings.HasPrefix(part, "_") {
					return true
				}
			}
			return false
		}
		pi := strings.Split(strings.TrimPrefix(routes[i].FilePath, "routes/"), "/")
		pj := strings.Split(strings.TrimPrefix(routes[j].FilePath, "routes/"), "/")

		return !isDynamic(pi) && isDynamic(pj)
	})

	r.routes = routes
}

func choose(a, b string) string {
	if fileExists(a) {
		return a
	}
	return b
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func (r *Router) loadComponentFiles() {
	var files []string
	_ = filepath.Walk("components", func(path string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() && strings.HasSuffix(path, ".html") {
			files = append(files, path)
		}
		return nil
	})
	r.componentFiles = files
}

func (r *Router) serveStatic(htmlPath, serverPath string, w http.ResponseWriter, req *http.Request, params map[string]string, resolvedPath string) {
	if _, err := os.Stat(htmlPath); err != nil {
		r.renderErrorPage(w, http.StatusNotFound, "Page not found", req.URL.Path)
		return
	}

	isXML := strings.HasSuffix(htmlPath, ".xml")
	routeKey := strings.TrimPrefix(resolvedPath, "/")

	if r.config.CacheEnabled {
		cacheDir := filepath.Join(r.config.OutputDir, routeKey)
		cachedFile := filepath.Join(cacheDir, "index."+getFileExt(htmlPath))
		gzFile := cachedFile + ".gz"

		if r.env == "prod" && acceptsGzip(req) {
			if data, err := os.ReadFile(gzFile); err == nil {
				etag := generateETag(data)
				if match := req.Header.Get("If-None-Match"); match == etag {
					if r.config.DebugLogs {
						fmt.Printf("🧩 304 Not Modified (gzip): /%s\n", routeKey)
					}
					w.WriteHeader(http.StatusNotModified)
					return
				}
				w.Header().Set("ETag", etag)
				w.Header().Set("Content-Encoding", "gzip")
				w.Header().Set("Vary", "Accept-Encoding")
				w.Header().Set("Content-Type", getContentType(htmlPath))
				w.Header().Set("Content-Length", strconv.Itoa(len(data)))
				if r.config.DebugHeaders {
					w.Header().Set("X-Barry-Cache", "HIT")
				}
				if r.config.DebugLogs {
					fmt.Printf("📦 Cache HIT (gzip): /%s\n", routeKey)
				}
				w.Write(data)
				return
			}
		}

		if data, err := os.ReadFile(cachedFile); err == nil {
			etag := generateETag(data)
			if match := req.Header.Get("If-None-Match"); match == etag {
				if r.config.DebugLogs {
					fmt.Printf("🧩 304 Not Modified: /%s\n", routeKey)
				}
				w.WriteHeader(http.StatusNotModified)
				return
			}
			w.Header().Set("ETag", etag)
			w.Header().Set("Content-Type", getContentType(htmlPath))
			w.Header().Set("Content-Length", strconv.Itoa(len(data)))
			if r.config.DebugHeaders {
				w.Header().Set("X-Barry-Cache", "HIT")
			}
			if r.config.DebugLogs {
				fmt.Printf("📦 Cache HIT: /%s\n", routeKey)
			}
			w.Write(data)
			return
		}
	}

	data := map[string]interface{}{}
	if fileExists(serverPath) {
		lock := getOrCreateCompileLock(serverPath)
		lock.Lock()
		result, err := ExecuteServerFile(serverPath, req, params)
		lock.Unlock()
		if err != nil {
			if IsNotFoundError(err) {
				r.renderErrorPage(w, http.StatusNotFound, "Page not found", req.URL.Path)
				return
			}
			http.Error(w, "Server logic error: "+err.Error(), http.StatusInternalServerError)
			return
		}
		data = result
	}

	layoutPath := r.getLayoutPath(htmlPath)
	tmplFiles := []string{}
	if layoutPath != "" && fileExists(layoutPath) {
		tmplFiles = append(tmplFiles, layoutPath)
	}
	tmplFiles = append(tmplFiles, htmlPath)
	tmplFiles = append(tmplFiles, r.componentFiles...)

	cacheKey := hashTemplateFiles(tmplFiles)

	var tmpl *template.Template
	if val, ok := r.templateCache.Load(cacheKey); ok {
		tmpl = val.(*template.Template)
	} else {
		tmpl = template.New("").Funcs(BarryTemplateFuncs(r.env, r.config.OutputDir))
		parsed, err := tmpl.ParseFiles(tmplFiles...)
		if err != nil {
			fmt.Printf("❌ Template parse error [%s]: %v\n", cacheKey, err)
			http.Error(w, "Template error: "+err.Error(), http.StatusInternalServerError)
			return
		}
		actual, _ := r.templateCache.LoadOrStore(cacheKey, parsed)
		tmpl = actual.(*template.Template)
	}

	var rendered bytes.Buffer
	templateName := filepath.Base(htmlPath)
	if layoutPath != "" && !isXML {
		templateName = "layout"
	}

	if err := tmpl.ExecuteTemplate(&rendered, templateName, data); err != nil {
		http.Error(w, "Template execution error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	html := rendered.Bytes()

	if r.env == "dev" && !isXML {
		html = bytes.Replace(html, []byte("</body>"), []byte(`
<script>
	if (typeof WebSocket !== "undefined") {
		const protocol = location.protocol === "https:" ? "wss" : "ws";
		const ws = new WebSocket(protocol + "://" + location.host + "/__barry_reload");
		ws.onmessage = e => {
			if (e.data === "reload") location.reload();
		};
	}
</script>
</body>`), 1)
	}

	w.Header().Set("Content-Type", getContentType(htmlPath))
	w.Header().Set("Content-Length", strconv.Itoa(len(html)))
	if r.config.DebugHeaders {
		w.Header().Set("X-Barry-Cache", "MISS")
	}
	w.Write(html)
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}

	if r.config.CacheEnabled {
		lock := getOrCreateLock(routeKey)
		ext := getFileExt(htmlPath)
		req := cacheWriteRequest{
			Config:   r.config,
			RouteKey: routeKey,
			HTML:     append([]byte(nil), html...),
			Lock:     lock,
			Ext:      ext,
		}

		select {
		case cacheQueue <- req:
			if r.config.DebugLogs {
				fmt.Printf("📝 Enqueued cache write: /%s\n", routeKey)
			}
		default:
			if r.config.DebugLogs {
				fmt.Printf("⚠️  Cache queue full — writing immediately for: /%s\n", routeKey)
			}
			go func() {
				req.Lock.Lock()
				err := SaveCachedHTMLFunc(req.Config, req.RouteKey, req.Ext, req.HTML)
				req.Lock.Unlock()
				if err != nil {
					fmt.Printf("❌ Cache write failed (immediate): /%s → %v\n", req.RouteKey, err)
				} else {
					fmt.Printf("✅ Cache write complete (immediate): /%s\n", req.RouteKey)
				}
			}()
		}
	}
}

func getContentType(path string) string {
	switch filepath.Ext(path) {
	case ".xml":
		return "application/xml"
	default:
		return "text/html"
	}
}

func getFileExt(path string) string {
	ext := filepath.Ext(path)
	if ext != "" {
		return ext[1:]
	}
	return "html"
}

func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	start := time.Now()
	path := strings.Trim(req.URL.Path, "/")
	recorder := &statusRecorder{ResponseWriter: w, status: 200}

	if strings.HasPrefix(path, "api/") {
		apiPath := strings.TrimPrefix(path, "api/")
		for _, route := range r.apiRoutes {
			if matches := route.URLPattern.FindStringSubmatch(apiPath); matches != nil {
				params := map[string]string{}
				for i, key := range route.ParamKeys {
					rawKey := route.ParamRawKeys[i]
					ext := filepath.Ext(rawKey)
					value := matches[i+1]

					if ext != "" && strings.HasSuffix(value, ext) {
						value = strings.TrimSuffix(value, ext)
					}

					params[key] = value
				}

				r.handleAPI(recorder, req, route, params)
				return
			}
		}
		http.Error(w, "API route not found", http.StatusNotFound)
		return
	}

	if path == "" {
		r.serveStatic("routes/index.html", "routes/index.server.go", recorder, req, map[string]string{}, "")
	} else {
		found := false
		for _, route := range r.routes {
			if matches := route.URLPattern.FindStringSubmatch(path); matches != nil {
				params := map[string]string{}
				for i, key := range route.ParamKeys {
					rawKey := route.ParamRawKeys[i]
					ext := filepath.Ext(rawKey)
					value := matches[i+1]

					if ext != "" && strings.HasSuffix(value, ext) {
						value = strings.TrimSuffix(value, ext)
					}

					params[key] = value
				}
				r.serveStatic(route.HTMLPath, route.ServerPath, recorder, req, params, path)
				found = true
				break
			}
		}
		if !found {
			r.renderErrorPage(recorder, http.StatusNotFound, "Page not found", req.URL.Path)
		}
	}

	if r.env == "dev" && shouldLogRequest(req.URL.Path) {
		duration := time.Since(start).Milliseconds()
		fmt.Printf("%s %d %dms\n", req.URL.Path, recorder.Status(), duration)
	}
}

func (r *Router) watchEverything() {
	watcher, err := newWatcher()
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

	debounce := time.NewTimer(0)
	if !debounce.Stop() {
		<-debounce.C
	}

	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			if event.Op&(fsnotify.Create|fsnotify.Write|fsnotify.Remove|fsnotify.Rename) != 0 {
				debounce.Reset(100 * time.Millisecond)
			}
		case <-debounce.C:
			r.loadRoutes()
			addDirs()
			if r.env == "dev" && r.onReload != nil {
				fmt.Println("🔄 Change detected and reloaded")
				r.onReload()
			}
		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			fmt.Println("❌ Watch error:", err)
		}
	}
}

func getOrCreateCompileLock(key string) *sync.Mutex {
	lock, ok := compileLocks.Load(key)
	if ok {
		return lock.(*sync.Mutex)
	}
	mutex := &sync.Mutex{}
	actual, _ := compileLocks.LoadOrStore(key, mutex)
	return actual.(*sync.Mutex)
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

func generateETag(data []byte) string {
	hash := sha256.Sum256(data)
	return fmt.Sprintf(`W/"%x"`, hash[:8])
}

func shouldLogRequest(path string) bool {
	return !strings.HasPrefix(path, "/.well-known") &&
		!strings.HasPrefix(path, "/favicon.ico") &&
		!strings.HasPrefix(path, "/robots.txt")
}

func acceptsGzip(r *http.Request) bool {
	return strings.Contains(r.Header.Get("Accept-Encoding"), "gzip")
}

func hashTemplateFiles(paths []string) string {
	h := sha256.New()
	for _, p := range paths {
		h.Write([]byte(p))
		if info, err := os.Stat(p); err == nil {
			mtime := info.ModTime().UnixNano()
			fmt.Fprintf(h, "%d", mtime)
		}
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}

func (r *Router) getLayoutPath(file string) string {
	if val, ok := r.layoutCache.Load(file); ok {
		return val.(string)
	}

	f, err := os.Open(file)
	if err != nil {
		r.layoutCache.Store(file, "")
		return ""
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for i := 0; i < 50 && scanner.Scan(); i++ {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "<!-- layout:") && strings.HasSuffix(line, "-->") {
			layout := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(line, "<!-- layout:"), "-->"))
			actual, _ := r.layoutCache.LoadOrStore(file, layout)
			return actual.(string)
		}
	}

	if err := scanner.Err(); err != nil {
		fmt.Printf("❌ Error scanning %s for layout directive: %v\n", file, err)
	}

	actual, _ := r.layoutCache.LoadOrStore(file, "")
	return actual.(string)
}

func (r *Router) renderErrorPage(w http.ResponseWriter, status int, message, path string) {
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
		layoutPath := r.getLayoutPath(file)

		tmplFiles := []string{}

		if layoutPath != "" {
			if _, err := os.Stat(layoutPath); err == nil {
				tmplFiles = append(tmplFiles, layoutPath)
			} else {
				fmt.Printf("⚠️ Skipping missing layout: %q\n", layoutPath)
			}
		}

		if file != "" {
			if _, err := os.Stat(file); err == nil {
				tmplFiles = append(tmplFiles, file)
			} else {
				fmt.Printf("⚠️ Skipping missing error template: %s\n", file)
			}
		}

		for _, f := range r.componentFiles {
			if f != "" {
				tmplFiles = append(tmplFiles, f)
			}
		}

		if len(tmplFiles) == 0 {
			http.Error(w, "Internal server error: no template files to parse", http.StatusInternalServerError)
			return false
		}

		name := filepath.Base(file)

		tmpl := template.New("").Funcs(BarryTemplateFuncs(r.env, r.config.OutputDir))
		tmpl, err := tmpl.ParseFiles(tmplFiles...)
		if err != nil {
			fmt.Println("❌ Error parsing error page:", err)
			http.Error(w, "Template error: "+err.Error(), http.StatusInternalServerError)
			return false
		}

		w.WriteHeader(status)

		if layoutPath != "" {
			err = tmpl.ExecuteTemplate(w, "layout", context)
		} else {
			err = tmpl.ExecuteTemplate(w, name, context)
		}

		if err != nil {
			fmt.Println("❌ Error executing error template:", err)
			http.Error(w, "Template execution error: "+err.Error(), http.StatusInternalServerError)
			return false
		}

		return true
	}

	if tryRender(statusFile) || tryRender(defaultFile) {
		return
	}

	w.WriteHeader(status)
	fmt.Fprintf(w, "%d - %s", status, message)
}
