package barry

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-barry/barry/core"
)

type RuntimeConfig struct {
	Env         string
	EnableCache bool
	Port        int
}

var Start = func(cfg RuntimeConfig) {
	fmt.Println("🚀 Starting Barry in", cfg.Env, "mode...")

	config := core.LoadConfig("barry.config.yml")
	config.CacheEnabled = cfg.EnableCache

	mux := http.NewServeMux()
	publicDir := "public"
	cacheStaticDir := filepath.Join(config.OutputDir, "static")

	if cfg.Env == "dev" {
		setupDevStaticRoutes(mux, publicDir)
	} else {
		mux.HandleFunc("/static/", makeStaticHandler(publicDir, cacheStaticDir))
		mux.HandleFunc("/favicon.ico", func(w http.ResponseWriter, r *http.Request) {
			serveFileWithHeaders(w, r, filepath.Join(publicDir, "favicon.ico"), "public, max-age=31536000, immutable")
		})
		mux.HandleFunc("/robots.txt", func(w http.ResponseWriter, r *http.Request) {
			serveFileWithHeaders(w, r, filepath.Join(publicDir, "robots.txt"), "public, max-age=31536000, immutable")
		})
	}

	router := core.NewRouter(config, core.RuntimeContext{
		Env:         cfg.Env,
		EnableWatch: cfg.Env == "dev",
		OnReload:    nil,
	})

	if cfg.Env == "dev" {
		reloader := core.NewLiveReloader()
		router = core.NewRouter(config, core.RuntimeContext{
			Env:         cfg.Env,
			EnableWatch: true,
			OnReload:    reloader.BroadcastReload,
		})
		mux.HandleFunc("/__barry_reload", reloader.Handler)
	}

	mux.Handle("/", router)

	addr := fmt.Sprintf(":%d", cfg.Port)
	fmt.Printf("✅ Barry running at http://localhost%s\n", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		fmt.Fprintf(os.Stderr, "❌ Server failed: %v\n", err)
		os.Exit(1)
	}
}

func acceptsGzip(r *http.Request) bool {
	return strings.Contains(r.Header.Get("Accept-Encoding"), "gzip")
}

func makeStaticHandler(publicDir, cacheStaticDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uri := r.URL.Path
		if i := strings.Index(uri, "?"); i != -1 {
			uri = uri[:i]
		}
		trimmed := strings.TrimPrefix(uri, "/static/")

		if strings.Contains(trimmed, "..") {
			http.Error(w, "Invalid path", http.StatusBadRequest)
			return
		}

		cachedFile := filepath.Join(cacheStaticDir, trimmed)
		gzFile := cachedFile + ".gz"

		if acceptsGzip(r) {
			if _, err := os.Stat(gzFile); err == nil {
				w.Header().Set("Content-Type", detectMimeType(cachedFile))
				w.Header().Set("Content-Encoding", "gzip")
				w.Header().Set("Vary", "Accept-Encoding")
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
				http.ServeFile(w, r, gzFile)
				return
			}
		}

		if _, err := os.Stat(cachedFile); err == nil {
			serveFileWithHeaders(w, r, cachedFile, "public, max-age=31536000, immutable")
			return
		}

		publicFile := filepath.Join(publicDir, trimmed)
		if _, err := os.Stat(publicFile); err == nil {
			serveFileWithHeaders(w, r, publicFile, "public, max-age=31536000, immutable")
			return
		}

		fmt.Printf("🛑 Static asset not found: %s\n", r.URL.Path)
		http.NotFound(w, r)
	}
}

func detectMimeType(path string) string {
	switch ext := strings.ToLower(filepath.Ext(path)); ext {
	case ".css":
		return "text/css"
	case ".js":
		return "application/javascript"
	case ".webp":
		return "image/webp"
	case ".svg":
		return "image/svg+xml"
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".woff":
		return "font/woff"
	case ".woff2":
		return "font/woff2"
	default:
		return "application/octet-stream"
	}
}

func serveFileWithHeaders(w http.ResponseWriter, r *http.Request, filePath, cacheControl string) {
	w.Header().Set("Cache-Control", cacheControl)
	w.Header().Set("Content-Type", detectMimeType(filePath))
	http.ServeFile(w, r, filePath)
}

func setupDevStaticRoutes(mux *http.ServeMux, publicDir string) {
	staticHandler := http.StripPrefix("/static/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		http.FileServer(http.Dir(publicDir)).ServeHTTP(w, r)
	}))
	mux.Handle("/static/", staticHandler)

	mux.HandleFunc("/favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		http.ServeFile(w, r, filepath.Join(publicDir, "favicon.ico"))
	})

	mux.HandleFunc("/robots.txt", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		http.ServeFile(w, r, filepath.Join(publicDir, "robots.txt"))
	})
}
