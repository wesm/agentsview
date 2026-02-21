import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { searchStore } from "./search.svelte.js";
import * as api from "../api/client.js";
import type { SearchResponse } from "../api/types.js";

vi.mock("../api/client.js", () => ({
  search: vi.fn(),
}));

function createDeferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((r) => {
    resolve = r;
  });
  return { promise, resolve };
}

function makeSearchResponse(
  query: string,
  count: number,
): SearchResponse {
  return {
    query,
    results: Array.from({ length: count }, (_, i) => ({
      session_id: `s${i}`,
      project: "proj",
      ordinal: i,
      role: "assistant",
      timestamp: new Date().toISOString(),
      snippet: `result ${i}`,
      rank: i,
    })),
    count,
    next: 0,
  };
}

function abortError(): DOMException {
  return new DOMException("The operation was aborted", "AbortError");
}

const DEBOUNCE_MS = 300;

describe("SearchStore", () => {
  beforeEach(() => {
    vi.useFakeTimers();
    searchStore.clear();
    vi.clearAllMocks();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it("should abort stale in-flight search when a new one starts", async () => {
    // First search: slow, will be aborted
    vi.mocked(api.search).mockImplementationOnce(
      (_q, _p, init) =>
        new Promise((resolve, reject) => {
          init?.signal?.addEventListener("abort", () => {
            reject(abortError());
          });
        }),
    );

    // Second search: resolves immediately
    vi.mocked(api.search).mockResolvedValueOnce(
      makeSearchResponse("world", 2),
    );

    // Trigger first search
    searchStore.search("hello");
    vi.advanceTimersByTime(DEBOUNCE_MS);

    // Trigger second search (aborts first)
    searchStore.search("world");
    vi.advanceTimersByTime(DEBOUNCE_MS);

    // Wait for all async work
    await vi.runAllTimersAsync();
    await Promise.resolve();

    // Results should be from the second search
    expect(searchStore.results.length).toBe(2);
    expect(searchStore.isSearching).toBe(false);
  });

  it("should abort in-flight search on clear()", async () => {
    vi.mocked(api.search).mockImplementationOnce(
      (_q, _p, init) =>
        new Promise((resolve, reject) => {
          init?.signal?.addEventListener("abort", () => {
            reject(abortError());
          });
        }),
    );

    // Trigger search
    searchStore.search("hello");
    vi.advanceTimersByTime(DEBOUNCE_MS);

    // Clear while search is in-flight
    searchStore.clear();
    await Promise.resolve();

    // Results should remain empty after clear
    expect(searchStore.results.length).toBe(0);
    expect(searchStore.query).toBe("");
    expect(searchStore.isSearching).toBe(false);
  });

  it("should debounce rapid queries and only fire the last one", async () => {
    vi.mocked(api.search).mockResolvedValueOnce(
      makeSearchResponse("final", 3),
    );

    // Type several queries within debounce window
    searchStore.search("f");
    vi.advanceTimersByTime(100);
    searchStore.search("fi");
    vi.advanceTimersByTime(100);
    searchStore.search("final");
    vi.advanceTimersByTime(DEBOUNCE_MS);

    await vi.runAllTimersAsync();
    await Promise.resolve();

    // Only one API call should have been made (for "final")
    expect(api.search).toHaveBeenCalledTimes(1);
    expect(api.search).toHaveBeenCalledWith(
      "final",
      expect.objectContaining({ limit: 30 }),
      expect.objectContaining({ signal: expect.any(AbortSignal) }),
    );
    expect(searchStore.results.length).toBe(3);
  });

  it("should clear results immediately for empty query", () => {
    // Manually set some results first
    searchStore.search("test");
    // Now clear via empty query
    searchStore.search("");

    expect(searchStore.results.length).toBe(0);
    expect(searchStore.isSearching).toBe(false);
    // No API call should be made for empty query
    vi.advanceTimersByTime(DEBOUNCE_MS);
    expect(api.search).not.toHaveBeenCalled();
  });

  it("should keep isSearching true while a newer search is pending", async () => {
    // First search: aborted when second starts
    vi.mocked(api.search).mockImplementationOnce(
      (_q, _p, init) =>
        new Promise((resolve, reject) => {
          init?.signal?.addEventListener("abort", () => {
            reject(abortError());
          });
        }),
    );

    // Second search: hangs until resolved
    const secondReq = createDeferred<SearchResponse>();
    vi.mocked(api.search).mockReturnValueOnce(secondReq.promise);

    // Trigger first search
    searchStore.search("first");
    vi.advanceTimersByTime(DEBOUNCE_MS);

    // Trigger second search (aborts first)
    searchStore.search("second");
    vi.advanceTimersByTime(DEBOUNCE_MS);
    await Promise.resolve();

    // isSearching should be true (second search is in-flight)
    expect(searchStore.isSearching).toBe(true);

    // Resolve second search
    secondReq.resolve(makeSearchResponse("second", 2));
    await Promise.resolve();

    expect(searchStore.isSearching).toBe(false);
    expect(searchStore.results.length).toBe(2);
  });

  it("should discard results from request that resolves during debounce window", async () => {
    const firstReq = createDeferred<SearchResponse>();

    // First search: resolves after query changes but before debounce fires
    vi.mocked(api.search).mockImplementationOnce(
      (_q, _p, init) =>
        new Promise((resolve, reject) => {
          init?.signal?.addEventListener("abort", () => {
            reject(abortError());
          });
          firstReq.promise.then(resolve, reject);
        }),
    );

    // Second search: resolves immediately
    vi.mocked(api.search).mockResolvedValueOnce(
      makeSearchResponse("beta", 2),
    );

    // Fire first search
    searchStore.search("alpha");
    vi.advanceTimersByTime(DEBOUNCE_MS);

    // Query changes to "beta" — this aborts the first request
    // immediately, before the debounce fires
    searchStore.search("beta");

    // First request tries to resolve during the debounce window
    // but its signal is already aborted
    firstReq.resolve(makeSearchResponse("alpha", 5));
    await Promise.resolve();

    // Alpha results must not appear
    expect(searchStore.results.length).toBe(0);

    // Now let the debounce fire for "beta"
    vi.advanceTimersByTime(DEBOUNCE_MS);
    await vi.runAllTimersAsync();
    await Promise.resolve();

    // Results should be from "beta"
    expect(searchStore.results.length).toBe(2);
    expect(searchStore.isSearching).toBe(false);
  });

  it("should pass signal to api.search", async () => {
    vi.mocked(api.search).mockResolvedValueOnce(
      makeSearchResponse("test", 1),
    );

    searchStore.search("test");
    vi.advanceTimersByTime(DEBOUNCE_MS);

    await vi.runAllTimersAsync();
    await Promise.resolve();

    expect(api.search).toHaveBeenCalledWith(
      "test",
      { project: undefined, limit: 30 },
      { signal: expect.any(AbortSignal) },
    );
  });
});
