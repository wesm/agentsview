import { test, expect } from "@playwright/test";
import { SessionsPage } from "./pages/sessions-page";

test.describe("Session list", () => {
  let sp: SessionsPage;

  test.beforeEach(async ({ page }) => {
    sp = new SessionsPage(page);
    await sp.goto();
  });

  test("sessions load and display", async () => {
    expect(await sp.sessionItems.count()).toBe(8);
  });

  test("session count header is visible", async () => {
    await expect(sp.sessionListHeader).toBeVisible();
    await expect(sp.sessionListHeader).toContainText("sessions");
  });

  test("clicking a session marks it active", async () => {
    await sp.sessionItems.first().click();
    await expect(sp.sessionItems.first()).toHaveClass(/active/);
  });

  test("project filter changes do not blank virtualized list", async () => {
    const filterCases = [
      { project: "project-alpha", expectedCount: 2 },
      { project: "project-beta", expectedCount: 3 },
      { project: "", expectedCount: 8 },
    ];

    for (const { project, expectedCount } of filterCases) {
      const label = project || "all";
      await test.step(`filter by ${label}`, async () => {
        if (project) {
          await sp.filterByProject(project);
        } else {
          await sp.clearProjectFilter();
        }
        await expect(sp.sessionItems.first()).toBeVisible();
        await expect(sp.sessionListHeader).toContainText(
          `${expectedCount} sessions`,
        );
        await expect(sp.sessionItems).toHaveCount(expectedCount);
      });
    }
  });
});
