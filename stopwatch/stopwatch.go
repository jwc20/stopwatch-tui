package stopwatch

import (
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	tea "charm.land/bubbletea/v2"
)

var lastID int64

func nextID() int {
	return int(atomic.AddInt64(&lastID, 1))
}

type Option func(*Model)

func WithInterval(interval time.Duration) Option {
	return func(m *Model) {
		m.Interval = interval
	}
}

func WithElapsed(d time.Duration) Option {
	return func(m *Model) {
		m.d = d
	}
}

func WithSplits(splits []SplitEntry) Option {
	return func(m *Model) {
		m.splits = splits
	}
}

func WithRunning(running bool) Option {
	return func(m *Model) {
		m.running = running
	}
}

type TickMsg struct {
	ID  int
	tag int
}

type StartStopMsg struct {
	ID      int
	running bool
}

type ResetMsg struct {
	ID int
}

type SplitMsg struct {
	ID         int
	RecordedAt time.Time
}

type DeleteSplitMsg struct {
	ID    int
	Index int
}

type SplitEntry struct {
	Elapsed    time.Duration
	RecordedAt time.Time
}

type Model struct {
	d       time.Duration
	id      int
	tag     int
	running bool
	splits  []SplitEntry

	Interval time.Duration
}

func New(opts ...Option) Model {
	m := Model{
		id: nextID(),
	}
	for _, opt := range opts {
		opt(&m)
	}
	return m
}

func (m Model) ID() int {
	return m.id
}

func (m Model) Init() tea.Cmd {
	if m.running {
		return tick(m.id, m.tag, m.Interval)
	}
	return nil
}

func (m Model) Start() tea.Cmd {
	return tea.Sequence(func() tea.Msg {
		return StartStopMsg{ID: m.id, running: true}
	}, tick(m.id, m.tag, m.Interval))
}

func (m Model) Stop() tea.Cmd {
	return func() tea.Msg {
		return StartStopMsg{ID: m.id, running: false}
	}
}

func (m Model) Split() tea.Cmd {
	return func() tea.Msg {
		return SplitMsg{ID: m.id, RecordedAt: time.Now()}
	}
}

func (m Model) DeleteSplit(index int) tea.Cmd {
	return func() tea.Msg {
		return DeleteSplitMsg{ID: m.id, Index: index}
	}
}

func (m Model) Toggle() tea.Cmd {
	if m.Running() {
		return m.Stop()
	}
	return m.Start()
}

func (m Model) Reset() tea.Cmd {
	return func() tea.Msg {
		return ResetMsg{ID: m.id}
	}
}

func (m Model) Running() bool {
	return m.running
}

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case StartStopMsg:
		if msg.ID != m.id {
			return m, nil
		}
		m.running = msg.running
	case ResetMsg:
		if msg.ID != m.id {
			return m, nil
		}
		m.d = 0
		m.splits = nil
	case SplitMsg:
		if msg.ID != m.id {
			return m, nil
		}
		m.splits = append(m.splits, SplitEntry{
			Elapsed:    m.d,
			RecordedAt: msg.RecordedAt,
		})
	case DeleteSplitMsg:
		if msg.ID != m.id {
			return m, nil
		}
		i := msg.Index
		if i < 0 || i >= len(m.splits) {
			return m, nil
		}
		if i > 0 {
			m.splits[i-1].Elapsed = m.splits[i].Elapsed
			m.splits[i-1].RecordedAt = m.splits[i].RecordedAt
		}
		m.splits = append(m.splits[:i], m.splits[i+1:]...)
	case TickMsg:
		if !m.running || msg.ID != m.id {
			break
		}
		if msg.tag > 0 && msg.tag != m.tag {
			return m, nil
		}
		m.d += m.Interval
		m.tag++
		return m, tick(m.id, m.tag, m.Interval)
	}
	return m, nil
}

func (m Model) Elapsed() time.Duration {
	return m.d
}

func (m Model) Splits() []SplitEntry {
	return m.splits
}

func (m Model) View() string {
	return formatDuration(m.d)
}

func (m Model) SplitsView() string {
	if len(m.splits) == 0 {
		return ""
	}
	var sb strings.Builder
	for i, s := range m.splits {
		var lap time.Duration
		if i == 0 {
			lap = s.Elapsed
		} else {
			lap = s.Elapsed - m.splits[i-1].Elapsed
		}
		sb.WriteString(fmt.Sprintf(
			"%2d.  %s  (+%s)  %s\n",
			i+1,
			formatDuration(s.Elapsed),
			formatDuration(lap),
			s.RecordedAt.Format("2006-01-02 15:04:05"),
		))
	}
	return sb.String()
}

func formatDuration(d time.Duration) string {
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

func tick(id int, tag int, d time.Duration) tea.Cmd {
	return tea.Tick(d, func(_ time.Time) tea.Msg {
		return TickMsg{ID: id, tag: tag}
	})
}
