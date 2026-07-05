package tui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/juanMaAV92/steer/internal/core"
	"github.com/stretchr/testify/require"
)

func manyServices(n int) []core.ServiceStatus {
	out := make([]core.ServiceStatus, n)
	for i := range out {
		out[i] = core.ServiceStatus{Name: fmt.Sprintf("svc-%02d", i), Running: 1, Desired: 1}
	}
	return out
}

func sampleServices() []core.ServiceStatus {
	return []core.ServiceStatus{
		{Name: "api", Running: 2, Desired: 2, Tag: "v1.4"},
		{Name: "web", Running: 3, Desired: 3, Tag: "v2.0"},
		{Name: "worker", Running: 1, Desired: 2, Tag: "v1.1"},
		{Name: "cron", Running: 0, Desired: 1, Tag: ""},
	}
}

// Con IMAGES/DATABASES colapsadas (default), las entradas navegables son:
// [0]=header SERVICES, [1..4]=servicios, [5]=header IMAGES, [6]=header DATABASES.
func TestSidebarNavEntriesAndInitialState(t *testing.T) {
	s := newSidebar()
	s.setServices(sampleServices())
	e, ok := s.cursorEntry()
	require.True(t, ok)
	require.Equal(t, entryService, e.Kind) // cursor inicial: primer servicio
	sel, ok := s.selected()
	require.True(t, ok)
	require.Equal(t, "api", sel.Name) // selección inicial: primer servicio
}

func TestSidebarCursorOverHeadersKeepsSelection(t *testing.T) {
	s := newSidebar()
	s.setServices(sampleServices())
	for range 4 { // baja hasta salir de los servicios
		s.moveDown()
	}
	e, _ := s.cursorEntry()
	require.Equal(t, entryHeader, e.Kind) // header IMAGES
	require.Equal(t, sectionImages, e.Section)
	sel, ok := s.selected()
	require.True(t, ok)
	require.Equal(t, "worker", sel.Name) // la selección quedó en el último servicio pisado
}

func TestSidebarToggleCollapsesServices(t *testing.T) {
	s := newSidebar()
	s.setServices(sampleServices())
	s.toggle(sectionServices)
	out := stripANSI(s.view(true))
	require.Contains(t, out, "▸")
	require.NotContains(t, out, "api") // sección colapsada oculta items
	// navegación: ya solo hay 3 headers
	s.moveDown()
	e, _ := s.cursorEntry()
	require.Equal(t, entryHeader, e.Kind)
}

func TestSidebarCollapsedByDefaultHidesStubs(t *testing.T) {
	s := newSidebar()
	s.setServices(sampleServices())
	out := stripANSI(s.view(true))
	require.NotContains(t, out, "coming soon") // IMAGES/DATABASES colapsadas
	s.toggle(sectionImages)
	require.Contains(t, stripANSI(s.view(true)), "configure images in steer.toml")
}

func TestEntryAtVisibleRowMatchesRows(t *testing.T) {
	s := newSidebar()
	s.setServices(sampleServices())
	rows := s.rows(true) // height=0 → sin ventana, misma semántica que rows()
	for i, r := range rows {
		e, ok := s.EntryAtVisibleRow(i)
		if r.Entry == nil {
			require.False(t, ok, "row %d", i)
		} else {
			require.True(t, ok, "row %d", i)
			require.Equal(t, *r.Entry, e)
		}
	}
	_, ok := s.EntryAtVisibleRow(-1)
	require.False(t, ok)
	_, ok = s.EntryAtVisibleRow(len(rows) + 5)
	require.False(t, ok)
}

func TestSidebarScrollWindowWithIndicators(t *testing.T) {
	s := newSidebar()
	s.height = 10
	s.setServices(manyServices(30))
	rows := s.visibleRows(true)
	require.Len(t, rows, 10)
	last := stripANSI(rows[len(rows)-1].Line)
	require.Contains(t, last, "more") // recorte abajo
	require.Contains(t, last, "↓")
	// scrollear al fondo produce indicador arriba y no abajo
	s.scrollBy(1000)
	rows = s.visibleRows(true)
	require.Contains(t, stripANSI(rows[0].Line), "↑")
	require.NotContains(t, stripANSI(rows[len(rows)-1].Line), "more")
}

