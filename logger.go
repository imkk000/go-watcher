package main

import (
	"os"
	"strings"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

var logger = log.Output(zerolog.ConsoleWriter{
	Out:             os.Stdout,
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
			return sprintRGB(255, 192, 203, msg)
		}
		return ""
	},
})
