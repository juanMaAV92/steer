// Package tui implementa el dashboard interactivo (steer tui) sobre core.Deployer.
package tui

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/juanMaAV92/steer/internal/core"
	"github.com/juanMaAV92/steer/internal/render"
)

type viewState int

const (
	viewList viewState = iota
	viewDetail
	viewConfirm
	viewDeploy
)

type actionKind int

const (
	actionRollback actionKind = iota
	actionDeploy
	actionScale
)

type pendingAction struct {
	kind    actionKind
	service string
	input   string // tag (deploy) o count (scale)
}

// actionDoneMsg es el resultado de ejecutar una acción.
type actionDoneMsg struct {
	msg string
	err error
}

type deployStartedMsg struct {
	steps  []string
	lastID string
	err    error
}

type deployPollMsg struct {
	events                    []core.ServiceEvent
	lastID                    string
	rollout                   string
	running, pending, desired int
	done, failed              bool
	err                       error
}

type deployPollTickMsg struct{}

// Model es el estado de la TUI (patrón Elm de Bubble Tea).
type Model struct {
	dep      core.Deployer
	cluster  string
	env      string
	writable bool
	services []core.ServiceStatus
	cursor   int
	view     viewState
	action   pendingAction
	loading  bool
	status   string
	notice   string
	err      error

	// deploy progress view
	deployLogs       []string
	deployStatusLine string
	deployDone       bool
	deployLastID     string
}

// servicesMsg transporta el resultado de listar servicios.
type servicesMsg struct {
	services []core.ServiceStatus
	err      error
}

// tickMsg dispara un auto-refresh periódico.
type tickMsg struct{}

const refreshInterval = 15 * time.Second

func tickCmd() tea.Cmd {
	return tea.Tick(refreshInterval, func(time.Time) tea.Msg { return tickMsg{} })
}

func startDeployCmd(dep core.Deployer, cluster, service, tag string) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		baseline := ""
		if evs, err := dep.ServiceEvents(ctx, cluster, service); err == nil && len(evs) > 0 {
			baseline = evs[0].ID
		}
		var steps []string
		err := dep.Deploy(ctx, cluster, service, tag, func(s string) { steps = append(steps, s) })
		return deployStartedMsg{steps: steps, lastID: baseline, err: err}
	}
}

func deployPollCmd(dep core.Deployer, cluster, service, lastID string) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		var fresh []core.ServiceEvent
		newLast := lastID
		if evs, err := dep.ServiceEvents(ctx, cluster, service); err == nil {
			for _, e := range evs {
				if e.ID == lastID {
					break
				}
				fresh = append(fresh, e)
			}
			if len(evs) > 0 {
				newLast = evs[0].ID
			}
		}
		d, err := dep.DeploymentStatus(ctx, cluster, service)
		return deployPollMsg{
			events: fresh, lastID: newLast,
			rollout: d.Rollout, running: d.Running, pending: d.Pending, desired: d.Desired,
			done:   d.Rollout == "COMPLETED" && d.Running >= d.Desired,
			failed: d.Rollout == "FAILED",
			err:    err,
		}
	}
}

func deployTickCmd() tea.Cmd {
	return tea.Tick(3*time.Second, func(time.Time) tea.Msg { return deployPollTickMsg{} })
}

// New crea el modelo inicial.
func New(dep core.Deployer, cluster, env string, writable bool) Model {
	return Model{dep: dep, cluster: cluster, env: env, writable: writable, loading: true}
}