func TestSidebarCursorFollow(t *testing.T) {
	s := newSidebar()
	s.height = 8
	s.setServices(manyServices(30))
	for range 20 {
		s.moveDown()
	}
	// el cursor (servicio 20) debe estar dentro de la ventana visible
	found := false
	for _, r := range s.visibleRows(true) {
		if r.Entry != nil && r.Entry.Kind == entryService && strings.Contains(stripANSI(r.Line), "svc-20") {
			found = true
		}
	}
	require.True(t, found)
}

func TestEntryAtVisibleRowWithScroll(t *testing.T) {
	s := newSidebar()
	s.height = 8
	s.setServices(manyServices(30))
	s.scrollBy(5)
	rows := s.visibleRows(false)
	for i, r := range rows {
		e, ok := s.EntryAtVisibleRow(i)
		if r.Entry == nil {
			require.False(t, ok, "row %d", i)
		} else {
			require.True(t, ok, "row %d", i)
			require.Equal(t, *r.Entry, e)
		}
	}
}

func TestSidebarSelectionSurvivesReload(t *testing.T) {
	s := newSidebar()
	s.setServices(sampleServices())
	s.moveDown() // cursor a "cron" (2º servicio ordenado) — lo selecciona
	sel, _ := s.selected()
	require.Equal(t, "cron", sel.Name)
	s.setServices(sampleServices()) // reload: la selección persiste por nombre
	sel, ok := s.selected()
	require.True(t, ok)
	require.Equal(t, "cron", sel.Name)
}

// TestSidebarSortOrder verifica que los servicios se muestran en orden alfabético.
func TestSidebarSortOrder(t *testing.T) {
	s := newSidebar()
	// entregar fuera de orden: worker, api, cron, web
	s.setServices([]core.ServiceStatus{
		{Name: "worker"},
		{Name: "api"},
		{Name: "cron"},
		{Name: "web"},
	})
	require.Equal(t, "api", s.services[0].Name)
	require.Equal(t, "cron", s.services[1].Name)
	require.Equal(t, "web", s.services[2].Name)
	require.Equal(t, "worker", s.services[3].Name)
}

func TestSidebarViewStyledSections(t *testing.T) {
	s := newSidebar()
	s.width, s.height = 30, 20
	s.setServices(sampleServices())
	out := stripANSI(s.view(true))
	require.Contains(t, out, "SERVICES")
	require.Contains(t, out, "(4)")
	require.NotContains(t, out, "coming soon") // IMAGES/DATABASES colapsadas por defecto
	require.NotContains(t, out, "próximamente")
	require.Contains(t, out, "···")
	s.toggle(sectionImages)
	out = stripANSI(s.view(true))
	require.Contains(t, out, "configure images in steer.toml")
	// línea en blanco tras el header: la fila 1 está vacía y la 2 tiene el primer servicio
	lines := strings.Split(out, "\n")
	require.Equal(t, "", strings.TrimSpace(lines[1]))
	require.Contains(t, lines[2], "api")
}

// Un reload periódico NO debe expulsar el cursor de un header.
func TestSetServicesKeepsCursorOnHeader(t *testing.T) {
	s := newSidebar()
	s.setServices(sampleServices())
	s.moveUp() // del primer servicio al header SERVICES (cursor 0)
	e, _ := s.cursorEntry()
	require.Equal(t, entryHeader, e.Kind)
	s.setServices(sampleServices()) // tick de refresh
	e, ok := s.cursorEntry()
	require.True(t, ok)
	require.Equal(t, entryHeader, e.Kind) // sigue en el header
}

// TestSidebarPrefixStrip verifica que el prefijo se oculta en la visualización
// pero el Name real permanece intacto para las acciones.
func TestSidebarPrefixStrip(t *testing.T) {
	s := newSidebar()
	s.prefix = "nao-v2-dev-"
	s.setServices([]core.ServiceStatus{
		{Name: "nao-v2-dev-audit-ms", Running: 1, Desired: 1, Tag: "v3"},
		{Name: "nao-v2-dev-billing", Running: 2, Desired: 2, Tag: "v1"},
	})

	out := s.view(true)

	// los nombres cortos deben aparecer
	require.Contains(t, out, "audit-ms")
	require.Contains(t, out, "billing")

	// los nombres completos NO deben aparecer en la visualización
	require.NotContains(t, out, "nao-v2-dev-audit-ms")
	require.NotContains(t, out, "nao-v2-dev-billing")

	// el Name real (con prefijo) sigue intacto en el slice
	sel, ok := s.selected()
	require.True(t, ok)
	require.Equal(t, "nao-v2-dev-audit-ms", sel.Name) // primer servicio alfabéticamente: audit-ms
}

