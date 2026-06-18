package tui

import (
	"strings"
	"testing"

	"tai/internal/config"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
)

var (
	kUp       = tea.KeyMsg{Type: tea.KeyUp}
	kDown     = tea.KeyMsg{Type: tea.KeyDown}
	kEnter    = tea.KeyMsg{Type: tea.KeyEnter}
	kEsc      = tea.KeyMsg{Type: tea.KeyEsc}
	kCtrlC    = tea.KeyMsg{Type: tea.KeyCtrlC}
	kTab      = tea.KeyMsg{Type: tea.KeyTab}
	kShiftTab = tea.KeyMsg{Type: tea.KeyShiftTab}
)

func kRunes(s string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }

// cfgStep applies one message and returns the resulting ConfigModel.
func cfgStep(t *testing.T, m ConfigModel, msg tea.Msg) ConfigModel {
	t.Helper()
	next, _ := m.Update(msg)
	cm, ok := next.(ConfigModel)
	if !ok {
		t.Fatalf("Update returned %T, want ConfigModel", next)
	}
	return cm
}

func cfgStepCmd(t *testing.T, m ConfigModel, msg tea.Msg) (ConfigModel, tea.Cmd) {
	t.Helper()
	next, cmd := m.Update(msg)
	cm, ok := next.(ConfigModel)
	if !ok {
		t.Fatalf("Update returned %T, want ConfigModel", next)
	}
	return cm, cmd
}

func sampleConfig() config.Config {
	return config.Config{
		DefaultProvider: "claude-code",
		Providers: map[string]config.ProviderConfig{
			"claude-code": {Type: config.TypeCLI, Command: "claude", Args: []string{"-p"}},
			"openai":      {Type: config.TypeOpenAI, Model: "gpt-4o", BaseURL: "https://api.openai.com/v1"},
		},
	}
}

func TestNewConfig_SortsNamesAndStartsOnList(t *testing.T) {
	m := NewConfig(sampleConfig())
	if len(m.names) != 2 || m.names[0] != "claude-code" || m.names[1] != "openai" {
		t.Fatalf("names = %v, want sorted [claude-code openai]", m.names)
	}
	if m.mode != modeList {
		t.Errorf("mode = %v, want modeList", m.mode)
	}
	if m.Init() != nil {
		t.Error("Init should return nil")
	}
}

func TestConfig_ListNavigationBounds(t *testing.T) {
	m := NewConfig(sampleConfig())
	m = cfgStep(t, m, kUp) // already at top, stays
	if m.cursor != 0 {
		t.Fatalf("cursor = %d, want 0", m.cursor)
	}
	m = cfgStep(t, m, kDown)
	if m.cursor != 1 {
		t.Fatalf("cursor = %d, want 1", m.cursor)
	}
	m = cfgStep(t, m, kDown) // at bottom, stays
	if m.cursor != 1 {
		t.Fatalf("cursor = %d, want 1 (clamped)", m.cursor)
	}
	m = cfgStep(t, m, kRunes("k")) // vim up
	if m.cursor != 0 {
		t.Fatalf("cursor = %d, want 0", m.cursor)
	}
	m = cfgStep(t, m, kRunes("j")) // vim down
	if m.cursor != 1 {
		t.Fatalf("cursor = %d, want 1", m.cursor)
	}
}

func TestConfig_SetDefault(t *testing.T) {
	m := NewConfig(sampleConfig())
	m = cfgStep(t, m, kDown)       // move to openai
	m = cfgStep(t, m, kRunes("d")) // set default
	if m.cfg.DefaultProvider != "openai" {
		t.Errorf("default provider = %q, want openai", m.cfg.DefaultProvider)
	}
}

func TestConfig_SaveQuit(t *testing.T) {
	m := NewConfig(sampleConfig())
	m, cmd := cfgStepCmd(t, m, kRunes("s"))
	if !m.Saved() {
		t.Error("expected Saved() to be true after 's'")
	}
	if cmd == nil {
		t.Error("expected a quit command after 's'")
	}
}

func TestConfig_QuitWithoutSaving(t *testing.T) {
	for _, msg := range []tea.Msg{kRunes("q"), kEsc, kCtrlC} {
		m := NewConfig(sampleConfig())
		m, cmd := cfgStepCmd(t, m, msg)
		if m.Saved() {
			t.Errorf("msg %v should not save", msg)
		}
		if !m.quit {
			t.Errorf("msg %v should set quit", msg)
		}
		if cmd == nil {
			t.Errorf("msg %v should return quit command", msg)
		}
	}
}

