// Package shell tests the bash, zsh and fish wrappers by running real
// interactive shells in a pseudo-terminal. The real pfs binary is replaced
// by a fake that records what the wrapper passed and prints a prepared reply.
package shell

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

const fakePfs = `#!/bin/sh
# Fake pfs: record the invocation, then print the next prepared reply.
d="$FAKEPFS_DIR"
n=$(cat "$d/count" 2>/dev/null || echo 0)
n=$((n + 1))
echo "$n" > "$d/count"
{
  printf 'CALL %s\n' "$n"
  printf 'ARGS=%s\n' "$*"
  printf 'PFS_EVAL=%s\n' "${PFS_EVAL-<unset>}"
  printf 'PFS_EXIT=%s\n' "${PFS_EXIT-<unset>}"
  printf 'PFS_PIPESTATUS=%s\n' "${PFS_PIPESTATUS-<unset>}"
  printf 'PFS_SHELL=%s\n' "${PFS_SHELL-<unset>}"
  printf 'PFS_CWD=%s\n' "${PFS_CWD-<unset>}"
  printf 'PFS_CMD<<\n%s\n>>\n' "${PFS_CMD-<unset>}"
} >> "$d/calls"
[ -f "$d/out.$n" ] && cat "$d/out.$n"
[ -f "$d/exit.$n" ] && exit "$(cat "$d/exit.$n")"
exit 0
`

type call struct {
	args, eval, exit, pipestatus, shell, cwd, cmd string
}

type session struct {
	t    *testing.T
	dir  string // FAKEPFS_DIR
	home string
	bin  string
}

func newSession(t *testing.T) *session {
	t.Helper()
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 is needed to drive a pseudo-terminal")
	}
	root := t.TempDir()
	s := &session{t: t, dir: filepath.Join(root, "fake"), home: filepath.Join(root, "home"), bin: filepath.Join(root, "bin")}
	for _, d := range []string{s.dir, s.home, s.bin, filepath.Join(s.home, ".config"), filepath.Join(s.home, ".local", "share")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(s.bin, "pfs"), []byte(fakePfs), 0o755); err != nil {
		t.Fatal(err)
	}
	return s
}

// reply prepares what the n-th pfs invocation prints and its exit status.
func (s *session) reply(n int, out string, code int) {
	s.t.Helper()
	if err := os.WriteFile(filepath.Join(s.dir, "out."+strconv.Itoa(n)), []byte(out), 0o644); err != nil {
		s.t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(s.dir, "exit."+strconv.Itoa(n)), []byte(strconv.Itoa(code)), 0o644); err != nil {
		s.t.Fatal(err)
	}
}

func (s *session) exists(name string) bool {
	_, err := os.Stat(filepath.Join(s.dir, name))
	return err == nil
}

func (s *session) read(name string) string {
	data, _ := os.ReadFile(filepath.Join(s.dir, name))
	return string(data)
}

func (s *session) calls() []call {
	var out []call
	var cur *call
	lines := strings.Split(s.read("calls"), "\n")
	for i := 0; i < len(lines); i++ {
		l := lines[i]
		switch {
		case strings.HasPrefix(l, "CALL "):
			out = append(out, call{})
			cur = &out[len(out)-1]
		case cur == nil:
		case strings.HasPrefix(l, "ARGS="):
			cur.args = strings.TrimPrefix(l, "ARGS=")
		case strings.HasPrefix(l, "PFS_EVAL="):
			cur.eval = strings.TrimPrefix(l, "PFS_EVAL=")
		case strings.HasPrefix(l, "PFS_EXIT="):
			cur.exit = strings.TrimPrefix(l, "PFS_EXIT=")
		case strings.HasPrefix(l, "PFS_PIPESTATUS="):
			cur.pipestatus = strings.TrimPrefix(l, "PFS_PIPESTATUS=")
		case strings.HasPrefix(l, "PFS_SHELL="):
			cur.shell = strings.TrimPrefix(l, "PFS_SHELL=")
		case strings.HasPrefix(l, "PFS_CWD="):
			cur.cwd = strings.TrimPrefix(l, "PFS_CWD=")
		case l == "PFS_CMD<<":
			var body []string
			for i++; i < len(lines) && lines[i] != ">>"; i++ {
				body = append(body, lines[i])
			}
			cur.cmd = strings.Join(body, "\n")
		}
	}
	return out
}

