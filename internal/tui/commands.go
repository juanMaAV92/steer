package tui

import (
	"context"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/juanMaAV92/steer/internal/core"
)

func tickCmd() tea.Cmd {
	return tea.Tick(refreshInterval, func(time.Time) tea.Msg { return tickMsg{} })
}

func deployTickCmd() tea.Cmd {
	return tea.Tick(3*time.Second, func(time.Time) tea.Msg { return deployPollTickMsg{} })
}

func logsTickCmd(gen int) tea.Cmd {
	return tea.Tick(3*time.Second, func(time.Time) tea.Msg { return logsTickMsg{gen: gen} })
}

func startDeployCmd(ctx context.Context, dep core.Deployer, service, tag string) tea.Cmd {
	return func() tea.Msg {
		baseline := ""
		if evs, err := dep.ServiceEvents(ctx, service); err == nil && len(evs) > 0 {
			baseline = evs[0].ID
		}
		var steps []string
		err := dep.Deploy(ctx, service, tag, func(s string) { steps = append(steps, s) })
		return deployStartedMsg{steps: steps, lastID: baseline, err: err}
	}
}

// startResizeCmd arranca el resize (nueva revisión con recursos + rollout) y
// devuelve el mismo deployStartedMsg del flujo de deploy, para compartir el
// watch en vivo (Events + poll).
func startResizeCmd(ctx context.Context, dep core.Deployer, service string, res core.Resources) tea.Cmd {
	return func() tea.Msg {
		baseline := ""
		if evs, err := dep.ServiceEvents(ctx, service); err == nil && len(evs) > 0 {
			baseline = evs[0].ID
		}
		var steps []string
		err := dep.Resize(ctx, service, res, func(s string) { steps = append(steps, s) })
		return deployStartedMsg{steps: steps, lastID: baseline, err: err}
	}
}

func deployPollCmd(ctx context.Context, dep core.Deployer, service, lastID string) tea.Cmd {
	return func() tea.Msg {
		var fresh []core.ServiceEvent
		newLast := lastID
		if evs, err := dep.ServiceEvents(ctx, service); err == nil {
			for _, e := range evs {
				if e.ID == lastID {
					break
				}
				fresh = append(fresh, e)
			}
			if len(evs) > 0 {
				newLast = evs[0].ID
			}
		}
		d, err := dep.DeploymentStatus(ctx, service)
		return deployPollMsg{
			events: fresh, lastID: newLast,
			rollout: d.Rollout, running: d.Running, pending: d.Pending, desired: d.Desired,
			done:   d.Rollout == core.RolloutCompleted && d.Running >= d.Desired,
			failed: d.Rollout == core.RolloutFailed,
			err:    err,
		}
	}
}
