package llm

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCorrectionJSONRoundtrip(t *testing.T) {
	cmd := "git commit -m 'fix'"
	explanation := "Fixed typo in subcommand"
	original := Correction{
		Diagnosis:           "Subcommand 'commti' is a typo",
		CorrectedCommand:    &cmd,
		Confidence:          "high",
		AlternativeCommands: []string{"git commit --amend"},
		Explanation:         &explanation,
	}

	data, err := json.Marshal(original)
	assert.NoError(t, err)

	var decoded Correction
	err = json.Unmarshal(data, &decoded)
	assert.NoError(t, err)

	assert.Equal(t, original.Diagnosis, decoded.Diagnosis)
	assert.NotNil(t, decoded.CorrectedCommand)
	assert.Equal(t, cmd, *decoded.CorrectedCommand)
	assert.Equal(t, original.Confidence, decoded.Confidence)
	assert.Equal(t, original.AlternativeCommands, decoded.AlternativeCommands)
	assert.NotNil(t, decoded.Explanation)
	assert.Equal(t, explanation, *decoded.Explanation)
}

func TestCorrectionNilCommand(t *testing.T) {
	c := Correction{
		Diagnosis:        "Command cannot be fixed",
		CorrectedCommand: nil,
		Confidence:       "low",
	}

	data, err := json.Marshal(c)
	assert.NoError(t, err)

	assert.Contains(t, string(data), `"corrected_command":null`)

	var decoded Correction
	err = json.Unmarshal(data, &decoded)
	assert.NoError(t, err)
	assert.Nil(t, decoded.CorrectedCommand)
}

func TestSanitizeStripsNullBytes(t *testing.T) {
	input := "hello\x00world\x00test"
	result := SanitizeOutput(input, 1000)
	assert.Equal(t, "helloworldtest", result)
}

func TestSanitizeTruncatesLargeOutput(t *testing.T) {
	input := strings.Repeat("a", 20*1024)
	result := SanitizeOutput(input, 10240)

	assert.True(t, len(result) <= 10240+len("\n[...truncated]"))
	assert.Contains(t, result, "[...truncated]")
	assert.True(t, strings.HasPrefix(result, "aaa"))
}

func TestCorrectionSchemaValidJSON(t *testing.T) {
	schema := CorrectionSchema()
	assert.True(t, json.Valid(schema), "CorrectionSchema should return valid JSON")

	var parsed map[string]interface{}
	err := json.Unmarshal(schema, &parsed)
	assert.NoError(t, err)

	props, ok := parsed["properties"].(map[string]interface{})
	assert.True(t, ok, "schema should have properties")
	assert.Contains(t, props, "diagnosis")
	assert.Contains(t, props, "corrected_command")
	assert.Contains(t, props, "confidence")
	assert.Contains(t, props, "alternative_commands")
	assert.Contains(t, props, "explanation")

	confidence := props["confidence"].(map[string]interface{})
	enum, ok := confidence["enum"].([]interface{})
	assert.True(t, ok, "confidence should have enum")
	assert.ElementsMatch(t, []interface{}{"high", "medium", "low"}, enum)
}
