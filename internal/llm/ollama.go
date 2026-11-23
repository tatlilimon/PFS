package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/ollama/ollama/api"
	"github.com/tatlilimon/PFS/internal/config"
)

// Errors that callers can match with errors.Is to give the user specific advice.
var (
	ErrOllamaUnavailable = errors.New("Ollama is not reachable")
	ErrModelNotFound     = errors.New("model is not available in Ollama")
	errResponseTooLarge  = errors.New("response from Ollama is too large")
)

// Timeouts and size limits. They are variables so tests can shorten them.
var (
	heartbeatTimeout = 5 * time.Second
	requestTimeout   = 60 * time.Second
	maxResponseBytes = int64(256 << 10)
)

// OllamaProvider implements LLM corrections via a local Ollama instance.
type OllamaProvider struct {
	client *api.Client
	cfg    *config.Config
	// debug receives retry diagnostics when debug_level > 0. It is never
	// stdout: the shell wrapper evaluates whatever pfs prints there.
	debug io.Writer
}

// NewOllamaProvider creates a new Ollama provider with the given configuration.
func NewOllamaProvider(cfg *config.Config) (*OllamaProvider, error) {
	if cfg.OllamaBaseURL == "" {
		return nil, fmt.Errorf("ollama_base_url is not set (run: pfs config set ollama_base_url http://localhost:11434)")
	}
	if cfg.OllamaModel == "" {
		return nil, fmt.Errorf("ollama_model is not set (run: pfs config set ollama_model llama3.2)")
	}

	return newOllamaProviderWithClient(cfg, http.DefaultClient)
}

// newOllamaProviderWithClient creates a new Ollama provider with a custom http.Client,
// allowing for testing and custom transport configurations.
func newOllamaProviderWithClient(cfg *config.Config, httpClient *http.Client) (*OllamaProvider, error) {
	parsedURL, err := url.Parse(cfg.OllamaBaseURL)
	if err != nil || parsedURL.Host == "" || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") {
		return nil, fmt.Errorf("invalid ollama_base_url %q: expected something like http://localhost:11434", cfg.OllamaBaseURL)
	}

	client := api.NewClient(parsedURL, limitResponses(httpClient))

	// Health check to ensure the server is running. A bounded wait keeps pfs
	// from hanging on an address that silently drops packets.
	ctx, cancel := context.WithTimeout(context.Background(), heartbeatTimeout)
	defer cancel()
	if err := client.Heartbeat(ctx); err != nil {
		return nil, fmt.Errorf("%w at %s (start it with 'ollama serve', or point pfs elsewhere with 'pfs config set ollama_base_url <url>'): %v",
			ErrOllamaUnavailable, cfg.OllamaBaseURL, err)
	}

	return &OllamaProvider{client: client, cfg: cfg, debug: os.Stderr}, nil
}

// ModelName returns the name of the Ollama model being used.
func (p *OllamaProvider) ModelName() string {
	return p.cfg.OllamaModel
}

// GetCorrection sends a request to the Ollama API to correct a failed shell command.
// It uses a 3-tier retry strategy:
//  1. Normal call with JSON schema enforcement
//  2. Self-correction retry including the invalid response and the error
//  3. Fallback with simplified prompt and basic JSON format
func (p *OllamaProvider) GetCorrection(ctx context.Context, command, output string, exitCode int, envCtx EnvironmentContext) (*Correction, error) {
	return p.correct(ctx, command, output, exitCode, envCtx, nil)
}

// GetCorrectionStream behaves like GetCorrection but streams the first
// attempt, passing each token to onToken as it arrives. Retries (tiers 2 and
// 3) are not streamed.
func (p *OllamaProvider) GetCorrectionStream(ctx context.Context, command, output string, exitCode int, envCtx EnvironmentContext, onToken func(string)) (*Correction, error) {
	if onToken == nil {
		onToken = func(string) {}
	}
	return p.correct(ctx, command, output, exitCode, envCtx, onToken)
}

