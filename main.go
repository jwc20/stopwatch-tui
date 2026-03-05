package main

import (
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
	content := "Elapsed: " + m.stopwatch.View() + "\n\n"

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
			m.keymap.navDown,
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

func (m model) currentAppState() AppState {
	entries := m.stopwatch.Splits()
	splitStates := make([]SplitState, len(entries))
	for i, e := range entries {
		name := ""
		if i < len(m.splitInputs) {
			name = m.splitInputs[i].Value()
		}
		splitStates[i] = SplitState{
			ElapsedNs:  e.Elapsed.Nanoseconds(),
			RecordedAt: e.RecordedAt,
			Name:       name,
		}
	}

	state := AppState{
		Running:    m.stopwatch.Running(),
		ElapsedNs:  m.stopwatch.Elapsed().Nanoseconds(),
		Splits:     splitStates,
		Fullscreen: m.altscreen,
	}
	if state.Running {
		state.StartedAt = time.Now()
	}
	return state
}

func saveCmd(state AppState) tea.Cmd {
	return func() tea.Msg {
		SaveState(state)
		return nil
	}
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
				cmd := m.setFocus(-1)
				return m, cmd
			case key.Matches(msg, m.keymap.deleteSplit):
				i := m.focusedInput
				cmd := m.stopwatch.DeleteSplit(i)
				m.splitInputs = append(m.splitInputs[:i], m.splitInputs[i+1:]...)
				if i >= len(m.splitInputs) {
					i = len(m.splitInputs) - 1
				}
				m.focusedInput = -1
				if i >= 0 {
					focusCmd := m.setFocus(i)
					return m, tea.Batch(cmd, focusCmd)
				}
				return m, cmd
			case key.Matches(msg, m.keymap.navUp):
				if m.focusedInput > 0 {
					cmd := m.setFocus(m.focusedInput - 1)
					return m, cmd
				}
				return m, nil
			case key.Matches(msg, m.keymap.navDown):
				if m.focusedInput < len(m.splitInputs)-1 {
					cmd := m.setFocus(m.focusedInput + 1)
					return m, cmd
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
				cmd := m.setFocus(0)
				return m, cmd
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
			return m, nil
		case key.Matches(msg, m.keymap.quit):
			m.quitting = true
			return m, tea.Sequence(saveCmd(m.currentAppState()), tea.Quit)
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

	case stopwatch.DeleteSplitMsg:
		var cmd tea.Cmd
		m.stopwatch, cmd = m.stopwatch.Update(msg)
		return m, tea.Batch(cmd, saveCmd(m.currentAppState()))

	case stopwatch.SplitMsg:
		var cmd tea.Cmd
		m.stopwatch, cmd = m.stopwatch.Update(msg)
		m.splitInputs = append(m.splitInputs, newSplitInput(len(m.splitInputs)))
		return m, tea.Batch(cmd, saveCmd(m.currentAppState()))

	case stopwatch.TickMsg:
		var cmd tea.Cmd
		m.stopwatch, cmd = m.stopwatch.Update(msg)
		return m, tea.Batch(cmd, saveCmd(m.currentAppState()))

	case stopwatch.StartStopMsg:
		var cmd tea.Cmd
		m.stopwatch, cmd = m.stopwatch.Update(msg)
		running := m.stopwatch.Running()
		m.keymap.start.SetEnabled(!running)
		m.keymap.stop.SetEnabled(running)
		m.keymap.reset.SetEnabled(!running)
		m.keymap.split.SetEnabled(running)
		return m, tea.Batch(cmd, saveCmd(m.currentAppState()))

	case stopwatch.ResetMsg:
		var cmd tea.Cmd
		m.stopwatch, cmd = m.stopwatch.Update(msg)
		m.keymap.start.SetEnabled(true)
		m.keymap.stop.SetEnabled(false)
		m.keymap.reset.SetEnabled(true)
		DeleteState()
		return m, cmd
	}

	var cmd tea.Cmd
	m.stopwatch, cmd = m.stopwatch.Update(msg)
	return m, cmd
}

func main() {
	opts := []stopwatch.Option{
		stopwatch.WithInterval(time.Millisecond),
	}

	state, err := LoadState()
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not load state: %v\n", err)
	}
	if state != nil {
		opts = append(opts,
			stopwatch.WithElapsed(state.ElapsedDuration()),
			stopwatch.WithSplits(state.SplitEntries()),
			stopwatch.WithRunning(state.Running),
		)
	}

	sw := stopwatch.New(opts...)

	splitInputs := make([]textinput.Model, len(sw.Splits()))
	for i := range splitInputs {
		splitInputs[i] = newSplitInput(i)
		if state != nil && i < len(state.Splits) && state.Splits[i].Name != "" {
			splitInputs[i].SetValue(state.Splits[i].Name)
		}
	}

	m := model{
		stopwatch:    sw,
		splitInputs:  splitInputs,
		focusedInput: -1,
		altscreen:    state == nil || state.Fullscreen,
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

	if state == nil || !state.Running {
		m.keymap.start.SetEnabled(true)
		m.keymap.stop.SetEnabled(false)
		m.keymap.reset.SetEnabled(true)
		m.keymap.split.SetEnabled(false)
	} else {
		m.keymap.start.SetEnabled(false)
		m.keymap.stop.SetEnabled(true)
		m.keymap.reset.SetEnabled(false)
		m.keymap.split.SetEnabled(true)
	}

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigs
		SaveState(m.currentAppState())
		os.Exit(0)
	}()

	if _, err := tea.NewProgram(m).Run(); err != nil {
		fmt.Println("Oh no, it didn't work:", err)
		os.Exit(1)
	}
}
