package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// AddContext valida y agrega un contexto; el primero se vuelve default_context.
func (c *Config) AddContext(ctx Context) error {
	if err := ctx.Validate(); err != nil {
		return err
	}
	if c.Contexts == nil {
		c.Contexts = map[string]Context{}
	}
	if _, ok := c.Contexts[ctx.Name]; ok {
		return fmt.Errorf("context %q already exists", ctx.Name)
	}
	name := ctx.Name
	ctx.Name = "" // la clave del mapa es la fuente del nombre
	c.Contexts[name] = ctx
	if c.DefaultContext == "" {
		c.DefaultContext = name
	}
	return nil
}

// RemoveContext elimina un contexto; si era el default, reasigna al primero
// alfabético restante (o lo deja vacío si no queda ninguno).
func (c *Config) RemoveContext(name string) (wasDefault bool, err error) {
	if _, ok := c.Contexts[name]; !ok {
		return false, fmt.Errorf("context %q not found", name)
	}
	delete(c.Contexts, name)
	if c.DefaultContext != name {
		return false, nil
	}
	c.DefaultContext = ""
	names := make([]string, 0, len(c.Contexts))
	for n := range c.Contexts {
		names = append(names, n)
	}
	sort.Strings(names)
	if len(names) > 0 {
		c.DefaultContext = names[0]
	}
	return true, nil
}

// GlobalPath es la ruta global de config (~/.config/steer/steer.toml).
func GlobalPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "steer", "steer.toml"), nil
}

// Write serializa la config de forma determinista (default primero, contextos
// alfabéticos) con permisos 0600, creando el directorio si falta. Los comentarios
// de un archivo previo NO se preservan.
func (c *Config) Write(path string) error {
	var b strings.Builder
	if c.DefaultContext != "" {
		fmt.Fprintf(&b, "default_context = %s\n\n", strconv.Quote(c.DefaultContext))
	}
	for i, ctx := range c.AllContexts() {
		if i > 0 {
			b.WriteString("\n")
		}
		fmt.Fprintf(&b, "[contexts.%s]\n", ctx.Name)
		writeKV(&b, "cloud", ctx.Cloud)
		writeKV(&b, "profile", ctx.Profile)
		writeKV(&b, "account_id", ctx.AccountID)
		writeKV(&b, "role_arn", ctx.RoleARN)
		writeKV(&b, "region", ctx.Region)
		writeKV(&b, "project", ctx.Project)
		writeKV(&b, "subscription", ctx.Subscription)
		writeKV(&b, "cluster", ctx.Cluster)
		writeKV(&b, "service_template", ctx.ServiceTemplate)
		fmt.Fprintf(&b, "writable = %t\n", ctx.Writable)
		if ctx.Images != nil {
			fmt.Fprintf(&b, "\n  [contexts.%s.images]\n", ctx.Name)
			fmt.Fprintf(&b, "  repo_template = %s\n", strconv.Quote(ctx.Images.RepoTemplate))
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(b.String()), 0o600)
}

// writeKV emite `clave = "valor"` omitiendo los vacíos (campos opcionales).
func writeKV(b *strings.Builder, key, val string) {
	if val == "" {
		return
	}
	fmt.Fprintf(b, "%s = %s\n", key, strconv.Quote(val))
}
