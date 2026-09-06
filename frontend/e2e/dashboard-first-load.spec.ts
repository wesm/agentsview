import { test, expect } from "@playwright/test";

test("root hydration precedes delayed analytics and paints within 2000 ms", async ({ page }) => {
  const requests: string[] = [];
  let analyticsResponded = false;
  let preHydrationRows: number | undefined;
  page.on("request", (request) => {
    if (request.method() === "GET") requests.push(new URL(request.url()).pathname);
  });
  page.on("response", (response) => {
    if (response.url().includes("/api/v1/analytics/")) analyticsResponded = true;
  });
  await page.route("**/api/v1/analytics/**", async (route) => {
    await new Promise((resolve) => setTimeout(resolve, 6000));
    await route.continue();
  });
  await page.route(/\/api\/v1\/sessions\/test-session-[^/?]+(?:\?|$)/, async (route) => {
    preHydrationRows ??= await page.locator(".session-item").count();
    await route.continue();
  });
  const fixture = await page.request.get("/api/v1/sessions?project=project-alpha");
  expect(fixture.ok()).toBe(true);
  const body = await fixture.json();
  expect(body.sessions).toHaveLength(2);
  expect(body.sessions.every((session: { project: string }) => session.project === "project-alpha")).toBe(true);
  const started = Date.now();
  await page.goto("/", { waitUntil: "commit" });
  const visible = await page.locator(".session-item").first().isVisible().then(async (ready) => {
    if (ready) return true;
    return page.locator(".session-item").first().waitFor({ state: "visible", timeout: Math.max(1, 2000 - (Date.now() - started)) }).then(() => true, () => false);
  });
  const rowMs = Date.now() - started;
  const respondedAtRow = analyticsResponded;
  await expect.poll(() => requests.some((path) => path.includes("/analytics/"))).toBe(true);
  const hydration = requests.findIndex((path) => /^\/api\/v1\/sessions\/test-session-/.test(path));
  const analytics = requests.findIndex((path) => path.includes("/analytics/"));
  console.log(JSON.stringify({ fixture: "project-alpha", fixtureRows: body.sessions.length, preHydrationRows, hydration, analytics, rowMs, visible, respondedAtRow, requests }));
  expect(preHydrationRows).toBe(0);
  expect.soft(hydration).toBeGreaterThanOrEqual(0);
  expect.soft(hydration).toBeLessThan(analytics);
  expect.soft(visible).toBe(true);
  expect.soft(rowMs).toBeLessThan(2000);
  expect(respondedAtRow).toBe(false);
});

for (const width of [1280, 768, 400]) {
  test(`root shell at ${width}`, async ({ page }, testInfo) => {
    await page.setViewportSize({ width, height: 900 });
    await page.route("**/api/v1/analytics/**", () => {});
    await page.goto("/");
    await expect(page.locator(".analytics-page")).toBeVisible();
    await expect(page.locator(".analytics-toolbar")).toBeVisible();
    await page.screenshot({ path: testInfo.outputPath(`1592-2-after-${width}.png`) });
  });
}
