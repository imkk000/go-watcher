package main

import (
	"bytes"
	"context"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/rs/zerolog/log"
)

func runFileWatcher(ctx context.Context, c Config) {
	d := c.Duration
	var rules []Rule
	if v := ctx.Value(rulesKey{}); v != nil {
		rs, ok := v.([]Rule)
		if !ok {
			log.Fatal().Msg("rules not found")
		}
		rules = rs
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Fatal().Err(err).Msg("new watcher")
	}
	defer func() {
		if err := watcher.Close(); err != nil {
			log.Error().Err(err).Msg("close watcher")
		}
	}()
	rootPath, err := os.Getwd()
	if err != nil {
		log.Fatal().Err(err).Msg("get working directory")
	}
	log.Debug().Str("dir", rootPath).Msg("get working directory")

	if err := walkDir(rootPath, watcher, rules); err != nil {
		log.Fatal().Err(err).Msg("walk directory")
	}

	// run first time
	startProcess(ctx, c)

	var debouncer *time.Timer
	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-watcher.Events:
			if !ok {
				log.Debug().Msg("channel closed")
				return
			}
			info, err := os.Stat(event.Name)
			if err != nil {
				continue
			}
			if info.IsDir() {
				if err := walkDir(event.Name, watcher, rules); err != nil {
					log.Debug().Err(err).Str("path", event.Name).Msg("add path")
					continue
				}
				continue
			}
			relPath, err := filepath.Rel(rootPath, event.Name)
			if err != nil {
				log.Debug().Err(err).Str("path", event.Name).Msg("relative path")
				continue
			}
			if !matchRules(relPath, rules) {
				continue
			}
			switch event.Op {
			case fsnotify.Create, fsnotify.Write, fsnotify.Rename:
			default:
				continue
			}
			if debouncer != nil {
				debouncer.Stop()
			}
			debouncer = time.AfterFunc(d, func() {
				startProcess(ctx, c)
			})
		case <-c.ReloadCh:
			if debouncer != nil {
				debouncer.Stop()
			}
			startProcess(ctx, c)
		case err, ok := <-watcher.Errors:
			if !ok {
				log.Debug().Msg("channel closed")
				return
			}
			log.Error().Err(err).Msg("received error")
		}
	}
}

func walkDir(rootPath string, watcher *fsnotify.Watcher, rules []Rule) error {
	return filepath.WalkDir(rootPath, func(path string, d fs.DirEntry, _ error) error {
		if !d.IsDir() {
			return nil
		}
		if rel, err := filepath.Rel(rootPath, path); err == nil && rel != "." {
			if shouldSkipDir(rel, rules) {
				return filepath.SkipDir
			}
		}
		log.Debug().Str("path", path).Msg("add path")
		return watcher.Add(path)
	})
}

// runTickWatcher repeatedly runs the command on an interval and repaints the
// terminal in place — cursor-home + clear-to-end-of-line per row + clear-below
// at the end. Same idea as unix watch(1) but without the alt-screen flash.
func runTickWatcher(ctx context.Context, c Config, interval time.Duration) {
	tickRun(ctx, c)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			tickRun(ctx, c)
		}
	}
}

func tickRun(ctx context.Context, c Config) {
	envs, err := readEnvs(ctx)
	if err != nil {
		log.Error().Err(err).Msg("read environment")
	}

	var buf bytes.Buffer
	cmd := exec.CommandContext(ctx, c.Name, c.Args...)
	cmd.Env = envs
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	_ = cmd.Run() // exit status is intentionally ignored, like watch(1)

	// per-line tail clear so a shorter new line doesn't leave residue
	out := bytes.ReplaceAll(buf.Bytes(), []byte("\n"), []byte("\x1b[K\n"))

	// move cursor home, write the frame, clear residue below
	os.Stdout.Write([]byte("\x1b[H"))
	os.Stdout.Write(out)
	os.Stdout.Write([]byte("\x1b[K\x1b[J"))
}

// runManualWatcher starts the process once and only restarts it when the TUI
// sends a ReloadCh signal (e.g. via the /reload command). No filesystem
// watching is performed.
func runManualWatcher(ctx context.Context, c Config) {
	startProcess(ctx, c)
	for {
		select {
		case <-ctx.Done():
			return
		case <-c.ReloadCh:
			startProcess(ctx, c)
		}
	}
}

// shouldSkipDir returns true when an exclude rule matches the directory itself
// or any path inside it. This avoids registering watches under dirs like
// .git/, node_modules/, etc.
func shouldSkipDir(rel string, rules []Rule) bool {
	probe := rel + "/x"
	for _, r := range rules {
		if r.Include {
			continue
		}
		if r.Match(rel) || r.Match(probe) {
			return true
		}
	}
	return false
}
