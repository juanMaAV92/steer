// Package config carga y resuelve la configuración de Steer (steer.toml).
package config

import "github.com/BurntSushi/toml"

// Config es la raíz de steer.toml.
type Config struct {
	DefaultContext string             `toml:"default_context"`
	Contexts       map[string]Context `toml:"contexts"`
}

// Load lee y parsea un steer.toml desde path.
func Load(path string) (*Config, error) {
	var cfg Config
	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}
