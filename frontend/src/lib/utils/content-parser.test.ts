import { describe, it, expect } from "vitest";
import {
  parseContent,
  isToolOnly,
  enrichSegments,
} from "./content-parser.js";
import type { Message, ToolCall } from "../api/types.js";

function makeMsg(
  overrides: Partial<Message> & { content: string },
): Message {
  const defaults: Message = {
    id: 1,
    session_id: "s1",
    ordinal: 0,
    role: "assistant",
    content: "",
    has_tool_use: false,
    has_thinking: false,
    content_length: 0,
    timestamp: "2024-01-01T00:00:00Z",
  };
  return { ...defaults, ...overrides };
}

describe("parseContent", () => {
  it("returns empty array for empty string", () => {
    expect(parseContent("")).toEqual([]);
  });

  it("preserves leading whitespace in plain text", () => {
    const segments = parseContent("  - Indented list item");
    expect(segments).toEqual([
      { type: "text", content: "  - Indented list item" },
    ]);
  });

  it("removes trailing whitespace in plain text", () => {
    const segments =
      parseContent("Text with trailing space   \n");
    expect(segments).toEqual([
      { type: "text", content: "Text with trailing space" },
    ]);
  });

  it("preserves leading whitespace before blocks", () => {
    const segments =
      parseContent("  Indented text\n[Thinking]\n...");
    expect(segments[0]).toEqual({
      type: "text",
      content: "  Indented text",
    });
    expect(segments[1]).toMatchObject({ type: "thinking" });
  });

  it("handles whitespace in gaps between blocks", () => {
    const text = "[Thinking]\nfoo\n[Bash]\necho hi";
    const segments = parseContent(text);
    expect(segments.map((s) => s.type)).toEqual([
      "thinking",
      "tool",
    ]);
  });

  it("preserves leading whitespace in tail text", () => {
    const segments =
      parseContent("```code\ncontent```\n  Trailing text");
    expect(segments).toHaveLength(2);
    expect(segments[0]).toMatchObject({ type: "code" });
    expect(segments[1]).toEqual({
      type: "text",
      content: "\n  Trailing text",
    });
  });

  it("skips code blocks inside tool blocks", () => {
    const text =
      "[Bash]\n```sh\necho hi\n```\n\nsome text after";
    const segments = parseContent(text);
    const types = segments.map((s) => s.type);
    expect(types).not.toContain("code");
    expect(types).toContain("tool");
  });

  it("extracts code block language as label", () => {
    const segments =
      parseContent("```typescript\nconst x = 1;\n```");
    expect(segments).toEqual([
      {
        type: "code",
        content: "const x = 1;\n",
        label: "typescript",
      },
    ]);
  });

  it("omits label for code blocks without language", () => {
    const segments = parseContent("```\nplain code\n```");
    expect(segments[0]).toEqual({
      type: "code",
      content: "plain code\n",
      label: undefined,
    });
  });

  it("extracts tool name and args as label", () => {
    const segments = parseContent("[Read /foo/bar.ts]\nfile");
    expect(segments[0]).toEqual({
      type: "tool",
      content: "file",
      label: "Read /foo/bar.ts",
    });
  });

  it("parses codex tool markers and normalizes label", () => {
    const segments = parseContent("[exec_command]\n$ rg --files");
    expect(segments[0]).toEqual({
      type: "tool",
      content: "$ rg --files",
      label: "Bash",
    });
  });

  it("drops overlapping matches", () => {
    const text = "[Thinking]\nI think\n[Bash]\necho ok";
    const segments = parseContent(text);
    // Both blocks should parse without overlap
    expect(segments.map((s) => s.type)).toEqual([
      "thinking",
      "tool",
    ]);
  });

  it("returns consistent results on repeated calls", () => {
    const text = "Hello world";
    const first = parseContent(text);
    const second = parseContent(text);
    expect(first).toEqual(second);
  });
});

describe("isToolOnly", () => {
  it("returns false for user messages", () => {
    const msg = makeMsg({ role: "user", content: "[Bash]\nhi" });
    expect(isToolOnly(msg)).toBe(false);
  });

  it("returns false for assistant without tool use", () => {
    const msg = makeMsg({ content: "just text" });
    expect(isToolOnly(msg)).toBe(false);
  });

  it("returns true when content is only tool blocks", () => {
    const msg = makeMsg({
      has_tool_use: true,
      content: "[Bash]\necho hi",
    });
    expect(isToolOnly(msg)).toBe(true);
  });

  it("returns true for multiple tool blocks", () => {
    const msg = makeMsg({
      has_tool_use: true,
      content: "[Read]\nfile.ts\n\n[Edit]\nchanges",
    });
    expect(isToolOnly(msg)).toBe(true);
  });

  it("returns false for plain text assistant messages", () => {
    const msg = makeMsg({ content: "Hello, how can I help?" });
    expect(isToolOnly(msg)).toBe(false);
  });

  it("returns false when text remains after stripping", () => {
    const msg = makeMsg({
      has_tool_use: true,
      content: "Some explanation\n[Bash]\necho hi",
    });
    expect(isToolOnly(msg)).toBe(false);
  });

  it("ignores thinking blocks when checking tool-only", () => {
    const msg = makeMsg({
      has_tool_use: true,
      content: "[Thinking]\nhmm\n[Bash]\necho hi",
    });
    expect(isToolOnly(msg)).toBe(true);
  });

  it("treats codex markers as tool-only content", () => {
    const msg = makeMsg({
      has_tool_use: true,
      content: "[exec_command]\n$ pwd",
    });
    expect(isToolOnly(msg)).toBe(true);
  });
});

