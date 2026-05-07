# go-watcher — CLAUDE.md

## Build & test

```sh
go build ./...
go test ./...
go vet ./...
```

No code generation step. All files are plain Go — edit and build directly.

## Architecture

Single `main` package. All `.go` files at the root compile into one binary.

| file          | responsibility                                                              |
| ------------- | --------------------------------------------------------------------------- |
| `main.go`     | entry point; sets up context, deferred kill, zombie reaper                  |
| `command.go`  | CLI definition (`rootCmd`, `watch`, `file`); `Config` struct                |
| `watcher.go`  | `runFileWatcher` — fsnotify event loop; calls `startProcess` on match       |
| `process.go`  | `startProcess`, `killProcess`, `parseSignal`, `filteredWriter`, `tuiWriter` |
| `strings.go`  | `Rule`, `matchRules`, `parseRule`, `mergeRules`, builtin rule table         |
| `variable.go` | package-level globals: `appVersion`, `cmd`, `killSig`                       |
| `logger.go`   | `newLogger(out io.Writer)` — zerolog console writer factory                 |
| `color.go`    | `ColoredWriter`, `rgb`, `sprintRGB` ANSI helpers                            |
| `tui.go`      | bubbletea TUI: `tuiModel`, `tuiWriter`, `tuiLogWriter`, `runTUI`            |
| `walkcmd/`    | sub-package; walks a `cli.Command` tree into JSON for hack-core             |

## Config and data flow

`Config` (defined in `command.go`) carries everything from the CLI flags into the watcher:

```
Config
  Name / Args    — command to run
  Duration       — debounce window
  LogFilter      — *regexp.Regexp, nil = no filter (non-TUI only)
  LineCh         — chan string, non-nil in TUI mode; subprocess output → TUI
  ReloadCh       — chan struct{}, non-nil in TUI mode; TUI /reload → watcher
```

`killSig` in `variable.go` is a package-level `syscall.Signal` set from `--signal` before the watcher starts. `killProcess()` always reads it from there.

## Key design decisions

**Rule matching** — glob-only (doublestar). Regex was removed in 0.4.0. Built-in `go` rule uses `**/*.go` (matches both flat and nested files).

**Exclude wins** — if any exclude rule matches a path, the file is always skipped, even if an include rule also matches.

**Directory pruning** — `shouldSkipDir` probes each directory against exclude rules before adding it to the fsnotify watcher, so `node_modules/`, `.git/`, etc. are never watched at the OS level.

**Process group** — `Setpgid: true` so `syscall.Kill(-pid, sig)` kills the whole child tree, not just the direct child.

**TUI / non-TUI split** — `LineCh == nil` means non-TUI path (direct stdout via `ColoredWriter` ± `filteredWriter`). `LineCh != nil` means TUI path (raw lines → channel → viewport). The two paths are selected inside `startProcess`.

**Nil channel in select** — `watcher.go` always selects on `c.ReloadCh`; when it is nil (non-TUI), Go's scheduler never picks that case, so no special-casing is needed.

**TUI command mode** — input starting with `/` switches to command mode. Viewport shows unfiltered content while in command mode. `Enter` executes, `Esc` cancels back to filter mode.

## Adding a new TUI command

1. Add a `case "yourcommand":` branch in `execCommand()` in `tui.go`.
2. If it needs to signal the watcher, add a new channel to `Config` (follow the `ReloadCh` pattern).
3. Add a `case <-c.YourCh:` to the select in `runFileWatcher` in `watcher.go`.
4. Update the TUI commands table in `README.md`.

## Signal support

Only four signals are supported by `parseSignal` in `process.go`: `SIGKILL`, `SIGTERM`, `SIGHUP`, `SIGINT`. Adding more means a new `case` in that switch — no other change needed.

## Version

`appVersion` in `variable.go`. Bump it there only.

## Dependencies

- `fsnotify` — OS file-system events
- `doublestar/v4` — glob matching with `**` support
- `urfave/cli/v3` — CLI framework
- `rs/zerolog` — structured logging
- `joho/godotenv` — `.env` file loading
- `charmbracelet/bubbletea` + `bubbles` + `lipgloss` — TUI framework
- `stretchr/testify` — test assertions (test-only)
