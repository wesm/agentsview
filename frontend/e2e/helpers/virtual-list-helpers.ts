import type { Locator } from "@playwright/test";

type ScrollPosition = "top" | "bottom" | "middle" | number;

/**
 * Scrolls a virtual list container to the given position
 * and dispatches a scroll event to trigger virtualizer updates.
 */
export async function scrollListTo(
  locator: Locator,
  position: ScrollPosition,
): Promise<void> {
  await locator.evaluate((el, pos) => {
    if (pos === "top") {
      el.scrollTop = 0;
    } else if (pos === "bottom") {
      el.scrollTop = el.scrollHeight;
    } else if (pos === "middle") {
      el.scrollTop = (el.scrollHeight - el.clientHeight) / 2;
    } else {
      el.scrollTop = pos;
    }
    el.dispatchEvent(new Event("scroll"));
  }, position);
}
