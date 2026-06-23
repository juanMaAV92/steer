package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Env devuelve el entorno con nombre name o un error si no existe.
func (c *Config) Env(name string) (Environment, error) {
	env, ok := c.Providers.AWS.Environments[name]
	if !ok {
		return Environment{}, fmt.Errorf("environment %q not found in config", name)
	}
	return env, nil
}

// Cluster resuelve el nombre del cluster para un entorno.
func (n Naming) Cluster(env string) string {
	tmpl := n.ClusterTemplate
	if tmpl == "" {
		tmpl = "{env}-cluster"
	}
	return strings.ReplaceAll(tmpl, "{env}", env)
}

// Service resuelve el nombre real de un servicio a partir del nombre corto y el entorno.
func (n Naming) Service(env, name string) string {
	tmpl := n.ServiceTemplate
	if tmpl == "" {
		tmpl = "{name}"
	}
	tmpl = strings.ReplaceAll(tmpl, "{env}", env)
	return strings.ReplaceAll(tmpl, "{name}", name)
}

// candidatePaths lista las rutas donde se busca steer.toml, en orden de prioridad.
func candidatePaths(cwd, home string) []string {
	return []string{
		filepath.Join(cwd, "steer.toml"),
		filepath.Join(home, ".config", "steer", "steer.toml"),
	}
}

// Find localiza el primer steer.toml existente (cwd, luego ~/.config/steer).
func Find() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	for _, p := range candidatePaths(cwd, home) {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	return "", fmt.Errorf("no steer.toml found (looked in ./ and ~/.config/steer/)")
}
