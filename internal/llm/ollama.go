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



// OllamaProvider implements the Provider interface for local Ollama models.
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
func (p *OllamaProvider) GetCorrection(ctx context.Context, command, output string, exitCode int) (*Correction, error) {
	prompt := buildPrompt(command, output, exitCode)
	return p.getCorrectionFromPrompt(ctx, prompt)
}

func (p *OllamaProvider) getCorrectionFromPrompt(ctx context.Context, prompt string) (*Correction, error) {
	correction, err := p.attemptCorrection(ctx, prompt)
	if err == nil && correction != nil && correction.CorrectedCommand != nil && *correction.CorrectedCommand != "" {
		return correction, nil
	}

	if config.DebugLevel(p.cfg.DebugLevel) > config.DebugLevelNone {
		fmt.Printf("First attempt failed, trying again with a more insistent prompt. Error: %v\n", err)
	}

	retryPrompt := buildRetryPromptFromOriginal(prompt)
	if config.DebugLevel(p.cfg.DebugLevel) > config.DebugLevelNone {
		fmt.Println("Retrying with a more insistent prompt...")
	}
	return p.attemptCorrection(ctx, retryPrompt)
}

func (p *OllamaProvider) attemptCorrection(ctx context.Context, prompt string) (*Correction, error) {
	responseText, _, _, err := p.getCorrectionText(ctx, prompt)
	if err != nil {
		return nil, err
	}

	jsonResponse := extractJSON(responseText)
	if jsonResponse == "" {
		return nil, fmt.Errorf("no valid JSON found for correction")
	}

	var correction Correction
	if err := json.Unmarshal([]byte(jsonResponse), &correction); err != nil {
		return nil, fmt.Errorf("failed to unmarshal correction: %w", err)
	}
	return &correction, nil
}

func (p *OllamaProvider) getCorrectionText(ctx context.Context, prompt string) (string, int, time.Duration, error) {
	stream := false
	req := &api.GenerateRequest{
		Model:  p.cfg.OllamaModel,
		Prompt: prompt,
		Stream: &stream,
		Options: map[string]interface{}{
			"temperature": 0,
		},
	}

	var responseText string
	var evalCount int
	var evalDuration time.Duration
	respFunc := func(resp api.GenerateResponse) error {
		responseText = resp.Response
		evalCount = resp.EvalCount
		evalDuration = resp.EvalDuration
		return nil
	}

	if err := p.client.Generate(ctx, req, respFunc); err != nil {
		return "", 0, 0, fmt.Errorf("ollama API error: %w", err)
	}

	// Check if the response is HTML, which might indicate a captive portal or proxy error.
	if strings.HasPrefix(strings.TrimSpace(responseText), "<!DOCTYPE html>") {
		return "", 0, 0, fmt.Errorf("received an HTML response instead of JSON. Check for captive portals or network proxy issues")
	}

	return responseText, evalCount, evalDuration, nil
}

// buildPrompt constructs the initial prompt for the LLM.
func buildPrompt(command, output string, exitCode int) string {
	return fmt.Sprintf(
		`You are a command-line expert. A user's command failed.
	           - Command: %s
	           - Exit Code: %d
	           - Command Output: %s

	           Analyze the command, exit code, and output. An exit code of 127 typically means "command not found".
	           Your response MUST be a single, raw JSON object with two keys: "corrected_command" and "explanation".
	           Do NOT include any other text, markdown, or conversational filler.

	           Example Response:
	           {
	             "corrected_command": "ls -a",
	             "explanation": "The command 'lsa' was likely a typo for 'ls'."
	           }`,
		command, exitCode, output,
	)
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

func buildRetryPromptFromOriginal(originalPrompt string) string {
	return fmt.Sprintf(
		`Your previous response was not valid JSON. You MUST try again.
		   The original request was:
		   ---
		   %s
		   ---
		   Provide a direct JSON object response.
		   DO NOT write any text other than the JSON object itself.`,
		originalPrompt,
	)
}