func (p *OllamaProvider) correct(ctx context.Context, command, output string, exitCode int, envCtx EnvironmentContext, onToken func(string)) (*Correction, error) {
	systemPrompt := SystemPrompt(envCtx)
	userPrompt := UserPrompt(command, output, exitCode, envCtx.CWD)

	// Tier 1: Normal call with schema-enforced structured output.
	raw, err := p.callLLM(ctx, systemPrompt, userPrompt, CorrectionSchema(), onToken)
	if err != nil {
		// If schema format is rejected (old Ollama), skip to tier 3 with plain JSON.
		if isSchemaRejected(err) {
			return p.fallbackCorrection(ctx, command, output, exitCode)
		}
		return nil, fmt.Errorf("tier 1 LLM call failed: %w", p.explain(err))
	}

	correction, decodeErr := decodeCorrection(raw)
	if decodeErr == nil {
		return correction, nil
	}
	p.debugf("Tier 1 failed (%v), retrying with self-correction...\n", decodeErr)

	// Tier 2: Self-correction — tell the model what went wrong.
	retryPrompt := RetryPrompt(userPrompt, raw, decodeErr.Error())
	raw, err = p.callLLM(ctx, systemPrompt, retryPrompt, CorrectionSchema(), nil)
	if err != nil {
		return nil, fmt.Errorf("tier 2 LLM call failed: %w", p.explain(err))
	}

	correction, decodeErr = decodeCorrection(raw)
	if decodeErr == nil {
		return correction, nil
	}
	p.debugf("Tier 2 failed (%v), falling back to simplified prompt...\n", decodeErr)

	return p.fallbackCorrection(ctx, command, output, exitCode)
}

// fallbackCorrection runs tier 3 with basic JSON format (no schema).
func (p *OllamaProvider) fallbackCorrection(ctx context.Context, command, output string, exitCode int) (*Correction, error) {
	fallbackUser := FallbackPrompt(command, output, exitCode)
	raw, err := p.callLLM(ctx, "", fallbackUser, json.RawMessage(`"json"`), nil)
	if err != nil {
		return nil, fmt.Errorf("tier 3 LLM call failed: %w", p.explain(err))
	}

	jsonStr := extractJSON(raw)
	if jsonStr == "" {
		return nil, fmt.Errorf("tier 3 fallback: no valid JSON found in response")
	}

	correction, decodeErr := decodeCorrection(jsonStr)
	if decodeErr != nil {
		return nil, fmt.Errorf("tier 3 fallback: unusable correction: %w", decodeErr)
	}

	return correction, nil
}

// callLLM sends a single generation request to Ollama with a bounded timeout.
// When onToken is non-nil the request is streamed and tokens are forwarded.
func (p *OllamaProvider) callLLM(ctx context.Context, systemPrompt, userPrompt string, format json.RawMessage, onToken func(string)) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	// The Ollama client stops quietly when its reader fails, so the transport
	// records an oversized body here and the call reports it explicitly.
	overflow := new(atomic.Bool)
	ctx = context.WithValue(ctx, overflowKey{}, overflow)

	stream := onToken != nil
	req := &api.GenerateRequest{
		Model:  p.cfg.OllamaModel,
		Prompt: userPrompt,
		System: systemPrompt,
		Format: format,
		Stream: &stream,
		Options: map[string]interface{}{
			"temperature": 0,
		},
	}

	var response strings.Builder
	respFunc := func(resp api.GenerateResponse) error {
		if int64(response.Len()+len(resp.Response)) > maxResponseBytes {
			return errResponseTooLarge
		}
		response.WriteString(resp.Response)
		if onToken != nil && resp.Response != "" {
			onToken(resp.Response)
		}
		return nil
	}

	err := p.client.Generate(ctx, req, respFunc)
	if overflow.Load() {
		return "", errResponseTooLarge
	}
	if err != nil {
		return "", fmt.Errorf("ollama API error: %w", err)
	}

	responseText := response.String()
	// Check if the response is HTML, which might indicate a captive portal or proxy error.
	if strings.HasPrefix(strings.TrimSpace(responseText), "<!DOCTYPE html>") {
		return "", fmt.Errorf("received an HTML response instead of JSON. Check for captive portals or network proxy issues")
	}

	return responseText, nil
}

