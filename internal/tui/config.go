package tui

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"tai/internal/config"
	"tai/internal/provider"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// configMode is which screen the config editor is on.
type configMode int

const (
	modeList   configMode = iota // browsing the provider list
	modeDetail                   // editing one provider's fields
	modeModels                   // picking a model from the provider's API
)

// modelsMsg carries the result of an async model-list fetch.
type modelsMsg struct {
	models []string
	err    error
}

// defaultFetchModels builds the provider from pc and asks it for its model
// list. Returns a clear error for providers that can't enumerate models (CLI
// backends), so the picker can tell the user to type one manually.
func defaultFetchModels(pc config.ProviderConfig) ([]string, error) {
	p, err := provider.New(pc)
	if err != nil {
		return nil, err
	}
	lister, ok := p.(provider.ModelLister)
	if !ok {
		return nil, fmt.Errorf("the %q provider can't list models — type one manually", pc.Type)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return lister.ListModels(ctx)
}

// isAPIType reports whether a provider type can fetch a model list.
func isAPIType(t string) bool {
	return t == config.TypeOpenAI || t == config.TypeGemini || t == config.TypeAnthropic
}

// ConfigModel is the Bubble Tea state for the interactive `tai config edit`
// editor. The list screen lets the user pick a default and drill into a
// provider; the detail screen edits that provider's fields.
type ConfigModel struct {
	cfg    config.Config
	names  []string // provider names, sorted for stable display
	cursor int      // selected provider in the list
	mode   configMode

	// Detail-screen state, rebuilt each time a provider is opened.
	fields []string          // field labels for the open provider
	inputs []textinput.Model // parallel to fields
	focus  int               // focused field index

	// Model-picker state.
	fetch       func(config.ProviderConfig) ([]string, error) // injectable for tests
	loading     bool
	models      []string
	modelsErr   error
	modelCursor int

	saved bool // user chose "save & quit"
	quit  bool // user quit without saving
	width int
}

// NewConfig builds a ConfigModel from cfg, starting on the provider list.
func NewConfig(cfg config.Config) ConfigModel {
	names := make([]string, 0, len(cfg.Providers))
	for name := range cfg.Providers {
		names = append(names, name)
	}
	sort.Strings(names)
	return ConfigModel{cfg: cfg, names: names, mode: modeList, fetch: defaultFetchModels}
}

func (m ConfigModel) Init() tea.Cmd { return nil }

// fieldsFor returns the editable field labels for a provider, tailored to its
// type so users only see fields that matter for that backend.
func fieldsFor(pc config.ProviderConfig) []string {
	if pc.Type == config.TypeCLI {
		return []string{"Command", "Args", "Model"}
	}
	return []string{"Model", "Base URL", "API Key", "API Key Env"}
}

func valueFor(pc config.ProviderConfig, label string) string {
	switch label {
	case "Command":
		return pc.Command
	case "Args":
		return strings.Join(pc.Args, " ")
	case "Model":
		return pc.Model
	case "Base URL":
		return pc.BaseURL
	case "API Key":
		return pc.APIKey
	case "API Key Env":
		return pc.APIKeyEnv
	}
	return ""
}

func applyField(pc config.ProviderConfig, label, val string) config.ProviderConfig {
	switch label {
	case "Command":
		pc.Command = val
	case "Args":
		if strings.TrimSpace(val) == "" {
			pc.Args = nil
		} else {
			pc.Args = strings.Fields(val)
		}
	case "Model":
		pc.Model = val
	case "Base URL":
		pc.BaseURL = val
	case "API Key":
		pc.APIKey = val
	case "API Key Env":
		pc.APIKeyEnv = val
	}
	return pc
}

// enterDetail builds the field inputs for the selected provider and switches to
// the detail screen.
func (m *ConfigModel) enterDetail() {
	pc := m.cfg.Providers[m.names[m.cursor]]
	m.fields = fieldsFor(pc)
	m.inputs = make([]textinput.Model, len(m.fields))
	for i, label := range m.fields {
		ti := textinput.New()
		ti.Prompt = "› "
		ti.CharLimit = 512
		ti.SetValue(valueFor(pc, label))
		if label == "API Key" {
			ti.EchoMode = textinput.EchoPassword
		}
		m.inputs[i] = ti
	}
	m.focus = 0
	if len(m.inputs) > 0 {
		m.inputs[0].Focus()
	}
	m.mode = modeDetail
}

// commitDetail writes the edited inputs back into the working config and
// returns to the list screen.
func (m *ConfigModel) commitDetail() {
	m.cfg.Providers[m.names[m.cursor]] = m.currentPC()
	m.mode = modeList
}

// currentPC folds the live detail inputs into the selected provider's config
// without changing screens — used to fetch models with the just-typed key/URL.
func (m ConfigModel) currentPC() config.ProviderConfig {
	pc := m.cfg.Providers[m.names[m.cursor]]
	for i, label := range m.fields {
		pc = applyField(pc, label, m.inputs[i].Value())
	}
	return pc
}

// setModelValue writes val into the Model input on the detail screen.
func (m *ConfigModel) setModelValue(val string) {
	for i, label := range m.fields {
		if label == "Model" {
			m.inputs[i].SetValue(val)
			return
		}
	}
}

// fetchModelsCmd runs the (injectable) fetcher off the UI thread.
func fetchModelsCmd(fetch func(config.ProviderConfig) ([]string, error), pc config.ProviderConfig) tea.Cmd {
	return func() tea.Msg {
		models, err := fetch(pc)
		return modelsMsg{models: models, err: err}
	}
}

// focusDelta moves the detail-screen focus by d, wrapping around. Callers
// guarantee len(inputs) > 0 (updateDetail guards before calling).
func (m *ConfigModel) focusDelta(d int) {
	m.inputs[m.focus].Blur()
	m.focus = (m.focus + d + len(m.inputs)) % len(m.inputs)
	m.inputs[m.focus].Focus()
}

func (m ConfigModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		return m, nil
	case modelsMsg:
		m.loading = false
		m.models = msg.models
		m.modelsErr = msg.err
		m.modelCursor = 0
		return m, nil
	case tea.KeyMsg:
		switch m.mode {
		case modeList:
			return m.updateList(msg)
		case modeModels:
			return m.updateModels(msg)
		default:
			return m.updateDetail(msg)
		}
	}
	return m, nil
}

