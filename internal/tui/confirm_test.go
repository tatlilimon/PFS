package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tatlilimon/PFS/internal/config"
	"github.com/tatlilimon/PFS/internal/llm"
)

func newConfirmModel(command string) Model {
	m := NewModel(&config.Config{}, nil, llm.EnvironmentContext{}, command, "error", 1)
	corrected := command + " --fixed"
	m.state = StateConfirm
	m.correction = &llm.Correction{
		Diagnosis:        "Something went wrong.",
		CorrectedCommand: &corrected,
		Confidence:       "high",
	}
	return m
}

func TestConfirmRun(t *testing.T) {
	m := newConfirmModel("git push")
	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'r'})

	updatedM, ok := updated.(Model)
	require.True(t, ok)
	assert.Equal(t, "git push --fixed", updatedM.CorrectedOutput)
	assert.NotNil(t, cmd)
}

func TestConfirmRun_UpperCase(t *testing.T) {
	m := newConfirmModel("git push")
	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'R'})

	updatedM, ok := updated.(Model)
	require.True(t, ok)
	assert.Equal(t, "git push --fixed", updatedM.CorrectedOutput)
	assert.NotNil(t, cmd)
}

func TestConfirmEdit(t *testing.T) {
	m := newConfirmModel("git push")
	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'e'})

	updatedM, ok := updated.(Model)
	require.True(t, ok)
	assert.Equal(t, "git push --fixed", updatedM.CorrectedOutput)
	assert.NotEmpty(t, updatedM.editTmpFile)
	assert.NotNil(t, cmd)

	_, statErr := os.Stat(updatedM.editTmpFile)
	assert.NoError(t, statErr, "temp file should exist")
	os.Remove(updatedM.editTmpFile)
}

func TestConfirmEdit_UpperCase(t *testing.T) {
	m := newConfirmModel("git push")
	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'E'})

	updatedM, ok := updated.(Model)
	require.True(t, ok)
	assert.NotEmpty(t, updatedM.editTmpFile)
	assert.NotNil(t, cmd)
	os.Remove(updatedM.editTmpFile)
}

func TestConfirmDismiss(t *testing.T) {
	m := newConfirmModel("git push")
	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'x'})

	updatedM, ok := updated.(Model)
	require.True(t, ok)
	assert.Equal(t, "", updatedM.CorrectedOutput)
	assert.NotNil(t, cmd)
}

func TestConfirmDismiss_QKey(t *testing.T) {
	m := newConfirmModel("git push")
	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'q'})

	updatedM, ok := updated.(Model)
	require.True(t, ok)
	assert.Equal(t, "", updatedM.CorrectedOutput)
	assert.NotNil(t, cmd)
}

func TestConfirmDismiss_UpperCaseX(t *testing.T) {
	m := newConfirmModel("git push")
	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'X'})

	updatedM, ok := updated.(Model)
	require.True(t, ok)
	assert.Equal(t, "", updatedM.CorrectedOutput)
	assert.NotNil(t, cmd)
}

func TestConfirmDismiss_Escape(t *testing.T) {
	m := newConfirmModel("git push")
	updated, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})

	updatedM, ok := updated.(Model)
	require.True(t, ok)
	assert.Equal(t, "", updatedM.CorrectedOutput)
	assert.NotNil(t, cmd)
}

func TestConfirmLowConfidence(t *testing.T) {
	corrected := "maybe-fix"
	m := NewModel(&config.Config{}, nil, llm.EnvironmentContext{}, "bad", "err", 1)
	m.state = StateConfirm
	m.correction = &llm.Correction{
		Diagnosis:        "Uncertain fix.",
		CorrectedCommand: &corrected,
		Confidence:       "low",
	}

	view := m.View()
	stripped := stripANSI(view.Content)
	assert.Contains(t, stripped, "Low confidence")
	assert.Contains(t, stripped, "review carefully")
}

func TestConfirmMediumConfidence_NoWarning(t *testing.T) {
	corrected := "likely-fix"
	m := NewModel(&config.Config{}, nil, llm.EnvironmentContext{}, "bad", "err", 1)
	m.state = StateConfirm
	m.correction = &llm.Correction{
		Diagnosis:        "Probable fix.",
		CorrectedCommand: &corrected,
		Confidence:       "medium",
	}

	view := m.View()
	stripped := stripANSI(view.Content)
	assert.NotContains(t, stripped, "Low confidence")
}

func TestConfirmEmptyCorrection_SkipsConfirm(t *testing.T) {
	m := NewModel(&config.Config{}, nil, llm.EnvironmentContext{}, "ls", "output", 1)

	updated, _ := m.Update(correctionResultMsg{
		correction: &llm.Correction{
			Diagnosis:  "Command is fine.",
			Confidence: "high",
		},
	})

	updatedM, ok := updated.(Model)
	require.True(t, ok)
	assert.Equal(t, StateResult, updatedM.state)
}

func TestConfirmWithCorrection_EntersConfirm(t *testing.T) {
	m := NewModel(&config.Config{}, nil, llm.EnvironmentContext{}, "ls", "output", 1)
	corrected := "ls -la"

	updated, _ := m.Update(correctionResultMsg{
		correction: &llm.Correction{
			Diagnosis:        "Use -la.",
			CorrectedCommand: &corrected,
			Confidence:       "high",
		},
	})

	updatedM, ok := updated.(Model)
	require.True(t, ok)
	assert.Equal(t, StateConfirm, updatedM.state)
}

func TestConfirmView_ShowsKeyHints(t *testing.T) {
	m := newConfirmModel("git push")
	view := m.View()
	content := view.Content
	assert.Contains(t, content, "[r]")
	assert.Contains(t, content, "[e]")
	assert.Contains(t, content, "[x]")
}

