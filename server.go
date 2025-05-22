package barry

import (
	"fmt"
	"net/http"

	"github.com/callumeddisford/barry/core"
)

func Start() {
	config := core.LoadConfig("barry.config.yml")
	router := core.NewRouter(config)

	fmt.Println("Barry dev server running at http://localhost:8080")
	http.ListenAndServe(":8080", router)
}
