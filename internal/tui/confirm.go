package tui

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/tatlilimon/PFS/internal/llm"
)

// ConfirmResult represents the user's choice in the confirmation dialog.
type ConfirmResult int

const (
	ConfirmRun ConfirmResult = iota
	ConfirmEdit
	ConfirmDismiss
)

// EditorError reports that editing the suggested command failed or was
// cancelled. Nothing is run in that case.
type EditorError struct{ Err error }

func (e *EditorError) Error() string { return "editor: " + e.Err.Error() + " (nothing was run)" }
func (e *EditorError) Unwrap() error { return e.Err }

type editorFinishedMsg struct{ err error }

// editorCommand opens path in $EDITOR (vi by default). EDITOR may contain
// arguments ("code --wait"), so the shell splits it, as git does; the file
// name is passed as a separate argument and never interpreted.
func editorCommand(path string) *exec.Cmd {
	editor := strings.TrimSpace(os.Getenv("EDITOR"))
	if editor == "" {
		editor = "vi"
	}
	return exec.Command("/bin/sh", "-c", editor+` "$1"`, "pfs-editor", path)
}

func openInEditorCmd(tmpFile string) tea.Cmd {
	// Bubbletea gives the editor the program's input and output (stderr),
	// so nothing the editor prints can reach stdout.
	return tea.ExecProcess(editorCommand(tmpFile), func(err error) tea.Msg {
		return editorFinishedMsg{err: err}
	})
}

func createEditTempFile(command string) (string, error) {
	f, err := os.CreateTemp("", "pfs-*.sh")
	if err != nil {
		return "", err
	}
	defer f.Close()

	if err := os.Chmod(f.Name(), 0600); err != nil {
		os.Remove(f.Name())
		return "", err
	}

	if _, err := f.WriteString(command + "\n"); err != nil {
		os.Remove(f.Name())
		return "", err
	}
	return f.Name(), nil
}

func readEditResult(path string) (string, error) {
	defer os.Remove(path)
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

func (m Model) handleEditorFinished(msg editorFinishedMsg) (tea.Model, tea.Cmd) {
	path := m.editTmpFile
	m.editTmpFile = ""
	m.CorrectedOutput = ""
	if msg.err != nil {
		if path != "" {
			os.Remove(path)
		}
		m.state = StateError
		m.err = &EditorError{Err: msg.err}
		return m, tea.Quit
	}
	if path == "" {
		return m, tea.Quit
	}
	edited, err := readEditResult(path)
	if err != nil {
		m.state = StateError
		m.err = &EditorError{Err: fmt.Errorf("failed to read edited file: %w", err)}
		return m, tea.Quit
	}
	// An emptied file means "do not run anything".
	m.CorrectedOutput = edited
	return m, tea.Quit
}

func (m Model) handleConfirmKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if !m.hasCommand() {
		if msg.String() == "x" || msg.String() == "X" || msg.String() == "q" || msg.Code == tea.KeyEscape {
			m.CorrectedOutput = ""
			return m, tea.Quit
		}
		return m, nil
	}
	cmd := *m.correction.CorrectedCommand

	// Second step for destructive commands: only y runs it, anything else
	// goes back to the normal dialog.
	if m.dangerArmed {
		m.dangerArmed = false
		if msg.String() == "y" || msg.String() == "Y" {
			m.CorrectedOutput = cmd
			return m, tea.Quit
		}
		return m, nil
	}

	// Handle escape via code check (String() may vary by terminal)
	if msg.Code == tea.KeyEscape && msg.Mod == 0 {
		m.CorrectedOutput = ""
		return m, tea.Quit
	}

	switch msg.String() {
	case "r", "R":
		if m.risk.Level == llm.RiskDanger {
			m.dangerArmed = true
			return m, nil
		}
		m.CorrectedOutput = cmd
		return m, tea.Quit
	case "e", "E":
		tmpFile, err := createEditTempFile(cmd)
		if err != nil {
			m.state = StateError
			m.err = &EditorError{Err: fmt.Errorf("failed to create temp file: %w", err)}
			return m, tea.Quit
		}
		m.editTmpFile = tmpFile
		m.CorrectedOutput = ""
		return m, openInEditorCmd(tmpFile)
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
		b.WriteString(m.styles.ExplanationStyle.Render(llm.SanitizeText(*m.correction.Explanation)))
		b.WriteString("\n\n")
	}

	if m.hasCommand() {
		cmd := *m.correction.CorrectedCommand
		boxed := m.styles.BorderStyle.Render(m.styles.CommandStyle.Render(cmd))
		b.WriteString(boxed)
		b.WriteString("\n")
		if n := strings.Count(cmd, "\n") + 1; n > 1 {
			b.WriteString(m.styles.MetadataStyle.Render(fmt.Sprintf("(%d lines — all of them will run)", n)))
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	if len(m.risk.Reasons) > 0 {
		label, style := "Note:", m.styles.WarningStyle
		if m.risk.Level == llm.RiskDanger {
			label, style = "⚠ Destructive:", m.styles.ErrorStyle
		}
		b.WriteString(style.Render(label + " " + strings.Join(m.risk.Reasons, "; ")))
		b.WriteString("\n")
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

	if m.dangerArmed {
		b.WriteString(m.styles.ErrorStyle.Render("This command looks destructive. Press y to run it anyway, any other key to go back."))
		return b.String()
	}

	runLabel := " Run  "
	if m.risk.Level == llm.RiskDanger {
		runLabel = " Run (asks again)  "
	}
	hints := m.styles.KeyHintStyle.Render("[r]") + runLabel +
		m.styles.KeyHintStyle.Render("[e]") + " Edit  " +
		m.styles.KeyHintStyle.Render("[x]") + " Dismiss"
	b.WriteString(hints)

	return b.String()
}
