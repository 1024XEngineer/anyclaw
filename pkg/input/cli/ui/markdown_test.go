package ui

import (
	"strings"
	"testing"
)

func TestRenderMarkdownFormatsHeadingsAndLists(t *testing.T) {
	rendered := RenderMarkdown("# Title\n- item one\n1. item two")

	if strings.Contains(rendered, "# Title") {
		t.Fatal("expected heading marker to be removed")
	}
	if strings.Contains(rendered, "- item one") {
		t.Fatal("expected bullet marker to be rendered")
	}
	if !strings.Contains(rendered, "Title") {
		t.Fatal("expected heading content to be present")
	}
	if !strings.Contains(rendered, "item one") || !strings.Contains(rendered, "item two") {
		t.Fatal("expected list content to be present")
	}
}

func TestRenderMarkdownFormatsCodeBlocksAndInlineCode(t *testing.T) {
	rendered := RenderMarkdown("Use `fmt.Println` here.\n```go\nfmt.Println(\"hi\")\n```")

	if strings.Contains(rendered, "```") {
		t.Fatal("expected fence markers to be removed")
	}
	if !strings.Contains(rendered, "fmt.Println") {
		t.Fatal("expected code content to be present")
	}
	if !strings.Contains(rendered, "GO") {
		t.Fatal("expected code block language label to be rendered")
	}
}

func TestRenderMarkdownFormatsTaskListsAndLinks(t *testing.T) {
	rendered := RenderMarkdown("- [x] done\n- [ ] pending\nRead [Guide](https://example.com/docs)")

	if strings.Contains(rendered, "- [x]") || strings.Contains(rendered, "- [ ]") {
		t.Fatal("expected task list markers to be reformatted")
	}
	if !strings.Contains(rendered, "[x] done") || !strings.Contains(rendered, "[ ] pending") {
		t.Fatal("expected task states to be present")
	}
	if !strings.Contains(rendered, "Guide") || !strings.Contains(rendered, "https://example.com/docs") {
		t.Fatal("expected markdown link label and URL to be present")
	}
}

func TestRenderMarkdownFormatsTables(t *testing.T) {
	rendered := RenderMarkdown("| Name | Status |\n| --- | --- |\n| Build | Ready |\n| Ship | Waiting |")

	if strings.Contains(rendered, "| --- | --- |") {
		t.Fatal("expected markdown separator row to be reformatted")
	}
	if !strings.Contains(rendered, "Name") || !strings.Contains(rendered, "Build") || !strings.Contains(rendered, "Waiting") {
		t.Fatal("expected table content to be present")
	}
}
