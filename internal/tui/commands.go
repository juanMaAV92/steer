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

func startDeployCmd(dep core.Deployer, cluster, service, tag string) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		baseline := ""
		if evs, err := dep.ServiceEvents(ctx, cluster, service); err == nil && len(evs) > 0 {
			baseline = evs[0].ID
		}
		var steps []string
		err := dep.Deploy(ctx, cluster, service, tag, func(s string) { steps = append(steps, s) })
		return deployStartedMsg{steps: steps, lastID: baseline, err: err}
	}
}

func deployPollCmd(dep core.Deployer, cluster, service, lastID string) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		var fresh []core.ServiceEvent
		newLast := lastID
		if evs, err := dep.ServiceEvents(ctx, cluster, service); err == nil {
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
		d, err := dep.DeploymentStatus(ctx, cluster, service)
		return deployPollMsg{
			events: fresh, lastID: newLast,
			rollout: d.Rollout, running: d.Running, pending: d.Pending, desired: d.Desired,
			done:   d.Rollout == core.RolloutCompleted && d.Running >= d.Desired,
			failed: d.Rollout == core.RolloutFailed,
			err:    err,
		}
	}
}
