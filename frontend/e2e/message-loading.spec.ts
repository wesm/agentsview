import { test, expect } from "@playwright/test";
import { SessionsPage } from "./pages/sessions-page";

test.describe("Message loading", () => {
  test("clicking session shows messages", async ({ page }) => {
    const sp = new SessionsPage(page);
    await sp.goto();
    await sp.selectFirstSession();
  });

  test("no request spam on session click", async ({ page }) => {
    const messageRequests: string[] = [];
    page.on("request", (req) => {
      if (req.url().includes("/messages")) {
        messageRequests.push(req.url());
      }
    });

    const sp = new SessionsPage(page);
    await sp.goto();
    await sp.selectFirstSession();

    // Wait for at least one message request to have fired
    await expect
      .poll(() => messageRequests.length, { timeout: 5_000 })
      .toBeGreaterThan(0);
    const settled = await stableValue(
      () => messageRequests.length,
      500,
    );
    expect(settled).toBe(true);

    // For large sessions we may fetch several pages while loading
    // into memory. With the reactive loop bug, this would be
    // dozens of parallel requests.
    expect(messageRequests.length).toBeLessThanOrEqual(15);
  });

  test("small session loads fast", async ({ page }) => {
    const sp = new SessionsPage(page);
    await sp.goto();
    await sp.selectLastSession();
  });

  test(
    "large session shows first page quickly",
    async ({ page }) => {
      const sp = new SessionsPage(page);
      await sp.goto();

      // First session is the largest (5500 messages)
      await sp.sessionItems.first().click();

      // First page should render within 3s
      await expect(sp.messageRows.first()).toBeVisible({
        timeout: 3_000,
      });
    },
  );

  test(
    "scroll does not reset to top during loading",
    async ({ page }) => {
      const sp = new SessionsPage(page);
      await sp.goto();
      await sp.selectFirstSession();

      // Wait for progressive loading to finish by polling
      // the message row count until it stabilizes.
      await waitForRowCountStable(page);

      // Scroll down
      await sp.scroller.evaluate((el) => {
        el.scrollTop = 3000;
      });

      // Wait for scroll position to settle
      await expect
        .poll(
          () => sp.scroller.evaluate((el) => el.scrollTop),
          { timeout: 2_000 },
        )
        .toBeGreaterThan(500);
    },
  );

});

/** Polls a value-producing function until it stays constant. */
async function stableValue(
  fn: () => number,
  durationMs: number,
  pollMs: number = 100,
): Promise<boolean> {
  const deadline = Date.now() + durationMs * 3;
  let last = fn();
  let stableStart = Date.now();

  while (Date.now() < deadline) {
    await new Promise((r) => setTimeout(r, pollMs));
    const current = fn();
    if (current !== last) {
      last = current;
      stableStart = Date.now();
    }
    if (Date.now() - stableStart >= durationMs) {
      return true;
    }
  }
  return false;
}

/** Waits for the virtual row count to stabilize (progressive loading done). */
async function waitForRowCountStable(
  page: import("@playwright/test").Page,
  durationMs: number = 800,
) {
  await expect
    .poll(
      async () => {
        const count = await page
          .locator(".virtual-row")
          .count();
        return count;
      },
      { timeout: 5_000 },
    )
    .toBeGreaterThan(0);

  // Wait for count to stop changing
  let lastCount = await page.locator(".virtual-row").count();
  let stableStart = Date.now();
  const deadline = Date.now() + durationMs * 3;

  while (Date.now() < deadline) {
    await new Promise((r) => setTimeout(r, 200));
    const current = await page.locator(".virtual-row").count();
    if (current !== lastCount) {
      lastCount = current;
      stableStart = Date.now();
    }
    if (Date.now() - stableStart >= durationMs) {
      return;
    }
  }
  throw new Error(
    `Row count did not stabilize within ${durationMs * 3}ms` +
      ` (last count: ${lastCount})`,
  );
}
