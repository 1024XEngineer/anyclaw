package ui

import (
	"regexp"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	mdHeading1Style = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#7C3AED"))
	mdHeading2Style = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#06B6D4"))
	mdHeading3Style = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#10B981"))
	mdInlineCode    = lipgloss.NewStyle().Foreground(lipgloss.Color("#F59E0B")).Background(lipgloss.Color("#1F2937")).Padding(0, 1)
	mdCodeBlock     = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#334155")).
			Padding(0, 1).
			Margin(1, 0)
	mdQuoteStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("245")).BorderLeft(true).BorderForeground(lipgloss.Color("#475569")).PaddingLeft(1)
	mdStrikeStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Strikethrough(true)
	mdLinkTextStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#60A5FA")).Underline(true)
	mdLinkURLStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	mdTaskDoneStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#10B981")).Bold(true)
	mdTaskPendingStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#94A3B8")).Bold(true)
	mdTableBorderStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	mdTableHeaderStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#06B6D4"))

	mdFenceRE        = regexp.MustCompile("^```\\s*([A-Za-z0-9_+.-]*)\\s*$")
	mdHeadingRE      = regexp.MustCompile(`^(#{1,6})\s+(.*)$`)
	mdTaskRE         = regexp.MustCompile(`^(\s*)[-*+]\s+\[([ xX])\]\s+(.*)$`)
	mdBulletRE       = regexp.MustCompile(`^(\s*)[-*+]\s+(.*)$`)
	mdNumberRE       = regexp.MustCompile(`^(\s*)(\d+)\.\s+(.*)$`)
	mdQuoteRE        = regexp.MustCompile(`^\s*>\s?(.*)$`)
	mdStrongRE       = regexp.MustCompile(`\*\*(.+?)\*\*`)
	mdEmRE           = regexp.MustCompile(`\*(.+?)\*`)
	mdStrikeRE       = regexp.MustCompile(`~~(.+?)~~`)
	mdLinkRE         = regexp.MustCompile(`\[([^\]]+)\]\(([^)\s]+)\)`)
	mdHRRE           = regexp.MustCompile(`^\s*(?:-{3,}|\*{3,}|_{3,})\s*$`)
	mdTableSepCellRE = regexp.MustCompile(`^:?-{3,}:?$`)
)

// RenderMarkdown renders a lightweight markdown view suitable for terminal chat output.
func RenderMarkdown(input string) string {
	if strings.TrimSpace(input) == "" {
		return ""
	}

	lines := strings.Split(strings.ReplaceAll(input, "\r\n", "\n"), "\n")
	out := make([]string, 0, len(lines))

	var codeLines []string
	codeLang := ""
	inCodeBlock := false

	flushCodeBlock := func() {
		header := ""
		if codeLang != "" {
			header = Dim.Sprint(strings.ToUpper(codeLang)) + "\n"
		}
		block := header + strings.Join(codeLines, "\n")
		out = append(out, mdCodeBlock.Render(block))
		codeLines = nil
		codeLang = ""
	}

	for i := 0; i < len(lines); i++ {
		rawLine := lines[i]
		line := strings.TrimRight(rawLine, " \t")
		if matches := mdFenceRE.FindStringSubmatch(strings.TrimSpace(line)); matches != nil {
			if inCodeBlock {
				flushCodeBlock()
				inCodeBlock = false
			} else {
				inCodeBlock = true
				codeLang = matches[1]
			}
			continue
		}

		if inCodeBlock {
			codeLines = append(codeLines, line)
			continue
		}

		if block, next, ok := renderMarkdownTableBlock(lines, i); ok {
			out = append(out, block)
			i = next - 1
			continue
		}

		if strings.TrimSpace(line) == "" {
			out = append(out, "")
			continue
		}

		if mdHRRE.MatchString(line) {
			out = append(out, Dim.Sprint(strings.Repeat("-", 56)))
			continue
		}

		if matches := mdHeadingRE.FindStringSubmatch(line); matches != nil {
			content := renderInlineMarkdown(matches[2])
			switch len(matches[1]) {
			case 1:
				out = append(out, mdHeading1Style.Render(content))
			case 2:
				out = append(out, mdHeading2Style.Render(content))
			default:
				out = append(out, mdHeading3Style.Render(content))
			}
			continue
		}

		if matches := mdQuoteRE.FindStringSubmatch(line); matches != nil {
			out = append(out, mdQuoteStyle.Render(renderInlineMarkdown(matches[1])))
			continue
		}

		if matches := mdTaskRE.FindStringSubmatch(line); matches != nil {
			markerStyle := mdTaskPendingStyle
			marker := "[ ]"
			if strings.EqualFold(matches[2], "x") {
				markerStyle = mdTaskDoneStyle
				marker = "[x]"
			}
			out = append(out, matches[1]+markerStyle.Render(marker)+" "+renderInlineMarkdown(matches[3]))
			continue
		}

		if matches := mdBulletRE.FindStringSubmatch(line); matches != nil {
			out = append(out, matches[1]+Cyan.Sprint("* ")+renderInlineMarkdown(matches[2]))
			continue
		}

		if matches := mdNumberRE.FindStringSubmatch(line); matches != nil {
			out = append(out, matches[1]+Yellow.Sprint(matches[2]+". ")+renderInlineMarkdown(matches[3]))
			continue
		}

		out = append(out, renderInlineMarkdown(line))
	}

	if inCodeBlock {
		flushCodeBlock()
	}

	return strings.Join(out, "\n")
}

