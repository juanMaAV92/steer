package tui

import (
	"strings"
	"testing"

	"github.com/juanMaAV92/steer/internal/config"
	"github.com/stretchr/testify/require"
)

func samplePickerContexts() []config.Context {
	return []config.Context{
		{Name: "nao-dev", Cloud: "aws", Cluster: "c1", Writable: true},
		{Name: "nao-prod", Cloud: "aws", Cluster: "c2", Writable: false},
		{Name: "acme-staging", Cloud: "gcp", Cluster: "c3", Writable: true},
	}
}

func TestPickerStartsAtCurrent(t *testing.T) {
	p := newContextPicker(samplePickerContexts(), "nao-prod")
	sel, ok := p.selected()
	require.True(t, ok)
	require.Equal(t, "nao-prod", sel.Name)
}

func TestPickerNavigationClamps(t *testing.T) {
	p := newContextPicker(samplePickerContexts(), "nao-dev")
	p.moveUp() // clamp en 0
	require.Equal(t, 0, p.cursor)
	p.moveDown()
	p.moveDown()
	p.moveDown() // clamp en 2
	require.Equal(t, 2, p.cursor)
}

func TestPickerViewGroupsByCloudAndMarksNotImpl(t *testing.T) {
	p := newContextPicker(samplePickerContexts(), "nao-dev")
	out := p.view()
	require.Contains(t, out, "AWS")
	require.Contains(t, out, "GCP")
	require.Contains(t, out, "nao-dev")
	require.Contains(t, out, "acme-staging")
	require.Contains(t, strings.ToLower(out), "read-only") // nao-prod
	require.Contains(t, strings.ToLower(out), "no impl")   // gcp
}

func TestPickerSelectIndex(t *testing.T) {
	p := newContextPicker(samplePickerContexts(), "nao-dev")
	p.selectIndex(2)
	sel, _ := p.selected()
	require.Equal(t, "acme-staging", sel.Name)
	p.selectIndex(99) // fuera de rango: no-op
	sel, _ = p.selected()
	require.Equal(t, "acme-staging", sel.Name)
}
