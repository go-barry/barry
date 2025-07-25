package core

import (
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type ApiRoute struct {
	Method       string
	URLPattern   *regexp.Regexp
	ParamKeys    []string
	ParamRawKeys []string
	ServerPath   string
	FilePath     string
}

func (r *Router) loadApiRoutes() {
	routes := []ApiRoute{}

	_ = filepath.WalkDir("api", func(path string, d os.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return nil
		}

		filePath := filepath.Join(path, "index.go")
		if _, err := os.Stat(filePath); err != nil {
			filePath = filepath.Join(path, "index.server.go")
			if _, err := os.Stat(filePath); err != nil {
				return nil
			}
		}

		rel := strings.TrimPrefix(path, "api")
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

		routes = append(routes, ApiRoute{
			Method:       "ANY",
			URLPattern:   regex,
			ParamKeys:    paramKeys,
			ParamRawKeys: paramRawKeys,
			ServerPath:   filePath,
			FilePath:     path,
		})

		return nil
	})

	r.apiRoutes = routes
}

func (r *Router) handleAPI(w http.ResponseWriter, req *http.Request, route ApiRoute, params map[string]string) {
	result, err := ExecuteAPIFile(route.ServerPath, req, params)
	if err != nil {
		if IsNotFoundError(err) {
			http.Error(w, "Not Found", http.StatusNotFound)
			return
		}
		http.Error(w, "Server error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(result)
}
