import { describe, expect, it } from "vitest";

import { normalizeSkillDescription } from "./skillDescription";
import {
  mergeOverviewAgentNames,
  mergeOverviewSkillNames,
  resolveSkillEnabled,
  resolveSkillLoaded,
} from "./useWorkspaceOverview";

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

describe("workspace overview skill fallback", () => {
  it("keeps snapshotted skills enabled and loaded when live data is unavailable", () => {
    expect(resolveSkillEnabled()).toBe(true);
    expect(resolveSkillLoaded()).toBe(true);
  });

  it("prefers explicit live flags when they are available", () => {
    expect(resolveSkillEnabled({ enabled: false })).toBe(false);
    expect(resolveSkillEnabled({ loaded: true })).toBe(true);
    expect(resolveSkillLoaded({ loaded: false })).toBe(false);
  });

  it("appends live-only skills to the overview", () => {
    expect(mergeOverviewSkillNames([{ name: "live-only-skill" }])).toContain("live-only-skill");
  });
});

describe("workspace overview agent merge", () => {
  it("appends live-only agents to the overview", () => {
    expect(mergeOverviewAgentNames([{ name: "Live Agent" }])).toContain("Live Agent");
  });
});
