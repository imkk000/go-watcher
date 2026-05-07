package main

import (
	"bytes"
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/rs/zerolog/log"
)

// reloadMark is a sentinel sent to the TUI channel when the process restarts.
const reloadMark = "\x01RELOAD\x01"

var (
	reloadLineStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("208"))
	filterBarStyle     = lipgloss.NewStyle().BorderStyle(lipgloss.NormalBorder()).BorderTop(true).BorderForeground(lipgloss.Color("240")).Padding(0, 1)
	filterLabelStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	cmdLabelStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Bold(true)
	searchLabelStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("117")).Bold(true)
	errorStyle         = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	matchStyle         = lipgloss.NewStyle().Background(lipgloss.Color("226")).Foreground(lipgloss.Color("0"))
	currentMatchStyle  = lipgloss.NewStyle().Background(lipgloss.Color("208")).Foreground(lipgloss.Color("0")).Bold(true)
	searchStatusStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("117"))
)

type newLineMsg struct{ line string }

type matchPos struct {
	lineIdx int
	start   int
	end     int
}

type tuiModel struct {
	lines         []string
	lineCh        <-chan string
	reloadCh      chan<- struct{}
	input         textinput.Model
	viewport      viewport.Model
	width         int
	height        int
	ready         bool
	cmdErr        string
	activeSearch  string
	searchMatches []matchPos
	searchIdx     int
}

func newTUIModel(lineCh <-chan string, reloadCh chan<- struct{}, initialFilter string) tuiModel {
	ti := textinput.New()
	ti.Placeholder = "filter… (/cmd, ?search)"
	ti.SetValue(initialFilter)
	ti.Focus()
	ti.CharLimit = 300

	return tuiModel{
		lineCh:   lineCh,
		reloadCh: reloadCh,
		input:    ti,
	}
}

func (m tuiModel) isCommandMode() bool {
	return strings.HasPrefix(m.input.Value(), "/")
}

func (m tuiModel) isSearchMode() bool {
	return strings.HasPrefix(m.input.Value(), "?")
}

// currentSearchQuery returns the live query while typing in search mode,
// otherwise the committed activeSearch (after Enter).
func (m tuiModel) currentSearchQuery() string {
	if m.isSearchMode() {
		return strings.TrimPrefix(m.input.Value(), "?")
	}
	return m.activeSearch
}

func waitForLine(ch <-chan string) tea.Cmd {
	return func() tea.Msg {
		return newLineMsg{line: <-ch}
	}
}

func (m tuiModel) Init() tea.Cmd {
	return tea.Batch(textinput.Blink, waitForLine(m.lineCh))
}

func (m tuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var (
		tiCmd tea.Cmd
		vpCmd tea.Cmd
	)

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit

		case "esc":
			if m.input.Value() != "" {
				m.input.SetValue("")
				m.cmdErr = ""
				m.recomputeMatches()
				m.refreshContent()
				return m, nil
			}
			if m.activeSearch != "" {
				m.activeSearch = ""
				m.searchMatches = nil
				m.searchIdx = 0
				m.refreshContent()
				return m, nil
			}
			return m, tea.Quit

		case "enter":
			if m.isCommandMode() {
				m, tiCmd = m.execCommand()
				return m, tiCmd
			}
			if m.isSearchMode() {
				m.activeSearch = strings.TrimPrefix(m.input.Value(), "?")
				m.input.SetValue("")
				m.recomputeMatches()
				m.searchIdx = 0
				m.jumpToCurrentMatch()
				m.refreshContent()
				return m, nil
			}

		case "n":
			if m.input.Value() == "" && len(m.searchMatches) > 0 {
				m.searchIdx = (m.searchIdx + 1) % len(m.searchMatches)
				m.jumpToCurrentMatch()
				m.refreshContent()
				return m, nil
			}
		case "N":
			if m.input.Value() == "" && len(m.searchMatches) > 0 {
				m.searchIdx--
				if m.searchIdx < 0 {
					m.searchIdx = len(m.searchMatches) - 1
				}
				m.jumpToCurrentMatch()
				m.refreshContent()
				return m, nil
			}
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		vpHeight := m.height - 3
		if vpHeight < 1 {
			vpHeight = 1
		}
		m.input.Width = m.width - 12
		if !m.ready {
			m.viewport = viewport.New(m.width, vpHeight)
			m.viewport.SetContent(m.renderContent())
			m.viewport.GotoBottom()
			m.ready = true
		} else {
			m.viewport.Width = m.width
			m.viewport.Height = vpHeight
		}

	case newLineMsg:
		m.lines = append(m.lines, msg.line)
		if m.activeSearch != "" || m.isSearchMode() {
			m.recomputeMatches()
		}
		if m.ready {
			atBottom := m.viewport.AtBottom()
			m.viewport.SetContent(m.renderContent())
			if atBottom {
				m.viewport.GotoBottom()
			}
		}
		return m, waitForLine(m.lineCh)
	}

	prevValue := m.input.Value()
	m.input, tiCmd = m.input.Update(msg)
	newValue := m.input.Value()

	if newValue != prevValue {
		m.cmdErr = ""
		if m.isSearchMode() {
			m.recomputeMatches()
			m.searchIdx = 0
			m.jumpToCurrentMatch()
		}
		if m.ready {
			m.viewport.SetContent(m.renderContent())
			// keep cursor at bottom only for filter mode (not search/command)
			if !m.isSearchMode() && !m.isCommandMode() {
				m.viewport.GotoBottom()
			}
		}
	}

	m.viewport, vpCmd = m.viewport.Update(msg)
	return m, tea.Batch(tiCmd, vpCmd)
}

