package canvas

import (
	"encoding/json"
	"fmt"
	"html/template"
	"strings"
)

type A2UIComponent struct {
	Type       string            `json:"type"`
	Props      map[string]any    `json:"props,omitempty"`
	Children   []A2UIComponent   `json:"children,omitempty"`
	Content    string            `json:"content,omitempty"`
	Attributes map[string]any    `json:"attributes,omitempty"`
	Events     map[string]string `json:"events,omitempty"`
	Styles     map[string]string `json:"styles,omitempty"`
}

type A2UIDocument struct {
	Version     string            `json:"version"`
	Title       string            `json:"title,omitempty"`
	Description string            `json:"description,omitempty"`
	Theme       string            `json:"theme,omitempty"`
	ThemeVars   map[string]string `json:"theme_vars,omitempty"`
	Components  []A2UIComponent   `json:"components"`
	Metadata    map[string]any    `json:"metadata,omitempty"`
}

type A2UIRenderer struct {
	themeStyles map[string]string
}

func NewA2UIRenderer() *A2UIRenderer {
	return &A2UIRenderer{
		themeStyles: defaultThemeStyles(),
	}
}

func (r *A2UIRenderer) Render(doc *A2UIDocument) (string, error) {
	if doc == nil {
		return "", fmt.Errorf("document is nil")
	}

	var sb strings.Builder
	sb.WriteString("<!DOCTYPE html>\n<html>\n<head>\n")
	sb.WriteString("<meta charset=\"UTF-8\">\n")
	sb.WriteString("<meta name=\"viewport\" content=\"width=device-width, initial-scale=1.0\">\n")

	title := "A2UI Canvas"
	if doc.Title != "" {
		title = doc.Title
	}
	sb.WriteString(fmt.Sprintf("<title>%s</title>\n", template.HTMLEscapeString(title)))
	if strings.TrimSpace(doc.Description) != "" {
		sb.WriteString(fmt.Sprintf("<meta name=\"description\" content=\"%s\">\n", template.HTMLEscapeString(doc.Description)))
	}

	sb.WriteString("<style>\n")
	sb.WriteString(r.themeStyles[doc.Theme])
	sb.WriteString(r.themeVariableStyles(doc))
	sb.WriteString(r.componentStyles())
	sb.WriteString("</style>\n")
	sb.WriteString("</head>\n")
	sb.WriteString(fmt.Sprintf("<body data-a2ui-theme=\"%s\">\n", template.HTMLEscapeString(firstNonEmptyTheme(doc.Theme, "light"))))

	for _, comp := range doc.Components {
		html, err := r.renderComponent(comp, "")
		if err != nil {
			return "", err
		}
		sb.WriteString(html)
	}

	sb.WriteString("\n</body>\n</html>")
	return sb.String(), nil
}

func (r *A2UIRenderer) renderComponent(comp A2UIComponent, indent string) (string, error) {
	switch comp.Type {
	case "container", "div":
		return r.renderContainer(comp, indent)
	case "section":
		return r.renderSection(comp, indent)
	case "stack":
		return r.renderStack(comp, indent)
	case "text", "p", "span":
		return r.renderText(comp, indent)
	case "heading", "h1", "h2", "h3", "h4", "h5", "h6":
		return r.renderHeading(comp, indent)
	case "button":
		return r.renderButton(comp, indent)
	case "input":
		return r.renderInput(comp, indent)
	case "textarea":
		return r.renderTextarea(comp, indent)
	case "image", "img":
		return r.renderImage(comp, indent)
	case "link", "a":
		return r.renderLink(comp, indent)
	case "list", "ul", "ol":
		return r.renderList(comp, indent)
	case "card":
		return r.renderCard(comp, indent)
	case "grid":
		return r.renderGrid(comp, indent)
	case "divider", "hr":
		return indent + "<hr>\n", nil
	case "code":
		return r.renderCode(comp, indent)
	case "table":
		return r.renderTable(comp, indent)
	case "progress":
		return r.renderProgress(comp, indent)
	case "badge":
		return r.renderBadge(comp, indent)
	case "alert":
		return r.renderAlert(comp, indent)
	default:
		return r.renderGeneric(comp, indent)
	}
}

