# UX overhaul plan — status

The UX overhaul was planned in `.sisyphus/plans/pfs-ux-overhaul.md` (a local
planning file, not in the repository). All ten plan tasks were marked done, but
the plan's own "Definition of Done" and final checklist were never ticked. This
page records what was verified on 2026-11-23 and the defects found doing so.

## Definition of Done

| Item | Status | Evidence |
|---|---|---|
| `go build ./cmd/` with zero warnings | Done | `go build ./...`, `go vet ./...` clean |
| `go test ./...` all green | Done | `go test -race ./...` (5 packages) |
| `pfs` after a failed command shows the TUI, in bash/zsh/fish | Done | `shell/e2e_test.go` drives all three shells in a pseudo-terminal with the real binary |
| Ctrl+X toggles spinner and streaming | Done | `internal/tui/streaming_test.go`, `TestStreamingUsesProvider` |
| Corrected command opens in `$EDITOR`, runs on save | Done | `TestEndToEnd_EditorInTerminal`, `internal/tui/safety_test.go` |
| `pfs config` shows and sets values | Done | `cmd/config_test.go`, `cmd/eval_test.go` |
| `pfs explain` shows the diagnosis without execution | Done | `TestExplain_ShowsDiagnosis` |
| `pfs setup` prints shell integration instructions | Fixed | It printed `/path/to/PFS` or a doubled `shell/shell` path; `cmd/setup_test.go` |
| `NO_COLOR=1` gives plain output | Done | `noColorStyles`; markdown styling is skipped |
| `~/.pfs.env` migrated automatically | Done | `TestMigration`; the notice the plan asked for was missing and is now printed |
| No temp files; direct eval | Done | Only the editor uses a 0600 temp file, as the plan specifies, removed after reading |
| Exit codes 0–5 mapped for all scenarios | Fixed | Usage (5) and ambiguous (4) were never produced; `cmd/eval_test.go` |

## Guardrails

| Guardrail | Status |
|---|---|
| G1 stdout reserved for the confirmed command | Fixed: debug output went to stdout, and non-interactive runs printed unconfirmed commands that the wrapper evaluated |
| G2 no Brave Search | Holds |
| G3 no jq | Holds |
| G4 no temp files except the editor's | Holds |
| G5 streaming uses a separate call path | Holds (`Stream: true` through `GetCorrectionStream`) |
| G6 separate fish wrapper | Holds; the fish wrapper itself was broken and is rewritten |
| G7 charm.land import paths | Holds |
| G8 exit codes | Fixed (see above) |
| G9 http:// default URL | Holds |
| G10 no new dependencies | Holds (the terminal tests use Python's standard library, no Go dependency) |
| G11 no provider interface | Holds |
| G12 no shell completions | Fixed: cobra's generated `completion` command is disabled |
| G13 one fix per invocation | Holds; alternatives are shown only when there is no single fix |
| G14 no config wizard | Holds |
| G15 no output capture yet | Holds; `PFS_OUTPUT` stays empty |

## Defects found while verifying

- bash wrapper: exit status always read as 0, wrong command recorded, broken
  chaining with an existing DEBUG trap.
- zsh wrapper: pipe status lost; `PFS_*` exported into the shell.
- fish wrapper: ran the binary when sourced, syntax error in `PFS_SHELL`,
  evaluated output of failed runs, flattened multi-line commands.
- A valid "cannot be fixed" answer crashed the retry logic (nil error).
- Cancelling or failing the editor ran the original suggestion.
- Model output could carry terminal escape sequences and hidden characters.
- The spinner drew mojibake (byte indexing of a multi-byte string).
- A partial config file lost the defaults of unset keys.
- The test suite could rename a real `~/.pfs.env` (HOME was not isolated).
