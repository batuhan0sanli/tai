package tui

import (
	"errors"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
)

func newModelForTest(suggested string, mp *mockProvider) Model {
	if mp == nil {
		mp = &mockProvider{}
	}
	return New("user prompt", suggested, mp)
}

func TestNew_InitialState(t *testing.T) {
	m := newModelForTest("ls -la", nil)
	if m.Command() != "ls -la" {
		t.Errorf("Command() = %q, want %q", m.Command(), "ls -la")
	}
	if m.ShouldExecute() {
		t.Error("ShouldExecute() should default to false")
	}
	if m.originalPrompt != "user prompt" {
		t.Errorf("originalPrompt = %q, want %q", m.originalPrompt, "user prompt")
	}
	if !m.input.Focused() {
		t.Error("input should be focused on init")
	}
	if m.loading {
		t.Error("model should not start in loading state")
	}
}

func TestInit_ReturnsTextinputBlink(t *testing.T) {
	m := newModelForTest("ls", nil)
	cmd := m.Init()
	if cmd == nil {
		t.Fatal("Init() returned nil command")
	}
}

func TestUpdate_WindowSizeAdjustsInputWidth(t *testing.T) {
	m := newModelForTest("ls", nil)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	mm := updated.(Model)
	if mm.width != 80 {
		t.Errorf("width = %d, want 80", mm.width)
	}
	if mm.input.Width != 76 { // width - 4
		t.Errorf("input.Width = %d, want 76", mm.input.Width)
	}
}

func TestUpdate_TinyWindowDoesNotPanic(t *testing.T) {
	m := newModelForTest("ls", nil)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 2, Height: 2})
	mm := updated.(Model)
	if mm.width != 2 {
		t.Errorf("width = %d, want 2", mm.width)
	}
	// input width should NOT have been set to a negative number
	if mm.input.Width < 0 {
		t.Errorf("input.Width = %d, want >= 0", mm.input.Width)
	}
}

func TestUpdate_EmptyEnterAcceptsAndQuits(t *testing.T) {
	m := newModelForTest("ls -la", nil)
	// input is empty by default
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	mm := updated.(Model)
	if !mm.ShouldExecute() {
		t.Error("ShouldExecute() should be true after empty Enter")
	}
	if cmd == nil {
		t.Fatal("expected tea.Quit cmd, got nil")
	}
	if got, ok := cmd().(tea.QuitMsg); !ok {
		t.Errorf("expected tea.QuitMsg, got %T (%v)", cmd(), got)
	}
}

func TestUpdate_WhitespaceEnterStillAccepts(t *testing.T) {
	m := newModelForTest("ls", nil)
	m.input.SetValue("   \t  ")
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	mm := updated.(Model)
	if !mm.ShouldExecute() {
		t.Error("whitespace-only input should be treated as empty and accept")
	}
	if cmd == nil {
		t.Fatal("expected tea.Quit cmd")
	}
}

func TestUpdate_NonEmptyEnterTriggersRevisionAndShowsSpinner(t *testing.T) {
	mp := &mockProvider{defaultResp: "new command"}
	m := newModelForTest("old command", mp)
	m.input.SetValue("make it recursive")

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	mm := updated.(Model)
	if !mm.loading {
		t.Error("loading should be true after revision Enter")
	}
	if mm.ShouldExecute() {
		t.Error("ShouldExecute should still be false during revision")
	}
	if cmd == nil {
		t.Fatal("expected a tea.Cmd for the async revision + spinner tick")
	}

	// Drain the batched cmd. tea.Batch returns a cmd that produces a BatchMsg
	// containing the underlying cmds — execute it to surface the revision msg.
	msg := cmd()
	switch v := msg.(type) {
	case aiResponseMsg:
		// Single cmd path (no batch wrapping)
		if v.command != "new command" {
			t.Errorf("revision returned %q, want %q", v.command, "new command")
		}
	case tea.BatchMsg:
		found := false
		for _, sub := range v {
			out := sub()
			if r, ok := out.(aiResponseMsg); ok {
				found = true
				if r.command != "new command" {
					t.Errorf("revision returned %q, want %q", r.command, "new command")
				}
			}
		}
		if !found {
			t.Error("BatchMsg did not contain an aiResponseMsg")
		}
	default:
		// Some versions surface other wrappers; verify the provider was called.
		if mp.callsMade() != 1 {
			t.Errorf("provider was not invoked exactly once (calls=%d, msg=%T)", mp.callsMade(), msg)
		}
	}
}

func TestUpdate_RevisionPromptIncludesAllThreeFields(t *testing.T) {
	mp := &mockProvider{defaultResp: "ls -laR"}
	m := newModelForTest("ls -la", mp)
	m.input.SetValue("make it recursive")

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected revision cmd")
	}
	// Drain the batch to actually execute the provider call.
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, sub := range batch {
			sub()
		}
	}

	calls := mp.callsSnapshot()
	if len(calls) == 0 {
		t.Fatal("provider was never called")
	}
	got := calls[0]
	for _, want := range []string{"user prompt", "ls -la", "make it recursive"} {
		if !strings.Contains(got, want) {
			t.Errorf("revision prompt missing %q\nfull prompt: %s", want, got)
		}
	}
}

func TestUpdate_AIResponseSuccessUpdatesCommand(t *testing.T) {
	m := newModelForTest("old command", nil)
	m.loading = true
	m.input.SetValue("some revision text")

	updated, _ := m.Update(aiResponseMsg{command: "new command", err: nil})
	mm := updated.(Model)
	if mm.loading {
		t.Error("loading should be cleared after response")
	}
	if mm.Command() != "new command" {
		t.Errorf("Command() = %q, want %q", mm.Command(), "new command")
	}
	if mm.input.Value() != "" {
		t.Errorf("input should be cleared, got %q", mm.input.Value())
	}
	if mm.err != nil {
		t.Errorf("err should be nil, got %v", mm.err)
	}
}

