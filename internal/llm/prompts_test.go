package llm

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSystemPromptContainsEnvironment(t *testing.T) {
	env := EnvironmentContext{
		OS:           "Linux",
		Shell:        "zsh",
		Architecture: "amd64",
	}
	prompt := SystemPrompt(env)
	assert.Contains(t, prompt, "## Environment")
	assert.Contains(t, prompt, "Linux")
	assert.Contains(t, prompt, "zsh")
	assert.Contains(t, prompt, "amd64")
}

func TestSystemPromptContainsExitCodes(t *testing.T) {
	env := EnvironmentContext{OS: "Linux", Shell: "bash", Architecture: "amd64"}
	prompt := SystemPrompt(env)
	assert.Contains(t, prompt, "## Common Exit Codes")
	assert.Contains(t, prompt, "127")
	assert.Contains(t, prompt, "126")
	assert.Contains(t, prompt, "128+N")
}

func TestSystemPromptContainsRules(t *testing.T) {
	env := EnvironmentContext{OS: "Linux", Shell: "bash", Architecture: "amd64"}
	prompt := SystemPrompt(env)
	assert.Contains(t, prompt, "## Rules")
	assert.Contains(t, prompt, "shell and OS")
	assert.Contains(t, prompt, "destructive")
	assert.Contains(t, prompt, "corrected_command to null")
}

func TestSystemPromptContainsExamples(t *testing.T) {
	env := EnvironmentContext{OS: "Linux", Shell: "bash", Architecture: "amd64"}
	prompt := SystemPrompt(env)
	assert.Contains(t, prompt, "## Examples")

	count := strings.Count(prompt, "### Example")
	assert.Equal(t, 3, count, "should contain exactly 3 few-shot examples")

	assert.Contains(t, prompt, "git commti")
	assert.Contains(t, prompt, "jq")
	assert.Contains(t, prompt, "ls --color=auto")
}

func TestSystemPromptContainsSchema(t *testing.T) {
	env := EnvironmentContext{OS: "Linux", Shell: "bash", Architecture: "amd64"}
	prompt := SystemPrompt(env)
	assert.Contains(t, prompt, "## Response Format")
	assert.Contains(t, prompt, "diagnosis")
	assert.Contains(t, prompt, "corrected_command")
	assert.Contains(t, prompt, "confidence")
	assert.Contains(t, prompt, "alternative_commands")
	assert.Contains(t, prompt, "explanation")
}

func TestUserPromptFormat(t *testing.T) {
	prompt := UserPrompt("lsa -l", "command not found", 127, "/home/user")
	assert.Contains(t, prompt, "lsa -l")
	assert.Contains(t, prompt, "127")
	assert.Contains(t, prompt, "command not found")
	assert.Contains(t, prompt, "/home/user")
}

func TestRetryPromptContainsError(t *testing.T) {
	prompt := RetryPrompt("original prompt here", `{"bad": json`, "unexpected end of JSON")
	assert.Contains(t, prompt, "original prompt here")
	assert.Contains(t, prompt, `{"bad": json`)
	assert.Contains(t, prompt, "unexpected end of JSON")
}

func TestFallbackPromptIsSimple(t *testing.T) {
	env := EnvironmentContext{OS: "Linux", Shell: "bash", Architecture: "amd64"}
	system := SystemPrompt(env)
	fallback := FallbackPrompt("ls -la", "permission denied", 1)

	assert.True(t, len(fallback) < len(system),
		"fallback prompt (%d bytes) should be shorter than system prompt (%d bytes)",
		len(fallback), len(system))
	assert.Contains(t, fallback, "ls -la")
	assert.Contains(t, fallback, "1")
}
