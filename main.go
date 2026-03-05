package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	stopwatch "github.com/jwc20/stopwatch-tui/stopwatch"
)

type model struct {
	stopwatch stopwatch.Model
	keymap    keymap
	help      help.Model
	quitting  bool
}

type keymap struct {
	start key.Binding
	split key.Binding
	stop  key.Binding
	reset key.Binding
	quit  key.Binding
}

func (m model) Init() tea.Cmd {
	return m.stopwatch.Init()
}

func (m model) View() tea.View {
	s := "Elapsed: " + m.stopwatch.View() + "\n\n"

	if splits := m.stopwatch.SplitsView(); splits != "" {
		s += splits + "\n"
	}

	if !m.quitting {
		s += m.helpView()
	}

	v := tea.NewView(s)
	v.AltScreen = true
	return v
}

func (m model) helpView() string {
	return "\n" + m.help.ShortHelpView([]key.Binding{
		m.keymap.start,
		m.keymap.stop,
		m.keymap.split,
		m.keymap.reset,
		m.keymap.quit,
	})
}

func (m model) currentAppState() AppState {
	splits := m.stopwatch.Splits()
	splitNs := make([]int64, len(splits))
	for i, s := range splits {
		splitNs[i] = s.Nanoseconds()
	}

	state := AppState{
		Running:   m.stopwatch.Running(),
		ElapsedNs: m.stopwatch.Elapsed().Nanoseconds(),
		Splits:    splitNs,
	}
	if state.Running {
		state.StartedAt = time.Now()
		state.ElapsedNs = m.stopwatch.Elapsed().Nanoseconds()
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
		case key.Matches(msg, m.keymap.quit):
			m.quitting = true
			return m, tea.Sequence(saveCmd(m.currentAppState()), tea.Quit)
		case key.Matches(msg, m.keymap.reset):
			cmd := m.stopwatch.Reset()
			DeleteState()
			return m, cmd
		case key.Matches(msg, m.keymap.split):
			var swCmd tea.Cmd
			m.stopwatch, swCmd = m.stopwatch.Update(stopwatch.SplitMsg{})
			return m, tea.Batch(swCmd, saveCmd(m.currentAppState()))
		case key.Matches(msg, m.keymap.start, m.keymap.stop):
			m.keymap.stop.SetEnabled(!m.stopwatch.Running())
			m.keymap.start.SetEnabled(m.stopwatch.Running())
			swCmd := m.stopwatch.Toggle()
			return m, tea.Batch(swCmd, saveCmd(m.currentAppState()))
		}
	case stopwatch.TickMsg:
		var cmd tea.Cmd
		m.stopwatch, cmd = m.stopwatch.Update(msg)
		return m, tea.Batch(cmd, saveCmd(m.currentAppState()))
	case stopwatch.StartStopMsg:
		var cmd tea.Cmd
		m.stopwatch, cmd = m.stopwatch.Update(msg)
		return m, tea.Batch(cmd, saveCmd(m.currentAppState()))
	case stopwatch.ResetMsg:
		var cmd tea.Cmd
		m.stopwatch, cmd = m.stopwatch.Update(msg)
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
			stopwatch.WithSplits(state.SplitDurations()),
			stopwatch.WithRunning(state.Running),
		)
	}

	m := model{
		stopwatch: stopwatch.New(opts...),
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
			quit: key.NewBinding(
				key.WithKeys("ctrl+c", "q"),
				key.WithHelp("q", "quit"),
			),
		},
		help: help.New(),
	}

	if state == nil || !state.Running {
		m.keymap.start.SetEnabled(false)
	} else {
		m.keymap.stop.SetEnabled(false)
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