describe("parseContent - hasToolUse flag", () => {
  it("skips tool blocks when hasToolUse is false", () => {
    const text = "Some text mentioning [Read: main.go] in prose";
    const segments = parseContent(text, false);
    expect(segments.every(s => s.type !== "tool")).toBe(true);
    expect(segments[0]!.content).toContain("[Read: main.go]");
  });

  it("still parses thinking blocks when hasToolUse is false", () => {
    const text = "[Thinking]\nsome thoughts\n\n[Read: main.go] in text";
    const segments = parseContent(text, false);
    expect(segments[0]!.type).toBe("thinking");
    // The [Read: ...] should be plain text, not a tool block
    const textSegs = segments.filter(s => s.type === "text");
    expect(textSegs.some(s => s.content.includes("[Read: main.go]"))).toBe(true);
  });

  it("still parses code blocks when hasToolUse is false", () => {
    const text = "```js\nconst x = 1\n```\n\n[Bash]\necho hi";
    const segments = parseContent(text, false);
    expect(segments[0]!.type).toBe("code");
    // [Bash] should be plain text
    const textSegs = segments.filter(s => s.type === "text");
    expect(textSegs.some(s => s.content.includes("[Bash]"))).toBe(true);
  });

  it("parses tool blocks normally when hasToolUse is true (default)", () => {
    const segments = parseContent("[Bash]\n$ echo hi");
    expect(segments[0]!.type).toBe("tool");
  });

  it("does not cross-contaminate cache between modes", () => {
    const text = "[Read: file.txt]\nsome output";
    // Parse with tools first — should produce a tool segment
    const withTools = parseContent(text, true);
    expect(withTools[0]!.type).toBe("tool");
    // Parse same text without tools — should be plain text
    const noTools = parseContent(text, false);
    expect(noTools.every((s) => s.type !== "tool")).toBe(true);
    // Reverse order: parse without tools first
    const text2 = "[Edit: other.go]\nreplacement";
    const noTools2 = parseContent(text2, false);
    expect(noTools2.every((s) => s.type !== "tool")).toBe(true);
    const withTools2 = parseContent(text2, true);
    expect(withTools2[0]!.type).toBe("tool");
  });
});

describe("parseContent - Skill tool", () => {
  it("recognizes Skill as a tool block", () => {
    const segments = parseContent(
      "[Skill: superpowers:brainstorming]\nprompt text",
    );
    expect(segments[0]).toEqual({
      type: "tool",
      content: "prompt text",
      label: "Skill : superpowers:brainstorming",
    });
  });

  it("treats Skill-only content as tool-only", () => {
    const msg = makeMsg({
      has_tool_use: true,
      content: "[Skill: commit]\ndo the thing",
    });
    expect(isToolOnly(msg)).toBe(true);
  });
});

describe("parseContent - TaskCreate/TaskUpdate/SendMessage tools", () => {
  it("recognizes TaskCreate as a tool block", () => {
    const segments = parseContent("[TaskCreate: Fix bug]");
    expect(segments[0]!.type).toBe("tool");
    expect(segments[0]!.label).toBe("TaskCreate : Fix bug");
  });

  it("recognizes TaskUpdate as a tool block", () => {
    const segments = parseContent("[TaskUpdate: #5 completed]");
    expect(segments[0]!.type).toBe("tool");
  });

  it("recognizes TaskGet as a tool block", () => {
    const segments = parseContent("[TaskGet: #3]");
    expect(segments[0]!.type).toBe("tool");
    expect(segments[0]!.label).toBe("TaskGet : #3");
  });

  it("recognizes TaskList as a tool block", () => {
    const segments = parseContent("[TaskList]");
    expect(segments[0]!.type).toBe("tool");
    expect(segments[0]!.label).toBe("TaskList");
  });

  it("recognizes SendMessage as a tool block", () => {
    const segments = parseContent("[SendMessage: message to researcher]");
    expect(segments[0]!.type).toBe("tool");
    expect(segments[0]!.label).toBe("SendMessage : message to researcher");
  });
});

