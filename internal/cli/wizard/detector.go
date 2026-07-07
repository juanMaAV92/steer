// Package wizard implementa el flujo de onboarding (config init/add) sobre un
// Detector por-provider: el flujo es agnóstico de cloud.
package wizard

import (
	"context"

	"github.com/juanMaAV92/steer/internal/config"
)

// Detector descubre credenciales y destinos en un cloud concreto (AWS hoy;
// GCP/Azure implementarán el suyo cuando lleguen sus providers).
type Detector interface {
	Profiles() ([]string, error)
	Clusters(ctx context.Context, profile, region string) ([]string, error)
	// SmokeTest construye el provider del contexto y devuelve cuántos servicios ve.
	SmokeTest(ctx context.Context, c config.Context) (int, error)
}
