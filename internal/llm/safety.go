package llm

import (
	"regexp"
	"strings"
)

// RiskLevel classifies a suggested command before it is offered to the user.
type RiskLevel int

const (
	// RiskNone: an ordinary command.
	RiskNone RiskLevel = iota
	// RiskNotice: worth a second look (several commands, substitution, sudo).
	RiskNotice
	// RiskDanger: likely destructive; the TUI asks for an extra confirmation.
	RiskDanger
)

// Risk is the result of AssessCommand.
type Risk struct {
	Level   RiskLevel
	Reasons []string
}

func (r *Risk) add(level RiskLevel, reason string) {
	for _, existing := range r.Reasons {
		if existing == reason {
			return
		}
	}
	if level > r.Level {
		r.Level = level
	}
	r.Reasons = append(r.Reasons, reason)
}

// AssessCommand inspects a suggested command for destructive patterns and for
// constructs that deserve a closer look. It is a heuristic safety net shown to
// the user, not a sandbox: the user always decides.
func AssessCommand(cmd string) Risk {
	var risk Risk
	parsed := parseShell(cmd)

	for i, sc := range parsed.commands {
		argv := stripWrappers(sc.words)
		if len(argv) == 0 {
			continue
		}
		if hasSudo(sc.words) {
			risk.add(RiskNotice, "uses sudo (runs with administrator privileges)")
		}
		assessSimple(argv, &risk)
		if isDownloader(argv[0]) && i+1 < len(parsed.commands) && parsed.commands[i+1].sep == "|" {
			next := stripWrappers(parsed.commands[i+1].words)
			if len(next) > 0 && isInterpreter(next[0]) {
				risk.add(RiskDanger, "pipes a downloaded script straight into "+next[0])
			}
		}
		for j, w := range argv {
			if (w == ">" || w == ">>" || w == "1>") && j+1 < len(argv) && blockDevice.MatchString(argv[j+1]) {
				risk.add(RiskDanger, "writes directly to a disk device")
			}
			if (strings.HasPrefix(w, ">") || strings.HasPrefix(w, "1>")) && blockDevice.MatchString(strings.TrimLeft(w, "1>")) {
				risk.add(RiskDanger, "writes directly to a disk device")
			}
		}
	}

	compact := strings.Join(strings.Fields(parsed.unquoted), "")
	if strings.Contains(compact, ":(){") {
		risk.add(RiskDanger, "looks like a fork bomb")
	}
	if processSubstFromNet.MatchString(parsed.unquoted) {
		risk.add(RiskDanger, "executes a downloaded script")
	}

	if parsed.chained {
		risk.add(RiskNotice, "runs several commands in a row")
	}
	if parsed.multiline {
		risk.add(RiskNotice, "spans multiple lines")
	}
	if parsed.substitution {
		risk.add(RiskNotice, "uses command substitution")
	}
	return risk
}

var (
	blockDevice         = regexp.MustCompile(`^/dev/(sd[a-z]|hd[a-z]|vd[a-z]|xvd[a-z]|nvme\d|mmcblk\d|disk\d)`)
	processSubstFromNet = regexp.MustCompile(`(^|[\s;&|])(source|\.|sh|bash|zsh|dash|ksh)\s+<\(\s*(curl|wget|fetch)\b`)
)

// Targets whose recursive removal or permission change wipes a home directory,
// the current directory or the whole system.
var sweepingTargets = map[string]bool{
	"/": true, "/*": true, "~": true, "~/": true, "~/*": true,
	"$HOME": true, "${HOME}": true, "$HOME/": true, "$HOME/*": true, "${HOME}/": true, "${HOME}/*": true,
	"*": true, ".": true, "./": true, "./*": true, "..": true, "../": true, "../*": true,
}