func TestConfirmView_ShowsCorrectedCommand(t *testing.T) {
	m := newConfirmModel("git push")
	view := m.View()
	content := view.Content
	assert.Contains(t, content, "git push --fixed")
}

func TestConfirmView_ShowsConfidenceIcon(t *testing.T) {
	m := newConfirmModel("git push")
	m.correction.Confidence = "high"
	view := m.View()
	stripped := stripANSI(view.Content)
	assert.Contains(t, stripped, "✓")

	m.correction.Confidence = "medium"
	view = m.View()
	stripped = stripANSI(view.Content)
	assert.Contains(t, stripped, "◆")

	m.correction.Confidence = "low"
	view = m.View()
	stripped = stripANSI(view.Content)
	assert.Contains(t, stripped, "⚠")
}

func TestConfirmRun_NoCorrection_NoOp(t *testing.T) {
	m := NewModel(&config.Config{}, nil, llm.EnvironmentContext{}, "ls", "err", 1)
	m.state = StateConfirm
	m.correction = &llm.Correction{
		Diagnosis:  "No fix available.",
		Confidence: "low",
	}

	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'r'})
	updatedM, ok := updated.(Model)
	require.True(t, ok)
	assert.Equal(t, "", updatedM.CorrectedOutput)
	assert.Nil(t, cmd)
}

func TestCtrlC_ClearsOutput(t *testing.T) {
	m := newConfirmModel("git push")
	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})

	updatedM, ok := updated.(Model)
	require.True(t, ok)
	assert.Equal(t, "", updatedM.CorrectedOutput)
	assert.NotNil(t, cmd)
}

func TestOpenInEditor_WithMockEditor(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "mock-editor.sh")
	editedContent := "git push origin main --force"

	err := os.WriteFile(script, []byte("#!/bin/sh\ncat > \"$1\" <<'EOF'\n"+editedContent+"\nEOF\n"), 0755)
	require.NoError(t, err)

	t.Setenv("EDITOR", script)
	result, err := OpenInEditor("git push")
	require.NoError(t, err)
	assert.Equal(t, editedContent, result)
}

func TestOpenInEditor_FallbackToVi(t *testing.T) {
	t.Setenv("EDITOR", "")
	assert.Equal(t, "", os.Getenv("EDITOR"))
}

func TestOpenInEditor_EditorFails(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "fail-editor.sh")
	err := os.WriteFile(script, []byte("#!/bin/sh\nexit 1\n"), 0755)
	require.NoError(t, err)

	t.Setenv("EDITOR", script)
	_, err = OpenInEditor("git push")
	assert.Error(t, err)
}

func TestGetCorrectedOutput(t *testing.T) {
	m := Model{CorrectedOutput: "ls -la"}
	assert.Equal(t, "ls -la", m.GetCorrectedOutput())

	m = Model{}
	assert.Equal(t, "", m.GetCorrectedOutput())
}

func TestConfirmResultConstants(t *testing.T) {
	assert.Equal(t, ConfirmResult(0), ConfirmRun)
	assert.Equal(t, ConfirmResult(1), ConfirmEdit)
	assert.Equal(t, ConfirmResult(2), ConfirmDismiss)
}

func TestConfirmView_IncludesDiagnosis(t *testing.T) {
	m := newConfirmModel("git push")
	m.correction.Diagnosis = "Missing remote and branch."
	view := m.View()
	stripped := stripANSI(view.Content)
	assert.Contains(t, stripped, "Missing remote and branch")
}

func TestConfirmView_IncludesExplanation(t *testing.T) {
	m := newConfirmModel("git push")
	explanation := "You forgot to specify the remote."
	m.correction.Explanation = &explanation
	view := m.View()
	stripped := stripANSI(view.Content)
	assert.Contains(t, stripped, "You forgot to specify the remote")
}

func TestConfirmView_NoExplanation(t *testing.T) {
	m := newConfirmModel("git push")
	m.correction.Explanation = nil
	view := m.View()
	stripped := stripANSI(view.Content)
	assert.Contains(t, stripped, "[r]")
}

func TestConfirm_IgnoresIrrelevantKeys(t *testing.T) {
	m := newConfirmModel("git push")
	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'a'})

	updatedM, ok := updated.(Model)
	require.True(t, ok)
	assert.Equal(t, "", updatedM.CorrectedOutput)
	assert.Nil(t, cmd)
}

func TestStreamingCorrection_EntersConfirm(t *testing.T) {
	m := NewModel(&config.Config{}, nil, llm.EnvironmentContext{}, "ls", "err", 1)
	m.state = StateStreaming
	corrected := "ls -la"

	updated, _ := m.Update(streamingDoneMsg{
		correction: &llm.Correction{
			Diagnosis:        "Use -la.",
			CorrectedCommand: &corrected,
			Confidence:       "high",
		},
	})

	updatedM, ok := updated.(Model)
	require.True(t, ok)
	assert.Equal(t, StateConfirm, updatedM.state)
}

func TestStreamingCorrection_NoCommand_EntersResult(t *testing.T) {
	m := NewModel(&config.Config{}, nil, llm.EnvironmentContext{}, "ls", "err", 1)
	m.state = StateStreaming

	updated, _ := m.Update(streamingDoneMsg{
		correction: &llm.Correction{
			Diagnosis:  "Looks fine.",
			Confidence: "medium",
		},
	})

	updatedM, ok := updated.(Model)
	require.True(t, ok)
	assert.Equal(t, StateResult, updatedM.state)
}

func stripANSIConfirm(s string) string {
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
