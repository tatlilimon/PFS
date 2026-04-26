package tui

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// ConfirmResult represents the user's choice in the confirmation dialog.
type ConfirmResult int

const (
	ConfirmRun     ConfirmResult = iota
	ConfirmEdit
	ConfirmDismiss
)

// openInEditor opens the command in $EDITOR (or vi) for editing.
// It creates a secure temp file, runs the editor, and reads back the result.
func openInEditor(command string) (string, error) {
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vi"
	}

	f, err := os.CreateTemp("", "pfs-*.sh")
	if err != nil {
		return "", err
	}
	defer os.Remove(f.Name())

	if _, err := f.WriteString(command); err != nil {
		f.Close()
		return "", err
	}
	f.Close()

	if err := os.Chmod(f.Name(), 0600); err != nil {
		return "", err
	}

	cmd := exec.Command(editor, f.Name())
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return "", err
	}

	modified, err := os.ReadFile(f.Name())
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(string(modified)), nil
}

func (m Model) handleConfirmKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	// Handle escape via code check (String() may vary by terminal)
	if msg.Code == tea.KeyEscape && msg.Mod == 0 {
		m.CorrectedOutput = ""
		return m, tea.Quit
	}

	switch msg.String() {
	case "r", "R":
		if m.correction != nil && m.correction.CorrectedCommand != nil {
			m.CorrectedOutput = *m.correction.CorrectedCommand
			return m, tea.Quit
		}
	case "e", "E":
		if m.correction != nil && m.correction.CorrectedCommand != nil {
			m.WantsEdit = true
			m.CorrectedOutput = *m.correction.CorrectedCommand
			return m, tea.Quit
		}
	case "x", "X", "q":
		m.CorrectedOutput = ""
		return m, tea.Quit
	}
	return m, nil
}

func (m Model) viewConfirm() string {
	var b strings.Builder

	b.WriteString(m.styles.TitleStyle.Render("Diagnosis"))
	b.WriteString("\n")
	b.WriteString(renderMarkdown(m.correction.Diagnosis))
	b.WriteString("\n\n")

	if m.correction.Explanation != nil && *m.correction.Explanation != "" {
		b.WriteString(m.styles.ExplanationStyle.Render(*m.correction.Explanation))
		b.WriteString("\n\n")
	}

	if m.correction.CorrectedCommand != nil && *m.correction.CorrectedCommand != "" {
		cmd := *m.correction.CorrectedCommand
		boxed := m.styles.BorderStyle.Render(m.styles.CommandStyle.Render(cmd))
		b.WriteString(boxed)
		b.WriteString("\n\n")
	}

	var confidenceIcon string
	switch m.correction.Confidence {
	case "high":
		confidenceIcon = "✓"
	case "medium":
		confidenceIcon = "◆"
	case "low":
		confidenceIcon = "⚠"
	default:
		confidenceIcon = "◆"
	}
	confidenceLabel := fmt.Sprintf("%s Confidence: %s", confidenceIcon, strings.ToUpper(m.correction.Confidence))
	b.WriteString(m.styles.ConfidenceStyle(m.correction.Confidence).Render(confidenceLabel))
	b.WriteString("\n")

	if m.correction.Confidence == "low" {
		b.WriteString(m.styles.WarningStyle.Render("⚠ Low confidence — review carefully"))
		b.WriteString("\n")
	}

	b.WriteString("\n")

	hints := m.styles.KeyHintStyle.Render("[r]") + " Run  " +
		m.styles.KeyHintStyle.Render("[e]") + " Edit  " +
		m.styles.KeyHintStyle.Render("[x]") + " Dismiss"
	b.WriteString(hints)

	return b.String()
}
