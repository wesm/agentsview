import { test, expect } from "@playwright/test";
import {
  createMockSessions,
  handleSessionsRoute,
} from "./helpers/mock-sessions";
import {
  getScrollTop,
  scrollListTo,
} from "./helpers/virtual-list-helpers";
import { SessionsPage } from "./pages/sessions-page";

const TOTAL_SESSIONS = 500;
const DEEP_SESSIONS = 2000;
const MIDDLE_INDEX = Math.floor(DEEP_SESSIONS / 2);
const LAST_INDEX = DEEP_SESSIONS - 1;

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
  let sp: SessionsPage;

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

    sp = new SessionsPage(page);
    await page.goto("/");
    await expect(sp.sessionItems.first()).toBeVisible();
  });

  test("renders end of list when scrolling down", async () => {
    await scrollListTo(sp.sessionListScroll, "bottom");

    await expect(
      sp.page.getByText(
        `Hello from session ${TOTAL_SESSIONS - 1}`,
      ),
    ).toBeVisible();
  });

  test("clamps scroll position when filtering", async () => {
    await scrollListTo(sp.sessionListScroll, 2000);

    await expect
      .poll(() => getScrollTop(sp.sessionListScroll))
      .toBeGreaterThan(0);

    await sp.filterByProject("tiny");

    await expect
      .poll(() => getScrollTop(sp.sessionListScroll), {
        timeout: 2000,
      })
      .toBe(0);
  });

  test("keeps loading after dragging into an unloaded middle range", async () => {
    await sp.filterByProject("deep");
    await expect(
      sp.page.getByRole("button", {
        name: /Hello from deep-session 0/i,
      }),
    ).toBeVisible();

    await scrollListTo(sp.sessionListScroll, "middle");

    await expect(
      sp.page.getByRole("button", {
        name: new RegExp(
          `Hello from deep-session ${MIDDLE_INDEX}`,
          "i",
        ),
      }),
    ).toBeVisible({ timeout: 10_000 });
  });

  test("keeps loading after dragging to the end of an unloaded range", async () => {
    await sp.filterByProject("deep");
    await expect(
      sp.page.getByRole("button", {
        name: /Hello from deep-session 0/i,
      }),
    ).toBeVisible();

    await scrollListTo(sp.sessionListScroll, "bottom");

    await expect(
      sp.page.getByRole("button", {
        name: new RegExp(
          `Hello from deep-session ${LAST_INDEX}`,
          "i",
        ),
      }),
    ).toBeVisible({ timeout: 10_000 });
  });
});
