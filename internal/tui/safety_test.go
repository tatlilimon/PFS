package tui

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/ollama/ollama/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tatlilimon/PFS/internal/config"
	"github.com/tatlilimon/PFS/internal/llm"
)

func confirmModelWith(cmd string) Model {
	m := NewModel(&config.Config{}, nil, llm.EnvironmentContext{}, "orig", "err", 1)
	updated, _ := m.Update(correctionResultMsg{correction: &llm.Correction{
		Diagnosis:        "diagnosis",
		CorrectedCommand: &cmd,
		Confidence:       "high",
	}})
	return updated.(Model)
}

func press(t *testing.T, m Model, key rune) (Model, tea.Cmd) {
	t.Helper()
	updated, cmd := m.Update(tea.KeyPressMsg{Code: key, Text: string(key)})
	return updated.(Model), cmd
}

// Pressing e must not decide the command to run: if the editor fails or is
// cancelled (for example :cq in vim), nothing may run.
func TestEdit_EditorFailureRunsNothing(t *testing.T) {
	m := confirmModelWith("git push origin main")
	m, cmd := press(t, m, 'e')
	require.NotNil(t, cmd)
	assert.Empty(t, m.CorrectedOutput, "output is only set once the editor has finished")
	tmp := m.editTmpFile
	require.NotEmpty(t, tmp)

	updated, cmd := m.Update(editorFinishedMsg{err: errors.New("exit status 1")})
	m = updated.(Model)
	assert.Empty(t, m.CorrectedOutput)
	assert.Equal(t, StateError, m.state)
	var editorErr *EditorError
	assert.ErrorAs(t, m.err, &editorErr)
	assert.NotNil(t, cmd)
	_, statErr := os.Stat(tmp)
	assert.True(t, os.IsNotExist(statErr), "temp file is removed")
}

func TestEdit_UnreadableResultRunsNothing(t *testing.T) {
	m := confirmModelWith("git push origin main")
	m, _ = press(t, m, 'e')
	require.NoError(t, os.Remove(m.editTmpFile))

	updated, _ := m.Update(editorFinishedMsg{})
	m = updated.(Model)
	assert.Empty(t, m.CorrectedOutput)
	assert.Equal(t, StateError, m.state)
}

func TestEdit_SavedCommandIsUsed(t *testing.T) {
	m := confirmModelWith("git push origin main")
	m, _ = press(t, m, 'e')
	tmp := m.editTmpFile
	info, err := os.Stat(tmp)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0600), info.Mode().Perm())
	require.NoError(t, os.WriteFile(tmp, []byte("git push -u origin main\n"), 0600))

	updated, cmd := m.Update(editorFinishedMsg{})
	m = updated.(Model)
	assert.Equal(t, "git push -u origin main", m.CorrectedOutput)
	assert.NotNil(t, cmd)
	_, statErr := os.Stat(tmp)
	assert.True(t, os.IsNotExist(statErr))
}

func TestEdit_EmptiedFileRunsNothing(t *testing.T) {
	m := confirmModelWith("git push origin main")
	m, _ = press(t, m, 'e')
	require.NoError(t, os.WriteFile(m.editTmpFile, []byte("  \n"), 0600))
	updated, _ := m.Update(editorFinishedMsg{})
	assert.Empty(t, updated.(Model).CorrectedOutput)
}

// EDITOR often carries arguments ("code --wait", "emacsclient -t").
func TestEditorCommand_SupportsArguments(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "editor.sh")
	require.NoError(t, os.WriteFile(script, []byte("#!/bin/sh\nprintf '%s|%s' \"$1\" \"$2\" > \"$2.args\"\n"), 0755))
	target := filepath.Join(dir, "cmd file.sh")
	require.NoError(t, os.WriteFile(target, []byte("ls"), 0600))

	t.Setenv("EDITOR", script+" --wait")
	c := editorCommand(target)
	c.Stdin, c.Stdout, c.Stderr = nil, nil, nil
	require.NoError(t, c.Run())
	data, err := os.ReadFile(target + ".args")
	require.NoError(t, err)
	assert.Equal(t, "--wait|"+target, string(data))
}

func TestEditorCommand_DefaultsToVi(t *testing.T) {
	t.Setenv("EDITOR", "")
	c := editorCommand("/tmp/x")
	assert.Contains(t, strings.Join(c.Args, " "), "vi")
}

func TestDangerousCommand_RequiresSecondConfirmation(t *testing.T) {
	m := confirmModelWith("rm -rf ~")
	assert.Contains(t, stripANSI(m.View().Content), "Destructive")

	m, cmd := press(t, m, 'r')
	assert.Nil(t, cmd, "first r only arms the confirmation")
	assert.Empty(t, m.CorrectedOutput)
	assert.Contains(t, stripANSI(m.View().Content), "Press y")

	m, cmd = press(t, m, 'n')
	assert.Nil(t, cmd)
	assert.Empty(t, m.CorrectedOutput)
	assert.NotContains(t, stripANSI(m.View().Content), "Press y", "any other key disarms")

	m, _ = press(t, m, 'r')
	m, cmd = press(t, m, 'y')
	assert.Equal(t, "rm -rf ~", m.CorrectedOutput)
	assert.NotNil(t, cmd)
}