// run starts the shell with the given rc file and types the steps.
func (s *session) run(argv []string, steps [][]string) string {
	s.t.Helper()
	wd, _ := os.Getwd()
	driver := filepath.Join(wd, "testdata", "drive.py")
	argvJSON, _ := json.Marshal(argv)
	stepsJSON, _ := json.Marshal(steps)
	cmd := exec.Command("python3", "-I", driver, string(argvJSON), string(stepsJSON), "15")
	cmd.Dir = s.home
	cmd.Env = []string{
		"PATH=" + s.bin + ":" + os.Getenv("PATH"),
		"HOME=" + s.home,
		"XDG_CONFIG_HOME=" + filepath.Join(s.home, ".config"),
		"XDG_DATA_HOME=" + filepath.Join(s.home, ".local", "share"),
		"FAKEPFS_DIR=" + s.dir,
		"FAKEPFS_WRAPPERS=" + wd,
		"TERM=dumb",
		"LANG=C.UTF-8",
		"LC_ALL=C.UTF-8",
	}
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	transcript := string(out)
	if err != nil {
		s.t.Fatalf("driver failed: %v\n%s\ntranscript:\n%s", err, stderr.String(), transcript)
	}
	return transcript
}

func (s *session) writeRC(name, content string) string {
	s.t.Helper()
	path := filepath.Join(s.home, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		s.t.Fatal(err)
	}
	return path
}

type shellCase struct {
	name     string
	start    func(s *session, preamble string) []string
	exit42   string
	pipeline string
	multi    []string
}

var shells = []shellCase{
	{
		name: "bash",
		start: func(s *session, preamble string) []string {
			rc := s.writeRC("bashrc", "PS1='PFSPROMPT> '\nHISTFILE=\"$HOME/.bash_history\"\nHISTCONTROL=ignorespace\n"+preamble+"\nsource \"$FAKEPFS_WRAPPERS/pfs.bash\"\n")
			return []string{"bash", "--noprofile", "--rcfile", rc, "-i"}
		},
		exit42:   "(exit 42)",
		pipeline: `printf '%s\n' "a b" 'çğş' | grep -q zzz`,
		multi:    []string{"if true; then", "  false", "fi"},
	},
	{
		name: "zsh",
		start: func(s *session, preamble string) []string {
			s.writeRC(".zshrc", "PROMPT='PFSPROMPT> '\nunsetopt PROMPT_SP\nHISTFILE=\"$HOME/.zsh_history\"\nSAVEHIST=100\nHISTSIZE=100\n"+preamble+"\nsource \"$FAKEPFS_WRAPPERS/pfs.zsh\"\n")
			return []string{"env", "ZDOTDIR=" + s.home, "zsh", "-i"}
		},
		exit42:   "(exit 42)",
		pipeline: `printf '%s\n' "a b" 'çğş' | grep -q zzz`,
		multi:    []string{"if true; then", "  false", "fi"},
	},
	{
		name: "fish",
		start: func(s *session, preamble string) []string {
			rc := s.writeRC("init.fish", "set -g fish_greeting ''\nfunction fish_prompt; printf 'PFSPROMPT> '; end\n"+preamble+"\nsource $FAKEPFS_WRAPPERS/pfs.fish\n")
			return []string{"fish", "--no-config", "-i", "-C", "source " + rc}
		},
		exit42:   "sh -c 'exit 42'",
		pipeline: `printf '%s\n' "a b" 'çğş' | grep -q zzz`,
		multi:    []string{"if true", "  false", "end"},
	},
}

