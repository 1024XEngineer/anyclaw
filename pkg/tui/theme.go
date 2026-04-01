package tui

import (
	"github.com/charmbracelet/lipgloss"
)

var (
	PrimaryColor   = lipgloss.Color("86")
	SecondaryColor = lipgloss.Color("75")
	AccentColor    = lipgloss.Color("208")
	ErrorColor     = lipgloss.Color("196")
	SuccessColor   = lipgloss.Color("82")
	WarningColor   = lipgloss.Color("226")
	DimColor       = lipgloss.Color("243")
	SubtleColor    = lipgloss.Color("240")

	BackgroundColor = lipgloss.Color("0")
	SurfaceColor    = lipgloss.Color("0")
)

type ThemeType int

const (
	ThemeDark ThemeType = iota
	ThemeLight
)

var currentTheme ThemeType = ThemeDark

func GetCurrentTheme() ThemeType {
	return currentTheme
}

func ToggleTheme() ThemeType {
	if currentTheme == ThemeDark {
		currentTheme = ThemeLight
		applyLightTheme()
	} else {
		currentTheme = ThemeDark
		applyDarkTheme()
	}
	return currentTheme
}

func applyDarkTheme() {
	PrimaryColor = lipgloss.Color("86")
	SecondaryColor = lipgloss.Color("75")
	AccentColor = lipgloss.Color("208")
	ErrorColor = lipgloss.Color("196")
	SuccessColor = lipgloss.Color("82")
	WarningColor = lipgloss.Color("226")
	DimColor = lipgloss.Color("243")
	SubtleColor = lipgloss.Color("240")
	BackgroundColor = lipgloss.Color("0")
	SurfaceColor = lipgloss.Color("0")
}

func applyLightTheme() {
	PrimaryColor = lipgloss.Color("33")
	SecondaryColor = lipgloss.Color("39")
	AccentColor = lipgloss.Color("208")
	ErrorColor = lipgloss.Color("196")
	SuccessColor = lipgloss.Color("34")
	WarningColor = lipgloss.Color("226")
	DimColor = lipgloss.Color("250")
	SubtleColor = lipgloss.Color("245")
	BackgroundColor = lipgloss.Color("255")
	SurfaceColor = lipgloss.Color("7")
}

var (
	HeaderStyle = lipgloss.NewStyle().
			Foreground(PrimaryColor).
			Bold(true).
			Padding(0, 1)

	FooterStyle = lipgloss.NewStyle().
			Foreground(DimColor).
			Padding(0, 1)

	StatusStyle = lipgloss.NewStyle().
			Foreground(SubtleColor)

	SuccessStyle = lipgloss.NewStyle().
			Foreground(SuccessColor)

	ErrorStyle = lipgloss.NewStyle().
			Foreground(ErrorColor)

	WarningStyle = lipgloss.NewStyle().
			Foreground(WarningColor)

	UserMessageStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("255")).
				Background(lipgloss.Color("59"))

	AssistantMessageStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("252"))

	SystemMessageStyle = lipgloss.NewStyle().
				Foreground(DimColor).
				Italic(true)

	ToolCallStyle = lipgloss.NewStyle().
			Foreground(AccentColor).
			Background(lipgloss.Color("236"))

	InputStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("255"))

	PromptStyle = lipgloss.NewStyle().
			Foreground(PrimaryColor)

	SpinnerStyle = lipgloss.NewStyle().
			Foreground(PrimaryColor)

	BorderStyle = lipgloss.NewStyle().
			Border(lipgloss.NormalBorder()).
			BorderForeground(SubtleColor)

	FocusBorderStyle = lipgloss.NewStyle().
				Border(lipgloss.NormalBorder()).
				BorderForeground(PrimaryColor)

	BoldStyle = lipgloss.NewStyle().
			Bold(true)

	ItalicStyle = lipgloss.NewStyle().
			Italic(true)

	CodeStyle = lipgloss.NewStyle().
			Foreground(AccentColor).
			Background(lipgloss.Color("236"))

	LinkStyle = lipgloss.NewStyle().
			Foreground(SecondaryColor).
			Underline(true)

	CodeBlockStyle = lipgloss.NewStyle().
			Foreground(SubtleColor).
			Background(lipgloss.Color("236"))

	CodeBlockContentStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("252"))

	ThinkingStyle = lipgloss.NewStyle().
			Foreground(WarningColor).
			Italic(true)

	ToolResultStyle = lipgloss.NewStyle().
			Foreground(SuccessColor)

	HelpBarStyle = lipgloss.NewStyle().
			Foreground(SubtleColor).
			Background(SurfaceColor).
			Padding(0, 1)
)

func ThinkingText(text string) string {
	return lipgloss.NewStyle().
		Foreground(DimColor).
		Italic(true).
		Render(text)
}

func ModelLabel(provider, model string) string {
	if provider == "" {
		return model
	}
	return provider + "/" + model
}

func RefreshStyles() {
	HeaderStyle = lipgloss.NewStyle().
		Foreground(PrimaryColor).
		Bold(true).
		Padding(0, 1)

	FooterStyle = lipgloss.NewStyle().
		Foreground(DimColor).
		Padding(0, 1)

	StatusStyle = lipgloss.NewStyle().
		Foreground(SubtleColor)

	SuccessStyle = lipgloss.NewStyle().
		Foreground(SuccessColor)

	ErrorStyle = lipgloss.NewStyle().
		Foreground(ErrorColor)

	WarningStyle = lipgloss.NewStyle().
		Foreground(WarningColor)

	UserMessageStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("255")).
		Background(lipgloss.Color("59"))

	AssistantMessageStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("252"))

	SystemMessageStyle = lipgloss.NewStyle().
		Foreground(DimColor).
		Italic(true)

	ToolCallStyle = lipgloss.NewStyle().
		Foreground(AccentColor).
		Background(lipgloss.Color("236"))

	InputStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("255"))

	PromptStyle = lipgloss.NewStyle().
		Foreground(PrimaryColor)

	SpinnerStyle = lipgloss.NewStyle().
		Foreground(PrimaryColor)

	BorderStyle = lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(SubtleColor)

	FocusBorderStyle = lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(PrimaryColor)

	BoldStyle = lipgloss.NewStyle().
		Bold(true)

	ItalicStyle = lipgloss.NewStyle().
		Italic(true)

	CodeStyle = lipgloss.NewStyle().
		Foreground(AccentColor).
		Background(lipgloss.Color("236"))

	LinkStyle = lipgloss.NewStyle().
		Foreground(SecondaryColor).
		Underline(true)

	CodeBlockStyle = lipgloss.NewStyle().
		Foreground(SubtleColor).
		Background(lipgloss.Color("236"))

	CodeBlockContentStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("252"))

	ThinkingStyle = lipgloss.NewStyle().
		Foreground(WarningColor).
		Italic(true)

	ToolResultStyle = lipgloss.NewStyle().
		Foreground(SuccessColor)
}
