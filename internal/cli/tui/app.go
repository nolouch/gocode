package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"github.com/nolouch/gcode/internal/bus"
)

// BusEventMsg wraps a bus.Event for delivery to the Bubble Tea update loop.
type BusEventMsg struct{ Event bus.Event }

// RunDoneMsg signals the agent turn finished.
type RunDoneMsg struct{ Err error }

type SessionCommandType int

const (
	SessionCommandNew SessionCommandType = iota + 1
	SessionCommandNext
)

type SessionCommand struct {
	Type SessionCommandType
}

// SessionSwitchedMsg updates the UI with a different session and preloaded history.
type SessionSwitchedMsg struct {
	SessionID string
	Entries   []msgEntry
	Notice    string
	Err       error
}

// msgEntry is a rendered message in the viewport.
type msgEntry struct {
	role    string // "user" | "assistant"
	content string // rendered text
}

// toolEntry tracks a live tool call display.
type toolEntry struct {
	callID     string
	name       string
	state      string // "running" | "done" | "error"
	durationMs int64
	output     string
}

// Model is the Bubble Tea application model.
type Model struct {
	// layout
	width  int
	height int

	// components
	viewport viewport.Model
	textarea textarea.Model
	spinner  spinner.Model

	// state
	model     string
	agentName string
	workDir   string
	sessionID string

	entries     []msgEntry
	tools       map[string]*toolEntry // callID -> tool
	toolOrder   []string              // insertion order
	thinking    bool
	thinkingBuf string
	thinkingMs  float64

	currentAssistant string // accumulates streaming text
	running          bool
	showHelp         bool
	statusNotice     string

	// glamour renderer
	renderer *glamour.TermRenderer

	// channel to receive bus events
	eventCh <-chan bus.Event
	// channel to send user messages to the runner goroutine
	sendCh chan<- string
	// channel to request abort of the active run
	abortCh chan<- struct{}
	// channel to request session control operations from runtime goroutine
	sessionCmdCh chan<- SessionCommand
}

// New creates a new TUI Model.
func New(modelName, agentName, workDir, sessionID string, eventCh <-chan bus.Event) (Model, error) {
	ta := textarea.New()
	ta.Placeholder = "Type a message... (Enter send, Shift+Enter newline, Esc interrupt)"
	ta.Focus()
	ta.SetWidth(80)
	ta.SetHeight(3)
	ta.ShowLineNumbers = false
	ta.CharLimit = 0

	sp := spinner.New()
	sp.Spinner = spinner.Points
	sp.Style = lipgloss.NewStyle().Foreground(colorWarning)

	vp := viewport.New(80, 20)
	vp.SetContent("")

	renderer, err := glamour.NewTermRenderer(
		glamour.WithStandardStyle("notty"),
		glamour.WithWordWrap(80),
	)
	if err != nil {
		renderer = nil
	}

	return Model{
		model:     modelName,
		agentName: agentName,
		workDir:   workDir,
		sessionID: sessionID,
		viewport:  vp,
		textarea:  ta,
		spinner:   sp,
		renderer:  renderer,
		tools:     make(map[string]*toolEntry),
		eventCh:   eventCh,
	}, nil
}

