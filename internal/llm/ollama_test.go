package llm

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ollama/ollama/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tatlilimon/PFS/internal/config"
)

func defaultEnvCtx() EnvironmentContext {
	return EnvironmentContext{
		OS:           "Linux",
		Shell:        "bash",
		Architecture: "amd64",
		CWD:          "/home/user",
	}
}

func validCorrectionJSON() string {
	corrected := "ls -l"
	explanation := "Typo in command name"
	corr := Correction{
		Diagnosis:        "Command 'lş' not found, likely a typo for 'ls'",
		CorrectedCommand: &corrected,
		Confidence:       "high",
		Explanation:      &explanation,
	}
	data, _ := json.Marshal(corr)
	return string(data)
}

func ollamaResponse(t *testing.T, responseJSON string) string {
	t.Helper()
	resp := api.GenerateResponse{
		Response: responseJSON,
		Done:     true,
	}
	data, err := json.Marshal(resp)
	require.NoError(t, err)
	return string(data)
}

func newTestProvider(server *httptest.Server) *OllamaProvider {
	parsedURL, err := url.Parse(server.URL)
	if err != nil {
		panic(err)
	}
	client := api.NewClient(parsedURL, http.DefaultClient)
	return &OllamaProvider{
		client: client,
		cfg:    &config.Config{OllamaModel: "test-model"},
	}
}

func TestGetCorrection_HappyPath(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/generate", r.URL.Path)
		assert.Equal(t, "POST", r.Method)

		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(ollamaResponse(t, validCorrectionJSON())))
	}))
	defer server.Close()

	provider := newTestProvider(server)
	correction, err := provider.GetCorrection(
		context.Background(), "lş -l", "lş: command not found", 127, defaultEnvCtx(),
	)

	require.NoError(t, err)
	require.NotNil(t, correction)
	assert.Equal(t, "ls -l", *correction.CorrectedCommand)
	assert.Equal(t, "high", correction.Confidence)
}

func TestGetCorrection_RetryTier2(t *testing.T) {
	var callCount int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		count := atomic.AddInt32(&callCount, 1)

		if count == 1 {
			w.Write([]byte(ollamaResponse(t, "this is not valid json at all")))
		} else {
			w.Write([]byte(ollamaResponse(t, validCorrectionJSON())))
		}
	}))
	defer server.Close()

	provider := newTestProvider(server)
	correction, err := provider.GetCorrection(
		context.Background(), "lş -l", "lş: command not found", 127, defaultEnvCtx(),
	)

	require.NoError(t, err)
	require.NotNil(t, correction)
	assert.Equal(t, "ls -l", *correction.CorrectedCommand)
	assert.Equal(t, int32(2), atomic.LoadInt32(&callCount))
}

func TestGetCorrection_Fallback(t *testing.T) {
	var callCount int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		count := atomic.AddInt32(&callCount, 1)

		if count <= 2 {
			w.Write([]byte(ollamaResponse(t, "still not json")))
		} else {
			w.Write([]byte(ollamaResponse(t, validCorrectionJSON())))
		}
	}))
	defer server.Close()

	provider := newTestProvider(server)
	correction, err := provider.GetCorrection(
		context.Background(), "lş -l", "lş: command not found", 127, defaultEnvCtx(),
	)

	require.NoError(t, err)
	require.NotNil(t, correction)
	assert.Equal(t, "ls -l", *correction.CorrectedCommand)
	assert.Equal(t, int32(3), atomic.LoadInt32(&callCount))
}

func TestGetCorrection_Fallback_ExtractJSON(t *testing.T) {
	var callCount int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		count := atomic.AddInt32(&callCount, 1)

		if count <= 2 {
			w.Write([]byte(ollamaResponse(t, "not json")))
		} else {
			w.Write([]byte(ollamaResponse(t, "Here is the result: "+validCorrectionJSON()+" done")))
		}
	}))
	defer server.Close()

	provider := newTestProvider(server)
	correction, err := provider.GetCorrection(
		context.Background(), "lş -l", "lş: command not found", 127, defaultEnvCtx(),
	)

	require.NoError(t, err)
	require.NotNil(t, correction)
	assert.Equal(t, "ls -l", *correction.CorrectedCommand)
}

