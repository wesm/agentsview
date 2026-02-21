import { describe, it, expect, beforeEach } from "vitest";
import { sanitizeSnippet, _resetNonceCounter } from "./format.js";

describe("sanitizeSnippet", () => {
  beforeEach(() => {
    _resetNonceCounter(0);
  });

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
    [
      "keeps pre-escaped mark entities as text",
      "&lt;mark&gt;not real&lt;/mark&gt;",
      "&lt;mark&gt;not real&lt;/mark&gt;",
    ],
    [
      "keeps pre-escaped entities alongside real mark tags",
      "<mark>real</mark> &lt;mark&gt;fake&lt;/mark&gt;",
      "<mark>real</mark> &lt;mark&gt;fake&lt;/mark&gt;",
    ],
    [
      "does not promote text matching old placeholder tokens",
      "text \x00MARK_O\x00 and \x00MARK_C\x00 here",
      "text \x00MARK_O\x00 and \x00MARK_C\x00 here",
    ],
    [
      "skips nonce when input contains the candidate placeholder",
      "text \x000\x00O\x000\x00 and \x000\x00C\x000\x00 here",
      "text \x000\x00O\x000\x00 and \x000\x00C\x000\x00 here",
    ],
  ])("%s", (_name, input, expected) => {
    expect(sanitizeSnippet(input)).toBe(expected);
  });
});
