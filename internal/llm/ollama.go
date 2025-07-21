package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/ollama/ollama/api"
)

// Correction represents the structured response from an LLM provider.
type Correction struct {
	Explanation      string `json:"explanation"`
	CorrectedCommand string `json:"corrected_command"`
}

// OllamaProvider implements the Provider interface for local Ollama models.
type OllamaProvider struct {
	client *api.Client
	model  string
}

// NewOllamaProvider creates a new Ollama provider with the given configuration.
func NewOllamaProvider() (*OllamaProvider, error) {
	baseURL := os.Getenv("OLLAMA_BASE_URL")
	if baseURL == "" {
		return nil, fmt.Errorf("OLLAMA_BASE_URL is not set")
	}
	model := os.Getenv("OLLAMA_MODEL")
	if model == "" {
		return nil, fmt.Errorf("OLLAMA_MODEL is not set")
	}

	return newOllamaProviderWithClient(baseURL, model, http.DefaultClient)
}

// newOllamaProviderWithClient creates a new Ollama provider with a custom http.Client,
// allowing for testing and custom transport configurations.
func newOllamaProviderWithClient(baseURL, model string, httpClient *http.Client) (*OllamaProvider, error) {
	parsedURL, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse ollama baseurl: %w", err)
	}

	client := api.NewClient(parsedURL, httpClient)

	// Health check to ensure the server is running.
	if err := client.Heartbeat(context.Background()); err != nil {
		return nil, fmt.Errorf("Ollama server is not running: %w", err)
	}

	return &OllamaProvider{client: client, model: model}, nil
}

// ModelName returns the name of the Ollama model being used.
func (p *OllamaProvider) ModelName() string {
	return p.model
}

// GetCorrection sends a request to the Ollama API to correct a failed shell command.
func (p *OllamaProvider) GetCorrection(ctx context.Context, command, output string, exitCode int, verbose bool) (*Correction, error) {
	// Define a function to make an attempt, which can be retried.
	attempt := func(prompt string) (*Correction, error) {
		// No verbose output here, as the loading animation will be running.
		responseText, tokenCount, duration, err := p.getCorrectionText(ctx, prompt)
		if err != nil {
			return nil, fmt.Errorf("Ollama API error: %w", err)
		}
		if verbose {
			fmt.Printf("\n\n Tokens Used: %d\n", tokenCount)
			fmt.Printf("Total Time: %s\n", duration)
			fmt.Printf("Raw Ollama Response: %s\n", responseText)
		}

		if responseText == "" {
			return nil, fmt.Errorf("empty response from Ollama")
		}

		var correction Correction
		// Extract the JSON part of the response, as the model may include other text.
		jsonResponse := extractJSON(responseText)
		if jsonResponse == "" {
			return nil, fmt.Errorf("no valid JSON found in the response from Ollama")
		}

		if err := json.Unmarshal([]byte(jsonResponse), &correction); err != nil {
			// If the JSON is malformed, also treat it as a failure.
			return nil, fmt.Errorf("failed to unmarshal JSON from Ollama response: %w", err)
		}

		// Success!
		return &correction, nil
	}

	// First attempt with the standard prompt.
	prompt := buildPrompt(command, output, exitCode)
	correction, err := attempt(prompt)
	if err == nil && correction != nil && correction.CorrectedCommand != "" {
		return correction, nil // Success on the first try.
	}
	if err != nil && verbose {
		fmt.Printf("First attempt failed with error: %v\n", err)
	}

	// If the first attempt failed (or returned an empty correction), retry with a more insistent prompt.
	prompt = buildRetryPrompt(command, output, exitCode)
	correction, err = attempt(prompt)
	if err == nil && correction != nil && correction.CorrectedCommand != "" {
		return correction, nil // Success on the second try.
	}
	if err != nil && verbose {
		fmt.Printf("Second attempt failed with error: %v\n", err)
	}

	// If both attempts fail, return a clear error message to the user.
	return nil, fmt.Errorf("the language model did not return a valid correction after two attempts")
}

func (p *OllamaProvider) getCorrectionText(ctx context.Context, prompt string) (string, int, time.Duration, error) {
	stream := false
	req := &api.GenerateRequest{
		Model:  p.model,
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

// buildRetryPrompt constructs a more insistent prompt for the LLM.
func buildRetryPrompt(command, output string, exitCode int) string {
	return fmt.Sprintf(
		`Your previous response was not valid JSON. You MUST try again.
	           The user's command was: %s
	           It failed with exit code: %d
	           The command output was: %s

	           Provide a direct JSON object response with the keys "corrected_command" and "explanation".
	           DO NOT write any text other than the JSON object itself.`,
		command, exitCode, output,
	)
}
