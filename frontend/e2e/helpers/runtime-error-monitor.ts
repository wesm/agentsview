import type { Page } from "@playwright/test";

/**
 * Captures runtime errors (uncaught exceptions and console.error)
 * from a Playwright page. Attach early in the test, then query
 * categorized results after interactions complete.
 */
export class RuntimeErrorMonitor {
  private readonly errors: string[] = [];

  constructor(page: Page) {
    page.on("pageerror", (err) => {
      this.errors.push(err.message);
    });
    page.on("console", (msg) => {
      if (msg.type() === "error") {
        this.errors.push(msg.text());
      }
    });
  }

  /** All captured error messages. */
  all(): readonly string[] {
    return this.errors;
  }

  /** Errors matching the given pattern. */
  matching(pattern: RegExp): string[] {
    return this.errors.filter((m) => {
      pattern.lastIndex = 0;
      return pattern.test(m);
    });
  }

  /** Errors not matching the given pattern. */
  excluding(pattern: RegExp): string[] {
    return this.errors.filter((m) => {
      pattern.lastIndex = 0;
      return !pattern.test(m);
    });
  }
}
