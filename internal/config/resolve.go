package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

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

// Context devuelve el contexto con nombre name (con Name poblado) o un error.
func (c *Config) Context(name string) (Context, error) {
	ctx, ok := c.Contexts[name]
	if !ok {
		return Context{}, fmt.Errorf("context %q not found in config", name)
	}
	ctx.Name = name
	return ctx, nil
}

// AllContexts devuelve los contextos ordenados alfabéticamente por nombre.
func (c *Config) AllContexts() []Context {
	names := make([]string, 0, len(c.Contexts))
	for n := range c.Contexts {
		names = append(names, n)
	}
	sort.Strings(names)
	out := make([]Context, 0, len(names))
	for _, n := range names {
		ctx := c.Contexts[n]
		ctx.Name = n
		out = append(out, ctx)
	}
	return out
}

// DefaultCtx devuelve el contexto por defecto: default_context, o el único, o error.
func (c *Config) DefaultCtx() (Context, error) {
	if c.DefaultContext != "" {
		return c.Context(c.DefaultContext)
	}
	if len(c.Contexts) == 1 {
		for n := range c.Contexts {
			return c.Context(n)
		}
	}
	return Context{}, fmt.Errorf("no default_context set and %d contexts available — pass --context", len(c.Contexts))
}

// ResolveContext resuelve por flag (si no vacío) o por defecto.
func (c *Config) ResolveContext(flag string) (Context, error) {
	if flag != "" {
		return c.Context(flag)
	}
	return c.DefaultCtx()
}
