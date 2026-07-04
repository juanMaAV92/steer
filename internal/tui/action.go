// internal/tui/action.go
package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/juanMaAV92/steer/internal/render"
)

// action es el overlay de input para deploy/scale/rollback.
type action struct {
	kind    actionKind
	service string
	input   string
}

func (a *action) open(kind actionKind, service string) {
	a.kind = kind
	a.service = service
	a.input = ""
}

func (a *action) typeKey(msg tea.KeyMsg) {
	if a.kind == actionRollback {
		return
	}
	switch msg.Type {
	case tea.KeyBackspace:
		if n := len(a.input); n > 0 {
			a.input = a.input[:n-1]
		}
	case tea.KeyRunes:
		a.input += string(msg.Runes)
	}
}

func (a action) ready() bool {
	return a.kind == actionRollback || a.input != ""
}

// modalView renderiza el diálogo de acción como una caja centrada en un área width×height.
func (a action) modalView(width, height int) string {
	var title, body, confirm string
	switch a.kind {
	case actionDeploy:
		title = "Deploy " + a.service
		body = "image tag:  " + render.Accent(a.input) + "_"
		confirm = "Deploy (↵)"
	case actionScale:
		title = "Scale " + a.service
		body = "desired count:  " + render.Accent(a.input) + "_"
		confirm = "Scale (↵)"
	case actionRollback:
		title = "Roll back " + a.service + "?"
		body = render.Dim("This reverts to the previous revision.")
		confirm = "Confirm (↵)"
	}
	inner := render.Bold(title) + "\n\n" + body + "\n\n" +
		render.Buttons([]string{confirm, "Cancel (esc)"})
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(render.BrandColor)).
		Padding(1, 2).
		Render(inner)
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, box)
}