// loadServicesCmd lista los servicios en segundo plano.
func (m Model) loadServicesCmd() tea.Cmd {
	return func() tea.Msg {
		s, err := m.dep.ListServices(context.Background(), m.cluster)
		return servicesMsg{services: s, err: err}
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.loadServicesCmd(), tickCmd())
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case servicesMsg:
		m.loading = false
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		m.services = msg.services
		if m.cursor >= len(m.services) {
			m.cursor = max(0, len(m.services)-1)
		}
		return m, nil

	case tickMsg:
		return m, tea.Batch(m.loadServicesCmd(), tickCmd())

	case actionDoneMsg:
		m.view = viewList
		if msg.err != nil {
			m.err = msg.err
		} else {
			m.status = msg.msg
		}
		return m, m.loadServicesCmd()

	case deployStartedMsg:
		for _, s := range msg.steps {
			m.deployLogs = append(m.deployLogs, render.Dim("[*] "+s))
		}
		if msg.err != nil {
			m.deployLogs = append(m.deployLogs, render.Danger("error: "+msg.err.Error()))
			m.deployDone = true
			return m, nil
		}
		m.deployLastID = msg.lastID
		return m, deployPollCmd(m.dep, m.cluster, m.action.service, m.deployLastID)

	case deployPollMsg:
		if msg.err != nil {
			m.deployLogs = append(m.deployLogs, render.Danger("error: "+msg.err.Error()))
			m.deployDone = true
			return m, m.loadServicesCmd()
		}
		for i := len(msg.events) - 1; i >= 0; i-- { // del más antiguo al más nuevo
			e := msg.events[i]
			m.deployLogs = append(m.deployLogs, render.Dim("["+e.At.Format("15:04:05")+"] "+e.Message))
		}
		m.deployLastID = msg.lastID
		m.deployStatusLine = "Rollout: " + rolloutColored(msg.rollout) +
			" | Running: " + itoa(msg.running) + " | Pending: " + itoa(msg.pending) + " | Desired: " + itoa(msg.desired)
		if msg.done {
			m.deployLogs = append(m.deployLogs, render.Success("✓ deployment completed"))
			m.deployDone = true
			return m, m.loadServicesCmd()
		}
		if msg.failed {
			m.deployLogs = append(m.deployLogs, render.Danger("✗ deployment failed"))
			m.deployDone = true
			return m, m.loadServicesCmd()
		}
		return m, deployTickCmd()

	case deployPollTickMsg:
		if m.view == viewDeploy && !m.deployDone {
			return m, deployPollCmd(m.dep, m.cluster, m.action.service, m.deployLastID)
		}
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// (1) confirm-view block: captures all runes before global q/ctrl+c
	if m.view == viewConfirm {
		switch msg.Type {
		case tea.KeyCtrlC:
			return m, tea.Quit
		case tea.KeyEsc:
			m.view = viewList
			return m, nil
		case tea.KeyEnter:
			if m.action.kind != actionRollback && m.action.input == "" {
				return m, nil // exige input para deploy/scale
			}
			if m.action.kind == actionDeploy {
				m.view = viewDeploy
				m.deployLogs = nil
				m.deployStatusLine = ""
				m.deployDone = false
				return m, startDeployCmd(m.dep, m.cluster, m.action.service, m.action.input)
			}
			return m, m.runActionCmd()
		case tea.KeyBackspace:
			if n := len(m.action.input); n > 0 {
				m.action.input = m.action.input[:n-1]
			}
		case tea.KeyRunes:
			if m.action.kind != actionRollback {
				m.action.input += string(msg.Runes)
			}
		}
		return m, nil
	}

	// (1b) deploy-view block
	if m.view == viewDeploy {
		switch msg.Type {
		case tea.KeyCtrlC:
			return m, tea.Quit
		case tea.KeyEsc, tea.KeyEnter:
			m.view = viewList
			return m, m.loadServicesCmd()
		}
		return m, nil
	}

	// (2) global q/ctrl+c quit
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	}

	// (3) viewDetail block
	if m.view == viewDetail {
		if msg.String() == "esc" {
			m.view = viewList
		}
		return m, nil
	}

	// (4) viewList switch
	switch msg.String() {
	case "r":
		m.loading = true
		return m, m.loadServicesCmd()
	case "d", "s", "R":
		if !m.writable {
			m.notice = "read-only environment (writable=false) — action blocked"
			return m, nil
		}
		s, ok := m.selected()
		if !ok {
			return m, nil
		}
		switch msg.String() {
		case "d":
			m.action = pendingAction{kind: actionDeploy, service: s.Name}
		case "s":
			m.action = pendingAction{kind: actionScale, service: s.Name}
		case "R":
			m.action = pendingAction{kind: actionRollback, service: s.Name}
		}
		m.notice = ""
		m.view = viewConfirm
	case "j", "down":
		if m.cursor < len(m.services)-1 {
			m.cursor++
		}
	case "k", "up":
		if m.cursor > 0 {
			m.cursor--
		}
	case "enter":
		if len(m.services) > 0 {
			m.view = viewDetail
		}
	}
	return m, nil
}

