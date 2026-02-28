package tui

import (
	"context"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/nolouch/gcode/internal/bus"
	"github.com/nolouch/gcode/internal/loop"
	"github.com/nolouch/gcode/internal/session"
)

// Run starts the Bubble Tea TUI, wiring the event bus to the model.
func Run(
	ctx context.Context,
	modelName, agentName, workDir string,
	store *session.Store,
	runner *loop.Runner,
	evBus *bus.Bus,
) error {
	// Create session
	sess := store.CreateSession(workDir)

	// Channel to pipe bus events into Bubble Tea
	eventCh := make(chan bus.Event, 128)
	unsub := evBus.Subscribe(func(e bus.Event) {
		select {
		case eventCh <- e:
		default:
		}
	})
	defer unsub()

	m, err := New(modelName, agentName, workDir, sess.ID, eventCh)
	if err != nil {
		return err
	}

	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithContext(ctx))

	// Intercept Ctrl+S / Ctrl+D to trigger agent run
	// We wrap Update to detect when the model wants to send a message.
	// The simplest approach: poll the model's "pending send" via a custom Cmd.
	// Instead, we use a goroutine that watches for user input signals via a channel.
	sendCh := make(chan string, 1)

	// Patch: override the model to use sendCh
	m.sendCh = sendCh
	p = tea.NewProgram(m, tea.WithAltScreen(), tea.WithContext(ctx))

	// Goroutine: run agent when user sends a message
	go func() {
		for {
			select {
			case text := <-sendCh:
				text = strings.TrimSpace(text)
				if text == "" {
					continue
				}
				err := runner.Run(ctx, sess.ID, text, agentName)
				p.Send(RunDoneMsg{Err: err})
			case <-ctx.Done():
				return
			}
		}
	}()

	_, err = p.Run()
	return err
}
