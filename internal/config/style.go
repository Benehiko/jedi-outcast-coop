package config

import "github.com/charmbracelet/lipgloss"

// palette — mirrors internal/gfx's blaster-scope blue over slate so the two
// settings menus and the graphics selector read as one system. Duplicated here
// (rather than imported) to keep config free of a gfx dependency.
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
	styVal     = lipgloss.NewStyle().Bold(true).Foreground(cAccent)
	styWarn    = lipgloss.NewStyle().Foreground(cWarn)
	styKey     = lipgloss.NewStyle().Foreground(cAccent)
	styHelp    = lipgloss.NewStyle().Foreground(cDim)
	styCursor  = lipgloss.NewStyle().Background(cCursorB)
)
