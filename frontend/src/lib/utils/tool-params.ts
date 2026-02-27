/** Extracts display metadata and fallback content from tool call input_json. */

export interface MetaTag {
  label: string;
  value: string;
}

export function truncate(s: string, max: number): string {
  return s.length > max ? s.slice(0, max) + "\u2026" : s;
}

type Params = Record<string, unknown>;

/** Extract metadata tags for common tool types.
 *  Returns null for Task/TaskCreate/TaskUpdate (handled separately). */
export function extractToolParamMeta(
  toolName: string,
  params: Params,
): MetaTag[] | null {
  const skip = ["Task", "TaskCreate", "TaskUpdate"];
  if (skip.includes(toolName)) return null;
  const meta: MetaTag[] = [];
  if (toolName === "Read") {
    if (params.file_path)
      meta.push({
        label: "file",
        value: truncate(String(params.file_path), 80),
      });
    if (params.offset)
      meta.push({
        label: "offset",
        value: String(params.offset),
      });
    if (params.limit)
      meta.push({
        label: "limit",
        value: String(params.limit),
      });
    if (params.pages)
      meta.push({
        label: "pages",
        value: String(params.pages),
      });
  } else if (toolName === "Edit") {
    if (params.file_path)
      meta.push({
        label: "file",
        value: truncate(String(params.file_path), 80),
      });
    if (params.replace_all)
      meta.push({ label: "mode", value: "replace_all" });
  } else if (toolName === "Write") {
    if (params.file_path)
      meta.push({
        label: "file",
        value: truncate(String(params.file_path), 80),
      });
  } else if (toolName === "Grep") {
    if (params.pattern)
      meta.push({
        label: "pattern",
        value: truncate(String(params.pattern), 60),
      });
    if (params.path)
      meta.push({
        label: "path",
        value: truncate(String(params.path), 80),
      });
    if (params.glob)
      meta.push({ label: "glob", value: String(params.glob) });
    if (params.output_mode)
      meta.push({
        label: "mode",
        value: String(params.output_mode),
      });
  } else if (toolName === "Glob") {
    if (params.pattern)
      meta.push({
        label: "pattern",
        value: String(params.pattern),
      });
    if (params.path)
      meta.push({
        label: "path",
        value: truncate(String(params.path), 80),
      });
  } else if (toolName === "Bash") {
    if (params.description)
      meta.push({
        label: "description",
        value: truncate(String(params.description), 80),
      });
  } else if (toolName === "Skill") {
    if (params.skill)
      meta.push({
        label: "skill",
        value: String(params.skill),
      });
  }
  return meta.length ? meta : null;
}

/** Generate displayable content from input params when
 *  the regex-captured content is empty. */
export function generateFallbackContent(
  toolName: string,
  params: Params,
): string | null {
  if (toolName === "Task") return null;
  if (toolName === "Edit") {
    const lines: string[] = [];
    if (params.old_string != null) {
      lines.push("--- old");
      lines.push(truncate(String(params.old_string), 500));
    }
    if (params.new_string != null) {
      lines.push("+++ new");
      lines.push(truncate(String(params.new_string), 500));
    }
    return lines.length ? lines.join("\n") : null;
  }
  if (toolName === "Write" && params.content) {
    return truncate(String(params.content), 500);
  }
  const lines: string[] = [];
  for (const [key, value] of Object.entries(params)) {
    if (value == null || value === "") continue;
    const strVal =
      typeof value === "string"
        ? value
        : JSON.stringify(value);
    lines.push(`${key}: ${truncate(strVal, 200)}`);
  }
  return lines.length ? lines.join("\n") : null;
}