func forEachShell(t *testing.T, fn func(t *testing.T, sh shellCase)) {
	for _, sh := range shells {
		sh := sh
		t.Run(sh.name, func(t *testing.T) {
			if _, err := exec.LookPath(sh.name); err != nil {
				t.Skip(sh.name + " is not installed")
			}
			fn(t, sh)
		})
	}
}

func TestWrapper_CapturesFailedCommands(t *testing.T) {
	forEachShell(t, func(t *testing.T, sh shellCase) {
		s := newSession(t)
		for n := 1; n <= 4; n++ {
			s.reply(n, "", 3) // "no fix": nothing to evaluate
		}
		transcript := s.run(sh.start(s, ""), [][]string{
			{"true", "pfs"},
			{"false", "pfs"},
			{sh.pipeline, "pfs"},
			{sh.exit42, "pfs"},
			append(append([]string{}, sh.multi...), "pfs"),
		})

		assert(t, strings.Contains(transcript, "Last command was successful"), "success is reported without calling pfs\n%s", transcript)
		calls := s.calls()
		if len(calls) != 4 {
			t.Fatalf("want 4 pfs calls, got %d: %+v\n%s", len(calls), calls, transcript)
		}
		assertEq(t, "false", calls[0].cmd, "command text")
		assertEq(t, "1", calls[0].exit, "exit status")
		assertEq(t, "1", calls[0].eval, "PFS_EVAL marks that stdout is evaluated")
		assertEq(t, sh.name, calls[0].shell, "shell name")
		assertEq(t, s.home, calls[0].cwd, "working directory")

		assertEq(t, sh.pipeline, calls[1].cmd, "whole pipeline with quotes and Unicode")
		assertEq(t, "1", calls[1].exit, "pipeline exit status")
		assertEq(t, "0 1", calls[1].pipestatus, "pipe status")

		assertEq(t, "42", calls[2].exit, "exit status 42")

		assert(t, strings.Contains(calls[3].cmd, "false") && strings.Contains(calls[3].cmd, "if"), "multi-line command is captured as a whole: %q", calls[3].cmd)
		assertEq(t, "1", calls[3].exit, "multi-line exit status")
	})
}

func TestWrapper_EvaluatesOnlyConfirmedOutput(t *testing.T) {
	forEachShell(t, func(t *testing.T, sh shellCase) {
		s := newSession(t)
		s.reply(1, `touch "$FAKEPFS_DIR/must-not-exist"`, 3)
		s.reply(2, "echo A > \"$FAKEPFS_DIR/a\"\necho B > \"$FAKEPFS_DIR/b\"", 0)
		s.reply(3, "", 3)
		transcript := s.run(sh.start(s, ""), [][]string{
			{"false", "pfs"},
			// After a fix runs, it becomes the last command: running pfs
			// again reports success instead of re-fixing the old failure.
			{"false", "pfs", "pfs"},
		})
		assert(t, !s.exists("must-not-exist"), "output of a failed pfs run is never evaluated\n%s", transcript)
		assert(t, s.exists("a") && s.exists("b"), "every line of a confirmed multi-line command runs\n%s", transcript)
		assertEq(t, "A\n", s.read("a"), "first line")
		assert(t, strings.Contains(transcript, "Last command was successful"), "the evaluated fix is tracked as the last command\n%s", transcript)
		assertEq(t, "2", strings.TrimSpace(s.read("count")), "pfs is not called again for a successful fix")
	})
}

