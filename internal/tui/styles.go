package tui

import (
	"os"

	"charm.land/lipgloss/v2"
)

// noColor indicates whether the NO_COLOR environment variable is set.
// When true, styles will be created without color.
var noColor bool

func init() {
	_, noColor = os.LookupEnv("NO_COLOR")
}

// Styles holds all Lipgloss styles used by the TUI.
type Styles struct {
	// TitleStyle - bold, bright accent for headers.
	TitleStyle lipgloss.Style

	// ExplanationStyle - dimmed for secondary text.
	ExplanationStyle lipgloss.Style

	// CommandStyle - bold green, highlighted background for corrected command.
	CommandStyle lipgloss.Style

	// ErrorStyle - red for errors.
	ErrorStyle lipgloss.Style

	// WarningStyle - yellow for warnings (low confidence).
	WarningStyle lipgloss.Style

	// MetadataStyle - dim gray for timestamps, model info.
	MetadataStyle lipgloss.Style

	// KeyHintStyle - bold for keyboard shortcut hints.
	KeyHintStyle lipgloss.Style

	// BorderStyle - for boxing the corrected command.
	BorderStyle lipgloss.Style

	// ConfidenceHigh - green indicator.
	ConfidenceHigh lipgloss.Style

	// ConfidenceMedium - yellow indicator.
	ConfidenceMedium lipgloss.Style

	// ConfidenceLow - red indicator.
	ConfidenceLow lipgloss.Style
}

// DefaultStyles returns the default style set. Colors are automatically
// downsampled based on the terminal's color profile when rendered via
// Bubbletea or lipgloss print functions.
func DefaultStyles() Styles {
	if noColor {
		return noColorStyles()
	}
	return colorStyles()
}

func colorStyles() Styles {
	return Styles{
		TitleStyle: lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#7D56F4")).
			MarginBottom(1),

		ExplanationStyle: lipgloss.NewStyle().
			Faint(true).
			MarginBottom(1),

		CommandStyle: lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#04B575")).
			Background(lipgloss.Color("#1A1A2E")).
			Padding(0, 1),

		ErrorStyle: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FF4757")).
			Bold(true),

		WarningStyle: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFC048")),

		MetadataStyle: lipgloss.NewStyle().
			Faint(true).
			Foreground(lipgloss.Color("#6C6C6C")),

		KeyHintStyle: lipgloss.NewStyle().
			Bold(true),

		BorderStyle: lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#7D56F4")).
			Padding(0, 1),

		ConfidenceHigh: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#04B575")).
			Bold(true),

		ConfidenceMedium: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFC048")),

		ConfidenceLow: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FF4757")),
	}
}

func noColorStyles() Styles {
	return Styles{
		TitleStyle: lipgloss.NewStyle().
			Bold(true).
			MarginBottom(1),

		ExplanationStyle: lipgloss.NewStyle().
			Faint(true).
			MarginBottom(1),

		CommandStyle: lipgloss.NewStyle().
			Bold(true).
			Padding(0, 1),

		ErrorStyle: lipgloss.NewStyle().
			Bold(true),

		WarningStyle: lipgloss.NewStyle(),

		MetadataStyle: lipgloss.NewStyle().
			Faint(true),

		KeyHintStyle: lipgloss.NewStyle().
			Bold(true),

		BorderStyle: lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			Padding(0, 1),

		ConfidenceHigh: lipgloss.NewStyle().
			Bold(true),

		ConfidenceMedium: lipgloss.NewStyle(),

		ConfidenceLow: lipgloss.NewStyle(),
	}
}

// ConfidenceStyle returns the style for the given confidence level.
func (s Styles) ConfidenceStyle(confidence string) lipgloss.Style {
	switch confidence {
	case "high":
		return s.ConfidenceHigh
	case "medium":
		return s.ConfidenceMedium
	case "low":
		return s.ConfidenceLow
	default:
		return s.ConfidenceMedium
	}
}