func (m ConfigModel) updateList(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "q", "esc":
		m.quit = true
		return m, tea.Quit
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < len(m.names)-1 {
			m.cursor++
		}
	case "d":
		if len(m.names) > 0 {
			m.cfg.DefaultProvider = m.names[m.cursor]
		}
	case "enter":
		if len(m.names) > 0 {
			m.enterDetail()
		}
	case "s":
		m.saved = true
		return m, tea.Quit
	}
	return m, nil
}

func (m ConfigModel) updateDetail(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		m.quit = true
		return m, tea.Quit
	case "esc":
		m.commitDetail()
		return m, nil
	}

	if len(m.inputs) == 0 {
		return m, nil
	}

	switch msg.String() {
	case "up", "shift+tab":
		m.focusDelta(-1)
		return m, nil
	case "down", "tab":
		m.focusDelta(1)
		return m, nil
	case "enter":
		// Enter on the Model field of an API provider opens the model picker.
		if m.fields[m.focus] == "Model" && isAPIType(m.currentPC().Type) {
			m.mode = modeModels
			m.loading = true
			m.models = nil
			m.modelsErr = nil
			m.modelCursor = 0
			return m, fetchModelsCmd(m.fetch, m.currentPC())
		}
		// Otherwise enter on the last field commits; else advance.
		if m.focus == len(m.inputs)-1 {
			m.commitDetail()
			return m, nil
		}
		m.focusDelta(1)
		return m, nil
	}

	// Any other key edits the focused input.
	var cmd tea.Cmd
	m.inputs[m.focus], cmd = m.inputs[m.focus].Update(msg)
	return m, cmd
}

func (m ConfigModel) updateModels(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		m.quit = true
		return m, tea.Quit
	case "esc":
		// Cancel: return to the detail form (where the Model field can be typed).
		m.mode = modeDetail
		return m, nil
	}

	if m.loading {
		return m, nil // ignore navigation while the fetch is in flight
	}

	switch msg.String() {
	case "up", "k":
		if m.modelCursor > 0 {
			m.modelCursor--
		}
	case "down", "j":
		if m.modelCursor < len(m.models)-1 {
			m.modelCursor++
		}
	case "enter":
		if len(m.models) > 0 {
			m.setModelValue(m.models[m.modelCursor])
		}
		m.mode = modeDetail
		return m, nil
	}
	return m, nil
}

