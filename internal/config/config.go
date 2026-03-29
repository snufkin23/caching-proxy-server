// Package config provides config for the caching proxy server.
package config

import (
	"fmt"
	"time"

	"github.com/caarlos0/env/v11"
)

// Config holds all configuration for the cache proxy server
type Config struct {
	Port string `env:"PORT" envDefault:"8080"`

	CacheTTL     time.Duration `env:"CACHE_TTL" envDefault:"5m"`
	MaxCacheSize int64         `env:"MAX_CACHE_SIZE" envDefault:"104857600"`

	ReadTimeout  time.Duration `env:"READ_TIMEOUT" envDefault:"10s"`
	WriteTimeout time.Duration `env:"WRITE_TIMEOUT" envDefault:"30s"`
	IdleTimeout  time.Duration `env:"IDLE_TIMEOUT" envDefault:"120s"`

	ShutdownTimeout time.Duration `env:"SHUTDOWN_TIMEOUT" envDefault:"10s"`

	UpstreamTimeout time.Duration `env:"UPSTREAM_TIMEOUT" envDefault:"30s"`
}

// Load reads configuration from environment variables
func Load() (*Config, error) {
	var cfg Config
	if err := env.Parse(&cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	return &cfg, nil
}
