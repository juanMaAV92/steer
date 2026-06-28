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
	}
}

func TestSidebarNavigationClamps(t *testing.T) {
	s := newSidebar()
	s.setServices(sampleServices())
	require.Equal(t, 0, s.cursor)
	s.moveUp() // clamp en 0
	require.Equal(t, 0, s.cursor)
	s.moveDown()
	s.moveDown()
	s.moveDown() // clamp en 2
	require.Equal(t, 2, s.cursor)
	sel, ok := s.selected()
	require.True(t, ok)
	require.Equal(t, "worker", sel.Name)
}

func TestSidebarSelectIndex(t *testing.T) {
	s := newSidebar()
	s.setServices(sampleServices())
	s.selectIndex(1)
	sel, _ := s.selected()
	require.Equal(t, "web", sel.Name)
	s.selectIndex(99) // fuera de rango: no-op
	sel, _ = s.selected()
	require.Equal(t, "web", sel.Name)
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
	out := s.view()
	require.Contains(t, out, "SERVICES")
	require.Contains(t, out, "api")
	require.Contains(t, out, "worker")
	require.Contains(t, out, "IMAGES")
	require.Contains(t, strings.ToLower(out), "próximamente")
}
