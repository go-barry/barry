package core

import (
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	OutputDir    string `yaml:"outputDir"`
	CacheEnabled bool   `yaml:"cache"`
	DebugHeaders bool   `yaml:"debugHeaders"`
}

func LoadConfig(path string) Config {
	data, err := os.ReadFile(path)
	if err != nil {
		// Default config if file doesn't exist
		return Config{
			OutputDir:    "./out",
			CacheEnabled: false,
			DebugHeaders: false,
		}
	}

	var cfg Config
	yaml.Unmarshal(data, &cfg)

	// Set defaults if missing
	if cfg.OutputDir == "" {
		cfg.OutputDir = "./out"
	}

	return cfg
}