func TestUpdate_AIResponseErrorKeepsOldCommand(t *testing.T) {
	m := newModelForTest("old command", nil)
	m.loading = true
	m.input.SetValue("revision text")

	wantErr := errors.New("model unavailable")
	updated, _ := m.Update(aiResponseMsg{err: wantErr})
	mm := updated.(Model)
	if mm.loading {
		t.Error("loading should be cleared on error")
	}
	if mm.Command() != "old command" {
		t.Errorf("command should be unchanged on error, got %q", mm.Command())
	}
	if mm.err == nil || mm.err.Error() != "model unavailable" {
		t.Errorf("err = %v, want %v", mm.err, wantErr)
	}
	// Input is preserved so the user can fix and retry.
	if mm.input.Value() != "revision text" {
		t.Errorf("input should be preserved on error, got %q", mm.input.Value())
	}
}

func TestUpdate_CtrlCQuits(t *testing.T) {
	m := newModelForTest("ls", nil)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Fatal("expected quit cmd")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Errorf("expected tea.QuitMsg, got %T", cmd())
	}
}

func TestUpdate_EscQuits(t *testing.T) {
	m := newModelForTest("ls", nil)
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	mm := updated.(Model)
	if mm.ShouldExecute() {
		t.Error("Esc should not set ShouldExecute")
	}
	if cmd == nil {
		t.Fatal("expected quit cmd")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Errorf("expected tea.QuitMsg, got %T", cmd())
	}
}

func TestUpdate_EnterWhileLoadingIsNoop(t *testing.T) {
	mp := &mockProvider{defaultResp: "x"}
	m := newModelForTest("ls", mp)
	m.loading = true

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	mm := updated.(Model)
	if !mm.loading {
		t.Error("loading should remain true when Enter is pressed mid-flight")
	}
	if cmd != nil {
		t.Errorf("expected nil cmd while loading, got %T", cmd)
	}
	if mm.ShouldExecute() {
		t.Error("Enter while loading must not accept the command")
	}
	if mp.callsMade() != 0 {
		t.Errorf("provider must not be re-invoked, got %d calls", mp.callsMade())
	}
}

func TestUpdate_SpinnerTickProgressesOnlyWhileLoading(t *testing.T) {
	m := newModelForTest("ls", nil)
	// not loading
	_, cmd := m.Update(spinner.TickMsg{})
	if cmd != nil {
		t.Errorf("spinner tick must be a no-op when not loading, got %T", cmd)
	}

	m.loading = true
	_, cmd = m.Update(spinner.TickMsg{})
	if cmd == nil {
		t.Error("spinner tick should produce a follow-up cmd while loading")
	}
}

func TestView_RendersCommandAndHelp(t *testing.T) {
	m := newModelForTest("ls -la", nil)
	m.width = 80
	out := m.View()
	for _, want := range []string{"tai suggestion", "ls -la", "enter", "esc"} {
		if !strings.Contains(out, want) {
			t.Errorf("View() missing %q\n--- output ---\n%s", want, out)
		}
	}
}

func TestView_RendersErrorWhenSet(t *testing.T) {
	m := newModelForTest("ls", nil)
	m.err = errors.New("kaboom")
	out := m.View()
	if !strings.Contains(out, "kaboom") {
		t.Errorf("View() should render error text, got:\n%s", out)
	}
}

func TestView_RendersLoadingStateInsteadOfPrompt(t *testing.T) {
	m := newModelForTest("ls", nil)
	m.loading = true
	out := m.View()
	if !strings.Contains(out, "revising") {
		t.Errorf("View() should show 'revising' when loading, got:\n%s", out)
	}
	if strings.Contains(out, "Revise the command") {
		t.Error("View() must not show the revise-prompt label while loading")
	}
}

func TestUpdate_TextInputReceivesUnhandledKeys(t *testing.T) {
	m := newModelForTest("ls", nil)
	// Typing a rune key while not loading should land in the textinput.
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	mm := updated.(Model)
	if mm.input.Value() != "x" {
		t.Errorf("textinput did not receive key, value = %q", mm.input.Value())
	}
}

func TestUpdate_TextInputIgnoredWhileLoading(t *testing.T) {
	m := newModelForTest("ls", nil)
	m.input.SetValue("preexisting")
	m.loading = true
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'z'}})
	mm := updated.(Model)
	if mm.input.Value() != "preexisting" {
		t.Errorf("input should not change while loading, got %q", mm.input.Value())
	}
}

func TestReviseCmd_FormatsCombinedPrompt(t *testing.T) {
	mp := &mockProvider{defaultResp: "out"}
	cmd := reviseCmd(mp, "orig prompt", "current cmd", "do X")
	msg := cmd().(aiResponseMsg)
	if msg.err != nil {
		t.Fatalf("unexpected err: %v", msg.err)
	}
	if msg.command != "out" {
		t.Errorf("got %q, want %q", msg.command, "out")
	}
	calls := mp.callsSnapshot()
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}
	for _, want := range []string{"orig prompt", "current cmd", "do X"} {
		if !strings.Contains(calls[0], want) {
			t.Errorf("combined prompt missing %q\nfull: %s", want, calls[0])
		}
	}
}

func TestReviseCmd_PropagatesProviderError(t *testing.T) {
	wantErr := errors.New("network down")
	mp := &mockProvider{defaultErr: wantErr}
	cmd := reviseCmd(mp, "orig", "cur", "rev")
	msg := cmd().(aiResponseMsg)
	if msg.err == nil || msg.err.Error() != "network down" {
		t.Errorf("err = %v, want %v", msg.err, wantErr)
	}
}
