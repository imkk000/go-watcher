package main

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParsePatterns(t *testing.T) {
	ps := parsePatterns([]string{
		"-w:.vscode",
		"+w:.git",
		"w:.DS_Store",
	})

	want := []Pattern{
		{Wildcard: true, Value: ".git"},
		{IsExclude: true, Wildcard: true, Value: ".vscode"},
		{Wildcard: true, Value: ".DS_Store"},
	}
	assert.Equal(t, want, ps)
}

func TestJoinPipe(t *testing.T) {
	s := joinPipe([]string{".git", ".vscode", ".DS_Store"})

	assert.Equal(t, ".git|.vscode|.DS_Store", s)
}

func TestPatternMatch(t *testing.T) {
	tcs := []struct {
		want bool
		p    Pattern
		s    string
	}{
		{
			want: false,
			p:    Pattern{Wildcard: true, Value: "!(script)"},
			s:    "script",
		},
		{
			want: true,
			p:    Pattern{Wildcard: true, Value: "**/gen/**"},
			s:    "gen",
		},
		{
			want: false,
			p:    Pattern{Regex: regexp.MustCompile(`.+\.go$`)},
			s:    "a/b/c/d/f/g/h/test.go.bak",
		},
		{
			want: true,
			p:    Pattern{Regex: regexp.MustCompile(`.+\.go$`)},
			s:    "a/b/c/d/f/g/h/test.go",
		},
		{
			want: true,
			p:    Pattern{Wildcard: true, Value: "**/.git/**"},
			s:    ".git",
		},
		{
			want: true,
			p:    Pattern{Wildcard: true, Value: ".git"},
			s:    ".git",
		},
		{
			want: true,
			p:    Pattern{Wildcard: true, Value: "src/**/*.js"},
			s:    "src/a/b/background.js",
		},
		{
			want: true,
			p:    Pattern{Wildcard: true, Value: "**/gen*/**"},
			s:    "generate/client/main.go",
		},
		{
			want: true,
			p:    Pattern{Wildcard: true, Value: "script/**"},
			s:    "script/client/main.go",
		},
		{
			want: true,
			p:    Pattern{Wildcard: true, Value: "script/**"},
			s:    "script/main.go",
		},
		{
			want: false,
			p:    Pattern{Wildcard: true, Value: "*.go"},
			s:    "main.go.bak",
		},
		{
			want: true,
			p:    Pattern{Wildcard: true, Value: "**/*.go"},
			s:    "cmd/main.go",
		},
		{
			want: true,
			p:    Pattern{Wildcard: true, Value: "*.go"},
			s:    "main.go",
		},
		{
			want: true,
			p:    Pattern{Wildcard: true, Value: "**/.git"},
			s:    "path/to/.git",
		},
		{
			want: true,
			p:    Pattern{Exact: true, Value: ".git"},
			s:    ".git",
		},
	}
	for _, tc := range tcs {
		t.Run(tc.s, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, tc.p.Match(tc.s))
		})
	}
}

func TestParsePattern(t *testing.T) {
	tcs := []struct {
		want Pattern
		s    string
	}{
		{
			want: Pattern{Wildcard: true, Value: ".git"},
			s:    `+w:.git`,
		},
		{
			want: Pattern{IsExclude: true, Wildcard: true, Value: ".git"},
			s:    `-w:.git`,
		},
		{
			want: Pattern{Wildcard: true, Value: ".git"},
			s:    `w:.git`,
		},
		{
			want: Pattern{Exact: true, Value: ".git"},
			s:    `e:.git`,
		},
		{
			want: Pattern{Regex: regexp.MustCompile(`\.git`)},
			s:    `r:\.git`,
		},
	}
	for _, tc := range tcs {
		t.Run(tc.s, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, parsePattern(tc.s))
		})
	}
}