func TestConfig_EnterBuildsDetailInputs(t *testing.T) {
	m := NewConfig(sampleConfig())
	m = cfgStep(t, m, kEnter) // open claude-code (cli)
	if m.mode != modeDetail {
		t.Fatalf("mode = %v, want modeDetail", m.mode)
	}
	if len(m.inputs) != 3 {
		t.Fatalf("cli provider should have 3 fields, got %d (%v)", len(m.inputs), m.fields)
	}
	if m.inputs[0].Value() != "claude" {
		t.Errorf("Command input = %q, want claude", m.inputs[0].Value())
	}
	if m.inputs[1].Value() != "-p" {
		t.Errorf("Args input = %q, want -p", m.inputs[1].Value())
	}
}

func TestConfig_DetailFieldNavigationWraps(t *testing.T) {
	m := NewConfig(sampleConfig())
	m = cfgStep(t, m, kEnter) // claude-code: 3 fields
	if m.focus != 0 {
		t.Fatalf("focus = %d, want 0", m.focus)
	}
	m = cfgStep(t, m, kDown)
	if m.focus != 1 {
		t.Fatalf("focus = %d, want 1", m.focus)
	}
	m = cfgStep(t, m, kTab)
	if m.focus != 2 {
		t.Fatalf("focus = %d, want 2", m.focus)
	}
	m = cfgStep(t, m, kDown) // wrap to 0
	if m.focus != 0 {
		t.Fatalf("focus = %d, want 0 (wrap)", m.focus)
	}
	m = cfgStep(t, m, kUp) // wrap to 2
	if m.focus != 2 {
		t.Fatalf("focus = %d, want 2 (wrap)", m.focus)
	}
	m = cfgStep(t, m, kShiftTab)
	if m.focus != 1 {
		t.Fatalf("focus = %d, want 1", m.focus)
	}
}

func TestConfig_EditModelAndCommitViaEsc(t *testing.T) {
	m := NewConfig(sampleConfig())
	m = cfgStep(t, m, kDown)  // openai
	m = cfgStep(t, m, kEnter) // detail; field 0 = Model = "gpt-4o"
	m = cfgStep(t, m, kRunes("X"))
	m = cfgStep(t, m, kEsc) // commit back to list
	if m.mode != modeList {
		t.Fatalf("mode = %v, want modeList after esc", m.mode)
	}
	if got := m.cfg.Providers["openai"].Model; got != "gpt-4oX" {
		t.Errorf("openai model = %q, want gpt-4oX", got)
	}
}

func TestConfig_EnterAdvancesThenCommitsOnLastField(t *testing.T) {
	m := NewConfig(sampleConfig())
	m = cfgStep(t, m, kEnter) // claude-code detail, focus 0
	m = cfgStep(t, m, kEnter) // -> focus 1
	if m.focus != 1 {
		t.Fatalf("focus = %d, want 1", m.focus)
	}
	m = cfgStep(t, m, kEnter) // -> focus 2 (last)
	if m.focus != 2 {
		t.Fatalf("focus = %d, want 2", m.focus)
	}
	m = cfgStep(t, m, kEnter) // on last field -> commit
	if m.mode != modeList {
		t.Errorf("mode = %v, want modeList after enter on last field", m.mode)
	}
}

func TestConfig_DetailCtrlCQuits(t *testing.T) {
	m := NewConfig(sampleConfig())
	m = cfgStep(t, m, kEnter)
	m, cmd := cfgStepCmd(t, m, kCtrlC)
	if !m.quit || cmd == nil {
		t.Errorf("ctrl+c in detail should quit; quit=%v cmd=%v", m.quit, cmd)
	}
}

func TestConfig_DetailTypingEditsFocusedInput(t *testing.T) {
	m := NewConfig(sampleConfig())
	m = cfgStep(t, m, kDown)  // openai
	m = cfgStep(t, m, kEnter) // Model field focused
	m = cfgStep(t, m, kRunes("9"))
	if !strings.HasSuffix(m.inputs[0].Value(), "9") {
		t.Errorf("focused input not edited, got %q", m.inputs[0].Value())
	}
}

func TestConfig_EmptyProvidersAreNoOps(t *testing.T) {
	m := NewConfig(config.Config{Providers: map[string]config.ProviderConfig{}})
	// nav / default / enter must not panic or change mode
	m = cfgStep(t, m, kDown)
	m = cfgStep(t, m, kUp)
	m = cfgStep(t, m, kRunes("d"))
	m = cfgStep(t, m, kEnter)
	if m.mode != modeList {
		t.Errorf("mode = %v, want modeList (no providers to edit)", m.mode)
	}
	if m.cfg.DefaultProvider != "" {
		t.Errorf("default should stay empty, got %q", m.cfg.DefaultProvider)
	}
}

