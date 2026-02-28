package tui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/nolouch/gcode/internal/bus"
	"github.com/nolouch/gcode/internal/sdk"
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
		socketPath = filepath.Join(home, ".gcode", "run", "gcode.sock")
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

	stream, errs, err := client.SubscribeEvents(ctx, sessID)
	if err != nil {
		return fmt.Errorf("subscribe events: %w", err)
	}

	// Channel to pipe sdk events into Bubble Tea.
	eventCh := make(chan bus.Event, 128)
	go func() {
		defer close(eventCh)
		for {
			select {
			case e, ok := <-stream:
				if !ok {
					return
				}
				select {
				case eventCh <- e:
				default:
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	go func() {
		for {
			select {
			case err, ok := <-errs:
				if ok && err != nil {
					return
				}
			case <-ctx.Done():
				return
			}
		}
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
				err := client.SendMessage(ctx, sessID, text, agentName)
				p.Send(RunDoneMsg{Err: err})
			case <-ctx.Done():
				return
			}
		}
	}()

	_, err = p.Run()
	return err
}
