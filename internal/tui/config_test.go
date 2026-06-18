package tui

import (
	"errors"
	"net/http"
	"net/http/httptest"
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

// openDetailOnOpenAI navigates to the openai provider and opens its detail
// screen (focus 0 = Model), with fetch overridden.
func openDetailOnOpenAI(t *testing.T, fetch func(config.ProviderConfig) ([]string, error)) ConfigModel {
	t.Helper()
	m := NewConfig(sampleConfig())
	m.fetch = fetch
	m = cfgStep(t, m, kDown)  // openai
	m = cfgStep(t, m, kEnter) // detail; field 0 = Model
	return m
}

func TestConfig_ModelPicker_FetchAndSelect(t *testing.T) {
	m := openDetailOnOpenAI(t, func(config.ProviderConfig) ([]string, error) {
		return []string{"gpt-4o", "gpt-4o-mini"}, nil
	})
	m, cmd := cfgStepCmd(t, m, kEnter) // open picker on Model field
	if m.mode != modeModels || !m.loading {
		t.Fatalf("expected modeModels+loading, got mode=%v loading=%v", m.mode, m.loading)
	}
	if cmd == nil {
		t.Fatal("expected a fetch command")
	}
	// Run the fetch command and feed its message back in.
	m = cfgStep(t, m, cmd())
	if m.loading {
		t.Fatal("loading should clear after modelsMsg")
	}
	if len(m.models) != 2 {
		t.Fatalf("models = %v, want 2", m.models)
	}
	// The loaded-list view (via the top-level View dispatch) renders both rows.
	if v := m.View(); !strings.Contains(v, "gpt-4o") || !strings.Contains(v, "enter: select") {
		t.Errorf("models list view missing rows/help:\n%s", v)
	}
	m = cfgStep(t, m, kDown) // select gpt-4o-mini
	if m.modelCursor != 1 {
		t.Fatalf("modelCursor = %d, want 1", m.modelCursor)
	}
	m = cfgStep(t, m, kEnter) // pick it
	if m.mode != modeDetail {
		t.Fatalf("mode = %v, want modeDetail after selecting", m.mode)
	}
	if m.inputs[0].Value() != "gpt-4o-mini" {
		t.Errorf("Model input = %q, want gpt-4o-mini", m.inputs[0].Value())
	}
}

func TestConfig_ModelPicker_EscCancels(t *testing.T) {
	m := openDetailOnOpenAI(t, func(config.ProviderConfig) ([]string, error) {
		return []string{"gpt-4o"}, nil
	})
	m, cmd := cfgStepCmd(t, m, kEnter)
	m = cfgStep(t, m, cmd())
	m = cfgStep(t, m, kEsc) // cancel without selecting
	if m.mode != modeDetail {
		t.Fatalf("mode = %v, want modeDetail after esc", m.mode)
	}
	if m.inputs[0].Value() != "gpt-4o" && m.inputs[0].Value() != "gpt-4o" {
		// model field keeps its original value (gpt-4o from sampleConfig)
		t.Errorf("Model input should be unchanged, got %q", m.inputs[0].Value())
	}
}

func TestConfig_ModelPicker_FetchError(t *testing.T) {
	m := openDetailOnOpenAI(t, func(config.ProviderConfig) ([]string, error) {
		return nil, errors.New("401 unauthorized")
	})
	m, cmd := cfgStepCmd(t, m, kEnter)
	m = cfgStep(t, m, cmd())
	if m.modelsErr == nil {
		t.Fatal("expected modelsErr to be set")
	}
	if !strings.Contains(m.modelsView(), "401 unauthorized") {
		t.Errorf("models view should show the error:\n%s", m.modelsView())
	}
	m = cfgStep(t, m, kEsc)
	if m.mode != modeDetail {
		t.Errorf("esc should return to detail, got %v", m.mode)
	}
}

func TestConfig_ModelPicker_EmptyResult(t *testing.T) {
	m := openDetailOnOpenAI(t, func(config.ProviderConfig) ([]string, error) {
		return nil, nil
	})
	m, cmd := cfgStepCmd(t, m, kEnter)
	m = cfgStep(t, m, cmd())
	if !strings.Contains(m.modelsView(), "no models returned") {
		t.Errorf("expected empty-models hint:\n%s", m.modelsView())
	}
	m = cfgStep(t, m, kEnter) // enter with no models -> back to detail, no change
	if m.mode != modeDetail {
		t.Errorf("mode = %v, want modeDetail", m.mode)
	}
}

func TestConfig_ModelPicker_LoadingIgnoresNavAndCtrlCQuits(t *testing.T) {
	m := openDetailOnOpenAI(t, func(config.ProviderConfig) ([]string, error) { return []string{"a"}, nil })
	m, _ = cfgStepCmd(t, m, kEnter) // now loading
	before := m.modelCursor
	m = cfgStep(t, m, kDown) // ignored while loading
	if m.modelCursor != before {
		t.Error("navigation should be ignored while loading")
	}
	if !strings.Contains(m.modelsView(), "fetching available models") {
		t.Errorf("loading view missing spinner text:\n%s", m.modelsView())
	}
	m, cmd := cfgStepCmd(t, m, kCtrlC)
	if !m.quit || cmd == nil {
		t.Errorf("ctrl+c should quit from the picker")
	}
}

func TestConfig_ModelPicker_NavBounds(t *testing.T) {
	m := openDetailOnOpenAI(t, func(config.ProviderConfig) ([]string, error) {
		return []string{"a", "b"}, nil
	})
	m, cmd := cfgStepCmd(t, m, kEnter)
	m = cfgStep(t, m, cmd())
	m = cfgStep(t, m, kUp) // already at 0
	if m.modelCursor != 0 {
		t.Fatalf("cursor = %d, want 0", m.modelCursor)
	}
	m = cfgStep(t, m, kRunes("j")) // down via vim
	m = cfgStep(t, m, kRunes("j")) // clamp at last
	if m.modelCursor != 1 {
		t.Fatalf("cursor = %d, want 1 (clamped)", m.modelCursor)
	}
	m = cfgStep(t, m, kRunes("k")) // up via vim
	if m.modelCursor != 0 {
		t.Fatalf("cursor = %d, want 0", m.modelCursor)
	}
}

func TestConfig_EnterOnNonModelFieldDoesNotOpenPicker(t *testing.T) {
	m := openDetailOnOpenAI(t, func(config.ProviderConfig) ([]string, error) {
		t.Fatal("fetch must not be called from a non-Model field")
		return nil, nil
	})
	m = cfgStep(t, m, kDown)  // move off Model to Base URL
	m = cfgStep(t, m, kEnter) // should advance, not open picker
	if m.mode != modeDetail {
		t.Errorf("mode = %v, want modeDetail", m.mode)
	}
}

func TestConfig_CLIModelFieldDoesNotOpenPicker(t *testing.T) {
	// claude-code is a cli provider; its Model field (last) should commit on
	// enter rather than open the picker.
	m := NewConfig(sampleConfig())
	m.fetch = func(config.ProviderConfig) ([]string, error) {
		t.Fatal("cli provider must not fetch models")
		return nil, nil
	}
	m = cfgStep(t, m, kEnter) // claude-code detail
	m = cfgStep(t, m, kEnter) // -> Args
	m = cfgStep(t, m, kEnter) // -> Model (last)
	m = cfgStep(t, m, kEnter) // commit
	if m.mode != modeList {
		t.Errorf("mode = %v, want modeList (cli Model enter commits)", m.mode)
	}
}

func TestDefaultFetchModels(t *testing.T) {
	t.Run("provider-new-error", func(t *testing.T) {
		if _, err := defaultFetchModels(config.ProviderConfig{}); err == nil {
			t.Fatal("expected error for empty provider type")
		}
	})
	t.Run("not-a-lister", func(t *testing.T) {
		_, err := defaultFetchModels(config.ProviderConfig{Type: config.TypeCLI, Command: "claude"})
		if err == nil || !strings.Contains(err.Error(), "can't list models") {
			t.Fatalf("expected can't-list error, got %v", err)
		}
	})
	t.Run("success", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(`{"data":[{"id":"gpt-4o"}]}`))
		}))
		defer srv.Close()
		got, err := defaultFetchModels(config.ProviderConfig{Type: config.TypeOpenAI, BaseURL: srv.URL})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 1 || got[0] != "gpt-4o" {
			t.Errorf("models = %v, want [gpt-4o]", got)
		}
	})
}

func TestConfig_DetailViewShowsModelListHint(t *testing.T) {
	m := openDetailOnOpenAI(t, defaultFetchModels)
	if !strings.Contains(m.View(), "(enter to list)") {
		t.Errorf("API-provider detail should hint at the model list:\n%s", m.View())
	}
}

func TestConfig_AccessorsAndIsAPIType(t *testing.T) {
	m := NewConfig(sampleConfig())
	if m.Config().DefaultProvider != "claude-code" {
		t.Errorf("Config() default = %q, want claude-code", m.Config().DefaultProvider)
	}
	if m.Saved() {
		t.Error("Saved() should be false on a fresh model")
	}
	for _, tc := range []struct {
		typ  string
		want bool
	}{
		{config.TypeOpenAI, true},
		{config.TypeGemini, true},
		{config.TypeAnthropic, true},
		{config.TypeCLI, false},
		{"mystery", false},
	} {
		if got := isAPIType(tc.typ); got != tc.want {
			t.Errorf("isAPIType(%q) = %v, want %v", tc.typ, got, tc.want)
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
