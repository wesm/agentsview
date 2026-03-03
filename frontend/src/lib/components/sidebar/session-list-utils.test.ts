import { describe, it, expect } from "vitest";
import type { Session } from "../../api/types.js";
import type { SessionGroup } from "../../stores/sessions.svelte.js";
import {
  ITEM_HEIGHT,
  HEADER_HEIGHT,
  STORAGE_KEY,
  buildAgentSections,
  buildDisplayItems,
  computeTotalSize,
  findStart,
} from "./session-list-utils.js";
import type { AgentSection, DisplayItem } from "./session-list-utils.js";

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function makeSession(overrides: Partial<Session> = {}): Session {
  return {
    id: overrides.id ?? crypto.randomUUID(),
    project: "test-project",
    machine: "localhost",
    agent: "claude",
    first_message: "hello",
    started_at: "2025-01-01T00:00:00Z",
    ended_at: "2025-01-01T01:00:00Z",
    message_count: 10,
    user_message_count: 5,
    created_at: "2025-01-01T00:00:00Z",
    ...overrides,
  };
}

function makeGroup(
  agent: string,
  sessionCount = 1,
  idPrefix?: string,
): SessionGroup {
  const prefix = idPrefix ?? agent;
  const sessions: Session[] = [];
  for (let i = 0; i < sessionCount; i++) {
    sessions.push(
      makeSession({
        id: `${prefix}-session-${i}`,
        agent,
      }),
    );
  }
  return {
    key: sessions[0]!.id,
    project: "test-project",
    sessions,
    primarySessionId: sessions[0]!.id,
    totalMessages: sessions.reduce((s, x) => s + x.message_count, 0),
    firstMessage: sessions[0]!.first_message,
    startedAt: sessions[0]!.started_at,
    endedAt: sessions[sessions.length - 1]!.ended_at,
  };
}

// ---------------------------------------------------------------------------
// buildAgentSections
// ---------------------------------------------------------------------------

describe("buildAgentSections", () => {
  it("returns empty array when groupByAgent is false", () => {
    const groups = [makeGroup("claude"), makeGroup("gpt")];
    const result = buildAgentSections(groups, false);
    expect(result).toEqual([]);
  });

  it("groups session groups by agent name", () => {
    const groups = [
      makeGroup("claude", 1, "c1"),
      makeGroup("gpt", 1, "g1"),
      makeGroup("claude", 1, "c2"),
    ];
    const result = buildAgentSections(groups, true);

    expect(result).toHaveLength(2);
    const claudeSection = result.find((s) => s.agent === "claude");
    const gptSection = result.find((s) => s.agent === "gpt");
    expect(claudeSection).toBeDefined();
    expect(gptSection).toBeDefined();
    expect(claudeSection!.groups).toHaveLength(2);
    expect(gptSection!.groups).toHaveLength(1);
  });

  it("sorts sections by count descending", () => {
    const groups = [
      makeGroup("gpt", 1, "g1"),
      makeGroup("claude", 1, "c1"),
      makeGroup("claude", 1, "c2"),
      makeGroup("claude", 1, "c3"),
      makeGroup("gpt", 1, "g2"),
    ];
    const result = buildAgentSections(groups, true);

    expect(result[0]!.agent).toBe("claude");
    expect(result[0]!.groups).toHaveLength(3);
    expect(result[1]!.agent).toBe("gpt");
    expect(result[1]!.groups).toHaveLength(2);
  });

  it("uses primary session to determine agent", () => {
    const session = makeSession({ id: "primary-1", agent: "gemini" });
    const group: SessionGroup = {
      key: "primary-1",
      project: "test",
      sessions: [session],
      primarySessionId: "primary-1",
      totalMessages: 10,
      firstMessage: "hi",
      startedAt: "2025-01-01T00:00:00Z",
      endedAt: "2025-01-01T01:00:00Z",
    };
    const result = buildAgentSections([group], true);

    expect(result).toHaveLength(1);
    expect(result[0]!.agent).toBe("gemini");
  });

  it("skips groups with no sessions", () => {
    const emptyGroup: SessionGroup = {
      key: "empty",
      project: "test",
      sessions: [],
      primarySessionId: "nonexistent",
      totalMessages: 0,
      firstMessage: null,
      startedAt: null,
      endedAt: null,
    };
    const result = buildAgentSections([emptyGroup], true);
    expect(result).toEqual([]);
  });
});