// waitForEvent returns a Cmd that waits for the next bus event.
func waitForEvent(ch <-chan bus.Event) tea.Cmd {
	return func() tea.Msg {
		e, ok := <-ch
		if !ok {
			return nil
		}
		return BusEventMsg{Event: e}
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(
		textarea.Blink,
		m.spinner.Tick,
		waitForEvent(m.eventCh),
	)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	updateTextarea := true

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.relayout()

	case tea.KeyMsg:
		if m.showHelp {
			if msg.String() == "ctrl+p" || msg.Type == tea.KeyEsc {
				m.showHelp = false
				updateTextarea = false
				break
			}
		}

		if msg.String() == "ctrl+p" {
			m.showHelp = !m.showHelp
			updateTextarea = false
			break
		}

		if msg.String() == "ctrl+n" {
			if !m.running && m.sessionCmdCh != nil {
				select {
				case m.sessionCmdCh <- SessionCommand{Type: SessionCommandNew}:
				default:
				}
			}
			updateTextarea = false
			break
		}

		if msg.String() == "ctrl+l" {
			if !m.running && m.sessionCmdCh != nil {
				select {
				case m.sessionCmdCh <- SessionCommand{Type: SessionCommandNext}:
				default:
				}
			}
			updateTextarea = false
			break
		}

		if isAbortKey(msg) {
			if m.running && m.abortCh != nil {
				select {
				case m.abortCh <- struct{}{}:
				default:
				}
			}
			updateTextarea = false
			break
		}

		if isExitKey(msg) {
			if !m.running {
				return m, tea.Quit
			}
			if m.abortCh != nil {
				select {
				case m.abortCh <- struct{}{}:
				default:
				}
			}
			updateTextarea = false
			break
		}

		if isSubmitKey(msg) {
			if !m.running {
				text := strings.TrimSpace(m.textarea.Value())
				if text != "" {
					m.textarea.Reset()
					m.addUserEntry(text)
					m.running = true
					m.currentAssistant = ""
					if m.sendCh != nil {
						m.sendCh <- text
					}
				}
			}
			updateTextarea = false
			break
		}

		if isNewlineKey(msg) {
			if !m.running {
				m.textarea.InsertString("\n")
			}
			updateTextarea = false
			break
		}

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		cmds = append(cmds, cmd)
		if m.running {
			m.refreshViewport()
		}

	case BusEventMsg:
		cmds = append(cmds, m.handleBusEvent(msg.Event)...)
		cmds = append(cmds, waitForEvent(m.eventCh))

	case RunDoneMsg:
		m.running = false
		if msg.Err != nil {
			m.addAssistantEntry(fmt.Sprintf("**Error:** %s", msg.Err.Error()))
		} else if m.currentAssistant != "" {
			m.addAssistantEntry(m.currentAssistant)
			m.currentAssistant = ""
		}
		m.refreshViewport()

	case SessionSwitchedMsg:
		if msg.Err != nil {
			m.statusNotice = "session: " + msg.Err.Error()
			break
		}
		m.sessionID = msg.SessionID
		m.entries = msg.Entries
		m.tools = make(map[string]*toolEntry)
		m.toolOrder = nil
		m.currentAssistant = ""
		m.running = false
		m.statusNotice = msg.Notice
		m.refreshViewport()
	}

	var taCmd, vpCmd tea.Cmd
	if updateTextarea {
		m.textarea, taCmd = m.textarea.Update(msg)
	}
	m.viewport, vpCmd = m.viewport.Update(msg)
	cmds = append(cmds, taCmd, vpCmd)

	return m, tea.Batch(cmds...)
}

func (m *Model) handleBusEvent(e bus.Event) []tea.Cmd {
	switch e.Type {
	case bus.EventTextDelta:
		if p, ok := e.Payload.(bus.TextDeltaPayload); ok {
			m.currentAssistant += p.Delta
			m.refreshViewport()
		}

	case bus.EventThinking:
		if p, ok := e.Payload.(bus.ThinkingPayload); ok && p.Delta != "" {
			m.thinkingBuf += p.Delta
		}
		m.thinking = true
		m.refreshViewport()

	case bus.EventThinkingDone:
		if p, ok := e.Payload.(bus.ThinkingPayload); ok {
			m.thinkingMs = p.Duration
		}
		m.thinking = false
		m.refreshViewport()

	case bus.EventToolStart:
		if p, ok := e.Payload.(bus.ToolPayload); ok {
			te := &toolEntry{callID: p.CallID, name: p.Tool, state: "running"}
			m.tools[p.CallID] = te
			m.toolOrder = append(m.toolOrder, p.CallID)
			m.refreshViewport()
		}

	case bus.EventToolDone:
		if p, ok := e.Payload.(bus.ToolPayload); ok {
			if te, ok := m.tools[p.CallID]; ok {
				te.state = "done"
				te.durationMs = p.DurationMs
				te.output = p.Output
			}
			m.refreshViewport()
		}

	case bus.EventToolError:
		if p, ok := e.Payload.(bus.ToolPayload); ok {
			if te, ok := m.tools[p.CallID]; ok {
				te.state = "error"
				te.durationMs = p.DurationMs
				te.output = p.Output
			}
			m.refreshViewport()
		}

	case bus.EventTurnDone, bus.EventTurnError:
		// RunDoneMsg will be sent by the goroutine running runner.Run
	}
	return nil
}

