import { test, expect } from "@playwright/test";
import { SessionsPage } from "./pages/sessions-page";

test.describe("Navigation", () => {
  let sp: SessionsPage;

  test.beforeEach(async ({ page }) => {
    sp = new SessionsPage(page);
    await sp.goto();
  });

  test("minimap renders with non-zero canvas", async () => {
    await sp.selectFirstSession();

    const canvas = sp.page.locator("canvas");
    await expect(canvas).toBeVisible();

    const box = await canvas.boundingBox();
    expect(box).not.toBeNull();
    expect(box!.width).toBeGreaterThan(0);
    expect(box!.height).toBeGreaterThan(0);
  });

  test("keyboard ] navigates to next session", async () => {
    await sp.sessionItems.first().click();
    await expect(sp.sessionItems.first()).toHaveClass(/active/);

    await sp.page.keyboard.press("]");
    await expect(sp.sessionItems.nth(1)).toHaveClass(/active/);
  });

  test("keyboard [ navigates to previous session", async () => {
    await sp.sessionItems.nth(1).click();
    await expect(sp.sessionItems.nth(1)).toHaveClass(/active/);

    await sp.page.keyboard.press("[");
    await expect(sp.sessionItems.first()).toHaveClass(/active/);
  });

  test("empty state shows when no session selected", async () => {
    const empty = sp.page.locator(".empty-state");
    await expect(empty).toBeVisible();
    await expect(empty).toContainText("Select a session");
  });
});