func (m ConfigModel) View() string {
	switch m.mode {
	case modeDetail:
		return m.detailView()
	case modeModels:
		return m.modelsView()
	default:
		return m.listView()
	}
}

func modelOrDash(model string) string {
	if model == "" {
		return "-"
	}
	return model
}

func (m ConfigModel) listView() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("🤖 tai config"))
	b.WriteString("\n\n")

	if len(m.names) == 0 {
		b.WriteString(loadingStyle.Render("no providers configured — edit the JSON or run `tai config init`"))
		b.WriteString("\n\n")
	}

	for i, name := range m.names {
		pc := m.cfg.Providers[name]
		marker := "  "
		if name == m.cfg.DefaultProvider {
			marker = "★ "
		}
		cursor := "  "
		if i == m.cursor {
			cursor = "› "
		}
		line := fmt.Sprintf("%s%s%-14s [%s] model=%s", cursor, marker, name, pc.Type, modelOrDash(pc.Model))
		if i == m.cursor {
			b.WriteString(promptStyle.Render(line))
		} else {
			b.WriteString(line)
		}
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(helpStyle.Render("↑/↓ move · enter: edit · d: set default (★) · s: save & quit · q/esc: quit"))
	b.WriteString("\n")
	return b.String()
}

func (m ConfigModel) detailView() string {
	var b strings.Builder
	name := m.names[m.cursor]
	b.WriteString(titleStyle.Render("🤖 edit provider: " + name))
	b.WriteString("\n\n")

	apiType := isAPIType(m.cfg.Providers[name].Type)
	for i, label := range m.fields {
		text := label + ":"
		if label == "Model" && apiType {
			text += " (enter to list)"
		}
		var head string
		if i == m.focus {
			head = promptStyle.Render("› " + text)
		} else {
			head = "  " + text
		}
		b.WriteString(head)
		b.WriteString("\n  ")
		b.WriteString(m.inputs[i].View())
		b.WriteString("\n\n")
	}

	b.WriteString(helpStyle.Render("↑/↓ / tab: move field · enter: next/save (Model: list) · esc: back to list"))
	b.WriteString("\n")
	return b.String()
}

func (m ConfigModel) modelsView() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("🤖 choose model · " + m.names[m.cursor]))
	b.WriteString("\n\n")

	switch {
	case m.loading:
		b.WriteString(loadingStyle.Render("fetching available models…"))
		b.WriteString("\n\n")
		b.WriteString(helpStyle.Render("esc: cancel"))
	case m.modelsErr != nil:
		b.WriteString(errorStyle.Render("❌ " + m.modelsErr.Error()))
		b.WriteString("\n\n")
		b.WriteString(helpStyle.Render("esc: back and type the model manually"))
	case len(m.models) == 0:
		b.WriteString(loadingStyle.Render("no models returned by the provider"))
		b.WriteString("\n\n")
		b.WriteString(helpStyle.Render("esc: back and type the model manually"))
	default:
		for i, name := range m.models {
			cursor := "  "
			if i == m.modelCursor {
				cursor = "› "
			}
			line := cursor + name
			if i == m.modelCursor {
				b.WriteString(promptStyle.Render(line))
			} else {
				b.WriteString(line)
			}
			b.WriteString("\n")
		}
		b.WriteString("\n")
		b.WriteString(helpStyle.Render("↑/↓ move · enter: select · esc: cancel"))
	}

	b.WriteString("\n")
	return b.String()
}

// Config returns the (possibly edited) configuration.
func (m ConfigModel) Config() config.Config { return m.cfg }

// Saved reports whether the user chose to save.
func (m ConfigModel) Saved() bool { return m.saved }

// RunConfig launches the config editor TUI and blocks until the user saves or
// quits. It returns the (possibly edited) config and whether to persist it.
func RunConfig(cfg config.Config) (config.Config, bool, error) {
	p := tea.NewProgram(NewConfig(cfg))
	final, err := p.Run()
	if err != nil {
		return cfg, false, err
	}
	m, ok := final.(ConfigModel)
	if !ok {
		return cfg, false, fmt.Errorf("unexpected config tui model type")
	}
	return m.Config(), m.Saved(), nil
}
