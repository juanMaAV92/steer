package panel

import (
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/juanMaAV92/steer/internal/render"
)

// Events es la pestaña de eventos ECS + feed de progreso de deploy (scrolleable).
type Events struct {
	vp         viewport.Model
	lines      []string
	statusLine string
}

func NewEvents() Events { return Events{vp: viewport.New(0, 0)} }

func (e *Events) SetSize(w, h int) {
	e.vp.Width = w
	e.vp.Height = h
	e.sync()
}

func (e *Events) AppendLine(line string) {
	e.lines = append(e.lines, line)
	e.sync()
	e.vp.GotoBottom()
}

func (e *Events) SetStatusLine(s string) {
	e.statusLine = s
	e.sync()
}

func (e *Events) Reset() {
	e.lines = nil
	e.statusLine = ""
	e.sync()
}

func (e *Events) sync() {
	body := strings.Join(e.lines, "\n")
	if e.statusLine != "" {
		if body != "" {
			body += "\n\n"
		}
		body += e.statusLine
	}
	if body == "" {
		body = render.Dim("no events yet")
	}
	e.vp.SetContent(body)
}

// Update delega scroll (rueda/teclas) al viewport interno.
func (e *Events) Update(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	e.vp, cmd = e.vp.Update(msg)
	return cmd
}

func (e Events) View() string { return e.vp.View() }
