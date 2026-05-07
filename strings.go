package main

import (
	"errors"
	"fmt"

	"github.com/bmatcuk/doublestar/v4"
)

// Rule decides whether a relative path should be watched.
// Match is by Glob (doublestar) only.
// Include=true means watch on match; false means skip on match.
// Name is set for built-in rules and lets users override polarity by name.
type Rule struct {
	Name    string
	Include bool
	Glob    string
}

func (r Rule) Match(s string) bool {
	ok, err := doublestar.Match(r.Glob, s)
	if err != nil {
		return false
	}
	return ok
}

// builtinRules are the defaults. A user --match=<name> with the same Name
// replaces the built-in (its polarity is taken from the user input).
var builtinRules = []Rule{
	{Name: "go", Include: true, Glob: "**/*.go"},
	{Name: "mod", Include: true, Glob: "**/go.mod"},
	{Name: "env", Include: true, Glob: "*.env"},
	{Name: "git", Include: false, Glob: "**/.git/**"},
	{Name: "vscode", Include: false, Glob: "**/.vscode/**"},
	{Name: "idea", Include: false, Glob: "**/.idea/**"},
	{Name: "ds-store", Include: false, Glob: "**/.DS_Store/**"},
	{Name: "node-modules", Include: false, Glob: "**/node_modules/**"},
	{Name: "script", Include: false, Glob: "**/script/**"},
}

func builtinByName(name string) (Rule, bool) {
	for _, r := range builtinRules {
		if r.Name == name {
			return r, true
		}
	}
	return Rule{}, false
}

// parseRule reads one --match value: [+-]<name|glob>.
// No prefix means +. A bare value matching a built-in name reuses that
// rule's pattern with the user's polarity. Anything else is treated as a glob.
func parseRule(s string) (Rule, error) {
	if s == "" {
		return Rule{}, errors.New("empty rule")
	}
	include := true
	switch s[0] {
	case '+':
		s = s[1:]
	case '-':
		include = false
		s = s[1:]
	}
	if s == "" {
		return Rule{}, errors.New("rule has no value")
	}
	if r, ok := builtinByName(s); ok {
		r.Include = include
		return r, nil
	}
	return Rule{Include: include, Glob: s}, nil
}

// parseRules parses each user input. Errors are returned for the first bad rule.
func parseRules(inputs []string) ([]Rule, error) {
	out := make([]Rule, 0, len(inputs))
	for _, s := range inputs {
		r, err := parseRule(s)
		if err != nil {
			return nil, fmt.Errorf("rule %q: %w", s, err)
		}
		out = append(out, r)
	}
	return out, nil
}

// mergeRules returns the effective rule set: built-in defaults with user
// rules applied. User rules whose Name matches a built-in REPLACE the built-in
// in place (so polarity flips work). Other user rules are prepended so they
// take priority over defaults.
func mergeRules(userRules []Rule) []Rule {
	defaults := make([]Rule, len(builtinRules))
	copy(defaults, builtinRules)

	var extras []Rule
	for _, ur := range userRules {
		if ur.Name != "" {
			replaced := false
			for i, r := range defaults {
				if r.Name == ur.Name {
					defaults[i] = ur
					replaced = true
					break
				}
			}
			if replaced {
				continue
			}
		}
		extras = append(extras, ur)
	}
	return append(extras, defaults...)
}

// matchRules decides whether s should be watched. Exclude wins on any match;
// otherwise an include match watches; no match means skip.
func matchRules(s string, rules []Rule) bool {
	included := false
	for _, r := range rules {
		if !r.Match(s) {
			continue
		}
		if !r.Include {
			return false
		}
		included = true
	}
	return included
}