// ---------------------------------------------------------------------------
// buildDisplayItems — ungrouped mode
// ---------------------------------------------------------------------------

describe("buildDisplayItems (ungrouped)", () => {
  it("creates flat session items with correct ids", () => {
    const groups = [
      makeGroup("claude", 1, "a"),
      makeGroup("gpt", 1, "b"),
    ];
    const items = buildDisplayItems(groups, [], false, new Set());

    expect(items).toHaveLength(2);
    expect(items[0]!.type).toBe("session");
    expect(items[1]!.type).toBe("session");
    expect(items[0]!.id).toBe(`session:${groups[0]!.primarySessionId}`);
    expect(items[1]!.id).toBe(`session:${groups[1]!.primarySessionId}`);
  });

  it("assigns consecutive top positions using ITEM_HEIGHT", () => {
    const groups = [
      makeGroup("claude", 1, "a"),
      makeGroup("gpt", 1, "b"),
      makeGroup("gemini", 1, "c"),
    ];
    const items = buildDisplayItems(groups, [], false, new Set());

    for (let i = 0; i < items.length; i++) {
      expect(items[i]!.top).toBe(i * ITEM_HEIGHT);
      expect(items[i]!.height).toBe(ITEM_HEIGHT);
    }
  });

  it("attaches correct group reference", () => {
    const groups = [makeGroup("claude", 2, "a")];
    const items = buildDisplayItems(groups, [], false, new Set());

    expect(items).toHaveLength(1);
    expect(items[0]!.group).toBe(groups[0]);
  });

  it("returns empty array for no groups", () => {
    const items = buildDisplayItems([], [], false, new Set());
    expect(items).toEqual([]);
  });

  it("all ids are unique", () => {
    const groups = [
      makeGroup("claude", 1, "a"),
      makeGroup("claude", 1, "b"),
      makeGroup("gpt", 1, "c"),
    ];
    const items = buildDisplayItems(groups, [], false, new Set());
    const ids = items.map((i) => i.id);
    expect(new Set(ids).size).toBe(ids.length);
  });
});

// ---------------------------------------------------------------------------
// buildDisplayItems — grouped mode
// ---------------------------------------------------------------------------

