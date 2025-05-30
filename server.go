package barry

import (
	"fmt"
	"net/http"
	"path/filepath"

	"github.com/callumeddisford/barry/core"
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
		fs := http.FileServer(http.Dir(publicDir))
		mux.Handle("/static/", http.StripPrefix("/static/", fs))
		mux.Handle("/favicon.ico", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fs.ServeHTTP(w, r)
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
