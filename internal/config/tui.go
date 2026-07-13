package config

import (
	"fmt"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Row is one editable setting in a settings form. Implementations render their
// value and mutate it in response to toggle (space) or left/right adjustment.
type Row interface {
	// Title is the short human name.
	Title() string
	// Desc is a one-line description.
	Desc() string
	// Display renders the current value (e.g. "on", "0.5", "4x").
	Display() string
	// Rebuild reports whether changing this row requires an engine rebuild
	// (shown as a hint; the caller decides what to do on save).
	Rebuild() bool
	// toggle flips a bool row; no-op for value rows.
	toggle()
	// adjust steps a value row by dir (-1 left, +1 right); no-op for bool rows.
	adjust(dir int)
	// changed reports whether the current value differs from the initial one.
	changed() bool
}

// BoolRow is an on/off toggle backed by a *bool.
type BoolRow struct {
	title, desc string
	rebuild     bool
	val         *bool
	initial     bool
}

// NewBoolRow builds a boolean row bound to val.
func NewBoolRow(title, desc string, rebuild bool, val *bool) *BoolRow {
	return &BoolRow{title: title, desc: desc, rebuild: rebuild, val: val, initial: *val}
}

func (r *BoolRow) Title() string { return r.title }
func (r *BoolRow) Desc() string  { return r.desc }
func (r *BoolRow) Rebuild() bool { return r.rebuild }
func (r *BoolRow) Display() string {
	if *r.val {
		return "on"
	}
	return "off"
}
func (r *BoolRow) toggle()        { *r.val = !*r.val }
func (r *BoolRow) adjust(dir int) { r.toggle() }
func (r *BoolRow) changed() bool  { return *r.val != r.initial }

// EnumRow cycles an *int through a fixed set of allowed values, rendering each
// with an optional label.
type EnumRow struct {
	title, desc string
	rebuild     bool
	val         *int
	initial     int
	values      []int
	labels      map[int]string
}

// NewEnumRow builds an enum row bound to val, cycling through values. labels may
// be nil (values render as their number).
func NewEnumRow(title, desc string, rebuild bool, val *int, values []int, labels map[int]string) *EnumRow {
	return &EnumRow{title: title, desc: desc, rebuild: rebuild, val: val, initial: *val, values: values, labels: labels}
}

func (r *EnumRow) Title() string { return r.title }
func (r *EnumRow) Desc() string  { return r.desc }
func (r *EnumRow) Rebuild() bool { return r.rebuild }
func (r *EnumRow) Display() string {
	if lbl, ok := r.labels[*r.val]; ok {
		return lbl
	}
	return strconv.Itoa(*r.val)
}
func (r *EnumRow) toggle() {}
func (r *EnumRow) adjust(dir int) {
	idx := 0
	for i, v := range r.values {
		if v == *r.val {
			idx = i
			break
		}
	}
	idx = (idx + dir + len(r.values)) % len(r.values)
	*r.val = r.values[idx]
}
func (r *EnumRow) changed() bool { return *r.val != r.initial }

// FloatRow steps a *float64 within [min,max] by a fixed increment.
type FloatRow struct {
	title, desc       string
	rebuild           bool
	val               *float64
	initial           float64
	fmin, fmax, fstep float64
}

// NewFloatRow builds a float row bound to val, stepping by step within [min,max].
func NewFloatRow(title, desc string, rebuild bool, val *float64, min, max, step float64) *FloatRow {
	return &FloatRow{title: title, desc: desc, rebuild: rebuild, val: val, initial: *val, fmin: min, fmax: max, fstep: step}
}

func (r *FloatRow) Title() string   { return r.title }
func (r *FloatRow) Desc() string    { return r.desc }
func (r *FloatRow) Rebuild() bool   { return r.rebuild }
func (r *FloatRow) Display() string { return strconv.FormatFloat(*r.val, 'f', -1, 64) }
func (r *FloatRow) toggle()         {}
func (r *FloatRow) adjust(dir int) {
	v := *r.val + float64(dir)*r.fstep
	// Round to the step grid to avoid float drift (e.g. 0.30000000004).
	v = float64(int(v/r.fstep+sign(v)*0.5)) * r.fstep
	v = max(r.fmin, min(r.fmax, v))
	*r.val = v
}
func (r *FloatRow) changed() bool { return *r.val != r.initial }

func sign(f float64) float64 {
	if f < 0 {
		return -1
	}
	return 1
}

// IntRow steps an *int within [min,max] by a fixed increment.
type IntRow struct {
	title, desc             string
	rebuild                 bool
	val                     *int
	initial, min, max, step int
	unit                    string
}

// NewIntRow builds an int row bound to val, stepping by step within [min,max].
func NewIntRow(title, desc string, rebuild bool, val *int, min, max, step int, unit string) *IntRow {
	return &IntRow{title: title, desc: desc, rebuild: rebuild, val: val, initial: *val, min: min, max: max, step: step, unit: unit}
}

func (r *IntRow) Title() string { return r.title }
func (r *IntRow) Desc() string  { return r.desc }
func (r *IntRow) Rebuild() bool { return r.rebuild }
func (r *IntRow) Display() string {
	if r.unit != "" {
		return fmt.Sprintf("%d %s", *r.val, r.unit)
	}
	return strconv.Itoa(*r.val)
}
func (r *IntRow) toggle() {}
func (r *IntRow) adjust(dir int) {
	*r.val = max(r.min, min(r.max, *r.val+dir*r.step))
}
func (r *IntRow) changed() bool { return *r.val != r.initial }

// ResolutionRow cycles a *Resolution through "auto" (0x0) plus a fixed list of
// common resolutions. It is rebuild-free: the values become r_customwidth /
// r_customheight cvars written to autoexec. When a monitor resolution is
// detected, that entry is annotated as the suggested native mode.
type ResolutionRow struct {
	title, desc string
	val         *Resolution
	initial     Resolution
	// choices is auto (0x0) followed by the offered resolutions, in cycle order.
	choices []Resolution
	// suggested is the detected native resolution (0x0 if none was detected).
	suggested Resolution
}

// NewResolutionRow builds a resolution row bound to val. suggested is the
// detected native resolution (pass a zero Resolution if detection failed); when
// non-zero it is guaranteed to appear in the cycle and is flagged as native.
func NewResolutionRow(title, desc string, val *Resolution, suggested Resolution) *ResolutionRow {
	choices := []Resolution{{}} // auto first
	seen := map[Resolution]bool{{}: true}
	add := func(r Resolution) {
		if r.W > 0 && r.H > 0 && !seen[r] {
			seen[r] = true
			choices = append(choices, r)
		}
	}
	add(suggested)
	for _, r := range CommonResolutions {
		add(r)
	}
	// Ensure the current value is selectable even if it is an odd custom size.
	add(*val)
	return &ResolutionRow{title: title, desc: desc, val: val, initial: *val, choices: choices, suggested: suggested}
}

func (r *ResolutionRow) Title() string { return r.title }
func (r *ResolutionRow) Desc() string  { return r.desc }
func (r *ResolutionRow) Rebuild() bool { return false }
func (r *ResolutionRow) Display() string {
	if r.val.W == 0 || r.val.H == 0 {
		if r.suggested.W > 0 {
			return fmt.Sprintf("auto (%dx%d)", r.suggested.W, r.suggested.H)
		}
		return "auto"
	}
	s := fmt.Sprintf("%dx%d", r.val.W, r.val.H)
	if *r.val == r.suggested {
		s += " (native)"
	}
	return s
}
func (r *ResolutionRow) toggle() {}
func (r *ResolutionRow) adjust(dir int) {
	idx := 0
	for i, c := range r.choices {
		if c == *r.val {
			idx = i
			break
		}
	}
	idx = (idx + dir + len(r.choices)) % len(r.choices)
	*r.val = r.choices[idx]
}
func (r *ResolutionRow) changed() bool { return *r.val != r.initial }

// FormResult is what a settings form returns after it exits.
type FormResult struct {
	// Confirmed is true if the user pressed enter to save; false if cancelled.
	Confirmed bool
	// Changed is true if any row differs from its initial value.
	Changed bool
	// RebuildNeeded is true if any changed row is rebuild-backed.
	RebuildNeeded bool
}

type formModel struct {
	eyebrow string
	title   string
	rows    []Row
	cursor  int
	result  FormResult
}

// NewForm builds a settings form over rows, titled for the given menu. The rows
// mutate the caller's config in place; read it back after Run.
func NewForm(eyebrow, title string, rows []Row) tea.Model {
	return &formModel{eyebrow: eyebrow, title: title, rows: rows}
}

func (m *formModel) Init() tea.Cmd { return nil }

func (m *formModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	km, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch km.String() {
	case "ctrl+c", "q", "esc":
		m.result = FormResult{Confirmed: false}
		return m, tea.Quit
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < len(m.rows)-1 {
			m.cursor++
		}
	case " ", "x":
		m.rows[m.cursor].toggle()
	case "left", "h":
		m.rows[m.cursor].adjust(-1)
	case "right", "l":
		m.rows[m.cursor].adjust(+1)
	case "enter":
		m.result = FormResult{Confirmed: true, Changed: m.changed(), RebuildNeeded: m.rebuildNeeded()}
		return m, tea.Quit
	}
	return m, nil
}

func (m *formModel) changed() bool {
	for _, r := range m.rows {
		if r.changed() {
			return true
		}
	}
	return false
}

func (m *formModel) rebuildNeeded() bool {
	for _, r := range m.rows {
		if r.changed() && r.Rebuild() {
			return true
		}
	}
	return false
}

func (m *formModel) View() string {
	var b strings.Builder
	b.WriteString(styEyebrow.Render(m.eyebrow) + "\n")
	b.WriteString(styTitle.Render(m.title) + "\n\n")

	for i, r := range m.rows {
		cursor := "  "
		if i == m.cursor {
			cursor = styKey.Render("▸ ")
		}
		valStyle := styVal
		if br, ok := r.(*BoolRow); ok {
			if *br.val {
				valStyle = styOn
			} else {
				valStyle = styOff
			}
		}
		val := valStyle.Render(fmt.Sprintf("%-7s", r.Display()))
		line := fmt.Sprintf("%s%s  %s", cursor, val, styTitleTx.Render(r.Title()))
		if i == m.cursor {
			line = styCursor.Render(lipgloss.NewStyle().Width(58).Render(line))
		}
		b.WriteString(line + "\n")
		b.WriteString("         " + styDesc.Render(r.Desc()) + "\n")
		if r.changed() && r.Rebuild() {
			b.WriteString("         " + styWarn.Render("• changed (needs an engine rebuild)") + "\n")
		}
		b.WriteString("\n")
	}
	b.WriteString(m.footer())
	return b.String()
}

func (m *formModel) footer() string {
	keys := []string{
		styKey.Render("↑/↓") + " move",
		styKey.Render("←/→") + " change",
		styKey.Render("space") + " toggle",
		styKey.Render("enter") + " save",
		styKey.Render("q") + " cancel",
	}
	help := styHelp.Render(strings.Join(keys, "   "))
	switch {
	case m.rebuildNeeded():
		return styWarn.Render("Changes pending — saving will offer to rebuild the engine.") + "\n" + help
	case m.changed():
		return styHelp.Render("Changes pending — saving writes your config.") + "\n" + help
	default:
		return styHelp.Render("No changes.") + "\n" + help
	}
}

// RunResult exposes the form's decision after tea.Program.Run returns.
func (m *formModel) RunResult() FormResult { return m.result }
