// Package coretest ofrece dobles de prueba de las interfaces de core.
package coretest

import (
	"context"
	"strconv"

	"github.com/juanMaAV92/steer/internal/core"
)

// FakeDeployer es un Deployer en memoria para tests.
type FakeDeployer struct {
	Services        []core.ServiceStatus
	CurrentTagValue string
	DeployErr       error
	DeploymentValue core.Deployment     // devuelto por DeploymentStatus
	Events          []core.ServiceEvent // devuelto por ServiceEvents

	DeployCalls   []string // "cluster/service/tag"
	ScaleCalls    []string // "cluster/service/count"
	RollbackCalls []string // "cluster/service"
}

func (f *FakeDeployer) ListServices(_ context.Context, _ string) ([]core.ServiceStatus, error) {
	return f.Services, nil
}

func (f *FakeDeployer) CurrentTag(_ context.Context, _, _ string) (string, error) {
	return f.CurrentTagValue, nil
}

func (f *FakeDeployer) Deploy(_ context.Context, cluster, service, tag string, log core.StepLogger) error {
	if log != nil {
		log("deploying")
	}
	f.DeployCalls = append(f.DeployCalls, cluster+"/"+service+"/"+tag)
	return f.DeployErr
}

func (f *FakeDeployer) Scale(_ context.Context, cluster, service string, count int) error {
	f.ScaleCalls = append(f.ScaleCalls, cluster+"/"+service+"/"+strconv.Itoa(count))
	return nil
}

func (f *FakeDeployer) Rollback(_ context.Context, cluster, service string) error {
	f.RollbackCalls = append(f.RollbackCalls, cluster+"/"+service)
	return nil
}

func (f *FakeDeployer) DeploymentStatus(_ context.Context, _, _ string) (core.Deployment, error) {
	return f.DeploymentValue, nil
}

func (f *FakeDeployer) ServiceEvents(_ context.Context, _, _ string) ([]core.ServiceEvent, error) {
	return f.Events, nil
}
