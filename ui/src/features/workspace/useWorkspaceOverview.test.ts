import { describe, expect, it } from "vitest";

import { normalizeSkillDescription } from "./skillDescription";

describe("normalizeSkillDescription", () => {
  it("strips wrapping quotes and inline code markers", () => {
    expect(normalizeSkillDescription('"Interact with GitHub using the `gh` CLI."', "fallback")).toBe(
      "Interact with GitHub using the gh CLI.",
    );
  });

  it("falls back when the description is empty", () => {
    expect(normalizeSkillDescription("   ", "fallback text")).toBe("fallback text");
  });

  it("compacts markdown links into readable text", () => {
    expect(normalizeSkillDescription("Read [the docs](https://example.com) first.", "fallback")).toBe(
      "Read the docs first.",
    );
  });
});
