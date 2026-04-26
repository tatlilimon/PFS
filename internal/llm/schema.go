package llm

import (
	"encoding/json"
	"strings"
)

// Correction represents the structured response from an LLM provider.
type Correction struct {
	Diagnosis           string   `json:"diagnosis"`
	CorrectedCommand    *string  `json:"corrected_command"`
	Confidence          string   `json:"confidence"` // "high", "medium", "low"
	AlternativeCommands []string `json:"alternative_commands"`
	Explanation         *string  `json:"explanation"`
}

// EnvironmentContext holds the user's shell environment details.
type EnvironmentContext struct {
	OS           string
	Shell        string
	ShellVersion string
	Architecture string
	CWD          string
}

// CorrectionSchema returns a JSON schema for Ollama's Format field,
// ensuring structured output matching the Correction struct.
func CorrectionSchema() json.RawMessage {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"diagnosis": map[string]interface{}{
				"type":        "string",
				"description": "Brief technical diagnosis of what went wrong with the command",
			},
			"corrected_command": map[string]interface{}{
				"type":        "string",
				"description": "The fixed command, or null if the command cannot be fixed",
				"nullable":    true,
			},
			"confidence": map[string]interface{}{
				"type":        "string",
				"description": "How confident the model is in the correction",
				"enum":        []string{"high", "medium", "low"},
			},
			"alternative_commands": map[string]interface{}{
				"type":        "array",
				"description": "Alternative approaches if the primary fix is uncertain",
				"items": map[string]interface{}{
					"type": "string",
				},
			},
			"explanation": map[string]interface{}{
				"type":        "string",
				"description": "User-friendly explanation of the error and fix, or null",
				"nullable":    true,
			},
		},
		"required": []string{"diagnosis", "confidence"},
	}

	data, _ := json.Marshal(schema)
	return json.RawMessage(data)
}

// SanitizeOutput strips null bytes and truncates output at maxLen
// with a truncation marker if the input exceeds the limit.
func SanitizeOutput(s string, maxLen int) string {
	s = strings.ReplaceAll(s, "\x00", "")
	if len(s) <= maxLen {
		return s
	}
	const marker = "\n[...truncated]"
	truncLen := maxLen - len(marker)
	if truncLen < 0 {
		truncLen = 0
	}
	return s[:truncLen] + marker
}
