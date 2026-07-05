package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/juanMaAV92/steer/internal/render"
)

// brandIcon es el glifo de marca del top bar.
const brandIcon = "⛵"

// topBar renderiza la barra de contexto: icono + wordmark + contexto a la izquierda
// y el estado writable/read-only alineado a la derecha (ancho de display = width).
func topBar(width int, cloud, env, cluster string, writable bool) string {
	state := render.Success("writable ●")
	if !writable {
		state = render.Warn("read-only ○")
	}
	left := brandIcon + " " + render.Brand("steer") + render.Dim(" · "+cloud+" · ") +
		render.Accent(env) + render.Dim(" (cluster: "+cluster+")")
	fill := width - lipgloss.Width(left) - lipgloss.Width(state)
	if fill < 1 {
		fill = 1
	}
	return left + strings.Repeat(" ", fill) + state
}

// hrule dibuja una regla horizontal tenue de ancho width.
func hrule(width int) string {
	if width < 1 {
		return ""
	}
	return render.Dim(strings.Repeat("─", width))
}

// vdivider dibuja el divisor vertical (una columna de │ tenues, height filas).
func vdivider(height int) string {
	col := render.Dim("│")
	rows := make([]string, height)
	for i := range rows {
		rows[i] = col
	}
	return strings.Join(rows, "\n")
}

// bottomBar muestra ayuda y, si hay, un aviso o estado que tiene prioridad visual.
func bottomBar(help, notice, status string) string {
	switch {
	case notice != "":
		return render.Warn(notice)
	case status != "":
		return render.Success(status)
	default:
		return render.Dim(help)
	}
}
