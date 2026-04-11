// Package tui provides the interactive terminal UI for the memsh AI agent.
package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"

	"github.com/amjadjibon/memsh/internal/agent"
)

// Run starts the bubbletea TUI and blocks until the user quits.
func Run(ctx context.Context, cancel context.CancelFunc, ag *agent.Agent, modelName string) error {
	m := newModel(ctx, cancel, ag, modelName)
	p := tea.NewProgram(m)
	_, err := p.Run()
	return err
}

// ── tea messages ─────────────────────────────────────────────

type (
	agentInterruptMsg struct{ history []*schema.Message }
	agentDoneMsg      struct{ result string }
	agentErrMsg       struct{ err error }
)

// ── chat entries ─────────────────────────────────────────────

type entryKind int

const (
	entryUser entryKind = iota
	entryAgent
	entryToolCall
	entryToolResult
	entryError
)

type chatEntry struct {
	kind    entryKind
	content string
	label   string // tool name for entryToolCall
}

// ── styles ───────────────────────────────────────────────────

var (
	headerStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("62")).
			Foreground(lipgloss.Color("230")).
			Bold(true).
			Padding(0, 1)

	userLabelStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("86")).
			Bold(true)

	userContentStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("252"))

	agentLabelStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("212")).
			Bold(true)

	agentContentStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("255"))

	toolLabelStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("220")).
			Bold(true)

	toolCmdStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("250")).
			Background(lipgloss.Color("235")).
			Padding(0, 1)

	toolResultStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("244"))

	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("196")).
			Bold(true)

	dividerStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("238"))

	thickDividerStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("62"))

	statusStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("242"))

	thinkingStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("220"))

	waitingStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("75"))

	footerStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241"))

	placeholderStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("240")).
				Italic(true)

	inputBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("238")).
			Padding(0, 1)

	inputBoxActiveStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("62")).
				Padding(0, 1)
)

// ── layout constants ──────────────────────────────────────────

// fixedLines is the number of lines consumed by everything except the
// viewport and the textarea content:
//
//	header(1) + thin-div(1) + thin-div(1) + status(1) + thick-div(1) + box-border(2) + footer(1) = 8
const fixedLines = 8

// ── model ─────────────────────────────────────────────────────

type model struct {
	viewport viewport.Model
	textarea textarea.Model
	spinner  spinner.Model

	entries    []chatEntry
	historyIdx int

	thinking    bool
	interrupted bool

	ag        *agent.Agent
	ctx       context.Context
	cancel    context.CancelFunc
	modelName string

	windowWidth  int
	windowHeight int
	ready        bool
}

func newModel(ctx context.Context, cancel context.CancelFunc, ag *agent.Agent, modelName string) model {
	ta := textarea.New()
	ta.Placeholder = "Message memsh agent… (Enter to send, Alt+Enter for newline)"
	ta.ShowLineNumbers = false
	ta.DynamicHeight = true
	ta.MinHeight = 1
	ta.MaxHeight = 5
	ta.CharLimit = 0

	// Remove the thick-border prompt marker — use clean indentation instead.
	ta.Prompt = "  "

	// Custom styles: strip default borders, clean look on dark background.
	s := textarea.DefaultDarkStyles()
	s.Focused.Base = lipgloss.NewStyle()
	s.Focused.CursorLine = lipgloss.NewStyle()
	s.Focused.Text = lipgloss.NewStyle().Foreground(lipgloss.Color("255"))
	s.Focused.Placeholder = lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Italic(true)
	s.Focused.Prompt = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	s.Blurred.Base = lipgloss.NewStyle()
	s.Blurred.Text = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	ta.SetStyles(s)

	// Set focus=true on the struct BEFORE storing (Init will return the blink Cmd).
	ta.Focus() //nolint:errcheck

	sp := spinner.New(spinner.WithSpinner(spinner.Dot))

	return model{
		textarea:  ta,
		spinner:   sp,
		ag:        ag,
		ctx:       ctx,
		cancel:    cancel,
		modelName: modelName,
	}
}

func (m model) Init() tea.Cmd {
	// m.textarea is a copy in Init — Focus() returns the cursor-blink Cmd only.
	// The focus=true state was already written into the struct in newModel.
	return tea.Batch(
		m.textarea.Focus(),
		func() tea.Msg { return m.spinner.Tick() },
	)
}

