package main

import (
	"database/sql"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/lipgloss"
	stopwatch "github.com/jwc20/stopwatch-tui/stopwatch"
)

type model struct {
	db           *sql.DB
	session      *Session
	splitIDs     []int64
	stopwatch    stopwatch.Model
	keymap       keymap
	help         help.Model
	splitInputs  []textinput.Model
	focusedInput int
	quitting     bool
	altscreen    bool
	width        int
	height       int
	statusMsg    string
}

type keymap struct {
	start       key.Binding
	split       key.Binding
	stop        key.Binding
	reset       key.Binding
	fullscreen  key.Binding
	export      key.Binding
	exportJSON  key.Binding
	navUp       key.Binding
	navDown     key.Binding
	unfocus     key.Binding
	deleteSplit key.Binding
	quit        key.Binding
}

func newSplitInput(index int) textinput.Model {
	ti := textinput.New()
	ti.Placeholder = fmt.Sprintf("Split %d", index+1)
	ti.CharLimit = 64
	ti.SetWidth(30)
	return ti
}

func (m model) Init() tea.Cmd {
	return m.stopwatch.Init()
}

func (m model) inputFocused() bool {
	return m.focusedInput >= 0 && m.focusedInput < len(m.splitInputs)
}

func (m model) View() tea.View {
	content := "Elapsed: " + m.stopwatch.View() + "\n"

	if m.stopwatch.HasLap() {
		// content += m.stopwatch.LapView() + "\n"
		subtextStyle := lipgloss.NewStyle().Faint(true).Foreground(lipgloss.Color("241"))
		lapText := m.stopwatch.LapView()
		content += subtextStyle.Render(lapText + "\n")
	}

	content += "\n"

	splits := m.stopwatch.Splits()
	if len(splits) > 0 {
		content += m.splitsView(splits) + "\n"
	}

	if m.statusMsg != "" {
		content += m.statusMsg + "\n"
	}

	if !m.quitting {
		content += m.helpView()
	}

	if m.altscreen {
		content = m.center(content)
	}

	v := tea.NewView(content)
	v.AltScreen = m.altscreen
	return v
}

func (m model) splitsView(splits []stopwatch.SplitEntry) string {
	var sb strings.Builder
	for i, s := range splits {
		var lap time.Duration
		if i == 0 {
			lap = s.Elapsed
		} else {
			lap = s.Elapsed - splits[i-1].Elapsed
		}

		cursor := "  "
		if m.focusedInput == i {
			cursor = "> "
		}

		var inputView string
		if i < len(m.splitInputs) {
			inputView = m.splitInputs[i].View()
		}

		sb.WriteString(fmt.Sprintf(
			"%s%2d.  %s  (+%s)  %s  %s\n",
			cursor,
			i+1,
			formatSplitDuration(s.Elapsed),
			formatSplitDuration(lap),
			s.RecordedAt.Format("2006-01-02 15:04:05"),
			inputView,
		))
	}
	return sb.String()
}

func formatSplitDuration(d time.Duration) string {
	ms := d.Milliseconds()
	h := ms / 3_600_000
	ms -= h * 3_600_000
	min := ms / 60_000
	ms -= min * 60_000
	sec := ms / 1_000
	ms -= sec * 1_000
	if h > 0 {
		return fmt.Sprintf("%02d:%02d:%02d.%03d", h, min, sec, ms)
	}
	return fmt.Sprintf("%02d:%02d.%03d", min, sec, ms)
}

func (m model) center(content string) string {
	if m.width == 0 || m.height == 0 {
		return content
	}
	lines := strings.Split(strings.TrimRight(content, "\n"), "\n")
	maxWidth := 0
	for _, line := range lines {
		if w := lipgloss.Width(line); w > maxWidth {
			maxWidth = w
		}
	}
	leftPad := (m.width - maxWidth) / 2
	if leftPad < 0 {
		leftPad = 0
	}
	prefix := strings.Repeat(" ", leftPad)
	topPad := (m.height - len(lines)) / 2
	if topPad < 0 {
		topPad = 0
	}
	var sb strings.Builder
	sb.WriteString(strings.Repeat("\n", topPad))
	for _, line := range lines {
		sb.WriteString(prefix + line + "\n")
	}
	return sb.String()
}

