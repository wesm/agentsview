import {
  describe,
  it,
  expect,
  beforeEach,
} from "vitest";
import { starred } from "./starred.svelte.js";

describe("StarredStore", () => {
  beforeEach(() => {
    // Clear all starred sessions
    for (const id of [...starred.ids]) {
      starred.unstar(id);
    }
  });

  it("starts empty", () => {
    expect(starred.count).toBe(0);
    expect(starred.isStarred("abc")).toBe(false);
  });

  it("can star a session", () => {
    starred.star("session-1");
    expect(starred.isStarred("session-1")).toBe(true);
    expect(starred.count).toBe(1);
  });

  it("can unstar a session", () => {
    starred.star("session-1");
    starred.unstar("session-1");
    expect(starred.isStarred("session-1")).toBe(false);
    expect(starred.count).toBe(0);
  });

  it("toggle adds then removes", () => {
    starred.toggle("session-2");
    expect(starred.isStarred("session-2")).toBe(true);

    starred.toggle("session-2");
    expect(starred.isStarred("session-2")).toBe(false);
  });

  it("star is idempotent", () => {
    starred.star("s1");
    starred.star("s1");
    expect(starred.count).toBe(1);
  });

  it("unstar is idempotent", () => {
    starred.unstar("nonexistent");
    expect(starred.count).toBe(0);
  });

  it("handles multiple starred sessions", () => {
    starred.star("a");
    starred.star("b");
    starred.star("c");
    expect(starred.count).toBe(3);
    expect(starred.isStarred("a")).toBe(true);
    expect(starred.isStarred("b")).toBe(true);
    expect(starred.isStarred("c")).toBe(true);
    expect(starred.isStarred("d")).toBe(false);
  });

  describe("filterOnly", () => {
    it("defaults to false", () => {
      expect(starred.filterOnly).toBe(false);
    });

    it("can be toggled on and off", () => {
      starred.filterOnly = true;
      expect(starred.filterOnly).toBe(true);
      starred.filterOnly = false;
      expect(starred.filterOnly).toBe(false);
    });

    it("is independent of starred ids", () => {
      starred.star("s1");
      starred.filterOnly = true;
      starred.unstar("s1");
      expect(starred.filterOnly).toBe(true);
      expect(starred.count).toBe(0);
    });
  });
});
