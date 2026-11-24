package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tatlilimon/PFS/internal/config"
)

func runRoot(t *testing.T, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	resetCmd(rootCmd)
	for _, c := range rootCmd.Commands() {
		resetCmd(c)
	}
	out, errBuf := new(bytes.Buffer), new(bytes.Buffer)
	rootCmd.SetOut(out)
	rootCmd.SetErr(errBuf)
	rootCmd.SetArgs(args)
	err = rootCmd.Execute()
	return out.String(), errBuf.String(), err
}

func runExecute(t *testing.T, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	resetCmd(rootCmd)
	for _, c := range rootCmd.Commands() {
		resetCmd(c)
	}
	out, errBuf := new(bytes.Buffer), new(bytes.Buffer)
	rootCmd.SetOut(out)
	rootCmd.SetErr(errBuf)
	rootCmd.SetArgs(args)
	code = Execute()
	return out.String(), errBuf.String(), code
}

func ambiguousJSON() string {
	return `{"diagnosis":"Could be either.","corrected_command":null,"confidence":"low","alternative_commands":["ls -a","ls -l"]}`
}

// The shell wrapper sets PFS_EVAL=1 and evaluates stdout. Without a terminal
// to confirm on, pfs must show the suggestion and print nothing on stdout.
func TestFix_EvalMode_WithoutTerminal_NeverEmitsCommand(t *testing.T) {
	server := newMockOllamaServer(t, validCorrectionJSON())
	defer server.Close()
	setupFixEnv(t, server.URL)
	t.Setenv("PFS_EVAL", "1")

	for _, args := range [][]string{{"fix"}, {"--format", "json", "fix"}} {
		stdout, stderr, err := runRoot(t, args...)
		assert.Empty(t, stdout, args)
		assert.Contains(t, stderr, "ls -la", args)
		assert.Contains(t, stderr, "not run", args)
		code, ok := err.(exitCode)
		require.True(t, ok, args)
		assert.Equal(t, ExitNoFix, int(code), args)
	}
}

func TestFix_NoFixAnswerAtFirstTry(t *testing.T) {
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			return
		}
		atomic.AddInt32(&calls, 1)
		w.Write([]byte(ollamaMockResponse(t, noFixCorrectionJSON())))
	}))
	defer server.Close()
	setupFixEnv(t, server.URL)

	var stdout, stderr string
	var err error
	require.NotPanics(t, func() { stdout, stderr, err = runRoot(t, "fix") })
	assert.Empty(t, stdout)
	assert.Contains(t, stderr, "Command is correct")
	code, ok := err.(exitCode)
	require.True(t, ok)
	assert.Equal(t, ExitNoFix, int(code))
	assert.Equal(t, int32(1), atomic.LoadInt32(&calls))
}

func TestFix_Ambiguous_ExitCode4(t *testing.T) {
	server := newMockOllamaServer(t, ambiguousJSON())
	defer server.Close()
	setupFixEnv(t, server.URL)

	stdout, stderr, err := runRoot(t, "fix")
	assert.Empty(t, stdout)
	assert.Contains(t, stderr, "ls -a")
	code, ok := err.(exitCode)
	require.True(t, ok)
	assert.Equal(t, ExitAmbiguous, int(code))

	stdout, _, err = runRoot(t, "--format", "json", "fix")
	assert.Contains(t, stdout, "alternative_commands")
	code, ok = err.(exitCode)
	require.True(t, ok)
	assert.Equal(t, ExitAmbiguous, int(code))
}

func TestFix_JSON_NoFix_ExitCode3(t *testing.T) {
	server := newMockOllamaServer(t, noFixCorrectionJSON())
	defer server.Close()
	setupFixEnv(t, server.URL)

	stdout, _, err := runRoot(t, "--format", "json", "fix")
	var parsed map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(stdout), &parsed))
	code, ok := err.(exitCode)
	require.True(t, ok)
	assert.Equal(t, ExitNoFix, int(code))
}