func TestGetCorrection_Timeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	provider := newTestProvider(server)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err := provider.GetCorrection(
		ctx, "lş -l", "lş: command not found", 127, defaultEnvCtx(),
	)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "tier 1")
}

func TestGetCorrection_AllTiersFail(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(ollamaResponse(t, "not json at all")))
	}))
	defer server.Close()

	provider := newTestProvider(server)
	_, err := provider.GetCorrection(
		context.Background(), "lş -l", "lş: command not found", 127, defaultEnvCtx(),
	)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "tier 3 fallback")
}

func TestNewOllamaProvider_HTTPS(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodHead, r.Method)
		assert.Equal(t, "/", r.URL.Path)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := &config.Config{OllamaBaseURL: server.URL, OllamaModel: "test-model"}
	provider, err := newOllamaProviderWithClient(cfg, server.Client())

	assert.NoError(t, err)
	assert.NotNil(t, provider)
}

func TestExtractJSON(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"clean json", `{"key":"value"}`, `{"key":"value"}`},
		{"json with prefix", `here is the result: {"key":"value"} done`, `{"key":"value"}`},
		{"no json", "no json here", ""},
		{"nested json", `{"outer":{"inner":"val"}}`, `{"outer":{"inner":"val"}}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractJSON(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func noFixJSON() string {
	return `{"diagnosis":"The file does not exist, so there is nothing to fix.","corrected_command":null,"confidence":"high"}`
}

// A first answer that says "cannot be fixed" used to dereference a nil parse
// error while building the retry prompt and crash the program.
func TestGetCorrection_NoFixAtTier1(t *testing.T) {
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.Write([]byte(ollamaResponse(t, noFixJSON())))
	}))
	defer server.Close()

	provider := newTestProvider(server)
	var correction *Correction
	var err error
	require.NotPanics(t, func() {
		correction, err = provider.GetCorrection(context.Background(), "cat nope.txt", "", 1, defaultEnvCtx())
	})
	require.NoError(t, err)
	assert.Nil(t, correction.CorrectedCommand)
	assert.Contains(t, correction.Diagnosis, "does not exist")
	assert.Equal(t, int32(1), atomic.LoadInt32(&calls), "a valid no-fix answer must not trigger retries")
}

// The shell wrapper evaluates whatever pfs prints on stdout, so diagnostics must
// never go there.
func TestGetCorrection_DebugOutputNeverReachesStdout(t *testing.T) {
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&calls, 1) == 1 {
			w.Write([]byte(ollamaResponse(t, "not json")))
			return
		}
		w.Write([]byte(ollamaResponse(t, validCorrectionJSON())))
	}))
	defer server.Close()

	provider := newTestProvider(server)
	provider.cfg.DebugLevel = int(config.DebugLevelDetailed)
	var debug strings.Builder
	provider.debug = &debug

	stdout := captureStdout(t, func() {
		_, err := provider.GetCorrection(context.Background(), "lş -l", "", 127, defaultEnvCtx())
		require.NoError(t, err)
	})
	assert.Empty(t, stdout)
	assert.Contains(t, debug.String(), "Tier 1")
}

func TestGetCorrection_RejectsUnsafeCommandAndRetries(t *testing.T) {
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&calls, 1) == 1 {
			w.Write([]byte(ollamaResponse(t, "{\"diagnosis\":\"x\",\"corrected_command\":\"rm -rf ~ #\\rls -l\",\"confidence\":\"high\"}")))
			return
		}
		w.Write([]byte(ollamaResponse(t, validCorrectionJSON())))
	}))
	defer server.Close()

	correction, err := newTestProvider(server).GetCorrection(context.Background(), "lş -l", "", 127, defaultEnvCtx())
	require.NoError(t, err)
	assert.Equal(t, "ls -l", *correction.CorrectedCommand)
	assert.Equal(t, int32(2), atomic.LoadInt32(&calls))
}

func TestGetCorrection_ModelNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"error":"model \"test-model\" not found, try pulling it first"}`))
	}))
	defer server.Close()

	_, err := newTestProvider(server).GetCorrection(context.Background(), "ls", "", 1, defaultEnvCtx())
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrModelNotFound)
	assert.Contains(t, err.Error(), "ollama pull test-model")
}

