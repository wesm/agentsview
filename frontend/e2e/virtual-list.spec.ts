import { test, expect } from "@playwright/test";
import {
  createMockSessions,
  handleSessionsRoute,
} from "./helpers/mock-sessions";
import { scrollListTo } from "./helpers/virtual-list-helpers";

const TOTAL_SESSIONS = 500;
const DEEP_SESSIONS = 2000;

const sessions = createMockSessions(
  TOTAL_SESSIONS,
  "session",
  (i) => (i % 2 === 0 ? "project-alpha" : "project-beta"),
);

const deepSessions = createMockSessions(
  DEEP_SESSIONS,
  "deep-session",
  () => "deep",
);

const tinySessions = [sessions[0]];

test.describe("Virtual list behavior", () => {
  test.beforeEach(async ({ page }) => {
    await page.route(
      "**/api/v1/sessions*",
      handleSessionsRoute([
        { sessions, project: null },
        { sessions: deepSessions, project: "deep" },
        { sessions: tinySessions, project: "tiny" },
      ]),
    );

    await page.route("**/api/v1/projects", async (route) => {
      await route.fulfill({
        json: {
          projects: [
            { name: "project-alpha", session_count: 250 },
            { name: "project-beta", session_count: 250 },
            { name: "tiny", session_count: 1 },
            { name: "deep", session_count: DEEP_SESSIONS },
          ],
        },
      });
    });
  });

  test("loads more items when scrolling down", async ({ page }) => {
    await page.goto("/");
    const list = page.locator(".session-list-scroll");

    await expect(page.locator("button.session-item").first()).toBeVisible();

    const requestPromise = page.waitForRequest(
      (req) =>
        req.url().includes("/sessions") &&
        req.url().includes("cursor="),
    );

    await scrollListTo(list, "bottom");

    const request = await requestPromise;
    expect(request).toBeTruthy();

    const url = new URL(request.url());
    expect(url.searchParams.get("cursor")).toBeTruthy();

    await expect(
      page.getByText(`Hello from session ${TOTAL_SESSIONS - 1}`),
    ).toBeVisible();
  });

  test("clamps scroll position when filtering", async ({
    page,
  }) => {
    await page.goto("/");
    const list = page.locator(".session-list-scroll");

    await scrollListTo(list, 2000);

    await expect
      .poll(async () => {
        return await list.evaluate((el) => el.scrollTop);
      })
      .toBeGreaterThan(0);

    const projectSelect = page.locator("select.project-select");
    await projectSelect.selectOption("tiny");

    await expect
      .poll(
        async () => {
          return await list.evaluate((el) => el.scrollTop);
        },
        { timeout: 2000 },
      )
      .toBe(0);
  });

  test("keeps loading after dragging into an unloaded middle range", async ({
    page,
  }) => {
    await page.goto("/");
    const list = page.locator(".session-list-scroll");
    const projectSelect = page.locator("select.project-select");

    await projectSelect.selectOption("deep");
    await expect(
      page.getByRole("button", {
        name: /Hello from deep-session 0/i,
      }),
    ).toBeVisible();

    await scrollListTo(list, "middle");

    await expect(
      page.getByRole("button", {
        name: /Hello from deep-session 1000/i,
      }),
    ).toBeVisible({ timeout: 10_000 });
  });

  test("keeps loading after dragging to the end of an unloaded range", async ({
    page,
  }) => {
    await page.goto("/");
    const list = page.locator(".session-list-scroll");
    const projectSelect = page.locator("select.project-select");

    await projectSelect.selectOption("deep");
    await expect(
      page.getByRole("button", {
        name: /Hello from deep-session 0/i,
      }),
    ).toBeVisible();

    await scrollListTo(list, "bottom");

    await expect(
      page.getByRole("button", {
        name: /Hello from deep-session 1999/i,
      }),
    ).toBeVisible({ timeout: 15_000 });
  });
});
