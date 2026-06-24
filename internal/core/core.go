// Package core define las interfaces agnósticas de capacidad que implementan
// los providers (AWS hoy; Azure/GCP en el futuro) y consumen CLI y TUI.
package core

import "context"

// ServiceStatus es el estado de un servicio/contenedor.
type ServiceStatus struct {
	Name    string
	Running int
	Desired int
	Pending int
	Status  string // estado del servicio (p.ej. ACTIVE)
	Tag     string // tag de imagen en uso (vacío si no se pudo resolver)
}

// Deployer despliega y consulta servicios de cómputo (ECS / Container Apps / Cloud Run).
type Deployer interface {
	ListServices(ctx context.Context, cluster string) ([]ServiceStatus, error)
	CurrentTag(ctx context.Context, cluster, service string) (string, error)
	Deploy(ctx context.Context, cluster, service, tag string) error
	Scale(ctx context.Context, cluster, service string, count int) error
	Rollback(ctx context.Context, cluster, service string) error
}
