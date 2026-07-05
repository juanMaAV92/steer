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

// sidebarSection identifica cada bloque apilado dentro del sidebar.
type sidebarSection int

const (
	sectionServices sidebarSection = iota
	sectionImages
	sectionDatabases
)

// sidebarHit ubica un click dentro de una sección del sidebar y su índice interno.
type sidebarHit struct {
	Section sidebarSection
	Index   int // índice dentro de la sección (solo services tiene filas hoy)
}

// HitAtRow replica la estructura de view(): fila 0 header, fila 1 en blanco,
// filas 2..n+1 los servicios; el resto no es accionable.
func (s sidebar) HitAtRow(row int) (sidebarHit, bool) {
	if row < 2 {
		return sidebarHit{}, false
	}
	if idx := row - 2; idx < len(s.services) {
		return sidebarHit{Section: sectionServices, Index: idx}, true
	}
	return sidebarHit{}, false
}

// sectionHeader alinea el título a la izquierda y el sufijo a la derecha del ancho del
// sidebar. El título se ilumina en cian de marca cuando la sección tiene el foco.
func (s sidebar) sectionHeader(title, suffix string, focused bool) string {
	fill := s.width - lipgloss.Width(title) - lipgloss.Width(suffix)
	if fill < 1 {
		fill = 1
	}
	t := render.Dim(title)
	if focused {
		t = render.Brand(title)
	}
	return t + strings.Repeat(" ", fill) + render.Dim(suffix)
}

func (s sidebar) view(focused bool) string {
	var b strings.Builder
	b.WriteString(s.sectionHeader("SERVICES", "("+strconv.Itoa(len(s.services))+")", focused) + "\n\n")
	for i, svc := range s.services {
		b.WriteString(s.serviceRow(svc, i == s.cursor) + "\n")
	}
	b.WriteString("\n" + s.sectionHeader("IMAGES (ECR)", "···", false) + "\n" + render.Dim("  coming soon") + "\n")
	b.WriteString("\n" + s.sectionHeader("DATABASES", "···", false) + "\n" + render.Dim("  coming soon") + "\n")
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