func assessSimple(argv []string, risk *Risk) {
	name := argv[0]
	args := argv[1:]
	switch {
	case name == "rm":
		recursive, sweeping := false, false
		for _, a := range args {
			switch {
			case a == "--recursive":
				recursive = true
			case a == "--no-preserve-root":
				sweeping = true
			case strings.HasPrefix(a, "-") && !strings.HasPrefix(a, "--") && strings.ContainsAny(a, "rR"):
				recursive = true
			case sweepingTargets[a]:
				sweeping = true
			}
		}
		if recursive && sweeping {
			risk.add(RiskDanger, "recursively deletes a home, current or root directory")
		}
	case strings.HasPrefix(name, "mkfs"), name == "wipefs", name == "shred":
		risk.add(RiskDanger, "destroys data on disk ("+name+")")
	case name == "dd":
		for _, a := range args {
			if strings.HasPrefix(a, "of=/dev/") {
				risk.add(RiskDanger, "dd writes directly to a device")
			}
		}
	case name == "chmod" || name == "chown" || name == "chgrp":
		recursive, sweeping := false, false
		for _, a := range args {
			if a == "-R" || a == "--recursive" || (strings.HasPrefix(a, "-") && !strings.HasPrefix(a, "--") && strings.Contains(a, "R")) {
				recursive = true
			}
			if sweepingTargets[a] && a != "." && a != "./" && a != "*" {
				sweeping = true
			}
		}
		if recursive && sweeping {
			risk.add(RiskDanger, "recursively changes ownership or permissions of a home or root directory")
		}
	case name == "shutdown" || name == "reboot" || name == "halt" || name == "poweroff":
		risk.add(RiskDanger, "shuts down or restarts the machine")
	case name == "init" && len(args) > 0 && (args[0] == "0" || args[0] == "6"):
		risk.add(RiskDanger, "shuts down or restarts the machine")
	case name == "systemctl" && len(args) > 0 && (args[0] == "poweroff" || args[0] == "reboot" || args[0] == "halt"):
		risk.add(RiskDanger, "shuts down or restarts the machine")
	case (name == "sh" || name == "bash" || name == "zsh" || name == "dash" || name == "ksh" || name == "eval") && containsNetSubstitution(args):
		risk.add(RiskDanger, "executes a downloaded script")
	case name == "git" && len(args) > 0:
		assessGit(args, risk)
	}
}

func assessGit(args []string, risk *Risk) {
	switch args[0] {
	case "reset":
		for _, a := range args[1:] {
			if a == "--hard" {
				risk.add(RiskDanger, "git reset --hard discards uncommitted work")
			}
		}
	case "clean":
		for _, a := range args[1:] {
			if a == "--force" || (strings.HasPrefix(a, "-") && !strings.HasPrefix(a, "--") && strings.Contains(a, "f")) {
				risk.add(RiskDanger, "git clean -f deletes untracked files")
			}
		}
	case "push":
		for _, a := range args[1:] {
			if a == "--force" || (strings.HasPrefix(a, "-") && !strings.HasPrefix(a, "--") && strings.Contains(a, "f")) {
				risk.add(RiskDanger, "git push --force can overwrite commits on the remote")
			}
		}
	}
}

func containsNetSubstitution(args []string) bool {
	for _, a := range args {
		for _, p := range []string{"$(curl", "$(wget", "$(fetch", "`curl", "`wget"} {
			if strings.Contains(a, p) {
				return true
			}
		}
	}
	return false
}

func isDownloader(name string) bool {
	return name == "curl" || name == "wget" || name == "fetch"
}

func isInterpreter(name string) bool {
	switch name {
	case "sh", "bash", "zsh", "fish", "dash", "ksh", "python", "python3", "perl", "ruby", "node":
		return true
	}
	return false
}

func hasSudo(words []string) bool {
	for _, w := range words {
		if w == "sudo" || w == "doas" {
			return true
		}
		if !strings.Contains(w, "=") {
			return false
		}
	}
	return false
}

// stripWrappers removes leading words that only change how the real command
// runs (sudo, env assignments, nohup, ...), returning the effective argv.
func stripWrappers(words []string) []string {
	i := 0
	for i < len(words) {
		w := words[i]
		switch {
		case w == "sudo" || w == "doas":
			i++
			for i < len(words) && strings.HasPrefix(words[i], "-") {
				// sudo -u <user> takes a value
				if words[i] == "-u" || words[i] == "-g" {
					i++
				}
				i++
			}
		case w == "env" || w == "command" || w == "exec" || w == "nohup" || w == "time" || w == "builtin":
			i++
		case w == "nice":
			i++
			if i < len(words) && words[i] == "-n" {
				i += 2
			}
		case strings.Contains(w, "=") && !strings.HasPrefix(w, "=") && !strings.HasPrefix(w, "-"):
			i++ // VAR=value prefix assignment
		default:
			return words[i:]
		}
	}
	return nil
}

