package tui

import (
	"errors"
	"os"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tatlilimon/PFS/internal/config"
	"github.com/tatlilimon/PFS/internal/llm"
)

func TestNewModel(t *testing.T) {
	cfg := &config.Config{OllamaModel: "test-model"}
	envCtx := llm.EnvironmentContext{OS: "linux", Shell: "bash"}

	m := NewModel(cfg, nil, envCtx, "git push", "error: failed", 1)

	assert.Equal(t, StateLoading, m.state)
	assert.Equal(t, cfg, m.config)
	assert.Equal(t, envCtx, m.envCtx)
	assert.Equal(t, "git push", m.command)
	assert.Equal(t, "error: failed", m.output)
	assert.Equal(t, 1, m.exitCode)
	assert.Nil(t, m.correction)
	assert.Nil(t, m.err)
}

func TestModelView_Loading(t *testing.T) {
	m := NewModel(&config.Config{}, nil, llm.EnvironmentContext{}, "ls", "no error", 0)
	view := m.View()
	content := view.Content
	assert.Contains(t, content, "Analyzing")
	assert.Contains(t, content, "Ctrl+X")
}

func TestModelView_Result(t *testing.T) {
	corrected := "git push origin main"
	explanation := "You need to specify the remote and branch."
	m := NewModel(&config.Config{}, nil, llm.EnvironmentContext{}, "git push", "error", 1)
	m.state = StateResult
	m.correction = &llm.Correction{
		Diagnosis:        "Missing remote and branch arguments.",
		CorrectedCommand: &corrected,
		Confidence:       "high",
		Explanation:      &explanation,
	}

	view := m.View()
	content := view.Content
	assert.Contains(t, content, "Missing remote and branch")
	assert.Contains(t, content, "git push origin main")
	assert.Contains(t, content, "HIGH")
	assert.Contains(t, content, "[r]")
	assert.Contains(t, content, "[e]")
	assert.Contains(t, content, "[x]")
}

func TestModelView_ResultWithoutCorrection(t *testing.T) {
	m := NewModel(&config.Config{}, nil, llm.EnvironmentContext{}, "ls", "", 0)
	m.state = StateResult
	m.correction = &llm.Correction{
		Diagnosis:  "Command is correct.",
		Confidence: "high",
	}

	view := m.View()
	stripped := stripANSI(view.Content)
	assert.Contains(t, stripped, "Command is correct")
}

func TestModelView_ResultLowConfidence(t *testing.T) {
	corrected := "maybe this"
	m := NewModel(&config.Config{}, nil, llm.EnvironmentContext{}, "bad", "err", 1)
	m.state = StateResult
	m.correction = &llm.Correction{
		Diagnosis:        "Uncertain.",
		CorrectedCommand: &corrected,
		Confidence:       "low",
	}

	view := m.View()
	content := view.Content
	assert.Contains(t, content, "LOW")
}

func TestModelView_Error(t *testing.T) {
	m := NewModel(&config.Config{}, nil, llm.EnvironmentContext{}, "ls", "", 1)
	m.state = StateError
	m.err = errors.New("connection refused")

	view := m.View()
	content := view.Content
	assert.Contains(t, content, "connection refused")
	assert.Contains(t, content, "pfs setup")
}

func TestModelUpdate_Quit(t *testing.T) {
	m := NewModel(&config.Config{}, nil, llm.EnvironmentContext{}, "ls", "", 0)

	_, cmd := m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	assert.NotNil(t, cmd)
}

func TestModelUpdate_CtrlX_Quit(t *testing.T) {
	m := NewModel(&config.Config{}, nil, llm.EnvironmentContext{}, "ls", "", 0)

	_, cmd := m.Update(tea.KeyPressMsg{Code: 'x', Mod: tea.ModCtrl})
	assert.NotNil(t, cmd)
}

func TestModelUpdate_SpinnerTick(t *testing.T) {
	m := NewModel(&config.Config{}, nil, llm.EnvironmentContext{}, "ls", "", 0)
	assert.Equal(t, 0, m.spinnerFrame)

	updated, cmd := m.Update(spinnerTickMsg{})
	require.NotNil(t, cmd)

	updatedM, ok := updated.(Model)
	require.True(t, ok)
	assert.Equal(t, 1, updatedM.spinnerFrame)
}

func TestModelUpdate_SpinnerStopsOnResult(t *testing.T) {
	m := NewModel(&config.Config{}, nil, llm.EnvironmentContext{}, "ls", "", 0)
	m.state = StateResult

	_, cmd := m.Update(spinnerTickMsg{})
	assert.Nil(t, cmd)
}

func TestModelUpdate_CorrectionResult(t *testing.T) {
	m := NewModel(&config.Config{}, nil, llm.EnvironmentContext{}, "ls", "", 0)
	corrected := "ls -la"

	updated, _ := m.Update(correctionResultMsg{
		correction: &llm.Correction{
			Diagnosis:        "Try with -la flag.",
			CorrectedCommand: &corrected,
			Confidence:       "high",
		},
	})

	updatedM, ok := updated.(Model)
	require.True(t, ok)
	assert.Equal(t, StateConfirm, updatedM.state)
	assert.NotNil(t, updatedM.correction)
	assert.Equal(t, "Try with -la flag.", updatedM.correction.Diagnosis)
}

