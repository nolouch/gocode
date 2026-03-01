package bus

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/nolouch/gocode/internal/model"
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
		case EventPartDelta:
			if p, ok := e.Payload.(PartDeltaPayload); ok {
				switch p.PartType {
				case model.PartTypeText:
					fmt.Print(p.Delta)
				case model.PartTypeReasoning:
					fmt.Print(p.Delta)
				}
			}

		case EventPartUpsert:
			if p, ok := e.Payload.(PartUpsertPayload); ok {
				if p.Part.Type != model.PartTypeTool || p.Part.Tool == nil {
					break
				}
				tp := p.Part.Tool
				durMs := int64(0)
				if !tp.StartAt.IsZero() && !tp.EndAt.IsZero() {
					durMs = tp.EndAt.Sub(tp.StartAt).Milliseconds()
					if durMs < 0 {
						durMs = 0
					}
				}
				switch tp.State {
				case model.ToolStatePending, model.ToolStateRunning:
					fmt.Printf("\n%s %s\n",
						styleRunning.Render("●"),
						styleToolName.Render("Tool: "+tp.Tool))
				case model.ToolStateCompleted:
					fmt.Printf("%s %s (%dms)\n",
						styleSuccess.Render("✓"),
						styleToolName.Render("Tool: "+tp.Tool),
						durMs)
					out := tp.Output
					if len(out) > 500 {
						out = out[:500] + "\n... (truncated)"
					}
					if out != "" {
						fmt.Println(out)
					}
				case model.ToolStateError:
					fmt.Printf("%s %s (%dms)\n",
						styleError.Render("✗"),
						styleToolName.Render("Tool: "+tp.Tool),
						durMs)
					fmt.Printf("[tool-error] %s\n", tp.Error)
				}
			}

		case EventPartDone:
			if p, ok := e.Payload.(PartDonePayload); ok && p.PartType == model.PartTypeReasoning {
				fmt.Println()
				fmt.Println(styleThinkDone.Render(fmt.Sprintf("✓ done (%.0fms)", p.DurationMs)))
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