func (m Model) runActionCmd() tea.Cmd {
	a := m.action
	dep := m.dep
	cluster := m.cluster
	return func() tea.Msg {
		ctx := context.Background()
		switch a.kind {
		case actionRollback:
			err := dep.Rollback(ctx, cluster, a.service)
			return actionDoneMsg{msg: "rolled back " + a.service, err: err}
		case actionDeploy:
			err := dep.Deploy(ctx, cluster, a.service, a.input, nil)
			return actionDoneMsg{msg: "deployed " + a.service + " -> " + a.input, err: err}
		case actionScale:
			n, convErr := strconv.Atoi(a.input)
			if convErr != nil {
				return actionDoneMsg{err: fmt.Errorf("invalid count %q", a.input)}
			}
			err := dep.Scale(ctx, cluster, a.service, n)
			return actionDoneMsg{msg: fmt.Sprintf("scaled %s to %d", a.service, n), err: err}
		}
		return actionDoneMsg{err: nil}
	}
}

// selected devuelve el servicio bajo el cursor (ok=false si la lista está vacía).
func (m Model) selected() (core.ServiceStatus, bool) {
	if m.cursor < 0 || m.cursor >= len(m.services) {
		return core.ServiceStatus{}, false
	}
	return m.services[m.cursor], true
}

func (m Model) View() string {
	if m.err != nil {
		return render.Danger("error: "+m.err.Error()) + "\n" + render.Dim("press q to quit")
	}
	switch m.view {
	case viewDetail:
		return m.detailView()
	case viewConfirm:
		return m.confirmView()
	case viewDeploy:
		return m.deployView()
	default:
		return m.listView()
	}
}

func (m Model) listView() string {
	var b strings.Builder
	b.WriteString(render.Bold("steer") + "  " + render.Dim(m.cluster) + "\n")
	for i, s := range m.services {
		cursor := "  "
		if i == m.cursor {
			cursor = render.Accent("> ")
		}
		line := render.Symbol(render.StatusLevel(s.Running, s.Desired))
		b.WriteString(cursor + line + " " + s.Name + "  " +
			strconv.Itoa(s.Running) + "/" + strconv.Itoa(s.Desired) + "  " + render.Dim(s.Tag) + "\n")
	}
	if m.status != "" {
		b.WriteString("\n" + render.Success(m.status) + "\n")
	}
	if m.notice != "" {
		b.WriteString("\n" + render.Warn(m.notice) + "\n")
	}
	b.WriteString(render.Dim("\n↑/↓ move · enter detail · d deploy · s scale · R rollback · r refresh · q quit"))
	return b.String()
}

func (m Model) detailView() string {
	s, ok := m.selected()
	if !ok {
		return render.Dim("no service selected\n") + render.Dim("esc to go back")
	}
	var b strings.Builder
	b.WriteString(render.Bold(s.Name) + "\n\n")
	b.WriteString("running:  " + strconv.Itoa(s.Running) + "/" + strconv.Itoa(s.Desired) + "\n")
	b.WriteString("pending:  " + strconv.Itoa(s.Pending) + "\n")
	b.WriteString("status:   " + s.Status + "\n")
	b.WriteString("tag:      " + render.Accent(s.Tag) + "\n")
	b.WriteString(render.Dim("\nesc to go back"))
	return b.String()
}

func (m Model) confirmView() string {
	a := m.action
	var b strings.Builder
	switch a.kind {
	case actionRollback:
		b.WriteString("Roll back " + render.Bold(a.service) + " to previous revision?\n")
		b.WriteString(render.Dim("enter to confirm · esc to cancel"))
	case actionDeploy:
		b.WriteString("Deploy " + render.Bold(a.service) + " — image tag: " + render.Accent(a.input) + "\n")
		b.WriteString(render.Dim("type the tag · enter to deploy · esc to cancel"))
	case actionScale:
		b.WriteString("Scale " + render.Bold(a.service) + " — desired count: " + render.Accent(a.input) + "\n")
		b.WriteString(render.Dim("type a number · enter to scale · esc to cancel"))
	}
	return b.String()
}

func (m Model) deployView() string {
	var b strings.Builder
	b.WriteString(render.Bold("Deploy "+m.action.service) + "\n\n")
	for _, l := range m.deployLogs {
		b.WriteString(l + "\n")
	}
	if m.deployStatusLine != "" {
		b.WriteString(m.deployStatusLine + "\n")
	}
	if m.deployDone {
		b.WriteString(render.Dim("\ndone · esc/enter to go back · q to quit"))
	} else {
		b.WriteString(render.Dim("\nwatching… · esc to background · q to quit"))
	}
	return b.String()
}

func itoa(n int) string { return strconv.Itoa(n) }

func rolloutColored(state string) string {
	switch state {
	case "COMPLETED":
		return render.Success(state)
	case "FAILED":
		return render.Danger(state)
	default:
		return render.Accent(state)
	}
}
