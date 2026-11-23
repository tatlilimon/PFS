package llm

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDecodeCorrection_Valid(t *testing.T) {
	c, err := decodeCorrection(`{"diagnosis":"typo","corrected_command":"ls -l","confidence":"high","explanation":"fixed"}`)
	require.NoError(t, err)
	require.NotNil(t, c.CorrectedCommand)
	assert.Equal(t, "ls -l", *c.CorrectedCommand)
	assert.Equal(t, "high", c.Confidence)
	require.NotNil(t, c.Explanation)
	assert.Equal(t, "fixed", *c.Explanation)
}

// A response with a diagnosis and no command is the model saying "this cannot
// be fixed". It is a valid answer, not a parse failure.
func TestDecodeCorrection_NoFixIsValid(t *testing.T) {
	for _, raw := range []string{
		`{"diagnosis":"the file does not exist","corrected_command":null,"confidence":"high"}`,
		`{"diagnosis":"the file does not exist","confidence":"high"}`,
		`{"diagnosis":"the file does not exist","corrected_command":"   ","confidence":"high"}`,
		`{"diagnosis":"the file does not exist","corrected_command":"null","confidence":"high"}`,
	} {
		c, err := decodeCorrection(raw)
		require.NoError(t, err, raw)
		assert.Nil(t, c.CorrectedCommand, raw)
		assert.Equal(t, "the file does not exist", c.Diagnosis)
	}
}

func TestDecodeCorrection_Rejects(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{"not json", `not json`, "unmarshal"},
		{"wrong type", `{"diagnosis":"x","corrected_command":42,"confidence":"high"}`, "unmarshal"},
		{"empty object", `{}`, "neither"},
		{"carriage return hides text", "{\"diagnosis\":\"x\",\"corrected_command\":\"rm -rf ~ #\\rls\",\"confidence\":\"high\"}", "control"},
		{"escape sequence", "{\"diagnosis\":\"x\",\"corrected_command\":\"ls\\u001b[2K\",\"confidence\":\"high\"}", "control"},
		{"bidi override", "{\"diagnosis\":\"x\",\"corrected_command\":\"ls \\u202e fr- mr\",\"confidence\":\"high\"}", "control"},
		{"zero width", "{\"diagnosis\":\"x\",\"corrected_command\":\"l\\u200bs\",\"confidence\":\"high\"}", "control"},
		{"too long", `{"diagnosis":"x","corrected_command":"` + strings.Repeat("a", maxCommandBytes+1) + `","confidence":"high"}`, "too long"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := decodeCorrection(tt.raw)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}

func TestDecodeCorrection_KeepsMultiLineAndTabs(t *testing.T) {
	c, err := decodeCorrection("{\"diagnosis\":\"x\",\"corrected_command\":\"cd /tmp\\n\\tls\",\"confidence\":\"medium\"}")
	require.NoError(t, err)
	assert.Equal(t, "cd /tmp\n\tls", *c.CorrectedCommand)
}

func TestDecodeCorrection_IgnoresUnknownFields(t *testing.T) {
	c, err := decodeCorrection(`{"diagnosis":"x","corrected_command":"ls","confidence":"high","extra":{"a":1},"score":3}`)
	require.NoError(t, err)
	assert.Equal(t, "ls", *c.CorrectedCommand)
}

func TestDecodeCorrection_NormalizesConfidence(t *testing.T) {
	for raw, want := range map[string]string{
		`{"diagnosis":"x","corrected_command":"ls","confidence":"HIGH"}`:     "high",
		`{"diagnosis":"x","corrected_command":"ls","confidence":" Medium "}`: "medium",
		`{"diagnosis":"x","corrected_command":"ls","confidence":"certain"}`:  "low",
		`{"diagnosis":"x","corrected_command":"ls"}`:                         "low",
	} {
		c, err := decodeCorrection(raw)
		require.NoError(t, err, raw)
		assert.Equal(t, want, c.Confidence, raw)
	}
}

// Text the model writes is shown in the terminal. Escape sequences in it could
// clear the screen, rewrite earlier lines or set the clipboard (OSC 52).
func TestDecodeCorrection_StripsTerminalControlFromText(t *testing.T) {
	raw := "{\"diagnosis\":\"bad \\u001b[31mred\\u001b[0m\\u001b]52;c;ZXZpbA==\\u0007 news\\r\",\"corrected_command\":\"ls\",\"confidence\":\"high\",\"explanation\":\"\\u202eoops\",\"alternative_commands\":[\"ls -a\\u001b[2K\",\"\",\"ls -l\"]}"
	c, err := decodeCorrection(raw)
	require.NoError(t, err)
	assert.Equal(t, "bad red news", c.Diagnosis)
	require.NotNil(t, c.Explanation)
	assert.Equal(t, "oops", *c.Explanation)
	assert.Equal(t, []string{"ls -a", "ls -l"}, c.AlternativeCommands)
}

func TestDecodeCorrection_CapsTextAndAlternatives(t *testing.T) {
	alts := make([]string, 0, 20)
	for i := 0; i < 20; i++ {
		alts = append(alts, `"ls"`)
	}
	raw := `{"diagnosis":"` + strings.Repeat("é", maxTextRunes+50) + `","corrected_command":"ls","confidence":"high","alternative_commands":[` + strings.Join(alts, ",") + `]}`
	c, err := decodeCorrection(raw)
	require.NoError(t, err)
	assert.LessOrEqual(t, len([]rune(c.Diagnosis)), maxTextRunes+1)
	assert.Len(t, c.AlternativeCommands, maxAlternatives)
}

func TestSanitizeText(t *testing.T) {
	assert.Equal(t, "a\nb\tc", SanitizeText("a\r\nb\tc"))
	assert.Equal(t, "title", SanitizeText("\x1b]0;title\x07title"))
	assert.Equal(t, "x", SanitizeText("\u009b2Jx"))
	assert.Equal(t, "çğış", SanitizeText("çğış"))
}