func (m *Model) addUserEntry(text string) {
	m.entries = append(m.entries, msgEntry{role: "user", content: text})
	m.tools = make(map[string]*toolEntry)
	m.toolOrder = nil
	m.thinking = false
	m.thinkingMs = 0
	m.thinkingBuf = ""
	m.refreshViewport()
}

func (m *Model) addAssistantEntry(text string) {
	rendered := m.renderMarkdown(text)
	m.entries = append(m.entries, msgEntry{role: "assistant", content: rendered})
}

func (m *Model) refreshViewport() {
	m.viewport.SetContent(m.renderMessages())
	m.viewport.GotoBottom()
}

func (m *Model) renderMessages() string {
	var sb strings.Builder
	w := m.width
	if w == 0 {
		w = 80
	}

	for _, e := range m.entries {
		if e.role == "user" {
			sb.WriteString(StyleUserBorder.Width(w - 4).Render(e.content))
			sb.WriteString("\n\n")
		} else {
			sb.WriteString(e.content)
			sb.WriteString("\n")
		}
	}

	// Live streaming assistant text
	if m.running && m.currentAssistant != "" {
		rendered := m.renderMarkdown(m.currentAssistant)
		sb.WriteString(rendered)
	}

	// Thinking indicator and content
	if m.thinking || m.thinkingBuf != "" || m.thinkingMs > 0 {
		if m.thinking {
			sb.WriteString(StyleThinkingLabel.Render("💭 Thinking "))
			sb.WriteString(m.spinner.View())
			sb.WriteString("\n")
		} else if m.thinkingMs > 0 {
			sb.WriteString(StyleThinkingMuted.Render(fmt.Sprintf("💭 Thought for %.0fms\n", m.thinkingMs)))
		}
		if t := strings.TrimSpace(m.thinkingBuf); t != "" {
			sb.WriteString(StyleThinkingMuted.Render(t))
			sb.WriteString("\n")
		}
	}

	// Live tool calls
	for _, id := range m.toolOrder {
		te := m.tools[id]
		if te == nil {
			continue
		}
		switch te.state {
		case "running":
			sb.WriteString(fmt.Sprintf("%s %s %s\n",
				StyleToolRunning.Render("●"),
				StyleToolName.Render(te.name),
				m.spinner.View()))
		case "done":
			sb.WriteString(fmt.Sprintf("%s %s %s\n",
				StyleToolDone.Render("✓"),
				StyleToolName.Render(te.name),
				StyleToolMeta.Render(fmt.Sprintf("(%s)", time.Duration(te.durationMs)*time.Millisecond))))
		case "error":
			sb.WriteString(fmt.Sprintf("%s %s %s\n",
				StyleToolError.Render("✗"),
				StyleToolName.Render(te.name),
				StyleToolMeta.Render(fmt.Sprintf("(%s)", time.Duration(te.durationMs)*time.Millisecond))))
			sb.WriteString(StyleToolError.Render("  "+te.output) + "\n")
		}
	}

	return sb.String()
}