func (r *A2UIRenderer) renderContainer(comp A2UIComponent, indent string) (string, error) {
	attrs := r.buildAttributes(comp)
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%s<div%s>\n", indent, attrs))
	childIndent := indent + "  "
	for _, child := range comp.Children {
		html, err := r.renderComponent(child, childIndent)
		if err != nil {
			return "", err
		}
		sb.WriteString(html)
	}
	sb.WriteString(fmt.Sprintf("%s</div>\n", indent))
	return sb.String(), nil
}

func (r *A2UIRenderer) renderSection(comp A2UIComponent, indent string) (string, error) {
	attrs := r.buildAttributes(comp)
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%s<section class=\"a2ui-section\"%s>\n", indent, attrs))
	childIndent := indent + "  "
	if title, ok := comp.Props["title"].(string); ok && strings.TrimSpace(title) != "" {
		sb.WriteString(fmt.Sprintf("%s  <h2>%s</h2>\n", indent, template.HTMLEscapeString(title)))
	}
	if description, ok := comp.Props["description"].(string); ok && strings.TrimSpace(description) != "" {
		sb.WriteString(fmt.Sprintf("%s  <p class=\"a2ui-section-description\">%s</p>\n", indent, template.HTMLEscapeString(description)))
	}
	for _, child := range comp.Children {
		html, err := r.renderComponent(child, childIndent)
		if err != nil {
			return "", err
		}
		sb.WriteString(html)
	}
	sb.WriteString(fmt.Sprintf("%s</section>\n", indent))
	return sb.String(), nil
}

func (r *A2UIRenderer) renderStack(comp A2UIComponent, indent string) (string, error) {
	attrs := r.buildAttributes(comp)
	direction := "column"
	if value, ok := comp.Props["direction"].(string); ok && strings.TrimSpace(value) != "" {
		direction = value
	}
	gap := "12px"
	if value, ok := comp.Props["gap"].(string); ok && strings.TrimSpace(value) != "" {
		gap = value
	}
	align := "stretch"
	if value, ok := comp.Props["align"].(string); ok && strings.TrimSpace(value) != "" {
		align = value
	}
	style := fmt.Sprintf(" style=\"display:flex;flex-direction:%s;gap:%s;align-items:%s;\"", template.HTMLEscapeString(direction), template.HTMLEscapeString(gap), template.HTMLEscapeString(align))
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%s<div class=\"a2ui-stack\"%s%s>\n", indent, style, attrs))
	childIndent := indent + "  "
	for _, child := range comp.Children {
		html, err := r.renderComponent(child, childIndent)
		if err != nil {
			return "", err
		}
		sb.WriteString(html)
	}
	sb.WriteString(fmt.Sprintf("%s</div>\n", indent))
	return sb.String(), nil
}

func (r *A2UIRenderer) renderText(comp A2UIComponent, indent string) (string, error) {
	tag := "p"
	if comp.Type == "span" {
		tag = "span"
	}
	attrs := r.buildAttributes(comp)
	content := template.HTMLEscapeString(comp.Content)
	return fmt.Sprintf("%s<%s%s>%s</%s>\n", indent, tag, attrs, content, tag), nil
}

func (r *A2UIRenderer) renderHeading(comp A2UIComponent, indent string) (string, error) {
	tag := comp.Type
	if tag == "heading" {
		level, _ := comp.Props["level"].(float64)
		if level == 0 {
			level = 2
		}
		tag = fmt.Sprintf("h%d", int(level))
	}
	attrs := r.buildAttributes(comp)
	content := template.HTMLEscapeString(comp.Content)
	return fmt.Sprintf("%s<%s%s>%s</%s>\n", indent, tag, attrs, content, tag), nil
}

func (r *A2UIRenderer) renderButton(comp A2UIComponent, indent string) (string, error) {
	attrs := r.buildAttributes(comp)
	content := template.HTMLEscapeString(comp.Content)
	if content == "" {
		content = "Button"
	}
	return fmt.Sprintf("%s<button%s>%s</button>\n", indent, attrs, content), nil
}

func (r *A2UIRenderer) renderInput(comp A2UIComponent, indent string) (string, error) {
	attrs := r.buildAttributes(comp)
	inputType := "text"
	if t, ok := comp.Props["inputType"].(string); ok {
		inputType = t
	}
	placeholder := ""
	if p, ok := comp.Props["placeholder"].(string); ok {
		placeholder = fmt.Sprintf(" placeholder=\"%s\"", template.HTMLEscapeString(p))
	}
	return fmt.Sprintf("%s<input type=\"%s\"%s%s>\n", indent, inputType, placeholder, attrs), nil
}

