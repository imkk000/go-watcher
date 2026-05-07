package main

import (
	"context"
	"io/fs"
	"os"
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
