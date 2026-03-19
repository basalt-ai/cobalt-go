// Package config handles loading and defaulting of cobalt.yaml configuration.
package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// JudgeConfig configures the LLM judge provider and model.
type JudgeConfig struct {
	// Model is the LLM model to use for judging (default: gpt-4o-mini).
	Model string `yaml:"model"`
	// Provider is the LLM provider (default: openai).
	Provider string `yaml:"provider"`
}

// Config holds all Cobalt configuration options.
type Config struct {
	// Judge configures the default LLM judge.
	Judge JudgeConfig `yaml:"judge"`
	// Concurrency is the default number of parallel items per experiment (default: 3).
	Concurrency int `yaml:"concurrency"`
	// Timeout is the default per-item deadline (default: 30s).
	Timeout duration `yaml:"timeout"`
	// TestDir is the directory to search for experiment files (default: ./experiments).
	TestDir string `yaml:"testDir"`
	// TestMatch is the glob patterns for experiment files.
	TestMatch []string `yaml:"testMatch"`
}

// duration is a yaml-unmarshalable time.Duration.
type duration time.Duration

func (d *duration) UnmarshalYAML(value *yaml.Node) error {
	dur, err := time.ParseDuration(value.Value)
	if err != nil {
		return err
	}
	*d = duration(dur)
	return nil
}

// ToDuration converts the wrapped duration to time.Duration.
func (d duration) ToDuration() time.Duration {
	return time.Duration(d)
}

// Default returns a Config with sensible defaults.
func Default() *Config {
	return &Config{
		Judge: JudgeConfig{
			Model:    "gpt-4o-mini",
			Provider: "openai",
		},
		Concurrency: 3,
		Timeout:     duration(30 * time.Second),
		TestDir:     "./experiments",
		TestMatch:   []string{"**/*.cobalt.go"},
	}
}

// Load reads cobalt.yaml from path and returns a Config merged with defaults.
func Load(path string) (*Config, error) {
	cfg := Default()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil // return defaults if file missing
		}
		return nil, fmt.Errorf("config: read %s: %w", path, err)
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("config: parse %s: %w", path, err)
	}

	// Apply defaults for unset fields.
	if cfg.Judge.Model == "" {
		cfg.Judge.Model = "gpt-4o-mini"
	}
	if cfg.Judge.Provider == "" {
		cfg.Judge.Provider = "openai"
	}
	if cfg.Concurrency <= 0 {
		cfg.Concurrency = 3
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = duration(30 * time.Second)
	}
	if cfg.TestDir == "" {
		cfg.TestDir = "./experiments"
	}
	if len(cfg.TestMatch) == 0 {
		cfg.TestMatch = []string{"**/*.cobalt.go"}
	}

	return cfg, nil
}