// With debug output enabled, retries print diagnostics. They must not end up on
// stdout next to the command.
func TestFix_DebugOutputStaysOffStdout(t *testing.T) {
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			return
		}
		if atomic.AddInt32(&calls, 1) == 1 {
			w.Write([]byte(ollamaMockResponse(t, "not json")))
			return
		}
		w.Write([]byte(ollamaMockResponse(t, validCorrectionJSON())))
	}))
	defer server.Close()
	setupFixEnv(t, server.URL)
	cfg, err := config.Load()
	require.NoError(t, err)
	cfg.DebugLevel = 2
	require.NoError(t, cfg.Save())

	stdout, _, err := runRoot(t, "fix")
	require.NoError(t, err)
	assert.Equal(t, "ls -la", stdout)
}

func TestFix_TerminalControlInDiagnosisIsStripped(t *testing.T) {
	server := newMockOllamaServer(t, "{\"diagnosis\":\"gone\\u001b]52;c;ZXZpbA==\\u0007 now\",\"corrected_command\":null,\"confidence\":\"high\"}")
	defer server.Close()
	setupFixEnv(t, server.URL)

	_, stderr, _ := runRoot(t, "fix")
	assert.NotContains(t, stderr, "\x1b")
	assert.Contains(t, stderr, "gone now")
}

func TestExecute_UsageErrorsExit5WithMessage(t *testing.T) {
	for _, args := range [][]string{
		{"--no-such-flag"},
		{"no-such-command"},
		{"--format", "xml", "fix"},
		{"config", "set", "ollama_model"},
	} {
		_, stderr, code := runExecute(t, args...)
		assert.Equal(t, ExitUsage, code, args)
		assert.NotEmpty(t, strings.TrimSpace(stderr), args)
	}
}

func TestCompletionCommandIsDisabled(t *testing.T) {
	stdout, _, err := runRoot(t, "--help")
	require.NoError(t, err)
	assert.NotContains(t, stdout, "completion")
}

func TestConfig_OfflineModeAlias(t *testing.T) {
	setupCmdTestDir(t)
	_, _, err := runRoot(t, "config", "set", "offline_mode", "false")
	require.NoError(t, err)
	stdout, _, err := runRoot(t, "config", "get", "offline_mode")
	require.NoError(t, err)
	assert.Equal(t, "false\n", stdout)
	stdout, _, err = runRoot(t, "config", "get", "offline")
	require.NoError(t, err)
	assert.Equal(t, "false\n", stdout)
}

func TestConfig_SetValidatesValues(t *testing.T) {
	setupCmdTestDir(t)
	for _, args := range [][]string{
		{"config", "set", "ollama_base_url", "localhost:11434"},
		{"config", "set", "ollama_base_url", "ftp://host"},
		{"config", "set", "ollama_model", "  "},
		{"config", "set", "debug_level", "7"},
	} {
		_, _, err := runRoot(t, args...)
		code, ok := err.(exitCode)
		require.True(t, ok, args)
		assert.Equal(t, ExitUsage, int(code), args)
	}
	_, _, err := runRoot(t, "config", "set", "ollama_base_url", "http://192.168.1.100:11434")
	require.NoError(t, err)
}

func TestGetShellVersion_OnlyKnownShells(t *testing.T) {
	assert.Empty(t, getShellVersion("rm"))
	assert.Empty(t, getShellVersion("/tmp/evil"))
	v := getShellVersion("bash")
	assert.NotContains(t, v, "\n")
}

func TestVerboseFlagShowsRetryDiagnosticsOnStderr(t *testing.T) {
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			return
		}
		if atomic.AddInt32(&calls, 1) == 1 {
			w.Write([]byte(ollamaMockResponse(t, "not json")))
			return
		}
		w.Write([]byte(ollamaMockResponse(t, validCorrectionJSON())))
	}))
	defer server.Close()
	setupFixEnv(t, server.URL)

	stdout, stderr, err := runRoot(t, "-v", "fix")
	require.NoError(t, err)
	assert.Equal(t, "ls -la", stdout)
	assert.Contains(t, stderr, "Tier 1")

	_, stderr, err = runRoot(t, "fix")
	require.NoError(t, err)
	assert.NotContains(t, stderr, "Tier 1", "quiet without -v")
}