func (m Model) View() string {
	if m.width == 0 {
		return "Loading…"
	}
	header := m.renderHeader()
	footer := m.renderFooter()
	input := m.renderInput()

	headerH := lipgloss.Height(header)
	footerH := lipgloss.Height(footer)
	inputH := lipgloss.Height(input)
	vpH := m.height - headerH - footerH - inputH
	if vpH < 1 {
		vpH = 1
	}
	m.viewport.Height = vpH

	return lipgloss.JoinVertical(lipgloss.Left,
		header,
		m.viewport.View(),
		m.renderHelp(),
		input,
		footer,
	)
}

func (m Model) renderHeader() string {
	title := StyleHeader.Render("gcode")
	meta := StyleHeaderMeta.Render(fmt.Sprintf("  %s · %s · %s", m.model, m.agentName, m.workDir))
	if m.sessionID != "" {
		sid := m.sessionID
		if len(sid) > 8 {
			sid = sid[:8]
		}
		meta = StyleHeaderMeta.Render(fmt.Sprintf("  %s · %s · %s · session:%s", m.model, m.agentName, m.workDir, sid))
	}
	line := lipgloss.JoinHorizontal(lipgloss.Top, title, meta)
	return StyleHeaderBorder.Width(m.width).Render(line)
}

func (m Model) renderInput() string {
	m.textarea.SetWidth(m.width - 2)
	return StyleInputBorder.Width(m.width).Render(m.textarea.View())
}

func (m Model) renderFooter() string {
	var status string
	if m.running {
		status = m.spinner.View() + " running"
	} else {
		status = StyleStatusOK.Render("●") + " ready"
	}
	hints := StyleFooter.Render("Enter send · Shift+Enter newline · Esc interrupt · Ctrl+N new · Ctrl+L switch · Ctrl+P help · Ctrl+C quit")
	if m.statusNotice != "" {
		hints = StyleFooter.Render(m.statusNotice + " · Enter send · Shift+Enter newline · Esc interrupt · Ctrl+N new · Ctrl+L switch · Ctrl+P help · Ctrl+C quit")
	}
	gap := m.width - lipgloss.Width(status) - lipgloss.Width(hints)
	if gap < 1 {
		gap = 1
	}
	line := status + strings.Repeat(" ", gap) + hints
	return StyleFooterBorder.Width(m.width).Render(line)
}

func (m Model) renderHelp() string {
	if !m.showHelp {
		return ""
	}
	text := "Shortcuts\n" +
		"Enter: submit\n" +
		"Shift/Ctrl/Alt+Enter or Ctrl+J: newline\n" +
		"Esc: interrupt current run\n" +
		"Ctrl+N: new session\n" +
		"Ctrl+L: switch session (cycle)\n" +
		"Ctrl+C / Ctrl+D: exit (idle) or interrupt (running)\n" +
		"Ctrl+P: toggle help"
	return StyleInputBorder.Width(m.width).Render(text)
}

func (m *Model) renderMarkdown(text string) string {
	if m.renderer == nil {
		return text
	}
	rendered, err := m.renderer.Render(text)
	if err != nil {
		return text
	}
	return rendered
}

func isSubmitKey(msg tea.KeyMsg) bool {
	s := strings.ToLower(msg.String())
	return s == "enter" || s == "return"
}

func isNewlineKey(msg tea.KeyMsg) bool {
	s := strings.ToLower(msg.String())
	switch s {
	case "shift+enter", "shift+return", "ctrl+enter", "ctrl+return", "alt+enter", "alt+return", "ctrl+j":
		return true
	default:
		return false
	}
}

func isAbortKey(msg tea.KeyMsg) bool {
	s := strings.ToLower(msg.String())
	return s == "esc" || s == "escape"
}

func isExitKey(msg tea.KeyMsg) bool {
	s := strings.ToLower(msg.String())
	return s == "ctrl+c" || s == "ctrl+d"
}

func (m *Model) relayout() {
	m.textarea.SetWidth(m.width - 2)
	if m.renderer != nil {
		m.renderer, _ = glamour.NewTermRenderer(
			glamour.WithStandardStyle("notty"),
			glamour.WithWordWrap(m.width-4),
		)
	}
	m.refreshViewport()
}
