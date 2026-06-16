// Package core define las interfaces agnósticas de capacidad que implementan
// los providers (AWS hoy; Azure/GCP en el futuro) y consumen CLI y TUI.
package core

import "context"

// ServiceStatus es el estado de un servicio/contenedor.
type ServiceStatus struct {
	Name    string
	Running int
	Desired int
}

// Deployer despliega y consulta servicios de cómputo (ECS / Container Apps / Cloud Run).
type Deployer interface {
	ListServices(ctx context.Context, cluster string) ([]ServiceStatus, error)
	Deploy(ctx context.Context, cluster, service, tag string) error
}
