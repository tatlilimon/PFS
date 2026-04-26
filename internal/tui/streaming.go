package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/ollama/ollama/api"
	tea "charm.land/bubbletea/v2"

	"github.com/tatlilimon/PFS/internal/config"
	"github.com/tatlilimon/PFS/internal/llm"
)

// streamState holds mutable streaming state behind a pointer to avoid
// copying the mutex when Model is passed by value through bubbletea's
// Update loop.
type streamState struct {
	mu  sync.Mutex
	buf strings.Builder
}

// streamingDoneMsg is sent when a streaming LLM call completes (or fails).
type streamingDoneMsg struct {
	correction *llm.Correction
	err        error
}

// startStreamingCmd returns a tea.Cmd that performs a streaming LLM call.
// Tokens are written to ss.buf (mutex-protected) for incremental display
// via the spinner tick loop. The Cmd returns streamingDoneMsg when the
// full response is received and parsed.
func startStreamingCmd(
	ctx context.Context,
	cfg *config.Config,
	ss *streamState,
	command, output string,
	exitCode int,
	envCtx llm.EnvironmentContext,
) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
		defer cancel()

		parsedURL, err := url.Parse(cfg.OllamaBaseURL)
		if err != nil {
			return streamingDoneMsg{err: fmt.Errorf("streaming: bad URL: %w", err)}
		}

		client := api.NewClient(parsedURL, http.DefaultClient)

		systemPrompt := llm.SystemPrompt(envCtx)
		userPrompt := llm.UserPrompt(command, output, exitCode, envCtx.CWD)
		schema := llm.CorrectionSchema()

		stream := true
		req := &api.GenerateRequest{
			Model:  cfg.OllamaModel,
			Prompt: userPrompt,
			System: systemPrompt,
			Format: schema,
			Stream: &stream,
			Options: map[string]interface{}{
				"temperature": 0,
			},
		}

		var fullResponse strings.Builder
		respFunc := func(resp api.GenerateResponse) error {
			token := resp.Response
			fullResponse.WriteString(token)

			ss.mu.Lock()
			ss.buf.WriteString(token)
			ss.mu.Unlock()

			return nil
		}

		if err := client.Generate(ctx, req, respFunc); err != nil {
			return streamingDoneMsg{err: fmt.Errorf("streaming API error: %w", err)}
		}

		raw := fullResponse.String()
		jsonStr := extractJSONFromStream(raw)
		if jsonStr == "" {
			return streamingDoneMsg{err: fmt.Errorf("streaming: no valid JSON in response")}
		}

		var correction llm.Correction
		if err := json.Unmarshal([]byte(jsonStr), &correction); err != nil {
			return streamingDoneMsg{err: fmt.Errorf("streaming: parse error: %w", err)}
		}

		return streamingDoneMsg{correction: &correction}
	}
}

// startNonStreamingCmd returns a tea.Cmd that performs a non-streaming LLM
// call. Used when toggling back from streaming to spinner mode. Returns
// correctionResultMsg so the existing handler in Update processes it.
func startNonStreamingCmd(
	ctx context.Context,
	cfg *config.Config,
	command, output string,
	exitCode int,
	envCtx llm.EnvironmentContext,
) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
		defer cancel()

		parsedURL, err := url.Parse(cfg.OllamaBaseURL)
		if err != nil {
			return correctionResultMsg{err: fmt.Errorf("non-streaming: bad URL: %w", err)}
		}

		client := api.NewClient(parsedURL, http.DefaultClient)

		systemPrompt := llm.SystemPrompt(envCtx)
		userPrompt := llm.UserPrompt(command, output, exitCode, envCtx.CWD)
		schema := llm.CorrectionSchema()

		stream := false
		req := &api.GenerateRequest{
			Model:  cfg.OllamaModel,
			Prompt: userPrompt,
			System: systemPrompt,
			Format: schema,
			Stream: &stream,
			Options: map[string]interface{}{
				"temperature": 0,
			},
		}

		var responseText string
		respFunc := func(resp api.GenerateResponse) error {
			responseText = resp.Response
			return nil
		}

		if err := client.Generate(ctx, req, respFunc); err != nil {
			return correctionResultMsg{err: fmt.Errorf("non-streaming API error: %w", err)}
		}

		jsonStr := extractJSONFromStream(responseText)
		if jsonStr == "" {
			return correctionResultMsg{err: fmt.Errorf("non-streaming: no valid JSON in response")}
		}

		var correction llm.Correction
		if err := json.Unmarshal([]byte(jsonStr), &correction); err != nil {
			return correctionResultMsg{err: fmt.Errorf("non-streaming: parse error: %w", err)}
		}

		return correctionResultMsg{correction: &correction}
	}
}

// extractJSONFromStream extracts the first JSON object from a string.
func extractJSONFromStream(s string) string {
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start == -1 || end == -1 || start > end {
		return ""
	}
	return s[start : end+1]
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
		m.ctx, m.cancel = context.WithCancel(context.Background())
		m.stream = &streamState{}
		m.state = StateStreaming
		return m, startStreamingCmd(
			m.ctx, m.config, m.stream,
			m.command, m.output, m.exitCode, m.envCtx,
		)

	case StateStreaming:
		if m.cancel != nil {
			m.cancel()
		}
		m.ctx, m.cancel = context.WithCancel(context.Background())
		m.stream = nil
		m.state = StateLoading
		return m, startNonStreamingCmd(
			m.ctx, m.config,
			m.command, m.output, m.exitCode, m.envCtx,
		)

	default:
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
		frame := string(spinnerFrames[m.spinnerFrame])
		return m.styles.TitleStyle.Render(frame) + " Streaming response...\n" + hint
	}

	rendered := renderMarkdown(text)
	return rendered + "\n" + hint
}
