package server

import "net/http"

func HandleRequest(r *http.Request, params map[string]string) (map[string]interface{}, error) {
	return map[string]interface{}{
		"Title": "barry",
		"Intro": "A developer-first HTML + Go framework. No JS. No builds. Just Go.",
		"Button": map[string]interface{}{
			"Text": "Read the docs",
		},
	}, nil
}
