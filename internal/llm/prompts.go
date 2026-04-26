package llm

import (
	"encoding/json"
	"fmt"
	"strings"
)

// SystemPrompt builds the system prompt with environment context, exit codes,
// rules, few-shot examples, and the JSON schema for structured output.
func SystemPrompt(env EnvironmentContext) string {
	var b strings.Builder

	fmt.Fprintln(&b, "You are a command-line error diagnosis expert. Analyze failed shell commands and provide structured corrections.")
	fmt.Fprintln(&b)

	fmt.Fprintln(&b, "## Environment")
	fmt.Fprintf(&b, "- OS: %s\n", env.OS)
	fmt.Fprintf(&b, "- Shell: %s\n", env.Shell)
	fmt.Fprintf(&b, "- Architecture: %s\n", env.Architecture)
	fmt.Fprintln(&b)

	fmt.Fprintln(&b, "## Common Exit Codes")
	fmt.Fprintln(&b, "- 1: General errors")
	fmt.Fprintln(&b, "- 2: Misuse of shell builtins or command arguments")
	fmt.Fprintln(&b, "- 126: Command not executable (permission denied)")
	fmt.Fprintln(&b, "- 127: Command not found")
	fmt.Fprintln(&b, "- 128+N: Terminated by signal N (e.g., 130 = SIGINT)")
	fmt.Fprintln(&b)

	fmt.Fprintln(&b, "## Rules")
	fmt.Fprintln(&b, "1. Only suggest commands compatible with the user's shell and OS")
	fmt.Fprintln(&b, "2. Preserve the user's original intent — fix, don't replace")
	fmt.Fprintln(&b, "3. If the command cannot be fixed, set corrected_command to null")
	fmt.Fprintln(&b, "4. Never suggest destructive commands (rm -rf /, :(){ :|:& };:, chmod -R 777 /, etc.)")
	fmt.Fprintln(&b, "5. If confidence is 'low', explain why in the diagnosis field")
	fmt.Fprintln(&b)

	fmt.Fprintln(&b, "## Examples")
	fmt.Fprintln(&b)

	fmt.Fprintln(&b, "### Example 1: Typo/flag error")
	fmt.Fprintln(&b, "Failed command: git commti -m 'fix'")
	fmt.Fprintln(&b, "Response:")
	example1 := Correction{
		Diagnosis:        "Subcommand 'commti' is a typo for 'commit'",
		CorrectedCommand: strPtr("git commit -m 'fix'"),
		Confidence:       "high",
		Explanation:      strPtr("The subcommand was misspelled. 'git commit' is the correct command."),
	}
	writeJSONExample(&b, example1)
	fmt.Fprintln(&b)

	fmt.Fprintln(&b, "### Example 2: Missing command")
	fmt.Fprintln(&b, "Failed command: jq '.name' data.json (exit 127)")
	fmt.Fprintln(&b, "Response:")
	cmd := "python3 -c \"import json,sys; print(json.load(sys.stdin)['name'])\" < data.json"
	example2 := Correction{
		Diagnosis:           "jq is not installed on this system",
		CorrectedCommand:    &cmd,
		Confidence:          "medium",
		AlternativeCommands: []string{"sudo dnf install jq", "sudo apt-get install jq"},
		Explanation:         strPtr("jq is not found. You can install it or use python3 as an alternative."),
	}
	writeJSONExample(&b, example2)
	fmt.Fprintln(&b)

	fmt.Fprintln(&b, "### Example 3: OS incompatibility")
	fmt.Fprintln(&b, "Failed command: ls --color=auto (on macOS)")
	fmt.Fprintln(&b, "Response:")
	example3 := Correction{
		Diagnosis:        "macOS ls does not support --color=auto; use -G instead",
		CorrectedCommand: strPtr("ls -G"),
		Confidence:       "high",
		Explanation:      strPtr("The --color flag is a GNU coreutils extension. On macOS, use -G for color output."),
	}
	writeJSONExample(&b, example3)
	fmt.Fprintln(&b)

	fmt.Fprintln(&b, "## Response Format")
	fmt.Fprintln(&b, "Respond with a single JSON object matching this schema:")
	schema := CorrectionSchema()
	fmt.Fprintf(&b, "%s\n", string(schema))
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "Do NOT include any text outside the JSON object.")

	return b.String()
}

// UserPrompt builds the user-facing prompt with the failed command details.
func UserPrompt(cmd, output string, exitCode int, cwd string) string {
	sanitized := SanitizeOutput(output, 8192)
	var b strings.Builder
	fmt.Fprintln(&b, "A command failed. Diagnose and correct it.")
	fmt.Fprintf(&b, "- Command: %s\n", cmd)
	fmt.Fprintf(&b, "- Exit code: %d\n", exitCode)
	fmt.Fprintf(&b, "- stderr/output: %s\n", sanitized)
	fmt.Fprintf(&b, "- Working directory: %s\n", cwd)
	return b.String()
}

// RetryPrompt builds a retry prompt when the model returned invalid JSON.
func RetryPrompt(originalUserPrompt, invalidResponse, parseError string) string {
	var b strings.Builder
	fmt.Fprintln(&b, "Your previous response was invalid. Try again.")
	fmt.Fprintf(&b, "Original request:\n%s\n", originalUserPrompt)
	fmt.Fprintf(&b, "Your invalid response:\n%s\n", invalidResponse)
	fmt.Fprintf(&b, "Parse error: %s\n", parseError)
	fmt.Fprintln(&b, "Respond with a single valid JSON object. No other text.")
	return b.String()
}

// FallbackPrompt builds a simplified prompt for fallback mode (tier 3 retry).
func FallbackPrompt(cmd, output string, exitCode int) string {
	sanitized := SanitizeOutput(output, 2048)
	var b strings.Builder
	fmt.Fprintf(&b, "Command failed: %s\n", cmd)
	fmt.Fprintf(&b, "Exit code: %d\n", exitCode)
	fmt.Fprintf(&b, "Output: %s\n", sanitized)
	fmt.Fprintln(&b, "Respond with a JSON object: {\"diagnosis\":\"...\",\"corrected_command\":\"...\",\"confidence\":\"high|medium|low\",\"alternative_commands\":[],\"explanation\":\"...\"}")
	return b.String()
}

func writeJSONExample(b *strings.Builder, v interface{}) {
	data, _ := json.MarshalIndent(v, "", "  ")
	b.Write(data)
	fmt.Fprintln(b)
}

func strPtr(s string) *string {
	return &s
}