// explain turns low-level errors into messages that say what to do.
func (p *OllamaProvider) explain(err error) error {
	msg := err.Error()
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return fmt.Errorf("Ollama did not answer within %s; the model may still be loading, try again: %w", requestTimeout, err)
	case errors.Is(err, errResponseTooLarge) || strings.Contains(msg, errResponseTooLarge.Error()):
		return fmt.Errorf("%w (limit %d bytes)", errResponseTooLarge, maxResponseBytes)
	case errors.Is(err, syscall.ECONNREFUSED):
		return fmt.Errorf("%w at %s (start it with 'ollama serve'): %v", ErrOllamaUnavailable, p.cfg.OllamaBaseURL, err)
	case strings.Contains(msg, "not found") && (strings.Contains(msg, "model") || strings.Contains(msg, "pull")):
		return fmt.Errorf("%w: %q (pull it with: ollama pull %s): %v", ErrModelNotFound, p.cfg.OllamaModel, p.cfg.OllamaModel, err)
	}
	var se api.StatusError
	if errors.As(err, &se) && se.StatusCode == http.StatusNotFound {
		return fmt.Errorf("%w: %q (pull it with: ollama pull %s): %v", ErrModelNotFound, p.cfg.OllamaModel, p.cfg.OllamaModel, err)
	}
	return err
}

func (p *OllamaProvider) debugf(format string, args ...interface{}) {
	if p.debug == nil || config.DebugLevel(p.cfg.DebugLevel) < config.DebugLevelBasic {
		return
	}
	fmt.Fprintf(p.debug, format, args...)
}

func isSchemaRejected(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "cannot unmarshal") || strings.Contains(msg, "format")
}

// extractJSON finds and returns the JSON part of a string.
func extractJSON(s string) string {
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start == -1 || end == -1 || start > end {
		return ""
	}
	return s[start : end+1]
}

// limitResponses returns a copy of base whose responses fail once they exceed
// maxResponseBytes, so a misbehaving server cannot exhaust memory.
func limitResponses(base *http.Client) *http.Client {
	if base == nil {
		base = http.DefaultClient
	}
	c := *base
	rt := c.Transport
	if rt == nil {
		rt = http.DefaultTransport
	}
	c.Transport = limitedTransport{rt: rt}
	return &c
}

type overflowKey struct{}

type limitedTransport struct{ rt http.RoundTripper }

func (t limitedTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	resp, err := t.rt.RoundTrip(r)
	if err != nil {
		return nil, err
	}
	flag, _ := r.Context().Value(overflowKey{}).(*atomic.Bool)
	resp.Body = &limitedBody{rc: resp.Body, remaining: maxResponseBytes, overflow: flag}
	return resp, nil
}

type limitedBody struct {
	rc        io.ReadCloser
	remaining int64
	overflow  *atomic.Bool
}

func (b *limitedBody) Read(p []byte) (int, error) {
	if b.remaining <= 0 {
		var one [1]byte
		n, err := b.rc.Read(one[:])
		if n == 0 && err != nil {
			return 0, err
		}
		if b.overflow != nil {
			b.overflow.Store(true)
		}
		return 0, errResponseTooLarge
	}
	if int64(len(p)) > b.remaining {
		p = p[:b.remaining]
	}
	n, err := b.rc.Read(p)
	b.remaining -= int64(n)
	return n, err
}

func (b *limitedBody) Close() error { return b.rc.Close() }
