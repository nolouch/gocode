package tui

import "github.com/charmbracelet/lipgloss"

var (
	// Colors
	colorPrimary  = lipgloss.Color("33")  // blue
	colorSuccess  = lipgloss.Color("35")  // green (ansi)
	colorError    = lipgloss.Color("1")   // red
	colorWarning  = lipgloss.Color("3")   // yellow
	colorMuted    = lipgloss.Color("245") // gray
	colorThinking = lipgloss.Color("99")  // purple
	colorBorder   = lipgloss.Color("238") // dark gray
	colorAccent   = lipgloss.Color("212") // pink

	// Header
	StyleHeader = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("255"))

	StyleHeaderMeta = lipgloss.NewStyle().
			Foreground(colorMuted)

	StyleHeaderBorder = lipgloss.NewStyle().
				BorderStyle(lipgloss.NormalBorder()).
				BorderBottom(true).
				BorderForeground(colorBorder)

	// User message
	StyleUserBorder = lipgloss.NewStyle().
			BorderStyle(lipgloss.ThickBorder()).
			BorderLeft(true).
			BorderForeground(colorAccent).
			PaddingLeft(1)

	// Assistant message
	StyleAssistantMeta = lipgloss.NewStyle().
				Foreground(colorMuted).
				Italic(true)

	// Tool call
	StyleToolName    = lipgloss.NewStyle().Foreground(colorPrimary).Bold(true)
	StyleToolRunning = lipgloss.NewStyle().Foreground(colorWarning).Bold(true)
	StyleToolDone    = lipgloss.NewStyle().Foreground(colorSuccess).Bold(true)
	StyleToolError   = lipgloss.NewStyle().Foreground(colorError).Bold(true)
	StyleToolMeta    = lipgloss.NewStyle().Foreground(colorMuted)

	// Thinking
	StyleThinkingLabel = lipgloss.NewStyle().Foreground(colorThinking).Bold(true)
	StyleThinkingMuted = lipgloss.NewStyle().Foreground(colorMuted).Italic(true)

	// Input
	StyleInputBorder = lipgloss.NewStyle().
				BorderStyle(lipgloss.NormalBorder()).
				BorderTop(true).
				BorderForeground(colorBorder).
				PaddingTop(1)

	// Footer
	StyleFooter = lipgloss.NewStyle().
			Foreground(colorMuted)

	StyleFooterBorder = lipgloss.NewStyle().
				BorderStyle(lipgloss.NormalBorder()).
				BorderTop(true).
				BorderForeground(colorBorder)

	// Status
	StyleStatusOK  = lipgloss.NewStyle().Foreground(colorSuccess)
	StyleStatusErr = lipgloss.NewStyle().Foreground(colorError)
)
