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
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/lipgloss"
	stopwatch "github.com/jwc20/stopwatch-tui/stopwatch"
)

type model struct {
	stopwatch stopwatch.Model
	keymap    keymap
	help      help.Model
	quitting  bool
	altscreen bool
	width     int
	height    int
	statusMsg string
}

type keymap struct {
	start      key.Binding
	split      key.Binding
	stop       key.Binding
	reset      key.Binding
	fullscreen key.Binding
	export     key.Binding
	quit       key.Binding
}

func (m model) Init() tea.Cmd {
	return m.stopwatch.Init()
}

func (m model) View() tea.View {
	content := "Elapsed: " + m.stopwatch.View() + "\n\n"

	if splits := m.stopwatch.SplitsView(); splits != "" {
		content += splits + "\n"
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
	return "\n" + m.help.ShortHelpView([]key.Binding{
		m.keymap.start,
		m.keymap.stop,
		m.keymap.split,
		m.keymap.reset,
		m.keymap.export,
		m.keymap.fullscreen,
		m.keymap.quit,
	})
}

func (m model) currentAppState() AppState {
	entries := m.stopwatch.Splits()
	splitStates := make([]SplitState, len(entries))
	for i, e := range entries {
		splitStates[i] = SplitState{
			ElapsedNs:  e.Elapsed.Nanoseconds(),
			RecordedAt: e.RecordedAt,
		}
	}

	state := AppState{
		Running:   m.stopwatch.Running(),
		ElapsedNs: m.stopwatch.Elapsed().Nanoseconds(),
		Splits:    splitStates,
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

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch {
		case key.Matches(msg, m.keymap.export):
			filename, err := exportICS(m.stopwatch.Splits())
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
			cmd := m.stopwatch.Reset()
			DeleteState()
			return m, cmd
		case key.Matches(msg, m.keymap.split):
			return m, m.stopwatch.Split()
		case key.Matches(msg, m.keymap.start, m.keymap.stop):
			m.keymap.stop.SetEnabled(!m.stopwatch.Running())
			m.keymap.start.SetEnabled(m.stopwatch.Running())
			swCmd := m.stopwatch.Toggle()
			return m, tea.Batch(swCmd, saveCmd(m.currentAppState()))
		}
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case stopwatch.TickMsg:
		var cmd tea.Cmd
		m.stopwatch, cmd = m.stopwatch.Update(msg)
		return m, tea.Batch(cmd, saveCmd(m.currentAppState()))
	case stopwatch.StartStopMsg:
		var cmd tea.Cmd
		m.stopwatch, cmd = m.stopwatch.Update(msg)
		m.keymap.reset.SetEnabled(!m.stopwatch.Running())
		return m, tea.Batch(cmd, saveCmd(m.currentAppState()))
	case stopwatch.SplitMsg:
		var cmd tea.Cmd
		m.stopwatch, cmd = m.stopwatch.Update(msg)
		return m, tea.Batch(cmd, saveCmd(m.currentAppState()))
	case stopwatch.ResetMsg:
		var cmd tea.Cmd
		m.stopwatch, cmd = m.stopwatch.Update(msg)
		m.keymap.reset.SetEnabled(true)
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

	m := model{
		stopwatch: stopwatch.New(opts...),
		altscreen: true,
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
			quit: key.NewBinding(
				key.WithKeys("ctrl+c", "q"),
				key.WithHelp("q", "quit"),
			),
		},
		help: help.New(),
	}

	if state == nil || !state.Running {
		m.keymap.start.SetEnabled(false)
		m.keymap.reset.SetEnabled(true)
	} else {
		m.keymap.stop.SetEnabled(false)
		m.keymap.reset.SetEnabled(false)
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
