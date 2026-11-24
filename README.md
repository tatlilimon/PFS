# PFS — Please Find Solution

Fix failed shell commands with a local LLM.

PFS is a CLI tool that diagnoses and corrects failed shell commands using Ollama. It runs entirely on your machine. No data leaves, no API keys needed, no cloud dependency.

## Features

- **Privacy-first**: Runs entirely on local Ollama. No data leaves your machine.
- **Interactive TUI**: Spinner, streaming mode (Ctrl+X) and a confirmation dialog, drawn on stderr.
- **Nothing runs without you**: a suggested command runs only after you press `r` (or save it from your editor). Likely destructive commands need a second key.
- **Smart confirmation**: Confidence indicators (high/medium/low), warnings for low confidence and for commands that chain, substitute or use sudo.
- **Editor mode**: Edit the suggested command in `$EDITOR` before running it.
- **3 shells**: Bash, Zsh and Fish wrappers that record the last command line, its exit status and pipe status.
- **Few moving parts**: No jq, no external tools. Just Go and Ollama.
- **Direct eval pattern**: The confirmed command goes to stdout and the wrapper evaluates it. Same approach as thefuck.
- **Structured output**: JSON schema enforcement with a 3-tier retry for reliable LLM responses.

## Requirements

- **Go 1.25.8+** (for building; see `go.mod`)
- **Ollama** running locally with at least one model pulled

Recommended model: `llama3.2` or larger for best results. `deepseek-r1:1.5b` works but gives lower quality suggestions.

```bash
ollama pull llama3.2
```

## Installation

Clone and build:

```bash
git clone https://github.com/tatlilimon/PFS.git
cd PFS
make build
sudo make install          # binary to /usr/local/bin, wrappers to /usr/local/share/pfs/shell
```

`make install PREFIX=$HOME/.local` installs without sudo; `DESTDIR` is supported for packaging.

Or manually:

```bash
go build -ldflags "-X main.Version=$(git describe --tags --always)" -o pfs ./cmd/
sudo cp pfs /usr/local/bin/
```

## Shell Setup

### Quick setup

```bash
pfs setup
```

This prints the right `source` line for your current shell. It finds the wrappers next to the binary (a binary built in the repository), under `<prefix>/share/pfs/shell` (after `make install`), or in `/usr/share/pfs/shell`.

### Manual setup

Add the wrapper to your shell config.

**Zsh** (`~/.zshrc`):
```bash
source /path/to/PFS/shell/pfs.zsh
```

**Bash** (`~/.bashrc`) — source it **after** other prompt tools (starship, bash-preexec, direnv), so it can read the exit status first:
```bash
source /path/to/PFS/shell/pfs.bash
```

**Fish** (`~/.config/fish/config.fish`):
```fish
source /path/to/PFS/shell/pfs.fish
```

Then reload your shell:

```bash
source ~/.zshrc   # or ~/.bashrc, or restart fish
```

## Usage

The basic workflow:

```
$ lsa -l
zsh: lsa: command not found...

$ pfs
```

PFS takes the failed command line, its exit status and pipe status from the wrapper, sends them to Ollama, and shows the diagnosis and the corrected command. You choose what to do. If the corrected command fails too, running `pfs` again works on it.

The command's output (stderr) is **not** captured yet: PFS works from the command text and exit status only. Capturing output is planned for a later version.

### Subcommands

| Command | Description |
|---|---|
| `pfs` | Fix the last failed command |
| `pfs explain` | Explain what went wrong (no fix, no execution, nothing on stdout) |
| `pfs config get <key>` | View a config value |
| `pfs config set <key> <value>` | Set a config value |
| `pfs setup` | Show shell integration instructions |
| `pfs --version` | Print version |
| `pfs --format json` | Machine-readable JSON output (no TUI) |
| `pfs -v` / `-vv` | Verbose retry diagnostics on stderr |

### TUI keybindings

| Key | Action |
|---|---|
| `r` | Run the corrected command |
| `y` | Confirm a command marked **Destructive** (after `r`; any other key goes back) |
| `e` | Edit in `$EDITOR`; saving runs the edited command |
| `x`, `q` or `Esc` | Dismiss |
| `Ctrl+X` | Toggle streaming mode mid-request |
| `Ctrl+C` | Cancel |

In the editor, save to run the command. To cancel, empty the file or quit with an error (`:cq` in vim): nothing runs. `EDITOR` may include arguments (`code --wait`); without `EDITOR`, `vi` is used. The command is edited in a private temporary file (mode 0600) that is removed right after.

## Safety

