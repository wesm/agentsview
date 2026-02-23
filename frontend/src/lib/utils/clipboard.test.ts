import { describe, it, expect } from "vitest";
import { getSessionCommand } from "./clipboard.js";
import type { Session } from "../api/types.js";

function makeSession(
  overrides: Partial<Session> & { id: string; agent: string },
): Session {
  return {
    project: "test-project",
    machine: "localhost",
    first_message: null,
    started_at: null,
    ended_at: null,
    message_count: 0,
    created_at: "2025-01-01T00:00:00Z",
    ...overrides,
  };
}

describe("getSessionCommand", () => {
  const cases: { agent: string; expected: string }[] = [
    {
      agent: "claude",
      expected: "claude --continue test-session-id",
    },
    {
      agent: "codex",
      expected: "codex --continue test-session-id",
    },
    {
      agent: "gemini",
      expected: "test-session-id",
    },
    {
      agent: "unknown-agent",
      expected: "test-session-id",
    },
  ];

  for (const { agent, expected } of cases) {
    it(`returns correct command for ${agent} sessions`, () => {
      const session = makeSession({
        id: "test-session-id",
        agent,
      });
      expect(getSessionCommand(session)).toBe(expected);
    });
  }

  it("returns null for ids with unsafe characters", () => {
    const session = makeSession({
      id: "id; rm -rf /",
      agent: "claude",
    });
    expect(getSessionCommand(session)).toBeNull();
  });

  it("returns null for ids containing single quotes", () => {
    const session = makeSession({
      id: "id'bad",
      agent: "claude",
    });
    expect(getSessionCommand(session)).toBeNull();
  });

  it("returns null for ids with shell metacharacters", () => {
    const session = makeSession({
      id: "id & calc &",
      agent: "claude",
    });
    expect(getSessionCommand(session)).toBeNull();
  });
});
