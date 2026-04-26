package tui

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"charm.land/glamour/v2"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/tatlilimon/PFS/internal/config"
	"github.com/tatlilimon/PFS/internal/llm"
)

type State int

const (
	StateLoading State = iota
	StateResult
	StateError
	StateConfirm
	StateStreaming
)

const spinnerFrames = "⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏"

type Model struct {
	state    State
	config   *config.Config
	envCtx   llm.EnvironmentContext
	command  string
	output   string
	exitCode int

	correction *llm.Correction
	err        error

	CorrectedOutput string
	WantsEdit       bool

	styles Styles
	width  int
	height int

	spinnerFrame int

	ctx    context.Context
	cancel context.CancelFunc

	stream *streamState
}

type spinnerTickMsg time.Time

type correctionResultMsg struct {
	correction *llm.Correction
	err        error
}

func NewModel(cfg *config.Config, envCtx llm.EnvironmentContext, command, output string, exitCode int) Model {
	ctx, cancel := context.WithCancel(context.Background())
	return Model{
		state:    StateLoading,
		config:   cfg,
		envCtx:   envCtx,
		command:  command,
		output:   output,
		exitCode: exitCode,
		styles:   DefaultStyles(),
		width:    80,
		ctx:      ctx,
		cancel:   cancel,
	}
}

func (m Model) Init() tea.Cmd {
	return tickSpinner()
}

func tickSpinner() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg {
		return spinnerTickMsg(t)
	})
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		return m.handleKey(msg)
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case spinnerTickMsg:
		if m.state == StateLoading {
			m.spinnerFrame = (m.spinnerFrame + 1) % len(spinnerFrames)
			return m, tickSpinner()
		}
		if m.state == StateStreaming {
			return m, tickSpinner()
		}
		return m, nil
	case streamingDoneMsg:
		if m.state != StateStreaming {
			return m, nil
		}
		if msg.err != nil {
			m.state = StateError
			m.err = msg.err
			return m, tea.Quit
		}
		m.correction = msg.correction
		if msg.correction.CorrectedCommand != nil && *msg.correction.CorrectedCommand != "" {
			m.state = StateConfirm
		} else {
			m.state = StateResult
		}
		return m, nil
	case correctionResultMsg:
		if m.state == StateStreaming {
			return m, nil
		}
		if msg.err != nil {
			m.state = StateError
			m.err = msg.err
			return m, tea.Quit
		}
		m.correction = msg.correction
		if msg.correction.CorrectedCommand != nil && *msg.correction.CorrectedCommand != "" {
			m.state = StateConfirm
		} else {
			m.state = StateResult
		}
		return m, nil
	default:
		return m, nil
	}
}

func (m Model) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		if m.cancel != nil {
			m.cancel()
		}
		m.CorrectedOutput = ""
		return m, tea.Quit
	case "ctrl+x":
		return m.handleCtrlX()
	}

	switch m.state {
	case StateConfirm:
		return m.handleConfirmKey(msg)
	case StateResult, StateError:
		if msg.String() == "x" || msg.String() == "q" {
			m.CorrectedOutput = ""
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m Model) View() tea.View {
	switch m.state {
	case StateLoading:
		return tea.NewView(m.viewLoading())
	case StateStreaming:
		return tea.NewView(m.viewStreaming())
	case StateConfirm:
		return tea.NewView(m.viewConfirm())
	case StateResult:
		return tea.NewView(m.viewResult())
	case StateError:
		return tea.NewView(m.viewError())
	default:
		return tea.NewView("")
	}
}

func (m Model) viewLoading() string {
	frame := string(spinnerFrames[m.spinnerFrame])
	spinner := m.styles.TitleStyle.Render(frame)
	text := fmt.Sprintf("%s Analyzing command...", spinner)
	hint := m.styles.MetadataStyle.Render("Press Ctrl+X for streaming mode")
	return text + "\n" + hint
}

func (m Model) viewResult() string {
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

	confidenceLabel := fmt.Sprintf("Confidence: %s", strings.ToUpper(m.correction.Confidence))
	b.WriteString(m.styles.ConfidenceStyle(m.correction.Confidence).Render(confidenceLabel))
	b.WriteString("\n\n")

	hints := m.styles.KeyHintStyle.Render("[r]") + " Run  " +
		m.styles.KeyHintStyle.Render("[e]") + " Edit  " +
		m.styles.KeyHintStyle.Render("[x]") + " Dismiss"
	b.WriteString(hints)

	return b.String()
}

func (m Model) viewError() string {
	errText := m.styles.ErrorStyle.Render(fmt.Sprintf("Error: %v", m.err))
	hint := m.styles.MetadataStyle.Render("Try running: pfs setup")
	return errText + "\n" + hint
}

// SetCorrection delivers an LLM correction result to the model.
func (m *Model) SetCorrection(correction *llm.Correction) {
	m.correction = correction
	m.state = StateResult
}

// SetError delivers an error to the model.
func (m *Model) SetError(err error) {
	m.err = err
	m.state = StateError
}

// CorrectionResultCmd creates a tea.Cmd that delivers a correction result.
func CorrectionResultCmd(correction *llm.Correction, err error) tea.Cmd {
	return func() tea.Msg {
		return correctionResultMsg{correction: correction, err: err}
	}
}

// RunProgram creates and runs a Bubbletea program with stderr output.
func RunProgram(model Model) (Model, error) {
	p := tea.NewProgram(model, tea.WithOutput(os.Stderr))
	final, err := p.Run()
	if err != nil {
		return model, fmt.Errorf("TUI error: %w", err)
	}
	return final.(Model), nil
}

// IsTerminal checks if stderr is a TTY.
func IsTerminal() bool {
	fi, err := os.Stderr.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

func renderMarkdown(text string) string {
	r, err := glamour.NewTermRenderer(glamour.WithEnvironmentConfig())
	if err != nil {
		return text
	}
	rendered, err := r.Render(text)
	if err != nil {
		return text
	}
	return strings.TrimSpace(rendered)
}

// GetCommand returns the corrected command if available.
func (m Model) GetCommand() string {
	if m.correction != nil && m.correction.CorrectedCommand != nil {
		return *m.correction.CorrectedCommand
	}
	return ""
}

// GetCorrectedOutput returns the final output for stdout after user confirmation.
func (m Model) GetCorrectedOutput() string {
	return m.CorrectedOutput
}

// GetState returns the current model state.
func (m Model) GetState() State {
	return m.state
}

// GetError returns the error if in error state.
func (m Model) GetError() error {
	return m.err
}

// InlineStyle returns a lipgloss.Style for inline text rendering.
func InlineStyle() lipgloss.Style {
	return lipgloss.NewStyle()
}