// ── Update ───────────────────────────────────────────────────

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.windowWidth = msg.Width
		m.windowHeight = msg.Height
		m.textarea.SetWidth(m.textareaWidth())
		vpH := m.viewportHeight()
		if !m.ready {
			m.viewport = viewport.New(
				viewport.WithWidth(msg.Width),
				viewport.WithHeight(vpH),
			)
			m.viewport.SetContent(m.renderEntries())
			m.ready = true
		} else {
			m.viewport.SetWidth(msg.Width)
			m.viewport.SetHeight(vpH)
		}

	case spinner.TickMsg:
		if m.thinking {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			cmds = append(cmds, cmd)
		}

	case tea.KeyPressMsg:
		switch msg.String() {
		case "ctrl+c":
			m.cancel()
			return m, tea.Quit
		case "enter":
			if !m.thinking {
				return m.handleSend()
			}
		default:
			if !m.thinking {
				cmds = append(cmds, m.updateTextarea(msg))
			}
		}

	case agentInterruptMsg:
		m.thinking = false
		m.interrupted = true
		m.processNewHistory(msg.history)
		m.refreshViewport()

	case agentDoneMsg:
		m.thinking = false
		m.interrupted = false
		if msg.result != "" {
			m.entries = append(m.entries, chatEntry{kind: entryAgent, content: msg.result})
		}
		m.refreshViewport()

	case agentErrMsg:
		m.thinking = false
		m.entries = append(m.entries, chatEntry{kind: entryError, content: msg.err.Error()})
		m.refreshViewport()

	default:
		// Viewport scroll events (mouse wheel, PgUp/PgDn).
		var vcmd tea.Cmd
		m.viewport, vcmd = m.viewport.Update(msg)
		cmds = append(cmds, vcmd)
		// Cursor blink ticks and other textarea-internal messages.
		if !m.thinking {
			cmds = append(cmds, m.updateTextarea(msg))
		}
	}

	return m, tea.Batch(cmds...)
}

// updateTextarea forwards a message to the textarea and adjusts the viewport
// height if the textarea grew or shrank (DynamicHeight).
func (m *model) updateTextarea(msg tea.Msg) tea.Cmd {
	prev := m.textarea.Height()
	var cmd tea.Cmd
	m.textarea, cmd = m.textarea.Update(msg)
	if m.textarea.Height() != prev && m.ready {
		m.viewport.SetHeight(m.viewportHeight())
	}
	return cmd
}

func (m model) handleSend() (tea.Model, tea.Cmd) {
	text := strings.TrimSpace(m.textarea.Value())
	if text == "" {
		return m, nil
	}
	m.textarea.Reset()
	m.entries = append(m.entries, chatEntry{kind: entryUser, content: text})
	m.thinking = true
	m.refreshViewport()

	var agentCmd tea.Cmd
	if m.interrupted {
		userInput := text
		agentCmd = func() tea.Msg {
			result, err := m.ag.Invoke(m.ctx, "",
				agent.WithCheckPointID("session"),
				compose.WithRuntimeMaxSteps(20),
				agent.WithStateModifier(func(_ context.Context, _ compose.NodePath, s any) error {
					s.(*agent.State).UserInput = userInput
					return nil
				}),
			)
			return resolveAgentResult(result, err)
		}
	} else {
		input := text
		agentCmd = func() tea.Msg {
			result, err := m.ag.Invoke(m.ctx, input,
				agent.WithCheckPointID("session"),
				compose.WithRuntimeMaxSteps(20),
			)
			return resolveAgentResult(result, err)
		}
	}

	return m, tea.Batch(
		func() tea.Msg { return m.spinner.Tick() },
		agentCmd,
	)
}

func resolveAgentResult(result string, err error) tea.Msg {
	info, interrupted := agent.ExtractInterruptInfo(err)
	if interrupted {
		return agentInterruptMsg{history: info.State.(*agent.State).History}
	}
	if err != nil {
		return agentErrMsg{err: err}
	}
	return agentDoneMsg{result: result}
}

func (m *model) processNewHistory(history []*schema.Message) {
	for _, msg := range history[m.historyIdx:] {
		switch msg.Role {
		case schema.Assistant:
			for _, tc := range msg.ToolCalls {
				var args struct {
					Command string `json:"command"`
				}
				_ = json.Unmarshal([]byte(tc.Function.Arguments), &args)
				cmd := args.Command
				if cmd == "" {
					cmd = tc.Function.Arguments
				}
				m.entries = append(m.entries, chatEntry{
					kind:    entryToolCall,
					content: cmd,
					label:   tc.Function.Name,
				})
			}
			if len(msg.ToolCalls) == 0 && msg.Content != "" {
				m.entries = append(m.entries, chatEntry{kind: entryAgent, content: msg.Content})
			}
		case schema.Tool:
			if msg.Content != "" {
				m.entries = append(m.entries, chatEntry{kind: entryToolResult, content: msg.Content})
			}
		}
	}
	m.historyIdx = len(history)
}

func (m *model) refreshViewport() {
	m.viewport.SetContent(m.renderEntries())
	m.viewport.GotoBottom()
}

// ── layout helpers ────────────────────────────────────────────

func (m *model) textareaWidth() int {
	// inputBoxStyle has 1 left + 1 right border + 1 left + 1 right padding = 4
	w := m.windowWidth - 4
	if w < 20 {
		w = 20
	}
	return w
}