func TestConfig_DetailWithNoInputsFallsThrough(t *testing.T) {
	// Defensive: force detail mode with no inputs and send a key/nav.
	m := NewConfig(sampleConfig())
	m.mode = modeDetail
	m.inputs = nil
	m.fields = nil
	m = cfgStep(t, m, kRunes("z")) // hits the len(inputs)==0 fallthrough
	m = cfgStep(t, m, kDown)       // focusDelta early-returns
	if m.mode != modeDetail {
		t.Errorf("mode = %v, want modeDetail", m.mode)
	}
}

func TestConfig_WindowSizeSetsWidth(t *testing.T) {
	m := NewConfig(sampleConfig())
	m = cfgStep(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	if m.width != 100 {
		t.Errorf("width = %d, want 100", m.width)
	}
}

func TestConfig_NonKeyMsgIgnored(t *testing.T) {
	m := NewConfig(sampleConfig())
	before := m.cursor
	m = cfgStep(t, m, spinner.TickMsg{})
	if m.cursor != before {
		t.Error("non-key message should not change state")
	}
}

func TestConfig_ListView(t *testing.T) {
	m := NewConfig(sampleConfig())
	v := m.View()
	for _, want := range []string{"tai config", "claude-code", "openai", "★", "save & quit"} {
		if !strings.Contains(v, want) {
			t.Errorf("list view missing %q\n%s", want, v)
		}
	}
}

func TestConfig_ListViewEmpty(t *testing.T) {
	m := NewConfig(config.Config{Providers: map[string]config.ProviderConfig{}})
	if !strings.Contains(m.View(), "no providers configured") {
		t.Errorf("empty list view should hint at no providers:\n%s", m.View())
	}
}

func TestConfig_DetailView(t *testing.T) {
	m := NewConfig(sampleConfig())
	m = cfgStep(t, m, kDown)  // openai
	m = cfgStep(t, m, kEnter) // detail
	v := m.View()
	for _, want := range []string{"edit provider: openai", "Model:", "Base URL:", "API Key:", "back to list"} {
		if !strings.Contains(v, want) {
			t.Errorf("detail view missing %q\n%s", want, v)
		}
	}
}

func TestConfig_PureHelpers(t *testing.T) {
	if got := modelOrDash(""); got != "-" {
		t.Errorf("modelOrDash(\"\") = %q, want -", got)
	}
	if got := modelOrDash("x"); got != "x" {
		t.Errorf("modelOrDash(x) = %q, want x", got)
	}

	cli := config.ProviderConfig{Type: config.TypeCLI}
	if got := fieldsFor(cli); strings.Join(got, ",") != "Command,Args,Model" {
		t.Errorf("cli fields = %v", got)
	}
	api := config.ProviderConfig{Type: config.TypeOpenAI}
	if got := fieldsFor(api); strings.Join(got, ",") != "Model,Base URL,API Key,API Key Env" {
		t.Errorf("api fields = %v", got)
	}

	pc := config.ProviderConfig{
		Type: config.TypeOpenAI, Model: "m", BaseURL: "u", APIKey: "k", APIKeyEnv: "e",
		Command: "c", Args: []string{"a", "b"},
	}
	if valueFor(pc, "Command") != "c" || valueFor(pc, "Args") != "a b" ||
		valueFor(pc, "Model") != "m" || valueFor(pc, "Base URL") != "u" ||
		valueFor(pc, "API Key") != "k" || valueFor(pc, "API Key Env") != "e" ||
		valueFor(pc, "Unknown") != "" {
		t.Error("valueFor returned unexpected value")
	}

	got := applyField(config.ProviderConfig{}, "Args", "x  y")
	if len(got.Args) != 2 || got.Args[0] != "x" || got.Args[1] != "y" {
		t.Errorf("applyField Args = %v", got.Args)
	}
	if got := applyField(config.ProviderConfig{Args: []string{"x"}}, "Args", "   "); got.Args != nil {
		t.Errorf("blank Args should clear to nil, got %v", got.Args)
	}
	got = applyField(config.ProviderConfig{}, "Command", "cc")
	got = applyField(got, "Model", "mm")
	got = applyField(got, "Base URL", "uu")
	got = applyField(got, "API Key", "kk")
	got = applyField(got, "API Key Env", "ee")
	got = applyField(got, "Unknown", "ignored")
	if got.Command != "cc" || got.Model != "mm" || got.BaseURL != "uu" || got.APIKey != "kk" || got.APIKeyEnv != "ee" {
		t.Errorf("applyField set unexpected fields: %+v", got)
	}
}