func (m model) helpView() string {
	var bindings []key.Binding
	if m.inputFocused() {
		bindings = []key.Binding{
			m.keymap.navUp,
			// m.keymap.navDown,
			m.keymap.deleteSplit,
			m.keymap.unfocus,
			m.keymap.quit,
		}
	} else {
		bindings = []key.Binding{
			m.keymap.start,
			m.keymap.stop,
			m.keymap.split,
			m.keymap.reset,
			m.keymap.export,
			m.keymap.exportJSON,
			m.keymap.fullscreen,
			m.keymap.quit,
		}
	}
	return "\n" + m.help.ShortHelpView(bindings)
}

func (m *model) setFocus(index int) tea.Cmd {
	if m.inputFocused() {
		m.splitInputs[m.focusedInput].Blur()
	}
	m.focusedInput = index
	if m.inputFocused() {
		return m.splitInputs[m.focusedInput].Focus()
	}
	return nil
}

func (m model) splitNames() []string {
	names := make([]string, len(m.splitInputs))
	for i, ti := range m.splitInputs {
		names[i] = ti.Value()
		if names[i] == "" {
			names[i] = fmt.Sprintf("Split %d", i+1)
		}
	}
	return names
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		if m.inputFocused() {
			switch {
			case key.Matches(msg, m.keymap.unfocus):
				if m.session != nil && m.focusedInput < len(m.splitIDs) {
					updateSplitName(m.db, m.splitIDs[m.focusedInput], m.splitInputs[m.focusedInput].Value())
				}
				return m, m.setFocus(-1)

			case key.Matches(msg, m.keymap.deleteSplit):
				i := m.focusedInput
				splits := m.stopwatch.Splits()

				if m.session != nil && i < len(m.splitIDs) {
					if i > 0 {
						mergeSplitUp(m.db, m.splitIDs[i-1], splits[i].Elapsed.Milliseconds(), splits[i].RecordedAt)
					}
					softDeleteSplit(m.db, m.splitIDs[i])
					m.splitIDs = append(m.splitIDs[:i], m.splitIDs[i+1:]...)
				}

				cmd := m.stopwatch.DeleteSplit(i)
				m.splitInputs = append(m.splitInputs[:i], m.splitInputs[i+1:]...)
				if i >= len(m.splitInputs) {
					i = len(m.splitInputs) - 1
				}
				m.focusedInput = -1
				if i >= 0 {
					return m, tea.Batch(cmd, m.setFocus(i))
				}
				return m, cmd

			case key.Matches(msg, m.keymap.navUp):
				if m.focusedInput > 0 {
					return m, m.setFocus(m.focusedInput - 1)
				}
				return m, nil

			case key.Matches(msg, m.keymap.navDown):
				if m.focusedInput < len(m.splitInputs)-1 {
					return m, m.setFocus(m.focusedInput + 1)
				}
				return m, nil

			default:
				var cmd tea.Cmd
				m.splitInputs[m.focusedInput], cmd = m.splitInputs[m.focusedInput].Update(msg)
				return m, cmd
			}
		}

		switch {
		case key.Matches(msg, m.keymap.navDown):
			if len(m.splitInputs) > 0 {
				return m, m.setFocus(0)
			}
			return m, nil

		case key.Matches(msg, m.keymap.export):
			filename, err := exportICS(m.stopwatch.Splits(), m.splitNames())
			if err != nil {
				m.statusMsg = "export failed: " + err.Error()
			} else {
				m.statusMsg = "exported: " + filename
			}
			return m, nil

		case key.Matches(msg, m.keymap.exportJSON):
			filename, err := exportGWSCommands(m.stopwatch.Splits(), m.splitNames())
			if err != nil {
				m.statusMsg = "export failed: " + err.Error()
			} else {
				m.statusMsg = "exported: " + filename
			}
			return m, nil

		case key.Matches(msg, m.keymap.fullscreen):
			m.altscreen = !m.altscreen
			if m.session != nil {
				updateSessionFullscreen(m.db, m.session.ID, m.altscreen)
			}
			return m, nil

		case key.Matches(msg, m.keymap.quit):
			if m.session != nil {
				saveAllSplitNames(m.db, m.splitIDs, m.splitNames())
			}
			m.quitting = true
			return m, tea.Quit

		case key.Matches(msg, m.keymap.reset):
			m.splitInputs = nil
			m.focusedInput = -1
			return m, m.stopwatch.Reset()

		case key.Matches(msg, m.keymap.split):
			return m, m.stopwatch.Split()

		case key.Matches(msg, m.keymap.start, m.keymap.stop):
			return m, m.stopwatch.Toggle()
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case stopwatch.StartStopMsg:
		var cmd tea.Cmd
		m.stopwatch, cmd = m.stopwatch.Update(msg)
		running := m.stopwatch.Running()

		m.keymap.start.SetEnabled(!running)
		m.keymap.stop.SetEnabled(running)
		m.keymap.reset.SetEnabled(!running)
		m.keymap.split.SetEnabled(running)

		if running {
			if m.session == nil {
				session, err := createSession(m.db, m.altscreen)
				if err != nil {
					m.statusMsg = "db error: " + err.Error()
				} else {
					m.session = session
				}
			} else if m.session.LastPausedAt != nil {
				if err := resumeSession(m.db, m.session.ID, *m.session.LastPausedAt); err != nil {
					m.statusMsg = "db error: " + err.Error()
				}
				m.session.TotalPausedDurationMs += time.Since(*m.session.LastPausedAt).Milliseconds()
				m.session.LastPausedAt = nil
			}
		} else {
			if m.session != nil {
				if err := pauseSession(m.db, m.session.ID); err != nil {
					m.statusMsg = "db error: " + err.Error()
				}
				now := time.Now()
				m.session.LastPausedAt = &now
			}
		}
		return m, cmd

	case stopwatch.ResetMsg:
		var cmd tea.Cmd
		m.stopwatch, cmd = m.stopwatch.Update(msg)

		if m.session != nil {
			if err := softDeleteSession(m.db, m.session.ID); err != nil {
				m.statusMsg = "db error: " + err.Error()
			}
		}
		m.session = nil
		m.splitIDs = nil

		m.keymap.start.SetEnabled(true)
		m.keymap.stop.SetEnabled(false)
		m.keymap.reset.SetEnabled(true)
		m.keymap.split.SetEnabled(false)
		return m, cmd

	case stopwatch.SplitMsg:
		var cmd tea.Cmd
		m.stopwatch, cmd = m.stopwatch.Update(msg)

		if m.session != nil {
			splits := m.stopwatch.Splits()
			newest := splits[len(splits)-1]
			row, err := insertSplit(
				m.db, m.session.ID,
				newest.Elapsed.Milliseconds(),
				len(splits),
				"",
				newest.RecordedAt,
			)
			if err != nil {
				m.statusMsg = "db error: " + err.Error()
			} else {
				m.splitIDs = append(m.splitIDs, row.ID)
			}
		}
		m.splitInputs = append(m.splitInputs, newSplitInput(len(m.splitInputs)))
		return m, cmd

	case stopwatch.DeleteSplitMsg:
		var cmd tea.Cmd
		m.stopwatch, cmd = m.stopwatch.Update(msg)
		return m, cmd

	case stopwatch.TickMsg:
		var cmd tea.Cmd
		m.stopwatch, cmd = m.stopwatch.Update(msg)
		return m, cmd
	}

	var cmd tea.Cmd
	m.stopwatch, cmd = m.stopwatch.Update(msg)
	return m, cmd
}

