import { describe, it, expect, vi, beforeEach } from "vitest";
import { copyToClipboard } from "./clipboard.js";

describe("copyToClipboard", () => {
  beforeEach(() => {
    vi.restoreAllMocks();
  });

  it("copies text and returns true on success", async () => {
    const writeText = vi.fn().mockResolvedValue(undefined);
    Object.assign(navigator, { clipboard: { writeText } });
    const result = await copyToClipboard("test-id");
    expect(writeText).toHaveBeenCalledWith("test-id");
    expect(result).toBe(true);
  });

  it("returns false when clipboard write fails", async () => {
    const writeText = vi.fn().mockRejectedValue(new Error("denied"));
    Object.assign(navigator, { clipboard: { writeText } });
    const result = await copyToClipboard("test-id");
    expect(result).toBe(false);
  });
});
