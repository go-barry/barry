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
	config := core.LoadConfig("barry.config.yml")
	config.CacheEnabled = cfg.EnableCache

	router := core.NewRouter(config)

	fmt.Printf("Barry running in %s mode at http://localhost:%d\n", cfg.Env, cfg.Port)
	http.ListenAndServe(fmt.Sprintf(":%d", cfg.Port), router)
}
