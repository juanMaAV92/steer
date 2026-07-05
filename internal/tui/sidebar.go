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
	entryRepo
)

// sidebarEntry es una entrada navegable: header de sección, servicio o repo.
type sidebarEntry struct {
	Kind    entryKind
	Section sidebarSection
	Index   int // índice del servicio/repo visible dentro de su sección
}

// imagesState refleja el ciclo de vida de la sección IMAGES.
type imagesState int

const (
	imagesDisabled imagesState = iota // sin bloque [images] en el contexto
	imagesLoading
	imagesReady
	imagesError
)

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
	services         []core.ServiceStatus
	collapsed        map[sidebarSection]bool
	cursor           int    // índice sobre las entradas navegables (headers + servicios/repos)
	selectedName     string // nombre REAL del servicio seleccionado (persiste por nombre)
	width, height    int
	scroll           int    // primera fila (de rows()) visible en la ventana
	prefix           string // prefijo a ocultar en la visualización (ej. "nao-v2-dev-")
	filterActive     bool   // true mientras se está tecleando el filtro
	filterQuery      string // substring del filtro (aplicado sobre el nombre de display)
	repos            []core.Repository
	repoPrefix       string // prefijo de repos a ocultar (config RepoPrefix)
	selectedRepoName string
	lastSelected     sidebarSection // qué sección alimenta el panel derecho
	imagesState      imagesState
	imagesErr        string
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
		if s.lastSelected != sectionImages {
			s.lastSelected = sectionServices
		}
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

// visibleServices aplica el filtro substring (case-insensitive) sobre el nombre de display.
func (s sidebar) visibleServices() []core.ServiceStatus {
	if s.filterQuery == "" {
		return s.services
	}
	q := strings.ToLower(s.filterQuery)
	var out []core.ServiceStatus
	for _, svc := range s.services {
		if strings.Contains(strings.ToLower(strings.TrimPrefix(svc.Name, s.prefix)), q) {
			out = append(out, svc)
		}
	}
	return out
}

// setRepos guarda los repos ordenados alfanuméricamente por nombre de display.
func (s *sidebar) setRepos(repos []core.Repository) {
	sorted := make([]core.Repository, len(repos))
	copy(sorted, repos)
	sort.SliceStable(sorted, func(i, j int) bool {
		di := strings.ToLower(strings.TrimPrefix(sorted[i].Name, s.repoPrefix))
		dj := strings.ToLower(strings.TrimPrefix(sorted[j].Name, s.repoPrefix))
		return di < dj
	})
	s.repos = sorted
	s.imagesState = imagesReady
	// la selección de repo persiste por nombre; si desapareció, se limpia
	if _, ok := s.selectedRepo(); !ok {
		s.selectedRepoName = ""
	}
	if s.cursor >= len(s.navEntries()) {
		s.cursor = max(0, len(s.navEntries())-1)
	}
}

// visibleRepos aplica el mismo filtro substring que los servicios.
func (s sidebar) visibleRepos() []core.Repository {
	if s.filterQuery == "" {
		return s.repos
	}
	q := strings.ToLower(s.filterQuery)
	var out []core.Repository
	for _, r := range s.repos {
		if strings.Contains(strings.ToLower(strings.TrimPrefix(r.Name, s.repoPrefix)), q) {
			out = append(out, r)
		}
	}
	return out
}

// selectedRepo devuelve el repo seleccionado (nombre real) si sigue existiendo.
func (s sidebar) selectedRepo() (string, bool) {
	for _, r := range s.repos {
		if r.Name == s.selectedRepoName {
			return r.Name, true
		}
	}
	return "", false
}