describe("buildDisplayItems (grouped)", () => {
  function setup(opts?: { collapsed?: string[] }) {
    const groups = [
      makeGroup("claude", 1, "c1"),
      makeGroup("claude", 1, "c2"),
      makeGroup("gpt", 1, "g1"),
    ];
    const sections = buildAgentSections(groups, true);
    const collapsed = new Set(opts?.collapsed ?? []);
    const items = buildDisplayItems(groups, sections, true, collapsed);
    return { groups, sections, items };
  }

  it("interleaves headers and session items", () => {
    const { items, sections } = setup();

    // claude section: 1 header + 2 sessions = 3
    // gpt section: 1 header + 1 session = 2
    expect(items).toHaveLength(5);
    expect(items[0]!.type).toBe("header");
    expect(items[0]!.agent).toBe("claude");
    expect(items[1]!.type).toBe("session");
    expect(items[2]!.type).toBe("session");
    expect(items[3]!.type).toBe("header");
    expect(items[3]!.agent).toBe("gpt");
    expect(items[4]!.type).toBe("session");
  });

  it("headers use HEADER_HEIGHT and sessions use ITEM_HEIGHT", () => {
    const { items } = setup();
    const headers = items.filter((i) => i.type === "header");
    const sessions = items.filter((i) => i.type === "session");

    for (const h of headers) {
      expect(h.height).toBe(HEADER_HEIGHT);
    }
    for (const s of sessions) {
      expect(s.height).toBe(ITEM_HEIGHT);
    }
  });

  it("top positions are calculated cumulatively", () => {
    const { items } = setup();

    let expectedTop = 0;
    for (const item of items) {
      expect(item.top).toBe(expectedTop);
      expectedTop += item.height;
    }
  });

  it("header items have correct count", () => {
    const { items } = setup();
    const claudeHeader = items.find(
      (i) => i.type === "header" && i.agent === "claude",
    );
    const gptHeader = items.find(
      (i) => i.type === "header" && i.agent === "gpt",
    );

    expect(claudeHeader!.count).toBe(2);
    expect(gptHeader!.count).toBe(1);
  });

  it("collapsed agents omit session items", () => {
    const { items } = setup({ collapsed: ["claude"] });

    // claude: header only (collapsed) = 1
    // gpt: header + 1 session = 2
    expect(items).toHaveLength(3);
    expect(items[0]!.type).toBe("header");
    expect(items[0]!.agent).toBe("claude");
    expect(items[1]!.type).toBe("header");
    expect(items[1]!.agent).toBe("gpt");
    expect(items[2]!.type).toBe("session");
  });

  it("collapsing all agents leaves only headers", () => {
    const { items } = setup({ collapsed: ["claude", "gpt"] });

    expect(items).toHaveLength(2);
    expect(items.every((i) => i.type === "header")).toBe(true);
  });

  it("collapsed agents still show correct header count", () => {
    const { items } = setup({ collapsed: ["claude"] });

    const claudeHeader = items.find(
      (i) => i.type === "header" && i.agent === "claude",
    );
    expect(claudeHeader!.count).toBe(2);
  });

  it("all ids are unique across the entire array", () => {
    const { items } = setup();
    const ids = items.map((i) => i.id);
    expect(new Set(ids).size).toBe(ids.length);
  });

  it("header ids differ from session ids with same agent", () => {
    const { items } = setup();
    const claudeHeader = items.find(
      (i) => i.type === "header" && i.agent === "claude",
    );
    const claudeSessions = items.filter(
      (i) => i.type === "session" && i.agent === "claude",
    );

    expect(claudeHeader!.id).toMatch(/^header:/);
    for (const s of claudeSessions) {
      expect(s.id).toMatch(/^session:/);
      expect(s.id).not.toBe(claudeHeader!.id);
    }
  });

  it("session ids in grouped mode include agent prefix for uniqueness", () => {
    // Two different agents could have groups referencing the same
    // primarySessionId in theory.  The id format should include the
    // agent so they stay unique.
    const { items } = setup();
    const sessionItems = items.filter((i) => i.type === "session");
    for (const s of sessionItems) {
      // Format: session:<agent>:<primarySessionId>
      const parts = s.id.split(":");
      expect(parts.length).toBeGreaterThanOrEqual(3);
      expect(parts[0]).toBe("session");
      expect(parts[1]).toBe(s.agent);
    }
  });
});

// ---------------------------------------------------------------------------
// computeTotalSize
// ---------------------------------------------------------------------------

describe("computeTotalSize", () => {
  it("returns 0 for empty array", () => {
    expect(computeTotalSize([])).toBe(0);
  });

  it("returns correct size for flat list", () => {
    const groups = [
      makeGroup("claude", 1, "a"),
      makeGroup("gpt", 1, "b"),
    ];
    const items = buildDisplayItems(groups, [], false, new Set());
    expect(computeTotalSize(items)).toBe(2 * ITEM_HEIGHT);
  });

  it("accounts for mixed header and session heights", () => {
    const groups = [
      makeGroup("claude", 1, "c1"),
      makeGroup("gpt", 1, "g1"),
    ];
    const sections = buildAgentSections(groups, true);
    const items = buildDisplayItems(groups, sections, true, new Set());
    // claude: HEADER_HEIGHT + ITEM_HEIGHT
    // gpt:    HEADER_HEIGHT + ITEM_HEIGHT
    expect(computeTotalSize(items)).toBe(
      2 * HEADER_HEIGHT + 2 * ITEM_HEIGHT,
    );
  });

  it("smaller total when agents are collapsed", () => {
    const groups = [
      makeGroup("claude", 1, "c1"),
      makeGroup("claude", 1, "c2"),
      makeGroup("gpt", 1, "g1"),
    ];
    const sections = buildAgentSections(groups, true);
    const expanded = buildDisplayItems(
      groups,
      sections,
      true,
      new Set(),
    );
    const collapsed = buildDisplayItems(
      groups,
      sections,
      true,
      new Set(["claude"]),
    );

    expect(computeTotalSize(collapsed)).toBeLessThan(
      computeTotalSize(expanded),
    );
    // Difference should be exactly the two collapsed claude sessions.
    expect(
      computeTotalSize(expanded) - computeTotalSize(collapsed),
    ).toBe(2 * ITEM_HEIGHT);
  });
});

// ---------------------------------------------------------------------------
// findStart (binary search)
// ---------------------------------------------------------------------------

