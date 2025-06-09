package main

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/rs/zerolog/log"
)

func runFileWatcher(ctx context.Context, c Config) {
	name, args := c.Name, c.Args
	d := c.Duration

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

	if err := walkDir(rootPath, watcher); err != nil {
		log.Fatal().Err(err).Msg("walk directory")
	}

	// run first time
	startProcess(ctx, name, args...)

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
				if err := walkDir(event.Name, watcher); err != nil {
					log.Debug().Err(err).Str("path", event.Name).Msg("add path")
					continue
				}
				log.Info().Str("path", event.Name).Msg("add path")
				continue
			}
			ext := strings.ToLower(filepath.Ext(event.Name))
			if !extensionRegex.MatchString(ext) ||
				exclusionRegex.MatchString(strings.ToLower(event.Name)) {
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
				startProcess(ctx, name, args...)
			})
		case err, ok := <-watcher.Errors:
			if !ok {
				log.Debug().Msg("channel closed")
				return
			}
			log.Error().Err(err).Msg("received error")
		}
	}
}

func walkDir(path string, watcher *fsnotify.Watcher) error {
	return filepath.WalkDir(path, func(path string, d fs.DirEntry, _ error) error {
		if !d.IsDir() {
			return nil
		}
		if !inclusionRegex.MatchString(strings.ToLower(path)) ||
			exclusionRegex.MatchString(strings.ToLower(path)) {
			return nil
		}
		log.Debug().Str("path", path).Msg("add path")
		return watcher.Add(path)
	})
}

func runCommandWatcher(ctx context.Context, c Config) {
	name, args := c.Name, c.Args
	d := c.Duration

	ticker := time.NewTicker(d)
	defer ticker.Stop()

	// run first time
	startProcess(ctx, name, args...)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if cmd != nil {
				if err := cmd.Wait(); err != nil {
					log.Error().Err(err).Msg("wait for command")
				}
			}
			startProcess(ctx, name, args...)
		}
	}
}
