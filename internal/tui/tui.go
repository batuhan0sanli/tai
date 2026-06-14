package tui

import (
	"fmt"
	"strings"

	"tai/internal/provider"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#7D56F4"))

	commandStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#00FF87")).
			Bold(true).
			Padding(0, 2).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("63"))

	promptStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFAF00")).
			Bold(true)

	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FF5F87")).
			Bold(true)

	loadingStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#B294BB"))

	helpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241")).
			Italic(true)
)

// aiResponseMsg is emitted when an async revision request returns.
type aiResponseMsg struct {
	command string
	err     error
}

// Model is the Bubble Tea state for the interactive confirmation TUI.
type Model struct {
	originalPrompt string
	command        string
	input          textinput.Model
	spinner        spinner.Model
	provider       provider.AIProvider

	loading       bool
	err           error
	shouldExecute bool
	width         int
}

// New builds a Model preloaded with the initial AI suggestion.
func New(originalPrompt, suggestedCommand string, ai provider.AIProvider) Model {
	ti := textinput.New()
	ti.Placeholder = "describe a change, or press Enter to run as-is"
	ti.Focus()
	ti.CharLimit = 512
	ti.Prompt = "› "

	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("#7D56F4"))

	return Model{
		originalPrompt: originalPrompt,
		command:        suggestedCommand,
		input:          ti,
		spinner:        sp,
		provider:       ai,
	}
}

func (m Model) Init() tea.Cmd {
	return textinput.Blink
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		if w := msg.Width - 4; w > 0 {
			m.input.Width = w
		}
		return m, nil

	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC, tea.KeyEsc:
			return m, tea.Quit

		case tea.KeyEnter:
			if m.loading {
				return m, nil
			}
			revision := strings.TrimSpace(m.input.Value())
			if revision == "" {
				m.shouldExecute = true
				return m, tea.Quit
			}
			m.loading = true
			m.err = nil
			return m, tea.Batch(reviseCmd(m.provider, m.originalPrompt, m.command, revision), m.spinner.Tick)
		}

	case aiResponseMsg:
		m.loading = false
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		m.command = msg.command
		m.input.SetValue("")
		return m, nil

	case spinner.TickMsg:
		if m.loading {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}
		return m, nil
	}

	if !m.loading {
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m Model) View() string {
	var b strings.Builder

	cmdBox := commandStyle
	if m.width > 6 {
		cmdBox = cmdBox.Width(m.width - 2)
	}

	b.WriteString(titleStyle.Render("🤖 tai suggestion"))
	b.WriteString("\n\n")
	b.WriteString(cmdBox.Render(m.command))
	b.WriteString("\n\n")

	if m.err != nil {
		b.WriteString(errorStyle.Render("❌ " + m.err.Error()))
		b.WriteString("\n\n")
	}

	if m.loading {
		b.WriteString(loadingStyle.Render(m.spinner.View() + " revising command..."))
		b.WriteString("\n\n")
	} else {
		b.WriteString(promptStyle.Render("💬 Revise the command (or press Enter to run):"))
		b.WriteString("\n")
		b.WriteString(m.input.View())
		b.WriteString("\n\n")
	}

	b.WriteString(helpStyle.Render("enter: run  ·  esc / ctrl+c: quit"))
	b.WriteString("\n")

	return b.String()
}

// ShouldExecute reports whether the user accepted the command.
func (m Model) ShouldExecute() bool { return m.shouldExecute }

// Command returns the (possibly revised) command the user landed on.
func (m Model) Command() string { return m.command }

// Run starts the TUI and blocks until the user accepts or cancels.
// It returns the final command, whether to execute it, and any TUI error.
func Run(originalPrompt, suggestedCommand string, ai provider.AIProvider) (string, bool, error) {
	p := tea.NewProgram(New(originalPrompt, suggestedCommand, ai))
	final, err := p.Run()
	if err != nil {
		return "", false, err
	}
	m, ok := final.(Model)
	if !ok {
		return "", false, fmt.Errorf("unexpected tui model type")
	}
	return m.Command(), m.ShouldExecute(), nil
}

func reviseCmd(ai provider.AIProvider, originalPrompt, currentCommand, revision string) tea.Cmd {
	return func() tea.Msg {
		combined := fmt.Sprintf(
			"Original request: %s\nPreviously generated command: %s\nRevision instruction: %s\n\nReturn the updated command only.",
			originalPrompt, currentCommand, revision,
		)
		out, err := ai.GenerateCommand(combined)
		return aiResponseMsg{command: out, err: err}
	}
}
