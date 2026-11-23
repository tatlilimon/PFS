package tui

import (
	"context"
	"strings"
	"sync"

	tea "charm.land/bubbletea/v2"

	"github.com/tatlilimon/PFS/internal/llm"
)

// maxStreamDisplay bounds the text kept for the live streaming view.
const maxStreamDisplay = 64 << 10

// streamState holds mutable streaming state behind a pointer to avoid
// copying the mutex when Model is passed by value through bubbletea's
// Update loop.
type streamState struct {
	mu  sync.Mutex
	buf strings.Builder
}

// streamingDoneMsg is sent when a streaming LLM call completes (or fails).
type streamingDoneMsg struct {
	gen        int
	correction *llm.Correction
	err        error
}

// startStreamingCmd returns a tea.Cmd that performs a streaming LLM call
// through the provider, so the answer is validated and retried exactly like
// a normal request. Tokens are written to ss.buf (mutex-protected) for
// incremental display via the spinner tick loop.
func startStreamingCmd(
	ctx context.Context,
	provider *llm.OllamaProvider,
	ss *streamState,
	gen int,
	command, output string,
	exitCode int,
	envCtx llm.EnvironmentContext,
) tea.Cmd {
	return func() tea.Msg {
		if provider == nil {
			return streamingDoneMsg{gen: gen, err: errNoProvider}
		}
		correction, err := provider.GetCorrectionStream(ctx, command, output, exitCode, envCtx, func(token string) {
			ss.mu.Lock()
			if ss.buf.Len() < maxStreamDisplay {
				ss.buf.WriteString(token)
			}
			ss.mu.Unlock()
		})
		return streamingDoneMsg{gen: gen, correction: correction, err: err}
	}
}

// startNonStreamingCmd returns a tea.Cmd that performs a non-streaming LLM
// call. Used for the first request and when toggling back from streaming to
// spinner mode. Returns correctionResultMsg.
func startNonStreamingCmd(
	ctx context.Context,
	provider *llm.OllamaProvider,
	gen int,
	command, output string,
	exitCode int,
	envCtx llm.EnvironmentContext,
) tea.Cmd {
	return func() tea.Msg {
		if provider == nil {
			return correctionResultMsg{gen: gen, err: errNoProvider}
		}
		correction, err := provider.GetCorrection(ctx, command, output, exitCode, envCtx)
		return correctionResultMsg{gen: gen, correction: correction, err: err}
	}
}

// handleCtrlX toggles between spinner (StateLoading) and streaming
// (StateStreaming) modes during an active LLM call, or quits the
// program in other states.
func (m Model) handleCtrlX() (tea.Model, tea.Cmd) {
	switch m.state {
	case StateLoading:
		if m.cancel != nil {
			m.cancel()
		}
		m.gen++
		m.ctx, m.cancel = context.WithCancel(context.Background())
		m.stream = &streamState{}
		m.state = StateStreaming
		return m, startStreamingCmd(
			m.ctx, m.provider, m.stream, m.gen,
			m.command, m.output, m.exitCode, m.envCtx,
		)

	case StateStreaming:
		if m.cancel != nil {
			m.cancel()
		}
		m.gen++
		m.ctx, m.cancel = context.WithCancel(context.Background())
		m.stream = nil
		m.state = StateLoading
		return m, startNonStreamingCmd(
			m.ctx, m.provider, m.gen,
			m.command, m.output, m.exitCode, m.envCtx,
		)

	default:
		m.CorrectedOutput = ""
		return m, tea.Quit
	}
}

// viewStreaming renders the streaming state. Reads tokens from the
// mutex-protected buffer and renders via glamour markdown. The spinner
// tick loop (100ms) provides natural debounce — View() reads the buffer
// on each tick, so display updates at most every 100ms.
func (m Model) viewStreaming() string {
	var text string
	if m.stream != nil {
		m.stream.mu.Lock()
		text = m.stream.buf.String()
		m.stream.mu.Unlock()
	}

	hint := m.styles.MetadataStyle.Render("Press Ctrl+X to switch back to spinner")

	if text == "" {
		frames := []rune(spinnerFrames)
		frame := string(frames[m.spinnerFrame%len(frames)])
		return m.styles.TitleStyle.Render(frame) + " Streaming response...\n" + hint
	}

	rendered := renderMarkdown(text)
	return rendered + "\n" + hint
}
