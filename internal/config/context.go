package config

import (
	"fmt"
	"strings"
)

// ImagesConfig es el bloque [contexts.<n>.images]: la capacidad de registry.
type ImagesConfig struct {
	RepoTemplate string `toml:"repo_template"`
}

// Context es un destino conmutable: una credencial + un cluster.
type Context struct {
	Name            string        `toml:"-"` // = clave del mapa [contexts.<name>]
	Cloud           string        `toml:"cloud"`
	Profile         string        `toml:"profile"`      // AWS
	AccountID       string        `toml:"account_id"`   // AWS (opcional)
	RoleARN         string        `toml:"role_arn"`     // AWS (opcional)
	Region          string        `toml:"region"`       // AWS/GCP (opcional)
	Project         string        `toml:"project"`      // GCP
	Subscription    string        `toml:"subscription"` // Azure
	Cluster         string        `toml:"cluster"`
	ServiceTemplate string        `toml:"service_template"`
	Writable        bool          `toml:"writable"`
	Images          *ImagesConfig `toml:"images"` // capacidad registry (opcional)
}

// ServiceName resuelve un nombre corto al nombre real vía service_template.
// Sin template, devuelve el nombre tal cual.
func (c Context) ServiceName(short string) string {
	if c.ServiceTemplate == "" {
		return short
	}
	return strings.ReplaceAll(c.ServiceTemplate, "{name}", short)
}

// Prefix es el prefijo a ocultar en la lista (= ServiceName("")).
func (c Context) Prefix() string { return c.ServiceName("") }

// RepoName resuelve un nombre corto al repo real vía images.repo_template.
// Sin bloque images (o sin template), devuelve el nombre tal cual.
func (c Context) RepoName(short string) string {
	if c.Images == nil || c.Images.RepoTemplate == "" {
		return short
	}
	return strings.ReplaceAll(c.Images.RepoTemplate, "{name}", short)
}

// RepoPrefix es el prefijo de repos a ocultar en la lista (= RepoName("")).
func (c Context) RepoPrefix() string {
	if c.Images == nil {
		return ""
	}
	return c.RepoName("")
}

// Validate comprueba los campos mínimos del contexto.
func (c Context) Validate() error {
	if c.Cloud == "" {
		return fmt.Errorf("context %q: missing cloud", c.Name)
	}
	if c.Cluster == "" {
		return fmt.Errorf("context %q: missing cluster", c.Name)
	}
	if c.Cloud == "aws" && c.Profile == "" {
		return fmt.Errorf("context %q: aws context needs a profile", c.Name)
	}
	if c.Images != nil {
		if c.Images.RepoTemplate == "" {
			return fmt.Errorf("context %q: images block needs repo_template", c.Name)
		}
		if !strings.Contains(c.Images.RepoTemplate, "{name}") {
			return fmt.Errorf("context %q: images.repo_template must contain {name}", c.Name)
		}
	}
	return nil
}
