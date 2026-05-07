package main

import (
	"bytes"
	"context"
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
	reloadLineStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("208"))
	filterBarStyle   = lipgloss.NewStyle().BorderStyle(lipgloss.NormalBorder()).BorderTop(true).BorderForeground(lipgloss.Color("240")).Padding(0, 1)
	filterLabelStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	cmdLabelStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Bold(true)
	errorStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
)

type newLineMsg struct{ line string }

type tuiModel struct {
	lines    []string
	lineCh   <-chan string
	reloadCh chan<- struct{}
	input    textinput.Model
	viewport viewport.Model
	width    int
	height   int
	ready    bool
	cmdErr   string
}

func newTUIModel(lineCh <-chan string, reloadCh chan<- struct{}, initialFilter string) tuiModel {
	ti := textinput.New()
	ti.Placeholder = "regex filter… (/ for commands)"
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
			if m.isCommandMode() {
				m.input.SetValue("")
				m.cmdErr = ""
				return m, nil
			}
			return m, tea.Quit

		case "enter":
			if m.isCommandMode() {
				m, tiCmd = m.execCommand()
				return m, tiCmd
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
			m.viewport.SetContent(m.filteredContent())
			m.viewport.GotoBottom()
			m.ready = true
		} else {
			m.viewport.Width = m.width
			m.viewport.Height = vpHeight
		}

	case newLineMsg:
		m.lines = append(m.lines, msg.line)
		if m.ready {
			atBottom := m.viewport.AtBottom()
			m.viewport.SetContent(m.filteredContent())
			if atBottom {
				m.viewport.GotoBottom()
			}
		}
		return m, waitForLine(m.lineCh)
	}

	prevFilter := m.input.Value()
	m.input, tiCmd = m.input.Update(msg)
	newFilter := m.input.Value()

	if newFilter != prevFilter {
		m.cmdErr = ""
		if m.ready {
			m.viewport.SetContent(m.filteredContent())
			m.viewport.GotoBottom()
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
	default:
		m.cmdErr = "unknown command — available: /reload"
	}
	return m, nil
}

func (m tuiModel) filteredContent() string {
	// in command mode show everything unfiltered
	if m.isCommandMode() {
		return m.renderLines("")
	}
	return m.renderLines(strings.TrimSpace(m.input.Value()))
}

func (m tuiModel) renderLines(filter string) string {
	var re *regexp.Regexp
	if filter != "" {
		var err error
		re, err = regexp.Compile(filter)
		if err != nil {
			re = nil
		}
	}

	var sb strings.Builder
	for _, line := range m.lines {
		if line == reloadMark {
			sb.WriteString(reloadLineStyle.Render("─── reload ───") + "\n")
			continue
		}
		if filter == "" || (re != nil && re.MatchString(line)) || (re == nil && strings.Contains(line, filter)) {
			sb.WriteString(line)
		}
	}
	return sb.String()
}

func (m tuiModel) View() string {
	if !m.ready {
		return "starting…\n"
	}

	var label, hint string
	if m.isCommandMode() {
		label = cmdLabelStyle.Render("cmd:    ")
		if m.cmdErr != "" {
			hint = " " + errorStyle.Render(m.cmdErr)
		}
	} else {
		label = filterLabelStyle.Render("filter: ")
		filter := strings.TrimSpace(m.input.Value())
		if filter != "" {
			if _, err := regexp.Compile(filter); err != nil {
				hint = " " + errorStyle.Render("(invalid regex — literal match)")
			}
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
