import {
  describe,
  it,
  expect,
  vi,
  beforeEach,
} from "vitest";
import { commitsDisagree, sync } from "./sync.svelte.js";
import * as api from "../api/client.js";
import type { SyncStats } from "../api/types.js";

vi.mock("../api/client.js", () => ({
  triggerSync: vi.fn(),
  triggerResync: vi.fn(),
  getSyncStatus: vi.fn(),
  getStats: vi.fn(),
  getVersion: vi.fn(),
  watchSession: vi.fn(),
}));

const MOCK_STATS: SyncStats = {
  synced: 5,
  skipped: 3,
  total_sessions: 8,
};

function mockResyncSuccess(): void {
  vi.mocked(api.triggerResync).mockReturnValue({
    abort: vi.fn(),
    done: Promise.resolve(MOCK_STATS),
  });
  vi.mocked(api.getStats).mockResolvedValue({
    session_count: 8,
    message_count: 100,
    project_count: 3,
    machine_count: 1,
  });
}

function mockResyncFailure(error: Error): void {
  vi.mocked(api.triggerResync).mockReturnValue({
    abort: vi.fn(),
    done: Promise.reject(error),
  });
}

describe("commitsDisagree", () => {
  it.each([
    // Unknown / undefined handling
    { expected: false, hash1: "unknown", hash2: "unknown", scenario: "both are unknown" },
    { expected: false, hash1: "unknown", hash2: "abc1234", scenario: "frontend is unknown" },
    { expected: false, hash1: "abc1234", hash2: "unknown", scenario: "server is unknown" },
    { expected: false, hash1: undefined, hash2: "abc1234", scenario: "first hash is undefined" },
    { expected: false, hash1: "abc1234", hash2: undefined, scenario: "second hash is undefined" },
    { expected: false, hash1: undefined, hash2: undefined, scenario: "both hashes are undefined" },

    // Empty strings
    { expected: false, hash1: "", hash2: "abc1234", scenario: "first hash is empty" },
    { expected: false, hash1: "abc1234", hash2: "", scenario: "second hash is empty" },
    { expected: false, hash1: "", hash2: "", scenario: "both hashes are empty" },

    // Matches
    { expected: false, hash1: "abc1234", hash2: "abc1234", scenario: "identical short hashes" },
    { expected: false, hash1: "abc1234", hash2: "abc1234def5678", scenario: "short matches full SHA prefix" },
    { expected: false, hash1: "abc1234aaaaaaaaaaaa", hash2: "abc1234aaaaaaaaaaaa", scenario: "identical full SHAs" },
    { expected: false, hash1: "abc12", hash2: "abc1234def5678", scenario: "short abbreviation matching prefix" },

    // Mismatches
    { expected: true, hash1: "abc1234", hash2: "def5678", scenario: "different hashes" },
    { expected: true, hash1: "abc1234aaaaaaaaaaaa", hash2: "def5678bbbbbbbbbbb", scenario: "full SHAs differ" },
    { expected: true, hash1: "abc1234aaaaaaaaaaaa", hash2: "abc1234bbbbbbbbbbb", scenario: "full SHAs share 7-char prefix" },
    { expected: true, hash1: "xyz99", hash2: "abc1234def5678", scenario: "short abbreviation not matching" },
  ])(
    "returns $expected when $scenario",
    ({ expected, hash1, hash2 }) => {
      expect(commitsDisagree(hash1, hash2)).toBe(expected);
    },
  );
});

describe("SyncStore.triggerResync", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    // Reset singleton state between tests.
    const s = sync as unknown as Record<string, unknown>;
    s.syncing = false;
    s.progress = null;
  });

  it("returns false when already syncing", () => {
    mockResyncSuccess();
    const first = sync.triggerResync();
    expect(first).toBe(true);
    expect(sync.syncing).toBe(true);

    const second = sync.triggerResync();
    expect(second).toBe(false);
  });

  it("calls onError on stream failure", async () => {
    const error = new Error("stream failed");
    mockResyncFailure(error);

    const onError = vi.fn();
    sync.triggerResync(undefined, onError);

    await vi.waitFor(() => {
      expect(onError).toHaveBeenCalledWith(error);
    });
    expect(sync.syncing).toBe(false);
  });

  it("resets syncing on non-Error rejection", async () => {
    vi.mocked(api.triggerResync).mockReturnValue({
      abort: vi.fn(),
      done: Promise.reject("string error"),
    });

    const onError = vi.fn();
    sync.triggerResync(undefined, onError);

    await vi.waitFor(() => {
      expect(onError).toHaveBeenCalled();
    });
    expect(onError).toHaveBeenCalledWith(
      expect.objectContaining({ message: "Sync failed" }),
    );
    expect(sync.syncing).toBe(false);
  });

  it("allows retry after error", async () => {
    mockResyncFailure(new Error("fail"));
    const onError = vi.fn();
    sync.triggerResync(undefined, onError);

    await vi.waitFor(() => {
      expect(onError).toHaveBeenCalled();
    });

    // Retry should succeed
    mockResyncSuccess();
    const onComplete = vi.fn();
    const started = sync.triggerResync(onComplete);
    expect(started).toBe(true);

    await vi.waitFor(() => {
      expect(onComplete).toHaveBeenCalled();
    });
    expect(sync.syncing).toBe(false);
  });

  it("calls onComplete on success", async () => {
    mockResyncSuccess();
    const onComplete = vi.fn();
    sync.triggerResync(onComplete);

    await vi.waitFor(() => {
      expect(onComplete).toHaveBeenCalled();
    });
    expect(sync.syncing).toBe(false);
    expect(sync.lastSyncStats).toEqual(MOCK_STATS);
  });
});
