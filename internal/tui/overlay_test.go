package tui

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPickerOverlayEnterEmitsChosenContext(t *testing.T) {
	o := newPickerOverlay(defaultKeys(), samplePickerContexts(), "nao-dev")
	// bajar a nao-prod y confirmar
	done, res := o.Update(keyMsg("j"))
	require.False(t, done)
	require.Nil(t, res)
	done, res = o.Update(keyMsg("enter"))
	require.True(t, done)
	chosen, ok := res.(contextChosenMsg)
	require.True(t, ok)
	require.Equal(t, "nao-prod", chosen.ctx.Name)
}

func TestPickerOverlayEscCloses(t *testing.T) {
	o := newPickerOverlay(defaultKeys(), samplePickerContexts(), "nao-dev")
	done, res := o.Update(keyMsg("esc"))
	require.True(t, done)
	require.Nil(t, res)
}

func TestOverlayViewsRender(t *testing.T) {
	p := newPickerOverlay(defaultKeys(), samplePickerContexts(), "nao-dev")
	require.Contains(t, p.View(80, 24), "Switch context")
}
