import { describe, expect, it } from "vitest";
import {
  truncate,
  extractToolParamMeta,
  generateFallbackContent,
} from "./tool-params.js";

describe("truncate", () => {
  it("returns short strings unchanged", () => {
    expect(truncate("hello", 10)).toBe("hello");
  });

  it("truncates at max and appends ellipsis", () => {
    expect(truncate("abcdef", 3)).toBe("abc\u2026");
  });

  it("returns exact-length strings unchanged", () => {
    expect(truncate("abc", 3)).toBe("abc");
  });
});

describe("extractToolParamMeta", () => {
  it("returns null for Task tool", () => {
    expect(
      extractToolParamMeta("Task", { prompt: "do stuff" }),
    ).toBeNull();
  });

  it("returns null for TaskCreate tool", () => {
    expect(
      extractToolParamMeta("TaskCreate", { subject: "x" }),
    ).toBeNull();
  });

  it("returns null for TaskUpdate tool", () => {
    expect(
      extractToolParamMeta("TaskUpdate", { taskId: "1" }),
    ).toBeNull();
  });

  it("extracts Read params", () => {
    const meta = extractToolParamMeta("Read", {
      file_path: "/src/app.ts",
      offset: 10,
      limit: 50,
    });
    expect(meta).toEqual([
      { label: "file", value: "/src/app.ts" },
      { label: "offset", value: "10" },
      { label: "limit", value: "50" },
    ]);
  });

  it("extracts Read pages param", () => {
    const meta = extractToolParamMeta("Read", {
      file_path: "/doc.pdf",
      pages: "1-5",
    });
    expect(meta).toEqual([
      { label: "file", value: "/doc.pdf" },
      { label: "pages", value: "1-5" },
    ]);
  });

  it("extracts Edit params with replace_all", () => {
    const meta = extractToolParamMeta("Edit", {
      file_path: "/src/app.ts",
      replace_all: true,
    });
    expect(meta).toEqual([
      { label: "file", value: "/src/app.ts" },
      { label: "mode", value: "replace_all" },
    ]);
  });

  it("extracts Write file_path", () => {
    const meta = extractToolParamMeta("Write", {
      file_path: "/src/new.ts",
      content: "export const x = 1;",
    });
    expect(meta).toEqual([
      { label: "file", value: "/src/new.ts" },
    ]);
  });

  it("extracts Grep params", () => {
    const meta = extractToolParamMeta("Grep", {
      pattern: "TODO",
      path: "/src",
      glob: "*.ts",
      output_mode: "content",
    });
    expect(meta).toEqual([
      { label: "pattern", value: "TODO" },
      { label: "path", value: "/src" },
      { label: "glob", value: "*.ts" },
      { label: "mode", value: "content" },
    ]);
  });

  it("extracts Glob params", () => {
    const meta = extractToolParamMeta("Glob", {
      pattern: "**/*.ts",
      path: "/src",
    });
    expect(meta).toEqual([
      { label: "pattern", value: "**/*.ts" },
      { label: "path", value: "/src" },
    ]);
  });

  it("extracts Bash description", () => {
    const meta = extractToolParamMeta("Bash", {
      command: "npm test",
      description: "Run test suite",
    });
    expect(meta).toEqual([
      { label: "description", value: "Run test suite" },
    ]);
  });

  it("returns null for Bash without description", () => {
    expect(
      extractToolParamMeta("Bash", { command: "ls" }),
    ).toBeNull();
  });

  it("extracts Skill name", () => {
    const meta = extractToolParamMeta("Skill", {
      skill: "commit",
    });
    expect(meta).toEqual([
      { label: "skill", value: "commit" },
    ]);
  });

  it("returns null for unknown tool with no matching params", () => {
    expect(
      extractToolParamMeta("CustomTool", { foo: "bar" }),
    ).toBeNull();
  });

  it("preserves zero-valued offset and limit", () => {
    const meta = extractToolParamMeta("Read", {
      file_path: "/src/app.ts",
      offset: 0,
      limit: 0,
    });
    expect(meta).toEqual([
      { label: "file", value: "/src/app.ts" },
      { label: "offset", value: "0" },
      { label: "limit", value: "0" },
    ]);
  });

  it("truncates long file paths", () => {
    const longPath = "/a".repeat(50);
    const meta = extractToolParamMeta("Read", {
      file_path: longPath,
    });
    expect(meta![0]!.value.length).toBeLessThanOrEqual(81);
    expect(meta![0]!.value).toContain("\u2026");
  });
});

describe("generateFallbackContent", () => {
  it("returns null for Task tool", () => {
    expect(
      generateFallbackContent("Task", { prompt: "do stuff" }),
    ).toBeNull();
  });

  it("shows diff for Edit tool", () => {
    const result = generateFallbackContent("Edit", {
      file_path: "/src/app.ts",
      old_string: "const x = 1;",
      new_string: "const x = 2;",
    });
    expect(result).toBe(
      "--- old\nconst x = 1;\n+++ new\nconst x = 2;",
    );
  });

  it("shows only new_string when old_string is empty", () => {
    const result = generateFallbackContent("Edit", {
      file_path: "/src/app.ts",
      old_string: "",
      new_string: "const x = 1;",
    });
    expect(result).toBe(
      "--- old\n\n+++ new\nconst x = 1;",
    );
  });

  it("truncates long Edit strings", () => {
    const long = "x".repeat(600);
    const result = generateFallbackContent("Edit", {
      old_string: long,
      new_string: "short",
    })!;
    const oldLine = result.split("\n")[1]!;
    expect(oldLine.length).toBeLessThanOrEqual(501);
    expect(oldLine).toContain("\u2026");
  });

  it("shows Write content preview", () => {
    const result = generateFallbackContent("Write", {
      file_path: "/src/new.ts",
      content: 'export const x = "hello";',
    });
    expect(result).toBe('export const x = "hello";');
  });

  it("truncates long Write content", () => {
    const long = "line\n".repeat(200);
    const result = generateFallbackContent("Write", {
      file_path: "/src/big.ts",
      content: long,
    })!;
    expect(result.length).toBeLessThanOrEqual(501);
  });

  it("shows empty-file marker for Write with empty content", () => {
    expect(
      generateFallbackContent("Write", {
        file_path: "/src/empty.ts",
        content: "",
      }),
    ).toBe("(empty file)");
  });

  it("falls back to generic display for Write without content", () => {
    expect(
      generateFallbackContent("Write", {
        file_path: "/src/new.ts",
      }),
    ).toBe("file_path: /src/new.ts");
  });

  it("shows generic key-value for Read", () => {
    const result = generateFallbackContent("Read", {
      file_path: "/src/app.ts",
      limit: 100,
    });
    expect(result).toBe(
      "file_path: /src/app.ts\nlimit: 100",
    );
  });

  it("shows generic key-value for unknown tools", () => {
    const result = generateFallbackContent("CustomTool", {
      foo: "bar",
      count: 42,
    });
    expect(result).toBe("foo: bar\ncount: 42");
  });

  it("skips null and empty values in generic mode", () => {
    const result = generateFallbackContent("CustomTool", {
      present: "yes",
      missing: null,
      empty: "",
    });
    expect(result).toBe("present: yes");
  });

  it("stringifies non-string values in generic mode", () => {
    const result = generateFallbackContent("CustomTool", {
      arr: [1, 2, 3],
      obj: { nested: true },
    });
    expect(result).toBe(
      "arr: [1,2,3]\nobj: {\"nested\":true}",
    );
  });

  it("returns null when params are all empty", () => {
    expect(
      generateFallbackContent("CustomTool", {}),
    ).toBeNull();
  });
});
