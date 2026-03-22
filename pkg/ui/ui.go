package ui

import (
	"fmt"
	"strings"
)

type Style struct {
	prefix string
	suffix string
}

var (
	Reset  = Style{"", ""}
	Bold   = Style{"\033[1m", "\033[0m"}
	Faint  = Style{"\033[2m", "\033[0m"}
	Italic = Style{"\033[3m", "\033[0m"}

	Black   = Style{"\033[30m", "\033[0m"}
	Red     = Style{"\033[31m", "\033[0m"}
	Green   = Style{"\033[32m", "\033[0m"}
	Yellow  = Style{"\033[33m", "\033[0m"}
	Blue    = Style{"\033[34m", "\033[0m"}
	Magenta = Style{"\033[35m", "\033[0m"}
	Cyan    = Style{"\033[36m", "\033[0m"}
	White   = Style{"\033[37m", "\033[0m"}

	BgBlack   = Style{"\033[40m", "\033[0m"}
	BgRed     = Style{"\033[41m", "\033[0m"}
	BgGreen   = Style{"\033[42m", "\033[0m"}
	BgYellow  = Style{"\033[43m", "\033[0m"}
	BgBlue    = Style{"\033[44m", "\033[0m"}
	BgMagenta = Style{"\033[45m", "\033[0m"}
	BgCyan    = Style{"\033[46m", "\033[0m"}
	BgWhite   = Style{"\033[47m", "\033[0m"}

	Success = Green
	Info    = Cyan
	Warning = Yellow
	Error   = Red
	Dim     = Faint
)

func (s Style) Text(text string) string {
	return s.prefix + text + s.suffix
}

func (s Style) Sprint(args ...interface{}) string {
	return s.prefix + fmt.Sprint(args...) + s.suffix
}

func (s Style) Sprintf(format string, args ...interface{}) string {
	return s.prefix + fmt.Sprintf(format, args...) + s.suffix
}

type UI struct {
	indent int
}

func New() *UI {
	return &UI{}
}

func (u *UI) Indent(level int) *UI {
	return &UI{indent: level}
}

func (u *UI) prefixStr() string {
	return strings.Repeat("  ", u.indent)
}

func (u *UI) Print(text string) {
	fmt.Print(u.prefixStr() + text)
}

func (u *UI) Println(text string) {
	fmt.Println(u.prefixStr() + text)
}

func (u *UI) Printf(format string, args ...interface{}) {
	fmt.Print(u.prefixStr() + fmt.Sprintf(format, args...))
}

func (u *UI) Success(text string) {
	u.Println(Success.Sprint("✓ ") + text)
}

func (u *UI) Error(text string) {
	u.Println(Error.Sprint("✗ ") + text)
}

func (u *UI) Info(text string) {
	u.Println(Info.Sprint("ℹ ") + text)
}

func (u *UI) Warning(text string) {
	u.Println(Warning.Sprint("⚠ ") + text)
}

func (u *UI) Header(text string) {
	u.Println("")
	u.Println(Bold.Sprint(text))
}

func (u *UI) Item(label, value string) {
	u.Printf("%-12s %s\n", Dim.Sprint(label+":")+" ", value)
}

func (u *UI) KeyValue(key string, value ...interface{}) {
	u.Printf("%-16s %s\n", Cyan.Sprint(key), fmt.Sprint(value...))
}

func (u *UI) Divider() {
	u.Println(Dim.Sprint(strings.Repeat("─", 50)))
}

func (u *UI) Spacer() {
	fmt.Println()
}

func (u *UI) Box(title, content string) {
	lines := strings.Split(content, "\n")
	width := 60
	if len(title) > width-4 {
		width = len(title) + 6
	}
	for _, line := range lines {
		if len(line) > width-4 {
			width = len(line) + 6
		}
	}

	border := "┌" + strings.Repeat("─", width-2) + "┐"
	bottom := "└" + strings.Repeat("─", width-2) + "┘"

	u.Println("")
	u.Println(Cyan.Sprint(border))
	u.Println(Cyan.Sprint("│ ") + Bold.Sprint(title) + strings.Repeat(" ", width-4-len(title)) + Cyan.Sprint(" │"))
	u.Println(Cyan.Sprint("├") + strings.Repeat("─", width-2) + Cyan.Sprint("┤"))

	for _, line := range lines {
		padding := width - 4 - len(line)
		if padding < 0 {
			padding = 0
		}
		u.Println(Cyan.Sprint("│ ") + line + strings.Repeat(" ", padding) + Cyan.Sprint(" │"))
	}

	u.Println(Cyan.Sprint(bottom))
	u.Println("")
}

func Banner() {
	banner := `
` + Cyan.Sprint(`    ╔══════════════════════════════════════════╗`) + `
` + Cyan.Sprint(`    ║`) + `  ` + Bold.Sprint("AnyClaw") + `  ` + Dim.Sprint("v2026.3.13") + `
` + Cyan.Sprint(`    ║`) + `  ` + Faint.Sprint("基于文件的记忆 AI 智能体") + `
` + Cyan.Sprint(`    ╚══════════════════════════════════════════╝`)

	fmt.Println(banner)
}

func Prompt() string {
	return Cyan.Sprint("❯ ") + " "
}

func Response(text string) string {
	return White.Sprint(text)
}

func System(text string) string {
	return Dim.Sprint(text)
}

func Spinner(step int) string {
	frames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	return Cyan.Sprint(frames[step%len(frames)])
}
