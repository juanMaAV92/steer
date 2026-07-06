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
// borde(0), título(1), prompt(2), estado(3 si hay), tags(…), botones, borde.
const (
	formContentX0 = 2 // columnas a la izquierda del contenido: borde(1) + padding(1)
)

// actionConfirmedMsg: el usuario confirmó una acción en el formulario inline.
type actionConfirmedMsg struct {
	kind      actionKind
	service   string
	input     string
	resources core.Resources // solo kind resize
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

	validating bool   // consulta HasTag en vuelo: botones y teclado inertes (esc cancela)
	errMsg     string // veredicto notFound: línea roja bajo el prompt

	resOpts  []core.ResourceOption // solo kind resize
	current  core.Resources        // combo actual del servicio (marca ● now)
	resField int                   // 0=cpu, 1=memoria, 2=botones
	cpuIdx   int
	memIdx   int
}

func newActionForm(kind actionKind, service string) *actionForm {
	return &actionForm{kind: kind, service: service, pick: -1}
}

// newResizeForm crea el formulario de resize preseleccionado en el combo actual.
func newResizeForm(service string, opts []core.ResourceOption, current core.Resources) *actionForm {
	f := &actionForm{kind: actionResize, service: service, resOpts: opts,
		current: current, resField: 0, pick: -1}
	for i, o := range opts {
		if o.CPUMilli == current.CPUMilli {
			f.cpuIdx = i
		}
	}
	f.memIdx = nearestIdx(opts[f.cpuIdx].MemoryMiB, current.MemoryMiB)
	return f
}

// nearestIdx devuelve el índice del valor más cercano a target.
func nearestIdx(vals []int, target int) int {
	best, bestDiff := 0, int(^uint(0)>>1)
	for i, v := range vals {
		diff := v - target
		if diff < 0 {
			diff = -diff
		}
		if diff < bestDiff {
			best, bestDiff = i, diff
		}
	}
	return best
}

// selectedResources devuelve el combo elegido en los pickers.
func (f actionForm) selectedResources() core.Resources {
	opt := f.resOpts[f.cpuIdx]
	return core.Resources{CPUMilli: opt.CPUMilli, MemoryMiB: opt.MemoryMiB[f.memIdx]}
}

// moveResField cambia el campo activo (cpu → memoria → botones), con wrap.
func (f *actionForm) moveResField(delta int) {
	f.resField = (f.resField + delta%3 + 3) % 3
}

