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

func Start(cfg RuntimeConfig) {
	fmt.Println("Starting Barry in", cfg.Env, "mode...")

	config := core.LoadConfig("barry.config.yml")
	config.CacheEnabled = cfg.EnableCache

	mux := http.NewServeMux()

	publicDir := "public"
	cacheStaticDir := filepath.Join(config.OutputDir, "static")

	if cfg.Env == "dev" {
		staticHandler := http.StripPrefix("/static/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Cache-Control", "no-store")
			http.FileServer(http.Dir(publicDir)).ServeHTTP(w, r)
		}))
		mux.Handle("/static/", staticHandler)

		mux.HandleFunc("/favicon.ico", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Cache-Control", "no-store")
			http.ServeFile(w, r, filepath.Join(publicDir, "favicon.ico"))
		})
	} else {
		mux.HandleFunc("/static/", func(w http.ResponseWriter, r *http.Request) {
			uri := r.URL.Path
			if i := strings.Index(uri, "?"); i != -1 {
				uri = uri[:i]
			}
			trimmed := strings.TrimPrefix(uri, "/static/")
			cachedFile := filepath.Join(cacheStaticDir, trimmed)
			gzipFile := cachedFile + ".gz"

			if acceptsGzip(r) {
				if _, err := os.Stat(gzipFile); err == nil {
					ext := filepath.Ext(cachedFile)
					switch ext {
					case ".css":
						w.Header().Set("Content-Type", "text/css")
					case ".js":
						w.Header().Set("Content-Type", "application/javascript")
					default:
						w.Header().Set("Content-Type", "application/octet-stream")
					}

					w.Header().Set("Content-Encoding", "gzip")
					w.Header().Set("Vary", "Accept-Encoding")
					w.Header().Set("Cache-Control", "public, max-age=31536000")
					http.ServeFile(w, r, gzipFile)
					return
				}
			}

			if _, err := os.Stat(cachedFile); err == nil {
				w.Header().Set("Cache-Control", "public, max-age=31536000")
				http.ServeFile(w, r, cachedFile)
				return
			}

			http.StripPrefix("/static/", http.FileServer(http.Dir(publicDir))).ServeHTTP(w, r)
		})

		mux.Handle("/favicon.ico", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Cache-Control", "public, max-age=31536000")
			http.ServeFile(w, r, filepath.Join(publicDir, "favicon.ico"))
		}))
	}

	if cfg.Env == "dev" {
		reloader := core.NewLiveReloader()
		mux.HandleFunc("/__barry_reload", reloader.Handler)

		router := core.NewRouter(config, core.RuntimeContext{
			Env:         cfg.Env,
			EnableWatch: true,
			OnReload:    reloader.BroadcastReload,
		})
		mux.Handle("/", router)
	} else {
		router := core.NewRouter(config, core.RuntimeContext{
			Env:         cfg.Env,
			EnableWatch: false,
			OnReload:    nil,
		})
		mux.Handle("/", router)
	}

	fmt.Printf("✅ Barry running at http://localhost:%d\n", cfg.Port)
	http.ListenAndServe(fmt.Sprintf(":%d", cfg.Port), mux)
}

func acceptsGzip(r *http.Request) bool {
	return strings.Contains(r.Header.Get("Accept-Encoding"), "gzip")
}