describe("enrichSegments", () => {
  it("returns segments unchanged when no tool_calls", () => {
    const segments = parseContent("[Bash]\n$ echo hi");
    const result = enrichSegments(segments);
    expect(result).toBe(segments);
  });

  it("returns segments unchanged for empty tool_calls", () => {
    const segments = parseContent("[Bash]\n$ echo hi");
    const result = enrichSegments(segments, []);
    expect(result).toBe(segments);
  });

  it("attaches toolCall to matching tool segment", () => {
    const segments = parseContent("[Bash]\n$ echo hi");
    const tc: ToolCall = {
      tool_name: "Bash",
      category: "Bash",
      input_json: '{"command":"echo hi","description":""}',
    };
    const result = enrichSegments(segments, [tc]);
    expect(result[0]!.toolCall).toBe(tc);
  });

  it("replaces truncated Bash content with full command", () => {
    // Simulate the \n\n truncation: regex stops at blank line
    const content =
      '[Bash: Create commit]\n$ git commit -m "$(cat <<\'EOF\')\n   Commit message here.';
    const orphaned =
      '\n\n   Co-Authored-By: Claude <noreply@anthropic.com>\n   EOF\n   )"';
    const segments = parseContent(content + orphaned);

    const fullCommand =
      'git commit -m "$(cat <<\'EOF\')\n   Commit message here.\n\n   Co-Authored-By: Claude <noreply@anthropic.com>\n   EOF\n   )"';
    const tc: ToolCall = {
      tool_name: "Bash",
      category: "Bash",
      input_json: JSON.stringify({
        command: fullCommand,
        description: "Create commit",
      }),
    };

    const result = enrichSegments(segments, [tc]);
    // Should have the full command as content
    expect(result[0]!.content).toBe(`$ ${fullCommand}`);
    // Orphaned text should be absorbed
    const textSegments = result.filter((s) => s.type === "text");
    for (const ts of textSegments) {
      // No orphaned fragment from the command should remain
      expect(ts.content).not.toContain("Co-Authored-By");
    }
  });

  it("does not replace single-line Bash content", () => {
    const segments = parseContent("[Bash]\n$ echo hi");
    const tc: ToolCall = {
      tool_name: "Bash",
      category: "Bash",
      input_json: '{"command":"echo hi"}',
    };
    const result = enrichSegments(segments, [tc]);
    expect(result[0]!.content).toBe("$ echo hi");
  });

  it("attaches toolCall to Task segment", () => {
    const segments = parseContent("[Task: run tests (type)]\n");
    const tc: ToolCall = {
      tool_name: "Task",
      category: "Task",
      input_json: JSON.stringify({
        prompt: "Run all unit tests and report failures",
        subagent_type: "test-runner",
        description: "run tests",
      }),
    };
    const result = enrichSegments(segments, [tc]);
    expect(result[0]!.toolCall).toBe(tc);
  });

  it("matches multiple tool calls in order", () => {
    const segments = parseContent(
      "[Read /foo.ts]\ncontents\n[Edit /foo.ts]\nchanges",
    );
    const tc1: ToolCall = {
      tool_name: "Read",
      category: "Read",
      input_json: '{"file_path":"/foo.ts"}',
    };
    const tc2: ToolCall = {
      tool_name: "Edit",
      category: "Edit",
      input_json: '{"file_path":"/foo.ts"}',
    };
    const result = enrichSegments(segments, [tc1, tc2]);
    expect(result[0]!.toolCall).toBe(tc1);
    expect(result[1]!.toolCall).toBe(tc2);
  });

  it("skips non-tool segments when matching", () => {
    const segments = parseContent(
      "Some text\n[Bash]\n$ echo hi",
    );
    const tc: ToolCall = {
      tool_name: "Bash",
      category: "Bash",
      input_json: '{"command":"echo hi"}',
    };
    const result = enrichSegments(segments, [tc]);
    // Text segment stays unchanged
    expect(result[0]!.type).toBe("text");
    expect(result[0]!.toolCall).toBeUndefined();
    // Tool segment gets the toolCall
    expect(result[1]!.toolCall).toBe(tc);
  });

  it("handles more tool_calls than tool segments gracefully", () => {
    const segments = parseContent("[Bash]\n$ echo hi");
    const tc1: ToolCall = {
      tool_name: "Bash",
      category: "Bash",
      input_json: '{"command":"echo hi"}',
    };
    const tc2: ToolCall = {
      tool_name: "Read",
      category: "Read",
    };
    const result = enrichSegments(segments, [tc1, tc2]);
    expect(result[0]!.toolCall).toBe(tc1);
    expect(result).toHaveLength(1);
  });
});