func TestNoticeCommand_RunsWithOneKeyButShowsReason(t *testing.T) {
	m := confirmModelWith("make && make install")
	assert.Contains(t, stripANSI(m.View().Content), "several commands")
	m, cmd := press(t, m, 'r')
	assert.Equal(t, "make && make install", m.CorrectedOutput)
	assert.NotNil(t, cmd)
}

func TestUnsafeCommandFromResultIsRejected(t *testing.T) {
	bad := "ls\r\x1b[2Krm -rf ~"
	m := NewModel(&config.Config{}, nil, llm.EnvironmentContext{}, "ls", "", 1)
	updated, cmd := m.Update(correctionResultMsg{correction: &llm.Correction{Diagnosis: "x", CorrectedCommand: &bad, Confidence: "high"}})
	m = updated.(Model)
	assert.Equal(t, StateError, m.state)
	assert.NotNil(t, cmd)
	assert.Empty(t, m.CorrectedOutput)
}

func TestView_StripsEscapeSequencesFromModelText(t *testing.T) {
	cmd := "ls -la"
	expl := "see \x1b]52;c;ZXZpbA==\x07this"
	m := NewModel(&config.Config{}, nil, llm.EnvironmentContext{}, "ls", "", 1)
	m.state = StateConfirm
	m.correction = &llm.Correction{Diagnosis: "bad\x1b[2Jthing", CorrectedCommand: &cmd, Confidence: "high", Explanation: &expl}
	content := m.View().Content
	assert.NotContains(t, content, "\x1b]52")
	assert.NotContains(t, content, "\x1b[2J")
	assert.Contains(t, stripANSI(content), "badthing")
}

func TestStreamingView_StripsEscapeSequences(t *testing.T) {
	m := NewModel(&config.Config{}, nil, llm.EnvironmentContext{}, "ls", "", 0)
	m.state = StateStreaming
	m.stream = &streamState{}
	m.stream.buf.WriteString("{\"diagnosis\":\"\x1b]0;pwned\x07x\"")
	assert.NotContains(t, m.View().Content, "\x1b]0;pwned")
}

func TestResultView_NoCommandHidesRunAndEditHints(t *testing.T) {
	m := NewModel(&config.Config{}, nil, llm.EnvironmentContext{}, "ls", "", 1)
	updated, _ := m.Update(correctionResultMsg{correction: &llm.Correction{
		Diagnosis:           "cannot tell",
		Confidence:          "low",
		AlternativeCommands: []string{"ls -a", "ls -l"},
	}})
	m = updated.(Model)
	content := stripANSI(m.View().Content)
	assert.NotContains(t, content, "[r]")
	assert.NotContains(t, content, "[e]")
	assert.Contains(t, content, "[x]")
	assert.Contains(t, content, "ls -a")
	assert.True(t, m.Ambiguous())
}

// Toggling streaming twice quickly must not let the cancelled first request
// end the session with a "context canceled" error.
func TestStaleResultsAfterDoubleToggleAreIgnored(t *testing.T) {
	m := NewModel(&config.Config{}, nil, llm.EnvironmentContext{}, "ls", "", 1)
	updated, _ := m.Update(tea.KeyPressMsg{Code: 'x', Mod: tea.ModCtrl})
	updated, _ = updated.(Model).Update(tea.KeyPressMsg{Code: 'x', Mod: tea.ModCtrl})
	m = updated.(Model)
	require.Equal(t, StateLoading, m.state)

	updated, _ = m.Update(correctionResultMsg{gen: 0, err: context.Canceled})
	m = updated.(Model)
	assert.Equal(t, StateLoading, m.state)
	assert.Nil(t, m.err)
}

func TestNilCorrectionWithoutErrorIsAnError(t *testing.T) {
	m := NewModel(&config.Config{}, nil, llm.EnvironmentContext{}, "ls", "", 1)
	var updated tea.Model
	require.NotPanics(t, func() { updated, _ = m.Update(correctionResultMsg{}) })
	assert.Equal(t, StateError, updated.(Model).state)
}

// Streaming goes through the provider, so it gets the same validation and
// retries as the normal request.
func TestStreamingUsesProvider(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			return
		}
		var req api.GenerateRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		if req.Stream != nil && *req.Stream {
			for _, tok := range []string{`{"diagnosis":"typo",`, `"corrected_command":"ls -l",`, `"confidence":"high"}`} {
				b, _ := json.Marshal(api.GenerateResponse{Response: tok})
				w.Write(append(b, '\n'))
			}
			b, _ := json.Marshal(api.GenerateResponse{Done: true})
			w.Write(append(b, '\n'))
			return
		}
		b, _ := json.Marshal(api.GenerateResponse{Response: `{"diagnosis":"typo","corrected_command":"ls -l","confidence":"high"}`, Done: true})
		w.Write(b)
	}))
	defer server.Close()

	cfg := &config.Config{OllamaBaseURL: server.URL, OllamaModel: "m"}
	provider, err := llm.NewOllamaProvider(cfg)
	require.NoError(t, err)

	m := NewModel(cfg, provider, llm.EnvironmentContext{}, "lls", "", 127)
	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'x', Mod: tea.ModCtrl})
	m = updated.(Model)
	require.NotNil(t, cmd)

	done := make(chan tea.Msg, 1)
	go func() { done <- cmd() }()
	var msg tea.Msg
	select {
	case msg = <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("streaming command did not finish")
	}
	updated, _ = m.Update(msg)
	m = updated.(Model)
	require.Equal(t, StateConfirm, m.state)
	assert.Equal(t, "ls -l", m.GetCommand())
}