type simpleCommand struct {
	sep   string // separator that preceded this command ("", ";", "&&", "||", "|", "&", "\n", "(")
	words []string
}

type shellParse struct {
	commands     []simpleCommand
	unquoted     string // the command with quoted text replaced, for pattern checks
	chained      bool
	multiline    bool
	substitution bool
}

// parseShell is a small, quote-aware splitter. It is not a full shell parser;
// it only needs to tell real separators and substitutions from quoted text.
func parseShell(s string) shellParse {
	var p shellParse
	var cur simpleCommand
	var word strings.Builder
	inWord := false
	var unq strings.Builder

	flushWord := func() {
		if inWord {
			cur.words = append(cur.words, word.String())
			word.Reset()
			inWord = false
		}
	}
	flushCmd := func(nextSep string) {
		flushWord()
		if len(cur.words) > 0 {
			p.commands = append(p.commands, cur)
		}
		cur = simpleCommand{sep: nextSep}
	}

	runes := []rune(s)
	const (
		normal = iota
		single
		double
	)
	state := normal
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		switch state {
		case single:
			if r == '\'' {
				state = normal
				continue
			}
			word.WriteRune(r)
			unq.WriteRune('x')
		case double:
			switch {
			case r == '"':
				state = normal
				continue
			case r == '\\' && i+1 < len(runes):
				i++
				word.WriteRune(runes[i])
				unq.WriteRune('x')
				continue
			case r == '`', r == '$' && i+1 < len(runes) && runes[i+1] == '(':
				p.substitution = true
			}
			word.WriteRune(r)
			unq.WriteRune('x')
		default:
			switch {
			case r == '\'':
				state = single
				inWord = true
			case r == '"':
				state = double
				inWord = true
			case r == '\\' && i+1 < len(runes):
				i++
				word.WriteRune(runes[i])
				inWord = true
				unq.WriteRune('x')
			case r == '#' && !inWord:
				for i < len(runes) && runes[i] != '\n' {
					i++
				}
				i--
			case r == ' ' || r == '\t':
				flushWord()
				unq.WriteRune(r)
			case r == '\n':
				if len(cur.words) > 0 || inWord || len(p.commands) > 0 {
					p.multiline = p.multiline || i < len(runes)-1
				}
				flushCmd("\n")
				unq.WriteRune(r)
			case r == ';':
				p.chained = true
				flushCmd(";")
				unq.WriteRune(r)
			case r == '&':
				if i+1 < len(runes) && runes[i+1] == '&' {
					i++
					p.chained = true
					flushCmd("&&")
					unq.WriteString("&&")
				} else if i > 0 && runes[i-1] == '>' {
					word.WriteRune(r) // >& redirection
					inWord = true
					unq.WriteRune(r)
				} else {
					p.chained = true
					flushCmd("&")
					unq.WriteRune(r)
				}
			case r == '|':
				if i+1 < len(runes) && runes[i+1] == '|' {
					i++
					p.chained = true
					flushCmd("||")
					unq.WriteString("||")
				} else {
					flushCmd("|")
					unq.WriteRune(r)
				}
			case r == '(' || r == ')' || r == '{' && !inWord || r == '}' && !inWord:
				if r == '(' && i > 0 && runes[i-1] == '$' {
					p.substitution = true
				}
				flushCmd(string(r))
				unq.WriteRune(r)
			case r == '`':
				p.substitution = true
				flushCmd("`")
				unq.WriteRune(r)
			default:
				word.WriteRune(r)
				inWord = true
				unq.WriteRune(r)
			}
		}
	}
	flushCmd("")
	p.unquoted = unq.String()
	return p
}
