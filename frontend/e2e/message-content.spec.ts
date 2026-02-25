import { test, expect, type Page } from "@playwright/test";

const LOC = {
  sessionItem: ".session-item",
  sessionProject: ".session-project",
  sessionCount: ".session-count",
  listScroll: ".message-list-scroll",
  row: ".virtual-row",
} as const;

const BETA_6 = {
  project: "project-beta",
  count: 3, // user_message_count shown in sidebar
  displayRows: 5,
};

function getSessionItem(
  page: Page,
  project: string,
  count: number,
) {
  return page
    .locator(LOC.sessionItem)
    .filter({
      has: page.locator(
        `${LOC.sessionProject}:text-is("${project}")`,
      ),
    })
    .filter({
      has: page.locator(
        `${LOC.sessionCount}:text-is("${count}")`,
      ),
    });
}

async function selectSession(
  page: Page,
  project: string,
  count: number,
): Promise<string> {
  const item = getSessionItem(page, project, count);
  const sessionId = await item.getAttribute("data-session-id");
  expect(sessionId).toBeTruthy();
  await item.click();
  await expect(item).toHaveClass(/active/);
  return sessionId!;
}

async function expectSessionLoaded(
  page: Page,
  sessionId: string,
  expectedRows?: number,
) {
  const messageList = page.locator(LOC.listScroll);
  await expect(messageList).toHaveAttribute(
    "data-session-id",
    sessionId,
  );
  await expect(messageList).toHaveAttribute(
    "data-messages-session-id",
    sessionId,
  );
  await expect(messageList).toHaveAttribute(
    "data-loaded",
    "true",
  );

  if (expectedRows !== undefined) {
    await expect(page.locator(LOC.row)).toHaveCount(expectedRows);
  } else {
    await expect(
      page.locator(LOC.row).first(),
    ).toBeVisible();
  }
}

test.describe("Mixed content rendering", () => {
  test.beforeEach(async ({ page }) => {
    await page.goto("/");
    await expect(
      page.locator(`button${LOC.sessionItem}`).first(),
    ).toBeVisible({ timeout: 5_000 });
  });

  test("tool group renders for consecutive tool-only messages", async ({
    page,
  }) => {
    const { project, count, displayRows } = BETA_6;
    const sid = await selectSession(page, project, count);
    await expectSessionLoaded(page, sid, displayRows);

    const toolGroup = page.locator(".tool-group");
    await expect(toolGroup).toBeVisible();
    await expect(toolGroup).toContainText(/tool calls?/i);

    const toolGroupBody = page.locator(".tool-group-body");
    await expect(toolGroupBody).toBeVisible();

    // Should contain exactly 2 tool blocks inside the group
    // (Indices 3 and 4 in the fixture are tool calls)
    const toolBlocks = toolGroupBody.locator(".tool-block");
    await expect(toolBlocks).toHaveCount(2);
  });

  test("thinking block is collapsed by default", async ({
    page,
  }) => {
    const { project, count, displayRows } = BETA_6;
    const sid = await selectSession(page, project, count);
    await expectSessionLoaded(page, sid, displayRows);

    const thinkingBlock = page.locator(".thinking-block");
    await expect(thinkingBlock).toBeVisible();

    // Content should be hidden (collapsed by default)
    const thinkingContent = page.locator(".thinking-content");
    await expect(thinkingContent).not.toBeVisible();

    // Click to expand
    const thinkingHeader = page.locator(".thinking-header");
    await thinkingHeader.click();

    // Content should now be visible
    await expect(thinkingContent).toBeVisible();
    await expect(thinkingContent).toContainText(
      "Let me analyze...",
    );
  });
});
