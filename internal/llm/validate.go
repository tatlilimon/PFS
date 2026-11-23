package llm

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Limits applied to everything the model returns. A corrected command beyond
// maxCommandBytes is rejected (and retried); display text is trimmed.
const (
	maxCommandBytes = 4096
	maxTextRunes    = 4000
	maxAlternatives = 5
)

// decodeCorrection parses a raw model response and normalizes it. It returns
// an error when the response is not usable, which triggers the next retry tier.
//
// A response with a diagnosis but no command is valid: it is the model's way of
// saying the command cannot be fixed.
func decodeCorrection(raw string) (*Correction, error) {
	var c Correction
	if err := json.Unmarshal([]byte(raw), &c); err != nil {
		return nil, fmt.Errorf("failed to unmarshal correction: %w", err)
	}
	if err := normalizeCorrection(&c); err != nil {
		return nil, err
	}
	return &c, nil
}

func normalizeCorrection(c *Correction) error {
	if c.CorrectedCommand != nil {
		cmd := strings.TrimSpace(*c.CorrectedCommand)
		if cmd == "" || strings.EqualFold(cmd, "null") {
			c.CorrectedCommand = nil
		} else {
			if err := validateCommand(cmd); err != nil {
				return err
			}
			c.CorrectedCommand = &cmd
		}
	}

	c.Diagnosis = capRunes(strings.TrimSpace(SanitizeText(c.Diagnosis)), maxTextRunes)
	if c.Explanation != nil {
		e := capRunes(strings.TrimSpace(SanitizeText(*c.Explanation)), maxTextRunes)
		if e == "" {
			c.Explanation = nil
		} else {
			c.Explanation = &e
		}
	}

	alts := c.AlternativeCommands[:0:0]
	for _, a := range c.AlternativeCommands {
		a = strings.TrimSpace(SanitizeText(a))
		if a == "" || len(a) > maxCommandBytes {
			continue
		}
		alts = append(alts, a)
		if len(alts) == maxAlternatives {
			break
		}
	}
	c.AlternativeCommands = alts

	switch strings.ToLower(strings.TrimSpace(c.Confidence)) {
	case "high":
		c.Confidence = "high"
	case "medium":
		c.Confidence = "medium"
	default:
		// Missing or unknown confidence is treated as the most cautious level.
		c.Confidence = "low"
	}

	if c.CorrectedCommand == nil && c.Diagnosis == "" {
		return errors.New("response has neither a diagnosis nor a corrected command")
	}
	return nil
}

// validateCommand rejects commands that would not display exactly as they run.
// Carriage returns, escape sequences, bidirectional overrides and zero-width
// characters can make a terminal show one command while the shell executes
// another, so such a command is never offered to the user.
func validateCommand(cmd string) error {
	if len(cmd) > maxCommandBytes {
		return fmt.Errorf("corrected_command is too long (%d bytes, limit %d)", len(cmd), maxCommandBytes)
	}
	if !utf8.ValidString(cmd) {
		return errors.New("corrected_command is not valid UTF-8")
	}
	for _, r := range cmd {
		if r == '\n' || r == '\t' {
			continue
		}
		if isHiddenRune(r) {
			return fmt.Errorf("corrected_command contains a control or invisible character (U+%04X)", r)
		}
	}
	return nil
}

// isHiddenRune reports characters that do not display as themselves: C0/C1
// controls, DEL, bidirectional formatting and zero-width characters.
func isHiddenRune(r rune) bool {
	switch {
	case r < 0x20, r == 0x7f, r >= 0x80 && r <= 0x9f:
		return true
	case r >= 0x200b && r <= 0x200f, // zero-width space/joiners, LRM/RLM
		r >= 0x202a && r <= 0x202e, // bidi embeddings and overrides
		r >= 0x2060 && r <= 0x2064, // word joiner, invisible operators
		r >= 0x2066 && r <= 0x2069, // bidi isolates
		r == 0xfeff:                // zero-width no-break space
		return true
	}
	return unicode.Is(unicode.Cf, r) && r != 0xad
}

var (
	csiSequence = regexp.MustCompile(`\x1b\[[0-?]*[ -/]*[@-~]`)
	oscSequence = regexp.MustCompile(`\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)`)
	escSequence = regexp.MustCompile(`\x1b[@-Z\\-_]`)
	c1CSI       = regexp.MustCompile("\u009b[0-?]*[ -/]*[@-~]")
)

// SanitizeText removes terminal escape sequences and invisible characters from
// model-written text before it is displayed. Newlines and tabs are kept.
func SanitizeText(s string) string {
	if s == "" {
		return s
	}
	s = strings.ToValidUTF8(s, "")
	s = oscSequence.ReplaceAllString(s, "")
	s = csiSequence.ReplaceAllString(s, "")
	s = c1CSI.ReplaceAllString(s, "")
	s = escSequence.ReplaceAllString(s, "")
	s = strings.ReplaceAll(s, "\r\n", "\n")
	return strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' {
			return r
		}
		if isHiddenRune(r) {
			return -1
		}
		return r
	}, s)
}

func capRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	runes := []rune(s)
	return string(runes[:n]) + "…"
}
