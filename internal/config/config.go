// Package config carga y resuelve la configuración de Steer (steer.toml).
package config

import "github.com/BurntSushi/toml"

// Config es la raíz de steer.toml.
type Config struct {
	Providers Providers `toml:"providers"`
}

type Providers struct {
	AWS AWS `toml:"aws"`
}

type AWS struct {
	Environments map[string]Environment `toml:"environments"`
	Naming       Naming                 `toml:"naming"`
}

// Environment describe un entorno (dev/stg/prod...).
type Environment struct {
	Profile   string `toml:"profile"`
	AccountID string `toml:"account_id"`
	RoleARN   string `toml:"role_arn"`
	Writable  bool   `toml:"writable"`
}

// Naming define cómo resolver nombres cortos a recursos AWS reales.
type Naming struct {
	ClusterTemplate string `toml:"cluster_template"`
	ServiceTemplate string `toml:"service_template"`
}

// Load lee y parsea un steer.toml desde path.
func Load(path string) (*Config, error) {
	var cfg Config
	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}
