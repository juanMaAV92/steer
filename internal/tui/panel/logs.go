package panel

import (
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/juanMaAV92/steer/internal/render"
)

// Logs es la pestaña de logs del servicio: viewport scrolleable alimentado por
// tail + follow (espejo estructural de Events).
type Logs struct {
	vp    viewport.Model
	lines []string
}

func NewLogs() Logs { return Logs{vp: viewport.New(0, 0)} }

func (l *Logs) SetSize(w, h int) {
	l.vp.Width = w
	l.vp.Height = h
	l.sync(false)
}

// SetLines reemplaza el contenido (tail inicial) y baja al fondo.
func (l *Logs) SetLines(lines []string) {
	l.lines = lines
	l.sync(true)
}

// AppendLines añade líneas del follow; solo baja al fondo si ya estabas al
// fondo (no roba la posición a quien subió a leer historia).
func (l *Logs) AppendLines(lines []string) {
	atBottom := l.vp.AtBottom()
	l.lines = append(l.lines, lines...)
	l.sync(atBottom)
}

func (l *Logs) Reset() {
	l.lines = nil
	l.sync(false)
}

func (l *Logs) sync(gotoBottom bool) {
	body := strings.Join(l.lines, "\n")
	if body == "" {
		body = render.Dim("no logs yet")
	}
	l.vp.SetContent(body)
	if gotoBottom {
		l.vp.GotoBottom()
	}
}

// Update delega scroll (rueda/teclas) al viewport interno.
func (l *Logs) Update(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	l.vp, cmd = l.vp.Update(msg)
	return cmd
}

func (l Logs) View() string { return l.vp.View() }
