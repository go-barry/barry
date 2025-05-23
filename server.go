package barry

import (
	"fmt"
	"net/http"

	"github.com/callumeddisford/barry/core"
)

type RuntimeConfig struct {
	Env         string
	EnableCache bool
	Port        int
}

func Start(cfg RuntimeConfig) {
	fmt.Println("🔧 Starting Barry in", cfg.Env, "mode...")

	config := core.LoadConfig("barry.config.yml")
	config.CacheEnabled = cfg.EnableCache

	mux := http.NewServeMux()

	if cfg.Env == "dev" {
		// Set up live reload WebSocket
		reloader := core.NewLiveReloader()
		mux.HandleFunc("/__barry_reload", reloader.Handler)

		// Create router with live reload hook
		router := core.NewRouter(config, core.RuntimeContext{
			Env:         cfg.Env,
			EnableWatch: true,
			OnReload:    reloader.BroadcastReload,
		})

		mux.Handle("/", router)
	} else {
		// Prod mode: no live reload
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