func TestSidebarFilterLive(t *testing.T) {
	s := newSidebar()
	s.setServices(sampleServices())
	s.setFilter("wo")
	require.Len(t, s.visibleServices(), 1)
	sel, ok := s.selected()
	require.True(t, ok)
	require.Equal(t, "worker", sel.Name) // la selección salta al primer visible
	out := stripANSI(s.view(true))
	require.Contains(t, out, "/wo")
	require.Contains(t, out, "(1/4)")
	require.NotContains(t, out, "api")
	s.clearFilter()
	require.Len(t, s.visibleServices(), 4)
}

// Filtrar con el cursor lejos debe resincronizar el cursor con la nueva selección.
func TestSetFilterResyncsCursor(t *testing.T) {
	s := newSidebar()
	s.setServices(sampleServices()) // api, cron, web, worker
	for range 3 {                   // cursor a "worker"
		s.moveDown()
	}
	s.setFilter("a") // solo "api" visible → selección salta a api
	sel, _ := s.selected()
	require.Equal(t, "api", sel.Name)
	e, ok := s.cursorEntry()
	require.True(t, ok)
	require.Equal(t, entryService, e.Kind) // el cursor está SOBRE la selección
	vis := s.visibleServices()
	require.Equal(t, "api", vis[e.Index].Name)
}

func sampleRepos() []core.Repository {
	return []core.Repository{{Name: "nao-v2-worker"}, {Name: "nao-v2-api"}}
}

func TestSetReposSortsAndNavigates(t *testing.T) {
	s := newSidebar()
	s.height = 30
	s.repoPrefix = "nao-v2-"
	s.setServices(sampleServices())
	s.setRepos(sampleRepos())
	s.collapsed[sectionImages] = false
	// el primer repo visible es "api" (alfanumérico por nombre de display)
	var repoEntries []sidebarEntry
	for _, e := range s.navEntries() {
		if e.Kind == entryRepo {
			repoEntries = append(repoEntries, e)
		}
	}
	require.Len(t, repoEntries, 2)
	require.Equal(t, "nao-v2-api", s.visibleRepos()[repoEntries[0].Index].Name)
}

func TestRepoSelectionSetsLastSelected(t *testing.T) {
	s := newSidebar()
	s.height = 30
	s.setServices(sampleServices())
	s.setRepos(sampleRepos())
	s.collapsed[sectionImages] = false
	// navegar hasta el primer repo
	for {
		e, ok := s.cursorEntry()
		require.True(t, ok)
		if e.Kind == entryRepo {
			break
		}
		s.moveDown()
	}
	repo, ok := s.selectedRepo()
	require.True(t, ok)
	require.Equal(t, "nao-v2-api", repo)
	require.Equal(t, sectionImages, s.lastSelected)
	// volver a un servicio devuelve lastSelected a services
	for {
		e, ok := s.cursorEntry()
		require.True(t, ok)
		if e.Kind == entryService {
			break
		}
		s.moveUp()
	}
	require.Equal(t, sectionServices, s.lastSelected)
}

func TestFilterAppliesToReposToo(t *testing.T) {
	s := newSidebar()
	s.height = 30
	s.repoPrefix = "nao-v2-"
	s.setServices(sampleServices())
	s.setRepos(sampleRepos())
	s.collapsed[sectionImages] = false
	s.setFilter("work")
	require.Len(t, s.visibleRepos(), 1)
	require.Equal(t, "nao-v2-worker", s.visibleRepos()[0].Name)
}

func TestImagesStatesRender(t *testing.T) {
	s := newSidebar()
	s.width = 40
	s.collapsed[sectionImages] = false
	join := func() string {
		var b strings.Builder
		for _, r := range s.rows(false) {
			b.WriteString(stripANSI(r.Line) + "\n")
		}
		return b.String()
	}
	s.imagesState = imagesDisabled
	require.Contains(t, join(), "configure images in steer.toml")
	s.imagesState = imagesLoading
	require.Contains(t, join(), "loading")
	s.imagesState = imagesError
	s.imagesErr = "boom"
	require.Contains(t, join(), "boom")
	s.imagesState = imagesReady
	require.Contains(t, join(), "no repositories") // ready sin repos
}