describe("findStart", () => {
  function flatItems(count: number): DisplayItem[] {
    const groups: SessionGroup[] = [];
    for (let i = 0; i < count; i++) {
      groups.push(makeGroup("claude", 1, `g${i}`));
    }
    return buildDisplayItems(groups, [], false, new Set());
  }

  it("returns 0 when scrolled to top", () => {
    const items = flatItems(100);
    expect(findStart(items, 0)).toBe(0);
  });

  it("returns 0 for negative scroll (clamped)", () => {
    const items = flatItems(100);
    expect(findStart(items, -100)).toBe(0);
  });

  it("returns index near the visible area", () => {
    const items = flatItems(100);
    // Scroll to row 50 (top = 50 * ITEM_HEIGHT)
    const scrollY = 50 * ITEM_HEIGHT;
    const start = findStart(items, scrollY);
    // Should be at most OVERSCAN rows before row 50.
    expect(start).toBeLessThanOrEqual(50);
    expect(start).toBeGreaterThanOrEqual(50 - 10); // OVERSCAN=10
  });

  it("returns last valid index when scrolled to end", () => {
    const items = flatItems(20);
    const scrollY = 20 * ITEM_HEIGHT;
    const start = findStart(items, scrollY);
    // Start should be within valid bounds.
    expect(start).toBeLessThan(items.length);
    expect(start).toBeGreaterThanOrEqual(0);
  });

  it("handles single-item list", () => {
    const items = flatItems(1);
    expect(findStart(items, 0)).toBe(0);
    expect(findStart(items, 1000)).toBe(0);
  });

  it("handles empty list", () => {
    // Edge case: empty items array.
    expect(findStart([], 0)).toBe(0);
  });

  it("works correctly with mixed-height items (grouped mode)", () => {
    const groups = [
      makeGroup("claude", 1, "c1"),
      makeGroup("claude", 1, "c2"),
      makeGroup("claude", 1, "c3"),
      makeGroup("gpt", 1, "g1"),
      makeGroup("gpt", 1, "g2"),
    ];
    const sections = buildAgentSections(groups, true);
    const items = buildDisplayItems(groups, sections, true, new Set());
    // items layout:
    // [0] header:claude  top=0     h=28
    // [1] session:c1     top=28    h=40
    // [2] session:c2     top=68    h=40
    // [3] session:c3     top=108   h=40
    // [4] header:gpt     top=148   h=28
    // [5] session:g1     top=176   h=40
    // [6] session:g2     top=216   h=40

    // Scroll to where the gpt header would be visible.
    const start = findStart(items, 148);
    // Should return an index before 148 accounting for overscan.
    expect(start).toBeLessThanOrEqual(4);
    expect(start).toBeGreaterThanOrEqual(0);
  });
});

// ---------------------------------------------------------------------------
// STORAGE_KEY constant
// ---------------------------------------------------------------------------

describe("STORAGE_KEY", () => {
  it("has the expected value for localStorage persistence", () => {
    expect(STORAGE_KEY).toBe("agentsview-group-by-agent");
  });
});

// ---------------------------------------------------------------------------
// Unique id stability
// ---------------------------------------------------------------------------

describe("DisplayItem id stability", () => {
  it("produces identical ids for the same input", () => {
    const groups = [
      makeGroup("claude", 1, "c1"),
      makeGroup("gpt", 1, "g1"),
    ];
    const sections = buildAgentSections(groups, true);
    const items1 = buildDisplayItems(groups, sections, true, new Set());
    const items2 = buildDisplayItems(groups, sections, true, new Set());

    expect(items1.map((i) => i.id)).toEqual(items2.map((i) => i.id));
  });

  it("ungrouped ids are deterministic from primarySessionId", () => {
    const groups = [
      makeGroup("claude", 1, "x"),
      makeGroup("gpt", 1, "y"),
    ];
    const items = buildDisplayItems(groups, [], false, new Set());

    expect(items[0]!.id).toBe("session:x-session-0");
    expect(items[1]!.id).toBe("session:y-session-0");
  });

  it("grouped ids are deterministic from agent + primarySessionId", () => {
    const groups = [makeGroup("claude", 1, "c1")];
    const sections = buildAgentSections(groups, true);
    const items = buildDisplayItems(groups, sections, true, new Set());

    const sessionItem = items.find((i) => i.type === "session");
    expect(sessionItem!.id).toBe("session:claude:c1-session-0");
  });
});
