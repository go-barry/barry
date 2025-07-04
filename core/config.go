package core

import (
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	OutputDir    string `yaml:"outputDir"`
	CacheEnabled bool   `yaml:"cache"`
	DebugHeaders bool   `yaml:"debugHeaders"`
	DebugLogs    bool   `yaml:"debugLogs"`
}

func LoadConfig(path string) Config {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{
			OutputDir:    "./cache",
			CacheEnabled: false,
			DebugHeaders: false,
			DebugLogs:    false,
		}
	}

	var cfg Config
	yaml.Unmarshal(data, &cfg)

	if cfg.OutputDir == "" {
		cfg.OutputDir = "./cache"
	}

	return cfg
}
