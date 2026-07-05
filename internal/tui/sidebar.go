package tui

import (
	"sort"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/juanMaAV92/steer/internal/core"
	"github.com/juanMaAV92/steer/internal/render"
)

// entryKind distingue entre un header de sección y un servicio dentro del sidebar.
type entryKind int

const (
	entryHeader entryKind = iota
	entryService
)

// sidebarEntry es una entrada navegable: header de sección o servicio.
type sidebarEntry struct {
	Kind    entryKind
	Section sidebarSection
	Index   int // índice del servicio visible dentro de su sección (solo entryService)
}

// sidebarRow es una fila renderizada; Entry nil = decorativa (blanco/stub/indicador).
type sidebarRow struct {
	Line  string
	Entry *sidebarEntry
}

// sidebarSection identifica cada bloque apilado dentro del sidebar.
type sidebarSection int

const (
	sectionServices sidebarSection = iota
	sectionImages
	sectionDatabases
)

// sidebar es la columna izquierda: secciones colapsables con entradas navegables.
type sidebar struct {
	services      []core.ServiceStatus
	collapsed     map[sidebarSection]bool
	cursor        int    // índice sobre las entradas navegables (headers + servicios)
	selectedName  string // nombre REAL del servicio seleccionado (persiste por nombre)
	width, height int
	prefix        string // prefijo a ocultar en la visualización (ej. "nao-v2-dev-")
}

func newSidebar() sidebar {
	return sidebar{collapsed: map[sidebarSection]bool{
		sectionImages:    true,
		sectionDatabases: true,
	}}
}

// setServices guarda los servicios y los ordena alfabéticamente por nombre de visualización.
func (s *sidebar) setServices(svc []core.ServiceStatus) {
	firstLoad := len(s.services) == 0 // antes de reemplazar
	// copiar para no mutar el slice original
	sorted := make([]core.ServiceStatus, len(svc))
	copy(sorted, svc)
	sort.SliceStable(sorted, func(i, j int) bool {
		di := strings.ToLower(strings.TrimPrefix(sorted[i].Name, s.prefix))
		dj := strings.ToLower(strings.TrimPrefix(sorted[j].Name, s.prefix))
		return di < dj
	})
	s.services = sorted
	// la selección persiste por nombre; si desapareció (o no había), primer servicio
	if _, ok := s.selected(); !ok && len(s.services) > 0 {
		s.selectedName = s.services[0].Name
	}
	// cursor inicial/re-clamp: sobre el primer servicio si existe
	nav := s.navEntries()
	if s.cursor >= len(nav) {
		s.cursor = max(0, len(nav)-1)
	}
	// salto inicial al primer servicio SOLO en la primera carga; un reload
	// periódico no debe expulsar al usuario del header si lo dejó ahí a propósito.
	if firstLoad && len(sorted) > 0 && s.cursor == 0 {
		s.cursor = 1 // [0] es el header SERVICES
	}
}

// rows es la fuente única: cada fila renderizada con su entrada (nil = decorativa).
func (s sidebar) rows(focused bool) []sidebarRow {
	var out []sidebarRow
	nav := 0

	appendHeader := func(sec sidebarSection, title, count string) {
		mark := " ▾"
		if s.collapsed[sec] {
			mark = " ▸"
		}
		var line string
		if nav == s.cursor {
			// header bajo el cursor: barra de fondo a lo ancho
			line = s.barLine(title + mark + "  " + count)
		} else {
			line = s.sectionHeader(title+mark, count, focused && sec == sectionServices)
		}
		out = append(out, sidebarRow{Line: line, Entry: &sidebarEntry{Kind: entryHeader, Section: sec}})
		nav++
	}
	appendBlank := func() { out = append(out, sidebarRow{Line: ""}) }

	// SERVICES
	appendHeader(sectionServices, "SERVICES", "("+strconv.Itoa(len(s.services))+")")
	appendBlank()
	if !s.collapsed[sectionServices] {
		for i, svc := range s.services {
			under := nav == s.cursor
			line := s.serviceRow(svc, under)
			out = append(out, sidebarRow{Line: line, Entry: &sidebarEntry{Kind: entryService, Section: sectionServices, Index: i}})
			nav++
		}
		appendBlank()
	}
	// IMAGES
	appendHeader(sectionImages, "IMAGES (ECR)", "···")
	if !s.collapsed[sectionImages] {
		out = append(out, sidebarRow{Line: render.Dim("  coming soon")})
	}
	appendBlank()
	// DATABASES
	appendHeader(sectionDatabases, "DATABASES", "···")
	if !s.collapsed[sectionDatabases] {
		out = append(out, sidebarRow{Line: render.Dim("  coming soon")})
	}
	return out
}