// moveResValue cambia el valor del campo activo con wrap; al cambiar el tier de
// CPU la memoria salta a la válida más cercana del nuevo tier.
func (f *actionForm) moveResValue(delta int) {
	switch f.resField {
	case 0:
		prevMem := f.resOpts[f.cpuIdx].MemoryMiB[f.memIdx]
		n := len(f.resOpts)
		f.cpuIdx = (f.cpuIdx + delta%n + n) % n
		f.memIdx = nearestIdx(f.resOpts[f.cpuIdx].MemoryMiB, prevMem)
	case 1:
		mems := f.resOpts[f.cpuIdx].MemoryMiB
		n := len(mems)
		f.memIdx = (f.memIdx + delta%n + n) % n
	case 2:
		f.moveFocus(delta)
	}
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
		f.errMsg = ""
	case tea.KeyRunes:
		f.input += string(msg.Runes)
		f.pick = -1
		f.query = f.input
		f.errMsg = ""
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

// statusRows: fila opcional de estado (validating o error) entre prompt y tags.
func (f actionForm) statusRows() int {
	if f.validating || f.errMsg != "" {
		return 1
	}
	return 0
}

// buttonRow es la fila de los botones dentro del view: la geometría base
// (borde, título, prompt, estado) más las filas visibles del picker.
func (f actionForm) buttonRow() int {
	if f.kind == actionResize {
		return 4 // borde(0) título(1) cpu(2) mem(3) botones(4)
	}
	return 3 + f.statusRows() + len(f.visibleTags())
}

// tagAt devuelve el índice del tag en la fila row del view, o -1.
func (f actionForm) tagAt(row int) int {
	base := 3 + f.statusRows()
	n := len(f.visibleTags())
	if n == 0 || row < base || row >= base+n {
		return -1
	}
	return row - base
}

// moveFocus mueve el foco entre confirmar(0) y cancelar(1), con wrap.
func (f *actionForm) moveFocus(delta int) {
	f.focus = (f.focus + delta%2 + 2) % 2
}

func (f actionForm) ready() bool {
	return f.kind == actionRollback || f.kind == actionResize || f.input != ""
}

// labels devuelve las etiquetas de los botones (fuente única con el render).
func (f actionForm) labels() []string {
	switch f.kind {
	case actionScale:
		return []string{"Scale (↵)", "Cancel (esc)"}
	case actionRollback:
		return []string{"Confirm (↵)", "Cancel (esc)"}
	case actionResize:
		return []string{"Resize (↵)", "Cancel (esc)"}
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
	if f.kind == actionResize {
		return true, actionConfirmedMsg{kind: f.kind, service: f.service, resources: f.selectedResources()}
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

// resizeValueAt mapea (row, x) local del view a (campo, índice de valor); -1 si nada.
// Filas: cpu=2, memoria=3; los valores usan LabelAtColumn con gap 3 (" · ") y pad 0,
// tras el prefijo de 8 runas.
func (f actionForm) resizeValueAt(row, x int) (field, idx int) {
	if f.kind != actionResize {
		return -1, -1
	}
	var labels []string
	switch row {
	case 2:
		field = 0
		for _, o := range f.resOpts {
			labels = append(labels, render.CPULabel(o.CPUMilli))
		}
	case 3:
		field = 1
		for _, m := range f.resOpts[f.cpuIdx].MemoryMiB {
			labels = append(labels, render.MemLabel(m))
		}
	default:
		return -1, -1
	}
	idx = render.LabelAtColumn(labels, 0, 3, x-formContentX0-8)
	if idx < 0 {
		return -1, -1
	}
	return field, idx
}

// resizeRow renderiza una fila de picker de resize: prefijo alineado a 8 runas +
// valores unidos por " · ", el seleccionado con la barra de selección, el
// prefijo en Brand si el campo está activo, y un marcador "● now" si el valor
// seleccionado coincide con el combo actual del servicio.
func (f actionForm) resizeRow(prefix string, labels []string, selected int, isNow bool, active bool) string {
	parts := make([]string, len(labels))
	for i, l := range labels {
		if i == selected {
			parts[i] = lipgloss.NewStyle().Background(lipgloss.Color(render.SelectionBarColor)).Render(l)
		} else {
			parts[i] = l
		}
	}
	line := strings.Join(parts, " · ")
	if isNow {
		line += render.Success(" ● now")
	}
	if active {
		prefix = render.Brand(prefix)
	}
	return prefix + line
}

// view renderiza la caja del formulario: título, prompt, picker de tags (si
// aplica) y botones con foco.
func (f actionForm) view() string {
	if f.kind == actionResize {
		return f.viewResize()
	}
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
	if f.validating {
		rows = append(rows, render.Dim("validating tag…"))
	} else if f.errMsg != "" {
		rows = append(rows, render.Danger(f.errMsg))
	}
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

// viewResize renderiza el formulario de resize: título, picker de cpu, picker de
// memoria y botones. Los valores se unen exactamente con " · " (fuente única con
// resizeValueAt); el prefijo de la fila activa se pinta en Brand y el valor
// seleccionado con la barra de selección; "● now" marca el combo actual.
func (f actionForm) viewResize() string {
	opt := f.resOpts[f.cpuIdx]
	sel := f.selectedResources()

	cpuLabels := make([]string, len(f.resOpts))
	for i, o := range f.resOpts {
		cpuLabels[i] = render.CPULabel(o.CPUMilli)
	}
	memLabels := make([]string, len(opt.MemoryMiB))
	for i, m := range opt.MemoryMiB {
		memLabels[i] = render.MemLabel(m)
	}

	cpuRow := f.resizeRow("cpu:    ", cpuLabels, f.cpuIdx, sel.CPUMilli == f.current.CPUMilli && sel.MemoryMiB == f.current.MemoryMiB, f.resField == 0)
	memRow := f.resizeRow("memory: ", memLabels, f.memIdx, sel.CPUMilli == f.current.CPUMilli && sel.MemoryMiB == f.current.MemoryMiB, f.resField == 1)

	rows := []string{render.Bold("Resize"), cpuRow, memRow, render.ButtonsWithFocus(f.labels(), f.focus)}
	inner := strings.Join(rows, "\n")
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(render.BrandColor)).
		Padding(0, 1).
		Render(inner)
}
