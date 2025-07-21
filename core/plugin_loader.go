package core

import (
	"errors"
	"net/http"
	"os"
	"plugin"
	"strings"
	"sync"
)

var pluginCache sync.Map
var ErrInvalidPlugin = errors.New("invalid plugin: missing HandleRequest")

type pluginWithLookup interface {
	Lookup(string) (plugin.Symbol, error)
}

var pluginOpen = plugin.Open

var loadPluginFunc = func(path string) (pluginWithLookup, error) {
	return pluginOpen(path)
}

func LoadPluginAndCall(filePath string, req *http.Request, params map[string]string) (map[string]interface{}, error) {
	soPath := strings.TrimSuffix(filePath, ".go") + ".so"
	if _, err := os.Stat(soPath); err != nil {
		return nil, nil
	}

	val, ok := pluginCache.Load(soPath)
	var p pluginWithLookup
	var err error

	if ok {
		p = val.(pluginWithLookup)
	} else {
		p, err = loadPluginFunc(soPath)
		if err != nil {
			return nil, err
		}
		pluginCache.Store(soPath, p)
	}

	sym, err := p.Lookup("HandleRequest")
	if err != nil {
		return nil, ErrInvalidPlugin
	}

	handler, ok := sym.(func(*http.Request, map[string]string) (map[string]interface{}, error))
	if !ok {
		return nil, ErrInvalidPlugin
	}

	return handler(req, params)
}
