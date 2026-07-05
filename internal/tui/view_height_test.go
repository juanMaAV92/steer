package tui

import (
	"strconv"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/juanMaAV92/steer/internal/core"
	"github.com/juanMaAV92/steer/internal/core/coretest"
	"github.com/stretchr/testify/require"
)

// viewLines cuenta las líneas reales del frame renderizado.
func viewLines(m Model) int { return strings.Count(m.View(), "\n") + 1 }

// TestViewHeightNeverExceedsTerminal: si View() emite más líneas que la terminal,
// esta scrollea y TODAS las coordenadas Y del mouse se corren (bug real: click en
// un servicio seleccionaba el de arriba). El frame debe medir EXACTO el alto.
func TestViewHeightNeverExceedsTerminal(t *testing.T) {
	manyRepos := func(n int) []core.Repository {
		out := make([]core.Repository, n)
		for i := range out {
			out[i] = core.Repository{Name: "repo-" + strconv.Itoa(i)}
		}
		return out
	}

	t.Run("sidebar con ventana de scroll llena", func(t *testing.T) {
		m := newTestModel(sampleServices())
		m, _ = applySize(m, 120, 40)
		m.sidebar.repoPrefix = ""
		m.sidebar.setRepos(manyRepos(30)) // fuerza windowing: filas > bodyH
		m.sidebar.collapsed[sectionImages] = false
		require.Greater(t, len(m.sidebar.rows(false)), m.bodyH, "precondición: windowing activo")
		require.Equal(t, 40, viewLines(m))
	})

	t.Run("panel TAGS con muchos tags", func(t *testing.T) {
		tags := make([]core.ImageTag, 50)
		for i := range tags {
			tags[i] = core.ImageTag{Tag: "v" + strconv.Itoa(i), Digest: "sha256:x",
				SizeBytes: 1024 * 1024, PushedAt: time.Now().Add(-time.Duration(i) * time.Hour)}
		}
		reg := &coretest.FakeRegistry{
			Repos: []core.Repository{{Name: "api"}},
			Tags:  map[string][]core.ImageTag{"api": tags},
		}
		m := newTestModelWithRegistry(servicesNamed("api"), reg)
		m = mustUpdate(t, m, reposMsg{repos: reg.Repos})
		m.sidebar.collapsed[sectionImages] = false
		// seleccionar el repo para que el panel muestre la tabla de 50 tags
		clickX, clickY := findInView(t, m.View(), "▣ api")
		updated, cmd := m.Update(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, X: clickX, Y: clickY})
		m = updated.(Model)
		m = mustUpdate(t, m, cmd().(tagsMsg))
		require.Equal(t, 40, viewLines(m))
	})
}
