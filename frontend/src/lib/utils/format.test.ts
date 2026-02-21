import { describe, it, expect } from "vitest";
import { sanitizeSnippet } from "./format.js";

describe("sanitizeSnippet", () => {
  it.each([
    [
      "preserves <mark> tags",
      "hello <mark>world</mark> end",
      "hello <mark>world</mark> end",
    ],
    [
      "escapes other HTML tags",
      '<script>alert("xss")</script>',
      '&lt;script&gt;alert("xss")&lt;/script&gt;',
    ],
    [
      "escapes img tags",
      "<img src=x onerror=alert(1)>",
      "&lt;img src=x onerror=alert(1)&gt;",
    ],
    [
      "handles mixed mark and other tags",
      '<b>bold</b> <mark>highlighted</mark> <i>italic</i>',
      "&lt;b&gt;bold&lt;/b&gt; <mark>highlighted</mark> &lt;i&gt;italic&lt;/i&gt;",
    ],
    [
      "handles case-insensitive mark tags",
      "<MARK>upper</MARK> <Mark>mixed</Mark>",
      "<mark>upper</mark> <mark>mixed</mark>",
    ],
    [
      "handles multiple mark spans",
      "<mark>first</mark> gap <mark>second</mark>",
      "<mark>first</mark> gap <mark>second</mark>",
    ],
    [
      "returns empty string for empty input",
      "",
      "",
    ],
    [
      "handles plain text without tags",
      "no tags here",
      "no tags here",
    ],
    [
      "escapes angle brackets in content",
      "x < y > z",
      "x &lt; y &gt; z",
    ],
    [
      "handles nested mark tags gracefully",
      "<mark>outer <mark>inner</mark></mark>",
      "<mark>outer <mark>inner</mark></mark>",
    ],
    [
      "escapes event handler attributes in mark-like tags",
      "<mark onload=alert(1)>text</mark>",
      "&lt;mark onload=alert(1)&gt;text</mark>",
    ],
  ])("%s", (_name, input, expected) => {
    expect(sanitizeSnippet(input)).toBe(expected);
  });
});
