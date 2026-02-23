import {
  describe,
  it,
  expect,
  vi,
  beforeEach,
} from "vitest";
import { insights } from "./insights.svelte.js";
import * as api from "../api/client.js";
import type { Summary } from "../api/types.js";

vi.mock("../api/client.js", () => ({
  listSummaries: vi.fn(),
  getSummary: vi.fn(),
  generateSummary: vi.fn(),
}));

function makeSummary(
  overrides: Partial<Summary> = {},
): Summary {
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
  insights.summaries = [];
  insights.selectedId = null;
  insights.loading = false;
  insights.tasks = [];
  insights.promptText = "";
});

describe("load", () => {
  it("fetches summaries and updates state", async () => {
    const s1 = makeSummary({ id: 1 });
    const s2 = makeSummary({ id: 2, project: "my-app" });
    vi.mocked(api.listSummaries).mockResolvedValueOnce({
      summaries: [s2, s1],
    });

    await insights.load();

    expect(api.listSummaries).toHaveBeenCalledWith({
      type: "daily_activity",
      date: insights.date,
      project: undefined,
    });
    expect(insights.summaries).toHaveLength(2);
    expect(insights.loading).toBe(false);
  });

  it("clears selectedId when summary no longer in list", async () => {
    insights.selectedId = 99;
    vi.mocked(api.listSummaries).mockResolvedValueOnce({
      summaries: [makeSummary({ id: 1 })],
    });

    await insights.load();

    expect(insights.selectedId).toBeNull();
  });

  it("preserves selectedId when summary is in list", async () => {
    insights.selectedId = 1;
    vi.mocked(api.listSummaries).mockResolvedValueOnce({
      summaries: [makeSummary({ id: 1 })],
    });

    await insights.load();

    expect(insights.selectedId).toBe(1);
  });
});

describe("setDate", () => {
  it("updates date, clears selection, and reloads", async () => {
    insights.selectedId = 1;
    vi.mocked(api.listSummaries).mockResolvedValueOnce({
      summaries: [],
    });

    insights.setDate("2025-02-01");

    expect(insights.date).toBe("2025-02-01");
    expect(insights.selectedId).toBeNull();
    expect(api.listSummaries).toHaveBeenCalled();
  });
});

describe("setType", () => {
  it("updates type and reloads", async () => {
    vi.mocked(api.listSummaries).mockResolvedValueOnce({
      summaries: [],
    });

    insights.setType("agent_analysis");

    expect(insights.type).toBe("agent_analysis");
    expect(api.listSummaries).toHaveBeenCalled();
  });
});

describe("setProject", () => {
  it("updates project and reloads", async () => {
    vi.mocked(api.listSummaries).mockResolvedValueOnce({
      summaries: [],
    });

    insights.setProject("my-app");

    expect(insights.project).toBe("my-app");
    expect(api.listSummaries).toHaveBeenCalled();
  });
});

describe("select", () => {
  it("sets selectedId", () => {
    insights.select(42);
    expect(insights.selectedId).toBe(42);
  });
});

describe("selectedSummary", () => {
  it("returns matching summary", () => {
    const s = makeSummary({ id: 5 });
    insights.summaries = [s];
    insights.selectedId = 5;
    expect(insights.selectedSummary).toEqual(s);
  });

  it("returns undefined when no match", () => {
    insights.summaries = [makeSummary({ id: 1 })];
    insights.selectedId = 99;
    expect(insights.selectedSummary).toBeUndefined();
  });
});

