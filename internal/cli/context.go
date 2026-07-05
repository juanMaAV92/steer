// Package cli contiene el armazón de la CLI (Cobra) y el contexto de aplicación.
package cli

import (
	"context"
	"errors"
	"fmt"

	"github.com/juanMaAV92/steer/internal/config"
	"github.com/juanMaAV92/steer/internal/core"
	"github.com/juanMaAV92/steer/internal/providers"
)

// AppContext es el estado compartido por todos los comandos.
type AppContext struct {
	Ctx     config.Context
	Config  *config.Config
	Factory providers.ProviderFactory

	provider providers.Provider // memoizado por comando
}

// IsProduction indica si el contexto activo es de solo lectura (prod).
func (a *AppContext) IsProduction() bool { return !a.Ctx.Writable }

// RequireWritable falla si el contexto activo es de solo lectura.
func (a *AppContext) RequireWritable() error {
	if !a.Ctx.Writable {
		return fmt.Errorf("context %q is read-only (writable=false)", a.Ctx.Name)
	}
	return nil
}

// Deployer construye (una vez) el provider del contexto activo y devuelve su Deployer.
func (a *AppContext) Deployer(ctx context.Context) (core.Deployer, error) {
	if a.provider == nil {
		p, err := a.Factory(ctx, a.Ctx)
		if err != nil {
			return nil, err
		}
		a.provider = p
	}
	return a.provider.Deployer()
}

// Registry construye (una vez) el provider y devuelve su Registry.
// core.ErrNoImagesConfig se traduce a un mensaje accionable con el snippet TOML.
func (a *AppContext) Registry(ctx context.Context) (core.Registry, error) {
	if a.provider == nil {
		p, err := a.Factory(ctx, a.Ctx)
		if err != nil {
			return nil, err
		}
		a.provider = p
	}
	reg, err := a.provider.Registry()
	if errors.Is(err, core.ErrNoImagesConfig) {
		return nil, fmt.Errorf("context %q has no images config; add to steer.toml:\n\n  [contexts.%s.images]\n  repo_template = \"myteam-{name}\"", a.Ctx.Name, a.Ctx.Name)
	}
	return reg, err
}
