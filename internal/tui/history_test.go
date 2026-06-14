package tui

import (
	"strings"
	"testing"
	"time"

	"tai/internal/history"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
)

var sampleHistoryEntries = []history.HistoryEntry{
	{
		Timestamp: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC),
		Prompt:    "list files",
		Command:   "ls -la",
	},
	{
		Timestamp: time.Date(2026, 1, 2, 12, 0, 0, 0, time.UTC),
		Prompt:    "find go files",
		Command:   "find . -name '*.go'",
	},
}

func TestHistoryItem_FieldsMatchSpec(t *testing.T) {
	it := historyItem{entry: sampleHistoryEntries[0]}
	if it.Title() != "ls -la" {
		t.Errorf("Title() = %q, want %q", it.Title(), "ls -la")
	}
	if it.Description() != "list files" {
		t.Errorf("Description() = %q, want %q", it.Description(), "list files")
	}
	if !strings.Contains(it.FilterValue(), "ls -la") {
		t.Errorf("FilterValue should include the command, got %q", it.FilterValue())
	}
	if !strings.Contains(it.FilterValue(), "list files") {
		t.Errorf("FilterValue should include the prompt, got %q", it.FilterValue())
	}
	if it.Entry().Prompt != "list files" {
		t.Errorf("Entry().Prompt = %q, want %q", it.Entry().Prompt, "list files")
	}
}

func TestNewHistory_LoadsItemsAndEnablesFiltering(t *testing.T) {
	m := NewHistory(sampleHistoryEntries)
	if got := len(m.list.Items()); got != 2 {
		t.Errorf("items = %d, want 2", got)
	}
	if m.Selected() != nil {
		t.Error("Selected() should be nil initially")
	}
	if !m.list.FilteringEnabled() {
		t.Error("filtering should be enabled")
	}
	if !strings.Contains(m.list.Title, "tai history") {
		t.Errorf("list title = %q, want one mentioning tai history", m.list.Title)
	}
}

func TestNewHistory_EmptyListRendersHint(t *testing.T) {
	m := NewHistory(nil)
	if got := len(m.list.Items()); got != 0 {
		t.Errorf("items = %d, want 0", got)
	}
	out := m.View()
	if !strings.Contains(out, "No history yet") {
		t.Errorf("empty View should hint at running a command, got %q", out)
	}
}

func TestHistoryInit_ReturnsNil(t *testing.T) {
	m := NewHistory(sampleHistoryEntries)
	if cmd := m.Init(); cmd != nil {
		t.Errorf("Init() should return nil, got %T", cmd)
	}
}

func TestHistoryUpdate_EnterSelectsCurrentItemAndQuits(t *testing.T) {
	m := NewHistory(sampleHistoryEntries)
	// Force a non-zero list size so SelectedItem is the first item.
	m.list.SetSize(80, 20)

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	mm := updated.(HistoryModel)
	if mm.Selected() == nil {
		t.Fatal("Selected() should be non-nil after Enter")
	}
	if mm.Selected().Command != "ls -la" {
		t.Errorf("selected command = %q, want %q", mm.Selected().Command, "ls -la")
	}
	if mm.Selected().Prompt != "list files" {
		t.Errorf("selected prompt = %q, want %q", mm.Selected().Prompt, "list files")
	}
	if cmd == nil {
		t.Fatal("expected tea.Quit cmd")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Errorf("expected QuitMsg, got %T", cmd())
	}
}

func TestHistoryUpdate_EnterOnEmptyListDoesNothing(t *testing.T) {
	m := NewHistory(nil)
	m.list.SetSize(80, 20)
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	mm := updated.(HistoryModel)
	if mm.Selected() != nil {
		t.Error("Selected() must remain nil when list is empty")
	}
	if cmd != nil {
		t.Errorf("expected nil cmd, got %T", cmd)
	}
}

func TestHistoryUpdate_EnterWhileFilteringFallsThroughToList(t *testing.T) {
	m := NewHistory(sampleHistoryEntries)
	m.list.SetSize(80, 20)
	// Programmatically enter filter mode so we don't depend on the '/' keybind.
	m.list.SetFilterState(list.Filtering)

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	mm := updated.(HistoryModel)
	if mm.Selected() != nil {
		t.Error("Enter while filtering must not select an item")
	}
}

func TestHistoryUpdate_EscQuitsWithoutSelection(t *testing.T) {
	m := NewHistory(sampleHistoryEntries)
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	mm := updated.(HistoryModel)
	if mm.Selected() != nil {
		t.Error("Esc must not select an item")
	}
	if cmd == nil {
		t.Fatal("expected quit cmd")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Errorf("expected QuitMsg, got %T", cmd())
	}
}

func TestHistoryUpdate_CtrlCQuits(t *testing.T) {
	m := NewHistory(sampleHistoryEntries)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Fatal("expected quit cmd")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Errorf("expected QuitMsg, got %T", cmd())
	}
}

func TestHistoryUpdate_WindowSizeResizesList(t *testing.T) {
	m := NewHistory(sampleHistoryEntries)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	mm := updated.(HistoryModel)
	if mm.list.Width() != 100 {
		t.Errorf("list width = %d, want 100", mm.list.Width())
	}
	if mm.list.Height() != 30 {
		t.Errorf("list height = %d, want 30", mm.list.Height())
	}
}

func TestHistoryUpdate_UnhandledKeyForwardedToList(t *testing.T) {
	m := NewHistory(sampleHistoryEntries)
	m.list.SetSize(80, 20)
	// 'j' is the default Down keybinding in bubbles/list.
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	mm := updated.(HistoryModel)
	// We can't easily assert the cursor moved without poking internals; instead
	// verify Selected() wasn't set (i.e. the key wasn't mis-routed to "select").
	if mm.Selected() != nil {
		t.Error("typing 'j' should not select an item")
	}
}

func TestHistoryView_RendersListWhenNonEmpty(t *testing.T) {
	m := NewHistory(sampleHistoryEntries)
	m.list.SetSize(80, 20)
	out := m.View()
	if !strings.Contains(out, "tai history") {
		t.Errorf("View should render the list title, got: %s", out)
	}
}

// Selecting after the user has navigated to a different row should return that
// row's entry. This locks in that we don't always hand back items[0].
func TestHistoryUpdate_EnterAfterNavigationReturnsCurrentRow(t *testing.T) {
	m := NewHistory(sampleHistoryEntries)
	m.list.SetSize(80, 20)
	m.list.Select(1) // move cursor to the second item

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	mm := updated.(HistoryModel)
	if mm.Selected() == nil {
		t.Fatal("Selected() should be set")
	}
	if mm.Selected().Command != "find . -name '*.go'" {
		t.Errorf("selected = %q, want second entry", mm.Selected().Command)
	}
}
