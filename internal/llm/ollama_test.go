package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
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
			w.Write([]byte(ollamaResponse(t, "Here is the result: " + validCorrectionJSON() + " done")))
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
