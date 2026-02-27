package processor

import "github.com/charmbracelet/lipgloss"

// Box styles for partitioned display
var (
	// ThinkingBox for reasoning output - simpler style
	ThinkingHeader = lipgloss.NewStyle().
			Foreground(lipgloss.Color("99")). // purple
			Bold(true)

	ThinkingContent = lipgloss.NewStyle().
				Foreground(lipgloss.Color("245")) // gray

	ThinkingDone = lipgloss.NewStyle().
			Foreground(lipgloss.Color("99")). // purple
			Italic(true)

	// Tool styles
	ToolHeader = lipgloss.NewStyle().
			Foreground(lipgloss.Color("33")). // blue
			Bold(true)

	ToolDone = lipgloss.NewStyle().
		  Foreground(lipgloss.Color("33")). // blue
		  Italic(true)

	// Status icons
	SuccessIcon = lipgloss.NewStyle().Foreground(lipgloss.Color("green")).Bold(true)
	ErrorIcon   = lipgloss.NewStyle().Foreground(lipgloss.Color("red")).Bold(true)
	RunningIcon = lipgloss.NewStyle().Foreground(lipgloss.Color("yellow")).Bold(true)
)
