package main

import (
	"context"
	"io/fs"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/fatih/color"
	"github.com/fsnotify/fsnotify"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/urfave/cli/v3"
)

var (
	ignoreRegex   *regexp.Regexp
	allowExtRegex *regexp.Regexp
	runCmd        *exec.Cmd
)

func main() {
	log.Logger = log.Output(zerolog.ConsoleWriter{
		Out:        os.Stdout,
		TimeFormat: time.Kitchen,
		NoColor:    false,
		FormatLevel: func(l any) string {
			if level, ok := l.(string); ok {
				return strings.ToUpper(level)
			}
			return ""
		},
		FormatMessage: func(f any) string {
			return color.YellowString(f.(string))
		},
	})

	rootCmd := &cli.Command{
		Version:                "0.0.1",
		EnableShellCompletion:  true,
		UseShortOptionHandling: true,
		ExitErrHandler: func(ctx context.Context, c *cli.Command, err error) {
		},
		Flags: []cli.Flag{
			&cli.Int8Flag{
				Aliases: []string{"l"},
				Name:    "log-level",
				Value:   1,
				Usage:   "Set the log level (0: DEBUG, 1: INFO)",
			},
			&cli.StringFlag{
				Aliases: []string{"i"},
				Name:    "ignore",
				Value:   "(.git|.DS_Store|.idea|.vscode|node_modules)",
				Usage:   "Set ignore regex for directories",
			},
			&cli.StringFlag{
				Aliases: []string{"e"},
				Name:    "extension",
				Value:   "^(.go|.env)$",
				Usage:   "Set allow extensions",
			},
		},
		Before: func(ctx context.Context, cmd *cli.Command) (context.Context, error) {
			level := zerolog.Level(cmd.Int8("log-level"))
			zerolog.SetGlobalLevel(level)
			log.Debug().Str("log_level", level.String()).Msg("set log level")

			regex, err := regexp.Compile(cmd.String("ignore"))
			if err != nil {
				return nil, cli.Exit("invalid ignore regex", 1)
			}
			ignoreRegex = regex
			log.Debug().Msg("compile ignore regex")

			regex, err = regexp.Compile(cmd.String("extension"))
			if err != nil {
				return nil, cli.Exit("invalid extension regex", 1)
			}
			allowExtRegex = regex
			log.Debug().Msg("compile extension regex")

			return ctx, nil
		},
		Commands: []*cli.Command{
			{
				Name: "watch",
				Before: func(ctx context.Context, cmd *cli.Command) (context.Context, error) {
					return ctx, nil
				},
				Action: func(ctx context.Context, cmd *cli.Command) error {
					args := cmd.Args()
					if args.Len() == 0 {
						return cli.Exit("no command provided to watch", 1)
					}
					log.Info().Strs("cmd", args.Slice()).Msgf("watching command")

					action := wrapAction(args.First(), args.Tail()...)
					return action(ctx, cmd)
				},
			},
		},
	}

	cmds := map[string]string{
		"run":  "go run cmd/main.go",
		"lint": "golangci-lint run --config=~/.config/nvim/linters/golangci.yaml",
		"test": "go test ./...",
	}
	for name, cmd := range cmds {
		fields := strings.Fields(cmd)
		wrapAction := wrapAction(fields[0], fields[1:]...)
		rootCmd.Commands = append(rootCmd.Commands, &cli.Command{
			Name:                  name,
			EnableShellCompletion: true,
			Action:                wrapAction,
		})
	}
	if err := rootCmd.Run(context.Background(), os.Args); err != nil {
		log.Fatal().Err(err).Msg("run application")
	}
}

func wrapAction(name string, args ...string) cli.ActionFunc {
	return func(_ context.Context, cmd *cli.Command) error {
		go reapZombieProcesses()
		go runWatcher(name, args...)

		sig := make(chan os.Signal, 1)
		defer close(sig)
		signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
		<-sig

		if runCmd != nil {
			if err := syscall.Kill(-runCmd.Process.Pid, syscall.SIGKILL); err != nil {
				log.Error().Err(err).Msg("kill process")
			}
			if err := runCmd.Wait(); err != nil {
				log.Error().Err(err).Msg("wait to finish process")
			}
		}

		return nil
	}
}

func runWatcher(name string, args ...string) {
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
	log.Debug().Str("wd", rootPath).Msg("get wd")

	if err := walkDir(rootPath, watcher); err != nil {
		log.Fatal().Err(err).Msg("walk directory")
	}

	var debouncer *time.Timer

	for {
		select {
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
			if !allowExtRegex.MatchString(ext) {
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
			debouncer = time.AfterFunc(100*time.Millisecond, func() {
				if runCmd != nil {
					if err := syscall.Kill(-runCmd.Process.Pid, syscall.SIGKILL); err != nil {
						log.Debug().Err(err).Msg("kill previous process")
					}
					log.Info().Msgf("killing (%d)", runCmd.Process.Pid)
				}
				log.Info().Msg("reloading")

				runCmd = exec.Command(name, args...)
				runCmd.Stdin = os.Stdin
				runCmd.Stdout = os.Stdout
				runCmd.Stderr = os.Stdout
				runCmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
				if err := runCmd.Start(); err != nil {
					log.Error().Err(err).Msg("start command")
				}
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

func reapZombieProcesses() {
	ch := make(chan os.Signal, 1)
	defer close(ch)
	signal.Notify(ch, syscall.SIGCHLD)

	var status syscall.WaitStatus
	for range ch {
		pid, err := syscall.Wait4(-1, &status, syscall.WNOHANG, nil)
		if err != nil {
			log.Error().Err(err).Msg("wait for child process")
			continue
		}
		log.Info().Msgf("reaped zombie process (%d)", pid)
	}
}

func walkDir(path string, watcher *fsnotify.Watcher) error {
	return filepath.WalkDir(path, func(path string, d fs.DirEntry, _ error) error {
		if !d.IsDir() || ignoreRegex.MatchString(strings.ToLower(path)) {
			return nil
		}
		log.Debug().Str("path", path).Msg("add path")
		return watcher.Add(path)
	})
}
