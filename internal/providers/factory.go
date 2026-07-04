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

// DeployerFactory construye un Deployer para un contexto (o un error).
type DeployerFactory func(ctx config.Context) (core.Deployer, error)

// NewDeployerFactory devuelve la fábrica por defecto (AWS real; otros → error).
func NewDeployerFactory() DeployerFactory {
	return func(c config.Context) (core.Deployer, error) {
		switch c.Cloud {
		case "aws":
			cfg, err := aws.LoadConfigForContext(context.Background(), c)
			if err != nil {
				return nil, err
			}
			return aws.NewDeployer(cfg, c.Cluster), nil
		default:
			return nil, fmt.Errorf("%w: %q", ErrProviderNotImplemented, c.Cloud)
		}
	}
}