func main() {
	db, err := openDB()
	if err != nil {
		fmt.Fprintln(os.Stderr, "failed to open database:", err)
		os.Exit(1)
	}
	defer db.Close()

	session, err := getActiveSession(db)
	if err != nil {
		fmt.Fprintln(os.Stderr, "warning: could not load session:", err)
	}

	opts := []stopwatch.Option{
		stopwatch.WithInterval(time.Millisecond),
	}

	var splitIDs []int64
	var splitInputs []textinput.Model
	isFullscreen := true

	if session != nil {
		opts = append(opts,
			stopwatch.WithElapsed(session.ElapsedDuration()),
			stopwatch.WithRunning(session.IsRunning()),
		)
		isFullscreen = session.IsFullscreen

		splitRows, err := getSplits(db, session.ID)
		if err != nil {
			fmt.Fprintln(os.Stderr, "warning: could not load splits:", err)
		}

		var entries []stopwatch.SplitEntry
		for _, row := range splitRows {
			splitIDs = append(splitIDs, row.ID)
			entries = append(entries, stopwatch.SplitEntry{
				Elapsed:    time.Duration(row.SplitTimeMs) * time.Millisecond,
				RecordedAt: row.CreatedAt,
			})
			ti := newSplitInput(len(splitInputs))
			if row.Name != "" {
				ti.SetValue(row.Name)
			}
			splitInputs = append(splitInputs, ti)
		}
		if len(entries) > 0 {
			opts = append(opts, stopwatch.WithSplits(entries))
		}
	}

	sw := stopwatch.New(opts...)

	m := model{
		db:           db,
		session:      session,
		splitIDs:     splitIDs,
		stopwatch:    sw,
		splitInputs:  splitInputs,
		focusedInput: -1,
		altscreen:    isFullscreen,
		keymap: keymap{
			start: key.NewBinding(
				key.WithKeys("s"),
				key.WithHelp("s", "start"),
			),
			stop: key.NewBinding(
				key.WithKeys("s"),
				key.WithHelp("s", "stop"),
			),
			split: key.NewBinding(
				key.WithKeys("p"),
				key.WithHelp("p", "split"),
			),
			reset: key.NewBinding(
				key.WithKeys("r"),
				key.WithHelp("r", "reset"),
			),
			fullscreen: key.NewBinding(
				key.WithKeys("f"),
				key.WithHelp("f", "fullscreen"),
			),
			export: key.NewBinding(
				key.WithKeys("e"),
				key.WithHelp("e", "export .ics"),
			),
			exportJSON: key.NewBinding(
				key.WithKeys("g"),
				key.WithHelp("g", "export gws"),
			),
			navUp: key.NewBinding(
				key.WithKeys("up"),
				key.WithHelp("↑/↓", "navigate"),
			),
			navDown: key.NewBinding(
				key.WithKeys("down"),
				key.WithHelp("↓", "navigate"),
			),
			unfocus: key.NewBinding(
				key.WithKeys("esc"),
				key.WithHelp("esc", "done editing"),
			),
			deleteSplit: key.NewBinding(
				key.WithKeys("ctrl+d"),
				key.WithHelp("ctrl+d", "delete split"),
			),
			quit: key.NewBinding(
				key.WithKeys("ctrl+c", "q"),
				key.WithHelp("q", "quit"),
			),
		},
		help: help.New(),
	}

	running := session != nil && session.IsRunning()
	m.keymap.start.SetEnabled(!running)
	m.keymap.stop.SetEnabled(running)
	m.keymap.reset.SetEnabled(!running)
	m.keymap.split.SetEnabled(running)

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigs
		db.Close()
		os.Exit(0)
	}()

	if _, err := tea.NewProgram(m).Run(); err != nil {
		fmt.Println("Oh no, it didn't work:", err)
		os.Exit(1)
	}
}