func (r *A2UIRenderer) renderTextarea(comp A2UIComponent, indent string) (string, error) {
	attrs := r.buildAttributes(comp)
	rows := "4"
	if value, ok := comp.Props["rows"].(string); ok && strings.TrimSpace(value) != "" {
		rows = value
	}
	placeholder := ""
	if p, ok := comp.Props["placeholder"].(string); ok {
		placeholder = fmt.Sprintf(" placeholder=\"%s\"", template.HTMLEscapeString(p))
	}
	content := template.HTMLEscapeString(comp.Content)
	return fmt.Sprintf("%s<textarea rows=\"%s\"%s%s>%s</textarea>\n", indent, rows, placeholder, attrs, content), nil
}

func (r *A2UIRenderer) renderImage(comp A2UIComponent, indent string) (string, error) {
	attrs := r.buildAttributes(comp)
	src := ""
	if s, ok := comp.Props["src"].(string); ok {
		src = template.HTMLEscapeString(s)
	}
	alt := ""
	if a, ok := comp.Props["alt"].(string); ok {
		alt = template.HTMLEscapeString(a)
	}
	return fmt.Sprintf("%s<img src=\"%s\" alt=\"%s\"%s>\n", indent, src, alt, attrs), nil
}

func (r *A2UIRenderer) renderLink(comp A2UIComponent, indent string) (string, error) {
	attrs := r.buildAttributes(comp)
	href := "#"
	if value, ok := comp.Props["href"].(string); ok && strings.TrimSpace(value) != "" {
		href = value
	}
	content := template.HTMLEscapeString(comp.Content)
	if content == "" {
		content = template.HTMLEscapeString(href)
	}
	return fmt.Sprintf("%s<a href=\"%s\"%s>%s</a>\n", indent, template.HTMLEscapeString(href), attrs, content), nil
}

func (r *A2UIRenderer) renderList(comp A2UIComponent, indent string) (string, error) {
	tag := "ul"
	if comp.Type == "ol" {
		tag = "ol"
	}
	attrs := r.buildAttributes(comp)
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%s<%s%s>\n", indent, tag, attrs))
	for _, child := range comp.Children {
		itemContent := template.HTMLEscapeString(child.Content)
		sb.WriteString(fmt.Sprintf("%s  <li>%s</li>\n", indent, itemContent))
	}
	sb.WriteString(fmt.Sprintf("%s</%s>\n", indent, tag))
	return sb.String(), nil
}

func (r *A2UIRenderer) renderCard(comp A2UIComponent, indent string) (string, error) {
	attrs := r.buildAttributes(comp)
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%s<div class=\"a2ui-card\"%s>\n", indent, attrs))
	childIndent := indent + "  "
	for _, child := range comp.Children {
		html, err := r.renderComponent(child, childIndent)
		if err != nil {
			return "", err
		}
		sb.WriteString(html)
	}
	sb.WriteString(fmt.Sprintf("%s</div>\n", indent))
	return sb.String(), nil
}

func (r *A2UIRenderer) renderGrid(comp A2UIComponent, indent string) (string, error) {
	attrs := r.buildAttributes(comp)
	cols := "3"
	if c, ok := comp.Props["columns"].(string); ok {
		cols = c
	}
	style := fmt.Sprintf(" style=\"display:grid;grid-template-columns:repeat(%s,1fr);gap:16px;\"", cols)
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%s<div class=\"a2ui-grid\"%s%s>\n", indent, style, attrs))
	childIndent := indent + "  "
	for _, child := range comp.Children {
		html, err := r.renderComponent(child, childIndent)
		if err != nil {
			return "", err
		}
		sb.WriteString(html)
	}
	sb.WriteString(fmt.Sprintf("%s</div>\n", indent))
	return sb.String(), nil
}

func (r *A2UIRenderer) renderCode(comp A2UIComponent, indent string) (string, error) {
	attrs := r.buildAttributes(comp)
	content := template.HTMLEscapeString(comp.Content)
	return fmt.Sprintf("%s<pre class=\"a2ui-code\"%s><code>%s</code></pre>\n", indent, attrs, content), nil
}