func renderInlineMarkdown(input string) string {
	if input == "" {
		return ""
	}

	parts := strings.Split(input, "`")
	for i := range parts {
		if i%2 == 1 {
			parts[i] = mdInlineCode.Render(parts[i])
			continue
		}
		parts[i] = renderInlineText(parts[i])
	}

	return strings.Join(parts, "")
}

func renderInlineText(input string) string {
	if input == "" {
		return ""
	}

	input = mdLinkRE.ReplaceAllStringFunc(input, func(match string) string {
		groups := mdLinkRE.FindStringSubmatch(match)
		if len(groups) < 3 {
			return match
		}
		label := renderInlineTextNoLinks(groups[1])
		return mdLinkTextStyle.Render(label) + mdLinkURLStyle.Render(" ("+groups[2]+")")
	})
	return renderInlineTextNoLinks(input)
}

func renderInlineTextNoLinks(input string) string {
	input = mdStrongRE.ReplaceAllStringFunc(input, func(match string) string {
		groups := mdStrongRE.FindStringSubmatch(match)
		if len(groups) < 2 {
			return match
		}
		return Bold.Sprint(groups[1])
	})
	input = mdEmRE.ReplaceAllStringFunc(input, func(match string) string {
		groups := mdEmRE.FindStringSubmatch(match)
		if len(groups) < 2 {
			return match
		}
		return Cyan.Sprint(groups[1])
	})
	input = mdStrikeRE.ReplaceAllStringFunc(input, func(match string) string {
		groups := mdStrikeRE.FindStringSubmatch(match)
		if len(groups) < 2 {
			return match
		}
		return mdStrikeStyle.Render(groups[1])
	})
	return input
}

func renderMarkdownTableBlock(lines []string, start int) (string, int, bool) {
	if start+1 >= len(lines) {
		return "", start, false
	}

	headerLine := strings.TrimRight(lines[start], " \t")
	separatorLine := strings.TrimRight(lines[start+1], " \t")
	if !strings.Contains(headerLine, "|") || !strings.Contains(separatorLine, "|") {
		return "", start, false
	}

	headerCells := splitMarkdownTableLine(headerLine)
	if len(headerCells) == 0 || !isMarkdownTableSeparator(separatorLine, len(headerCells)) {
		return "", start, false
	}

	rows := [][]string{headerCells}
	next := start + 2
	for next < len(lines) {
		line := strings.TrimRight(lines[next], " \t")
		if strings.TrimSpace(line) == "" || !strings.Contains(line, "|") {
			break
		}
		rows = append(rows, normalizeTableCells(splitMarkdownTableLine(line), len(headerCells)))
		next++
	}

	return renderMarkdownTable(rows), next, true
}

func splitMarkdownTableLine(line string) []string {
	trimmed := strings.TrimSpace(line)
	if !strings.Contains(trimmed, "|") {
		return nil
	}

	trimmed = strings.TrimPrefix(trimmed, "|")
	trimmed = strings.TrimSuffix(trimmed, "|")
	parts := strings.Split(trimmed, "|")
	cells := make([]string, 0, len(parts))
	for _, part := range parts {
		cells = append(cells, strings.TrimSpace(part))
	}
	return cells
}

func isMarkdownTableSeparator(line string, columns int) bool {
	cells := splitMarkdownTableLine(line)
	if len(cells) == 0 {
		return false
	}
	if columns > 0 && len(cells) != columns {
		return false
	}
	for _, cell := range cells {
		if !mdTableSepCellRE.MatchString(strings.TrimSpace(cell)) {
			return false
		}
	}
	return true
}

func normalizeTableCells(cells []string, columns int) []string {
	if columns <= 0 {
		return cells
	}
	normalized := make([]string, columns)
	copy(normalized, cells)
	return normalized
}

func renderMarkdownTable(rows [][]string) string {
	if len(rows) == 0 {
		return ""
	}

	columnCount := 0
	for _, row := range rows {
		if len(row) > columnCount {
			columnCount = len(row)
		}
	}
	if columnCount == 0 {
		return ""
	}

	renderedRows := make([][]string, len(rows))
	widths := make([]int, columnCount)
	for i, row := range rows {
		normalized := normalizeTableCells(row, columnCount)
		renderedRows[i] = make([]string, columnCount)
		for j, cell := range normalized {
			rendered := renderInlineMarkdown(cell)
			renderedRows[i][j] = rendered
			if width := lipgloss.Width(rendered); width > widths[j] {
				widths[j] = width
			}
		}
	}

	for i := range widths {
		if widths[i] < 3 {
			widths[i] = 3
		}
	}

	out := []string{
		buildMarkdownTableRow(renderedRows[0], widths, true),
		buildMarkdownTableSeparator(widths),
	}
	for _, row := range renderedRows[1:] {
		out = append(out, buildMarkdownTableRow(row, widths, false))
	}
	return strings.Join(out, "\n")
}

func buildMarkdownTableRow(cells []string, widths []int, header bool) string {
	parts := make([]string, 0, len(widths)*2+1)
	parts = append(parts, mdTableBorderStyle.Render("|"))
	for i, width := range widths {
		cell := ""
		if i < len(cells) {
			cell = cells[i]
		}
		if header {
			cell = mdTableHeaderStyle.Render(cell)
		}
		padded := lipgloss.NewStyle().Width(width).Render(cell)
		parts = append(parts, " "+padded+" ", mdTableBorderStyle.Render("|"))
	}
	return strings.Join(parts, "")
}

func buildMarkdownTableSeparator(widths []int) string {
	parts := []string{mdTableBorderStyle.Render("|")}
	for _, width := range widths {
		parts = append(parts, mdTableBorderStyle.Render(strings.Repeat("-", width+2)), mdTableBorderStyle.Render("|"))
	}
	return strings.Join(parts, "")
}
