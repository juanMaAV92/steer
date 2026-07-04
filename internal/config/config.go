// Package config carga y resuelve la configuración de Steer (steer.toml).
package config

import (
	"fmt"
	"strings"

	"github.com/BurntSushi/toml"
)

// Config es la raíz de steer.toml.
type Config struct {
	DefaultContext string             `toml:"default_context"`
	Contexts       map[string]Context `toml:"contexts"`

	// hasLegacyProviders indica que el TOML crudo contiene claves del
	// esquema legacy [providers.*], detectado vía toml.MetaData.Undecoded().
	hasLegacyProviders bool
}

// Load lee y parsea un steer.toml desde path. Load NO valida invariantes
// globales (ver Validate); eso queda a cargo del llamador.
func Load(path string) (*Config, error) {
	var cfg Config
	md, err := toml.DecodeFile(path, &cfg)
	if err != nil {
		return nil, err
	}
	for _, k := range md.Undecoded() {
		if strings.HasPrefix(k.String(), "providers") {
			cfg.hasLegacyProviders = true
			break
		}
	}
	return &cfg, nil
}

// Validate comprueba invariantes globales del steer.toml: que existan
// contextos, que default_context (si está definido) apunte a uno existente,
// y que cada contexto individual sea válido.
func (c *Config) Validate() error {
	if len(c.Contexts) == 0 {
		if c.hasLegacyProviders {
			return fmt.Errorf("steer.toml uses the legacy [providers.*] schema; migrate to [contexts.*] (see 'steer config init' for the new format)")
		}
		return fmt.Errorf("steer.toml has no contexts; run 'steer config init'")
	}
	if c.DefaultContext != "" {
		if _, err := c.Context(c.DefaultContext); err != nil {
			return fmt.Errorf("default_context %q not found in contexts", c.DefaultContext)
		}
	}
	for _, ctx := range c.AllContexts() {
		if err := ctx.Validate(); err != nil {
			return err
		}
	}
	return nil
}
