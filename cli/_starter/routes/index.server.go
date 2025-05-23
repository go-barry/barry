package server

import "net/http"

func HandleRequest(r *http.Request, params map[string]string) (map[string]interface{}, error) {
    return map[string]interface{}{
        "Title": "Welcome to Barry!",
        "Intro": "A developer-first HTML + Go framework. No JS. No builds. Just Go.",
        "Feature": map[string]interface{}{
            "Title": "Reusable Component",
            "Summary": "This card is rendered using a shared component!",
        },
    }, nil
}