func (m tuiModel) execCommand() (tuiModel, tea.Cmd) {
	raw := strings.TrimSpace(m.input.Value())
	cmd := strings.ToLower(strings.TrimPrefix(raw, "/"))

	switch cmd {
	case "reload":
		m.input.SetValue("")
		m.cmdErr = ""
		select {
		case m.reloadCh <- struct{}{}:
		default:
		}
	case "clear":
		m.input.SetValue("")
		m.cmdErr = ""
		m.lines = nil
		m.searchMatches = nil
		m.searchIdx = 0
		if m.ready {
			m.viewport.SetContent("")
			m.viewport.GotoBottom()
		}
	case "quit", "exit":
		return m, tea.Quit
	default:
		m.cmdErr = "unknown command — available: /reload, /clear, /quit"
	}
	return m, nil
}

// refreshContent re-renders the viewport without changing scroll position
// unless we're in plain filter mode (where new content should follow the tail).
func (m *tuiModel) refreshContent() {
	if !m.ready {
		return
	}
	m.viewport.SetContent(m.renderContent())
}

// recomputeMatches rebuilds searchMatches against the current query.
// searchIdx is preserved if still in range, otherwise clamped.
func (m *tuiModel) recomputeMatches() {
	m.searchMatches = nil
	query := m.currentSearchQuery()
	if query == "" {
		m.searchIdx = 0
		return
	}
	re, _ := regexp.Compile(query)
	for i, line := range m.lines {
		if line == reloadMark {
			continue
		}
		if re != nil {
			for _, idx := range re.FindAllStringIndex(line, -1) {
				m.searchMatches = append(m.searchMatches, matchPos{i, idx[0], idx[1]})
			}
			continue
		}
		// regex didn't compile — fall back to literal substring search
		offset := 0
		for {
			idx := strings.Index(line[offset:], query)
			if idx < 0 {
				break
			}
			start := offset + idx
			end := start + len(query)
			m.searchMatches = append(m.searchMatches, matchPos{i, start, end})
			offset = end
		}
	}
	if m.searchIdx >= len(m.searchMatches) {
		m.searchIdx = 0
	}
}

// jumpToCurrentMatch scrolls the viewport so the current match is visible
// (centered if possible).
func (m *tuiModel) jumpToCurrentMatch() {
	if !m.ready || len(m.searchMatches) == 0 {
		return
	}
	target := m.searchMatches[m.searchIdx].lineIdx
	half := m.viewport.Height / 2
	y := target - half
	if y < 0 {
		y = 0
	}
	m.viewport.YOffset = y
}

func (m tuiModel) renderContent() string {
	switch {
	case m.isCommandMode():
		// command mode shows everything unfiltered, no highlight
		return m.renderLines("", false)
	case m.isSearchMode() || m.activeSearch != "":
		// search mode: show all lines, highlight matches
		return m.renderLines("", true)
	default:
		// filter mode: hide non-matching lines
		return m.renderLines(strings.TrimSpace(m.input.Value()), false)
	}
}

// renderLines renders m.lines. If filter is non-empty, lines that don't match
// are dropped. If highlight is true, matched substrings of currentSearchQuery
// are wrapped with the highlight style.
func (m tuiModel) renderLines(filter string, highlight bool) string {
	var filterRe *regexp.Regexp
	if filter != "" {
		filterRe, _ = regexp.Compile(filter)
	}

	query := ""
	var queryRe *regexp.Regexp
	if highlight {
		query = m.currentSearchQuery()
		if query != "" {
			queryRe, _ = regexp.Compile(query)
		}
	}

	var sb strings.Builder
	for i, line := range m.lines {
		if line == reloadMark {
			sb.WriteString(reloadLineStyle.Render("─── reload ───") + "\n")
			continue
		}
		if filter != "" {
			matched := (filterRe != nil && filterRe.MatchString(line)) ||
				(filterRe == nil && strings.Contains(line, filter))
			if !matched {
				continue
			}
		}
		if highlight && query != "" {
			sb.WriteString(highlightLine(line, i, query, queryRe, m.searchMatches, m.searchIdx))
			continue
		}
		sb.WriteString(line)
	}
	return sb.String()
}

