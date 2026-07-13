package gfx

import (
	"fmt"
	"maps"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// palette — a cool blaster-scope blue accent over a neutral slate, echoing the
// in-game HUD dials. Kept legible on both light and dark terminals by leaning on
// ANSI-adaptive colors.
var (
	cAccent  = lipgloss.AdaptiveColor{Light: "#0f6fc4", Dark: "#5fb8ff"}
	cOn      = lipgloss.AdaptiveColor{Light: "#1f8f4e", Dark: "#57d08a"}
	cOff     = lipgloss.AdaptiveColor{Light: "#8a8f9c", Dark: "#6f7688"}
	cDim     = lipgloss.AdaptiveColor{Light: "#5a6070", Dark: "#a4a9b8"}
	cWarn    = lipgloss.AdaptiveColor{Light: "#b26a00", Dark: "#e8c15a"}
	cInk     = lipgloss.AdaptiveColor{Light: "#171a22", Dark: "#e8eaf0"}
	cCursorB = lipgloss.AdaptiveColor{Light: "#e6f0fb", Dark: "#1c2634"}
)

var (
	styTitle   = lipgloss.NewStyle().Bold(true).Foreground(cAccent)
	styEyebrow = lipgloss.NewStyle().Foreground(cDim)
	styTitleTx = lipgloss.NewStyle().Bold(true).Foreground(cInk)
	styDesc    = lipgloss.NewStyle().Foreground(cDim)
	styOn      = lipgloss.NewStyle().Bold(true).Foreground(cOn)
	styOff     = lipgloss.NewStyle().Foreground(cOff)
	styWarn    = lipgloss.NewStyle().Foreground(cWarn)
	styKey     = lipgloss.NewStyle().Foreground(cAccent)
	styHelp    = lipgloss.NewStyle().Foreground(cDim)
	styCursor  = lipgloss.NewStyle().Background(cCursorB)
)

// Result is what the TUI returns to the caller after it exits.
type Result struct {
	// Confirmed is true if the user pressed enter to apply; false if quit.
	Confirmed bool
	// Selected is the chosen enabled-state per feature key.
	Selected map[string]bool
	// Changed is true if Selected differs from the initial state.
	Changed bool
}

type model struct {
	features []Feature
	initial  map[string]bool
	sel      map[string]bool
	cursor   int
	result   Result
}

// NewModel builds the selector seeded with the currently-applied state.
func NewModel(initial map[string]bool) tea.Model {
	sel := make(map[string]bool, len(initial))
	for _, f := range Features {
		sel[f.Key] = initial[f.Key]
	}
	return &model{
		features: Features,
		initial:  initial,
		sel:      sel,
	}
}

func (m *model) Init() tea.Cmd { return nil }

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	km, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch km.String() {
	case "ctrl+c", "q", "esc":
		m.result = Result{Confirmed: false}
		return m, tea.Quit
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < len(m.features)-1 {
			m.cursor++
		}
	case " ", "x":
		f := m.features[m.cursor]
		m.sel[f.Key] = !m.sel[f.Key]
	case "a":
		for _, f := range m.features {
			m.sel[f.Key] = true
		}
	case "n":
		for _, f := range m.features {
			m.sel[f.Key] = false
		}
	case "enter":
		m.result = Result{Confirmed: true, Selected: m.cloneSel(), Changed: m.changed()}
		return m, tea.Quit
	}
	return m, nil
}

func (m *model) cloneSel() map[string]bool {
	c := make(map[string]bool, len(m.sel))
	maps.Copy(c, m.sel)
	return c
}

func (m *model) changed() bool {
	for _, f := range m.features {
		if m.sel[f.Key] != m.initial[f.Key] {
			return true
		}
	}
	return false
}

func (m *model) View() string {
	var b strings.Builder
	b.WriteString(styEyebrow.Render("jedi outcast co-op · graphics") + "\n")
	b.WriteString(styTitle.Render("Graphics features") + "\n\n")

	for i, f := range m.features {
		cursor := "  "
		if i == m.cursor {
			cursor = styKey.Render("▸ ")
		}
		box := styOff.Render("○ off")
		if m.sel[f.Key] {
			box = styOn.Render("● on ")
		}
		title := styTitleTx.Render(f.Title)
		line := fmt.Sprintf("%s%s  %s", cursor, box, title)
		if i == m.cursor {
			line = styCursor.Render(lipgloss.NewStyle().Width(58).Render(line))
		}
		b.WriteString(line + "\n")
		b.WriteString("       " + styDesc.Render(f.Desc) + "\n")

		// flag a pending change inline
		if m.sel[f.Key] != m.initial[f.Key] {
			verb := "will enable"
			if !m.sel[f.Key] {
				verb = "will disable"
			}
			b.WriteString("       " + styWarn.Render("• "+verb+" (needs rebuild)") + "\n")
		}
		b.WriteString("\n")
	}

	b.WriteString(m.footer())
	return b.String()
}

func (m *model) footer() string {
	keys := []string{
		styKey.Render("↑/↓") + " move",
		styKey.Render("space") + " toggle",
		styKey.Render("a") + "ll",
		styKey.Render("n") + "one",
		styKey.Render("enter") + " apply",
		styKey.Render("q") + " cancel",
	}
	help := styHelp.Render(strings.Join(keys, "   "))
	if m.changed() {
		return styWarn.Render("Changes pending — enter resets the submodule, reapplies patches, and rebuilds.") + "\n" + help
	}
	return styHelp.Render("No changes.") + "\n" + help
}

// SummaryLine renders a one-line human summary of a selection, e.g.
// "widescreen, render-fidelity (modern-combat off)".
func SummaryLine(sel map[string]bool) string {
	var on, off []string
	for _, f := range Features {
		if sel[f.Key] {
			on = append(on, f.Key)
		} else {
			off = append(off, f.Key)
		}
	}
	sort.Strings(on)
	sort.Strings(off)
	switch {
	case len(on) == 0:
		return "all graphics features off"
	case len(off) == 0:
		return strings.Join(on, ", ") + " (all on)"
	default:
		return strings.Join(on, ", ") + " (" + strings.Join(off, ", ") + " off)"
	}
}

// RunResult runs the interactive selector and returns the user's decision.
func (m *model) RunResult() Result { return m.result }