func TestModelUpdate_CorrectionError(t *testing.T) {
	m := NewModel(&config.Config{}, nil, llm.EnvironmentContext{}, "ls", "", 0)

	updated, _ := m.Update(correctionResultMsg{
		err: errors.New("timeout"),
	})

	updatedM, ok := updated.(Model)
	require.True(t, ok)
	assert.Equal(t, StateError, updatedM.state)
	assert.EqualError(t, updatedM.err, "timeout")
}

func TestModelUpdate_WindowSize(t *testing.T) {
	m := NewModel(&config.Config{}, nil, llm.EnvironmentContext{}, "ls", "", 0)

	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})

	updatedM, ok := updated.(Model)
	require.True(t, ok)
	assert.Equal(t, 120, updatedM.width)
	assert.Equal(t, 40, updatedM.height)
}

func TestModelUpdate_XKeyOnResult(t *testing.T) {
	m := NewModel(&config.Config{}, nil, llm.EnvironmentContext{}, "ls", "", 0)
	m.state = StateResult
	corrected := "ls -la"
	m.correction = &llm.Correction{
		Diagnosis:        "fix",
		CorrectedCommand: &corrected,
		Confidence:       "high",
	}

	_, cmd := m.Update(tea.KeyPressMsg{Code: 'x'})
	assert.NotNil(t, cmd)
}

func TestSetCorrection(t *testing.T) {
	m := NewModel(&config.Config{}, nil, llm.EnvironmentContext{}, "ls", "", 0)
	corrected := "ls -la"
	c := &llm.Correction{
		Diagnosis:        "Use -la.",
		CorrectedCommand: &corrected,
		Confidence:       "medium",
	}

	m.SetCorrection(c)
	assert.Equal(t, StateResult, m.state)
	assert.Equal(t, c, m.correction)
}

func TestSetError(t *testing.T) {
	m := NewModel(&config.Config{}, nil, llm.EnvironmentContext{}, "ls", "", 0)
	err := errors.New("test error")
	m.SetError(err)
	assert.Equal(t, StateError, m.state)
	assert.Equal(t, err, m.err)
}

func TestGetCommand(t *testing.T) {
	m := NewModel(&config.Config{}, nil, llm.EnvironmentContext{}, "ls", "", 0)
	assert.Equal(t, "", m.GetCommand())

	corrected := "ls -la"
	m.correction = &llm.Correction{
		CorrectedCommand: &corrected,
	}
	assert.Equal(t, "ls -la", m.GetCommand())
}

func TestGetState(t *testing.T) {
	m := NewModel(&config.Config{}, nil, llm.EnvironmentContext{}, "ls", "", 0)
	assert.Equal(t, StateLoading, m.GetState())

	m.state = StateResult
	assert.Equal(t, StateResult, m.GetState())
}

func TestGetError(t *testing.T) {
	m := NewModel(&config.Config{}, nil, llm.EnvironmentContext{}, "ls", "", 0)
	assert.Nil(t, m.GetError())

	err := errors.New("fail")
	m.err = err
	assert.Equal(t, err, m.GetError())
}

func TestCorrectionResultCmd(t *testing.T) {
	corrected := "fixed"
	c := &llm.Correction{
		Diagnosis:        "Fixed.",
		CorrectedCommand: &corrected,
		Confidence:       "high",
	}
	cmd := CorrectionResultCmd(c, nil)
	msg := cmd()
	result, ok := msg.(correctionResultMsg)
	require.True(t, ok)
	assert.Equal(t, c, result.correction)
	assert.Nil(t, result.err)
}

func TestCorrectionResultCmd_WithError(t *testing.T) {
	err := errors.New("boom")
	cmd := CorrectionResultCmd(nil, err)
	msg := cmd()
	result, ok := msg.(correctionResultMsg)
	require.True(t, ok)
	assert.Nil(t, result.correction)
	assert.EqualError(t, result.err, "boom")
}

func TestIsTerminal(t *testing.T) {
	fi, err := os.Stderr.Stat()
	if err != nil {
		assert.False(t, IsTerminal())
		return
	}
	expected := (fi.Mode() & os.ModeCharDevice) != 0
	assert.Equal(t, expected, IsTerminal())
}

func TestViewEmptyOnUnknownState(t *testing.T) {
	m := NewModel(&config.Config{}, nil, llm.EnvironmentContext{}, "ls", "", 0)
	m.state = State(99)
	view := m.View()
	assert.Equal(t, "", strings.TrimSpace(view.Content))
}

func TestRenderMarkdown(t *testing.T) {
	result := renderMarkdown("**bold text**")
	assert.Contains(t, result, "bold text")
}

func stripANSI(s string) string {
	var result strings.Builder
	inEscape := false
	for _, r := range s {
		if r == '\x1b' {
			inEscape = true
			continue
		}
		if inEscape {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				inEscape = false
			}
			continue
		}
		result.WriteRune(r)
	}
	return result.String()
}
