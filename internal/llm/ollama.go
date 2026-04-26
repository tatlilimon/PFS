package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/ollama/ollama/api"
	"github.com/tatlilimon/PFS/internal/config"
)

// OllamaProvider implements LLM corrections via a local Ollama instance.
type OllamaProvider struct {
	client *api.Client
	cfg    *config.Config
}

// NewOllamaProvider creates a new Ollama provider with the given configuration.
func NewOllamaProvider(cfg *config.Config) (*OllamaProvider, error) {
	if cfg.OllamaBaseURL == "" {
		return nil, fmt.Errorf("OLLAMA_BASE_URL is not set")
	}
	if cfg.OllamaModel == "" {
		return nil, fmt.Errorf("OLLAMA_MODEL is not set")
	}

	return newOllamaProviderWithClient(cfg, http.DefaultClient)
}

// newOllamaProviderWithClient creates a new Ollama provider with a custom http.Client,
// allowing for testing and custom transport configurations.
func newOllamaProviderWithClient(cfg *config.Config, httpClient *http.Client) (*OllamaProvider, error) {
	parsedURL, err := url.Parse(cfg.OllamaBaseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse ollama baseurl: %w", err)
	}

	client := api.NewClient(parsedURL, httpClient)

	// Health check to ensure the server is running.
	if err := client.Heartbeat(context.Background()); err != nil {
		return nil, fmt.Errorf("Ollama server is not running: %w", err)
	}

	return &OllamaProvider{client: client, cfg: cfg}, nil
}

// ModelName returns the name of the Ollama model being used.
func (p *OllamaProvider) ModelName() string {
	return p.cfg.OllamaModel
}

// GetCorrection sends a request to the Ollama API to correct a failed shell command.
// It uses a 3-tier retry strategy:
//  1. Normal call with JSON schema enforcement
//  2. Self-correction retry including the invalid response and parse error
//  3. Fallback with simplified prompt and basic JSON format
func (p *OllamaProvider) GetCorrection(ctx context.Context, command, output string, exitCode int, envCtx EnvironmentContext) (*Correction, error) {
	systemPrompt := SystemPrompt(envCtx)
	userPrompt := UserPrompt(command, output, exitCode, envCtx.CWD)

	// Tier 1: Normal call with schema-enforced structured output.
	raw, err := p.callLLM(ctx, systemPrompt, userPrompt, CorrectionSchema())
	if err != nil {
		return nil, fmt.Errorf("tier 1 LLM call failed: %w", err)
	}

	correction, parseErr := parseCorrection(raw)
	if parseErr == nil && isValidCorrection(correction) {
		return correction, nil
	}

	if config.DebugLevel(p.cfg.DebugLevel) >= config.DebugLevelBasic {
		fmt.Printf("Tier 1 failed (parse error: %v), retrying with self-correction...\n", parseErr)
	}

	// Tier 2: Self-correction — tell the model what went wrong.
	retryPrompt := RetryPrompt(userPrompt, raw, parseErr.Error())
	raw, err = p.callLLM(ctx, systemPrompt, retryPrompt, CorrectionSchema())
	if err != nil {
		return nil, fmt.Errorf("tier 2 LLM call failed: %w", err)
	}

	correction, parseErr = parseCorrection(raw)
	if parseErr == nil && isValidCorrection(correction) {
		return correction, nil
	}

	if config.DebugLevel(p.cfg.DebugLevel) >= config.DebugLevelBasic {
		fmt.Printf("Tier 2 failed (parse error: %v), falling back to simplified prompt...\n", parseErr)
	}

	// Tier 3: Fallback — simplified prompt, basic JSON format (no schema).
	fallbackUser := FallbackPrompt(command, output, exitCode)
	raw, err = p.callLLM(ctx, "", fallbackUser, json.RawMessage(`"json"`))
	if err != nil {
		return nil, fmt.Errorf("tier 3 LLM call failed: %w", err)
	}

	// Use extractJSON as safety net for tier 3 parsing.
	jsonStr := extractJSON(raw)
	if jsonStr == "" {
		return nil, fmt.Errorf("tier 3 fallback: no valid JSON found in response")
	}

	correction, parseErr = parseCorrection(jsonStr)
	if parseErr != nil {
		return nil, fmt.Errorf("tier 3 fallback: failed to parse correction: %w", parseErr)
	}

	return correction, nil
}

// callLLM sends a single generation request to Ollama with a 60-second context timeout.
func (p *OllamaProvider) callLLM(ctx context.Context, systemPrompt, userPrompt string, format json.RawMessage) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	stream := false
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

	var responseText string
	respFunc := func(resp api.GenerateResponse) error {
		responseText = resp.Response
		return nil
	}

	if err := p.client.Generate(ctx, req, respFunc); err != nil {
		return "", fmt.Errorf("ollama API error: %w", err)
	}

	// Check if the response is HTML, which might indicate a captive portal or proxy error.
	if strings.HasPrefix(strings.TrimSpace(responseText), "<!DOCTYPE html>") {
		return "", fmt.Errorf("received an HTML response instead of JSON. Check for captive portals or network proxy issues")
	}

	return responseText, nil
}

func parseCorrection(raw string) (*Correction, error) {
	var correction Correction
	if err := json.Unmarshal([]byte(raw), &correction); err != nil {
		return nil, fmt.Errorf("failed to unmarshal correction: %w", err)
	}
	return &correction, nil
}

func isValidCorrection(c *Correction) bool {
	return c != nil && c.CorrectedCommand != nil && *c.CorrectedCommand != ""
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
