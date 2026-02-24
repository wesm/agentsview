import type { Message, ToolCall } from "../api/types.js";
import { LRUCache } from "./cache.js";

export type SegmentType = "text" | "thinking" | "tool" | "code";

export interface ContentSegment {
  type: SegmentType;
  content: string;
  /** Tool name or code language, when applicable */
  label?: string;
  /** Structured tool call data from the API, when available */
  toolCall?: ToolCall;
}

/**
 * Regex patterns matching Go backend at
 * internal/server/export.go:403-412
 */
const THINKING_RE =
  /\[Thinking\]\n?([\s\S]*?)(?=\n\[|\n\n\[|$)/g;

const TOOL_NAMES =
  "Tool|Read|Write|Edit|Bash|Glob|Grep|TaskCreate|TaskUpdate|TaskGet|TaskList|Task|Skill|" +
  "SendMessage|Question|Todo List|Entering Plan Mode|" +
  "Exiting Plan Mode|exec_command|shell_command|" +
  "write_stdin|apply_patch|shell|parallel|view_image|" +
  "request_user_input|update_plan";

const TOOL_ALIASES: Record<string, string> = {
  exec_command: "Bash",
  shell_command: "Bash",
  write_stdin: "Bash",
  shell: "Bash",
  apply_patch: "Edit",
};

const TOOL_RE = new RegExp(
  `\\[(${TOOL_NAMES})([^\\]]*)\\]([\\s\\S]*?)(?=\\n\\[|\\n\\n|$)`,
  "g",
);

const CODE_BLOCK_RE = /```(\w*)\n([\s\S]*?)```/g;

interface Match {
  start: number;
  end: number;
  segment: ContentSegment;
}

const toolOnlyCache = new LRUCache<string, boolean>(12000);
const segmentCache = new LRUCache<string, ContentSegment[]>(8000);

/** Returns true if the message contains only tool calls (no text) */
export function isToolOnly(msg: Message): boolean {
  const key =
    `${msg.role}|${msg.has_tool_use ? 1 : 0}|${msg.content}`;
  const cached = toolOnlyCache.get(key);
  if (cached !== undefined) return cached;

  if (msg.role !== "assistant") return false;
  if (!msg.has_tool_use) {
    toolOnlyCache.set(key, false);
    return false;
  }
  const stripped = msg.content
    .replace(THINKING_RE, "")
    .replace(TOOL_RE, "")
    .trim();
  const result = stripped.length === 0;
  toolOnlyCache.set(key, result);
  return result;
}

function extractMatches(text: string, parseTools = true): Match[] {
  const matches: Match[] = [];

  for (const m of text.matchAll(THINKING_RE)) {
    matches.push({
      start: m.index!,
      end: m.index! + m[0].length,
      segment: {
        type: "thinking",
        content: (m[1] ?? "").trim(),
      },
    });
  }

  if (parseTools) {
    for (const m of text.matchAll(TOOL_RE)) {
      const toolName = m[1] ?? "";
      const toolArgs = (m[2] ?? "").trim();
      const displayName = TOOL_ALIASES[toolName] ?? toolName;
      const label = toolArgs
        ? `${displayName} ${toolArgs}`
        : displayName;
      matches.push({
        start: m.index!,
        end: m.index! + m[0].length,
        segment: {
          type: "tool",
          content: (m[3] ?? "").trim(),
          label,
        },
      });
    }
  }

  for (const m of text.matchAll(CODE_BLOCK_RE)) {
    const idx = m.index!;
    const insideOther = matches.some(
      (o) => idx >= o.start && idx < o.end,
    );
    if (insideOther) continue;

    matches.push({
      start: idx,
      end: idx + m[0].length,
      segment: {
        type: "code",
        content: m[2] ?? "",
        label: m[1] || undefined,
      },
    });
  }

  return matches;
}

function resolveOverlaps(matches: Match[]): Match[] {
  matches.sort((a, b) => a.start - b.start);
  const deduped: Match[] = [];
  let lastEnd = 0;
  for (const m of matches) {
    if (m.start < lastEnd) continue;
    deduped.push(m);
    lastEnd = m.end;
  }
  return deduped;
}

function buildSegments(
  text: string,
  matches: Match[],
): ContentSegment[] {
  const segments: ContentSegment[] = [];
  let pos = 0;

  for (const m of matches) {
    if (m.start > pos) {
      const gap = text.slice(pos, m.start).trimEnd();
      if (gap) {
        segments.push({ type: "text", content: gap });
      }
    }
    segments.push(m.segment);
    pos = m.end;
  }

  if (pos < text.length) {
    const tail = text.slice(pos).trimEnd();
    if (tail) {
      segments.push({ type: "text", content: tail });
    }
  }

  return segments;
}

/** Parse message content into typed segments */
export function parseContent(text: string, hasToolUse = true): ContentSegment[] {
  if (!text) return [];
  const cacheKey = hasToolUse
    ? `tools\0${text}`
    : `notools\0${text}`;
  const cached = segmentCache.get(cacheKey);
  if (cached) return cached;

  const matches = extractMatches(text, hasToolUse);

  if (matches.length === 0) {
    const onlyText: ContentSegment[] = [
      { type: "text", content: text.trimEnd() },
    ];
    segmentCache.set(cacheKey, onlyText);
    return onlyText;
  }

  const deduped = resolveOverlaps(matches);
  const segments = buildSegments(text, deduped);

  segmentCache.set(cacheKey, segments);
  return segments;
}

/** Attach structured tool_calls data to parsed segments.
 *  For Bash tools with multi-line commands, replaces truncated
 *  regex content with the full command from input_json and
 *  absorbs orphaned text fragments. */
export function enrichSegments(
  segments: ContentSegment[],
  toolCalls?: ToolCall[],
): ContentSegment[] {
  if (!toolCalls?.length) return segments;

  const result: ContentSegment[] = [];
  let tcIdx = 0;

  for (let i = 0; i < segments.length; i++) {
    const seg = segments[i]!;

    if (seg.type === "tool" && tcIdx < toolCalls.length) {
      const tc = toolCalls[tcIdx]!;
      tcIdx++;
      const enriched: ContentSegment = { ...seg, toolCall: tc };

      if (tc.tool_name === "Bash" && tc.input_json) {
        try {
          const input = JSON.parse(tc.input_json);
          const fullCmd = input.command;
          if (fullCmd && fullCmd.includes("\n")) {
            enriched.content = `$ ${fullCmd}`;
            // Absorb orphaned text segments from truncated command
            while (i + 1 < segments.length) {
              const next = segments[i + 1]!;
              if (next.type !== "text") break;
              if (!next.content.trim() ||
                  fullCmd.includes(next.content.trim())) {
                i++;
              } else {
                break;
              }
            }
          }
        } catch {
          /* fallback to regex content */
        }
      }

      result.push(enriched);
    } else {
      result.push(seg);
    }
  }

  return result;
}
