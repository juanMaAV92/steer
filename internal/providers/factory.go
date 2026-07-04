// Package providers cablea la construcción de capacidades por contexto/cloud.
package providers

import (
	"context"
	"errors"
	"fmt"

	"github.com/juanMaAV92/steer/internal/config"
	"github.com/juanMaAV92/steer/internal/core"
	"github.com/juanMaAV92/steer/internal/providers/aws"
)

// ErrProviderNotImplemented indica que el cloud del contexto aún no tiene provider.
var ErrProviderNotImplemented = errors.New("provider not implemented")

// Provider agrupa las capacidades de un contexto; cachea la sesión del cloud.
type Provider interface {
	Deployer() (core.Deployer, error)
	// Registry() (core.Registry, error)  ← se añade en el hito registry
}

// ProviderFactory construye el bundle de un contexto. ctx permite cancelar la
// carga de sesión (SSO, red).
type ProviderFactory func(ctx context.Context, c config.Context) (Provider, error)

// NewProviderFactory devuelve la fábrica por defecto (AWS real; otros → error).
func NewProviderFactory() ProviderFactory {
	return func(ctx context.Context, c config.Context) (Provider, error) {
		switch c.Cloud {
		case "aws":
			return aws.NewProvider(ctx, c)
		default:
			return nil, fmt.Errorf("%w: %q", ErrProviderNotImplemented, c.Cloud)
		}
	}
}
