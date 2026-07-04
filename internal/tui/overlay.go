// internal/tui/overlay.go
package tui

import (
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/juanMaAV92/steer/internal/config"
)

// overlay es una capa modal que captura teclado y mouse mientras está activa.
// done=true cierra el overlay; result (opcional) es un tea.Msg tipado que el
// Model ejecuta (contextChosenMsg, actionConfirmedMsg). ctrl+c NO llega aquí:
// el Model lo intercepta antes (quit global).
type overlay interface {
	Update(msg tea.Msg) (done bool, result tea.Msg)
	View(width, height int) string
}

// contextChosenMsg: el usuario eligió un contexto en el picker.
type contextChosenMsg struct{ ctx config.Context }

// actionConfirmedMsg: el usuario confirmó una acción en el modal.
type actionConfirmedMsg struct {
	kind    actionKind
	service string
	input   string
}

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
			// la línea 0 del overlay es el título del picker (el Model resta el top bar)
			if idx, ok := o.picker.indexAtLine(msg.Y - topBarHeight); ok {
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

// ---- actionOverlay: envuelve action ----

type actionOverlay struct {
	keys keyMap
	act  action
}

func newActionOverlay(keys keyMap, kind actionKind, service string) *actionOverlay {
	o := &actionOverlay{keys: keys}
	o.act.open(kind, service)
	return o
}

func (o *actionOverlay) Update(msg tea.Msg) (bool, tea.Msg) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, o.keys.Esc):
			return true, nil
		case key.Matches(msg, o.keys.Enter):
			if !o.act.ready() {
				return false, nil
			}
			return true, actionConfirmedMsg{kind: o.act.kind, service: o.act.service, input: o.act.input}
		default:
			o.act.typeKey(msg)
			return false, nil
		}
	case tea.MouseMsg:
		// cualquier click cancela el modal (comportamiento actual)
		if msg.Action == tea.MouseActionPress {
			return true, nil
		}
		return false, nil
	}
	return false, nil
}

func (o *actionOverlay) View(width, height int) string { return o.act.modalView(width, height) }
