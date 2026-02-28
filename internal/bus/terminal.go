package bus

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	styleThinking  = lipgloss.NewStyle().Foreground(lipgloss.Color("99")).Bold(true)
	styleThinkDone = lipgloss.NewStyle().Foreground(lipgloss.Color("99")).Italic(true)
	styleToolName  = lipgloss.NewStyle().Foreground(lipgloss.Color("33")).Bold(true)
	styleSuccess   = lipgloss.NewStyle().Foreground(lipgloss.Color("green")).Bold(true)
	styleError     = lipgloss.NewStyle().Foreground(lipgloss.Color("red")).Bold(true)
	styleRunning   = lipgloss.NewStyle().Foreground(lipgloss.Color("yellow")).Bold(true)
)

// SubscribeTerminal attaches a simple terminal printer to the bus.
// This is the fallback renderer used before the TUI is implemented.
func SubscribeTerminal(b *Bus) {
	b.Subscribe(func(e Event) {
		switch e.Type {
		case EventTextDelta:
			if p, ok := e.Payload.(TextDeltaPayload); ok {
				fmt.Print(p.Delta)
			}

		case EventThinking:
			if p, ok := e.Payload.(ThinkingPayload); ok {
				if p.Delta == "" {
					fmt.Print(styleThinking.Render("💭 Thinking: "))
				} else {
					fmt.Print(p.Delta)
				}
			}

		case EventThinkingDone:
			if p, ok := e.Payload.(ThinkingPayload); ok {
				fmt.Println()
				fmt.Println(styleThinkDone.Render(fmt.Sprintf("✓ done (%.0fms)", p.Duration)))
			}

		case EventToolStart:
			if p, ok := e.Payload.(ToolPayload); ok {
				fmt.Printf("\n%s %s\n",
					styleRunning.Render("●"),
					styleToolName.Render("Tool: "+p.Tool))
			}

		case EventToolDone:
			if p, ok := e.Payload.(ToolPayload); ok {
				fmt.Printf("%s %s (%dms)\n",
					styleSuccess.Render("✓"),
					styleToolName.Render("Tool: "+p.Tool),
					p.DurationMs)
				out := p.Output
				if len(out) > 500 {
					out = out[:500] + "\n... (truncated)"
				}
				if out != "" {
					fmt.Println(out)
				}
			}

		case EventToolError:
			if p, ok := e.Payload.(ToolPayload); ok {
				fmt.Printf("%s %s (%dms)\n",
					styleError.Render("✗"),
					styleToolName.Render("Tool: "+p.Tool),
					p.DurationMs)
				fmt.Printf("[tool-error] %s\n", p.Output)
			}

		case EventTurnDone:
			if p, ok := e.Payload.(TurnDonePayload); ok {
				if !strings.HasPrefix(p.FinishReason, "tool") {
					fmt.Println()
				}
			}

		case EventTurnError:
			if p, ok := e.Payload.(TurnDonePayload); ok {
				fmt.Printf("\n[error] turn failed: %s\n", p.FinishReason)
			}
		}
	})
}
