# PFS — Please Find Solution

Fix failed shell commands with a local LLM.

PFS is a CLI tool that diagnoses and corrects failed shell commands using Ollama. It runs entirely on your machine. No data leaves, no API keys needed, no cloud dependency.

## Features

- **Privacy-first**: Runs entirely on local Ollama. No data leaves your machine.
- **Interactive TUI**: Rich terminal UI with spinner, streaming mode (Ctrl+X), and confirmation dialog.
- **Smart confirmation**: Confidence indicators (high/medium/low) with warnings before executing low-confidence fixes.
- **Editor mode**: Edit the suggested command in `$EDITOR` before running it.
- **3 shells**: Bash, Zsh, and Fish wrappers with reliable command capture via hooks.
- **No dependencies**: No jq, no temp files, no external tools. Just Go and Ollama.
- **Direct eval pattern**: Corrected command goes to stdout, shell evals it. Same proven approach as thefuck.
- **Structured output**: JSON schema enforcement with 3-tier retry for reliable LLM responses.

## Requirements

- **Go 1.22+** (for building)
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
sudo make install
```

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

This prints the right source line for your current shell.

### Manual setup

Add the wrapper to your shell config.

**Zsh** (`~/.zshrc`):
```bash
source /path/to/PFS/shell/pfs.zsh
```

**Bash** (`~/.bashrc`):
```bash
source /path/to/PFS/shell/pfs.bash
```

**Fish** (`~/.config/fish/config.fish`):
```bash
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

PFS reads the failed command, its exit code, and output. It sends them to Ollama, gets a diagnosis and fix, then shows a TUI with the corrected command. You choose what to do.

### Subcommands

| Command | Description |
|---|---|
| `pfs` | Fix the last failed command |
| `pfs explain` | Explain what went wrong (no fix, no execution) |
| `pfs config get <key>` | View a config value |
| `pfs config set <key> <value>` | Set a config value |
| `pfs setup` | Show shell integration instructions |
| `pfs --version` | Print version |
| `pfs --format json` | Machine-readable JSON output (no TUI) |

### TUI keybindings

| Key | Action |
|---|---|
| `r` | Run the corrected command |
| `e` | Edit in `$EDITOR` before running |
| `x` or `q` | Dismiss |
| `Ctrl+X` | Toggle streaming mode mid-request |
| `Ctrl+C` | Cancel |

## Configuration

Config file: `~/.config/pfs/config.yaml` (XDG standard)

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

Config keys: `ollama_base_url`, `ollama_model`, `offline_mode`, `debug_level`

If `~/.pfs.env` exists from an old installation, PFS auto-migrates it to YAML on first run.

## How It Works

1. Shell wrapper captures the command, exit code, and pipestatus via hooks (preexec/precmd)
2. `pfs` reads the captured env vars, calls Ollama with structured prompts and JSON schema
3. 3-tier retry: schema enforcement, then self-correction, then fallback
4. TUI displays the result on stderr; user confirms
5. Corrected command prints to stdout; shell evals it

## Architecture

```
cmd/                # CLI entry point (Cobra commands)
internal/config/    # YAML config, migration from legacy .pfs.env
internal/llm/       # Ollama client, prompts, JSON schema, retry logic
internal/tui/       # Bubbletea v2 TUI: spinner, streaming, confirmation
shell/              # Shell wrappers (bash, zsh, fish)
```

Communication between shell wrapper and Go binary uses env vars: `PFS_CMD`, `PFS_EXIT`, `PFS_OUTPUT`, `PFS_CWD`, `PFS_SHELL`. The corrected command goes to stdout, TUI output goes to stderr.

Exit codes: 0 success, 1 error, 2 LLM error, 3 no fix, 4 ambiguous, 5 usage.

## Contributing

PRs and issues welcome. Run `make test` before submitting.

## Feel Free to Contribute This Project!

You can help develop this app by opening a pull request or issue.
