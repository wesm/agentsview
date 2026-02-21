import { describe, it, expect } from "vitest";
import { parseContent, isToolOnly } from "./content-parser.js";
import type { Message } from "../api/types.js";

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
});