func TestWrapper_PassThroughAndNoLeaks(t *testing.T) {
	forEachShell(t, func(t *testing.T, sh shellCase) {
		s := newSession(t)
		s.reply(1, "pfs version test\n", 0)
		s.reply(2, "", 3)
		transcript := s.run(sh.start(s, ""), [][]string{
			{"pfs --version"},
			{"false", "pfs"},
			{`env | grep '^PFS_' > "$FAKEPFS_DIR/leak"; true`},
		})
		calls := s.calls()
		if len(calls) != 2 {
			t.Fatalf("want 2 calls, got %+v\n%s", calls, transcript)
		}
		assertEq(t, "--version", calls[0].args, "arguments are passed through")
		assertEq(t, "<unset>", calls[0].eval, "pass-through output is not evaluated")
		assert(t, strings.Contains(transcript, "pfs version test"), "pass-through output is shown\n%s", transcript)
		assertEq(t, "", s.read("leak"), "PFS_* variables do not leak into the shell environment")
	})
}

// Leading-space commands are not saved in bash history (HISTCONTROL=ignorespace).
// The wrapper must not report the previous history entry instead.
func TestWrapper_BashCommandsMissingFromHistory(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash is not installed")
	}
	s := newSession(t)
	s.reply(1, "", 3)
	s.run(shells[0].start(s, ""), [][]string{
		{"echo first"},
		{" false", "pfs"},
	})
	calls := s.calls()
	if len(calls) != 1 {
		t.Fatalf("want 1 call, got %+v", calls)
	}
	assertEq(t, "false", calls[0].cmd, "command typed with a leading space")
	assertEq(t, "1", calls[0].exit, "its exit status")
}

// Other tools (bash-preexec, starship, direnv) install DEBUG traps and
// PROMPT_COMMAND entries. The wrapper must keep them working.
func TestWrapper_BashCoexistsWithExistingHooks(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash is not installed")
	}
	s := newSession(t)
	s.reply(1, "", 3)
	preamble := "__prev_trap=0\ntrap '__prev_trap=$((__prev_trap + 1))' DEBUG\nPROMPT_COMMAND='__user_prompt=yes'\n"
	transcript := s.run(shells[0].start(s, preamble), [][]string{
		{"false", "pfs"},
		{`echo "trap=$__prev_trap prompt=$__user_prompt" > "$FAKEPFS_DIR/hooks"`},
	})
	assert(t, !strings.Contains(transcript, "command not found"), "no errors from the chained hooks\n%s", transcript)
	hooks := s.read("hooks")
	assert(t, strings.Contains(hooks, "prompt=yes"), "the existing PROMPT_COMMAND still runs: %q", hooks)
	assert(t, !strings.Contains(hooks, "trap=0 "), "the existing DEBUG trap still runs: %q", hooks)
	calls := s.calls()
	if len(calls) != 1 {
		t.Fatalf("want 1 call, got %+v\n%s", calls, transcript)
	}
	assertEq(t, "false", calls[0].cmd, "command text")
	assertEq(t, "1", calls[0].exit, "exit status")
}

func TestWrapper_SourcingTwiceIsHarmless(t *testing.T) {
	forEachShell(t, func(t *testing.T, sh shellCase) {
		s := newSession(t)
		s.reply(1, "", 3)
		var again string
		switch sh.name {
		case "fish":
			again = "source $FAKEPFS_WRAPPERS/pfs.fish"
		default:
			again = "source \"$FAKEPFS_WRAPPERS/pfs." + sh.name + "\""
		}
		transcript := s.run(sh.start(s, ""), [][]string{
			{again},
			{"false", "pfs"},
		})
		calls := s.calls()
		if len(calls) != 1 {
			t.Fatalf("want 1 call, got %+v\n%s", calls, transcript)
		}
		assertEq(t, "false", calls[0].cmd, "command text")
	})
}

func assert(t *testing.T, ok bool, format string, args ...interface{}) {
	t.Helper()
	if !ok {
		t.Errorf(format, args...)
	}
}

func assertEq(t *testing.T, want, got, what string) {
	t.Helper()
	if want != got {
		t.Errorf("%s: want %q, got %q", what, want, got)
	}
}

