// Package core define las interfaces agnósticas de capacidad que implementan
// los providers (AWS hoy; Azure/GCP en el futuro) y consumen CLI y TUI.
package core

import (
	"context"
	"time"
)

// ServiceEvent es un evento del servicio (mensajes de despliegue de ECS).
type ServiceEvent struct {
	ID      string
	At      time.Time
	Message string
	IsError bool
}

// ServiceStatus es el estado de un servicio/contenedor.
type ServiceStatus struct {
	Name    string
	Running int
	Desired int
	Pending int
	Status  string // estado del servicio (p.ej. ACTIVE)
	Tag     string // tag de imagen en uso (vacío si no se pudo resolver)
}

// RolloutState es el estado del despliegue activo, normalizado entre providers.
type RolloutState string

const (
	RolloutInProgress RolloutState = "IN_PROGRESS"
	RolloutCompleted  RolloutState = "COMPLETED"
	RolloutFailed     RolloutState = "FAILED"
)

// Deployment es el estado del despliegue activo (rollout) de un servicio.
type Deployment struct {
	Rollout RolloutState
	Running int
	Pending int
	Desired int
}

// StepLogger recibe mensajes de progreso de una operación (puede ser nil).
type StepLogger func(step string)

// Deployer despliega y consulta servicios de cómputo (ECS / Container Apps / Cloud Run).
type Deployer interface {
	ListServices(ctx context.Context) ([]ServiceStatus, error)
	CurrentTag(ctx context.Context, service string) (string, error)
	Deploy(ctx context.Context, service, tag string, log StepLogger) error
	Scale(ctx context.Context, service string, count int) error
	Rollback(ctx context.Context, service string) error
	DeploymentStatus(ctx context.Context, service string) (Deployment, error)
	// ServiceEvents devuelve los eventos del servicio, más recientes primero.
	ServiceEvents(ctx context.Context, service string) ([]ServiceEvent, error)
}
