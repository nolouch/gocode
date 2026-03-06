package tui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/nolouch/gocode/pkg/sdk"
)

// Run starts the Bubble Tea TUI, wiring the event bus to the model.
func Run(
	ctx context.Context,
	modelName, agentName, workDir string,
	serverAddr string,
	socketPath string,
) error {
	if socketPath == "" {
		home, _ := os.UserHomeDir()
		socketPath = filepath.Join(home, ".gocode", "run", "gcode.sock")
	}
	cfg := sdk.Config{SocketPath: socketPath}
	if serverAddr != "" {
		cfg = sdk.Config{BaseURL: "http://" + serverAddr}
	}
	client := sdk.New(cfg)

	var sessIDErr error
	var sessID string
	for attempt := 0; attempt < 10; attempt++ {
		created, err := client.CreateSession(ctx, workDir)
		if err == nil {
			sessID = created.ID
			sessIDErr = nil
			break
		}
		sessIDErr = err
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
	if sessIDErr != nil {
		return fmt.Errorf("create session: %w", sessIDErr)
	}

	// Channel to pipe sdk events into Bubble Tea.
	eventCh := make(chan sdk.Event, 128)

	var streamMu sync.Mutex
	var streamCancel context.CancelFunc
	var streamStateMu sync.Mutex
	streamInconsistent := false
	markStreamInconsistent := func() {
		streamStateMu.Lock()
		streamInconsistent = true
		streamStateMu.Unlock()
	}
	resetStreamInconsistent := func() {
		streamStateMu.Lock()
		streamInconsistent = false
		streamStateMu.Unlock()
	}
	consumeStreamInconsistent := func() bool {
		streamStateMu.Lock()
		defer streamStateMu.Unlock()
		v := streamInconsistent
		streamInconsistent = false
		return v
	}
	subscribeSessionEvents := func(sessionID string) error {
		streamMu.Lock()
		defer streamMu.Unlock()
		if streamCancel != nil {
			streamCancel()
			streamCancel = nil
		}

		sctx, cancel := context.WithCancel(ctx)
		stream, errs, err := client.SubscribeEvents(sctx, sessionID)
		if err != nil {
			cancel()
			return err
		}
		streamCancel = cancel
		resetStreamInconsistent()

		go func() {
			for {
				select {
				case e, ok := <-stream:
					if !ok {
						return
					}
					select {
					case eventCh <- e:
					default:
						markStreamInconsistent()
					}
				case <-sctx.Done():
					return
				}
			}
		}()

		go func() {
			for {
				select {
				case err, ok := <-errs:
					if !ok {
						return
					}
					if err != nil {
						markStreamInconsistent()
						return
					}
				case <-sctx.Done():
					return
				}
			}
		}()

		return nil
	}

	if err := subscribeSessionEvents(sessID); err != nil {
		return fmt.Errorf("subscribe events: %w", err)
	}
	defer func() {
		streamMu.Lock()
		if streamCancel != nil {
			streamCancel()
		}
		streamMu.Unlock()
		close(eventCh)
	}()

	m, err := New(modelName, agentName, workDir, sessID, eventCh)
	if err != nil {
		return err
	}

	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithContext(ctx))

	// Intercept Ctrl+S / Ctrl+D to trigger agent run
	// We wrap Update to detect when the model wants to send a message.
	// The simplest approach: poll the model's "pending send" via a custom Cmd.
	// Instead, we use a goroutine that watches for user input signals via a channel.
	sendCh := make(chan string, 1)
	abortCh := make(chan struct{}, 1)
	sessionCmdCh := make(chan SessionCommand, 4)
	var runMu sync.Mutex
	activeRunID := ""
	currentSessionID := sessID

	// Patch: override the model to use sendCh
	m.sendCh = sendCh
	m.abortCh = abortCh
	m.sessionCmdCh = sessionCmdCh
	p = tea.NewProgram(m, tea.WithAltScreen(), tea.WithContext(ctx))

	// Goroutine: run agent when user sends a message
	go func() {
		buildEntries := func(msgs []*sdk.Message) []msgEntry {
			renderToolBlock := func(parts []sdk.Part) string {
				var tb strings.Builder
				for _, p := range parts {
					if p.Type != sdk.PartTypeTool || p.Tool == nil {
						continue
					}
					sym := "●"
					suffix := ""
					switch p.Tool.State {
					case sdk.ToolStateCompleted:
						sym = "✓"
					case sdk.ToolStateError:
						sym = "✗"
					}
					if !p.Tool.StartAt.IsZero() && !p.Tool.EndAt.IsZero() {
						d := p.Tool.EndAt.Sub(p.Tool.StartAt)
						if d < 0 {
							d = 0
						}
						suffix = fmt.Sprintf(" (%s)", d)
					}
					tb.WriteString(fmt.Sprintf("%s %s%s\n", sym, p.Tool.Tool, suffix))
					if p.Tool.State == sdk.ToolStateError && p.Tool.Error != "" {
						out := p.Tool.Error
						if len(out) > 200 {
							out = out[:200] + "..."
						}
						tb.WriteString("  " + out + "\n")
					}
				}
				return strings.TrimSpace(tb.String())
			}

			out := make([]msgEntry, 0, len(msgs))
			for _, m := range msgs {
				switch m.Role {
				case sdk.RoleUser:
					text := strings.TrimSpace(m.Text)
					if text == "" {
						for _, p := range m.Parts {
							if p.Type == sdk.PartTypeText {
								text = strings.TrimSpace(p.Text)
								if text != "" {
									break
								}
							}
						}
					}
					if text != "" {
						out = append(out, msgEntry{role: "user", content: text})
					}
				case sdk.RoleAssistant:
					var reasoning strings.Builder
					toolBlock := renderToolBlock(m.Parts)
					var sb strings.Builder
					for _, p := range m.Parts {
						switch p.Type {
						case sdk.PartTypeReasoning:
							if text := strings.TrimSpace(p.Text); text != "" {
								if reasoning.Len() > 0 {
									reasoning.WriteString("\n\n")
								}
								reasoning.WriteString("💭 Thinking\n")
								reasoning.WriteString(text)
							}
						case sdk.PartTypeText:
							if p.Text != "" {
								sb.WriteString(p.Text)
							}
						}
					}
					text := strings.TrimSpace(sb.String())
					combined := strings.TrimSpace(reasoning.String())
					if toolBlock != "" {
						if combined != "" {
							combined += "\n\n"
						}
						combined += toolBlock
					}
					if text != "" {
						if combined != "" {
							combined += "\n\n"
						}
						combined += text
					}
					if combined != "" {
						out = append(out, msgEntry{role: "assistant", content: combined})
					}
				}
			}
			return out
		}

		loadAndSwitchSession := func(targetSessionID string, notice string) {
			if err := subscribeSessionEvents(targetSessionID); err != nil {
				p.Send(SessionSwitchedMsg{Err: err})
				return
			}
			msgs, err := client.GetMessages(ctx, targetSessionID)
			if err != nil {
				p.Send(SessionSwitchedMsg{Err: err})
				return
			}
			currentSessionID = targetSessionID
			p.Send(SessionSwitchedMsg{SessionID: targetSessionID, Entries: buildEntries(msgs), Messages: msgs, Notice: notice})
		}

		pollRun := func(runID string) error {
			for {
				run, err := client.GetRun(ctx, runID)
				if err != nil {
					return err
				}
				switch run.Status {
				case "completed":
					return nil
				case "failed":
					if run.Error != "" {
						return fmt.Errorf("%s", run.Error)
					}
					return fmt.Errorf("run failed")
				case "aborted":
					return fmt.Errorf("run aborted")
				}

				select {
				case <-abortCh:
					_ = client.AbortRun(ctx, runID)
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(200 * time.Millisecond):
				}
			}
		}

		for {
			select {
			case text := <-sendCh:
				text = strings.TrimSpace(text)
				if text == "" {
					continue
				}
				consumeStreamInconsistent()
				run, err := client.CreateRun(ctx, currentSessionID, text, agentName)
				if err == nil {
					runMu.Lock()
					activeRunID = run.ID
					runMu.Unlock()
					err = pollRun(run.ID)
					runMu.Lock()
					activeRunID = ""
					runMu.Unlock()
				}
				if err == nil && consumeStreamInconsistent() {
					msgs, syncErr := client.GetMessages(ctx, currentSessionID)
					if syncErr == nil {
						p.Send(SessionSwitchedMsg{SessionID: currentSessionID, Entries: buildEntries(msgs), Messages: msgs, Notice: "synced after stream gap"})
					}
				}
				p.Send(RunDoneMsg{Err: err})
			case <-abortCh:
				runMu.Lock()
				rid := activeRunID
				runMu.Unlock()
				if rid != "" {
					_ = client.AbortRun(ctx, rid)
				}
			case cmd := <-sessionCmdCh:
				runMu.Lock()
				busy := activeRunID != ""
				runMu.Unlock()
				if busy {
					p.Send(SessionSwitchedMsg{Err: fmt.Errorf("cannot switch session while a run is active")})
					continue
				}
				switch cmd.Type {
				case SessionCommandNew:
					sess, err := client.CreateSession(ctx, workDir)
					if err != nil {
						p.Send(SessionSwitchedMsg{Err: err})
						continue
					}
					loadAndSwitchSession(sess.ID, "new session created")
				case SessionCommandNext:
					sessions, err := client.ListSessions(ctx)
					if err != nil {
						p.Send(SessionSwitchedMsg{Err: err})
						continue
					}
					if len(sessions) == 0 {
						p.Send(SessionSwitchedMsg{Err: fmt.Errorf("no sessions available")})
						continue
					}
					sort.Slice(sessions, func(i, j int) bool {
						return sessions[i].UpdatedAt.After(sessions[j].UpdatedAt)
					})
					next := sessions[0].ID
					for i, s := range sessions {
						if s.ID == currentSessionID {
							next = sessions[(i+1)%len(sessions)].ID
							break
						}
					}
					if next == currentSessionID {
						p.Send(SessionSwitchedMsg{SessionID: currentSessionID, Notice: "already on latest session"})
						continue
					}
					loadAndSwitchSession(next, "session switched")
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	_, err = p.Run()
	return err
}
