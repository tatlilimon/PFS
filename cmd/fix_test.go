package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"

	"github.com/ollama/ollama/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tatlilimon/PFS/internal/config"
	"github.com/tatlilimon/PFS/internal/llm"
)

func addCallCount(counter *int32) int32 {
	return atomic.AddInt32(counter, 1)
}

func ollamaMockResponse(t *testing.T, responseJSON string) string {
	t.Helper()
	resp := api.GenerateResponse{
		Response: responseJSON,
		Done:     true,
	}
	data, err := json.Marshal(resp)
	require.NoError(t, err)
	return string(data)
}

func validCorrectionJSON() string {
	corrected := "ls -la"
	explanation := "Add -la flag for detailed listing"
	corr := llm.Correction{
		Diagnosis:        "Command needs more flags.",
		CorrectedCommand: &corrected,
		Confidence:       "high",
		Explanation:      &explanation,
	}
	data, _ := json.Marshal(corr)
	return string(data)
}

func noFixCorrectionJSON() string {
	corr := llm.Correction{
		Diagnosis:  "Command is correct, no fix needed.",
		Confidence: "high",
	}
	data, _ := json.Marshal(corr)
	return string(data)
}

func newMockOllamaServer(t *testing.T, responseJSON string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodHead {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.Write([]byte(ollamaMockResponse(t, responseJSON)))
	}))
}

func setupFixEnv(t *testing.T, serverURL string) {
	t.Helper()
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)
	t.Setenv("PFS_CMD", "ls -l")
	t.Setenv("PFS_EXIT", "0")
	t.Setenv("PFS_OUTPUT", "")
	t.Setenv("PFS_CWD", "/home/user")
	t.Setenv("PFS_SHELL", "bash")

	cfg, err := config.Load()
	require.NoError(t, err)
	cfg.OllamaBaseURL = serverURL
	cfg.OllamaModel = "test-model"
	require.NoError(t, cfg.Save())
}

func TestFix_NoWrapper_ReturnsError(t *testing.T) {
	os.Unsetenv("PFS_CMD")

	resetCmd(rootCmd)
	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"fix"})

	err := rootCmd.Execute()
	assert.Error(t, err)

	code, ok := err.(exitCode)
	assert.True(t, ok, "error should be exitCode type")
	assert.Equal(t, ExitError, int(code))
	assert.Contains(t, buf.String(), "shell integration")
}

func TestFix_NonInteractive_JSON(t *testing.T) {
	server := newMockOllamaServer(t, validCorrectionJSON())
	defer server.Close()

	setupFixEnv(t, server.URL)

	resetCmd(rootCmd)
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	rootCmd.SetOut(stdout)
	rootCmd.SetErr(stderr)
	rootCmd.SetArgs([]string{"--format", "json", "fix"})

	err := rootCmd.Execute()
	assert.NoError(t, err)

	output := stdout.String()
	assert.Contains(t, output, "diagnosis")
	assert.Contains(t, output, "Command needs more flags")

	var correction llm.Correction
	require.NoError(t, json.Unmarshal([]byte(output[:len(output)-1]), &correction))
	assert.Equal(t, "ls -la", *correction.CorrectedCommand)
}

func TestFix_NonInteractive_PlainText(t *testing.T) {
	server := newMockOllamaServer(t, validCorrectionJSON())
	defer server.Close()

	setupFixEnv(t, server.URL)

	resetCmd(rootCmd)
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	rootCmd.SetOut(stdout)
	rootCmd.SetErr(stderr)
	rootCmd.SetArgs([]string{"fix"})

	err := rootCmd.Execute()
	assert.NoError(t, err)

	assert.Equal(t, "ls -la", stdout.String())
}

func TestFix_NonInteractive_NoFix(t *testing.T) {
	var callCount int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodHead {
			w.WriteHeader(http.StatusOK)
			return
		}
		count := addCallCount(&callCount)
		if count <= 2 {
			w.Write([]byte(ollamaMockResponse(t, "not json")))
		} else {
			w.Write([]byte(ollamaMockResponse(t, noFixCorrectionJSON())))
		}
	}))
	defer server.Close()

	setupFixEnv(t, server.URL)

	resetCmd(rootCmd)
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	rootCmd.SetOut(stdout)
	rootCmd.SetErr(stderr)
	rootCmd.SetArgs([]string{"fix"})

	err := rootCmd.Execute()
	assert.Error(t, err)
	code, ok := err.(exitCode)
	require.True(t, ok)
	assert.Equal(t, ExitNoFix, int(code))
	assert.Contains(t, stderr.String(), "Command is correct")
}

func TestFix_LLMError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	setupFixEnv(t, server.URL)

	resetCmd(rootCmd)
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	rootCmd.SetOut(stdout)
	rootCmd.SetErr(stderr)
	rootCmd.SetArgs([]string{"fix"})

	err := rootCmd.Execute()
	assert.Error(t, err)
	code, ok := err.(exitCode)
	require.True(t, ok)
	assert.Equal(t, ExitLLMError, int(code))
}

func TestExplain_ShowsDiagnosis(t *testing.T) {
	server := newMockOllamaServer(t, validCorrectionJSON())
	defer server.Close()

	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)
	t.Setenv("PFS_CMD", "ls -l")
	t.Setenv("PFS_EXIT", "0")
	t.Setenv("PFS_OUTPUT", "")
	t.Setenv("PFS_CWD", "/home/user")
	t.Setenv("PFS_SHELL", "bash")

	cfg, err := config.Load()
	require.NoError(t, err)
	cfg.OllamaBaseURL = server.URL
	cfg.OllamaModel = "test-model"
	require.NoError(t, cfg.Save())

	resetCmd(rootCmd)
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	rootCmd.SetOut(stdout)
	rootCmd.SetErr(stderr)
	rootCmd.SetArgs([]string{"explain"})

	err = rootCmd.Execute()
	assert.NoError(t, err)

	errOutput := stderr.String()
	assert.Contains(t, errOutput, "Command needs more flags")
	assert.Contains(t, errOutput, "Add -la flag")
	assert.Empty(t, stdout.String(), "explain should not write to stdout")
}

func TestExplain_NoWrapper_ReturnsError(t *testing.T) {
	os.Unsetenv("PFS_CMD")

	resetCmd(rootCmd)
	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"explain"})

	err := rootCmd.Execute()
	assert.Error(t, err)
	code, ok := err.(exitCode)
	require.True(t, ok)
	assert.Equal(t, ExitError, int(code))
}
