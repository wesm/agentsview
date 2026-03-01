import { describe, it, expect } from "vitest";
import {
  KNOWN_AGENTS,
  agentColor,
  agentLabel,
} from "./agents.js";

describe("KNOWN_AGENTS", () => {
  it("contains all expected agents", () => {
    const names = KNOWN_AGENTS.map((a) => a.name);
    expect(names).toEqual([
      "claude",
      "codex",
      "copilot",
      "gemini",
      "opencode",
      "cursor",
      "amp",
      "pi",
    ]);
  });

  it("has a color for every agent", () => {
    for (const agent of KNOWN_AGENTS) {
      expect(agent.color).toMatch(/^var\(--accent-/);
    }
  });
});

describe("agentColor", () => {
  it("returns correct color for known agents", () => {
    expect(agentColor("claude")).toBe(
      "var(--accent-blue)",
    );
    expect(agentColor("codex")).toBe(
      "var(--accent-green)",
    );
    expect(agentColor("copilot")).toBe(
      "var(--accent-amber)",
    );
    expect(agentColor("gemini")).toBe(
      "var(--accent-rose)",
    );
    expect(agentColor("opencode")).toBe(
      "var(--accent-purple)",
    );
    expect(agentColor("cursor")).toBe(
      "var(--accent-black)",
    );
    expect(agentColor("amp")).toBe(
      "var(--accent-coral)",
    );
  });

  it("returns correct color for pi", () => {
    expect(agentColor("pi")).toBe("var(--accent-teal)");
  });

  it("falls back to blue for unknown agents", () => {
    expect(agentColor("unknown")).toBe(
      "var(--accent-blue)",
    );
    expect(agentColor("")).toBe("var(--accent-blue)");
  });
});

describe("agentLabel", () => {
  it("returns label field when present", () => {
    expect(agentLabel("pi")).toBe("Pi");
  });

  it("capitalizes first letter for agents without label", () => {
    expect(agentLabel("claude")).toBe("Claude");
    expect(agentLabel("gemini")).toBe("Gemini");
  });

  it("handles unknown agents", () => {
    expect(agentLabel("unknown")).toBe("Unknown");
    expect(agentLabel("")).toBe("");
  });
});