// highlightLine wraps every match of query in line with matchStyle,
// using currentMatchStyle for the active match (matches[currentIdx]).
func highlightLine(line string, lineIdx int, query string, re *regexp.Regexp, matches []matchPos, currentIdx int) string {
	var positions [][2]int
	if re != nil {
		for _, idx := range re.FindAllStringIndex(line, -1) {
			positions = append(positions, [2]int{idx[0], idx[1]})
		}
	} else {
		offset := 0
		for {
			idx := strings.Index(line[offset:], query)
			if idx < 0 {
				break
			}
			start := offset + idx
			end := start + len(query)
			positions = append(positions, [2]int{start, end})
			offset = end
		}
	}
	if len(positions) == 0 {
		return line
	}

	isCurrent := func(s, e int) bool {
		if currentIdx < 0 || currentIdx >= len(matches) {
			return false
		}
		mp := matches[currentIdx]
		return mp.lineIdx == lineIdx && mp.start == s && mp.end == e
	}

	var sb strings.Builder
	cursor := 0
	for _, pos := range positions {
		sb.WriteString(line[cursor:pos[0]])
		style := matchStyle
		if isCurrent(pos[0], pos[1]) {
			style = currentMatchStyle
		}
		sb.WriteString(style.Render(line[pos[0]:pos[1]]))
		cursor = pos[1]
	}
	sb.WriteString(line[cursor:])
	return sb.String()
}

func (m tuiModel) View() string {
	if !m.ready {
		return "starting…\n"
	}

	var label, hint string
	switch {
	case m.isCommandMode():
		label = cmdLabelStyle.Render("cmd:    ")
		if m.cmdErr != "" {
			hint = " " + errorStyle.Render(m.cmdErr)
		}
	case m.isSearchMode():
		label = searchLabelStyle.Render("search: ")
		query := strings.TrimPrefix(m.input.Value(), "?")
		if query != "" {
			if _, err := regexp.Compile(query); err != nil {
				hint = " " + errorStyle.Render("(invalid regex — literal match)")
			}
		}
	default:
		label = filterLabelStyle.Render("filter: ")
		filter := strings.TrimSpace(m.input.Value())
		if filter != "" {
			if _, err := regexp.Compile(filter); err != nil {
				hint = " " + errorStyle.Render("(invalid regex — literal match)")
			}
		} else if m.activeSearch != "" && len(m.searchMatches) > 0 {
			hint = " " + searchStatusStyle.Render(fmt.Sprintf("search %q %d/%d  (n/N to navigate, Esc to clear)", m.activeSearch, m.searchIdx+1, len(m.searchMatches)))
		} else if m.activeSearch != "" {
			hint = " " + searchStatusStyle.Render(fmt.Sprintf("search %q 0/0  (Esc to clear)", m.activeSearch))
		}
	}

	bar := filterBarStyle.Width(m.width - 2).Render(label + m.input.View() + hint)
	return m.viewport.View() + "\n" + bar
}

// tuiLogWriter sends zerolog output lines to the TUI channel so watcher
// messages appear in the viewport alongside subprocess output.
type tuiLogWriter struct {
	ch  chan<- string
	buf []byte
}

func (w *tuiLogWriter) Write(p []byte) (int, error) {
	w.buf = append(w.buf, p...)
	for {
		i := bytes.IndexByte(w.buf, '\n')
		if i < 0 {
			break
		}
		line := string(w.buf[:i+1])
		w.buf = w.buf[i+1:]
		select {
		case w.ch <- line:
		default:
		}
	}
	return len(p), nil
}

// tuiWriter is the io.Writer wired to the subprocess in TUI mode.
// It buffers output line-by-line and forwards each line to the TUI channel.
type tuiWriter struct {
	ch  chan<- string
	buf []byte
}

func (w *tuiWriter) Write(p []byte) (int, error) {
	w.buf = append(w.buf, p...)
	for {
		i := bytes.IndexByte(w.buf, '\n')
		if i < 0 {
			break
		}
		line := string(w.buf[:i+1])
		w.buf = w.buf[i+1:]
		w.ch <- line
	}
	return len(p), nil
}

func runTUI(ctx context.Context, lineCh chan string, reloadCh chan<- struct{}, initialFilter string) {
	m := newTUIModel(lineCh, reloadCh, initialFilter)
	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		log.Error().Err(err).Msg("tui error")
	}
	if cancel, ok := ctx.Value(cancelKey{}).(context.CancelFunc); ok {
		cancel()
	}
}
