# go-watcher

A small file-watcher that re-runs a command whenever matching files change.
Built for Go projects but works for anything. Extended features by Claude Code.

---

## Install

```sh
git clone https://github.com/imkk000/go-watcher
cd go-watcher
go build -o go-watcher .
mv go-watcher ~/.bin/   # or anywhere on $PATH
```

Optional: link it as a `hack-core` module under the name `watcher`.

```sh
ln -s ~/.bin/go-watcher ~/.bin/hack-watcher
```

`go-watcher completion json` is implemented for `hack-core` to consume.

---

## Quick start

Re-run `go run .` whenever a `.go` file changes:

```sh
go-watcher watch file -- go run .
```

Same, but also load env vars from `./.env` first:

```sh
go-watcher watch --env=. file -- go run .
```

Anything after `--` is the command to run (and re-run on changes).

---

## Command structure

```
go-watcher
├── watch                   shared flags: --log-level, --env
│   └── file (alias: fs)    file watcher; flags: --match, --delay, --signal, --log-filter, --tui
└── completion json         dump CLI structure as JSON (for hack-core)
```

### `watch` flags

| flag          | default | meaning                                                                                                 |
| ------------- | ------- | ------------------------------------------------------------------------------------------------------- |
| `--log-level` | `info`  | zerolog level: `trace` / `debug` / `info` / `warn` / `error`                                            |
| `--env`       | `off`   | env file(s) to load before each run. Pass `--env=.` to mean `./.env`, or any explicit path. Repeatable. |

### `watch file` flags

| flag           | aliases    | default         | meaning                                                            |
| -------------- | ---------- | --------------- | ------------------------------------------------------------------ |
| `--match`      | `-m`       | (defaults only) | match rule, see below. Repeatable.                                 |
| `--delay`      | `-n`, `-d` | `500ms`         | debounce window — multiple events collapse into one restart        |
| `--signal`     | `-s`       | `SIGKILL`       | signal sent to the process on each reload                          |
| `--log-filter` | `-f`       | (none)          | regex to filter subprocess output; only matching lines are printed |
| `--tui`        |            | `false`         | launch the interactive TUI with a live filter search box           |

---

## Match rules

A rule is `[+-]<value>` where the prefix sets polarity:

- `+` — **include** (watch files matched by this rule)
- `-` — **exclude** (do not watch files matched by this rule)
- no prefix is treated as `+`

`<value>` is one of:

