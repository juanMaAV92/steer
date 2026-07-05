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

const refreshInterval = 15 * time.Second
