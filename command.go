package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"github.com/imkk000/hack-my-laziness/pkg/walkcmd"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/urfave/cli/v3"
)

type Config struct {
	Name      string
	Args      []string
	Duration  time.Duration
	LogFilter *regexp.Regexp
	LineCh    chan string   // non-nil when TUI mode is active
	ReloadCh  chan struct{} // non-nil when TUI mode is active
}

var fileCmd = &cli.Command{
	Aliases: []string{"fs"},
	Name:    "file",
	Flags: []cli.Flag{
		&cli.StringSliceFlag{
			Aliases: []string{"m"},
			Name:    "match",
			Usage:   "rule [+-]<name|glob>; built-in names: go, mod, env, git, vscode, idea, ds-store, node-modules, script",
		},
		&cli.DurationFlag{
			Aliases: []string{"n", "d"},
			Name:    "delay",
			Value:   500 * time.Millisecond,
			Usage:   "set debounce delay before restart",
		},
		&cli.StringFlag{
			Aliases: []string{"f"},
			Name:    "log-filter",
			Usage:   "regex to filter subprocess log lines (only matching lines are shown)",
		},
		&cli.BoolFlag{
			Name:  "tui",
			Usage: "interactive TUI with live filter search box",
		},
		&cli.StringFlag{
			Aliases: []string{"s"},
			Name:    "signal",
			Value:   "SIGKILL",
			Usage:   "signal sent to process on reload: SIGKILL, SIGTERM, SIGHUP, SIGINT",
		},
	},
	Before: func(ctx context.Context, c *cli.Command) (context.Context, error) {
		userRules, err := parseRules(c.StringSlice("match"))
		if err != nil {
			return nil, cli.Exit(err.Error(), 1)
		}
		rules := mergeRules(userRules)
		ctx = context.WithValue(ctx, rulesKey{}, rules)
		log.Debug().Interface("rules", rules).Msg("merged rules")

		return validateArgs(ctx, c)
	},
	Action: func(ctx context.Context, c *cli.Command) error {
		args := c.Args()
		d := c.Duration("delay")
		filterPattern := c.String("log-filter")

		sig, err := parseSignal(c.String("signal"))
		if err != nil {
			return cli.Exit(err.Error(), 1)
		}
		killSig = sig

		var logFilter *regexp.Regexp
		if filterPattern != "" && !c.Bool("tui") {
			var err error
			logFilter, err = regexp.Compile(filterPattern)
			if err != nil {
				return cli.Exit(fmt.Sprintf("invalid log-filter: %v", err), 1)
			}
		}

		var (
			lineCh   chan string
			reloadCh chan struct{}
		)
		if c.Bool("tui") {
			lineCh = make(chan string, 512)
			reloadCh = make(chan struct{}, 1)
			// redirect watcher log output into the TUI viewport
			log.Logger = newLogger(&tuiLogWriter{ch: lineCh})
		}

		log.Info().
			Str("version", appVersion).
			Int("pid", os.Getpid()).
			Strs("command", args.Slice()).
			Msgf("watching command")

		cfg := Config{
			Duration:  d,
			Name:      args.First(),
			Args:      args.Tail(),
			LogFilter: logFilter,
			LineCh:    lineCh,
			ReloadCh:  reloadCh,
		}

		go runFileWatcher(ctx, cfg)

		if lineCh != nil {
			runTUI(ctx, lineCh, reloadCh, filterPattern)
		} else {
			killSignal(ctx)
		}

		return nil
	},
}

var tickCmd = &cli.Command{
	Aliases: []string{"t"},
	Name:    "tick",
	Usage:   "re-run command on an interval, repainting in place (like watch(1))",
	Flags: []cli.Flag{
		&cli.DurationFlag{
			Aliases: []string{"n", "i"},
			Name:    "interval",
			Value:   time.Second,
			Usage:   "interval between runs",
		},
	},
	Before: validateArgs,
	Action: func(ctx context.Context, c *cli.Command) error {
		args := c.Args()
		interval := c.Duration("interval")

		log.Info().
			Str("version", appVersion).
			Int("pid", os.Getpid()).
			Dur("interval", interval).
			Strs("command", args.Slice()).
			Msgf("tick mode")

		cfg := Config{
			Name: args.First(),
			Args: args.Tail(),
		}

		go runTickWatcher(ctx, cfg, interval)
		killSignal(ctx)

		return nil
	},
}

