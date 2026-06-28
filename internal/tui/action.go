// internal/tui/action.go
package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/juanMaAV92/steer/internal/render"
)

// action es el overlay de input para deploy/scale/rollback.
// (actionKind y sus constantes ya existen en model.go y se reutilizan;
// se eliminará el viejo pendingAction al borrar model.go en la Task 8.)
type action struct {
	kind    actionKind
	service string
	input   string
	active  bool
}

func (a *action) open(kind actionKind, service string) {
	a.kind = kind
	a.service = service
	a.input = ""
	a.active = true
}

func (a *action) close() { a.active = false }

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

func (a action) view() string {
	switch a.kind {
	case actionRollback:
		return "Roll back " + render.Bold(a.service) + " to previous revision?\n" +
			render.Dim("enter to confirm · esc to cancel")
	case actionDeploy:
		return "Deploy " + render.Bold(a.service) + " — image tag: " + render.Accent(a.input) + "\n" +
			render.Dim("type the tag · enter to deploy · esc to cancel")
	case actionScale:
		return "Scale " + render.Bold(a.service) + " — desired count: " + render.Accent(a.input) + "\n" +
			render.Dim("type a number · enter to scale · esc to cancel")
	}
	return ""
}
