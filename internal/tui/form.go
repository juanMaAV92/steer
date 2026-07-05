// internal/tui/form.go
package tui

import (
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/juanMaAV92/steer/internal/core"
	"github.com/juanMaAV92/steer/internal/render"
)

// Geometría del formulario inline, fuente única para render y hit-testing:
// borde(0), título(1), prompt(2), tags(3..3+n-1), botones(3+n), borde.
const (
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
	focus   int             // 0 = confirmar, 1 = cancelar
	tags    []core.ImageTag // tags del repo hermano (solo kind deploy); nil = sin picker
	pick    int             // índice en visibleTags() rellenado por ↑↓; -1 = tecleando
	query   string          // lo tecleado por el usuario; distinto de input mientras hay pick,
	// para que ↑↓ siga filtrando sobre lo escrito y no sobre el tag ya elegido
}

func newActionForm(kind actionKind, service string) *actionForm {
	return &actionForm{kind: kind, service: service, pick: -1}
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
		f.pick = -1
		f.query = f.input
	case tea.KeyRunes:
		f.input += string(msg.Runes)
		f.pick = -1
		f.query = f.input
	}
}

// setTags habilita el picker (solo tiene efecto visual en deploy).
func (f *actionForm) setTags(tags []core.ImageTag) { f.tags = tags }

// visibleTags filtra los tags por lo tecleado (substring, máx 5 visibles). Usa
// query en vez de input para que, tras un ↑↓, no se autofiltre por el tag ya
// elegido y siga permitiendo ciclar entre todas las coincidencias del texto escrito.
func (f actionForm) visibleTags() []core.ImageTag {
	if f.kind != actionDeploy || len(f.tags) == 0 {
		return nil
	}
	q := strings.ToLower(f.query)
	var out []core.ImageTag
	for _, t := range f.tags {
		if q == "" || strings.Contains(strings.ToLower(t.Tag), q) {
			out = append(out, t)
		}
		if len(out) == 5 {
			break
		}
	}
	return out
}

// movePick mueve la selección del picker y rellena el input con el tag elegido.
func (f *actionForm) movePick(delta int) {
	vis := f.visibleTags()
	if len(vis) == 0 {
		return
	}
	// el primer ↓ entra en la lista; después se desplaza con clamp
	f.pick = min(max(f.pick+delta, 0), len(vis)-1)
	f.input = vis[f.pick].Tag
}

// buttonRow es la fila de los botones dentro del view: la geometría base
// (borde, título, prompt) más las filas visibles del picker.
func (f actionForm) buttonRow() int { return 3 + len(f.visibleTags()) }

// tagAt devuelve el índice del tag en la fila row del view, o -1.
func (f actionForm) tagAt(row int) int {
	n := len(f.visibleTags())
	if n == 0 || row < 3 || row >= 3+n {
		return -1
	}
	return row - 3
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
	if row != f.buttonRow() {
		return -1
	}
	return render.ButtonAtColumn(f.labels(), x-formContentX0)
}

// view renderiza la caja del formulario: título, prompt, picker de tags (si
// aplica) y botones con foco.
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
	rows := []string{render.Bold(title), prompt}
	if vis := f.visibleTags(); len(vis) > 0 {
		now := time.Now()
		for i, t := range vis {
			line := "  " + t.Tag + "  " + render.Age(t.PushedAt, now)
			if i == f.pick {
				line = lipgloss.NewStyle().Background(lipgloss.Color(render.SelectionBarColor)).Render(line)
			} else {
				line = render.Dim(line)
			}
			rows = append(rows, line)
		}
	}
	rows = append(rows, render.ButtonsWithFocus(f.labels(), f.focus))
	inner := strings.Join(rows, "\n")
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(render.BrandColor)).
		Padding(0, 1).
		Render(inner)
}
