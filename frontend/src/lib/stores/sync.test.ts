import { describe, it, expect } from "vitest";
import { commitsDisagree } from "./sync.svelte.js";

describe("commitsDisagree", () => {
  it("returns false when both are unknown", () => {
    expect(commitsDisagree("unknown", "unknown")).toBe(false);
  });

  it("returns false when frontend is unknown", () => {
    expect(commitsDisagree("unknown", "abc1234")).toBe(false);
  });

  it("returns false when server is unknown", () => {
    expect(commitsDisagree("abc1234", "unknown")).toBe(false);
  });

  it("returns false for identical short hashes", () => {
    expect(commitsDisagree("abc1234", "abc1234")).toBe(false);
  });

  it("returns false when short matches full SHA prefix", () => {
    expect(
      commitsDisagree("abc1234", "abc1234def5678"),
    ).toBe(false);
  });

  it("returns true for different hashes", () => {
    expect(commitsDisagree("abc1234", "def5678")).toBe(true);
  });

  it("returns true for full SHAs that differ", () => {
    expect(
      commitsDisagree(
        "abc1234aaaaaaaaaaaa",
        "def5678bbbbbbbbbbb",
      ),
    ).toBe(true);
  });

  it("returns true for full SHAs sharing 7-char prefix", () => {
    expect(
      commitsDisagree(
        "abc1234aaaaaaaaaaaa",
        "abc1234bbbbbbbbbbb",
      ),
    ).toBe(true);
  });

  it("returns false for identical full SHAs", () => {
    expect(
      commitsDisagree(
        "abc1234aaaaaaaaaaaa",
        "abc1234aaaaaaaaaaaa",
      ),
    ).toBe(false);
  });
});
