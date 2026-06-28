package tui

import (
	"time"

	"github.com/juanMaAV92/steer/internal/core"
)

type servicesMsg struct {
	services []core.ServiceStatus
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
	rollout                   string
	running, pending, desired int
	done, failed              bool
	err                       error
}

type deployPollTickMsg struct{}

const refreshInterval = 15 * time.Second
