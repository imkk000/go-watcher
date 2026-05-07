package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseRule(t *testing.T) {
	tcs := []struct {
		name string
		in   string
		want Rule
	}{
		{
			name: "plus glob",
			in:   "+*.go",
			want: Rule{Include: true, Glob: "*.go"},
		},
		{
			name: "minus glob",
			in:   "-**/build/**",
			want: Rule{Include: false, Glob: "**/build/**"},
		},
		{
			name: "default polarity is include",
			in:   "*.go",
			want: Rule{Include: true, Glob: "*.go"},
		},
		{
			name: "builtin name include is default",
			in:   "+go",
			want: Rule{Name: "go", Include: true, Glob: "**/*.go"},
		},
		{
			name: "builtin name override polarity",
			in:   "-go",
			want: Rule{Name: "go", Include: false, Glob: "**/*.go"},
		},
		{
			name: "builtin git default exclude",
			in:   "-git",
			want: Rule{Name: "git", Include: false, Glob: "**/.git/**"},
		},
		{
			name: "builtin git flipped to include",
			in:   "+git",
			want: Rule{Name: "git", Include: true, Glob: "**/.git/**"},
		},
	}
	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := parseRule(tc.in)
			assert.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestParseRuleErrors(t *testing.T) {
	cases := []string{"", "+", "-"}
	for _, in := range cases {
		t.Run(in, func(t *testing.T) {
			t.Parallel()

			_, err := parseRule(in)
			assert.Error(t, err)
		})
	}
}

func TestRuleMatch(t *testing.T) {
	tcs := []struct {
		name string
		want bool
		r    Rule
		s    string
	}{
		{name: "glob *.go matches main.go", want: true, r: Rule{Glob: "*.go"}, s: "main.go"},
		{name: "glob *.go does not match .go.bak", want: false, r: Rule{Glob: "*.go"}, s: "main.go.bak"},
		{name: "glob **/*.go matches nested", want: true, r: Rule{Glob: "**/*.go"}, s: "cmd/main.go"},
		{name: "glob **/*.go matches flat", want: true, r: Rule{Glob: "**/*.go"}, s: "main.go"},
		{name: "glob src/**/*.js matches deep js", want: true, r: Rule{Glob: "src/**/*.js"}, s: "src/a/b/c.js"},
		{name: "glob **/.git/** matches inside", want: true, r: Rule{Glob: "**/.git/**"}, s: ".git/HEAD"},
	}
	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, tc.r.Match(tc.s))
		})
	}
}

func TestMergeRulesNoUserPreservesDefaults(t *testing.T) {
	rules := mergeRules(nil)
	assert.Equal(t, len(builtinRules), len(rules))
}

func TestMergeRulesNamedOverrideFlipsInPlace(t *testing.T) {
	flip, err := parseRule("-go")
	assert.NoError(t, err)

	rules := mergeRules([]Rule{flip})
	assert.Equal(t, len(builtinRules), len(rules), "user override must not change rule count")

	var found bool
	for _, r := range rules {
		if r.Name == "go" {
			found = true
			assert.False(t, r.Include, "go must now be exclude")
		}
	}
	assert.True(t, found, "go rule must still be present")
}

func TestMergeRulesUnnamedAppended(t *testing.T) {
	r, err := parseRule("+*.proto")
	assert.NoError(t, err)

	rules := mergeRules([]Rule{r})
	assert.Equal(t, len(builtinRules)+1, len(rules))

	var found bool
	for _, rule := range rules {
		if rule.Glob == "*.proto" && rule.Include {
			found = true
		}
	}
	assert.True(t, found)
}

func TestMatchRulesExcludeWins(t *testing.T) {
	rules := mergeRules(nil)
	// .git/HEAD is matched by both no include rule and the git exclude
	assert.False(t, matchRules(".git/HEAD", rules))
	// main.go is matched by go include and no exclude
	assert.True(t, matchRules("main.go", rules))
	// main.go inside .git is excluded because git exclude wins over go include
	assert.False(t, matchRules(".git/main.go", rules))
}

func TestMatchRulesUserFlipExcludesGo(t *testing.T) {
	flip, err := parseRule("-go")
	assert.NoError(t, err)

	rules := mergeRules([]Rule{flip})
	assert.False(t, matchRules("main.go", rules))
}
