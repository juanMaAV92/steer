// internal/tui/form.go
package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/juanMaAV92/steer/internal/render"
)

// Geometría del formulario inline, fuente única para render y hit-testing:
// borde(0), título(1), prompt(2), botones(3), borde(4).
const (
	formButtonRow = 3 // fila 0-based de los botones dentro del view del formulario
	formContentX0 = 2 // columnas a la izquierda del contenido: borde(1) + padding(1)
)

// actionConfirmedMsg: el usuario confirmó una acción en el formulario inline.
type actionConfirmedMsg struct {
	kind    actionKind
	service string
	input   string
}

// actionForm es el formulario inline de deploy/scale/rollback que se dibuja
// dentro del panel Details, bajo la fila de botones de acción. Reemplaza al
// modal centrado: no es overlay, el click fuera no lo cierra.
type actionForm struct {
	kind    actionKind
	service string
	input   string
	focus   int // 0 = confirmar, 1 = cancelar
}

func newActionForm(kind actionKind, service string) *actionForm {
	return &actionForm{kind: kind, service: service}
}

func (f *actionForm) typeKey(msg tea.KeyMsg) {
	if f.kind == actionRollback {
		return
	}
	switch msg.Type {
	case tea.KeyBackspace:
		if n := len(f.input); n > 0 {
			f.input = f.input[:n-1]
		}
	case tea.KeyRunes:
		f.input += string(msg.Runes)
	}
}

// moveFocus mueve el foco entre confirmar(0) y cancelar(1), con wrap.
func (f *actionForm) moveFocus(delta int) {
	f.focus = (f.focus + delta%2 + 2) % 2
}

func (f actionForm) ready() bool {
	return f.kind == actionRollback || f.input != ""
}

// labels devuelve las etiquetas de los botones (fuente única con el render).
func (f actionForm) labels() []string {
	switch f.kind {
	case actionScale:
		return []string{"Scale (↵)", "Cancel (esc)"}
	case actionRollback:
		return []string{"Confirm (↵)", "Cancel (esc)"}
	default:
		return []string{"Deploy (↵)", "Cancel (esc)"}
	}
}

// activate ejecuta el botón enfocado: cancelar cierra sin resultado; confirmar
// emite actionConfirmedMsg solo si el formulario está listo (si no, sigue abierto).
func (f actionForm) activate() (bool, tea.Msg) {
	if f.focus == 1 {
		return true, nil
	}
	if !f.ready() {
		return false, nil
	}
	return true, actionConfirmedMsg{kind: f.kind, service: f.service, input: f.input}
}

// activateIndex enfoca el botón idx y lo ejecuta (ruta del click).
func (f *actionForm) activateIndex(idx int) (bool, tea.Msg) {
	f.focus = idx
	return f.activate()
}

// buttonAt devuelve el índice del botón bajo la coordenada (row, x) local al
// view del formulario, o -1 si no cae en ninguno.
func (f actionForm) buttonAt(row, x int) int {
	if row != formButtonRow {
		return -1
	}
	return render.ButtonAtColumn(f.labels(), x-formContentX0)
}

// view renderiza la caja del formulario: título, prompt y botones con foco.
func (f actionForm) view() string {
	var title, prompt string
	switch f.kind {
	case actionDeploy:
		title = "Deploy"
		prompt = "image tag:  " + render.Accent(f.input) + "_"
	case actionScale:
		title = "Scale"
		prompt = "desired count:  " + render.Accent(f.input) + "_"
	case actionRollback:
		title = "Roll back?"
		prompt = render.Dim("This reverts to the previous revision.")
	}
	inner := render.Bold(title) + "\n" + prompt + "\n" +
		render.ButtonsWithFocus(f.labels(), f.focus)
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(render.BrandColor)).
		Padding(0, 1).
		Render(inner)
}