- The wrappers evaluate what `pfs` prints on stdout, so PFS prints a command there only after you confirm it. The wrappers set `PFS_EVAL=1`; when no terminal is available for the dialog (for example `pfs 2>/dev/null`) or `--format json` is used through the wrapper, PFS shows the suggestion on stderr, prints nothing on stdout and exits 3.
- Commands that look destructive need `r` and then `y`: recursive deletes of `/`, `~`, `.` or `*`, `mkfs`/`wipefs`/`shred`, `dd` or redirects to a disk device, recursive `chmod`/`chown` of `/` or `~`, fork bombs, downloaded scripts piped into a shell, `git reset --hard`, `git clean -f`, `git push --force`, and shutdown/reboot. This is a heuristic safety net, not a sandbox: read the command.
- Commands that run several steps, span multiple lines, use command substitution or `sudo` get a note under the command.
- Suggestions containing control characters, escape sequences, bidirectional overrides or zero-width characters are rejected, because they could display as one command and run as another. Escape sequences are removed from all text the model writes before it is shown.

## Configuration

Config file: `~/.config/pfs/config.yaml` (XDG standard). Keys you leave out keep their defaults.

Defaults:

```yaml
ollama_base_url: http://localhost:11434
ollama_model: llama3.2
offline_mode: true
debug_level: 0
```

CLI configuration:

```bash
pfs config get ollama_model              # → llama3.2
pfs config set ollama_model codellama    # persists to config.yaml
pfs config set ollama_base_url http://192.168.1.100:11434
```

| Key | Values |
|---|---|
| `ollama_base_url` | An `http://` or `https://` URL |
| `ollama_model` | Any model name you have pulled |
| `offline_mode` (alias `offline`) | `true`/`false`. Kept for compatibility; PFS only ever talks to your Ollama |
| `debug_level` | `0`, `1` or `2`: retry diagnostics on stderr |

If `~/.pfs.env` exists from an old installation, PFS migrates it to YAML on first run, keeps the old file as `~/.pfs.env.bak` and prints a notice on stderr.

If Ollama is not running, PFS says so and suggests `ollama serve`; if the model is missing, it suggests `ollama pull <model>`.

## How It Works

1. The shell wrapper records the command line, exit status and pipe status via hooks (bash: DEBUG trap + `PROMPT_COMMAND`, zsh: preexec/precmd, fish: `fish_preexec`/`fish_postexec`)
2. `pfs` reads them from its environment (`PFS_CMD`, `PFS_EXIT`, `PFS_PIPESTATUS`, `PFS_CWD`, `PFS_SHELL`; `PFS_OUTPUT` is empty for now). They are passed to the `pfs` process only, not exported into your shell
3. Ollama is called with structured prompts and a JSON schema; 3-tier retry: schema enforcement, then self-correction, then fallback
4. The answer is validated, the TUI shows it on stderr, and you confirm
5. The confirmed command prints to stdout; the shell evaluates it and records it as the last command

## Exit codes

| Code | Meaning |
|---|---|
| 0 | A command was confirmed (or printed, in script mode) |
| 1 | Error (no shell integration, config error, editor failed or was cancelled) |
| 2 | LLM error (Ollama unreachable, model missing, no usable answer) |
| 3 | No fix: the model found none, you dismissed it, or no terminal to confirm |
| 4 | Ambiguous: no single fix, but alternative approaches were shown |
| 5 | Usage error (unknown flag, command or config value; invalid `--format`) |

Without the wrapper (scripts), `pfs` with a non-terminal stderr prints the bare corrected command on stdout, and `--format json` prints the full answer as JSON; the exit codes above still apply.

## Architecture

```
cmd/                # CLI entry point (Cobra commands)
internal/config/    # YAML config, migration from legacy .pfs.env
internal/llm/       # Ollama client, prompts, JSON schema, retry, output validation, command risk
internal/tui/       # Bubbletea v2 TUI: spinner, streaming, confirmation, editor
shell/              # Shell wrappers (bash, zsh, fish) and their terminal tests
```

## Development

```bash
make test         # all tests
make test-short   # skips the end-to-end terminal test
go test -race ./...
make vet
```

The shell tests drive real bash, zsh and fish in a pseudo-terminal (`shell/testdata/drive.py`, Python standard library) and skip shells that are not installed. The end-to-end tests build the binary, point it at a fake Ollama and press the TUI keys. No test calls a real Ollama.

## Contributing

PRs and issues welcome. Run `make test` before submitting.

## Feel Free to Contribute This Project!

You can help develop this app by opening a pull request or issue.
