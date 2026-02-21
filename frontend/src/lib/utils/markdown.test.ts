// @vitest-environment jsdom
import { describe, it, expect } from "vitest";
import { renderMarkdown } from "./markdown.js";

/**
 * Parse HTML string into a DOM container for semantic assertions.
 * Avoids brittle exact-string comparisons that break on harmless
 * formatting changes in the renderer or sanitizer.
 */
function parseHTML(html: string): HTMLElement {
  const div = document.createElement("div");
  div.innerHTML = html;
  return div;
}

describe("renderMarkdown", () => {
  describe("inline formatting", () => {
    it("renders bold text", () => {
      const dom = parseHTML(renderMarkdown("**bold**"));
      const strong = dom.querySelector("p > strong");
      expect(strong).not.toBeNull();
      expect(strong!.textContent).toBe("bold");
    });

    it("renders italic text", () => {
      const dom = parseHTML(renderMarkdown("*italic*"));
      const em = dom.querySelector("p > em");
      expect(em).not.toBeNull();
      expect(em!.textContent).toBe("italic");
    });

    it("renders inline code", () => {
      const dom = parseHTML(renderMarkdown("`code`"));
      const code = dom.querySelector("p > code");
      expect(code).not.toBeNull();
      expect(code!.textContent).toBe("code");
    });

    it("renders links", () => {
      const dom = parseHTML(
        renderMarkdown("[text](https://example.com)"),
      );
      const a = dom.querySelector("p > a");
      expect(a).not.toBeNull();
      expect(a!.textContent).toBe("text");
      expect(a!.getAttribute("href")).toBe("https://example.com");
    });
  });

  describe("block elements", () => {
    it("renders headings", () => {
      const dom = parseHTML(renderMarkdown("## Heading 2"));
      const h2 = dom.querySelector("h2");
      expect(h2).not.toBeNull();
      expect(h2!.textContent).toBe("Heading 2");
    });

    it("renders unordered lists", () => {
      const dom = parseHTML(
        renderMarkdown("- item one\n- item two"),
      );
      const items = dom.querySelectorAll("ul > li");
      expect(items).toHaveLength(2);
      expect(items[0]!.textContent).toBe("item one");
      expect(items[1]!.textContent).toBe("item two");
    });

    it("renders ordered lists", () => {
      const dom = parseHTML(
        renderMarkdown("1. first\n2. second"),
      );
      const items = dom.querySelectorAll("ol > li");
      expect(items).toHaveLength(2);
      expect(items[0]!.textContent).toBe("first");
      expect(items[1]!.textContent).toBe("second");
    });

    it("renders blockquotes", () => {
      const dom = parseHTML(renderMarkdown("> quoted text"));
      const bq = dom.querySelector("blockquote");
      expect(bq).not.toBeNull();
      expect(bq!.textContent!.trim()).toBe("quoted text");
    });

    it("renders tables", () => {
      const md = "| A | B |\n| --- | --- |\n| 1 | 2 |";
      const dom = parseHTML(renderMarkdown(md));
      const ths = dom.querySelectorAll("thead th");
      expect(ths).toHaveLength(2);
      expect(ths[0]!.textContent).toBe("A");
      expect(ths[1]!.textContent).toBe("B");
      const tds = dom.querySelectorAll("tbody td");
      expect(tds).toHaveLength(2);
      expect(tds[0]!.textContent).toBe("1");
      expect(tds[1]!.textContent).toBe("2");
    });

    it("renders horizontal rules", () => {
      const dom = parseHTML(renderMarkdown("---"));
      expect(dom.querySelector("hr")).not.toBeNull();
    });

    it("converts single newlines to <br>", () => {
      const dom = parseHTML(
        renderMarkdown("line one\nline two"),
      );
      const p = dom.querySelector("p");
      expect(p).not.toBeNull();
      expect(p!.querySelector("br")).not.toBeNull();
      expect(p!.textContent).toBe("line oneline two");
    });
  });

  describe("security and sanitization", () => {
    it("strips script tags (XSS)", () => {
      expect(renderMarkdown('<script>alert("xss")</script>')).toBe(
        "",
      );
    });

    it("strips event handlers (XSS)", () => {
      const dom = parseHTML(
        renderMarkdown('<img src=x onerror="alert(1)">'),
      );
      const img = dom.querySelector("img");
      expect(img).not.toBeNull();
      expect(img!.hasAttribute("onerror")).toBe(false);
    });

    it("strips javascript: URLs (XSS)", () => {
      const dom = parseHTML(
        renderMarkdown("[click](javascript:alert(1))"),
      );
      const a = dom.querySelector("a");
      expect(a).not.toBeNull();
      expect(a!.textContent).toBe("click");
      expect(a!.hasAttribute("href")).toBe(false);
    });

    const xssPayloads: Array<{
      name: string;
      input: string;
      assert: (html: string) => void;
    }> = [
      {
        name: "mixed-case javascript: URL",
        input: "[click](jAvAsCrIpT:alert(1))",
        assert(html) {
          const dom = parseHTML(html);
          const a = dom.querySelector("a");
          expect(a).not.toBeNull();
          expect(a!.hasAttribute("href")).toBe(false);
        },
      },
      {
        name: "tab-padded javascript: URL",
        input: "[click](java\tscript:alert(1))",
        assert(html) {
          const dom = parseHTML(html);
          const a = dom.querySelector("a");
          if (a) {
            const href = a.getAttribute("href") ?? "";
            expect(href).not.toMatch(/javascript/i);
          }
        },
      },
      {
        name: "newline-padded javascript: URL",
        input: "[click](java\nscript:alert(1))",
        assert(html) {
          const dom = parseHTML(html);
          const a = dom.querySelector("a");
          if (a) {
            const href = a.getAttribute("href") ?? "";
            expect(href).not.toMatch(/javascript/i);
          }
        },
      },
      {
        name: "URL-encoded javascript: scheme",
        input: "[click](&#106;avascript:alert(1))",
        assert(html) {
          const dom = parseHTML(html);
          const a = dom.querySelector("a");
          if (a) {
            const href = a.getAttribute("href") ?? "";
            expect(href).not.toMatch(/javascript/i);
          }
        },
      },
      {
        name: "data: text/html payload",
        input:
          '[click](data:text/html,<script>alert(1)</script>)',
        assert(html) {
          const dom = parseHTML(html);
          const a = dom.querySelector("a");
          if (a) {
            const href = a.getAttribute("href") ?? "";
            expect(href).not.toMatch(/^data:/i);
          }
        },
      },
      {
        name: "data: base64 payload",
        input:
          "[click](data:text/html;base64,PHNjcmlwdD5hbGVydCgxKTwvc2NyaXB0Pg==)",
        assert(html) {
          const dom = parseHTML(html);
          const a = dom.querySelector("a");
          if (a) {
            const href = a.getAttribute("href") ?? "";
            expect(href).not.toMatch(/^data:/i);
          }
        },
      },
      {
        name: "vbscript: URL",
        input: "[click](vbscript:MsgBox(1))",
        assert(html) {
          const dom = parseHTML(html);
          const a = dom.querySelector("a");
          if (a) {
            const href = a.getAttribute("href") ?? "";
            expect(href).not.toMatch(/vbscript/i);
          }
        },
      },
      {
        name: "onload event handler on body tag",
        input: '<body onload="alert(1)">',
        assert(html) {
          const dom = parseHTML(html);
          for (const el of dom.querySelectorAll("*")) {
            expect(el.hasAttribute("onload")).toBe(false);
          }
        },
      },
      {
        name: "onfocus event handler with autofocus",
        input: '<input onfocus="alert(1)" autofocus>',
        assert(html) {
          const dom = parseHTML(html);
          for (const el of dom.querySelectorAll("*")) {
            expect(el.hasAttribute("onfocus")).toBe(false);
          }
        },
      },
      {
        name: "SVG with onload",
        input: '<svg onload="alert(1)">',
        assert(html) {
          const dom = parseHTML(html);
          for (const el of dom.querySelectorAll("*")) {
            expect(el.hasAttribute("onload")).toBe(false);
          }
        },
      },
    ];

    it.each(xssPayloads)(
      "sanitizes $name",
      ({ input, assert: assertFn }) => {
        assertFn(renderMarkdown(input));
      },
    );
  });

  describe("edge cases", () => {
    it("returns empty string for empty input", () => {
      expect(renderMarkdown("")).toBe("");
    });

    it("passes through plain text", () => {
      const dom = parseHTML(renderMarkdown("just plain text"));
      const p = dom.querySelector("p");
      expect(p).not.toBeNull();
      expect(p!.textContent).toBe("just plain text");
    });

    it("removes trailing newlines to prevent extra height", () => {
      const dom = parseHTML(renderMarkdown("text\n\n"));
      const p = dom.querySelector("p");
      expect(p).not.toBeNull();
      expect(p!.textContent).toBe("text");
    });
  });
});
