package tui

import (
	"fmt"

	"tai/internal/history"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// historyItem adapts a history.HistoryEntry to bubbles/list.DefaultItem.
// Title is the generated command (what the user wants to re-use); the
// description is the original natural-language prompt that produced it.
// FilterValue concatenates both so the built-in fuzzy filter matches either.
type historyItem struct {
	entry history.HistoryEntry
}

func (h historyItem) Title() string               { return h.entry.Command }
func (h historyItem) Description() string         { return h.entry.Prompt }
func (h historyItem) FilterValue() string         { return h.entry.Command + " " + h.entry.Prompt }
func (h historyItem) Entry() history.HistoryEntry { return h.entry }

// HistoryModel is the Bubble Tea model wrapping a bubbles/list of history
// entries. Pressing Enter on a row captures the entry in `selected` and quits;
// Esc / Ctrl+C quit without selection.
type HistoryModel struct {
	list     list.Model
	selected *history.HistoryEntry
}

// NewHistory builds a HistoryModel pre-populated with the given entries.
// The list delegate is re-tinted so selected rows pick up tai's accent color
// instead of the bubbles/list default magenta.
func NewHistory(entries []history.HistoryEntry) HistoryModel {
	items := make([]list.Item, 0, len(entries))
	for _, e := range entries {
		items = append(items, historyItem{entry: e})
	}

	accent := lipgloss.Color("#7D56F4")
	descTint := lipgloss.Color("#B294BB")

	delegate := list.NewDefaultDelegate()
	delegate.Styles.SelectedTitle = delegate.Styles.SelectedTitle.
		Foreground(accent).
		BorderForeground(accent)
	delegate.Styles.SelectedDesc = delegate.Styles.SelectedDesc.
		Foreground(descTint).
		BorderForeground(accent)

	l := list.New(items, delegate, 0, 0)
	l.Title = "📜 tai history"
	l.SetFilteringEnabled(true)
	l.SetShowStatusBar(true)
	l.SetStatusBarItemName("command", "commands")
	l.Styles.Title = l.Styles.Title.Background(accent)

	return HistoryModel{list: l}
}

func (m HistoryModel) Init() tea.Cmd { return nil }

func (m HistoryModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.list.SetSize(msg.Width, msg.Height)
		return m, nil

	case tea.KeyMsg:
		// While the filter input is focused, Enter belongs to the list itself
		// (it commits the filter); we must not steal it as "select".
		if m.list.FilterState() != list.Filtering {
			switch msg.Type {
			case tea.KeyEnter:
				if it, ok := m.list.SelectedItem().(historyItem); ok {
					entry := it.entry
					m.selected = &entry
					return m, tea.Quit
				}
				return m, nil
			case tea.KeyCtrlC, tea.KeyEsc:
				return m, tea.Quit
			}
		}
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m HistoryModel) View() string {
	if len(m.list.Items()) == 0 {
		return helpStyle.Render("No history yet — run a command first.\n")
	}
	return m.list.View()
}

// Selected returns the entry the user chose, or nil if they cancelled.
func (m HistoryModel) Selected() *history.HistoryEntry { return m.selected }

// RunHistory starts the history TUI and blocks until the user picks an entry
// or quits. Returns the selected entry (nil on cancel) and any TUI error.
func RunHistory(entries []history.HistoryEntry) (*history.HistoryEntry, error) {
	p := tea.NewProgram(NewHistory(entries), tea.WithAltScreen())
	final, err := p.Run()
	if err != nil {
		return nil, err
	}
	m, ok := final.(HistoryModel)
	if !ok {
		return nil, fmt.Errorf("unexpected history model type")
	}
	return m.Selected(), nil
}
