package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
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

func TestActionOverlayTypingAndConfirm(t *testing.T) {
	o := newActionOverlay(defaultKeys(), actionDeploy, "api")
	for _, r := range "v2" {
		done, _ := o.Update(keyMsg(string(r)))
		require.False(t, done)
	}
	done, res := o.Update(keyMsg("enter"))
	require.True(t, done)
	conf, ok := res.(actionConfirmedMsg)
	require.True(t, ok)
	require.Equal(t, actionDeploy, conf.kind)
	require.Equal(t, "api", conf.service)
	require.Equal(t, "v2", conf.input)
}

func TestActionOverlayEnterWithoutInputStaysOpen(t *testing.T) {
	o := newActionOverlay(defaultKeys(), actionDeploy, "api")
	done, res := o.Update(keyMsg("enter")) // input vacío → no listo
	require.False(t, done)
	require.Nil(t, res)
}

func TestActionOverlayClickCancels(t *testing.T) {
	o := newActionOverlay(defaultKeys(), actionScale, "api")
	click := tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, X: 1, Y: 1}
	done, res := o.Update(click)
	require.True(t, done)
	require.Nil(t, res)
}

func TestOverlayViewsRender(t *testing.T) {
	p := newPickerOverlay(defaultKeys(), samplePickerContexts(), "nao-dev")
	require.Contains(t, p.View(80, 24), "Switch context")
	a := newActionOverlay(defaultKeys(), actionRollback, "api")
	require.Contains(t, strings.ToLower(a.View(80, 24)), "roll back")
}
