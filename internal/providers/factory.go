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

// IsImplemented indica si un cloud tiene provider real. Fuente única: la fábrica
// y la UI (marca "(no impl.)") deben coincidir siempre.
func IsImplemented(cloud string) bool { return cloud == "aws" }

// Friendly traduce errores comunes del cloud a mensajes que enseñan el remedio;
// sin mapeo devuelve err.Error() tal cual. Fachada por-provider (AWS hoy).
func Friendly(err error) string {
	if err == nil {
		return "" // guard: la fachada es pública; evita panic si un llamador no chequea
	}
	if msg, ok := aws.FriendlyError(err); ok {
		return msg
	}
	return err.Error()
}

// Provider agrupa las capacidades de un contexto; cachea la sesión del cloud.
type Provider interface {
	Deployer() (core.Deployer, error)
	// Registry devuelve la capacidad de imágenes; core.ErrNoImagesConfig si el
	// contexto no tiene bloque [images].
	Registry() (core.Registry, error)
	// Logs devuelve la capacidad de lectura de logs del contexto. El origen se
	// descubre por servicio: core.ErrNoLogSource llega por operación, no aquí.
	Logs() (core.LogSource, error)
}

// ProviderFactory construye el bundle de un contexto. ctx permite cancelar la
// carga de sesión (SSO, red).
type ProviderFactory func(ctx context.Context, c config.Context) (Provider, error)

// NewProviderFactory devuelve la fábrica por defecto (AWS real; otros → error).
func NewProviderFactory() ProviderFactory {
	return func(ctx context.Context, c config.Context) (Provider, error) {
		if !IsImplemented(c.Cloud) {
			return nil, fmt.Errorf("%w: %q", ErrProviderNotImplemented, c.Cloud)
		}
		return aws.NewProvider(ctx, c)
	}
}
