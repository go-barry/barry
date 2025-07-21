package core

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"plugin"
	"strings"
	"testing"
)

func TestLoadPluginAndCall_PluginNotFound(t *testing.T) {
	tmp := t.TempDir()
	testPath := filepath.Join(tmp, "not-found.go")
	req, _ := http.NewRequest(http.MethodGet, "/", nil)

	result, err := LoadPluginAndCall(testPath, req, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil result when plugin .so does not exist")
	}
}

func TestLoadPluginAndCall_InvalidSymbol(t *testing.T) {
	original := pluginOpen
	defer func() { pluginOpen = original }()

	pluginOpen = func(path string) (*plugin.Plugin, error) {
		return &plugin.Plugin{}, errors.New("mock plugin open failure")
	}

	tmp := t.TempDir()
	soPath := filepath.Join(tmp, "bad.so")
	_ = os.WriteFile(soPath, []byte{}, 0644)

	req, _ := http.NewRequest(http.MethodGet, "/", nil)
	_, err := LoadPluginAndCall(strings.TrimSuffix(soPath, ".so")+".go", req, nil)

	if err == nil || !strings.Contains(err.Error(), "mock plugin open failure") {
		t.Errorf("expected plugin open failure, got: %v", err)
	}
}

type badPlugin struct{}

func (badPlugin) Lookup(name string) (plugin.Symbol, error) {
	return func(x int) int { return x }, nil
}

func TestLoadPluginAndCall_InvalidHandlerType(t *testing.T) {
	original := loadPluginFunc
	defer func() { loadPluginFunc = original }()

	loadPluginFunc = func(path string) (pluginWithLookup, error) {
		return badPlugin{}, nil
	}

	tmp := t.TempDir()
	soPath := filepath.Join(tmp, "plugin.so")
	_ = os.WriteFile(soPath, []byte("fake plugin"), 0644)

	req, _ := http.NewRequest(http.MethodGet, "/", nil)
	_, err := LoadPluginAndCall(strings.TrimSuffix(soPath, ".so")+".go", req, nil)

	if err == nil || !errors.Is(err, ErrInvalidPlugin) {
		t.Errorf("expected ErrInvalidPlugin, got: %v", err)
	}
}

type goodPlugin struct{}

func (goodPlugin) Lookup(name string) (plugin.Symbol, error) {
	return func(r *http.Request, p map[string]string) (map[string]interface{}, error) {
		return map[string]interface{}{"foo": "bar"}, nil
	}, nil
}

func TestLoadPluginAndCall_Success(t *testing.T) {
	original := loadPluginFunc
	defer func() { loadPluginFunc = original }()

	loadPluginFunc = func(path string) (pluginWithLookup, error) {
		return goodPlugin{}, nil
	}

	tmp := t.TempDir()
	soPath := filepath.Join(tmp, "valid.so")
	_ = os.WriteFile(soPath, []byte("dummy"), 0644)

	req, _ := http.NewRequest(http.MethodGet, "/", nil)
	result, err := LoadPluginAndCall(strings.TrimSuffix(soPath, ".so")+".go", req, map[string]string{"key": "value"})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result["foo"] != "bar" {
		t.Errorf("expected foo: bar, got %v", result)
	}
}

type cachedPlugin struct {
	wasCalled *bool
}

func (cp cachedPlugin) Lookup(name string) (plugin.Symbol, error) {
	*cp.wasCalled = true
	return func(r *http.Request, p map[string]string) (map[string]interface{}, error) {
		return map[string]interface{}{"cached": true}, nil
	}, nil
}

func TestLoadPluginAndCall_UsesCachedPlugin(t *testing.T) {
	tmp := t.TempDir()
	soPath := filepath.Join(tmp, "plugin.so")
	_ = os.WriteFile(soPath, []byte("dummy"), 0644)

	called := false
	cp := cachedPlugin{wasCalled: &called}
	pluginCache.Store(soPath, cp)
	defer pluginCache.Delete(soPath)

	req, _ := http.NewRequest(http.MethodGet, "/", nil)
	result, err := LoadPluginAndCall(strings.TrimSuffix(soPath, ".so")+".go", req, nil)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result["cached"] != true {
		t.Errorf("expected cached plugin result, got: %v", result)
	}
	if !called {
		t.Errorf("expected plugin Lookup to be called")
	}
}

type pluginMissingSymbol struct{}

func (pluginMissingSymbol) Lookup(name string) (plugin.Symbol, error) {
	return nil, errors.New("symbol not found")
}

func TestLoadPluginAndCall_LookupFails(t *testing.T) {
	original := loadPluginFunc
	defer func() { loadPluginFunc = original }()

	loadPluginFunc = func(path string) (pluginWithLookup, error) {
		return pluginMissingSymbol{}, nil
	}

	tmp := t.TempDir()
	soPath := filepath.Join(tmp, "missing.so")
	_ = os.WriteFile(soPath, []byte("fake"), 0644)

	req, _ := http.NewRequest(http.MethodGet, "/", nil)
	_, err := LoadPluginAndCall(strings.TrimSuffix(soPath, ".so")+".go", req, nil)

	if !errors.Is(err, ErrInvalidPlugin) {
		t.Errorf("expected ErrInvalidPlugin from missing symbol, got: %v", err)
	}
}