// barLine pinta una línea con la barra de fondo del cursor a lo ancho del sidebar.
func (s sidebar) barLine(text string) string {
	bg := lipgloss.Color(colorSelectionBar)
	w := s.width
	if w < 1 {
		w = lipgloss.Width(text) + 2
	}
	return lipgloss.NewStyle().Background(bg).Width(w).Render(" " + text)
}

func (s sidebar) view(focused bool) string {
	var b strings.Builder
	for _, r := range s.rows(focused) {
		b.WriteString(r.Line + "\n")
	}
	return b.String()
}

// navEntries devuelve las entradas navegables en orden (headers + servicios visibles).
func (s sidebar) navEntries() []sidebarEntry {
	var out []sidebarEntry
	for _, r := range s.rows(false) {
		if r.Entry != nil {
			out = append(out, *r.Entry)
		}
	}
	return out
}

func (s sidebar) cursorEntry() (sidebarEntry, bool) {
	nav := s.navEntries()
	if s.cursor < 0 || s.cursor >= len(nav) {
		return sidebarEntry{}, false
	}
	return nav[s.cursor], true
}

func (s *sidebar) moveDown() { s.moveCursor(1) }
func (s *sidebar) moveUp()   { s.moveCursor(-1) }

// moveCursor desplaza el cursor sobre las entradas navegables; al pisar un servicio,
// lo selecciona (pasar por headers no toca la selección).
func (s *sidebar) moveCursor(delta int) {
	nav := s.navEntries()
	if len(nav) == 0 {
		return
	}
	s.cursor = min(max(s.cursor+delta, 0), len(nav)-1)
	if e := nav[s.cursor]; e.Kind == entryService {
		s.selectedName = s.services[e.Index].Name
	}
}

// toggle colapsa/expande una sección conservando el cursor sobre su header.
func (s *sidebar) toggle(sec sidebarSection) {
	s.collapsed[sec] = !s.collapsed[sec]
	// re-ubicar el cursor sobre el header de la sección toggleada
	for i, e := range s.navEntries() {
		if e.Kind == entryHeader && e.Section == sec {
			s.cursor = i
			return
		}
	}
}

// selectEntry ubica el cursor en la entrada e (click); si es servicio, lo selecciona.
func (s *sidebar) selectEntry(target sidebarEntry) {
	for i, e := range s.navEntries() {
		if e == target {
			s.cursor = i
			if e.Kind == entryService {
				s.selectedName = s.services[e.Index].Name
			}
			return
		}
	}
}

// EntryAtRow mapea una fila renderizada a su entrada (ok=false si es decorativa).
func (s sidebar) EntryAtRow(row int) (sidebarEntry, bool) {
	rows := s.rows(false)
	if row < 0 || row >= len(rows) || rows[row].Entry == nil {
		return sidebarEntry{}, false
	}
	return *rows[row].Entry, true
}

func (s sidebar) selected() (core.ServiceStatus, bool) {
	for _, svc := range s.services {
		if svc.Name == s.selectedName {
			return svc, true
		}
	}
	return core.ServiceStatus{}, false
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

// serviceRow renderiza una fila de servicio. La fila bajo el cursor lleva una barra
// de fondo cian oscuro (estilo lazydocker), conservando el color del punto de estado.
func (s sidebar) serviceRow(svc core.ServiceStatus, underCursor bool) string {
	level := render.StatusLevel(svc.Running, svc.Desired)
	name := strings.TrimPrefix(svc.Name, s.prefix)
	counts := strconv.Itoa(svc.Running) + "/" + strconv.Itoa(svc.Desired)
	tag := svc.Tag
	if tag == "" {
		tag = "—"
	}

	if !underCursor {
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
