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
│   ├── file (alias: fs)    file watcher; flags: --match, --delay, --signal, --log-filter, --tui
│   ├── manual (alias: m)   no file watching; always TUI; reload only via /reload
│   └── tick (alias: t)     re-run command on an interval, in-place repaint (like watch(1))
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

### `watch tick` flags

`tick` re-runs the command on a fixed interval and repaints the terminal in place — cursor goes to home, new output overwrites old, residue is cleared per line. No alt-screen flash. Behaves like `watch(1)` without the title bar.

| flag         | aliases    | default | meaning                |
| ------------ | ---------- | ------- | ---------------------- |
| `--interval` | `-n`, `-i` | `1s`    | interval between runs  |

```sh
# refresh every second
go-watcher watch tick -n 1s -- date

# poll docker ps in place
go-watcher watch t -- docker ps
```

### `watch manual` flags

`manual` runs the command once in the TUI and never restarts it on file change.
Use `/reload` from the TUI command bar when you want to rebuild — handy for
slow rebuilds (Docker images, codegen, etc.) where automatic reload would be
disruptive.

| flag           | aliases | default   | meaning                                                              |
| -------------- | ------- | --------- | -------------------------------------------------------------------- |
| `--log-filter` | `-f`    | (none)    | regex pre-filled into the TUI filter input at startup                |
| `--signal`     | `-s`    | `SIGKILL` | signal sent to the process on `/reload`                              |

```sh
# manually rebuild a docker image, only when you press /reload in the TUI
go-watcher watch manual -- docker build -t myimage .

# short form
go-watcher watch m -- make release
```

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

### Modes

The input box has three modes, selected by the first character you type:

| first char | mode    | what it does                                                                            |
| ---------- | ------- | --------------------------------------------------------------------------------------- |
| (any)      | filter  | live regex filter — non-matching lines are hidden                                       |
| `/`        | command | run a TUI command on `Enter` (`/reload`, `/clear`, `/quit`)                             |
| `?`        | search  | vim-style search — highlight matches, jump to them, navigate with `n`/`N`. No filtering |

### Key bindings

| key              | action                                                                                   |
| ---------------- | ---------------------------------------------------------------------------------------- |
| type text        | live filter (regex; falls back to literal match on invalid regex)                        |
| `/` (first char) | command mode (label turns orange)                                                        |
| `?` (first char) | search mode (label turns cyan); type a regex, hit `Enter` to commit                      |
| `Enter`          | run command (command mode), or commit search (search mode)                               |
| `n` / `N`        | jump to next / previous search match (only when input is empty and a search is active)   |
| `Esc`            | clear input; if input is already empty, clear active search; if no search, quit          |
| `Ctrl+C`         | quit                                                                                     |

### TUI commands

| command            | action                                                |
| ------------------ | ----------------------------------------------------- |
| `/reload`          | immediately restart the watched process (no debounce) |
| `/clear`           | clear the viewport (drops the buffered log history)   |
| `/quit` (`/exit`)  | quit the TUI                                          |

### Search

Type `?keyword` and press `Enter`. The TUI:

1. Highlights every match across all buffered lines (current match in orange, others in yellow).
2. Scrolls the viewport so the current match is centered.
3. Clears the input — now `n` and `N` move to the next / previous match.
4. New log lines arriving while a search is active are highlighted automatically.
5. `Esc` clears the active search and removes highlights.

The query accepts the same Go regex syntax as the filter; an invalid regex falls back to a literal substring search.

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