// setFilter fija el query y reajusta selección/cursor sobre los visibles resultantes.
func (s *sidebar) setFilter(q string) {
	s.filterQuery = q
	vis := s.visibleServices()
	// si la selección quedó fuera del filtro, saltar al primer visible
	stillVisible := false
	for _, svc := range vis {
		if svc.Name == s.selectedName {
			stillVisible = true
			break
		}
	}
	if !stillVisible {
		s.selectedName = ""
		if len(vis) > 0 {
			s.selectedName = vis[0].Name
		}
	}
	// resincronizar el cursor con la selección (evita resaltar una fila equivocada)
	nav := s.navEntries()
	synced := false
	for i, e := range nav {
		if e.Kind == entryService && vis[e.Index].Name == s.selectedName {
			s.cursor = i
			synced = true
			break
		}
	}
	if !synced && s.cursor >= len(nav) {
		s.cursor = max(0, len(nav)-1)
	}
	s.ensureCursorVisible()
}

// clearFilter desactiva el filtro y restaura la vista completa de servicios.
func (s *sidebar) clearFilter() {
	s.filterActive = false
	s.setFilter("")
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
	visible := s.visibleServices()
	count := "(" + strconv.Itoa(len(visible)) + "/" + strconv.Itoa(len(s.services)) + ")"
	if s.filterQuery == "" && !s.filterActive {
		count = "(" + strconv.Itoa(len(s.services)) + ")"
	}
	title := "SERVICES"
	if s.filterActive {
		title = "SERVICES /" + s.filterQuery + "▌"
	} else if s.filterQuery != "" {
		title = "SERVICES /" + s.filterQuery
	}
	appendHeader(sectionServices, title, count)
	appendBlank()
	if !s.collapsed[sectionServices] {
		for i, svc := range visible {
			under := nav == s.cursor
			line := s.serviceRow(svc, under)
			out = append(out, sidebarRow{Line: line, Entry: &sidebarEntry{Kind: entryService, Section: sectionServices, Index: i}})
			nav++
		}
		appendBlank()
	}
	// IMAGES
	visRepos := s.visibleRepos()
	repoCount := "(" + strconv.Itoa(len(visRepos)) + "/" + strconv.Itoa(len(s.repos)) + ")"
	if s.filterQuery == "" && !s.filterActive {
		repoCount = "(" + strconv.Itoa(len(s.repos)) + ")"
	}
	if s.imagesState != imagesReady {
		repoCount = "···"
	}
	appendHeader(sectionImages, "IMAGES", repoCount)
	if !s.collapsed[sectionImages] {
		switch s.imagesState {
		case imagesDisabled:
			out = append(out, sidebarRow{Line: render.Dim("  configure images in steer.toml")})
		case imagesLoading:
			out = append(out, sidebarRow{Line: render.Dim("  loading…")})
		case imagesError:
			out = append(out, sidebarRow{Line: render.Dim("  registry error: " + s.imagesErr)})
		case imagesReady:
			if len(visRepos) == 0 {
				out = append(out, sidebarRow{Line: render.Dim("  no repositories")})
			}
			for i, r := range visRepos {
				under := nav == s.cursor
				out = append(out, sidebarRow{Line: s.repoRow(r, under),
					Entry: &sidebarEntry{Kind: entryRepo, Section: sectionImages, Index: i}})
				nav++
			}
		}
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

// visibleRows devuelve la ventana de s.height filas sobre rows(), con indicadores
// ↑/↓ N more cuando hay recorte (filas decorativas, no navegables).
func (s sidebar) visibleRows(focused bool) []sidebarRow {
	all := s.rows(focused)
	if s.height <= 0 || len(all) <= s.height {
		return all
	}
	scroll := min(max(s.scroll, 0), len(all)-s.height)
	win := make([]sidebarRow, s.height)
	copy(win, all[scroll:scroll+s.height])
	if scroll > 0 {
		win[0] = sidebarRow{Line: render.Dim("  ↑ " + strconv.Itoa(scroll) + " more")}
	}
	if hidden := len(all) - (scroll + s.height); hidden > 0 {
		win[len(win)-1] = sidebarRow{Line: render.Dim("  ↓ " + strconv.Itoa(hidden) + " more")}
	}
	return win
}

// scrollBy desplaza la ventana delta filas, con clamp a [0, max].
func (s *sidebar) scrollBy(delta int) {
	all := len(s.rows(false))
	maxScroll := max(0, all-s.height)
	s.scroll = min(max(s.scroll+delta, 0), maxScroll)
}

// ensureCursorVisible ajusta el scroll para que la fila del cursor quede en la ventana
// (dejando sitio a los indicadores).
func (s *sidebar) ensureCursorVisible() {
	if s.height <= 0 {
		return
	}
	row := s.cursorRow()
	if row < 0 {
		return
	}
	if row < s.scroll+1 { // +1: fila del indicador ↑
		s.scroll = max(0, row-1)
	}
	if row > s.scroll+s.height-2 { // -2: fila del indicador ↓
		s.scroll = row - s.height + 2
	}
	s.scrollBy(0) // clamp final
}

// cursorRow devuelve la fila (en rows completas) de la entrada bajo el cursor.
func (s sidebar) cursorRow() int {
	nav := 0
	for i, r := range s.rows(false) {
		if r.Entry != nil {
			if nav == s.cursor {
				return i
			}
			nav++
		}
	}
	return -1
}

// EntryAtVisibleRow mapea una fila EN PANTALLA (ventana con indicadores) a su entrada.
func (s sidebar) EntryAtVisibleRow(row int) (sidebarEntry, bool) {
	rows := s.visibleRows(false)
	if row < 0 || row >= len(rows) || rows[row].Entry == nil {
		return sidebarEntry{}, false
	}
	return *rows[row].Entry, true
}

// view une las filas visibles SIN newline final: con la ventana de scroll llena
// (exactamente height filas), un \n de cola haría el bloque una línea más alto que
// bodyH, el frame excedería la terminal y todos los clicks se correrían una fila.
func (s sidebar) view(focused bool) string {
	rows := s.visibleRows(focused)
	lines := make([]string, len(rows))
	for i, r := range rows {
		lines[i] = r.Line
	}
	return strings.Join(lines, "\n")
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
	switch e := nav[s.cursor]; e.Kind {
	case entryService:
		s.selectedName = s.visibleServices()[e.Index].Name
		s.lastSelected = sectionServices
	case entryRepo:
		s.selectedRepoName = s.visibleRepos()[e.Index].Name
		s.lastSelected = sectionImages
	}
	s.ensureCursorVisible()
}

// toggle colapsa/expande una sección conservando el cursor sobre su header.
func (s *sidebar) toggle(sec sidebarSection) {
	s.collapsed[sec] = !s.collapsed[sec]
	// re-ubicar el cursor sobre el header de la sección toggleada
	for i, e := range s.navEntries() {
		if e.Kind == entryHeader && e.Section == sec {
			s.cursor = i
			break
		}
	}
	s.ensureCursorVisible()
}

// selectEntry ubica el cursor en la entrada e (click); si es servicio, lo selecciona.
func (s *sidebar) selectEntry(target sidebarEntry) {
	for i, e := range s.navEntries() {
		if e == target {
			s.cursor = i
			switch e.Kind {
			case entryService:
				s.selectedName = s.visibleServices()[e.Index].Name
				s.lastSelected = sectionServices
			case entryRepo:
				s.selectedRepoName = s.visibleRepos()[e.Index].Name
				s.lastSelected = sectionImages
			}
			break
		}
	}
	s.ensureCursorVisible()
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

// repoRow renderiza una fila de repo; bajo el cursor lleva la barra de selección.
func (s sidebar) repoRow(r core.Repository, underCursor bool) string {
	name := strings.TrimPrefix(r.Name, s.repoPrefix)
	if !underCursor {
		return "  " + render.Dim("▣") + " " + name
	}
	return s.barLine("▣ " + name)
}
