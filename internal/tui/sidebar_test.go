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

// TestSidebarNavigationClamps verifica que el cursor no excede los límites.
// Orden alfabético post-sort: api(0), cron(1), web(2), worker(3).
func TestSidebarNavigationClamps(t *testing.T) {
	s := newSidebar()
	s.setServices(sampleServices())
	require.Equal(t, 0, s.cursor)
	s.moveUp() // clamp en 0
	require.Equal(t, 0, s.cursor)
	s.moveDown()
	s.moveDown()
	s.moveDown()
	s.moveDown() // clamp en 3 (worker es el último)
	require.Equal(t, 3, s.cursor)
	sel, ok := s.selected()
	require.True(t, ok)
	require.Equal(t, "worker", sel.Name)
}

// TestSidebarSelectIndex verifica selección directa por índice.
// Orden: api(0), cron(1), web(2), worker(3).
func TestSidebarSelectIndex(t *testing.T) {
	s := newSidebar()
	s.setServices(sampleServices())
	s.selectIndex(1)
	sel, _ := s.selected()
	require.Equal(t, "cron", sel.Name) // índice 1 = cron tras el sort
	s.selectIndex(99)                  // fuera de rango: no-op
	sel, _ = s.selected()
	require.Equal(t, "cron", sel.Name)
}

func TestSidebarSetServicesReclampsCursor(t *testing.T) {
	s := newSidebar()
	s.setServices(sampleServices())
	s.selectIndex(2)
	s.setServices([]core.ServiceStatus{{Name: "api"}}) // lista se encoge
	require.Equal(t, 0, s.cursor)
}

func TestSidebarViewListsServicesAndSections(t *testing.T) {
	s := newSidebar()
	s.width, s.height = 30, 20
	s.setServices(sampleServices())
	out := s.view(true)
	require.Contains(t, out, "SERVICES")
	require.Contains(t, out, "api")
	require.Contains(t, out, "worker")
	require.Contains(t, out, "cron")
	require.Contains(t, out, "IMAGES")
	require.Contains(t, strings.ToLower(out), "coming soon")
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

// TestHitAtRow verifica que HitAtRow replica la estructura de view(): fila 0 header,
// fila 1 en blanco, filas 2..n+1 son los servicios, y todo lo demás no es accionable.
func TestHitAtRow(t *testing.T) {
	s := newSidebar()
	s.setServices(sampleServices())
	for _, row := range []int{-1, 0, 1, 6, 99} {
		_, ok := s.HitAtRow(row)
		require.False(t, ok, "row %d", row)
	}
	hit, ok := s.HitAtRow(2)
	require.True(t, ok)
	require.Equal(t, sectionServices, hit.Section)
	require.Equal(t, 0, hit.Index)
	hit, ok = s.HitAtRow(5)
	require.True(t, ok)
	require.Equal(t, 3, hit.Index)
}

func TestSidebarViewStyledSections(t *testing.T) {
	s := newSidebar()
	s.width = 30
	s.setServices(sampleServices())
	out := stripANSI(s.view(true))
	require.Contains(t, out, "SERVICES")
	require.Contains(t, out, "(4)")
	require.Contains(t, out, "coming soon")
	require.NotContains(t, out, "próximamente")
	require.Contains(t, out, "···")
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