func (r *A2UIRenderer) renderTable(comp A2UIComponent, indent string) (string, error) {
	attrs := r.buildAttributes(comp)
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%s<table class=\"a2ui-table\"%s>\n", indent, attrs))

	if headers, ok := comp.Props["headers"].([]any); ok {
		sb.WriteString(fmt.Sprintf("%s  <thead><tr>\n", indent))
		for _, h := range headers {
			sb.WriteString(fmt.Sprintf("%s    <th>%s</th>\n", indent, template.HTMLEscapeString(fmt.Sprintf("%v", h))))
		}
		sb.WriteString(fmt.Sprintf("%s  </tr></thead>\n", indent))
	}

	if rows, ok := comp.Props["rows"].([]any); ok {
		sb.WriteString(fmt.Sprintf("%s  <tbody>\n", indent))
		for _, row := range rows {
			sb.WriteString(fmt.Sprintf("%s    <tr>\n", indent))
			if cells, ok := row.([]any); ok {
				for _, cell := range cells {
					sb.WriteString(fmt.Sprintf("%s      <td>%s</td>\n", indent, template.HTMLEscapeString(fmt.Sprintf("%v", cell))))
				}
			}
			sb.WriteString(fmt.Sprintf("%s    </tr>\n", indent))
		}
		sb.WriteString(fmt.Sprintf("%s  </tbody>\n", indent))
	}

	sb.WriteString(fmt.Sprintf("%s</table>\n", indent))
	return sb.String(), nil
}

func (r *A2UIRenderer) renderProgress(comp A2UIComponent, indent string) (string, error) {
	attrs := r.buildAttributes(comp)
	value := "0"
	if v, ok := comp.Props["value"].(string); ok {
		value = v
	}
	max := "100"
	if m, ok := comp.Props["max"].(string); ok {
		max = m
	}
	return fmt.Sprintf("%s<progress value=\"%s\" max=\"%s\"%s></progress>\n", indent, value, max, attrs), nil
}

func (r *A2UIRenderer) renderBadge(comp A2UIComponent, indent string) (string, error) {
	attrs := r.buildAttributes(comp)
	content := template.HTMLEscapeString(comp.Content)
	return fmt.Sprintf("%s<span class=\"a2ui-badge\"%s>%s</span>\n", indent, attrs, content), nil
}

func (r *A2UIRenderer) renderAlert(comp A2UIComponent, indent string) (string, error) {
	attrs := r.buildAttributes(comp)
	level := "info"
	if l, ok := comp.Props["level"].(string); ok {
		level = l
	}
	content := template.HTMLEscapeString(comp.Content)
	return fmt.Sprintf("%s<div class=\"a2ui-alert a2ui-alert-%s\"%s>%s</div>\n", indent, level, attrs, content), nil
}

func (r *A2UIRenderer) renderGeneric(comp A2UIComponent, indent string) (string, error) {
	tag := "div"
	if t, ok := comp.Props["tag"].(string); ok {
		tag = t
	}
	attrs := r.buildAttributes(comp)
	content := template.HTMLEscapeString(comp.Content)
	return fmt.Sprintf("%s<%s%s>%s</%s>\n", indent, tag, attrs, content, tag), nil
}

func (r *A2UIRenderer) buildAttributes(comp A2UIComponent) string {
	var sb strings.Builder

	if comp.Styles != nil && len(comp.Styles) > 0 {
		sb.WriteString(" style=\"")
		for k, v := range comp.Styles {
			sb.WriteString(fmt.Sprintf("%s:%s;", k, v))
		}
		sb.WriteString("\"")
	}

	if comp.Attributes != nil {
		for k, v := range comp.Attributes {
			if k == "style" {
				continue
			}
			sb.WriteString(fmt.Sprintf(" %s=\"%v\"", k, v))
		}
	}

	if comp.Events != nil && len(comp.Events) > 0 {
		eventsJSON, _ := json.Marshal(comp.Events)
		sb.WriteString(fmt.Sprintf(" data-a2ui-events=\"%s\"", template.HTMLEscapeString(string(eventsJSON))))
	}

	if id, ok := comp.Props["id"].(string); ok {
		sb.WriteString(fmt.Sprintf(" id=\"%s\"", template.HTMLEscapeString(id)))
	}

	if className, ok := comp.Props["className"].(string); ok {
		sb.WriteString(fmt.Sprintf(" class=\"%s\"", template.HTMLEscapeString(className)))
	}

	return sb.String()
}

