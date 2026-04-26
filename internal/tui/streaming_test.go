package tui

import (
	"errors"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tatlilimon/PFS/internal/config"
	"github.com/tatlilimon/PFS/internal/llm"
)

func TestStreamingToggle_SpinnerToStream(t *testing.T) {
	cfg := &config.Config{
		OllamaBaseURL: "http://localhost:11434",
		OllamaModel:   "test",
	}
	m := NewModel(cfg, llm.EnvironmentContext{}, "ls", "err", 1)
	assert.Equal(t, StateLoading, m.GetState())

	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'x', Mod: tea.ModCtrl})
	m2, ok := updated.(Model)
	require.True(t, ok)

	assert.Equal(t, StateStreaming, m2.GetState())
	assert.NotNil(t, m2.stream)
	assert.NotNil(t, cmd)
}

func TestStreamingToggle_StreamToSpinner(t *testing.T) {
	cfg := &config.Config{
		OllamaBaseURL: "http://localhost:11434",
		OllamaModel:   "test",
	}
	m := NewModel(cfg, llm.EnvironmentContext{}, "ls", "err", 1)

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'x', Mod: tea.ModCtrl})
	m2 := updated.(Model)
	assert.Equal(t, StateStreaming, m2.GetState())

	updated2, cmd := m2.Update(tea.KeyPressMsg{Code: 'x', Mod: tea.ModCtrl})
	m3, ok := updated2.(Model)
	require.True(t, ok)

	assert.Equal(t, StateLoading, m3.GetState())
	assert.Nil(t, m3.stream)
	assert.NotNil(t, cmd)
}

func TestStreamingView(t *testing.T) {
	m := NewModel(&config.Config{}, llm.EnvironmentContext{}, "ls", "", 0)
	m.state = StateStreaming
	m.stream = &streamState{}
	m.stream.buf.WriteString("partial response text")

	view := m.View()
	stripped := stripANSI(view.Content)
	assert.Contains(t, stripped, "partial")
	assert.Contains(t, stripped, "response")
	assert.Contains(t, stripped, "Ctrl+X")
}

func TestStreamingView_Empty(t *testing.T) {
	m := NewModel(&config.Config{}, llm.EnvironmentContext{}, "ls", "", 0)
	m.state = StateStreaming
	m.stream = &streamState{}

	view := m.View()
	content := view.Content
	assert.Contains(t, content, "Streaming")
	assert.Contains(t, content, "Ctrl+X")
}

func TestStreamingCancel(t *testing.T) {
	m := NewModel(&config.Config{}, llm.EnvironmentContext{}, "ls", "", 0)
	m.state = StateStreaming
	m.stream = &streamState{}

	_, cmd := m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	assert.NotNil(t, cmd)
}

func TestStreaming_StaleCorrectionResultIgnored(t *testing.T) {
	m := NewModel(&config.Config{}, llm.EnvironmentContext{}, "ls", "", 0)
	m.state = StateStreaming

	corrected := "ls -la"
	updated, _ := m.Update(correctionResultMsg{
		correction: &llm.Correction{
			Diagnosis:        "Use -la.",
			CorrectedCommand: &corrected,
			Confidence:       "high",
		},
	})

	m2, ok := updated.(Model)
	require.True(t, ok)
	assert.Equal(t, StateStreaming, m2.GetState())
	assert.Nil(t, m2.correction)
}

func TestStreaming_DoneMsgTransitionsToResult(t *testing.T) {
	m := NewModel(&config.Config{}, llm.EnvironmentContext{}, "ls", "", 0)
	m.state = StateStreaming

	corrected := "ls -la"
	updated, _ := m.Update(streamingDoneMsg{
		correction: &llm.Correction{
			Diagnosis:        "Use -la.",
			CorrectedCommand: &corrected,
			Confidence:       "high",
		},
	})

	m2, ok := updated.(Model)
	require.True(t, ok)
	assert.Equal(t, StateConfirm, m2.GetState())
	assert.NotNil(t, m2.correction)
	assert.Equal(t, "Use -la.", m2.correction.Diagnosis)
}

func TestStreaming_DoneMsgError(t *testing.T) {
	m := NewModel(&config.Config{}, llm.EnvironmentContext{}, "ls", "", 0)
	m.state = StateStreaming

	updated, cmd := m.Update(streamingDoneMsg{
		err: errors.New("connection refused"),
	})

	m2, ok := updated.(Model)
	require.True(t, ok)
	assert.Equal(t, StateError, m2.GetState())
	assert.EqualError(t, m2.err, "connection refused")
	assert.NotNil(t, cmd)
}

func TestStreaming_StaleDoneMsgIgnored(t *testing.T) {
	m := NewModel(&config.Config{}, llm.EnvironmentContext{}, "ls", "", 0)
	m.state = StateResult

	corrected := "ls -la"
	updated, _ := m.Update(streamingDoneMsg{
		correction: &llm.Correction{
			Diagnosis:        "Stale.",
			CorrectedCommand: &corrected,
			Confidence:       "high",
		},
	})

	m2, ok := updated.(Model)
	require.True(t, ok)
	assert.Equal(t, StateResult, m2.GetState())
	assert.Nil(t, m2.correction)
}

func TestStreaming_TickContinuesDuringStreaming(t *testing.T) {
	m := NewModel(&config.Config{}, llm.EnvironmentContext{}, "ls", "", 0)
	m.state = StateStreaming
	m.stream = &streamState{}

	_, cmd := m.Update(spinnerTickMsg{})
	require.NotNil(t, cmd)
}

func TestStreaming_CtrlXInResultStateQuits(t *testing.T) {
	m := NewModel(&config.Config{}, llm.EnvironmentContext{}, "ls", "", 0)
	m.state = StateResult
	corrected := "ls -la"
	m.correction = &llm.Correction{
		Diagnosis:        "fix",
		CorrectedCommand: &corrected,
		Confidence:       "high",
	}

	_, cmd := m.Update(tea.KeyPressMsg{Code: 'x', Mod: tea.ModCtrl})
	assert.NotNil(t, cmd)
}

func TestExtractJSONFromStream(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"valid JSON", `{"key":"value"}`, `{"key":"value"}`},
		{"text before", `some text {"key":"value"}`, `{"key":"value"}`},
		{"text after", `{"key":"value"} more text`, `{"key":"value"}`},
		{"nested", `{"outer":{"inner":1}}`, `{"outer":{"inner":1}}`},
		{"no JSON", "plain text", ""},
		{"empty", "", ""},
		{"reversed braces", "}something{", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractJSONFromStream(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}
