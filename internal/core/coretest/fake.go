// Package coretest ofrece dobles de prueba de las interfaces de core.
package coretest

import (
	"context"

	"github.com/juanMaAV92/steer/internal/core"
)

// FakeDeployer es un Deployer en memoria para tests.
type FakeDeployer struct {
	Services    []core.ServiceStatus
	DeployErr   error
	DeployCalls []string // "cluster/service/tag" por cada Deploy
}

func (f *FakeDeployer) ListServices(_ context.Context, _ string) ([]core.ServiceStatus, error) {
	return f.Services, nil
}

func (f *FakeDeployer) Deploy(_ context.Context, cluster, service, tag string) error {
	f.DeployCalls = append(f.DeployCalls, cluster+"/"+service+"/"+tag)
	return f.DeployErr
}