func (r *A2UIRenderer) componentStyles() string {
	return `
.a2ui-section {
  margin-bottom: 24px;
}
.a2ui-section-description {
  color: #5b5249;
  margin-top: -8px;
}
.a2ui-card {
  background: #fff;
  border-radius: 12px;
  padding: 20px;
  box-shadow: 0 2px 8px rgba(0,0,0,0.08);
  margin-bottom: 16px;
}
.a2ui-code {
  background: #1e1e1e;
  color: #d4d4d4;
  padding: 16px;
  border-radius: 8px;
  overflow-x: auto;
  font-family: 'Cascadia Code', Consolas, monospace;
  font-size: 14px;
}
.a2ui-table {
  width: 100%;
  border-collapse: collapse;
  margin: 16px 0;
}
.a2ui-table th, .a2ui-table td {
  padding: 10px 14px;
  text-align: left;
  border-bottom: 1px solid #e5e7eb;
}
.a2ui-table th {
  background: #f9fafb;
  font-weight: 600;
}
.a2ui-badge {
  display: inline-block;
  padding: 4px 10px;
  border-radius: 999px;
  font-size: 12px;
  font-weight: 600;
  background: #e0e7ff;
  color: #3730a3;
}
.a2ui-alert {
  padding: 14px 18px;
  border-radius: 10px;
  margin: 12px 0;
  font-size: 14px;
}
.a2ui-alert-info {
  background: #eff6ff;
  color: #1e40af;
  border: 1px solid #bfdbfe;
}
.a2ui-alert-success {
  background: #f0fdf4;
  color: #166534;
  border: 1px solid #bbf7d0;
}
.a2ui-alert-warning {
  background: #fffbeb;
  color: #92400e;
  border: 1px solid #fde68a;
}
.a2ui-alert-error {
  background: #fef2f2;
  color: #991b1b;
  border: 1px solid #fecaca;
}
body {
  font-family: 'Aptos', 'Segoe UI Variable', 'Segoe UI', sans-serif;
  color: #1c1a18;
  background: #f7f1e6;
  padding: 24px;
  line-height: 1.6;
}
button {
  padding: 10px 18px;
  border-radius: 10px;
  border: none;
  background: linear-gradient(135deg, #0f766e, #115e59);
  color: white;
  font-weight: 600;
  cursor: pointer;
  transition: transform 0.15s ease;
}
button:hover {
  transform: translateY(-1px);
}
input {
  padding: 10px 14px;
  border-radius: 10px;
  border: 1px solid #d1d5db;
  font: inherit;
  width: 100%;
  max-width: 400px;
}
textarea {
  padding: 12px 14px;
  border-radius: 12px;
  border: 1px solid #d1d5db;
  font: inherit;
  width: 100%;
  min-height: 120px;
  resize: vertical;
}
a {
  color: #0f766e;
  text-decoration: none;
}
a:hover {
  text-decoration: underline;
}
`
}

func defaultThemeStyles() map[string]string {
	return map[string]string{
		"":        "",
		"light":   "",
		"dark":    "body { background: #1a1a2e; color: #e0e0e0; } .a2ui-card { background: #16213e; } .a2ui-table th { background: #0f3460; } .a2ui-table td { border-color: #1a1a2e; }",
		"minimal": "body { background: #fff; color: #333; padding: 16px; } .a2ui-card { box-shadow: none; border: 1px solid #eee; }",
	}
}

func (r *A2UIRenderer) themeVariableStyles(doc *A2UIDocument) string {
	if doc == nil || len(doc.ThemeVars) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString(":root{")
	for key, value := range doc.ThemeVars {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" || value == "" {
			continue
		}
		sb.WriteString(fmt.Sprintf("--%s:%s;", template.HTMLEscapeString(key), template.HTMLEscapeString(value)))
	}
	sb.WriteString("}\n")
	return sb.String()
}

func firstNonEmptyTheme(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func ParseA2UI(content string) (*A2UIDocument, error) {
	var doc A2UIDocument
	if err := json.Unmarshal([]byte(content), &doc); err != nil {
		return nil, fmt.Errorf("parse a2ui: %w", err)
	}
	if doc.Version == "" {
		doc.Version = "1.0"
	}
	return &doc, nil
}
