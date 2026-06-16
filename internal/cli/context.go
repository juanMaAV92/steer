// Package cli contiene el armazón de la CLI (Cobra) y el contexto de aplicación.
package cli

import (
	"fmt"

	"github.com/juanMaAV92/steer/internal/config"
)

// AppContext es el estado compartido por todos los comandos.
type AppContext struct {
	EnvName string
	Env     config.Environment
	Config  *config.Config
}

// IsProduction indica si el entorno activo es prod.
func (a *AppContext) IsProduction() bool { return a.EnvName == "prod" }

// RequireWritable falla si el entorno activo es de solo lectura.
func (a *AppContext) RequireWritable() error {
	if !a.Env.Writable {
		return fmt.Errorf("environment %q is read-only (writable=false)", a.EnvName)
	}
	return nil
}
