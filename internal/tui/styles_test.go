package tui

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDefaultStyles(t *testing.T) {
	s := DefaultStyles()
	assert.NotNil(t, s.TitleStyle)
	assert.NotNil(t, s.CommandStyle)
	assert.NotNil(t, s.ErrorStyle)
}

func TestStylesRender(t *testing.T) {
	s := DefaultStyles()
	rendered := s.TitleStyle.Render("Test Title")
	assert.NotEmpty(t, rendered)
	assert.Contains(t, rendered, "Test Title")
}

func TestCommandStyle(t *testing.T) {
	s := DefaultStyles()
	rendered := s.CommandStyle.Render("ls -la")
	assert.Contains(t, rendered, "ls -la")
	plain := s.CommandStyle.Render("hello")
	plainStripped := stripANSI(plain)
	assert.Equal(t, "hello", strings.TrimSpace(plainStripped))
}

func TestErrorStyle(t *testing.T) {
	s := DefaultStyles()
	rendered := s.ErrorStyle.Render("something broke")
	assert.Contains(t, rendered, "something broke")
}

func TestBorderStyle(t *testing.T) {
	s := DefaultStyles()
	rendered := s.BorderStyle.Render("content")
	assert.Contains(t, rendered, "content")
	assert.True(t, strings.Contains(rendered, "╭") || strings.Contains(rendered, "┌") || strings.Contains(rendered, "("))
}

func TestConfidenceStyle_High(t *testing.T) {
	s := DefaultStyles()
	style := s.ConfidenceStyle("high")
	rendered := style.Render("HIGH")
	assert.Contains(t, rendered, "HIGH")
}

func TestConfidenceStyle_Medium(t *testing.T) {
	s := DefaultStyles()
	style := s.ConfidenceStyle("medium")
	rendered := style.Render("MEDIUM")
	assert.Contains(t, rendered, "MEDIUM")
}

func TestConfidenceStyle_Low(t *testing.T) {
	s := DefaultStyles()
	style := s.ConfidenceStyle("low")
	rendered := style.Render("LOW")
	assert.Contains(t, rendered, "LOW")
}

func TestConfidenceStyle_Unknown(t *testing.T) {
	s := DefaultStyles()
	style := s.ConfidenceStyle("unknown")
	rendered := style.Render("UNKNOWN")
	assert.Contains(t, rendered, "UNKNOWN")
}

func TestNoColorStyles(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	noColor = true
	t.Cleanup(func() { noColor = false })

	s := DefaultStyles()
	rendered := s.TitleStyle.Render("No Color Title")
	stripped := stripANSI(rendered)
	assert.Contains(t, stripped, "No Color Title")
}

func TestCommandStyleVisuallyDistinct(t *testing.T) {
	s := DefaultStyles()
	cmdRendered := s.CommandStyle.Render("git commit")
	titleRendered := s.TitleStyle.Render("git commit")
	assert.NotEqual(t, cmdRendered, titleRendered)
}