var manualCmd = &cli.Command{
	Aliases: []string{"m"},
	Name:    "manual",
	Usage:   "run command in TUI without file watching; reload only via /reload",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Aliases: []string{"f"},
			Name:    "log-filter",
			Usage:   "regex pre-filled into the TUI filter input at startup",
		},
		&cli.StringFlag{
			Aliases: []string{"s"},
			Name:    "signal",
			Value:   "SIGKILL",
			Usage:   "signal sent to process on reload: SIGKILL, SIGTERM, SIGHUP, SIGINT",
		},
	},
	Before: validateArgs,
	Action: func(ctx context.Context, c *cli.Command) error {
		args := c.Args()
		filterPattern := c.String("log-filter")

		sig, err := parseSignal(c.String("signal"))
		if err != nil {
			return cli.Exit(err.Error(), 1)
		}
		killSig = sig

		lineCh := make(chan string, 512)
		reloadCh := make(chan struct{}, 1)
		log.Logger = newLogger(&tuiLogWriter{ch: lineCh})

		log.Info().
			Str("version", appVersion).
			Int("pid", os.Getpid()).
			Strs("command", args.Slice()).
			Msgf("manual mode (reload via /reload)")

		cfg := Config{
			Name:     args.First(),
			Args:     args.Tail(),
			LineCh:   lineCh,
			ReloadCh: reloadCh,
		}

		go runManualWatcher(ctx, cfg)
		runTUI(ctx, lineCh, reloadCh, filterPattern)

		return nil
	},
}

var rootCmd = &cli.Command{
	Version:                  appVersion,
	EnableShellCompletion:    false,
	UseShortOptionHandling:   true,
	Suggest:                  true,
	ExitErrHandler:           func(_ context.Context, _ *cli.Command, _ error) {},
	CommandNotFound:          func(context.Context, *cli.Command, string) {},
	OnUsageError:             func(_ context.Context, _ *cli.Command, _ error, _ bool) error { return nil },
	InvalidFlagAccessHandler: func(context.Context, *cli.Command, string) {},
	Commands: []*cli.Command{
		// support hack-core
		{
			Name:  "completion",
			Usage: "Get shell completion",
			Commands: []*cli.Command{
				{
					Name:  "json",
					Usage: "Get completion in JSON format",
					Action: func(_ context.Context, c *cli.Command) error {
						info := walkcmd.Walk(c.Root())
						data, err := json.Marshal(info)
						if err != nil {
							return err
						}
						fmt.Println(string(data))

						return nil
					},
				},
			},
		},
		{
			Name: "watch",
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:  "log-level",
					Value: "info",
					Usage: "set the log level",
				},
				&cli.StringSliceFlag{
					Name:  "env",
					Value: []string{"off"},
					Usage: "set env files",
				},
			},
			Before: func(ctx context.Context, c *cli.Command) (context.Context, error) {
				level, err := zerolog.ParseLevel(c.String("log-level"))
				if err != nil {
					return nil, cli.Exit(err, 1)
				}
				zerolog.SetGlobalLevel(level)
				log.Debug().
					Str("log_level", level.String()).
					Msg("set log level")

				envFiles := getEnvFiles(c.StringSlice("env"))

				return context.WithValue(ctx, envFilesKey{}, envFiles), nil
			},
			Commands: []*cli.Command{fileCmd, manualCmd, tickCmd},
		},
	},
}

func getEnvFiles(files []string) []string {
	out := make([]string, 0, len(files))
	for _, file := range files {
		if file == "off" {
			return nil
		}
		dir := filepath.Dir(file)
		base := filepath.Base(file)
		if base == "." {
			file = filepath.Join(dir, ".env")
		}
		out = append(out, file)
	}
	return out
}

func validateArgs(ctx context.Context, c *cli.Command) (context.Context, error) {
	args := c.Args()
	if args.Len() == 0 {
		return nil, cli.Exit("no command provided to watch", 1)
	}
	return ctx, nil
}
