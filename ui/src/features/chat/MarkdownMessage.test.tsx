import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { MarkdownMessage } from "./MarkdownMessage";

describe("MarkdownMessage", () => {
  it("renders markdown structure and links", () => {
    render(
      <MarkdownMessage
        content={[
          "# 标题",
          "",
          "- 第一项",
          "- 第二项",
          "",
          "这是 **重点** 和 `code`。",
          "",
          "[OpenAI](https://openai.com)",
        ].join("\n")}
      />,
    );

    expect(screen.getByRole("heading", { name: "标题" })).toBeInTheDocument();
    expect(screen.getByText("第一项")).toBeInTheDocument();
    expect(screen.getByText("重点")).toBeInTheDocument();
    expect(screen.getByText("code")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "OpenAI" })).toHaveAttribute("href", "https://openai.com");
    expect(screen.getByRole("link", { name: "OpenAI" })).toHaveAttribute("target", "_blank");
  });

  it("does not render unsafe markdown urls as clickable links", () => {
    render(<MarkdownMessage content="[危险链接](javascript:alert(1))" />);

    expect(screen.getByText("危险链接")).toBeInTheDocument();
    expect(screen.queryByRole("link", { name: "危险链接" })).not.toBeInTheDocument();
  });
});
