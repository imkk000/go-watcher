package main

import (
	"regexp"
	"slices"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
)

func joinPipe(v []string) (s string) {
	s = strings.Join(v, "|")
	s = strings.ReplaceAll(s, ",", "|")
	return
}

func parsePatterns(patterns []string) (result []Pattern) {
	slices.SortFunc(patterns, func(a, b string) int {
		if a == "+" || b == "+" {
			return 0
		}
		if a == "-" || b == "-" {
			return 1
		}
		return strings.Compare(a, b)
	})
	for _, pattern := range patterns {
		result = append(result, parsePattern(pattern))
	}
	return
}

func parsePattern(pattern string) (pat Pattern) {
	// - for exclude patterns
	// r: regexp
	// e: exact match
	// w: wildcard match
	// [+-][rew]:<pattern>
	pat = Pattern{
		IsExclude: true,
	}
	if pattern == "" {
		return
	}
	if pattern[0] == '+' {
		pat.IsExclude = false
		pattern = pattern[1:]
	}
	if pattern[0] == '-' {
		pattern = pattern[1:]
	} else {
		pat.IsExclude = false
	}
	switch pattern[0] {
	case 'r':
		pat.Regex = regexp.MustCompile(pattern[2:])
	case 'e':
		pat.Exact = true
		pat.Value = pattern[2:]
	case 'w':
		pat.Wildcard = true
		pat.Value = pattern[2:]
	}
	return
}

type Pattern struct {
	IsExclude bool
	Exact     bool
	Wildcard  bool
	Regex     *regexp.Regexp
	Value     string
}

func (p Pattern) Match(s string) bool {
	if p.Exact {
		return p.Value == s
	}
	if p.Wildcard {
		valid, err := doublestar.Match(p.Value, s)
		if err != nil {
			return false
		}
		return valid
	}
	if p.Regex != nil {
		return p.Regex.MatchString(s)
	}
	return false
}

func matchPatterns(s string, patterns []Pattern) (bool, bool) {
	for _, p := range patterns {
		if ok := p.Match(s); ok {
			return true, p.IsExclude
		}
	}
	return false, true
}
