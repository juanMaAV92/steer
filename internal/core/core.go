// Package core define las interfaces agnósticas de capacidad que implementan
// los providers (AWS hoy; Azure/GCP en el futuro) y consumen CLI y TUI.
package core

import (
	"context"
	"errors"
	"strings"
	"time"
)

// ServiceEvent es un evento del servicio (mensajes de despliegue de ECS).
type ServiceEvent struct {
	ID      string
	At      time.Time
	Message string
	IsError bool
}

// Resources son los recursos de cómputo de un servicio, en unidades agnósticas.
type Resources struct {
	CPUMilli  int // mili-vCPU: 1000 = 1 vCPU
	MemoryMiB int
}

// ResourceOption es un tier de CPU con sus memorias válidas, ordenadas ascendente.
type ResourceOption struct {
	CPUMilli  int
	MemoryMiB []int
}

// ServiceStatus es el estado de un servicio/contenedor.
type ServiceStatus struct {
	Name      string
	Running   int
	Desired   int
	Pending   int
	Status    string    // estado del servicio (p.ej. ACTIVE)
	Tag       string    // tag de imagen en uso (vacío si no se pudo resolver)
	Resources Resources // recursos de la task; cero = desconocido (p. ej. EC2)
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
	// Resize registra una nueva revisión con los recursos y actualiza el servicio
	// (rollout). El Rollback existente revierte también los recursos.
	Resize(ctx context.Context, service string, res Resources, log StepLogger) error
	// ResourceOptions devuelve la tabla de combos válidos del provider, por CPU
	// ascendente. Estática: no consulta el cloud.
	ResourceOptions() []ResourceOption
}

// ErrNoImagesConfig indica que el contexto no tiene bloque [images]; la
// capacidad está deshabilitada, no es un fallo del cloud.
var ErrNoImagesConfig = errors.New("images not configured for this context")

// ErrRepoNotFound indica que el repositorio no existe en el registry. A diferencia
// de un fallo transitorio, es una respuesta definitiva: el deploy debe bloquearse.
var ErrRepoNotFound = errors.New("repository not found in registry")

// Repository es un repositorio de imágenes del registry del contexto.
type Repository struct {
	Name string // nombre real (con prefijo); la UI lo acorta con RepoPrefix
}

// ImageTag es una imagen de contenedor etiquetada y desplegable.
type ImageTag struct {
	Tag       string
	Digest    string // completo; la UI lo acorta con render.ShortDigest
	SizeBytes int64
	PushedAt  time.Time
}

// Registry lista repositorios e imágenes del registry del contexto
// (ECR / Artifact Registry / ACR). Solo lectura.
type Registry interface {
	// ListRepositories devuelve los repos del prefijo del contexto, alfanuméricos.
	ListRepositories(ctx context.Context) ([]Repository, error)
	// ListTags devuelve solo imágenes con tag desplegables (sin manifiestos
	// colgantes, attestations ni firmas), más recientes primero, tope 50.
	ListTags(ctx context.Context, repo string) ([]ImageTag, error)
	// HasTag verifica si el tag existe en el repo. Consulta puntual: no depende
	// del tope de ListTags (valida tags viejos que el picker no muestra).
	HasTag(ctx context.Context, repo, tag string) (bool, error)
}

// ErrNoLogSource indica que el servicio no expone logs legibles por steer
// (driver de logs no soportado o sin logConfiguration). No es un fallo del cloud.
var ErrNoLogSource = errors.New("no log source for this service")

// LogLine es una línea de log de un servicio.
type LogLine struct {
	At        time.Time
	Container string // vacío si la task tiene un solo contenedor
	Message   string
}

// LogPage es un lote de líneas + cursor opaco para continuar leyendo.
type LogPage struct {
	Lines  []LogLine // orden cronológico ascendente
	Cursor string
}

// LogSource lee logs de un servicio (CloudWatch Logs / Cloud Logging / Log
// Analytics). El cursor es opaco: lo define cada provider; el contrato solo
// exige que FollowLogs(cursor) no repita ni pierda líneas.
type LogSource interface {
	// TailLogs devuelve las últimas `limit` líneas del servicio.
	TailLogs(ctx context.Context, service string, limit int) (LogPage, error)
	// FollowLogs devuelve las líneas posteriores al cursor.
	FollowLogs(ctx context.Context, service string, cursor string) (LogPage, error)
}

// IsProvisioningFailure detecta eventos de fallo de aprovisionamiento en el texto
// que reporta el provider. Heurística documentada por provider (ECS hoy): un
// rollout que acumula estos eventos está atascado reintentando (p. ej. un tag
// que no existe) y ECS no lo reporta como FAILED sin circuit breaker.
func IsProvisioningFailure(msg string) bool {
	m := strings.ToLower(msg)
	return strings.Contains(m, "cannotpullcontainererror") ||
		strings.Contains(m, "unable to place a task")
}
