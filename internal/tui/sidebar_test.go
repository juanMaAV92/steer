package tui

import (
	"strings"
	"testing"

	"github.com/juanMaAV92/steer/internal/core"
	"github.com/stretchr/testify/require"
)

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
	require.Contains(t, stripANSI(s.view(true)), "coming soon")
}

func TestEntryAtRowMatchesRows(t *testing.T) {
	s := newSidebar()
	s.setServices(sampleServices())
	rows := s.rows(true)
	for i, r := range rows {
		e, ok := s.EntryAtRow(i)
		if r.Entry == nil {
			require.False(t, ok, "row %d", i)
		} else {
			require.True(t, ok, "row %d", i)
			require.Equal(t, *r.Entry, e)
		}
	}
	_, ok := s.EntryAtRow(-1)
	require.False(t, ok)
	_, ok = s.EntryAtRow(len(rows) + 5)
	require.False(t, ok)
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
	require.Contains(t, out, "coming soon")
	// línea en blanco tras el header: la fila 1 está vacía y la 2 tiene el primer servicio
	lines := strings.Split(out, "\n")
	require.Equal(t, "", strings.TrimSpace(lines[1]))
	require.Contains(t, lines[2], "api")
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