func (m *model) viewportHeight() int {
	taH := m.textarea.Height()
	if taH < 1 {
		taH = 1
	}
	h := m.windowHeight - fixedLines - taH
	if h < 1 {
		h = 1
	}
	return h
}

// ── View ─────────────────────────────────────────────────────

func (m model) View() tea.View {
	var v tea.View
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	v.WindowTitle = "memsh agent"
	v.SetContent(m.render())
	return v
}

func (m model) render() string {
	if !m.ready {
		return "\n  Initializing…"
	}

	w := m.windowWidth
	thin := dividerStyle.Render(strings.Repeat("─", w))
	thick := thickDividerStyle.Render(strings.Repeat("─", w))

	// ── Header ───────────────────────────────────────────────
	left := "  memsh agent"
	right := "model: " + m.modelName + "  "
	gap := strings.Repeat(" ", max(0, w-lipgloss.Width(left)-lipgloss.Width(right)))
	header := headerStyle.Width(w).Render(left + gap + right)

	// ── Status ───────────────────────────────────────────────
	var status string
	switch {
	case m.thinking:
		status = "  " + m.spinner.View() + " " + thinkingStyle.Render("Thinking…")
	case m.interrupted:
		status = "  " + waitingStyle.Render("● Waiting for your reply")
	default:
		status = "  " + statusStyle.Render("● Ready")
	}

	// ── Input box ────────────────────────────────────────────
	var inputBox string
	if m.thinking {
		locked := lipgloss.NewStyle().
			Foreground(lipgloss.Color("240")).Italic(true).
			Render("  (agent is thinking…)")
		inputBox = inputBoxStyle.Width(w - 2).Render(locked)
	} else {
		inputBox = inputBoxActiveStyle.Width(w - 2).Render(m.textarea.View())
	}

	// ── Footer ───────────────────────────────────────────────
	footer := footerStyle.Render("  Enter: send  ·  Alt+Enter: newline  ·  PgUp/PgDn: scroll  ·  Ctrl+C: quit")

	return strings.Join([]string{
		header,
		thin,
		m.viewport.View(),
		thin,
		status,
		thick,
		inputBox,
		footer,
	}, "\n")
}

// ── chat rendering ────────────────────────────────────────────

func (m *model) renderEntries() string {
	if len(m.entries) == 0 {
		return placeholderStyle.Render("\n  Start a conversation — type a message below.\n")
	}

	w := m.viewport.Width()
	if w < 20 {
		w = 80
	}
	contentW := w - 4

	var sb strings.Builder
	for i, e := range m.entries {
		if i > 0 {
			sb.WriteString("\n")
		}
		switch e.kind {
		case entryUser:
			sb.WriteString(userLabelStyle.Render("  You") + "\n")
			sb.WriteString(renderWrapped(e.content, contentW, userContentStyle, "  ") + "\n")

		case entryAgent:
			sb.WriteString(agentLabelStyle.Render("  Agent") + "\n")
			sb.WriteString(renderWrapped(e.content, contentW, agentContentStyle, "  ") + "\n")

		case entryToolCall:
			label := e.label
			if label == "" {
				label = "tool"
			}
			sb.WriteString(toolLabelStyle.Render("  ⚙ "+label) + "\n")
			sb.WriteString("  " + toolCmdStyle.Render("$ "+e.content) + "\n")

		case entryToolResult:
			lines := strings.Split(strings.TrimRight(e.content, "\n"), "\n")
			const maxLines = 30
			if len(lines) > maxLines {
				lines = append(lines[:maxLines], fmt.Sprintf("  … (%d more lines)", len(lines)-maxLines))
			}
			for _, l := range lines {
				sb.WriteString(toolResultStyle.Render("  "+l) + "\n")
			}

		case entryError:
			sb.WriteString(errorStyle.Render("  ✗ "+e.content) + "\n")
		}
	}
	return sb.String()
}

// renderWrapped word-wraps text and applies a style to each line.
func renderWrapped(text string, width int, style lipgloss.Style, indent string) string {
	var sb strings.Builder
	for _, paragraph := range strings.Split(text, "\n") {
		for _, line := range wordWrap(paragraph, width) {
			sb.WriteString(style.Render(indent+line) + "\n")
		}
	}
	return strings.TrimSuffix(sb.String(), "\n")
}

func wordWrap(text string, width int) []string {
	if width <= 0 || len(text) == 0 {
		return []string{text}
	}
	words := strings.Fields(text)
	if len(words) == 0 {
		return []string{""}
	}
	var lines []string
	cur := words[0]
	for _, w := range words[1:] {
		if len(cur)+1+len(w) <= width {
			cur += " " + w
		} else {
			lines = append(lines, cur)
			cur = w
		}
	}
	return append(lines, cur)
}
