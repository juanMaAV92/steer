package tui

import (
	"time"

	"github.com/juanMaAV92/steer/internal/core"
)

type servicesMsg struct {
	services []core.ServiceStatus
	err      error
}

type reposMsg struct {
	repos    []core.Repository
	disabled bool // contexto sin bloque [images]
	err      error
}

type tagsMsg struct {
	repo string
	tags []core.ImageTag
	err  error
}

type tickMsg struct{}

type actionDoneMsg struct {
	msg string
	err error
}

type deployStartedMsg struct {
	steps  []string
	lastID string
	err    error
}

type deployPollMsg struct {
	events                    []core.ServiceEvent
	lastID                    string
	rollout                   core.RolloutState
	running, pending, desired int
	done, failed              bool
	err                       error
}

type deployPollTickMsg struct{}

// formTagsMsg trae los tags cargados para el picker del formulario de deploy.
type formTagsMsg struct {
	service string
	tags    []core.ImageTag
}

// tagVerdict es el resultado de validar un tag contra el registry.
type tagVerdict int

const (
	tagOK tagVerdict = iota
	tagNotFound
	tagRepoNotFound // el repo no existe: bloquea con mensaje propio
	tagSkipped      // sin [images] o registry con error: se despliega sin verificar
)

// tagValidatedMsg trae el veredicto de validateTagCmd para un service+tag dados.
type tagValidatedMsg struct {
	service string
	tag     string
	repo    string
	verdict tagVerdict
}

const refreshInterval = 15 * time.Second
