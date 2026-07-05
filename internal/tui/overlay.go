// internal/tui/overlay.go
package tui

import (
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/juanMaAV92/steer/internal/config"
)

// overlay es una capa modal que captura teclado y mouse mientras está activa.
// done=true cierra el overlay; result (opcional) es un tea.Msg tipado que el
// Model ejecuta (contextChosenMsg). ctrl+c NO llega aquí: el Model lo
// intercepta antes (quit global).
type overlay interface {
	Update(msg tea.Msg) (done bool, result tea.Msg)
	View(width, height int) string
}

// contextChosenMsg: el usuario eligió un contexto en el picker.
type contextChosenMsg struct{ ctx config.Context }

// ---- pickerOverlay: envuelve contextPicker ----

type pickerOverlay struct {
	keys   keyMap
	picker contextPicker
}

func newPickerOverlay(keys keyMap, contexts []config.Context, current string) *pickerOverlay {
	return &pickerOverlay{keys: keys, picker: newContextPicker(contexts, current)}
}

func (o *pickerOverlay) Update(msg tea.Msg) (bool, tea.Msg) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, o.keys.Esc):
			return true, nil
		case key.Matches(msg, o.keys.Enter):
			if sel, ok := o.picker.selected(); ok {
				return true, contextChosenMsg{ctx: sel}
			}
			return true, nil
		case key.Matches(msg, o.keys.Down):
			o.picker.moveDown()
		case key.Matches(msg, o.keys.Up):
			o.picker.moveUp()
		}
		return false, nil
	case tea.MouseMsg:
		if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft {
			// la línea 0 del overlay es el título del picker; el overlay se dibuja
			// tras la regla, así que restamos top bar + regla.
			if idx, ok := o.picker.indexAtLine(msg.Y - (topBarHeight + borderTop)); ok {
				o.picker.selectIndex(idx)
				if sel, ok := o.picker.selected(); ok {
					return true, contextChosenMsg{ctx: sel}
				}
			}
		}
		return false, nil
	}
	return false, nil
}

func (o *pickerOverlay) View(width, height int) string { return o.picker.view() }