describe("generate (multi-task)", () => {
  it("adds task to tasks[] and prepends result on completion", async () => {
    const newSummary = makeSummary({ id: 10 });
    const mockHandle = {
      abort: vi.fn(),
      done: Promise.resolve(newSummary),
    };
    vi.mocked(api.generateSummary).mockReturnValueOnce(
      mockHandle,
    );

    insights.generate();

    expect(insights.tasks).toHaveLength(1);
    expect(insights.tasks[0]!.status).toBe("generating");

    // Wait for the promise chain to settle
    await new Promise((r) => setTimeout(r, 0));

    expect(insights.tasks).toHaveLength(0);
    expect(insights.summaries[0]).toEqual(newSummary);
    expect(insights.selectedId).toBe(10);
  });

  it("supports multiple concurrent tasks", async () => {
    const s1 = makeSummary({ id: 10 });
    const s2 = makeSummary({ id: 11 });
    let resolve1!: (s: Summary) => void;
    let resolve2!: (s: Summary) => void;

    vi.mocked(api.generateSummary)
      .mockReturnValueOnce({
        abort: vi.fn(),
        done: new Promise((r) => {
          resolve1 = r;
        }),
      })
      .mockReturnValueOnce({
        abort: vi.fn(),
        done: new Promise((r) => {
          resolve2 = r;
        }),
      });

    insights.generate();
    insights.generate();

    expect(insights.tasks).toHaveLength(2);

    resolve1(s1);
    await new Promise((r) => setTimeout(r, 0));

    expect(insights.tasks).toHaveLength(1);
    expect(insights.summaries[0]).toEqual(s1);

    resolve2(s2);
    await new Promise((r) => setTimeout(r, 0));

    expect(insights.tasks).toHaveLength(0);
    expect(insights.summaries[0]).toEqual(s2);
  });

  it("sets error on task failure", async () => {
    const mockHandle = {
      abort: vi.fn(),
      done: Promise.reject(new Error("CLI not found")),
    };
    vi.mocked(api.generateSummary).mockReturnValueOnce(
      mockHandle,
    );

    insights.generate();
    await new Promise((r) => setTimeout(r, 0));

    expect(insights.tasks).toHaveLength(1);
    expect(insights.tasks[0]!.status).toBe("error");
    expect(insights.tasks[0]!.error).toBe("CLI not found");
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

    insights.generate();

    // Change date while generation is in flight.
    insights.date = "2025-03-01";

    resolveDone(newSummary);
    await new Promise((r) => setTimeout(r, 0));

    // Should not have prepended -- should have called load.
    expect(api.listSummaries).toHaveBeenCalled();
    expect(insights.selectedId).not.toBe(20);
  });

  it("removes task on abort without error", async () => {
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

    insights.generate();
    await new Promise((r) => setTimeout(r, 0));

    expect(insights.tasks).toHaveLength(0);
  });
});

describe("cancelTask", () => {
  it("aborts a specific task", async () => {
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

    insights.generate();
    const clientId = insights.tasks[0]!.clientId;

    insights.cancelTask(clientId);
    expect(abortFn).toHaveBeenCalled();

    rejectDone(
      new DOMException("Aborted", "AbortError"),
    );
    await new Promise((r) => setTimeout(r, 0));

    expect(insights.tasks).toHaveLength(0);
  });
});

describe("dismissTask", () => {
  it("removes an errored task", async () => {
    const mockHandle = {
      abort: vi.fn(),
      done: Promise.reject(new Error("fail")),
    };
    vi.mocked(api.generateSummary).mockReturnValueOnce(
      mockHandle,
    );

    insights.generate();
    await new Promise((r) => setTimeout(r, 0));

    expect(insights.tasks).toHaveLength(1);
    const clientId = insights.tasks[0]!.clientId;

    insights.dismissTask(clientId);

    expect(insights.tasks).toHaveLength(0);
  });
});

describe("generatingCount", () => {
  it("counts active generating tasks", async () => {
    vi.mocked(api.generateSummary)
      .mockReturnValueOnce({
        abort: vi.fn(),
        done: new Promise(() => {}),
      })
      .mockReturnValueOnce({
        abort: vi.fn(),
        done: Promise.reject(new Error("fail")),
      });

    insights.generate();
    insights.generate();
    await new Promise((r) => setTimeout(r, 0));

    // One still generating, one errored
    expect(insights.generatingCount).toBe(1);
  });
});
