package tui

import (
	"sort"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/juanMaAV92/steer/internal/core"
	"github.com/juanMaAV92/steer/internal/render"
)

// sidebar es la columna izquierda con secciones apiladas por dominio.
type sidebar struct {
	services      []core.ServiceStatus
	cursor        int
	focused       bool
	width, height int
	prefix        string // prefijo a ocultar en la visualización (ej. "nao-v2-dev-")
}

func newSidebar() sidebar { return sidebar{} }

// setServices guarda los servicios y los ordena alfabéticamente por nombre de visualización.
func (s *sidebar) setServices(svc []core.ServiceStatus) {
	// copiar para no mutar el slice original
	sorted := make([]core.ServiceStatus, len(svc))
	copy(sorted, svc)
	sort.SliceStable(sorted, func(i, j int) bool {
		di := strings.ToLower(strings.TrimPrefix(sorted[i].Name, s.prefix))
		dj := strings.ToLower(strings.TrimPrefix(sorted[j].Name, s.prefix))
		return di < dj
	})
	s.services = sorted
	if s.cursor >= len(s.services) {
		s.cursor = max(0, len(s.services)-1)
	}
}

func (s *sidebar) moveDown() {
	if s.cursor < len(s.services)-1 {
		s.cursor++
	}
}

func (s *sidebar) moveUp() {
	if s.cursor > 0 {
		s.cursor--
	}
}

func (s *sidebar) selectIndex(i int) {
	if i >= 0 && i < len(s.services) {
		s.cursor = i
	}
}

func (s sidebar) selected() (core.ServiceStatus, bool) {
	if s.cursor < 0 || s.cursor >= len(s.services) {
		return core.ServiceStatus{}, false
	}
	return s.services[s.cursor], true
}

// serviceRowCount es el nº de filas de servicio clicables (mapeo de mouse).
func (s sidebar) serviceRowCount() int { return len(s.services) }

func (s sidebar) view() string {
	var b strings.Builder
	b.WriteString(render.Bold("SERVICES") + render.Dim("  ("+strconv.Itoa(len(s.services))+")") + "\n")
	for i, svc := range s.services {
		b.WriteString(s.serviceRow(svc, i == s.cursor) + "\n")
	}
	b.WriteString("\n" + render.Dim("IMAGES (ECR)") + "\n" + render.Dim("  (próximamente)") + "\n")
	b.WriteString("\n" + render.Dim("DATABASES") + "\n" + render.Dim("  (próximamente)") + "\n")
	return b.String()
}

// serviceRow renderiza una fila de servicio. La fila seleccionada lleva una barra
// de fondo cian oscuro (estilo lazydocker), conservando el color del punto de estado.
func (s sidebar) serviceRow(svc core.ServiceStatus, selected bool) string {
	level := render.StatusLevel(svc.Running, svc.Desired)
	name := strings.TrimPrefix(svc.Name, s.prefix)
	counts := strconv.Itoa(svc.Running) + "/" + strconv.Itoa(svc.Desired)
	tag := svc.Tag
	if tag == "" {
		tag = "—"
	}

	if !selected {
		return "  " + render.Symbol(level) + " " + name + "  " + counts + "  " + render.Dim(tag)
	}

	bg := lipgloss.Color(colorSelectionBar)
	on := func(fg string) lipgloss.Style {
		st := lipgloss.NewStyle().Background(bg)
		if fg != "" {
			st = st.Foreground(lipgloss.Color(fg))
		}
		return st
	}
	dot := on(render.LevelColor(level)).Render("●")
	inner := "  " + dot + " " +
		on("").Bold(true).Render(name) + "  " + // nombre en blanco/negrita para contraste sobre la barra
		on("").Render(counts) + "  " +
		on(render.BrandColor).Render(tag)
	width := s.width
	if width < 1 {
		width = lipgloss.Width(inner)
	}
	return lipgloss.NewStyle().Background(bg).Width(width).Render(inner)
}
