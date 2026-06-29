package config

import (
	"fmt"
	"strings"
)

// Context es un destino conmutable: una credencial + un cluster.
type Context struct {
	Name            string `toml:"-"` // = clave del mapa [contexts.<name>]
	Cloud           string `toml:"cloud"`
	Profile         string `toml:"profile"`      // AWS
	AccountID       string `toml:"account_id"`   // AWS (opcional)
	RoleARN         string `toml:"role_arn"`     // AWS (opcional)
	Region          string `toml:"region"`       // AWS/GCP (opcional)
	Project         string `toml:"project"`      // GCP
	Subscription    string `toml:"subscription"` // Azure
	Cluster         string `toml:"cluster"`
	ServiceTemplate string `toml:"service_template"`
	Writable        bool   `toml:"writable"`
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
	return nil
}
