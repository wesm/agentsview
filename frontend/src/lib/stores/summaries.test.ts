import {
  describe,
  it,
  expect,
  vi,
  beforeEach,
} from "vitest";
import { summaries } from "./summaries.svelte.js";
import * as api from "../api/client.js";
import type { Summary } from "../api/types.js";

vi.mock("../api/client.js", () => ({
  listSummaries: vi.fn(),
  getSummary: vi.fn(),
  generateSummary: vi.fn(),
}));

function makeSummary(overrides: Partial<Summary> = {}): Summary {
  return {
    id: 1,
    type: "daily_activity",
    date: "2025-01-15",
    project: null,
    agent: "claude",
    model: "claude-sonnet-4-20250514",
    prompt: null,
    content: "# Summary\nThings happened.",
    created_at: "2025-01-15T12:00:00.000Z",
    ...overrides,
  };
}

beforeEach(() => {
  vi.clearAllMocks();
  summaries.summaries = [];
  summaries.selectedId = null;
  summaries.loading = false;
  summaries.generating = false;
  summaries.generateError = null;
  summaries.promptText = "";
});

describe("load", () => {
  it("fetches summaries and updates state", async () => {
    const s1 = makeSummary({ id: 1 });
    const s2 = makeSummary({ id: 2, project: "my-app" });
    vi.mocked(api.listSummaries).mockResolvedValueOnce({
      summaries: [s2, s1],
    });

    await summaries.load();

    expect(api.listSummaries).toHaveBeenCalledWith({
      type: "daily_activity",
      date: summaries.date,
      project: undefined,
    });
    expect(summaries.summaries).toHaveLength(2);
    expect(summaries.loading).toBe(false);
  });

  it("clears selectedId when summary no longer in list", async () => {
    summaries.selectedId = 99;
    vi.mocked(api.listSummaries).mockResolvedValueOnce({
      summaries: [makeSummary({ id: 1 })],
    });

    await summaries.load();

    expect(summaries.selectedId).toBeNull();
  });

  it("preserves selectedId when summary is in list", async () => {
    summaries.selectedId = 1;
    vi.mocked(api.listSummaries).mockResolvedValueOnce({
      summaries: [makeSummary({ id: 1 })],
    });

    await summaries.load();

    expect(summaries.selectedId).toBe(1);
  });
});

describe("setDate", () => {
  it("updates date, clears selection, and reloads", async () => {
    summaries.selectedId = 1;
    vi.mocked(api.listSummaries).mockResolvedValueOnce({
      summaries: [],
    });

    summaries.setDate("2025-02-01");

    expect(summaries.date).toBe("2025-02-01");
    expect(summaries.selectedId).toBeNull();
    expect(api.listSummaries).toHaveBeenCalled();
  });
});

describe("setType", () => {
  it("updates type and reloads", async () => {
    vi.mocked(api.listSummaries).mockResolvedValueOnce({
      summaries: [],
    });

    summaries.setType("agent_analysis");

    expect(summaries.type).toBe("agent_analysis");
    expect(api.listSummaries).toHaveBeenCalled();
  });
});

describe("setProject", () => {
  it("updates project and reloads", async () => {
    vi.mocked(api.listSummaries).mockResolvedValueOnce({
      summaries: [],
    });

    summaries.setProject("my-app");

    expect(summaries.project).toBe("my-app");
    expect(api.listSummaries).toHaveBeenCalled();
  });
});

describe("select", () => {
  it("sets selectedId", () => {
    summaries.select(42);
    expect(summaries.selectedId).toBe(42);
  });
});

describe("selectedSummary", () => {
  it("returns matching summary", () => {
    const s = makeSummary({ id: 5 });
    summaries.summaries = [s];
    summaries.selectedId = 5;
    expect(summaries.selectedSummary).toEqual(s);
  });

  it("returns undefined when no match", () => {
    summaries.summaries = [makeSummary({ id: 1 })];
    summaries.selectedId = 99;
    expect(summaries.selectedSummary).toBeUndefined();
  });
});

describe("generate", () => {
  it("sets generating state and prepends result", async () => {
    const newSummary = makeSummary({ id: 10 });
    const mockHandle = {
      abort: vi.fn(),
      done: Promise.resolve(newSummary),
    };
    vi.mocked(api.generateSummary).mockReturnValueOnce(
      mockHandle,
    );

    await summaries.generate();

    expect(summaries.generating).toBe(false);
    expect(summaries.summaries[0]).toEqual(newSummary);
    expect(summaries.selectedId).toBe(10);
  });

  it("sets generateError on failure", async () => {
    const mockHandle = {
      abort: vi.fn(),
      done: Promise.reject(new Error("CLI not found")),
    };
    vi.mocked(api.generateSummary).mockReturnValueOnce(
      mockHandle,
    );

    await summaries.generate();

    expect(summaries.generating).toBe(false);
    expect(summaries.generateError).toBe("CLI not found");
  });

  it("calls load instead of prepend when filters changed", async () => {
    const newSummary = makeSummary({ id: 20 });
    let resolveDone!: (s: Summary) => void;
    const mockHandle = {
      abort: vi.fn(),
      done: new Promise<Summary>((resolve) => {
        resolveDone = resolve;
      }),
    };
    vi.mocked(api.generateSummary).mockReturnValueOnce(
      mockHandle,
    );
    vi.mocked(api.listSummaries).mockResolvedValue({
      summaries: [newSummary],
    });

    const promise = summaries.generate();

    // Change date while generation is in flight.
    summaries.date = "2025-03-01";

    resolveDone(newSummary);
    await promise;

    // Should not have prepended — should have called load.
    expect(api.listSummaries).toHaveBeenCalled();
    expect(summaries.selectedId).not.toBe(20);
  });

  it("ignores abort errors", async () => {
    const abortError = new DOMException(
      "Aborted",
      "AbortError",
    );
    const mockHandle = {
      abort: vi.fn(),
      done: Promise.reject(abortError),
    };
    vi.mocked(api.generateSummary).mockReturnValueOnce(
      mockHandle,
    );

    await summaries.generate();

    expect(summaries.generating).toBe(false);
    expect(summaries.generateError).toBeNull();
  });
});

describe("cancelGeneration", () => {
  it("calls abort on active handle", async () => {
    const abortFn = vi.fn();
    let rejectDone!: (err: Error) => void;
    const mockHandle = {
      abort: abortFn,
      done: new Promise<Summary>((_resolve, reject) => {
        rejectDone = reject;
      }),
    };
    vi.mocked(api.generateSummary).mockReturnValueOnce(
      mockHandle,
    );

    const promise = summaries.generate();

    // Cancel triggers abort
    summaries.cancelGeneration();
    expect(abortFn).toHaveBeenCalled();

    // Unblock by rejecting with AbortError
    rejectDone(
      new DOMException("Aborted", "AbortError"),
    );
    await promise;

    expect(summaries.generating).toBe(false);
    expect(summaries.generateError).toBeNull();
  });
});
