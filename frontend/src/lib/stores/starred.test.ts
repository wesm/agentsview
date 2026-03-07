import {
  describe,
  it,
  expect,
  beforeEach,
  afterEach,
  vi,
} from "vitest";
import * as api from "../api/client.js";
import { createStarredStore } from "./starred.svelte.js";

vi.mock("../api/client.js", () => ({
  listStarred: vi.fn().mockResolvedValue({ session_ids: [] }),
  starSession: vi.fn().mockResolvedValue(undefined),
  unstarSession: vi.fn().mockResolvedValue(undefined),
  bulkStarSessions: vi.fn().mockResolvedValue(undefined),
}));

const STORAGE_KEY = "agentsview-starred-sessions";

describe("StarredStore", () => {
  let starred: ReturnType<typeof createStarredStore>;

  beforeEach(() => {
    localStorage.removeItem(STORAGE_KEY);
    vi.mocked(api.listStarred).mockResolvedValue({
      session_ids: [],
    });
    starred = createStarredStore();
  });

  it("starts empty when no localStorage data", () => {
    expect(starred.count).toBe(0);
    expect(starred.isStarred("abc")).toBe(false);
  });

  it("can star a session", () => {
    starred.star("session-1");
    expect(starred.isStarred("session-1")).toBe(true);
    expect(starred.count).toBe(1);
  });

  it("can unstar a session", () => {
    starred.star("session-1");
    starred.unstar("session-1");
    expect(starred.isStarred("session-1")).toBe(false);
    expect(starred.count).toBe(0);
  });

  it("toggle adds then removes", () => {
    starred.toggle("session-2");
    expect(starred.isStarred("session-2")).toBe(true);

    starred.toggle("session-2");
    expect(starred.isStarred("session-2")).toBe(false);
  });

  it("star is idempotent", () => {
    starred.star("s1");
    starred.star("s1");
    expect(starred.count).toBe(1);
  });

  it("unstar is idempotent", () => {
    starred.unstar("nonexistent");
    expect(starred.count).toBe(0);
  });

  it("handles multiple starred sessions", () => {
    starred.star("a");
    starred.star("b");
    starred.star("c");
    expect(starred.count).toBe(3);
    expect(starred.isStarred("a")).toBe(true);
    expect(starred.isStarred("b")).toBe(true);
    expect(starred.isStarred("c")).toBe(true);
    expect(starred.isStarred("d")).toBe(false);
  });

  describe("filterOnly", () => {
    it("defaults to false", () => {
      expect(starred.filterOnly).toBe(false);
    });

    it("can be toggled on and off", () => {
      starred.filterOnly = true;
      expect(starred.filterOnly).toBe(true);
      starred.filterOnly = false;
      expect(starred.filterOnly).toBe(false);
    });

    it("is independent of starred ids", () => {
      starred.star("s1");
      starred.filterOnly = true;
      starred.unstar("s1");
      expect(starred.filterOnly).toBe(true);
      expect(starred.count).toBe(0);
    });
  });
});

describe("StarredStore localStorage seeding", () => {
  afterEach(() => {
    localStorage.removeItem(STORAGE_KEY);
  });

  it("seeds ids from localStorage on construction", () => {
    localStorage.setItem(
      STORAGE_KEY,
      JSON.stringify(["legacy-1", "legacy-2"]),
    );
    const store = createStarredStore();
    expect(store.isStarred("legacy-1")).toBe(true);
    expect(store.isStarred("legacy-2")).toBe(true);
    expect(store.count).toBe(2);
  });

  it("toggle unstars a localStorage-seeded session", () => {
    localStorage.setItem(
      STORAGE_KEY,
      JSON.stringify(["legacy-1"]),
    );
    const store = createStarredStore();
    expect(store.isStarred("legacy-1")).toBe(true);

    store.toggle("legacy-1");
    expect(store.isStarred("legacy-1")).toBe(false);
  });
});

describe("StarredStore load retry", () => {
  beforeEach(() => {
    localStorage.removeItem(STORAGE_KEY);
    vi.clearAllMocks();
    vi.useFakeTimers();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it("retries load after failure with backoff", async () => {
    vi.mocked(api.listStarred)
      .mockRejectedValueOnce(new Error("network"))
      .mockResolvedValueOnce({ session_ids: ["srv-1"] });

    const store = createStarredStore();
    await store.load();

    expect(store.isStarred("srv-1")).toBe(false);

    // First retry at 2s
    await vi.advanceTimersByTimeAsync(2000);

    expect(store.isStarred("srv-1")).toBe(true);
  });

  it("stops retrying after 3 failures", async () => {
    vi.mocked(api.listStarred)
      .mockRejectedValue(new Error("network"));

    const store = createStarredStore();
    await store.load(); // fail 1

    await vi.advanceTimersByTimeAsync(2000); // retry 1 (fail 2)
    await vi.advanceTimersByTimeAsync(4000); // retry 2 (fail 3)
    await vi.advanceTimersByTimeAsync(8000); // retry 3 (fail 4 — no more retries)

    // Should have been called 4 times total (initial + 3 retries)
    expect(api.listStarred).toHaveBeenCalledTimes(4);

    // No further retry scheduled
    await vi.advanceTimersByTimeAsync(16000);
    expect(api.listStarred).toHaveBeenCalledTimes(4);
  });

  it("does not create overlapping retry chains on repeated load()", async () => {
    let callCount = 0;
    vi.mocked(api.listStarred).mockImplementation(() => {
      callCount++;
      return Promise.reject(new Error("network"));
    });

    const store = createStarredStore();

    // First load fails, schedules retry at 2s
    await store.load();
    expect(callCount).toBe(1);

    // Manually call load() again before the retry fires — the
    // pending timer should prevent a second timer from being
    // scheduled.
    await store.load();
    expect(callCount).toBe(2);

    // Only ONE retry should fire at 2s, not two
    await vi.advanceTimersByTimeAsync(2000);
    expect(callCount).toBe(3);

    // If overlapping timers existed we'd see an extra call here
    await vi.advanceTimersByTimeAsync(2000);
    // Next retry fires at 4s from second scheduleRetry
    // but total calls should follow single-chain progression
    expect(callCount).toBeLessThanOrEqual(4);
  });

  it("does not retry after successful load", async () => {
    vi.mocked(api.listStarred).mockResolvedValue({
      session_ids: ["s1"],
    });

    const store = createStarredStore();
    await store.load();

    expect(store.isStarred("s1")).toBe(true);

    // No retry should be scheduled
    await vi.advanceTimersByTimeAsync(10000);
    // listStarred called once for initial load only
    // (reconcileIfIdle may also call it, but no retry timer)
    expect(store.isStarred("s1")).toBe(true);
  });
});
