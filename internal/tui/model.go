package tui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/glamour/v2"
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
	provider *llm.OllamaProvider
	envCtx   llm.EnvironmentContext
	command  string
	output   string
	exitCode int

	correction *llm.Correction
	err        error

	// CorrectedOutput is what the caller prints on stdout, where the shell
	// wrapper evaluates it. It is only set by an explicit user action.
	CorrectedOutput string
	WantsEdit       bool
	editTmpFile     string

	// risk is the assessment of the suggested command; dangerArmed is set
	// after the first "run" key press on a destructive command.
	risk        llm.Risk
	dangerArmed bool

	styles Styles
	width  int
	height int

	spinnerFrame int

	// gen identifies the active LLM request. Results carrying another
	// generation come from a request that was cancelled by a mode toggle.
	gen    int
	ctx    context.Context
	cancel context.CancelFunc

	stream *streamState
}

type spinnerTickMsg time.Time

type correctionResultMsg struct {
	gen        int
	correction *llm.Correction
	err        error
}

var errNoProvider = errors.New("no LLM provider configured")

func NewModel(cfg *config.Config, provider *llm.OllamaProvider, envCtx llm.EnvironmentContext, command, output string, exitCode int) Model {
	ctx, cancel := context.WithCancel(context.Background())
	return Model{
		state:    StateLoading,
		config:   cfg,
		provider: provider,
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
	return tea.Batch(tickSpinner(), m.startLLMCall())
}

func (m Model) startLLMCall() tea.Cmd {
	if m.provider == nil {
		return nil
	}
	return startNonStreamingCmd(m.ctx, m.provider, m.gen, m.command, m.output, m.exitCode, m.envCtx)
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
			m.spinnerFrame = (m.spinnerFrame + 1) % len([]rune(spinnerFrames))
			return m, tickSpinner()
		}
		if m.state == StateStreaming {
			return m, tickSpinner()
		}
		return m, nil
	case streamingDoneMsg:
		if m.state != StateStreaming || msg.gen != m.gen {
			return m, nil
		}
		return m.applyResult(msg.correction, msg.err)
	case correctionResultMsg:
		if m.state == StateStreaming || msg.gen != m.gen {
			return m, nil
		}
		return m.applyResult(msg.correction, msg.err)
	case editorFinishedMsg:
		return m.handleEditorFinished(msg)
	default:
		return m, nil
	}
}

// applyResult moves the model to the state that matches an LLM answer.
func (m Model) applyResult(c *llm.Correction, err error) (tea.Model, tea.Cmd) {
	if err == nil && c == nil {
		err = errors.New("the model returned no answer")
	}
	if err == nil && c.CorrectedCommand != nil && *c.CorrectedCommand != "" {
		if verr := llm.ValidateCommand(*c.CorrectedCommand); verr != nil {
			err = fmt.Errorf("unsafe suggestion rejected: %w", verr)
		}
	}
	if err != nil {
		m.state = StateError
		m.err = err
		return m, tea.Quit
	}
	m.correction = c
	m.dangerArmed = false
	if m.hasCommand() {
		m.state = StateConfirm
		m.risk = llm.AssessCommand(*c.CorrectedCommand)
	} else {
		m.state = StateResult
		m.risk = llm.Risk{}
	}
	return m, nil
}

func (m Model) hasCommand() bool {
	return m.correction != nil && m.correction.CorrectedCommand != nil && *m.correction.CorrectedCommand != ""
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
	case StateResult:
		if m.hasCommand() {
			return m.handleConfirmKey(msg)
		}
		fallthrough
	case StateError:
		if msg.String() == "x" || msg.String() == "q" || msg.Code == tea.KeyEscape {
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
	frame := string([]rune(spinnerFrames)[m.spinnerFrame%len([]rune(spinnerFrames))])
	spinner := m.styles.TitleStyle.Render(frame)
	text := fmt.Sprintf("%s Analyzing command...", spinner)
	hint := m.styles.MetadataStyle.Render("Press Ctrl+X for streaming mode")
	return text + "\n" + hint
}

func (m Model) viewResult() string {
	if m.hasCommand() {
		return m.viewConfirm()
	}

	var b strings.Builder
	b.WriteString(m.styles.TitleStyle.Render("Diagnosis"))
	b.WriteString("\n")
	b.WriteString(renderMarkdown(m.correction.Diagnosis))
	b.WriteString("\n\n")

	if m.correction.Explanation != nil && *m.correction.Explanation != "" {
		b.WriteString(m.styles.ExplanationStyle.Render(llm.SanitizeText(*m.correction.Explanation)))
		b.WriteString("\n\n")
	}

	if len(m.correction.AlternativeCommands) > 0 {
		b.WriteString(m.styles.TitleStyle.Render("No single fix. Possible approaches (not run):"))
		b.WriteString("\n")
		for _, alt := range m.correction.AlternativeCommands {
			b.WriteString("  • " + llm.SanitizeText(alt) + "\n")
		}
		b.WriteString("\n")
	}

	confidenceLabel := fmt.Sprintf("Confidence: %s", strings.ToUpper(m.correction.Confidence))
	b.WriteString(m.styles.ConfidenceStyle(m.correction.Confidence).Render(confidenceLabel))
	b.WriteString("\n\n")

	b.WriteString(m.styles.KeyHintStyle.Render("[x]") + " Dismiss")
	return b.String()
}

func (m Model) viewError() string {
	errText := m.styles.ErrorStyle.Render("Error: " + llm.SanitizeText(fmt.Sprint(m.err)))
	var editorErr *EditorError
	var hint string
	switch {
	case errors.As(m.err, &editorErr):
		hint = "Nothing was run."
	case errors.Is(m.err, llm.ErrModelNotFound):
		hint = "Pull the model first: ollama pull " + m.modelName()
	case errors.Is(m.err, llm.ErrOllamaUnavailable):
		hint = "Start Ollama with: ollama serve"
	default:
		hint = "Check that Ollama is running, or try running: pfs setup"
	}
	return errText + "\n" + m.styles.MetadataStyle.Render(hint)
}

func (m Model) modelName() string {
	if m.config != nil && m.config.OllamaModel != "" {
		return m.config.OllamaModel
	}
	return "<model>"
}

// SetCorrection delivers an LLM correction result to the model.
func (m *Model) SetCorrection(correction *llm.Correction) {
	m.correction = correction
	m.state = StateResult
	m.dangerArmed = false
	if m.hasCommand() {
		m.risk = llm.AssessCommand(*correction.CorrectedCommand)
	}
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
	return isCharDevice(os.Stderr)
}

// IsInteractive reports whether a confirmation dialog can be shown: the TUI
// draws on stderr and reads keys from stdin, so both must be terminals.
func IsInteractive() bool {
	return isCharDevice(os.Stderr) && isCharDevice(os.Stdin)
}

func isCharDevice(f *os.File) bool {
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

// renderMarkdown renders model-written text. Escape sequences are removed
// first, and NO_COLOR disables markdown styling entirely.
func renderMarkdown(text string) string {
	text = llm.SanitizeText(text)
	if noColor {
		return strings.TrimSpace(text)
	}
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

// GetCorrection returns the LLM answer, or nil if none arrived.
func (m Model) GetCorrection() *llm.Correction {
	return m.correction
}

// Ambiguous reports an answer with no single fix but several possible approaches.
func (m Model) Ambiguous() bool {
	return m.correction != nil && !m.hasCommand() && len(m.correction.AlternativeCommands) > 0
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
