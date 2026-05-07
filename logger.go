package main

import (
	"io"
	"os"
	"strings"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

var logger = newLogger(os.Stdout)

func newLogger(out io.Writer) zerolog.Logger {
	return log.Output(zerolog.ConsoleWriter{
		Out:             out,
		NoColor:         false,
		FormatTimestamp: func(any) string { return "" },
		FormatLevel: func(l any) string {
			if level, ok := l.(string); ok {
				switch level {
				case zerolog.LevelErrorValue, zerolog.LevelFatalValue, zerolog.LevelPanicValue:
					return sprintRGB(255, 0, 0, strings.ToUpper(level))
				}
				return sprintRGB(102, 163, 255, strings.ToUpper(level))
			}
			return ""
		},
		FormatMessage: func(f any) string {
			if msg, ok := f.(string); ok {
				return sprintRGB(140, 140, 140, msg)
			}
			return ""
		},
		FieldsOrder: []string{
			"version",
			"pid",
		},
	})
}