1. **Built-in name** — e.g. `go`, `git`, `node-modules`. Reuses a built-in pattern; the user-supplied polarity wins.
2. **Glob** — anything else. Uses [doublestar](https://github.com/bmatcuk/doublestar) syntax.
   - `*.go`, `**/*.go`, `src/**/*.js`, `**/build/**`

### Built-in defaults

These are always applied unless you override them by name.

| name           | pattern              | default polarity |
| -------------- | -------------------- | ---------------- |
| `go`           | `**/*.go`            | include          |
| `mod`          | `**/go.mod`          | include          |
| `env`          | `*.env`              | include          |
| `git`          | `**/.git/**`         | exclude          |
| `vscode`       | `**/.vscode/**`      | exclude          |
| `idea`         | `**/.idea/**`        | exclude          |
| `ds-store`     | `**/.DS_Store/**`    | exclude          |
| `node-modules` | `**/node_modules/**` | exclude          |
| `script`       | `**/script/**`       | exclude          |

### How user rules combine with defaults

- Defaults are **always** in the rule set.
- A user rule whose value matches a **built-in name** replaces that built-in in place — its pattern is reused, only the polarity flips.
- Any other user rule is **added on top** of the defaults with higher priority.

This means `--match=-go` flips the `go` rule from include → exclude without removing any other defaults.

### Match semantics

For each changed file, every rule is checked:

1. If **any exclude rule** matches → the file is skipped.
2. Otherwise, if **any include rule** matches → the file triggers a restart.
3. Otherwise → the file is ignored.

Excludes always win. So `.git/main.go` is ignored even though it matches `+go`, because `-git` (built-in) also matches.

The same logic also prunes the watch tree: any directory matched by an exclude rule is skipped during the initial walk and never registered with the OS watcher.

---

## Process signal

By default the watched process is killed with `SIGKILL` on each reload. Use `--signal` (or `-s`) to change it.

Supported values (with or without the `SIG` prefix, case-insensitive):

| value              | signal                     |
| ------------------ | -------------------------- |
| `SIGKILL` / `KILL` | force kill (default)       |
| `SIGTERM` / `TERM` | graceful termination       |
| `SIGHUP` / `HUP`   | hangup (reload convention) |
| `SIGINT` / `INT`   | interrupt (like Ctrl-C)    |

```sh
# graceful shutdown on each reload
go-watcher watch file -s SIGTERM -- go run .
```

---

## Log filter

`--log-filter` (`-f`) takes a regex and hides any subprocess output line that does not match. Useful for noisy processes when you only care about errors.

```sh
go-watcher watch file -f 'ERROR|WARN' -- go run .
```

---

## TUI mode

`--tui` launches an interactive full-screen view:

```
┌──────────────────────────────────────────┐
│  INFO  watching command                  │
│  INFO  started (12345)                   │
│  listening on :8080                      │
│  GET /api/users 200                      │
│── reload ──                              │
│  INFO  started (12350)                   │
│  listening on :8080                      │
│                                          │
├──────────────────────────────────────────┤
│ filter: ERROR▌                           │
└──────────────────────────────────────────┘
```

- The viewport auto-scrolls when you are at the bottom; scroll up to freeze it.
- All log lines are buffered — clearing the filter restores the full history.
- `--log-filter` pre-populates the filter input at startup.
- Watcher messages (reload, started, etc.) appear in the viewport alongside subprocess output.

### Key bindings

| key              | action                                                                      |
| ---------------- | --------------------------------------------------------------------------- |
| type text        | filter log lines live (regex; falls back to literal match on invalid regex) |
| `/` (first char) | switch to command mode                                                      |
| `Enter`          | execute command (command mode only)                                         |
| `Esc`            | cancel command and return to filter mode; or quit if already in filter mode |
| `Ctrl+C`         | quit                                                                        |

### TUI commands

Type `/` to enter command mode — the label turns orange and the viewport shows all lines unfiltered while you type.

| command   | action                                                |
| --------- | ----------------------------------------------------- |
| `/reload` | immediately restart the watched process (no debounce) |

Press `Enter` to run the command. Press `Esc` to cancel without running it.

```sh
# open TUI, pre-filter to ERROR lines
go-watcher watch file --tui -f 'ERROR' -- go run .
```

---

## Examples

Watch Go files only (default behaviour):

```sh
go-watcher watch file -- go run .
```

Add proto files; ignore generated output:

```sh
go-watcher watch file -m '+*.proto' -m '-**/gen/**' -- buf generate
```

Stop watching `.go` files (rare, but supported):

```sh
go-watcher watch file -m '-go' -- ./run.sh
```

Re-enable a built-in that's excluded by default — watch inside `script/`:

```sh
go-watcher watch file -m '+script' -- ./build.sh
```

Watch SQL migrations:

```sh
go-watcher watch file -m '+migrations/**/*.sql' -- make migrate
```

Load multiple env files and turn on debug logs:

```sh
go-watcher watch --log-level=debug --env=.env --env=.env.local file -- go run .
```

Slow down the debounce (useful when `go build` triggers cascading writes):

```sh
go-watcher watch file --delay=2s -- go run .
```

TUI with SIGTERM and a pre-set filter:

```sh
go-watcher watch file --tui -s SIGTERM -f 'ERROR|WARN' -- go run .
```

---

## Process behaviour

- The command runs in its **own process group** (`Setpgid`).
- On restart or shutdown, the whole group is sent the configured signal (default `SIGKILL`).
- A `SIGCHLD` handler reaps zombie children left by orphaned grandchildren.
- `Ctrl-C` on the watcher terminates the child group and exits.

---

## Troubleshooting

- **Nothing triggers when I save a file** — run with `--log-level=debug` and look at the `merged rules` line. If your file isn't matched by any include rule, add one with `-m '+<glob>'`.
- **Too many restarts on one save** — your editor probably writes via temp-file + rename, which fires multiple events. Increase `--delay`.
- **`too many open files` / inotify limit** — exclude noisy directories with `-m '-**/big-dir/**'` so they aren't added to the watcher in the first place.