func TestNewOllamaProvider_ServerDown(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	url := server.URL
	server.Close() // nothing listens on this port any more

	_, err := NewOllamaProvider(&config.Config{OllamaBaseURL: url, OllamaModel: "m"})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrOllamaUnavailable)
	assert.Contains(t, err.Error(), "ollama serve")
}

func TestNewOllamaProvider_HeartbeatTimesOut(t *testing.T) {
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-release
	}))
	defer server.Close()
	defer close(release)

	old := heartbeatTimeout
	heartbeatTimeout = 200 * time.Millisecond
	defer func() { heartbeatTimeout = old }()

	start := time.Now()
	_, err := NewOllamaProvider(&config.Config{OllamaBaseURL: server.URL, OllamaModel: "m"})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrOllamaUnavailable)
	assert.Less(t, time.Since(start), 3*time.Second)
}

func TestGetCorrection_ResponseTooLarge(t *testing.T) {
	old := maxResponseBytes
	maxResponseBytes = 4096
	defer func() { maxResponseBytes = old }()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			return
		}
		w.Write([]byte(ollamaResponse(t, `{"diagnosis":"`+strings.Repeat("a", 10000)+`"}`)))
	}))
	defer server.Close()

	provider, err := NewOllamaProvider(&config.Config{OllamaBaseURL: server.URL, OllamaModel: "m"})
	require.NoError(t, err)
	_, err = provider.GetCorrection(context.Background(), "ls", "", 1, defaultEnvCtx())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "too large")
}

func TestGetCorrectionStream_DeliversTokensAndParses(t *testing.T) {
	full := validCorrectionJSON()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req api.GenerateRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		require.NotNil(t, req.Stream)
		assert.True(t, *req.Stream)
		w.Header().Set("Content-Type", "application/x-ndjson")
		for i := 0; i < len(full); i += 7 {
			end := i + 7
			if end > len(full) {
				end = len(full)
			}
			chunk, _ := json.Marshal(api.GenerateResponse{Response: full[i:end]})
			w.Write(append(chunk, '\n'))
		}
		done, _ := json.Marshal(api.GenerateResponse{Done: true})
		w.Write(append(done, '\n'))
	}))
	defer server.Close()

	var tokens strings.Builder
	correction, err := newTestProvider(server).GetCorrectionStream(context.Background(), "lş -l", "", 127, defaultEnvCtx(), func(tok string) {
		tokens.WriteString(tok)
	})
	require.NoError(t, err)
	assert.Equal(t, "ls -l", *correction.CorrectedCommand)
	assert.Equal(t, full, tokens.String())
}

func TestGetCorrectionStream_InvalidStreamFallsBackToRetries(t *testing.T) {
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&calls, 1) == 1 {
			chunk, _ := json.Marshal(api.GenerateResponse{Response: "not json", Done: true})
			w.Write(append(chunk, '\n'))
			return
		}
		w.Write([]byte(ollamaResponse(t, validCorrectionJSON())))
	}))
	defer server.Close()

	correction, err := newTestProvider(server).GetCorrectionStream(context.Background(), "lş -l", "", 127, defaultEnvCtx(), func(string) {})
	require.NoError(t, err)
	assert.Equal(t, "ls -l", *correction.CorrectedCommand)
	assert.Equal(t, int32(2), atomic.LoadInt32(&calls))
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	require.NoError(t, err)
	orig := os.Stdout
	os.Stdout = w
	defer func() { os.Stdout = orig }()
	fn()
	require.NoError(t, w.Close())
	data, err := io.ReadAll(r)
	require.NoError(t, err)
	return string(data)
}
